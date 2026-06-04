// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"bytes"
	"encoding/binary"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	minweight "github.com/JimChengLin/minweight_store"
	"modernc.org/libc"
)

type minweightBtreeTestHarness struct {
	tls     *libc.TLS
	engine  *minweightStorageEngine
	bt      *minweightBtree
	ctx     BtreeContext
	btree   BtreeHandle
	cursors []*BtCursor
}

func newMinweightBtreeTestHarness(t *testing.T) *minweightBtreeTestHarness {
	t.Helper()
	tls := libc.NewTLS()
	t.Cleanup(tls.Close)
	if rc := Xsqlite3_initialize(tls); rc != SQLITE_OK {
		t.Fatalf("sqlite3_initialize rc = %d, want SQLITE_OK", rc)
	}

	engine := NewMinweightStorageEngine().(*minweightStorageEngine)
	bt := &minweightBtree{minweightDatabase: newMinweightTestDatabase()}
	engine.btrees[1] = bt
	return &minweightBtreeTestHarness{
		tls:    tls,
		engine: engine,
		bt:     bt,
		ctx:    BtreeContext{tls: tls},
		btree:  BtreeHandle{tls: tls, ptr: 1},
	}
}

func newMinweightTestDatabase() *minweightDatabase {
	return minweightNewDatabase(minweight.New(), "")
}

func newMinweightBtreeOut(t *testing.T, tls *libc.TLS) BtreeMemoryHandle {
	t.Helper()
	ptr := _sqlite3MallocZero(tls, uint64(unsafe.Sizeof(uintptr(0))))
	if ptr == 0 {
		t.Fatal("sqlite3 malloc returned nil btree out parameter")
	}
	t.Cleanup(func() { Xsqlite3_free(tls, ptr) })
	return BtreeMemoryHandle{tls: tls, ptr: ptr}
}

func (h *minweightBtreeTestHarness) putRow(t *testing.T, rowid int64, payload []byte) {
	t.Helper()
	if err := h.bt.store.Put(minweightTableKey(1, rowid), payload); err != nil {
		t.Fatal(err)
	}
	h.bt.noteInsert(1, rowid, false)
	h.bt.dataVer++
}

func (h *minweightBtreeTestHarness) cursor(t *testing.T, writable bool) BtreeCursorHandle {
	t.Helper()
	rawCursor := new(BtCursor)
	h.cursors = append(h.cursors, rawCursor)
	cursor := BtreeCursorHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(rawCursor))}
	var wrFlag int32
	if writable {
		wrFlag = 1
	}
	if rc := h.engine.BtreeCursor(h.ctx, h.btree, 1, wrFlag, BtreeKeyInfoHandle{}, cursor); rc != SQLITE_OK {
		t.Fatalf("BtreeCursor rc = %d, want SQLITE_OK", rc)
	}
	return cursor
}

func (h *minweightBtreeTestHarness) indexCursor(t *testing.T, root uint32, keyInfo uintptr) BtreeCursorHandle {
	t.Helper()
	rawCursor := new(BtCursor)
	h.cursors = append(h.cursors, rawCursor)
	cursor := BtreeCursorHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(rawCursor))}
	if rc := h.engine.BtreeCursor(h.ctx, h.btree, root, 0, BtreeKeyInfoHandle{tls: h.tls, ptr: keyInfo}, cursor); rc != SQLITE_OK {
		t.Fatalf("BtreeCursor rc = %d, want SQLITE_OK", rc)
	}
	return cursor
}

func (h *minweightBtreeTestHarness) moveToRow(t *testing.T, cursor BtreeCursorHandle, rowid int64) {
	t.Helper()
	var moveResult int32
	if rc := h.engine.BtreeTableMoveto(h.ctx, cursor, rowid, 0, BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&moveResult))}); rc != SQLITE_OK {
		t.Fatalf("BtreeTableMoveto rc = %d, want SQLITE_OK", rc)
	}
	if moveResult != 0 {
		t.Fatalf("moveResult = %d, want 0", moveResult)
	}
}

func (h *minweightBtreeTestHarness) putIndexRecord(t *testing.T, root uint32, keyInfo uintptr, record []byte) []byte {
	t.Helper()
	key, err := minweightIndexStoreKey(h.ctx, keyInfo, root, record)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.bt.store.Put(key, record); err != nil {
		t.Fatal(err)
	}
	h.bt.mu.Lock()
	h.bt.updateStateLocked(func(state *minweightDBState) {
		table := state.tables[root]
		table.intKey = false
		table.rowCount++
		state.tables[root] = table
	})
	h.bt.mu.Unlock()
	h.bt.dataVer++
	return key
}

func (h *minweightBtreeTestHarness) replaceRow(t *testing.T, rowid int64, payload []byte) {
	t.Helper()
	cursor := h.cursor(t, true)
	p := BtreePayload{
		FnKey:  Tsqlite3_int64(rowid),
		FnData: int32(len(payload)),
	}
	if len(payload) != 0 {
		p.FpData = uintptr(unsafe.Pointer(&payload[0]))
	}
	if rc := h.engine.BtreeInsert(h.ctx, cursor, BtreePayloadHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&p))}, 0, 0); rc != SQLITE_OK {
		t.Fatalf("BtreeInsert rc = %d, want SQLITE_OK", rc)
	}
}

func (h *minweightBtreeTestHarness) assertIncrblobExpired(t *testing.T, cursor BtreeCursorHandle) {
	t.Helper()
	if got := h.engine.BtreeCursorHasMoved(h.ctx, cursor); got != 1 {
		t.Fatalf("BtreeCursorHasMoved = %d, want 1", got)
	}
	if got := h.engine.BtreeCursorIsValidNN(h.ctx, cursor); got != 0 {
		t.Fatalf("BtreeCursorIsValidNN = %d, want 0", got)
	}
	var differentRow int32
	if rc := h.engine.BtreeCursorRestore(h.ctx, cursor, BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&differentRow))}); rc != SQLITE_OK {
		t.Fatalf("BtreeCursorRestore rc = %d, want SQLITE_OK", rc)
	}
	if differentRow != 1 {
		t.Fatalf("differentRow = %d, want 1", differentRow)
	}
	buf := make([]byte, 1)
	if rc := h.engine.BtreePayloadChecked(h.ctx, cursor, 0, 1, BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&buf[0]))}); rc != SQLITE_ABORT {
		t.Fatalf("BtreePayloadChecked rc = %d, want SQLITE_ABORT", rc)
	}
	data := []byte("z")
	if rc := h.engine.BtreePutData(h.ctx, cursor, 0, 1, BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&data[0]))}); rc != SQLITE_ABORT {
		t.Fatalf("BtreePutData rc = %d, want SQLITE_ABORT", rc)
	}
}

func (h *minweightBtreeTestHarness) assertCursorFault(t *testing.T, cursor BtreeCursorHandle, faultCode int32) {
	t.Helper()
	if got := h.engine.BtreeCursorHasMoved(h.ctx, cursor); got != 1 {
		t.Fatalf("BtreeCursorHasMoved = %d, want 1", got)
	}
	if got := h.engine.BtreeCursorIsValidNN(h.ctx, cursor); got != 0 {
		t.Fatalf("BtreeCursorIsValidNN = %d, want 0", got)
	}
	var differentRow int32
	if rc := h.engine.BtreeCursorRestore(h.ctx, cursor, BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&differentRow))}); rc != faultCode {
		t.Fatalf("BtreeCursorRestore rc = %d, want %d", rc, faultCode)
	}
	if differentRow != 1 {
		t.Fatalf("differentRow = %d, want 1", differentRow)
	}
	buf := make([]byte, 1)
	if rc := h.engine.BtreePayloadChecked(h.ctx, cursor, 0, 1, BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&buf[0]))}); rc != faultCode {
		t.Fatalf("BtreePayloadChecked rc = %d, want %d", rc, faultCode)
	}
	if got := minweightBtCursorFromPointer(cursor.ptr).FeState; got != uint8(CURSOR_FAULT) {
		t.Fatalf("raw cursor state = %d, want CURSOR_FAULT", got)
	}
	if got := minweightBtCursorFromPointer(cursor.ptr).FskipNext; got != faultCode {
		t.Fatalf("raw cursor fault code = %d, want %d", got, faultCode)
	}
}

