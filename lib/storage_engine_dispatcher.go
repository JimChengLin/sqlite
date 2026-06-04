// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite3

var _ StorageEngine = storageEngineDispatcher{}
var _ StorageEngineBtreeSetMmapLimit = storageEngineDispatcher{}
var _ StorageEngineBtreeIsEmpty = storageEngineDispatcher{}
var _ StorageEngineBtreeIntegrityCheck = storageEngineDispatcher{}
var _ StorageEngineBtreeIntegrityCheckFreebsd386 = storageEngineDispatcher{}
var _ StorageEngineBtreeIntegrityCheckNetbsdAmd64 = storageEngineDispatcher{}
var _ StorageEngineBtreeClearCache = storageEngineDispatcher{}

func (storageEngineDispatcher) BtreeEnter(ctx BtreeContext, p BtreeHandle) {
	storageEngineForBtreeHandle(p).BtreeEnter(ctx, p)
}

func (storageEngineDispatcher) BtreeLeave(ctx BtreeContext, p BtreeHandle) {
	storageEngineForBtreeHandle(p).BtreeLeave(ctx, p)
}

func (storageEngineDispatcher) BtreeEnterAll(ctx BtreeContext, db SQLiteHandle) {
	storageEngineForDB(db).BtreeEnterAll(ctx, db)
}

func (storageEngineDispatcher) BtreeLeaveAll(ctx BtreeContext, db SQLiteHandle) {
	storageEngineForDB(db).BtreeLeaveAll(ctx, db)
}

func (storageEngineDispatcher) BtreeEnterCursor(ctx BtreeContext, pCur BtreeCursorHandle) {
	storageEngineForCursorHandle(pCur).BtreeEnterCursor(ctx, pCur)
}

func (storageEngineDispatcher) BtreeLeaveCursor(ctx BtreeContext, pCur BtreeCursorHandle) {
	storageEngineForCursorHandle(pCur).BtreeLeaveCursor(ctx, pCur)
}

func (storageEngineDispatcher) BtreeClearCursor(ctx BtreeContext, pCur BtreeCursorHandle) {
	storageEngineForCursorHandle(pCur).BtreeClearCursor(ctx, pCur)
}

func (storageEngineDispatcher) BtreeCursorHasMoved(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreeCursorHasMoved(ctx, pCur)
}

func (storageEngineDispatcher) BtreeFakeValidCursor(ctx BtreeContext) (r BtreeCursorHandle) {
	return selectedStorageEngine().BtreeFakeValidCursor(ctx)
}

func (storageEngineDispatcher) BtreeCursorRestore(ctx BtreeContext, pCur BtreeCursorHandle, pDifferentRow BtreeMemoryHandle) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreeCursorRestore(ctx, pCur, pDifferentRow)
}

func (storageEngineDispatcher) BtreeCursorHintFlags(ctx BtreeContext, pCur BtreeCursorHandle, x uint32) {
	storageEngineForCursorHandle(pCur).BtreeCursorHintFlags(ctx, pCur, x)
}

func (storageEngineDispatcher) BtreeLastPage(ctx BtreeContext, p BtreeHandle) (r uint32) {
	return storageEngineForBtreeHandle(p).BtreeLastPage(ctx, p)
}

func (storageEngineDispatcher) BtreeSetCacheSize(ctx BtreeContext, p BtreeHandle, mxPage int32) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeSetCacheSize(ctx, p, mxPage)
}

func (storageEngineDispatcher) BtreeSetSpillSize(ctx BtreeContext, p BtreeHandle, mxPage int32) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeSetSpillSize(ctx, p, mxPage)
}

func (storageEngineDispatcher) BtreeSetPagerFlags(ctx BtreeContext, p BtreeHandle, pgFlags uint32) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeSetPagerFlags(ctx, p, pgFlags)
}

func (storageEngineDispatcher) BtreeSetPageSize(ctx BtreeContext, p BtreeHandle, pageSize int32, nReserve int32, iFix int32) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeSetPageSize(ctx, p, pageSize, nReserve, iFix)
}

func (storageEngineDispatcher) BtreeGetPageSize(ctx BtreeContext, p BtreeHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeGetPageSize(ctx, p)
}

func (storageEngineDispatcher) BtreeGetReserveNoMutex(ctx BtreeContext, p BtreeHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeGetReserveNoMutex(ctx, p)
}

func (storageEngineDispatcher) BtreeGetRequestedReserve(ctx BtreeContext, p BtreeHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeGetRequestedReserve(ctx, p)
}

