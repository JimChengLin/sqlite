// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite_test

import (
	"reflect"
	"sync"
	"testing"

	sqlite "modernc.org/sqlite"
)

var _ sqlite.StorageEngine = externalStorageEngine{}

type externalStorageEngine struct {
	sqlite.StorageEngine
}

func TestStorageEngineAPIIsExternallyImplementable(t *testing.T) {}

func TestStorageEngineCanBeSelectedFromExternalPackage(t *testing.T) {
	sqlite.SetStorageEngine(externalStorageEngine{})
	sqlite.SetStorageEngine(nil)
}

func TestStorageEngineCanBeSelectedConcurrently(t *testing.T) {
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				sqlite.SetStorageEngine(externalStorageEngine{})
				sqlite.SetStorageEngine(nil)
			}
		}()
	}
	wg.Wait()
	sqlite.SetStorageEngine(nil)
}

func TestStorageEngineHandleAPIsAreExternallyReachable(t *testing.T) {
	var btree sqlite.BtreeHandle
	var sqliteDB sqlite.SQLiteHandle
	var vfs sqlite.BtreeVFSHandle
	var cursor sqlite.BtreeCursorHandle
	var memory sqlite.BtreeMemoryHandle
	var cstring sqlite.BtreeCStringHandle
	var payload sqlite.BtreePayloadHandle
	var pager sqlite.BtreePagerHandle
	var schema sqlite.BtreeSchemaHandle
	var keyInfo sqlite.BtreeKeyInfoHandle
	var indexKey sqlite.BtreeIndexKeyHandle
	var fn sqlite.BtreeFunctionHandle

	_ = btree.Token()
	_ = sqliteDB.Token()
	_ = vfs.Token()
	_ = cursor.Token()
	_ = memory.Token()
	_ = cstring.Token()
	_ = payload.Token()
	_ = pager.Token()
	_ = schema.Token()
	_ = keyInfo.Token()
	_ = indexKey.Token()
	_ = fn.Token()

	_ = sqlite.BtreeMemoryHandle.ReadBytes
	_ = sqlite.BtreeMemoryHandle.GetInt32
	_ = sqlite.BtreeMemoryHandle.GetUint32
	_ = sqlite.BtreeMemoryHandle.GetInt64
	_ = sqlite.BtreeMemoryHandle.GetUintptr
	_ = sqlite.BtreeMemoryHandle.PutBtreeToken

	_ = sqlite.BtreePayloadHandle.KeyHandle
	_ = sqlite.BtreePayloadHandle.KeySize
	_ = sqlite.BtreePayloadHandle.KeyBytes
	_ = sqlite.BtreePayloadHandle.DataHandle
	_ = sqlite.BtreePayloadHandle.DataSize
	_ = sqlite.BtreePayloadHandle.DataBytes
	_ = sqlite.BtreePayloadHandle.ZeroSize
	_ = sqlite.BtreePayloadHandle.MemoryHandle
	_ = sqlite.BtreePayloadHandle.MemoryCount
}

func TestStorageEngineAPIDoesNotExposeRawABIInputs(t *testing.T) {
	assertInterfaceDoesNotExposeRawABIInputs(t, reflect.TypeOf((*sqlite.StorageEngine)(nil)).Elem())
	assertInterfaceDoesNotExposeRawABIInputs(t, reflect.TypeOf((*sqlite.StorageEngineBtreeSetMmapLimit)(nil)).Elem())
	assertInterfaceDoesNotExposeRawABIInputs(t, reflect.TypeOf((*sqlite.StorageEngineBtreeIsEmpty)(nil)).Elem())
	assertInterfaceDoesNotExposeRawABIInputs(t, reflect.TypeOf((*sqlite.StorageEngineBtreeClearCache)(nil)).Elem())
	assertInterfaceDoesNotExposeRawABIInputs(t, reflect.TypeOf((*sqlite.StorageEngineBtreeIntegrityCheck)(nil)).Elem())
	assertInterfaceDoesNotExposeRawABIInputs(t, reflect.TypeOf((*sqlite.StorageEngineBtreeIntegrityCheckFreebsd386)(nil)).Elem())
	assertInterfaceDoesNotExposeRawABIInputs(t, reflect.TypeOf((*sqlite.StorageEngineBtreeIntegrityCheckNetbsdAmd64)(nil)).Elem())
}

func assertInterfaceDoesNotExposeRawABIInputs(t *testing.T, typ reflect.Type) {
	t.Helper()
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		for j := 1; j < m.Type.NumIn(); j++ {
			assertNotRawABIType(t, m.Name, m.Type.In(j))
		}
		for j := 0; j < m.Type.NumOut(); j++ {
			assertNotRawABIType(t, m.Name, m.Type.Out(j))
		}
	}
}

func assertNotRawABIType(t *testing.T, method string, typ reflect.Type) {
	t.Helper()
	if typ.Kind() == reflect.Uintptr {
		t.Fatalf("%s exposes raw uintptr", method)
	}
	if typ.Kind() == reflect.Pointer && typ.String() == "*libc.TLS" {
		t.Fatalf("%s exposes raw libc TLS", method)
	}
}
