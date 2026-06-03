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
	"strings"
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
	mu            sync.Mutex
	store         *minweight.Store
	meta          [SQLITE_N_BTREE_META]uint32
	tables        map[uint32]minweightTable
	next          uint32
	dataVer       uint32
	pageSize      int32
	reserve       int32
	reserveWanted int32
	pageSizeFixed bool
	maxPageCount  uint32
	secureDelete  int32
	autoVacuum    int32
	cacheSize     int32
	spillSize     int32
	freeRoots     []uint32
	readers       map[*minweightBtree]bool
	writer        *minweightBtree
	sharedRefs    int32
	tableLocks    map[uint32]map[*minweightBtree]uint8
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
	sharable    bool
	persistWAL  bool
	backupCount int32
	mmapSize    int64
	walActive   bool
	txSnapshot  *minweightSnapshot
	savepoints  []minweightSnapshot
}

type minweightFile struct {
	Tsqlite3_file
	btreeToken uintptr
}

var minweightFileMethods = Tsqlite3_io_methods{
	FiVersion:               1,
	FxFileControl:           __ccgo_fp(minweightFileControl),
	FxSectorSize:            __ccgo_fp(minweightFileSectorSize),
	FxDeviceCharacteristics: __ccgo_fp(minweightFileDeviceCharacteristics),
}

type minweightTable struct {
	intKey   bool
	rowCount int64
	minRowid int64
	maxRowid int64
}

type minweightCursor struct {
	btree           *minweightBtree
	root            uint32
	intKey          bool
	rows            []minweightRow
	index           int
	valid           bool
	writable        bool
	incrblob        bool
	incrblobInvalid bool
	faultCode       int32
	dataVer         uint32
	lastRow         minweightRow
	hasLastRow      bool
	payloadBuf      uintptr
	transferRow     minweightRow
	hasTransferRow  bool
}

type minweightRow struct {
	rowid   int64
	key     []byte
	payload []byte
}

type minweightSnapshot struct {
	items         []minweightSnapshotItem
	meta          [SQLITE_N_BTREE_META]uint32
	tables        map[uint32]minweightTable
	next          uint32
	dataVer       uint32
	pageSize      int32
	reserve       int32
	reserveWanted int32
	pageSizeFixed bool
	maxPageCount  uint32
	secureDelete  int32
	autoVacuum    int32
	freeRoots     []uint32
}

type minweightSnapshotItem struct {
	key   []byte
	value []byte
}

