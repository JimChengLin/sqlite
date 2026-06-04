// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"os"
	"path/filepath"
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
	minweightMetaPrefix  byte = 'm'

	minweightMetaVersion uint32 = 1

	minweightIndexKeyVersion byte = 0
)

var minweightMetaKey = []byte{minweightMetaPrefix}

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
	path          string
	refs          int
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
	readers       map[*minweightBtree]int
	readerViews   map[*minweightBtree]uint64
	pinnedViews   map[uint64]int
	writer        *minweightBtree
	sharedRefs    int32
	tableLocks    map[uint32]map[*minweightBtree]uint8
	generation    uint64
	changes       []minweightCommitChange
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
	txn         *minweightTxn
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

var errMinweightTxnConflict = errors.New("minweight transaction read set conflict")

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
	readTracked     bool
	payloadBuf      uintptr
	transferRow     minweightRow
	hasTransferRow  bool
}

type minweightRow struct {
	rowid    int64
	key      []byte
	storeKey []byte
	payload  []byte
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
	cacheSize     int32
	spillSize     int32
	freeRoots     []uint32
}

type minweightSnapshotItem struct {
	key   []byte
	value []byte
}

type minweightDBState struct {
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
}

type minweightTxn struct {
	baseGeneration uint64
	state          minweightDBState
	reads          map[string]struct{}
	readRoots      map[uint32]struct{}
	readMeta       bool
	writes         map[string]minweightTxnWrite
	savepoints     []minweightTxnSavepoint
}

type minweightTxnWrite struct {
	key     []byte
	value   []byte
	deleted bool
}

type minweightTxnSavepoint struct {
	state  minweightDBState
	writes map[string]minweightTxnWrite
}

type minweightCommitChange struct {
	generation  uint64
	keys        map[string]minweightCommittedKeyChange
	roots       map[uint32]struct{}
	meta        bool
	beforeState minweightDBState
	afterState  minweightDBState
}

type minweightCommittedKeyChange struct {
	key         []byte
	before      []byte
	beforeExist bool
	after       []byte
	afterExists bool
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

func minweightFileFromPointer(pFile uintptr) *minweightFile {
	return (*minweightFile)(unsafe.Pointer(pFile)) //nolint:govet // pFile is a SQLite ABI pointer to minweightFile.
}

func minweightInt64FromPointer(pArg uintptr) *Tsqlite3_int64 {
	return (*Tsqlite3_int64)(unsafe.Pointer(pArg)) //nolint:govet // pArg is a SQLite ABI pointer to a sqlite3_int64.
}

func minweightPagerFromPointer(pPager uintptr) *Pager {
	return (*Pager)(unsafe.Pointer(pPager)) //nolint:govet // pPager is a SQLite ABI pointer to this adapter's fake Pager.
}

func minweightByteSliceFromPointer(p uintptr, n int) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(p)), n) //nolint:govet // p is a SQLite allocation owned by this adapter.
}

func minweightUint8FromPointer(p uintptr) uint8 {
	return *(*uint8)(unsafe.Pointer(p)) //nolint:govet // p is a SQLite ABI pointer to an uint8 slot.
}

func minweightUint8PointerFromPointer(p uintptr) *uint8 {
	return (*uint8)(unsafe.Pointer(p)) //nolint:govet // p is a SQLite ABI pointer to an uint8 slot.
}

func minweightUintptrFromPointer(p uintptr) uintptr {
	return *(*uintptr)(unsafe.Pointer(p)) //nolint:govet // p is a SQLite ABI pointer to a uintptr slot.
}

func minweightUintptrPointerFromPointer(p uintptr) *uintptr {
	return (*uintptr)(unsafe.Pointer(p)) //nolint:govet // p is a SQLite ABI pointer to a uintptr slot.
}

func minweightUnpackedRecordFromPointer(pIdxKey uintptr) *TUnpackedRecord {
	return (*TUnpackedRecord)(unsafe.Pointer(pIdxKey)) //nolint:govet // pIdxKey is a SQLite ABI pointer to UnpackedRecord.
}

func minweightMemFromPointer(pMem uintptr) *TMem {
	return (*TMem)(unsafe.Pointer(pMem)) //nolint:govet // pMem is a SQLite ABI pointer to Mem.
}

func minweightKeyInfoFromPointer(keyInfo uintptr) *TKeyInfo {
	return (*TKeyInfo)(unsafe.Pointer(keyInfo)) //nolint:govet // keyInfo is a SQLite ABI pointer to KeyInfo.
}

func minweightCollSeqFromPointer(collation uintptr) *TCollSeq {
	return (*TCollSeq)(unsafe.Pointer(collation)) //nolint:govet // collation is a SQLite ABI pointer to CollSeq.
}

func minweightSQLiteFromPointer(db uintptr) *Tsqlite3 {
	return (*Tsqlite3)(unsafe.Pointer(db)) //nolint:govet // db is a SQLite ABI pointer to sqlite3.
}

func minweightBtCursorFromPointer(pCur uintptr) *BtCursor {
	return (*BtCursor)(unsafe.Pointer(pCur)) //nolint:govet // pCur is a SQLite ABI pointer to BtCursor.
}

func minweightSQLiteFileFromPointer(file uintptr) *Tsqlite3_file {
	return (*Tsqlite3_file)(unsafe.Pointer(file)) //nolint:govet // file is a SQLite ABI pointer to sqlite3_file.
}

func minweightMemInt64(pMem uintptr) int64 {
	return *(*int64)(unsafe.Pointer(pMem)) //nolint:govet // SQLite Mem stores integer payload in the leading union field.
}

func minweightMemFloat64(pMem uintptr) float64 {
	return *(*float64)(unsafe.Pointer(pMem)) //nolint:govet // SQLite Mem stores real payload in the leading union field.
}

func minweightMemZeroLength(pMem uintptr) int32 {
	return *(*int32)(unsafe.Pointer(pMem)) //nolint:govet // SQLite Mem stores zeroblob length in the leading union field.
}

func minweightBtreeForFile(pFile uintptr) *minweightBtree {
	file := minweightFileFromPointer(pFile)
	engine, ok := storageEngineForBtreeToken(file.btreeToken).(*minweightStorageEngine)
	if !ok {
		return nil
	}
	return engine.btreeForToken(file.btreeToken)
}

func minweightSQLiteError(err error) int32 {
	if err == nil {
		return SQLITE_OK
	}
	if errors.Is(err, errMinweightTxnConflict) {
		return SQLITE_BUSY
	}
	if errors.Is(err, minweight.ErrLocked) {
		return SQLITE_BUSY
	}
	if errors.Is(err, minweight.ErrCorruptWAL) ||
		errors.Is(err, minweight.ErrCorruptIndex) ||
		errors.Is(err, minweight.ErrManifest) ||
		errors.Is(err, minweight.ErrParquet) {
		return SQLITE_CORRUPT
	}
	if errors.Is(err, minweight.ErrWalFull) {
		return SQLITE_FULL
	}
	if os.IsPermission(err) {
		return SQLITE_PERM
	}
	return SQLITE_ERROR
}

