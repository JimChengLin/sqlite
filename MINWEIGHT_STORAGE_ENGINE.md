# Minweight Storage Engine Progress

Last updated: 2026-06-03.

## Current Status

- Storage engine dispatch enters through the Go `StorageEngine` interface. The native implementation still delegates to the translated SQLite btree code, and minweight uses `github.com/JimChengLin/minweight_store`.
- `SQLITE_TEST_STORAGE_ENGINE=minweight` installs the minweight engine in `TestMain`.
- Table rows are stored as `t || root:u32be || sortableRowid:u64be -> recordPayload`.
- Index entries are stored as `i || root:u32be || sqliteIndexRecordBytes -> sqliteIndexRecordBytes`.
- Non-integer btrees are ordered with SQLite's own record comparator through `KeyInfo`; byte order alone is not treated as SQLite sort order.
- `Serialize` and `Deserialize` fail fast under minweight because they expose SQLite page images, which minweight does not model.

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

## Focused Test Policy

Default script concurrency is 8. Override with `TEST_PARALLEL=N` only when the machine is overloaded.

Routine minweight check:

```sh
./test-minweight-storage-engine.sh
```

Full top-level minweight check, run after engine semantics change or before commit:

```sh
env SQLITE_TEST_STORAGE_ENGINE=minweight TEST_PARALLEL=8 GOCACHE=${TMPDIR:-/tmp}/sqlite-go-cache go test -p 8 -parallel 8 -timeout 10m ./
```

Native btree storage-engine check:

```sh
TEST_PARALLEL=8 ./test-storage-engine.sh
```

Do not run the full `TestRegisteredFunctions` with a 180s timeout as a routine native regression. It includes the two expiring-context stress subtests below and times out before finishing on this machine. Run the targeted subtests that touch the current change, or give the full test a longer timeout when specifically working on context interruption.

## Slow Or Reduced-Frequency Tests

- `TestRegisteredFunctions/QueryContext_with_context_expiring`: native interrupt stress, about 200s worst-case by construction. Skipped under minweight until running-query interrupt semantics are implemented.
- `TestRegisteredFunctions/ExecContext_with_context_expiring`: native interrupt stress, about 200s worst-case by construction. Skipped under minweight until running-exec interrupt semantics are implemented.
- `TestIssue53`: passes under minweight but takes about 13s on darwin/arm64. Keep it out of the focused script; run it in full minweight checks or when index seek/order code changes.
- Full `env SQLITE_TEST_STORAGE_ENGINE=minweight go test ./`: currently about 88s on darwin/arm64. Run before commit and after broad engine changes, not after every local edit.
- `./test-storage-engine.sh`: includes cross-target lib compilation matrix. Run before commit, not after each narrow code edit.

## Minweight-Specific Skips

These tests are intentionally skipped only when `SQLITE_TEST_STORAGE_ENGINE=minweight` is set:

- `TestDBPageVtab`: `sqlite_dbpage` exposes physical SQLite pages.
- `TestIssue97`: read-only database file mode.
- `TestOpenV2FailureErrorMessage`: invalid filesystem path handling.
- `TestOpenV2FailureResourceLeak`: invalid path failure leak path.
- `TestVFS`: VFS-backed SQLite page file contents.
- `TestIsReadOnly`: chmod/read-only filesystem state.
- `TestFcntlPersistWAL`: WAL files and `PERSIST_WAL` file-control.
- `TestRegisteredFunctions/serialize_and_deserialize`: SQLite page image API.
- `TestRegisteredFunctions/serialize_and_deserialize_allocator`: SQLite page image API.
- `TestRegisteredFunctions/QueryContext_with_context_expiring`: running-query interrupt stress semantics.
- `TestRegisteredFunctions/ExecContext_with_context_expiring`: running-exec interrupt stress semantics.

## TODO

- Decide whether physical page features remain explicitly unsupported or get a page-file compatibility layer: `sqlite_dbpage`, VFS-backed DB files, `Serialize`, `Deserialize`, WAL persistence, read-only chmod state, and invalid path open behavior are in this bucket.
- Implement minweight-compatible running statement interruption for expiring context stress tests.
