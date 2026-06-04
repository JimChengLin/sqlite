# 用 Minweight Store 替换 SQLite Btree

最后更新：2026-06-04。

这份文档说明当前分支如何用 `github.com/JimChengLin/minweight_store` 替换 SQLite 原生 btree。关键边界是：这是 SQLite btree API 层替换，不是 SQLite page file 替换。SQLite parser、planner、VDBE、schema、collation、function、hook 和 `database/sql` driver 仍然走现有 modernc SQLite 代码；替换发生在 `_sqlite3Btree*` 调用面下面。

## 当前范围

已经确定的方向：

- native SQLite btree 和 minweight 都实现同一个 Go `StorageEngine` interface。
- 生成出来的 SQLite btree entrypoint 会 dispatch 到当前 btree handle 绑定的 engine。
- path-backed minweight database 使用真实 `minweight.Open(...)` store directory，KV 数据和逻辑 btree metadata 都会持久化。
- table/index row 按 SQLite root page number 映射到逻辑 KV。
- 写事务走 adapter transaction overlay，commit 阶段用 `minweight.Store.WriteBatch` 发布。
- 正常 cursor movement 走 minweight seek/range API，不再 materialize whole root。

当前明确不做或还不声明支持：

- 不兼容 SQLite 物理 page image。
- 不实现 SQLite WAL frame、wal-index、checkpoint、shared-memory。
- 不实现 minweight 的 writable/native VFS I/O。
- 暂不声明支持超大单写事务。
- 不保留未发布过的 legacy raw index key fallback。

## StorageEngine 入口

包级公开入口在 `storage_engine.go`：

- `SetStorageEngine(engine StorageEngine)`
- `NewMinweightStorageEngine()`
- `StorageEngine` 是 `modernc.org/sqlite/lib.StorageEngine` 的别名。

核心 interface 在 `lib/storage_engine_api.go`：

- `type StorageEngine interface`
- `type nativeBtreeStorageEngine struct{}`
- `func SetStorageEngine(engine StorageEngine)`

各平台 wrapper 在：

- `lib/storage_engine_<goos>_<goarch>.go`

这些 wrapper 把原来的 generated btree call 改成 engine dispatch。例如 `_sqlite3BtreeOpen(...)` 会变成：

```go
return storageEngine().BtreeOpen(
    btreeContext(tls),
    btreeVFSHandle(tls, pVfs),
    btreeCStringHandle(tls, zFilename),
    sqliteHandle(tls, db),
    btreeMemoryHandle(tls, ppBtree),
    flags,
    vfsFlags,
)
```

native engine 实现同一个方法，然后调用重命名后的翻译版 SQLite btree 函数。minweight engine 实现同一个方法，但下面走逻辑 KV。

这样 VDBE 和大部分翻译出来的 SQLite 上层代码不用理解 minweight；它们仍然认为自己在调用 btree。

## Engine 选择和 Handle 绑定

`SetStorageEngine` 只决定后续新 open 的默认 engine，不能决定已经打开的 btree handle 怎么 dispatch。

dispatcher 维护一个 handle graph：

```text
btree  -> {engine, sqlite3*}
cursor -> btree
db     -> {engine, refs}
```

效果：

- btree handle 永远 dispatch 到 `BtreeOpen` 时选中的 engine。
- cursor call 通过 `cursor -> btree -> engine` 找实现。
- sqlite3-level call 通过 sqlite3* binding 找实现。
- connection close 会清理 sqlite3* binding，避免 allocator 复用地址后把新连接 dispatch 到旧 engine。
- 全局 `SetStorageEngine` 切换只影响之后打开的新 btree。

这也是为什么 `_sqlite3BtreeEnter(tls *libc.TLS, p uintptr)` 这类 ABI 入口仍然可用。对 minweight 来说，`uintptr` 不必是真正的 SQLite `BtShared` 指针；它是 handle/token。dispatcher 用 token 找 engine，minweight engine 再用 token 找 Go object。

`tls *libc.TLS` 仍然有用，但不是存储状态。它主要用于：

- 读取 SQLite C string；
- 写 SQLite out 参数；
- 读写 generated SQLite struct，比如 `BtCursor`；
- 分配或暴露 SQLite 上层期待的 fake pager/file 指针。

## Minweight 运行时对象

主要实现文件：

- `lib/minweight_storage_engine.go`
- `lib/minweight_storage_engine_cursor.go`
- `lib/minweight_storage_engine_cursor_lifecycle.go`
- `lib/minweight_storage_engine_generation.go`
- `lib/minweight_storage_engine_roots.go`
- `lib/minweight_storage_engine_copy.go`
- `lib/minweight_storage_engine_integrity.go`

核心对象：

