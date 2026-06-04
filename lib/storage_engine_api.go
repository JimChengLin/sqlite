// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite3

import (
	"errors"
	"sync"
	"sync/atomic"
	"unsafe"

	"modernc.org/libc"
)

// ErrStorageEngineUnsupported is returned by storage-engine adapters when an
// engine does not implement an operation SQLite needs for the current query.
var ErrStorageEngineUnsupported = errors.New("sqlite storage engine: unsupported operation")

func storageEngineBtCursorFromPointer(pCur uintptr) *BtCursor {
	return (*BtCursor)(unsafe.Pointer(pCur)) //nolint:govet // pCur is a SQLite ABI pointer to BtCursor.
}

func storageEngineByteSliceFromPointer(p uintptr, n int) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(p)), n) //nolint:govet // p is a SQLite ABI pointer to n bytes.
}

func storageEnginePayloadFromPointer(p uintptr) *BtreePayload {
	return (*BtreePayload)(unsafe.Pointer(p)) //nolint:govet // p is a SQLite ABI pointer to BtreePayload.
}

func storageEngineInt32FromPointer(p uintptr) *int32 {
	return (*int32)(unsafe.Pointer(p)) //nolint:govet // p is a SQLite ABI pointer to int32.
}

func storageEngineUint32FromPointer(p uintptr) *uint32 {
	return (*uint32)(unsafe.Pointer(p)) //nolint:govet // p is a SQLite ABI pointer to uint32.
}

func storageEngineInt64FromPointer(p uintptr) *int64 {
	return (*int64)(unsafe.Pointer(p)) //nolint:govet // p is a SQLite ABI pointer to int64.
}

func storageEngineUintptrFromPointer(p uintptr) *uintptr {
	return (*uintptr)(unsafe.Pointer(p)) //nolint:govet // p is a SQLite ABI pointer to uintptr.
}