func (storageEngineDispatcher) BtreeMaxPageCount(ctx BtreeContext, p BtreeHandle, mxPage uint32) (r uint32) {
	return storageEngineForBtreeHandle(p).BtreeMaxPageCount(ctx, p, mxPage)
}

func (storageEngineDispatcher) BtreeSecureDelete(ctx BtreeContext, p BtreeHandle, newFlag int32) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeSecureDelete(ctx, p, newFlag)
}

func (storageEngineDispatcher) BtreeSetAutoVacuum(ctx BtreeContext, p BtreeHandle, autoVacuum int32) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeSetAutoVacuum(ctx, p, autoVacuum)
}

func (storageEngineDispatcher) BtreeGetAutoVacuum(ctx BtreeContext, p BtreeHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeGetAutoVacuum(ctx, p)
}

func (storageEngineDispatcher) BtreeNewDb(ctx BtreeContext, p BtreeHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeNewDb(ctx, p)
}

func (storageEngineDispatcher) BtreeBeginTrans(ctx BtreeContext, p BtreeHandle, wrflag int32, pSchemaVersion BtreeMemoryHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeBeginTrans(ctx, p, wrflag, pSchemaVersion)
}

func (storageEngineDispatcher) BtreeIncrVacuum(ctx BtreeContext, p BtreeHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeIncrVacuum(ctx, p)
}

func (storageEngineDispatcher) BtreeCommitPhaseOne(ctx BtreeContext, p BtreeHandle, zSuperJrnl BtreeCStringHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeCommitPhaseOne(ctx, p, zSuperJrnl)
}

func (storageEngineDispatcher) BtreeCommitPhaseTwo(ctx BtreeContext, p BtreeHandle, bCleanup int32) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeCommitPhaseTwo(ctx, p, bCleanup)
}

func (storageEngineDispatcher) BtreeCommit(ctx BtreeContext, p BtreeHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeCommit(ctx, p)
}

func (storageEngineDispatcher) BtreeTripAllCursors(ctx BtreeContext, pBtree BtreeHandle, errCode int32, writeOnly int32) (r int32) {
	return storageEngineForBtreeHandle(pBtree).BtreeTripAllCursors(ctx, pBtree, errCode, writeOnly)
}

func (storageEngineDispatcher) BtreeRollback(ctx BtreeContext, p BtreeHandle, tripCode int32, writeOnly int32) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeRollback(ctx, p, tripCode, writeOnly)
}

func (storageEngineDispatcher) BtreeBeginStmt(ctx BtreeContext, p BtreeHandle, iStatement int32) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeBeginStmt(ctx, p, iStatement)
}

func (storageEngineDispatcher) BtreeSavepoint(ctx BtreeContext, p BtreeHandle, op int32, iSavepoint int32) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeSavepoint(ctx, p, op, iSavepoint)
}

func (storageEngineDispatcher) BtreeCursorSize(ctx BtreeContext) (r int32) {
	return selectedStorageEngine().BtreeCursorSize(ctx)
}

func (storageEngineDispatcher) BtreeCursorIsValidNN(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreeCursorIsValidNN(ctx, pCur)
}

func (storageEngineDispatcher) BtreeIntegerKey(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	return storageEngineForCursorHandle(pCur).BtreeIntegerKey(ctx, pCur)
}

func (storageEngineDispatcher) BtreeCursorPin(ctx BtreeContext, pCur BtreeCursorHandle) {
	storageEngineForCursorHandle(pCur).BtreeCursorPin(ctx, pCur)
}

func (storageEngineDispatcher) BtreeCursorUnpin(ctx BtreeContext, pCur BtreeCursorHandle) {
	storageEngineForCursorHandle(pCur).BtreeCursorUnpin(ctx, pCur)
}

func (storageEngineDispatcher) BtreeOffset(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	return storageEngineForCursorHandle(pCur).BtreeOffset(ctx, pCur)
}

func (storageEngineDispatcher) BtreePayloadSize(ctx BtreeContext, pCur BtreeCursorHandle) (r uint32) {
	return storageEngineForCursorHandle(pCur).BtreePayloadSize(ctx, pCur)
}

func (storageEngineDispatcher) BtreeMaxRecordSize(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	return storageEngineForCursorHandle(pCur).BtreeMaxRecordSize(ctx, pCur)
}

