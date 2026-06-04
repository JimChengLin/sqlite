#!/bin/sh
set -eu

parallel=${TEST_PARALLEL:-8}
: "${GOCACHE:=${TMPDIR:-/tmp}/sqlite-go-cache}"
export GOCACHE
matrix=${STORAGE_ENGINE_MATRIX:-focused}

case "$matrix" in
focused)
	matrix_targets='darwin/arm64 linux/amd64'
	;;
full)
	matrix_targets='darwin/amd64 darwin/arm64 freebsd/amd64 freebsd/arm64 linux/386 linux/amd64 linux/arm linux/arm64 linux/loong64 linux/ppc64le linux/riscv64 linux/s390x openbsd/amd64 openbsd/arm64 windows/386 windows/amd64 windows/arm64'
	;;
*)
	matrix_targets=$matrix
	;;
esac

storage_engine_api_tests='^(TestStorageEngineAPIIsExternallyImplementable|TestStorageEngineCanBeSelectedFromExternalPackage|TestStorageEngineCanBeSelectedConcurrently|TestStorageEngineOpenConnectionsKeepBoundEngineAfterGlobalSwitch|TestStorageEngineHandleAPIsAreExternallyReachable|TestStorageEngineAPIDoesNotExposeRawABIInputs)$'
minweight_tests='^(TestMinweightStorageEngineSimpleSPJ|TestMinweightStorageEngineIntegrityCheck|TestMinweightStorageEngineAttachCommitRollback|TestMinweightStorageEngineWithoutRowidCompositePrimaryKey|TestMinweightStorageEngineUncommittedWriteInvisibleToOtherConnection|TestMinweightStorageEngineRejectsConcurrentWriters|TestMinweightStorageEngineBusyTimeoutWaitsForWriter|TestMinweightStorageEngineOpenReadCursorBlocksWriterCommit|TestMinweightStorageEngineTableCursorSeekSkipsRowidGapsAndOverlay|TestMinweightStorageEngineLogicalSerializeRoundTrip|TestMinweightStorageEngineVacuumTransfersRows|TestMinweightStorageEngineBtreePragmaState|TestMinweightStorageEnginePathBackedStorePersistsAcrossEngine|TestMinweightStorageEnginePathBackedRollbackDoesNotPersistOverlay|TestMinweightStorageEngineReadOnlyPathOpenFailsFast|TestMinweightStorageEngineLogicalSerializePreservesBtreePragmaState|TestMinweightStorageEngineLogicalSerializePreservesIndexRootBelowTable|TestMinweightStorageEngineLogicalBackupRestorePreservesIndexRootBelowTable|TestMinweightStorageEngineLogicalRoundTripPreservesFreelistReuseOrder|TestMinweightStorageEngineTransactionRollbackRestoresRows|TestMinweightStorageEngineSavepointRollbackRestoresRows|TestMinweightStorageEngineStatementRollbackRestoresRows)$'
native_behavior_tests='^(TestScalar|TestBlob|TestMemDB|TestSingleConn|TestConcurrentGoroutines|TestBinding|TestBeginMode|TestTxCommitBusyFix|TestConstraintPrimaryKeyError|TestConstraintUniqueError|TestBackupProgress|TestPreUpdateHook|TestFcntlDataVersion|TestFcntlPersistWAL|TestDBPageVtab|TestExecReturningMultiRow|TestExecReturningMultiStatement|TestVtabUpdaterInsertUpdateDelete|TestColumnTextScan)$'

echo 'compile all packages without running tests'
go test -p "$parallel" -parallel "$parallel" -run '^$' ./...

echo 'run focused storage-engine API tests'
go test -p "$parallel" -parallel "$parallel" -run "$storage_engine_api_tests"

echo 'run focused lib storage-engine binding tests'
go test -p "$parallel" -parallel "$parallel" -run '^TestStorageEngineConnectionClosedClearsDBBinding$' ./lib

echo 'run focused minweight adapter tests'
go test -p "$parallel" -parallel "$parallel" -run "$minweight_tests"

echo 'run focused native/top-level behavior tests'
go test -p "$parallel" -parallel "$parallel" -run "$native_behavior_tests"

echo 'run focused serialize behavior tests'
go test -p "$parallel" -parallel "$parallel" -run '^TestRegisteredFunctions/(serialize_and_deserialize|serialize_and_deserialize_allocator)$'

echo 'run VFS tests'
go test -p "$parallel" -parallel "$parallel" ./vfs/...

echo "compile lib storage-engine matrix: $matrix_targets"
for target in $matrix_targets; do
	echo "$target"
	GOOS=${target%/*} GOARCH=${target#*/} go test -c -o "${TMPDIR:-/tmp}/sqlite-lib-${target%/*}-${target#*/}.test" ./lib
done
