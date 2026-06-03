// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite3

import "modernc.org/libc"

// storageEngine is the btree-facing storage abstraction. The default
// implementation delegates to the generated SQLite btree code.
type storageEngine interface {
	Xsqlite3BtreeEnter(tls *libc.TLS, p uintptr)
	Xsqlite3BtreeLeave(tls *libc.TLS, p uintptr)
	Xsqlite3BtreeEnterAll(tls *libc.TLS, db uintptr)
	Xsqlite3BtreeLeaveAll(tls *libc.TLS, db uintptr)
	Xsqlite3BtreeEnterCursor(tls *libc.TLS, pCur uintptr)
	Xsqlite3BtreeLeaveCursor(tls *libc.TLS, pCur uintptr)
	Xsqlite3BtreeClearCursor(tls *libc.TLS, pCur uintptr)
	Xsqlite3BtreeCursorHasMoved(tls *libc.TLS, pCur uintptr) int32
	Xsqlite3BtreeFakeValidCursor(tls *libc.TLS) uintptr
	Xsqlite3BtreeCursorRestore(tls *libc.TLS, pCur uintptr, pDifferentRow uintptr) int32
	Xsqlite3BtreeCursorHintFlags(tls *libc.TLS, pCur uintptr, x uint32)
	Xsqlite3BtreeLastPage(tls *libc.TLS, p uintptr) Pgno
	Xsqlite3BtreeOpen(tls *libc.TLS, pVfs uintptr, zFilename uintptr, db uintptr, ppBtree uintptr, flags int32, vfsFlags int32) int32
	Xsqlite3BtreeClose(tls *libc.TLS, p uintptr) int32
	Xsqlite3BtreeSetCacheSize(tls *libc.TLS, p uintptr, mxPage int32) int32
	Xsqlite3BtreeSetSpillSize(tls *libc.TLS, p uintptr, mxPage int32) int32
	Xsqlite3BtreeSetPagerFlags(tls *libc.TLS, p uintptr, pgFlags uint32) int32
	Xsqlite3BtreeSetPageSize(tls *libc.TLS, p uintptr, pageSize int32, nReserve int32, iFix int32) int32
	Xsqlite3BtreeGetPageSize(tls *libc.TLS, p uintptr) int32
	Xsqlite3BtreeGetReserveNoMutex(tls *libc.TLS, p uintptr) int32
	Xsqlite3BtreeGetRequestedReserve(tls *libc.TLS, p uintptr) int32
	Xsqlite3BtreeMaxPageCount(tls *libc.TLS, p uintptr, mxPage Pgno) Pgno
	Xsqlite3BtreeSecureDelete(tls *libc.TLS, p uintptr, newFlag int32) int32
	Xsqlite3BtreeSetAutoVacuum(tls *libc.TLS, p uintptr, autoVacuum int32) int32
	Xsqlite3BtreeGetAutoVacuum(tls *libc.TLS, p uintptr) int32
	Xsqlite3BtreeNewDb(tls *libc.TLS, p uintptr) int32
	Xsqlite3BtreeBeginTrans(tls *libc.TLS, p uintptr, wrflag int32, pSchemaVersion uintptr) int32
	Xsqlite3BtreeIncrVacuum(tls *libc.TLS, p uintptr) int32
	Xsqlite3BtreeCommitPhaseOne(tls *libc.TLS, p uintptr, zSuperJrnl uintptr) int32
	Xsqlite3BtreeCommitPhaseTwo(tls *libc.TLS, p uintptr, bCleanup int32) int32
	Xsqlite3BtreeCommit(tls *libc.TLS, p uintptr) int32
	Xsqlite3BtreeTripAllCursors(tls *libc.TLS, pBtree uintptr, errCode int32, writeOnly int32) int32
	Xsqlite3BtreeRollback(tls *libc.TLS, p uintptr, tripCode int32, writeOnly int32) int32
	Xsqlite3BtreeBeginStmt(tls *libc.TLS, p uintptr, iStatement int32) int32
	Xsqlite3BtreeSavepoint(tls *libc.TLS, p uintptr, op int32, iSavepoint int32) int32
	Xsqlite3BtreeCursor(tls *libc.TLS, p uintptr, iTable Pgno, wrFlag int32, pKeyInfo uintptr, pCur uintptr) int32
	Xsqlite3BtreeCursorSize(tls *libc.TLS) int32
	Xsqlite3BtreeCursorZero(tls *libc.TLS, p uintptr)
	Xsqlite3BtreeCloseCursor(tls *libc.TLS, pCur uintptr) int32
	Xsqlite3BtreeCursorIsValidNN(tls *libc.TLS, pCur uintptr) int32
	Xsqlite3BtreeIntegerKey(tls *libc.TLS, pCur uintptr) I64
	Xsqlite3BtreeCursorPin(tls *libc.TLS, pCur uintptr)
	Xsqlite3BtreeCursorUnpin(tls *libc.TLS, pCur uintptr)
	Xsqlite3BtreeOffset(tls *libc.TLS, pCur uintptr) I64
	Xsqlite3BtreePayloadSize(tls *libc.TLS, pCur uintptr) U32
	Xsqlite3BtreeMaxRecordSize(tls *libc.TLS, pCur uintptr) Sqlite3_int64
	Xsqlite3BtreePayload(tls *libc.TLS, pCur uintptr, offset U32, amt U32, pBuf uintptr) int32
	Xsqlite3BtreePayloadChecked(tls *libc.TLS, pCur uintptr, offset U32, amt U32, pBuf uintptr) int32
	Xsqlite3BtreePayloadFetch(tls *libc.TLS, pCur uintptr, pAmt uintptr) uintptr
	Xsqlite3BtreeFirst(tls *libc.TLS, pCur uintptr, pRes uintptr) int32
	Xsqlite3BtreeLast(tls *libc.TLS, pCur uintptr, pRes uintptr) int32
	Xsqlite3BtreeTableMoveto(tls *libc.TLS, pCur uintptr, intKey I64, biasRight int32, pRes uintptr) int32
	Xsqlite3BtreeIndexMoveto(tls *libc.TLS, pCur uintptr, pIdxKey uintptr, pRes uintptr) int32
	Xsqlite3BtreeEof(tls *libc.TLS, pCur uintptr) int32
	Xsqlite3BtreeRowCountEst(tls *libc.TLS, pCur uintptr) I64
	Xsqlite3BtreeNext(tls *libc.TLS, pCur uintptr, flags int32) int32
	Xsqlite3BtreePrevious(tls *libc.TLS, pCur uintptr, flags int32) int32
	Xsqlite3BtreeInsert(tls *libc.TLS, pCur uintptr, pX uintptr, flags int32, seekResult int32) int32
	Xsqlite3BtreeTransferRow(tls *libc.TLS, pDest uintptr, pSrc uintptr, iKey I64) int32
	Xsqlite3BtreeDelete(tls *libc.TLS, pCur uintptr, flags U8) int32
	Xsqlite3BtreeCreateTable(tls *libc.TLS, p uintptr, piTable uintptr, flags int32) int32
	Xsqlite3BtreeClearTable(tls *libc.TLS, p uintptr, iTable int32, pnChange uintptr) int32
	Xsqlite3BtreeClearTableOfCursor(tls *libc.TLS, pCur uintptr) int32
	Xsqlite3BtreeDropTable(tls *libc.TLS, p uintptr, iTable int32, piMoved uintptr) int32
	Xsqlite3BtreeGetMeta(tls *libc.TLS, p uintptr, idx int32, pMeta uintptr)
	Xsqlite3BtreeUpdateMeta(tls *libc.TLS, p uintptr, idx int32, iMeta U32) int32
	Xsqlite3BtreeCount(tls *libc.TLS, db uintptr, pCur uintptr, pnEntry uintptr) int32
	Xsqlite3BtreePager(tls *libc.TLS, p uintptr) uintptr
	Xsqlite3BtreeIntegrityCheck(tls *libc.TLS, db uintptr, p uintptr, aRoot uintptr, nRoot int32, mxErr int32, pnErr uintptr) uintptr
	Xsqlite3BtreeGetFilename(tls *libc.TLS, p uintptr) uintptr
	Xsqlite3BtreeGetJournalname(tls *libc.TLS, p uintptr) uintptr
	Xsqlite3BtreeTxnState(tls *libc.TLS, p uintptr) int32
	Xsqlite3BtreeCheckpoint(tls *libc.TLS, p uintptr, eMode int32, pnLog uintptr, pnCkpt uintptr) int32
	Xsqlite3BtreeIsInBackup(tls *libc.TLS, p uintptr) int32
	Xsqlite3BtreeSchema(tls *libc.TLS, p uintptr, nBytes int32, xFree uintptr) uintptr
	Xsqlite3BtreeSchemaLocked(tls *libc.TLS, p uintptr) int32
	Xsqlite3BtreeLockTable(tls *libc.TLS, p uintptr, iTab int32, isWriteLock U8) int32
	Xsqlite3BtreePutData(tls *libc.TLS, pCsr uintptr, offset U32, amt U32, z uintptr) int32
	Xsqlite3BtreeIncrblobCursor(tls *libc.TLS, pCur uintptr)
	Xsqlite3BtreeSetVersion(tls *libc.TLS, pBtree uintptr, iVersion int32) int32
	Xsqlite3BtreeCursorHasHint(tls *libc.TLS, pCsr uintptr, mask uint32) int32
	Xsqlite3BtreeIsReadonly(tls *libc.TLS, p uintptr) int32
	Xsqlite3BtreeSharable(tls *libc.TLS, p uintptr) int32
	Xsqlite3BtreeConnectionCount(tls *libc.TLS, p uintptr) int32
	Xsqlite3BtreeCopyFile(tls *libc.TLS, pTo uintptr, pFrom uintptr) int32
}

