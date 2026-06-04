# Minweight 替换 SQLite Btree 介绍

本文说明当前代码如何用 `github.com/JimChengLin/minweight_store` 替换 SQLite 原生 btree。重点是架构边界、KV 映射、事务处理方式，以及目前还没有完全对齐的部分。

## 目标和边界

这个改造不是 virtual table 路线，而是直接替换 SQLite 内部 btree 调用面：

- SQL parser、planner、VDBE、schema 管理、函数、collation、driver 层仍然使用原来的 SQLite/modernc 代码。
- 原来由 SQLite btree/pager 管理的数据页，改为通过 Go `StorageEngine` interface dispatch。
- 默认实现仍然是翻译自 SQLite C 的原生 btree；minweight 实现同一个 interface。
- minweight 当前是逻辑 Btree/KV 引擎，不实现 SQLite 物理 page file、pager cache、真实 WAL frame、mmap 或完整 `sqlite_dbpage` 页面内容。
- 当前 path-backed database 已改为真实落盘的 minweight store；下一层是完成 SQLite btree 语义需要的 transaction view / overlay。WAL 以后只做逻辑事务模式，不做 SQLite WAL frame、shared-memory 或 pager I/O。
- 在 transaction view 落地前，不新增会让用户误解为真实 WAL、VFS 或 page-file 支持的 hook。pager/file fake 只保留为 ABI 占位。

因此当前替换点是“SQLite btree API 语义层”，不是“SQLite 文件格式层”。

## Dispatch 入口

公开入口在 `storage_engine.go`：

- `SetStorageEngine(engine StorageEngine)` 设置当前 btree engine。
- `NewMinweightStorageEngine()` 返回 minweight-backed engine。
- `StorageEngine` 是 `modernc.org/sqlite/lib.StorageEngine` 的别名。

`SetStorageEngine` 只决定新 btree open 的默认 engine。已经打开的 btree handle 会绑定到打开时选中的 engine；driver connection close 会清理 sqlite3* 级别绑定，避免 allocator 复用 sqlite3* 地址后把新连接的 db-level 调用 dispatch 到旧 engine。

真正的 interface 定义在 `lib/storage_engine_api.go`。它覆盖 SQLite btree 调用面，例如：

- 打开/关闭：`BtreeOpen`、`BtreeClose`
- 事务：`BtreeBeginTrans`、`BtreeCommitPhaseOne`、`BtreeCommitPhaseTwo`、`BtreeRollback`、`BtreeSavepoint`
- cursor：`BtreeCursor`、`BtreeFirst`、`BtreeNext`、`BtreeTableMoveto`、`BtreeIndexMoveto`
- 写入：`BtreeInsert`、`BtreeDelete`、`BtreeCreateTable`、`BtreeDropTable`
- meta/状态：`BtreeGetMeta`、`BtreeUpdateMeta`、`BtreeTxnState`、`BtreeLockTable`

各平台的 `lib/storage_engine_<goos>_<goarch>.go` 把原来生成代码里的 `_sqlite3Btree*` 函数改成 dispatch：

```go
return storageEngine().BtreeOpen(...)
```

同时 `nativeBtreeStorageEngine` 实现同一个 interface，并继续调用翻译来的 SQLite btree 函数。所以切换 engine 不需要改 VDBE 上层调用；上层仍然认为自己在调用 SQLite btree。

## Handle 模型

SQLite 生成代码仍然传入 `tls *libc.TLS` 和 `uintptr`。当前 interface 把这些裸参数包成 typed handle：

- `BtreeContext` 保存 TLS。
- `BtreeHandle` 表示 SQLite btree 指针或 minweight token。
- `SQLiteHandle` 表示 owning `sqlite3*`。
- `BtreeCursorHandle` 表示 SQLite cursor 内存。
- `BtreeMemoryHandle`、`BtreeCStringHandle` 等用于读写 SQLite ABI 里的 out 参数和 C string。

minweight 不直接使用 SQLite btree 指针作为真实对象，而是在 `minweightStorageEngine` 内部维护 token map：

