// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func TestMinweightStorageEngineUpsertReplaceMaintainsIndexes(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, db)

	execMinweightSQL(t, db, `
		CREATE TABLE users(
			id INTEGER PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			updates INTEGER NOT NULL DEFAULT 0
		)
	`)
	execMinweightSQL(t, db, "CREATE INDEX users_name ON users(name)")
	execMinweightSQL(t, db, "INSERT INTO users(id, email, name) VALUES (1, 'a@example.test', 'alice'), (2, 'b@example.test', 'bob')")

	rows, err := db.Query(`
		INSERT INTO users(email, name) VALUES ('a@example.test', 'alicia')
		ON CONFLICT(email) DO UPDATE SET name = excluded.name, updates = users.updates + 1
		RETURNING id, name, updates
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		closeMinweightRows(t, rows)
		t.Fatal("upsert returning produced no row")
	}
	var id int
	var name string
	var updates int
	if err := rows.Scan(&id, &name, &updates); err != nil {
		closeMinweightRows(t, rows)
		t.Fatal(err)
	}
	if id != 1 || name != "alicia" || updates != 1 {
		closeMinweightRows(t, rows)
		t.Fatalf("upsert returning = (%d, %q, %d), want (1, alicia, 1)", id, name, updates)
	}
	if rows.Next() {
		closeMinweightRows(t, rows)
		t.Fatal("upsert returning produced extra row")
	}
	if err := rows.Err(); err != nil {
		closeMinweightRows(t, rows)
		t.Fatal(err)
	}
	closeMinweightRows(t, rows)

	if got := minweightQueryInt(t, db, "SELECT count(*) FROM users INDEXED BY users_name WHERE name = 'alice'"); got != 0 {
		t.Fatalf("old indexed name count = %d, want 0", got)
	}
	if got := minweightQueryInt(t, db, "SELECT id FROM users INDEXED BY users_name WHERE name = 'alicia'"); got != 1 {
		t.Fatalf("new indexed name id = %d, want 1", got)
	}

	execMinweightSQL(t, db, "INSERT OR REPLACE INTO users(id, email, name, updates) VALUES (2, 'c@example.test', 'carol', 7)")
	want := []string{"1:a@example.test:alicia:1", "2:c@example.test:carol:7"}
	if got := minweightQueryStrings(t, db, "SELECT printf('%d:%s:%s:%d', id, email, name, updates) FROM users ORDER BY id"); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows after replace = %v, want %v", got, want)
	}
	if got := minweightQueryInt(t, db, "SELECT count(*) FROM users WHERE email = 'b@example.test'"); got != 0 {
		t.Fatalf("old replaced email count = %d, want 0", got)
	}
	if got := minweightQueryInt(t, db, "SELECT id FROM users WHERE email = 'c@example.test'"); got != 2 {
		t.Fatalf("new replaced email id = %d, want 2", got)
	}
}

func TestMinweightStorageEngineForeignKeyCascadeAndTriggers(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, db)

	execMinweightSQL(t, db, "PRAGMA foreign_keys = ON")
	execMinweightSQL(t, db, `
		CREATE TABLE parent(id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE);
		CREATE TABLE child(
			id INTEGER PRIMARY KEY,
			parent_id INTEGER NOT NULL REFERENCES parent(id) ON UPDATE CASCADE ON DELETE CASCADE,
			value TEXT NOT NULL
		);
		CREATE TABLE audit(seq INTEGER PRIMARY KEY AUTOINCREMENT, event TEXT NOT NULL);
		CREATE TRIGGER parent_ai AFTER INSERT ON parent BEGIN
			INSERT INTO audit(event) VALUES ('parent-insert:' || new.id);
		END;
		CREATE TRIGGER child_au AFTER UPDATE ON child BEGIN
			INSERT INTO audit(event) VALUES ('child-update:' || old.parent_id || '->' || new.parent_id);
		END;
		CREATE TRIGGER child_ad AFTER DELETE ON child BEGIN
			INSERT INTO audit(event) VALUES ('child-delete:' || old.id);
		END;
	`)
	execMinweightSQL(t, db, "INSERT INTO parent(id, name) VALUES (10, 'ten')")
	execMinweightSQL(t, db, "INSERT INTO child(id, parent_id, value) VALUES (1, 10, 'a'), (2, 10, 'b')")
	execMinweightSQL(t, db, "UPDATE parent SET id = 20 WHERE id = 10")

	if got := minweightQueryStrings(t, db, "SELECT printf('%d:%d:%s', id, parent_id, value) FROM child ORDER BY id"); !reflect.DeepEqual(got, []string{"1:20:a", "2:20:b"}) {
		t.Fatalf("children after cascade update = %v", got)
	}

	execMinweightSQL(t, db, "DELETE FROM parent WHERE id = 20")
	if got := minweightQueryInt(t, db, "SELECT count(*) FROM child"); got != 0 {
		t.Fatalf("children after cascade delete = %d, want 0", got)
	}
	wantAudit := []string{
		"parent-insert:10",
		"child-update:10->20",
		"child-update:10->20",
		"child-delete:1",
		"child-delete:2",
	}
	if got := minweightQueryStrings(t, db, "SELECT event FROM audit ORDER BY seq"); !reflect.DeepEqual(got, wantAudit) {
		t.Fatalf("audit rows = %v, want %v", got, wantAudit)
	}
}

func TestMinweightStorageEnginePartialExpressionIndexWrites(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, db)

	execMinweightSQL(t, db, `
		CREATE TABLE tasks(
			id INTEGER PRIMARY KEY,
			status TEXT NOT NULL,
			owner TEXT NOT NULL,
			payload TEXT NOT NULL
		)
	`)
	execMinweightSQL(t, db, "CREATE UNIQUE INDEX tasks_open_owner ON tasks(lower(owner)) WHERE status = 'open'")
	execMinweightSQL(t, db, "INSERT INTO tasks(id, status, owner, payload) VALUES (1, 'open', 'Alice', 'first'), (2, 'closed', 'alice', 'archived')")

	if _, err := db.Exec("INSERT INTO tasks(id, status, owner, payload) VALUES (3, 'open', 'ALICE', 'duplicate')"); err == nil {
		t.Fatal("duplicate expression-index insert succeeded")
	}
	if got := minweightQueryInt(t, db, "SELECT count(*) FROM tasks WHERE id = 3"); got != 0 {
		t.Fatalf("duplicate insert row count = %d, want 0", got)
	}

	if _, err := db.Exec("UPDATE tasks SET status = 'open' WHERE id = 2"); err == nil {
		t.Fatal("duplicate expression-index update succeeded")
	}
	if got := minweightQueryStrings(t, db, "SELECT printf('%d:%s:%s', id, status, owner) FROM tasks ORDER BY id"); !reflect.DeepEqual(got, []string{"1:open:Alice", "2:closed:alice"}) {
		t.Fatalf("rows after failed update = %v", got)
	}

	execMinweightSQL(t, db, "UPDATE tasks SET status = 'closed' WHERE id = 1")
	execMinweightSQL(t, db, "UPDATE tasks SET status = 'open' WHERE id = 2")
	if got := minweightQueryInt(t, db, "SELECT id FROM tasks INDEXED BY tasks_open_owner WHERE status = 'open' AND lower(owner) = 'alice'"); got != 2 {
		t.Fatalf("expression-index lookup id = %d, want 2", got)
	}
}

func TestMinweightStorageEngineRowidUpdateMaintainsIndexes(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, db)

	execMinweightSQL(t, db, `
		CREATE TABLE events(
			id INTEGER PRIMARY KEY,
			code TEXT NOT NULL UNIQUE,
			payload TEXT NOT NULL
		);
		CREATE INDEX events_payload ON events(payload);
	`)
	execMinweightSQL(t, db, "INSERT INTO events(id, code, payload) VALUES (1, 'c1', 'p1'), (2, 'c2', 'p2')")
	execMinweightSQL(t, db, "UPDATE events SET id = 10, code = 'c10', payload = 'p10' WHERE id = 1")

	if got := minweightQueryStrings(t, db, "SELECT printf('%d:%s:%s', id, code, payload) FROM events ORDER BY id"); !reflect.DeepEqual(got, []string{"2:c2:p2", "10:c10:p10"}) {
		t.Fatalf("rows after rowid update = %v", got)
	}
	if got := minweightQueryInt(t, db, "SELECT count(*) FROM events WHERE id = 1 OR code = 'c1' OR payload = 'p1'"); got != 0 {
		t.Fatalf("old rowid/index entries count = %d, want 0", got)
	}
	if got := minweightQueryInt(t, db, "SELECT id FROM events INDEXED BY events_payload WHERE payload = 'p10'"); got != 10 {
		t.Fatalf("secondary-index lookup id = %d, want 10", got)
	}

	if _, err := db.Exec("UPDATE events SET id = 20, code = 'c2', payload = 'bad' WHERE id = 10"); err == nil {
		t.Fatal("conflicting rowid update succeeded")
	}
	if got := minweightQueryStrings(t, db, "SELECT printf('%d:%s:%s', id, code, payload) FROM events ORDER BY id"); !reflect.DeepEqual(got, []string{"2:c2:p2", "10:c10:p10"}) {
		t.Fatalf("rows after failed rowid update = %v", got)
	}
}

func TestMinweightStorageEngineWithoutRowidPrimaryKeyUpdateMaintainsIndexes(t *testing.T) {
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
			tag TEXT NOT NULL UNIQUE,
			payload TEXT NOT NULL,
			PRIMARY KEY(account, seq)
		) WITHOUT ROWID;
		CREATE INDEX account_events_payload ON account_events(payload);
	`)
	execMinweightSQL(t, db, `
		INSERT INTO account_events(account, seq, tag, payload) VALUES
			('acct-a', 1, 'tag-a1', 'p-a1'),
			('acct-b', 1, 'tag-b1', 'p-b1')
	`)
	execMinweightSQL(t, db, "UPDATE account_events SET account = 'acct-c', seq = 3, tag = 'tag-c3', payload = 'p-c3' WHERE account = 'acct-a' AND seq = 1")

	if got := minweightQueryStrings(t, db, "SELECT account || ':' || seq || ':' || tag || ':' || payload FROM account_events ORDER BY account, seq"); !reflect.DeepEqual(got, []string{"acct-b:1:tag-b1:p-b1", "acct-c:3:tag-c3:p-c3"}) {
		t.Fatalf("rows after WITHOUT ROWID primary-key update = %v", got)
	}
	if got := minweightQueryInt(t, db, "SELECT count(*) FROM account_events WHERE account = 'acct-a' OR tag = 'tag-a1' OR payload = 'p-a1'"); got != 0 {
		t.Fatalf("old WITHOUT ROWID key/index entries count = %d, want 0", got)
	}
	if got := minweightQueryStrings(t, db, "SELECT account || ':' || seq FROM account_events INDEXED BY account_events_payload WHERE payload = 'p-c3'"); !reflect.DeepEqual(got, []string{"acct-c:3"}) {
		t.Fatalf("secondary-index lookup after primary-key update = %v", got)
	}

	if _, err := db.Exec("UPDATE account_events SET account = 'acct-b', seq = 1, tag = 'tag-bad', payload = 'bad' WHERE account = 'acct-c' AND seq = 3"); err == nil {
		t.Fatal("conflicting WITHOUT ROWID primary-key update succeeded")
	}
	if got := minweightQueryStrings(t, db, "SELECT account || ':' || seq || ':' || tag || ':' || payload FROM account_events ORDER BY account, seq"); !reflect.DeepEqual(got, []string{"acct-b:1:tag-b1:p-b1", "acct-c:3:tag-c3:p-c3"}) {
		t.Fatalf("rows after failed WITHOUT ROWID primary-key update = %v", got)
	}
}

