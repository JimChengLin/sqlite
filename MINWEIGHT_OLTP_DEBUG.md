# Minweight OLTP Debug Notes

Generated: 2026-06-04T19:39:14+08:00

Environment: `darwin/arm64`, Go `go1.25.1`, GOMAXPROCS `10`.

Direct core rows: `20000`; SQL bulk sizes: `1000,2000,5000,10000`.

## Direct minweight_store Core

| Check | Ops | Elapsed | Ops/s |
| --- | ---: | ---: | ---: |
| `direct_build_write_batch` | 60000 | 23.316416ms | 2573294 |
| `direct_memory_store_put` | 60000 | 33.162458ms | 1809275 |
| `direct_store_write_batch` | 60000 | 34.332375ms | 1747622 |
| `direct_store_get_hit` | 20000 | 6.134458ms | 3260272 |
| `direct_store_seek_ge` | 20000 | 14.041625ms | 1424337 |

## SQL Adapter Bulk Insert Scale

| Rows | Native elapsed | Native rows/s | Minweight elapsed | Minweight rows/s | Minweight / Native |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 1000 | 3.691375ms | 270902 | 6.316208ms | 158323 | 0.584x |
| 2000 | 12.353083ms | 161903 | 10.843ms | 184451 | 1.139x |
| 5000 | 19.203958ms | 260363 | 27.750792ms | 180175 | 0.692x |
| 10000 | 48.608792ms | 205724 | 56.571333ms | 176768 | 0.859x |

## SQL Bulk Insert Phase Breakdown

Rows: `10000`; `Tx total` excludes schema creation and matches the transaction-shaped bulk insert cost.

| Engine | Schema | Insert loop | Commit | Tx total |
| --- | ---: | ---: | ---: | ---: |
| `native_btree` | 2.070875ms | 31.563792ms | 2.708791ms | 34.272583ms |
| `minweight_store` | 180.417µs | 32.286458ms | 22.19025ms | 54.476708ms |

## SQL Update Phase Breakdown

Rows: `5000`; update ops: `20000`.

| Engine | Seed | Update loop | Commit | Total |
| --- | ---: | ---: | ---: | ---: |
| `native_btree` | 24.347083ms | 64.766833ms | 5.958375ms | 95.072291ms |
| `minweight_store` | 28.600708ms | 63.401875ms | 11.922125ms | 103.924708ms |

## SQL Update Shape Isolation

Rows: `5000`; update ops: `20000`; elapsed excludes schema creation and seed inserts.

| Shape | Native elapsed | Minweight elapsed | Minweight / Native |
| --- | ---: | ---: | ---: |
| `no_index_update_nonindexed` | 23.068ms | 30.866292ms | 0.747x |
| `indexed_table_update_nonindexed` | 25.781375ms | 32.826ms | 0.785x |
| `indexed_table_update_indexed` | 56.494083ms | 72.801291ms | 0.776x |

## Optimization Checkpoint

