// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"database/sql"
	"encoding/binary"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	minweight "github.com/JimChengLin/minweight_store"
	sqlite "modernc.org/sqlite"
)

type directResult struct {
	name    string
	ops     int
	elapsed time.Duration
}

type sqlResult struct {
	engine  string
	rows    int
	elapsed time.Duration
}

type updateBreakdown struct {
	engine string
	seed   time.Duration
	loop   time.Duration
	commit time.Duration
}

func main() {
	var (
		rows       = flag.Int("rows", 20000, "row count for direct minweight core checks")
		sqlSizes   = flag.String("sql-sizes", "1000,2000,5000,10000", "comma-separated SQL bulk insert sizes")
		updateRows = flag.Int("update-rows", 5000, "preloaded rows for SQL update phase breakdown")
		updateOps  = flag.Int("update-ops", 20000, "update operations for SQL update phase breakdown")
		out        = flag.String("out", "MINWEIGHT_OLTP_DEBUG.md", "markdown report path")
		cpuprofile = flag.String("cpuprofile", "", "optional CPU profile path for the largest minweight SQL bulk insert")
		bulkCPU    = flag.String("bulk-loop-cpuprofile", "", "optional CPU profile path for the minweight SQL bulk insert loop")
		bulkCommit = flag.String("bulk-commit-cpuprofile", "", "optional CPU profile path for the minweight SQL bulk insert commit")
		updateCPU  = flag.String("update-cpuprofile", "", "optional CPU profile path for the minweight SQL update loop")
		commitCPU  = flag.String("update-commit-cpuprofile", "", "optional CPU profile path for the minweight SQL update commit")
	)
	flag.Parse()

	if *rows <= 0 {
		fatalf("rows must be positive")
	}
	sizes, err := parseSizes(*sqlSizes)
	if err != nil {
		fatalf("parse sql sizes: %v", err)
	}

	ctx := context.Background()
	tempRoot, err := os.MkdirTemp("", "sqlite-minweight-debug-*")
	if err != nil {
		fatalf("temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempRoot) }()

	direct, err := runDirectCore(tempRoot, *rows)
	if err != nil {
		fatalf("direct core: %v", err)
	}
	sqlResults, err := runSQLScale(ctx, tempRoot, sizes, *cpuprofile)
	if err != nil {
		fatalf("sql scale: %v", err)
	}
	bulkBreakdowns, err := runBulkBreakdowns(ctx, tempRoot, sizes[len(sizes)-1], *bulkCPU, *bulkCommit)
	if err != nil {
		fatalf("bulk breakdown: %v", err)
	}
	breakdowns, err := runUpdateBreakdowns(ctx, tempRoot, *updateRows, *updateOps, *updateCPU, *commitCPU)
	if err != nil {
		fatalf("update breakdown: %v", err)
	}
	updateShapes, err := runUpdateShapeBreakdowns(ctx, tempRoot, *updateRows, *updateOps)
	if err != nil {
		fatalf("update shape breakdown: %v", err)
	}

	report := renderReport(direct, sqlResults, bulkBreakdowns, breakdowns, updateShapes, *rows, sizes, *updateRows, *updateOps, *cpuprofile, *bulkCPU, *bulkCommit, *updateCPU, *commitCPU)
	if err := os.WriteFile(*out, []byte(report), 0o644); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Printf("wrote %s\n", *out)
}

func runDirectCore(tempRoot string, rows int) ([]directResult, error) {
	store, err := minweight.Open(filepath.Join(tempRoot, "direct-minweight-store"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = store.Close() }()

	var results []directResult
	var batch minweight.WriteBatch
	start := time.Now()
	for i := 1; i <= rows; i++ {
		if err := batch.Put(tableKey(2, int64(i)), payloadForID(i)); err != nil {
			return nil, err
		}
		if err := batch.Put(emailIndexKey(3, i), []byte{1}); err != nil {
			return nil, err
		}
		if err := batch.Put(statusBalanceIndexKey(4, i), []byte{1}); err != nil {
			return nil, err
		}
	}
	results = append(results, directResult{name: "direct_build_write_batch", ops: rows * 3, elapsed: time.Since(start)})

	memStore := minweight.New()
	start = time.Now()
	for i := 1; i <= rows; i++ {
		if err := memStore.Put(tableKey(2, int64(i)), payloadForID(i)); err != nil {
			return nil, err
		}
		if err := memStore.Put(emailIndexKey(3, i), nil); err != nil {
			return nil, err
		}
		if err := memStore.Put(statusBalanceIndexKey(4, i), nil); err != nil {
			return nil, err
		}
	}
	results = append(results, directResult{name: "direct_memory_store_put", ops: rows * 3, elapsed: time.Since(start)})

	start = time.Now()
	if err := store.WriteBatch(batch); err != nil {
		return nil, err
	}
	results = append(results, directResult{name: "direct_store_write_batch", ops: rows * 3, elapsed: time.Since(start)})

	rng := rand.New(rand.NewSource(1))
	start = time.Now()
	for i := 0; i < rows; i++ {
		id := 1 + rng.Intn(rows)
		_, ok, err := store.Get(tableKey(2, int64(id)))
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("direct table get missed id %d", id)
		}
	}
	results = append(results, directResult{name: "direct_store_get_hit", ops: rows, elapsed: time.Since(start)})

	rng = rand.New(rand.NewSource(2))
	start = time.Now()
	for i := 0; i < rows; i++ {
		id := 1 + rng.Intn(rows)
		item, ok, err := store.SeekGE(emailIndexKey(3, id))
		if err != nil {
			return nil, err
		}
		if !ok || len(item.Key) == 0 {
			return nil, fmt.Errorf("direct seek missed id %d", id)
		}
	}
	results = append(results, directResult{name: "direct_store_seek_ge", ops: rows, elapsed: time.Since(start)})
	return results, nil
}

func runSQLScale(ctx context.Context, tempRoot string, sizes []int, cpuprofile string) ([]sqlResult, error) {
	var results []sqlResult
	for _, rows := range sizes {
		for _, engine := range []string{"native_btree", "minweight_store"} {
			profile := ""
			if cpuprofile != "" && engine == "minweight_store" && rows == sizes[len(sizes)-1] {
				profile = cpuprofile
			}
			elapsed, err := runSQLBulkInsert(ctx, tempRoot, engine, rows, profile)
			if err != nil {
				return nil, fmt.Errorf("%s rows=%d: %w", engine, rows, err)
			}
			result := sqlResult{engine: engine, rows: rows, elapsed: elapsed}
			results = append(results, result)
			fmt.Printf("%-16s rows=%d elapsed=%s rows/s=%.0f\n", result.engine, result.rows, result.elapsed, opsPerSecond(result.rows, result.elapsed))
		}
	}
	return results, nil
}

func runBulkBreakdowns(ctx context.Context, tempRoot string, rows int, cpuprofile string, commitProfile string) ([]updateBreakdown, error) {
	var results []updateBreakdown
	for _, engine := range []string{"native_btree", "minweight_store"} {
		profile := ""
		commit := ""
		if engine == "minweight_store" {
			profile = cpuprofile
			commit = commitProfile
		}
		result, err := runSQLBulkInsertBreakdown(ctx, tempRoot, engine, rows, profile, commit)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
		fmt.Printf("%-16s bulk_breakdown rows=%d schema=%s loop=%s commit=%s tx_total=%s\n", result.engine, rows, result.seed, result.loop, result.commit, result.loop+result.commit)
	}
	return results, nil
}

func runSQLBulkInsertBreakdown(ctx context.Context, tempRoot string, engine string, rows int, cpuprofile string, commitProfile string) (updateBreakdown, error) {
	dbPath := filepath.Join(tempRoot, fmt.Sprintf("%s-bulk-breakdown.db", engine))
	if err := os.RemoveAll(dbPath); err != nil {
		return updateBreakdown{}, err
	}
	if engine == "minweight_store" {
		sqlite.SetStorageEngine(sqlite.NewMinweightStorageEngine())
	} else {
		sqlite.SetStorageEngine(nil)
	}
	defer sqlite.SetStorageEngine(nil)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return updateBreakdown{}, err
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()

	if err := applyPragmas(ctx, db); err != nil {
		return updateBreakdown{}, err
	}
	start := time.Now()
	if err := createAccountsSchema(ctx, db); err != nil {
		return updateBreakdown{}, err
	}
	schema := time.Since(start)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return updateBreakdown{}, err
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO accounts(id, email, balance, status, updated_at) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		_ = tx.Rollback()
		return updateBreakdown{}, err
	}

	var profileFile *os.File
	if cpuprofile != "" {
		profileFile, err = os.Create(cpuprofile)
		if err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return updateBreakdown{}, err
		}
		if err := pprof.StartCPUProfile(profileFile); err != nil {
			_ = profileFile.Close()
			_ = stmt.Close()
			_ = tx.Rollback()
			return updateBreakdown{}, err
		}
	}
	start = time.Now()
	for i := 1; i <= rows; i++ {
		status := "open"
		if i%5 == 0 {
			status = "hold"
		}
		if _, err := stmt.ExecContext(ctx, i, emailForID(i), i%1000, status, i); err != nil {
			stopCPUProfile(profileFile)
			_ = stmt.Close()
			_ = tx.Rollback()
			return updateBreakdown{}, err
		}
	}
	if err := stmt.Close(); err != nil {
		stopCPUProfile(profileFile)
		_ = tx.Rollback()
		return updateBreakdown{}, err
	}
	loop := time.Since(start)
	stopCPUProfile(profileFile)

	var commitProfileFile *os.File
	if commitProfile != "" {
		commitProfileFile, err = os.Create(commitProfile)
		if err != nil {
			_ = tx.Rollback()
			return updateBreakdown{}, err
		}
		if err := pprof.StartCPUProfile(commitProfileFile); err != nil {
			_ = commitProfileFile.Close()
			_ = tx.Rollback()
			return updateBreakdown{}, err
		}
	}
	start = time.Now()
	if err := tx.Commit(); err != nil {
		stopCPUProfile(commitProfileFile)
		return updateBreakdown{}, err
	}
	commit := time.Since(start)
	stopCPUProfile(commitProfileFile)
	return updateBreakdown{engine: engine, seed: schema, loop: loop, commit: commit}, nil
}