- `btrees map[uintptr]*minweightBtree`
- `aliases map[uintptr]uintptr`，用于把 sqlite3 handle 映射回 btree token
- `dbs map[string]*minweightDatabase`，用于缓存当前 engine 已打开的 path-backed minweight store，并用 refcount 管理生命周期
- `cursors map[uintptr]*minweightCursor`

因此对 minweight 来说，`uintptr` 是 handle/token；`tls` 主要用于满足 SQLite ABI 的内存读写、C string、fake pager/file 等场景。

需要特别注意：db-level 调用如 logical backup metadata 只能把 `sqlite3*` 当 connection handle 查找当前 main btree，不能把它当稳定对象地址长期持有。连接关闭时必须删除 sqlite3* -> engine/db alias；btree/cursor 生命周期仍由各自 handle close 管理。

## 运行时对象

minweight 实现在 `lib/minweight_storage_engine.go`。核心对象有三层：

`minweightStorageEngine`

- 全局 engine 状态。
- 管理 btree token、sqlite3 alias、path database、cursor。

`minweightDatabase`

- 共享的逻辑数据库。
- `:memory:`/temp database 用 `minweight.New()` 创建内存 store；path-backed database 用 `minweight.Open(dir, Options...)` 打开真实磁盘 store。
- 持有 committed `store *minweight.Store`，以及 path-backed store 的目录和 refcount。
- 持有 SQLite btree meta、root-page 分配状态、表统计信息、PRAGMA 相关逻辑状态。
- 持有 reader/writer/table-lock 状态。

`minweightBtree`

- 单个 SQLite open btree handle。
- 指向一个 `minweightDatabase`。
- 保存 fake pager/file/journal 指针、SQLite db 指针、readonly/shared-cache 状态。
- 写事务期间保存 per-writer overlay、working metadata、savepoint/statement rollback 状态；后续需要补短生命周期 statement/read-cursor generation pin 和读写集检测，显式长读事务先保持 unsupported。

## 打开数据库

`BtreeOpen` 做几件事：

1. 从 SQLite 传入的 filename 计算 database key。
2. `:memory:` 和空 filename 使用独立的 `minweight.New()` 内存 store。
3. 普通 path-backed filename 被视为 minweight store directory：不存在则由 `minweight.Open(filename, ...)` 创建目录，存在且是目录则重开已有 store。
4. 如果 filename 已存在于 `engine.dbs`，复用同一个已打开 `minweightDatabase` 并增加 refcount；否则 `minweight.Open` 后放入 `engine.dbs`。
5. 分配极薄的 fake `Pager`、`sqlite3_file`、journal file，让 SQLite 上层需要 pager-shaped 指针时能读到少量状态。
6. 分配 btree token，写入 SQLite 的 `ppBtree` out 参数。

path-backed store 关闭时按 refcount 管理：每个 path-backed `BtreeOpen` 增加 refcount，最后一个 `BtreeClose` 从 `engine.dbs` 删除并调用 `store.Close()`，释放 minweight_store 目录锁并触发 minweight_store 的 close checkpoint。

如果 filename 已存在且是普通 SQLite page file，minweight 不会假装能原地写入；`minweight.Open` 会失败并向 SQLite 返回 open/corrupt 类错误。需要导入 SQLite page file 时只能走单独的 read-only snapshot import 路径。

当前没有真实 read-only path open。`mode=ro`、chmod-only 只读打开会 fail fast，直到 minweight_store 提供 read-only store/open-view 语义。

这里的 fake 只应该是 ABI 占位，不应该扩展成重型 pager/page-file 模拟。只要逻辑能力不存在，就应该明确 fail fast 或标为 unsupported，不能用 fake 让用户以为已经支持 SQLite page file、WAL 或 VFS。

## KV 映射

SQLite btree 里每张 table/index 都有一个 root page number。minweight 继续使用这个 root page number 作为逻辑 btree id。`sqlite_schema` 本身是 root page `1`。

### Table btree

普通 rowid table 是 int-key btree：