```text
minweightStorageEngine
  btrees:  token -> minweightBtree
  cursors: token/slot -> minweightCursor
  dbs:     path key -> minweightDatabase

minweightDatabase
  committed minweight.Store
  path/refcount/lifecycle
  logical btree metadata
  root allocation state
  table stats
  generation/read-view state
  writer and table-lock state

minweightBtree
  one SQLite btree handle
  pointer to minweightDatabase
  fake pager/file fields
  transaction overlay and working metadata
  savepoint/statement rollback state

minweightCursor
  one SQLite cursor handle
  root, int-key/non-int-key shape
  current store key/value
  seek anchor/state
  incrblob and stale-cursor metadata
```

## 打开数据库

`BtreeOpen` 根据 SQLite filename 决定 backing store：

- `:memory:` 和 temp database 使用 `minweight.New()`。
- path-backed database 把 SQLite filename 当作 minweight store directory，用 `minweight.Open(...)` 打开。
- engine 用 `engine.dbs` 缓存当前打开的 path-backed store，并按每个 `BtreeOpen` 增加 refcount。
- 最后一个 `BtreeClose` 会从缓存删除 database，并调用 `Store.Close()`。

所以 path-backed minweight database 是 minweight store directory，不是 SQLite page file placeholder。逻辑 metadata 存在一个内部 minweight key 里，包含 root allocation、btree meta、table/index root kind、page-size 这类逻辑状态。

当前 read-only path open 是 fail fast。`mode=ro` 和 chmod-only readonly 不算支持，直到 minweight_store 提供真实 read-only open/view 语义。

已有 SQLite page file 不能直接当 writable minweight DB 打开。当前 VFS 路径只是 read-only logical snapshot import：先用 native btree 读 VFS-backed page file，逻辑 serialize，再 replay 到 minweight，并把 minweight handle 标成 readonly。

## KV 映射

SQLite btree 用 root page number 标识每棵 table/index btree。minweight 继续把 root page number 当逻辑 root id。

`sqlite_schema` 是 root page `1`。

### Rowid table

普通 rowid table 是 int-key btree：

```text
key   = 't' || root:u32be || sortableRowid:u64be
value = SQLite record payload bytes
```

`sortableRowid` 编码：

```text
uint64(rowid) ^ (1 << 63)
```

然后 big-endian 写入。这样字节序和 signed int64 rowid 排序一致。

### Index / WITHOUT ROWID

index 和 non-int-key btree 使用 versioned sortable key：

```text
key   = 'i' || root:u32be || 0x00 || sqliteComparableKey
value = SQLite index record bytes
```

value 保留 SQLite 原始 record bytes，继续作为 SQLite btree payload 返回给上层。

`sqliteComparableKey` 是 adapter 按 `KeyInfo` 生成的物理 key suffix，目标是让 minweight byte order 等于 SQLite btree order。

当前支持：

- SQLite storage class：NULL、INTEGER、REAL、TEXT、BLOB。
- collation：`BINARY`、`NOCASE`、`RTRIM`。
- ASC 和 DESC。

当前 fail-fast：

- unsupported custom collation；
- non-UTF-8 `KeyInfo`；
- `KEYINFO_ORDER_BIGNULL`；
- 未发布过的 legacy raw index key。

旧 raw 格式：

```text
'i' || root:u32be || SQLite index record bytes
```

已经明确不支持。这个项目还没发布 minweight 格式，没有兼容压力；保留 fallback 会重新引入全 root scan/sort，方向不对。

## Cursor 模型

minweight cursor 是 seek cursor，不是 materialized root。

正常 movement：

- `BtreeFirst`：seek 到 root lower bound。
- `BtreeLast`：reverse seek 到 root upper bound。
- `BtreeTableMoveto`：table key exact lookup 或 `SeekGE`。
- `BtreeIndexMoveto`：生成 sortable probe key，用 `SeekGE` 定位，再用 SQLite 自己的 `_sqlite3VdbeRecordCompare` 验证。
- `BtreeNext`：从 current key 后面继续 seek。
- `BtreePrevious`：从 current key 前面 reverse seek。

seek path 会 merge：

- committed minweight store；
- pinned reader 需要的 retained old-generation value；
- 当前 writer 自己的 overlay。

`loadRows()` / `refreshCursorRows()` 已经从正常 cursor movement 中删除。cursor 如果缺少 versioned store-key 或 last-row anchor，会 fail fast 成 corrupt，而不是扫描整棵 root。

这条规则很重要：把 root 扫进 `[]minweightRow` 再用 slice index 假装 cursor，本质上把 minweight ordered KV 优势又吃掉了。

## 事务模型

native SQLite btree 直接写 pager 管理的 page。minweight 走 adapter transaction overlay。

