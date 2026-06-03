// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows && (amd64 || arm64)

package sqlite3

import "modernc.org/libc"

// storageEngine is the btree-facing storage abstraction. The default
// implementation delegates to the generated SQLite btree code.
type storageEngine interface {
	_sqlite3BtreeEnter(tls *libc.TLS, p uintptr)
	_sqlite3BtreeLeave(tls *libc.TLS, p uintptr)
	_sqlite3BtreeEnterAll(tls *libc.TLS, db uintptr)
	_sqlite3BtreeLeaveAll(tls *libc.TLS, db uintptr)
	_sqlite3BtreeEnterCursor(tls *libc.TLS, pCur uintptr)
	_sqlite3BtreeLeaveCursor(tls *libc.TLS, pCur uintptr)
	_sqlite3BtreeClearCursor(tls *libc.TLS, pCur uintptr)
	_sqlite3BtreeCursorHasMoved(tls *libc.TLS, pCur uintptr) (r int32)
	_sqlite3BtreeFakeValidCursor(tls *libc.TLS) (r uintptr)
	_sqlite3BtreeCursorRestore(tls *libc.TLS, pCur uintptr, pDifferentRow uintptr) (r int32)
	_sqlite3BtreeCursorHintFlags(tls *libc.TLS, pCur uintptr, x uint32)
	_sqlite3BtreeLastPage(tls *libc.TLS, p uintptr) (r TPgno)
	_sqlite3BtreeOpen(tls *libc.TLS, pVfs uintptr, zFilename uintptr, db uintptr, ppBtree uintptr, flags int32, vfsFlags int32) (r int32)
	_sqlite3BtreeClose(tls *libc.TLS, p uintptr) (r int32)
	_sqlite3BtreeSetCacheSize(tls *libc.TLS, p uintptr, mxPage int32) (r int32)
	_sqlite3BtreeSetSpillSize(tls *libc.TLS, p uintptr, mxPage int32) (r int32)
	_sqlite3BtreeSetMmapLimit(tls *libc.TLS, p uintptr, szMmap Tsqlite3_int64) (r int32)
	_sqlite3BtreeSetPagerFlags(tls *libc.TLS, p uintptr, pgFlags uint32) (r int32)
	_sqlite3BtreeSetPageSize(tls *libc.TLS, p uintptr, pageSize int32, nReserve int32, iFix int32) (r int32)
	_sqlite3BtreeGetPageSize(tls *libc.TLS, p uintptr) (r int32)
	_sqlite3BtreeGetReserveNoMutex(tls *libc.TLS, p uintptr) (r int32)
	_sqlite3BtreeGetRequestedReserve(tls *libc.TLS, p uintptr) (r int32)
	_sqlite3BtreeMaxPageCount(tls *libc.TLS, p uintptr, mxPage TPgno) (r TPgno)
	_sqlite3BtreeSecureDelete(tls *libc.TLS, p uintptr, newFlag int32) (r int32)
	_sqlite3BtreeSetAutoVacuum(tls *libc.TLS, p uintptr, autoVacuum int32) (r int32)
	_sqlite3BtreeGetAutoVacuum(tls *libc.TLS, p uintptr) (r int32)
	_sqlite3BtreeNewDb(tls *libc.TLS, p uintptr) (r int32)
	_sqlite3BtreeBeginTrans(tls *libc.TLS, p uintptr, wrflag int32, pSchemaVersion uintptr) (r int32)
	_sqlite3BtreeIncrVacuum(tls *libc.TLS, p uintptr) (r int32)
	_sqlite3BtreeCommitPhaseOne(tls *libc.TLS, p uintptr, zSuperJrnl uintptr) (r int32)
	_sqlite3BtreeCommitPhaseTwo(tls *libc.TLS, p uintptr, bCleanup int32) (r int32)
	_sqlite3BtreeCommit(tls *libc.TLS, p uintptr) (r int32)
	_sqlite3BtreeTripAllCursors(tls *libc.TLS, pBtree uintptr, errCode int32, writeOnly int32) (r int32)
	_sqlite3BtreeRollback(tls *libc.TLS, p uintptr, tripCode int32, writeOnly int32) (r int32)
	_sqlite3BtreeBeginStmt(tls *libc.TLS, p uintptr, iStatement int32) (r int32)
	_sqlite3BtreeSavepoint(tls *libc.TLS, p uintptr, op int32, iSavepoint int32) (r int32)
	_sqlite3BtreeCursor(tls *libc.TLS, p uintptr, iTable TPgno, wrFlag int32, pKeyInfo uintptr, pCur uintptr) (r int32)
	_sqlite3BtreeCursorSize(tls *libc.TLS) (r int32)
	_sqlite3BtreeCursorZero(tls *libc.TLS, p uintptr)
	_sqlite3BtreeCloseCursor(tls *libc.TLS, pCur uintptr) (r int32)
	_sqlite3BtreeCursorIsValidNN(tls *libc.TLS, pCur uintptr) (r int32)
	_sqlite3BtreeIntegerKey(tls *libc.TLS, pCur uintptr) (r Ti64)
	_sqlite3BtreeCursorPin(tls *libc.TLS, pCur uintptr)
	_sqlite3BtreeCursorUnpin(tls *libc.TLS, pCur uintptr)
	_sqlite3BtreeOffset(tls *libc.TLS, pCur uintptr) (r Ti64)
	_sqlite3BtreePayloadSize(tls *libc.TLS, pCur uintptr) (r Tu32)
	_sqlite3BtreeMaxRecordSize(tls *libc.TLS, pCur uintptr) (r Tsqlite3_int64)
	_sqlite3BtreePayload(tls *libc.TLS, pCur uintptr, offset Tu32, amt Tu32, pBuf uintptr) (r int32)
	_sqlite3BtreePayloadChecked(tls *libc.TLS, pCur uintptr, offset Tu32, amt Tu32, pBuf uintptr) (r int32)
	_sqlite3BtreePayloadFetch(tls *libc.TLS, pCur uintptr, pAmt uintptr) (r uintptr)
	_sqlite3BtreeFirst(tls *libc.TLS, pCur uintptr, pRes uintptr) (r int32)
	_sqlite3BtreeIsEmpty(tls *libc.TLS, pCur uintptr, pRes uintptr) (r int32)
	_sqlite3BtreeLast(tls *libc.TLS, pCur uintptr, pRes uintptr) (r int32)
	_sqlite3BtreeTableMoveto(tls *libc.TLS, pCur uintptr, intKey Ti64, biasRight int32, pRes uintptr) (r int32)
	_sqlite3BtreeIndexMoveto(tls *libc.TLS, pCur uintptr, pIdxKey uintptr, pRes uintptr) (r int32)
	_sqlite3BtreeEof(tls *libc.TLS, pCur uintptr) (r int32)
	_sqlite3BtreeRowCountEst(tls *libc.TLS, pCur uintptr) (r Ti64)
	_sqlite3BtreeNext(tls *libc.TLS, pCur uintptr, flags int32) (r int32)
	_sqlite3BtreePrevious(tls *libc.TLS, pCur uintptr, flags int32) (r int32)
	_sqlite3BtreeInsert(tls *libc.TLS, pCur uintptr, pX uintptr, flags int32, seekResult int32) (r int32)
	_sqlite3BtreeTransferRow(tls *libc.TLS, pDest uintptr, pSrc uintptr, iKey Ti64) (r int32)
	_sqlite3BtreeDelete(tls *libc.TLS, pCur uintptr, flags Tu8) (r int32)
	_sqlite3BtreeCreateTable(tls *libc.TLS, p uintptr, piTable uintptr, flags int32) (r int32)
	_sqlite3BtreeClearTable(tls *libc.TLS, p uintptr, iTable int32, pnChange uintptr) (r int32)
	_sqlite3BtreeClearTableOfCursor(tls *libc.TLS, pCur uintptr) (r int32)
	_sqlite3BtreeDropTable(tls *libc.TLS, p uintptr, iTable int32, piMoved uintptr) (r int32)
	_sqlite3BtreeGetMeta(tls *libc.TLS, p uintptr, idx int32, pMeta uintptr)
	_sqlite3BtreeUpdateMeta(tls *libc.TLS, p uintptr, idx int32, iMeta Tu32) (r int32)
	_sqlite3BtreeCount(tls *libc.TLS, db uintptr, pCur uintptr, pnEntry uintptr) (r int32)
	_sqlite3BtreePager(tls *libc.TLS, p uintptr) (r uintptr)
	_sqlite3BtreeIntegrityCheck(tls *libc.TLS, db uintptr, p uintptr, aRoot uintptr, aCnt uintptr, nRoot int32, mxErr int32, pnErr uintptr, pzOut uintptr) (r int32)
	_sqlite3BtreeGetFilename(tls *libc.TLS, p uintptr) (r uintptr)
	_sqlite3BtreeGetJournalname(tls *libc.TLS, p uintptr) (r uintptr)
	_sqlite3BtreeTxnState(tls *libc.TLS, p uintptr) (r int32)
	_sqlite3BtreeCheckpoint(tls *libc.TLS, p uintptr, eMode int32, pnLog uintptr, pnCkpt uintptr) (r int32)
	_sqlite3BtreeIsInBackup(tls *libc.TLS, p uintptr) (r int32)
	_sqlite3BtreeSchema(tls *libc.TLS, p uintptr, nBytes int32, __ccgo_fp_xFree uintptr) (r uintptr)
	_sqlite3BtreeSchemaLocked(tls *libc.TLS, p uintptr) (r int32)
	_sqlite3BtreeLockTable(tls *libc.TLS, p uintptr, iTab int32, isWriteLock Tu8) (r int32)
	_sqlite3BtreePutData(tls *libc.TLS, pCsr uintptr, offset Tu32, amt Tu32, z uintptr) (r int32)
	_sqlite3BtreeIncrblobCursor(tls *libc.TLS, pCur uintptr)
	_sqlite3BtreeSetVersion(tls *libc.TLS, pBtree uintptr, iVersion int32) (r int32)
	_sqlite3BtreeCursorHasHint(tls *libc.TLS, pCsr uintptr, mask uint32) (r int32)
	_sqlite3BtreeIsReadonly(tls *libc.TLS, p uintptr) (r int32)
	_sqlite3BtreeClearCache(tls *libc.TLS, p uintptr)
	_sqlite3BtreeSharable(tls *libc.TLS, p uintptr) (r int32)
	_sqlite3BtreeConnectionCount(tls *libc.TLS, p uintptr) (r int32)
	_sqlite3BtreeCopyFile(tls *libc.TLS, pTo uintptr, pFrom uintptr) (r int32)
}