func (h *minweightBtreeTestHarness) assertIndexCursorRecord(t *testing.T, cursor BtreeCursorHandle, record []byte) {
	t.Helper()
	row, ok := h.engine.cursor(cursor).current()
	if !ok {
		t.Fatal("index cursor is not valid")
	}
	if !bytes.Equal(row.key, record) {
		t.Fatalf("index cursor key = %x, want %x", row.key, record)
	}
	if !bytes.Equal(row.payload, record) {
		t.Fatalf("index cursor payload = %x, want %x", row.payload, record)
	}
	if !minweightIndexKeyVersionedForRoot(h.engine.cursor(cursor).root, row.storeKey) {
		t.Fatalf("index cursor store key = %x, want versioned key", row.storeKey)
	}
}

type minweightRecordTestField struct {
	serial byte
	data   []byte
}

type minweightUnpackedTestValue struct {
	text    string
	integer int64
	isText  bool
}

func minweightUnpackedText(s string) minweightUnpackedTestValue {
	return minweightUnpackedTestValue{text: s, isText: true}
}

func minweightUnpackedInt(v int64) minweightUnpackedTestValue {
	return minweightUnpackedTestValue{integer: v}
}

func minweightTestUnpackedRecord(keyInfo uintptr, defaultRC int8, values ...minweightUnpackedTestValue) (BtreeIndexKeyHandle, *TUnpackedRecord, []TMem, [][]byte) {
	mems := make([]TMem, len(values))
	keepalive := make([][]byte, 0, len(values))
	for i, value := range values {
		if value.isText {
			data := []byte(value.text)
			keepalive = append(keepalive, data)
			mems[i].Fflags = uint16(MEM_Str | MEM_Static)
			mems[i].Fenc = uint8(SQLITE_UTF8)
			mems[i].Fn = int32(len(data))
			if len(data) != 0 {
				mems[i].Fz = uintptr(unsafe.Pointer(&data[0]))
			}
			continue
		}
		*(*int64)(unsafe.Pointer(&mems[i])) = value.integer
		mems[i].Fflags = uint16(MEM_Int)
	}
	rec := &TUnpackedRecord{
		FpKeyInfo:   keyInfo,
		FnField:     uint16(len(values)),
		Fdefault_rc: defaultRC,
	}
	if len(mems) != 0 {
		rec.FaMem = uintptr(unsafe.Pointer(&mems[0]))
	}
	return BtreeIndexKeyHandle{ptr: uintptr(unsafe.Pointer(rec))}, rec, mems, keepalive
}

func minweightKeepUnpackedRecordAlive(rec *TUnpackedRecord, mems []TMem, data [][]byte) {
	runtime.KeepAlive(rec)
	runtime.KeepAlive(mems)
	runtime.KeepAlive(data)
}

func minweightTestRecord(fields ...minweightRecordTestField) []byte {
	record := []byte{byte(1 + len(fields))}
	for _, field := range fields {
		record = append(record, field.serial)
	}
	for _, field := range fields {
		record = append(record, field.data...)
	}
	return record
}

func minweightTestIntRecord(v int64) minweightRecordTestField {
	switch v {
	case 0:
		return minweightRecordTestField{serial: 8}
	case 1:
		return minweightRecordTestField{serial: 9}
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(v))
	return minweightRecordTestField{serial: 6, data: append([]byte(nil), buf[:]...)}
}

func minweightTestTextRecord(s string) minweightRecordTestField {
	return minweightRecordTestField{serial: byte(13 + len(s)*2), data: []byte(s)}
}

func minweightTestBlobRecord(b []byte) minweightRecordTestField {
	return minweightRecordTestField{serial: byte(12 + len(b)*2), data: b}
}

func minweightTestKeyInfo(t *testing.T, tls *libc.TLS, collations []string, sortFlags []uint8) uintptr {
	t.Helper()
	n := len(collations)
	ptrSize := int(unsafe.Sizeof(uintptr(0)))
	keyInfoSize := int(unsafe.Sizeof(TKeyInfo{}))
	total := keyInfoSize + n*ptrSize + n
	p := libc.Xmalloc(tls, uint64(total))
	if p == 0 {
		t.Fatal("sqlite3 malloc returned nil KeyInfo")
	}
	clear(minweightByteSliceFromPointer(p, total))
	t.Cleanup(func() { libc.Xfree(tls, p) })
	info := minweightKeyInfoFromPointer(p)
	info.Fenc = uint8(SQLITE_UTF8)
	info.FnKeyField = uint16(n)
	info.FnAllField = uint16(n)
	info.FaSortFlags = p + uintptr(keyInfoSize+n*ptrSize)
	for i, flag := range sortFlags {
		*minweightUint8PointerFromPointer(info.FaSortFlags + uintptr(i)) = flag
	}
	for i, name := range collations {
		if name == "" || strings.EqualFold(name, "BINARY") {
			continue
		}
		coll := libc.Xmalloc(tls, uint64(unsafe.Sizeof(TCollSeq{})))
		if coll == 0 {
			t.Fatal("sqlite3 malloc returned nil CollSeq")
		}
		clear(minweightByteSliceFromPointer(coll, int(unsafe.Sizeof(TCollSeq{}))))
		t.Cleanup(func() { libc.Xfree(tls, coll) })
		zName, err := libc.CString(name)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { libc.Xfree(tls, zName) })
		minweightCollSeqFromPointer(coll).FzName = zName
		minweightCollSeqFromPointer(coll).Fenc = uint8(SQLITE_UTF8)
		*minweightUintptrPointerFromPointer(p + uintptr(keyInfoSize+i*ptrSize)) = coll
	}
	return p
}

