# Minweight Storage Engine Progress

Last updated: 2026-06-04.

## Current Status

- Storage engine dispatch enters through the Go `StorageEngine` interface. The native implementation still delegates to the translated SQLite btree code, and minweight uses `github.com/JimChengLin/minweight_store`.
- `SQLITE_TEST_STORAGE_ENGINE=minweight` installs the minweight engine in `TestMain`.
- Open btree handles are bound to the engine selected when they were opened. Closed driver connections explicitly clear their sqlite3* -> engine binding, so a reused sqlite3* address cannot dispatch new db-level calls such as logical backup metadata to a stale engine.
- Table rows are stored as `t || root:u32be || sortableRowid:u64be -> recordPayload`.
- Index and WITHOUT ROWID entries are stored as `i || root:u32be || 0x00 || sqliteComparableKey -> sqliteIndexRecordBytes`. The value remains the original SQLite record bytes returned to SQLite as btree payload. The unpublished raw `i || root:u32be || sqliteIndexRecordBytes` format is no longer supported; open/stat recompute and integrity check treat it as corrupt instead of silently materializing it.
- `sqliteComparableKey` currently covers SQLite NULL, INTEGER, REAL, TEXT, and BLOB storage classes plus `BINARY`, `NOCASE`, `RTRIM`, and DESC order. Custom collations, non-UTF-8 `KeyInfo`, and `KEYINFO_ORDER_BIGNULL` fail fast until they have real sort-key implementations.
- Non-integer `First`/`Last`/`Next`/`Previous` seek over versioned `sqliteComparableKey` entries and merge the current writer overlay. `BtreeIndexMoveto` builds a sortable probe key from SQLite `UnpackedRecord`/`TMem`, seeks with `SeekGE`, then verifies the result with SQLite's own record comparator.
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
- Consolidated storage-engine dispatch bindings into an explicit handle graph: `btree -> {engine, sqlite3*}`, `cursor -> btree`, and `sqlite3* -> {engine, refs}`. Cursor dispatch now resolves through the owning btree binding instead of storing an engine copy directly, while db-level aliases still clear on connection close.
- Tightened the minweight single-writer protocol: `BtreeBeginTrans(... wrflag != 0)` now rejects a competing writer with `SQLITE_BUSY` instead of replacing the active writer, and SQL-level coverage verifies the second writer cannot commit rows until the first writer finishes.
- Matched busy-handler retry for competing minweight writers. With `PRAGMA busy_timeout` set, a second writer now waits through SQLite's busy handler and succeeds if the active writer commits before timeout.
- Added statement reader tracking for minweight read cursors. An open read-only `BtreeCursor` now pins the current committed generation before reading root metadata and releases it at `BtreeCloseCursor`; ordinary autocommit `SELECT` rows can keep reading that old view while a concurrent writer commits, while explicit long read transactions still block writer commit with `SQLITE_BUSY`.
- Replaced direct writes to the committed minweight store with a transaction overlay. `BtreeInsert`/`BtreeDelete`/clear/drop/root metadata changes now write a per-writer delta plus working metadata, and `BtreeCommitPhaseTwo` publishes the delta with one `minweight.Store.WriteBatch`.
- Removed write-transaction whole-store snapshots from rollback, savepoint rollback, and statement rollback paths. Those paths now discard or restore the overlay and working metadata instead of scanning and replaying the entire KV store; path-backed rollback/reopen coverage verifies rolled-back rows, indexes, and `PRAGMA user_version` do not persist.
- Fixed `BtreeIndexMoveto` probe-key generation for SQLite `TMem` values whose `Fn` is negative but irrelevant for the active storage class, such as INTEGER. The adapter now sizes scratch buffers only from positive lengths, so WITHOUT ROWID composite-key seeks cannot panic while encoding integer probe fields.
- Moved int-key table cursor positioning off whole-root materialization. `BtreeFirst`/`BtreeLast`/`BtreeTableMoveto`/`BtreeNext`/`BtreePrevious` now use minweight `SeekGE`/`SeekLE` plus the current transaction overlay, with coverage for large rowid gaps and overlay insert/delete/update visibility.
- Added versioned physical index keys. New index/WITHOUT ROWID entries use `i || root || 0x00 || sqliteComparableKey` while preserving the original SQLite record bytes as value/payload; deletes, root moves, clears, and row transfer now track the actual store key instead of recomputing a raw-record key.
- Added direct sortable-key adapter coverage for storage-class ordering, numeric encoding, `BINARY`/`NOCASE`/`RTRIM`, DESC, versioned key decode, and unsupported custom collation fail-fast.
- Moved non-int-key `BtreeFirst`/`BtreeLast`/`BtreeNext`/`BtreePrevious` onto versioned-key seek paths. They use `SeekGE`/`ReverseScanRange` plus overlay merge for versioned keys, and direct lib coverage verifies ordered cursor movement with overlay insert/delete without materializing the full root.
- Moved versioned-root `BtreeIndexMoveto` onto sortable probe seek. The adapter encodes `UnpackedRecord`/`TMem` prefixes into versioned `sqliteComparableKey` probes, seeks with minweight `SeekGE`, merges the writer overlay, and still checks the found payload with `_sqlite3VdbeRecordCompare` before returning SQLite's compare result.
- Moved root maintenance for int-key roots and versioned non-int-key roots off `loadRows()`: `clearRoot` and `moveRoot` now iterate with seek paths and preserve versioned destination store keys.
- Removed the unpublished raw index-key compatibility path. New writes always produce versioned `sqliteComparableKey` store keys, `keyInfo == 0` still uses the versioned format with default BINARY/ASC semantics, and raw index keys now fail fast as corrupt metadata instead of becoming a long-term fallback.
- Continued splitting the large minweight adapter file by moving root clear/move maintenance into `lib/minweight_storage_engine_roots.go`, cursor movement/positioning into `lib/minweight_storage_engine_cursor.go`, and cursor lifecycle/payload/incrblob handling into `lib/minweight_storage_engine_cursor_lifecycle.go`; key encoding, transaction view, and pager/file shims are still planned split boundaries.
- Moved `BtreeCopyFile` off the logical `snapshot()/restoreSnapshot()` path. It now streams source committed KV plus the source writer overlay into the target writer overlay or one target `minweight.Store.WriteBatch`, and direct lib coverage verifies target transaction visibility before commit.
- Started splitting minweight direct tests by feature: key-format tests, root-maintenance tests, and copy-file tests now live in separate `_test.go` files instead of further growing `lib/minweight_storage_engine_test.go`.
- Moved `BtreeCursorRestore` for stale versioned index cursors onto point lookup plus `SeekGE`, moved boundary `Next`/`Previous` continuation onto last-row versioned anchors, and moved int-key stats recompute onto seek iteration.
- Removed `loadRows()` / `refreshCursorRows()` and the cursor `rows/index` materialization state from the minweight adapter. Versioned index cursors must carry either the current versioned store key or a versioned last-row anchor; missing anchors fail fast with `SQLITE_CORRUPT` instead of scanning and materializing the root.
- Split cursor-restore tests into `lib/minweight_storage_engine_cursor_test.go` and added direct coverage for stale index cursor restore, last-row anchored boundary `Next`/`Previous`, and missing store-key fail-fast behavior.
- Moved `BtreeIntegrityCheck` off the whole-store `snapshot()` helper. Full checks now stream committed KV plus writer overlay without copying every item; partial checks scan only the selected roots' table/index ranges. The old `minweightSnapshot` item copy path was removed.
- Added minweight logical `Serialize`/`Deserialize` round-trip support for schema and row data without pretending to expose SQLite page bytes.
- Matched `SQLITE_FCNTL_PERSIST_WAL` as visible FileControl state for minweight path-backed databases without creating SQLite `-wal` placeholder files.
- Matched write transaction rollback, explicit savepoint rollback/release, and statement-level rollback for minweight logical state.
- Added minweight `BtreeTransferRow`/`BTREE_PREFORMAT` row transfer support for SQLite's VACUUM row-copy path.
- Added logical `BtreeCopyFile` snapshot restore and `BtreeSetVersion` file-format cookie updates so VACUUM can replace the target btree without physical SQLite page images.
- Modeled minweight BTree PRAGMA state for page size, reserve bytes, max page count, secure delete, and auto-vacuum flags.
- Extended minweight logical `Serialize`/`Deserialize` payloads with those BTree settings while keeping older logical payloads readable.
- Matched minweight cursor moved/restore behavior for stale cursor snapshots so `OP_Column` and incremental-blob paths can refresh changed rows instead of reading old payloads.
- Added minweight logical `BtreeIntegrityCheck`: it scans visible minweight KV entries, validates table/index key shapes, root metadata, row counts, and int-key rowid bounds, then feeds row counts back to SQLite's `integrity_check` registers.
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
- Added the first optimistic transaction-view foundation: committed generation numbers, short reader-view pins, retained per-generation committed key changes, writer point-read tracking, result-bounded byte-range read tracking for seek paths, commit conflict validation, and pruning of old generation changes after the last reader releases them. Current coverage is direct adapter-level validation for pinned old-version retention, point read-set conflict rejection, byte-range conflict rejection, same-root range-outside non-conflict, and changes before/after returned seek results; full SQL statement lifecycle integration remains P0.
- Tightened optimistic write conflict reporting: minweight read-set/range conflict now returns `SQLITE_BUSY_SNAPSHOT` instead of generic `SQLITE_BUSY`, because waiting cannot make a stale writer snapshot current.
- Wired pinned/base generation range seeks into the adapter. `seekTableGE`/`seekTableLE` and versioned `seekIndexGE`/`seekIndexLE` now merge the visible committed store, retained old-generation before-images, and the writer overlay, so pinned readers and writers reading their base generation do not see later inserts/deletes/updates on seek paths.
- Wired pinned generation point reads into the adapter `get` path. A reader pinned to an older generation reconstructs updated, inserted, and deleted keys from retained commit changes for point lookup.
- Wired pinned generation metadata reads into `visibleState()`/`visibleMeta()`/`visibleTable()`. Pinned readers now reconstruct older btree meta values, data version, root allocation, and table stats by walking retained committed `beforeState` records.
- Allowed writer commit to proceed while ordinary autocommit read cursors are open. The old generation remains retained for the open cursor; explicit read transactions remain unsupported as long MVCC views and still make writer commit return `SQLITE_BUSY`.
- Removed minweight `-wal` placeholder creation from write transactions. `PRAGMA journal_mode=WAL` now stays in rollback `delete` mode for minweight and direct coverage verifies that no `-wal` file is created; `SQLITE_FCNTL_PERSIST_WAL` remains only a visible FileControl state until real WAL semantics exist.
- Cleaned the changed `./lib` lint surface. `golangci-lint run ./lib --timeout 5m` and `golangci-lint run ./lib --enable-only=gocyclo,funlen --new --timeout 5m` are the routine gates for this branch. A full historical `gocyclo,funlen` run still reports historical long/complex minweight functions and old tests; track those as planned refactors instead of re-running them after every narrow edit.
- File size is not covered by the current golangci-lint gates. Keep `lib/minweight_storage_engine.go` on the explicit refactor list and split it by ownership boundary instead of assuming lint will catch it.
- Preserved `sqlite_sequence` data through minweight logical `Serialize`/`Deserialize` and logical `Backup`. AUTOINCREMENT tables with no current rows now keep their advanced sequence value, so the next inserted rowid continues after the source sequence instead of restarting from 1.
- Preserved generated-column tables through minweight logical `Serialize`/`Deserialize` and logical `Backup`. Logical row copy now uses `PRAGMA table_xinfo` to insert only writable columns while replayed schema SQL keeps STORED and VIRTUAL generated expressions intact.
- Preserved logical rowids in minweight logical `Backup` for ordinary rowid tables and FTS5 virtual tables by copying an available hidden rowid alias alongside writable columns.
- Preserved FTS5 virtual tables through minweight logical `Serialize`/`Deserialize` and logical `Backup` by skipping FTS5 shadow tables during schema/data replay and copying only the virtual table's logical rows.
- Added high-value SQL write coverage for minweight quick/focused gates: UPSERT with `RETURNING`, `INSERT OR REPLACE` index maintenance, foreign-key cascade writes with triggers, and partial/expression unique-index statement rollback.

