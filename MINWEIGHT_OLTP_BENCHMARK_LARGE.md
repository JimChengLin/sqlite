# Minweight OLTP Benchmark

Generated: 2026-06-04T19:41:29+08:00

Environment: `darwin/arm64`, Go `go1.25.1`, GOMAXPROCS `10`.

Command shape: `go run ./tools/minweight_oltp_bench -rows 50000 -ops 50000 -runs 3`.

Both engines ran through `database/sql` with the same SQL and a single open connection. Path-backed temp databases were used. Pragmas: `foreign_keys=ON`, `journal_mode=DELETE`, `synchronous=OFF`, `temp_store=MEMORY`, `cache_size=-20000`. Native uses SQLite btree pages; minweight uses `sqlite.NewMinweightStorageEngine()` and path-backed minweight stores.

## Median Results

| Scenario | Ops | Native ops/s | Minweight ops/s | Minweight / Native | Native median | Minweight median |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `bulk_insert_tx` | 50000 | 263402 | 165312 | 0.63x | 189.824ms | 302.459083ms |
| `mixed_small_tx` | 50000 | 16615 | 76563 | 4.61x | 3.009384833s | 653.059834ms |
| `point_select_pk` | 50000 | 83906 | 124994 | 1.49x | 595.905209ms | 400.018917ms |
| `point_select_secondary` | 50000 | 73876 | 99356 | 1.34x | 676.806083ms | 503.243208ms |
| `update_by_pk_tx` | 50000 | 137627 | 96644 | 0.70x | 363.301042ms | 517.363041ms |
| `upsert_by_pk_tx` | 50000 | 126178 | 88616 | 0.70x | 396.265916ms | 564.232083ms |

## Raw Runs

| Engine | Scenario | Run durations |
| --- | --- | --- |
| `minweight_store` | `bulk_insert_tx` | 294.569583ms, 302.459083ms, 308.756666ms |
| `native_btree` | `bulk_insert_tx` | 178.675584ms, 189.824ms, 191.943083ms |
| `minweight_store` | `mixed_small_tx` | 624.3015ms, 653.059834ms, 667.591833ms |
| `native_btree` | `mixed_small_tx` | 2.991363542s, 3.009384833s, 3.487531083s |
| `minweight_store` | `point_select_pk` | 392.279042ms, 400.018917ms, 408.97225ms |
| `native_btree` | `point_select_pk` | 589.405292ms, 595.905209ms, 636.817167ms |
| `minweight_store` | `point_select_secondary` | 500.249542ms, 503.243208ms, 530.598375ms |
| `native_btree` | `point_select_secondary` | 649.065083ms, 676.806083ms, 707.128708ms |
| `minweight_store` | `update_by_pk_tx` | 509.764166ms, 517.363041ms, 546.454833ms |
| `native_btree` | `update_by_pk_tx` | 357.38175ms, 363.301042ms, 387.297625ms |
| `minweight_store` | `upsert_by_pk_tx` | 549.6185ms, 564.232083ms, 569.890042ms |
| `native_btree` | `upsert_by_pk_tx` | 380.748458ms, 396.265916ms, 401.32125ms |

## Notes

- This benchmark measures current adapter behavior, not only the standalone minweight KV core. SQLite parsing, VDBE execution, storage-engine dispatch, key encoding, transaction overlay, and minweight store calls are all included.
- `synchronous=OFF` reduces native fsync cost so the comparison focuses more on btree/minweight execution and less on filesystem durability policy.
- `mixed_small_tx` uses small explicit transactions with 10 primary-key reads, 5 secondary-index reads, 4 updates, and 1 ledger insert per transaction.
- Current minweight wins read-heavy and many-small-transaction shapes because point/range lookups avoid SQLite page btree traversal and commit batching is cheap. Large write transactions are much closer after full-key index probes were moved to exact `Get`, append/index-beyond-max misses started using root stats instead of creating the ordered overlay, SQL insert writes skip duplicate payload/key copies, hot point reads stopped filling `tx.reads`, update cursors reuse known current keys, comparable-key builders append into one buffer, write-map keys reuse owned key backing arrays, base-known writes skip redundant previous-write lookup, index replace keeps the new key's transaction `base` state separate from the old key delete, known-existing deletes keep owned cursor store keys, cursor dispatch resolves raw `BtCursor.FpBtree` before falling back to the cursor map, minweight cursor lookup uses an RWMutex read path and a raw `BtCursor.FpBt` cursor slot before falling back to the cursor map, no-incrblob table writes avoid cursor-map locking, commit applies final tombstones before final puts, current-generation reads reuse owned minweight values, range row decode takes ownership of minweight items, and indexed-column update deletes the old index key through the known-existing path. Update/upsert still trail native because indexed-column writes rebuild SQLite-comparable secondary keys and large commits still pay the real minweight `WriteBatch` cost.
- Debug runs show standalone path-backed `minweight_store` core batch writes tens of thousands of table/index-like entries in tens of milliseconds, with point `Get` and `SeekGE` in the million ops/s range. The remaining write gap is adapter work, not minweight_store raw KV throughput.
- Results are local to this machine and this git tree; rerun the command after storage-engine changes.
