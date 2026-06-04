# Minweight Storage Engine Progress

Last updated: 2026-06-04.

## Current Status

- Storage engine dispatch enters through the Go `StorageEngine` interface. The native implementation still delegates to the translated SQLite btree code, and minweight uses `github.com/JimChengLin/minweight_store`.
- `SQLITE_TEST_STORAGE_ENGINE=minweight` installs the minweight engine in `TestMain`.
- Open btree handles are bound to the engine selected when they were opened. Closed driver connections explicitly clear their sqlite3* -> engine binding, so a reused sqlite3* address cannot dispatch new db-level calls such as logical backup metadata to a stale engine.
- Table rows are stored as `t || root:u32be || sortableRowid:u64be -> recordPayload`.
- New index and WITHOUT ROWID entries are stored as `i || root:u32be || 0x00 || sqliteComparableKey -> sqliteIndexRecordBytes`. The value remains the original SQLite record bytes returned to SQLite as btree payload. Legacy `i || root:u32be || sqliteIndexRecordBytes` keys remain readable.
- `sqliteComparableKey` currently covers SQLite NULL, INTEGER, REAL, TEXT, and BLOB storage classes plus `BINARY`, `NOCASE`, `RTRIM`, and DESC order. Custom collations, non-UTF-8 `KeyInfo`, and `KEYINFO_ORDER_BIGNULL` fail fast until they have real sort-key implementations.
- Non-integer `First`/`Last`/`Next`/`Previous` now seek over versioned `sqliteComparableKey` entries and merge the current writer overlay. `BtreeIndexMoveto` also builds a sortable probe key from SQLite `UnpackedRecord`/`TMem` and seeks with `SeekGE` for versioned roots, then verifies the result with SQLite's own record comparator. Legacy raw index roots still use the materialized compatibility path until a migration/fail-fast policy is chosen.
- `Serialize` and `Deserialize` use SQLite page images for the native engine and a minweight logical snapshot for non-native engines.
- Minweight is currently a logical BTree/KV engine. `:memory:` and temp databases use `minweight.New()`, while path-backed minweight databases use the SQLite filename as a minweight store directory and open it with `minweight.Open(...)`.
- Path-backed minweight stores persist both KV rows and logical btree metadata. The metadata is stored under an internal minweight key and records root-page allocation, table/index root kind, btree meta values, and page-related logical settings; row counts are recomputed from KV at open.
- It does not implement SQLite pager page files, VFS I/O, WAL frame format, mmap, or real `sqlite_dbpage` page images.
- Lower-priority compatibility shims exist where SQLite expects pager-shaped state: minweight models user-visible BTree settings for `PRAGMA page_size`, reserve bytes, `PRAGMA auto_vacuum`, `PRAGMA secure_delete`, `PRAGMA max_page_count`, `PRAGMA cache_size`, `PRAGMA cache_spill`, and path-backed `PRAGMA mmap_size`; these are logical settings, not real page-file contents.
- Custom VFS opens under minweight are not native VFS support. The current path briefly opens the VFS-backed SQLite page file with the native btree, serializes it into a logical snapshot, replays that snapshot into minweight, then marks the minweight handle read-only.

## Fixed In This Round