type nativeBtreeStorageEngine struct{}

var currentStorageEngine storageEngine = nativeBtreeStorageEngine{}

func Xsqlite3BtreeEnter(tls *libc.TLS, p uintptr) {
	currentStorageEngine.Xsqlite3BtreeEnter(tls, p)
}

func Xsqlite3BtreeLeave(tls *libc.TLS, p uintptr) {
	currentStorageEngine.Xsqlite3BtreeLeave(tls, p)
}

func Xsqlite3BtreeEnterAll(tls *libc.TLS, db uintptr) {
	currentStorageEngine.Xsqlite3BtreeEnterAll(tls, db)
}

func Xsqlite3BtreeLeaveAll(tls *libc.TLS, db uintptr) {
	currentStorageEngine.Xsqlite3BtreeLeaveAll(tls, db)
}

func Xsqlite3BtreeEnterCursor(tls *libc.TLS, pCur uintptr) {
	currentStorageEngine.Xsqlite3BtreeEnterCursor(tls, pCur)
}

func Xsqlite3BtreeLeaveCursor(tls *libc.TLS, pCur uintptr) {
	currentStorageEngine.Xsqlite3BtreeLeaveCursor(tls, pCur)
}

func Xsqlite3BtreeClearCursor(tls *libc.TLS, pCur uintptr) {
	currentStorageEngine.Xsqlite3BtreeClearCursor(tls, pCur)
}

