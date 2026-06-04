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

func TestMinweightStorageEngineLogicalBackupPreservesHiddenRowid(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	type backuper interface {
		NewBackup(string) (*sqlite.Backup, error)
	}

	src, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, src)

	execMinweightSQL(t, src, "CREATE TABLE t(v TEXT UNIQUE)")
	execMinweightSQL(t, src, "INSERT INTO t(rowid, v) VALUES (42, 'alpha'), (99, 'beta')")

	path := filepath.Join(t.TempDir(), "rowid-backup.db")
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
	minweightAssertQueryStrings(t, dst,
		"SELECT printf('%d:%s', rowid, v) FROM t ORDER BY rowid",
		[]string{"42:alpha", "99:beta"},
	)
}

func TestMinweightStorageEngineLogicalSerializePreservesFTS5VirtualTable(t *testing.T) {
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
	minweightCreateFTS5Table(t, src)

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
	minweightAssertFTS5Table(t, dst)
}

func TestMinweightStorageEngineLogicalBackupPreservesFTS5VirtualTable(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	type backuper interface {
		NewBackup(string) (*sqlite.Backup, error)
	}

	src, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, src)
	minweightCreateFTS5Table(t, src)

	path := filepath.Join(t.TempDir(), "fts5-backup.db")
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
	minweightAssertFTS5Table(t, dst)
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

func minweightCreateFTS5Table(t *testing.T, db *sql.DB) {
	t.Helper()
	execMinweightSQL(t, db, "CREATE VIRTUAL TABLE docs USING fts5(title, body)")
	execMinweightSQL(t, db, "INSERT INTO docs(rowid, title, body) VALUES (3, 'minweight', 'storage engine'), (9, 'sqlite', 'btree compatibility')")
}

func minweightAssertFTS5Table(t *testing.T, db *sql.DB) {
	t.Helper()
	minweightAssertQueryStrings(t, db,
		"SELECT printf('%d:%s', rowid, title) FROM docs WHERE docs MATCH 'storage' ORDER BY rowid",
		[]string{"3:minweight"},
	)
	minweightAssertQueryStrings(t, db,
		"SELECT printf('%d:%s', rowid, title) FROM docs WHERE docs MATCH 'compatibility' ORDER BY rowid",
		[]string{"9:sqlite"},
	)
	execMinweightSQL(t, db, "INSERT INTO docs(rowid, title, body) VALUES (12, 'after', 'logical copy')")
	minweightAssertQueryStrings(t, db,
		"SELECT printf('%d:%s', rowid, title) FROM docs WHERE docs MATCH 'logical' ORDER BY rowid",
		[]string{"12:after"},
	)
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