写事务流程：

1. `BtreeBeginTrans(... wrflag != 0)` 抢 single writer。
2. `BtreeInsert`、`BtreeDelete`、`BtreePutData`、root allocation、metadata write 都写 per-writer state。
3. exact lookup 走 write map。
4. range/cursor movement 只有需要时才懒创建 in-memory ordered overlay。
5. statement rollback / savepoint rollback 恢复 write map、overlay、working metadata。
6. `BtreeCommitPhaseTwo` 把最终 write map 转成一个 `minweight.Store.WriteBatch`。
7. commit 发布 KV、logical metadata、root stats 和新的 committed generation。

这个模型替掉了旧的 direct-write-plus-whole-store-snapshot。rollback 不再复制或 replay 整个 committed store。

reader 模型：

- 普通 autocommit read cursor pin 一个短生命周期 committed generation。
- writer 可以在这些短 reader 打开时 commit。
- 只要 reader 还可能访问旧 generation，旧 value 会留在内存里。
- 显式长读事务当前仍不支持，可能让 writer commit 返回 `SQLITE_BUSY`。
- 还不声明 WAL-like 长 reader 语义。

已经有的 optimistic foundation：

- writer point read set；
- seek path 的 bounded range read set；
- write set；
- committed generation number；
- live old generation 的 before/after image；
- direct adapter check 里 stale writer snapshot 返回 `SQLITE_BUSY_SNAPSHOT`。

当前 single-writer SQL 边界下，不是所有 conflict shape 都能通过自然 SQL 跑出来。不要在显式 transaction view 生命周期完成前宣传完整 MVCC。

## Metadata、Root 和 Logical 功能

minweight 维护 SQLite SQL 语义需要的逻辑 btree 状态：

- schema/user version 等 btree meta；
- root page allocation 和 free root reuse；
- table/index root kind；
- row count、min/max rowid stats；
- page size、reserve、auto-vacuum、secure-delete 等逻辑设置；
- fake pager/file 指针；
- logical serialize/deserialize 和 logical backup。

root maintenance 对 int-key 和 versioned non-int-key root 使用 range/seek path。clear、drop、root move、copy-file、integrity check、cursor restore 不再走 whole-root materialization。

logical backup / serialize 不是 SQLite page-image backup。它们是逻辑 schema/data 备份，目前已覆盖 `sqlite_sequence`、generated column、普通 rowid、FTS5 virtual table、root reuse metadata 等关键 case。

## 不支持项和 Shim 边界

这些边界要持续写清楚：

- WAL 禁用。minweight 下 `PRAGMA journal_mode=WAL` 保持 rollback `delete`，不能创建 fake `-wal`。
- VFS 没有实现。当前只是 read-only snapshot import。
- `sqlite_dbpage` 只有 logical page-1/header shim，不是真实多页 DB image。
- mmap/cache/spill/persist-WAL 只是 visible-state shim，不是 pager 功能。
- path-backed minweight DB 是 minweight store directory，不是 SQLite page file。
- read-only path-backed minweight open 现在 fail fast。
- 超大单写事务是已知 adapter 限制，修好前不要混入常规 OLTP 报告。
- unknown btree/cursor handle 应该返回 SQLite error；panic path 是技术债。
- error mapping 还需要从 generic `SQLITE_ERROR` 细化到 IOERR/BUSY/CORRUPT 等。

## 后续优先级

高价值方向：

1. 继续优化 SQL 写路径，优先级高于继续堆 PRAGMA shim。
2. 大单事务写测试和读/小事务 benchmark 分开。
3. 显式长读事务要么稳定 fail fast，要么完成 generation-pinned lifecycle。
4. 降低 comparable secondary key、base exact lookup、write-set churn、commit batching 的 adapter 成本。
5. 继续按 ownership boundary 拆 `lib/minweight_storage_engine*.go` 的大函数/大文件。
6. legacy raw index key 保持删除；除非已经发布格式需要迁移，否则不要加 fallback。
7. WAL/VFS/page-image 不做假支持，实现前保持 unsupported。

推荐快速检查：

```sh
TEST_PARALLEL=8 ./test-minweight.sh quick
```

storage 语义有明显变化时：

```sh
TEST_PARALLEL=8 ./test-minweight-storage-engine.sh
```

benchmark 工具变化时：

```sh
go test ./tools/minweight_oltp_bench
golangci-lint run --new-from-rev HEAD ./tools/minweight_oltp_bench
```

需要刷新 benchmark 数字时看 `MINWEIGHT_OLTP_BENCHMARK.md`。10GB benchmark 不作为每轮常规测试，只在 read/write path 有实质变化或需要刷新报告时跑。
