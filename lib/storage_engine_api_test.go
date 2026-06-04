// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite3

import "testing"

type testStorageEngine struct {
	nativeBtreeStorageEngine
}

func TestStorageEngineConnectionClosedClearsDBBinding(t *testing.T) {
	engine := testStorageEngine{}
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
