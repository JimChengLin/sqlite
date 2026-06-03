// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"bytes"
	"strings"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

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