- Fixed index comparison for TEXT and VARCHAR keys by keeping the unpack buffer alive for `_sqlite3VdbeRecordCompare`.
- Zeroed the raw allocated `UnpackedRecord` `Mem` array before `_sqlite3VdbeRecordUnpack`.
- Sorted non-int-key `BtreeFirst` and `BtreeLast` cursor snapshots with SQLite index comparison instead of raw key bytes.
- Added payload fetch padding for SQLite record overread assumptions.
- Added minweight regressions for unique TEXT lookup, multi-statement `QueryRow`, VARCHAR primary key shape, Issue19-shaped inserts, and ORDER BY column preservation.
- Matched backup-init destination-in-use failure for minweight logical backup/restore.
- Added compact VARCHAR primary-key equality lookup coverage.
- Added direct `ATTACH` coverage for cross-database rollback, commit, join, `DETACH`, and reopening the attached minweight path.
- Added direct `WITHOUT ROWID` coverage for a composite TEXT/INTEGER primary key, ordered scans, point lookup, update, delete, hidden-rowid rejection, and `PRAGMA integrity_check`.
- Added direct multi-connection visibility coverage: while one connection has an uncommitted write transaction, another path-backed connection reads the previous committed rows; commit makes the changes visible and rollback keeps them hidden.
- Added built-in window `sum(...) OVER (...)` coverage to isolate the remaining Go UDF window gap.
- Fixed minweight cursor refresh across ephemeral-table inserts/deletes so built-in and Go UDF window frames see the same rows as SQLite btree.
- Converted path-backed minweight databases from placeholder files plus process-local `engine.dbs` state to real `minweight.Open(filename)` stores. The SQLite filename is now the minweight store directory.
- Added path-backed store lifecycle management: `engine.dbs[key]` caches only currently open stores, increments a refcount per `BtreeOpen`, and removes/closes the store with `Store.Close()` when the last handle closes.
- Added persistent logical btree metadata for path-backed stores and a fresh-engine reopen regression that verifies schema, rows, index lookup, and `PRAGMA user_version` survive close/reopen.
- Changed read-only path handling to fail fast. `mode=ro`/chmod-only path-backed minweight opens are not treated as supported until minweight_store exposes a real read-only open mode.
- Fixed stale sqlite3* engine bindings on connection close. This closes a P0 dispatch bug where a later connection could reuse the same sqlite3* address and have db-level logical metadata calls routed to an old minweight engine while btree-handle calls still used the new handle.
- Tightened the minweight single-writer protocol: `BtreeBeginTrans(... wrflag != 0)` now rejects a competing writer with `SQLITE_BUSY` instead of replacing the active writer, and SQL-level coverage verifies the second writer cannot commit rows until the first writer finishes.
- Matched busy-handler retry for competing minweight writers. With `PRAGMA busy_timeout` set, a second writer now waits through SQLite's busy handler and succeeds if the active writer commits before timeout.
- Added statement reader tracking for minweight read cursors. An open read-only `BtreeCursor` now holds a counted reader slot until `BtreeCloseCursor`, so an ordinary unclosed `SELECT` rows cursor makes a concurrent writer commit return `SQLITE_BUSY` like rollback-journal SQLite; coverage also checks the failed commit does not leak its write.
- Replaced direct writes to the committed minweight store with a transaction overlay. `BtreeInsert`/`BtreeDelete`/clear/drop/root metadata changes now write a per-writer delta plus working metadata, and `BtreeCommitPhaseTwo` publishes the delta with one `minweight.Store.WriteBatch`.
- Removed write-transaction whole-store snapshots from rollback, savepoint rollback, and statement rollback paths. Those paths now discard or restore the overlay and working metadata instead of scanning and replaying the entire KV store; path-backed rollback/reopen coverage verifies rolled-back rows, indexes, and `PRAGMA user_version` do not persist.
- Moved int-key table cursor positioning off whole-root materialization. `BtreeFirst`/`BtreeLast`/`BtreeTableMoveto`/`BtreeNext`/`BtreePrevious` now use minweight `SeekGE`/`SeekLE` plus the current transaction overlay, with coverage for large rowid gaps and overlay insert/delete/update visibility.
- Added versioned physical index keys. New index/WITHOUT ROWID entries use `i || root || 0x00 || sqliteComparableKey` while preserving the original SQLite record bytes as value/payload; deletes, root moves, clears, and row transfer now track the actual store key instead of recomputing a raw-record key.
- Added direct sortable-key adapter coverage for storage-class ordering, numeric encoding, `BINARY`/`NOCASE`/`RTRIM`, DESC, versioned key decode, and unsupported custom collation fail-fast.
- Moved non-int-key `BtreeFirst`/`BtreeLast`/`BtreeNext`/`BtreePrevious` onto versioned-key seek paths. They use `SeekGE`/`ReverseScanRange` plus overlay merge for new versioned keys; legacy raw index keys still fall back to materialized compatibility until a migration/fail-fast policy is chosen. Direct lib coverage verifies ordered cursor movement with overlay insert/delete without materializing the full root.
- Moved versioned-root `BtreeIndexMoveto` onto sortable probe seek. The adapter encodes `UnpackedRecord`/`TMem` prefixes into versioned `sqliteComparableKey` probes, seeks with minweight `SeekGE`, merges the writer overlay, and still checks the found payload with `_sqlite3VdbeRecordCompare` before returning SQLite's compare result.
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
- Matched raw cursor pin flags for minweight `BtreeCursorPin`/`BtreeCursorUnpin`: pinning toggles only `BTCF_Pinned` and preserves other `BtCursor.FcurFlags` bits like native btree.
- Matched minweight `PRAGMA cache_size`/`cache_spill` visible state by tracking `BtreeSetCacheSize` and returning native-style effective values from `BtreeSetSpillSize`. This is a low-priority compatibility shim, not functional storage behavior.
- Matched logical backup source state for minweight: unfinished backups now make `BtreeIsInBackup` true, so SQLite's busy checks reject closing the source connection until `Finish`/`Commit` releases the backup.
- Matched minweight `PRAGMA mmap_size` visible state for path-backed databases by attaching minimal fake-file `xFileControl` support for `SQLITE_FCNTL_MMAP_SIZE`; this records the advisory limit but does not implement real memory mapping.
- Matched minweight `BtreeSetPagerFlags` fake-pager state by reusing SQLite's generated pager flag logic for sync mode, WAL sync flags, and cache-spill flags; this keeps internal pager metadata aligned without adding real page-file sync behavior.
- Matched auto-vacuum `DROP TABLE` root-page movement: when SQLite asks to drop a non-largest root, minweight now moves the largest logical root into the gap, reports `piMoved`, and keeps `sqlite_schema.rootpage` plus indexed lookups aligned with native btree.
- Matched ordinary `DROP TABLE` root reuse: minweight now records freed logical root pages and lets subsequent `BtreeCreateTable` reuse them, keeping `sqlite_schema.rootpage` stable like native btree without modeling the physical freelist pages.
- Matched logical serialize/backup schema replay order for reused table roots by sorting root-bearing schema objects by `rootpage` within each object kind, so table-level root reuse survives minweight logical round-trips.
- Matched logical `Serialize`/`Deserialize` replay for the concrete shape where an index rootpage is lower than its owning table rootpage. The logical payload now records `tbl_name` and `rootpage`; replay uses temporary filler tables to reserve lower roots, then frees the matching filler immediately before creating the lower-root index.
- Matched logical `Backup`/`Restore` schema replay with the same rootpage-aware path used by `Serialize`/`Deserialize`, including the tested lower-root index shape.
- Fixed logical restore `Finish`: the backup state is now released before closing the remote source connection, so `FinishLogicalBackup` still sees the source handle.
- Preserved minweight hidden root allocation metadata (`nextRoot` and `freeRoots`) across logical `Serialize`/`Deserialize`, `Backup`, and `Restore`, so later `CREATE TABLE` root reuse follows the source freelist order even when visible `sqlite_schema.rootpage` is ambiguous.
- Removed the chmod-only readonly placeholder behavior for path-backed minweight databases. That behavior checked a fake path rather than minweight_store data, so it is now documented and tested as unsupported/fail-fast.
- Matched the existing `TestDBPageVtab` read path for minweight by exposing a logical page-1 blob with the SQLite file header instead of letting the generated dbpage virtual table dereference the fake pager. This is a low-priority compatibility shim, not a full physical page-image implementation.
- Matched the existing read-only `TestVFS` path for minweight by importing a native-btree logical snapshot and marking the resulting minweight handle readonly. This is a low-priority compatibility shim, not minweight VFS support.
- Matched basic path-backed multi-connection read visibility by letting non-writer handles read the writer's transaction-start snapshot until the writer commits or rolls back. This is committed-view isolation for the tested row/meta/count/cursor read paths, not a complete MVCC implementation.
- Added the first optimistic transaction-view foundation: committed generation numbers, short reader-view pins, retained per-generation committed key changes, writer point-read tracking, commit conflict validation, and pruning of old generation changes after the last reader releases them. Current coverage is direct adapter-level validation for pinned old-version retention and read-set conflict rejection; range read sets and full SQL statement lifecycle integration remain P0.
- Cleaned the changed `./lib` lint surface. `golangci-lint run ./lib --timeout 5m` and `golangci-lint run ./lib --enable-only=gocyclo,funlen --new --timeout 5m` are the routine gates for this branch. A full historical `gocyclo,funlen` run still reports legacy long/complex minweight functions and old tests; track those as planned refactors instead of re-running them after every narrow edit.
- File size is not covered by the current golangci-lint gates. Keep `lib/minweight_storage_engine.go` on the explicit refactor list and split it by ownership boundary instead of assuming lint will catch it.

