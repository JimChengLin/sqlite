// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
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
		aliases: map[uintptr]uintptr{},
		dbs:     map[string]*minweightDatabase{},
		cursors: map[uintptr]*minweightCursor{},
		next:    1,
	}
}

type minweightStorageEngine struct {
	mu      sync.Mutex
	next    uintptr
	btrees  map[uintptr]*minweightBtree
	aliases map[uintptr]uintptr
	dbs     map[string]*minweightDatabase
	cursors map[uintptr]*minweightCursor
}

type minweightDatabase struct {
	mu      sync.Mutex
	store   *minweight.Store
	meta    [SQLITE_N_BTREE_META]uint32
	tables  map[uint32]minweightTable
	next    uint32
	dataVer uint32
	readers map[*minweightBtree]bool
	writer  *minweightBtree
}

type minweightBtree struct {
	*minweightDatabase
	pager       uintptr
	file        uintptr
	journal     uintptr
	filename    uintptr
	journalName uintptr
	schema      uintptr
	db          uintptr
	txnState    int32
	readOnly    bool
	persistWAL  bool
	walActive   bool
}

type minweightTable struct {
	intKey   bool
	rowCount int64
	minRowid int64
	maxRowid int64
}

type minweightCursor struct {
	btree      *minweightBtree
	root       uint32
	intKey     bool
	rows       []minweightRow
	index      int
	valid      bool
	dataVer    uint32
	lastRow    minweightRow
	hasLastRow bool
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

func (e *minweightStorageEngine) btreeTokenLocked(ptr uintptr) uintptr {
	if e.btrees[ptr] != nil {
		return ptr
	}
	if token := e.aliases[ptr]; token != 0 {
		return token
	}
	return ptr
}

func (e *minweightStorageEngine) btree(p BtreeHandle) *minweightBtree {
	e.mu.Lock()
	defer e.mu.Unlock()
	bt := e.btrees[e.btreeTokenLocked(p.ptr)]
	if bt == nil {
		panic(fmt.Sprintf("sqlite minweight storage engine: unknown btree handle %#x", p.ptr))
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

func minweightNewDatabase() *minweightDatabase {
	return &minweightDatabase{
		store:   minweight.New(),
		tables:  map[uint32]minweightTable{1: {intKey: true}},
		next:    1,
		readers: map[*minweightBtree]bool{},
	}
}

func minweightDatabaseKey(zFilename BtreeCStringHandle) string {
	if zFilename.IsNil() {
		return ""
	}
	name := zFilename.String()
	switch name {
	case "", ":memory:", "file::memory:":
		return ""
	default:
		return name
	}
}

func minweightOpenPlaceholder(filename string, readOnly bool) int32 {
	if filename == "" {
		return SQLITE_OK
	}
	if readOnly {
		f, err := os.Open(filename)
		if err != nil {
			return SQLITE_CANTOPEN
		}
		if err := f.Close(); err != nil {
			return SQLITE_CANTOPEN
		}
		return SQLITE_OK
	}
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return SQLITE_CANTOPEN
	}
	if err := f.Close(); err != nil {
		return SQLITE_CANTOPEN
	}
	return SQLITE_OK
}

func minweightAllocCString(ctx BtreeContext, s string) uintptr {
	p := _sqlite3Malloc(ctx.tls, uint64(len(s)+1))
	if p == 0 {
		return 0
	}
	if len(s) != 0 {
		copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(s)+1), s)
	}
	*(*byte)(unsafe.Pointer(p + uintptr(len(s)))) = 0
	return p
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
	var rows []minweightRow
	err := bt.store.Scan(func(item minweight.Item) bool {
		if !bytes.HasPrefix(item.Key, prefix) {
			return true
		}
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

func (bt *minweightBtree) noteInsert(root uint32, rowid int64, existed bool) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	table := bt.tables[root]
	if !existed {
		table.rowCount++
	}
	if table.intKey && !existed {
		if table.rowCount == 1 {
			table.minRowid = rowid
			table.maxRowid = rowid
		} else {
			if rowid < table.minRowid {
				table.minRowid = rowid
			}
			if rowid > table.maxRowid {
				table.maxRowid = rowid
			}
		}
	}
	bt.tables[root] = table
}

func (bt *minweightBtree) resetTableStats(root uint32) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	table := bt.tables[root]
	table.rowCount = 0
	table.minRowid = 0
	table.maxRowid = 0
	bt.tables[root] = table
}

func (bt *minweightBtree) recomputeIntKeyStats(root uint32) error {
	rows, err := bt.loadRows(root, true)
	if err != nil {
		return err
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	table := bt.tables[root]
	table.rowCount = int64(len(rows))
	if len(rows) == 0 {
		table.minRowid = 0
		table.maxRowid = 0
	} else {
		table.minRowid = rows[0].rowid
		table.maxRowid = rows[len(rows)-1].rowid
	}
	bt.tables[root] = table
	return nil
}

func (bt *minweightBtree) noteDelete(root uint32, row minweightRow, deleted bool, intKey bool) error {
	if !deleted {
		return nil
	}
	bt.mu.Lock()
	table := bt.tables[root]
	if table.rowCount > 0 {
		table.rowCount--
	}
	bt.tables[root] = table
	if !intKey {
		bt.mu.Unlock()
		return nil
	}
	if table.rowCount == 0 {
		table.minRowid = 0
		table.maxRowid = 0
		bt.tables[root] = table
		bt.mu.Unlock()
		return nil
	}
	if row.rowid != table.minRowid && row.rowid != table.maxRowid {
		bt.mu.Unlock()
		return nil
	}
	bt.mu.Unlock()
	return bt.recomputeIntKeyStats(root)
}

func (bt *minweightBtree) beginTrans(wrflag int32) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if wrflag != 0 {
		delete(bt.readers, bt)
		bt.writer = bt
		bt.txnState = SQLITE_TXN_WRITE
		return
	}
	if bt.db != 0 && (*Tsqlite3)(unsafe.Pointer(bt.db)).FautoCommit == 0 && bt.txnState == SQLITE_TXN_NONE {
		bt.readers[bt] = true
		bt.txnState = SQLITE_TXN_READ
	}
}

func (bt *minweightBtree) ensureWritable() int32 {
	if bt.readOnly {
		return SQLITE_READONLY
	}
	return SQLITE_OK
}

func (bt *minweightBtree) walFilename() string {
	if bt.filename == 0 {
		return ""
	}
	return libc.GoString(bt.filename) + "-wal"
}

func (bt *minweightBtree) createWALPlaceholder() int32 {
	name := bt.walFilename()
	if name == "" {
		return SQLITE_OK
	}
	bt.mu.Lock()
	active := bt.walActive
	bt.mu.Unlock()
	if active {
		return SQLITE_OK
	}
	f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return SQLITE_CANTOPEN
	}
	if err := f.Close(); err != nil {
		return SQLITE_CANTOPEN
	}
	bt.mu.Lock()
	bt.walActive = true
	bt.mu.Unlock()
	return SQLITE_OK
}