// StorageEngine is the btree storage engine dispatch surface.
// The default implementation delegates to the generated SQLite btree code.
type StorageEngine interface {
	BtreeEnter(ctx BtreeContext, p BtreeHandle)
	BtreeLeave(ctx BtreeContext, p BtreeHandle)
	BtreeEnterAll(ctx BtreeContext, db SQLiteHandle)
	BtreeLeaveAll(ctx BtreeContext, db SQLiteHandle)
	BtreeEnterCursor(ctx BtreeContext, pCur BtreeCursorHandle)
	BtreeLeaveCursor(ctx BtreeContext, pCur BtreeCursorHandle)
	BtreeClearCursor(ctx BtreeContext, pCur BtreeCursorHandle)
	BtreeCursorHasMoved(ctx BtreeContext, pCur BtreeCursorHandle) (r int32)
	BtreeFakeValidCursor(ctx BtreeContext) (r BtreeCursorHandle)
	BtreeCursorRestore(ctx BtreeContext, pCur BtreeCursorHandle, pDifferentRow BtreeMemoryHandle) (r int32)
	BtreeCursorHintFlags(ctx BtreeContext, pCur BtreeCursorHandle, x uint32)
	BtreeLastPage(ctx BtreeContext, p BtreeHandle) (r uint32)
	BtreeOpen(ctx BtreeContext, pVfs BtreeVFSHandle, zFilename BtreeCStringHandle, db SQLiteHandle, ppBtree BtreeMemoryHandle, flags int32, vfsFlags int32) (r int32)
	BtreeClose(ctx BtreeContext, p BtreeHandle) (r int32)
	BtreeSetCacheSize(ctx BtreeContext, p BtreeHandle, mxPage int32) (r int32)
	BtreeSetSpillSize(ctx BtreeContext, p BtreeHandle, mxPage int32) (r int32)
	BtreeSetPagerFlags(ctx BtreeContext, p BtreeHandle, pgFlags uint32) (r int32)
	BtreeSetPageSize(ctx BtreeContext, p BtreeHandle, pageSize int32, nReserve int32, iFix int32) (r int32)
	BtreeGetPageSize(ctx BtreeContext, p BtreeHandle) (r int32)
	BtreeGetReserveNoMutex(ctx BtreeContext, p BtreeHandle) (r int32)
	BtreeGetRequestedReserve(ctx BtreeContext, p BtreeHandle) (r int32)
	BtreeMaxPageCount(ctx BtreeContext, p BtreeHandle, mxPage uint32) (r uint32)
	BtreeSecureDelete(ctx BtreeContext, p BtreeHandle, newFlag int32) (r int32)
	BtreeSetAutoVacuum(ctx BtreeContext, p BtreeHandle, autoVacuum int32) (r int32)
	BtreeGetAutoVacuum(ctx BtreeContext, p BtreeHandle) (r int32)
	BtreeNewDb(ctx BtreeContext, p BtreeHandle) (r int32)
	BtreeBeginTrans(ctx BtreeContext, p BtreeHandle, wrflag int32, pSchemaVersion BtreeMemoryHandle) (r int32)
	BtreeIncrVacuum(ctx BtreeContext, p BtreeHandle) (r int32)
	BtreeCommitPhaseOne(ctx BtreeContext, p BtreeHandle, zSuperJrnl BtreeCStringHandle) (r int32)
	BtreeCommitPhaseTwo(ctx BtreeContext, p BtreeHandle, bCleanup int32) (r int32)
	BtreeCommit(ctx BtreeContext, p BtreeHandle) (r int32)
	BtreeTripAllCursors(ctx BtreeContext, pBtree BtreeHandle, errCode int32, writeOnly int32) (r int32)
	BtreeRollback(ctx BtreeContext, p BtreeHandle, tripCode int32, writeOnly int32) (r int32)
	BtreeBeginStmt(ctx BtreeContext, p BtreeHandle, iStatement int32) (r int32)
	BtreeSavepoint(ctx BtreeContext, p BtreeHandle, op int32, iSavepoint int32) (r int32)
	BtreeCursor(ctx BtreeContext, p BtreeHandle, iTable uint32, wrFlag int32, pKeyInfo BtreeKeyInfoHandle, pCur BtreeCursorHandle) (r int32)
	BtreeCursorSize(ctx BtreeContext) (r int32)
	BtreeCursorZero(ctx BtreeContext, p BtreeCursorHandle)
	BtreeCloseCursor(ctx BtreeContext, pCur BtreeCursorHandle) (r int32)
	BtreeCursorIsValidNN(ctx BtreeContext, pCur BtreeCursorHandle) (r int32)
	BtreeIntegerKey(ctx BtreeContext, pCur BtreeCursorHandle) (r int64)
	BtreeCursorPin(ctx BtreeContext, pCur BtreeCursorHandle)
	BtreeCursorUnpin(ctx BtreeContext, pCur BtreeCursorHandle)
	BtreeOffset(ctx BtreeContext, pCur BtreeCursorHandle) (r int64)
	BtreePayloadSize(ctx BtreeContext, pCur BtreeCursorHandle) (r uint32)
	BtreeMaxRecordSize(ctx BtreeContext, pCur BtreeCursorHandle) (r int64)
	BtreePayload(ctx BtreeContext, pCur BtreeCursorHandle, offset uint32, amt uint32, pBuf BtreeMemoryHandle) (r int32)
	BtreePayloadChecked(ctx BtreeContext, pCur BtreeCursorHandle, offset uint32, amt uint32, pBuf BtreeMemoryHandle) (r int32)
	BtreePayloadFetch(ctx BtreeContext, pCur BtreeCursorHandle, pAmt BtreeMemoryHandle) (r BtreeMemoryHandle)
	BtreeFirst(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32)
	BtreeLast(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32)
	BtreeTableMoveto(ctx BtreeContext, pCur BtreeCursorHandle, intKey int64, biasRight int32, pRes BtreeMemoryHandle) (r int32)
	BtreeIndexMoveto(ctx BtreeContext, pCur BtreeCursorHandle, pIdxKey BtreeIndexKeyHandle, pRes BtreeMemoryHandle) (r int32)
	BtreeEof(ctx BtreeContext, pCur BtreeCursorHandle) (r int32)
	BtreeRowCountEst(ctx BtreeContext, pCur BtreeCursorHandle) (r int64)
	BtreeNext(ctx BtreeContext, pCur BtreeCursorHandle, flags int32) (r int32)
	BtreePrevious(ctx BtreeContext, pCur BtreeCursorHandle, flags int32) (r int32)
	BtreeInsert(ctx BtreeContext, pCur BtreeCursorHandle, pX BtreePayloadHandle, flags int32, seekResult int32) (r int32)
	BtreeTransferRow(ctx BtreeContext, pDest BtreeCursorHandle, pSrc BtreeCursorHandle, iKey int64) (r int32)
	BtreeDelete(ctx BtreeContext, pCur BtreeCursorHandle, flags uint8) (r int32)
	BtreeCreateTable(ctx BtreeContext, p BtreeHandle, piTable BtreeMemoryHandle, flags int32) (r int32)
	BtreeClearTable(ctx BtreeContext, p BtreeHandle, iTable int32, pnChange BtreeMemoryHandle) (r int32)
	BtreeClearTableOfCursor(ctx BtreeContext, pCur BtreeCursorHandle) (r int32)
	BtreeDropTable(ctx BtreeContext, p BtreeHandle, iTable int32, piMoved BtreeMemoryHandle) (r int32)
	BtreeGetMeta(ctx BtreeContext, p BtreeHandle, idx int32, pMeta BtreeMemoryHandle)
	BtreeUpdateMeta(ctx BtreeContext, p BtreeHandle, idx int32, iMeta uint32) (r int32)
	BtreeCount(ctx BtreeContext, db SQLiteHandle, pCur BtreeCursorHandle, pnEntry BtreeMemoryHandle) (r int32)
	BtreePager(ctx BtreeContext, p BtreeHandle) (r BtreePagerHandle)
	BtreeGetFilename(ctx BtreeContext, p BtreeHandle) (r BtreeCStringHandle)
	BtreeGetJournalname(ctx BtreeContext, p BtreeHandle) (r BtreeCStringHandle)
	BtreeTxnState(ctx BtreeContext, p BtreeHandle) (r int32)
	BtreeCheckpoint(ctx BtreeContext, p BtreeHandle, eMode int32, pnLog BtreeMemoryHandle, pnCkpt BtreeMemoryHandle) (r int32)
	BtreeIsInBackup(ctx BtreeContext, p BtreeHandle) (r int32)
	BtreeSchema(ctx BtreeContext, p BtreeHandle, nBytes int32, __ccgo_fp_xFree BtreeFunctionHandle) (r BtreeSchemaHandle)
	BtreeSchemaLocked(ctx BtreeContext, p BtreeHandle) (r int32)
	BtreeLockTable(ctx BtreeContext, p BtreeHandle, iTab int32, isWriteLock uint8) (r int32)
	BtreePutData(ctx BtreeContext, pCsr BtreeCursorHandle, offset uint32, amt uint32, z BtreeMemoryHandle) (r int32)
	BtreeIncrblobCursor(ctx BtreeContext, pCur BtreeCursorHandle)
	BtreeSetVersion(ctx BtreeContext, pBtree BtreeHandle, iVersion int32) (r int32)
	BtreeCursorHasHint(ctx BtreeContext, pCsr BtreeCursorHandle, mask uint32) (r int32)
	BtreeIsReadonly(ctx BtreeContext, p BtreeHandle) (r int32)
	BtreeSharable(ctx BtreeContext, p BtreeHandle) (r int32)
	BtreeConnectionCount(ctx BtreeContext, p BtreeHandle) (r int32)
	BtreeCopyFile(ctx BtreeContext, pTo BtreeHandle, pFrom BtreeHandle) (r int32)
}

