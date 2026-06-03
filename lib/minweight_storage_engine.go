// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"encoding/binary"
	"sort"
	"sync"
	"unsafe"

	minweight "github.com/JimChengLin/minweight_store"
	"modernc.org/libc"
)

const (
	minweightTablePrefix byte = 't'
	minweightIndexPrefix byte = 'i'
)

// NewMinweightStorageEngine returns a StorageEngine backed by minweight_store.
func NewMinweightStorageEngine() StorageEngine {
	return &minweightStorageEngine{
		btrees:  map[uintptr]*minweightBtree{},
		cursors: map[uintptr]*minweightCursor{},
		next:    1,
	}
}

type minweightStorageEngine struct {
	mu      sync.Mutex
	next    uintptr
	btrees  map[uintptr]*minweightBtree
	cursors map[uintptr]*minweightCursor
}

type minweightBtree struct {
	store   *minweight.Store
	meta    [SQLITE_N_BTREE_META]uint32
	tables  map[uint32]minweightTable
	next    uint32
	pager   uintptr
	schema  uintptr
	dataVer uint32
}

type minweightTable struct {
	intKey bool
}

type minweightCursor struct {
	btree      *minweightBtree
	root       uint32
	intKey     bool
	rows       []minweightRow
	index      int
	valid      bool
	payloadBuf uintptr
}

type minweightRow struct {
	rowid   int64
	key     []byte
	payload []byte
}

func (e *minweightStorageEngine) nextToken() uintptr {
	e.next++
	return e.next
}

func (e *minweightStorageEngine) btree(p BtreeHandle) *minweightBtree {
	e.mu.Lock()
	defer e.mu.Unlock()
	bt := e.btrees[p.ptr]
	if bt == nil {
		panic("sqlite minweight storage engine: unknown btree handle")
	}
	return bt
}

func (e *minweightStorageEngine) cursor(pCur BtreeCursorHandle) *minweightCursor {
	e.mu.Lock()
	defer e.mu.Unlock()
	cur := e.cursors[pCur.ptr]
	if cur == nil {
		panic("sqlite minweight storage engine: unknown cursor handle")
	}
	return cur
}

func minweightSQLiteError(err error) int32 {
	if err == nil {
		return SQLITE_OK
	}
	return SQLITE_ERROR
}

func minweightTableKey(root uint32, rowid int64) []byte {
	key := make([]byte, 13)
	key[0] = minweightTablePrefix
	binary.BigEndian.PutUint32(key[1:5], root)
	binary.BigEndian.PutUint64(key[5:13], uint64(rowid)^(1<<63))
	return key
}

func minweightIndexKey(root uint32, keyBytes []byte) []byte {
	key := make([]byte, 5+len(keyBytes))
	key[0] = minweightIndexPrefix
	binary.BigEndian.PutUint32(key[1:5], root)
	copy(key[5:], keyBytes)
	return key
}

func minweightRootPrefix(root uint32, intKey bool) []byte {
	prefix := make([]byte, 5)
	if intKey {
		prefix[0] = minweightTablePrefix
	} else {
		prefix[0] = minweightIndexPrefix
	}
	binary.BigEndian.PutUint32(prefix[1:5], root)
	return prefix
}

func minweightPrefixUpper(prefix []byte) []byte {
	upper := append([]byte(nil), prefix...)
	for i := len(upper) - 1; i >= 0; i-- {
		if upper[i] != 0xff {
			upper[i]++
			return upper[:i+1]
		}
	}
	return nil
}

func minweightDecodeRow(item minweight.Item, intKey bool) minweightRow {
	row := minweightRow{payload: append([]byte(nil), item.Value...)}
	if intKey {
		u := binary.BigEndian.Uint64(item.Key[5:13]) ^ (1 << 63)
		row.rowid = int64(u)
		return row
	}
	row.key = append([]byte(nil), item.Key[5:]...)
	return row
}