func (storageEngineDispatcher) BtreePayload(ctx BtreeContext, pCur BtreeCursorHandle, offset uint32, amt uint32, pBuf BtreeMemoryHandle) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreePayload(ctx, pCur, offset, amt, pBuf)
}

func (storageEngineDispatcher) BtreePayloadChecked(ctx BtreeContext, pCur BtreeCursorHandle, offset uint32, amt uint32, pBuf BtreeMemoryHandle) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreePayloadChecked(ctx, pCur, offset, amt, pBuf)
}

func (storageEngineDispatcher) BtreePayloadFetch(ctx BtreeContext, pCur BtreeCursorHandle, pAmt BtreeMemoryHandle) (r BtreeMemoryHandle) {
	return storageEngineForCursorHandle(pCur).BtreePayloadFetch(ctx, pCur, pAmt)
}

func (storageEngineDispatcher) BtreeFirst(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreeFirst(ctx, pCur, pRes)
}

func (storageEngineDispatcher) BtreeLast(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreeLast(ctx, pCur, pRes)
}

func (storageEngineDispatcher) BtreeTableMoveto(ctx BtreeContext, pCur BtreeCursorHandle, intKey int64, biasRight int32, pRes BtreeMemoryHandle) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreeTableMoveto(ctx, pCur, intKey, biasRight, pRes)
}

func (storageEngineDispatcher) BtreeIndexMoveto(ctx BtreeContext, pCur BtreeCursorHandle, pIdxKey BtreeIndexKeyHandle, pRes BtreeMemoryHandle) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreeIndexMoveto(ctx, pCur, pIdxKey, pRes)
}

func (storageEngineDispatcher) BtreeEof(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreeEof(ctx, pCur)
}

func (storageEngineDispatcher) BtreeRowCountEst(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	return storageEngineForCursorHandle(pCur).BtreeRowCountEst(ctx, pCur)
}

func (storageEngineDispatcher) BtreeNext(ctx BtreeContext, pCur BtreeCursorHandle, flags int32) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreeNext(ctx, pCur, flags)
}

func (storageEngineDispatcher) BtreePrevious(ctx BtreeContext, pCur BtreeCursorHandle, flags int32) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreePrevious(ctx, pCur, flags)
}

func (storageEngineDispatcher) BtreeInsert(ctx BtreeContext, pCur BtreeCursorHandle, pX BtreePayloadHandle, flags int32, seekResult int32) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreeInsert(ctx, pCur, pX, flags, seekResult)
}

func (storageEngineDispatcher) BtreeTransferRow(ctx BtreeContext, pDest BtreeCursorHandle, pSrc BtreeCursorHandle, iKey int64) (r int32) {
	return storageEngineForCursorHandle(pDest).BtreeTransferRow(ctx, pDest, pSrc, iKey)
}

func (storageEngineDispatcher) BtreeDelete(ctx BtreeContext, pCur BtreeCursorHandle, flags uint8) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreeDelete(ctx, pCur, flags)
}

func (storageEngineDispatcher) BtreeCreateTable(ctx BtreeContext, p BtreeHandle, piTable BtreeMemoryHandle, flags int32) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeCreateTable(ctx, p, piTable, flags)
}

func (storageEngineDispatcher) BtreeClearTable(ctx BtreeContext, p BtreeHandle, iTable int32, pnChange BtreeMemoryHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeClearTable(ctx, p, iTable, pnChange)
}

func (storageEngineDispatcher) BtreeClearTableOfCursor(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	return storageEngineForCursorHandle(pCur).BtreeClearTableOfCursor(ctx, pCur)
}

func (storageEngineDispatcher) BtreeDropTable(ctx BtreeContext, p BtreeHandle, iTable int32, piMoved BtreeMemoryHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeDropTable(ctx, p, iTable, piMoved)
}

func (storageEngineDispatcher) BtreeGetMeta(ctx BtreeContext, p BtreeHandle, idx int32, pMeta BtreeMemoryHandle) {
	storageEngineForBtreeHandle(p).BtreeGetMeta(ctx, p, idx, pMeta)
}

func (storageEngineDispatcher) BtreeUpdateMeta(ctx BtreeContext, p BtreeHandle, idx int32, iMeta uint32) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeUpdateMeta(ctx, p, idx, iMeta)
}

func (storageEngineDispatcher) BtreeCount(ctx BtreeContext, db SQLiteHandle, pCur BtreeCursorHandle, pnEntry BtreeMemoryHandle) (r int32) {
	return storageEngineForCursorOrDB(pCur, db).BtreeCount(ctx, db, pCur, pnEntry)
}

