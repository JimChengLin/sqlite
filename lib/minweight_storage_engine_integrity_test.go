// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"encoding/binary"
	"strings"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

func TestMinweightIntegrityCheckSeesWriterOverlay(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	if rc, _ := h.bt.beginTrans(h.ctx, 1); rc != SQLITE_OK {
		t.Fatalf("beginTrans rc = %d, want SQLITE_OK", rc)
	}
	t.Cleanup(h.bt.releaseTrans)
	if err := h.bt.put(minweightTableKey(1, 1), []byte("txn")); err != nil {
		t.Fatal(err)
	}
	h.bt.noteInsert(1, 1, false)

	pnErr, pzOut := minweightRunIntegrityCheck(t, h, nil, 0)
	if pnErr != 0 {
		t.Fatalf("pnErr = %d, want 0", pnErr)
	}
	if pzOut != 0 {
		t.Fatalf("pzOut = %#x, want 0", pzOut)
	}
}

func TestMinweightIntegrityCheckPartialRejectsSelectedRawIndexKey(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	root := uint32(2)
	h.bt.tables[root] = minweightTable{intKey: false}
	h.bt.next = root
	record := minweightTestRecord(minweightTestTextRecord("raw"))
	rawKey := append(minweightRootPrefix(root, false), record...)
	if err := h.bt.store.Put(rawKey, record); err != nil {
		t.Fatal(err)
	}
	if minweightIndexKeyInVersionedRange(root, rawKey) {
		t.Fatalf("raw key %x unexpectedly looks versioned", rawKey)
	}

	roots := []uint32{0, root}
	rootHandle := minweightIntegrityRootHandle(t, h, roots)
	check := newMinweightIntegrityCheck(h.bt, rootHandle, int32(len(roots)), 100)
	if err := check.run(); err != nil {
		t.Fatal(err)
	}
	if len(check.errors) == 0 {
		t.Fatal("direct integrity check errors = 0, want selected raw index corruption")
	}
	pnErr, pzOut := minweightRunIntegrityCheck(t, h, roots, int32(len(roots)))
	if pnErr == 0 {
		t.Fatal("pnErr = 0, want selected raw index corruption")
	}
	defer Xsqlite3_free(h.tls, pzOut)
	if got := libc.GoString(pzOut); !strings.Contains(got, "unsupported raw index key in root 2") {
		t.Fatalf("integrity text = %q", got)
	}
}

func minweightRunIntegrityCheck(t *testing.T, h *minweightBtreeTestHarness, roots []uint32, nRoot int32) (int32, uintptr) {
	t.Helper()
	rootHandle := minweightIntegrityRootHandle(t, h, roots)
	pnErrHandle := minweightIntegrityOutHandle(t, h, int(unsafe.Sizeof(int32(0))))
	pzOutHandle := minweightIntegrityOutHandle(t, h, int(unsafe.Sizeof(uintptr(0))))
	if rc := h.engine.BtreeIntegrityCheck(
		h.ctx,
		SQLiteHandle{},
		h.btree,
		rootHandle,
		BtreeMemoryHandle{},
		nRoot,
		100,
		pnErrHandle,
		pzOutHandle,
	); rc != SQLITE_OK {
		t.Fatalf("BtreeIntegrityCheck rc = %d, want SQLITE_OK", rc)
	}
	return *storageEngineInt32FromPointer(pnErrHandle.ptr), pzOutHandle.GetUintptr()
}

func minweightIntegrityRootHandle(t *testing.T, h *minweightBtreeTestHarness, roots []uint32) BtreeMemoryHandle {
	t.Helper()
	var rootHandle BtreeMemoryHandle
	if len(roots) != 0 {
		ptr := _sqlite3MallocZero(h.tls, uint64(len(roots)*4))
		if ptr == 0 {
			t.Fatal("sqlite3 malloc returned nil root array")
		}
		t.Cleanup(func() { Xsqlite3_free(h.tls, ptr) })
		rootBytes := minweightByteSliceFromPointer(ptr, len(roots)*4)
		for i, root := range roots {
			binary.NativeEndian.PutUint32(rootBytes[i*4:i*4+4], root)
		}
		rootHandle = BtreeMemoryHandle{tls: h.tls, ptr: ptr}
	}
	return rootHandle
}

func minweightIntegrityOutHandle(t *testing.T, h *minweightBtreeTestHarness, size int) BtreeMemoryHandle {
	t.Helper()
	ptr := _sqlite3MallocZero(h.tls, uint64(size))
	if ptr == 0 {
		t.Fatal("sqlite3 malloc returned nil integrity output")
	}
	t.Cleanup(func() { Xsqlite3_free(h.tls, ptr) })
	return BtreeMemoryHandle{tls: h.tls, ptr: ptr}
}