func (bt *minweightBtree) loadRows(root uint32, intKey bool) ([]minweightRow, error) {
	prefix := minweightRootPrefix(root, intKey)
	upper := minweightPrefixUpper(prefix)
	var rows []minweightRow
	err := bt.store.ScanRange(prefix, upper, func(item minweight.Item) bool {
		rows = append(rows, minweightDecodeRow(item, intKey))
		return true
	})
	return rows, err
}

func (bt *minweightBtree) clearRoot(root uint32, intKey bool) (int, error) {
	rows, err := bt.loadRows(root, intKey)
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		var key []byte
		if intKey {
			key = minweightTableKey(root, row.rowid)
		} else {
			key = minweightIndexKey(root, row.key)
		}
		if _, err := bt.store.Delete(key); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

func minweightWriteResult(pRes BtreeMemoryHandle, v int32) {
	if !pRes.IsNil() {
		pRes.PutInt32(v)
	}
}

func (c *minweightCursor) closePayload(ctx BtreeContext) {
	if c.payloadBuf != 0 {
		Xsqlite3_free(ctx.tls, c.payloadBuf)
		c.payloadBuf = 0
	}
}

func (c *minweightCursor) current() (minweightRow, bool) {
	if !c.valid || c.index < 0 || c.index >= len(c.rows) {
		return minweightRow{}, false
	}
	return c.rows[c.index], true
}

func minweightCompareIndexKey(ctx BtreeContext, key []byte, pIdxKey uintptr) (int32, int32) {
	if len(key) == 0 {
		return 0, SQLITE_CORRUPT
	}
	buf := _sqlite3MallocZero(ctx.tls, uint64(len(key)+18))
	if buf == 0 {
		return 0, SQLITE_NOMEM
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(buf)), len(key)), key)
	cmp := _sqlite3VdbeRecordCompare(ctx.tls, int32(len(key)), buf, pIdxKey)
	Xsqlite3_free(ctx.tls, buf)
	if (*TUnpackedRecord)(unsafe.Pointer(pIdxKey)).FerrCode != uint8(SQLITE_OK) {
		return cmp, SQLITE_CORRUPT
	}
	return cmp, SQLITE_OK
}

func minweightCompareIndexRows(ctx BtreeContext, keyInfo uintptr, a []byte, b []byte) (int32, int32) {
	pIdxKey := _sqlite3VdbeAllocUnpackedRecord(ctx.tls, keyInfo)
	if pIdxKey == 0 {
		return 0, SQLITE_NOMEM
	}
	buf := _sqlite3MallocZero(ctx.tls, uint64(len(b)+18))
	if buf == 0 {
		_sqlite3DbFreeNN(ctx.tls, (*TKeyInfo)(unsafe.Pointer(keyInfo)).Fdb, pIdxKey)
		return 0, SQLITE_NOMEM
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(buf)), len(b)), b)
	_sqlite3VdbeRecordUnpack(ctx.tls, int32(len(b)), buf, pIdxKey)
	Xsqlite3_free(ctx.tls, buf)
	cmp, rc := minweightCompareIndexKey(ctx, a, pIdxKey)
	_sqlite3DbFreeNN(ctx.tls, (*TKeyInfo)(unsafe.Pointer(keyInfo)).Fdb, pIdxKey)
	return cmp, rc
}

func minweightSortIndexRows(ctx BtreeContext, keyInfo uintptr, rows []minweightRow) int32 {
	var rc int32
	sort.SliceStable(rows, func(i, j int) bool {
		if rc != 0 {
			return false
		}
		var cmp int32
		cmp, rc = minweightCompareIndexRows(ctx, keyInfo, rows[i].key, rows[j].key)
		return cmp < 0
	})
	return rc
}