func (storageEngineDispatcher) BtreePager(ctx BtreeContext, p BtreeHandle) (r BtreePagerHandle) {
	return storageEngineForBtreeHandle(p).BtreePager(ctx, p)
}

func (storageEngineDispatcher) BtreeGetFilename(ctx BtreeContext, p BtreeHandle) (r BtreeCStringHandle) {
	return storageEngineForBtreeHandle(p).BtreeGetFilename(ctx, p)
}

func (storageEngineDispatcher) BtreeGetJournalname(ctx BtreeContext, p BtreeHandle) (r BtreeCStringHandle) {
	return storageEngineForBtreeHandle(p).BtreeGetJournalname(ctx, p)
}

func (storageEngineDispatcher) BtreeTxnState(ctx BtreeContext, p BtreeHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeTxnState(ctx, p)
}

func (storageEngineDispatcher) BtreeCheckpoint(ctx BtreeContext, p BtreeHandle, eMode int32, pnLog BtreeMemoryHandle, pnCkpt BtreeMemoryHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeCheckpoint(ctx, p, eMode, pnLog, pnCkpt)
}

func (storageEngineDispatcher) BtreeIsInBackup(ctx BtreeContext, p BtreeHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeIsInBackup(ctx, p)
}

func (storageEngineDispatcher) BtreeSchema(ctx BtreeContext, p BtreeHandle, nBytes int32, __ccgo_fp_xFree BtreeFunctionHandle) (r BtreeSchemaHandle) {
	return storageEngineForBtreeHandle(p).BtreeSchema(ctx, p, nBytes, __ccgo_fp_xFree)
}

func (storageEngineDispatcher) BtreeSchemaLocked(ctx BtreeContext, p BtreeHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeSchemaLocked(ctx, p)
}

func (storageEngineDispatcher) BtreeLockTable(ctx BtreeContext, p BtreeHandle, iTab int32, isWriteLock uint8) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeLockTable(ctx, p, iTab, isWriteLock)
}

func (storageEngineDispatcher) BtreePutData(ctx BtreeContext, pCsr BtreeCursorHandle, offset uint32, amt uint32, z BtreeMemoryHandle) (r int32) {
	return storageEngineForCursorHandle(pCsr).BtreePutData(ctx, pCsr, offset, amt, z)
}

func (storageEngineDispatcher) BtreeIncrblobCursor(ctx BtreeContext, pCur BtreeCursorHandle) {
	storageEngineForCursorHandle(pCur).BtreeIncrblobCursor(ctx, pCur)
}

func (storageEngineDispatcher) BtreeSetVersion(ctx BtreeContext, pBtree BtreeHandle, iVersion int32) (r int32) {
	return storageEngineForBtreeHandle(pBtree).BtreeSetVersion(ctx, pBtree, iVersion)
}

func (storageEngineDispatcher) BtreeCursorHasHint(ctx BtreeContext, pCsr BtreeCursorHandle, mask uint32) (r int32) {
	return storageEngineForCursorHandle(pCsr).BtreeCursorHasHint(ctx, pCsr, mask)
}

func (storageEngineDispatcher) BtreeIsReadonly(ctx BtreeContext, p BtreeHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeIsReadonly(ctx, p)
}

func (storageEngineDispatcher) BtreeSharable(ctx BtreeContext, p BtreeHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeSharable(ctx, p)
}

func (storageEngineDispatcher) BtreeConnectionCount(ctx BtreeContext, p BtreeHandle) (r int32) {
	return storageEngineForBtreeHandle(p).BtreeConnectionCount(ctx, p)
}

func (storageEngineDispatcher) BtreeCopyFile(ctx BtreeContext, pTo BtreeHandle, pFrom BtreeHandle) (r int32) {
	return storageEngineForBtreeHandle(pTo).BtreeCopyFile(ctx, pTo, pFrom)
}

func (storageEngineDispatcher) BtreeOpen(ctx BtreeContext, pVfs BtreeVFSHandle, zFilename BtreeCStringHandle, db SQLiteHandle, ppBtree BtreeMemoryHandle, flags int32, vfsFlags int32) (r int32) {
	engine := storageEngineForDB(db)
	if db.ptr == 0 {
		engine = selectedStorageEngine()
	}
	r = engine.BtreeOpen(ctx, pVfs, zFilename, db, ppBtree, flags, vfsFlags)
	if r == SQLITE_OK && !ppBtree.IsNil() {
		registerStorageEngineBtree(btreeHandle(ctx.tls, ppBtree.GetUintptr()), db, engine)
	}
	return r
}