func Xsqlite3BtreeCursorHasMoved(tls *libc.TLS, pCur uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeCursorHasMoved(tls, pCur)
}

func Xsqlite3BtreeFakeValidCursor(tls *libc.TLS) uintptr {
	return currentStorageEngine.Xsqlite3BtreeFakeValidCursor(tls)
}

func Xsqlite3BtreeCursorRestore(tls *libc.TLS, pCur uintptr, pDifferentRow uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeCursorRestore(tls, pCur, pDifferentRow)
}

func Xsqlite3BtreeCursorHintFlags(tls *libc.TLS, pCur uintptr, x uint32) {
	currentStorageEngine.Xsqlite3BtreeCursorHintFlags(tls, pCur, x)
}

func Xsqlite3BtreeLastPage(tls *libc.TLS, p uintptr) Pgno {
	return currentStorageEngine.Xsqlite3BtreeLastPage(tls, p)
}

func Xsqlite3BtreeOpen(tls *libc.TLS, pVfs uintptr, zFilename uintptr, db uintptr, ppBtree uintptr, flags int32, vfsFlags int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeOpen(tls, pVfs, zFilename, db, ppBtree, flags, vfsFlags)
}

func Xsqlite3BtreeClose(tls *libc.TLS, p uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeClose(tls, p)
}

func Xsqlite3BtreeSetCacheSize(tls *libc.TLS, p uintptr, mxPage int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeSetCacheSize(tls, p, mxPage)
}

func Xsqlite3BtreeSetSpillSize(tls *libc.TLS, p uintptr, mxPage int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeSetSpillSize(tls, p, mxPage)
}

func Xsqlite3BtreeSetPagerFlags(tls *libc.TLS, p uintptr, pgFlags uint32) int32 {
	return currentStorageEngine.Xsqlite3BtreeSetPagerFlags(tls, p, pgFlags)
}

func Xsqlite3BtreeSetPageSize(tls *libc.TLS, p uintptr, pageSize int32, nReserve int32, iFix int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeSetPageSize(tls, p, pageSize, nReserve, iFix)
}

func Xsqlite3BtreeGetPageSize(tls *libc.TLS, p uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeGetPageSize(tls, p)
}

func Xsqlite3BtreeGetReserveNoMutex(tls *libc.TLS, p uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeGetReserveNoMutex(tls, p)
}

func Xsqlite3BtreeGetRequestedReserve(tls *libc.TLS, p uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeGetRequestedReserve(tls, p)
}

func Xsqlite3BtreeMaxPageCount(tls *libc.TLS, p uintptr, mxPage Pgno) Pgno {
	return currentStorageEngine.Xsqlite3BtreeMaxPageCount(tls, p, mxPage)
}

func Xsqlite3BtreeSecureDelete(tls *libc.TLS, p uintptr, newFlag int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeSecureDelete(tls, p, newFlag)
}

func Xsqlite3BtreeSetAutoVacuum(tls *libc.TLS, p uintptr, autoVacuum int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeSetAutoVacuum(tls, p, autoVacuum)
}

func Xsqlite3BtreeGetAutoVacuum(tls *libc.TLS, p uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeGetAutoVacuum(tls, p)
}

func Xsqlite3BtreeNewDb(tls *libc.TLS, p uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeNewDb(tls, p)
}

func Xsqlite3BtreeBeginTrans(tls *libc.TLS, p uintptr, wrflag int32, pSchemaVersion uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeBeginTrans(tls, p, wrflag, pSchemaVersion)
}

func Xsqlite3BtreeIncrVacuum(tls *libc.TLS, p uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeIncrVacuum(tls, p)
}