type minweightIntegrityStats struct {
	rowCount int64
	minRowid int64
	maxRowid int64
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

func (e *minweightStorageEngine) btreeForDB(db SQLiteHandle) *minweightBtree {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.btrees[e.aliases[db.ptr]]
}

func (e *minweightStorageEngine) btreeForToken(token uintptr) *minweightBtree {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.btrees[token]
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

func minweightBtreeForFile(pFile uintptr) *minweightBtree {
	engine, ok := storageEngine().(*minweightStorageEngine)
	if !ok {
		return nil
	}
	return engine.btreeForToken((*minweightFile)(unsafe.Pointer(pFile)).btreeToken)
}

func minweightSQLiteError(err error) int32 {
	if err == nil {
		return SQLITE_OK
	}
	return SQLITE_ERROR
}

func minweightValidPageSize(pageSize int32) bool {
	return pageSize >= 512 && pageSize <= SQLITE_MAX_PAGE_SIZE && pageSize&(pageSize-1) == 0
}

func minweightCachePages(pageSize int32, cacheSize int32) int32 {
	if cacheSize >= 0 {
		return cacheSize
	}
	n := int64(-1024) * int64(cacheSize) / int64(pageSize)
	if n > 1000000000 {
		n = 1000000000
	}
	return int32(n)
}

func minweightEffectiveSpillSize(pageSize int32, cacheSize int32, spillSize int32) int32 {
	if spillSize < 0 {
		spillSize = int32(int64(-1024) * int64(spillSize) / int64(pageSize))
	}
	cachePages := minweightCachePages(pageSize, cacheSize)
	if cachePages > spillSize {
		return cachePages
	}
	return spillSize
}

func minweightNewDatabase() *minweightDatabase {
	return &minweightDatabase{
		store:        minweight.New(),
		tables:       map[uint32]minweightTable{1: {intKey: true}},
		next:         1,
		pageSize:     SQLITE_DEFAULT_PAGE_SIZE,
		maxPageCount: SQLITE_MAX_PAGE_COUNT,
		cacheSize:    SQLITE_DEFAULT_CACHE_SIZE,
		spillSize:    1,
		readers:      map[*minweightBtree]bool{},
		tableLocks:   map[uint32]map[*minweightBtree]uint8{},
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

func minweightClampMmapLimit(szMmap Tsqlite3_int64) Tsqlite3_int64 {
	if szMmap > _sqlite3Config.FmxMmap {
		return _sqlite3Config.FmxMmap
	}
	return szMmap
}

func minweightFileControl(tls *libc.TLS, pFile uintptr, op int32, pArg uintptr) int32 {
	if op != int32(SQLITE_FCNTL_MMAP_SIZE) {
		return SQLITE_NOTFOUND
	}
	bt := minweightBtreeForFile(pFile)
	if bt == nil {
		return SQLITE_NOTFOUND
	}
	newLimit := minweightClampMmapLimit(*(*Tsqlite3_int64)(unsafe.Pointer(pArg)))
	bt.mu.Lock()
	defer bt.mu.Unlock()
	*(*Tsqlite3_int64)(unsafe.Pointer(pArg)) = Tsqlite3_int64(bt.mmapSize)
	if newLimit >= 0 {
		bt.mmapSize = int64(newLimit)
		(*Pager)(unsafe.Pointer(bt.pager)).FszMmap = newLimit
	}
	return SQLITE_OK
}

func minweightFileSectorSize(tls *libc.TLS, pFile uintptr) int32 {
	return SQLITE_DEFAULT_SECTOR_SIZE
}

func minweightFileDeviceCharacteristics(tls *libc.TLS, pFile uintptr) int32 {
	return 0
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

func minweightIntegrityRoots(aRoot BtreeMemoryHandle, nRoot int32) ([]uint32, bool) {
	if aRoot.IsNil() || nRoot <= 0 {
		return nil, false
	}
	roots := make([]uint32, nRoot)
	for i := range roots {
		roots[i] = *(*uint32)(unsafe.Pointer(aRoot.ptr + uintptr(i)*4))
	}
	return roots, roots[0] == 0
}

func minweightIntegritySelectedRoots(roots []uint32, partial bool) map[uint32]bool {
	if len(roots) == 0 {
		return nil
	}
	selected := make(map[uint32]bool, len(roots))
	start := 0
	if partial {
		start = 1
	}
	for _, root := range roots[start:] {
		if root != 0 {
			selected[root] = true
		}
	}
	return selected
}

func minweightIntegrityRootChecked(root uint32, partial bool, selected map[uint32]bool) bool {
	if !partial {
		return true
	}
	return selected[root]
}

func minweightAddIntegrityError(errors *[]string, mxErr int32, format string, args ...any) bool {
	if mxErr <= 0 || int32(len(*errors)) >= mxErr {
		return false
	}
	*errors = append(*errors, fmt.Sprintf(format, args...))
	return int32(len(*errors)) < mxErr
}

func minweightAddIntegrityRowid(stats map[uint32]minweightIntegrityStats, root uint32, rowid int64) {
	stat := stats[root]
	if stat.rowCount == 0 {
		stat.minRowid = rowid
		stat.maxRowid = rowid
	} else {
		if rowid < stat.minRowid {
			stat.minRowid = rowid
		}
		if rowid > stat.maxRowid {
			stat.maxRowid = rowid
		}
	}
	stat.rowCount++
	stats[root] = stat
}

func minweightAddIntegrityIndexRow(stats map[uint32]minweightIntegrityStats, root uint32) {
	stat := stats[root]
	stat.rowCount++
	stats[root] = stat
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

func minweightCloneTables(src map[uint32]minweightTable) map[uint32]minweightTable {
	dst := make(map[uint32]minweightTable, len(src))
	for root, table := range src {
		dst[root] = table
	}
	return dst
}

func minweightCloneRootList(src []uint32) []uint32 {
	return append([]uint32(nil), src...)
}

func (bt *minweightBtree) snapshot() (*minweightSnapshot, error) {
	s := &minweightSnapshot{}
	if err := bt.store.Scan(func(item minweight.Item) bool {
		s.items = append(s.items, minweightSnapshotItem{
			key:   append([]byte(nil), item.Key...),
			value: append([]byte(nil), item.Value...),
		})
		return true
	}); err != nil {
		return nil, err
	}
	bt.mu.Lock()
	s.meta = bt.meta
	s.tables = minweightCloneTables(bt.tables)
	s.next = bt.next
	s.dataVer = bt.dataVer
	s.pageSize = bt.pageSize
	s.reserve = bt.reserve
	s.reserveWanted = bt.reserveWanted
	s.pageSizeFixed = bt.pageSizeFixed
	s.maxPageCount = bt.maxPageCount
	s.secureDelete = bt.secureDelete
	s.autoVacuum = bt.autoVacuum
	s.freeRoots = minweightCloneRootList(bt.freeRoots)
	bt.mu.Unlock()
	return s, nil
}

func (bt *minweightBtree) restoreSnapshot(s minweightSnapshot) error {
	store := minweight.New()
	for _, item := range s.items {
		if err := store.Put(item.key, item.value); err != nil {
			return err
		}
	}
	bt.mu.Lock()
	dataVer := bt.dataVer + 1
	bt.store = store
	bt.meta = s.meta
	bt.tables = minweightCloneTables(s.tables)
	bt.next = s.next
	bt.pageSize = s.pageSize
	bt.reserve = s.reserve
	bt.reserveWanted = s.reserveWanted
	bt.pageSizeFixed = s.pageSizeFixed
	bt.maxPageCount = s.maxPageCount
	bt.secureDelete = s.secureDelete
	bt.autoVacuum = s.autoVacuum
	bt.freeRoots = minweightCloneRootList(s.freeRoots)
	bt.dataVer = dataVer
	bt.mu.Unlock()
	return nil
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

func (bt *minweightBtree) moveRoot(from uint32, to uint32, intKey bool) error {
	rows, err := bt.loadRows(from, intKey)
	if err != nil {
		return err
	}
	for _, row := range rows {
		var key []byte
		if intKey {
			key = minweightTableKey(to, row.rowid)
		} else {
			key = minweightIndexKey(to, row.key)
		}
		if err := bt.store.Put(key, row.payload); err != nil {
			return err
		}
	}
	for _, row := range rows {
		var key []byte
		if intKey {
			key = minweightTableKey(from, row.rowid)
		} else {
			key = minweightIndexKey(from, row.key)
		}
		if _, err := bt.store.Delete(key); err != nil {
			return err
		}
	}
	return nil
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

func (bt *minweightBtree) ensureTransactionSnapshot() error {
	bt.mu.Lock()
	needsSnapshot := bt.txSnapshot == nil
	bt.mu.Unlock()
	if !needsSnapshot {
		return nil
	}
	s, err := bt.snapshot()
	if err != nil {
		return err
	}
	bt.mu.Lock()
	if bt.txSnapshot == nil {
		bt.txSnapshot = s
	}
	bt.mu.Unlock()
	return nil
}

func (bt *minweightBtree) ensureSavepoints(n int32) error {
	if n <= 0 {
		return nil
	}
	for {
		bt.mu.Lock()
		needsSnapshot := len(bt.savepoints) < int(n)
		bt.mu.Unlock()
		if !needsSnapshot {
			return nil
		}
		s, err := bt.snapshot()
		if err != nil {
			return err
		}
		bt.mu.Lock()
		if len(bt.savepoints) < int(n) {
			bt.savepoints = append(bt.savepoints, *s)
		}
		bt.mu.Unlock()
	}
}

func (bt *minweightBtree) sqliteSavepointCount() int32 {
	if bt.db == 0 {
		return 0
	}
	return (*Tsqlite3)(unsafe.Pointer(bt.db)).FnSavepoint
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

func (bt *minweightBtree) retainConnection() {
	if !bt.sharable {
		return
	}
	bt.mu.Lock()
	bt.sharedRefs++
	bt.mu.Unlock()
}

func (bt *minweightBtree) releaseConnection() {
	if !bt.sharable {
		return
	}
	bt.mu.Lock()
	if bt.sharedRefs > 0 {
		bt.sharedRefs--
	}
	bt.mu.Unlock()
}

func (bt *minweightBtree) connectionCount() int32 {
	if !bt.sharable {
		return 1
	}
	bt.mu.Lock()
	sharedRefs := bt.sharedRefs
	bt.mu.Unlock()
	if sharedRefs == 0 {
		return 1
	}
	return sharedRefs
}

func (bt *minweightBtree) queryTableLockLocked(root uint32, lock uint8) int32 {
	if !bt.sharable {
		return SQLITE_OK
	}
	for holder, held := range bt.tableLocks[root] {
		if holder != bt && held != lock {
			return SQLITE_LOCKED_SHAREDCACHE
		}
	}
	return SQLITE_OK
}

func (bt *minweightBtree) lockTable(root uint32, lock uint8) int32 {
	if !bt.sharable {
		return SQLITE_OK
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if rc := bt.queryTableLockLocked(root, lock); rc != SQLITE_OK {
		return rc
	}
	holders := bt.tableLocks[root]
	if holders == nil {
		holders = map[*minweightBtree]uint8{}
		bt.tableLocks[root] = holders
	}
	if holders[bt] < lock {
		holders[bt] = lock
	}
	return SQLITE_OK
}

func (bt *minweightBtree) schemaLocked() int32 {
	if !bt.sharable {
		return SQLITE_OK
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return bt.queryTableLockLocked(uint32(SCHEMA_ROOT), uint8(READ_LOCK))
}

func (bt *minweightBtree) clearTableLocksLocked() {
	for root, holders := range bt.tableLocks {
		delete(holders, bt)
		if len(holders) == 0 {
			delete(bt.tableLocks, root)
		}
	}
}

func (bt *minweightBtree) hasOpenTransaction() bool {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if bt.writer != nil || len(bt.readers) != 0 {
		return true
	}
	for _, holders := range bt.tableLocks {
		if len(holders) != 0 {
			return true
		}
	}
	return false
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
	bt.clearTableLocksLocked()
	bt.txnState = SQLITE_TXN_NONE
	bt.txSnapshot = nil
	bt.savepoints = nil
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

func (e *minweightStorageEngine) invalidateIncrblobCursors(bt *minweightBtree, root uint32, rowid int64, clearTable bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, cur := range e.cursors {
		if !cur.incrblob || cur.incrblobInvalid || !cur.intKey {
			continue
		}
		if cur.btree.minweightDatabase != bt.minweightDatabase || cur.root != root {
			continue
		}
		if !clearTable {
			row, ok := cur.current()
			if !ok || row.rowid != rowid {
				continue
			}
		}
		cur.valid = false
		cur.hasLastRow = false
		cur.incrblobInvalid = true
	}
}

func (e *minweightStorageEngine) tripCursors(ctx BtreeContext, bt *minweightBtree, errCode int32, writeOnly int32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for ptr, cur := range e.cursors {
		if cur.btree != bt {
			continue
		}
		if writeOnly != 0 && !cur.writable {
			continue
		}
		cur.closePayload(ctx)
		cur.valid = false
		cur.hasLastRow = false
		cur.incrblobInvalid = false
		cur.faultCode = errCode
		raw := (*BtCursor)(unsafe.Pointer(ptr))
		raw.FeState = uint8(CURSOR_FAULT)
		raw.FskipNext = errCode
	}
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
	if pCur.IsNil() {
		return 0
	}
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return 1
	}
	if cur.incrblobInvalid {
		return 1
	}
	if cur.valid && cur.dataVer != cur.btree.dataVer {
		return 1
	}
	return 0
}

func (e *minweightStorageEngine) BtreeFakeValidCursor(ctx BtreeContext) (r BtreeCursorHandle) {
	return BtreeCursorHandle{}
}

func (e *minweightStorageEngine) BtreeCursorRestore(ctx BtreeContext, pCur BtreeCursorHandle, pDifferentRow BtreeMemoryHandle) (r int32) {
	if pCur.IsNil() {
		minweightWriteResult(pDifferentRow, 0)
		return SQLITE_OK
	}
	cur := e.cursor(pCur)
	differentRow := int32(0)
	if cur.faultCode != SQLITE_OK {
		minweightWriteResult(pDifferentRow, 1)
		return cur.faultCode
	}
	if cur.incrblobInvalid {
		minweightWriteResult(pDifferentRow, 1)
		return SQLITE_OK
	}
	if cur.valid && cur.dataVer != cur.btree.dataVer {
		row, ok := cur.current()
		if !ok {
			cur.valid = false
			differentRow = 1
		} else {
			if rc := e.refreshCursorRows(ctx, pCur, cur); rc != SQLITE_OK {
				return rc
			}
			if i := minweightFindRow(cur.rows, row, cur.intKey); i >= 0 {
				cur.index = i
				cur.valid = true
				cur.hasLastRow = false
			} else {
				cur.index = minweightFindRowAtOrAfter(cur.rows, row, cur.intKey)
				cur.valid = cur.index < len(cur.rows)
				cur.lastRow = row
				cur.hasLastRow = true
				differentRow = 1
			}
		}
	}
	minweightWriteResult(pDifferentRow, differentRow)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCursorHintFlags(ctx BtreeContext, pCur BtreeCursorHandle, x uint32) {
	(*BtCursor)(unsafe.Pointer(pCur.ptr)).Fhints = uint8(x)
}

func (e *minweightStorageEngine) BtreeLastPage(ctx BtreeContext, p BtreeHandle) (r uint32) {
	return e.btree(p).next
}

func (e *minweightStorageEngine) BtreeOpen(ctx BtreeContext, pVfs BtreeVFSHandle, zFilename BtreeCStringHandle, db SQLiteHandle, ppBtree BtreeMemoryHandle, flags int32, vfsFlags int32) (r int32) {
	key := minweightDatabaseKey(zFilename)
	readOnly := vfsFlags&SQLITE_OPEN_READONLY != 0
	sharable := vfsFlags&SQLITE_OPEN_SHAREDCACHE != 0
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
	file := _sqlite3MallocZero(ctx.tls, uint64(unsafe.Sizeof(minweightFile{})))
	journal := _sqlite3MallocZero(ctx.tls, uint64(unsafe.Sizeof(Tsqlite3_file{})))
	(*Tsqlite3_file)(unsafe.Pointer(file)).FpMethods = uintptr(unsafe.Pointer(&minweightFileMethods))
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
		sharable:          sharable,
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
	(*minweightFile)(unsafe.Pointer(file)).btreeToken = token
	if db.ptr != 0 && e.aliases[db.ptr] == 0 {
		e.aliases[db.ptr] = token
	}
	e.mu.Unlock()
	bt.retainConnection()
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
		bt.releaseConnection()
		bt.releaseTrans()
	}
	return SQLITE_OK
}

func (e *minweightStorageEngine) FileControlPersistWAL(ctx BtreeContext, db SQLiteHandle, dbName string, mode int32) (int32, int32) {
	if dbName != "" && dbName != "main" {
		return mode, SQLITE_ERROR
	}
	bt := e.btreeForDB(db)
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

func (e *minweightStorageEngine) BeginLogicalBackup(ctx BtreeContext, db SQLiteHandle) int32 {
	bt := e.btreeForDB(db)
	if bt == nil {
		return SQLITE_ERROR
	}
	bt.mu.Lock()
	bt.backupCount++
	bt.mu.Unlock()
	return SQLITE_OK
}

func (e *minweightStorageEngine) FinishLogicalBackup(ctx BtreeContext, db SQLiteHandle) int32 {
	bt := e.btreeForDB(db)
	if bt == nil {
		return SQLITE_ERROR
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if bt.backupCount == 0 {
		return SQLITE_ERROR
	}
	bt.backupCount--
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeSetCacheSize(ctx BtreeContext, p BtreeHandle, mxPage int32) (r int32) {
	bt := e.btree(p)
	bt.mu.Lock()
	bt.cacheSize = mxPage
	bt.mu.Unlock()
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeSetSpillSize(ctx BtreeContext, p BtreeHandle, mxPage int32) (r int32) {
	bt := e.btree(p)
	bt.mu.Lock()
	if mxPage != 0 {
		bt.spillSize = mxPage
	}
	spillSize := minweightEffectiveSpillSize(bt.pageSize, bt.cacheSize, bt.spillSize)
	bt.mu.Unlock()
	return spillSize
}
func (e *minweightStorageEngine) BtreeSetPagerFlags(ctx BtreeContext, p BtreeHandle, pgFlags uint32) (r int32) {
	bt := e.btree(p)
	_sqlite3PagerSetFlags(ctx.tls, bt.pager, pgFlags)
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeSetPageSize(ctx BtreeContext, p BtreeHandle, pageSize int32, nReserve int32, iFix int32) (r int32) {
	bt := e.btree(p)
	if rc := bt.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if nReserve < 0 {
		nReserve = bt.reserve
	} else {
		bt.reserveWanted = nReserve
	}
	if bt.reserve == nReserve && (pageSize == 0 || pageSize == bt.pageSize) {
		return SQLITE_OK
	}
	if nReserve < bt.reserve {
		nReserve = bt.reserve
	}
	if bt.pageSizeFixed {
		return SQLITE_READONLY
	}
	if minweightValidPageSize(pageSize) {
		if nReserve > 32 && pageSize == 512 {
			pageSize = 1024
		}
		bt.pageSize = pageSize
	}
	bt.reserve = nReserve
	if iFix != 0 {
		bt.pageSizeFixed = true
	}
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeGetPageSize(ctx BtreeContext, p BtreeHandle) (r int32) {
	bt := e.btree(p)
	bt.mu.Lock()
	pageSize := bt.pageSize
	bt.mu.Unlock()
	return pageSize
}
func (e *minweightStorageEngine) BtreeGetReserveNoMutex(ctx BtreeContext, p BtreeHandle) (r int32) {
	bt := e.btree(p)
	bt.mu.Lock()
	reserve := bt.reserve
	bt.mu.Unlock()
	return reserve
}
func (e *minweightStorageEngine) BtreeGetRequestedReserve(ctx BtreeContext, p BtreeHandle) (r int32) {
	bt := e.btree(p)
	bt.mu.Lock()
	reserve := bt.reserve
	if bt.reserveWanted > reserve {
		reserve = bt.reserveWanted
	}
	bt.mu.Unlock()
	return reserve
}
func (e *minweightStorageEngine) BtreeMaxPageCount(ctx BtreeContext, p BtreeHandle, mxPage uint32) (r uint32) {
	bt := e.btree(p)
	bt.mu.Lock()
	if mxPage > 0 {
		bt.maxPageCount = mxPage
	}
	maxPageCount := bt.maxPageCount
	bt.mu.Unlock()
	return maxPageCount
}
func (e *minweightStorageEngine) BtreeSecureDelete(ctx BtreeContext, p BtreeHandle, newFlag int32) (r int32) {
	if p.IsNil() {
		return 0
	}
	bt := e.btree(p)
	bt.mu.Lock()
	if newFlag >= 0 {
		bt.secureDelete = newFlag
	}
	secureDelete := bt.secureDelete
	bt.mu.Unlock()
	return secureDelete
}
func (e *minweightStorageEngine) BtreeSetAutoVacuum(ctx BtreeContext, p BtreeHandle, autoVacuum int32) (r int32) {
	bt := e.btree(p)
	if rc := bt.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if bt.pageSizeFixed && (autoVacuum != 0) != (bt.autoVacuum != BTREE_AUTOVACUUM_NONE) {
		return SQLITE_READONLY
	}
	if autoVacuum == BTREE_AUTOVACUUM_INCR {
		bt.autoVacuum = BTREE_AUTOVACUUM_INCR
	} else if autoVacuum != 0 {
		bt.autoVacuum = BTREE_AUTOVACUUM_FULL
	} else {
		bt.autoVacuum = BTREE_AUTOVACUUM_NONE
	}
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeGetAutoVacuum(ctx BtreeContext, p BtreeHandle) (r int32) {
	bt := e.btree(p)
	bt.mu.Lock()
	autoVacuum := bt.autoVacuum
	bt.mu.Unlock()
	return autoVacuum
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
	bt.freeRoots = nil
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
	}
	if rc := bt.lockTable(uint32(SCHEMA_ROOT), uint8(READ_LOCK)); rc != SQLITE_OK {
		return rc
	}
	if wrflag != 0 {
		if err := bt.ensureTransactionSnapshot(); err != nil {
			return minweightSQLiteError(err)
		}
		if err := bt.ensureSavepoints(bt.sqliteSavepointCount()); err != nil {
			return minweightSQLiteError(err)
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
	if pBtree.IsNil() {
		return SQLITE_OK
	}
	e.tripCursors(ctx, e.btree(pBtree), errCode, writeOnly)
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeRollback(ctx BtreeContext, p BtreeHandle, tripCode int32, writeOnly int32) (r int32) {
	if p.IsNil() {
		return SQLITE_OK
	}
	bt := e.btree(p)
	bt.mu.Lock()
	s := bt.txSnapshot
	bt.mu.Unlock()
	if s != nil {
		if err := bt.restoreSnapshot(*s); err != nil {
			return minweightSQLiteError(err)
		}
	}
	if tripCode != SQLITE_OK {
		e.tripCursors(ctx, bt, tripCode, writeOnly)
	}
	bt.releaseTrans()
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeBeginStmt(ctx BtreeContext, p BtreeHandle, iStatement int32) (r int32) {
	if p.IsNil() {
		return SQLITE_OK
	}
	bt := e.btree(p)
	if err := bt.ensureTransactionSnapshot(); err != nil {
		return minweightSQLiteError(err)
	}
	if err := bt.ensureSavepoints(iStatement); err != nil {
		return minweightSQLiteError(err)
	}
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeSavepoint(ctx BtreeContext, p BtreeHandle, op int32, iSavepoint int32) (r int32) {
	if p.IsNil() {
		return SQLITE_OK
	}
	bt := e.btree(p)
	if op != SAVEPOINT_ROLLBACK && op != SAVEPOINT_RELEASE {
		return SQLITE_OK
	}
	if iSavepoint < 0 {
		if op == SAVEPOINT_ROLLBACK {
			bt.mu.Lock()
			s := bt.txSnapshot
			bt.savepoints = nil
			bt.mu.Unlock()
			if s != nil {
				if err := bt.restoreSnapshot(*s); err != nil {
					return minweightSQLiteError(err)
				}
			}
		}
		return SQLITE_OK
	}
	bt.mu.Lock()
	savepoint := int(iSavepoint)
	if savepoint >= len(bt.savepoints) {
		bt.mu.Unlock()
		return SQLITE_OK
	}
	s := bt.savepoints[savepoint]
	if op == SAVEPOINT_RELEASE {
		bt.savepoints = bt.savepoints[:savepoint]
		bt.mu.Unlock()
		return SQLITE_OK
	}
	bt.savepoints = bt.savepoints[:savepoint+1]
	bt.mu.Unlock()
	if err := bt.restoreSnapshot(s); err != nil {
		return minweightSQLiteError(err)
	}
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
	if wrFlag != 0 {
		(*BtCursor)(unsafe.Pointer(pCur.ptr)).FcurFlags |= uint8(BTCF_WriteFlag)
	}
	cur := &minweightCursor{
		btree:    bt,
		root:     iTable,
		intKey:   table.intKey,
		writable: wrFlag != 0,
		index:    -1,
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
	if cur.faultCode == SQLITE_OK && cur.valid {
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

func (e *minweightStorageEngine) BtreeCursorPin(ctx BtreeContext, pCur BtreeCursorHandle) {
	(*BtCursor)(unsafe.Pointer(pCur.ptr)).FcurFlags |= uint8(BTCF_Pinned)
}
func (e *minweightStorageEngine) BtreeCursorUnpin(ctx BtreeContext, pCur BtreeCursorHandle) {
	(*BtCursor)(unsafe.Pointer(pCur.ptr)).FcurFlags &^= uint8(BTCF_Pinned)
}
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
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	row, ok := cur.current()
	if !ok {
		return SQLITE_ERROR
	}
	data := row.payload
	if !cur.intKey {
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
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	if cur.incrblobInvalid {
		return SQLITE_ABORT
	}
	return e.BtreePayload(ctx, pCur, offset, amt, pBuf)
}

func (e *minweightStorageEngine) BtreePayloadFetch(ctx BtreeContext, pCur BtreeCursorHandle, pAmt BtreeMemoryHandle) (r BtreeMemoryHandle) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		pAmt.PutInt32(0)
		return BtreeMemoryHandle{}
	}
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
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
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
	cur.index = 0
	cur.hasLastRow = false
	cur.incrblobInvalid = false
	minweightWriteResult(pRes, 0)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeLast(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
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
		cur.incrblobInvalid = false
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
	cur.incrblobInvalid = false
	minweightWriteResult(pRes, 0)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeTableMoveto(ctx BtreeContext, pCur BtreeCursorHandle, intKey int64, biasRight int32, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
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
		cur.incrblobInvalid = false
		return SQLITE_OK
	}
	cur.valid = false
	cur.index = len(rows)
	minweightWriteResult(pRes, -1)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeIndexMoveto(ctx BtreeContext, pCur BtreeCursorHandle, pIdxKey BtreeIndexKeyHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
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
			cur.incrblobInvalid = false
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
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
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
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
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
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	var key []byte
	var payload []byte
	var rowid int64
	if flags&int32(BTREE_PREFORMAT) != 0 {
		if !cur.hasTransferRow {
			return SQLITE_CORRUPT
		}
		row := cur.transferRow
		cur.transferRow = minweightRow{}
		cur.hasTransferRow = false
		if cur.intKey {
			rowid = row.rowid
			payload = append([]byte(nil), row.payload...)
			key = minweightTableKey(cur.root, rowid)
		} else {
			payload = append([]byte(nil), row.key...)
			key = minweightIndexKey(cur.root, payload)
		}
	} else if cur.intKey {
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
	if cur.intKey {
		e.invalidateIncrblobCursors(cur.btree, cur.root, rowid, false)
	}
	cur.btree.noteInsert(cur.root, rowid, existed)
	cur.btree.dataVer++
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeTransferRow(ctx BtreeContext, pDest BtreeCursorHandle, pSrc BtreeCursorHandle, iKey int64) (r int32) {
	dest := e.cursor(pDest)
	if rc := dest.btree.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	if dest.faultCode != SQLITE_OK {
		return dest.faultCode
	}
	src := e.cursor(pSrc)
	if src.faultCode != SQLITE_OK {
		return src.faultCode
	}
	if dest.intKey != src.intKey {
		return SQLITE_CORRUPT
	}
	row, ok := src.current()
	if !ok {
		return SQLITE_ERROR
	}
	if dest.intKey {
		row.rowid = iKey
	}
	dest.transferRow = minweightRow{
		rowid:   row.rowid,
		key:     append([]byte(nil), row.key...),
		payload: append([]byte(nil), row.payload...),
	}
	dest.hasTransferRow = true
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeDelete(ctx BtreeContext, pCur BtreeCursorHandle, flags uint8) (r int32) {
	cur := e.cursor(pCur)
	if rc := cur.btree.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
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
	if cur.intKey {
		e.invalidateIncrblobCursors(cur.btree, cur.root, row.rowid, false)
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
	var root uint32
	if bt.autoVacuum == BTREE_AUTOVACUUM_NONE && len(bt.freeRoots) != 0 {
		last := len(bt.freeRoots) - 1
		root = bt.freeRoots[last]
		bt.freeRoots = bt.freeRoots[:last]
	} else {
		bt.next++
		root = bt.next
	}
	bt.tables[root] = minweightTable{intKey: flags&int32(BTREE_INTKEY) != 0}
	if root > bt.meta[BTREE_LARGEST_ROOT_PAGE] {
		bt.meta[BTREE_LARGEST_ROOT_PAGE] = root
	}
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
	if table.intKey {
		e.invalidateIncrblobCursors(bt, uint32(iTable), 0, true)
	}
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
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	if cur.intKey {
		e.invalidateIncrblobCursors(cur.btree, cur.root, 0, true)
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
	root := uint32(iTable)
	bt.mu.Lock()
	table := bt.tables[root]
	autoVacuum := bt.autoVacuum
	lastRoot := bt.next
	movedTable := bt.tables[lastRoot]
	bt.mu.Unlock()
	if table.intKey {
		e.invalidateIncrblobCursors(bt, root, 0, true)
	}
	if _, err := bt.clearRoot(root, table.intKey); err != nil {
		return minweightSQLiteError(err)
	}
	moved := uint32(0)
	if autoVacuum != BTREE_AUTOVACUUM_NONE && root < lastRoot {
		if err := bt.moveRoot(lastRoot, root, movedTable.intKey); err != nil {
			return minweightSQLiteError(err)
		}
		moved = lastRoot
	}
	bt.mu.Lock()
	delete(bt.tables, root)
	if autoVacuum != BTREE_AUTOVACUUM_NONE {
		delete(bt.tables, lastRoot)
		if moved != 0 {
			bt.tables[root] = movedTable
		}
		if lastRoot > 1 {
			bt.next = lastRoot - 1
		} else {
			bt.next = 1
		}
		bt.meta[BTREE_LARGEST_ROOT_PAGE] = bt.next
	} else {
		bt.freeRoots = append(bt.freeRoots, root)
	}
	bt.mu.Unlock()
	if !piMoved.IsNil() {
		piMoved.PutInt32(int32(moved))
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
	if idx == int32(BTREE_INCR_VACUUM) && bt.autoVacuum != BTREE_AUTOVACUUM_NONE {
		if iMeta != 0 {
			bt.autoVacuum = BTREE_AUTOVACUUM_INCR
		} else {
			bt.autoVacuum = BTREE_AUTOVACUUM_FULL
		}
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
	if !p.IsNil() && e.btree(p).hasOpenTransaction() {
		return SQLITE_LOCKED
	}
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeIsInBackup(ctx BtreeContext, p BtreeHandle) (r int32) {
	if p.IsNil() {
		return 0
	}
	bt := e.btree(p)
	bt.mu.Lock()
	active := bt.backupCount != 0
	bt.mu.Unlock()
	return libc.BoolInt32(active)
}

func (e *minweightStorageEngine) BtreeSchema(ctx BtreeContext, p BtreeHandle, nBytes int32, __ccgo_fp_xFree BtreeFunctionHandle) (r BtreeSchemaHandle) {
	bt := e.btree(p)
	if bt.schema == 0 && nBytes != 0 {
		bt.schema = _sqlite3DbMallocZero(ctx.tls, uintptr(0), uint64(nBytes))
	}
	return btreeSchemaHandle(ctx.tls, bt.schema)
}

func (e *minweightStorageEngine) BtreeSchemaLocked(ctx BtreeContext, p BtreeHandle) (r int32) {
	return e.btree(p).schemaLocked()
}
func (e *minweightStorageEngine) BtreeLockTable(ctx BtreeContext, p BtreeHandle, iTab int32, isWriteLock uint8) (r int32) {
	return e.btree(p).lockTable(uint32(iTab), uint8(READ_LOCK)+isWriteLock)
}
func (e *minweightStorageEngine) BtreePutData(ctx BtreeContext, pCsr BtreeCursorHandle, offset uint32, amt uint32, z BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCsr)
	if rc := cur.btree.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	if cur.incrblobInvalid {
		return SQLITE_ABORT
	}
	row, ok := cur.current()
	if !ok || !cur.intKey {
		return SQLITE_ERROR
	}
	if !cur.writable {
		return SQLITE_READONLY
	}
	payload := append([]byte(nil), row.payload...)
	end := uint64(offset) + uint64(amt)
	if end > uint64(len(payload)) {
		return SQLITE_CORRUPT
	}
	copy(payload[int(offset):int(end)], z.ReadBytes(int(amt)))
	if err := cur.btree.store.Put(minweightTableKey(cur.root, row.rowid), payload); err != nil {
		return minweightSQLiteError(err)
	}
	cur.btree.dataVer++
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeIncrblobCursor(ctx BtreeContext, pCur BtreeCursorHandle) {
	cur := e.cursor(pCur)
	cur.incrblob = true
	(*BtCursor)(unsafe.Pointer(pCur.ptr)).FcurFlags |= uint8(BTCF_Incrblob)
}
func (e *minweightStorageEngine) BtreeSetVersion(ctx BtreeContext, pBtree BtreeHandle, iVersion int32) (r int32) {
	bt := e.btree(pBtree)
	if rc := bt.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	bt.meta[BTREE_FILE_FORMAT] = uint32(iVersion)
	bt.dataVer++
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeCursorHasHint(ctx BtreeContext, pCsr BtreeCursorHandle, mask uint32) (r int32) {
	if uint32((*BtCursor)(unsafe.Pointer(pCsr.ptr)).Fhints)&mask != 0 {
		return 1
	}
	return 0
}
func (e *minweightStorageEngine) BtreeIsReadonly(ctx BtreeContext, p BtreeHandle) (r int32) {
	if e.btree(p).readOnly {
		return 1
	}
	return 0
}
func (e *minweightStorageEngine) BtreeSharable(ctx BtreeContext, p BtreeHandle) (r int32) {
	if e.btree(p).sharable {
		return 1
	}
	return 0
}
func (e *minweightStorageEngine) BtreeConnectionCount(ctx BtreeContext, p BtreeHandle) (r int32) {
	return e.btree(p).connectionCount()
}
func (e *minweightStorageEngine) BtreeCopyFile(ctx BtreeContext, pTo BtreeHandle, pFrom BtreeHandle) (r int32) {
	to := e.btree(pTo)
	if rc := to.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	from := e.btree(pFrom)
	s, err := from.snapshot()
	if err != nil {
		return minweightSQLiteError(err)
	}
	if err := to.restoreSnapshot(*s); err != nil {
		return minweightSQLiteError(err)
	}
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeSetMmapLimit(ctx BtreeContext, p BtreeHandle, szMmap int64) (r int32) {
	bt := e.btree(p)
	bt.mu.Lock()
	mmapSize := minweightClampMmapLimit(Tsqlite3_int64(szMmap))
	bt.mmapSize = int64(mmapSize)
	(*Pager)(unsafe.Pointer(bt.pager)).FszMmap = mmapSize
	bt.mu.Unlock()
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
	if mxErr <= 0 {
		return SQLITE_OK
	}
	bt := e.btree(p)
	snapshot, err := bt.snapshot()
	if err != nil {
		return minweightSQLiteError(err)
	}
	roots, partial := minweightIntegrityRoots(aRoot, nRoot)
	selected := minweightIntegritySelectedRoots(roots, partial)
	stats := make(map[uint32]minweightIntegrityStats, len(snapshot.tables))
	var errors []string
	for _, item := range snapshot.items {
		if len(item.key) == 0 {
			if !partial && !minweightAddIntegrityError(&errors, mxErr, "minweight malformed empty key") {
				break
			}
			continue
		}
		switch item.key[0] {
		case minweightTablePrefix:
			if len(item.key) != 13 {
				if !partial && !minweightAddIntegrityError(&errors, mxErr, "minweight malformed table key length %d", len(item.key)) {
					break
				}
				continue
			}
			root := binary.BigEndian.Uint32(item.key[1:5])
			if !minweightIntegrityRootChecked(root, partial, selected) {
				continue
			}
			table, ok := snapshot.tables[root]
			if !ok {
				if !minweightAddIntegrityError(&errors, mxErr, "minweight table key references unknown root %d", root) {
					break
				}
				continue
			}
			if !table.intKey {
				if !minweightAddIntegrityError(&errors, mxErr, "minweight root %d has table key in index btree", root) {
					break
				}
				continue
			}
			u := binary.BigEndian.Uint64(item.key[5:13]) ^ (1 << 63)
			minweightAddIntegrityRowid(stats, root, int64(u))
		case minweightIndexPrefix:
			if len(item.key) < 5 {
				if !partial && !minweightAddIntegrityError(&errors, mxErr, "minweight malformed index key length %d", len(item.key)) {
					break
				}
				continue
			}
			root := binary.BigEndian.Uint32(item.key[1:5])
			if !minweightIntegrityRootChecked(root, partial, selected) {
				continue
			}
			table, ok := snapshot.tables[root]
			if !ok {
				if !minweightAddIntegrityError(&errors, mxErr, "minweight index key references unknown root %d", root) {
					break
				}
				continue
			}
			if table.intKey {
				if !minweightAddIntegrityError(&errors, mxErr, "minweight root %d has index key in table btree", root) {
					break
				}
				continue
			}
			minweightAddIntegrityIndexRow(stats, root)
		default:
			if !partial && !minweightAddIntegrityError(&errors, mxErr, "minweight unknown key prefix 0x%02x", item.key[0]) {
				break
			}
		}
		if int32(len(errors)) >= mxErr {
			break
		}
	}
	for root, table := range snapshot.tables {
		if int32(len(errors)) >= mxErr {
			break
		}
		if !minweightIntegrityRootChecked(root, partial, selected) {
			continue
		}
		stat := stats[root]
		if root == 0 {
			if !minweightAddIntegrityError(&errors, mxErr, "minweight metadata contains root 0") {
				break
			}
		}
		if root > snapshot.next {
			if !minweightAddIntegrityError(&errors, mxErr, "minweight root %d is greater than largest root %d", root, snapshot.next) {
				break
			}
		}
		if table.rowCount < 0 {
			if !minweightAddIntegrityError(&errors, mxErr, "minweight root %d has negative row count %d", root, table.rowCount) {
				break
			}
		}
		if table.rowCount != stat.rowCount {
			if !minweightAddIntegrityError(&errors, mxErr, "minweight root %d row count metadata %d != actual %d", root, table.rowCount, stat.rowCount) {
				break
			}
		}
		if !table.intKey {
			continue
		}
		if stat.rowCount == 0 {
			if table.minRowid != 0 || table.maxRowid != 0 {
				if !minweightAddIntegrityError(&errors, mxErr, "minweight root %d empty table has rowid bounds %d..%d", root, table.minRowid, table.maxRowid) {
					break
				}
			}
			continue
		}
		if table.minRowid != stat.minRowid || table.maxRowid != stat.maxRowid {
			if !minweightAddIntegrityError(&errors, mxErr, "minweight root %d rowid bounds metadata %d..%d != actual %d..%d", root, table.minRowid, table.maxRowid, stat.minRowid, stat.maxRowid) {
				break
			}
		}
	}
	if !aCnt.IsNil() {
		for i, root := range roots {
			var rowCount int64
			if root != 0 {
				rowCount = stats[root].rowCount
			}
			_sqlite3MemSetArrayInt64(ctx.tls, aCnt.ptr, int32(i), Ti64(rowCount))
		}
	}
	if len(errors) == 0 {
		return SQLITE_OK
	}
	if !pnErr.IsNil() {
		pnErr.PutInt32(int32(len(errors)))
	}
	if !pzOut.IsNil() {
		out := minweightAllocCString(ctx, strings.Join(errors, "\n"))
		if out == 0 {
			return SQLITE_NOMEM
		}
		pzOut.PutUintptr(out)
	}
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeClearCache(ctx BtreeContext, p BtreeHandle) {}

var _ StorageEngine = (*minweightStorageEngine)(nil)
var _ StorageEngineBtreeSetMmapLimit = (*minweightStorageEngine)(nil)
var _ StorageEngineBtreeIsEmpty = (*minweightStorageEngine)(nil)
var _ StorageEngineBtreeIntegrityCheck = (*minweightStorageEngine)(nil)
var _ StorageEngineBtreeClearCache = (*minweightStorageEngine)(nil)