```text
key   = 't' || root:u32be || sortableRowid:u64be
value = SQLite record payload bytes
```

`sortableRowid` 的编码是：

```text
uint64(rowid) ^ (1 << 63)
```

然后按 big-endian 写入。这样 signed int64 rowid 的字节序和数值排序一致。

### Index / WITHOUT ROWID btree

当前 index 和 non-int-key btree 新写入使用 versioned sortable key：

```text
key   = 'i' || root:u32be || 0x00 || sqliteComparableKey
value = SQLite index record bytes
```

这里的 `SQLite index record bytes` 是 SQLite VDBE 已经编码好的 record，继续作为 value 和 btree payload 返回给 SQLite 上层；`sqliteComparableKey` 是 adapter 根据 `KeyInfo` 生成的物理 KV suffix，目标是让 minweight 的 byte order 等于 SQLite btree order。旧的 raw key 布局 `i || root || SQLite index record bytes` 仍可兼容读取，避免已有 path-backed store 失效。

`WITHOUT ROWID` 表在 SQLite btree 层表现为 non-int-key btree，因此走 index-like 路径。它的主键 record bytes 由 SQLite 上层生成，minweight 只负责存取和调用 SQLite comparator。

当前 `sqliteComparableKey` 底座已支持 SQLite record 的 NULL、INTEGER、REAL、TEXT、BLOB 存储类，以及 `BINARY`、`NOCASE`、`RTRIM`、DESC 排序。NaN 按 SQLite 比较语义编码为 NULL。`KEYINFO_ORDER_BIGNULL`、非 UTF-8 `KeyInfo`、没有 sort-key 能力的自定义 collation 会 fail fast，不能静默退回全表扫描作为正常路径。

当前 non-int-key `BtreeFirst` / `BtreeLast` / `BtreeNext` / `BtreePrevious` 已经能对 versioned key 使用 `SeekGE` / `ReverseScanRange`，并 merge 当前 writer overlay。`BtreeIndexMoveto` 对 versioned root 也会从 `UnpackedRecord` / `TMem` 生成 sortable probe key，用 `SeekGE` 定位，再调用 `_sqlite3VdbeRecordCompare` 验证并返回 SQLite 期待的 compare result。旧 raw index key 仍走 materialized compatibility path；这个路径只能用于迁移期，不能成为长期正常路径。

## Cursor 读路径

`BtreeCursor` 会：

1. 初始化 SQLite 原始 `BtCursor` 内存中上层依赖的少量字段。
2. 创建 `minweightCursor`，记录 root、是否 int-key、是否 writable。
3. 把 cursor handle 放进 `engine.cursors`。

当前读操作大致如下：

- `BtreeFirst` / `BtreeLast` / `BtreeTableMoveto` 以及普通 `BtreeNext` / `BtreePrevious` 已经使用 seek/range API。
- versioned-root `BtreeIndexMoveto` 使用 sortable probe key 和 `SeekGE`，并 merge 当前 writer overlay。
- legacy raw index root 的 `BtreeIndexMoveto` 仍从 minweight store 扫描当前 root prefix，这是迁移期兼容路径。
- int-key table 按 rowid 排序。
- `BtreeIndexMoveto` 的最终 compare result 仍来自 SQLite record comparator，避免 adapter 自己重新定义 SQLite record 比较语义。
- `BtreePayload` / `BtreePayloadFetch` 把当前 row 的 payload 写回 SQLite 期待的内存位置。

cursor 会记录 `dataVer`。写入后 database 的 `dataVer++`，后续 `BtreeNext` / `BtreePrevious` / `BtreeEof` 发现版本变化，会重新加载当前 root 的 rows，并尽量把 cursor 定位回原来的 row 或相邻位置。

这也是临时实现。adapter 不应该把当前 root 甚至整库扫出来 materialize 成 `[]minweightRow`，再用 slice index 假装 cursor。

正确方向是 seek cursor：

```text
cursor:
  root
  lowerBound
  upperBound
  currentKey
  currentValue
  direction
  valid
```