func minweightComparableTestKey(t *testing.T, tls *libc.TLS, keyInfo uintptr, record []byte) []byte {
	t.Helper()
	key, err := minweightComparableIndexKey(BtreeContext{tls: tls}, keyInfo, record)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func minweightAssertKeyLess(t *testing.T, left []byte, right []byte) {
	t.Helper()
	if bytes.Compare(left, right) >= 0 {
		t.Fatalf("key order mismatch:\nleft  = %x\nright = %x", left, right)
	}
}

func TestMinweightComparableIndexKeyOrdersSQLiteStorageClasses(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	keyInfo := minweightTestKeyInfo(t, tls, []string{"BINARY"}, nil)

	nullKey := minweightComparableTestKey(t, tls, keyInfo, minweightTestRecord(minweightRecordTestField{serial: 0}))
	negativeKey := minweightComparableTestKey(t, tls, keyInfo, minweightTestRecord(minweightTestIntRecord(-1)))
	zeroKey := minweightComparableTestKey(t, tls, keyInfo, minweightTestRecord(minweightTestIntRecord(0)))
	positiveKey := minweightComparableTestKey(t, tls, keyInfo, minweightTestRecord(minweightTestIntRecord(1)))
	textKey := minweightComparableTestKey(t, tls, keyInfo, minweightTestRecord(minweightTestTextRecord("a")))
	blobKey := minweightComparableTestKey(t, tls, keyInfo, minweightTestRecord(minweightTestBlobRecord([]byte("a"))))

	minweightAssertKeyLess(t, nullKey, negativeKey)
	minweightAssertKeyLess(t, negativeKey, zeroKey)
	minweightAssertKeyLess(t, zeroKey, positiveKey)
	minweightAssertKeyLess(t, positiveKey, textKey)
	minweightAssertKeyLess(t, textKey, blobKey)
}

func TestMinweightComparableIndexKeyCollationAndDesc(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	ctx := BtreeContext{tls: tls}
	binaryKeyInfo := minweightTestKeyInfo(t, tls, []string{"BINARY", "BINARY"}, nil)
	nocaseKeyInfo := minweightTestKeyInfo(t, tls, []string{"NOCASE", "BINARY"}, nil)
	rtrimKeyInfo := minweightTestKeyInfo(t, tls, []string{"RTRIM", "BINARY"}, nil)
	descKeyInfo := minweightTestKeyInfo(t, tls, []string{"BINARY"}, []uint8{uint8(KEYINFO_ORDER_DESC)})

	bUpper := minweightTestRecord(minweightTestTextRecord("B"), minweightTestIntRecord(1))
	aLower := minweightTestRecord(minweightTestTextRecord("a"), minweightTestIntRecord(2))
	binaryUpper, err := minweightComparableIndexKey(ctx, binaryKeyInfo, bUpper)
	if err != nil {
		t.Fatal(err)
	}
	binaryLower, err := minweightComparableIndexKey(ctx, binaryKeyInfo, aLower)
	if err != nil {
		t.Fatal(err)
	}
	minweightAssertKeyLess(t, binaryUpper, binaryLower)
	nocaseUpper, err := minweightComparableIndexKey(ctx, nocaseKeyInfo, bUpper)
	if err != nil {
		t.Fatal(err)
	}
	nocaseLower, err := minweightComparableIndexKey(ctx, nocaseKeyInfo, aLower)
	if err != nil {
		t.Fatal(err)
	}
	minweightAssertKeyLess(t, nocaseLower, nocaseUpper)

	aSpace := minweightTestRecord(minweightTestTextRecord("a "), minweightTestIntRecord(1))
	aPlain := minweightTestRecord(minweightTestTextRecord("a"), minweightTestIntRecord(2))
	rtrimSpace, err := minweightComparableIndexKey(ctx, rtrimKeyInfo, aSpace)
	if err != nil {
		t.Fatal(err)
	}
	rtrimPlain, err := minweightComparableIndexKey(ctx, rtrimKeyInfo, aPlain)
	if err != nil {
		t.Fatal(err)
	}
	minweightAssertKeyLess(t, rtrimSpace, rtrimPlain)

	descOne, err := minweightComparableIndexKey(ctx, descKeyInfo, minweightTestRecord(minweightTestIntRecord(1)))
	if err != nil {
		t.Fatal(err)
	}
	descTwo, err := minweightComparableIndexKey(ctx, descKeyInfo, minweightTestRecord(minweightTestIntRecord(2)))
	if err != nil {
		t.Fatal(err)
	}
	minweightAssertKeyLess(t, descTwo, descOne)
}

func TestMinweightIndexStoreKeyKeepsLogicalRecordPayload(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	keyInfo := minweightTestKeyInfo(t, tls, []string{"BINARY"}, nil)
	record := minweightTestRecord(minweightTestTextRecord("alpha"))
	key, err := minweightIndexStoreKey(BtreeContext{tls: tls}, keyInfo, 7, record)
	if err != nil {
		t.Fatal(err)
	}
	if len(key) <= 6 || key[0] != minweightIndexPrefix || binary.BigEndian.Uint32(key[1:5]) != 7 || key[5] != minweightIndexKeyVersion {
		t.Fatalf("store key = %x, want versioned index key for root 7", key)
	}
	row := minweightDecodeRow(minweight.Item{Key: key, Value: record}, false)
	if !bytes.Equal(row.storeKey, key) {
		t.Fatalf("decoded store key = %x, want %x", row.storeKey, key)
	}
	if !bytes.Equal(row.key, record) || !bytes.Equal(row.payload, record) {
		t.Fatalf("decoded logical key/payload = %x/%x, want %x", row.key, row.payload, record)
	}
}

func TestMinweightComparableIndexKeyRejectsUnsupportedCollation(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	keyInfo := minweightTestKeyInfo(t, tls, []string{"CUSTOM"}, nil)
	record := minweightTestRecord(minweightTestTextRecord("alpha"))
	_, err := minweightComparableIndexKey(BtreeContext{tls: tls}, keyInfo, record)
	if err == nil || !strings.Contains(err.Error(), "unsupported collation") {
		t.Fatalf("error = %v, want unsupported collation", err)
	}
}

type minweightIndexOverlayFixture struct {
	h       *minweightBtreeTestHarness
	root    uint32
	keyInfo uintptr
	recordA []byte
	recordB []byte
}

func newMinweightIndexOverlayFixture(t *testing.T) minweightIndexOverlayFixture {
	t.Helper()
	h := newMinweightBtreeTestHarness(t)
	root := uint32(2)
	h.bt.tables[root] = minweightTable{intKey: false}
	keyInfo := minweightTestKeyInfo(t, h.tls, []string{"BINARY", "BINARY"}, nil)
	recordA := minweightTestRecord(minweightTestTextRecord("a"), minweightTestIntRecord(1))
	recordB := minweightTestRecord(minweightTestTextRecord("b"), minweightTestIntRecord(2))
	recordC := minweightTestRecord(minweightTestTextRecord("c"), minweightTestIntRecord(3))
	keyB, err := minweightIndexStoreKey(h.ctx, keyInfo, root, recordB)
	if err != nil {
		t.Fatal(err)
	}
	keyC := h.putIndexRecord(t, root, keyInfo, recordC)
	h.putIndexRecord(t, root, keyInfo, recordA)

	if rc, _ := h.bt.beginTrans(h.ctx, 1); rc != SQLITE_OK {
		t.Fatalf("beginTrans rc = %d, want SQLITE_OK", rc)
	}
	if err := h.bt.put(keyB, recordB); err != nil {
		t.Fatal(err)
	}
	if _, err := h.bt.delete(keyC); err != nil {
		t.Fatal(err)
	}
	return minweightIndexOverlayFixture{h: h, root: root, keyInfo: keyInfo, recordA: recordA, recordB: recordB}
}

func TestMinweightIndexCursorSeekMergesOverlay(t *testing.T) {
	f := newMinweightIndexOverlayFixture(t)
	cursor := f.h.indexCursor(t, f.root, f.keyInfo)
	var res int32
	if rc := f.h.engine.BtreeFirst(f.h.ctx, cursor, BtreeMemoryHandle{tls: f.h.tls, ptr: uintptr(unsafe.Pointer(&res))}); rc != SQLITE_OK {
		t.Fatalf("BtreeFirst rc = %d, want SQLITE_OK", rc)
	}
	if res != 0 {
		t.Fatalf("BtreeFirst res = %d, want 0", res)
	}
	f.h.assertIndexCursorRecord(t, cursor, f.recordA)
	if got := len(f.h.engine.cursor(cursor).rows); got != 1 {
		t.Fatalf("cursor rows after seek first = %d, want 1", got)
	}
	if rc := f.h.engine.BtreeNext(f.h.ctx, cursor, 0); rc != SQLITE_OK {
		t.Fatalf("BtreeNext rc = %d, want SQLITE_OK", rc)
	}
	f.h.assertIndexCursorRecord(t, cursor, f.recordB)
	if got := len(f.h.engine.cursor(cursor).rows); got != 1 {
		t.Fatalf("cursor rows after seek next = %d, want 1", got)
	}
	if rc := f.h.engine.BtreeNext(f.h.ctx, cursor, 0); rc != SQLITE_DONE {
		t.Fatalf("BtreeNext rc = %d, want SQLITE_DONE", rc)
	}

	if rc := f.h.engine.BtreeLast(f.h.ctx, cursor, BtreeMemoryHandle{tls: f.h.tls, ptr: uintptr(unsafe.Pointer(&res))}); rc != SQLITE_OK {
		t.Fatalf("BtreeLast rc = %d, want SQLITE_OK", rc)
	}
	if res != 0 {
		t.Fatalf("BtreeLast res = %d, want 0", res)
	}
	f.h.assertIndexCursorRecord(t, cursor, f.recordB)
	if rc := f.h.engine.BtreePrevious(f.h.ctx, cursor, 0); rc != SQLITE_OK {
		t.Fatalf("BtreePrevious rc = %d, want SQLITE_OK", rc)
	}
	f.h.assertIndexCursorRecord(t, cursor, f.recordA)
	if rc := f.h.engine.BtreePrevious(f.h.ctx, cursor, 0); rc != SQLITE_DONE {
		t.Fatalf("BtreePrevious rc = %d, want SQLITE_DONE", rc)
	}
}

func minweightIndexMovetoProbe(t *testing.T, h *minweightBtreeTestHarness, cursor BtreeCursorHandle, keyInfo uintptr, defaultRC int8, values ...minweightUnpackedTestValue) (int32, *TUnpackedRecord) {
	t.Helper()
	var res int32
	probe, rec, mems, keepalive := minweightTestUnpackedRecord(keyInfo, defaultRC, values...)
	if rc := h.engine.BtreeIndexMoveto(h.ctx, cursor, probe, BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&res))}); rc != SQLITE_OK {
		t.Fatalf("BtreeIndexMoveto rc = %d, want SQLITE_OK", rc)
	}
	minweightKeepUnpackedRecordAlive(rec, mems, keepalive)
	return res, rec
}