func (e *minweightStorageEngine) BtreeEnter(ctx BtreeContext, p BtreeHandle)                {}
func (e *minweightStorageEngine) BtreeLeave(ctx BtreeContext, p BtreeHandle)                {}
func (e *minweightStorageEngine) BtreeEnterAll(ctx BtreeContext, db SQLiteHandle)           {}
func (e *minweightStorageEngine) BtreeLeaveAll(ctx BtreeContext, db SQLiteHandle)           {}
func (e *minweightStorageEngine) BtreeEnterCursor(ctx BtreeContext, pCur BtreeCursorHandle) {}
func (e *minweightStorageEngine) BtreeLeaveCursor(ctx BtreeContext, pCur BtreeCursorHandle) {}

func (e *minweightStorageEngine) BtreeClearCursor(ctx BtreeContext, pCur BtreeCursorHandle) {
	cur := e.cursor(pCur)
	cur.valid = false
}

func (e *minweightStorageEngine) BtreeCursorHasMoved(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	return 0
}

func (e *minweightStorageEngine) BtreeFakeValidCursor(ctx BtreeContext) (r BtreeCursorHandle) {
	return BtreeCursorHandle{}
}

func (e *minweightStorageEngine) BtreeCursorRestore(ctx BtreeContext, pCur BtreeCursorHandle, pDifferentRow BtreeMemoryHandle) (r int32) {
	minweightWriteResult(pDifferentRow, 0)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCursorHintFlags(ctx BtreeContext, pCur BtreeCursorHandle, x uint32) {
}

func (e *minweightStorageEngine) BtreeLastPage(ctx BtreeContext, p BtreeHandle) (r uint32) {
	return e.btree(p).next
}

func (e *minweightStorageEngine) BtreeOpen(ctx BtreeContext, pVfs BtreeVFSHandle, zFilename BtreeCStringHandle, db SQLiteHandle, ppBtree BtreeMemoryHandle, flags int32, vfsFlags int32) (r int32) {
	pager := _sqlite3MallocZero(ctx.tls, uint64(unsafe.Sizeof(Pager{})))
	(*Pager)(unsafe.Pointer(pager)).FpageSize = 4096
	bt := &minweightBtree{
		pager:  pager,
		store:  minweight.New(),
		tables: map[uint32]minweightTable{1: {intKey: true}},
		next:   1,
	}
	e.mu.Lock()
	token := e.nextToken()
	e.btrees[token] = bt
	e.mu.Unlock()
	ppBtree.PutBtreeToken(BtreeToken(token))
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeClose(ctx BtreeContext, p BtreeHandle) (r int32) {
	e.mu.Lock()
	bt := e.btrees[p.ptr]
	delete(e.btrees, p.ptr)
	e.mu.Unlock()
	if bt != nil && bt.schema != 0 {
		Xsqlite3_free(ctx.tls, bt.schema)
	}
	if bt != nil && bt.pager != 0 {
		Xsqlite3_free(ctx.tls, bt.pager)
	}
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeSetCacheSize(ctx BtreeContext, p BtreeHandle, mxPage int32) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeSetSpillSize(ctx BtreeContext, p BtreeHandle, mxPage int32) (r int32) {
	return mxPage
}
func (e *minweightStorageEngine) BtreeSetPagerFlags(ctx BtreeContext, p BtreeHandle, pgFlags uint32) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeSetPageSize(ctx BtreeContext, p BtreeHandle, pageSize int32, nReserve int32, iFix int32) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeGetPageSize(ctx BtreeContext, p BtreeHandle) (r int32) {
	return 4096
}
func (e *minweightStorageEngine) BtreeGetReserveNoMutex(ctx BtreeContext, p BtreeHandle) (r int32) {
	return 0
}
func (e *minweightStorageEngine) BtreeGetRequestedReserve(ctx BtreeContext, p BtreeHandle) (r int32) {
	return 0
}
func (e *minweightStorageEngine) BtreeMaxPageCount(ctx BtreeContext, p BtreeHandle, mxPage uint32) (r uint32) {
	return mxPage
}
func (e *minweightStorageEngine) BtreeSecureDelete(ctx BtreeContext, p BtreeHandle, newFlag int32) (r int32) {
	return 0
}
func (e *minweightStorageEngine) BtreeSetAutoVacuum(ctx BtreeContext, p BtreeHandle, autoVacuum int32) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeGetAutoVacuum(ctx BtreeContext, p BtreeHandle) (r int32) {
	return BTREE_AUTOVACUUM_NONE
}

func (e *minweightStorageEngine) BtreeNewDb(ctx BtreeContext, p BtreeHandle) (r int32) {
	bt := e.btree(p)
	bt.store = minweight.New()
	bt.tables = map[uint32]minweightTable{1: {intKey: true}}
	bt.next = 1
	bt.meta = [SQLITE_N_BTREE_META]uint32{}
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeBeginTrans(ctx BtreeContext, p BtreeHandle, wrflag int32, pSchemaVersion BtreeMemoryHandle) (r int32) {
	if !pSchemaVersion.IsNil() {
		pSchemaVersion.PutUint32(e.btree(p).meta[BTREE_SCHEMA_VERSION])
	}
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeIncrVacuum(ctx BtreeContext, p BtreeHandle) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeCommitPhaseOne(ctx BtreeContext, p BtreeHandle, zSuperJrnl BtreeCStringHandle) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeCommitPhaseTwo(ctx BtreeContext, p BtreeHandle, bCleanup int32) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeCommit(ctx BtreeContext, p BtreeHandle) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeTripAllCursors(ctx BtreeContext, pBtree BtreeHandle, errCode int32, writeOnly int32) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeRollback(ctx BtreeContext, p BtreeHandle, tripCode int32, writeOnly int32) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeBeginStmt(ctx BtreeContext, p BtreeHandle, iStatement int32) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeSavepoint(ctx BtreeContext, p BtreeHandle, op int32, iSavepoint int32) (r int32) {
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCursor(ctx BtreeContext, p BtreeHandle, iTable uint32, wrFlag int32, pKeyInfo BtreeKeyInfoHandle, pCur BtreeCursorHandle) (r int32) {
	bt := e.btree(p)
	table, ok := bt.tables[iTable]
	if !ok {
		table = minweightTable{intKey: pKeyInfo.IsNil()}
		bt.tables[iTable] = table
	}
	*(*BtCursor)(unsafe.Pointer(pCur.ptr)) = BtCursor{}
	(*BtCursor)(unsafe.Pointer(pCur.ptr)).FpBtree = p.ptr
	(*BtCursor)(unsafe.Pointer(pCur.ptr)).FpgnoRoot = TPgno(iTable)
	(*BtCursor)(unsafe.Pointer(pCur.ptr)).FcurIntKey = libc.Uint8FromInt32(libc.BoolInt32(table.intKey))
	(*BtCursor)(unsafe.Pointer(pCur.ptr)).FpKeyInfo = pKeyInfo.ptr
	(*BtCursor)(unsafe.Pointer(pCur.ptr)).FiPage = -1
	cur := &minweightCursor{
		btree:  bt,
		root:   iTable,
		intKey: table.intKey,
		index:  -1,
	}
	e.mu.Lock()
	e.cursors[pCur.ptr] = cur
	e.mu.Unlock()
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCursorSize(ctx BtreeContext) (r int32) {
	return int32(unsafe.Sizeof(BtCursor{}))
}

func (e *minweightStorageEngine) BtreeCursorZero(ctx BtreeContext, p BtreeCursorHandle) {
	*(*BtCursor)(unsafe.Pointer(p.ptr)) = BtCursor{}
}

func (e *minweightStorageEngine) BtreeCloseCursor(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	e.mu.Lock()
	cur := e.cursors[pCur.ptr]
	delete(e.cursors, pCur.ptr)
	e.mu.Unlock()
	if cur != nil {
		cur.closePayload(ctx)
	}
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCursorIsValidNN(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.valid {
		return 1
	}
	return 0
}

func (e *minweightStorageEngine) BtreeIntegerKey(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	row, ok := e.cursor(pCur).current()
	if !ok {
		return 0
	}
	return row.rowid
}

func (e *minweightStorageEngine) BtreeCursorPin(ctx BtreeContext, pCur BtreeCursorHandle)   {}
func (e *minweightStorageEngine) BtreeCursorUnpin(ctx BtreeContext, pCur BtreeCursorHandle) {}
func (e *minweightStorageEngine) BtreeOffset(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	return 0
}

func (e *minweightStorageEngine) BtreePayloadSize(ctx BtreeContext, pCur BtreeCursorHandle) (r uint32) {
	row, ok := e.cursor(pCur).current()
	if !ok {
		return 0
	}
	if e.cursor(pCur).intKey {
		return uint32(len(row.payload))
	}
	return uint32(len(row.key))
}

func (e *minweightStorageEngine) BtreeMaxRecordSize(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	return int64(e.BtreePayloadSize(ctx, pCur))
}

func (e *minweightStorageEngine) BtreePayload(ctx BtreeContext, pCur BtreeCursorHandle, offset uint32, amt uint32, pBuf BtreeMemoryHandle) (r int32) {
	row, ok := e.cursor(pCur).current()
	if !ok {
		return SQLITE_ERROR
	}
	data := row.payload
	if !e.cursor(pCur).intKey {
		data = row.key
	}
	end := int(offset + amt)
	if int(offset) > len(data) || end > len(data) {
		return SQLITE_CORRUPT
	}
	pBuf.WriteBytes(data[offset:end])
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreePayloadChecked(ctx BtreeContext, pCur BtreeCursorHandle, offset uint32, amt uint32, pBuf BtreeMemoryHandle) (r int32) {
	return e.BtreePayload(ctx, pCur, offset, amt, pBuf)
}

func (e *minweightStorageEngine) BtreePayloadFetch(ctx BtreeContext, pCur BtreeCursorHandle, pAmt BtreeMemoryHandle) (r BtreeMemoryHandle) {
	cur := e.cursor(pCur)
	row, ok := cur.current()
	if !ok {
		pAmt.PutInt32(0)
		return BtreeMemoryHandle{}
	}
	data := row.payload
	if !cur.intKey {
		data = row.key
	}
	cur.closePayload(ctx)
	cur.payloadBuf = _sqlite3Malloc(ctx.tls, uint64(len(data)))
	if len(data) != 0 {
		copy(unsafe.Slice((*byte)(unsafe.Pointer(cur.payloadBuf)), len(data)), data)
	}
	pAmt.PutInt32(int32(len(data)))
	return btreeMemoryHandle(ctx.tls, cur.payloadBuf)
}

func (e *minweightStorageEngine) BtreeFirst(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	rows, err := cur.btree.loadRows(cur.root, cur.intKey)
	if err != nil {
		return minweightSQLiteError(err)
	}
	cur.rows = rows
	if len(rows) == 0 {
		cur.valid = false
		cur.index = -1
		minweightWriteResult(pRes, 1)
		return SQLITE_OK
	}
	cur.valid = true
	cur.index = 0
	minweightWriteResult(pRes, 0)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeLast(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	rows, err := cur.btree.loadRows(cur.root, cur.intKey)
	if err != nil {
		return minweightSQLiteError(err)
	}
	cur.rows = rows
	if len(rows) == 0 {
		cur.valid = false
		cur.index = -1
		minweightWriteResult(pRes, 1)
		return SQLITE_OK
	}
	cur.valid = true
	cur.index = len(rows) - 1
	minweightWriteResult(pRes, 0)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeTableMoveto(ctx BtreeContext, pCur BtreeCursorHandle, intKey int64, biasRight int32, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	rows, err := cur.btree.loadRows(cur.root, true)
	if err != nil {
		return minweightSQLiteError(err)
	}
	cur.rows = rows
	i := sort.Search(len(rows), func(i int) bool { return rows[i].rowid >= intKey })
	if i < len(rows) {
		cur.valid = true
		cur.index = i
		if rows[i].rowid == intKey {
			minweightWriteResult(pRes, 0)
		} else {
			minweightWriteResult(pRes, 1)
		}
		return SQLITE_OK
	}
	cur.valid = false
	cur.index = len(rows)
	minweightWriteResult(pRes, -1)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeIndexMoveto(ctx BtreeContext, pCur BtreeCursorHandle, pIdxKey BtreeIndexKeyHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	rows, err := cur.btree.loadRows(cur.root, false)
	if err != nil {
		return minweightSQLiteError(err)
	}
	keyInfo := (*TUnpackedRecord)(unsafe.Pointer(pIdxKey.ptr)).FpKeyInfo
	if rc := minweightSortIndexRows(ctx, keyInfo, rows); rc != SQLITE_OK {
		return rc
	}
	cur.rows = rows
	rec := (*TUnpackedRecord)(unsafe.Pointer(pIdxKey.ptr))
	rec.FerrCode = uint8(SQLITE_OK)
	rec.FeqSeen = uint8(0)
	for i, row := range rows {
		cmp, rc := minweightCompareIndexKey(ctx, row.key, pIdxKey.ptr)
		if rc != SQLITE_OK {
			return rc
		}
		if cmp == 0 {
			rec.FeqSeen = uint8(1)
		}
		if cmp >= 0 {
			cur.valid = true
			cur.index = i
			minweightWriteResult(pRes, cmp)
			return SQLITE_OK
		}
	}
	cur.valid = false
	cur.index = len(rows)
	minweightWriteResult(pRes, -1)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeEof(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	if e.cursor(pCur).valid {
		return 0
	}
	return 1
}

func (e *minweightStorageEngine) BtreeRowCountEst(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	cur := e.cursor(pCur)
	rows, err := cur.btree.loadRows(cur.root, cur.intKey)
	if err != nil {
		return 0
	}
	return int64(len(rows))
}

func (e *minweightStorageEngine) BtreeNext(ctx BtreeContext, pCur BtreeCursorHandle, flags int32) (r int32) {
	cur := e.cursor(pCur)
	if !cur.valid {
		return SQLITE_DONE
	}
	cur.index++
	if cur.index >= len(cur.rows) {
		cur.valid = false
		return SQLITE_DONE
	}
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreePrevious(ctx BtreeContext, pCur BtreeCursorHandle, flags int32) (r int32) {
	cur := e.cursor(pCur)
	if !cur.valid {
		return SQLITE_DONE
	}
	cur.index--
	if cur.index < 0 {
		cur.valid = false
		return SQLITE_DONE
	}
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeInsert(ctx BtreeContext, pCur BtreeCursorHandle, pX BtreePayloadHandle, flags int32, seekResult int32) (r int32) {
	cur := e.cursor(pCur)
	var key []byte
	var payload []byte
	if cur.intKey {
		payload = pX.DataBytes()
		if zeros := pX.ZeroSize(); zeros > 0 {
			payload = append(payload, make([]byte, zeros)...)
		}
		key = minweightTableKey(cur.root, pX.KeySize())
	} else {
		payload = pX.KeyBytes()
		key = minweightIndexKey(cur.root, payload)
	}
	if err := cur.btree.store.Put(key, payload); err != nil {
		return minweightSQLiteError(err)
	}
	cur.btree.dataVer++
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeTransferRow(ctx BtreeContext, pDest BtreeCursorHandle, pSrc BtreeCursorHandle, iKey int64) (r int32) {
	return SQLITE_ERROR
}

func (e *minweightStorageEngine) BtreeDelete(ctx BtreeContext, pCur BtreeCursorHandle, flags uint8) (r int32) {
	cur := e.cursor(pCur)
	row, ok := cur.current()
	if !ok {
		return SQLITE_OK
	}
	var key []byte
	if cur.intKey {
		key = minweightTableKey(cur.root, row.rowid)
	} else {
		key = minweightIndexKey(cur.root, row.key)
	}
	_, err := cur.btree.store.Delete(key)
	cur.btree.dataVer++
	return minweightSQLiteError(err)
}

func (e *minweightStorageEngine) BtreeCreateTable(ctx BtreeContext, p BtreeHandle, piTable BtreeMemoryHandle, flags int32) (r int32) {
	bt := e.btree(p)
	bt.next++
	root := bt.next
	bt.tables[root] = minweightTable{intKey: flags&int32(BTREE_INTKEY) != 0}
	bt.meta[BTREE_LARGEST_ROOT_PAGE] = root
	piTable.PutUint32(root)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeClearTable(ctx BtreeContext, p BtreeHandle, iTable int32, pnChange BtreeMemoryHandle) (r int32) {
	bt := e.btree(p)
	table := bt.tables[uint32(iTable)]
	n, err := bt.clearRoot(uint32(iTable), table.intKey)
	if !pnChange.IsNil() {
		pnChange.PutInt32(int32(n))
	}
	bt.dataVer++
	return minweightSQLiteError(err)
}

func (e *minweightStorageEngine) BtreeClearTableOfCursor(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	cur := e.cursor(pCur)
	_, err := cur.btree.clearRoot(cur.root, cur.intKey)
	cur.btree.dataVer++
	return minweightSQLiteError(err)
}

func (e *minweightStorageEngine) BtreeDropTable(ctx BtreeContext, p BtreeHandle, iTable int32, piMoved BtreeMemoryHandle) (r int32) {
	bt := e.btree(p)
	table := bt.tables[uint32(iTable)]
	if _, err := bt.clearRoot(uint32(iTable), table.intKey); err != nil {
		return minweightSQLiteError(err)
	}
	delete(bt.tables, uint32(iTable))
	if !piMoved.IsNil() {
		piMoved.PutInt32(0)
	}
	bt.dataVer++
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeGetMeta(ctx BtreeContext, p BtreeHandle, idx int32, pMeta BtreeMemoryHandle) {
	bt := e.btree(p)
	if idx == int32(BTREE_DATA_VERSION) {
		pMeta.PutUint32(bt.dataVer)
		return
	}
	if idx >= 0 && idx < int32(len(bt.meta)) {
		pMeta.PutUint32(bt.meta[idx])
		return
	}
	pMeta.PutUint32(0)
}

func (e *minweightStorageEngine) BtreeUpdateMeta(ctx BtreeContext, p BtreeHandle, idx int32, iMeta uint32) (r int32) {
	bt := e.btree(p)
	if idx >= 0 && idx < int32(len(bt.meta)) {
		bt.meta[idx] = iMeta
	}
	bt.dataVer++
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCount(ctx BtreeContext, db SQLiteHandle, pCur BtreeCursorHandle, pnEntry BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	rows, err := cur.btree.loadRows(cur.root, cur.intKey)
	if err != nil {
		return minweightSQLiteError(err)
	}
	pnEntry.PutInt64(int64(len(rows)))
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreePager(ctx BtreeContext, p BtreeHandle) (r BtreePagerHandle) {
	return btreePagerHandle(ctx.tls, e.btree(p).pager)
}
func (e *minweightStorageEngine) BtreeGetFilename(ctx BtreeContext, p BtreeHandle) (r BtreeCStringHandle) {
	return BtreeCStringHandle{}
}
func (e *minweightStorageEngine) BtreeGetJournalname(ctx BtreeContext, p BtreeHandle) (r BtreeCStringHandle) {
	return BtreeCStringHandle{}
}
func (e *minweightStorageEngine) BtreeTxnState(ctx BtreeContext, p BtreeHandle) (r int32) {
	return 0
}
func (e *minweightStorageEngine) BtreeCheckpoint(ctx BtreeContext, p BtreeHandle, eMode int32, pnLog BtreeMemoryHandle, pnCkpt BtreeMemoryHandle) (r int32) {
	if !pnLog.IsNil() {
		pnLog.PutInt32(0)
	}
	if !pnCkpt.IsNil() {
		pnCkpt.PutInt32(0)
	}
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeIsInBackup(ctx BtreeContext, p BtreeHandle) (r int32) {
	return 0
}

func (e *minweightStorageEngine) BtreeSchema(ctx BtreeContext, p BtreeHandle, nBytes int32, __ccgo_fp_xFree BtreeFunctionHandle) (r BtreeSchemaHandle) {
	bt := e.btree(p)
	if bt.schema == 0 && nBytes != 0 {
		bt.schema = _sqlite3DbMallocZero(ctx.tls, uintptr(0), uint64(nBytes))
	}
	return btreeSchemaHandle(ctx.tls, bt.schema)
}

func (e *minweightStorageEngine) BtreeSchemaLocked(ctx BtreeContext, p BtreeHandle) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeLockTable(ctx BtreeContext, p BtreeHandle, iTab int32, isWriteLock uint8) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreePutData(ctx BtreeContext, pCsr BtreeCursorHandle, offset uint32, amt uint32, z BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCsr)
	row, ok := cur.current()
	if !ok || !cur.intKey {
		return SQLITE_ERROR
	}
	payload := append([]byte(nil), row.payload...)
	end := int(offset + amt)
	if int(offset) > len(payload) {
		return SQLITE_CORRUPT
	}
	if end > len(payload) {
		payload = append(payload, make([]byte, end-len(payload))...)
	}
	copy(payload[offset:end], z.ReadBytes(int(amt)))
	if err := cur.btree.store.Put(minweightTableKey(cur.root, row.rowid), payload); err != nil {
		return minweightSQLiteError(err)
	}
	cur.btree.dataVer++
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeIncrblobCursor(ctx BtreeContext, pCur BtreeCursorHandle) {}
func (e *minweightStorageEngine) BtreeSetVersion(ctx BtreeContext, pBtree BtreeHandle, iVersion int32) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeCursorHasHint(ctx BtreeContext, pCsr BtreeCursorHandle, mask uint32) (r int32) {
	return 0
}
func (e *minweightStorageEngine) BtreeIsReadonly(ctx BtreeContext, p BtreeHandle) (r int32) {
	return 0
}
func (e *minweightStorageEngine) BtreeSharable(ctx BtreeContext, p BtreeHandle) (r int32) {
	return 0
}
func (e *minweightStorageEngine) BtreeConnectionCount(ctx BtreeContext, p BtreeHandle) (r int32) {
	return 1
}
func (e *minweightStorageEngine) BtreeCopyFile(ctx BtreeContext, pTo BtreeHandle, pFrom BtreeHandle) (r int32) {
	return SQLITE_ERROR
}

func (e *minweightStorageEngine) BtreeSetMmapLimit(ctx BtreeContext, p BtreeHandle, szMmap int64) (r int32) {
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeIsEmpty(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	rows, err := cur.btree.loadRows(cur.root, cur.intKey)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if len(rows) == 0 {
		pRes.PutInt32(1)
	} else {
		pRes.PutInt32(0)
	}
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeIntegrityCheck(ctx BtreeContext, db SQLiteHandle, p BtreeHandle, aRoot BtreeMemoryHandle, aCnt BtreeMemoryHandle, nRoot int32, mxErr int32, pnErr BtreeMemoryHandle, pzOut BtreeMemoryHandle) (r int32) {
	if !pnErr.IsNil() {
		pnErr.PutInt32(0)
	}
	if !pzOut.IsNil() {
		pzOut.PutUintptr(0)
	}
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeClearCache(ctx BtreeContext, p BtreeHandle) {}

var _ StorageEngine = (*minweightStorageEngine)(nil)
var _ StorageEngineBtreeSetMmapLimit = (*minweightStorageEngine)(nil)
var _ StorageEngineBtreeIsEmpty = (*minweightStorageEngine)(nil)
var _ StorageEngineBtreeIntegrityCheck = (*minweightStorageEngine)(nil)
var _ StorageEngineBtreeClearCache = (*minweightStorageEngine)(nil)