## Focused Test Policy

Default script concurrency is 8. Override with `TEST_PARALLEL=N` only when the machine is overloaded.

Default per-turn minweight quick gate, target under 30s:

```sh
./test-minweight.sh quick
```

Latest quick run: 7.30s on 2026-06-04 with `TEST_PARALLEL=8`.

The quick gate intentionally covers high-value storage semantics only: path-backed minweight persistence, rollback overlay behavior, `WITHOUT ROWID`/sortable index seek behavior, short reader pinned views, WAL fail-fast behavior, logical AUTOINCREMENT sequence preservation, logical generated-column preservation, logical rowid/FTS5 virtual table preservation, SQL write semantics for UPSERT/REPLACE/foreign-key cascade/triggers/partial expression indexes, handle-bound dispatch after global engine switching, direct optimistic read-set/range conflict checks, and the no-legacy-raw-index-key path. Keep low-priority shim checks such as `sqlite_dbpage`, cache/mmap visible state, and custom-VFS snapshot import out of this default list.

The old `./test-minweight-quick.sh`, `./test-minweight-broad.sh`, and `./test-minweight-full.sh` entry points are thin wrappers around `./test-minweight.sh quick|broad|full`.

Focused minweight integration check, run before commits that touch storage semantics when the quick gate is not enough:

