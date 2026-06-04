# Minweight OLTP Benchmark

最后更新：2026-06-04。

这份文档记录当前 minweight storage engine 的端到端 OLTP benchmark。这里测的是 SQLite adapter 整体路径，不是单独的 `minweight_store` KV core：SQL parsing、VDBE、storage-engine dispatch、SQLite record/key 编码、transaction overlay、commit batching 和 minweight store 调用都算在内。

## Benchmark 工具

工具入口：

```sh
go run ./tools/minweight_oltp_bench
```

常用参数：

- `-rows`：每个场景预加载的 `accounts` 行数。
- `-ops`：每个场景执行的逻辑操作数。
- `-runs`：每个 engine/scenario 跑几次。
- `-payload-bytes`：每行 `accounts.payload` 的字节数。用它扩大数据库体积，不需要提高 row count。
- `-seed-batch-size`：preload 每个事务写多少行。大数据库读测试和小事务测试应该设置这个参数，避免 preload 变成一个超大事务。
- `-scenarios`：逗号分隔的场景列表；为空时跑所有场景。

工具会在每个 run 关闭 DB 后统计递归 allocated disk bytes。单个 run 失败会写入报告，不会中断整轮 benchmark，这样 unsupported 场景会直接暴露出来。

当前场景：

- `bulk_insert_tx`：一个大事务插入 `ops` 行。
- `point_select_pk`：preload 后按 INTEGER PRIMARY KEY 点读。
- `point_select_secondary`：preload 后按唯一二级索引点读。
- `update_by_pk_tx`：preload 后一个大事务做 `ops` 次主键 update。
- `upsert_by_pk_tx`：preload 后一个大事务做 `ops` 次 UPSERT。
- `mixed_small_tx`：preload 后跑小显式事务；每个事务包含 10 次主键读、5 次二级索引读、4 次 update、1 次 ledger insert。

两边 engine 都通过 `database/sql` 跑同一份 SQL，单连接，path-backed temp database，并使用：

```sql
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = DELETE;
PRAGMA synchronous = OFF;
PRAGMA temp_store = MEMORY;
PRAGMA cache_size = -20000;
```

`synchronous=OFF` 是刻意的：它降低 native fsync 成本，让比较更集中在 btree/minweight 执行路径，而不是文件系统 durability policy。

## 100K 全场景

命令：

```sh
go run ./tools/minweight_oltp_bench -rows 100000 -ops 100000 -runs 3
```

环境：`darwin/arm64`，Go `go1.25.1`，GOMAXPROCS `10`。

| 场景 | Ops | Native median | Minweight median | Native ops/s | Minweight ops/s | Minweight / Native |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `bulk_insert_tx` | 100000 | 397.367334ms | 667.992958ms | 251656 | 149702 | 0.59x |
| `mixed_small_tx` | 100000 | 8.706159584s | 1.505860584s | 11486 | 66407 | 5.78x |
| `point_select_pk` | 100000 | 1.235136208s | 910.410625ms | 80963 | 109841 | 1.36x |
| `point_select_secondary` | 100000 | 1.437472875s | 1.1156275s | 69567 | 89636 | 1.29x |
| `update_by_pk_tx` | 100000 | 862.453375ms | 1.2567795s | 115948 | 79568 | 0.69x |
| `upsert_by_pk_tx` | 100000 | 918.745334ms | 1.356925459s | 108844 | 73696 | 0.68x |

结论：

- minweight 当前在点读和大量小事务 OLTP 上明显快于 native btree。
- native btree 当前在大单写事务上仍然更快。
- 写入差距主要在 adapter：SQL write 路径、comparable secondary key 编码、base exact lookup、write-set churn 和 commit batching。之前直接测过 `minweight_store` core，它不是当前主要瓶颈。

## 10GB 大数据库

命令：

```sh
go run ./tools/minweight_oltp_bench \
  -rows 100000 \
  -ops 100000 \
  -runs 1 \
  -payload-bytes 100000 \
  -seed-batch-size 1000 \
  -scenarios point_select_pk,point_select_secondary,mixed_small_tx
```

环境：`darwin/arm64`，Go `go1.25.1`，GOMAXPROCS `10`。

这轮刻意去掉了 `bulk_insert_tx`、`update_by_pk_tx`、`upsert_by_pk_tx`。这三个都是大单写事务场景；当前 adapter 不声明支持超大单事务，把它们混进 10GB 读/小事务报告会误导结论。

preload 每 1000 行一个事务。每个 `accounts` 行在未索引的 payload 列里存 `97.66KiB`。

| 场景 | Ops | Native median | Minweight median | Native ops/s | Minweight ops/s | Minweight / Native | Native disk | Minweight disk | Minweight / Native disk |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `point_select_pk` | 100000 | 53.508994333s | 29.260387166s | 1869 | 3418 | 1.83x | 9.35GiB | 9.37GiB | 1.00x |
| `point_select_secondary` | 100000 | 48.389080208s | 29.475236083s | 2067 | 3393 | 1.64x | 9.37GiB | 9.37GiB | 1.00x |
| `mixed_small_tx` | 100000 | 1m18.61439125s | 34.101238083s | 1272 | 2932 | 2.31x | 9.35GiB | 11.20GiB | 1.20x |

结论：

- 到 10GB 级别，minweight 在点读和小事务混合场景仍然快于 native btree。
- 纯读场景磁盘占用基本相同；`mixed_small_tx` 后 minweight 当前约为 native 的 `1.20x`。
- 之前 10GB 跑出 `database or disk is full` 的原因是把 preload/写入做成单个超大事务。改成分批 preload 并去掉大单事务场景后，path-backed minweight store 确实落盘并完成测试。

## 边界

这份 benchmark 不证明以下能力：

- SQLite WAL frame/checkpoint。minweight 当前让 `PRAGMA journal_mode=WAL` 保持 rollback `delete` 模式，也不会创建 fake `-wal` 文件。
- SQLite 物理 page file 兼容。minweight path-backed DB 是 minweight store 目录，不是 SQLite page image。
- 完整 VFS。当前 VFS 路径只是 read-only logical snapshot import。
- 超大单写事务。这个要单独修 adapter transaction/write path 后再测，不能混在常规 OLTP 报告里。

## 日常命令

benchmark 工具改动后的快速 smoke：

```sh
go test ./tools/minweight_oltp_bench
go run ./tools/minweight_oltp_bench \
  -rows 100 \
  -ops 200 \
  -runs 1 \
  -payload-bytes 128 \
  -seed-batch-size 50 \
  -scenarios point_select_pk,point_select_secondary,mixed_small_tx \
  -out /tmp/minweight_oltp_smoke.md
```

当前 new-code lint gate：

```sh
golangci-lint run --new-from-rev HEAD ./tools/minweight_oltp_bench
```

不要把 10GB benchmark 当作每次修改后的常规检查。只有 storage read/write path 有实质变化，或者需要刷新发布数字时再跑。