// StorageEngineBtreeSetMmapLimit is implemented by engines on platforms where BtreeSetMmapLimit exists.
type StorageEngineBtreeSetMmapLimit interface {
	BtreeSetMmapLimit(ctx BtreeContext, p BtreeHandle, szMmap int64) (r int32)
}

// StorageEngineBtreeIsEmpty is implemented by engines on platforms where BtreeIsEmpty exists.
type StorageEngineBtreeIsEmpty interface {
	BtreeIsEmpty(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32)
}

// StorageEngineBtreeIntegrityCheck is implemented by engines on platforms where BtreeIntegrityCheck exists.
type StorageEngineBtreeIntegrityCheck interface {
	BtreeIntegrityCheck(ctx BtreeContext, db SQLiteHandle, p BtreeHandle, aRoot BtreeMemoryHandle, aCnt BtreeMemoryHandle, nRoot int32, mxErr int32, pnErr BtreeMemoryHandle, pzOut BtreeMemoryHandle) (r int32)
}

// StorageEngineBtreeIntegrityCheckFreebsd386 is implemented by engines on platforms where BtreeIntegrityCheckFreebsd386 exists.
type StorageEngineBtreeIntegrityCheckFreebsd386 interface {
	BtreeIntegrityCheckFreebsd386(ctx BtreeContext, db SQLiteHandle, p BtreeHandle, aRoot BtreeMemoryHandle, nRoot int32, mxErr int32, pnErr BtreeMemoryHandle, pzOut BtreeMemoryHandle) (r int32)
}

// StorageEngineBtreeIntegrityCheckNetbsdAmd64 is implemented by engines on platforms where BtreeIntegrityCheckNetbsdAmd64 exists.
type StorageEngineBtreeIntegrityCheckNetbsdAmd64 interface {
	BtreeIntegrityCheckNetbsdAmd64(ctx BtreeContext, db SQLiteHandle, p BtreeHandle, aRoot BtreeMemoryHandle, nRoot int32, mxErr int32, pnErr BtreeMemoryHandle) (r BtreeCStringHandle)
}

// StorageEngineBtreeClearCache is implemented by engines on platforms where BtreeClearCache exists.
type StorageEngineBtreeClearCache interface {
	BtreeClearCache(ctx BtreeContext, p BtreeHandle)
}

// StorageEngineFileControlPersistWAL is implemented by engines that model
// SQLITE_FCNTL_PERSIST_WAL outside SQLite's native VFS file object.
type StorageEngineFileControlPersistWAL interface {
	FileControlPersistWAL(ctx BtreeContext, db SQLiteHandle, dbName string, mode int32) (int32, int32)
}

// StorageEngineLogicalBackup is implemented by engines that need to model
// sqlite3_backup source state outside SQLite's native TBtree.FnBackup field.
type StorageEngineLogicalBackup interface {
	BeginLogicalBackup(ctx BtreeContext, db SQLiteHandle) int32
	FinishLogicalBackup(ctx BtreeContext, db SQLiteHandle) int32
}

// StorageEngineLogicalMetadata is the logical btree state needed to make
// future root-page allocation match after a logical serialize or backup.
type StorageEngineLogicalMetadata struct {
	NextRoot  uint32
	FreeRoots []uint32
}