```sh
./test-minweight-storage-engine.sh
```

Latest focused minweight run: passed on 2026-06-04 with `TEST_PARALLEL=8`, including the competing-writer, busy-timeout retry, open-reader-cursor pinned-view, explicit-read-transaction `SQLITE_BUSY`, WAL-disabled/no-placeholder, and versioned `BtreeIndexMoveto` probe-seek regressions.

This focused list includes `TestMinweightStorageEngineIntegrityCheck`, `TestMinweightStorageEngineJournalModeWALStaysRollback`, direct `./lib` minweight integrity/cursor/index-probe tests, and storage-engine binding cleanup coverage.
It prioritizes real SQL/storage semantics such as `ATTACH`, `WITHOUT ROWID`, multi-connection committed visibility, rollback/savepoint behavior, backup/serialize logical round-trips, index lookup/order behavior, blob invalidation, and shared-lock behavior.
The script runs minweight adapter-specific tests and generic top-level SQL behavior tests in separate `go test` processes. This keeps failures easier to attribute; stale sqlite3* reuse is covered by connection-close binding cleanup rather than by relying on process splitting.
It does not include the low-priority `sqlite_dbpage` and custom-VFS snapshot shims; broad/full minweight runs still cover `sqlite_dbpage`, while `TestVFS` remains skipped until minweight has real VFS I/O semantics.
It also excludes native SQLite file-open/read-only tests such as `TestIssue97`, `TestIsReadOnly`, and `TestOpenV2FailureErrorMessage` because path-backed minweight filenames are store directories and read-only path opens currently fail fast instead of emulating SQLite page-file readonly behavior.

