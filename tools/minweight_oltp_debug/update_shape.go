// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"time"

	sqlite "modernc.org/sqlite"
)

type updateShapeResult struct {
	engine  string
	shape   string
	elapsed time.Duration
}

const (
	updateShapeNoIndexNonIndexed      = "no_index_update_nonindexed"
	updateShapeIndexedTableNonIndexed = "indexed_table_update_nonindexed"
	updateShapeIndexedTableIndexed    = "indexed_table_update_indexed"
)

func runUpdateShapeBreakdowns(ctx context.Context, tempRoot string, rows int, ops int) ([]updateShapeResult, error) {
	var results []updateShapeResult
	for _, engine := range []string{"native_btree", "minweight_store"} {
		for _, shape := range []string{
			updateShapeNoIndexNonIndexed,
			updateShapeIndexedTableNonIndexed,
			updateShapeIndexedTableIndexed,
		} {
			elapsed, err := runSQLUpdateShape(ctx, tempRoot, engine, shape, rows, ops)
			if err != nil {
				return nil, err
			}
			result := updateShapeResult{engine: engine, shape: shape, elapsed: elapsed}
			results = append(results, result)
			fmt.Printf("%-16s update_shape=%-32s elapsed=%s ops/s=%.0f\n",
				result.engine, result.shape, result.elapsed, opsPerSecond(ops, result.elapsed))
		}
	}
	return results, nil
}

func runSQLUpdateShape(ctx context.Context, tempRoot string, engine string, shape string, rows int, ops int) (time.Duration, error) {
	dbPath := filepath.Join(tempRoot, fmt.Sprintf("%s-%s.db", engine, shape))
	if err := os.RemoveAll(dbPath); err != nil {
		return 0, err
	}
	if engine == "minweight_store" {
		sqlite.SetStorageEngine(sqlite.NewMinweightStorageEngine())
	} else {
		sqlite.SetStorageEngine(nil)
	}
	defer sqlite.SetStorageEngine(nil)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, err
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()

	if err := applyPragmas(ctx, db); err != nil {
		return 0, err
	}
	if err := seedUpdateShape(ctx, db, shape, rows); err != nil {
		return 0, err
	}

	start := time.Now()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, updateShapeSQL(shape))
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < ops; i++ {
		id := 1 + rng.Intn(rows)
		if _, err := stmt.ExecContext(ctx, i, id); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return 0, err
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func seedUpdateShape(ctx context.Context, db *sql.DB, shape string, rows int) error {
	switch shape {
	case updateShapeNoIndexNonIndexed:
		return seedSimpleAccounts(ctx, db, rows)
	case updateShapeIndexedTableNonIndexed, updateShapeIndexedTableIndexed:
		return seedAccounts(ctx, db, rows)
	default:
		return fmt.Errorf("unknown update shape %q", shape)
	}
}

func seedSimpleAccounts(ctx context.Context, db *sql.DB, rows int) error {
	if _, err := db.ExecContext(ctx, `
CREATE TABLE simple_accounts(
	id INTEGER PRIMARY KEY,
	balance INTEGER NOT NULL,
	status TEXT NOT NULL,
	updated_at INTEGER NOT NULL
);
`); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO simple_accounts(id, balance, status, updated_at) VALUES (?, ?, ?, ?)")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for i := 1; i <= rows; i++ {
		status := "open"
		if i%5 == 0 {
			status = "hold"
		}
		if _, err := stmt.ExecContext(ctx, i, i%1000, status, i); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func updateShapeSQL(shape string) string {
	switch shape {
	case updateShapeNoIndexNonIndexed:
		return "UPDATE simple_accounts SET updated_at = ? WHERE id = ?"
	case updateShapeIndexedTableNonIndexed:
		return "UPDATE accounts SET updated_at = ? WHERE id = ?"
	case updateShapeIndexedTableIndexed:
		return "UPDATE accounts SET balance = balance + 1, updated_at = ? WHERE id = ?"
	default:
		panic(fmt.Sprintf("unknown update shape %q", shape))
	}
}

func updateShapeOrder(results []updateShapeResult) []string {
	order := []string{
		updateShapeNoIndexNonIndexed,
		updateShapeIndexedTableNonIndexed,
		updateShapeIndexedTableIndexed,
	}
	seen := map[string]bool{}
	for _, result := range results {
		seen[result.shape] = true
	}
	shapes := make([]string, 0, len(seen))
	for _, shape := range order {
		if seen[shape] {
			shapes = append(shapes, shape)
			delete(seen, shape)
		}
	}
	for shape := range seen {
		shapes = append(shapes, shape)
	}
	sort.Strings(shapes[len(shapes)-len(seen):])
	return shapes
}
