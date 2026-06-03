// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin || linux

package sqlite // import "modernc.org/sqlite"

import sqlite3 "modernc.org/sqlite/lib"

// NewMinweightStorageEngine returns a StorageEngine backed by minweight_store.
func NewMinweightStorageEngine() StorageEngine {
	return sqlite3.NewMinweightStorageEngine()
}
