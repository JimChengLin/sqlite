// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite // import "modernc.org/sqlite"

import sqlite3 "modernc.org/sqlite/lib"

// StorageEngine is the btree storage engine dispatch surface.
type StorageEngine = sqlite3.StorageEngine

// StorageEngineBtreeSetMmapLimit is implemented by engines on platforms where
// BtreeSetMmapLimit exists.
type StorageEngineBtreeSetMmapLimit = sqlite3.StorageEngineBtreeSetMmapLimit

// StorageEngineBtreeIsEmpty is implemented by engines on platforms where
// BtreeIsEmpty exists.
type StorageEngineBtreeIsEmpty = sqlite3.StorageEngineBtreeIsEmpty

// StorageEngineBtreeClearCache is implemented by engines on platforms where
// BtreeClearCache exists.
type StorageEngineBtreeClearCache = sqlite3.StorageEngineBtreeClearCache

// StorageEngineBtreeIntegrityCheck is implemented by engines on platforms
// where BtreeIntegrityCheck uses the current SQLite ABI.
type StorageEngineBtreeIntegrityCheck = sqlite3.StorageEngineBtreeIntegrityCheck

// StorageEngineBtreeIntegrityCheckFreebsd386 is implemented by engines on
// freebsd/386 where BtreeIntegrityCheck uses that platform's ABI.
type StorageEngineBtreeIntegrityCheckFreebsd386 = sqlite3.StorageEngineBtreeIntegrityCheckFreebsd386

// StorageEngineBtreeIntegrityCheckNetbsdAmd64 is implemented by engines on
// netbsd/amd64 where BtreeIntegrityCheck uses that platform's ABI.
type StorageEngineBtreeIntegrityCheckNetbsdAmd64 = sqlite3.StorageEngineBtreeIntegrityCheckNetbsdAmd64

// StorageEngineLogicalBackup is implemented by engines that model logical
// backup source state outside SQLite's native btree.
type StorageEngineLogicalBackup = sqlite3.StorageEngineLogicalBackup

// BtreeContext is the per-call SQLite runtime context seen by storage engines.
type BtreeContext = sqlite3.BtreeContext

// BtreeHandle identifies a SQLite btree object for the active storage engine.
type BtreeHandle = sqlite3.BtreeHandle

// SQLiteHandle identifies the owning sqlite3 connection.
type SQLiteHandle = sqlite3.SQLiteHandle

// BtreeVFSHandle identifies the SQLite VFS passed to btree open.
type BtreeVFSHandle = sqlite3.BtreeVFSHandle

// BtreeCursorHandle identifies a btree cursor for the active storage engine.
type BtreeCursorHandle = sqlite3.BtreeCursorHandle

// BtreeMemoryHandle identifies SQLite-owned memory used for out parameters or buffers.
type BtreeMemoryHandle = sqlite3.BtreeMemoryHandle

// BtreeCStringHandle identifies a SQLite-owned C string.
type BtreeCStringHandle = sqlite3.BtreeCStringHandle

// BtreePayloadHandle identifies SQLite's internal insert payload descriptor.
type BtreePayloadHandle = sqlite3.BtreePayloadHandle

// BtreePagerHandle identifies SQLite's pager object for a btree.
type BtreePagerHandle = sqlite3.BtreePagerHandle

// BtreeSchemaHandle identifies SQLite schema memory associated with a btree.
type BtreeSchemaHandle = sqlite3.BtreeSchemaHandle

// BtreeKeyInfoHandle identifies SQLite key metadata used to open an index cursor.
type BtreeKeyInfoHandle = sqlite3.BtreeKeyInfoHandle

// BtreeIndexKeyHandle identifies SQLite's unpacked index key for cursor movement.
type BtreeIndexKeyHandle = sqlite3.BtreeIndexKeyHandle

// BtreeFunctionHandle identifies a SQLite function pointer passed through btree APIs.
type BtreeFunctionHandle = sqlite3.BtreeFunctionHandle

// BtreeToken is an opaque btree identity token.
type BtreeToken = sqlite3.BtreeToken

// SQLiteToken is an opaque sqlite3 connection identity token.
type SQLiteToken = sqlite3.SQLiteToken

// BtreeVFSToken is an opaque VFS identity token.
type BtreeVFSToken = sqlite3.BtreeVFSToken

// BtreeCursorToken is an opaque cursor identity token.
type BtreeCursorToken = sqlite3.BtreeCursorToken

// BtreeMemoryToken is an opaque memory identity token.
type BtreeMemoryToken = sqlite3.BtreeMemoryToken

// BtreeCStringToken is an opaque C string identity token.
type BtreeCStringToken = sqlite3.BtreeCStringToken

// BtreePayloadToken is an opaque payload identity token.
type BtreePayloadToken = sqlite3.BtreePayloadToken

// BtreePagerToken is an opaque pager identity token.
type BtreePagerToken = sqlite3.BtreePagerToken

// BtreeSchemaToken is an opaque schema identity token.
type BtreeSchemaToken = sqlite3.BtreeSchemaToken

// BtreeKeyInfoToken is an opaque key-info identity token.
type BtreeKeyInfoToken = sqlite3.BtreeKeyInfoToken

// BtreeIndexKeyToken is an opaque index-key identity token.
type BtreeIndexKeyToken = sqlite3.BtreeIndexKeyToken

// BtreeFunctionToken is an opaque function identity token.
type BtreeFunctionToken = sqlite3.BtreeFunctionToken

// SetStorageEngine sets the btree storage engine. Passing nil restores the
// generated SQLite btree implementation.
func SetStorageEngine(engine StorageEngine) {
	sqlite3.SetStorageEngine(engine)
}

// ErrStorageEngineUnsupported reports an operation the engine does not implement.
var ErrStorageEngineUnsupported = sqlite3.ErrStorageEngineUnsupported
