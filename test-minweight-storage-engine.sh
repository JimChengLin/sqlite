#!/bin/sh
set -eu

parallel=${TEST_PARALLEL:-8}
: "${GOCACHE:=${TMPDIR:-/tmp}/sqlite-go-cache}"
export GOCACHE
export SQLITE_TEST_STORAGE_ENGINE=minweight

# Current minweight unsupported list:
# See MINWEIGHT_STORAGE_ENGINE.md for progress, TODOs, and slow-test policy.
# - TestDBPageVtab is run: minweight exposes a logical page-1 header, not full physical page images.
# - TestVFS is run: read-only VFS SQLite files are imported into minweight logical storage on open.
# - Native sqlite3_serialize/sqlite3_deserialize expose SQLite page images; minweight uses a logical snapshot for round-trip behavior.
minweight_tests='^(TestMinweightStorageEngineSimpleSPJ|TestMinweightStorageEngineUniqueTextLookup|TestMinweightStorageEngineIntegrityCheck|TestMinweightStorageEngineQueryRowMultiStatement|TestMinweightStorageEngineVarcharPrimaryKey|TestMinweightStorageEngineIssue19Shape|TestMinweightStorageEngineOrderByPreservesColumns|TestMinweightStorageEngineBuiltinWindowSum|TestMinweightStorageEngineLogicalSerializeRoundTrip|TestMinweightStorageEngineVacuumTransfersRows|TestMinweightStorageEngineDropTableMovesAutoVacuumRoot|TestMinweightStorageEngineDropTableReusesFreedRoot|TestMinweightStorageEngineBtreePragmaState|TestMinweightStorageEngineMmapSizePragma|TestMinweightStorageEngineDBPageVtabLogicalHeader|TestMinweightStorageEngineChmodOnlyReadOnlyOpen|TestMinweightStorageEngineLogicalSerializePreservesBtreePragmaState|TestMinweightStorageEngineLogicalSerializePreservesReusedTableRoot|TestMinweightStorageEngineLogicalSerializePreservesIndexRootBelowTable|TestMinweightStorageEngineLogicalBackupRestorePreservesIndexRootBelowTable|TestMinweightStorageEngineLogicalRoundTripPreservesFreelistReuseOrder|TestMinweightStorageEngineTransactionRollbackRestoresRows|TestMinweightStorageEngineSavepointRollbackRestoresRows|TestMinweightStorageEngineStatementRollbackRestoresRows|TestIssue19|TestIssue97|TestScalar|TestBlob|TestBinding|TestColumnTextScan|TestExecReturningMultiRow|TestExecReturningMultiStatement|TestMemDB|TestSingleConn|TestBeginMode|TestTxCommitBusyFix|TestConstraintPrimaryKeyError|TestConstraintUniqueError|TestOpenV2FailureErrorMessage|TestConcurrentGoroutines|TestBackupProgress|TestPreUpdateHook|TestFcntlDataVersion|TestFcntlPersistWAL|TestIsReadOnly|TestDBPageVtab|TestVFS|TestVtabUpdaterInsertUpdateDelete)$'

echo 'run focused minweight storage-engine behavior tests'
go test -p "$parallel" -parallel "$parallel" -timeout 180s -run "$minweight_tests"

echo 'run focused minweight lib cursor tests'
go test -p "$parallel" -parallel "$parallel" -timeout 180s ./lib

echo 'run focused minweight serialize behavior tests'
go test -p "$parallel" -parallel "$parallel" -timeout 180s -run '^TestRegisteredFunctions/(serialize_and_deserialize|serialize_and_deserialize_allocator)$'