// StorageEngineLogicalMetadataProvider is implemented by engines whose
// logical serialize/backup path needs to preserve hidden allocation state.
type StorageEngineLogicalMetadataProvider interface {
	SaveLogicalMetadata(ctx BtreeContext, db SQLiteHandle) (StorageEngineLogicalMetadata, int32)
	RestoreLogicalMetadata(ctx BtreeContext, db SQLiteHandle, meta StorageEngineLogicalMetadata) int32
}

// StorageEngineReadOnlyMarker is implemented by engines that can mark a
// database handle readonly after loading external immutable content.
type StorageEngineReadOnlyMarker interface {
	MarkReadOnly(ctx BtreeContext, db SQLiteHandle) int32
}

type nativeBtreeStorageEngine struct{}

type storageEngineHolder struct {
	engine StorageEngine
}

var currentStorageEngine atomic.Value
var storageEngineSwitchMu sync.Mutex
var storageEngineDispatch storageEngineDispatcher
var storageEngineBinding = storageEngineBindings{
	btrees:  map[uintptr]storageEngineBtreeBinding{},
	cursors: map[uintptr]uintptr{},
	dbs:     map[uintptr]storageEngineDBBinding{},
}

type storageEngineBindings struct {
	mu      sync.RWMutex
	btrees  map[uintptr]storageEngineBtreeBinding
	cursors map[uintptr]uintptr
	dbs     map[uintptr]storageEngineDBBinding
}

type storageEngineBtreeBinding struct {
	engine StorageEngine
	db     uintptr
}

type storageEngineDBBinding struct {
	engine StorageEngine
	refs   int
}

type storageEngineDispatcher struct{}

func init() {
	currentStorageEngine.Store(storageEngineHolder{engine: nativeBtreeStorageEngine{}})
}

func selectedStorageEngine() StorageEngine {
	return currentStorageEngine.Load().(storageEngineHolder).engine
}

func storageEngine() StorageEngine { return storageEngineDispatch }

func storageEngineForDB(db SQLiteHandle) StorageEngine {
	if db.ptr == 0 {
		return selectedStorageEngine()
	}
	storageEngineBinding.mu.RLock()
	binding := storageEngineBinding.dbs[db.ptr]
	storageEngineBinding.mu.RUnlock()
	if binding.engine != nil {
		return binding.engine
	}
	return selectedStorageEngine()
}

func storageEngineForBtreeHandle(p BtreeHandle) StorageEngine {
	if p.ptr == 0 {
		return selectedStorageEngine()
	}
	storageEngineBinding.mu.RLock()
	binding := storageEngineBinding.btrees[p.ptr]
	storageEngineBinding.mu.RUnlock()
	if binding.engine != nil {
		return binding.engine
	}
	return selectedStorageEngine()
}

func storageEngineForBtreeToken(ptr uintptr) StorageEngine {
	return storageEngineForBtreeHandle(BtreeHandle{ptr: ptr})
}

func storageEngineForCursorHandle(pCur BtreeCursorHandle) StorageEngine {
	if pCur.ptr == 0 {
		return selectedStorageEngine()
	}
	storageEngineBinding.mu.RLock()
	pBtree := storageEngineBinding.cursors[pCur.ptr]
	storageEngineBinding.mu.RUnlock()
	if pBtree != 0 {
		return storageEngineForBtreeHandle(btreeHandle(pCur.tls, pBtree))
	}
	rawBtree := storageEngineBtCursorFromPointer(pCur.ptr).FpBtree
	if rawBtree != 0 {
		return storageEngineForBtreeHandle(btreeHandle(pCur.tls, rawBtree))
	}
	return selectedStorageEngine()
}

func storageEngineForCursorOrDB(pCur BtreeCursorHandle, db SQLiteHandle) StorageEngine {
	if pCur.ptr != 0 {
		storageEngineBinding.mu.RLock()
		pBtree := storageEngineBinding.cursors[pCur.ptr]
		storageEngineBinding.mu.RUnlock()
		if pBtree != 0 {
			return storageEngineForBtreeHandle(btreeHandle(pCur.tls, pBtree))
		}
		rawBtree := storageEngineBtCursorFromPointer(pCur.ptr).FpBtree
		if rawBtree != 0 {
			return storageEngineForBtreeHandle(btreeHandle(pCur.tls, rawBtree))
		}
	}
	return storageEngineForDB(db)
}

func registerStorageEngineBtree(p BtreeHandle, db SQLiteHandle, engine StorageEngine) {
	if p.ptr == 0 {
		return
	}
	storageEngineBinding.mu.Lock()
	storageEngineBinding.btrees[p.ptr] = storageEngineBtreeBinding{
		engine: engine,
		db:     db.ptr,
	}
	if db.ptr != 0 {
		binding := storageEngineBinding.dbs[db.ptr]
		if binding.engine == nil {
			binding.engine = engine
		}
		binding.refs++
		storageEngineBinding.dbs[db.ptr] = binding
	}
	storageEngineBinding.mu.Unlock()
}