## Focused Test Policy

Default script concurrency is 8. Override with `TEST_PARALLEL=N` only when the machine is overloaded.

Routine minweight check:

```sh
./test-minweight-storage-engine.sh
```

Latest focused minweight run: passed on 2026-06-04 with `TEST_PARALLEL=8`, including the competing-writer, busy-timeout retry, open-reader-cursor `SQLITE_BUSY`, and versioned `BtreeIndexMoveto` probe-seek regressions.

This focused list includes `TestMinweightStorageEngineIntegrityCheck` plus direct `./lib` minweight integrity/cursor/index-probe tests and storage-engine binding cleanup coverage.
It prioritizes real SQL/storage semantics such as `ATTACH`, `WITHOUT ROWID`, multi-connection committed visibility, rollback/savepoint behavior, backup/serialize logical round-trips, index lookup/order behavior, blob invalidation, and shared-lock behavior.
The script runs minweight adapter-specific tests and generic top-level SQL behavior tests in separate `go test` processes. This keeps failures easier to attribute; stale sqlite3* reuse is covered by connection-close binding cleanup rather than by relying on process splitting.
It does not include the low-priority `sqlite_dbpage` and custom-VFS snapshot shims; broad/full minweight runs still cover those.
It also excludes native SQLite file-open/read-only tests such as `TestIssue97`, `TestIsReadOnly`, and `TestOpenV2FailureErrorMessage` because path-backed minweight filenames are store directories and read-only path opens currently fail fast instead of emulating SQLite page-file readonly behavior.

