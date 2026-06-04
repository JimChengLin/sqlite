// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"bytes"
	"testing"
)

func TestMinweightClearVersionedIndexRoot(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	root := uint32(2)
	h.bt.tables[root] = minweightTable{intKey: false}
	keyInfo := minweightTestKeyInfo(t, h.tls, []string{"BINARY"}, nil)
	recordA := minweightTestRecord(minweightTestTextRecord("a"))
	recordB := minweightTestRecord(minweightTestTextRecord("b"))
	keyA := h.putIndexRecord(t, root, keyInfo, recordA)
	keyB := h.putIndexRecord(t, root, keyInfo, recordB)

	n, err := h.bt.clearRoot(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("cleared rows = %d, want 2", n)
	}
	if _, ok, err := h.bt.seekIndexGE(root, minweightVersionedIndexLower(root), false); err != nil || ok {
		t.Fatalf("root has row after clear ok=%v err=%v, want empty", ok, err)
	}
	for _, key := range [][]byte{keyA, keyB} {
		if _, ok, err := h.bt.store.Get(key); err != nil || ok {
			t.Fatalf("old key %x exists=%v err=%v, want absent", key, ok, err)
		}
	}
}

func TestMinweightMoveVersionedIndexRoot(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	from := uint32(2)
	to := uint32(5)
	h.bt.tables[from] = minweightTable{intKey: false}
	h.bt.tables[to] = minweightTable{intKey: false}
	keyInfo := minweightTestKeyInfo(t, h.tls, []string{"BINARY"}, nil)
	recordA := minweightTestRecord(minweightTestTextRecord("a"))
	recordB := minweightTestRecord(minweightTestTextRecord("b"))
	oldKeyA := h.putIndexRecord(t, from, keyInfo, recordA)
	oldKeyB := h.putIndexRecord(t, from, keyInfo, recordB)

	if err := h.bt.moveRoot(from, to, false); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := h.bt.seekIndexGE(from, minweightVersionedIndexLower(from), false); err != nil || ok {
		t.Fatalf("source root has row after move ok=%v err=%v, want empty", ok, err)
	}
	row, ok, err := h.bt.seekIndexGE(to, minweightVersionedIndexLower(to), false)
	if err != nil || !ok {
		t.Fatalf("destination first row ok=%v err=%v, want row", ok, err)
	}
	if !bytes.Equal(row.payload, recordA) {
		t.Fatalf("destination first payload = %x, want %x", row.payload, recordA)
	}
	if !minweightIndexKeyVersionedForRoot(to, row.storeKey) {
		t.Fatalf("destination store key = %x, want versioned root %d", row.storeKey, to)
	}
	for _, key := range [][]byte{oldKeyA, oldKeyB} {
		if _, ok, err := h.bt.store.Get(key); err != nil || ok {
			t.Fatalf("old key %x exists=%v err=%v, want absent", key, ok, err)
		}
	}
}