Reduced-frequency broad top-level minweight check without context-expiration stress tests and native physical-file/VFS tests:

```sh
./test-minweight.sh broad
```

Latest broad run: 493.597s on 2026-06-04 with `TEST_PARALLEL=8`. It skips `TestRegisteredFunctions/QueryContext_with_context_expiring`, `TestRegisteredFunctions/ExecContext_with_context_expiring`, `TestIssue97`, `TestOpenV2FailureErrorMessage`, `TestVFS`, and `TestIsReadOnly`. Run this after broad engine behavior changes or before larger milestones when the full context stress coverage and native physical-file/VFS behavior are not the point; do not run it after narrow edits.

Full top-level minweight check, run after broad engine semantics changes, context-interrupt changes, or before larger milestones:

```sh
./test-minweight.sh full
```

Latest completed full run before the physical-file skip split: 494.331s on 2026-06-04 with `-p 8 -parallel 8`. A later full attempt on 2026-06-04 was stopped after the quick-gate policy change; do not restart it unless the change specifically needs context-interrupt or milestone coverage.

Native btree storage-engine check:

```sh
TEST_PARALLEL=8 ./test-storage-engine.sh
```

Latest storage-engine script run: passed on 2026-06-04 with `TEST_PARALLEL=8`, including the lib binding cleanup test, competing-writer/busy-timeout/open-reader-cursor/WAL-disabled/versioned-index-probe minweight regressions, native focused behavior tests, and the focused `./lib` test-binary compile matrix for `darwin/arm64` plus `linux/amd64`.

This native/minweight mixed script is not a default per-turn gate; it compiles packages, runs VFS tests, and builds the focused lib matrix. Run it when storage-engine API shape, dispatch binding, native/minweight shared behavior, or compile-target surface changes.

