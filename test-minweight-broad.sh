#!/bin/sh
set -eu

parallel=${TEST_PARALLEL:-8}
: "${GOCACHE:=${TMPDIR:-/tmp}/sqlite-go-cache}"
export GOCACHE
export SQLITE_TEST_STORAGE_ENGINE=minweight

go test -p "$parallel" -parallel "$parallel" -timeout 10m \
	-skip '^(TestRegisteredFunctions/(QueryContext_with_context_expiring|ExecContext_with_context_expiring)|TestIssue97|TestOpenV2FailureErrorMessage|TestVFS|TestIsReadOnly)$' \
	./
