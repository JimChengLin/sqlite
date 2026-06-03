#!/bin/sh
set -eu

parallel=${TEST_PARALLEL:-8}
: "${GOCACHE:=${TMPDIR:-/tmp}/sqlite-go-cache}"
export GOCACHE
export SQLITE_TEST_STORAGE_ENGINE=minweight

# Current minweight unsupported list:
# See MINWEIGHT_STORAGE_ENGINE.md for progress, TODOs, and slow-test policy.
# - TestDBPageVtab is run and explicitly skipped: sqlite_dbpage exposes physical SQLite pages, which minweight does not model yet.
# - Physical file behavior is covered by the normal SQLite btree suite; minweight currently skips read-only/path/VFS/WAL persistence page-file checks.
# - sqlite3_serialize/sqlite3_deserialize expose SQLite page images; minweight fails fast instead of pretending it can serialize them.
minweight_tests='^(TestMinweightStorageEngineSimpleSPJ|TestMinweightStorageEngineUniqueTextLookup|TestMinweightStorageEngineQueryRowMultiStatement|TestMinweightStorageEngineVarcharPrimaryKey|TestMinweightStorageEngineIssue19Shape|TestMinweightStorageEngineOrderByPreservesColumns|TestIssue19|TestScalar|TestBlob|TestBinding|TestColumnTextScan|TestExecReturningMultiRow|TestExecReturningMultiStatement|TestMemDB|TestSingleConn|TestBeginMode|TestTxCommitBusyFix|TestConstraintPrimaryKeyError|TestConstraintUniqueError|TestConcurrentGoroutines|TestBackupProgress|TestPreUpdateHook|TestFcntlDataVersion|TestDBPageVtab|TestVtabUpdaterInsertUpdateDelete)$'

echo 'run focused minweight storage-engine behavior tests'
go test -p "$parallel" -parallel "$parallel" -timeout 180s -run "$minweight_tests"