func Xsqlite3BtreeCommitPhaseOne(tls *libc.TLS, p uintptr, zSuperJrnl uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeCommitPhaseOne(tls, p, zSuperJrnl)
}

func Xsqlite3BtreeCommitPhaseTwo(tls *libc.TLS, p uintptr, bCleanup int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeCommitPhaseTwo(tls, p, bCleanup)
}

func Xsqlite3BtreeCommit(tls *libc.TLS, p uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeCommit(tls, p)
}

func Xsqlite3BtreeTripAllCursors(tls *libc.TLS, pBtree uintptr, errCode int32, writeOnly int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeTripAllCursors(tls, pBtree, errCode, writeOnly)
}

func Xsqlite3BtreeRollback(tls *libc.TLS, p uintptr, tripCode int32, writeOnly int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeRollback(tls, p, tripCode, writeOnly)
}

func Xsqlite3BtreeBeginStmt(tls *libc.TLS, p uintptr, iStatement int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeBeginStmt(tls, p, iStatement)
}

func Xsqlite3BtreeSavepoint(tls *libc.TLS, p uintptr, op int32, iSavepoint int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeSavepoint(tls, p, op, iSavepoint)
}

func Xsqlite3BtreeCursor(tls *libc.TLS, p uintptr, iTable Pgno, wrFlag int32, pKeyInfo uintptr, pCur uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeCursor(tls, p, iTable, wrFlag, pKeyInfo, pCur)
}

func Xsqlite3BtreeCursorSize(tls *libc.TLS) int32 {
	return currentStorageEngine.Xsqlite3BtreeCursorSize(tls)
}

func Xsqlite3BtreeCursorZero(tls *libc.TLS, p uintptr) {
	currentStorageEngine.Xsqlite3BtreeCursorZero(tls, p)
}

func Xsqlite3BtreeCloseCursor(tls *libc.TLS, pCur uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeCloseCursor(tls, pCur)
}

func Xsqlite3BtreeCursorIsValidNN(tls *libc.TLS, pCur uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeCursorIsValidNN(tls, pCur)
}

func Xsqlite3BtreeIntegerKey(tls *libc.TLS, pCur uintptr) I64 {
	return currentStorageEngine.Xsqlite3BtreeIntegerKey(tls, pCur)
}

func Xsqlite3BtreeCursorPin(tls *libc.TLS, pCur uintptr) {
	currentStorageEngine.Xsqlite3BtreeCursorPin(tls, pCur)
}

func Xsqlite3BtreeCursorUnpin(tls *libc.TLS, pCur uintptr) {
	currentStorageEngine.Xsqlite3BtreeCursorUnpin(tls, pCur)
}

func Xsqlite3BtreeOffset(tls *libc.TLS, pCur uintptr) I64 {
	return currentStorageEngine.Xsqlite3BtreeOffset(tls, pCur)
}

func Xsqlite3BtreePayloadSize(tls *libc.TLS, pCur uintptr) U32 {
	return currentStorageEngine.Xsqlite3BtreePayloadSize(tls, pCur)
}

func Xsqlite3BtreeMaxRecordSize(tls *libc.TLS, pCur uintptr) Sqlite3_int64 {
	return currentStorageEngine.Xsqlite3BtreeMaxRecordSize(tls, pCur)
}

func Xsqlite3BtreePayload(tls *libc.TLS, pCur uintptr, offset U32, amt U32, pBuf uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreePayload(tls, pCur, offset, amt, pBuf)
}

func Xsqlite3BtreePayloadChecked(tls *libc.TLS, pCur uintptr, offset U32, amt U32, pBuf uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreePayloadChecked(tls, pCur, offset, amt, pBuf)
}

func Xsqlite3BtreePayloadFetch(tls *libc.TLS, pCur uintptr, pAmt uintptr) uintptr {
	return currentStorageEngine.Xsqlite3BtreePayloadFetch(tls, pCur, pAmt)
}

func Xsqlite3BtreeFirst(tls *libc.TLS, pCur uintptr, pRes uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeFirst(tls, pCur, pRes)
}

func Xsqlite3BtreeLast(tls *libc.TLS, pCur uintptr, pRes uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeLast(tls, pCur, pRes)
}

func Xsqlite3BtreeTableMoveto(tls *libc.TLS, pCur uintptr, intKey I64, biasRight int32, pRes uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeTableMoveto(tls, pCur, intKey, biasRight, pRes)
}

func Xsqlite3BtreeIndexMoveto(tls *libc.TLS, pCur uintptr, pIdxKey uintptr, pRes uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeIndexMoveto(tls, pCur, pIdxKey, pRes)
}

func Xsqlite3BtreeEof(tls *libc.TLS, pCur uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeEof(tls, pCur)
}

func Xsqlite3BtreeRowCountEst(tls *libc.TLS, pCur uintptr) I64 {
	return currentStorageEngine.Xsqlite3BtreeRowCountEst(tls, pCur)
}

