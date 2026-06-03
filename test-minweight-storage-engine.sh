#!/bin/sh
set -eu

parallel=${TEST_PARALLEL:-8}
: "${GOCACHE:=${TMPDIR:-/tmp}/sqlite-go-cache}"
export GOCACHE
export SQLITE_TEST_STORAGE_ENGINE=minweight

# Current minweight unsupported list:
# See MINWEIGHT_STORAGE_ENGINE.md for progress, TODOs, and slow-test policy.
# - TestDBPageVtab is run and explicitly skipped: sqlite_dbpage exposes physical SQLite pages, which minweight does not model yet.
# - VFS/WAL persistence page-file checks are still physical page-file behavior.
# - Native sqlite3_serialize/sqlite3_deserialize expose SQLite page images; minweight uses a logical snapshot for round-trip behavior.
minweight_tests='^(TestMinweightStorageEngineSimpleSPJ|TestMinweightStorageEngineUniqueTextLookup|TestMinweightStorageEngineQueryRowMultiStatement|TestMinweightStorageEngineVarcharPrimaryKey|TestMinweightStorageEngineIssue19Shape|TestMinweightStorageEngineOrderByPreservesColumns|TestMinweightStorageEngineBuiltinWindowSum|TestMinweightStorageEngineLogicalSerializeRoundTrip|TestIssue19|TestIssue97|TestScalar|TestBlob|TestBinding|TestColumnTextScan|TestExecReturningMultiRow|TestExecReturningMultiStatement|TestMemDB|TestSingleConn|TestBeginMode|TestTxCommitBusyFix|TestConstraintPrimaryKeyError|TestConstraintUniqueError|TestOpenV2FailureErrorMessage|TestConcurrentGoroutines|TestBackupProgress|TestPreUpdateHook|TestFcntlDataVersion|TestIsReadOnly|TestDBPageVtab|TestVtabUpdaterInsertUpdateDelete)$'

echo 'run focused minweight storage-engine behavior tests'
go test -p "$parallel" -parallel "$parallel" -timeout 180s -run "$minweight_tests"

echo 'run focused minweight serialize behavior tests'
go test -p "$parallel" -parallel "$parallel" -timeout 180s -run '^TestRegisteredFunctions/(serialize_and_deserialize|serialize_and_deserialize_allocator)$'
