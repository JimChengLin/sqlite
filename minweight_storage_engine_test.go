// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func TestMinweightStorageEngineSimpleSPJ(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, "CREATE TABLE users(id INTEGER, name TEXT)")
	execMinweightSQL(t, db, "CREATE TABLE orders(user_id INTEGER, item TEXT)")
	execMinweightSQL(t, db, "INSERT INTO users VALUES (1, 'alice'), (2, 'bob')")
	execMinweightSQL(t, db, "INSERT INTO orders VALUES (1, 'book'), (1, 'pen'), (2, 'mug')")

	rows, err := db.Query(`
		SELECT users.name, orders.item
		  FROM users, orders
		 WHERE users.id = orders.user_id
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightRows(t, rows)

	var got []string
	for rows.Next() {
		var name, item string
		if err := rows.Scan(&name, &item); err != nil {
			t.Fatal(err)
		}
		got = append(got, name+"="+item)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{"alice=book", "alice=pen", "bob=mug"}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %v, want %v", got, want)
		}
	}
}

func TestMinweightStorageEngineUniqueTextLookup(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, "CREATE TABLE loginst(instid INTEGER PRIMARY KEY, name VARCHAR UNIQUE)")
	for i := 0; i < 16; i++ {
		name := "foo" + strconv.Itoa(i)
		if _, err := db.Exec("INSERT OR IGNORE INTO loginst(name) VALUES (?)", name); err != nil {
			t.Fatalf("insert iteration %d: %v", i, err)
		}
		var id int
		if err := db.QueryRow("SELECT instid FROM loginst WHERE name = ?", name).Scan(&id); err != nil {
			t.Fatalf("select iteration %d: %v", i, err)
		}
		if id != i+1 {
			t.Fatalf("iteration %d: id = %d, want %d", i, id, i+1)
		}
	}
}

func TestMinweightStorageEngineIntegrityCheck(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT UNIQUE)")
	execMinweightSQL(t, db, "INSERT INTO t(v) VALUES ('a'), ('b'), ('c')")

	var got string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Fatalf("integrity_check = %q, want ok", got)
	}
}

func TestMinweightStorageEngineQueryRowMultiStatement(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, "CREATE TABLE loginst(instid INTEGER PRIMARY KEY, name VARCHAR UNIQUE)")
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	for i := 0; i < 16; i++ {
		name := "foo" + strconv.Itoa(i)
		var id int
		if err := tx.QueryRow("INSERT OR IGNORE INTO loginst(name) VALUES (?); SELECT instid FROM loginst WHERE name = ?", name, name).Scan(&id); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if id != i+1 {
			t.Fatalf("iteration %d: id = %d, want %d", i, id, i+1)
		}
	}
}

func TestMinweightStorageEngineVarcharPrimaryKey(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, `
		CREATE TABLE products(
			id VARCHAR(255),
			user_id VARCHAR(255),
			name VARCHAR(255),
			PRIMARY KEY(id)
		)
	`)
	execMinweightSQL(t, db, `
		INSERT INTO products(id, user_id, name) VALUES ('9be4398c-d527-4efb-93a4-fc532cbaf804', 'u1', 'a');
		INSERT INTO products(id, user_id, name) VALUES ('759f10bd-9e1d-4ec7-b764-0868758d7b85', 'u1', 'b')
	`)

	var count int
	if err := db.QueryRow("SELECT count(*) FROM products WHERE user_id = 'u1'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	var name string
	if err := db.QueryRow("SELECT name FROM products WHERE id = ?", "759f10bd-9e1d-4ec7-b764-0868758d7b85").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "b" {
		t.Fatalf("name = %q, want b", name)
	}
}

func TestMinweightStorageEngineAttachCommitRollback(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.db")
	auxPath := filepath.Join(dir, "aux.db")
	db, err := sql.Open("sqlite", mainPath)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, db)

	execMinweightSQL(t, db, "CREATE TABLE main.t(id INTEGER PRIMARY KEY, v TEXT UNIQUE)")
	execMinweightSQL(t, db, "ATTACH DATABASE "+minweightSQLLiteral(auxPath)+" AS aux")
	execMinweightSQL(t, db, "CREATE TABLE aux.t(id INTEGER PRIMARY KEY, main_id INTEGER, v TEXT UNIQUE)")

	execMinweightSQL(t, db, "BEGIN")
	execMinweightSQL(t, db, "INSERT INTO main.t(id, v) VALUES (1, 'main-rollback')")
	execMinweightSQL(t, db, "INSERT INTO aux.t(id, main_id, v) VALUES (10, 1, 'aux-rollback')")
	execMinweightSQL(t, db, "ROLLBACK")
	if got := minweightQueryInt(t, db, "SELECT count(*) FROM main.t"); got != 0 {
		t.Fatalf("main rows after rollback = %d, want 0", got)
	}
	if got := minweightQueryInt(t, db, "SELECT count(*) FROM aux.t"); got != 0 {
		t.Fatalf("aux rows after rollback = %d, want 0", got)
	}

	execMinweightSQL(t, db, "BEGIN")
	execMinweightSQL(t, db, "INSERT INTO main.t(id, v) VALUES (1, 'main-commit')")
	execMinweightSQL(t, db, "INSERT INTO aux.t(id, main_id, v) SELECT 101, id, v || '-copy' FROM main.t")
	execMinweightSQL(t, db, "COMMIT")

	var got string
	if err := db.QueryRow(`
		SELECT main.t.v || ':' || aux.t.v
		  FROM main.t JOIN aux.t ON aux.t.main_id = main.t.id
	`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "main-commit:main-commit-copy" {
		t.Fatalf("joined row = %q, want main-commit:main-commit-copy", got)
	}

	execMinweightSQL(t, db, "DETACH DATABASE aux")

	aux, err := sql.Open("sqlite", auxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, aux)
	if got := minweightQueryInt(t, aux, "SELECT count(*) FROM t WHERE main_id = 1 AND v = 'main-commit-copy'"); got != 1 {
		t.Fatalf("reopened aux rows = %d, want 1", got)
	}
}

func TestMinweightStorageEngineWithoutRowidCompositePrimaryKey(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, db)

	execMinweightSQL(t, db, `
		CREATE TABLE account_events(
			account TEXT NOT NULL,
			seq INTEGER NOT NULL,
			payload TEXT NOT NULL,
			PRIMARY KEY(account, seq)
		) WITHOUT ROWID
	`)
	execMinweightSQL(t, db, `
		INSERT INTO account_events(account, seq, payload) VALUES
			('acct-b', 2, 'b2'),
			('acct-a', 2, 'a2'),
			('acct-b', 1, 'b1'),
			('acct-a', 1, 'a1')
	`)

	rows, err := db.Query("SELECT account, seq, payload FROM account_events ORDER BY account, seq")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightRows(t, rows)
	var got []string
	for rows.Next() {
		var account string
		var seq int
		var payload string
		if err := rows.Scan(&account, &seq, &payload); err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%s:%d:%s", account, seq, payload))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{"acct-a:1:a1", "acct-a:2:a2", "acct-b:1:b1", "acct-b:2:b2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ordered rows = %v, want %v", got, want)
	}

	execMinweightSQL(t, db, "UPDATE account_events SET payload = 'a2-updated' WHERE account = 'acct-a' AND seq = 2")
	execMinweightSQL(t, db, "DELETE FROM account_events WHERE account = 'acct-b' AND seq = 1")

	var payload string
	if err := db.QueryRow("SELECT payload FROM account_events WHERE account = 'acct-a' AND seq = 2").Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if payload != "a2-updated" {
		t.Fatalf("updated payload = %q, want a2-updated", payload)
	}
	if got := minweightQueryInt(t, db, "SELECT count(*) FROM account_events WHERE account = 'acct-b'"); got != 1 {
		t.Fatalf("remaining acct-b rows = %d, want 1", got)
	}
	if rows, err := db.Query("SELECT rowid FROM account_events"); err == nil {
		_ = rows.Close()
		t.Fatal("rowid query succeeded for WITHOUT ROWID table")
	}
	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check = %q, want ok", integrity)
	}
}

func TestMinweightStorageEngineUncommittedWriteInvisibleToOtherConnection(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	path := filepath.Join(t.TempDir(), "isolation.db")
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, writer)
	writer.SetMaxOpenConns(1)

	reader, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, reader)
	reader.SetMaxOpenConns(1)

	execMinweightSQL(t, writer, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT UNIQUE)")
	execMinweightSQL(t, writer, "INSERT INTO t(id, v) VALUES (1, 'committed')")
	if got := minweightQueryInt(t, reader, "SELECT count(*) FROM t WHERE v = 'committed'"); got != 1 {
		t.Fatalf("initial reader rows = %d, want 1", got)
	}

	tx, err := writer.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("UPDATE t SET v = 'uncommitted-update' WHERE id = 1"); err != nil {
		t.Fatalf("update in writer transaction: %v", err)
	}
	if _, err := tx.Exec("INSERT INTO t(id, v) VALUES (2, 'uncommitted-insert')"); err != nil {
		t.Fatalf("insert in writer transaction: %v", err)
	}

	var v string
	if err := reader.QueryRow("SELECT v FROM t WHERE id = 1").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "committed" {
		t.Fatalf("reader saw id=1 value %q before commit, want committed", v)
	}
	if got := minweightQueryInt(t, reader, "SELECT count(*) FROM t WHERE id = 2"); got != 0 {
		t.Fatalf("reader saw uncommitted insert count = %d, want 0", got)
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := reader.QueryRow("SELECT v FROM t WHERE id = 1").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "uncommitted-update" {
		t.Fatalf("reader saw id=1 value %q after commit, want uncommitted-update", v)
	}
	if got := minweightQueryInt(t, reader, "SELECT count(*) FROM t WHERE id = 2"); got != 1 {
		t.Fatalf("reader saw committed insert count = %d, want 1", got)
	}

	tx, err = writer.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("UPDATE t SET v = 'rolled-back-update' WHERE id = 1"); err != nil {
		t.Fatalf("rollback update in writer transaction: %v", err)
	}
	if _, err := tx.Exec("INSERT INTO t(id, v) VALUES (3, 'rolled-back-insert')"); err != nil {
		t.Fatalf("rollback insert in writer transaction: %v", err)
	}
	if err := reader.QueryRow("SELECT v FROM t WHERE id = 1").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "uncommitted-update" {
		t.Fatalf("reader saw id=1 value %q before rollback, want uncommitted-update", v)
	}
	if got := minweightQueryInt(t, reader, "SELECT count(*) FROM t WHERE id = 3"); got != 0 {
		t.Fatalf("reader saw rolled-back insert count = %d, want 0", got)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := reader.QueryRow("SELECT v FROM t WHERE id = 1").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "uncommitted-update" {
		t.Fatalf("reader saw id=1 value %q after rollback, want uncommitted-update", v)
	}
	if got := minweightQueryInt(t, reader, "SELECT count(*) FROM t WHERE id = 3"); got != 0 {
		t.Fatalf("reader saw rolled-back insert after rollback count = %d, want 0", got)
	}
}

func TestMinweightStorageEngineRejectsConcurrentWriters(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	path := filepath.Join(t.TempDir(), "writers.db")
	first, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, first)
	first.SetMaxOpenConns(1)

	second, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, second)
	second.SetMaxOpenConns(1)

	execMinweightSQL(t, first, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, first, "INSERT INTO t(id, v) VALUES (1, 'one')")

	tx, err := first.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackMinweightTx(t, tx)
	if _, err := tx.Exec("UPDATE t SET v = 'writer-one' WHERE id = 1"); err != nil {
		t.Fatalf("first writer update: %v", err)
	}

	_, err = second.Exec("INSERT INTO t(id, v) VALUES (2, 'writer-two')")
	if err == nil {
		t.Fatal("second writer succeeded while first writer was active")
	}
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		t.Fatalf("second writer error = %T %v, want sqlite.Error", err, err)
	}
	if sqliteErr.Code() != sqlite3.SQLITE_BUSY {
		t.Fatalf("second writer code = %d, want SQLITE_BUSY", sqliteErr.Code())
	}
	if got := minweightQueryInt(t, second, "SELECT count(*) FROM t WHERE id = 2"); got != 0 {
		t.Fatalf("second writer row count = %d, want 0", got)
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	execMinweightSQL(t, second, "INSERT INTO t(id, v) VALUES (2, 'writer-two')")
	if got := minweightQueryInt(t, first, "SELECT count(*) FROM t WHERE id = 2"); got != 1 {
		t.Fatalf("second writer row count after first commit = %d, want 1", got)
	}
}

func TestMinweightStorageEngineBusyTimeoutWaitsForWriter(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	path := filepath.Join(t.TempDir(), "writer-timeout.db")
	first, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, first)
	first.SetMaxOpenConns(1)

	second, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, second)
	second.SetMaxOpenConns(1)

	execMinweightSQL(t, first, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, first, "INSERT INTO t(id, v) VALUES (1, 'one')")
	execMinweightSQL(t, second, "PRAGMA busy_timeout=2000")

	tx, err := first.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackMinweightTx(t, tx)
	if _, err := tx.Exec("UPDATE t SET v = 'writer-one' WHERE id = 1"); err != nil {
		t.Fatalf("first writer update: %v", err)
	}

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := second.Exec("INSERT INTO t(id, v) VALUES (2, 'writer-two')")
		done <- err
	}()
	<-started
	time.Sleep(100 * time.Millisecond)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("second writer completed before first writer committed")
		}
		t.Fatalf("second writer returned before first writer committed: %v", err)
	default:
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second writer after first commit: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second writer did not finish after first writer committed")
	}
	if got := minweightQueryInt(t, first, "SELECT count(*) FROM t WHERE id = 2"); got != 1 {
		t.Fatalf("second writer row count = %d, want 1", got)
	}
}

func TestMinweightStorageEngineOpenReadCursorBlocksWriterCommit(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	path := filepath.Join(t.TempDir(), "reader-cursor.db")
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, writer)
	writer.SetMaxOpenConns(1)

	reader, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, reader)
	reader.SetMaxOpenConns(1)

	execMinweightSQL(t, writer, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, writer, "INSERT INTO t(id, v) VALUES (1, 'one')")

	rows, err := reader.Query("SELECT id, v FROM t")
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		closeMinweightRows(t, rows)
		t.Fatal("reader cursor returned no rows")
	}
	var id int
	var v string
	if err := rows.Scan(&id, &v); err != nil {
		closeMinweightRows(t, rows)
		t.Fatal(err)
	}
	if id != 1 || v != "one" {
		closeMinweightRows(t, rows)
		t.Fatalf("reader first row = (%d, %q), want (1, one)", id, v)
	}

	tx, err := writer.Begin()
	if err != nil {
		closeMinweightRows(t, rows)
		t.Fatal(err)
	}
	if _, err := tx.Exec("INSERT INTO t(id, v) VALUES (2, 'two')"); err != nil {
		closeMinweightRows(t, rows)
		rollbackMinweightTx(t, tx)
		t.Fatalf("writer insert: %v", err)
	}
	err = tx.Commit()
	if err == nil {
		closeMinweightRows(t, rows)
		t.Fatal("writer commit succeeded while reader cursor was open")
	}
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		closeMinweightRows(t, rows)
		t.Fatalf("writer commit error = %T %v, want sqlite.Error", err, err)
	}
	if sqliteErr.Code() != sqlite3.SQLITE_BUSY {
		closeMinweightRows(t, rows)
		t.Fatalf("writer commit code = %d, want SQLITE_BUSY", sqliteErr.Code())
	}
	closeMinweightRows(t, rows)
	if got := minweightQueryInt(t, writer, "SELECT count(*) FROM t WHERE id = 2"); got != 0 {
		t.Fatalf("row committed after busy commit = %d, want 0", got)
	}

	execMinweightSQL(t, writer, "INSERT INTO t(id, v) VALUES (2, 'two')")
	if got := minweightQueryInt(t, reader, "SELECT count(*) FROM t WHERE id = 2"); got != 1 {
		t.Fatalf("row count after reader cursor close = %d, want 1", got)
	}
}

func TestMinweightStorageEngineTableCursorSeekSkipsRowidGapsAndOverlay(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, db)

	execMinweightSQL(t, db, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, db, "INSERT INTO t(id, v) VALUES (-1000000000000, 'low'), (1, 'one'), (1000000000000, 'high')")

	wantAsc := []string{"-1000000000000:low", "1:one", "1000000000000:high"}
	if got := minweightRowStrings(t, db); !reflect.DeepEqual(got, wantAsc) {
		t.Fatalf("initial rows = %v, want %v", got, wantAsc)
	}
	wantDesc := []string{"1000000000000:high", "1:one", "-1000000000000:low"}
	if got := minweightQueryStrings(t, db, "SELECT printf('%lld:%s', id, v) FROM t NOT INDEXED ORDER BY id DESC"); !reflect.DeepEqual(got, wantDesc) {
		t.Fatalf("initial desc rows = %v, want %v", got, wantDesc)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("DELETE FROM t WHERE id = 1"); err != nil {
		t.Fatalf("delete in tx: %v", err)
	}
	if _, err := tx.Exec("INSERT INTO t(id, v) VALUES (50, 'mid')"); err != nil {
		t.Fatalf("insert in tx: %v", err)
	}
	if _, err := tx.Exec("UPDATE t SET v = 'highest' WHERE id = 1000000000000"); err != nil {
		t.Fatalf("update in tx: %v", err)
	}
	wantTxAsc := []string{"-1000000000000:low", "50:mid", "1000000000000:highest"}
	if got := minweightTxQueryStrings(t, tx, "SELECT printf('%lld:%s', id, v) FROM t NOT INDEXED ORDER BY id"); !reflect.DeepEqual(got, wantTxAsc) {
		t.Fatalf("tx asc rows = %v, want %v", got, wantTxAsc)
	}
	wantTxDesc := []string{"1000000000000:highest", "50:mid", "-1000000000000:low"}
	if got := minweightTxQueryStrings(t, tx, "SELECT printf('%lld:%s', id, v) FROM t NOT INDEXED ORDER BY id DESC"); !reflect.DeepEqual(got, wantTxDesc) {
		t.Fatalf("tx desc rows = %v, want %v", got, wantTxDesc)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if got := minweightRowStrings(t, db); !reflect.DeepEqual(got, wantAsc) {
		t.Fatalf("rows after rollback = %v, want %v", got, wantAsc)
	}
}

func TestStorageEngineOpenConnectionsKeepBoundEngineAfterGlobalSwitch(t *testing.T) {
	sqlite.SetStorageEngine(sqlite.NewMinweightStorageEngine())
	t.Cleanup(func() { sqlite.SetStorageEngine(nil) })

	minweightDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, minweightDB)
	execMinweightSQL(t, minweightDB, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, minweightDB, "INSERT INTO t VALUES (1, 'minweight')")

	sqlite.SetStorageEngine(nil)
	if got := minweightQueryInt(t, minweightDB, "SELECT count(*) FROM t WHERE v = 'minweight'"); got != 1 {
		t.Fatalf("minweight rows after switching global engine to native = %d, want 1", got)
	}

	nativeDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, nativeDB)
	if _, err := nativeDB.Exec("CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := nativeDB.Exec("INSERT INTO t VALUES (1, 'native')"); err != nil {
		t.Fatal(err)
	}

	sqlite.SetStorageEngine(sqlite.NewMinweightStorageEngine())
	var nativeCount int64
	if err := nativeDB.QueryRow("SELECT count(*) FROM t WHERE v = 'native'").Scan(&nativeCount); err != nil {
		t.Fatal(err)
	}
	if nativeCount != 1 {
		t.Fatalf("native rows after switching global engine to minweight = %d, want 1", nativeCount)
	}
	if got := minweightQueryInt(t, minweightDB, "SELECT count(*) FROM t WHERE v = 'minweight'"); got != 1 {
		t.Fatalf("minweight rows after switching global engine again = %d, want 1", got)
	}
}

func TestMinweightStorageEngineIssue19Shape(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, `
		CREATE TABLE products(
			id VARCHAR(255),
			user_id VARCHAR(255),
			name VARCHAR(255),
			description VARCHAR(255),
			created_at BIGINT,
			credits_price BIGINT,
			enabled BOOLEAN,
			PRIMARY KEY(id)
		)
	`)
	inserts := []string{
		"INSERT INTO products(id, user_id, name, description, created_at, credits_price, enabled) VALUES ('9be4398c-d527-4efb-93a4-fc532cbaf804', '16935690-348b-41a6-bb20-f8bb16011015', 'dqdwqdwqdwqwqdwqd', 'qwdwqwqdwqdwqdwqd', '1577448686', '1', '0')",
		"INSERT INTO products(id, user_id, name, description, created_at, credits_price, enabled) VALUES ('759f10bd-9e1d-4ec7-b764-0868758d7b85', '16935690-348b-41a6-bb20-f8bb16011015', 'qdqwqwdwqdwqdwqwqd', 'wqdwqdwqdwqdwqdwq', '1577448692', '1', '1')",
		"INSERT INTO products(id, user_id, name, description, created_at, credits_price, enabled) VALUES ('512956e7-224d-4b2a-9153-b83a52c4aa38', '16935690-348b-41a6-bb20-f8bb16011015', 'qwdwqwdqwdqdwqwqd', 'wqdwdqwqdwqdwqdwqdwqdqw', '1577448699', '2', '1')",
		"INSERT INTO products(id, user_id, name, description, created_at, credits_price, enabled) VALUES ('02cd138f-6fa6-4909-9db7-a9d0eca4a7b7', '16935690-348b-41a6-bb20-f8bb16011015', 'qdwqdwqdwqwqdwdq', 'wqddwqwqdwqdwdqwdqwq', '1577448706', '3', '1')",
	}
	for i, query := range inserts {
		if _, err := db.Exec(query); err != nil {
			t.Fatalf("insert %d: %v", i+1, err)
		}
	}

	var count int
	if err := db.QueryRow("SELECT count(*) FROM products WHERE user_id = ?", "16935690-348b-41a6-bb20-f8bb16011015").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("count = %d, want 4", count)
	}
}

func TestMinweightStorageEngineOrderByPreservesColumns(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, "CREATE TABLE t3(x, y)")
	execMinweightSQL(t, db, "INSERT INTO t3 VALUES('a', 4), ('b', 5), ('c', 3), ('d', 8), ('e', 1)")

	rows, err := db.Query("SELECT x, y FROM t3 ORDER BY x")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	got := map[string]int64{}
	for rows.Next() {
		var x string
		var y int64
		if err := rows.Scan(&x, &y); err != nil {
			t.Fatal(err)
		}
		got[x] = y
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"a": 4, "b": 5, "c": 3, "d": 8, "e": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func TestMinweightStorageEngineBuiltinWindowSum(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, "CREATE TABLE t3(x, y)")
	execMinweightSQL(t, db, "INSERT INTO t3 VALUES('a', 4), ('b', 5), ('c', 3), ('d', 8), ('e', 1)")

	rows, err := db.Query("SELECT x, sum(y) OVER (ORDER BY x ROWS BETWEEN 1 PRECEDING AND 1 FOLLOWING) FROM t3 ORDER BY x")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	got := map[string]int64{}
	for rows.Next() {
		var x string
		var y int64
		if err := rows.Scan(&x, &y); err != nil {
			t.Fatal(err)
		}
		got[x] = y
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"a": 9, "b": 12, "c": 16, "d": 12, "e": 9}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func TestMinweightStorageEngineLogicalSerializeRoundTrip(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	type serializer interface {
		Serialize() ([]byte, error)
		Deserialize([]byte) error
	}

	src, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	execMinweightSQL(t, src, "CREATE TABLE t(v TEXT NOT NULL, b BLOB, n INTEGER)")
	execMinweightSQL(t, src, "CREATE INDEX idx_t_v ON t(v)")
	execMinweightSQL(t, src, "INSERT INTO t(rowid, v, b, n) VALUES (42, 'alpha', x'000102', 7), (99, 'beta', NULL, NULL)")

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
	defer dst.Close()

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

	var indexName string
	if err := dst.QueryRow("SELECT name FROM sqlite_schema WHERE type='index' AND name='idx_t_v'").Scan(&indexName); err != nil {
		t.Fatal(err)
	}

	rows, err := dst.Query("SELECT rowid, v, b, n FROM t INDEXED BY idx_t_v ORDER BY rowid")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	type row struct {
		rowid int64
		v     string
		b     []byte
		n     sql.NullInt64
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.rowid, &r.v, &r.b, &r.n); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("rows = %v, want 2 rows", got)
	}
	if got[0].rowid != 42 || got[0].v != "alpha" || !bytes.Equal(got[0].b, []byte{0, 1, 2}) || !got[0].n.Valid || got[0].n.Int64 != 7 {
		t.Fatalf("row 0 = %+v", got[0])
	}
	if got[1].rowid != 99 || got[1].v != "beta" || got[1].b != nil || got[1].n.Valid {
		t.Fatalf("row 1 = %+v", got[1])
	}
}

func TestMinweightStorageEngineVacuumTransfersRows(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT NOT NULL, n INTEGER)")
	execMinweightSQL(t, db, "CREATE UNIQUE INDEX t_v ON t(v)")
	execMinweightSQL(t, db, "INSERT INTO t(id, v, n) VALUES (1, 'a', 10), (2, 'b', 20), (5, 'e', 50)")

	execMinweightSQL(t, db, "VACUUM")

	rows, err := db.Query("SELECT id, v, n FROM t INDEXED BY t_v ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var id int
		var v string
		var n int
		if err := rows.Scan(&id, &v, &n); err != nil {
			t.Fatal(err)
		}
		got = append(got, strconv.Itoa(id)+":"+v+":"+strconv.Itoa(n))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{"1:a:10", "2:b:20", "5:e:50"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}

	var id int
	if err := db.QueryRow("SELECT id FROM t WHERE v = 'b'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != 2 {
		t.Fatalf("id for v=b = %d, want 2", id)
	}
}

func TestMinweightStorageEngineDropTableMovesAutoVacuumRoot(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, "PRAGMA auto_vacuum = FULL")
	execMinweightSQL(t, db, "CREATE TABLE a(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, db, "CREATE TABLE b(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, db, "CREATE INDEX b_v ON b(v)")
	execMinweightSQL(t, db, "INSERT INTO b(id, v) VALUES (1, 'one'), (2, 'two')")

	before := minweightRootPages(t, db, "a", "b", "b_v")
	wantBefore := map[string]int{"a": 2, "b": 3, "b_v": 4}
	if !reflect.DeepEqual(before, wantBefore) {
		t.Fatalf("rootpages before drop = %v, want %v", before, wantBefore)
	}

	execMinweightSQL(t, db, "DROP TABLE a")

	after := minweightRootPages(t, db, "b", "b_v")
	wantAfter := map[string]int{"b": 3, "b_v": 2}
	if !reflect.DeepEqual(after, wantAfter) {
		t.Fatalf("rootpages after drop = %v, want %v", after, wantAfter)
	}
	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check = %q, want ok", integrity)
	}

	var id int
	if err := db.QueryRow("SELECT id FROM b INDEXED BY b_v WHERE v = 'two'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != 2 {
		t.Fatalf("indexed lookup id = %d, want 2", id)
	}
}

func TestMinweightStorageEngineDropTableReusesFreedRoot(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, "CREATE TABLE a(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, db, "CREATE TABLE b(id INTEGER PRIMARY KEY, v TEXT)")
	before := minweightRootPages(t, db, "a", "b")
	wantBefore := map[string]int{"a": 2, "b": 3}
	if !reflect.DeepEqual(before, wantBefore) {
		t.Fatalf("rootpages before drop = %v, want %v", before, wantBefore)
	}

	execMinweightSQL(t, db, "DROP TABLE a")
	execMinweightSQL(t, db, "CREATE TABLE c(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, db, "INSERT INTO c(id, v) VALUES (7, 'seven')")

	after := minweightRootPages(t, db, "b", "c")
	wantAfter := map[string]int{"b": 3, "c": 2}
	if !reflect.DeepEqual(after, wantAfter) {
		t.Fatalf("rootpages after reuse = %v, want %v", after, wantAfter)
	}
	var v string
	if err := db.QueryRow("SELECT v FROM c WHERE id = 7").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "seven" {
		t.Fatalf("c.v = %q, want seven", v)
	}
}

func TestMinweightStorageEngineBtreePragmaState(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if got := minweightQueryInt(t, db, "PRAGMA page_size"); got != 4096 {
		t.Fatalf("default page_size = %d, want 4096", got)
	}
	execMinweightSQL(t, db, "PRAGMA page_size = 8192")
	if got := minweightQueryInt(t, db, "PRAGMA page_size"); got != 8192 {
		t.Fatalf("page_size = %d, want 8192", got)
	}

	if got := minweightQueryInt(t, db, "PRAGMA secure_delete"); got != 0 {
		t.Fatalf("default secure_delete = %d, want 0", got)
	}
	if got := minweightQueryInt(t, db, "PRAGMA secure_delete = FAST"); got != 2 {
		t.Fatalf("secure_delete FAST = %d, want 2", got)
	}
	if got := minweightQueryInt(t, db, "PRAGMA secure_delete = ON"); got != 1 {
		t.Fatalf("secure_delete ON = %d, want 1", got)
	}
	if got := minweightQueryInt(t, db, "PRAGMA secure_delete = OFF"); got != 0 {
		t.Fatalf("secure_delete OFF = %d, want 0", got)
	}

	if got := minweightQueryInt(t, db, "PRAGMA max_page_count"); got != 4294967294 {
		t.Fatalf("default max_page_count = %d, want 4294967294", got)
	}
	if got := minweightQueryInt(t, db, "PRAGMA max_page_count = 12345"); got != 12345 {
		t.Fatalf("max_page_count set result = %d, want 12345", got)
	}
	if got := minweightQueryInt(t, db, "PRAGMA max_page_count"); got != 12345 {
		t.Fatalf("max_page_count = %d, want 12345", got)
	}

	if got := minweightQueryInt(t, db, "PRAGMA cache_size"); got != -2000 {
		t.Fatalf("default cache_size = %d, want -2000", got)
	}
	execMinweightSQL(t, db, "PRAGMA cache_size = 10")
	if got := minweightQueryInt(t, db, "PRAGMA cache_size"); got != 10 {
		t.Fatalf("cache_size = %d, want 10", got)
	}
	if got := minweightQueryInt(t, db, "PRAGMA cache_spill"); got != 10 {
		t.Fatalf("default effective cache_spill = %d, want 10", got)
	}
	execMinweightSQL(t, db, "PRAGMA cache_spill = 20")
	if got := minweightQueryInt(t, db, "PRAGMA cache_spill"); got != 20 {
		t.Fatalf("cache_spill = %d, want 20", got)
	}
	execMinweightSQL(t, db, "PRAGMA cache_spill = 5")
	if got := minweightQueryInt(t, db, "PRAGMA cache_spill"); got != 10 {
		t.Fatalf("cache_spill below cache_size = %d, want 10", got)
	}
	execMinweightSQL(t, db, "PRAGMA cache_spill = OFF")
	if got := minweightQueryInt(t, db, "PRAGMA cache_spill"); got != 0 {
		t.Fatalf("cache_spill off = %d, want 0", got)
	}
	execMinweightSQL(t, db, "PRAGMA cache_spill = ON")
	if got := minweightQueryInt(t, db, "PRAGMA cache_spill"); got != 10 {
		t.Fatalf("cache_spill on = %d, want 10", got)
	}
	execMinweightSQL(t, db, "PRAGMA cache_size = 40")
	if got := minweightQueryInt(t, db, "PRAGMA cache_spill"); got != 40 {
		t.Fatalf("cache_spill after cache_size growth = %d, want 40", got)
	}

	if got := minweightQueryInt(t, db, "PRAGMA auto_vacuum"); got != 0 {
		t.Fatalf("default auto_vacuum = %d, want 0", got)
	}
	execMinweightSQL(t, db, "PRAGMA auto_vacuum = INCREMENTAL")
	if got := minweightQueryInt(t, db, "PRAGMA auto_vacuum"); got != 2 {
		t.Fatalf("auto_vacuum incremental = %d, want 2", got)
	}
	execMinweightSQL(t, db, "PRAGMA auto_vacuum = FULL")
	if got := minweightQueryInt(t, db, "PRAGMA auto_vacuum"); got != 1 {
		t.Fatalf("auto_vacuum full = %d, want 1", got)
	}

	execMinweightSQL(t, db, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, db, "INSERT INTO t(v) VALUES ('a'), ('b')")
	execMinweightSQL(t, db, "VACUUM")
	if got := minweightQueryInt(t, db, "PRAGMA page_size"); got != 8192 {
		t.Fatalf("page_size after VACUUM = %d, want 8192", got)
	}
	if got := minweightQueryInt(t, db, "PRAGMA auto_vacuum"); got != 1 {
		t.Fatalf("auto_vacuum after VACUUM = %d, want 1", got)
	}
}

func TestMinweightStorageEngineMmapSizePragma(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	dbPath := t.TempDir() + "/mmap.db"
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if got := minweightQueryInt(t, db, "PRAGMA mmap_size"); got != 0 {
		t.Fatalf("default mmap_size = %d, want 0", got)
	}
	if got := minweightQueryInt(t, db, "PRAGMA mmap_size = 65536"); got != 65536 {
		t.Fatalf("mmap_size set result = %d, want 65536", got)
	}
	if got := minweightQueryInt(t, db, "PRAGMA mmap_size"); got != 65536 {
		t.Fatalf("mmap_size = %d, want 65536", got)
	}
	if got := minweightQueryInt(t, db, "PRAGMA mmap_size = 0"); got != 0 {
		t.Fatalf("mmap_size reset result = %d, want 0", got)
	}
}

func TestMinweightStorageEngineDBPageVtabLogicalHeader(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, `
		CREATE TABLE test_dbpage (id INTEGER PRIMARY KEY, val TEXT);
		INSERT INTO test_dbpage (val) VALUES ('hello dbpage');
	`)

	if got := minweightQueryInt(t, db, "SELECT count(*) FROM sqlite_dbpage"); got != 1 {
		t.Fatalf("sqlite_dbpage row count = %d, want 1", got)
	}

	var pgno int
	var data []byte
	if err := db.QueryRow("SELECT pgno, data FROM sqlite_dbpage WHERE pgno = 1").Scan(&pgno, &data); err != nil {
		t.Fatal(err)
	}
	if pgno != 1 {
		t.Fatalf("pgno = %d, want 1", pgno)
	}
	if pageSize := int(minweightQueryInt(t, db, "PRAGMA page_size")); len(data) != pageSize {
		t.Fatalf("page len = %d, want %d", len(data), pageSize)
	}
	if header := []byte("SQLite format 3\x00"); !bytes.Equal(data[:len(header)], header) {
		t.Fatalf("page header = %q, want %q", data[:len(header)], header)
	}
	if got := minweightQueryInt(t, db, "SELECT count(*) FROM sqlite_dbpage WHERE pgno = 0"); got != 0 {
		t.Fatalf("pgno=0 row count = %d, want 0", got)
	}
	if got := minweightQueryInt(t, db, "SELECT count(*) FROM sqlite_dbpage WHERE pgno = 2"); got != 0 {
		t.Fatalf("pgno=2 row count = %d, want 0", got)
	}
}

func TestMinweightStorageEnginePathBackedStorePersistsAcrossEngine(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	path := filepath.Join(t.TempDir(), "persistent.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	execMinweightSQL(t, db, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, db, "CREATE INDEX t_v ON t(v)")
	execMinweightSQL(t, db, "PRAGMA user_version = 123")
	execMinweightSQL(t, db, "INSERT INTO t(id, v) VALUES (1, 'alpha'), (2, 'beta')")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("path-backed minweight database %s is not a directory", path)
	}

	sqlite.SetStorageEngine(sqlite.NewMinweightStorageEngine())
	reopened, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, reopened)

	if got := minweightQueryInt(t, reopened, "PRAGMA user_version"); got != 123 {
		t.Fatalf("reopened user_version = %d, want 123", got)
	}
	if got := minweightQueryInt(t, reopened, "SELECT count(*) FROM t WHERE v >= 'alpha'"); got != 2 {
		t.Fatalf("reopened row count = %d, want 2", got)
	}
	var v string
	if err := reopened.QueryRow("SELECT v FROM t WHERE v = 'beta'").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "beta" {
		t.Fatalf("reopened indexed lookup = %q, want beta", v)
	}
}

func TestMinweightStorageEnginePathBackedRollbackDoesNotPersistOverlay(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	path := filepath.Join(t.TempDir(), "rollback-overlay.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	execMinweightSQL(t, db, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, db, "INSERT INTO t(id, v) VALUES (1, 'base')")

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("UPDATE t SET v = 'rolled-back' WHERE id = 1"); err != nil {
		t.Fatalf("update in tx: %v", err)
	}
	if _, err := tx.Exec("INSERT INTO t(id, v) VALUES (2, 'transient')"); err != nil {
		t.Fatalf("insert in tx: %v", err)
	}
	if _, err := tx.Exec("CREATE INDEX t_v_transient ON t(v)"); err != nil {
		t.Fatalf("create index in tx: %v", err)
	}
	if _, err := tx.Exec("PRAGMA user_version = 77"); err != nil {
		t.Fatalf("user_version in tx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var v string
	if err := db.QueryRow("SELECT v FROM t WHERE id = 1").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "base" {
		t.Fatalf("value after rollback = %q, want base", v)
	}
	if got := minweightQueryInt(t, db, "SELECT count(*) FROM t WHERE id = 2"); got != 0 {
		t.Fatalf("transient row count after rollback = %d, want 0", got)
	}
	if got := minweightQueryInt(t, db, "SELECT count(*) FROM sqlite_schema WHERE name = 't_v_transient'"); got != 0 {
		t.Fatalf("transient index count after rollback = %d, want 0", got)
	}
	if got := minweightQueryInt(t, db, "PRAGMA user_version"); got != 0 {
		t.Fatalf("user_version after rollback = %d, want 0", got)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	sqlite.SetStorageEngine(sqlite.NewMinweightStorageEngine())
	reopened, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, reopened)

	if err := reopened.QueryRow("SELECT v FROM t WHERE id = 1").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "base" {
		t.Fatalf("reopened value after rollback = %q, want base", v)
	}
	if got := minweightQueryInt(t, reopened, "SELECT count(*) FROM t WHERE id = 2"); got != 0 {
		t.Fatalf("reopened transient row count = %d, want 0", got)
	}
	if got := minweightQueryInt(t, reopened, "SELECT count(*) FROM sqlite_schema WHERE name = 't_v_transient'"); got != 0 {
		t.Fatalf("reopened transient index count = %d, want 0", got)
	}
	if got := minweightQueryInt(t, reopened, "PRAGMA user_version"); got != 0 {
		t.Fatalf("reopened user_version = %d, want 0", got)
	}
}

func TestMinweightStorageEngineReadOnlyPathOpenFailsFast(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	path := filepath.Join(t.TempDir(), "readonly.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	execMinweightSQL(t, db, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, db, "INSERT INTO t(v) VALUES ('a')")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	ro, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", path))
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, ro)
	if err := ro.Ping(); err == nil {
		t.Fatal("read-only minweight path open succeeded, want fail-fast")
	}
}

func TestMinweightStorageEngineLogicalSerializePreservesBtreePragmaState(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	type serializer interface {
		Serialize() ([]byte, error)
		Deserialize([]byte) error
	}

	src, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	execMinweightSQL(t, src, "PRAGMA page_size = 8192")
	if got := minweightQueryInt(t, src, "PRAGMA secure_delete = FAST"); got != 2 {
		t.Fatalf("secure_delete FAST = %d, want 2", got)
	}
	if got := minweightQueryInt(t, src, "PRAGMA max_page_count = 12345"); got != 12345 {
		t.Fatalf("max_page_count set result = %d, want 12345", got)
	}
	execMinweightSQL(t, src, "PRAGMA auto_vacuum = INCREMENTAL")
	execMinweightSQL(t, src, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, src, "INSERT INTO t(v) VALUES ('a')")

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
	defer dst.Close()

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

	if got := minweightQueryInt(t, dst, "PRAGMA page_size"); got != 8192 {
		t.Fatalf("deserialized page_size = %d, want 8192", got)
	}
	if got := minweightQueryInt(t, dst, "PRAGMA secure_delete"); got != 2 {
		t.Fatalf("deserialized secure_delete = %d, want 2", got)
	}
	if got := minweightQueryInt(t, dst, "PRAGMA max_page_count"); got != 12345 {
		t.Fatalf("deserialized max_page_count = %d, want 12345", got)
	}
	if got := minweightQueryInt(t, dst, "PRAGMA auto_vacuum"); got != 2 {
		t.Fatalf("deserialized auto_vacuum = %d, want 2", got)
	}
	if got := minweightQueryInt(t, dst, "SELECT count(*) FROM t"); got != 1 {
		t.Fatalf("deserialized row count = %d, want 1", got)
	}
}

func TestMinweightStorageEngineLogicalSerializePreservesReusedTableRoot(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	type serializer interface {
		Serialize() ([]byte, error)
		Deserialize([]byte) error
	}

	src, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	execMinweightSQL(t, src, "CREATE TABLE a(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, src, "CREATE TABLE b(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, src, "DROP TABLE a")
	execMinweightSQL(t, src, "CREATE TABLE c(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, src, "INSERT INTO b(id, v) VALUES (1, 'one')")
	execMinweightSQL(t, src, "INSERT INTO c(id, v) VALUES (2, 'two')")

	wantRootPages := map[string]int{"b": 3, "c": 2}
	if got := minweightRootPages(t, src, "b", "c"); !reflect.DeepEqual(got, wantRootPages) {
		t.Fatalf("source rootpages = %v, want %v", got, wantRootPages)
	}

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
	defer dst.Close()

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

	if got := minweightRootPages(t, dst, "b", "c"); !reflect.DeepEqual(got, wantRootPages) {
		t.Fatalf("deserialized rootpages = %v, want %v", got, wantRootPages)
	}
	if got := minweightQueryInt(t, dst, "SELECT count(*) FROM b WHERE v = 'one'"); got != 1 {
		t.Fatalf("deserialized b row count = %d, want 1", got)
	}
	if got := minweightQueryInt(t, dst, "SELECT count(*) FROM c WHERE v = 'two'"); got != 1 {
		t.Fatalf("deserialized c row count = %d, want 1", got)
	}
}

func TestMinweightStorageEngineLogicalSerializePreservesIndexRootBelowTable(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	type serializer interface {
		Serialize() ([]byte, error)
		Deserialize([]byte) error
	}

	src, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	execMinweightSQL(t, src, "CREATE TABLE a(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, src, "CREATE TABLE b(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, src, "DROP TABLE a")
	execMinweightSQL(t, src, "CREATE INDEX b_v ON b(v)")
	execMinweightSQL(t, src, "INSERT INTO b(id, v) VALUES (1, 'one'), (2, 'two')")

	wantRootPages := map[string]int{"b": 3, "b_v": 2}
	if got := minweightRootPages(t, src, "b", "b_v"); !reflect.DeepEqual(got, wantRootPages) {
		t.Fatalf("source rootpages = %v, want %v", got, wantRootPages)
	}

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
	defer dst.Close()

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

	if got := minweightRootPages(t, dst, "b", "b_v"); !reflect.DeepEqual(got, wantRootPages) {
		t.Fatalf("deserialized rootpages = %v, want %v", got, wantRootPages)
	}
	if got := minweightQueryInt(t, dst, "SELECT id FROM b INDEXED BY b_v WHERE v = 'two'"); got != 2 {
		t.Fatalf("indexed lookup id = %d, want 2", got)
	}
}

func TestMinweightStorageEngineLogicalBackupRestorePreservesIndexRootBelowTable(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	type backuper interface {
		NewBackup(string) (*sqlite.Backup, error)
		NewRestore(string) (*sqlite.Backup, error)
	}

	src, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	execMinweightSQL(t, src, "CREATE TABLE a(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, src, "CREATE TABLE b(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, src, "DROP TABLE a")
	execMinweightSQL(t, src, "CREATE INDEX b_v ON b(v)")
	execMinweightSQL(t, src, "INSERT INTO b(id, v) VALUES (1, 'one'), (2, 'two')")

	wantRootPages := map[string]int{"b": 3, "b_v": 2}
	if got := minweightRootPages(t, src, "b", "b_v"); !reflect.DeepEqual(got, wantRootPages) {
		t.Fatalf("source rootpages = %v, want %v", got, wantRootPages)
	}

	dstPath := filepath.Join(t.TempDir(), "backup.db")
	srcConn, err := src.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := srcConn.Raw(func(dc any) error {
		bck, err := dc.(backuper).NewBackup(dstPath)
		if err != nil {
			return err
		}
		for more := true; more; {
			more, err = bck.Step(-1)
			if err != nil {
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

	dst, err := sql.Open("sqlite", dstPath)
	if err != nil {
		t.Fatal(err)
	}

	if got := minweightRootPages(t, dst, "b", "b_v"); !reflect.DeepEqual(got, wantRootPages) {
		t.Fatalf("backup rootpages = %v, want %v", got, wantRootPages)
	}
	if got := minweightQueryInt(t, dst, "SELECT id FROM b INDEXED BY b_v WHERE v = 'two'"); got != 2 {
		t.Fatalf("backup indexed lookup id = %d, want 2", got)
	}
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}

	restored, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()

	restoredConn, err := restored.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := restoredConn.Raw(func(dc any) error {
		bck, err := dc.(backuper).NewRestore(dstPath)
		if err != nil {
			return err
		}
		for more := true; more; {
			more, err = bck.Step(-1)
			if err != nil {
				return err
			}
		}
		return bck.Finish()
	}); err != nil {
		t.Fatal(err)
	}
	if err := restoredConn.Close(); err != nil {
		t.Fatal(err)
	}

	if got := minweightRootPages(t, restored, "b", "b_v"); !reflect.DeepEqual(got, wantRootPages) {
		t.Fatalf("restored rootpages = %v, want %v", got, wantRootPages)
	}
	if got := minweightQueryInt(t, restored, "SELECT id FROM b INDEXED BY b_v WHERE v = 'two'"); got != 2 {
		t.Fatalf("restored indexed lookup id = %d, want 2", got)
	}
}

func TestMinweightStorageEngineLogicalRoundTripPreservesFreelistReuseOrder(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	type serializer interface {
		Serialize() ([]byte, error)
		Deserialize([]byte) error
	}
	type backuper interface {
		NewBackup(string) (*sqlite.Backup, error)
		NewRestore(string) (*sqlite.Backup, error)
	}

	src, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	execMinweightSQL(t, src, "CREATE TABLE a(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, src, "CREATE TABLE b(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, src, "CREATE TABLE c(id INTEGER PRIMARY KEY, v TEXT)")
	execMinweightSQL(t, src, "DROP TABLE a")
	execMinweightSQL(t, src, "DROP TABLE b")
	execMinweightSQL(t, src, "INSERT INTO c(id, v) VALUES (1, 'keep')")

	if got := minweightRootPages(t, src, "c"); !reflect.DeepEqual(got, map[string]int{"c": 4}) {
		t.Fatalf("source rootpages = %v, want c root 4", got)
	}

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

	tmpDir := t.TempDir()
	backupPath := filepath.Join(tmpDir, "freelist.db")
	restorePath := filepath.Join(tmpDir, "freelist-restore.db")
	if err := srcConn.Raw(func(dc any) error {
		bck, err := dc.(backuper).NewBackup(backupPath)
		if err != nil {
			return err
		}
		for more := true; more; {
			more, err = bck.Step(-1)
			if err != nil {
				return err
			}
		}
		return bck.Finish()
	}); err != nil {
		t.Fatal(err)
	}
	if err := srcConn.Raw(func(dc any) error {
		bck, err := dc.(backuper).NewBackup(restorePath)
		if err != nil {
			return err
		}
		for more := true; more; {
			more, err = bck.Step(-1)
			if err != nil {
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

	execMinweightSQL(t, src, "CREATE TABLE d(id INTEGER PRIMARY KEY, v TEXT)")
	if got := minweightRootPages(t, src, "d"); !reflect.DeepEqual(got, map[string]int{"d": 3}) {
		t.Fatalf("source next reused rootpages = %v, want d root 3", got)
	}

	dstSerialize, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer dstSerialize.Close()
	dstSerializeConn, err := dstSerialize.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := dstSerializeConn.Raw(func(dc any) error {
		return dc.(serializer).Deserialize(snapshot)
	}); err != nil {
		t.Fatal(err)
	}
	if err := dstSerializeConn.Close(); err != nil {
		t.Fatal(err)
	}
	execMinweightSQL(t, dstSerialize, "CREATE TABLE d(id INTEGER PRIMARY KEY, v TEXT)")
	if got := minweightRootPages(t, dstSerialize, "c", "d"); !reflect.DeepEqual(got, map[string]int{"c": 4, "d": 3}) {
		t.Fatalf("deserialized next reused rootpages = %v, want c root 4 and d root 3", got)
	}

	dstBackup, err := sql.Open("sqlite", backupPath)
	if err != nil {
		t.Fatal(err)
	}
	execMinweightSQL(t, dstBackup, "CREATE TABLE d(id INTEGER PRIMARY KEY, v TEXT)")
	if got := minweightRootPages(t, dstBackup, "c", "d"); !reflect.DeepEqual(got, map[string]int{"c": 4, "d": 3}) {
		t.Fatalf("backup next reused rootpages = %v, want c root 4 and d root 3", got)
	}
	if err := dstBackup.Close(); err != nil {
		t.Fatal(err)
	}

	dstRestore, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer dstRestore.Close()
	dstRestoreConn, err := dstRestore.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := dstRestoreConn.Raw(func(dc any) error {
		bck, err := dc.(backuper).NewRestore(restorePath)
		if err != nil {
			return err
		}
		for more := true; more; {
			more, err = bck.Step(-1)
			if err != nil {
				return err
			}
		}
		return bck.Finish()
	}); err != nil {
		t.Fatal(err)
	}
	if err := dstRestoreConn.Close(); err != nil {
		t.Fatal(err)
	}
	execMinweightSQL(t, dstRestore, "CREATE TABLE d(id INTEGER PRIMARY KEY, v TEXT)")
	if got := minweightRootPages(t, dstRestore, "c", "d"); !reflect.DeepEqual(got, map[string]int{"c": 4, "d": 3}) {
		t.Fatalf("restored next reused rootpages = %v, want c root 4 and d root 3", got)
	}
}

func TestMinweightStorageEngineTransactionRollbackRestoresRows(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT UNIQUE)")
	execMinweightSQL(t, db, "INSERT INTO t(id, v) VALUES (1, 'a'), (2, 'b')")

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("INSERT INTO t(id, v) VALUES (3, 'c')"); err != nil {
		t.Fatalf("insert in tx: %v", err)
	}
	if _, err := tx.Exec("UPDATE t SET v = 'aa' WHERE id = 1"); err != nil {
		t.Fatalf("update in tx: %v", err)
	}
	if _, err := tx.Exec("DELETE FROM t WHERE id = 2"); err != nil {
		t.Fatalf("delete in tx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	want := []string{"1:a", "2:b"}
	if got := minweightRowStrings(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}

	var id int
	if err := db.QueryRow("SELECT id FROM t WHERE v = 'a'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Fatalf("id for v=a = %d, want 1", id)
	}
}

func TestMinweightStorageEngineSavepointRollbackRestoresRows(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT UNIQUE)")
	execMinweightSQL(t, db, "INSERT INTO t(id, v) VALUES (1, 'a'), (2, 'b')")

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("INSERT INTO t(id, v) VALUES (3, 'keep')"); err != nil {
		t.Fatalf("insert before savepoint: %v", err)
	}
	if _, err := tx.Exec("SAVEPOINT sp"); err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	if _, err := tx.Exec("UPDATE t SET v = 'changed' WHERE id = 1"); err != nil {
		t.Fatalf("update after savepoint: %v", err)
	}
	if _, err := tx.Exec("DELETE FROM t WHERE id = 2"); err != nil {
		t.Fatalf("delete after savepoint: %v", err)
	}
	if _, err := tx.Exec("INSERT INTO t(id, v) VALUES (4, 'drop')"); err != nil {
		t.Fatalf("insert after savepoint: %v", err)
	}
	if _, err := tx.Exec("ROLLBACK TO sp"); err != nil {
		t.Fatalf("rollback to savepoint: %v", err)
	}
	if _, err := tx.Exec("RELEASE sp"); err != nil {
		t.Fatalf("release savepoint: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	want := []string{"1:a", "2:b", "3:keep"}
	if got := minweightRowStrings(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

func TestMinweightStorageEngineStatementRollbackRestoresRows(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	execMinweightSQL(t, db, "CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT UNIQUE)")
	execMinweightSQL(t, db, "INSERT INTO t(v) VALUES ('a')")

	if _, err := db.Exec("INSERT INTO t(v) VALUES ('b'), ('a'), ('c')"); err == nil {
		t.Fatal("duplicate insert succeeded, want constraint error")
	}

	want := []string{"1:a"}
	if got := minweightRowStrings(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}

	if _, err := db.Exec("INSERT INTO t(v) VALUES ('b')"); err != nil {
		t.Fatalf("insert after statement rollback: %v", err)
	}
}

func installMinweightStorageEngineForTest(t *testing.T) {
	t.Helper()
	sqlite.SetStorageEngine(sqlite.NewMinweightStorageEngine())
	t.Cleanup(func() {
		if os.Getenv("SQLITE_TEST_STORAGE_ENGINE") == "minweight" {
			sqlite.SetStorageEngine(sqlite.NewMinweightStorageEngine())
			return
		}
		sqlite.SetStorageEngine(nil)
	})
}

func execMinweightSQL(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.Exec(query); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func closeMinweightDB(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func closeMinweightRows(t *testing.T, rows *sql.Rows) {
	t.Helper()
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
}

func rollbackMinweightTx(t *testing.T, tx *sql.Tx) {
	t.Helper()
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		t.Fatal(err)
	}
}

func minweightQueryInt(t *testing.T, db *sql.DB, query string) int64 {
	t.Helper()
	var got int64
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return got
}

func minweightRootPages(t *testing.T, db *sql.DB, names ...string) map[string]int {
	t.Helper()
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}
	rows, err := db.Query("SELECT name, rootpage FROM sqlite_schema WHERE rootpage > 0")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	got := map[string]int{}
	for rows.Next() {
		var name string
		var rootpage int
		if err := rows.Scan(&name, &rootpage); err != nil {
			t.Fatal(err)
		}
		if wanted[name] {
			got[name] = rootpage
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(wanted) {
		t.Fatalf("rootpages = %v, missing one of %v", got, names)
	}
	return got
}

func minweightRowStrings(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query("SELECT id, v FROM t ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var id int
		var v string
		if err := rows.Scan(&id, &v); err != nil {
			t.Fatal(err)
		}
		got = append(got, strconv.Itoa(id)+":"+v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func minweightQueryStrings(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer closeMinweightRows(t, rows)
	return minweightScanStrings(t, rows)
}

func minweightTxQueryStrings(t *testing.T, tx *sql.Tx, query string) []string {
	t.Helper()
	rows, err := tx.Query(query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer closeMinweightRows(t, rows)
	return minweightScanStrings(t, rows)
}

func minweightScanStrings(t *testing.T, rows *sql.Rows) []string {
	t.Helper()
	var got []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		got = append(got, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func minweightSQLLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