func (bt *minweightBtree) closeWALPlaceholder() int32 {
	name := bt.walFilename()
	if name == "" {
		return SQLITE_OK
	}
	bt.mu.Lock()
	remove := bt.walActive && !bt.persistWAL
	bt.walActive = false
	bt.mu.Unlock()
	if !remove {
		return SQLITE_OK
	}
	if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
		return SQLITE_IOERR
	}
	return SQLITE_OK
}

func (bt *minweightBtree) releaseTrans() {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	delete(bt.readers, bt)
	if bt.writer == bt {
		bt.writer = nil
	}
	bt.txnState = SQLITE_TXN_NONE
}

func (bt *minweightBtree) commitPhaseOne(ctx BtreeContext) int32 {
	for {
		bt.mu.Lock()
		busy := false
		if bt.txnState == SQLITE_TXN_WRITE {
			for reader := range bt.readers {
				if reader != bt {
					busy = true
					break
				}
			}
		}
		bt.mu.Unlock()
		if !busy {
			return SQLITE_OK
		}
		if bt.db == 0 {
			return SQLITE_BUSY
		}
		pBusy := bt.db + unsafe.Offsetof(Tsqlite3{}.FbusyHandler)
		if _sqlite3InvokeBusyHandler(ctx.tls, pBusy) == 0 {
			return SQLITE_BUSY
		}
	}
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

func minweightFindRow(rows []minweightRow, target minweightRow, intKey bool) int {
	for i, row := range rows {
		if intKey {
			if row.rowid == target.rowid {
				return i
			}
			continue
		}
		if bytes.Equal(row.key, target.key) {
			return i
		}
	}
	return -1
}

func minweightFindRowAfter(rows []minweightRow, target minweightRow, intKey bool) int {
	if intKey {
		return sort.Search(len(rows), func(i int) bool { return rows[i].rowid > target.rowid })
	}
	for i, row := range rows {
		if bytes.Compare(row.key, target.key) > 0 {
			return i
		}
	}
	return len(rows)
}

func minweightFindRowAtOrAfter(rows []minweightRow, target minweightRow, intKey bool) int {
	if intKey {
		return sort.Search(len(rows), func(i int) bool { return rows[i].rowid >= target.rowid })
	}
	for i, row := range rows {
		if bytes.Compare(row.key, target.key) >= 0 {
			return i
		}
	}
	return len(rows)
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
	pIdxRecord := (*TUnpackedRecord)(unsafe.Pointer(pIdxKey))
	nMem := int((*TKeyInfo)(unsafe.Pointer(keyInfo)).FnKeyField) + 1
	clear(unsafe.Slice((*byte)(unsafe.Pointer(pIdxRecord.FaMem)), int(unsafe.Sizeof(TMem{}))*nMem))
	pIdxRecord.FerrCode = uint8(SQLITE_OK)
	pIdxRecord.FeqSeen = uint8(0)
	buf := _sqlite3MallocZero(ctx.tls, uint64(len(b)+18))
	if buf == 0 {
		_sqlite3DbFreeNN(ctx.tls, (*TKeyInfo)(unsafe.Pointer(keyInfo)).Fdb, pIdxKey)
		return 0, SQLITE_NOMEM
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(buf)), len(b)), b)
	_sqlite3VdbeRecordUnpack(ctx.tls, int32(len(b)), buf, pIdxKey)
	cmp, rc := minweightCompareIndexKey(ctx, a, pIdxKey)
	Xsqlite3_free(ctx.tls, buf)
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