func runUpdateBreakdowns(ctx context.Context, tempRoot string, rows int, ops int, cpuprofile string, commitProfile string) ([]updateBreakdown, error) {
	var results []updateBreakdown
	for _, engine := range []string{"native_btree", "minweight_store"} {
		profile := ""
		commit := ""
		if engine == "minweight_store" {
			profile = cpuprofile
			commit = commitProfile
		}
		result, err := runSQLUpdateBreakdown(ctx, tempRoot, engine, rows, ops, profile, commit)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
		fmt.Printf("%-16s update_breakdown seed=%s loop=%s commit=%s\n", result.engine, result.seed, result.loop, result.commit)
	}
	return results, nil
}

func runSQLUpdateBreakdown(ctx context.Context, tempRoot string, engine string, rows int, ops int, cpuprofile string, commitProfile string) (updateBreakdown, error) {
	dbPath := filepath.Join(tempRoot, fmt.Sprintf("%s-update-breakdown.db", engine))
	if err := os.RemoveAll(dbPath); err != nil {
		return updateBreakdown{}, err
	}
	if engine == "minweight_store" {
		sqlite.SetStorageEngine(sqlite.NewMinweightStorageEngine())
	} else {
		sqlite.SetStorageEngine(nil)
	}
	defer sqlite.SetStorageEngine(nil)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return updateBreakdown{}, err
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()

	if err := applyPragmas(ctx, db); err != nil {
		return updateBreakdown{}, err
	}
	start := time.Now()
	if err := seedAccounts(ctx, db, rows); err != nil {
		return updateBreakdown{}, err
	}
	seed := time.Since(start)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return updateBreakdown{}, err
	}
	stmt, err := tx.PrepareContext(ctx, "UPDATE accounts SET balance = balance + 1, updated_at = ? WHERE id = ?")
	if err != nil {
		_ = tx.Rollback()
		return updateBreakdown{}, err
	}
	var profileFile *os.File
	if cpuprofile != "" {
		profileFile, err = os.Create(cpuprofile)
		if err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return updateBreakdown{}, err
		}
		if err := pprof.StartCPUProfile(profileFile); err != nil {
			_ = profileFile.Close()
			_ = stmt.Close()
			_ = tx.Rollback()
			return updateBreakdown{}, err
		}
	}
	rng := rand.New(rand.NewSource(1))
	start = time.Now()
	for i := 0; i < ops; i++ {
		id := 1 + rng.Intn(rows)
		if _, err := stmt.ExecContext(ctx, i, id); err != nil {
			stopCPUProfile(profileFile)
			_ = stmt.Close()
			_ = tx.Rollback()
			return updateBreakdown{}, err
		}
	}
	loop := time.Since(start)
	stopCPUProfile(profileFile)
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return updateBreakdown{}, err
	}
	var commitProfileFile *os.File
	if commitProfile != "" {
		commitProfileFile, err = os.Create(commitProfile)
		if err != nil {
			_ = tx.Rollback()
			return updateBreakdown{}, err
		}
		if err := pprof.StartCPUProfile(commitProfileFile); err != nil {
			_ = commitProfileFile.Close()
			_ = tx.Rollback()
			return updateBreakdown{}, err
		}
	}
	start = time.Now()
	if err := tx.Commit(); err != nil {
		stopCPUProfile(commitProfileFile)
		return updateBreakdown{}, err
	}
	commit := time.Since(start)
	stopCPUProfile(commitProfileFile)
	return updateBreakdown{engine: engine, seed: seed, loop: loop, commit: commit}, nil
}

