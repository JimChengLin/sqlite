// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
	"unsafe"

	minweight "github.com/JimChengLin/minweight_store"
	"modernc.org/libc"
)

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

func TestMinweightIndexStoreKeyWithoutKeyInfoUsesVersionedKey(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	record := minweightTestRecord(minweightTestTextRecord("alpha"), minweightTestIntRecord(1))
	key, err := minweightIndexStoreKey(BtreeContext{tls: tls}, 0, 9, record)
	if err != nil {
		t.Fatal(err)
	}
	if !minweightIndexKeyVersionedForRoot(9, key) {
		t.Fatalf("store key = %x, want versioned index key for root 9", key)
	}
	row := minweightDecodeRow(minweight.Item{Key: key, Value: record}, false)
	if !bytes.Equal(row.key, record) || !bytes.Equal(row.payload, record) {
		t.Fatalf("decoded logical key/payload = %x/%x, want %x", row.key, row.payload, record)
	}
}

func TestMinweightComparableMemKeyIgnoresNegativeLengthForInteger(t *testing.T) {
	var mem TMem
	*(*int64)(unsafe.Pointer(&mem)) = 42
	mem.Fflags = uint16(MEM_Int)
	mem.Fn = -1

	key, err := minweightComparableMemKey(BtreeContext{}, 0, 0, uintptr(unsafe.Pointer(&mem)))
	if err != nil {
		t.Fatal(err)
	}
	want := minweightAppendNumberKey(nil, false, 42, 0)
	if !bytes.Equal(key, want) {
		t.Fatalf("integer mem key = %x, want %x", key, want)
	}
}

func TestMinweightStatsRejectLegacyRawIndexKey(t *testing.T) {
	h := newMinweightBtreeTestHarness(t)
	root := uint32(2)
	h.bt.tables[root] = minweightTable{intKey: false}
	record := minweightTestRecord(minweightTestTextRecord("raw"))
	rawKey := append(minweightRootPrefix(root, false), record...)
	if err := h.bt.store.Put(rawKey, record); err != nil {
		t.Fatal(err)
	}
	if err := h.bt.recomputeTableStats(); !errors.Is(err, minweight.ErrManifest) {
		t.Fatalf("recomputeTableStats error = %v, want minweight.ErrManifest", err)
	}
}
