// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite3

import "testing"

type testStorageEngine struct {
	nativeBtreeStorageEngine
	id int
}

func TestStorageEngineConnectionClosedClearsDBBinding(t *testing.T) {
	engine := testStorageEngine{id: 1}
	SetStorageEngine(engine)
	t.Cleanup(func() {
		SetStorageEngine(nil)
		unregisterStorageEngineBtree(BtreeHandle{ptr: 0x200})
		unregisterStorageEngineDB(SQLiteHandle{ptr: 0x100})
	})

	db := SQLiteHandle{ptr: 0x100}
	btree := BtreeHandle{ptr: 0x200}
	registerStorageEngineBtree(btree, db, engine)
	if got := storageEngineForDB(db); got != engine {
		t.Fatalf("bound engine = %T, want %T", got, engine)
	}

	SetStorageEngine(nil)
	if _, ok := storageEngineForDB(db).(nativeBtreeStorageEngine); ok {
		t.Fatal("db binding was lost before connection close")
	}

	StorageEngineConnectionClosed(nil, db.ptr)
	if _, ok := storageEngineForDB(db).(nativeBtreeStorageEngine); !ok {
		t.Fatalf("db binding survived connection close: %T", storageEngineForDB(db))
	}
}

func TestStorageEngineDBBindingTracksBtreeRefs(t *testing.T) {
	engine := testStorageEngine{id: 1}
	db := SQLiteHandle{ptr: 0x110}
	first := BtreeHandle{ptr: 0x210}
	second := BtreeHandle{ptr: 0x220}
	t.Cleanup(func() {
		unregisterStorageEngineBtree(first)
		unregisterStorageEngineBtree(second)
		unregisterStorageEngineDB(db)
	})

	registerStorageEngineBtree(first, db, engine)
	registerStorageEngineBtree(second, db, engine)
	if got := storageEngineForDB(db); got != engine {
		t.Fatalf("db engine with two refs = %#v, want %#v", got, engine)
	}

	unregisterStorageEngineBtree(first)
	if got := storageEngineForDB(db); got != engine {
		t.Fatalf("db engine after one btree close = %#v, want %#v", got, engine)
	}

	unregisterStorageEngineBtree(second)
	if _, ok := storageEngineForDB(db).(nativeBtreeStorageEngine); !ok {
		t.Fatalf("db engine after last btree close = %T, want native", storageEngineForDB(db))
	}
}

func TestStorageEngineConnectionCloseKeepsOpenBtreeBinding(t *testing.T) {
	engine := testStorageEngine{id: 1}
	db := SQLiteHandle{ptr: 0x120}
	btree := BtreeHandle{ptr: 0x230}
	t.Cleanup(func() {
		unregisterStorageEngineBtree(btree)
		unregisterStorageEngineDB(db)
	})

	registerStorageEngineBtree(btree, db, engine)
	unregisterStorageEngineDB(db)
	if _, ok := storageEngineForDB(db).(nativeBtreeStorageEngine); !ok {
		t.Fatalf("db binding after connection close = %T, want native", storageEngineForDB(db))
	}
	if got := storageEngineForBtreeHandle(btree); got != engine {
		t.Fatalf("open btree engine after db close = %#v, want %#v", got, engine)
	}
}

func TestStorageEngineCursorDispatchesThroughBtreeBinding(t *testing.T) {
	engine := testStorageEngine{id: 1}
	db := SQLiteHandle{ptr: 0x130}
	btree := BtreeHandle{ptr: 0x240}
	cursor := BtreeCursorHandle{ptr: 0x340}
	t.Cleanup(func() {
		SetStorageEngine(nil)
		unregisterStorageEngineCursor(cursor)
		unregisterStorageEngineBtree(btree)
		unregisterStorageEngineDB(db)
	})

	SetStorageEngine(engine)
	registerStorageEngineBtree(btree, db, engine)
	registerStorageEngineCursor(cursor, btree)
	SetStorageEngine(nil)
	if got := storageEngineForCursorHandle(cursor); got != engine {
		t.Fatalf("cursor engine after global switch = %#v, want %#v", got, engine)
	}

	unregisterStorageEngineBtree(btree)
	if _, ok := storageEngineForCursorHandle(cursor).(nativeBtreeStorageEngine); !ok {
		t.Fatalf("cursor engine after btree unbind = %T, want native", storageEngineForCursorHandle(cursor))
	}
}