func minweightAssertIndexMovetoFound(t *testing.T, h *minweightBtreeTestHarness, cursor BtreeCursorHandle, record []byte, keyInfo uintptr, defaultRC int8, values ...minweightUnpackedTestValue) (int32, *TUnpackedRecord) {
	t.Helper()
	res, rec := minweightIndexMovetoProbe(t, h, cursor, keyInfo, defaultRC, values...)
	h.assertIndexCursorRecord(t, cursor, record)
	if got := len(h.engine.cursor(cursor).rows); got != 1 {
		t.Fatalf("cursor rows after index moveto = %d, want 1", got)
	}
	return res, rec
}

func TestMinweightIndexMovetoUsesVersionedSeek(t *testing.T) {
	f := newMinweightIndexOverlayFixture(t)
	cursor := f.h.indexCursor(t, f.root, f.keyInfo)
	res, rec := minweightAssertIndexMovetoFound(t, f.h, cursor, f.recordB, f.keyInfo, 1, minweightUnpackedText("b"))
	if res <= 0 {
		t.Fatalf("moveto prefix res = %d, want positive default_rc compare", res)
	}
	if rec.FeqSeen == 0 {
		t.Fatal("moveto prefix FeqSeen = 0, want equality seen")
	}

	res, rec = minweightAssertIndexMovetoFound(t, f.h, cursor, f.recordB, f.keyInfo, -1, minweightUnpackedText("a"))
	if res <= 0 {
		t.Fatalf("moveto skip-prefix res = %d, want positive compare against b", res)
	}
	if rec.FeqSeen != 0 {
		t.Fatal("moveto skip-prefix FeqSeen = 1, want no equality on returned row")
	}

	res, rec = minweightAssertIndexMovetoFound(t, f.h, cursor, f.recordB, f.keyInfo, 0, minweightUnpackedText("b"), minweightUnpackedInt(2))
	if res != 0 {
		t.Fatalf("moveto full-key res = %d, want 0", res)
	}
	if rec.FeqSeen == 0 {
		t.Fatal("moveto full-key FeqSeen = 0, want equality seen")
	}

	res, _ = minweightIndexMovetoProbe(t, f.h, cursor, f.keyInfo, 1, minweightUnpackedText("c"))
	if res != -1 {
		t.Fatalf("moveto missing res = %d, want -1", res)
	}
	if got := f.h.engine.BtreeCursorIsValidNN(f.h.ctx, cursor); got != 0 {
		t.Fatalf("cursor valid after missing moveto = %d, want 0", got)
	}
}

func TestMinweightIndexMovetoUsesDescProbeSeek(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	root := uint32(2)
	h.bt.tables[root] = minweightTable{intKey: false}
	keyInfo := minweightTestKeyInfo(t, h.tls, []string{"BINARY", "BINARY"}, []uint8{uint8(KEYINFO_ORDER_DESC)})

	recordA := minweightTestRecord(minweightTestTextRecord("a"), minweightTestIntRecord(1))
	recordB := minweightTestRecord(minweightTestTextRecord("b"), minweightTestIntRecord(2))
	recordC := minweightTestRecord(minweightTestTextRecord("c"), minweightTestIntRecord(3))
	h.putIndexRecord(t, root, keyInfo, recordA)
	h.putIndexRecord(t, root, keyInfo, recordB)
	h.putIndexRecord(t, root, keyInfo, recordC)

	cursor := h.indexCursor(t, root, keyInfo)
	var res int32
	probe, rec, mems, keepalive := minweightTestUnpackedRecord(keyInfo, 1, minweightUnpackedText("b"))
	if rc := h.engine.BtreeIndexMoveto(h.ctx, cursor, probe, BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&res))}); rc != SQLITE_OK {
		t.Fatalf("BtreeIndexMoveto DESC prefix rc = %d, want SQLITE_OK", rc)
	}
	h.assertIndexCursorRecord(t, cursor, recordB)
	if got := len(h.engine.cursor(cursor).rows); got != 1 {
		t.Fatalf("cursor rows after DESC index moveto = %d, want 1", got)
	}
	if res <= 0 {
		t.Fatalf("DESC moveto prefix res = %d, want positive default_rc compare", res)
	}
	if rec.FeqSeen == 0 {
		t.Fatal("DESC moveto prefix FeqSeen = 0, want equality seen")
	}
	minweightKeepUnpackedRecordAlive(rec, mems, keepalive)

	probe, rec, mems, keepalive = minweightTestUnpackedRecord(keyInfo, 0, minweightUnpackedText("b"), minweightUnpackedInt(2))
	if rc := h.engine.BtreeIndexMoveto(h.ctx, cursor, probe, BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&res))}); rc != SQLITE_OK {
		t.Fatalf("BtreeIndexMoveto DESC full key rc = %d, want SQLITE_OK", rc)
	}
	h.assertIndexCursorRecord(t, cursor, recordB)
	if res != 0 {
		t.Fatalf("DESC moveto full-key res = %d, want 0", res)
	}
	if rec.FeqSeen == 0 {
		t.Fatal("DESC moveto full-key FeqSeen = 0, want equality seen")
	}
	minweightKeepUnpackedRecordAlive(rec, mems, keepalive)
}