Broad top-level minweight check without the two context-expiration stress subtests:

```sh
./test-minweight-broad.sh
```

Latest broad run: 94.366s on 2026-06-04 with `-p 8 -parallel 8`. Run this after non-interrupt engine behavior changes when the full context stress coverage is not the point. The broad script skips only `TestRegisteredFunctions/QueryContext_with_context_expiring` and `TestRegisteredFunctions/ExecContext_with_context_expiring`.

Full top-level minweight check, run after broad engine semantics changes, context-interrupt changes, or before larger milestones:

```sh
./test-minweight-full.sh
```

Latest full run: 494.331s on 2026-06-04 with `-p 8 -parallel 8`.

Native btree storage-engine check:

```sh
TEST_PARALLEL=8 ./test-storage-engine.sh
```

Latest storage-engine script run: passed on 2026-06-04 with `TEST_PARALLEL=8`, including the lib binding cleanup test, competing-writer/busy-timeout/open-reader-cursor/versioned-index-probe minweight regressions, and the focused `./lib` test-binary compile matrix for `darwin/arm64` plus `linux/amd64`.

Do not run the full `TestRegisteredFunctions` with a 180s timeout as a routine native regression. It includes the two expiring-context stress subtests below and times out before finishing on this machine. Run the targeted subtests that touch the current change, or give the full test a longer timeout when specifically working on context interruption.
`test-storage-engine.sh` also runs storage-engine API tests, lib binding cleanup tests, minweight adapter tests, and native/top-level behavior tests as separate `go test` processes. Keep that split for focused signal and faster diagnosis; correctness must still come from handle/db binding cleanup in the storage-engine dispatch layer.
The default lib compile matrix is intentionally only `darwin/arm64` and `linux/amd64`. Use `STORAGE_ENGINE_MATRIX=full ./test-storage-engine.sh` for the full cross-target matrix, or set `STORAGE_ENGINE_MATRIX='linux/amd64 windows/amd64'` for an explicit target list.

## Slow Or Reduced-Frequency Tests