type nativeBtreeStorageEngine struct{}

var currentStorageEngine storageEngine = nativeBtreeStorageEngine{}

func _sqlite3BtreeEnter(tls *libc.TLS, p uintptr) {
	currentStorageEngine._sqlite3BtreeEnter(tls, p)
}

func _sqlite3BtreeLeave(tls *libc.TLS, p uintptr) {
	currentStorageEngine._sqlite3BtreeLeave(tls, p)
}

func _sqlite3BtreeEnterAll(tls *libc.TLS, db uintptr) {
	currentStorageEngine._sqlite3BtreeEnterAll(tls, db)
}

func _sqlite3BtreeLeaveAll(tls *libc.TLS, db uintptr) {
	currentStorageEngine._sqlite3BtreeLeaveAll(tls, db)
}

func _sqlite3BtreeEnterCursor(tls *libc.TLS, pCur uintptr) {
	currentStorageEngine._sqlite3BtreeEnterCursor(tls, pCur)
}

func _sqlite3BtreeLeaveCursor(tls *libc.TLS, pCur uintptr) {
	currentStorageEngine._sqlite3BtreeLeaveCursor(tls, pCur)
}

func _sqlite3BtreeClearCursor(tls *libc.TLS, pCur uintptr) {
	currentStorageEngine._sqlite3BtreeClearCursor(tls, pCur)
}

