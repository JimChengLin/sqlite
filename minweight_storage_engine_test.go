// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite_test

import (
	"database/sql"
	"sort"
	"testing"

	sqlite "modernc.org/sqlite"
)

func TestMinweightStorageEngineSimpleSPJ(t *testing.T) {
	sqlite.SetStorageEngine(sqlite.NewMinweightStorageEngine())
	defer sqlite.SetStorageEngine(nil)

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
	defer rows.Close()

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

func execMinweightSQL(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.Exec(query); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}