func unregisterStorageEngineBtree(p BtreeHandle) {
	if p.ptr == 0 {
		return
	}
	storageEngineBinding.mu.Lock()
	binding := storageEngineBinding.btrees[p.ptr]
	delete(storageEngineBinding.btrees, p.ptr)
	if binding.db != 0 {
		dbBinding := storageEngineBinding.dbs[binding.db]
		dbBinding.refs--
		if dbBinding.refs <= 0 {
			delete(storageEngineBinding.dbs, binding.db)
		} else {
			storageEngineBinding.dbs[binding.db] = dbBinding
		}
	}
	storageEngineBinding.mu.Unlock()
}

func unregisterStorageEngineDB(db SQLiteHandle) {
	if db.ptr == 0 {
		return
	}
	storageEngineBinding.mu.Lock()
	delete(storageEngineBinding.dbs, db.ptr)
	for btree, binding := range storageEngineBinding.btrees {
		if binding.db == db.ptr {
			binding.db = 0
			storageEngineBinding.btrees[btree] = binding
		}
	}
	storageEngineBinding.mu.Unlock()
}

func registerStorageEngineCursor(pCur BtreeCursorHandle, p BtreeHandle) {
	if pCur.ptr == 0 || p.ptr == 0 {
		return
	}
	storageEngineBinding.mu.Lock()
	storageEngineBinding.cursors[pCur.ptr] = p.ptr
	storageEngineBinding.mu.Unlock()
}

func unregisterStorageEngineCursor(pCur BtreeCursorHandle) {
	if pCur.ptr == 0 {
		return
	}
	storageEngineBinding.mu.Lock()
	delete(storageEngineBinding.cursors, pCur.ptr)
	storageEngineBinding.mu.Unlock()
}

// StorageEngineIsNative reports whether calls are currently dispatched to the
// generated SQLite btree implementation.
func StorageEngineIsNative() bool {
	_, ok := selectedStorageEngine().(nativeBtreeStorageEngine)
	return ok
}

// StorageEngineIsNativeForDB reports whether db is bound to the generated
// SQLite btree implementation. If db has not opened a btree yet, it reports the
// engine that would be used for a new btree open.
func StorageEngineIsNativeForDB(tls *libc.TLS, db uintptr) bool {
	_, ok := storageEngineForDB(sqliteHandle(tls, db)).(nativeBtreeStorageEngine)
	return ok
}

// WithNativeStorageEngine runs fn while btree calls dispatch to the generated
// SQLite btree implementation, then restores the previous engine.
func WithNativeStorageEngine(fn func() error) error {
	storageEngineSwitchMu.Lock()
	defer storageEngineSwitchMu.Unlock()
	prev := currentStorageEngine.Load().(storageEngineHolder)
	currentStorageEngine.Store(storageEngineHolder{engine: nativeBtreeStorageEngine{}})
	defer currentStorageEngine.Store(prev)
	return fn()
}

func storageEngineLogicalDBPageResult(tls *libc.TLS, ctx uintptr, pageSize int32) int32 {
	if pageSize <= 0 {
		pageSize = 4096
	}
	p := tls.Alloc(int(pageSize))
	if p == 0 {
		return SQLITE_NOMEM
	}
	defer tls.Free(int(pageSize))
	page := storageEngineByteSliceFromPointer(p, int(pageSize))
	clear(page)
	copy(page, "SQLite format 3\x00")
	Xsqlite3_result_blob(tls, ctx, p, pageSize, SQLITE_TRANSIENT)
	return SQLITE_OK
}

// SetStorageEngine sets the btree storage engine. Passing nil restores the generated SQLite btree implementation.
func SetStorageEngine(engine StorageEngine) {
	if engine == nil {
		engine = nativeBtreeStorageEngine{}
	}
	currentStorageEngine.Store(storageEngineHolder{engine: engine})
}

func StorageEngineConnectionClosed(tls *libc.TLS, db uintptr) {
	unregisterStorageEngineDB(sqliteHandle(tls, db))
}

func StorageEngineFileControlPersistWALMode(tls *libc.TLS, db uintptr, dbName string, mode int32) (int32, int32) {
	engine, ok := storageEngineForDB(sqliteHandle(tls, db)).(StorageEngineFileControlPersistWAL)
	if !ok {
		return mode, SQLITE_ERROR
	}
	return engine.FileControlPersistWAL(btreeContext(tls), sqliteHandle(tls, db), dbName, mode)
}

func StorageEngineBeginLogicalBackup(tls *libc.TLS, db uintptr) int32 {
	engine, ok := storageEngineForDB(sqliteHandle(tls, db)).(StorageEngineLogicalBackup)
	if !ok {
		return SQLITE_ERROR
	}
	return engine.BeginLogicalBackup(btreeContext(tls), sqliteHandle(tls, db))
}

func StorageEngineFinishLogicalBackup(tls *libc.TLS, db uintptr) int32 {
	engine, ok := storageEngineForDB(sqliteHandle(tls, db)).(StorageEngineLogicalBackup)
	if !ok {
		return SQLITE_ERROR
	}
	return engine.FinishLogicalBackup(btreeContext(tls), sqliteHandle(tls, db))
}