- Direct `minweight_store` is fast in this workload shape: the path-backed core batch writes 60k table/index-like entries in tens of milliseconds, and point reads/seeks are also in the million ops/s range.
- The previous near-quadratic SQL bulk insert cliff came from transaction overlay seeks scanning every entry in `tx.writes`. The current adapter keeps the exact lookup map but uses an in-memory minweight overlay store for GE/LE cursor movement, so the scale curve is now close to linear.
- The ordered transaction overlay is now lazy: exact-write transactions only update the write map until a range cursor needs ordered movement; after the overlay is built, later writes update it incrementally instead of rebuilding it.
- Monotonic table append and index probes beyond a root's tracked max key return miss/EOF from root stats instead of creating the ordered overlay. In the 50k bulk-insert profile this removed `setWriteOwned -> tx.overlay.Put` from the hot path; the remaining bulk cost is mainly the real commit `WriteBatch` plus SQLite record/key encoding.
- `BtreeInsert` now trusts non-zero SQLite `seekResult` for adjacent-position inserts and skips the extra exact-key probe on that path. Writes that may overwrite an existing key still verify existence.
- Writable table cursors now try exact rowid lookup before `SeekGE`, so primary-key UPDATE avoids paying range seek cost for known hits. Read-tracked cursors keep the older seek path for pinned-reader semantics.
- Commit no longer builds before/after history when no pinned reader can use it; in that common single-connection OLTP shape it publishes the `WriteBatch` and generation directly. Pinned readers still get retained before images.
- Versioned index store keys now use the SQLite-comparable field encoding directly, with the original SQLite record kept as the value. Full-key `BtreeIndexMoveto` probes can use exact `Get`, while prefix/range probes still use `SeekGE` and fall back to `_sqlite3VdbeRecordCompare` when needed.
- Write transactions no longer record every point read into `tx.reads` on the single-writer hot path. The conflict checker remains available for explicit read-set validation, but SQL UPDATE no longer pays a string/map allocation for each base lookup.
- `BtreeInsert` reuses the current cursor key when SQLite is replacing the same row/index entry, and `BtreeDelete` uses a known-existing delete path when the cursor already proves the entry exists. Both avoid redundant committed-store probes during UPDATE.
- Known-existing deletes now keep the owned cursor store key instead of cloning it again before writing the tombstone. This trims allocation/copy cost from SQLite's delete+insert secondary-index UPDATE pattern.
- Index replace keeps the new physical key's transaction `base` state separate from the old key delete. The old key still preserves root row-count accounting, but the new key stays `base=false` when it was only created by the current transaction, so repeated secondary-index updates can fold away intermediate writes instead of retaining false tombstones.
- `minweightComparableMemKey` reads transient SQLite `Mem` bytes as a view while building the final sortable key, avoiding an extra payload copy for BINARY/RTRIM text and blob probe keys.
- Comparable-key builders now append each field into the final key buffer instead of allocating one temporary byte slice per field. This reduced the `BtreeIndexMoveto` probe-key hot path in the indexed-column UPDATE profile.
- Transaction write-map keys now point at the owned write key backing array; savepoint clones and commit-history entries rebind their map key to the cloned key. This removes a hot `string(write.key)` copy without keeping stale backing arrays alive.
- Transaction writes that already know `base=true` skip the previous-write map lookup in `setWriteOwned`; only `base=false` writes need to check whether an earlier write must preserve base provenance. This removes a visible `mapaccess2_faststr` cost from large UPDATE/UPSERT transactions.
- Cursor dispatch still follows the `cursor -> btree -> engine` handle graph, but cursor-bound calls first read `BtCursor.FpBtree` and resolve the btree binding directly before falling back to the cursor map. In the 200k UPDATE diagnostic this dropped the minweight update loop from about 676ms to about 624ms.
- Minweight cursor lookup now stores a 1-based cursor slot id in the raw `BtCursor.FpBt` field. The Go cursor object stays owned by the engine slice/map, so cursor-bound calls can try array lookup before falling back to the cursor map without storing Go pointers in SQLite ABI memory.
- Minweight cursor lookup now uses an RWMutex read path, and the normal no-incrblob table-write path checks an atomic cursor count before locking and scanning cursors.
- Commit builds the final `WriteBatch` with tombstone deletes before puts. The write map still stores one final state per key; delete-first ordering reduced the large UPDATE commit profile by making minweight/minpatricia apply deletes before installing replacement records.
- Current-generation reads now reuse the already-owned value returned by `minweight_store.Get` / seek APIs. Only commit-history before-images are cloned, so table moveto and cursor seek paths avoid an extra payload copy.
- Row decoding now takes ownership of minweight items instead of cloning key/value again. This keeps range cursor movement on owned minweight_store data without materializing another row copy.
- Index replace during `BtreeInsert` uses the current cursor as proof that the old key exists and deletes it through the known-existing path. That removes a committed-store probe from SQLite's indexed-column update pattern.
- Read-range tracking no longer clones seek bounds a second time, and versioned-index range checks use the versioned key prefix directly. Transaction writes keep an exact map for lookup/commit, and only create the ordered overlay when cursor movement cannot be answered from exact lookup or root stats.
- SQL `BtreeInsert` now uses an owned-write path because `KeyBytes` / `DataBytes` already copy SQLite payloads into Go memory; generic write helpers still copy caller-owned slices. Delete metadata updates mutate the active transaction state directly instead of cloning visible metadata.
- Index writes still parse SQLite record bytes into a comparable key on every insert. That is needed for SQLite order compatibility; it is adapter CPU, not minweight_store CPU.
- Bulk insert phase breakdown separates schema, insert loop, and commit. The minweight insert loop is close to native in this diagnostic; the remaining bulk gap is concentrated in transaction commit, where the CPU profile points to `minweight_store.WriteBatch` / `minpatricia` apply rather than transaction overlay writes.

## Rejected Optimization Experiments

- Do not sort pure-Put transaction writes by physical key before building `minweight.Store.WriteBatch`. It has been tested more than once and did not improve the end-to-end OLTP benchmark on this tree. The 2026-06-04 rerun moved minweight `bulk_insert_tx` from the current baseline around `111.9ms` to about `120.7ms`, with `update_by_pk_tx` and `upsert_by_pk_tx` also slower. Revisit only if `minweight_store.WriteBatch` / `minpatricia` apply semantics change.
- Do not prioritize transaction ordered-overlay reuse for bulk insert. The current bulk phase breakdown shows the minweight insert loop close to native; the remaining gap is in commit and profiles as `minweight_store.WriteBatch` / `minpatricia`, not `tx.overlay.Put`.
- Do not reuse `minweightCursor` Go objects with a simple cursor pool. That experiment failed `TestMinweightIncrblobCursorInvalidatedByClearTable`: clear-table changes went from `1` to `0`. Cursor slot lookup is fine; cursor object reuse needs a stricter ownership design before it can be retried.

## Next Fix Direction

- Reduce update/upsert large transaction cost next. Current profile now mostly shows SQLite VDBE execution, runtime allocation/GC, real commit `WriteBatch`, `BtreeInsert`, exact base lookups, and record-to-comparable-key encoding rather than ordered-overlay writes or raw minweight KV cost.
- For bulk insert specifically, the next high-value work is on the minweight_store batch-apply path or a narrower adapter-to-store owned batch API. Reusing the in-memory overlay or changing commit iteration order is not supported by the current profiles.
- Next useful work is to reduce write-set churn for repeated secondary-index updates, avoid redundant exact base lookups during SQLite's delete+insert update pattern, and keep checking that comparable-key-only physical index keys preserve SQLite collation/tie-break behavior across broader SQL cases.
- After the update/upsert path is reduced, rerun the full OLTP benchmark and decide whether the remaining gap is SQLite VDBE/record-encoding overhead or adapter work that can still be removed.
- CPU profile for the minweight SQL bulk insert loop: `/tmp/minweight_sql_bulk_loop.pprof`.
- CPU profile for the minweight SQL bulk insert commit: `/tmp/minweight_sql_bulk_commit.pprof`.
- CPU profile for the minweight SQL update loop: `/tmp/minweight_sql_update_loop.pprof`.
- CPU profile for the minweight SQL update commit: `/tmp/minweight_sql_update_commit.pprof`.