在 minweight_store 目前没有长期 iterator 的情况下，cursor 可以用已有的 `SeekGE`、`SeekLE`、`ScanRange`、`ReverseScanRange` 实现：

- `BtreeFirst`：`SeekGE(rootLowerBound)`，检查 key 是否小于 `rootUpperBound`。
- `BtreeNext`：`SeekGE(nextLexicographicKey(currentKey))`，检查 key 是否仍在 root range 内。
- `BtreeLast`：`SeekLE(rootUpperBoundPrev)`，检查 key 是否大于等于 `rootLowerBound`。
- `BtreePrevious`：`SeekLE(prevLexicographicKey(currentKey))`，检查 key 是否仍在 root range 内。
- 如果用 `ScanRange`，也只能 visit 第一个 item 后立刻停止，不能把整个 range 收集到内存。

minweight_store 未来可以提供长期 iterator；如果有长期 iterator，adapter 可以直接把 iterator 放进 cursor state。但无论有没有长期 iterator，adapter 都不能全表扫出来假装 cursor。

如果另一个 handle 正在写事务中，非 writer handle 的 cursor、row-count、meta 等读路径不应该看见 writer overlay。当前代码仍有 transaction-start snapshot 兼容残留，用来挡住未提交写；这能覆盖基本 committed-view 可见性，但方向不对。

正确方向是 statement/read-cursor pin 一个 committed generation，writer commit 后旧 generation 只在仍有 reader pin 时留在内存里。这样读路径按 key/range 查自己的 pinned view，不复制整库，也不遍历 snapshot items。`snapshot()`、`minweightSnapshotGet()` 这类 `O(DB)` 读放大路径要从普通事务隔离里删掉，只保留给 logical backup/serialize 这类本来就需要全量逻辑快照的功能。

## 写路径

当前写入入口不再直接落到 committed minweight store。写事务开始后：

- `BtreeInsert` / `BtreeDelete` / `BtreePutData` 写入 per-writer overlay。
- `BtreeCreateTable` / `BtreeDropTable` / `BtreeClearTable` / `BtreeUpdateMeta` 修改 writer 的 working metadata。
- savepoint 和 statement rollback 保存/恢复 overlay 与 working metadata。
- `BtreeCommitPhaseTwo` 把 overlay delta 转成一个 `minweight.Store.WriteBatch`，再一次性发布到 committed store；path-backed store 的逻辑 metadata 同批写入内部 meta key。

写完会更新 table row count、min/max rowid、root 分配状态、`dataVer` 等逻辑状态。incremental blob cursor 会在相关 row 被替换、删除、清表或 drop table 时失效。

## 事务和锁

当前 minweight 对齐的是一部分 SQLite btree 可见行为，不是完整 MVCC。目标模型也不应该实现成 SQLite pager/WAL，而应该在 adapter 层做 optimistic transaction view。

写事务开始时，writer 获得单 writer slot，并创建 overlay 与 working metadata。已有 writer 会让新的 writer 返回 `SQLITE_BUSY`，带 busy handler 的连接可以等待。rollback、savepoint rollback 和 statement rollback 只丢弃/恢复 overlay 与 working metadata，不再扫描整库、复制整库或重建 store。

commit 时，`BtreeCommitPhaseOne` 检查其它 reader；有冲突则调用 SQLite busy handler 或返回 `SQLITE_BUSY`。`BtreeCommitPhaseTwo` 用 `minweight.Store.WriteBatch` 发布写集和 path-backed metadata，然后清理 transaction state、locks、savepoint/statement state。

下一步事务模型应改成读写集检测：