Do not run the full `TestRegisteredFunctions` with a 180s timeout as a routine native regression. It includes the two expiring-context stress subtests below and times out before finishing on this machine. Run the targeted subtests that touch the current change, or give the full test a longer timeout when specifically working on context interruption.
`test-storage-engine.sh` also runs storage-engine API tests, lib binding cleanup tests, minweight adapter tests, and native/top-level behavior tests as separate `go test` processes. Keep that split for focused signal and faster diagnosis; correctness must still come from handle/db binding cleanup in the storage-engine dispatch layer.
The default lib compile matrix is intentionally only `darwin/arm64` and `linux/amd64`. Use `STORAGE_ENGINE_MATRIX=full ./test-storage-engine.sh` for the full cross-target matrix, or set `STORAGE_ENGINE_MATRIX='linux/amd64 windows/amd64'` for an explicit target list.

## Slow Or Reduced-Frequency Tests

- `./test-minweight-storage-engine.sh`: focused integration script. Use it before commits that touch storage semantics or when quick-gate coverage is too narrow, but not after every tiny edit.
- `./test-minweight.sh broad`: currently about 8m14s on darwin/arm64 even with context-expiration and native physical-file/VFS skips. Latest broad run: 493.597s on 2026-06-04 with `TEST_PARALLEL=8`. Run after broad engine behavior changes or before larger milestones, not as routine feedback.
- `TestRegisteredFunctions/QueryContext_with_context_expiring`: native interrupt stress, about 200s worst-case by construction. Verified under minweight on 2026-06-03; keep it out of the focused script and run it only when specifically checking interrupt behavior.
- `TestRegisteredFunctions/ExecContext_with_context_expiring`: native interrupt stress, about 200s worst-case by construction. Verified under minweight on 2026-06-03; keep it out of the focused script and run it only when specifically checking interrupt behavior.
- `TestIssue53`: passes under minweight; latest targeted run after index seek changes was 3.145s on 2026-06-04. Keep it out of the focused script; run it in full minweight checks or when index seek/order code changes.
- `TestIssue51`: intentionally loops for about 1 minute and repeatedly opens/closes path-backed connections around `INSERT OR REPLACE`. Use a smaller targeted write probe for routine work; run `TestIssue51` only in broad/milestone checks for repeated-open churn.
- `sqlite_dbpage` and custom-VFS snapshot compatibility: useful to reduce user surprise, but low priority because minweight does not implement physical page images or VFS. Keep these out of the focused script; run them in broad/full checks or when editing those shims.
- Native SQLite page-file open/read-only behavior tests (`TestIssue97`, `TestIsReadOnly`, `TestOpenV2FailureErrorMessage`): keep them out of focused and broad minweight scripts. Minweight path-backed databases are directories opened with `minweight.Open`, and `mode=ro` is covered by `TestMinweightStorageEngineReadOnlyPathOpenFailsFast` until real minweight read-only open exists.
- `TestVFS`: keep it out of focused and broad minweight scripts. Minweight does not implement VFS I/O; the current VFS path is only read-only logical snapshot import coverage, not writable/native VFS support.
- Native SQLite WAL file lifecycle tests (`TestFcntlPersistWAL`): skip under minweight because minweight does not implement SQLite WAL files. Minweight coverage is `TestMinweightStorageEngineJournalModeWALStaysRollback`, which verifies `PRAGMA journal_mode=WAL` remains `delete` and no `-wal` placeholder is created.
- Full `./test-minweight.sh full`: currently about 8m15s on darwin/arm64 because it includes the two expiring-context stress tests. Latest completed full run before the physical-file skip split: 494.331s on 2026-06-04 with `-p 8 -parallel 8`; a later attempt on 2026-06-04 was stopped after adopting the quick-gate policy. Run after context-interrupt changes, broad engine changes, or before larger milestones, not after every narrow commit.
- `STORAGE_ENGINE_MATRIX=full ./test-storage-engine.sh`: full cross-target lib test-binary compilation matrix. Run when storage-engine ABI signatures or generated-code wrappers change broadly, not after every commit. The default script already covers the high-signal `darwin/arm64` and `linux/amd64` targets.
- Full historical `golangci-lint run ./lib --enable-only=gocyclo,funlen --timeout 5m`: useful when specifically splitting old debt, but not a routine gate yet. Current new-code gate is `--new`; known old debt includes large minweight btree entrypoints such as insert/delete and historical test helpers.

## Minweight-Specific Skips