func (e *minweightStorageEngine) refreshCursorRows(ctx BtreeContext, pCur BtreeCursorHandle, cur *minweightCursor) int32 {
	rows, err := cur.btree.loadRows(cur.root, cur.intKey)
	if err != nil {
		return minweightSQLiteError(err)
	}
	cur.rows = rows
	cur.dataVer = cur.btree.dataVer
	if !cur.intKey {
		keyInfo := (*BtCursor)(unsafe.Pointer(pCur.ptr)).FpKeyInfo
		if keyInfo != 0 {
			if rc := minweightSortIndexRows(ctx, keyInfo, cur.rows); rc != SQLITE_OK {
				return rc
			}
		}
	}
	return SQLITE_OK
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
	key := minweightDatabaseKey(zFilename)
	readOnly := vfsFlags&SQLITE_OPEN_READONLY != 0
	if key != "" {
		if readOnly {
			e.mu.Lock()
			existing := e.dbs[key]
			e.mu.Unlock()
			if existing == nil {
				return SQLITE_CANTOPEN
			}
		}
		if rc := minweightOpenPlaceholder(key, readOnly); rc != SQLITE_OK {
			return rc
		}
	}
	pager := _sqlite3MallocZero(ctx.tls, uint64(unsafe.Sizeof(Pager{})))
	file := _sqlite3MallocZero(ctx.tls, uint64(unsafe.Sizeof(Tsqlite3_file{})))
	journal := _sqlite3MallocZero(ctx.tls, uint64(unsafe.Sizeof(Tsqlite3_file{})))
	(*Pager)(unsafe.Pointer(pager)).FpageSize = 4096
	(*Pager)(unsafe.Pointer(pager)).Ffd = file
	(*Pager)(unsafe.Pointer(pager)).Fjfd = journal
	(*Pager)(unsafe.Pointer(pager)).FnoLock = 1
	(*Pager)(unsafe.Pointer(pager)).FreadOnly = libc.Uint8FromInt32(libc.BoolInt32(readOnly))
	database := minweightNewDatabase()
	var filename uintptr
	var journalName uintptr
	if key != "" {
		filename = minweightAllocCString(ctx, key)
		journalName = minweightAllocCString(ctx, key+"-journal")
		if filename == 0 || journalName == 0 {
			if filename != 0 {
				Xsqlite3_free(ctx.tls, filename)
			}
			if journalName != 0 {
				Xsqlite3_free(ctx.tls, journalName)
			}
			Xsqlite3_free(ctx.tls, pager)
			Xsqlite3_free(ctx.tls, file)
			Xsqlite3_free(ctx.tls, journal)
			return SQLITE_NOMEM
		}
	}
	bt := &minweightBtree{
		minweightDatabase: database,
		pager:             pager,
		file:              file,
		journal:           journal,
		filename:          filename,
		journalName:       journalName,
		db:                db.ptr,
		readOnly:          readOnly,
	}
	e.mu.Lock()
	if key != "" {
		if existing := e.dbs[key]; existing != nil {
			bt.minweightDatabase = existing
		} else if readOnly {
			e.mu.Unlock()
			if filename != 0 {
				Xsqlite3_free(ctx.tls, filename)
			}
			if journalName != 0 {
				Xsqlite3_free(ctx.tls, journalName)
			}
			Xsqlite3_free(ctx.tls, pager)
			Xsqlite3_free(ctx.tls, file)
			Xsqlite3_free(ctx.tls, journal)
			return SQLITE_CANTOPEN
		} else {
			e.dbs[key] = database
		}
	}
	token := e.nextToken()
	e.btrees[token] = bt
	if db.ptr != 0 && e.aliases[db.ptr] == 0 {
		e.aliases[db.ptr] = token
	}
	e.mu.Unlock()
	ppBtree.PutBtreeToken(BtreeToken(token))
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeClose(ctx BtreeContext, p BtreeHandle) (r int32) {
	e.mu.Lock()
	token := e.btreeTokenLocked(p.ptr)
	bt := e.btrees[token]
	delete(e.btrees, token)
	for alias, target := range e.aliases {
		if alias == p.ptr || target == token {
			delete(e.aliases, alias)
		}
	}
	e.mu.Unlock()
	if bt != nil && bt.schema != 0 {
		Xsqlite3_free(ctx.tls, bt.schema)
	}
	if bt != nil {
		if rc := bt.closeWALPlaceholder(); rc != SQLITE_OK {
			return rc
		}
	}
	if bt != nil && bt.pager != 0 {
		Xsqlite3_free(ctx.tls, bt.pager)
	}
	if bt != nil && bt.file != 0 {
		Xsqlite3_free(ctx.tls, bt.file)
	}
	if bt != nil && bt.journal != 0 {
		Xsqlite3_free(ctx.tls, bt.journal)
	}
	if bt != nil && bt.filename != 0 {
		Xsqlite3_free(ctx.tls, bt.filename)
	}
	if bt != nil && bt.journalName != 0 {
		Xsqlite3_free(ctx.tls, bt.journalName)
	}
	if bt != nil {
		bt.releaseTrans()
	}
	return SQLITE_OK
}

func (e *minweightStorageEngine) FileControlPersistWAL(ctx BtreeContext, db SQLiteHandle, dbName string, mode int32) (int32, int32) {
	if dbName != "" && dbName != "main" {
		return mode, SQLITE_ERROR
	}
	e.mu.Lock()
	token := e.aliases[db.ptr]
	bt := e.btrees[token]
	e.mu.Unlock()
	if bt == nil {
		return mode, SQLITE_ERROR
	}
	bt.mu.Lock()
	if mode >= 0 {
		bt.persistWAL = mode != 0
	}
	if bt.persistWAL {
		mode = 1
	} else {
		mode = 0
	}
	bt.mu.Unlock()
	return mode, SQLITE_OK
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
	if rc := bt.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	bt.mu.Lock()
	bt.store = minweight.New()
	bt.tables = map[uint32]minweightTable{1: {intKey: true}}
	bt.next = 1
	bt.meta = [SQLITE_N_BTREE_META]uint32{}
	bt.mu.Unlock()
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeBeginTrans(ctx BtreeContext, p BtreeHandle, wrflag int32, pSchemaVersion BtreeMemoryHandle) (r int32) {
	bt := e.btree(p)
	if !pSchemaVersion.IsNil() {
		pSchemaVersion.PutUint32(bt.meta[BTREE_SCHEMA_VERSION])
	}
	if wrflag != 0 {
		if rc := bt.ensureWritable(); rc != SQLITE_OK {
			return rc
		}
		if rc := bt.createWALPlaceholder(); rc != SQLITE_OK {
			return rc
		}
	}
	bt.beginTrans(wrflag)
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeIncrVacuum(ctx BtreeContext, p BtreeHandle) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeCommitPhaseOne(ctx BtreeContext, p BtreeHandle, zSuperJrnl BtreeCStringHandle) (r int32) {
	return e.btree(p).commitPhaseOne(ctx)
}
func (e *minweightStorageEngine) BtreeCommitPhaseTwo(ctx BtreeContext, p BtreeHandle, bCleanup int32) (r int32) {
	e.btree(p).releaseTrans()
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeCommit(ctx BtreeContext, p BtreeHandle) (r int32) {
	e.btree(p).releaseTrans()
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeTripAllCursors(ctx BtreeContext, pBtree BtreeHandle, errCode int32, writeOnly int32) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeRollback(ctx BtreeContext, p BtreeHandle, tripCode int32, writeOnly int32) (r int32) {
	e.btree(p).releaseTrans()
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
	if wrFlag != 0 {
		if rc := bt.ensureWritable(); rc != SQLITE_OK {
			return rc
		}
	}
	bt.mu.Lock()
	table, ok := bt.tables[iTable]
	if !ok {
		table = minweightTable{intKey: pKeyInfo.IsNil()}
		bt.tables[iTable] = table
	}
	bt.mu.Unlock()
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
	cur.payloadBuf = _sqlite3MallocZero(ctx.tls, uint64(len(data)+18))
	if len(data) != 0 {
		copy(unsafe.Slice((*byte)(unsafe.Pointer(cur.payloadBuf)), len(data)), data)
	}
	pAmt.PutInt32(int32(len(data)))
	return btreeMemoryHandle(ctx.tls, cur.payloadBuf)
}

func (e *minweightStorageEngine) BtreeFirst(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	if rc := e.refreshCursorRows(ctx, pCur, cur); rc != SQLITE_OK {
		return rc
	}
	if len(cur.rows) == 0 {
		cur.valid = false
		cur.index = -1
		minweightWriteResult(pRes, 1)
		return SQLITE_OK
	}
	cur.valid = true
	cur.index = 0
	cur.hasLastRow = false
	minweightWriteResult(pRes, 0)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeLast(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	cur.btree.mu.Lock()
	table := cur.btree.tables[cur.root]
	cur.btree.mu.Unlock()
	if cur.intKey {
		if table.rowCount == 0 {
			cur.valid = false
			cur.index = -1
			minweightWriteResult(pRes, 1)
			return SQLITE_OK
		}
		payload, ok, err := cur.btree.store.Get(minweightTableKey(cur.root, table.maxRowid))
		if err != nil {
			return minweightSQLiteError(err)
		}
		if !ok {
			return SQLITE_CORRUPT
		}
		cur.rows = []minweightRow{{rowid: table.maxRowid, payload: append([]byte(nil), payload...)}}
		cur.dataVer = cur.btree.dataVer
		cur.valid = true
		cur.index = 0
		cur.hasLastRow = false
		minweightWriteResult(pRes, 0)
		return SQLITE_OK
	}
	if rc := e.refreshCursorRows(ctx, pCur, cur); rc != SQLITE_OK {
		return rc
	}
	if len(cur.rows) == 0 {
		cur.valid = false
		cur.index = -1
		minweightWriteResult(pRes, 1)
		return SQLITE_OK
	}
	cur.valid = true
	cur.index = len(cur.rows) - 1
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
	cur.dataVer = cur.btree.dataVer
	i := sort.Search(len(rows), func(i int) bool { return rows[i].rowid >= intKey })
	if i < len(rows) {
		cur.valid = true
		cur.index = i
		if rows[i].rowid == intKey {
			minweightWriteResult(pRes, 0)
		} else {
			minweightWriteResult(pRes, 1)
		}
		cur.hasLastRow = false
		return SQLITE_OK
	}
	cur.valid = false
	cur.index = len(rows)
	minweightWriteResult(pRes, -1)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeIndexMoveto(ctx BtreeContext, pCur BtreeCursorHandle, pIdxKey BtreeIndexKeyHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	keyInfo := (*TUnpackedRecord)(unsafe.Pointer(pIdxKey.ptr)).FpKeyInfo
	if rc := e.refreshCursorRows(ctx, pCur, cur); rc != SQLITE_OK {
		return rc
	}
	if rc := minweightSortIndexRows(ctx, keyInfo, cur.rows); rc != SQLITE_OK {
		return rc
	}
	rec := (*TUnpackedRecord)(unsafe.Pointer(pIdxKey.ptr))
	rec.FerrCode = uint8(SQLITE_OK)
	rec.FeqSeen = uint8(0)
	for i, row := range cur.rows {
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
			cur.hasLastRow = false
			minweightWriteResult(pRes, cmp)
			return SQLITE_OK
		}
	}
	cur.valid = false
	cur.index = len(cur.rows)
	minweightWriteResult(pRes, -1)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeEof(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.valid {
		return 0
	}
	oldIndex := cur.index
	if cur.hasLastRow {
		if cur.dataVer != cur.btree.dataVer {
			if rc := e.refreshCursorRows(ctx, pCur, cur); rc != SQLITE_OK {
				return 1
			}
		}
		i := minweightFindRowAfter(cur.rows, cur.lastRow, cur.intKey)
		if i < len(cur.rows) {
			cur.valid = true
			cur.index = i
			cur.hasLastRow = false
			return 0
		}
	}
	if oldIndex >= 0 {
		if cur.dataVer != cur.btree.dataVer {
			if rc := e.refreshCursorRows(ctx, pCur, cur); rc != SQLITE_OK {
				return 1
			}
		}
		if oldIndex < len(cur.rows) {
			cur.valid = true
			cur.index = oldIndex
			return 0
		}
		cur.index = oldIndex
	}
	return 1
}

func (e *minweightStorageEngine) BtreeRowCountEst(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	cur := e.cursor(pCur)
	cur.btree.mu.Lock()
	rowCount := cur.btree.tables[cur.root].rowCount
	cur.btree.mu.Unlock()
	return rowCount
}

func (e *minweightStorageEngine) BtreeNext(ctx BtreeContext, pCur BtreeCursorHandle, flags int32) (r int32) {
	cur := e.cursor(pCur)
	if !cur.valid {
		if cur.hasLastRow {
			if cur.dataVer != cur.btree.dataVer {
				if rc := e.refreshCursorRows(ctx, pCur, cur); rc != SQLITE_OK {
					return rc
				}
			}
			i := minweightFindRowAfter(cur.rows, cur.lastRow, cur.intKey)
			if i < len(cur.rows) {
				cur.valid = true
				cur.index = i
				cur.hasLastRow = false
				return SQLITE_OK
			}
			return SQLITE_DONE
		}
		oldIndex := cur.index
		if oldIndex >= 0 {
			if cur.dataVer != cur.btree.dataVer {
				if rc := e.refreshCursorRows(ctx, pCur, cur); rc != SQLITE_OK {
					return rc
				}
			}
			if oldIndex < len(cur.rows) {
				cur.valid = true
				cur.index = oldIndex
				return SQLITE_OK
			}
			cur.index = oldIndex
		}
		return SQLITE_DONE
	}
	row, ok := cur.current()
	if cur.dataVer != cur.btree.dataVer {
		oldRows := cur.rows
		oldIndex := cur.index
		if rc := e.refreshCursorRows(ctx, pCur, cur); rc != SQLITE_OK {
			return rc
		}
		if i := minweightFindRow(cur.rows, row, cur.intKey); i >= 0 {
			cur.index = i
		} else if cur.intKey {
			cur.index = minweightFindRowAfter(cur.rows, row, true) - 1
		} else {
			cur.rows = oldRows
			cur.index = oldIndex
		}
	}
	cur.index++
	if cur.index >= len(cur.rows) {
		cur.valid = false
		if ok {
			cur.lastRow = row
			cur.hasLastRow = true
		}
		return SQLITE_DONE
	}
	cur.hasLastRow = false
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreePrevious(ctx BtreeContext, pCur BtreeCursorHandle, flags int32) (r int32) {
	cur := e.cursor(pCur)
	if !cur.valid {
		if cur.hasLastRow {
			if cur.dataVer != cur.btree.dataVer {
				if rc := e.refreshCursorRows(ctx, pCur, cur); rc != SQLITE_OK {
					return rc
				}
			}
			i := minweightFindRowAtOrAfter(cur.rows, cur.lastRow, cur.intKey) - 1
			if i >= 0 {
				cur.valid = true
				cur.index = i
				cur.hasLastRow = false
				return SQLITE_OK
			}
			return SQLITE_DONE
		}
		oldIndex := cur.index
		if oldIndex < 0 {
			return SQLITE_DONE
		}
		if cur.dataVer != cur.btree.dataVer {
			if rc := e.refreshCursorRows(ctx, pCur, cur); rc != SQLITE_OK {
				return rc
			}
		}
		if oldIndex > len(cur.rows) {
			oldIndex = len(cur.rows)
		}
		cur.valid = true
		cur.index = oldIndex
	}
	row, ok := cur.current()
	if cur.dataVer != cur.btree.dataVer || cur.intKey && len(cur.rows) == 1 {
		oldRows := cur.rows
		oldIndex := cur.index
		if rc := e.refreshCursorRows(ctx, pCur, cur); rc != SQLITE_OK {
			return rc
		}
		if i := minweightFindRow(cur.rows, row, cur.intKey); i >= 0 {
			cur.index = i
		} else if cur.intKey {
			cur.index = minweightFindRowAtOrAfter(cur.rows, row, true)
		} else {
			cur.rows = oldRows
			cur.index = oldIndex
		}
	}
	cur.index--
	if cur.index < 0 {
		cur.valid = false
		if ok {
			cur.lastRow = row
			cur.hasLastRow = true
		}
		return SQLITE_DONE
	}
	cur.hasLastRow = false
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeInsert(ctx BtreeContext, pCur BtreeCursorHandle, pX BtreePayloadHandle, flags int32, seekResult int32) (r int32) {
	cur := e.cursor(pCur)
	if rc := cur.btree.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	var key []byte
	var payload []byte
	var rowid int64
	if cur.intKey {
		rowid = pX.KeySize()
		payload = pX.DataBytes()
		if zeros := pX.ZeroSize(); zeros > 0 {
			payload = append(payload, make([]byte, zeros)...)
		}
		key = minweightTableKey(cur.root, rowid)
	} else {
		payload = pX.KeyBytes()
		key = minweightIndexKey(cur.root, payload)
	}
	_, existed, err := cur.btree.store.Get(key)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if err := cur.btree.store.Put(key, payload); err != nil {
		return minweightSQLiteError(err)
	}
	cur.btree.noteInsert(cur.root, rowid, existed)
	cur.btree.dataVer++
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeTransferRow(ctx BtreeContext, pDest BtreeCursorHandle, pSrc BtreeCursorHandle, iKey int64) (r int32) {
	if rc := e.cursor(pDest).btree.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	return SQLITE_ERROR
}

func (e *minweightStorageEngine) BtreeDelete(ctx BtreeContext, pCur BtreeCursorHandle, flags uint8) (r int32) {
	cur := e.cursor(pCur)
	if rc := cur.btree.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
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
	deleted, err := cur.btree.store.Delete(key)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if err := cur.btree.noteDelete(cur.root, row, deleted, cur.intKey); err != nil {
		return minweightSQLiteError(err)
	}
	cur.btree.dataVer++
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCreateTable(ctx BtreeContext, p BtreeHandle, piTable BtreeMemoryHandle, flags int32) (r int32) {
	bt := e.btree(p)
	if rc := bt.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	bt.mu.Lock()
	bt.next++
	root := bt.next
	bt.tables[root] = minweightTable{intKey: flags&int32(BTREE_INTKEY) != 0}
	bt.meta[BTREE_LARGEST_ROOT_PAGE] = root
	bt.mu.Unlock()
	piTable.PutUint32(root)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeClearTable(ctx BtreeContext, p BtreeHandle, iTable int32, pnChange BtreeMemoryHandle) (r int32) {
	bt := e.btree(p)
	if rc := bt.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	bt.mu.Lock()
	table := bt.tables[uint32(iTable)]
	bt.mu.Unlock()
	n, err := bt.clearRoot(uint32(iTable), table.intKey)
	if err == nil {
		bt.resetTableStats(uint32(iTable))
	}
	if !pnChange.IsNil() {
		pnChange.PutInt32(int32(n))
	}
	bt.dataVer++
	return minweightSQLiteError(err)
}

func (e *minweightStorageEngine) BtreeClearTableOfCursor(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	cur := e.cursor(pCur)
	if rc := cur.btree.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	_, err := cur.btree.clearRoot(cur.root, cur.intKey)
	if err == nil {
		cur.btree.resetTableStats(cur.root)
	}
	cur.btree.dataVer++
	return minweightSQLiteError(err)
}

func (e *minweightStorageEngine) BtreeDropTable(ctx BtreeContext, p BtreeHandle, iTable int32, piMoved BtreeMemoryHandle) (r int32) {
	bt := e.btree(p)
	if rc := bt.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	bt.mu.Lock()
	table := bt.tables[uint32(iTable)]
	bt.mu.Unlock()
	if _, err := bt.clearRoot(uint32(iTable), table.intKey); err != nil {
		return minweightSQLiteError(err)
	}
	bt.mu.Lock()
	delete(bt.tables, uint32(iTable))
	bt.mu.Unlock()
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
	if rc := bt.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	if idx >= 0 && idx < int32(len(bt.meta)) {
		bt.meta[idx] = iMeta
	}
	bt.dataVer++
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCount(ctx BtreeContext, db SQLiteHandle, pCur BtreeCursorHandle, pnEntry BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	cur.btree.mu.Lock()
	rowCount := cur.btree.tables[cur.root].rowCount
	cur.btree.mu.Unlock()
	pnEntry.PutInt64(rowCount)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreePager(ctx BtreeContext, p BtreeHandle) (r BtreePagerHandle) {
	bt := e.btree(p)
	(*Pager)(unsafe.Pointer(bt.pager)).FiDataVersion = bt.dataVer
	return btreePagerHandle(ctx.tls, bt.pager)
}
func (e *minweightStorageEngine) BtreeGetFilename(ctx BtreeContext, p BtreeHandle) (r BtreeCStringHandle) {
	return btreeCStringHandle(ctx.tls, e.btree(p).filename)
}
func (e *minweightStorageEngine) BtreeGetJournalname(ctx BtreeContext, p BtreeHandle) (r BtreeCStringHandle) {
	return btreeCStringHandle(ctx.tls, e.btree(p).journalName)
}
func (e *minweightStorageEngine) BtreeTxnState(ctx BtreeContext, p BtreeHandle) (r int32) {
	if p.IsNil() {
		return SQLITE_TXN_NONE
	}
	return e.btree(p).txnState
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
	if rc := cur.btree.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
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
	if rc := e.btree(pBtree).ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeCursorHasHint(ctx BtreeContext, pCsr BtreeCursorHandle, mask uint32) (r int32) {
	return 0
}
func (e *minweightStorageEngine) BtreeIsReadonly(ctx BtreeContext, p BtreeHandle) (r int32) {
	if e.btree(p).readOnly {
		return 1
	}
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
	cur.btree.mu.Lock()
	empty := cur.btree.tables[cur.root].rowCount == 0
	cur.btree.mu.Unlock()
	if empty {
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
