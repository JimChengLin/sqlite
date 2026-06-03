#!/bin/sh
set -eu

parallel=${TEST_PARALLEL:-4}
: "${GOCACHE:=${TMPDIR:-/tmp}/sqlite-go-cache}"
export GOCACHE
export SQLITE_TEST_STORAGE_ENGINE=minweight

# Current minweight unsupported list:
# - TestDBPageVtab is run and explicitly skipped: sqlite_dbpage exposes physical SQLite pages, which minweight does not model yet.
minweight_tests='^(TestMinweightStorageEngineSimpleSPJ|TestScalar|TestBlob|TestBinding|TestColumnTextScan|TestExecReturningMultiRow|TestExecReturningMultiStatement|TestMemDB|TestSingleConn|TestBeginMode|TestTxCommitBusyFix|TestConstraintPrimaryKeyError|TestConstraintUniqueError|TestConcurrentGoroutines|TestBackupProgress|TestPreUpdateHook|TestFcntlDataVersion|TestDBPageVtab|TestVtabUpdaterInsertUpdateDelete)$'

echo 'run focused minweight storage-engine behavior tests'
go test -p "$parallel" -parallel "$parallel" -timeout 180s -run "$minweight_tests"