func TestMinweightCursorRestoreRefreshesChangedRow(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()

	engine := NewMinweightStorageEngine().(*minweightStorageEngine)
	bt := &minweightBtree{minweightDatabase: newMinweightTestDatabase()}
	engine.btrees[1] = bt
	ctx := BtreeContext{tls: tls}
	btree := BtreeHandle{tls: tls, ptr: 1}
	var rawCursor BtCursor
	cursor := BtreeCursorHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&rawCursor))}

	if err := bt.store.Put(minweightTableKey(1, 1), []byte("old")); err != nil {
		t.Fatal(err)
	}
	bt.noteInsert(1, 1, false)
	bt.dataVer++

	if rc := engine.BtreeCursor(ctx, btree, 1, 0, BtreeKeyInfoHandle{}, cursor); rc != SQLITE_OK {
		t.Fatalf("BtreeCursor rc = %d, want SQLITE_OK", rc)
	}
	var moveResult int32
	if rc := engine.BtreeTableMoveto(ctx, cursor, 1, 0, BtreeMemoryHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&moveResult))}); rc != SQLITE_OK {
		t.Fatalf("BtreeTableMoveto rc = %d, want SQLITE_OK", rc)
	}
	if moveResult != 0 {
		t.Fatalf("moveResult = %d, want 0", moveResult)
	}

	if err := bt.store.Put(minweightTableKey(1, 1), []byte("new")); err != nil {
		t.Fatal(err)
	}
	bt.dataVer++

	if got := engine.BtreeCursorHasMoved(ctx, cursor); got != 1 {
		t.Fatalf("BtreeCursorHasMoved = %d, want 1", got)
	}
	var differentRow int32
	if rc := engine.BtreeCursorRestore(ctx, cursor, BtreeMemoryHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&differentRow))}); rc != SQLITE_OK {
		t.Fatalf("BtreeCursorRestore rc = %d, want SQLITE_OK", rc)
	}
	if differentRow != 0 {
		t.Fatalf("differentRow = %d, want 0", differentRow)
	}
	row, ok := engine.cursor(cursor).current()
	if !ok {
		t.Fatal("cursor is not valid after restore")
	}
	if !bytes.Equal(row.payload, []byte("new")) {
		t.Fatalf("payload = %q, want new", row.payload)
	}
	if got := engine.BtreeCursorHasMoved(ctx, cursor); got != 0 {
		t.Fatalf("BtreeCursorHasMoved after restore = %d, want 0", got)
	}
}

func TestMinweightCommitGenerationRetainsPinnedOldVersion(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	reader := &minweightBtree{minweightDatabase: h.bt.minweightDatabase}
	if got := h.bt.generation; got != 1 {
		t.Fatalf("initial generation = %d, want 1", got)
	}

	reader.retainReader()
	if got := h.bt.readerViews[reader]; got != 1 {
		t.Fatalf("reader generation = %d, want 1", got)
	}
	if got := h.bt.pinnedViews[1]; got != 1 {
		t.Fatalf("pinned generation refs = %d, want 1", got)
	}

	if rc, _ := h.bt.beginTrans(h.ctx, 1); rc != SQLITE_OK {
		t.Fatalf("beginTrans rc = %d, want SQLITE_OK", rc)
	}
	key := minweightTableKey(1, 1)
	if err := h.bt.put(key, []byte("one")); err != nil {
		t.Fatal(err)
	}
	h.bt.noteInsert(1, 1, false)
	h.bt.bumpDataVer()
	if err := h.bt.commitActiveWriteTxn(); err != nil {
		t.Fatal(err)
	}
	if got := h.bt.generation; got != 2 {
		t.Fatalf("generation after commit = %d, want 2", got)
	}
	if got := len(h.bt.changes); got != 1 {
		t.Fatalf("retained commit changes = %d, want 1", got)
	}
	change := h.bt.changes[0]
	if change.generation != 2 {
		t.Fatalf("change generation = %d, want 2", change.generation)
	}
	if _, ok := change.roots[1]; !ok {
		t.Fatalf("change roots = %#v, want root 1", change.roots)
	}
	keyChange, ok := change.keys[string(key)]
	if !ok {
		t.Fatalf("change keys missing table key %x", key)
	}
	if keyChange.beforeExist {
		t.Fatal("key before commit exists, want absent")
	}
	if !keyChange.afterExists || !bytes.Equal(keyChange.after, []byte("one")) {
		t.Fatalf("key after commit = exists:%v value:%q, want one", keyChange.afterExists, keyChange.after)
	}

	reader.releaseReader()
	if got := len(h.bt.pinnedViews); got != 0 {
		t.Fatalf("pinned views after release = %d, want 0", got)
	}
	if got := len(h.bt.changes); got != 0 {
		t.Fatalf("commit changes after reader release = %d, want 0", got)
	}
}

func TestMinweightCommitDetectsReadSetConflict(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	if err := h.bt.store.Put(minweightTableKey(1, 1), []byte("old")); err != nil {
		t.Fatal(err)
	}
	if rc, _ := h.bt.beginTrans(h.ctx, 1); rc != SQLITE_OK {
		t.Fatalf("beginTrans rc = %d, want SQLITE_OK", rc)
	}
	if _, ok, err := h.bt.get(minweightTableKey(1, 1)); err != nil || !ok {
		t.Fatalf("transaction read ok=%v err=%v, want existing row", ok, err)
	}
	h.bt.changes = append(h.bt.changes, minweightCommitChange{
		generation: 2,
		keys: map[string]minweightCommittedKeyChange{
			string(minweightTableKey(1, 1)): {key: minweightTableKey(1, 1)},
		},
	})
	h.bt.generation = 2
	if err := h.bt.put(minweightTableKey(1, 2), []byte("new")); err != nil {
		t.Fatal(err)
	}
	err := h.bt.commitActiveWriteTxn()
	if !errors.Is(err, errMinweightTxnConflict) {
		t.Fatalf("commit error = %v, want read-set conflict", err)
	}
	if _, ok, err := h.bt.store.Get(minweightTableKey(1, 2)); err != nil || ok {
		t.Fatalf("conflicted write visible ok=%v err=%v, want absent", ok, err)
	}
}

func TestMinweightOpenTracksSharedCacheConnectionCount(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	if rc := Xsqlite3_initialize(tls); rc != SQLITE_OK {
		t.Fatalf("sqlite3_initialize rc = %d, want SQLITE_OK", rc)
	}

	engine := NewMinweightStorageEngine().(*minweightStorageEngine)
	ctx := BtreeContext{tls: tls}
	filename := filepath.Join(t.TempDir(), "shared.db")
	zFilename := minweightAllocCString(ctx, filename)
	if zFilename == 0 {
		t.Fatal("minweightAllocCString returned 0")
	}
	defer Xsqlite3_free(tls, zFilename)

	open := func(t *testing.T, db uintptr, vfsFlags int32) BtreeHandle {
		t.Helper()
		ppBtree := newMinweightBtreeOut(t, tls)
		rc := engine.BtreeOpen(
			ctx,
			BtreeVFSHandle{},
			BtreeCStringHandle{tls: tls, ptr: zFilename},
			SQLiteHandle{tls: tls, ptr: db},
			ppBtree,
			0,
			vfsFlags,
		)
		if rc != SQLITE_OK {
			t.Fatalf("BtreeOpen rc = %d, want SQLITE_OK", rc)
		}
		token := ppBtree.GetUintptr()
		if token == 0 {
			t.Fatal("BtreeOpen returned nil token")
		}
		return BtreeHandle{tls: tls, ptr: token}
	}
	closeBtree := func(t *testing.T, btree BtreeHandle) {
		t.Helper()
		if rc := engine.BtreeClose(ctx, btree); rc != SQLITE_OK {
			t.Fatalf("BtreeClose rc = %d, want SQLITE_OK", rc)
		}
	}

	sharedFlags := int32(SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE | SQLITE_OPEN_SHAREDCACHE)
	shared1 := open(t, 101, sharedFlags)
	shared2 := open(t, 102, sharedFlags)
	if got := engine.BtreeSharable(ctx, shared1); got != 1 {
		t.Fatalf("BtreeSharable(shared1) = %d, want 1", got)
	}
	if got := engine.BtreeConnectionCount(ctx, shared1); got != 2 {
		t.Fatalf("BtreeConnectionCount(shared1) = %d, want 2", got)
	}
	if got := engine.BtreeConnectionCount(ctx, shared2); got != 2 {
		t.Fatalf("BtreeConnectionCount(shared2) = %d, want 2", got)
	}
	closeBtree(t, shared1)
	if got := engine.BtreeConnectionCount(ctx, shared2); got != 1 {
		t.Fatalf("BtreeConnectionCount(shared2 after close) = %d, want 1", got)
	}
	closeBtree(t, shared2)

	privateFlags := int32(SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE)
	private1 := open(t, 201, privateFlags)
	private2 := open(t, 202, privateFlags)
	if got := engine.BtreeSharable(ctx, private1); got != 0 {
		t.Fatalf("BtreeSharable(private1) = %d, want 0", got)
	}
	if got := engine.BtreeConnectionCount(ctx, private1); got != 1 {
		t.Fatalf("BtreeConnectionCount(private1) = %d, want 1", got)
	}
	if got := engine.BtreeConnectionCount(ctx, private2); got != 1 {
		t.Fatalf("BtreeConnectionCount(private2) = %d, want 1", got)
	}
	closeBtree(t, private1)
	closeBtree(t, private2)
}

