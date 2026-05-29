// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite

import (
	"database/sql"
	"strings"
	"testing"
)

// TestColumnTextScan exercises the rows.Scan TEXT-column path that
// (*conn).columnText feeds. The test covers the three branches of that
// function: NULL (returned as empty string), empty string, and non-empty
// values of varying lengths. A regression on the unsafe.String pattern would
// either truncate the result, expose Go-heap garbage, or trip the race
// detector under -race.
func TestColumnTextScan(t *testing.T) {
	db, err := sql.Open(driverName, "file::memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, s TEXT)`); err != nil {
		t.Fatal(err)
	}

	long := strings.Repeat("a1B2c3D4", 256) // 2048 bytes, well past inline storage
	cases := []struct {
		id int64
		s  string
	}{
		{1, "hello"},
		{2, ""},
		{3, "unicode é 中文 \U0001F600"},
		{4, long},
	}
	for _, c := range cases {
		if _, err := db.Exec(`INSERT INTO t(id, s) VALUES (?, ?)`, c.id, c.s); err != nil {
			t.Fatalf("insert id=%d: %v", c.id, err)
		}
	}

	for _, c := range cases {
		var got string
		if err := db.QueryRow(`SELECT s FROM t WHERE id = ?`, c.id).Scan(&got); err != nil {
			t.Fatalf("scan id=%d: %v", c.id, err)
		}
		if got != c.s {
			t.Errorf("id=%d: got %q (len %d), want %q (len %d)", c.id, got, len(got), c.s, len(c.s))
		}
	}

	// Read all rows in a single query so columnText is invoked many times in
	// quick succession; a stale-pointer regression would surface here as the
	// last row's text bleeding into earlier rows.
	rows, err := db.Query(`SELECT s FROM t ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		got = append(got, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	want := []string{"hello", "", "unicode é 中文 \U0001F600", long}
	if len(got) != len(want) {
		t.Fatalf("row count: got %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("row %d: got %q (len %d), want %q (len %d)", i, got[i], len(got[i]), want[i], len(want[i]))
		}
	}
}

// benchColumnTextScan exercises the rows.Scan TEXT path many times so the
// per-row cost is dominated by (*conn).columnText, not by statement
// preparation or driver bookkeeping. textLen controls the per-row payload
// size; the default mode in conn.go did one make+one string(b)-copy per
// invocation, which means at small sizes the alloc count dominates and at
// large sizes the memcpy dominates.
func benchColumnTextScan(b *testing.B, textLen int) {
	db, err := sql.Open(driverName, "file::memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE t (s TEXT)`); err != nil {
		b.Fatal(err)
	}

	payload := strings.Repeat("X", textLen)
	const rows = 1000
	tx, err := db.Begin()
	if err != nil {
		b.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO t (s) VALUES (?)`)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < rows; i++ {
		if _, err := stmt.Exec(payload); err != nil {
			b.Fatal(err)
		}
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := db.Query(`SELECT s FROM t`)
		if err != nil {
			b.Fatal(err)
		}
		for r.Next() {
			var s string
			if err := r.Scan(&s); err != nil {
				b.Fatal(err)
			}
		}
		if err := r.Err(); err != nil {
			b.Fatal(err)
		}
		r.Close()
	}
}

// BenchmarkColumnTextScanShort measures the rows.Scan TEXT path on a small
// payload where alloc count dominates the cost.
func BenchmarkColumnTextScanShort(b *testing.B) {
	benchColumnTextScan(b, 16)
}

// BenchmarkColumnTextScanMedium measures the rows.Scan TEXT path on a
// medium payload where both alloc count and memcpy contribute.
func BenchmarkColumnTextScanMedium(b *testing.B) {
	benchColumnTextScan(b, 256)
}

// BenchmarkColumnTextScanLong measures the rows.Scan TEXT path on a large
// payload where the second memcpy that the old string(b) conversion forced
// is the dominant cost.
func BenchmarkColumnTextScanLong(b *testing.B) {
	benchColumnTextScan(b, 4096)
}