- database 维护递增 `generation`，每个 committed key/meta/range 有可校验的版本信息。
- statement reader 或普通 cursor 打开时 pin 当前 generation；关闭 cursor/statement 后 release。旧 generation 只要仍可能被 reader 访问，就作为内存 read view 留住；最后一个 reader release 后清理。
- writer 记录 `readSet`、`rangeReadSet`、`writeSet` 和 working metadata。读自己的写时走 `writeSet + pinned/base generation` merge。
- commit phase one 校验 read set/range read set 从 writer base generation 到当前 generation 没被其它已提交 writer 改动；冲突返回 `SQLITE_BUSY`/`SQLITE_LOCKED` 一类 SQLite 可处理错误，不覆盖 committed store。
- commit phase two 才把 write set 通过 `minweight.Store.WriteBatch` 发布到 minweight_store，并发布新的 in-memory generation。
- 显式长读事务先不支持：不能让用户以为有完整 MVCC。普通 autocommit statement/read cursor 可以有短生命周期 pinned generation；需要跨多个 statement 保持旧 view 的事务，在实现完整 pin/release 之前应 fail fast 或保持 rollback-journal busy 语义。

shared-cache 锁：

- `BtreeLockTable` 用 `tableLocks` 模拟表级 read/write lock。
- read/read 可以共存。
- read/write 或 write/read 冲突返回 `SQLITE_LOCKED_SHAREDCACHE`。
- transaction end 释放该 handle 持有的 table locks。

当前模型能覆盖单连接 rollback、savepoint rollback、statement rollback、基本多连接 committed-view 可见性、部分 shared-cache 锁和 busy 行为。但它仍有明确问题：

- 显式长读事务还没有稳定 read view；在上面的 generation pin 模型落地前，不支持长事务。
- legacy raw index key 的 cursor 仍 materialize root，再用 SQLite comparator 排序/定位。
- legacy raw index key 的 path-backed store 迁移策略还没定；在 cursor 依赖物理 byte order 前必须处理。
- WAL 只能等 stable read view 后做逻辑事务模式，不能假装有 SQLite WAL frame。

因此下一步不是补 pager/VFS/dbpage shim，而是把 read/write set validation、in-memory generation pin、legacy raw key 策略和剩余 whole-root rewrite 补上。

### 后续方向：adapter transaction view，不要求 minweight_store MVCC

`minweight_store` 当前提供的是有序 KV、`WriteBatch`、`SeekGE`、`SeekLE`、`ScanRange`、`ReverseScanRange` 和自己的 WAL replay；它没有对外的 transaction snapshot / MVCC API。因此 SQLite 事务语义应由 sqlite-minweight adapter 负责。目标不是支持任意长事务，而是支持短生命周期 statement/read-cursor view 和写事务提交校验：

```text
db.committed: *minweight.Store
db.generation: u64
db.memoryVersions: pinned committed deltas

read statement/cursor:
  viewGeneration = current generation
  readSet/rangeReadSet = keys/ranges actually read when needed for validation
  release on cursor/statement close

write transaction:
  baseGeneration = generation captured at BEGIN IMMEDIATE/EXCLUSIVE or first write
  readSet        = keys/meta read by this writer
  rangeReadSet   = root/key ranges read by this writer when range stability matters
  writeSet       = ordered overlay / write log
  metaDelta      = working metadata / root allocation / table stats
```

规则如下：

- `BtreeInsert`、`BtreeDelete`、`BtreeCreateTable`、`BtreeDropTable`、`BtreeUpdateMeta` 不直接写 `db.committed`，只写当前 writer 的 `delta` 和 working metadata。
- 当前 writer 自己读取时，`Get` 和 range seek merge `baseGeneration + writeSet`；writeSet 里的 put 覆盖 base，delete 隐藏 base，同时记录 read set/range read set。
- 其它连接读取自己的 pinned statement/cursor generation 或当前 committed generation，看不到未提交写。
- `BtreeBeginStmt` 和 `BtreeSavepoint` 记录 delta/working metadata 的轻量事务状态；`ROLLBACK TO` 恢复该状态；整事务 `ROLLBACK` 直接丢弃 delta。
- `BtreeCommitPhaseOne` 校验 writer 的 read set/range read set。若 baseGeneration 之后被冲突写改过，调用 busy handler 或返回 `SQLITE_BUSY`/`SQLITE_LOCKED`；不能覆盖 committed store。
- `BtreeCommitPhaseTwo` 把 writeSet 转成 `minweight.Store.WriteBatch`，一次性提交到 `db.committed`，发布新 generation，并把旧 generation 留在内存里直到所有可能访问它的 reader release。