func TestMinweightBtreeSetPagerFlagsUpdatesFakePager(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	if rc := Xsqlite3_initialize(tls); rc != SQLITE_OK {
		t.Fatalf("sqlite3_initialize rc = %d, want SQLITE_OK", rc)
	}

	engine := NewMinweightStorageEngine().(*minweightStorageEngine)
	ctx := BtreeContext{tls: tls}
	filename := filepath.Join(t.TempDir(), "pager-flags.db")
	zFilename := minweightAllocCString(ctx, filename)
	if zFilename == 0 {
		t.Fatal("minweightAllocCString returned 0")
	}
	defer Xsqlite3_free(tls, zFilename)

	ppBtree := newMinweightBtreeOut(t, tls)
	if rc := engine.BtreeOpen(
		ctx,
		BtreeVFSHandle{},
		BtreeCStringHandle{tls: tls, ptr: zFilename},
		SQLiteHandle{},
		ppBtree,
		0,
		int32(SQLITE_OPEN_READWRITE|SQLITE_OPEN_CREATE),
	); rc != SQLITE_OK {
		t.Fatalf("BtreeOpen rc = %d, want SQLITE_OK", rc)
	}
	token := ppBtree.GetUintptr()
	btree := BtreeHandle{tls: tls, ptr: token}
	defer func() {
		if rc := engine.BtreeClose(ctx, btree); rc != SQLITE_OK {
			t.Fatalf("BtreeClose rc = %d, want SQLITE_OK", rc)
		}
	}()
	pager := minweightPagerFromPointer(engine.btree(btree).pager)

	if rc := engine.BtreeSetPagerFlags(ctx, btree, PAGER_SYNCHRONOUS_OFF); rc != SQLITE_OK {
		t.Fatalf("BtreeSetPagerFlags(OFF) rc = %d, want SQLITE_OK", rc)
	}
	if pager.FnoSync != 1 {
		t.Fatalf("FnoSync = %d, want 1", pager.FnoSync)
	}
	if pager.FfullSync != 0 {
		t.Fatalf("FfullSync = %d, want 0", pager.FfullSync)
	}
	if pager.FextraSync != 0 {
		t.Fatalf("FextraSync = %d, want 0", pager.FextraSync)
	}
	if pager.FsyncFlags != 0 {
		t.Fatalf("FsyncFlags = %d, want 0", pager.FsyncFlags)
	}
	if pager.FwalSyncFlags != 0 {
		t.Fatalf("FwalSyncFlags = %d, want 0", pager.FwalSyncFlags)
	}
	if pager.FdoNotSpill&SPILLFLAG_OFF == 0 {
		t.Fatalf("FdoNotSpill = %#x, want SPILLFLAG_OFF set", pager.FdoNotSpill)
	}

	flags := uint32(PAGER_SYNCHRONOUS_EXTRA | PAGER_FULLFSYNC | PAGER_CKPT_FULLFSYNC | PAGER_CACHESPILL)
	if rc := engine.BtreeSetPagerFlags(ctx, btree, flags); rc != SQLITE_OK {
		t.Fatalf("BtreeSetPagerFlags(EXTRA) rc = %d, want SQLITE_OK", rc)
	}
	if pager.FnoSync != 0 {
		t.Fatalf("FnoSync = %d, want 0", pager.FnoSync)
	}
	if pager.FfullSync != 1 {
		t.Fatalf("FfullSync = %d, want 1", pager.FfullSync)
	}
	if pager.FextraSync != 1 {
		t.Fatalf("FextraSync = %d, want 1", pager.FextraSync)
	}
	if pager.FsyncFlags != SQLITE_SYNC_FULL {
		t.Fatalf("FsyncFlags = %d, want SQLITE_SYNC_FULL", pager.FsyncFlags)
	}
	if pager.FwalSyncFlags != SQLITE_SYNC_FULL<<2|SQLITE_SYNC_FULL {
		t.Fatalf("FwalSyncFlags = %d, want full sync plus checkpoint full sync", pager.FwalSyncFlags)
	}
	if pager.FdoNotSpill&SPILLFLAG_OFF != 0 {
		t.Fatalf("FdoNotSpill = %#x, want SPILLFLAG_OFF clear", pager.FdoNotSpill)
	}
}

func TestMinweightSharedCacheTableLocks(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	if rc := Xsqlite3_initialize(tls); rc != SQLITE_OK {
		t.Fatalf("sqlite3_initialize rc = %d, want SQLITE_OK", rc)
	}

	engine := NewMinweightStorageEngine().(*minweightStorageEngine)
	ctx := BtreeContext{tls: tls}
	filename := filepath.Join(t.TempDir(), "locks.db")
	zFilename := minweightAllocCString(ctx, filename)
	if zFilename == 0 {
		t.Fatal("minweightAllocCString returned 0")
	}
	defer Xsqlite3_free(tls, zFilename)

	open := func(t *testing.T) BtreeHandle {
		t.Helper()
		ppBtree := newMinweightBtreeOut(t, tls)
		rc := engine.BtreeOpen(
			ctx,
			BtreeVFSHandle{},
			BtreeCStringHandle{tls: tls, ptr: zFilename},
			SQLiteHandle{},
			ppBtree,
			0,
			int32(SQLITE_OPEN_READWRITE|SQLITE_OPEN_CREATE|SQLITE_OPEN_SHAREDCACHE),
		)
		if rc != SQLITE_OK {
			t.Fatalf("BtreeOpen rc = %d, want SQLITE_OK", rc)
		}
		token := ppBtree.GetUintptr()
		return BtreeHandle{tls: tls, ptr: token}
	}
	closeBtree := func(t *testing.T, btree BtreeHandle) {
		t.Helper()
		if rc := engine.BtreeClose(ctx, btree); rc != SQLITE_OK {
			t.Fatalf("BtreeClose rc = %d, want SQLITE_OK", rc)
		}
	}

	first := open(t)
	second := open(t)
	if rc := engine.BtreeBeginTrans(ctx, first, 0, BtreeMemoryHandle{}); rc != SQLITE_OK {
		t.Fatalf("first BtreeBeginTrans rc = %d, want SQLITE_OK", rc)
	}
	if rc := engine.BtreeBeginTrans(ctx, second, 0, BtreeMemoryHandle{}); rc != SQLITE_OK {
		t.Fatalf("second BtreeBeginTrans rc = %d, want SQLITE_OK", rc)
	}
	if rc := engine.BtreeLockTable(ctx, first, 2, 0); rc != SQLITE_OK {
		t.Fatalf("first read lock rc = %d, want SQLITE_OK", rc)
	}
	if rc := engine.BtreeLockTable(ctx, second, 2, 0); rc != SQLITE_OK {
		t.Fatalf("second read lock rc = %d, want SQLITE_OK", rc)
	}
	if rc := engine.BtreeLockTable(ctx, second, 2, 1); rc != SQLITE_LOCKED_SHAREDCACHE {
		t.Fatalf("second write lock rc = %d, want SQLITE_LOCKED_SHAREDCACHE", rc)
	}
	if rc := engine.BtreeCommit(ctx, first); rc != SQLITE_OK {
		t.Fatalf("first BtreeCommit rc = %d, want SQLITE_OK", rc)
	}
	if rc := engine.BtreeLockTable(ctx, second, 2, 1); rc != SQLITE_OK {
		t.Fatalf("second write lock after release rc = %d, want SQLITE_OK", rc)
	}
	closeBtree(t, first)
	closeBtree(t, second)

	schemaWriter := open(t)
	schemaReader := open(t)
	if rc := engine.BtreeLockTable(ctx, schemaWriter, SCHEMA_ROOT, 1); rc != SQLITE_OK {
		t.Fatalf("schema write lock rc = %d, want SQLITE_OK", rc)
	}
	if rc := engine.BtreeSchemaLocked(ctx, schemaReader); rc != SQLITE_LOCKED_SHAREDCACHE {
		t.Fatalf("schema locked rc = %d, want SQLITE_LOCKED_SHAREDCACHE", rc)
	}
	if rc := engine.BtreeCommit(ctx, schemaWriter); rc != SQLITE_OK {
		t.Fatalf("schema writer BtreeCommit rc = %d, want SQLITE_OK", rc)
	}
	if rc := engine.BtreeSchemaLocked(ctx, schemaReader); rc != SQLITE_OK {
		t.Fatalf("schema locked after release rc = %d, want SQLITE_OK", rc)
	}
	closeBtree(t, schemaWriter)
	closeBtree(t, schemaReader)
}

