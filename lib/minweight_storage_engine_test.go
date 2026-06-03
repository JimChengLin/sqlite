// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

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

	engine := NewMinweightStorageEngine().(*minweightStorageEngine)
	bt := &minweightBtree{minweightDatabase: minweightNewDatabase()}
	engine.btrees[1] = bt
	return &minweightBtreeTestHarness{
		tls:    tls,
		engine: engine,
		bt:     bt,
		ctx:    BtreeContext{tls: tls},
		btree:  BtreeHandle{tls: tls, ptr: 1},
	}
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
	if got := (*BtCursor)(unsafe.Pointer(cursor.ptr)).FeState; got != uint8(CURSOR_FAULT) {
		t.Fatalf("raw cursor state = %d, want CURSOR_FAULT", got)
	}
	if got := (*BtCursor)(unsafe.Pointer(cursor.ptr)).FskipNext; got != faultCode {
		t.Fatalf("raw cursor fault code = %d, want %d", got, faultCode)
	}
}

func TestMinweightCursorRestoreRefreshesChangedRow(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()

	engine := NewMinweightStorageEngine().(*minweightStorageEngine)
	bt := &minweightBtree{minweightDatabase: minweightNewDatabase()}
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
		var token uintptr
		rc := engine.BtreeOpen(
			ctx,
			BtreeVFSHandle{},
			BtreeCStringHandle{tls: tls, ptr: zFilename},
			SQLiteHandle{tls: tls, ptr: db},
			BtreeMemoryHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&token))},
			0,
			vfsFlags,
		)
		if rc != SQLITE_OK {
			t.Fatalf("BtreeOpen rc = %d, want SQLITE_OK", rc)
		}
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
		var token uintptr
		rc := engine.BtreeOpen(
			ctx,
			BtreeVFSHandle{},
			BtreeCStringHandle{tls: tls, ptr: zFilename},
			SQLiteHandle{},
			BtreeMemoryHandle{tls: tls, ptr: uintptr(unsafe.Pointer(&token))},
			0,
			int32(SQLITE_OPEN_READWRITE|SQLITE_OPEN_CREATE|SQLITE_OPEN_SHAREDCACHE),
		)
		if rc != SQLITE_OK {
			t.Fatalf("BtreeOpen rc = %d, want SQLITE_OK", rc)
		}
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
	bt := &minweightBtree{minweightDatabase: minweightNewDatabase()}
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
	bt := &minweightBtree{minweightDatabase: minweightNewDatabase()}
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
	bt := &minweightBtree{minweightDatabase: minweightNewDatabase()}
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
