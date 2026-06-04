// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMinweightStorageEngineJournalModeWALStaysRollback(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	path := filepath.Join(t.TempDir(), "wal-disabled.db")
	walPath := path + "-wal"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, db)

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "delete" {
		t.Fatalf("journal_mode after WAL request = %q, want delete", mode)
	}

	execMinweightSQL(t, db, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, db, "INSERT INTO t(v) VALUES ('one')")

	if _, err := os.Stat(walPath); err == nil {
		t.Fatalf("minweight created WAL placeholder %s", walPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}