- `TestRegisteredFunctions/QueryContext_with_context_expiring`: native interrupt stress, about 200s worst-case by construction. Verified under minweight on 2026-06-03; keep it out of the focused script and run it only when specifically checking interrupt behavior.
- `TestRegisteredFunctions/ExecContext_with_context_expiring`: native interrupt stress, about 200s worst-case by construction. Verified under minweight on 2026-06-03; keep it out of the focused script and run it only when specifically checking interrupt behavior.
- `TestIssue53`: passes under minweight; latest targeted run after index seek changes was 3.145s on 2026-06-04. Keep it out of the focused script; run it in full minweight checks or when index seek/order code changes.
- `sqlite_dbpage` and custom-VFS snapshot compatibility: useful to reduce user surprise, but low priority because minweight does not implement physical page images or VFS. Keep these out of the focused script; run them in broad/full checks or when editing those shims.
- Native SQLite page-file open/read-only behavior tests (`TestIssue97`, `TestIsReadOnly`, `TestOpenV2FailureErrorMessage`): keep them out of the focused minweight script. Minweight path-backed databases are directories opened with `minweight.Open`, and `mode=ro` is covered by `TestMinweightStorageEngineReadOnlyPathOpenFailsFast` until real minweight read-only open exists.
- Full `./test-minweight-full.sh`: currently about 8m15s on darwin/arm64 because it includes the two expiring-context stress tests. Latest full run: 494.331s on 2026-06-04 with `-p 8 -parallel 8`. Run after broad engine changes or before larger milestones, not after every narrow commit.
- `STORAGE_ENGINE_MATRIX=full ./test-storage-engine.sh`: full cross-target lib test-binary compilation matrix. Run when storage-engine ABI signatures or generated-code wrappers change broadly, not after every commit. The default script already covers the high-signal `darwin/arm64` and `linux/amd64` targets.
- Full historical `golangci-lint run ./lib --enable-only=gocyclo,funlen --timeout 5m`: useful when specifically splitting old debt, but not a routine gate yet. Current new-code gate is `--new`; known old debt includes large minweight btree entrypoints such as cursor movement, insert/delete, integrity check, and legacy test helpers.

## Minweight-Specific Skips

No top-level tests are currently skipped only because `SQLITE_TEST_STORAGE_ENGINE=minweight` is set.

## TODO

- P0: finish replacing materialized cursor compatibility paths. Int-key table movement, non-int-key sequential movement, and versioned-root `BtreeIndexMoveto` now use seek/range APIs; legacy raw index roots still materialize until migration/fail-fast policy exists.
- P0: decide the migration/fail-fast policy for legacy raw index keys in path-backed stores before relying on physical byte order for index cursors.
- P0: replace remaining transaction-start snapshot read paths with adapter-owned optimistic transaction views: statement/read cursor generation pins, writer read set/range read set/write set, commit-phase validation, and in-memory old-generation retention while readers may still access it.
- P0: finish optimistic transaction-view integration. The current foundation tracks committed generations, point read sets, reader pins, retained key changes, and commit conflict tests; next steps are range read sets, SQL statement/cursor pin boundaries, busy-handler behavior for conflicts, and removal of transaction-start snapshot reads from normal visibility.
- P0: keep explicit long read transactions unsupported until the generation pin/release model is complete. Autocommit statement readers can pin short views; multi-statement old-view semantics must fail fast or keep rollback-journal `SQLITE_BUSY` behavior rather than pretending full MVCC.
- P0: keep WAL disabled/fail-fast until stable generation-pinned read views exist. The adapter no longer advertises pager WAL support; `-wal` placeholder behavior and `SQLITE_FCNTL_PERSIST_WAL` remain low-priority compatibility shims, not WAL support.
- P0: continue removing metadata races around read-view lifetime. Writer metadata now uses a working copy and commit publishes it with the overlay, but generation-pinned readers need versioned metadata/read-set validation before WAL-like concurrency can be claimed.
- P1: multi-database `ATTACH` commit is still logical best-effort, not crash-atomic master-journal semantics. Treat it as SQL-level coverage only until a cross-database commit protocol exists.
- P1: logical backup/serialize remain SQL replay/logical snapshot features, not SQLite page-image backup. Keep virtual tables, shadow tables, generated/hidden columns, stats tables, and backup progress semantics on the risk list.
- P1: replace `clearRoot`, `moveRoot`, and `BtreeCopyFile` whole-root/whole-store rewrites with range delete, root rewrite, and batch operations.
- P1: refine minweight error-code mapping and remove panic paths for unknown btree/cursor handles from user-facing ABI error paths.
- P1: keep splitting historical large/complex minweight functions and tests. New/changed code must stay clean under `golangci-lint --new --enable-only=gocyclo,funlen`; full historical cleanup can continue in smaller commits instead of hiding semantic changes inside a giant mechanical refactor.
- P1: split `lib/minweight_storage_engine.go` into focused files, with likely boundaries for handles/lifecycle, metadata persistence, transaction overlay/generation view, key encoding, table cursors, index cursors, backup/serialize, and pager/file compatibility shims.
- P2: keep fake pager/file, `sqlite_dbpage`, mmap, cache/spill, and custom-VFS snapshot import explicitly documented as compatibility shims. Do not count extra no-op PRAGMA alignment as storage semantics progress.