func Xsqlite3BtreeNext(tls *libc.TLS, pCur uintptr, flags int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeNext(tls, pCur, flags)
}

func Xsqlite3BtreePrevious(tls *libc.TLS, pCur uintptr, flags int32) int32 {
	return currentStorageEngine.Xsqlite3BtreePrevious(tls, pCur, flags)
}

func Xsqlite3BtreeInsert(tls *libc.TLS, pCur uintptr, pX uintptr, flags int32, seekResult int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeInsert(tls, pCur, pX, flags, seekResult)
}

func Xsqlite3BtreeTransferRow(tls *libc.TLS, pDest uintptr, pSrc uintptr, iKey I64) int32 {
	return currentStorageEngine.Xsqlite3BtreeTransferRow(tls, pDest, pSrc, iKey)
}

func Xsqlite3BtreeDelete(tls *libc.TLS, pCur uintptr, flags U8) int32 {
	return currentStorageEngine.Xsqlite3BtreeDelete(tls, pCur, flags)
}

func Xsqlite3BtreeCreateTable(tls *libc.TLS, p uintptr, piTable uintptr, flags int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeCreateTable(tls, p, piTable, flags)
}

func Xsqlite3BtreeClearTable(tls *libc.TLS, p uintptr, iTable int32, pnChange uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeClearTable(tls, p, iTable, pnChange)
}

func Xsqlite3BtreeClearTableOfCursor(tls *libc.TLS, pCur uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeClearTableOfCursor(tls, pCur)
}

func Xsqlite3BtreeDropTable(tls *libc.TLS, p uintptr, iTable int32, piMoved uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeDropTable(tls, p, iTable, piMoved)
}

func Xsqlite3BtreeGetMeta(tls *libc.TLS, p uintptr, idx int32, pMeta uintptr) {
	currentStorageEngine.Xsqlite3BtreeGetMeta(tls, p, idx, pMeta)
}

func Xsqlite3BtreeUpdateMeta(tls *libc.TLS, p uintptr, idx int32, iMeta U32) int32 {
	return currentStorageEngine.Xsqlite3BtreeUpdateMeta(tls, p, idx, iMeta)
}

func Xsqlite3BtreeCount(tls *libc.TLS, db uintptr, pCur uintptr, pnEntry uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeCount(tls, db, pCur, pnEntry)
}

func Xsqlite3BtreePager(tls *libc.TLS, p uintptr) uintptr {
	return currentStorageEngine.Xsqlite3BtreePager(tls, p)
}

func Xsqlite3BtreeIntegrityCheck(tls *libc.TLS, db uintptr, p uintptr, aRoot uintptr, nRoot int32, mxErr int32, pnErr uintptr) uintptr {
	return currentStorageEngine.Xsqlite3BtreeIntegrityCheck(tls, db, p, aRoot, nRoot, mxErr, pnErr)
}

func Xsqlite3BtreeGetFilename(tls *libc.TLS, p uintptr) uintptr {
	return currentStorageEngine.Xsqlite3BtreeGetFilename(tls, p)
}

func Xsqlite3BtreeGetJournalname(tls *libc.TLS, p uintptr) uintptr {
	return currentStorageEngine.Xsqlite3BtreeGetJournalname(tls, p)
}

func Xsqlite3BtreeTxnState(tls *libc.TLS, p uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeTxnState(tls, p)
}

func Xsqlite3BtreeCheckpoint(tls *libc.TLS, p uintptr, eMode int32, pnLog uintptr, pnCkpt uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeCheckpoint(tls, p, eMode, pnLog, pnCkpt)
}

func Xsqlite3BtreeIsInBackup(tls *libc.TLS, p uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeIsInBackup(tls, p)
}

func Xsqlite3BtreeSchema(tls *libc.TLS, p uintptr, nBytes int32, xFree uintptr) uintptr {
	return currentStorageEngine.Xsqlite3BtreeSchema(tls, p, nBytes, xFree)
}

func Xsqlite3BtreeSchemaLocked(tls *libc.TLS, p uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeSchemaLocked(tls, p)
}

func Xsqlite3BtreeLockTable(tls *libc.TLS, p uintptr, iTab int32, isWriteLock U8) int32 {
	return currentStorageEngine.Xsqlite3BtreeLockTable(tls, p, iTab, isWriteLock)
}

func Xsqlite3BtreePutData(tls *libc.TLS, pCsr uintptr, offset U32, amt U32, z uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreePutData(tls, pCsr, offset, amt, z)
}

func Xsqlite3BtreeIncrblobCursor(tls *libc.TLS, pCur uintptr) {
	currentStorageEngine.Xsqlite3BtreeIncrblobCursor(tls, pCur)
}

func Xsqlite3BtreeSetVersion(tls *libc.TLS, pBtree uintptr, iVersion int32) int32 {
	return currentStorageEngine.Xsqlite3BtreeSetVersion(tls, pBtree, iVersion)
}

