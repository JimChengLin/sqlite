# Minweight Storage Engine Progress

Last updated: 2026-06-04.

## Current Status

- Storage engine dispatch enters through the Go `StorageEngine` interface. The native implementation still delegates to the translated SQLite btree code, and minweight uses `github.com/JimChengLin/minweight_store`.
- `SQLITE_TEST_STORAGE_ENGINE=minweight` installs the minweight engine in `TestMain`.
- Table rows are stored as `t || root:u32be || sortableRowid:u64be -> recordPayload`.
- Index entries are stored as `i || root:u32be || sqliteIndexRecordBytes -> sqliteIndexRecordBytes`.
- Non-integer btrees are ordered with SQLite's own record comparator through `KeyInfo`; byte order alone is not treated as SQLite sort order.
- `Serialize` and `Deserialize` use SQLite page images for the native engine and a minweight logical snapshot for non-native engines.
- Minweight models user-visible BTree settings for `PRAGMA page_size`, `PRAGMA auto_vacuum`, `PRAGMA secure_delete`, and `PRAGMA max_page_count`; these are logical settings, not real page-file contents.

## Fixed In This Round

- Fixed index comparison for TEXT and VARCHAR keys by keeping the unpack buffer alive for `_sqlite3VdbeRecordCompare`.
- Zeroed the raw allocated `UnpackedRecord` `Mem` array before `_sqlite3VdbeRecordUnpack`.
- Sorted non-int-key `BtreeFirst` and `BtreeLast` cursor snapshots with SQLite index comparison instead of raw key bytes.
- Added payload fetch padding for SQLite record overread assumptions.
- Added minweight regressions for unique TEXT lookup, multi-statement `QueryRow`, VARCHAR primary key shape, Issue19-shaped inserts, and ORDER BY column preservation.
- Matched backup-init destination-in-use failure for minweight logical backup/restore.
- Added compact VARCHAR primary-key equality lookup coverage.
- Added built-in window `sum(...) OVER (...)` coverage to isolate the remaining Go UDF window gap.
- Fixed minweight cursor refresh across ephemeral-table inserts/deletes so built-in and Go UDF window frames see the same rows as SQLite btree.
- Matched `mode=ro` readonly handles: minweight now reports `sqlite3_db_readonly` through `BtreeIsReadonly` and rejects write transactions with `SQLITE_READONLY`.
- Added physical placeholder open handling for path-backed minweight databases so chmod checks can target an on-disk name and invalid parent directories fail with `SQLITE_CANTOPEN`.
- Added minweight logical `Serialize`/`Deserialize` round-trip support for schema and row data without pretending to expose SQLite page bytes.
- Matched `SQLITE_FCNTL_PERSIST_WAL` for minweight's path-backed databases with `-wal` placeholder cleanup/persistence behavior.
- Matched write transaction rollback, explicit savepoint rollback/release, and statement-level rollback for minweight logical state.
- Added minweight `BtreeTransferRow`/`BTREE_PREFORMAT` row transfer support for SQLite's VACUUM row-copy path.
- Added logical `BtreeCopyFile` snapshot restore and `BtreeSetVersion` file-format cookie updates so VACUUM can replace the target btree without physical SQLite page images.
- Modeled minweight BTree PRAGMA state for page size, reserve bytes, max page count, secure delete, and auto-vacuum flags.
- Extended minweight logical `Serialize`/`Deserialize` payloads with those BTree settings while keeping older logical payloads readable.
- Matched minweight cursor moved/restore behavior for stale cursor snapshots so `OP_Column` and incremental-blob paths can refresh changed rows instead of reading old payloads.
- Added minweight logical `BtreeIntegrityCheck`: it scans the minweight KV snapshot, validates table/index key shapes, root metadata, row counts, and int-key rowid bounds, then feeds row counts back to SQLite's `integrity_check` registers.
- Matched incremental blob `BtreePutData` bounds: writes past the existing payload now return `SQLITE_CORRUPT` without growing or modifying the row.
- Matched incremental blob `BtreePutData` cursor permissions: writes through read-only cursors now return `SQLITE_READONLY` and leave the row unchanged.
- Matched incremental blob cursor invalidation: replacing/deleting a row, or clearing/dropping an int-key table, expires matching open blob cursors so checked blob read/write returns `SQLITE_ABORT`.
- Matched `BtreeTripAllCursors` faulting for minweight cursors, including `writeOnly` filtering and rollback trip-code propagation through `BtreeRollback`.
- Matched minweight shared-cache handle metadata for `BtreeSharable` and `BtreeConnectionCount`: shared-cache handles report the current shared refcount, while private-cache handles report 1 like native btree.
- Matched minweight shared-cache table locking for `BtreeLockTable` and `BtreeSchemaLocked`: read locks can coexist, conflicting read/write locks return `SQLITE_LOCKED_SHAREDCACHE`, and transaction end releases the handle's locks.
- Matched `BtreeCheckpoint` transaction locking: minweight now returns `SQLITE_LOCKED` when checkpoint is requested while the logical btree has an open transaction, while keeping logical WAL counters at zero.
- Matched cursor hint flag storage for minweight `BtreeCursorHintFlags`/`BtreeCursorHasHint`, including replacing old hints and mirroring SQLite's raw `BtCursor.Fhints` byte.