func TestMinweightCheckpointLockedDuringTransaction(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)

	if rc := h.engine.BtreeBeginTrans(h.ctx, h.btree, 1, BtreeMemoryHandle{}); rc != SQLITE_OK {
		t.Fatalf("BtreeBeginTrans rc = %d, want SQLITE_OK", rc)
	}
	var nLog int32 = -1
	var nCkpt int32 = -1
	rc := h.engine.BtreeCheckpoint(
		h.ctx,
		h.btree,
		SQLITE_CHECKPOINT_PASSIVE,
		BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&nLog))},
		BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&nCkpt))},
	)
	if rc != SQLITE_LOCKED {
		t.Fatalf("BtreeCheckpoint during transaction rc = %d, want SQLITE_LOCKED", rc)
	}
	if nLog != 0 || nCkpt != 0 {
		t.Fatalf("checkpoint counters = %d/%d, want 0/0", nLog, nCkpt)
	}

	if rc := h.engine.BtreeCommit(h.ctx, h.btree); rc != SQLITE_OK {
		t.Fatalf("BtreeCommit rc = %d, want SQLITE_OK", rc)
	}
	nLog = -1
	nCkpt = -1
	rc = h.engine.BtreeCheckpoint(
		h.ctx,
		h.btree,
		SQLITE_CHECKPOINT_PASSIVE,
		BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&nLog))},
		BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&nCkpt))},
	)
	if rc != SQLITE_OK {
		t.Fatalf("BtreeCheckpoint after commit rc = %d, want SQLITE_OK", rc)
	}
	if nLog != 0 || nCkpt != 0 {
		t.Fatalf("checkpoint counters after commit = %d/%d, want 0/0", nLog, nCkpt)
	}
}

func TestMinweightCursorHintFlags(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	cursor := h.cursor(t, false)

	h.engine.BtreeCursorHintFlags(h.ctx, cursor, BTREE_SEEK_EQ)
	if got := h.engine.BtreeCursorHasHint(h.ctx, cursor, BTREE_SEEK_EQ); got != 1 {
		t.Fatalf("BtreeCursorHasHint(BTREE_SEEK_EQ) = %d, want 1", got)
	}
	if got := h.engine.BtreeCursorHasHint(h.ctx, cursor, BTREE_BULKLOAD); got != 0 {
		t.Fatalf("BtreeCursorHasHint(BTREE_BULKLOAD) = %d, want 0", got)
	}
	if got := minweightBtCursorFromPointer(cursor.ptr).Fhints; got != BTREE_SEEK_EQ {
		t.Fatalf("raw cursor hints = %d, want %d", got, BTREE_SEEK_EQ)
	}

	h.engine.BtreeCursorHintFlags(h.ctx, cursor, BTREE_BULKLOAD)
	if got := h.engine.BtreeCursorHasHint(h.ctx, cursor, BTREE_SEEK_EQ); got != 0 {
		t.Fatalf("BtreeCursorHasHint(BTREE_SEEK_EQ after reset) = %d, want 0", got)
	}
	if got := h.engine.BtreeCursorHasHint(h.ctx, cursor, BTREE_BULKLOAD); got != 1 {
		t.Fatalf("BtreeCursorHasHint(BTREE_BULKLOAD after reset) = %d, want 1", got)
	}
	if got := minweightBtCursorFromPointer(cursor.ptr).Fhints; got != BTREE_BULKLOAD {
		t.Fatalf("raw cursor hints after reset = %d, want %d", got, BTREE_BULKLOAD)
	}
}

func TestMinweightCursorPinTogglesRawFlag(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	cursor := h.cursor(t, true)
	raw := minweightBtCursorFromPointer(cursor.ptr)
	if raw.FcurFlags&uint8(BTCF_WriteFlag) == 0 {
		t.Fatalf("raw cursor flags = %d, want BTCF_WriteFlag set", raw.FcurFlags)
	}

	h.engine.BtreeCursorPin(h.ctx, cursor)
	if raw.FcurFlags&uint8(BTCF_Pinned) == 0 {
		t.Fatalf("raw cursor flags after pin = %d, want BTCF_Pinned set", raw.FcurFlags)
	}
	if raw.FcurFlags&uint8(BTCF_WriteFlag) == 0 {
		t.Fatalf("raw cursor flags after pin = %d, want BTCF_WriteFlag preserved", raw.FcurFlags)
	}

	h.engine.BtreeCursorUnpin(h.ctx, cursor)
	if raw.FcurFlags&uint8(BTCF_Pinned) != 0 {
		t.Fatalf("raw cursor flags after unpin = %d, want BTCF_Pinned clear", raw.FcurFlags)
	}
	if raw.FcurFlags&uint8(BTCF_WriteFlag) == 0 {
		t.Fatalf("raw cursor flags after unpin = %d, want BTCF_WriteFlag preserved", raw.FcurFlags)
	}
}

func TestMinweightIncrblobCursorInvalidatedByReplace(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	h.putRow(t, 1, []byte("abc"))
	h.putRow(t, 2, []byte("xyz"))

	expired := h.cursor(t, false)
	h.moveToRow(t, expired, 1)
	h.engine.BtreeIncrblobCursor(h.ctx, expired)

	stillValid := h.cursor(t, false)
	h.moveToRow(t, stillValid, 2)
	h.engine.BtreeIncrblobCursor(h.ctx, stillValid)

	h.replaceRow(t, 1, []byte("def"))
	h.assertIncrblobExpired(t, expired)

	buf := make([]byte, 3)
	if rc := h.engine.BtreePayloadChecked(h.ctx, stillValid, 0, 3, BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&buf[0]))}); rc != SQLITE_OK {
		t.Fatalf("BtreePayloadChecked for other row rc = %d, want SQLITE_OK", rc)
	}
	if !bytes.Equal(buf, []byte("xyz")) {
		t.Fatalf("other row payload = %q, want xyz", buf)
	}
	got, ok, err := h.bt.store.Get(minweightTableKey(1, 1))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("replaced row disappeared")
	}
	if !bytes.Equal(got, []byte("def")) {
		t.Fatalf("replaced row payload = %q, want def", got)
	}
}

func TestMinweightIncrblobCursorInvalidatedByClearTable(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	h.putRow(t, 1, []byte("abc"))

	cursor := h.cursor(t, false)
	h.moveToRow(t, cursor, 1)
	h.engine.BtreeIncrblobCursor(h.ctx, cursor)

	var changes int32
	if rc := h.engine.BtreeClearTable(h.ctx, h.btree, 1, BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&changes))}); rc != SQLITE_OK {
		t.Fatalf("BtreeClearTable rc = %d, want SQLITE_OK", rc)
	}
	if changes != 1 {
		t.Fatalf("changes = %d, want 1", changes)
	}
	h.assertIncrblobExpired(t, cursor)
	if _, ok, err := h.bt.store.Get(minweightTableKey(1, 1)); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("cleared row still exists")
	}
}