func TestMinweightStorageEngineFailedStatementRollsBackTriggerWrites(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, db)

	execMinweightSQL(t, db, `
		CREATE TABLE items(id INTEGER PRIMARY KEY, sku TEXT NOT NULL UNIQUE);
		CREATE TABLE audit(seq INTEGER PRIMARY KEY AUTOINCREMENT, event TEXT NOT NULL);
		CREATE TRIGGER items_ai AFTER INSERT ON items BEGIN
			INSERT INTO audit(event) VALUES ('insert:' || new.id || ':' || new.sku);
		END;
	`)
	execMinweightSQL(t, db, "INSERT INTO items(id, sku) VALUES (1, 'base')")

	if _, err := db.Exec("INSERT INTO items(id, sku) VALUES (2, 'ok-before-conflict'), (3, 'base'), (4, 'ok-after-conflict')"); err == nil {
		t.Fatal("conflicting multi-row insert succeeded")
	}
	if got := minweightQueryStrings(t, db, "SELECT printf('%d:%s', id, sku) FROM items ORDER BY id"); !reflect.DeepEqual(got, []string{"1:base"}) {
		t.Fatalf("items after failed statement = %v", got)
	}
	if got := minweightQueryStrings(t, db, "SELECT event FROM audit ORDER BY seq"); !reflect.DeepEqual(got, []string{"insert:1:base"}) {
		t.Fatalf("audit after failed statement = %v", got)
	}

	execMinweightSQL(t, db, "INSERT INTO items(id, sku) VALUES (2, 'ok-after-rollback')")
	if got := minweightQueryStrings(t, db, "SELECT printf('%d:%s', id, sku) FROM items ORDER BY id"); !reflect.DeepEqual(got, []string{"1:base", "2:ok-after-rollback"}) {
		t.Fatalf("items after follow-up insert = %v", got)
	}
}

