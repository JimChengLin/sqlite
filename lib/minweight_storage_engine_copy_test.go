// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"bytes"
	"testing"

	"modernc.org/libc"
)

func newMinweightCopyFileHarness(t *testing.T) (BtreeContext, *minweightStorageEngine, *minweightBtree, *minweightBtree, BtreeHandle, BtreeHandle) {
	t.Helper()
	tls := libc.NewTLS()
	t.Cleanup(tls.Close)
	if rc := Xsqlite3_initialize(tls); rc != SQLITE_OK {
		t.Fatalf("sqlite3_initialize rc = %d, want SQLITE_OK", rc)
	}
	engine := NewMinweightStorageEngine().(*minweightStorageEngine)
	src := &minweightBtree{minweightDatabase: newMinweightTestDatabase()}
	dst := &minweightBtree{minweightDatabase: newMinweightTestDatabase()}
	engine.btrees[1] = src
	engine.btrees[2] = dst
	ctx := BtreeContext{tls: tls}
	srcHandle := BtreeHandle{tls: tls, ptr: 1}
	dstHandle := BtreeHandle{tls: tls, ptr: 2}
	return ctx, engine, src, dst, srcHandle, dstHandle
}

func minweightPutCommittedTestRow(t *testing.T, bt *minweightBtree, rowid int64, payload string) {
	t.Helper()
	if err := bt.store.Put(minweightTableKey(1, rowid), []byte(payload)); err != nil {
		t.Fatal(err)
	}
	bt.noteInsert(1, rowid, false)
	bt.dataVer++
}

func minweightAssertVisibleTestRow(t *testing.T, bt *minweightBtree, rowid int64, want string) {
	t.Helper()
	got, ok, err := bt.get(minweightTableKey(1, rowid))
	if err != nil {
		t.Fatal(err)
	}
	if want == "" {
		if ok {
			t.Fatalf("row %d visible payload = %q, want missing", rowid, got)
		}
		return
	}
	if !ok || !bytes.Equal(got, []byte(want)) {
		t.Fatalf("row %d visible payload = %q ok=%v, want %q", rowid, got, ok, want)
	}
}

func minweightAssertStoreTestRow(t *testing.T, bt *minweightBtree, rowid int64, want string) {
	t.Helper()
	got, ok, err := bt.store.Get(minweightTableKey(1, rowid))
	if err != nil {
		t.Fatal(err)
	}
	if want == "" {
		if ok {
			t.Fatalf("row %d store payload = %q, want missing", rowid, got)
		}
		return
	}
	if !ok || !bytes.Equal(got, []byte(want)) {
		t.Fatalf("row %d store payload = %q ok=%v, want %q", rowid, got, ok, want)
	}
}

func TestMinweightBtreeCopyFileUsesSourceOverlayAndTargetTxn(t *testing.T) {
	ctx, engine, src, dst, srcHandle, dstHandle := newMinweightCopyFileHarness(t)
	minweightPutCommittedTestRow(t, src, 1, "one")
	minweightPutCommittedTestRow(t, src, 2, "two")
	minweightPutCommittedTestRow(t, dst, 99, "trash")
	if rc := engine.BtreeBeginTrans(ctx, srcHandle, 1, BtreeMemoryHandle{}); rc != SQLITE_OK {
		t.Fatalf("source BtreeBeginTrans rc = %d, want SQLITE_OK", rc)
	}
	deleted, err := src.delete(minweightTableKey(1, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := src.noteDelete(1, minweightRow{rowid: 1}, deleted, true); err != nil {
		t.Fatal(err)
	}
	if err := src.put(minweightTableKey(1, 2), []byte("two-updated")); err != nil {
		t.Fatal(err)
	}
	if err := src.put(minweightTableKey(1, 3), []byte("three")); err != nil {
		t.Fatal(err)
	}
	src.noteInsert(1, 3, false)

	if rc := engine.BtreeBeginTrans(ctx, dstHandle, 1, BtreeMemoryHandle{}); rc != SQLITE_OK {
		t.Fatalf("target BtreeBeginTrans rc = %d, want SQLITE_OK", rc)
	}
	if rc := engine.BtreeCopyFile(ctx, dstHandle, srcHandle); rc != SQLITE_OK {
		t.Fatalf("BtreeCopyFile rc = %d, want SQLITE_OK", rc)
	}
	minweightAssertVisibleTestRow(t, dst, 1, "")
	minweightAssertVisibleTestRow(t, dst, 2, "two-updated")
	minweightAssertVisibleTestRow(t, dst, 3, "three")
	minweightAssertVisibleTestRow(t, dst, 99, "")
	if _, ok, err := dst.store.Get(minweightTableKey(1, 99)); err != nil || !ok {
		t.Fatalf("target committed row before commit exists=%v err=%v, want still committed", ok, err)
	}
	table := dst.visibleState().tables[1]
	if table.rowCount != 2 || table.minRowid != 2 || table.maxRowid != 3 {
		t.Fatalf("visible target stats = %+v, want rows 2..3", table)
	}

	if rc := engine.BtreeCommit(ctx, dstHandle); rc != SQLITE_OK {
		t.Fatalf("target BtreeCommit rc = %d, want SQLITE_OK", rc)
	}
	minweightAssertStoreTestRow(t, dst, 99, "")
	minweightAssertStoreTestRow(t, dst, 2, "two-updated")
	minweightAssertStoreTestRow(t, dst, 3, "three")
}