func _sqlite3BtreeCursorHasMoved(tls *libc.TLS, pCur uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeCursorHasMoved(tls, pCur)
}

func _sqlite3BtreeFakeValidCursor(tls *libc.TLS) (r uintptr) {
	return currentStorageEngine._sqlite3BtreeFakeValidCursor(tls)
}

func _sqlite3BtreeCursorRestore(tls *libc.TLS, pCur uintptr, pDifferentRow uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeCursorRestore(tls, pCur, pDifferentRow)
}

func _sqlite3BtreeCursorHintFlags(tls *libc.TLS, pCur uintptr, x uint32) {
	currentStorageEngine._sqlite3BtreeCursorHintFlags(tls, pCur, x)
}

func _sqlite3BtreeLastPage(tls *libc.TLS, p uintptr) (r TPgno) {
	return currentStorageEngine._sqlite3BtreeLastPage(tls, p)
}

func _sqlite3BtreeOpen(tls *libc.TLS, pVfs uintptr, zFilename uintptr, db uintptr, ppBtree uintptr, flags int32, vfsFlags int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeOpen(tls, pVfs, zFilename, db, ppBtree, flags, vfsFlags)
}

func _sqlite3BtreeClose(tls *libc.TLS, p uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeClose(tls, p)
}

func _sqlite3BtreeSetCacheSize(tls *libc.TLS, p uintptr, mxPage int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeSetCacheSize(tls, p, mxPage)
}

func _sqlite3BtreeSetSpillSize(tls *libc.TLS, p uintptr, mxPage int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeSetSpillSize(tls, p, mxPage)
}

func _sqlite3BtreeSetMmapLimit(tls *libc.TLS, p uintptr, szMmap Tsqlite3_int64) (r int32) {
	return currentStorageEngine._sqlite3BtreeSetMmapLimit(tls, p, szMmap)
}

func _sqlite3BtreeSetPagerFlags(tls *libc.TLS, p uintptr, pgFlags uint32) (r int32) {
	return currentStorageEngine._sqlite3BtreeSetPagerFlags(tls, p, pgFlags)
}

func _sqlite3BtreeSetPageSize(tls *libc.TLS, p uintptr, pageSize int32, nReserve int32, iFix int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeSetPageSize(tls, p, pageSize, nReserve, iFix)
}

func _sqlite3BtreeGetPageSize(tls *libc.TLS, p uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeGetPageSize(tls, p)
}

func _sqlite3BtreeGetReserveNoMutex(tls *libc.TLS, p uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeGetReserveNoMutex(tls, p)
}

func _sqlite3BtreeGetRequestedReserve(tls *libc.TLS, p uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeGetRequestedReserve(tls, p)
}

func _sqlite3BtreeMaxPageCount(tls *libc.TLS, p uintptr, mxPage TPgno) (r TPgno) {
	return currentStorageEngine._sqlite3BtreeMaxPageCount(tls, p, mxPage)
}

func _sqlite3BtreeSecureDelete(tls *libc.TLS, p uintptr, newFlag int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeSecureDelete(tls, p, newFlag)
}

func _sqlite3BtreeSetAutoVacuum(tls *libc.TLS, p uintptr, autoVacuum int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeSetAutoVacuum(tls, p, autoVacuum)
}

func _sqlite3BtreeGetAutoVacuum(tls *libc.TLS, p uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeGetAutoVacuum(tls, p)
}

func _sqlite3BtreeNewDb(tls *libc.TLS, p uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeNewDb(tls, p)
}

func _sqlite3BtreeBeginTrans(tls *libc.TLS, p uintptr, wrflag int32, pSchemaVersion uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeBeginTrans(tls, p, wrflag, pSchemaVersion)
}

func _sqlite3BtreeIncrVacuum(tls *libc.TLS, p uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeIncrVacuum(tls, p)
}

