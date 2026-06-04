// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"bytes"
	"testing"
)

func minweightBeginWriteTxn(t *testing.T, bt *minweightBtree, ctx BtreeContext) {
	t.Helper()
	if rc, _ := bt.beginTrans(ctx, 1); rc != SQLITE_OK {
		t.Fatalf("beginTrans rc = %d, want SQLITE_OK", rc)
	}
}

func minweightCommitTableGenerationChange(t *testing.T, h *minweightBtreeTestHarness, updateKey []byte, insertKey []byte, deleteKey []byte) {
	t.Helper()
	minweightBeginWriteTxn(t, h.bt, h.ctx)
	if err := h.bt.put(updateKey, []byte("new-10")); err != nil {
		t.Fatal(err)
	}
	if err := h.bt.put(insertKey, []byte("new-20")); err != nil {
		t.Fatal(err)
	}
	if _, err := h.bt.delete(deleteKey); err != nil {
		t.Fatal(err)
	}
	if err := h.bt.commitActiveWriteTxn(); err != nil {
		t.Fatal(err)
	}
	h.bt.releaseTrans()
}

func minweightCommitTableSecondUpdate(t *testing.T, h *minweightBtreeTestHarness, updateKey []byte) {
	t.Helper()
	minweightBeginWriteTxn(t, h.bt, h.ctx)
	if err := h.bt.put(updateKey, []byte("newer-10")); err != nil {
		t.Fatal(err)
	}
	if err := h.bt.commitActiveWriteTxn(); err != nil {
		t.Fatal(err)
	}
	h.bt.releaseTrans()
}

func TestMinweightPinnedReaderTableSeekUsesOldGeneration(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	reader := &minweightBtree{minweightDatabase: h.bt.minweightDatabase}
	updateKey := minweightTableKey(1, 10)
	insertKey := minweightTableKey(1, 20)
	deleteKey := minweightTableKey(1, 30)
	if err := h.bt.store.Put(updateKey, []byte("old-10")); err != nil {
		t.Fatal(err)
	}
	if err := h.bt.store.Put(deleteKey, []byte("old-30")); err != nil {
		t.Fatal(err)
	}

	reader.retainReader()
	minweightCommitTableGenerationChange(t, h, updateKey, insertKey, deleteKey)
	minweightCommitTableSecondUpdate(t, h, updateKey)

	row, ok, err := reader.seekTableGE(1, 15)
	if err != nil || !ok || row.rowid != 30 || !bytes.Equal(row.payload, []byte("old-30")) {
		t.Fatalf("reader seekTableGE row=%d payload=%q ok=%v err=%v, want old rowid 30", row.rowid, row.payload, ok, err)
	}
	row, ok, err = reader.seekTableLE(1, 25)
	if err != nil || !ok || row.rowid != 10 || !bytes.Equal(row.payload, []byte("old-10")) {
		t.Fatalf("reader seekTableLE row=%d payload=%q ok=%v err=%v, want old rowid 10", row.rowid, row.payload, ok, err)
	}
	row, ok, err = reader.seekTableGE(1, 10)
	if err != nil || !ok || row.rowid != 10 || !bytes.Equal(row.payload, []byte("old-10")) {
		t.Fatalf("reader seekTableGE(update) row=%d payload=%q ok=%v err=%v, want old rowid 10", row.rowid, row.payload, ok, err)
	}

	row, ok, err = h.bt.seekTableGE(1, 15)
	if err != nil || !ok || row.rowid != 20 || !bytes.Equal(row.payload, []byte("new-20")) {
		t.Fatalf("current seekTableGE row=%d payload=%q ok=%v err=%v, want current rowid 20", row.rowid, row.payload, ok, err)
	}
	row, ok, err = h.bt.seekTableLE(1, 30)
	if err != nil || !ok || row.rowid != 20 || !bytes.Equal(row.payload, []byte("new-20")) {
		t.Fatalf("current seekTableLE row=%d payload=%q ok=%v err=%v, want current rowid 20", row.rowid, row.payload, ok, err)
	}

	reader.releaseReader()
}

