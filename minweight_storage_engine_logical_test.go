// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	sqlite "modernc.org/sqlite"
)

func TestMinweightStorageEngineLogicalSerializePreservesSQLiteSequence(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	type serializer interface {
		Serialize() ([]byte, error)
		Deserialize([]byte) error
	}

	src, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, src)
	minweightCreateAdvancedSQLiteSequence(t, src)

	var snapshot []byte
	srcConn, err := src.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := srcConn.Raw(func(dc any) error {
		var err error
		snapshot, err = dc.(serializer).Serialize()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := srcConn.Close(); err != nil {
		t.Fatal(err)
	}

	dst, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, dst)

	dstConn, err := dst.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := dstConn.Raw(func(dc any) error {
		return dc.(serializer).Deserialize(snapshot)
	}); err != nil {
		t.Fatal(err)
	}
	if err := dstConn.Close(); err != nil {
		t.Fatal(err)
	}
	minweightAssertSQLiteSequenceAdvanced(t, dst)
}

func TestMinweightStorageEngineLogicalBackupPreservesSQLiteSequence(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	type backuper interface {
		NewBackup(string) (*sqlite.Backup, error)
	}

	src, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, src)
	minweightCreateAdvancedSQLiteSequence(t, src)

	path := filepath.Join(t.TempDir(), "backup.db")
	srcConn, err := src.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := srcConn.Raw(func(dc any) error {
		bck, err := dc.(backuper).NewBackup(path)
		if err != nil {
			return err
		}
		for more := true; more; {
			more, err = bck.Step(-1)
			if err != nil {
				_ = bck.Finish()
				return err
			}
		}
		return bck.Finish()
	}); err != nil {
		t.Fatal(err)
	}
	if err := srcConn.Close(); err != nil {
		t.Fatal(err)
	}

	dst, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, dst)
	minweightAssertSQLiteSequenceAdvanced(t, dst)
}

func TestMinweightStorageEngineLogicalSerializePreservesGeneratedColumns(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	type serializer interface {
		Serialize() ([]byte, error)
		Deserialize([]byte) error
	}

	src, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, src)
	minweightCreateGeneratedColumnTable(t, src)

	var snapshot []byte
	srcConn, err := src.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := srcConn.Raw(func(dc any) error {
		var err error
		snapshot, err = dc.(serializer).Serialize()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := srcConn.Close(); err != nil {
		t.Fatal(err)
	}

	dst, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, dst)

	dstConn, err := dst.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := dstConn.Raw(func(dc any) error {
		return dc.(serializer).Deserialize(snapshot)
	}); err != nil {
		t.Fatal(err)
	}
	if err := dstConn.Close(); err != nil {
		t.Fatal(err)
	}
	minweightAssertGeneratedColumnTable(t, dst)
}

func TestMinweightStorageEngineLogicalBackupPreservesGeneratedColumns(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	type backuper interface {
		NewBackup(string) (*sqlite.Backup, error)
	}

	src, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, src)
	minweightCreateGeneratedColumnTable(t, src)

	path := filepath.Join(t.TempDir(), "generated-backup.db")
	srcConn, err := src.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := srcConn.Raw(func(dc any) error {
		bck, err := dc.(backuper).NewBackup(path)
		if err != nil {
			return err
		}
		for more := true; more; {
			more, err = bck.Step(-1)
			if err != nil {
				_ = bck.Finish()
				return err
			}
		}
		return bck.Finish()
	}); err != nil {
		t.Fatal(err)
	}
	if err := srcConn.Close(); err != nil {
		t.Fatal(err)
	}

	dst, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, dst)
	minweightAssertGeneratedColumnTable(t, dst)
}

func minweightCreateAdvancedSQLiteSequence(t *testing.T, db *sql.DB) {
	t.Helper()
	execMinweightSQL(t, db, "CREATE TABLE t(id INTEGER PRIMARY KEY AUTOINCREMENT, v TEXT)")
	execMinweightSQL(t, db, "INSERT INTO t(id, v) VALUES (5, 'advance-sequence')")
	execMinweightSQL(t, db, "DELETE FROM t")
	if got := minweightQueryInt(t, db, "SELECT seq FROM sqlite_sequence WHERE name = 't'"); got != 5 {
		t.Fatalf("source sqlite_sequence seq = %d, want 5", got)
	}
}

func minweightAssertSQLiteSequenceAdvanced(t *testing.T, db *sql.DB) {
	t.Helper()
	if got := minweightQueryInt(t, db, "SELECT seq FROM sqlite_sequence WHERE name = 't'"); got != 5 {
		t.Fatalf("sqlite_sequence seq after logical copy = %d, want 5", got)
	}
	execMinweightSQL(t, db, "INSERT INTO t(v) VALUES ('after-copy')")
	if got := minweightQueryInt(t, db, "SELECT id FROM t WHERE v = 'after-copy'"); got != 6 {
		t.Fatalf("AUTOINCREMENT id after logical copy = %d, want 6", got)
	}
}

func minweightCreateGeneratedColumnTable(t *testing.T, db *sql.DB) {
	t.Helper()
	execMinweightSQL(t, db, `
		CREATE TABLE t(
			id INTEGER PRIMARY KEY,
			base INTEGER NOT NULL,
			doubled INTEGER GENERATED ALWAYS AS (base * 2) STORED,
			tripled INTEGER GENERATED ALWAYS AS (base * 3) VIRTUAL
		)
	`)
	execMinweightSQL(t, db, "INSERT INTO t(id, base) VALUES (7, 4), (9, 5)")
}

func minweightAssertGeneratedColumnTable(t *testing.T, db *sql.DB) {
	t.Helper()
	minweightAssertQueryStrings(t, db,
		"SELECT printf('%d:%d:%d:%d', id, base, doubled, tripled) FROM t ORDER BY id",
		[]string{"7:4:8:12", "9:5:10:15"},
	)
	execMinweightSQL(t, db, "INSERT INTO t(id, base) VALUES (11, 6)")
	minweightAssertQueryStrings(t, db,
		"SELECT printf('%d:%d:%d:%d', id, base, doubled, tripled) FROM t WHERE id = 11",
		[]string{"11:6:12:18"},
	)
}

func minweightAssertQueryStrings(t *testing.T, db *sql.DB, query string, want []string) {
	t.Helper()
	got := minweightQueryStrings(t, db, query)
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", query, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", query, got, want)
		}
	}
}