func StorageEngineSaveLogicalMetadata(tls *libc.TLS, db uintptr) (StorageEngineLogicalMetadata, bool, int32) {
	engine, ok := storageEngineForDB(sqliteHandle(tls, db)).(StorageEngineLogicalMetadataProvider)
	if !ok {
		return StorageEngineLogicalMetadata{}, false, SQLITE_OK
	}
	meta, rc := engine.SaveLogicalMetadata(btreeContext(tls), sqliteHandle(tls, db))
	return meta, true, rc
}

func StorageEngineRestoreLogicalMetadata(tls *libc.TLS, db uintptr, meta StorageEngineLogicalMetadata) (bool, int32) {
	engine, ok := storageEngineForDB(sqliteHandle(tls, db)).(StorageEngineLogicalMetadataProvider)
	if !ok {
		return false, SQLITE_OK
	}
	return true, engine.RestoreLogicalMetadata(btreeContext(tls), sqliteHandle(tls, db), meta)
}

func StorageEngineMarkReadOnly(tls *libc.TLS, db uintptr) (bool, int32) {
	engine, ok := storageEngineForDB(sqliteHandle(tls, db)).(StorageEngineReadOnlyMarker)
	if !ok {
		return false, SQLITE_OK
	}
	return true, engine.MarkReadOnly(btreeContext(tls), sqliteHandle(tls, db))
}

// BtreeContext is the per-call SQLite runtime context seen by storage engines.
type BtreeContext struct {
	tls *libc.TLS
}

// BtreeHandle identifies a SQLite btree object for the active storage engine.
type BtreeHandle struct {
	tls *libc.TLS
	ptr uintptr
}

// SQLiteHandle identifies the owning sqlite3 connection.
type SQLiteHandle struct {
	tls *libc.TLS
	ptr uintptr
}

// BtreeVFSHandle identifies the SQLite VFS passed to btree open.
type BtreeVFSHandle struct {
	tls *libc.TLS
	ptr uintptr
}

// BtreeCursorHandle identifies a btree cursor for the active storage engine.
type BtreeCursorHandle struct {
	tls *libc.TLS
	ptr uintptr
}

// BtreeMemoryHandle identifies SQLite-owned memory used for out parameters or buffers.
type BtreeMemoryHandle struct {
	tls *libc.TLS
	ptr uintptr
}

// BtreeCStringHandle identifies a SQLite-owned C string.
type BtreeCStringHandle struct {
	tls *libc.TLS
	ptr uintptr
}

// BtreePayloadHandle identifies SQLite's internal insert payload descriptor.
type BtreePayloadHandle struct {
	tls *libc.TLS
	ptr uintptr
}

// BtreePagerHandle identifies SQLite's pager object for a btree.
type BtreePagerHandle struct {
	tls *libc.TLS
	ptr uintptr
}

// BtreeSchemaHandle identifies SQLite schema memory associated with a btree.
type BtreeSchemaHandle struct {
	tls *libc.TLS
	ptr uintptr
}

// BtreeKeyInfoHandle identifies SQLite key metadata used to open an index cursor.
type BtreeKeyInfoHandle struct {
	tls *libc.TLS
	ptr uintptr
}

// BtreeIndexKeyHandle identifies SQLite's unpacked index key for cursor movement.
type BtreeIndexKeyHandle struct {
	tls *libc.TLS
	ptr uintptr
}

// BtreeFunctionHandle identifies a SQLite function pointer passed through btree APIs.
type BtreeFunctionHandle struct {
	tls *libc.TLS
	ptr uintptr
}

// BtreeToken is an opaque btree identity token.
type BtreeToken uintptr

// SQLiteToken is an opaque sqlite3 connection identity token.
type SQLiteToken uintptr

// BtreeVFSToken is an opaque VFS identity token.
type BtreeVFSToken uintptr

// BtreeCursorToken is an opaque cursor identity token.
type BtreeCursorToken uintptr

// BtreeMemoryToken is an opaque memory identity token.
type BtreeMemoryToken uintptr

// BtreeCStringToken is an opaque C string identity token.
type BtreeCStringToken uintptr

// BtreePayloadToken is an opaque payload identity token.
type BtreePayloadToken uintptr

// BtreePagerToken is an opaque pager identity token.
type BtreePagerToken uintptr

// BtreeSchemaToken is an opaque schema identity token.
type BtreeSchemaToken uintptr

// BtreeKeyInfoToken is an opaque key-info identity token.
type BtreeKeyInfoToken uintptr

// BtreeIndexKeyToken is an opaque index-key identity token.
type BtreeIndexKeyToken uintptr

// BtreeFunctionToken is an opaque function identity token.
type BtreeFunctionToken uintptr

func btreeContext(tls *libc.TLS) BtreeContext {
	return BtreeContext{tls: tls}
}

func btreeHandle(tls *libc.TLS, ptr uintptr) BtreeHandle {
	return BtreeHandle{tls: tls, ptr: ptr}
}

func sqliteHandle(tls *libc.TLS, ptr uintptr) SQLiteHandle {
	return SQLiteHandle{tls: tls, ptr: ptr}
}