func _sqlite3BtreeCommitPhaseOne(tls *libc.TLS, p uintptr, zSuperJrnl uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeCommitPhaseOne(tls, p, zSuperJrnl)
}

func _sqlite3BtreeCommitPhaseTwo(tls *libc.TLS, p uintptr, bCleanup int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeCommitPhaseTwo(tls, p, bCleanup)
}

func _sqlite3BtreeCommit(tls *libc.TLS, p uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeCommit(tls, p)
}

func _sqlite3BtreeTripAllCursors(tls *libc.TLS, pBtree uintptr, errCode int32, writeOnly int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeTripAllCursors(tls, pBtree, errCode, writeOnly)
}

func _sqlite3BtreeRollback(tls *libc.TLS, p uintptr, tripCode int32, writeOnly int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeRollback(tls, p, tripCode, writeOnly)
}

func _sqlite3BtreeBeginStmt(tls *libc.TLS, p uintptr, iStatement int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeBeginStmt(tls, p, iStatement)
}

func _sqlite3BtreeSavepoint(tls *libc.TLS, p uintptr, op int32, iSavepoint int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeSavepoint(tls, p, op, iSavepoint)
}

func _sqlite3BtreeCursor(tls *libc.TLS, p uintptr, iTable TPgno, wrFlag int32, pKeyInfo uintptr, pCur uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeCursor(tls, p, iTable, wrFlag, pKeyInfo, pCur)
}

func _sqlite3BtreeCursorSize(tls *libc.TLS) (r int32) {
	return currentStorageEngine._sqlite3BtreeCursorSize(tls)
}

func _sqlite3BtreeCursorZero(tls *libc.TLS, p uintptr) {
	currentStorageEngine._sqlite3BtreeCursorZero(tls, p)
}

func _sqlite3BtreeCloseCursor(tls *libc.TLS, pCur uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeCloseCursor(tls, pCur)
}

func _sqlite3BtreeCursorIsValidNN(tls *libc.TLS, pCur uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeCursorIsValidNN(tls, pCur)
}

func _sqlite3BtreeIntegerKey(tls *libc.TLS, pCur uintptr) (r Ti64) {
	return currentStorageEngine._sqlite3BtreeIntegerKey(tls, pCur)
}

func _sqlite3BtreeCursorPin(tls *libc.TLS, pCur uintptr) {
	currentStorageEngine._sqlite3BtreeCursorPin(tls, pCur)
}

func _sqlite3BtreeCursorUnpin(tls *libc.TLS, pCur uintptr) {
	currentStorageEngine._sqlite3BtreeCursorUnpin(tls, pCur)
}

func _sqlite3BtreeOffset(tls *libc.TLS, pCur uintptr) (r Ti64) {
	return currentStorageEngine._sqlite3BtreeOffset(tls, pCur)
}

func _sqlite3BtreePayloadSize(tls *libc.TLS, pCur uintptr) (r Tu32) {
	return currentStorageEngine._sqlite3BtreePayloadSize(tls, pCur)
}

func _sqlite3BtreeMaxRecordSize(tls *libc.TLS, pCur uintptr) (r Tsqlite3_int64) {
	return currentStorageEngine._sqlite3BtreeMaxRecordSize(tls, pCur)
}

func _sqlite3BtreePayload(tls *libc.TLS, pCur uintptr, offset Tu32, amt Tu32, pBuf uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreePayload(tls, pCur, offset, amt, pBuf)
}

func _sqlite3BtreePayloadChecked(tls *libc.TLS, pCur uintptr, offset Tu32, amt Tu32, pBuf uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreePayloadChecked(tls, pCur, offset, amt, pBuf)
}

func _sqlite3BtreePayloadFetch(tls *libc.TLS, pCur uintptr, pAmt uintptr) (r uintptr) {
	return currentStorageEngine._sqlite3BtreePayloadFetch(tls, pCur, pAmt)
}

func _sqlite3BtreeFirst(tls *libc.TLS, pCur uintptr, pRes uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeFirst(tls, pCur, pRes)
}

func _sqlite3BtreeIsEmpty(tls *libc.TLS, pCur uintptr, pRes uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeIsEmpty(tls, pCur, pRes)
}

func _sqlite3BtreeLast(tls *libc.TLS, pCur uintptr, pRes uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeLast(tls, pCur, pRes)
}

func _sqlite3BtreeTableMoveto(tls *libc.TLS, pCur uintptr, intKey Ti64, biasRight int32, pRes uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeTableMoveto(tls, pCur, intKey, biasRight, pRes)
}

func _sqlite3BtreeIndexMoveto(tls *libc.TLS, pCur uintptr, pIdxKey uintptr, pRes uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeIndexMoveto(tls, pCur, pIdxKey, pRes)
}

func _sqlite3BtreeEof(tls *libc.TLS, pCur uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeEof(tls, pCur)
}

func _sqlite3BtreeRowCountEst(tls *libc.TLS, pCur uintptr) (r Ti64) {
	return currentStorageEngine._sqlite3BtreeRowCountEst(tls, pCur)
}

func _sqlite3BtreeNext(tls *libc.TLS, pCur uintptr, flags int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeNext(tls, pCur, flags)
}

func _sqlite3BtreePrevious(tls *libc.TLS, pCur uintptr, flags int32) (r int32) {
	return currentStorageEngine._sqlite3BtreePrevious(tls, pCur, flags)
}

