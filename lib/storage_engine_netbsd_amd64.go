// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite3

import "modernc.org/libc"

func Xsqlite3BtreeEnter(tls *libc.TLS, p uintptr) {
	storageEngine().BtreeEnter(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeLeave(tls *libc.TLS, p uintptr) {
	storageEngine().BtreeLeave(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeEnterAll(tls *libc.TLS, db uintptr) {
	storageEngine().BtreeEnterAll(btreeContext(tls), sqliteHandle(tls, db))
}

func Xsqlite3BtreeLeaveAll(tls *libc.TLS, db uintptr) {
	storageEngine().BtreeLeaveAll(btreeContext(tls), sqliteHandle(tls, db))
}

func Xsqlite3BtreeEnterCursor(tls *libc.TLS, pCur uintptr) {
	storageEngine().BtreeEnterCursor(btreeContext(tls), btreeCursorHandle(tls, pCur))
}

func Xsqlite3BtreeLeaveCursor(tls *libc.TLS, pCur uintptr) {
	storageEngine().BtreeLeaveCursor(btreeContext(tls), btreeCursorHandle(tls, pCur))
}

func Xsqlite3BtreeClearCursor(tls *libc.TLS, pCur uintptr) {
	storageEngine().BtreeClearCursor(btreeContext(tls), btreeCursorHandle(tls, pCur))
}

func Xsqlite3BtreeCursorHasMoved(tls *libc.TLS, pCur uintptr) (r int32) {
	return storageEngine().BtreeCursorHasMoved(btreeContext(tls), btreeCursorHandle(tls, pCur))
}

func Xsqlite3BtreeFakeValidCursor(tls *libc.TLS) (r uintptr) {
	return storageEngine().BtreeFakeValidCursor(btreeContext(tls)).ptr
}

func Xsqlite3BtreeCursorRestore(tls *libc.TLS, pCur uintptr, pDifferentRow uintptr) (r int32) {
	return storageEngine().BtreeCursorRestore(btreeContext(tls), btreeCursorHandle(tls, pCur), btreeMemoryHandle(tls, pDifferentRow))
}

func Xsqlite3BtreeCursorHintFlags(tls *libc.TLS, pCur uintptr, x uint32) {
	storageEngine().BtreeCursorHintFlags(btreeContext(tls), btreeCursorHandle(tls, pCur), x)
}

func Xsqlite3BtreeLastPage(tls *libc.TLS, p uintptr) (r Pgno) {
	return Pgno(storageEngine().BtreeLastPage(btreeContext(tls), btreeHandle(tls, p)))
}

func Xsqlite3BtreeOpen(tls *libc.TLS, pVfs uintptr, zFilename uintptr, db uintptr, ppBtree uintptr, flags int32, vfsFlags int32) (r int32) {
	return storageEngine().BtreeOpen(btreeContext(tls), btreeVFSHandle(tls, pVfs), btreeCStringHandle(tls, zFilename), sqliteHandle(tls, db), btreeMemoryHandle(tls, ppBtree), flags, vfsFlags)
}

func Xsqlite3BtreeClose(tls *libc.TLS, p uintptr) (r int32) {
	return storageEngine().BtreeClose(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeSetCacheSize(tls *libc.TLS, p uintptr, mxPage int32) (r int32) {
	return storageEngine().BtreeSetCacheSize(btreeContext(tls), btreeHandle(tls, p), mxPage)
}

func Xsqlite3BtreeSetSpillSize(tls *libc.TLS, p uintptr, mxPage int32) (r int32) {
	return storageEngine().BtreeSetSpillSize(btreeContext(tls), btreeHandle(tls, p), mxPage)
}

func Xsqlite3BtreeSetPagerFlags(tls *libc.TLS, p uintptr, pgFlags uint32) (r int32) {
	return storageEngine().BtreeSetPagerFlags(btreeContext(tls), btreeHandle(tls, p), pgFlags)
}

func Xsqlite3BtreeSetPageSize(tls *libc.TLS, p uintptr, pageSize int32, nReserve int32, iFix int32) (r int32) {
	return storageEngine().BtreeSetPageSize(btreeContext(tls), btreeHandle(tls, p), pageSize, nReserve, iFix)
}

func Xsqlite3BtreeGetPageSize(tls *libc.TLS, p uintptr) (r int32) {
	return storageEngine().BtreeGetPageSize(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeGetReserveNoMutex(tls *libc.TLS, p uintptr) (r int32) {
	return storageEngine().BtreeGetReserveNoMutex(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeGetRequestedReserve(tls *libc.TLS, p uintptr) (r int32) {
	return storageEngine().BtreeGetRequestedReserve(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeMaxPageCount(tls *libc.TLS, p uintptr, mxPage Pgno) (r Pgno) {
	return Pgno(storageEngine().BtreeMaxPageCount(btreeContext(tls), btreeHandle(tls, p), uint32(mxPage)))
}

func Xsqlite3BtreeSecureDelete(tls *libc.TLS, p uintptr, newFlag int32) (r int32) {
	return storageEngine().BtreeSecureDelete(btreeContext(tls), btreeHandle(tls, p), newFlag)
}

func Xsqlite3BtreeSetAutoVacuum(tls *libc.TLS, p uintptr, autoVacuum int32) (r int32) {
	return storageEngine().BtreeSetAutoVacuum(btreeContext(tls), btreeHandle(tls, p), autoVacuum)
}

func Xsqlite3BtreeGetAutoVacuum(tls *libc.TLS, p uintptr) (r int32) {
	return storageEngine().BtreeGetAutoVacuum(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeNewDb(tls *libc.TLS, p uintptr) (r int32) {
	return storageEngine().BtreeNewDb(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeBeginTrans(tls *libc.TLS, p uintptr, wrflag int32, pSchemaVersion uintptr) (r int32) {
	return storageEngine().BtreeBeginTrans(btreeContext(tls), btreeHandle(tls, p), wrflag, btreeMemoryHandle(tls, pSchemaVersion))
}

func Xsqlite3BtreeIncrVacuum(tls *libc.TLS, p uintptr) (r int32) {
	return storageEngine().BtreeIncrVacuum(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeCommitPhaseOne(tls *libc.TLS, p uintptr, zSuperJrnl uintptr) (r int32) {
	return storageEngine().BtreeCommitPhaseOne(btreeContext(tls), btreeHandle(tls, p), btreeCStringHandle(tls, zSuperJrnl))
}

func Xsqlite3BtreeCommitPhaseTwo(tls *libc.TLS, p uintptr, bCleanup int32) (r int32) {
	return storageEngine().BtreeCommitPhaseTwo(btreeContext(tls), btreeHandle(tls, p), bCleanup)
}

func Xsqlite3BtreeCommit(tls *libc.TLS, p uintptr) (r int32) {
	return storageEngine().BtreeCommit(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeTripAllCursors(tls *libc.TLS, pBtree uintptr, errCode int32, writeOnly int32) (r int32) {
	return storageEngine().BtreeTripAllCursors(btreeContext(tls), btreeHandle(tls, pBtree), errCode, writeOnly)
}

func Xsqlite3BtreeRollback(tls *libc.TLS, p uintptr, tripCode int32, writeOnly int32) (r int32) {
	return storageEngine().BtreeRollback(btreeContext(tls), btreeHandle(tls, p), tripCode, writeOnly)
}

func Xsqlite3BtreeBeginStmt(tls *libc.TLS, p uintptr, iStatement int32) (r int32) {
	return storageEngine().BtreeBeginStmt(btreeContext(tls), btreeHandle(tls, p), iStatement)
}

func Xsqlite3BtreeSavepoint(tls *libc.TLS, p uintptr, op int32, iSavepoint int32) (r int32) {
	return storageEngine().BtreeSavepoint(btreeContext(tls), btreeHandle(tls, p), op, iSavepoint)
}

func Xsqlite3BtreeCursor(tls *libc.TLS, p uintptr, iTable Pgno, wrFlag int32, pKeyInfo uintptr, pCur uintptr) (r int32) {
	return storageEngine().BtreeCursor(btreeContext(tls), btreeHandle(tls, p), uint32(iTable), wrFlag, btreeKeyInfoHandle(tls, pKeyInfo), btreeCursorHandle(tls, pCur))
}

func Xsqlite3BtreeCursorSize(tls *libc.TLS) (r int32) {
	return storageEngine().BtreeCursorSize(btreeContext(tls))
}

func Xsqlite3BtreeCursorZero(tls *libc.TLS, p uintptr) {
	storageEngine().BtreeCursorZero(btreeContext(tls), btreeCursorHandle(tls, p))
}

func Xsqlite3BtreeCloseCursor(tls *libc.TLS, pCur uintptr) (r int32) {
	return storageEngine().BtreeCloseCursor(btreeContext(tls), btreeCursorHandle(tls, pCur))
}

func Xsqlite3BtreeCursorIsValidNN(tls *libc.TLS, pCur uintptr) (r int32) {
	return storageEngine().BtreeCursorIsValidNN(btreeContext(tls), btreeCursorHandle(tls, pCur))
}

func Xsqlite3BtreeIntegerKey(tls *libc.TLS, pCur uintptr) (r I64) {
	return I64(storageEngine().BtreeIntegerKey(btreeContext(tls), btreeCursorHandle(tls, pCur)))
}

func Xsqlite3BtreeCursorPin(tls *libc.TLS, pCur uintptr) {
	storageEngine().BtreeCursorPin(btreeContext(tls), btreeCursorHandle(tls, pCur))
}

func Xsqlite3BtreeCursorUnpin(tls *libc.TLS, pCur uintptr) {
	storageEngine().BtreeCursorUnpin(btreeContext(tls), btreeCursorHandle(tls, pCur))
}

func Xsqlite3BtreeOffset(tls *libc.TLS, pCur uintptr) (r I64) {
	return I64(storageEngine().BtreeOffset(btreeContext(tls), btreeCursorHandle(tls, pCur)))
}

func Xsqlite3BtreePayloadSize(tls *libc.TLS, pCur uintptr) (r U32) {
	return U32(storageEngine().BtreePayloadSize(btreeContext(tls), btreeCursorHandle(tls, pCur)))
}

func Xsqlite3BtreeMaxRecordSize(tls *libc.TLS, pCur uintptr) (r Sqlite3_int64) {
	return Sqlite3_int64(storageEngine().BtreeMaxRecordSize(btreeContext(tls), btreeCursorHandle(tls, pCur)))
}

func Xsqlite3BtreePayload(tls *libc.TLS, pCur uintptr, offset U32, amt U32, pBuf uintptr) (r int32) {
	return storageEngine().BtreePayload(btreeContext(tls), btreeCursorHandle(tls, pCur), uint32(offset), uint32(amt), btreeMemoryHandle(tls, pBuf))
}

func Xsqlite3BtreePayloadChecked(tls *libc.TLS, pCur uintptr, offset U32, amt U32, pBuf uintptr) (r int32) {
	return storageEngine().BtreePayloadChecked(btreeContext(tls), btreeCursorHandle(tls, pCur), uint32(offset), uint32(amt), btreeMemoryHandle(tls, pBuf))
}

func Xsqlite3BtreePayloadFetch(tls *libc.TLS, pCur uintptr, pAmt uintptr) (r uintptr) {
	return storageEngine().BtreePayloadFetch(btreeContext(tls), btreeCursorHandle(tls, pCur), btreeMemoryHandle(tls, pAmt)).ptr
}

func Xsqlite3BtreeFirst(tls *libc.TLS, pCur uintptr, pRes uintptr) (r int32) {
	return storageEngine().BtreeFirst(btreeContext(tls), btreeCursorHandle(tls, pCur), btreeMemoryHandle(tls, pRes))
}

func Xsqlite3BtreeLast(tls *libc.TLS, pCur uintptr, pRes uintptr) (r int32) {
	return storageEngine().BtreeLast(btreeContext(tls), btreeCursorHandle(tls, pCur), btreeMemoryHandle(tls, pRes))
}

func Xsqlite3BtreeTableMoveto(tls *libc.TLS, pCur uintptr, intKey I64, biasRight int32, pRes uintptr) (r int32) {
	return storageEngine().BtreeTableMoveto(btreeContext(tls), btreeCursorHandle(tls, pCur), int64(intKey), biasRight, btreeMemoryHandle(tls, pRes))
}

func Xsqlite3BtreeIndexMoveto(tls *libc.TLS, pCur uintptr, pIdxKey uintptr, pRes uintptr) (r int32) {
	return storageEngine().BtreeIndexMoveto(btreeContext(tls), btreeCursorHandle(tls, pCur), btreeIndexKeyHandle(tls, pIdxKey), btreeMemoryHandle(tls, pRes))
}

func Xsqlite3BtreeEof(tls *libc.TLS, pCur uintptr) (r int32) {
	return storageEngine().BtreeEof(btreeContext(tls), btreeCursorHandle(tls, pCur))
}

func Xsqlite3BtreeRowCountEst(tls *libc.TLS, pCur uintptr) (r I64) {
	return I64(storageEngine().BtreeRowCountEst(btreeContext(tls), btreeCursorHandle(tls, pCur)))
}

func Xsqlite3BtreeNext(tls *libc.TLS, pCur uintptr, flags int32) (r int32) {
	return storageEngine().BtreeNext(btreeContext(tls), btreeCursorHandle(tls, pCur), flags)
}

func Xsqlite3BtreePrevious(tls *libc.TLS, pCur uintptr, flags int32) (r int32) {
	return storageEngine().BtreePrevious(btreeContext(tls), btreeCursorHandle(tls, pCur), flags)
}

func Xsqlite3BtreeInsert(tls *libc.TLS, pCur uintptr, pX uintptr, flags int32, seekResult int32) (r int32) {
	return storageEngine().BtreeInsert(btreeContext(tls), btreeCursorHandle(tls, pCur), btreePayloadHandle(tls, pX), flags, seekResult)
}

func Xsqlite3BtreeTransferRow(tls *libc.TLS, pDest uintptr, pSrc uintptr, iKey I64) (r int32) {
	return storageEngine().BtreeTransferRow(btreeContext(tls), btreeCursorHandle(tls, pDest), btreeCursorHandle(tls, pSrc), int64(iKey))
}

func Xsqlite3BtreeDelete(tls *libc.TLS, pCur uintptr, flags U8) (r int32) {
	return storageEngine().BtreeDelete(btreeContext(tls), btreeCursorHandle(tls, pCur), uint8(flags))
}

func Xsqlite3BtreeCreateTable(tls *libc.TLS, p uintptr, piTable uintptr, flags int32) (r int32) {
	return storageEngine().BtreeCreateTable(btreeContext(tls), btreeHandle(tls, p), btreeMemoryHandle(tls, piTable), flags)
}

func Xsqlite3BtreeClearTable(tls *libc.TLS, p uintptr, iTable int32, pnChange uintptr) (r int32) {
	return storageEngine().BtreeClearTable(btreeContext(tls), btreeHandle(tls, p), iTable, btreeMemoryHandle(tls, pnChange))
}

func Xsqlite3BtreeClearTableOfCursor(tls *libc.TLS, pCur uintptr) (r int32) {
	return storageEngine().BtreeClearTableOfCursor(btreeContext(tls), btreeCursorHandle(tls, pCur))
}

func Xsqlite3BtreeDropTable(tls *libc.TLS, p uintptr, iTable int32, piMoved uintptr) (r int32) {
	return storageEngine().BtreeDropTable(btreeContext(tls), btreeHandle(tls, p), iTable, btreeMemoryHandle(tls, piMoved))
}

func Xsqlite3BtreeGetMeta(tls *libc.TLS, p uintptr, idx int32, pMeta uintptr) {
	storageEngine().BtreeGetMeta(btreeContext(tls), btreeHandle(tls, p), idx, btreeMemoryHandle(tls, pMeta))
}

func Xsqlite3BtreeUpdateMeta(tls *libc.TLS, p uintptr, idx int32, iMeta U32) (r int32) {
	return storageEngine().BtreeUpdateMeta(btreeContext(tls), btreeHandle(tls, p), idx, uint32(iMeta))
}

func Xsqlite3BtreeCount(tls *libc.TLS, db uintptr, pCur uintptr, pnEntry uintptr) (r int32) {
	return storageEngine().BtreeCount(btreeContext(tls), sqliteHandle(tls, db), btreeCursorHandle(tls, pCur), btreeMemoryHandle(tls, pnEntry))
}

func Xsqlite3BtreePager(tls *libc.TLS, p uintptr) (r uintptr) {
	return storageEngine().BtreePager(btreeContext(tls), btreeHandle(tls, p)).ptr
}

func Xsqlite3BtreeIntegrityCheck(tls *libc.TLS, db uintptr, p uintptr, aRoot uintptr, nRoot int32, mxErr int32, pnErr uintptr) (r uintptr) {
	return storageEngine().(StorageEngineBtreeIntegrityCheckNetbsdAmd64).BtreeIntegrityCheckNetbsdAmd64(btreeContext(tls), sqliteHandle(tls, db), btreeHandle(tls, p), btreeMemoryHandle(tls, aRoot), nRoot, mxErr, btreeMemoryHandle(tls, pnErr)).ptr
}

func Xsqlite3BtreeGetFilename(tls *libc.TLS, p uintptr) (r uintptr) {
	return storageEngine().BtreeGetFilename(btreeContext(tls), btreeHandle(tls, p)).ptr
}

func Xsqlite3BtreeGetJournalname(tls *libc.TLS, p uintptr) (r uintptr) {
	return storageEngine().BtreeGetJournalname(btreeContext(tls), btreeHandle(tls, p)).ptr
}

func Xsqlite3BtreeTxnState(tls *libc.TLS, p uintptr) (r int32) {
	return storageEngine().BtreeTxnState(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeCheckpoint(tls *libc.TLS, p uintptr, eMode int32, pnLog uintptr, pnCkpt uintptr) (r int32) {
	return storageEngine().BtreeCheckpoint(btreeContext(tls), btreeHandle(tls, p), eMode, btreeMemoryHandle(tls, pnLog), btreeMemoryHandle(tls, pnCkpt))
}

func Xsqlite3BtreeIsInBackup(tls *libc.TLS, p uintptr) (r int32) {
	return storageEngine().BtreeIsInBackup(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeSchema(tls *libc.TLS, p uintptr, nBytes int32, xFree uintptr) (r uintptr) {
	return storageEngine().BtreeSchema(btreeContext(tls), btreeHandle(tls, p), nBytes, btreeFunctionHandle(tls, xFree)).ptr
}

func Xsqlite3BtreeSchemaLocked(tls *libc.TLS, p uintptr) (r int32) {
	return storageEngine().BtreeSchemaLocked(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeLockTable(tls *libc.TLS, p uintptr, iTab int32, isWriteLock U8) (r int32) {
	return storageEngine().BtreeLockTable(btreeContext(tls), btreeHandle(tls, p), iTab, uint8(isWriteLock))
}

func Xsqlite3BtreePutData(tls *libc.TLS, pCsr uintptr, offset U32, amt U32, z uintptr) (r int32) {
	return storageEngine().BtreePutData(btreeContext(tls), btreeCursorHandle(tls, pCsr), uint32(offset), uint32(amt), btreeMemoryHandle(tls, z))
}

func Xsqlite3BtreeIncrblobCursor(tls *libc.TLS, pCur uintptr) {
	storageEngine().BtreeIncrblobCursor(btreeContext(tls), btreeCursorHandle(tls, pCur))
}

func Xsqlite3BtreeSetVersion(tls *libc.TLS, pBtree uintptr, iVersion int32) (r int32) {
	return storageEngine().BtreeSetVersion(btreeContext(tls), btreeHandle(tls, pBtree), iVersion)
}

func Xsqlite3BtreeCursorHasHint(tls *libc.TLS, pCsr uintptr, mask uint32) (r int32) {
	return storageEngine().BtreeCursorHasHint(btreeContext(tls), btreeCursorHandle(tls, pCsr), mask)
}

func Xsqlite3BtreeIsReadonly(tls *libc.TLS, p uintptr) (r int32) {
	return storageEngine().BtreeIsReadonly(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeSharable(tls *libc.TLS, p uintptr) (r int32) {
	return storageEngine().BtreeSharable(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeConnectionCount(tls *libc.TLS, p uintptr) (r int32) {
	return storageEngine().BtreeConnectionCount(btreeContext(tls), btreeHandle(tls, p))
}

func Xsqlite3BtreeCopyFile(tls *libc.TLS, pTo uintptr, pFrom uintptr) (r int32) {
	return storageEngine().BtreeCopyFile(btreeContext(tls), btreeHandle(tls, pTo), btreeHandle(tls, pFrom))
}

func (nativeBtreeStorageEngine) BtreeEnter(ctx BtreeContext, p BtreeHandle) {
	XnativeSqlite3BtreeEnter(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeLeave(ctx BtreeContext, p BtreeHandle) {
	XnativeSqlite3BtreeLeave(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeEnterAll(ctx BtreeContext, db SQLiteHandle) {
	XnativeSqlite3BtreeEnterAll(ctx.tls, db.ptr)
}

func (nativeBtreeStorageEngine) BtreeLeaveAll(ctx BtreeContext, db SQLiteHandle) {
	XnativeSqlite3BtreeLeaveAll(ctx.tls, db.ptr)
}

func (nativeBtreeStorageEngine) BtreeEnterCursor(ctx BtreeContext, pCur BtreeCursorHandle) {
	XnativeSqlite3BtreeEnterCursor(ctx.tls, pCur.ptr)
}

func (nativeBtreeStorageEngine) BtreeLeaveCursor(ctx BtreeContext, pCur BtreeCursorHandle) {
	XnativeSqlite3BtreeLeaveCursor(ctx.tls, pCur.ptr)
}

func (nativeBtreeStorageEngine) BtreeClearCursor(ctx BtreeContext, pCur BtreeCursorHandle) {
	XnativeSqlite3BtreeClearCursor(ctx.tls, pCur.ptr)
}

func (nativeBtreeStorageEngine) BtreeCursorHasMoved(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	return XnativeSqlite3BtreeCursorHasMoved(ctx.tls, pCur.ptr)
}

func (nativeBtreeStorageEngine) BtreeFakeValidCursor(ctx BtreeContext) (r BtreeCursorHandle) {
	return btreeCursorHandle(ctx.tls, XnativeSqlite3BtreeFakeValidCursor(ctx.tls))
}

func (nativeBtreeStorageEngine) BtreeCursorRestore(ctx BtreeContext, pCur BtreeCursorHandle, pDifferentRow BtreeMemoryHandle) (r int32) {
	return XnativeSqlite3BtreeCursorRestore(ctx.tls, pCur.ptr, pDifferentRow.ptr)
}

func (nativeBtreeStorageEngine) BtreeCursorHintFlags(ctx BtreeContext, pCur BtreeCursorHandle, x uint32) {
	XnativeSqlite3BtreeCursorHintFlags(ctx.tls, pCur.ptr, x)
}

func (nativeBtreeStorageEngine) BtreeLastPage(ctx BtreeContext, p BtreeHandle) (r uint32) {
	return uint32(XnativeSqlite3BtreeLastPage(ctx.tls, p.ptr))
}

func (nativeBtreeStorageEngine) BtreeOpen(ctx BtreeContext, pVfs BtreeVFSHandle, zFilename BtreeCStringHandle, db SQLiteHandle, ppBtree BtreeMemoryHandle, flags int32, vfsFlags int32) (r int32) {
	return XnativeSqlite3BtreeOpen(ctx.tls, pVfs.ptr, zFilename.ptr, db.ptr, ppBtree.ptr, flags, vfsFlags)
}

func (nativeBtreeStorageEngine) BtreeClose(ctx BtreeContext, p BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeClose(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeSetCacheSize(ctx BtreeContext, p BtreeHandle, mxPage int32) (r int32) {
	return XnativeSqlite3BtreeSetCacheSize(ctx.tls, p.ptr, mxPage)
}

func (nativeBtreeStorageEngine) BtreeSetSpillSize(ctx BtreeContext, p BtreeHandle, mxPage int32) (r int32) {
	return XnativeSqlite3BtreeSetSpillSize(ctx.tls, p.ptr, mxPage)
}

func (nativeBtreeStorageEngine) BtreeSetPagerFlags(ctx BtreeContext, p BtreeHandle, pgFlags uint32) (r int32) {
	return XnativeSqlite3BtreeSetPagerFlags(ctx.tls, p.ptr, pgFlags)
}

func (nativeBtreeStorageEngine) BtreeSetPageSize(ctx BtreeContext, p BtreeHandle, pageSize int32, nReserve int32, iFix int32) (r int32) {
	return XnativeSqlite3BtreeSetPageSize(ctx.tls, p.ptr, pageSize, nReserve, iFix)
}

func (nativeBtreeStorageEngine) BtreeGetPageSize(ctx BtreeContext, p BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeGetPageSize(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeGetReserveNoMutex(ctx BtreeContext, p BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeGetReserveNoMutex(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeGetRequestedReserve(ctx BtreeContext, p BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeGetRequestedReserve(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeMaxPageCount(ctx BtreeContext, p BtreeHandle, mxPage uint32) (r uint32) {
	return uint32(XnativeSqlite3BtreeMaxPageCount(ctx.tls, p.ptr, Pgno(mxPage)))
}

func (nativeBtreeStorageEngine) BtreeSecureDelete(ctx BtreeContext, p BtreeHandle, newFlag int32) (r int32) {
	return XnativeSqlite3BtreeSecureDelete(ctx.tls, p.ptr, newFlag)
}

func (nativeBtreeStorageEngine) BtreeSetAutoVacuum(ctx BtreeContext, p BtreeHandle, autoVacuum int32) (r int32) {
	return XnativeSqlite3BtreeSetAutoVacuum(ctx.tls, p.ptr, autoVacuum)
}

func (nativeBtreeStorageEngine) BtreeGetAutoVacuum(ctx BtreeContext, p BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeGetAutoVacuum(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeNewDb(ctx BtreeContext, p BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeNewDb(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeBeginTrans(ctx BtreeContext, p BtreeHandle, wrflag int32, pSchemaVersion BtreeMemoryHandle) (r int32) {
	return XnativeSqlite3BtreeBeginTrans(ctx.tls, p.ptr, wrflag, pSchemaVersion.ptr)
}

func (nativeBtreeStorageEngine) BtreeIncrVacuum(ctx BtreeContext, p BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeIncrVacuum(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeCommitPhaseOne(ctx BtreeContext, p BtreeHandle, zSuperJrnl BtreeCStringHandle) (r int32) {
	return XnativeSqlite3BtreeCommitPhaseOne(ctx.tls, p.ptr, zSuperJrnl.ptr)
}

func (nativeBtreeStorageEngine) BtreeCommitPhaseTwo(ctx BtreeContext, p BtreeHandle, bCleanup int32) (r int32) {
	return XnativeSqlite3BtreeCommitPhaseTwo(ctx.tls, p.ptr, bCleanup)
}

func (nativeBtreeStorageEngine) BtreeCommit(ctx BtreeContext, p BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeCommit(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeTripAllCursors(ctx BtreeContext, pBtree BtreeHandle, errCode int32, writeOnly int32) (r int32) {
	return XnativeSqlite3BtreeTripAllCursors(ctx.tls, pBtree.ptr, errCode, writeOnly)
}

func (nativeBtreeStorageEngine) BtreeRollback(ctx BtreeContext, p BtreeHandle, tripCode int32, writeOnly int32) (r int32) {
	return XnativeSqlite3BtreeRollback(ctx.tls, p.ptr, tripCode, writeOnly)
}

func (nativeBtreeStorageEngine) BtreeBeginStmt(ctx BtreeContext, p BtreeHandle, iStatement int32) (r int32) {
	return XnativeSqlite3BtreeBeginStmt(ctx.tls, p.ptr, iStatement)
}

func (nativeBtreeStorageEngine) BtreeSavepoint(ctx BtreeContext, p BtreeHandle, op int32, iSavepoint int32) (r int32) {
	return XnativeSqlite3BtreeSavepoint(ctx.tls, p.ptr, op, iSavepoint)
}

func (nativeBtreeStorageEngine) BtreeCursor(ctx BtreeContext, p BtreeHandle, iTable uint32, wrFlag int32, pKeyInfo BtreeKeyInfoHandle, pCur BtreeCursorHandle) (r int32) {
	return XnativeSqlite3BtreeCursor(ctx.tls, p.ptr, Pgno(iTable), wrFlag, pKeyInfo.ptr, pCur.ptr)
}

func (nativeBtreeStorageEngine) BtreeCursorSize(ctx BtreeContext) (r int32) {
	return XnativeSqlite3BtreeCursorSize(ctx.tls)
}

func (nativeBtreeStorageEngine) BtreeCursorZero(ctx BtreeContext, p BtreeCursorHandle) {
	XnativeSqlite3BtreeCursorZero(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeCloseCursor(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	return XnativeSqlite3BtreeCloseCursor(ctx.tls, pCur.ptr)
}

func (nativeBtreeStorageEngine) BtreeCursorIsValidNN(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	return XnativeSqlite3BtreeCursorIsValidNN(ctx.tls, pCur.ptr)
}

func (nativeBtreeStorageEngine) BtreeIntegerKey(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	return int64(XnativeSqlite3BtreeIntegerKey(ctx.tls, pCur.ptr))
}

func (nativeBtreeStorageEngine) BtreeCursorPin(ctx BtreeContext, pCur BtreeCursorHandle) {
	XnativeSqlite3BtreeCursorPin(ctx.tls, pCur.ptr)
}

func (nativeBtreeStorageEngine) BtreeCursorUnpin(ctx BtreeContext, pCur BtreeCursorHandle) {
	XnativeSqlite3BtreeCursorUnpin(ctx.tls, pCur.ptr)
}

func (nativeBtreeStorageEngine) BtreeOffset(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	return int64(XnativeSqlite3BtreeOffset(ctx.tls, pCur.ptr))
}

func (nativeBtreeStorageEngine) BtreePayloadSize(ctx BtreeContext, pCur BtreeCursorHandle) (r uint32) {
	return uint32(XnativeSqlite3BtreePayloadSize(ctx.tls, pCur.ptr))
}

func (nativeBtreeStorageEngine) BtreeMaxRecordSize(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	return int64(XnativeSqlite3BtreeMaxRecordSize(ctx.tls, pCur.ptr))
}

func (nativeBtreeStorageEngine) BtreePayload(ctx BtreeContext, pCur BtreeCursorHandle, offset uint32, amt uint32, pBuf BtreeMemoryHandle) (r int32) {
	return XnativeSqlite3BtreePayload(ctx.tls, pCur.ptr, U32(offset), U32(amt), pBuf.ptr)
}

func (nativeBtreeStorageEngine) BtreePayloadChecked(ctx BtreeContext, pCur BtreeCursorHandle, offset uint32, amt uint32, pBuf BtreeMemoryHandle) (r int32) {
	return XnativeSqlite3BtreePayloadChecked(ctx.tls, pCur.ptr, U32(offset), U32(amt), pBuf.ptr)
}

func (nativeBtreeStorageEngine) BtreePayloadFetch(ctx BtreeContext, pCur BtreeCursorHandle, pAmt BtreeMemoryHandle) (r BtreeMemoryHandle) {
	return btreeMemoryHandle(ctx.tls, XnativeSqlite3BtreePayloadFetch(ctx.tls, pCur.ptr, pAmt.ptr))
}

func (nativeBtreeStorageEngine) BtreeFirst(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	return XnativeSqlite3BtreeFirst(ctx.tls, pCur.ptr, pRes.ptr)
}

func (nativeBtreeStorageEngine) BtreeLast(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	return XnativeSqlite3BtreeLast(ctx.tls, pCur.ptr, pRes.ptr)
}

func (nativeBtreeStorageEngine) BtreeTableMoveto(ctx BtreeContext, pCur BtreeCursorHandle, intKey int64, biasRight int32, pRes BtreeMemoryHandle) (r int32) {
	return XnativeSqlite3BtreeTableMoveto(ctx.tls, pCur.ptr, I64(intKey), biasRight, pRes.ptr)
}

func (nativeBtreeStorageEngine) BtreeIndexMoveto(ctx BtreeContext, pCur BtreeCursorHandle, pIdxKey BtreeIndexKeyHandle, pRes BtreeMemoryHandle) (r int32) {
	return XnativeSqlite3BtreeIndexMoveto(ctx.tls, pCur.ptr, pIdxKey.ptr, pRes.ptr)
}

func (nativeBtreeStorageEngine) BtreeEof(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	return XnativeSqlite3BtreeEof(ctx.tls, pCur.ptr)
}

func (nativeBtreeStorageEngine) BtreeRowCountEst(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	return int64(XnativeSqlite3BtreeRowCountEst(ctx.tls, pCur.ptr))
}

func (nativeBtreeStorageEngine) BtreeNext(ctx BtreeContext, pCur BtreeCursorHandle, flags int32) (r int32) {
	return XnativeSqlite3BtreeNext(ctx.tls, pCur.ptr, flags)
}

func (nativeBtreeStorageEngine) BtreePrevious(ctx BtreeContext, pCur BtreeCursorHandle, flags int32) (r int32) {
	return XnativeSqlite3BtreePrevious(ctx.tls, pCur.ptr, flags)
}

func (nativeBtreeStorageEngine) BtreeInsert(ctx BtreeContext, pCur BtreeCursorHandle, pX BtreePayloadHandle, flags int32, seekResult int32) (r int32) {
	return XnativeSqlite3BtreeInsert(ctx.tls, pCur.ptr, pX.ptr, flags, seekResult)
}

func (nativeBtreeStorageEngine) BtreeTransferRow(ctx BtreeContext, pDest BtreeCursorHandle, pSrc BtreeCursorHandle, iKey int64) (r int32) {
	return XnativeSqlite3BtreeTransferRow(ctx.tls, pDest.ptr, pSrc.ptr, I64(iKey))
}

func (nativeBtreeStorageEngine) BtreeDelete(ctx BtreeContext, pCur BtreeCursorHandle, flags uint8) (r int32) {
	return XnativeSqlite3BtreeDelete(ctx.tls, pCur.ptr, U8(flags))
}

func (nativeBtreeStorageEngine) BtreeCreateTable(ctx BtreeContext, p BtreeHandle, piTable BtreeMemoryHandle, flags int32) (r int32) {
	return XnativeSqlite3BtreeCreateTable(ctx.tls, p.ptr, piTable.ptr, flags)
}

func (nativeBtreeStorageEngine) BtreeClearTable(ctx BtreeContext, p BtreeHandle, iTable int32, pnChange BtreeMemoryHandle) (r int32) {
	return XnativeSqlite3BtreeClearTable(ctx.tls, p.ptr, iTable, pnChange.ptr)
}

func (nativeBtreeStorageEngine) BtreeClearTableOfCursor(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	return XnativeSqlite3BtreeClearTableOfCursor(ctx.tls, pCur.ptr)
}

func (nativeBtreeStorageEngine) BtreeDropTable(ctx BtreeContext, p BtreeHandle, iTable int32, piMoved BtreeMemoryHandle) (r int32) {
	return XnativeSqlite3BtreeDropTable(ctx.tls, p.ptr, iTable, piMoved.ptr)
}

func (nativeBtreeStorageEngine) BtreeGetMeta(ctx BtreeContext, p BtreeHandle, idx int32, pMeta BtreeMemoryHandle) {
	XnativeSqlite3BtreeGetMeta(ctx.tls, p.ptr, idx, pMeta.ptr)
}

func (nativeBtreeStorageEngine) BtreeUpdateMeta(ctx BtreeContext, p BtreeHandle, idx int32, iMeta uint32) (r int32) {
	return XnativeSqlite3BtreeUpdateMeta(ctx.tls, p.ptr, idx, U32(iMeta))
}

func (nativeBtreeStorageEngine) BtreeCount(ctx BtreeContext, db SQLiteHandle, pCur BtreeCursorHandle, pnEntry BtreeMemoryHandle) (r int32) {
	return XnativeSqlite3BtreeCount(ctx.tls, db.ptr, pCur.ptr, pnEntry.ptr)
}

func (nativeBtreeStorageEngine) BtreePager(ctx BtreeContext, p BtreeHandle) (r BtreePagerHandle) {
	return btreePagerHandle(ctx.tls, XnativeSqlite3BtreePager(ctx.tls, p.ptr))
}

func (nativeBtreeStorageEngine) BtreeIntegrityCheckNetbsdAmd64(ctx BtreeContext, db SQLiteHandle, p BtreeHandle, aRoot BtreeMemoryHandle, nRoot int32, mxErr int32, pnErr BtreeMemoryHandle) (r BtreeCStringHandle) {
	return btreeCStringHandle(ctx.tls, XnativeSqlite3BtreeIntegrityCheck(ctx.tls, db.ptr, p.ptr, aRoot.ptr, nRoot, mxErr, pnErr.ptr))
}

func (nativeBtreeStorageEngine) BtreeGetFilename(ctx BtreeContext, p BtreeHandle) (r BtreeCStringHandle) {
	return btreeCStringHandle(ctx.tls, XnativeSqlite3BtreeGetFilename(ctx.tls, p.ptr))
}

func (nativeBtreeStorageEngine) BtreeGetJournalname(ctx BtreeContext, p BtreeHandle) (r BtreeCStringHandle) {
	return btreeCStringHandle(ctx.tls, XnativeSqlite3BtreeGetJournalname(ctx.tls, p.ptr))
}

func (nativeBtreeStorageEngine) BtreeTxnState(ctx BtreeContext, p BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeTxnState(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeCheckpoint(ctx BtreeContext, p BtreeHandle, eMode int32, pnLog BtreeMemoryHandle, pnCkpt BtreeMemoryHandle) (r int32) {
	return XnativeSqlite3BtreeCheckpoint(ctx.tls, p.ptr, eMode, pnLog.ptr, pnCkpt.ptr)
}

func (nativeBtreeStorageEngine) BtreeIsInBackup(ctx BtreeContext, p BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeIsInBackup(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeSchema(ctx BtreeContext, p BtreeHandle, nBytes int32, xFree BtreeFunctionHandle) (r BtreeSchemaHandle) {
	return btreeSchemaHandle(ctx.tls, XnativeSqlite3BtreeSchema(ctx.tls, p.ptr, nBytes, xFree.ptr))
}

func (nativeBtreeStorageEngine) BtreeSchemaLocked(ctx BtreeContext, p BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeSchemaLocked(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeLockTable(ctx BtreeContext, p BtreeHandle, iTab int32, isWriteLock uint8) (r int32) {
	return XnativeSqlite3BtreeLockTable(ctx.tls, p.ptr, iTab, U8(isWriteLock))
}

func (nativeBtreeStorageEngine) BtreePutData(ctx BtreeContext, pCsr BtreeCursorHandle, offset uint32, amt uint32, z BtreeMemoryHandle) (r int32) {
	return XnativeSqlite3BtreePutData(ctx.tls, pCsr.ptr, U32(offset), U32(amt), z.ptr)
}

func (nativeBtreeStorageEngine) BtreeIncrblobCursor(ctx BtreeContext, pCur BtreeCursorHandle) {
	XnativeSqlite3BtreeIncrblobCursor(ctx.tls, pCur.ptr)
}

func (nativeBtreeStorageEngine) BtreeSetVersion(ctx BtreeContext, pBtree BtreeHandle, iVersion int32) (r int32) {
	return XnativeSqlite3BtreeSetVersion(ctx.tls, pBtree.ptr, iVersion)
}

func (nativeBtreeStorageEngine) BtreeCursorHasHint(ctx BtreeContext, pCsr BtreeCursorHandle, mask uint32) (r int32) {
	return XnativeSqlite3BtreeCursorHasHint(ctx.tls, pCsr.ptr, mask)
}

func (nativeBtreeStorageEngine) BtreeIsReadonly(ctx BtreeContext, p BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeIsReadonly(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeSharable(ctx BtreeContext, p BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeSharable(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeConnectionCount(ctx BtreeContext, p BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeConnectionCount(ctx.tls, p.ptr)
}

func (nativeBtreeStorageEngine) BtreeCopyFile(ctx BtreeContext, pTo BtreeHandle, pFrom BtreeHandle) (r int32) {
	return XnativeSqlite3BtreeCopyFile(ctx.tls, pTo.ptr, pFrom.ptr)
}