func (storageEngineDispatcher) BtreeClose(ctx BtreeContext, p BtreeHandle) (r int32) {
	engine := storageEngineForBtreeHandle(p)
	r = engine.BtreeClose(ctx, p)
	unregisterStorageEngineBtree(p)
	return r
}

func (storageEngineDispatcher) BtreeCursor(ctx BtreeContext, p BtreeHandle, iTable uint32, wrFlag int32, pKeyInfo BtreeKeyInfoHandle, pCur BtreeCursorHandle) (r int32) {
	engine := storageEngineForBtreeHandle(p)
	r = engine.BtreeCursor(ctx, p, iTable, wrFlag, pKeyInfo, pCur)
	if r == SQLITE_OK {
		registerStorageEngineCursor(pCur, engine)
	}
	return r
}

func (storageEngineDispatcher) BtreeCursorZero(ctx BtreeContext, p BtreeCursorHandle) {
	nativeBtreeStorageEngine{}.BtreeCursorZero(ctx, p)
}

func (storageEngineDispatcher) BtreeCloseCursor(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	engine := storageEngineForCursorHandle(pCur)
	r = engine.BtreeCloseCursor(ctx, pCur)
	unregisterStorageEngineCursor(pCur)
	return r
}

func (storageEngineDispatcher) BtreeSetMmapLimit(ctx BtreeContext, p BtreeHandle, szMmap int64) (r int32) {
	engine, ok := storageEngineForBtreeHandle(p).(StorageEngineBtreeSetMmapLimit)
	if !ok {
		return SQLITE_ERROR
	}
	return engine.BtreeSetMmapLimit(ctx, p, szMmap)
}

func (storageEngineDispatcher) BtreeIsEmpty(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	engine, ok := storageEngineForCursorHandle(pCur).(StorageEngineBtreeIsEmpty)
	if !ok {
		return SQLITE_ERROR
	}
	return engine.BtreeIsEmpty(ctx, pCur, pRes)
}

func (storageEngineDispatcher) BtreeIntegrityCheck(ctx BtreeContext, db SQLiteHandle, p BtreeHandle, aRoot BtreeMemoryHandle, aCnt BtreeMemoryHandle, nRoot int32, mxErr int32, pnErr BtreeMemoryHandle, pzOut BtreeMemoryHandle) (r int32) {
	engine, ok := storageEngineForBtreeHandle(p).(StorageEngineBtreeIntegrityCheck)
	if !ok {
		return SQLITE_ERROR
	}
	return engine.BtreeIntegrityCheck(ctx, db, p, aRoot, aCnt, nRoot, mxErr, pnErr, pzOut)
}

func (storageEngineDispatcher) BtreeIntegrityCheckFreebsd386(ctx BtreeContext, db SQLiteHandle, p BtreeHandle, aRoot BtreeMemoryHandle, nRoot int32, mxErr int32, pnErr BtreeMemoryHandle, pzOut BtreeMemoryHandle) (r int32) {
	engine, ok := storageEngineForBtreeHandle(p).(StorageEngineBtreeIntegrityCheckFreebsd386)
	if !ok {
		return SQLITE_ERROR
	}
	return engine.BtreeIntegrityCheckFreebsd386(ctx, db, p, aRoot, nRoot, mxErr, pnErr, pzOut)
}

func (storageEngineDispatcher) BtreeIntegrityCheckNetbsdAmd64(ctx BtreeContext, db SQLiteHandle, p BtreeHandle, aRoot BtreeMemoryHandle, nRoot int32, mxErr int32, pnErr BtreeMemoryHandle) (r BtreeCStringHandle) {
	engine, ok := storageEngineForBtreeHandle(p).(StorageEngineBtreeIntegrityCheckNetbsdAmd64)
	if !ok {
		return BtreeCStringHandle{}
	}
	return engine.BtreeIntegrityCheckNetbsdAmd64(ctx, db, p, aRoot, nRoot, mxErr, pnErr)
}

func (storageEngineDispatcher) BtreeClearCache(ctx BtreeContext, p BtreeHandle) {
	engine, ok := storageEngineForBtreeHandle(p).(StorageEngineBtreeClearCache)
	if !ok {
		return
	}
	engine.BtreeClearCache(ctx, p)
}