func TestMinweightTripAllCursorsHonorsWriteOnly(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	h.putRow(t, 1, []byte("abc"))

	readCursor := h.cursor(t, false)
	h.moveToRow(t, readCursor, 1)
	writeCursor := h.cursor(t, true)
	h.moveToRow(t, writeCursor, 1)

	if rc := h.engine.BtreeTripAllCursors(h.ctx, h.btree, SQLITE_ABORT_ROLLBACK, 1); rc != SQLITE_OK {
		t.Fatalf("BtreeTripAllCursors rc = %d, want SQLITE_OK", rc)
	}
	h.assertCursorFault(t, writeCursor, SQLITE_ABORT_ROLLBACK)

	buf := make([]byte, 3)
	if rc := h.engine.BtreePayloadChecked(h.ctx, readCursor, 0, 3, BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&buf[0]))}); rc != SQLITE_OK {
		t.Fatalf("read cursor payload rc = %d, want SQLITE_OK", rc)
	}
	if !bytes.Equal(buf, []byte("abc")) {
		t.Fatalf("read cursor payload = %q, want abc", buf)
	}
}

func TestMinweightRollbackTripsCursors(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	h.putRow(t, 1, []byte("abc"))

	cursor := h.cursor(t, false)
	h.moveToRow(t, cursor, 1)

	if rc := h.engine.BtreeRollback(h.ctx, h.btree, SQLITE_ABORT, 0); rc != SQLITE_OK {
		t.Fatalf("BtreeRollback rc = %d, want SQLITE_OK", rc)
	}
	h.assertCursorFault(t, cursor, SQLITE_ABORT)
}

func TestMinweightIntegrityCheckReportsLogicalCorruption(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	if rc := Xsqlite3_initialize(tls); rc != SQLITE_OK {
		t.Fatalf("sqlite3_initialize rc = %d, want SQLITE_OK", rc)
	}

	engine := NewMinweightStorageEngine().(*minweightStorageEngine)
	bt := &minweightBtree{minweightDatabase: newMinweightTestDatabase()}
	engine.btrees[1] = bt
	ctx := BtreeContext{tls: tls}
	btree := BtreeHandle{tls: tls, ptr: 1}

	if err := bt.store.Put(minweightTableKey(1, 1), []byte("ok")); err != nil {
		t.Fatal(err)
	}
	bt.noteInsert(1, 1, false)

	var pnErr int32
	var pzOut uintptr
	if rc := engine.BtreeIntegrityCheck(
		ctx,
		SQLiteHandle{},
		btree,
		BtreeMemoryHandle{},
		BtreeMemoryHandle{},
		0,
		100,
		BtreeMemoryHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&pnErr))},
		BtreeMemoryHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&pzOut))},
	); rc != SQLITE_OK {
		t.Fatalf("BtreeIntegrityCheck rc = %d, want SQLITE_OK", rc)
	}
	if pnErr != 0 {
		t.Fatalf("pnErr = %d, want 0", pnErr)
	}
	if pzOut != 0 {
		t.Fatalf("pzOut = %#x, want 0", pzOut)
	}

	bt.mu.Lock()
	table := bt.tables[1]
	table.rowCount = 2
	bt.tables[1] = table
	bt.mu.Unlock()

	pnErr = 0
	pzOut = 0
	if rc := engine.BtreeIntegrityCheck(
		ctx,
		SQLiteHandle{},
		btree,
		BtreeMemoryHandle{},
		BtreeMemoryHandle{},
		0,
		100,
		BtreeMemoryHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&pnErr))},
		BtreeMemoryHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&pzOut))},
	); rc != SQLITE_OK {
		t.Fatalf("BtreeIntegrityCheck rc = %d, want SQLITE_OK", rc)
	}
	if pnErr == 0 {
		t.Fatal("pnErr = 0, want logical corruption")
	}
	if pzOut == 0 {
		t.Fatal("pzOut = 0, want error text")
	}
	defer Xsqlite3_free(tls, pzOut)
	if got := libc.GoString(pzOut); !strings.Contains(got, "row count metadata 2 != actual 1") {
		t.Fatalf("integrity text = %q", got)
	}
}

func TestMinweightPutDataRejectsBlobGrowth(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()

	engine := NewMinweightStorageEngine().(*minweightStorageEngine)
	bt := &minweightBtree{minweightDatabase: newMinweightTestDatabase()}
	engine.btrees[1] = bt
	ctx := BtreeContext{tls: tls}
	btree := BtreeHandle{tls: tls, ptr: 1}
	var rawCursor BtCursor
	cursor := BtreeCursorHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&rawCursor))}

	key := minweightTableKey(1, 1)
	if err := bt.store.Put(key, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	bt.noteInsert(1, 1, false)

	if rc := engine.BtreeCursor(ctx, btree, 1, 1, BtreeKeyInfoHandle{}, cursor); rc != SQLITE_OK {
		t.Fatalf("BtreeCursor rc = %d, want SQLITE_OK", rc)
	}
	var moveResult int32
	if rc := engine.BtreeTableMoveto(ctx, cursor, 1, 0, BtreeMemoryHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&moveResult))}); rc != SQLITE_OK {
		t.Fatalf("BtreeTableMoveto rc = %d, want SQLITE_OK", rc)
	}
	if moveResult != 0 {
		t.Fatalf("moveResult = %d, want 0", moveResult)
	}

	data := []byte("xy")
	rc := engine.BtreePutData(ctx, cursor, 2, 2, BtreeMemoryHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&data[0]))})
	if rc != SQLITE_CORRUPT {
		t.Fatalf("BtreePutData rc = %d, want SQLITE_CORRUPT", rc)
	}
	got, ok, err := bt.store.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("row disappeared")
	}
	if !bytes.Equal(got, []byte("abc")) {
		t.Fatalf("payload = %q, want abc", got)
	}
}

func TestMinweightPutDataRequiresWriteCursor(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()

	engine := NewMinweightStorageEngine().(*minweightStorageEngine)
	bt := &minweightBtree{minweightDatabase: newMinweightTestDatabase()}
	engine.btrees[1] = bt
	ctx := BtreeContext{tls: tls}
	btree := BtreeHandle{tls: tls, ptr: 1}
	var rawCursor BtCursor
	cursor := BtreeCursorHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&rawCursor))}

	key := minweightTableKey(1, 1)
	if err := bt.store.Put(key, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	bt.noteInsert(1, 1, false)

	if rc := engine.BtreeCursor(ctx, btree, 1, 0, BtreeKeyInfoHandle{}, cursor); rc != SQLITE_OK {
		t.Fatalf("BtreeCursor rc = %d, want SQLITE_OK", rc)
	}
	var moveResult int32
	if rc := engine.BtreeTableMoveto(ctx, cursor, 1, 0, BtreeMemoryHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&moveResult))}); rc != SQLITE_OK {
		t.Fatalf("BtreeTableMoveto rc = %d, want SQLITE_OK", rc)
	}
	if moveResult != 0 {
		t.Fatalf("moveResult = %d, want 0", moveResult)
	}

	data := []byte("z")
	rc := engine.BtreePutData(ctx, cursor, 0, 1, BtreeMemoryHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&data[0]))})
	if rc != SQLITE_READONLY {
		t.Fatalf("BtreePutData rc = %d, want SQLITE_READONLY", rc)
	}
	got, ok, err := bt.store.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("row disappeared")
	}
	if !bytes.Equal(got, []byte("abc")) {
		t.Fatalf("payload = %q, want abc", got)
	}
}