## Focused Test Policy

Default script concurrency is 8. Override with `TEST_PARALLEL=N` only when the machine is overloaded.

Routine minweight check:

```sh
./test-minweight-storage-engine.sh
```

This focused list includes `TestMinweightStorageEngineIntegrityCheck` plus the direct `./lib` minweight integrity/cursor tests.

Full top-level minweight check, run after broad engine semantics changes or before larger milestones:

```sh
env SQLITE_TEST_STORAGE_ENGINE=minweight TEST_PARALLEL=8 GOCACHE=${TMPDIR:-/tmp}/sqlite-go-cache go test -p 8 -parallel 8 -timeout 10m ./
```

Native btree storage-engine check:

```sh
TEST_PARALLEL=8 ./test-storage-engine.sh
```

Do not run the full `TestRegisteredFunctions` with a 180s timeout as a routine native regression. It includes the two expiring-context stress subtests below and times out before finishing on this machine. Run the targeted subtests that touch the current change, or give the full test a longer timeout when specifically working on context interruption.

## Slow Or Reduced-Frequency Tests

- `TestRegisteredFunctions/QueryContext_with_context_expiring`: native interrupt stress, about 200s worst-case by construction. Verified under minweight on 2026-06-03; keep it out of the focused script and run it only when specifically checking interrupt behavior.
- `TestRegisteredFunctions/ExecContext_with_context_expiring`: native interrupt stress, about 200s worst-case by construction. Verified under minweight on 2026-06-03; keep it out of the focused script and run it only when specifically checking interrupt behavior.
- `TestIssue53`: passes under minweight but takes about 13s on darwin/arm64. Keep it out of the focused script; run it in full minweight checks or when index seek/order code changes.
- Full `env SQLITE_TEST_STORAGE_ENGINE=minweight go test ./`: currently about 8m20s on darwin/arm64 because it includes the two expiring-context stress tests. Latest full run: 500.931s on 2026-06-04. Run after broad engine changes or before larger milestones, not after every narrow commit.
- `./test-storage-engine.sh`: includes cross-target lib test-binary compilation matrix. Run before commit, not after each narrow code edit.

## Minweight-Specific Skips

These tests are intentionally skipped only when `SQLITE_TEST_STORAGE_ENGINE=minweight` is set:

- `TestDBPageVtab`: `sqlite_dbpage` exposes physical SQLite pages.
- `TestVFS`: VFS-backed SQLite page file contents.

## TODO

- Decide whether physical page features remain explicitly unsupported or get a page-file compatibility layer: `sqlite_dbpage`, VFS-backed DB files, valid WAL frame contents, and chmod-only read-only detection are in this bucket. The current minweight integrity check is logical only and does not validate SQLite page images.