这样 rollback 成本是 `O(本事务写集)`，不是 `O(整个 DB)`；隔离靠 adapter overlay 和锁协议保证，不需要 minweight_store 提供多版本。

WAL 语义不是 pager hook，而是 transaction view 的策略变化。只有当短 reader pin 和旧 generation 内存保留已经稳定，且明确支持对应生命周期时，才可以让 writer commit 不等待旧 reader。没有这个能力时，`PRAGMA journal_mode=WAL` 应 fail fast 或保持 delete 模式，不能只让 `_sqlite3PagerWalSupported` 返回 true。

如果 minweight_store 后续提供 immutable snapshot / COW view，adapter 可以直接引用它；在此之前，adapter 需要自己维护 read view 边界，并继续保证未提交 overlay 不进入 committed read path。

## 重新规划

下一步不再优先补 pager/VFS/dbpage 表面兼容，而是按下面顺序推进 btree 语义：

1. 已完成：path-backed persistence 从 `minweight.New()` + placeholder file 改为 `minweight.Open(filename, options...)`；`BtreeClose` 在最后一个 handle 关闭时从 `engine.dbs` 删除并调用 `store.Close()`。测试覆盖 close/reopen 后数据仍在，且换一个 `NewMinweightStorageEngine()` 后仍能读到同一路径数据。
2. 已完成：清理 WAL 半支持。未验证的 `StorageEnginePagerWalSupport` / `_sqlite3PagerWalSupported` hook 已移除；`SQLITE_FCNTL_PERSIST_WAL` 这类文件名占位只能叫 placeholder，不代表 WAL 支持。
3. 已完成：修复 sqlite3* 复用后的 stale engine binding。连接关闭会清理 db-level binding；旧连接仍通过已经打开的 btree/cursor handle dispatch，新连接不会继承旧 engine 的 db alias。
4. 已完成：transaction overlay + WriteBatch commit。写入口写 per-writer delta 和 working metadata；commit phase two 用 `minweight.Store.WriteBatch` 批量落到 committed store；rollback/savepoint/statement rollback 恢复或丢弃 overlay，不再扫描/重放整库。
5. adapter view 接口：读路径已经不再用 writer whole-store snapshot 隐藏未提交写；int-key table cursor movement、versioned non-int-key sequential movement、versioned-root `BtreeIndexMoveto` 已经只依赖 `Get`/`SeekGE`/`SeekLE`/`ReverseScanRange` 和 overlay。下一步要把剩余 snapshot read path 换成 generation pin + read/write set validation；显式长读事务先不支持。
6. 部分完成：seek cursor。int-key table cursor 的 `First`/`Last`/`TableMoveto`/`Next`/`Previous` 已经从 materialized root rows 迁到 `SeekGE`/`SeekLE`；non-int-key 顺序 cursor movement 已经迁到 versioned key 的 `SeekGE`/`ReverseScanRange` 并 merge overlay；versioned-root `BtreeIndexMoveto` 已经迁到 sortable probe `SeekGE`。legacy raw index root 仍会 materialize。
7. 已完成底座：`sqliteComparableKey` 为 index / WITHOUT ROWID btree 生成 versioned physical key。内置存储类、内置 collation 和 DESC 先支持；无法编码的自定义 collation、BIGNULL 和非 UTF-8 KeyInfo fail fast。
8. 部分完成：optimistic transaction view。当前已有 committed generation、短 reader-view pin、旧 generation key-change retention/prune、writer point read set、commit 冲突校验和 direct adapter tests。还缺 range read set、SQL statement/cursor 生命周期 pin、冲突 busy-handler 集成，以及把普通读可见性从 transaction-start snapshot 完整迁到 generation view。显式长读事务在这个能力完整前 fail fast 或保持 rollback-journal busy 语义。
9. legacy raw key 策略：versioned-root `BtreeIndexMoveto` 已经基于 `sqliteComparableKey` probe `SeekGE`；旧 raw key root 必须补迁移或 fail-fast 策略，不能静默退回成长期常规路径。
10. 在 transaction view 之后再做 WAL 逻辑模式：`PRAGMA journal_mode=WAL` 只改变 reader/writer commit policy 和 view 生命周期，不创建真实 WAL frame。没有稳定旧 view 时返回 delete 或 unsupported。
11. 最后补逻辑 backup/serialize、constraints/triggers/incremental blob 和 shared-cache 边界测试；page-image、VFS、mmap、dbpage 继续留在低优先级 shim。

