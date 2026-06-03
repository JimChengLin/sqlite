#!/bin/sh
set -eu

parallel=${TEST_PARALLEL:-4}
: "${GOCACHE:=${TMPDIR:-/tmp}/sqlite-go-cache}"
export GOCACHE

storage_engine_tests='^(TestStorageEngineAPIIsExternallyImplementable|TestStorageEngineCanBeSelectedFromExternalPackage|TestStorageEngineCanBeSelectedConcurrently|TestStorageEngineHandleAPIsAreExternallyReachable|TestStorageEngineAPIDoesNotExposeRawABIInputs|TestMinweightStorageEngineSimpleSPJ|TestScalar|TestBlob|TestMemDB|TestSingleConn|TestConcurrentGoroutines|TestBinding|TestBeginMode|TestTxCommitBusyFix|TestConstraintPrimaryKeyError|TestConstraintUniqueError|TestBackupProgress|TestPreUpdateHook|TestFcntlDataVersion|TestDBPageVtab|TestExecReturningMultiRow|TestExecReturningMultiStatement|TestVtabUpdaterInsertUpdateDelete|TestColumnTextScan)$'

echo 'compile all packages without running tests'
go test -p "$parallel" -parallel "$parallel" -run '^$' ./...

echo 'run focused storage-engine behavior tests'
go test -p "$parallel" -parallel "$parallel" -run "$storage_engine_tests"

echo 'run VFS tests'
go test -p "$parallel" -parallel "$parallel" ./vfs/...

echo 'compile lib storage-engine matrix'
for target in \
	darwin/amd64 \
	darwin/arm64 \
	freebsd/amd64 \
	freebsd/arm64 \
	linux/386 \
	linux/amd64 \
	linux/arm \
	linux/arm64 \
	linux/loong64 \
	linux/ppc64le \
	linux/riscv64 \
	linux/s390x \
	openbsd/amd64 \
	openbsd/arm64 \
	windows/386 \
	windows/amd64 \
	windows/arm64
do
	echo "$target"
	GOOS=${target%/*} GOARCH=${target#*/} go test ./lib
done
