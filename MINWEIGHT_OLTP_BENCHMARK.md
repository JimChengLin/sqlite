# Minweight OLTP Benchmark

Generated: 2026-06-04T19:39:23+08:00

Environment: `darwin/arm64`, Go `go1.25.1`, GOMAXPROCS `10`.

Command shape: `go run ./tools/minweight_oltp_bench -rows 5000 -ops 20000 -runs 3`.

Both engines ran through `database/sql` with the same SQL and a single open connection. Path-backed temp databases were used. Pragmas: `foreign_keys=ON`, `journal_mode=DELETE`, `synchronous=OFF`, `temp_store=MEMORY`, `cache_size=-20000`. Native uses SQLite btree pages; minweight uses `sqlite.NewMinweightStorageEngine()` and path-backed minweight stores.

## Median Results

| Scenario | Ops | Native ops/s | Minweight ops/s | Minweight / Native | Native median | Minweight median |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `bulk_insert_tx` | 20000 | 237750 | 170553 | 0.72x | 84.122ms | 117.265292ms |
| `mixed_small_tx` | 20000 | 11548 | 111676 | 9.67x | 1.73196625s | 179.089625ms |
| `point_select_pk` | 20000 | 102798 | 266986 | 2.60x | 194.557041ms | 74.910167ms |
| `point_select_secondary` | 20000 | 91894 | 162842 | 1.77x | 217.642167ms | 122.818584ms |
| `update_by_pk_tx` | 20000 | 199556 | 199839 | 1.00x | 100.222416ms | 100.080334ms |
| `upsert_by_pk_tx` | 20000 | 175514 | 170337 | 0.97x | 113.950916ms | 117.414042ms |

## Raw Runs

| Engine | Scenario | Run durations |
| --- | --- | --- |
| `minweight_store` | `bulk_insert_tx` | 115.619167ms, 117.265292ms, 146.23475ms |
| `native_btree` | `bulk_insert_tx` | 79.397292ms, 84.122ms, 96.809875ms |
| `minweight_store` | `mixed_small_tx` | 178.491959ms, 179.089625ms, 182.030209ms |
| `native_btree` | `mixed_small_tx` | 1.487913125s, 1.73196625s, 2.900006666s |
| `minweight_store` | `point_select_pk` | 74.145333ms, 74.910167ms, 85.229542ms |
| `native_btree` | `point_select_pk` | 183.077917ms, 194.557041ms, 206.590208ms |
| `minweight_store` | `point_select_secondary` | 106.87675ms, 122.818584ms, 122.926ms |
| `native_btree` | `point_select_secondary` | 214.554042ms, 217.642167ms, 224.389791ms |
| `minweight_store` | `update_by_pk_tx` | 97.391167ms, 100.080334ms, 114.156459ms |
| `native_btree` | `update_by_pk_tx` | 94.492084ms, 100.222416ms, 107.60975ms |
| `minweight_store` | `upsert_by_pk_tx` | 115.2795ms, 117.414042ms, 124.944042ms |
| `native_btree` | `upsert_by_pk_tx` | 83.781459ms, 113.950916ms, 116.158667ms |

## Notes

- This benchmark measures current adapter behavior, not only the standalone minweight KV core. SQLite parsing, VDBE execution, storage-engine dispatch, key encoding, transaction overlay, and minweight store calls are all included.
- `synchronous=OFF` reduces native fsync cost so the comparison focuses more on btree/minweight execution and less on filesystem durability policy.
- `mixed_small_tx` uses small explicit transactions with 10 primary-key reads, 5 secondary-index reads, 4 updates, and 1 ledger insert per transaction.
- Current minweight wins read-heavy and many-small-transaction shapes because point/range lookups avoid SQLite page btree traversal and commit batching is cheap. Large write transactions are much closer after full-key index probes were moved to exact `Get`, append/index-beyond-max misses started using root stats instead of creating the ordered overlay, SQL insert writes skip duplicate payload/key copies, hot point reads stopped filling `tx.reads`, update cursors reuse known current keys, comparable-key builders append into one buffer, write-map keys reuse owned key backing arrays, base-known writes skip redundant previous-write lookup, index replace keeps the new key's transaction `base` state separate from the old key delete, known-existing deletes keep owned cursor store keys, cursor dispatch resolves raw `BtCursor.FpBtree` before falling back to the cursor map, minweight cursor lookup uses an RWMutex read path, no-incrblob table writes avoid cursor-map locking, commit applies final tombstones before final puts, current-generation reads reuse owned minweight values, range row decode takes ownership of minweight items, and indexed-column update deletes the old index key through the known-existing path. Update/upsert still trail native because indexed-column writes rebuild SQLite-comparable secondary keys and large commits still pay the real minweight `WriteBatch` cost.
- Debug runs show standalone path-backed `minweight_store` core batch writes tens of thousands of table/index-like entries in tens of milliseconds, with point `Get` and `SeekGE` in the million ops/s range. The remaining write gap is adapter work, not minweight_store raw KV throughput.
- Results are local to this machine and this git tree; rerun the command after storage-engine changes.