## 逻辑备份和序列化

native engine 的 `Serialize` / `Deserialize` 使用 SQLite page image。minweight 没有真实 page image，因此走逻辑 snapshot：

- schema SQL 和 row data 通过 SQL 层重放。
- root page、`nextRoot`、`freeRoots` 等隐藏分配状态被额外保存，避免 round-trip 后 root 分配顺序漂移。
- backup/restore 复用同一套逻辑 replay 思路。

这能保持 SQL 语义和 rootpage 可见状态，但不是 SQLite 文件格式序列化。

## Fake / Shim 清单和原则

SQLite 上层仍有一些地方会碰 pager/file-shaped 状态。minweight 当前存在几类 fake/shim，必须按轻重和语义边界区分。

### 轻量 ABI 占位

- fake `Pager` / fake `sqlite3_file` / fake journal file：只保存 SQLite 上层会读取的少量字段，例如 page size、readonly、data version、filename、journal name。
- readonly 标记：用于 `sqlite3_db_readonly` 和写事务拒绝。
- `BtreePager` 返回 fake pager pointer：只允许上层读取极少量状态，不能让 pager 路径真的执行 page cache、journal、WAL 或 mmap I/O。

这类 fake 是为了让 btree interface 能接住 SQLite ABI，成本很小，可以保留。但它们不能继续膨胀成 pager 的替代实现。

### 低价值可见状态 shim

- `PRAGMA page_size`、reserve bytes、secure_delete、auto_vacuum、max_page_count、cache_size、cache_spill：只是逻辑状态或可见状态。
- `PRAGMA mmap_size`：通过 fake file control 记录 advisory value，不实现 mmap。
- `BtreeSetPagerFlags`：只同步 fake pager 上的 flags，不实现 sync、journal 或 cache spill 行为。

这些 shim 只能用于减少已有测试或上层代码的意外，不应该继续加重。新增时必须说明它没有真实存储语义。

### 容易误导的 shim

- `SQLITE_FCNTL_PERSIST_WAL` / `-wal` 占位文件：当前只是文件名层面的占位和清理策略，不代表真实 WAL。
- `sqlite_dbpage` 逻辑 page 1 header：只避免 dbpage 读路径直接解 fake pager，不代表完整 SQLite page image。
- read-only VFS snapshot import：当前不是 minweight VFS 支持，而是短暂切回 native btree，把 VFS-backed SQLite page file 序列化成逻辑 snapshot，再 replay 到 minweight，并标记 readonly。

这些 shim 都容易误导用户。文档、测试名和错误信息不能把它们命名成“支持 WAL”、“支持 sqlite_dbpage”或“支持 custom VFS”。更准确的说法是：

- `read-only VFS snapshot import`
- `logical dbpage header compatibility`
- `WAL placeholder cleanup`

不支持的能力应明确 fail fast：

- writable custom VFS
- VFS I/O backed minweight store
- valid WAL frames
- mmap-backed reads/writes
- full multi-page or writable `sqlite_dbpage`

总体原则：轻量 ABI 占位可以保留；没有逻辑语义的重型 fake 不做。宁可返回 unsupported，也不要让用户误以为 minweight 已经支持 SQLite pager/VFS/WAL。

## 测试入口

minweight 测试通过环境变量安装 engine：

```sh
SQLITE_TEST_STORAGE_ENGINE=minweight go test ...
```

常用脚本：

```sh
./test-minweight-storage-engine.sh
./test-minweight-broad.sh
./test-minweight-full.sh
TEST_PARALLEL=8 ./test-storage-engine.sh
```