func btreeVFSHandle(tls *libc.TLS, ptr uintptr) BtreeVFSHandle {
	return BtreeVFSHandle{tls: tls, ptr: ptr}
}

func btreeCursorHandle(tls *libc.TLS, ptr uintptr) BtreeCursorHandle {
	return BtreeCursorHandle{tls: tls, ptr: ptr}
}

func btreeMemoryHandle(tls *libc.TLS, ptr uintptr) BtreeMemoryHandle {
	return BtreeMemoryHandle{tls: tls, ptr: ptr}
}

func btreeCStringHandle(tls *libc.TLS, ptr uintptr) BtreeCStringHandle {
	return BtreeCStringHandle{tls: tls, ptr: ptr}
}

func btreePayloadHandle(tls *libc.TLS, ptr uintptr) BtreePayloadHandle {
	return BtreePayloadHandle{tls: tls, ptr: ptr}
}

func btreePagerHandle(tls *libc.TLS, ptr uintptr) BtreePagerHandle {
	return BtreePagerHandle{tls: tls, ptr: ptr}
}

func btreeSchemaHandle(tls *libc.TLS, ptr uintptr) BtreeSchemaHandle {
	return BtreeSchemaHandle{tls: tls, ptr: ptr}
}

func btreeKeyInfoHandle(tls *libc.TLS, ptr uintptr) BtreeKeyInfoHandle {
	return BtreeKeyInfoHandle{tls: tls, ptr: ptr}
}

func btreeIndexKeyHandle(tls *libc.TLS, ptr uintptr) BtreeIndexKeyHandle {
	return BtreeIndexKeyHandle{tls: tls, ptr: ptr}
}

func btreeFunctionHandle(tls *libc.TLS, ptr uintptr) BtreeFunctionHandle {
	return BtreeFunctionHandle{tls: tls, ptr: ptr}
}

// IsNil reports whether h is the zero C pointer.
func (h BtreeHandle) IsNil() bool { return h.ptr == 0 }

// IsNil reports whether h is the zero C pointer.
func (h SQLiteHandle) IsNil() bool { return h.ptr == 0 }

// IsNil reports whether h is the zero C pointer.
func (h BtreeVFSHandle) IsNil() bool { return h.ptr == 0 }

// IsNil reports whether h is the zero C pointer.
func (h BtreeCursorHandle) IsNil() bool { return h.ptr == 0 }

// IsNil reports whether h is the zero C pointer.
func (h BtreeMemoryHandle) IsNil() bool { return h.ptr == 0 }

// IsNil reports whether h is the zero C pointer.
func (h BtreeCStringHandle) IsNil() bool { return h.ptr == 0 }

// IsNil reports whether h is the zero C pointer.
func (h BtreePayloadHandle) IsNil() bool { return h.ptr == 0 }

// IsNil reports whether h is the zero C pointer.
func (h BtreePagerHandle) IsNil() bool { return h.ptr == 0 }

// IsNil reports whether h is the zero C pointer.
func (h BtreeSchemaHandle) IsNil() bool { return h.ptr == 0 }

// IsNil reports whether h is the zero C pointer.
func (h BtreeKeyInfoHandle) IsNil() bool { return h.ptr == 0 }

// IsNil reports whether h is the zero C pointer.
func (h BtreeIndexKeyHandle) IsNil() bool { return h.ptr == 0 }

// IsNil reports whether h is the zero C pointer.
func (h BtreeFunctionHandle) IsNil() bool { return h.ptr == 0 }

// Token returns an opaque identity token for h.
func (h BtreeHandle) Token() BtreeToken { return BtreeToken(h.ptr) }

// Token returns an opaque identity token for h.
func (h SQLiteHandle) Token() SQLiteToken { return SQLiteToken(h.ptr) }

// Token returns an opaque identity token for h.
func (h BtreeVFSHandle) Token() BtreeVFSToken { return BtreeVFSToken(h.ptr) }

// Token returns an opaque identity token for h.
func (h BtreeCursorHandle) Token() BtreeCursorToken { return BtreeCursorToken(h.ptr) }

// Token returns an opaque identity token for h.
func (h BtreeMemoryHandle) Token() BtreeMemoryToken { return BtreeMemoryToken(h.ptr) }

// Token returns an opaque identity token for h.
func (h BtreeCStringHandle) Token() BtreeCStringToken { return BtreeCStringToken(h.ptr) }

// Token returns an opaque identity token for h.
func (h BtreePayloadHandle) Token() BtreePayloadToken { return BtreePayloadToken(h.ptr) }

// Token returns an opaque identity token for h.
func (h BtreePagerHandle) Token() BtreePagerToken { return BtreePagerToken(h.ptr) }

// Token returns an opaque identity token for h.
func (h BtreeSchemaHandle) Token() BtreeSchemaToken { return BtreeSchemaToken(h.ptr) }

// Token returns an opaque identity token for h.
func (h BtreeKeyInfoHandle) Token() BtreeKeyInfoToken { return BtreeKeyInfoToken(h.ptr) }