func minweightOpenError(err error) int32 {
	rc := minweightSQLiteError(err)
	if rc != SQLITE_ERROR {
		return rc
	}
	return SQLITE_CANTOPEN
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

func minweightAppendUint32(buf []byte, v uint32) []byte {
	var scratch [4]byte
	binary.BigEndian.PutUint32(scratch[:], v)
	return append(buf, scratch[:]...)
}

func minweightReadUint32(buf []byte, off *int) (uint32, bool) {
	if *off+4 > len(buf) {
		return 0, false
	}
	v := binary.BigEndian.Uint32(buf[*off : *off+4])
	*off += 4
	return v, true
}

func minweightReadBool(buf []byte, off *int) (bool, bool) {
	if *off >= len(buf) {
		return false, false
	}
	v := buf[*off] != 0
	*off += 1
	return v, true
}

func minweightCorruptMetadata(format string, args ...any) error {
	return fmt.Errorf("%w: minweight sqlite metadata: "+format, append([]any{minweight.ErrManifest}, args...)...)
}

func minweightEncodeDatabaseState(state minweightDBState) []byte {
	buf := make([]byte, 0, 128+len(state.tables)*17+len(state.freeRoots)*4)
	buf = minweightAppendUint32(buf, minweightMetaVersion)
	buf = minweightAppendUint32(buf, state.next)
	buf = minweightAppendUint32(buf, state.dataVer)
	buf = minweightAppendUint32(buf, uint32(state.pageSize))
	buf = minweightAppendUint32(buf, uint32(state.reserve))
	buf = minweightAppendUint32(buf, uint32(state.reserveWanted))
	if state.pageSizeFixed {
		buf = append(buf, 1)
	} else {
		buf = append(buf, 0)
	}
	buf = minweightAppendUint32(buf, state.maxPageCount)
	buf = minweightAppendUint32(buf, uint32(state.secureDelete))
	buf = minweightAppendUint32(buf, uint32(state.autoVacuum))
	buf = minweightAppendUint32(buf, uint32(state.cacheSize))
	buf = minweightAppendUint32(buf, uint32(state.spillSize))
	for _, v := range state.meta {
		buf = minweightAppendUint32(buf, v)
	}
	roots := make([]int, 0, len(state.tables))
	for root := range state.tables {
		roots = append(roots, int(root))
	}
	sort.Ints(roots)
	buf = minweightAppendUint32(buf, uint32(len(roots)))
	for _, root := range roots {
		table := state.tables[uint32(root)]
		buf = minweightAppendUint32(buf, uint32(root))
		if table.intKey {
			buf = append(buf, 1)
		} else {
			buf = append(buf, 0)
		}
	}
	buf = minweightAppendUint32(buf, uint32(len(state.freeRoots)))
	for _, root := range state.freeRoots {
		buf = minweightAppendUint32(buf, root)
	}
	return buf
}

func minweightEncodeDatabaseMetadata(db *minweightDatabase) []byte {
	return minweightEncodeDatabaseState(db.stateLocked())
}

func minweightDecodeDatabaseSettings(db *minweightDatabase, data []byte, off *int) error {
	version, ok := minweightReadUint32(data, off)
	if !ok || version != minweightMetaVersion {
		return minweightCorruptMetadata("bad version")
	}
	var u uint32
	if u, ok = minweightReadUint32(data, off); !ok {
		return minweightCorruptMetadata("missing next root")
	}
	db.next = u
	if u, ok = minweightReadUint32(data, off); !ok {
		return minweightCorruptMetadata("missing data version")
	}
	db.dataVer = u
	if u, ok = minweightReadUint32(data, off); !ok {
		return minweightCorruptMetadata("missing page size")
	}
	db.pageSize = int32(u)
	if u, ok = minweightReadUint32(data, off); !ok {
		return minweightCorruptMetadata("missing reserve")
	}
	db.reserve = int32(u)
	if u, ok = minweightReadUint32(data, off); !ok {
		return minweightCorruptMetadata("missing requested reserve")
	}
	db.reserveWanted = int32(u)
	if db.pageSizeFixed, ok = minweightReadBool(data, off); !ok {
		return minweightCorruptMetadata("missing fixed-page-size flag")
	}
	if db.maxPageCount, ok = minweightReadUint32(data, off); !ok {
		return minweightCorruptMetadata("missing max page count")
	}
	if u, ok = minweightReadUint32(data, off); !ok {
		return minweightCorruptMetadata("missing secure delete")
	}
	db.secureDelete = int32(u)
	if u, ok = minweightReadUint32(data, off); !ok {
		return minweightCorruptMetadata("missing auto vacuum")
	}
	db.autoVacuum = int32(u)
	if u, ok = minweightReadUint32(data, off); !ok {
		return minweightCorruptMetadata("missing cache size")
	}
	db.cacheSize = int32(u)
	if u, ok = minweightReadUint32(data, off); !ok {
		return minweightCorruptMetadata("missing spill size")
	}
	db.spillSize = int32(u)
	return nil
}

func minweightDecodeDatabaseMeta(db *minweightDatabase, data []byte, off *int) error {
	var ok bool
	for i := range db.meta {
		if db.meta[i], ok = minweightReadUint32(data, off); !ok {
			return minweightCorruptMetadata("missing btree meta")
		}
	}
	return nil
}

func minweightDecodeDatabaseTables(db *minweightDatabase, data []byte, off *int) error {
	tableCount, ok := minweightReadUint32(data, off)
	if !ok {
		return minweightCorruptMetadata("missing table count")
	}
	db.tables = make(map[uint32]minweightTable, tableCount)
	for range tableCount {
		root, ok := minweightReadUint32(data, off)
		if !ok {
			return minweightCorruptMetadata("missing table root")
		}
		intKey, ok := minweightReadBool(data, off)
		if !ok {
			return minweightCorruptMetadata("missing table kind")
		}
		db.tables[root] = minweightTable{intKey: intKey}
	}
	return nil
}

func minweightDecodeDatabaseFreeRoots(db *minweightDatabase, data []byte, off *int) error {
	freeCount, ok := minweightReadUint32(data, off)
	if !ok {
		return minweightCorruptMetadata("missing free root count")
	}
	db.freeRoots = make([]uint32, freeCount)
	for i := range db.freeRoots {
		if db.freeRoots[i], ok = minweightReadUint32(data, off); !ok {
			return minweightCorruptMetadata("missing free root")
		}
	}
	return nil
}

func minweightDecodeDatabaseMetadata(store *minweight.Store, path string, data []byte) (*minweightDatabase, error) {
	db := minweightNewDatabase(store, path)
	off := 0
	if err := minweightDecodeDatabaseSettings(db, data, &off); err != nil {
		return nil, err
	}
	if err := minweightDecodeDatabaseMeta(db, data, &off); err != nil {
		return nil, err
	}
	if err := minweightDecodeDatabaseTables(db, data, &off); err != nil {
		return nil, err
	}
	if err := minweightDecodeDatabaseFreeRoots(db, data, &off); err != nil {
		return nil, err
	}
	if off != len(data) {
		return nil, minweightCorruptMetadata("trailing bytes")
	}
	if _, ok := db.tables[uint32(SCHEMA_ROOT)]; !ok {
		return nil, minweightCorruptMetadata("missing schema root")
	}
	return db, db.recomputeTableStats()
}

func minweightNewDatabase(store *minweight.Store, path string) *minweightDatabase {
	return &minweightDatabase{
		store:        store,
		path:         path,
		tables:       map[uint32]minweightTable{1: {intKey: true}},
		next:         1,
		pageSize:     SQLITE_DEFAULT_PAGE_SIZE,
		maxPageCount: SQLITE_MAX_PAGE_COUNT,
		cacheSize:    SQLITE_DEFAULT_CACHE_SIZE,
		spillSize:    1,
		readers:      map[*minweightBtree]int{},
		readerViews:  map[*minweightBtree]uint64{},
		pinnedViews:  map[uint64]int{},
		tableLocks:   map[uint32]map[*minweightBtree]uint8{},
		generation:   1,
	}
}

func minweightOpenDatabase(path string) (*minweightDatabase, error) {
	store, err := minweight.Open(path)
	if err != nil {
		return nil, err
	}
	value, ok, err := store.Get(minweightMetaKey)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	if !ok {
		return minweightNewDatabase(store, path), nil
	}
	db, err := minweightDecodeDatabaseMetadata(store, path, value)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return db, nil
}

func (e *minweightStorageEngine) openDatabase(key string, readOnly bool) (*minweightDatabase, int32) {
	if key == "" {
		return minweightNewDatabase(minweight.New(), ""), SQLITE_OK
	}
	if readOnly {
		return nil, SQLITE_CANTOPEN
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if existing := e.dbs[key]; existing != nil {
		existing.refs++
		return existing, SQLITE_OK
	}
	database, err := minweightOpenDatabase(key)
	if err != nil {
		return nil, minweightOpenError(err)
	}
	database.refs = 1
	e.dbs[key] = database
	return database, SQLITE_OK
}

func (e *minweightStorageEngine) releaseDatabase(database *minweightDatabase) int32 {
	if database == nil || database.path == "" {
		return SQLITE_OK
	}
	e.mu.Lock()
	database.refs--
	refs := database.refs
	if refs == 0 {
		delete(e.dbs, database.path)
	}
	e.mu.Unlock()
	if refs != 0 {
		return SQLITE_OK
	}
	return minweightSQLiteError(database.store.Close())
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
		return minweightNormalizePath(name)
	}
}

func minweightNormalizePath(name string) string {
	path, err := filepath.Abs(name)
	if err != nil {
		return filepath.Clean(name)
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	dir, base := filepath.Split(path)
	if realDir, err := filepath.EvalSymlinks(filepath.Clean(dir)); err == nil {
		return filepath.Join(realDir, base)
	}
	return filepath.Clean(path)
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
	arg := minweightInt64FromPointer(pArg)
	newLimit := minweightClampMmapLimit(*arg)
	bt.mu.Lock()
	defer bt.mu.Unlock()
	*arg = Tsqlite3_int64(bt.mmapSize)
	if newLimit >= 0 {
		bt.mmapSize = int64(newLimit)
		minweightPagerFromPointer(bt.pager).FszMmap = newLimit
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
	buf := minweightByteSliceFromPointer(p, len(s)+1)
	copy(buf, s)
	buf[len(s)] = 0
	return p
}

func minweightIntegrityRoots(aRoot BtreeMemoryHandle, nRoot int32) ([]uint32, bool) {
	if aRoot.IsNil() || nRoot <= 0 {
		return nil, false
	}
	roots := make([]uint32, nRoot)
	rootBytes := aRoot.ReadBytes(int(nRoot) * 4)
	for i := range roots {
		roots[i] = binary.NativeEndian.Uint32(rootBytes[i*4 : i*4+4])
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

func minweightVersionedIndexKey(root uint32, suffix []byte) []byte {
	key := make([]byte, 6+len(suffix))
	key[0] = minweightIndexPrefix
	binary.BigEndian.PutUint32(key[1:5], root)
	key[5] = minweightIndexKeyVersion
	copy(key[6:], suffix)
	return key
}

func minweightIndexStoreKey(ctx BtreeContext, keyInfo uintptr, root uint32, keyBytes []byte) ([]byte, error) {
	suffix, err := minweightComparableIndexKey(ctx, keyInfo, keyBytes)
	if err != nil {
		return nil, err
	}
	return minweightVersionedIndexKey(root, suffix), nil
}

func minweightRowIndexStoreKey(root uint32, row minweightRow) []byte {
	return append([]byte(nil), row.storeKey...)
}

func minweightMoveIndexStoreKey(root uint32, row minweightRow) []byte {
	key := append([]byte(nil), row.storeKey...)
	binary.BigEndian.PutUint32(key[1:5], root)
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

func minweightVersionedIndexLower(root uint32) []byte {
	key := make([]byte, 6)
	key[0] = minweightIndexPrefix
	binary.BigEndian.PutUint32(key[1:5], root)
	key[5] = minweightIndexKeyVersion
	return key
}

func minweightVersionedIndexUpper(root uint32) []byte {
	key := make([]byte, 6)
	key[0] = minweightIndexPrefix
	binary.BigEndian.PutUint32(key[1:5], root)
	key[5] = minweightIndexKeyVersion + 1
	return key
}

func minweightIndexKeyVersionedForRoot(root uint32, key []byte) bool {
	return len(key) >= 6 &&
		key[0] == minweightIndexPrefix &&
		binary.BigEndian.Uint32(key[1:5]) == root &&
		key[5] == minweightIndexKeyVersion
}

func minweightIndexKeyInVersionedRange(root uint32, key []byte) bool {
	return minweightIndexKeyVersionedForRoot(root, key) &&
		bytes.Compare(key, minweightVersionedIndexLower(root)) >= 0 &&
		bytes.Compare(key, minweightVersionedIndexUpper(root)) < 0
}

func minweightIndexSeekAfter(key []byte) []byte {
	next := append([]byte(nil), key...)
	return append(next, 0)
}

func minweightIndexSeekAfterPrefix(key []byte) []byte {
	next := append([]byte(nil), key...)
	return append(next, 0xff)
}

func minweightDecodeRow(item minweight.Item, intKey bool) minweightRow {
	row := minweightRow{
		storeKey: append([]byte(nil), item.Key...),
		payload:  append([]byte(nil), item.Value...),
	}
	if intKey {
		u := binary.BigEndian.Uint64(item.Key[5:13]) ^ (1 << 63)
		row.rowid = int64(u)
		return row
	}
	if len(item.Value) != 0 {
		row.key = append([]byte(nil), item.Value...)
		row.payload = append([]byte(nil), item.Value...)
		return row
	}
	return row
}

type minweightRecordField struct {
	serial uint64
	data   []byte
}

func minweightComparableIndexKey(ctx BtreeContext, keyInfo uintptr, record []byte) ([]byte, error) {
	fields, err := minweightParseRecord(record)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(record)*2+16)
	for i, field := range fields {
		sortFlags := minweightKeyInfoSortFlags(keyInfo, i)
		if sortFlags&uint8(KEYINFO_ORDER_BIGNULL) != 0 {
			return nil, fmt.Errorf("minweight sqlite index key: NULLS FIRST/LAST sort flag is not supported")
		}
		fieldKey, err := minweightComparableFieldKey(ctx, keyInfo, i, field)
		if err != nil {
			return nil, err
		}
		if sortFlags&uint8(KEYINFO_ORDER_DESC) != 0 {
			for j := range fieldKey {
				fieldKey[j] = ^fieldKey[j]
			}
		}
		out = append(out, fieldKey...)
	}
	out = append(out, 0x50)
	out = minweightAppendEscapedBytes(out, record)
	return out, nil
}

func minweightComparableIndexProbeKey(ctx BtreeContext, root uint32, pIdxKey uintptr) ([]byte, error) {
	rec := minweightUnpackedRecordFromPointer(pIdxKey)
	suffix, err := minweightComparableUnpackedPrefix(ctx, rec)
	if err != nil {
		return nil, err
	}
	key := minweightVersionedIndexKey(root, suffix)
	if rec.Fdefault_rc < 0 {
		if len(suffix) == 0 {
			return minweightVersionedIndexUpper(root), nil
		}
		key = minweightIndexSeekAfterPrefix(key)
	}
	return key, nil
}

func minweightComparableUnpackedPrefix(ctx BtreeContext, rec *TUnpackedRecord) ([]byte, error) {
	out := []byte{}
	for i := 0; i < int(rec.FnField); i++ {
		sortFlags := minweightKeyInfoSortFlags(rec.FpKeyInfo, i)
		if sortFlags&uint8(KEYINFO_ORDER_BIGNULL) != 0 {
			return nil, fmt.Errorf("minweight sqlite index key: NULLS FIRST/LAST sort flag is not supported")
		}
		fieldKey, err := minweightComparableMemKey(ctx, rec.FpKeyInfo, i, rec.FaMem+uintptr(i)*unsafe.Sizeof(TMem{}))
		if err != nil {
			return nil, err
		}
		if sortFlags&uint8(KEYINFO_ORDER_DESC) != 0 {
			for j := range fieldKey {
				fieldKey[j] = ^fieldKey[j]
			}
		}
		out = append(out, fieldKey...)
	}
	return out, nil
}

func minweightParseRecord(record []byte) ([]minweightRecordField, error) {
	headerSize, n, ok := minweightGetVarint(record)
	if !ok || headerSize < uint64(n) || headerSize > uint64(len(record)) {
		return nil, fmt.Errorf("minweight sqlite index key: corrupt record header")
	}
	headerEnd := int(headerSize)
	headerOffset := n
	dataOffset := headerEnd
	fields := []minweightRecordField{}
	for headerOffset < headerEnd {
		serial, consumed, ok := minweightGetVarint(record[headerOffset:headerEnd])
		if !ok {
			return nil, fmt.Errorf("minweight sqlite index key: corrupt record serial type")
		}
		headerOffset += consumed
		size, ok := minweightSerialTypeLen(serial)
		if !ok {
			return nil, fmt.Errorf("minweight sqlite index key: unsupported serial type %d", serial)
		}
		if size > len(record)-dataOffset {
			return nil, fmt.Errorf("minweight sqlite index key: corrupt record payload")
		}
		fields = append(fields, minweightRecordField{
			serial: serial,
			data:   record[dataOffset : dataOffset+size],
		})
		dataOffset += size
	}
	if headerOffset != headerEnd {
		return nil, fmt.Errorf("minweight sqlite index key: corrupt record header")
	}
	return fields, nil
}

func minweightGetVarint(p []byte) (uint64, int, bool) {
	var v uint64
	for i := 0; i < 8; i++ {
		if i >= len(p) {
			return 0, 0, false
		}
		b := p[i]
		v = (v << 7) | uint64(b&0x7f)
		if b < 0x80 {
			return v, i + 1, true
		}
	}
	if len(p) < 9 {
		return 0, 0, false
	}
	v = (v << 8) | uint64(p[8])
	return v, 9, true
}

func minweightSerialTypeLen(serial uint64) (int, bool) {
	switch serial {
	case 0, 8, 9:
		return 0, true
	case 1:
		return 1, true
	case 2:
		return 2, true
	case 3:
		return 3, true
	case 4:
		return 4, true
	case 5:
		return 6, true
	case 6, 7:
		return 8, true
	case 10, 11:
		return 0, false
	}
	if serial >= 12 {
		return int((serial - 12) / 2), true
	}
	return 0, false
}

func minweightComparableFieldKey(ctx BtreeContext, keyInfo uintptr, fieldIndex int, field minweightRecordField) ([]byte, error) {
	out := make([]byte, 0, len(field.data)+16)
	switch field.serial {
	case 0:
		return append(out, 0x10), nil
	case 1, 2, 3, 4, 5, 6, 8, 9:
		v := minweightDecodeRecordInteger(field)
		return minweightAppendNumberKey(out, v < 0, minweightIntegerMagnitude(v), 0), nil
	case 7:
		f := math.Float64frombits(binary.BigEndian.Uint64(field.data))
		if math.IsNaN(f) {
			return append(out, 0x10), nil
		}
		if f == 0 {
			return append(out, 0x21), nil
		}
		negative, mantissa, exponent := minweightFloatMagnitude(f)
		return minweightAppendNumberKey(out, negative, mantissa, exponent), nil
	}
	if field.serial >= 12 && field.serial&1 == 0 {
		out = append(out, 0x40)
		return minweightAppendEscapedBytes(out, field.data), nil
	}
	if field.serial >= 13 && field.serial&1 == 1 {
		normalized, err := minweightNormalizeTextForCollation(ctx, keyInfo, fieldIndex, field.data)
		if err != nil {
			return nil, err
		}
		out = append(out, 0x30)
		return minweightAppendEscapedBytes(out, normalized), nil
	}
	return nil, fmt.Errorf("minweight sqlite index key: unsupported serial type %d", field.serial)
}

func minweightComparableMemKey(ctx BtreeContext, keyInfo uintptr, fieldIndex int, pMem uintptr) ([]byte, error) {
	mem := minweightMemFromPointer(pMem)
	flags := int(mem.Fflags)
	out := make([]byte, 0, int(mem.Fn)+16)
	if flags&MEM_Null != 0 {
		return append(out, 0x10), nil
	}
	if flags&(MEM_Int|MEM_IntReal) != 0 {
		v := minweightMemInt64(pMem)
		return minweightAppendNumberKey(out, v < 0, minweightIntegerMagnitude(v), 0), nil
	}
	if flags&MEM_Real != 0 {
		f := minweightMemFloat64(pMem)
		if math.IsNaN(f) {
			return append(out, 0x10), nil
		}
		if f == 0 {
			return append(out, 0x21), nil
		}
		negative, mantissa, exponent := minweightFloatMagnitude(f)
		return minweightAppendNumberKey(out, negative, mantissa, exponent), nil
	}
	if flags&MEM_Str != 0 {
		if mem.Fenc != uint8(SQLITE_UTF8) {
			return nil, fmt.Errorf("minweight sqlite index key: only UTF-8 text is supported")
		}
		data, err := minweightMemBytes(mem)
		if err != nil {
			return nil, err
		}
		normalized, err := minweightNormalizeTextForCollation(ctx, keyInfo, fieldIndex, data)
		if err != nil {
			return nil, err
		}
		out = append(out, 0x30)
		return minweightAppendEscapedBytes(out, normalized), nil
	}
	data, err := minweightMemBytes(mem)
	if err != nil {
		return nil, err
	}
	if flags&MEM_Zero != 0 {
		nZero := minweightMemZeroLength(pMem)
		if nZero < 0 {
			return nil, fmt.Errorf("minweight sqlite index key: negative zeroblob length")
		}
		data = append(data, make([]byte, int(nZero))...)
	}
	out = append(out, 0x40)
	return minweightAppendEscapedBytes(out, data), nil
}

func minweightMemBytes(mem *TMem) ([]byte, error) {
	if mem.Fn < 0 {
		return nil, fmt.Errorf("minweight sqlite index key: negative value length")
	}
	if mem.Fn == 0 {
		return nil, nil
	}
	if mem.Fz == 0 {
		return nil, fmt.Errorf("minweight sqlite index key: nil value pointer")
	}
	return append([]byte(nil), minweightByteSliceFromPointer(mem.Fz, int(mem.Fn))...), nil
}

func minweightDecodeRecordInteger(field minweightRecordField) int64 {
	switch field.serial {
	case 8:
		return 0
	case 9:
		return 1
	}
	var u uint64
	for _, b := range field.data {
		u = (u << 8) | uint64(b)
	}
	bitsUsed := uint(len(field.data) * 8)
	if len(field.data) != 0 && field.data[0]&0x80 != 0 {
		u |= ^uint64(0) << bitsUsed
	}
	return int64(u)
}

func minweightIntegerMagnitude(v int64) uint64 {
	if v < 0 {
		return uint64(-(v + 1)) + 1
	}
	return uint64(v)
}

func minweightFloatMagnitude(f float64) (bool, uint64, int) {
	negative := math.Signbit(f)
	bits64 := math.Float64bits(math.Abs(f))
	exponentBits := int((bits64 >> 52) & 0x7ff)
	fraction := bits64 & ((uint64(1) << 52) - 1)
	if exponentBits == 0x7ff {
		return negative, 1, math.MaxInt32
	}
	if exponentBits == 0 {
		return negative, fraction, -1074
	}
	return negative, (uint64(1) << 52) | fraction, exponentBits - 1023 - 52
}

func minweightAppendNumberKey(out []byte, negative bool, magnitude uint64, exponent int) []byte {
	if magnitude == 0 {
		return append(out, 0x21)
	}
	trailingZeros := bits.TrailingZeros64(magnitude)
	magnitude >>= uint(trailingZeros)
	exponent += trailingZeros
	bitLen := bits.Len64(magnitude)
	scale := 64 - bitLen
	norm := magnitude << uint(scale)
	logExponent := exponent + bitLen - 1
	expKey := uint32(int32(logExponent)) ^ (uint32(1) << 31)
	var buf [12]byte
	binary.BigEndian.PutUint32(buf[:4], expKey)
	binary.BigEndian.PutUint64(buf[4:], norm)
	if negative {
		out = append(out, 0x20)
		for _, b := range buf {
			out = append(out, ^b)
		}
		return out
	}
	out = append(out, 0x22)
	return append(out, buf[:]...)
}

func minweightAppendEscapedBytes(out []byte, data []byte) []byte {
	for _, b := range data {
		if b == 0 {
			out = append(out, 0, 0xff)
			continue
		}
		out = append(out, b)
	}
	return append(out, 0, 0)
}

func minweightKeyInfoSortFlags(keyInfo uintptr, fieldIndex int) uint8 {
	if keyInfo == 0 || fieldIndex >= int(minweightKeyInfoFromPointer(keyInfo).FnKeyField) {
		return 0
	}
	sortFlags := minweightKeyInfoFromPointer(keyInfo).FaSortFlags
	if sortFlags == 0 {
		return 0
	}
	return minweightUint8FromPointer(sortFlags + uintptr(fieldIndex))
}

func minweightKeyInfoCollation(keyInfo uintptr, fieldIndex int) uintptr {
	if keyInfo == 0 || fieldIndex >= int(minweightKeyInfoFromPointer(keyInfo).FnAllField) {
		return 0
	}
	return minweightUintptrFromPointer(keyInfo + unsafe.Sizeof(TKeyInfo{}) + uintptr(fieldIndex)*unsafe.Sizeof(uintptr(0)))
}

func minweightNormalizeTextForCollation(ctx BtreeContext, keyInfo uintptr, fieldIndex int, data []byte) ([]byte, error) {
	if keyInfo != 0 && minweightKeyInfoFromPointer(keyInfo).Fenc != uint8(SQLITE_UTF8) {
		return nil, fmt.Errorf("minweight sqlite index key: only UTF-8 KeyInfo is supported")
	}
	collation := minweightKeyInfoCollation(keyInfo, fieldIndex)
	if collation == 0 {
		return data, nil
	}
	name := strings.ToUpper(libc.GoString(minweightCollSeqFromPointer(collation).FzName))
	switch name {
	case "BINARY":
		return data, nil
	case "NOCASE":
		normalized := append([]byte(nil), data...)
		for i, b := range normalized {
			if b >= 'A' && b <= 'Z' {
				normalized[i] = b + ('a' - 'A')
			}
		}
		return normalized, nil
	case "RTRIM":
		n := len(data)
		for n > 0 && data[n-1] == ' ' {
			n--
		}
		return data[:n], nil
	}
	_ = ctx
	return nil, fmt.Errorf("minweight sqlite index key: unsupported collation %q", name)
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

func minweightRootListsEqual(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func minweightTablesEqual(a, b map[uint32]minweightTable) bool {
	if len(a) != len(b) {
		return false
	}
	for root, table := range a {
		if b[root] != table {
			return false
		}
	}
	return true
}

func minweightStatesEqual(a, b minweightDBState) bool {
	return a.meta == b.meta &&
		a.next == b.next &&
		a.dataVer == b.dataVer &&
		a.pageSize == b.pageSize &&
		a.reserve == b.reserve &&
		a.reserveWanted == b.reserveWanted &&
		a.pageSizeFixed == b.pageSizeFixed &&
		a.maxPageCount == b.maxPageCount &&
		a.secureDelete == b.secureDelete &&
		a.autoVacuum == b.autoVacuum &&
		a.cacheSize == b.cacheSize &&
		a.spillSize == b.spillSize &&
		minweightTablesEqual(a.tables, b.tables) &&
		minweightRootListsEqual(a.freeRoots, b.freeRoots)
}

func minweightCloneState(src minweightDBState) minweightDBState {
	src.tables = minweightCloneTables(src.tables)
	src.freeRoots = minweightCloneRootList(src.freeRoots)
	return src
}

func (db *minweightDatabase) stateLocked() minweightDBState {
	return minweightDBState{
		meta:          db.meta,
		tables:        minweightCloneTables(db.tables),
		next:          db.next,
		dataVer:       db.dataVer,
		pageSize:      db.pageSize,
		reserve:       db.reserve,
		reserveWanted: db.reserveWanted,
		pageSizeFixed: db.pageSizeFixed,
		maxPageCount:  db.maxPageCount,
		secureDelete:  db.secureDelete,
		autoVacuum:    db.autoVacuum,
		cacheSize:     db.cacheSize,
		spillSize:     db.spillSize,
		freeRoots:     minweightCloneRootList(db.freeRoots),
	}
}

func (db *minweightDatabase) applyStateLocked(state minweightDBState) {
	db.meta = state.meta
	db.tables = minweightCloneTables(state.tables)
	db.next = state.next
	db.dataVer = state.dataVer
	db.pageSize = state.pageSize
	db.reserve = state.reserve
	db.reserveWanted = state.reserveWanted
	db.pageSizeFixed = state.pageSizeFixed
	db.maxPageCount = state.maxPageCount
	db.secureDelete = state.secureDelete
	db.autoVacuum = state.autoVacuum
	db.cacheSize = state.cacheSize
	db.spillSize = state.spillSize
	db.freeRoots = minweightCloneRootList(state.freeRoots)
}

func minweightKeyRoot(key []byte) (uint32, bool) {
	if len(key) < 5 {
		return 0, false
	}
	if key[0] != minweightTablePrefix && key[0] != minweightIndexPrefix {
		return 0, false
	}
	return binary.BigEndian.Uint32(key[1:5]), true
}

func (db *minweightDatabase) pruneCommitChangesLocked() {
	if len(db.pinnedViews) == 0 {
		db.changes = nil
		return
	}
	oldest := db.generation
	for generation := range db.pinnedViews {
		if generation < oldest {
			oldest = generation
		}
	}
	n := 0
	for _, change := range db.changes {
		if change.generation > oldest {
			db.changes[n] = change
			n++
		}
	}
	clear(db.changes[n:])
	db.changes = db.changes[:n]
}

func (db *minweightDatabase) releaseReaderViewLocked(bt *minweightBtree) {
	generation, ok := db.readerViews[bt]
	if !ok {
		return
	}
	delete(db.readerViews, bt)
	if refs := db.pinnedViews[generation]; refs <= 1 {
		delete(db.pinnedViews, generation)
	} else {
		db.pinnedViews[generation] = refs - 1
	}
	db.pruneCommitChangesLocked()
}

func (db *minweightDatabase) retainReaderViewLocked(bt *minweightBtree) {
	if _, ok := db.readerViews[bt]; ok {
		return
	}
	generation := db.generation
	db.readerViews[bt] = generation
	db.pinnedViews[generation]++
}

func (db *minweightDatabase) releaseAllReaderStateLocked(bt *minweightBtree) {
	delete(db.readers, bt)
	db.releaseReaderViewLocked(bt)
}

func minweightCloneTxnWrites(src map[string]minweightTxnWrite) map[string]minweightTxnWrite {
	dst := make(map[string]minweightTxnWrite, len(src))
	for key, write := range src {
		write.key = append([]byte(nil), write.key...)
		write.value = append([]byte(nil), write.value...)
		dst[key] = write
	}
	return dst
}

func (tx *minweightTxn) cloneSavepoint() minweightTxnSavepoint {
	return minweightTxnSavepoint{
		state:  minweightCloneState(tx.state),
		writes: minweightCloneTxnWrites(tx.writes),
	}
}

func (tx *minweightTxn) restoreSavepoint(savepoint minweightTxnSavepoint) {
	tx.state = minweightCloneState(savepoint.state)
	tx.writes = minweightCloneTxnWrites(savepoint.writes)
}

func (bt *minweightBtree) newTxnLocked() *minweightTxn {
	return &minweightTxn{
		baseGeneration: bt.generation,
		state:          bt.stateLocked(),
		reads:          map[string]struct{}{},
		readRoots:      map[uint32]struct{}{},
		readMeta:       true,
		writes:         map[string]minweightTxnWrite{},
	}
}

func (bt *minweightBtree) activeTxnLocked() *minweightTxn {
	if bt.writer == bt {
		return bt.txn
	}
	return nil
}

func (bt *minweightBtree) visibleState() minweightDBState {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if tx := bt.activeTxnLocked(); tx != nil {
		return minweightCloneState(tx.state)
	}
	return bt.stateLocked()
}

func (bt *minweightBtree) visibleStateLocked() minweightDBState {
	if tx := bt.activeTxnLocked(); tx != nil {
		return minweightCloneState(tx.state)
	}
	return bt.stateLocked()
}

func (bt *minweightBtree) mutableStateLocked() *minweightDBState {
	if tx := bt.activeTxnLocked(); tx != nil {
		return &tx.state
	}
	return nil
}

func (bt *minweightBtree) updateStateLocked(fn func(*minweightDBState)) {
	if state := bt.mutableStateLocked(); state != nil {
		fn(state)
		return
	}
	state := bt.stateLocked()
	fn(&state)
	bt.applyStateLocked(state)
}

func (bt *minweightBtree) bumpDataVer() {
	bt.mu.Lock()
	bt.updateStateLocked(func(state *minweightDBState) {
		state.dataVer++
	})
	bt.mu.Unlock()
}

func (bt *minweightBtree) txnWritesSnapshot() map[string]minweightTxnWrite {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	tx := bt.activeTxnLocked()
	if tx == nil || len(tx.writes) == 0 {
		return nil
	}
	return minweightCloneTxnWrites(tx.writes)
}

func (bt *minweightBtree) get(key []byte) ([]byte, bool, error) {
	bt.mu.Lock()
	if tx := bt.activeTxnLocked(); tx != nil {
		if write, ok := tx.writes[string(key)]; ok {
			bt.mu.Unlock()
			if write.deleted {
				return nil, false, nil
			}
			return append([]byte(nil), write.value...), true, nil
		}
	}
	bt.mu.Unlock()
	value, ok, err := bt.store.Get(key)
	if err != nil {
		return nil, false, err
	}
	bt.mu.Lock()
	if tx := bt.activeTxnLocked(); tx != nil {
		tx.reads[string(key)] = struct{}{}
	}
	bt.mu.Unlock()
	return value, ok, nil
}

func (bt *minweightBtree) noteRootRead(root uint32) {
	bt.mu.Lock()
	if tx := bt.activeTxnLocked(); tx != nil {
		tx.readRoots[root] = struct{}{}
	}
	bt.mu.Unlock()
}

func (bt *minweightBtree) put(key, value []byte) error {
	bt.mu.Lock()
	if tx := bt.activeTxnLocked(); tx != nil {
		tx.writes[string(key)] = minweightTxnWrite{
			key:   append([]byte(nil), key...),
			value: append([]byte(nil), value...),
		}
		bt.mu.Unlock()
		return nil
	}
	bt.mu.Unlock()
	return bt.store.Put(key, value)
}

func (bt *minweightBtree) delete(key []byte) (bool, error) {
	bt.mu.Lock()
	if tx := bt.activeTxnLocked(); tx != nil {
		keyString := string(key)
		if write, ok := tx.writes[keyString]; ok {
			existed := !write.deleted
			tx.writes[keyString] = minweightTxnWrite{
				key:     append([]byte(nil), key...),
				deleted: true,
			}
			bt.mu.Unlock()
			return existed, nil
		}
		bt.mu.Unlock()
		_, existed, err := bt.store.Get(key)
		if err != nil {
			return false, err
		}
		bt.mu.Lock()
		if tx := bt.activeTxnLocked(); tx != nil {
			tx.reads[keyString] = struct{}{}
			tx.writes[keyString] = minweightTxnWrite{
				key:     append([]byte(nil), key...),
				deleted: true,
			}
			bt.mu.Unlock()
			return existed, nil
		}
		bt.mu.Unlock()
		return bt.store.Delete(key)
	}
	bt.mu.Unlock()
	return bt.store.Delete(key)
}

func (bt *minweightBtree) snapshot() (*minweightSnapshot, error) {
	s := &minweightSnapshot{}
	writes := bt.txnWritesSnapshot()
	items := map[string]minweightSnapshotItem{}
	if err := bt.store.Scan(func(item minweight.Item) bool {
		items[string(item.Key)] = minweightSnapshotItem{
			key:   append([]byte(nil), item.Key...),
			value: append([]byte(nil), item.Value...),
		}
		return true
	}); err != nil {
		return nil, err
	}
	for _, write := range writes {
		key := string(write.key)
		if write.deleted {
			delete(items, key)
			continue
		}
		items[key] = minweightSnapshotItem{
			key:   append([]byte(nil), write.key...),
			value: append([]byte(nil), write.value...),
		}
	}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		s.items = append(s.items, items[key])
	}
	bt.mu.Lock()
	state := bt.visibleStateLocked()
	s.meta = state.meta
	s.tables = minweightCloneTables(state.tables)
	s.next = state.next
	s.dataVer = state.dataVer
	s.pageSize = state.pageSize
	s.reserve = state.reserve
	s.reserveWanted = state.reserveWanted
	s.pageSizeFixed = state.pageSizeFixed
	s.maxPageCount = state.maxPageCount
	s.secureDelete = state.secureDelete
	s.autoVacuum = state.autoVacuum
	s.cacheSize = state.cacheSize
	s.spillSize = state.spillSize
	s.freeRoots = minweightCloneRootList(state.freeRoots)
	bt.mu.Unlock()
	return s, nil
}

func (bt *minweightBtree) visibleDataVer() uint32 {
	return bt.visibleState().dataVer
}

func (bt *minweightBtree) visibleTable(root uint32) (minweightTable, bool) {
	state := bt.visibleState()
	table, ok := state.tables[root]
	return table, ok
}

func (bt *minweightBtree) visibleMeta(idx int32) uint32 {
	state := bt.visibleState()
	if idx == int32(BTREE_DATA_VERSION) {
		return state.dataVer
	}
	if idx >= 0 && idx < int32(len(state.meta)) {
		return state.meta[idx]
	}
	return 0
}

func (bt *minweightBtree) persistMetadataLocked() error {
	if bt.path == "" {
		return nil
	}
	if bt.activeTxnLocked() != nil {
		return nil
	}
	return bt.store.Put(minweightMetaKey, minweightEncodeDatabaseMetadata(bt.minweightDatabase))
}

func (db *minweightDatabase) recomputeTableStats() error {
	for root, table := range db.tables {
		table.rowCount = 0
		table.minRowid = 0
		table.maxRowid = 0
		db.tables[root] = table
	}
	corrupt := false
	err := db.store.Scan(func(item minweight.Item) bool {
		if len(item.Key) == 0 {
			return true
		}
		switch item.Key[0] {
		case minweightTablePrefix:
			if len(item.Key) != 13 {
				corrupt = true
				return false
			}
			root := binary.BigEndian.Uint32(item.Key[1:5])
			table, ok := db.tables[root]
			if !ok || !table.intKey {
				corrupt = true
				return false
			}
			rowid := int64(binary.BigEndian.Uint64(item.Key[5:13]) ^ (1 << 63))
			table.rowCount++
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
			db.tables[root] = table
		case minweightIndexPrefix:
			if len(item.Key) < 5 {
				corrupt = true
				return false
			}
			root := binary.BigEndian.Uint32(item.Key[1:5])
			if !minweightIndexKeyInVersionedRange(root, item.Key) {
				corrupt = true
				return false
			}
			table, ok := db.tables[root]
			if !ok || table.intKey {
				corrupt = true
				return false
			}
			table.rowCount++
			db.tables[root] = table
		}
		return true
	})
	if err != nil {
		return err
	}
	if corrupt {
		return minweightCorruptMetadata("kv root metadata mismatch")
	}
	return nil
}

func (bt *minweightBtree) loadRows(root uint32, intKey bool) ([]minweightRow, error) {
	bt.noteRootRead(root)
	prefix := minweightRootPrefix(root, intKey)
	writes := bt.txnWritesSnapshot()
	rowsByKey := map[string]minweightRow{}
	corrupt := false
	err := bt.store.Scan(func(item minweight.Item) bool {
		if !bytes.HasPrefix(item.Key, prefix) {
			return true
		}
		if intKey {
			if _, _, ok := minweightTableRootRowid(item.Key); !ok {
				corrupt = true
				return false
			}
		} else if !minweightIndexKeyInVersionedRange(root, item.Key) {
			corrupt = true
			return false
		}
		rowsByKey[string(item.Key)] = minweightDecodeRow(item, intKey)
		return true
	})
	if err != nil {
		return nil, err
	}
	if corrupt {
		return nil, minweightCorruptMetadata("kv root contains unsupported raw index key")
	}
	for _, write := range writes {
		if !bytes.HasPrefix(write.key, prefix) {
			continue
		}
		if intKey {
			if _, _, ok := minweightTableRootRowid(write.key); !ok {
				return nil, minweightCorruptMetadata("kv root contains malformed table key")
			}
		} else if !minweightIndexKeyInVersionedRange(root, write.key) {
			return nil, minweightCorruptMetadata("kv root contains unsupported raw index key")
		}
		key := string(write.key)
		if write.deleted {
			delete(rowsByKey, key)
			continue
		}
		rowsByKey[key] = minweightDecodeRow(minweight.Item{Key: write.key, Value: write.value}, intKey)
	}
	rows := make([]minweightRow, 0, len(rowsByKey))
	for _, row := range rowsByKey {
		rows = append(rows, row)
	}
	if intKey {
		sort.Slice(rows, func(i, j int) bool { return rows[i].rowid < rows[j].rowid })
	} else {
		sort.Slice(rows, func(i, j int) bool { return bytes.Compare(rows[i].key, rows[j].key) < 0 })
	}
	return rows, nil
}

func minweightTableRootRowid(key []byte) (uint32, int64, bool) {
	if len(key) != 13 || key[0] != minweightTablePrefix {
		return 0, 0, false
	}
	root := binary.BigEndian.Uint32(key[1:5])
	rowid := int64(binary.BigEndian.Uint64(key[5:13]) ^ (1 << 63))
	return root, rowid, true
}

func minweightTableRowFromItem(root uint32, item minweight.Item) (minweightRow, bool) {
	itemRoot, rowid, ok := minweightTableRootRowid(item.Key)
	if !ok || itemRoot != root {
		return minweightRow{}, false
	}
	return minweightRow{
		rowid:    rowid,
		storeKey: append([]byte(nil), item.Key...),
		payload:  append([]byte(nil), item.Value...),
	}, true
}

func minweightIndexRowFromItem(root uint32, item minweight.Item) (minweightRow, bool) {
	if !minweightIndexKeyInVersionedRange(root, item.Key) {
		return minweightRow{}, false
	}
	return minweightDecodeRow(item, false), true
}

func (bt *minweightBtree) txnWriteForKey(key []byte) (minweightTxnWrite, bool) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	tx := bt.activeTxnLocked()
	if tx == nil {
		return minweightTxnWrite{}, false
	}
	write, ok := tx.writes[string(key)]
	return write, ok
}

func (bt *minweightBtree) indexOverlayCandidate(root uint32, target []byte, ge bool, strict bool) (minweightRow, bool) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	tx := bt.activeTxnLocked()
	if tx == nil {
		return minweightRow{}, false
	}
	var best minweightRow
	found := false
	for _, write := range tx.writes {
		if write.deleted || !minweightIndexKeyInVersionedRange(root, write.key) {
			continue
		}
		cmp := bytes.Compare(write.key, target)
		if ge {
			if cmp < 0 || strict && cmp == 0 || found && bytes.Compare(write.key, best.storeKey) >= 0 {
				continue
			}
		} else if cmp > 0 || strict && cmp == 0 || found && bytes.Compare(write.key, best.storeKey) <= 0 {
			continue
		}
		best = minweightDecodeRow(minweight.Item{Key: write.key, Value: write.value}, false)
		found = true
	}
	return best, found
}

func (bt *minweightBtree) tableOverlayCandidate(root uint32, target int64, ge bool) (minweightRow, bool) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	tx := bt.activeTxnLocked()
	if tx == nil {
		return minweightRow{}, false
	}
	var best minweightRow
	found := false
	for _, write := range tx.writes {
		if write.deleted {
			continue
		}
		itemRoot, rowid, ok := minweightTableRootRowid(write.key)
		if !ok || itemRoot != root {
			continue
		}
		if ge {
			if rowid < target || found && rowid >= best.rowid {
				continue
			}
		} else if rowid > target || found && rowid <= best.rowid {
			continue
		}
		best = minweightRow{
			rowid:    rowid,
			storeKey: append([]byte(nil), write.key...),
			payload:  append([]byte(nil), write.value...),
		}
		found = true
	}
	return best, found
}

func minweightBetterIndexGERow(a minweightRow, aOK bool, b minweightRow, bOK bool) (minweightRow, bool) {
	if !aOK {
		return b, bOK
	}
	if !bOK || bytes.Compare(a.storeKey, b.storeKey) <= 0 {
		return a, true
	}
	return b, true
}

func minweightBetterIndexLERow(a minweightRow, aOK bool, b minweightRow, bOK bool) (minweightRow, bool) {
	if !aOK {
		return b, bOK
	}
	if !bOK || bytes.Compare(a.storeKey, b.storeKey) >= 0 {
		return a, true
	}
	return b, true
}

func minweightBetterTableGERow(a minweightRow, aOK bool, b minweightRow, bOK bool) (minweightRow, bool) {
	if !aOK {
		return b, bOK
	}
	if !bOK || a.rowid <= b.rowid {
		return a, true
	}
	return b, true
}

func minweightBetterTableLERow(a minweightRow, aOK bool, b minweightRow, bOK bool) (minweightRow, bool) {
	if !aOK {
		return b, bOK
	}
	if !bOK || a.rowid >= b.rowid {
		return a, true
	}
	return b, true
}

func (bt *minweightBtree) seekIndexGE(root uint32, target []byte, strict bool) (minweightRow, bool, error) {
	bt.noteRootRead(root)
	overlayRow, overlayOK := bt.indexOverlayCandidate(root, target, true, strict)
	seekKey := append([]byte(nil), target...)
	if strict {
		seekKey = minweightIndexSeekAfter(seekKey)
	}
	upper := minweightVersionedIndexUpper(root)
	for {
		item, ok, err := bt.store.SeekGE(seekKey)
		if err != nil {
			return minweightRow{}, false, err
		}
		if !ok || bytes.Compare(item.Key, upper) >= 0 {
			return overlayRow, overlayOK, nil
		}
		if !minweightIndexKeyInVersionedRange(root, item.Key) {
			return overlayRow, overlayOK, nil
		}
		if _, ok := bt.txnWriteForKey(item.Key); ok {
			seekKey = minweightIndexSeekAfter(item.Key)
			continue
		}
		baseRow, ok := minweightIndexRowFromItem(root, item)
		if !ok {
			return overlayRow, overlayOK, nil
		}
		row, rowOK := minweightBetterIndexGERow(baseRow, true, overlayRow, overlayOK)
		return row, rowOK, nil
	}
}

func (bt *minweightBtree) seekIndexLE(root uint32, target []byte, strict bool) (minweightRow, bool, error) {
	bt.noteRootRead(root)
	overlayRow, overlayOK := bt.indexOverlayCandidate(root, target, false, strict)
	lower := minweightVersionedIndexLower(root)
	var baseRow minweightRow
	baseOK := false
	var scanErr error
	err := bt.store.ReverseScanRange(target, minweightRootPrefix(root, false), func(item minweight.Item) bool {
		if bytes.Compare(item.Key, lower) < 0 {
			return false
		}
		if strict && bytes.Equal(item.Key, target) {
			return true
		}
		if !minweightIndexKeyInVersionedRange(root, item.Key) {
			return false
		}
		if _, ok := bt.txnWriteForKey(item.Key); ok {
			return true
		}
		var ok bool
		baseRow, ok = minweightIndexRowFromItem(root, item)
		if !ok {
			scanErr = fmt.Errorf("minweight sqlite index key: corrupt versioned index key")
			return false
		}
		baseOK = true
		return false
	})
	if err != nil {
		return minweightRow{}, false, err
	}
	if scanErr != nil {
		return minweightRow{}, false, scanErr
	}
	row, rowOK := minweightBetterIndexLERow(baseRow, baseOK, overlayRow, overlayOK)
	return row, rowOK, nil
}

func (bt *minweightBtree) seekTableGE(root uint32, target int64) (minweightRow, bool, error) {
	bt.noteRootRead(root)
	overlayRow, overlayOK := bt.tableOverlayCandidate(root, target, true)
	seekKey := minweightTableKey(root, target)
	for {
		item, ok, err := bt.store.SeekGE(seekKey)
		if err != nil {
			return minweightRow{}, false, err
		}
		if !ok {
			return overlayRow, overlayOK, nil
		}
		itemRoot, rowid, ok := minweightTableRootRowid(item.Key)
		if !ok || itemRoot != root {
			return overlayRow, overlayOK, nil
		}
		if _, ok := bt.txnWriteForKey(item.Key); ok {
			if rowid == math.MaxInt64 {
				return overlayRow, overlayOK, nil
			}
			seekKey = minweightTableKey(root, rowid+1)
			continue
		}
		baseRow, ok := minweightTableRowFromItem(root, item)
		if !ok {
			return overlayRow, overlayOK, nil
		}
		row, rowOK := minweightBetterTableGERow(baseRow, true, overlayRow, overlayOK)
		return row, rowOK, nil
	}
}

func (bt *minweightBtree) seekTableLE(root uint32, target int64) (minweightRow, bool, error) {
	bt.noteRootRead(root)
	overlayRow, overlayOK := bt.tableOverlayCandidate(root, target, false)
	seekKey := minweightTableKey(root, target)
	for {
		item, ok, err := bt.store.SeekLE(seekKey)
		if err != nil {
			return minweightRow{}, false, err
		}
		if !ok {
			return overlayRow, overlayOK, nil
		}
		itemRoot, rowid, ok := minweightTableRootRowid(item.Key)
		if !ok || itemRoot != root {
			return overlayRow, overlayOK, nil
		}
		if _, ok := bt.txnWriteForKey(item.Key); ok {
			if rowid == math.MinInt64 {
				return overlayRow, overlayOK, nil
			}
			seekKey = minweightTableKey(root, rowid-1)
			continue
		}
		baseRow, ok := minweightTableRowFromItem(root, item)
		if !ok {
			return overlayRow, overlayOK, nil
		}
		row, rowOK := minweightBetterTableLERow(baseRow, true, overlayRow, overlayOK)
		return row, rowOK, nil
	}
}

func (bt *minweightBtree) clearAllItems() error {
	bt.mu.Lock()
	tx := bt.activeTxnLocked()
	if tx != nil {
		tx.writes = map[string]minweightTxnWrite{}
	}
	bt.mu.Unlock()
	if tx != nil {
		var scanErr error
		if err := bt.store.Scan(func(item minweight.Item) bool {
			bt.mu.Lock()
			if tx := bt.activeTxnLocked(); tx != nil {
				tx.writes[string(item.Key)] = minweightTxnWrite{
					key:     append([]byte(nil), item.Key...),
					deleted: true,
				}
			} else {
				scanErr = errors.New("minweight sqlite transaction closed during clear")
			}
			bt.mu.Unlock()
			return scanErr == nil
		}); err != nil {
			return err
		}
		return scanErr
	}
	if bt.path == "" {
		bt.store = minweight.New()
		return nil
	}
	var batch minweight.WriteBatch
	var batchErr error
	if err := bt.store.Scan(func(item minweight.Item) bool {
		if batchErr = batch.Delete(item.Key); batchErr != nil {
			return false
		}
		return true
	}); err != nil {
		return err
	}
	if batchErr != nil {
		return batchErr
	}
	return bt.store.WriteBatch(batch)
}

func (bt *minweightBtree) noteInsert(root uint32, rowid int64, existed bool) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.updateStateLocked(func(state *minweightDBState) {
		table := state.tables[root]
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
		state.tables[root] = table
	})
}

func (bt *minweightBtree) resetTableStats(root uint32) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.updateStateLocked(func(state *minweightDBState) {
		table := state.tables[root]
		table.rowCount = 0
		table.minRowid = 0
		table.maxRowid = 0
		state.tables[root] = table
	})
}

func (bt *minweightBtree) recomputeIntKeyStats(root uint32) error {
	var rowCount int64
	var minRowid int64
	var maxRowid int64
	row, ok, err := bt.seekTableGE(root, math.MinInt64)
	for ok {
		if rowCount == 0 {
			minRowid = row.rowid
		}
		maxRowid = row.rowid
		rowCount++
		if row.rowid == math.MaxInt64 {
			break
		}
		row, ok, err = bt.seekTableGE(root, row.rowid+1)
	}
	if err != nil {
		return err
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.updateStateLocked(func(state *minweightDBState) {
		table := state.tables[root]
		table.rowCount = rowCount
		if rowCount == 0 {
			table.minRowid = 0
			table.maxRowid = 0
		} else {
			table.minRowid = minRowid
			table.maxRowid = maxRowid
		}
		state.tables[root] = table
	})
	return nil
}

func (bt *minweightBtree) noteDelete(root uint32, row minweightRow, deleted bool, intKey bool) error {
	if !deleted {
		return nil
	}
	bt.mu.Lock()
	state := bt.visibleStateLocked()
	table := state.tables[root]
	if !intKey {
		if table.rowCount > 0 {
			table.rowCount--
		}
		bt.updateStateLocked(func(state *minweightDBState) {
			state.tables[root] = table
		})
		bt.mu.Unlock()
		return nil
	}
	if table.rowCount > 0 {
		table.rowCount--
	}
	if table.rowCount == 0 {
		table.minRowid = 0
		table.maxRowid = 0
		bt.updateStateLocked(func(state *minweightDBState) {
			state.tables[root] = table
		})
		bt.mu.Unlock()
		return nil
	}
	if row.rowid != table.minRowid && row.rowid != table.maxRowid {
		bt.updateStateLocked(func(state *minweightDBState) {
			state.tables[root] = table
		})
		bt.mu.Unlock()
		return nil
	}
	bt.mu.Unlock()
	return bt.recomputeIntKeyStats(root)
}

func (bt *minweightBtree) ensureSavepoints(n int32) error {
	if n <= 0 {
		return nil
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	tx := bt.activeTxnLocked()
	if tx == nil {
		return nil
	}
	for len(tx.savepoints) < int(n) {
		tx.savepoints = append(tx.savepoints, tx.cloneSavepoint())
	}
	return nil
}

func (bt *minweightBtree) sqliteSavepointCount() int32 {
	if bt.db == 0 {
		return 0
	}
	return minweightSQLiteFromPointer(bt.db).FnSavepoint
}

func (bt *minweightBtree) retainReaderLocked() {
	bt.retainReaderViewLocked(bt)
	bt.readers[bt]++
}

func (bt *minweightBtree) retainReader() {
	bt.mu.Lock()
	bt.retainReaderLocked()
	bt.mu.Unlock()
}

func (bt *minweightBtree) releaseReader() {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	readers := bt.readers[bt]
	if readers <= 1 {
		bt.releaseAllReaderStateLocked(bt)
		return
	}
	bt.readers[bt] = readers - 1
}

func (bt *minweightBtree) invokeBusyHandler(ctx BtreeContext) bool {
	if bt.db == 0 {
		return false
	}
	pBusy := bt.db + unsafe.Offsetof(Tsqlite3{}.FbusyHandler)
	return _sqlite3InvokeBusyHandler(ctx.tls, pBusy) != 0
}

func (bt *minweightBtree) beginTrans(ctx BtreeContext, wrflag int32) (int32, bool) {
	if wrflag != 0 {
		for {
			bt.mu.Lock()
			if bt.writer == nil {
				bt.releaseAllReaderStateLocked(bt)
				bt.writer = bt
				bt.txnState = SQLITE_TXN_WRITE
				bt.txn = bt.newTxnLocked()
				bt.mu.Unlock()
				return SQLITE_OK, true
			}
			if bt.writer == bt {
				if bt.txnState != SQLITE_TXN_WRITE {
					bt.txnState = SQLITE_TXN_WRITE
					if bt.txn == nil {
						bt.txn = bt.newTxnLocked()
					}
					bt.mu.Unlock()
					return SQLITE_OK, true
				}
				if bt.txn == nil {
					bt.txn = bt.newTxnLocked()
				}
				bt.mu.Unlock()
				return SQLITE_OK, false
			}
			bt.mu.Unlock()
			if !bt.invokeBusyHandler(ctx) {
				return SQLITE_BUSY, false
			}
		}
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if bt.db != 0 && minweightSQLiteFromPointer(bt.db).FautoCommit == 0 && bt.txnState == SQLITE_TXN_NONE {
		bt.retainReaderLocked()
		bt.txnState = SQLITE_TXN_READ
	}
	return SQLITE_OK, false
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
	bt.releaseAllReaderStateLocked(bt)
	if bt.writer == bt {
		bt.writer = nil
	}
	bt.clearTableLocksLocked()
	bt.txnState = SQLITE_TXN_NONE
	bt.txn = nil
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
		if !bt.invokeBusyHandler(ctx) {
			return SQLITE_BUSY
		}
	}
}

func minweightTxnWriteKeys(writes map[string]minweightTxnWrite) []string {
	keys := make([]string, 0, len(writes))
	for key := range writes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (db *minweightDatabase) txnReadConflictLocked(tx *minweightTxn, stateChanged bool) bool {
	if tx.baseGeneration == db.generation {
		return false
	}
	for _, change := range db.changes {
		if change.generation <= tx.baseGeneration {
			continue
		}
		if (tx.readMeta || stateChanged) && change.meta {
			return true
		}
		for key := range tx.reads {
			if _, ok := change.keys[key]; ok {
				return true
			}
		}
		for root := range tx.readRoots {
			if _, ok := change.roots[root]; ok {
				return true
			}
		}
	}
	return false
}

func (db *minweightDatabase) collectCommitChangeLocked(tx *minweightTxn, beforeState minweightDBState, afterState minweightDBState, stateChanged bool) (minweightCommitChange, error) {
	change := minweightCommitChange{
		keys:        map[string]minweightCommittedKeyChange{},
		roots:       map[uint32]struct{}{},
		meta:        stateChanged,
		beforeState: beforeState,
		afterState:  minweightCloneState(afterState),
	}
	for _, key := range minweightTxnWriteKeys(tx.writes) {
		write := tx.writes[key]
		before, beforeExists, err := db.store.Get(write.key)
		if err != nil {
			return minweightCommitChange{}, err
		}
		keyChange := minweightCommittedKeyChange{
			key:         append([]byte(nil), write.key...),
			before:      append([]byte(nil), before...),
			beforeExist: beforeExists,
		}
		if !write.deleted {
			keyChange.after = append([]byte(nil), write.value...)
			keyChange.afterExists = true
		}
		change.keys[key] = keyChange
		if root, ok := minweightKeyRoot(write.key); ok {
			change.roots[root] = struct{}{}
		}
	}
	return change, nil
}

func (db *minweightDatabase) publishCommitChangeLocked(change minweightCommitChange) {
	db.generation++
	change.generation = db.generation
	db.changes = append(db.changes, change)
	db.pruneCommitChangesLocked()
}

func (bt *minweightBtree) commitActiveWriteTxn() error {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	tx := bt.activeTxnLocked()
	if tx == nil {
		return nil
	}
	beforeState := bt.stateLocked()
	stateChanged := !minweightStatesEqual(beforeState, tx.state)
	if len(tx.writes) == 0 && !stateChanged {
		bt.txn = nil
		return nil
	}
	if bt.txnReadConflictLocked(tx, stateChanged) {
		return errMinweightTxnConflict
	}
	change, err := bt.collectCommitChangeLocked(tx, beforeState, tx.state, stateChanged)
	if err != nil {
		return err
	}
	var batch minweight.WriteBatch
	for _, key := range minweightTxnWriteKeys(tx.writes) {
		write := tx.writes[key]
		if write.deleted {
			err = batch.Delete(write.key)
		} else {
			err = batch.Put(write.key, write.value)
		}
		if err != nil {
			return err
		}
	}
	if bt.path != "" {
		if err := batch.Put(minweightMetaKey, minweightEncodeDatabaseState(tx.state)); err != nil {
			return err
		}
	}
	if err := bt.store.WriteBatch(batch); err != nil {
		return err
	}
	bt.applyStateLocked(tx.state)
	bt.publishCommitChangeLocked(change)
	bt.txn = nil
	return nil
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

func (c *minweightCursor) setCurrent(row minweightRow) {
	c.rows = []minweightRow{row}
	c.index = 0
	c.valid = true
	c.dataVer = c.btree.visibleDataVer()
	c.hasLastRow = false
	c.incrblobInvalid = false
}

func (c *minweightCursor) clearCurrent() {
	c.valid = false
	c.index = -1
	c.hasLastRow = false
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
	copy(minweightByteSliceFromPointer(buf, len(key)), key)
	cmp := _sqlite3VdbeRecordCompare(ctx.tls, int32(len(key)), buf, pIdxKey)
	Xsqlite3_free(ctx.tls, buf)
	if minweightUnpackedRecordFromPointer(pIdxKey).FerrCode != uint8(SQLITE_OK) {
		return cmp, SQLITE_CORRUPT
	}
	return cmp, SQLITE_OK
}

func minweightCompareIndexRows(ctx BtreeContext, keyInfo uintptr, a []byte, b []byte) (int32, int32) {
	pIdxKey := _sqlite3VdbeAllocUnpackedRecord(ctx.tls, keyInfo)
	if pIdxKey == 0 {
		return 0, SQLITE_NOMEM
	}
	pIdxRecord := minweightUnpackedRecordFromPointer(pIdxKey)
	nMem := int(minweightKeyInfoFromPointer(keyInfo).FnKeyField) + 1
	clear(minweightByteSliceFromPointer(pIdxRecord.FaMem, int(unsafe.Sizeof(TMem{}))*nMem))
	pIdxRecord.FerrCode = uint8(SQLITE_OK)
	pIdxRecord.FeqSeen = uint8(0)
	buf := _sqlite3MallocZero(ctx.tls, uint64(len(b)+18))
	if buf == 0 {
		_sqlite3DbFreeNN(ctx.tls, minweightKeyInfoFromPointer(keyInfo).Fdb, pIdxKey)
		return 0, SQLITE_NOMEM
	}
	copy(minweightByteSliceFromPointer(buf, len(b)), b)
	_sqlite3VdbeRecordUnpack(ctx.tls, int32(len(b)), buf, pIdxKey)
	cmp, rc := minweightCompareIndexKey(ctx, a, pIdxKey)
	Xsqlite3_free(ctx.tls, buf)
	_sqlite3DbFreeNN(ctx.tls, minweightKeyInfoFromPointer(keyInfo).Fdb, pIdxKey)
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
	cur.dataVer = cur.btree.visibleDataVer()
	if !cur.intKey {
		keyInfo := minweightBtCursorFromPointer(pCur.ptr).FpKeyInfo
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
		raw := minweightBtCursorFromPointer(ptr)
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
	if cur.valid && cur.dataVer != cur.btree.visibleDataVer() {
		return 1
	}
	return 0
}

func (e *minweightStorageEngine) BtreeFakeValidCursor(ctx BtreeContext) (r BtreeCursorHandle) {
	return BtreeCursorHandle{}
}

func minweightRestoreIndexCursor(cur *minweightCursor, row minweightRow) (int32, error) {
	if !minweightIndexKeyVersionedForRoot(cur.root, row.storeKey) {
		return 0, minweightCorruptMetadata("index cursor restore missing versioned store key")
	}
	payload, ok, err := cur.btree.get(row.storeKey)
	if err != nil {
		return 0, err
	}
	if ok {
		cur.setCurrent(minweightRow{
			key:      append([]byte(nil), payload...),
			storeKey: append([]byte(nil), row.storeKey...),
			payload:  append([]byte(nil), payload...),
		})
		return 0, nil
	}
	next, ok, err := cur.btree.seekIndexGE(cur.root, row.storeKey, false)
	if err != nil {
		return 0, err
	}
	if ok {
		cur.setCurrent(next)
	} else {
		cur.clearCurrent()
		cur.lastRow = row
		cur.hasLastRow = true
	}
	return 1, nil
}

func minweightSeekIndexAtOrAfterOldIndex(cur *minweightCursor, oldIndex int) (minweightRow, bool, error) {
	if oldIndex < len(cur.rows) && minweightIndexKeyVersionedForRoot(cur.root, cur.rows[oldIndex].storeKey) {
		return cur.btree.seekIndexGE(cur.root, cur.rows[oldIndex].storeKey, false)
	}
	if oldIndex > 0 && oldIndex-1 < len(cur.rows) && minweightIndexKeyVersionedForRoot(cur.root, cur.rows[oldIndex-1].storeKey) {
		return cur.btree.seekIndexGE(cur.root, cur.rows[oldIndex-1].storeKey, true)
	}
	if len(cur.rows) == 0 && oldIndex == 0 {
		return cur.btree.seekIndexGE(cur.root, minweightVersionedIndexLower(cur.root), false)
	}
	return minweightRow{}, false, nil
}

func minweightSeekIndexBeforeOldIndex(cur *minweightCursor, oldIndex int) (minweightRow, bool, error) {
	if oldIndex < len(cur.rows) && minweightIndexKeyVersionedForRoot(cur.root, cur.rows[oldIndex].storeKey) {
		return cur.btree.seekIndexLE(cur.root, cur.rows[oldIndex].storeKey, true)
	}
	if oldIndex > 0 && oldIndex-1 < len(cur.rows) && minweightIndexKeyVersionedForRoot(cur.root, cur.rows[oldIndex-1].storeKey) {
		return cur.btree.seekIndexLE(cur.root, cur.rows[oldIndex-1].storeKey, false)
	}
	return minweightRow{}, false, nil
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
	if cur.valid && cur.dataVer != cur.btree.visibleDataVer() {
		row, ok := cur.current()
		if !ok {
			cur.valid = false
			differentRow = 1
		} else if cur.intKey {
			payload, ok, err := cur.btree.get(minweightTableKey(cur.root, row.rowid))
			if err != nil {
				return minweightSQLiteError(err)
			}
			if ok {
				cur.setCurrent(minweightRow{rowid: row.rowid, storeKey: minweightTableKey(cur.root, row.rowid), payload: payload})
			} else {
				next, ok, err := cur.btree.seekTableGE(cur.root, row.rowid)
				if err != nil {
					return minweightSQLiteError(err)
				}
				if ok {
					cur.setCurrent(next)
				} else {
					cur.clearCurrent()
					cur.lastRow = row
					cur.hasLastRow = true
				}
				differentRow = 1
			}
		} else {
			moved, err := minweightRestoreIndexCursor(cur, row)
			if err != nil {
				return minweightSQLiteError(err)
			}
			differentRow = moved
		}
	}
	minweightWriteResult(pDifferentRow, differentRow)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCursorHintFlags(ctx BtreeContext, pCur BtreeCursorHandle, x uint32) {
	minweightBtCursorFromPointer(pCur.ptr).Fhints = uint8(x)
}

func (e *minweightStorageEngine) BtreeLastPage(ctx BtreeContext, p BtreeHandle) (r uint32) {
	return e.btree(p).visibleState().next
}

func (e *minweightStorageEngine) BtreeOpen(ctx BtreeContext, pVfs BtreeVFSHandle, zFilename BtreeCStringHandle, db SQLiteHandle, ppBtree BtreeMemoryHandle, flags int32, vfsFlags int32) (r int32) {
	key := minweightDatabaseKey(zFilename)
	readOnly := vfsFlags&SQLITE_OPEN_READONLY != 0
	sharable := vfsFlags&SQLITE_OPEN_SHAREDCACHE != 0
	database, rc := e.openDatabase(key, readOnly)
	if rc != SQLITE_OK {
		return rc
	}
	pager := _sqlite3MallocZero(ctx.tls, uint64(unsafe.Sizeof(Pager{})))
	file := _sqlite3MallocZero(ctx.tls, uint64(unsafe.Sizeof(minweightFile{})))
	journal := _sqlite3MallocZero(ctx.tls, uint64(unsafe.Sizeof(Tsqlite3_file{})))
	minweightSQLiteFileFromPointer(file).FpMethods = uintptr(unsafe.Pointer(&minweightFileMethods))
	rawPager := minweightPagerFromPointer(pager)
	rawPager.FpageSize = 4096
	rawPager.FjournalMode = PAGER_JOURNALMODE_DELETE
	rawPager.Ffd = file
	rawPager.Fjfd = journal
	rawPager.FnoLock = 1
	rawPager.FreadOnly = libc.Uint8FromInt32(libc.BoolInt32(readOnly))
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
			e.releaseDatabase(database)
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
	token := e.nextToken()
	e.btrees[token] = bt
	minweightFileFromPointer(file).btreeToken = token
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
		if rc := e.releaseDatabase(bt.minweightDatabase); rc != SQLITE_OK {
			return rc
		}
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

func (e *minweightStorageEngine) MarkReadOnly(ctx BtreeContext, db SQLiteHandle) int32 {
	bt := e.btreeForDB(db)
	if bt == nil {
		return SQLITE_ERROR
	}
	bt.mu.Lock()
	bt.readOnly = true
	if bt.pager != 0 {
		minweightPagerFromPointer(bt.pager).FreadOnly = 1
	}
	bt.mu.Unlock()
	return SQLITE_OK
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

func (e *minweightStorageEngine) SaveLogicalMetadata(ctx BtreeContext, db SQLiteHandle) (StorageEngineLogicalMetadata, int32) {
	bt := e.btreeForDB(db)
	if bt == nil {
		return StorageEngineLogicalMetadata{}, SQLITE_ERROR
	}
	state := bt.visibleState()
	return StorageEngineLogicalMetadata{
		NextRoot:  state.next,
		FreeRoots: minweightCloneRootList(state.freeRoots),
	}, SQLITE_OK
}

func (e *minweightStorageEngine) RestoreLogicalMetadata(ctx BtreeContext, db SQLiteHandle, meta StorageEngineLogicalMetadata) int32 {
	bt := e.btreeForDB(db)
	if bt == nil {
		return SQLITE_ERROR
	}
	if meta.NextRoot < uint32(SCHEMA_ROOT) {
		return SQLITE_CORRUPT
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	state := bt.visibleStateLocked()
	seen := map[uint32]bool{}
	for root := range state.tables {
		if root > meta.NextRoot {
			return SQLITE_CORRUPT
		}
		seen[root] = true
	}
	freeRoots := minweightCloneRootList(meta.FreeRoots)
	for _, root := range freeRoots {
		if root <= uint32(SCHEMA_ROOT) || root > meta.NextRoot || seen[root] {
			return SQLITE_CORRUPT
		}
		seen[root] = true
	}
	bt.updateStateLocked(func(state *minweightDBState) {
		state.next = meta.NextRoot
		state.freeRoots = freeRoots
		state.meta[BTREE_LARGEST_ROOT_PAGE] = meta.NextRoot
	})
	if err := bt.persistMetadataLocked(); err != nil {
		return minweightSQLiteError(err)
	}
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeSetCacheSize(ctx BtreeContext, p BtreeHandle, mxPage int32) (r int32) {
	bt := e.btree(p)
	bt.mu.Lock()
	bt.updateStateLocked(func(state *minweightDBState) {
		state.cacheSize = mxPage
	})
	bt.mu.Unlock()
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeSetSpillSize(ctx BtreeContext, p BtreeHandle, mxPage int32) (r int32) {
	bt := e.btree(p)
	bt.mu.Lock()
	state := bt.visibleStateLocked()
	if mxPage != 0 {
		bt.updateStateLocked(func(state *minweightDBState) {
			state.spillSize = mxPage
		})
		state = bt.visibleStateLocked()
	}
	spillSize := minweightEffectiveSpillSize(state.pageSize, state.cacheSize, state.spillSize)
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
	state := bt.visibleStateLocked()
	if nReserve < 0 {
		nReserve = state.reserve
	} else {
		bt.updateStateLocked(func(state *minweightDBState) {
			state.reserveWanted = nReserve
		})
		state = bt.visibleStateLocked()
	}
	if state.reserve == nReserve && (pageSize == 0 || pageSize == state.pageSize) {
		return SQLITE_OK
	}
	if nReserve < state.reserve {
		nReserve = state.reserve
	}
	if state.pageSizeFixed {
		return SQLITE_READONLY
	}
	bt.updateStateLocked(func(state *minweightDBState) {
		if minweightValidPageSize(pageSize) {
			if nReserve > 32 && pageSize == 512 {
				pageSize = 1024
			}
			state.pageSize = pageSize
		}
		state.reserve = nReserve
		if iFix != 0 {
			state.pageSizeFixed = true
		}
	})
	if err := bt.persistMetadataLocked(); err != nil {
		return minweightSQLiteError(err)
	}
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeGetPageSize(ctx BtreeContext, p BtreeHandle) (r int32) {
	return e.btree(p).visibleState().pageSize
}
func (e *minweightStorageEngine) BtreeGetReserveNoMutex(ctx BtreeContext, p BtreeHandle) (r int32) {
	return e.btree(p).visibleState().reserve
}
func (e *minweightStorageEngine) BtreeGetRequestedReserve(ctx BtreeContext, p BtreeHandle) (r int32) {
	state := e.btree(p).visibleState()
	reserve := state.reserve
	if state.reserveWanted > reserve {
		reserve = state.reserveWanted
	}
	return reserve
}
func (e *minweightStorageEngine) BtreeMaxPageCount(ctx BtreeContext, p BtreeHandle, mxPage uint32) (r uint32) {
	bt := e.btree(p)
	bt.mu.Lock()
	if mxPage > 0 {
		bt.updateStateLocked(func(state *minweightDBState) {
			state.maxPageCount = mxPage
		})
	}
	maxPageCount := bt.visibleStateLocked().maxPageCount
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
		bt.updateStateLocked(func(state *minweightDBState) {
			state.secureDelete = newFlag
		})
		if err := bt.persistMetadataLocked(); err != nil {
			bt.mu.Unlock()
			return minweightSQLiteError(err)
		}
	}
	secureDelete := bt.visibleStateLocked().secureDelete
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
	state := bt.visibleStateLocked()
	if state.pageSizeFixed && (autoVacuum != 0) != (state.autoVacuum != BTREE_AUTOVACUUM_NONE) {
		return SQLITE_READONLY
	}
	bt.updateStateLocked(func(state *minweightDBState) {
		if autoVacuum == BTREE_AUTOVACUUM_INCR {
			state.autoVacuum = BTREE_AUTOVACUUM_INCR
		} else if autoVacuum != 0 {
			state.autoVacuum = BTREE_AUTOVACUUM_FULL
		} else {
			state.autoVacuum = BTREE_AUTOVACUUM_NONE
		}
	})
	if err := bt.persistMetadataLocked(); err != nil {
		return minweightSQLiteError(err)
	}
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeGetAutoVacuum(ctx BtreeContext, p BtreeHandle) (r int32) {
	return e.btree(p).visibleState().autoVacuum
}

func (e *minweightStorageEngine) BtreeNewDb(ctx BtreeContext, p BtreeHandle) (r int32) {
	bt := e.btree(p)
	if rc := bt.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	if err := bt.clearAllItems(); err != nil {
		return minweightSQLiteError(err)
	}
	bt.mu.Lock()
	bt.updateStateLocked(func(state *minweightDBState) {
		state.tables = map[uint32]minweightTable{1: {intKey: true}}
		state.next = 1
		state.freeRoots = nil
		state.meta = [SQLITE_N_BTREE_META]uint32{}
		state.dataVer++
	})
	if err := bt.persistMetadataLocked(); err != nil {
		bt.mu.Unlock()
		return minweightSQLiteError(err)
	}
	bt.mu.Unlock()
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeBeginTrans(ctx BtreeContext, p BtreeHandle, wrflag int32, pSchemaVersion BtreeMemoryHandle) (r int32) {
	bt := e.btree(p)
	if !pSchemaVersion.IsNil() {
		pSchemaVersion.PutUint32(bt.visibleMeta(BTREE_SCHEMA_VERSION))
	}
	if wrflag != 0 {
		if rc := bt.ensureWritable(); rc != SQLITE_OK {
			return rc
		}
	}
	if wrflag != 0 {
		rc, acquired := bt.beginTrans(ctx, wrflag)
		if rc != SQLITE_OK {
			return rc
		}
		if rc := bt.lockTable(uint32(SCHEMA_ROOT), uint8(READ_LOCK)); rc != SQLITE_OK {
			if acquired {
				bt.releaseTrans()
			}
			return rc
		}
		if err := bt.ensureSavepoints(bt.sqliteSavepointCount()); err != nil {
			if acquired {
				bt.releaseTrans()
			}
			return minweightSQLiteError(err)
		}
		if rc := bt.createWALPlaceholder(); rc != SQLITE_OK {
			if acquired {
				bt.releaseTrans()
			}
			return rc
		}
		return SQLITE_OK
	}
	if rc := bt.lockTable(uint32(SCHEMA_ROOT), uint8(READ_LOCK)); rc != SQLITE_OK {
		return rc
	}
	bt.beginTrans(ctx, wrflag)
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeIncrVacuum(ctx BtreeContext, p BtreeHandle) (r int32) {
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeCommitPhaseOne(ctx BtreeContext, p BtreeHandle, zSuperJrnl BtreeCStringHandle) (r int32) {
	return e.btree(p).commitPhaseOne(ctx)
}
func (e *minweightStorageEngine) BtreeCommitPhaseTwo(ctx BtreeContext, p BtreeHandle, bCleanup int32) (r int32) {
	bt := e.btree(p)
	if err := bt.commitActiveWriteTxn(); err != nil {
		return minweightSQLiteError(err)
	}
	bt.releaseTrans()
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeCommit(ctx BtreeContext, p BtreeHandle) (r int32) {
	bt := e.btree(p)
	if err := bt.commitActiveWriteTxn(); err != nil {
		return minweightSQLiteError(err)
	}
	bt.releaseTrans()
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
			if tx := bt.activeTxnLocked(); tx != nil {
				tx.state = bt.stateLocked()
				tx.writes = map[string]minweightTxnWrite{}
				tx.savepoints = nil
			}
			bt.mu.Unlock()
		}
		return SQLITE_OK
	}
	bt.mu.Lock()
	tx := bt.activeTxnLocked()
	if tx == nil {
		bt.mu.Unlock()
		return SQLITE_OK
	}
	savepoint := int(iSavepoint)
	if savepoint >= len(tx.savepoints) {
		bt.mu.Unlock()
		return SQLITE_OK
	}
	s := tx.savepoints[savepoint]
	if op == SAVEPOINT_RELEASE {
		tx.savepoints = tx.savepoints[:savepoint]
		bt.mu.Unlock()
		return SQLITE_OK
	}
	tx.savepoints = tx.savepoints[:savepoint+1]
	tx.restoreSavepoint(s)
	bt.mu.Unlock()
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCursor(ctx BtreeContext, p BtreeHandle, iTable uint32, wrFlag int32, pKeyInfo BtreeKeyInfoHandle, pCur BtreeCursorHandle) (r int32) {
	bt := e.btree(p)
	if wrFlag != 0 {
		if rc := bt.ensureWritable(); rc != SQLITE_OK {
			return rc
		}
	}
	table, ok := bt.visibleTable(iTable)
	if !ok {
		table = minweightTable{intKey: pKeyInfo.IsNil()}
		if wrFlag != 0 {
			bt.mu.Lock()
			bt.updateStateLocked(func(state *minweightDBState) {
				state.tables[iTable] = table
			})
			bt.mu.Unlock()
		}
	}
	rawCursor := minweightBtCursorFromPointer(pCur.ptr)
	*rawCursor = BtCursor{}
	rawCursor.FpBtree = p.ptr
	rawCursor.FpgnoRoot = TPgno(iTable)
	rawCursor.FcurIntKey = libc.Uint8FromInt32(libc.BoolInt32(table.intKey))
	rawCursor.FpKeyInfo = pKeyInfo.ptr
	rawCursor.FiPage = -1
	if wrFlag != 0 {
		rawCursor.FcurFlags |= uint8(BTCF_WriteFlag)
	}
	cur := &minweightCursor{
		btree:    bt,
		root:     iTable,
		intKey:   table.intKey,
		writable: wrFlag != 0,
		index:    -1,
	}
	if !cur.writable {
		bt.retainReader()
		cur.readTracked = true
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
	*minweightBtCursorFromPointer(p.ptr) = BtCursor{}
}

func (e *minweightStorageEngine) BtreeCloseCursor(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	e.mu.Lock()
	cur := e.cursors[pCur.ptr]
	delete(e.cursors, pCur.ptr)
	e.mu.Unlock()
	if cur != nil {
		if cur.readTracked {
			cur.btree.releaseReader()
			cur.readTracked = false
		}
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
	minweightBtCursorFromPointer(pCur.ptr).FcurFlags |= uint8(BTCF_Pinned)
}
func (e *minweightStorageEngine) BtreeCursorUnpin(ctx BtreeContext, pCur BtreeCursorHandle) {
	minweightBtCursorFromPointer(pCur.ptr).FcurFlags &^= uint8(BTCF_Pinned)
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
		copy(minweightByteSliceFromPointer(cur.payloadBuf, len(data)), data)
	}
	pAmt.PutInt32(int32(len(data)))
	return btreeMemoryHandle(ctx.tls, cur.payloadBuf)
}

func (e *minweightStorageEngine) BtreeFirst(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	if cur.intKey {
		table, _ := cur.btree.visibleTable(cur.root)
		if table.rowCount == 0 {
			cur.clearCurrent()
			minweightWriteResult(pRes, 1)
			return SQLITE_OK
		}
		payload, ok, err := cur.btree.get(minweightTableKey(cur.root, table.minRowid))
		if err != nil {
			return minweightSQLiteError(err)
		}
		if !ok {
			return SQLITE_CORRUPT
		}
		cur.setCurrent(minweightRow{rowid: table.minRowid, storeKey: minweightTableKey(cur.root, table.minRowid), payload: payload})
		minweightWriteResult(pRes, 0)
		return SQLITE_OK
	}
	row, ok, err := cur.btree.seekIndexGE(cur.root, minweightVersionedIndexLower(cur.root), false)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if ok {
		cur.setCurrent(row)
		minweightWriteResult(pRes, 0)
		return SQLITE_OK
	}
	table, _ := cur.btree.visibleTable(cur.root)
	if table.rowCount == 0 {
		cur.clearCurrent()
		minweightWriteResult(pRes, 1)
		return SQLITE_OK
	}
	return SQLITE_CORRUPT
}

func (e *minweightStorageEngine) BtreeLast(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	if cur.intKey {
		table, _ := cur.btree.visibleTable(cur.root)
		if table.rowCount == 0 {
			cur.clearCurrent()
			minweightWriteResult(pRes, 1)
			return SQLITE_OK
		}
		payload, ok, err := cur.btree.get(minweightTableKey(cur.root, table.maxRowid))
		if err != nil {
			return minweightSQLiteError(err)
		}
		if !ok {
			return SQLITE_CORRUPT
		}
		cur.setCurrent(minweightRow{rowid: table.maxRowid, storeKey: minweightTableKey(cur.root, table.maxRowid), payload: payload})
		minweightWriteResult(pRes, 0)
		return SQLITE_OK
	}
	row, ok, err := cur.btree.seekIndexLE(cur.root, minweightVersionedIndexUpper(cur.root), false)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if ok {
		cur.setCurrent(row)
		minweightWriteResult(pRes, 0)
		return SQLITE_OK
	}
	table, _ := cur.btree.visibleTable(cur.root)
	if table.rowCount == 0 {
		cur.clearCurrent()
		minweightWriteResult(pRes, 1)
		return SQLITE_OK
	}
	return SQLITE_CORRUPT
}

func (e *minweightStorageEngine) BtreeTableMoveto(ctx BtreeContext, pCur BtreeCursorHandle, intKey int64, biasRight int32, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	row, ok, err := cur.btree.seekTableGE(cur.root, intKey)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if ok {
		cur.setCurrent(row)
		if row.rowid == intKey {
			minweightWriteResult(pRes, 0)
		} else {
			minweightWriteResult(pRes, 1)
		}
		return SQLITE_OK
	}
	cur.clearCurrent()
	cur.lastRow = minweightRow{rowid: intKey}
	cur.hasLastRow = true
	cur.dataVer = cur.btree.visibleDataVer()
	minweightWriteResult(pRes, -1)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeIndexMoveto(ctx BtreeContext, pCur BtreeCursorHandle, pIdxKey BtreeIndexKeyHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	rec := minweightUnpackedRecordFromPointer(pIdxKey.ptr)
	rec.FerrCode = uint8(SQLITE_OK)
	rec.FeqSeen = uint8(0)
	probeKey, err := minweightComparableIndexProbeKey(ctx, cur.root, pIdxKey.ptr)
	if err != nil {
		return minweightSQLiteError(err)
	}
	for {
		row, ok, err := cur.btree.seekIndexGE(cur.root, probeKey, false)
		if err != nil {
			return minweightSQLiteError(err)
		}
		if !ok {
			cur.valid = false
			cur.index = 0
			minweightWriteResult(pRes, -1)
			return SQLITE_OK
		}
		cmp, rc := minweightCompareIndexKey(ctx, row.key, pIdxKey.ptr)
		if rc != SQLITE_OK {
			return rc
		}
		if cmp >= 0 {
			cur.setCurrent(row)
			minweightWriteResult(pRes, cmp)
			return SQLITE_OK
		}
		probeKey = minweightIndexSeekAfter(row.storeKey)
	}
}

func (e *minweightStorageEngine) BtreeEof(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.valid {
		return 0
	}
	if cur.intKey {
		if cur.hasLastRow && cur.dataVer != cur.btree.visibleDataVer() && cur.lastRow.rowid != math.MaxInt64 {
			row, ok, err := cur.btree.seekTableGE(cur.root, cur.lastRow.rowid+1)
			if err != nil {
				return 1
			}
			if ok {
				cur.setCurrent(row)
				return 0
			}
		}
		return 1
	}
	oldIndex := cur.index
	if cur.hasLastRow {
		if minweightIndexKeyVersionedForRoot(cur.root, cur.lastRow.storeKey) {
			row, ok, err := cur.btree.seekIndexGE(cur.root, cur.lastRow.storeKey, true)
			if err != nil {
				return 1
			}
			if ok {
				cur.setCurrent(row)
				return 0
			}
			return 1
		}
		if cur.dataVer != cur.btree.visibleDataVer() {
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
		row, ok, err := minweightSeekIndexAtOrAfterOldIndex(cur, oldIndex)
		if err != nil {
			return 1
		}
		if ok {
			cur.setCurrent(row)
			return 0
		}
		if cur.dataVer != cur.btree.visibleDataVer() {
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
	table, _ := cur.btree.visibleTable(cur.root)
	rowCount := table.rowCount
	return rowCount
}

func (e *minweightStorageEngine) BtreeNext(ctx BtreeContext, pCur BtreeCursorHandle, flags int32) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	if cur.intKey {
		if !cur.valid {
			if !cur.hasLastRow || cur.lastRow.rowid == math.MaxInt64 {
				return SQLITE_DONE
			}
			row, ok, err := cur.btree.seekTableGE(cur.root, cur.lastRow.rowid+1)
			if err != nil {
				return minweightSQLiteError(err)
			}
			if !ok {
				return SQLITE_DONE
			}
			cur.setCurrent(row)
			return SQLITE_OK
		}
		row, ok := cur.current()
		if !ok {
			return SQLITE_DONE
		}
		if row.rowid == math.MaxInt64 {
			cur.valid = false
			cur.lastRow = row
			cur.hasLastRow = true
			return SQLITE_DONE
		}
		next, ok, err := cur.btree.seekTableGE(cur.root, row.rowid+1)
		if err != nil {
			return minweightSQLiteError(err)
		}
		if !ok {
			cur.valid = false
			cur.lastRow = row
			cur.hasLastRow = true
			return SQLITE_DONE
		}
		cur.setCurrent(next)
		return SQLITE_OK
	}
	if !cur.valid {
		if cur.hasLastRow && minweightIndexKeyVersionedForRoot(cur.root, cur.lastRow.storeKey) {
			row, ok, err := cur.btree.seekIndexGE(cur.root, cur.lastRow.storeKey, true)
			if err != nil {
				return minweightSQLiteError(err)
			}
			if !ok {
				return SQLITE_DONE
			}
			cur.setCurrent(row)
			return SQLITE_OK
		}
		if cur.hasLastRow {
			if cur.dataVer != cur.btree.visibleDataVer() {
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
			row, ok, err := minweightSeekIndexAtOrAfterOldIndex(cur, oldIndex)
			if err != nil {
				return minweightSQLiteError(err)
			}
			if ok {
				cur.setCurrent(row)
				return SQLITE_OK
			}
			if cur.dataVer != cur.btree.visibleDataVer() {
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
	if ok && minweightIndexKeyVersionedForRoot(cur.root, row.storeKey) {
		next, nextOK, err := cur.btree.seekIndexGE(cur.root, row.storeKey, true)
		if err != nil {
			return minweightSQLiteError(err)
		}
		if !nextOK {
			cur.valid = false
			cur.lastRow = row
			cur.hasLastRow = true
			return SQLITE_DONE
		}
		cur.setCurrent(next)
		return SQLITE_OK
	}
	if cur.dataVer != cur.btree.visibleDataVer() {
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
	if cur.intKey {
		if !cur.valid {
			if !cur.hasLastRow || cur.lastRow.rowid == math.MinInt64 {
				return SQLITE_DONE
			}
			row, ok, err := cur.btree.seekTableLE(cur.root, cur.lastRow.rowid-1)
			if err != nil {
				return minweightSQLiteError(err)
			}
			if !ok {
				return SQLITE_DONE
			}
			cur.setCurrent(row)
			return SQLITE_OK
		}
		row, ok := cur.current()
		if !ok {
			return SQLITE_DONE
		}
		if row.rowid == math.MinInt64 {
			cur.valid = false
			cur.lastRow = row
			cur.hasLastRow = true
			return SQLITE_DONE
		}
		prev, ok, err := cur.btree.seekTableLE(cur.root, row.rowid-1)
		if err != nil {
			return minweightSQLiteError(err)
		}
		if !ok {
			cur.valid = false
			cur.lastRow = row
			cur.hasLastRow = true
			return SQLITE_DONE
		}
		cur.setCurrent(prev)
		return SQLITE_OK
	}
	if !cur.valid {
		if cur.hasLastRow && minweightIndexKeyVersionedForRoot(cur.root, cur.lastRow.storeKey) {
			row, ok, err := cur.btree.seekIndexLE(cur.root, cur.lastRow.storeKey, true)
			if err != nil {
				return minweightSQLiteError(err)
			}
			if !ok {
				return SQLITE_DONE
			}
			cur.setCurrent(row)
			return SQLITE_OK
		}
		if cur.hasLastRow {
			if cur.dataVer != cur.btree.visibleDataVer() {
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
		row, ok, err := minweightSeekIndexBeforeOldIndex(cur, oldIndex)
		if err != nil {
			return minweightSQLiteError(err)
		}
		if ok {
			cur.setCurrent(row)
			return SQLITE_OK
		}
		if cur.dataVer != cur.btree.visibleDataVer() {
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
	if ok && minweightIndexKeyVersionedForRoot(cur.root, row.storeKey) {
		prev, prevOK, err := cur.btree.seekIndexLE(cur.root, row.storeKey, true)
		if err != nil {
			return minweightSQLiteError(err)
		}
		if !prevOK {
			cur.valid = false
			cur.lastRow = row
			cur.hasLastRow = true
			return SQLITE_DONE
		}
		cur.setCurrent(prev)
		return SQLITE_OK
	}
	if cur.dataVer != cur.btree.visibleDataVer() || cur.intKey && len(cur.rows) == 1 {
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
	keyInfo := minweightBtCursorFromPointer(pCur.ptr).FpKeyInfo
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
			var err error
			key, err = minweightIndexStoreKey(ctx, keyInfo, cur.root, payload)
			if err != nil {
				return minweightSQLiteError(err)
			}
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
		var err error
		key, err = minweightIndexStoreKey(ctx, keyInfo, cur.root, payload)
		if err != nil {
			return minweightSQLiteError(err)
		}
	}
	existed := false
	if !cur.intKey && (seekResult == 0 || flags&int32(BTREE_SAVEPOSITION) != 0) {
		if row, ok := cur.current(); ok {
			oldKey := minweightRowIndexStoreKey(cur.root, row)
			if !bytes.Equal(oldKey, key) {
				deleted, err := cur.btree.delete(oldKey)
				if err != nil {
					return minweightSQLiteError(err)
				}
				existed = deleted
			}
		}
	}
	_, keyExisted, err := cur.btree.get(key)
	if err != nil {
		return minweightSQLiteError(err)
	}
	existed = existed || keyExisted
	if err := cur.btree.put(key, payload); err != nil {
		return minweightSQLiteError(err)
	}
	if cur.intKey {
		e.invalidateIncrblobCursors(cur.btree, cur.root, rowid, false)
	}
	cur.btree.noteInsert(cur.root, rowid, existed)
	cur.btree.bumpDataVer()
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
		rowid:    row.rowid,
		key:      append([]byte(nil), row.key...),
		storeKey: append([]byte(nil), row.storeKey...),
		payload:  append([]byte(nil), row.payload...),
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
		key = minweightRowIndexStoreKey(cur.root, row)
	}
	if cur.intKey {
		e.invalidateIncrblobCursors(cur.btree, cur.root, row.rowid, false)
	}
	deleted, err := cur.btree.delete(key)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if err := cur.btree.noteDelete(cur.root, row, deleted, cur.intKey); err != nil {
		return minweightSQLiteError(err)
	}
	cur.btree.bumpDataVer()
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCreateTable(ctx BtreeContext, p BtreeHandle, piTable BtreeMemoryHandle, flags int32) (r int32) {
	bt := e.btree(p)
	if rc := bt.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	bt.mu.Lock()
	var root uint32
	bt.updateStateLocked(func(state *minweightDBState) {
		if state.autoVacuum == BTREE_AUTOVACUUM_NONE && len(state.freeRoots) != 0 {
			last := len(state.freeRoots) - 1
			root = state.freeRoots[last]
			state.freeRoots = state.freeRoots[:last]
		} else {
			state.next++
			root = state.next
		}
		state.tables[root] = minweightTable{intKey: flags&int32(BTREE_INTKEY) != 0}
		if root > state.meta[BTREE_LARGEST_ROOT_PAGE] {
			state.meta[BTREE_LARGEST_ROOT_PAGE] = root
		}
	})
	if err := bt.persistMetadataLocked(); err != nil {
		bt.mu.Unlock()
		return minweightSQLiteError(err)
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
	table, _ := bt.visibleTable(uint32(iTable))
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
	bt.bumpDataVer()
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
	cur.btree.bumpDataVer()
	return minweightSQLiteError(err)
}

func (e *minweightStorageEngine) BtreeDropTable(ctx BtreeContext, p BtreeHandle, iTable int32, piMoved BtreeMemoryHandle) (r int32) {
	bt := e.btree(p)
	if rc := bt.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	root := uint32(iTable)
	state := bt.visibleState()
	table := state.tables[root]
	autoVacuum := state.autoVacuum
	lastRoot := state.next
	movedTable := state.tables[lastRoot]
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
	bt.updateStateLocked(func(state *minweightDBState) {
		delete(state.tables, root)
		if autoVacuum != BTREE_AUTOVACUUM_NONE {
			delete(state.tables, lastRoot)
			if moved != 0 {
				state.tables[root] = movedTable
			}
			if lastRoot > 1 {
				state.next = lastRoot - 1
			} else {
				state.next = 1
			}
			state.meta[BTREE_LARGEST_ROOT_PAGE] = state.next
		} else {
			state.freeRoots = append(state.freeRoots, root)
		}
	})
	if err := bt.persistMetadataLocked(); err != nil {
		bt.mu.Unlock()
		return minweightSQLiteError(err)
	}
	bt.mu.Unlock()
	if !piMoved.IsNil() {
		piMoved.PutInt32(int32(moved))
	}
	bt.bumpDataVer()
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeGetMeta(ctx BtreeContext, p BtreeHandle, idx int32, pMeta BtreeMemoryHandle) {
	bt := e.btree(p)
	pMeta.PutUint32(bt.visibleMeta(idx))
}

func (e *minweightStorageEngine) BtreeUpdateMeta(ctx BtreeContext, p BtreeHandle, idx int32, iMeta uint32) (r int32) {
	bt := e.btree(p)
	if rc := bt.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.updateStateLocked(func(state *minweightDBState) {
		if idx >= 0 && idx < int32(len(state.meta)) {
			state.meta[idx] = iMeta
		}
		if idx == int32(BTREE_INCR_VACUUM) && state.autoVacuum != BTREE_AUTOVACUUM_NONE {
			if iMeta != 0 {
				state.autoVacuum = BTREE_AUTOVACUUM_INCR
			} else {
				state.autoVacuum = BTREE_AUTOVACUUM_FULL
			}
		}
		state.dataVer++
	})
	if err := bt.persistMetadataLocked(); err != nil {
		return minweightSQLiteError(err)
	}
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCount(ctx BtreeContext, db SQLiteHandle, pCur BtreeCursorHandle, pnEntry BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	table, _ := cur.btree.visibleTable(cur.root)
	rowCount := table.rowCount
	pnEntry.PutInt64(rowCount)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreePager(ctx BtreeContext, p BtreeHandle) (r BtreePagerHandle) {
	bt := e.btree(p)
	minweightPagerFromPointer(bt.pager).FiDataVersion = bt.visibleDataVer()
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
	if err := cur.btree.put(minweightTableKey(cur.root, row.rowid), payload); err != nil {
		return minweightSQLiteError(err)
	}
	cur.btree.bumpDataVer()
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeIncrblobCursor(ctx BtreeContext, pCur BtreeCursorHandle) {
	cur := e.cursor(pCur)
	cur.incrblob = true
	minweightBtCursorFromPointer(pCur.ptr).FcurFlags |= uint8(BTCF_Incrblob)
}
func (e *minweightStorageEngine) BtreeSetVersion(ctx BtreeContext, pBtree BtreeHandle, iVersion int32) (r int32) {
	bt := e.btree(pBtree)
	if rc := bt.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.updateStateLocked(func(state *minweightDBState) {
		state.meta[BTREE_FILE_FORMAT] = uint32(iVersion)
		state.dataVer++
	})
	if err := bt.persistMetadataLocked(); err != nil {
		return minweightSQLiteError(err)
	}
	return SQLITE_OK
}
func (e *minweightStorageEngine) BtreeCursorHasHint(ctx BtreeContext, pCsr BtreeCursorHandle, mask uint32) (r int32) {
	if uint32(minweightBtCursorFromPointer(pCsr.ptr).Fhints)&mask != 0 {
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
	if err := to.copyContentsFrom(from); err != nil {
		return minweightSQLiteError(err)
	}
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeSetMmapLimit(ctx BtreeContext, p BtreeHandle, szMmap int64) (r int32) {
	bt := e.btree(p)
	bt.mu.Lock()
	mmapSize := minweightClampMmapLimit(Tsqlite3_int64(szMmap))
	bt.mmapSize = int64(mmapSize)
	minweightPagerFromPointer(bt.pager).FszMmap = mmapSize
	bt.mu.Unlock()
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeIsEmpty(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	table, _ := cur.btree.visibleTable(cur.root)
	empty := table.rowCount == 0
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
scanItems:
	for _, item := range snapshot.items {
		if len(item.key) == 0 {
			if !partial && !minweightAddIntegrityError(&errors, mxErr, "minweight malformed empty key") {
				break scanItems
			}
			continue
		}
		switch item.key[0] {
		case minweightTablePrefix:
			if len(item.key) != 13 {
				if !partial && !minweightAddIntegrityError(&errors, mxErr, "minweight malformed table key length %d", len(item.key)) {
					break scanItems
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
					break scanItems
				}
				continue
			}
			if !table.intKey {
				if !minweightAddIntegrityError(&errors, mxErr, "minweight root %d has table key in index btree", root) {
					break scanItems
				}
				continue
			}
			u := binary.BigEndian.Uint64(item.key[5:13]) ^ (1 << 63)
			minweightAddIntegrityRowid(stats, root, int64(u))
		case minweightIndexPrefix:
			if len(item.key) < 5 {
				if !partial && !minweightAddIntegrityError(&errors, mxErr, "minweight malformed index key length %d", len(item.key)) {
					break scanItems
				}
				continue
			}
			root := binary.BigEndian.Uint32(item.key[1:5])
			if !minweightIndexKeyInVersionedRange(root, item.key) {
				if !partial && !minweightAddIntegrityError(&errors, mxErr, "minweight unsupported raw index key in root %d", root) {
					break scanItems
				}
				continue
			}
			if !minweightIntegrityRootChecked(root, partial, selected) {
				continue
			}
			table, ok := snapshot.tables[root]
			if !ok {
				if !minweightAddIntegrityError(&errors, mxErr, "minweight index key references unknown root %d", root) {
					break scanItems
				}
				continue
			}
			if table.intKey {
				if !minweightAddIntegrityError(&errors, mxErr, "minweight root %d has index key in table btree", root) {
					break scanItems
				}
				continue
			}
			minweightAddIntegrityIndexRow(stats, root)
		default:
			if !partial && !minweightAddIntegrityError(&errors, mxErr, "minweight unknown key prefix 0x%02x", item.key[0]) {
				break scanItems
			}
		}
		if int32(len(errors)) >= mxErr {
			break scanItems
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
