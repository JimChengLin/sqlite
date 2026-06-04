#!/bin/sh
set -eu

parallel=${TEST_PARALLEL:-8}
: "${GOCACHE:=${TMPDIR:-/tmp}/sqlite-go-cache}"
export GOCACHE
export SQLITE_TEST_STORAGE_ENGINE=minweight

# Reduced-frequency full top-level sweep. It still includes the slow context-expiration
# stress tests, so run it only for interrupt work, broad engine changes, or milestones.
go test -p "$parallel" -parallel "$parallel" -timeout 10m \
	-skip '^(TestIssue97|TestOpenV2FailureErrorMessage|TestVFS|TestIsReadOnly)$' \
	./