func _sqlite3BtreeInsert(tls *libc.TLS, pCur uintptr, pX uintptr, flags int32, seekResult int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeInsert(tls, pCur, pX, flags, seekResult)
}

func _sqlite3BtreeTransferRow(tls *libc.TLS, pDest uintptr, pSrc uintptr, iKey Ti64) (r int32) {
	return currentStorageEngine._sqlite3BtreeTransferRow(tls, pDest, pSrc, iKey)
}

func _sqlite3BtreeDelete(tls *libc.TLS, pCur uintptr, flags Tu8) (r int32) {
	return currentStorageEngine._sqlite3BtreeDelete(tls, pCur, flags)
}

func _sqlite3BtreeCreateTable(tls *libc.TLS, p uintptr, piTable uintptr, flags int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeCreateTable(tls, p, piTable, flags)
}

func _sqlite3BtreeClearTable(tls *libc.TLS, p uintptr, iTable int32, pnChange uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeClearTable(tls, p, iTable, pnChange)
}

func _sqlite3BtreeClearTableOfCursor(tls *libc.TLS, pCur uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeClearTableOfCursor(tls, pCur)
}

func _sqlite3BtreeDropTable(tls *libc.TLS, p uintptr, iTable int32, piMoved uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeDropTable(tls, p, iTable, piMoved)
}

func _sqlite3BtreeGetMeta(tls *libc.TLS, p uintptr, idx int32, pMeta uintptr) {
	currentStorageEngine._sqlite3BtreeGetMeta(tls, p, idx, pMeta)
}

func _sqlite3BtreeUpdateMeta(tls *libc.TLS, p uintptr, idx int32, iMeta Tu32) (r int32) {
	return currentStorageEngine._sqlite3BtreeUpdateMeta(tls, p, idx, iMeta)
}

func _sqlite3BtreeCount(tls *libc.TLS, db uintptr, pCur uintptr, pnEntry uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeCount(tls, db, pCur, pnEntry)
}

func _sqlite3BtreePager(tls *libc.TLS, p uintptr) (r uintptr) {
	return currentStorageEngine._sqlite3BtreePager(tls, p)
}

func _sqlite3BtreeIntegrityCheck(tls *libc.TLS, db uintptr, p uintptr, aRoot uintptr, aCnt uintptr, nRoot int32, mxErr int32, pnErr uintptr, pzOut uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeIntegrityCheck(tls, db, p, aRoot, aCnt, nRoot, mxErr, pnErr, pzOut)
}

func _sqlite3BtreeGetFilename(tls *libc.TLS, p uintptr) (r uintptr) {
	return currentStorageEngine._sqlite3BtreeGetFilename(tls, p)
}

func _sqlite3BtreeGetJournalname(tls *libc.TLS, p uintptr) (r uintptr) {
	return currentStorageEngine._sqlite3BtreeGetJournalname(tls, p)
}

func _sqlite3BtreeTxnState(tls *libc.TLS, p uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeTxnState(tls, p)
}

func _sqlite3BtreeCheckpoint(tls *libc.TLS, p uintptr, eMode int32, pnLog uintptr, pnCkpt uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeCheckpoint(tls, p, eMode, pnLog, pnCkpt)
}

func _sqlite3BtreeIsInBackup(tls *libc.TLS, p uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeIsInBackup(tls, p)
}

func _sqlite3BtreeSchema(tls *libc.TLS, p uintptr, nBytes int32, __ccgo_fp_xFree uintptr) (r uintptr) {
	return currentStorageEngine._sqlite3BtreeSchema(tls, p, nBytes, __ccgo_fp_xFree)
}

func _sqlite3BtreeSchemaLocked(tls *libc.TLS, p uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeSchemaLocked(tls, p)
}

func _sqlite3BtreeLockTable(tls *libc.TLS, p uintptr, iTab int32, isWriteLock Tu8) (r int32) {
	return currentStorageEngine._sqlite3BtreeLockTable(tls, p, iTab, isWriteLock)
}

func _sqlite3BtreePutData(tls *libc.TLS, pCsr uintptr, offset Tu32, amt Tu32, z uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreePutData(tls, pCsr, offset, amt, z)
}

func _sqlite3BtreeIncrblobCursor(tls *libc.TLS, pCur uintptr) {
	currentStorageEngine._sqlite3BtreeIncrblobCursor(tls, pCur)
}

func _sqlite3BtreeSetVersion(tls *libc.TLS, pBtree uintptr, iVersion int32) (r int32) {
	return currentStorageEngine._sqlite3BtreeSetVersion(tls, pBtree, iVersion)
}

func _sqlite3BtreeCursorHasHint(tls *libc.TLS, pCsr uintptr, mask uint32) (r int32) {
	return currentStorageEngine._sqlite3BtreeCursorHasHint(tls, pCsr, mask)
}

func _sqlite3BtreeIsReadonly(tls *libc.TLS, p uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeIsReadonly(tls, p)
}

func _sqlite3BtreeClearCache(tls *libc.TLS, p uintptr) {
	currentStorageEngine._sqlite3BtreeClearCache(tls, p)
}

func _sqlite3BtreeSharable(tls *libc.TLS, p uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeSharable(tls, p)
}