func Xsqlite3BtreeCursorHasHint(tls *libc.TLS, pCsr uintptr, mask uint32) int32 {
	return currentStorageEngine.Xsqlite3BtreeCursorHasHint(tls, pCsr, mask)
}

func Xsqlite3BtreeIsReadonly(tls *libc.TLS, p uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeIsReadonly(tls, p)
}

func Xsqlite3BtreeSharable(tls *libc.TLS, p uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeSharable(tls, p)
}

func Xsqlite3BtreeConnectionCount(tls *libc.TLS, p uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeConnectionCount(tls, p)
}

func Xsqlite3BtreeCopyFile(tls *libc.TLS, pTo uintptr, pFrom uintptr) int32 {
	return currentStorageEngine.Xsqlite3BtreeCopyFile(tls, pTo, pFrom)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeEnter(tls *libc.TLS, p uintptr) {
	XnativeSqlite3BtreeEnter(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeLeave(tls *libc.TLS, p uintptr) {
	XnativeSqlite3BtreeLeave(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeEnterAll(tls *libc.TLS, db uintptr) {
	XnativeSqlite3BtreeEnterAll(tls, db)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeLeaveAll(tls *libc.TLS, db uintptr) {
	XnativeSqlite3BtreeLeaveAll(tls, db)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeEnterCursor(tls *libc.TLS, pCur uintptr) {
	XnativeSqlite3BtreeEnterCursor(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeLeaveCursor(tls *libc.TLS, pCur uintptr) {
	XnativeSqlite3BtreeLeaveCursor(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeClearCursor(tls *libc.TLS, pCur uintptr) {
	XnativeSqlite3BtreeClearCursor(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCursorHasMoved(tls *libc.TLS, pCur uintptr) int32 {
	return XnativeSqlite3BtreeCursorHasMoved(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeFakeValidCursor(tls *libc.TLS) uintptr {
	return XnativeSqlite3BtreeFakeValidCursor(tls)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCursorRestore(tls *libc.TLS, pCur uintptr, pDifferentRow uintptr) int32 {
	return XnativeSqlite3BtreeCursorRestore(tls, pCur, pDifferentRow)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCursorHintFlags(tls *libc.TLS, pCur uintptr, x uint32) {
	XnativeSqlite3BtreeCursorHintFlags(tls, pCur, x)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeLastPage(tls *libc.TLS, p uintptr) Pgno {
	return XnativeSqlite3BtreeLastPage(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeOpen(tls *libc.TLS, pVfs uintptr, zFilename uintptr, db uintptr, ppBtree uintptr, flags int32, vfsFlags int32) int32 {
	return XnativeSqlite3BtreeOpen(tls, pVfs, zFilename, db, ppBtree, flags, vfsFlags)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeClose(tls *libc.TLS, p uintptr) int32 {
	return XnativeSqlite3BtreeClose(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeSetCacheSize(tls *libc.TLS, p uintptr, mxPage int32) int32 {
	return XnativeSqlite3BtreeSetCacheSize(tls, p, mxPage)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeSetSpillSize(tls *libc.TLS, p uintptr, mxPage int32) int32 {
	return XnativeSqlite3BtreeSetSpillSize(tls, p, mxPage)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeSetPagerFlags(tls *libc.TLS, p uintptr, pgFlags uint32) int32 {
	return XnativeSqlite3BtreeSetPagerFlags(tls, p, pgFlags)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeSetPageSize(tls *libc.TLS, p uintptr, pageSize int32, nReserve int32, iFix int32) int32 {
	return XnativeSqlite3BtreeSetPageSize(tls, p, pageSize, nReserve, iFix)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeGetPageSize(tls *libc.TLS, p uintptr) int32 {
	return XnativeSqlite3BtreeGetPageSize(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeGetReserveNoMutex(tls *libc.TLS, p uintptr) int32 {
	return XnativeSqlite3BtreeGetReserveNoMutex(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeGetRequestedReserve(tls *libc.TLS, p uintptr) int32 {
	return XnativeSqlite3BtreeGetRequestedReserve(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeMaxPageCount(tls *libc.TLS, p uintptr, mxPage Pgno) Pgno {
	return XnativeSqlite3BtreeMaxPageCount(tls, p, mxPage)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeSecureDelete(tls *libc.TLS, p uintptr, newFlag int32) int32 {
	return XnativeSqlite3BtreeSecureDelete(tls, p, newFlag)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeSetAutoVacuum(tls *libc.TLS, p uintptr, autoVacuum int32) int32 {
	return XnativeSqlite3BtreeSetAutoVacuum(tls, p, autoVacuum)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeGetAutoVacuum(tls *libc.TLS, p uintptr) int32 {
	return XnativeSqlite3BtreeGetAutoVacuum(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeNewDb(tls *libc.TLS, p uintptr) int32 {
	return XnativeSqlite3BtreeNewDb(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeBeginTrans(tls *libc.TLS, p uintptr, wrflag int32, pSchemaVersion uintptr) int32 {
	return XnativeSqlite3BtreeBeginTrans(tls, p, wrflag, pSchemaVersion)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeIncrVacuum(tls *libc.TLS, p uintptr) int32 {
	return XnativeSqlite3BtreeIncrVacuum(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCommitPhaseOne(tls *libc.TLS, p uintptr, zSuperJrnl uintptr) int32 {
	return XnativeSqlite3BtreeCommitPhaseOne(tls, p, zSuperJrnl)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCommitPhaseTwo(tls *libc.TLS, p uintptr, bCleanup int32) int32 {
	return XnativeSqlite3BtreeCommitPhaseTwo(tls, p, bCleanup)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCommit(tls *libc.TLS, p uintptr) int32 {
	return XnativeSqlite3BtreeCommit(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeTripAllCursors(tls *libc.TLS, pBtree uintptr, errCode int32, writeOnly int32) int32 {
	return XnativeSqlite3BtreeTripAllCursors(tls, pBtree, errCode, writeOnly)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeRollback(tls *libc.TLS, p uintptr, tripCode int32, writeOnly int32) int32 {
	return XnativeSqlite3BtreeRollback(tls, p, tripCode, writeOnly)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeBeginStmt(tls *libc.TLS, p uintptr, iStatement int32) int32 {
	return XnativeSqlite3BtreeBeginStmt(tls, p, iStatement)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeSavepoint(tls *libc.TLS, p uintptr, op int32, iSavepoint int32) int32 {
	return XnativeSqlite3BtreeSavepoint(tls, p, op, iSavepoint)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCursor(tls *libc.TLS, p uintptr, iTable Pgno, wrFlag int32, pKeyInfo uintptr, pCur uintptr) int32 {
	return XnativeSqlite3BtreeCursor(tls, p, iTable, wrFlag, pKeyInfo, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCursorSize(tls *libc.TLS) int32 {
	return XnativeSqlite3BtreeCursorSize(tls)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCursorZero(tls *libc.TLS, p uintptr) {
	XnativeSqlite3BtreeCursorZero(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCloseCursor(tls *libc.TLS, pCur uintptr) int32 {
	return XnativeSqlite3BtreeCloseCursor(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCursorIsValidNN(tls *libc.TLS, pCur uintptr) int32 {
	return XnativeSqlite3BtreeCursorIsValidNN(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeIntegerKey(tls *libc.TLS, pCur uintptr) I64 {
	return XnativeSqlite3BtreeIntegerKey(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCursorPin(tls *libc.TLS, pCur uintptr) {
	XnativeSqlite3BtreeCursorPin(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCursorUnpin(tls *libc.TLS, pCur uintptr) {
	XnativeSqlite3BtreeCursorUnpin(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeOffset(tls *libc.TLS, pCur uintptr) I64 {
	return XnativeSqlite3BtreeOffset(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreePayloadSize(tls *libc.TLS, pCur uintptr) U32 {
	return XnativeSqlite3BtreePayloadSize(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeMaxRecordSize(tls *libc.TLS, pCur uintptr) Sqlite3_int64 {
	return XnativeSqlite3BtreeMaxRecordSize(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreePayload(tls *libc.TLS, pCur uintptr, offset U32, amt U32, pBuf uintptr) int32 {
	return XnativeSqlite3BtreePayload(tls, pCur, offset, amt, pBuf)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreePayloadChecked(tls *libc.TLS, pCur uintptr, offset U32, amt U32, pBuf uintptr) int32 {
	return XnativeSqlite3BtreePayloadChecked(tls, pCur, offset, amt, pBuf)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreePayloadFetch(tls *libc.TLS, pCur uintptr, pAmt uintptr) uintptr {
	return XnativeSqlite3BtreePayloadFetch(tls, pCur, pAmt)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeFirst(tls *libc.TLS, pCur uintptr, pRes uintptr) int32 {
	return XnativeSqlite3BtreeFirst(tls, pCur, pRes)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeLast(tls *libc.TLS, pCur uintptr, pRes uintptr) int32 {
	return XnativeSqlite3BtreeLast(tls, pCur, pRes)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeTableMoveto(tls *libc.TLS, pCur uintptr, intKey I64, biasRight int32, pRes uintptr) int32 {
	return XnativeSqlite3BtreeTableMoveto(tls, pCur, intKey, biasRight, pRes)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeIndexMoveto(tls *libc.TLS, pCur uintptr, pIdxKey uintptr, pRes uintptr) int32 {
	return XnativeSqlite3BtreeIndexMoveto(tls, pCur, pIdxKey, pRes)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeEof(tls *libc.TLS, pCur uintptr) int32 {
	return XnativeSqlite3BtreeEof(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeRowCountEst(tls *libc.TLS, pCur uintptr) I64 {
	return XnativeSqlite3BtreeRowCountEst(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeNext(tls *libc.TLS, pCur uintptr, flags int32) int32 {
	return XnativeSqlite3BtreeNext(tls, pCur, flags)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreePrevious(tls *libc.TLS, pCur uintptr, flags int32) int32 {
	return XnativeSqlite3BtreePrevious(tls, pCur, flags)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeInsert(tls *libc.TLS, pCur uintptr, pX uintptr, flags int32, seekResult int32) int32 {
	return XnativeSqlite3BtreeInsert(tls, pCur, pX, flags, seekResult)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeTransferRow(tls *libc.TLS, pDest uintptr, pSrc uintptr, iKey I64) int32 {
	return XnativeSqlite3BtreeTransferRow(tls, pDest, pSrc, iKey)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeDelete(tls *libc.TLS, pCur uintptr, flags U8) int32 {
	return XnativeSqlite3BtreeDelete(tls, pCur, flags)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCreateTable(tls *libc.TLS, p uintptr, piTable uintptr, flags int32) int32 {
	return XnativeSqlite3BtreeCreateTable(tls, p, piTable, flags)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeClearTable(tls *libc.TLS, p uintptr, iTable int32, pnChange uintptr) int32 {
	return XnativeSqlite3BtreeClearTable(tls, p, iTable, pnChange)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeClearTableOfCursor(tls *libc.TLS, pCur uintptr) int32 {
	return XnativeSqlite3BtreeClearTableOfCursor(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeDropTable(tls *libc.TLS, p uintptr, iTable int32, piMoved uintptr) int32 {
	return XnativeSqlite3BtreeDropTable(tls, p, iTable, piMoved)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeGetMeta(tls *libc.TLS, p uintptr, idx int32, pMeta uintptr) {
	XnativeSqlite3BtreeGetMeta(tls, p, idx, pMeta)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeUpdateMeta(tls *libc.TLS, p uintptr, idx int32, iMeta U32) int32 {
	return XnativeSqlite3BtreeUpdateMeta(tls, p, idx, iMeta)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCount(tls *libc.TLS, db uintptr, pCur uintptr, pnEntry uintptr) int32 {
	return XnativeSqlite3BtreeCount(tls, db, pCur, pnEntry)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreePager(tls *libc.TLS, p uintptr) uintptr {
	return XnativeSqlite3BtreePager(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeIntegrityCheck(tls *libc.TLS, db uintptr, p uintptr, aRoot uintptr, nRoot int32, mxErr int32, pnErr uintptr) uintptr {
	return XnativeSqlite3BtreeIntegrityCheck(tls, db, p, aRoot, nRoot, mxErr, pnErr)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeGetFilename(tls *libc.TLS, p uintptr) uintptr {
	return XnativeSqlite3BtreeGetFilename(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeGetJournalname(tls *libc.TLS, p uintptr) uintptr {
	return XnativeSqlite3BtreeGetJournalname(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeTxnState(tls *libc.TLS, p uintptr) int32 {
	return XnativeSqlite3BtreeTxnState(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCheckpoint(tls *libc.TLS, p uintptr, eMode int32, pnLog uintptr, pnCkpt uintptr) int32 {
	return XnativeSqlite3BtreeCheckpoint(tls, p, eMode, pnLog, pnCkpt)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeIsInBackup(tls *libc.TLS, p uintptr) int32 {
	return XnativeSqlite3BtreeIsInBackup(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeSchema(tls *libc.TLS, p uintptr, nBytes int32, xFree uintptr) uintptr {
	return XnativeSqlite3BtreeSchema(tls, p, nBytes, xFree)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeSchemaLocked(tls *libc.TLS, p uintptr) int32 {
	return XnativeSqlite3BtreeSchemaLocked(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeLockTable(tls *libc.TLS, p uintptr, iTab int32, isWriteLock U8) int32 {
	return XnativeSqlite3BtreeLockTable(tls, p, iTab, isWriteLock)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreePutData(tls *libc.TLS, pCsr uintptr, offset U32, amt U32, z uintptr) int32 {
	return XnativeSqlite3BtreePutData(tls, pCsr, offset, amt, z)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeIncrblobCursor(tls *libc.TLS, pCur uintptr) {
	XnativeSqlite3BtreeIncrblobCursor(tls, pCur)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeSetVersion(tls *libc.TLS, pBtree uintptr, iVersion int32) int32 {
	return XnativeSqlite3BtreeSetVersion(tls, pBtree, iVersion)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCursorHasHint(tls *libc.TLS, pCsr uintptr, mask uint32) int32 {
	return XnativeSqlite3BtreeCursorHasHint(tls, pCsr, mask)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeIsReadonly(tls *libc.TLS, p uintptr) int32 {
	return XnativeSqlite3BtreeIsReadonly(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeSharable(tls *libc.TLS, p uintptr) int32 {
	return XnativeSqlite3BtreeSharable(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeConnectionCount(tls *libc.TLS, p uintptr) int32 {
	return XnativeSqlite3BtreeConnectionCount(tls, p)
}

func (nativeBtreeStorageEngine) Xsqlite3BtreeCopyFile(tls *libc.TLS, pTo uintptr, pFrom uintptr) int32 {
	return XnativeSqlite3BtreeCopyFile(tls, pTo, pFrom)
}