func TestMinweightPinnedReaderIndexSeekUsesOldGeneration(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	reader := &minweightBtree{minweightDatabase: h.bt.minweightDatabase}
	root := uint32(2)
	recordA := minweightTestRecord(minweightTestTextRecord("a"))
	recordB := minweightTestRecord(minweightTestTextRecord("b"))
	recordC := minweightTestRecord(minweightTestTextRecord("c"))
	keyA, err := minweightIndexStoreKey(h.ctx, 0, root, recordA)
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := minweightIndexStoreKey(h.ctx, 0, root, recordB)
	if err != nil {
		t.Fatal(err)
	}
	keyC, err := minweightIndexStoreKey(h.ctx, 0, root, recordC)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.bt.store.Put(keyA, recordA); err != nil {
		t.Fatal(err)
	}
	if err := h.bt.store.Put(keyC, recordC); err != nil {
		t.Fatal(err)
	}

	reader.retainReader()
	minweightBeginWriteTxn(t, h.bt, h.ctx)
	if err := h.bt.put(keyB, recordB); err != nil {
		t.Fatal(err)
	}
	if _, err := h.bt.delete(keyC); err != nil {
		t.Fatal(err)
	}
	if err := h.bt.commitActiveWriteTxn(); err != nil {
		t.Fatal(err)
	}
	h.bt.releaseTrans()

	row, ok, err := reader.seekIndexGE(root, keyB, false)
	if err != nil || !ok || !bytes.Equal(row.storeKey, keyC) || !bytes.Equal(row.payload, recordC) {
		t.Fatalf("reader seekIndexGE key=%x payload=%x ok=%v err=%v, want old C", row.storeKey, row.payload, ok, err)
	}
	row, ok, err = reader.seekIndexLE(root, keyB, false)
	if err != nil || !ok || !bytes.Equal(row.storeKey, keyA) || !bytes.Equal(row.payload, recordA) {
		t.Fatalf("reader seekIndexLE key=%x payload=%x ok=%v err=%v, want old A", row.storeKey, row.payload, ok, err)
	}
	row, ok, err = h.bt.seekIndexGE(root, keyB, false)
	if err != nil || !ok || !bytes.Equal(row.storeKey, keyB) || !bytes.Equal(row.payload, recordB) {
		t.Fatalf("current seekIndexGE key=%x payload=%x ok=%v err=%v, want current B", row.storeKey, row.payload, ok, err)
	}

	reader.releaseReader()
}

func TestMinweightWriteTxnTableSeekUsesBaseGenerationAndOverlay(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	updateKey := minweightTableKey(1, 10)
	insertKey := minweightTableKey(1, 20)
	overlayKey := minweightTableKey(1, 25)
	deleteKey := minweightTableKey(1, 30)
	if err := h.bt.store.Put(updateKey, []byte("old-10")); err != nil {
		t.Fatal(err)
	}
	if err := h.bt.store.Put(deleteKey, []byte("old-30")); err != nil {
		t.Fatal(err)
	}

	minweightBeginWriteTxn(t, h.bt, h.ctx)
	if err := h.bt.store.Put(updateKey, []byte("new-10")); err != nil {
		t.Fatal(err)
	}
	if err := h.bt.store.Put(insertKey, []byte("new-20")); err != nil {
		t.Fatal(err)
	}
	if _, err := h.bt.store.Delete(deleteKey); err != nil {
		t.Fatal(err)
	}
	h.bt.changes = append(h.bt.changes, minweightCommitChange{
		generation: 2,
		keys: map[string]minweightCommittedKeyChange{
			string(updateKey): {key: updateKey, before: []byte("old-10"), beforeExist: true, after: []byte("new-10"), afterExists: true},
			string(insertKey): {key: insertKey, after: []byte("new-20"), afterExists: true},
			string(deleteKey): {key: deleteKey, before: []byte("old-30"), beforeExist: true},
		},
	})
	h.bt.generation = 2

	row, ok, err := h.bt.seekTableGE(1, 15)
	if err != nil || !ok || row.rowid != 30 || !bytes.Equal(row.payload, []byte("old-30")) {
		t.Fatalf("writer seekTableGE row=%d payload=%q ok=%v err=%v, want base rowid 30", row.rowid, row.payload, ok, err)
	}
	row, ok, err = h.bt.seekTableLE(1, 25)
	if err != nil || !ok || row.rowid != 10 || !bytes.Equal(row.payload, []byte("old-10")) {
		t.Fatalf("writer seekTableLE row=%d payload=%q ok=%v err=%v, want base rowid 10", row.rowid, row.payload, ok, err)
	}
	if err := h.bt.put(overlayKey, []byte("own-25")); err != nil {
		t.Fatal(err)
	}
	row, ok, err = h.bt.seekTableGE(1, 15)
	if err != nil || !ok || row.rowid != 25 || !bytes.Equal(row.payload, []byte("own-25")) {
		t.Fatalf("writer overlay seekTableGE row=%d payload=%q ok=%v err=%v, want own rowid 25", row.rowid, row.payload, ok, err)
	}

	h.bt.releaseTrans()
}

func TestMinweightWriteTxnDeleteUsesBaseGeneration(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	key := minweightTableKey(1, 42)
	minweightBeginWriteTxn(t, h.bt, h.ctx)
	if err := h.bt.store.Put(key, []byte("new-after-base")); err != nil {
		t.Fatal(err)
	}
	h.bt.changes = append(h.bt.changes, minweightCommitChange{
		generation: 2,
		keys: map[string]minweightCommittedKeyChange{
			string(key): {
				key:         append([]byte(nil), key...),
				after:       []byte("new-after-base"),
				afterExists: true,
			},
		},
	})
	h.bt.generation = 2

	deleted, err := h.bt.delete(key)
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("delete reported key existed, want absent at transaction base generation")
	}
	h.bt.releaseTrans()
}