func _sqlite3BtreeConnectionCount(tls *libc.TLS, p uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeConnectionCount(tls, p)
}

func _sqlite3BtreeCopyFile(tls *libc.TLS, pTo uintptr, pFrom uintptr) (r int32) {
	return currentStorageEngine._sqlite3BtreeCopyFile(tls, pTo, pFrom)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeEnter(tls *libc.TLS, p uintptr) {
	_nativeSqlite3BtreeEnter(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeLeave(tls *libc.TLS, p uintptr) {
	_nativeSqlite3BtreeLeave(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeEnterAll(tls *libc.TLS, db uintptr) {
	_nativeSqlite3BtreeEnterAll(tls, db)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeLeaveAll(tls *libc.TLS, db uintptr) {
	_nativeSqlite3BtreeLeaveAll(tls, db)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeEnterCursor(tls *libc.TLS, pCur uintptr) {
	_nativeSqlite3BtreeEnterCursor(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeLeaveCursor(tls *libc.TLS, pCur uintptr) {
	_nativeSqlite3BtreeLeaveCursor(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeClearCursor(tls *libc.TLS, pCur uintptr) {
	_nativeSqlite3BtreeClearCursor(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCursorHasMoved(tls *libc.TLS, pCur uintptr) (r int32) {
	return _nativeSqlite3BtreeCursorHasMoved(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeFakeValidCursor(tls *libc.TLS) (r uintptr) {
	return _nativeSqlite3BtreeFakeValidCursor(tls)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCursorRestore(tls *libc.TLS, pCur uintptr, pDifferentRow uintptr) (r int32) {
	return _nativeSqlite3BtreeCursorRestore(tls, pCur, pDifferentRow)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCursorHintFlags(tls *libc.TLS, pCur uintptr, x uint32) {
	_nativeSqlite3BtreeCursorHintFlags(tls, pCur, x)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeLastPage(tls *libc.TLS, p uintptr) (r TPgno) {
	return _nativeSqlite3BtreeLastPage(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeOpen(tls *libc.TLS, pVfs uintptr, zFilename uintptr, db uintptr, ppBtree uintptr, flags int32, vfsFlags int32) (r int32) {
	return _nativeSqlite3BtreeOpen(tls, pVfs, zFilename, db, ppBtree, flags, vfsFlags)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeClose(tls *libc.TLS, p uintptr) (r int32) {
	return _nativeSqlite3BtreeClose(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeSetCacheSize(tls *libc.TLS, p uintptr, mxPage int32) (r int32) {
	return _nativeSqlite3BtreeSetCacheSize(tls, p, mxPage)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeSetSpillSize(tls *libc.TLS, p uintptr, mxPage int32) (r int32) {
	return _nativeSqlite3BtreeSetSpillSize(tls, p, mxPage)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeSetMmapLimit(tls *libc.TLS, p uintptr, szMmap Tsqlite3_int64) (r int32) {
	return _nativeSqlite3BtreeSetMmapLimit(tls, p, szMmap)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeSetPagerFlags(tls *libc.TLS, p uintptr, pgFlags uint32) (r int32) {
	return _nativeSqlite3BtreeSetPagerFlags(tls, p, pgFlags)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeSetPageSize(tls *libc.TLS, p uintptr, pageSize int32, nReserve int32, iFix int32) (r int32) {
	return _nativeSqlite3BtreeSetPageSize(tls, p, pageSize, nReserve, iFix)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeGetPageSize(tls *libc.TLS, p uintptr) (r int32) {
	return _nativeSqlite3BtreeGetPageSize(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeGetReserveNoMutex(tls *libc.TLS, p uintptr) (r int32) {
	return _nativeSqlite3BtreeGetReserveNoMutex(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeGetRequestedReserve(tls *libc.TLS, p uintptr) (r int32) {
	return _nativeSqlite3BtreeGetRequestedReserve(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeMaxPageCount(tls *libc.TLS, p uintptr, mxPage TPgno) (r TPgno) {
	return _nativeSqlite3BtreeMaxPageCount(tls, p, mxPage)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeSecureDelete(tls *libc.TLS, p uintptr, newFlag int32) (r int32) {
	return _nativeSqlite3BtreeSecureDelete(tls, p, newFlag)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeSetAutoVacuum(tls *libc.TLS, p uintptr, autoVacuum int32) (r int32) {
	return _nativeSqlite3BtreeSetAutoVacuum(tls, p, autoVacuum)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeGetAutoVacuum(tls *libc.TLS, p uintptr) (r int32) {
	return _nativeSqlite3BtreeGetAutoVacuum(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeNewDb(tls *libc.TLS, p uintptr) (r int32) {
	return _nativeSqlite3BtreeNewDb(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeBeginTrans(tls *libc.TLS, p uintptr, wrflag int32, pSchemaVersion uintptr) (r int32) {
	return _nativeSqlite3BtreeBeginTrans(tls, p, wrflag, pSchemaVersion)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeIncrVacuum(tls *libc.TLS, p uintptr) (r int32) {
	return _nativeSqlite3BtreeIncrVacuum(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCommitPhaseOne(tls *libc.TLS, p uintptr, zSuperJrnl uintptr) (r int32) {
	return _nativeSqlite3BtreeCommitPhaseOne(tls, p, zSuperJrnl)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCommitPhaseTwo(tls *libc.TLS, p uintptr, bCleanup int32) (r int32) {
	return _nativeSqlite3BtreeCommitPhaseTwo(tls, p, bCleanup)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCommit(tls *libc.TLS, p uintptr) (r int32) {
	return _nativeSqlite3BtreeCommit(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeTripAllCursors(tls *libc.TLS, pBtree uintptr, errCode int32, writeOnly int32) (r int32) {
	return _nativeSqlite3BtreeTripAllCursors(tls, pBtree, errCode, writeOnly)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeRollback(tls *libc.TLS, p uintptr, tripCode int32, writeOnly int32) (r int32) {
	return _nativeSqlite3BtreeRollback(tls, p, tripCode, writeOnly)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeBeginStmt(tls *libc.TLS, p uintptr, iStatement int32) (r int32) {
	return _nativeSqlite3BtreeBeginStmt(tls, p, iStatement)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeSavepoint(tls *libc.TLS, p uintptr, op int32, iSavepoint int32) (r int32) {
	return _nativeSqlite3BtreeSavepoint(tls, p, op, iSavepoint)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCursor(tls *libc.TLS, p uintptr, iTable TPgno, wrFlag int32, pKeyInfo uintptr, pCur uintptr) (r int32) {
	return _nativeSqlite3BtreeCursor(tls, p, iTable, wrFlag, pKeyInfo, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCursorSize(tls *libc.TLS) (r int32) {
	return _nativeSqlite3BtreeCursorSize(tls)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCursorZero(tls *libc.TLS, p uintptr) {
	_nativeSqlite3BtreeCursorZero(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCloseCursor(tls *libc.TLS, pCur uintptr) (r int32) {
	return _nativeSqlite3BtreeCloseCursor(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCursorIsValidNN(tls *libc.TLS, pCur uintptr) (r int32) {
	return _nativeSqlite3BtreeCursorIsValidNN(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeIntegerKey(tls *libc.TLS, pCur uintptr) (r Ti64) {
	return _nativeSqlite3BtreeIntegerKey(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCursorPin(tls *libc.TLS, pCur uintptr) {
	_nativeSqlite3BtreeCursorPin(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCursorUnpin(tls *libc.TLS, pCur uintptr) {
	_nativeSqlite3BtreeCursorUnpin(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeOffset(tls *libc.TLS, pCur uintptr) (r Ti64) {
	return _nativeSqlite3BtreeOffset(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreePayloadSize(tls *libc.TLS, pCur uintptr) (r Tu32) {
	return _nativeSqlite3BtreePayloadSize(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeMaxRecordSize(tls *libc.TLS, pCur uintptr) (r Tsqlite3_int64) {
	return _nativeSqlite3BtreeMaxRecordSize(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreePayload(tls *libc.TLS, pCur uintptr, offset Tu32, amt Tu32, pBuf uintptr) (r int32) {
	return _nativeSqlite3BtreePayload(tls, pCur, offset, amt, pBuf)
}

func (nativeBtreeStorageEngine) _sqlite3BtreePayloadChecked(tls *libc.TLS, pCur uintptr, offset Tu32, amt Tu32, pBuf uintptr) (r int32) {
	return _nativeSqlite3BtreePayloadChecked(tls, pCur, offset, amt, pBuf)
}

func (nativeBtreeStorageEngine) _sqlite3BtreePayloadFetch(tls *libc.TLS, pCur uintptr, pAmt uintptr) (r uintptr) {
	return _nativeSqlite3BtreePayloadFetch(tls, pCur, pAmt)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeFirst(tls *libc.TLS, pCur uintptr, pRes uintptr) (r int32) {
	return _nativeSqlite3BtreeFirst(tls, pCur, pRes)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeIsEmpty(tls *libc.TLS, pCur uintptr, pRes uintptr) (r int32) {
	return _nativeSqlite3BtreeIsEmpty(tls, pCur, pRes)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeLast(tls *libc.TLS, pCur uintptr, pRes uintptr) (r int32) {
	return _nativeSqlite3BtreeLast(tls, pCur, pRes)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeTableMoveto(tls *libc.TLS, pCur uintptr, intKey Ti64, biasRight int32, pRes uintptr) (r int32) {
	return _nativeSqlite3BtreeTableMoveto(tls, pCur, intKey, biasRight, pRes)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeIndexMoveto(tls *libc.TLS, pCur uintptr, pIdxKey uintptr, pRes uintptr) (r int32) {
	return _nativeSqlite3BtreeIndexMoveto(tls, pCur, pIdxKey, pRes)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeEof(tls *libc.TLS, pCur uintptr) (r int32) {
	return _nativeSqlite3BtreeEof(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeRowCountEst(tls *libc.TLS, pCur uintptr) (r Ti64) {
	return _nativeSqlite3BtreeRowCountEst(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeNext(tls *libc.TLS, pCur uintptr, flags int32) (r int32) {
	return _nativeSqlite3BtreeNext(tls, pCur, flags)
}

func (nativeBtreeStorageEngine) _sqlite3BtreePrevious(tls *libc.TLS, pCur uintptr, flags int32) (r int32) {
	return _nativeSqlite3BtreePrevious(tls, pCur, flags)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeInsert(tls *libc.TLS, pCur uintptr, pX uintptr, flags int32, seekResult int32) (r int32) {
	return _nativeSqlite3BtreeInsert(tls, pCur, pX, flags, seekResult)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeTransferRow(tls *libc.TLS, pDest uintptr, pSrc uintptr, iKey Ti64) (r int32) {
	return _nativeSqlite3BtreeTransferRow(tls, pDest, pSrc, iKey)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeDelete(tls *libc.TLS, pCur uintptr, flags Tu8) (r int32) {
	return _nativeSqlite3BtreeDelete(tls, pCur, flags)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCreateTable(tls *libc.TLS, p uintptr, piTable uintptr, flags int32) (r int32) {
	return _nativeSqlite3BtreeCreateTable(tls, p, piTable, flags)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeClearTable(tls *libc.TLS, p uintptr, iTable int32, pnChange uintptr) (r int32) {
	return _nativeSqlite3BtreeClearTable(tls, p, iTable, pnChange)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeClearTableOfCursor(tls *libc.TLS, pCur uintptr) (r int32) {
	return _nativeSqlite3BtreeClearTableOfCursor(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeDropTable(tls *libc.TLS, p uintptr, iTable int32, piMoved uintptr) (r int32) {
	return _nativeSqlite3BtreeDropTable(tls, p, iTable, piMoved)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeGetMeta(tls *libc.TLS, p uintptr, idx int32, pMeta uintptr) {
	_nativeSqlite3BtreeGetMeta(tls, p, idx, pMeta)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeUpdateMeta(tls *libc.TLS, p uintptr, idx int32, iMeta Tu32) (r int32) {
	return _nativeSqlite3BtreeUpdateMeta(tls, p, idx, iMeta)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCount(tls *libc.TLS, db uintptr, pCur uintptr, pnEntry uintptr) (r int32) {
	return _nativeSqlite3BtreeCount(tls, db, pCur, pnEntry)
}

func (nativeBtreeStorageEngine) _sqlite3BtreePager(tls *libc.TLS, p uintptr) (r uintptr) {
	return _nativeSqlite3BtreePager(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeIntegrityCheck(tls *libc.TLS, db uintptr, p uintptr, aRoot uintptr, aCnt uintptr, nRoot int32, mxErr int32, pnErr uintptr, pzOut uintptr) (r int32) {
	return _nativeSqlite3BtreeIntegrityCheck(tls, db, p, aRoot, aCnt, nRoot, mxErr, pnErr, pzOut)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeGetFilename(tls *libc.TLS, p uintptr) (r uintptr) {
	return _nativeSqlite3BtreeGetFilename(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeGetJournalname(tls *libc.TLS, p uintptr) (r uintptr) {
	return _nativeSqlite3BtreeGetJournalname(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeTxnState(tls *libc.TLS, p uintptr) (r int32) {
	return _nativeSqlite3BtreeTxnState(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCheckpoint(tls *libc.TLS, p uintptr, eMode int32, pnLog uintptr, pnCkpt uintptr) (r int32) {
	return _nativeSqlite3BtreeCheckpoint(tls, p, eMode, pnLog, pnCkpt)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeIsInBackup(tls *libc.TLS, p uintptr) (r int32) {
	return _nativeSqlite3BtreeIsInBackup(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeSchema(tls *libc.TLS, p uintptr, nBytes int32, __ccgo_fp_xFree uintptr) (r uintptr) {
	return _nativeSqlite3BtreeSchema(tls, p, nBytes, __ccgo_fp_xFree)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeSchemaLocked(tls *libc.TLS, p uintptr) (r int32) {
	return _nativeSqlite3BtreeSchemaLocked(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeLockTable(tls *libc.TLS, p uintptr, iTab int32, isWriteLock Tu8) (r int32) {
	return _nativeSqlite3BtreeLockTable(tls, p, iTab, isWriteLock)
}

func (nativeBtreeStorageEngine) _sqlite3BtreePutData(tls *libc.TLS, pCsr uintptr, offset Tu32, amt Tu32, z uintptr) (r int32) {
	return _nativeSqlite3BtreePutData(tls, pCsr, offset, amt, z)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeIncrblobCursor(tls *libc.TLS, pCur uintptr) {
	_nativeSqlite3BtreeIncrblobCursor(tls, pCur)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeSetVersion(tls *libc.TLS, pBtree uintptr, iVersion int32) (r int32) {
	return _nativeSqlite3BtreeSetVersion(tls, pBtree, iVersion)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCursorHasHint(tls *libc.TLS, pCsr uintptr, mask uint32) (r int32) {
	return _nativeSqlite3BtreeCursorHasHint(tls, pCsr, mask)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeIsReadonly(tls *libc.TLS, p uintptr) (r int32) {
	return _nativeSqlite3BtreeIsReadonly(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeClearCache(tls *libc.TLS, p uintptr) {
	_nativeSqlite3BtreeClearCache(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeSharable(tls *libc.TLS, p uintptr) (r int32) {
	return _nativeSqlite3BtreeSharable(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeConnectionCount(tls *libc.TLS, p uintptr) (r int32) {
	return _nativeSqlite3BtreeConnectionCount(tls, p)
}

func (nativeBtreeStorageEngine) _sqlite3BtreeCopyFile(tls *libc.TLS, pTo uintptr, pFrom uintptr) (r int32) {
	return _nativeSqlite3BtreeCopyFile(tls, pTo, pFrom)
}
