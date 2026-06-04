#!/bin/sh
set -eu

mode=${1:-quick}
parallel=${TEST_PARALLEL:-8}
: "${GOCACHE:=${TMPDIR:-/tmp}/sqlite-go-cache}"
export GOCACHE
export SQLITE_TEST_STORAGE_ENGINE=minweight

case "$mode" in
quick)
	# Default per-turn gate. Keep this list high-signal and under about 30s.
	# Broader modes are intentionally reduced-frequency; see MINWEIGHT_STORAGE_ENGINE.md.
	quick_tests='^(TestStorageEngineOpenConnectionsKeepBoundEngineAfterGlobalSwitch|TestMinweightStorageEngineSimpleSPJ|TestMinweightStorageEngineWithoutRowidCompositePrimaryKey|TestMinweightStorageEnginePathBackedStorePersistsAcrossEngine|TestMinweightStorageEnginePathBackedRollbackDoesNotPersistOverlay|TestMinweightStorageEngineOpenReadCursorUsesPinnedViewAfterWriterCommit|TestMinweightStorageEngineJournalModeWALStaysRollback|TestMinweightStorageEngineLogicalSerializePreservesSQLiteSequence|TestMinweightStorageEngineLogicalBackupPreservesSQLiteSequence|TestMinweightStorageEngineLogicalSerializePreservesGeneratedColumns|TestMinweightStorageEngineLogicalBackupPreservesGeneratedColumns|TestMinweightStorageEngineTransactionRollbackRestoresRows|TestMinweightStorageEngineSavepointRollbackRestoresRows|TestMinweightStorageEngineStatementRollbackRestoresRows|TestMinweightComparableMemKeyIgnoresNegativeLengthForInteger|TestMinweightIndexMovetoUsesVersionedSeek|TestMinweightBtreeCommitReturnsBusySnapshotOnReadConflict|TestMinweightCommitDetectsReadSetConflict|TestMinweightCommitDetectsRangeReadConflict|TestMinweightPinnedReaderPointGetUsesOldGeneration|TestMinweightPinnedReaderTableSeekUsesOldGeneration|TestMinweightPinnedReaderIndexSeekUsesOldGeneration|TestMinweightPinnedReaderMetadataUsesOldGeneration|TestMinweightIndexNextWithoutVersionedStoreKeyFailsFast|TestMinweightStatsRejectLegacyRawIndexKey|TestMinweightBtreeCopyFileUsesSourceOverlayAndTargetTxn)$'
	go test -p "$parallel" -parallel "$parallel" -timeout 30s -run "$quick_tests" ./ ./lib
	;;
broad)
	# Reduced-frequency broad top-level sweep. It skips native physical-file/VFS
	# cases and context-expiration stress.
	go test -p "$parallel" -parallel "$parallel" -timeout 10m \
		-skip '^(TestRegisteredFunctions/(QueryContext_with_context_expiring|ExecContext_with_context_expiring)|TestIssue97|TestOpenV2FailureErrorMessage|TestVFS|TestIsReadOnly)$' \
		./
	;;
full)
	# Reduced-frequency full top-level sweep. It still includes the slow
	# context-expiration stress tests.
	go test -p "$parallel" -parallel "$parallel" -timeout 10m \
		-skip '^(TestIssue97|TestOpenV2FailureErrorMessage|TestVFS|TestIsReadOnly)$' \
		./
	;;
*)
	echo "usage: $0 [quick|broad|full]" >&2
	exit 2
	;;
esac