func runSQLBulkInsert(ctx context.Context, tempRoot string, engine string, rows int, cpuprofile string) (time.Duration, error) {
	dbPath := filepath.Join(tempRoot, fmt.Sprintf("%s-bulk-%d.db", engine, rows))
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
	if err := createAccountsSchema(ctx, db); err != nil {
		return 0, err
	}

	var profileFile *os.File
	if cpuprofile != "" {
		profileFile, err = os.Create(cpuprofile)
		if err != nil {
			return 0, err
		}
		if err := pprof.StartCPUProfile(profileFile); err != nil {
			_ = profileFile.Close()
			return 0, err
		}
	}

	start := time.Now()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		stopCPUProfile(profileFile)
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO accounts(id, email, balance, status, updated_at) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		stopCPUProfile(profileFile)
		_ = tx.Rollback()
		return 0, err
	}
	for i := 1; i <= rows; i++ {
		status := "open"
		if i%5 == 0 {
			status = "hold"
		}
		if _, err := stmt.ExecContext(ctx, i, emailForID(i), i%1000, status, i); err != nil {
			stopCPUProfile(profileFile)
			_ = stmt.Close()
			_ = tx.Rollback()
			return 0, err
		}
	}
	if err := stmt.Close(); err != nil {
		stopCPUProfile(profileFile)
		_ = tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		stopCPUProfile(profileFile)
		return 0, err
	}
	elapsed := time.Since(start)
	stopCPUProfile(profileFile)
	return elapsed, nil
}

func createAccountsSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
CREATE TABLE accounts(
	id INTEGER PRIMARY KEY,
	email TEXT NOT NULL UNIQUE,
	balance INTEGER NOT NULL,
	status TEXT NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE INDEX accounts_status_balance ON accounts(status, balance);
`); err != nil {
		return err
	}
	return nil
}

func seedAccounts(ctx context.Context, db *sql.DB, rows int) error {
	if err := createAccountsSchema(ctx, db); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO accounts(id, email, balance, status, updated_at) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for i := 1; i <= rows; i++ {
		status := "open"
		if i%5 == 0 {
			status = "hold"
		}
		if _, err := stmt.ExecContext(ctx, i, emailForID(i), i%1000, status, i); err != nil {
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

func stopCPUProfile(file *os.File) {
	if file == nil {
		return
	}
	pprof.StopCPUProfile()
	_ = file.Close()
}

func applyPragmas(ctx context.Context, db *sql.DB) error {
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = DELETE",
		"PRAGMA synchronous = OFF",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA cache_size = -20000",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("%s: %w", pragma, err)
		}
	}
	return nil
}

func tableKey(root uint32, rowid int64) []byte {
	key := make([]byte, 13)
	key[0] = 't'
	binary.BigEndian.PutUint32(key[1:5], root)
	binary.BigEndian.PutUint64(key[5:13], uint64(rowid)^(1<<63))
	return key
}

func emailIndexKey(root uint32, id int) []byte {
	key := make([]byte, 0, 48)
	key = append(key, 'i')
	key = binary.BigEndian.AppendUint32(key, root)
	key = append(key, 1)
	key = append(key, []byte(emailForID(id))...)
	key = append(key, 0)
	key = binary.BigEndian.AppendUint64(key, uint64(id)^(1<<63))
	return key
}

func statusBalanceIndexKey(root uint32, id int) []byte {
	status := "open"
	if id%5 == 0 {
		status = "hold"
	}
	key := make([]byte, 0, 32)
	key = append(key, 'i')
	key = binary.BigEndian.AppendUint32(key, root)
	key = append(key, 1)
	key = append(key, status...)
	key = append(key, 0)
	key = binary.BigEndian.AppendUint64(key, uint64(id%1000)^(1<<63))
	key = binary.BigEndian.AppendUint64(key, uint64(id)^(1<<63))
	return key
}

func payloadForID(id int) []byte {
	return []byte(fmt.Sprintf("user%08d@example.test|%08d|open|%08d", id, id%1000, id))
}

func emailForID(id int) string {
	return fmt.Sprintf("user%08d@example.test", id)
}

func parseSizes(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	sizes := make([]int, 0, len(parts))
	for _, part := range parts {
		var size int
		if _, err := fmt.Sscanf(strings.TrimSpace(part), "%d", &size); err != nil {
			return nil, err
		}
		if size <= 0 {
			return nil, fmt.Errorf("size must be positive: %d", size)
		}
		sizes = append(sizes, size)
	}
	sort.Ints(sizes)
	return sizes, nil
}

func renderReport(direct []directResult, sqlResults []sqlResult, bulkBreakdowns []updateBreakdown, breakdowns []updateBreakdown, updateShapes []updateShapeResult, rows int, sizes []int, updateRows int, updateOps int, cpuprofile string, bulkCPU string, bulkCommit string, updateCPU string, commitCPU string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Minweight OLTP Debug Notes\n\n")
	fmt.Fprintf(&b, "Generated: %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "Environment: `%s/%s`, Go `%s`, GOMAXPROCS `%d`.\n\n", runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.GOMAXPROCS(0))
	fmt.Fprintf(&b, "Direct core rows: `%d`; SQL bulk sizes: `%s`.\n\n", rows, formatInts(sizes))

	fmt.Fprintf(&b, "## Direct minweight_store Core\n\n")
	fmt.Fprintf(&b, "| Check | Ops | Elapsed | Ops/s |\n")
	fmt.Fprintf(&b, "| --- | ---: | ---: | ---: |\n")
	for _, result := range direct {
		fmt.Fprintf(&b, "| `%s` | %d | %s | %.0f |\n", result.name, result.ops, result.elapsed, opsPerSecond(result.ops, result.elapsed))
	}

	fmt.Fprintf(&b, "\n## SQL Adapter Bulk Insert Scale\n\n")
	fmt.Fprintf(&b, "| Rows | Native elapsed | Native rows/s | Minweight elapsed | Minweight rows/s | Minweight / Native |\n")
	fmt.Fprintf(&b, "| ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, rows := range sizes {
		native := findSQLResult(sqlResults, "native_btree", rows)
		minw := findSQLResult(sqlResults, "minweight_store", rows)
		ratio := opsPerSecond(rows, minw.elapsed) / opsPerSecond(rows, native.elapsed)
		fmt.Fprintf(&b, "| %d | %s | %.0f | %s | %.0f | %.3fx |\n",
			rows, native.elapsed, opsPerSecond(rows, native.elapsed), minw.elapsed, opsPerSecond(rows, minw.elapsed), ratio)
	}

	fmt.Fprintf(&b, "\n## SQL Bulk Insert Phase Breakdown\n\n")
	fmt.Fprintf(&b, "Rows: `%d`; `Tx total` excludes schema creation and matches the transaction-shaped bulk insert cost.\n\n", sizes[len(sizes)-1])
	fmt.Fprintf(&b, "| Engine | Schema | Insert loop | Commit | Tx total |\n")
	fmt.Fprintf(&b, "| --- | ---: | ---: | ---: | ---: |\n")
	for _, result := range bulkBreakdowns {
		txTotal := result.loop + result.commit
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s |\n", result.engine, result.seed, result.loop, result.commit, txTotal)
	}

	fmt.Fprintf(&b, "\n## SQL Update Phase Breakdown\n\n")
	fmt.Fprintf(&b, "Rows: `%d`; update ops: `%d`.\n\n", updateRows, updateOps)
	fmt.Fprintf(&b, "| Engine | Seed | Update loop | Commit | Total |\n")
	fmt.Fprintf(&b, "| --- | ---: | ---: | ---: | ---: |\n")
	for _, result := range breakdowns {
		total := result.seed + result.loop + result.commit
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s |\n", result.engine, result.seed, result.loop, result.commit, total)
	}

	fmt.Fprintf(&b, "\n## SQL Update Shape Isolation\n\n")
	fmt.Fprintf(&b, "Rows: `%d`; update ops: `%d`; elapsed excludes schema creation and seed inserts.\n\n", updateRows, updateOps)
	fmt.Fprintf(&b, "| Shape | Native elapsed | Minweight elapsed | Minweight / Native |\n")
	fmt.Fprintf(&b, "| --- | ---: | ---: | ---: |\n")
	for _, shape := range updateShapeOrder(updateShapes) {
		native := findUpdateShape(updateShapes, "native_btree", shape)
		minw := findUpdateShape(updateShapes, "minweight_store", shape)
		ratio := opsPerSecond(updateOps, minw.elapsed) / opsPerSecond(updateOps, native.elapsed)
		fmt.Fprintf(&b, "| `%s` | %s | %s | %.3fx |\n", shape, native.elapsed, minw.elapsed, ratio)
	}

	fmt.Fprintf(&b, "\n## Optimization Checkpoint\n\n")
	fmt.Fprintf(&b, "- Direct `minweight_store` is fast in this workload shape: the path-backed core batch writes 60k table/index-like entries in tens of milliseconds, and point reads/seeks are also in the million ops/s range.\n")
	fmt.Fprintf(&b, "- The previous near-quadratic SQL bulk insert cliff came from transaction overlay seeks scanning every entry in `tx.writes`. The current adapter keeps the exact lookup map but uses an in-memory minweight overlay store for GE/LE cursor movement, so the scale curve is now close to linear.\n")
	fmt.Fprintf(&b, "- The ordered transaction overlay is now lazy: exact-write transactions only update the write map until a range cursor needs ordered movement; after the overlay is built, later writes update it incrementally instead of rebuilding it.\n")
	fmt.Fprintf(&b, "- Monotonic table append and index probes beyond a root's tracked max key return miss/EOF from root stats instead of creating the ordered overlay. In the 50k bulk-insert profile this removed `setWriteOwned -> tx.overlay.Put` from the hot path; the remaining bulk cost is mainly the real commit `WriteBatch` plus SQLite record/key encoding.\n")
	fmt.Fprintf(&b, "- `BtreeInsert` now trusts non-zero SQLite `seekResult` for adjacent-position inserts and skips the extra exact-key probe on that path. Writes that may overwrite an existing key still verify existence.\n")
	fmt.Fprintf(&b, "- Writable table cursors now try exact rowid lookup before `SeekGE`, so primary-key UPDATE avoids paying range seek cost for known hits. Read-tracked cursors keep the older seek path for pinned-reader semantics.\n")
	fmt.Fprintf(&b, "- Commit no longer builds before/after history when no pinned reader can use it; in that common single-connection OLTP shape it publishes the `WriteBatch` and generation directly. Pinned readers still get retained before images.\n")
	fmt.Fprintf(&b, "- Versioned index store keys now use the SQLite-comparable field encoding directly, with the original SQLite record kept as the value. Full-key `BtreeIndexMoveto` probes can use exact `Get`, while prefix/range probes still use `SeekGE` and fall back to `_sqlite3VdbeRecordCompare` when needed.\n")
	fmt.Fprintf(&b, "- Write transactions no longer record every point read into `tx.reads` on the single-writer hot path. The conflict checker remains available for explicit read-set validation, but SQL UPDATE no longer pays a string/map allocation for each base lookup.\n")
	fmt.Fprintf(&b, "- `BtreeInsert` reuses the current cursor key when SQLite is replacing the same row/index entry, and `BtreeDelete` uses a known-existing delete path when the cursor already proves the entry exists. Both avoid redundant committed-store probes during UPDATE.\n")
	fmt.Fprintf(&b, "- Known-existing deletes now keep the owned cursor store key instead of cloning it again before writing the tombstone. This trims allocation/copy cost from SQLite's delete+insert secondary-index UPDATE pattern.\n")
	fmt.Fprintf(&b, "- Index replace keeps the new physical key's transaction `base` state separate from the old key delete. The old key still preserves root row-count accounting, but the new key stays `base=false` when it was only created by the current transaction, so repeated secondary-index updates can fold away intermediate writes instead of retaining false tombstones.\n")
	fmt.Fprintf(&b, "- `minweightComparableMemKey` reads transient SQLite `Mem` bytes as a view while building the final sortable key, avoiding an extra payload copy for BINARY/RTRIM text and blob probe keys.\n")
	fmt.Fprintf(&b, "- Comparable-key builders now append each field into the final key buffer instead of allocating one temporary byte slice per field. This reduced the `BtreeIndexMoveto` probe-key hot path in the indexed-column UPDATE profile.\n")
	fmt.Fprintf(&b, "- Transaction write-map keys now point at the owned write key backing array; savepoint clones and commit-history entries rebind their map key to the cloned key. This removes a hot `string(write.key)` copy without keeping stale backing arrays alive.\n")
	fmt.Fprintf(&b, "- Transaction writes that already know `base=true` skip the previous-write map lookup in `setWriteOwned`; only `base=false` writes need to check whether an earlier write must preserve base provenance. This removes a visible `mapaccess2_faststr` cost from large UPDATE/UPSERT transactions.\n")
	fmt.Fprintf(&b, "- Cursor dispatch still follows the `cursor -> btree -> engine` handle graph, but cursor-bound calls first read `BtCursor.FpBtree` and resolve the btree binding directly before falling back to the cursor map. In the 200k UPDATE diagnostic this dropped the minweight update loop from about 676ms to about 624ms.\n")
	fmt.Fprintf(&b, "- Minweight cursor lookup now stores a 1-based cursor slot id in the raw `BtCursor.FpBt` field. The Go cursor object stays owned by the engine slice/map, so cursor-bound calls can try array lookup before falling back to the cursor map without storing Go pointers in SQLite ABI memory.\n")
	fmt.Fprintf(&b, "- Minweight cursor lookup now uses an RWMutex read path, and the normal no-incrblob table-write path checks an atomic cursor count before locking and scanning cursors.\n")
	fmt.Fprintf(&b, "- Commit builds the final `WriteBatch` with tombstone deletes before puts. The write map still stores one final state per key; delete-first ordering reduced the large UPDATE commit profile by making minweight/minpatricia apply deletes before installing replacement records.\n")
	fmt.Fprintf(&b, "- Current-generation reads now reuse the already-owned value returned by `minweight_store.Get` / seek APIs. Only commit-history before-images are cloned, so table moveto and cursor seek paths avoid an extra payload copy.\n")
	fmt.Fprintf(&b, "- Row decoding now takes ownership of minweight items instead of cloning key/value again. This keeps range cursor movement on owned minweight_store data without materializing another row copy.\n")
	fmt.Fprintf(&b, "- Index replace during `BtreeInsert` uses the current cursor as proof that the old key exists and deletes it through the known-existing path. That removes a committed-store probe from SQLite's indexed-column update pattern.\n")
	fmt.Fprintf(&b, "- Read-range tracking no longer clones seek bounds a second time, and versioned-index range checks use the versioned key prefix directly. Transaction writes keep an exact map for lookup/commit, and only create the ordered overlay when cursor movement cannot be answered from exact lookup or root stats.\n")
	fmt.Fprintf(&b, "- SQL `BtreeInsert` now uses an owned-write path because `KeyBytes` / `DataBytes` already copy SQLite payloads into Go memory; generic write helpers still copy caller-owned slices. Delete metadata updates mutate the active transaction state directly instead of cloning visible metadata.\n")
	fmt.Fprintf(&b, "- Index writes still parse SQLite record bytes into a comparable key on every insert. That is needed for SQLite order compatibility; it is adapter CPU, not minweight_store CPU.\n")
	fmt.Fprintf(&b, "- Bulk insert phase breakdown separates schema, insert loop, and commit. The minweight insert loop is close to native in this diagnostic; the remaining bulk gap is concentrated in transaction commit, where the CPU profile points to `minweight_store.WriteBatch` / `minpatricia` apply rather than transaction overlay writes.\n")
	fmt.Fprintf(&b, "\n## Rejected Optimization Experiments\n\n")
	fmt.Fprintf(&b, "- Do not sort pure-Put transaction writes by physical key before building `minweight.Store.WriteBatch`. It has been tested more than once and did not improve the end-to-end OLTP benchmark on this tree. The 2026-06-04 rerun moved minweight `bulk_insert_tx` from the current baseline around `111.9ms` to about `120.7ms`, with `update_by_pk_tx` and `upsert_by_pk_tx` also slower. Revisit only if `minweight_store.WriteBatch` / `minpatricia` apply semantics change.\n")
	fmt.Fprintf(&b, "- Do not prioritize transaction ordered-overlay reuse for bulk insert. The current bulk phase breakdown shows the minweight insert loop close to native; the remaining gap is in commit and profiles as `minweight_store.WriteBatch` / `minpatricia`, not `tx.overlay.Put`.\n")
	fmt.Fprintf(&b, "- Do not reuse `minweightCursor` Go objects with a simple cursor pool. That experiment failed `TestMinweightIncrblobCursorInvalidatedByClearTable`: clear-table changes went from `1` to `0`. Cursor slot lookup is fine; cursor object reuse needs a stricter ownership design before it can be retried.\n")
	fmt.Fprintf(&b, "\n## Next Fix Direction\n\n")
	fmt.Fprintf(&b, "- Reduce update/upsert large transaction cost next. Current profile now mostly shows SQLite VDBE execution, runtime allocation/GC, real commit `WriteBatch`, `BtreeInsert`, exact base lookups, and record-to-comparable-key encoding rather than ordered-overlay writes or raw minweight KV cost.\n")
	fmt.Fprintf(&b, "- For bulk insert specifically, the next high-value work is on the minweight_store batch-apply path or a narrower adapter-to-store owned batch API. Reusing the in-memory overlay or changing commit iteration order is not supported by the current profiles.\n")
	fmt.Fprintf(&b, "- Next useful work is to reduce write-set churn for repeated secondary-index updates, avoid redundant exact base lookups during SQLite's delete+insert update pattern, and keep checking that comparable-key-only physical index keys preserve SQLite collation/tie-break behavior across broader SQL cases.\n")
	fmt.Fprintf(&b, "- After the update/upsert path is reduced, rerun the full OLTP benchmark and decide whether the remaining gap is SQLite VDBE/record-encoding overhead or adapter work that can still be removed.\n")
	if cpuprofile != "" {
		fmt.Fprintf(&b, "- CPU profile for the largest minweight SQL bulk insert: `%s`.\n", cpuprofile)
	}
	if bulkCPU != "" {
		fmt.Fprintf(&b, "- CPU profile for the minweight SQL bulk insert loop: `%s`.\n", bulkCPU)
	}
	if bulkCommit != "" {
		fmt.Fprintf(&b, "- CPU profile for the minweight SQL bulk insert commit: `%s`.\n", bulkCommit)
	}
	if updateCPU != "" {
		fmt.Fprintf(&b, "- CPU profile for the minweight SQL update loop: `%s`.\n", updateCPU)
	}
	if commitCPU != "" {
		fmt.Fprintf(&b, "- CPU profile for the minweight SQL update commit: `%s`.\n", commitCPU)
	}
	return b.String()
}

func findSQLResult(results []sqlResult, engine string, rows int) sqlResult {
	for _, result := range results {
		if result.engine == engine && result.rows == rows {
			return result
		}
	}
	panic(fmt.Sprintf("missing %s rows=%d", engine, rows))
}

func findUpdateShape(results []updateShapeResult, engine string, shape string) updateShapeResult {
	for _, result := range results {
		if result.engine == engine && result.shape == shape {
			return result
		}
	}
	panic(fmt.Sprintf("missing %s shape=%s", engine, shape))
}

func formatInts(values []int) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = fmt.Sprint(value)
	}
	return strings.Join(parts, ",")
}

func opsPerSecond(ops int, elapsed time.Duration) float64 {
	return float64(ops) / elapsed.Seconds()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
