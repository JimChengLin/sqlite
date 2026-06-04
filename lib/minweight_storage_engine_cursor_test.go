// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"bytes"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

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

func TestMinweightIndexCursorRestoreSeeksAfterDeletedRow(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	root := uint32(2)
	h.bt.tables[root] = minweightTable{intKey: false}
	keyInfo := minweightTestKeyInfo(t, h.tls, []string{"BINARY"}, nil)
	recordA := minweightTestRecord(minweightTestTextRecord("a"))
	recordB := minweightTestRecord(minweightTestTextRecord("b"))
	h.putIndexRecord(t, root, keyInfo, recordA)
	h.putIndexRecord(t, root, keyInfo, recordB)
	cursor := h.indexCursor(t, root, keyInfo)
	var result int32
	if rc := h.engine.BtreeFirst(h.ctx, cursor, BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&result))}); rc != SQLITE_OK {
		t.Fatalf("BtreeFirst rc = %d, want SQLITE_OK", rc)
	}
	if result != 0 {
		t.Fatalf("BtreeFirst result = %d, want 0", result)
	}
	h.assertIndexCursorRecord(t, cursor, recordA)

	if rc := h.engine.BtreeDelete(h.ctx, cursor, 0); rc != SQLITE_OK {
		t.Fatalf("BtreeDelete rc = %d, want SQLITE_OK", rc)
	}
	if got := h.engine.BtreeCursorHasMoved(h.ctx, cursor); got != 1 {
		t.Fatalf("BtreeCursorHasMoved = %d, want 1", got)
	}
	var differentRow int32
	if rc := h.engine.BtreeCursorRestore(h.ctx, cursor, BtreeMemoryHandle{tls: h.tls, ptr: uintptr(unsafe.Pointer(&differentRow))}); rc != SQLITE_OK {
		t.Fatalf("BtreeCursorRestore rc = %d, want SQLITE_OK", rc)
	}
	if differentRow != 1 {
		t.Fatalf("differentRow = %d, want 1", differentRow)
	}
	h.assertIndexCursorRecord(t, cursor, recordB)
}