当前新增的高价值覆盖：

- `ATTACH` 多数据库 rollback、commit、join、`DETACH` 和 attached path 重开。
- `WITHOUT ROWID` 复合主键的 ordered scan、point lookup、update、delete、隐藏 rowid 拒绝和 `integrity_check`。
- non-int-key btree overwrite：`WITHOUT ROWID` 更新非主键列时，旧 record key 必须被删除，新 record 作为同一逻辑行覆盖写入，不能变成重复 key。
- 多连接 committed-view 可见性：writer 未提交的 update/insert 对其它连接不可见，commit 后可见，rollback 后保持不可见。
- 单 writer 协议：已有 writer 未结束时，第二个 writer 返回 `SQLITE_BUSY`，不能覆盖 active writer 或 direct-write 到 committed store。
- busy handler：第二个 writer 设置 `PRAGMA busy_timeout` 后，会通过 SQLite busy handler 等待 active writer 释放；超时前释放则写入成功。
- statement reader 边界：普通 `SELECT` rows 未关闭时，writer commit 返回 `SQLITE_BUSY`；失败 commit 后写集不能漏出，关闭 rows 后 writer 可继续提交。
- path-backed minweight store close/reopen 持久化，且新 engine 进程内状态为空时仍能读取旧数据、schema、index lookup 和 `PRAGMA user_version`。
- read-only path open fail-fast：`mode=ro` 不再通过 placeholder 冒充支持。
- sortable index key adapter 单测：覆盖 SQLite storage class 顺序、INTEGER/REAL 数值编码、TEXT/BLOB 分界、`BINARY`/`NOCASE`/`RTRIM`、DESC、versioned store key 解码，以及 unsupported custom collation fail-fast。
- non-int-key sequential cursor seek：lib 级测试覆盖 versioned index cursor 的 `First`/`Next`/`Last`/`Previous`，并验证 writer overlay 的插入/删除会被 seek 路径正确合并，cursor 不 materialize 整个 root。
- versioned `BtreeIndexMoveto` probe seek：lib 级测试覆盖 `UnpackedRecord` prefix seek、`default_rc` skip-prefix 行为、完整 key equality、DESC index、writer overlay merge、delete 覆盖，以及 cursor 不 materialize 整个 root。

当前应优先补高价值语义测试：

- writer overlay 未提交不可见
- commit phase two 后新 reader 可见
- reader 活跃时 writer commit 返回 busy 或走 busy handler
- rollback / savepoint / statement rollback 只回滚本事务写集
- index ordering/lookup/collation
- backup/restore/serialize round-trip
- constraints、triggers、incremental blob

低价值 shim 测试，例如只验证 no-op PRAGMA 可见状态，应放在较低优先级。

## 当前结论

目前已经完成的是：SQLite btree API 可以 dispatch 到 minweight，handle 绑定不再依赖进程级全局开关，path-backed database 会真实打开 minweight_store 目录并在最后一个 handle 关闭时 `Store.Close()`，逻辑 metadata 也会随 store 持久化；competing writer 和 open statement reader 会让 writer 返回 `SQLITE_BUSY`，busy handler 可等待 active writer 释放，不会覆盖 active writer 或漏出失败 commit 的写集；index/WITHOUT ROWID 新写入已经使用 versioned `sqliteComparableKey` 物理 key，value 保留原始 SQLite record；non-int-key sequential cursor movement 和 versioned-root `BtreeIndexMoveto` 已经用 seek/range API。

目前没有完成的是：legacy raw index root 迁移/fail-fast 策略、range read set 和 SQL 生命周期级 generation pin、完整 reader/writer lock protocol、物理 page file、真实 WAL、mmap、writable VFS，以及完整 `sqlite_dbpage` 页面模型。

下一步如果目标是“行为对齐 btree”，第一优先级转为 optimistic transaction view 和 legacy raw index root 策略：versioned 新写入路径已经能 seek，剩下要避免旧 raw key path 继续作为隐藏的全 root materialization 正常路径，并把剩余 snapshot 隔离改成 generation pin + read/write set validation。