func TestMinweightStorageEngineDeferredForeignKeyCommitFailureDoesNotPublish(t *testing.T) {
	installMinweightStorageEngineForTest(t)

	path := filepath.Join(t.TempDir(), "deferred-fk.db")
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, writer)
	writer.SetMaxOpenConns(1)

	execMinweightSQL(t, writer, "PRAGMA foreign_keys = ON")
	execMinweightSQL(t, writer, `
		CREATE TABLE parent(id INTEGER PRIMARY KEY);
		CREATE TABLE child(
			id INTEGER PRIMARY KEY,
			parent_id INTEGER NOT NULL REFERENCES parent(id) DEFERRABLE INITIALLY DEFERRED
		);
	`)

	tx, err := writer.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("INSERT INTO child(id, parent_id) VALUES (1, 99)"); err != nil {
		rollbackMinweightTx(t, tx)
		t.Fatalf("insert deferred orphan child: %v", err)
	}
	err = tx.Commit()
	if err == nil {
		t.Fatal("deferred foreign-key commit succeeded")
	}
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		t.Fatalf("commit error = %T %v, want sqlite.Error", err, err)
	}
	if sqliteErr.Code() != sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY {
		t.Fatalf("commit code = %d, want SQLITE_CONSTRAINT_FOREIGNKEY", sqliteErr.Code())
	}

	observer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeMinweightDB(t, observer)
	observer.SetMaxOpenConns(1)
	if got := minweightQueryInt(t, observer, "SELECT count(*) FROM child"); got != 0 {
		t.Fatalf("observer child rows after failed commit = %d, want 0", got)
	}

	if _, err := writer.Exec("ROLLBACK"); err != nil && !strings.Contains(err.Error(), "no transaction") {
		t.Fatalf("cleanup rollback: %v", err)
	}
	execMinweightSQL(t, writer, "INSERT INTO parent(id) VALUES (99)")
	execMinweightSQL(t, writer, "INSERT INTO child(id, parent_id) VALUES (1, 99)")
	if got := minweightQueryInt(t, observer, "SELECT count(*) FROM child WHERE parent_id = 99"); got != 1 {
		t.Fatalf("observer child rows after valid insert = %d, want 1", got)
	}
}