- `TestFcntlPersistWAL`: skipped under minweight because it asserts native SQLite WAL file creation/cleanup. Minweight currently keeps `journal_mode=WAL` in rollback `delete` mode and must not create fake `-wal` placeholders.
- `TestIssue97`, `TestOpenV2FailureErrorMessage`, `TestIsReadOnly`: skipped by broad/full minweight scripts because they assert native SQLite page-file/read-only path behavior. Minweight path-backed databases are minweight store directories and `mode=ro` is fail-fast until minweight_store has a real read-only open mode.
- `TestVFS`: skipped by broad/full minweight scripts because minweight does not implement SQLite VFS I/O. The existing VFS path is read-only logical snapshot import coverage, not native minweight VFS support.

## TODO

- P0: finish replacing remaining materialized root consumers. Int-key table movement, non-int-key sequential movement, versioned-root `BtreeIndexMoveto`, root clear/move, `BtreeCopyFile`, `BtreeCursorRestore`, last-row anchored versioned index `Next`/`Previous`, and int-key stats recompute now avoid root materialization. `loadRows()` / `refreshCursorRows()` and cursor `rows/index` state have been removed from the adapter; old raw index keys are unsupported and should not gain an implicit migration fallback in normal cursor movement.
- P0: replace remaining transaction-start snapshot read paths with adapter-owned optimistic transaction views: autocommit read cursor generation pins now own ordinary `SELECT` views, while writer read set/range read set/write set, commit-phase validation, and in-memory old-generation retention cover current direct adapter paths.
- P0: finish optimistic transaction-view integration. The current foundation tracks committed generations, point read sets, result-bounded seek-path byte-range read sets, autocommit cursor reader pins, retained key/state changes, pinned-generation point/range/metadata reads, and commit conflict tests; next steps are SQL-level `SQLITE_BUSY_SNAPSHOT` propagation coverage for stale writer conflicts and removal of any remaining transaction-start snapshot read paths from normal visibility.
- P0: keep explicit long read transactions unsupported until the generation pin/release model is complete. Autocommit statement readers now pin short views; multi-statement old-view semantics still fail with rollback-journal-style `SQLITE_BUSY` rather than pretending full MVCC.
- P0: keep WAL disabled/fail-fast until stable generation-pinned read views exist. The adapter no longer advertises pager WAL support and no longer creates `-wal` placeholders; `SQLITE_FCNTL_PERSIST_WAL` remains a low-priority visible-state shim, not WAL support.
- P0: continue removing metadata races around read-view lifetime. Writer metadata now uses a working copy, commit publishes it with the overlay, and generation-pinned readers reconstruct old metadata from retained states; explicit transaction view lifetime still needs a clearer fail-fast/lock boundary before WAL-like concurrency can be claimed.
- P1: multi-database `ATTACH` commit is still logical best-effort, not crash-atomic master-journal semantics. Treat it as SQL-level coverage only until a cross-database commit protocol exists.
- P1: logical backup/serialize remain SQL replay/logical snapshot features, not SQLite page-image backup. `sqlite_sequence`, generated columns, ordinary rowid tables, and FTS5 virtual tables now have direct logical coverage; keep other virtual tables, non-FTS shadow table families, hidden virtual-table columns, stats tables, and backup progress semantics on the risk list.
- P1: continue replacing root/store maintenance fallbacks. `clearRoot`/`moveRoot` now use seek iteration for int-key and versioned non-int-key roots; `BtreeIntegrityCheck` now uses streaming full scans or selected-root range scans instead of whole-store snapshots; `BtreeCopyFile` now uses streaming batch/overlay copy, but still scans source and target because the operation replaces a whole btree.
- P1: refine minweight error-code mapping and remove panic paths for unknown btree/cursor handles from user-facing ABI error paths.
- P1: keep splitting historical large/complex minweight functions and tests. New/changed code must stay clean under `golangci-lint --new --enable-only=gocyclo,funlen`; full historical cleanup can continue in smaller commits instead of hiding semantic changes inside a giant mechanical refactor.
- P1: split `lib/minweight_storage_engine.go` into focused files, with likely boundaries for handles/lifecycle, metadata persistence, transaction overlay/generation view, key encoding, backup/serialize, and pager/file compatibility shims.
- P2: keep fake pager/file, `sqlite_dbpage`, mmap, cache/spill, and custom-VFS snapshot import explicitly documented as compatibility shims. Do not count extra no-op PRAGMA alignment as storage semantics progress.