// Token returns an opaque identity token for h.
func (h BtreeIndexKeyHandle) Token() BtreeIndexKeyToken { return BtreeIndexKeyToken(h.ptr) }

// Token returns an opaque identity token for h.
func (h BtreeFunctionHandle) Token() BtreeFunctionToken { return BtreeFunctionToken(h.ptr) }

// String copies the pointed-to C string into Go memory.
func (h BtreeCStringHandle) String() string {
	if h.ptr == 0 {
		return ""
	}
	return libc.GoString(h.ptr)
}

func (h BtreePayloadHandle) payload() *BtreePayload {
	return storageEnginePayloadFromPointer(h.ptr)
}

// KeyHandle identifies the key bytes inside h.
func (h BtreePayloadHandle) KeyHandle() BtreeMemoryHandle {
	return btreeMemoryHandle(h.tls, h.payload().FpKey)
}

// KeySize returns the key byte count or integer rowid carried by h.
func (h BtreePayloadHandle) KeySize() int64 {
	return int64(h.payload().FnKey)
}

// KeyBytes copies the key bytes carried by h into Go memory.
func (h BtreePayloadHandle) KeyBytes() []byte {
	n := h.KeySize()
	if n <= 0 {
		return nil
	}
	return h.KeyHandle().ReadBytes(int(n))
}

// DataHandle identifies the data bytes inside h.
func (h BtreePayloadHandle) DataHandle() BtreeMemoryHandle {
	return btreeMemoryHandle(h.tls, h.payload().FpData)
}

// DataSize returns the data byte count carried by h.
func (h BtreePayloadHandle) DataSize() int32 {
	return h.payload().FnData
}

// DataBytes copies the data bytes carried by h into Go memory.
func (h BtreePayloadHandle) DataBytes() []byte {
	n := h.DataSize()
	if n <= 0 {
		return nil
	}
	return h.DataHandle().ReadBytes(int(n))
}

// ZeroSize returns the number of zero bytes appended to h's data.
func (h BtreePayloadHandle) ZeroSize() int32 {
	return h.payload().FnZero
}

// MemoryHandle identifies the SQLite Mem array carried by h.
func (h BtreePayloadHandle) MemoryHandle() BtreeMemoryHandle {
	return btreeMemoryHandle(h.tls, h.payload().FaMem)
}

// MemoryCount returns the number of SQLite Mem values carried by h.
func (h BtreePayloadHandle) MemoryCount() uint16 {
	return uint16(h.payload().FnMem)
}

// WriteBytes copies b into the SQLite-owned memory pointed to by h.
func (h BtreeMemoryHandle) WriteBytes(b []byte) {
	if len(b) == 0 {
		return
	}
	copy(storageEngineByteSliceFromPointer(h.ptr, len(b)), b)
}

// ReadBytes copies n bytes from the SQLite-owned memory pointed to by h.
func (h BtreeMemoryHandle) ReadBytes(n int) []byte {
	if n == 0 {
		return nil
	}
	b := make([]byte, n)
	copy(b, storageEngineByteSliceFromPointer(h.ptr, n))
	return b
}

// GetInt32 reads an int32 from the SQLite-owned memory pointed to by h.
func (h BtreeMemoryHandle) GetInt32() int32 {
	return *storageEngineInt32FromPointer(h.ptr)
}

// GetUint32 reads a uint32 from the SQLite-owned memory pointed to by h.
func (h BtreeMemoryHandle) GetUint32() uint32 {
	return *storageEngineUint32FromPointer(h.ptr)
}

// GetInt64 reads an int64 from the SQLite-owned memory pointed to by h.
func (h BtreeMemoryHandle) GetInt64() int64 {
	return *storageEngineInt64FromPointer(h.ptr)
}

// GetUintptr reads a uintptr from the SQLite-owned memory pointed to by h.
func (h BtreeMemoryHandle) GetUintptr() uintptr {
	return *storageEngineUintptrFromPointer(h.ptr)
}

// PutInt32 writes v to the SQLite-owned memory pointed to by h.
func (h BtreeMemoryHandle) PutInt32(v int32) {
	*storageEngineInt32FromPointer(h.ptr) = v
}

// PutUint32 writes v to the SQLite-owned memory pointed to by h.
func (h BtreeMemoryHandle) PutUint32(v uint32) {
	*storageEngineUint32FromPointer(h.ptr) = v
}

// PutInt64 writes v to the SQLite-owned memory pointed to by h.
func (h BtreeMemoryHandle) PutInt64(v int64) {
	*storageEngineInt64FromPointer(h.ptr) = v
}

// PutUintptr writes v to the SQLite-owned memory pointed to by h.
func (h BtreeMemoryHandle) PutUintptr(v uintptr) {
	*storageEngineUintptrFromPointer(h.ptr) = v
}

// PutBtreeToken writes v to the SQLite-owned memory pointed to by h.
func (h BtreeMemoryHandle) PutBtreeToken(v BtreeToken) {
	h.PutUintptr(uintptr(v))
}
