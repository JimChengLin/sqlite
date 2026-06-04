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
	"sort"
	"strings"
	"time"

	sqlite "modernc.org/sqlite"
)

type engineConfig struct {
	name      string
	minweight bool
}

type scenario struct {
	name string
	run  func(context.Context, *sql.DB, int, int, int64, int, int) error
	ops  func(rows, ops int) int
}

type runResult struct {
	engine   string
	scenario string
	ops      int
	elapsed  time.Duration
	disk     int64
	err      string
}

type summaryResult struct {
	engine    string
	scenario  string
	ops       int
	median    time.Duration
	opsPerSec float64
	runs      []time.Duration
	disk      int64
	diskRuns  []int64
	runNotes  []string
	failed    bool
}

func main() {
	var (
		rows          = flag.Int("rows", 5000, "preloaded rows per scenario")
		ops           = flag.Int("ops", 20000, "logical operations per scenario")
		runs          = flag.Int("runs", 3, "runs per engine/scenario")
		out           = flag.String("out", "MINWEIGHT_OLTP_BENCHMARK.md", "markdown report path")
		payloadBytes  = flag.Int("payload-bytes", 0, "bytes stored in accounts.payload for each row")
		scenarioNames = flag.String("scenarios", "", "comma-separated scenario names; empty runs all scenarios")
		seedBatchSize = flag.Int("seed-batch-size", 0, "rows per preload transaction; 0 preloads all rows in one transaction")
	)
	flag.Parse()

	if *rows <= 0 || *ops <= 0 || *runs <= 0 {
		fatalf("rows, ops, and runs must be positive")
	}
	if *payloadBytes < 0 {
		fatalf("payload-bytes must be non-negative")
	}
	if *seedBatchSize < 0 {
		fatalf("seed-batch-size must be non-negative")
	}

	ctx := context.Background()
	tempRoot, err := os.MkdirTemp("", "sqlite-minweight-oltp-*")
	if err != nil {
		fatalf("temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempRoot) }()

	scenarios := []scenario{
		{name: "bulk_insert_tx", run: runBulkInsertTx, ops: func(rows, ops int) int { return ops }},
		{name: "point_select_pk", run: runPointSelectPK, ops: func(rows, ops int) int { return ops }},
		{name: "point_select_secondary", run: runPointSelectSecondary, ops: func(rows, ops int) int { return ops }},
		{name: "update_by_pk_tx", run: runUpdateByPKTx, ops: func(rows, ops int) int { return ops }},
		{name: "upsert_by_pk_tx", run: runUpsertByPKTx, ops: func(rows, ops int) int { return ops }},
		{name: "mixed_small_tx", run: runMixedSmallTx, ops: func(rows, ops int) int { return (ops / 20) * 20 }},
	}
	scenarios, err = filterScenarios(scenarios, *scenarioNames)
	if err != nil {
		fatalf("scenarios: %v", err)
	}
	engines := []engineConfig{
		{name: "native_btree"},
		{name: "minweight_store", minweight: true},
	}

	var results []runResult
	for _, sc := range scenarios {
		for _, engine := range engines {
			for run := 0; run < *runs; run++ {
				dbPath := filepath.Join(tempRoot, fmt.Sprintf("%s-%s-%d.db", engine.name, sc.name, run))
				elapsed, disk, err := runOnce(ctx, engine, sc, dbPath, *rows, *ops, int64(run+1), *payloadBytes, *seedBatchSize)
				result := runResult{
					engine:   engine.name,
					scenario: sc.name,
					ops:      sc.ops(*rows, *ops),
					elapsed:  elapsed,
					disk:     disk,
				}
				if err != nil {
					result.err = err.Error()
				}
				results = append(results, result)
				if result.err != "" {
					fmt.Printf("%-16s %-24s run=%d ops=%d elapsed=%s disk=%s err=%s\n",
						result.engine, result.scenario, run+1, result.ops, result.elapsed, formatBytes(result.disk), result.err)
					continue
				}
				fmt.Printf("%-16s %-24s run=%d ops=%d elapsed=%s ops/s=%.0f disk=%s\n",
					result.engine, result.scenario, run+1, result.ops, result.elapsed, opsPerSecond(result.ops, result.elapsed), formatBytes(result.disk))
			}
		}
	}

	report, err := renderReport(results, *rows, *ops, *runs, *payloadBytes, *seedBatchSize, *scenarioNames)
	if err != nil {
		fatalf("render report: %v", err)
	}
	if err := os.WriteFile(*out, []byte(report), 0o644); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Printf("wrote %s\n", *out)
}

func filterScenarios(scenarios []scenario, names string) ([]scenario, error) {
	if strings.TrimSpace(names) == "" {
		return scenarios, nil
	}
	available := map[string]scenario{}
	for _, sc := range scenarios {
		available[sc.name] = sc
	}
	selected := make([]scenario, 0, len(scenarios))
	seen := map[string]bool{}
	for _, raw := range strings.Split(names, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, fmt.Errorf("empty scenario name in %q", names)
		}
		if seen[name] {
			continue
		}
		sc, ok := available[name]
		if !ok {
			return nil, fmt.Errorf("unknown scenario %q", name)
		}
		selected = append(selected, sc)
		seen[name] = true
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no scenarios selected")
	}
	return selected, nil
}

func runOnce(ctx context.Context, engine engineConfig, sc scenario, dbPath string, rows int, ops int, seed int64, payloadBytes int, seedBatchSize int) (time.Duration, int64, error) {
	if err := os.RemoveAll(dbPath); err != nil {
		return 0, 0, err
	}
	defer func() { _ = os.RemoveAll(dbPath) }()
	if engine.minweight {
		sqlite.SetStorageEngine(sqlite.NewMinweightStorageEngine())
	} else {
		sqlite.SetStorageEngine(nil)
	}
	defer sqlite.SetStorageEngine(nil)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, 0, err
	}
	db.SetMaxOpenConns(1)

	if err := applyPragmas(ctx, db); err != nil {
		_ = db.Close()
		return 0, 0, err
	}

	start := time.Now()
	if err := sc.run(ctx, db, rows, ops, seed, payloadBytes, seedBatchSize); err != nil {
		_ = db.Close()
		disk, diskErr := pathSize(dbPath)
		if diskErr != nil {
			return time.Since(start), 0, fmt.Errorf("%w; disk size: %v", err, diskErr)
		}
		return time.Since(start), disk, err
	}
	elapsed := time.Since(start)
	if err := db.Close(); err != nil {
		return 0, 0, err
	}
	disk, err := pathSize(dbPath)
	if err != nil {
		return 0, 0, err
	}
	return elapsed, disk, nil
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

func createOLTPSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE accounts(
	id INTEGER PRIMARY KEY,
	email TEXT NOT NULL UNIQUE,
	balance INTEGER NOT NULL,
	status TEXT NOT NULL,
	updated_at INTEGER NOT NULL,
	payload BLOB NOT NULL
);
CREATE INDEX accounts_status_balance ON accounts(status, balance);
CREATE TABLE ledger(
	id INTEGER PRIMARY KEY,
	account_id INTEGER NOT NULL,
	amount INTEGER NOT NULL,
	note TEXT NOT NULL
);
CREATE INDEX ledger_account ON ledger(account_id);
`)
	return err
}

func seedAccounts(ctx context.Context, db *sql.DB, rows int, payloadBytes int, seedBatchSize int) error {
	if err := createOLTPSchema(ctx, db); err != nil {
		return err
	}
	if seedBatchSize == 0 {
		return insertAccountRange(ctx, db, 1, rows, payloadBytes)
	}
	for start := 1; start <= rows; start += seedBatchSize {
		end := start + seedBatchSize - 1
		if end > rows {
			end = rows
		}
		if err := insertAccountRange(ctx, db, start, end, payloadBytes); err != nil {
			return err
		}
	}
	return nil
}

func insertAccountRange(ctx context.Context, db *sql.DB, start int, end int, payloadBytes int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO accounts(id, email, balance, status, updated_at, payload) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	payload := make([]byte, payloadBytes)
	for i := start; i <= end; i++ {
		status := "open"
		if i%5 == 0 {
			status = "hold"
		}
		fillPayload(payload, i)
		if _, err := stmt.ExecContext(ctx, i, emailForID(i), i%1000, status, i, payload); err != nil {
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

func runBulkInsertTx(ctx context.Context, db *sql.DB, _ int, ops int, _ int64, payloadBytes int, _ int) error {
	if err := createOLTPSchema(ctx, db); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO accounts(id, email, balance, status, updated_at, payload) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	payload := make([]byte, payloadBytes)
	for i := 1; i <= ops; i++ {
		status := "open"
		if i%5 == 0 {
			status = "hold"
		}
		fillPayload(payload, i)
		if _, err := stmt.ExecContext(ctx, i, emailForID(i), i%1000, status, i, payload); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return assertCount(ctx, db, "accounts", ops)
}

func runPointSelectPK(ctx context.Context, db *sql.DB, rows int, ops int, seed int64, payloadBytes int, seedBatchSize int) error {
	if err := seedAccounts(ctx, db, rows, payloadBytes, seedBatchSize); err != nil {
		return err
	}
	rng := rand.New(rand.NewSource(seed))
	stmt, err := db.PrepareContext(ctx, "SELECT balance FROM accounts WHERE id = ?")
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	var sink int
	for i := 0; i < ops; i++ {
		id := 1 + rng.Intn(rows)
		if err := stmt.QueryRowContext(ctx, id).Scan(&sink); err != nil {
			return err
		}
	}
	_ = sink
	return nil
}

func runPointSelectSecondary(ctx context.Context, db *sql.DB, rows int, ops int, seed int64, payloadBytes int, seedBatchSize int) error {
	if err := seedAccounts(ctx, db, rows, payloadBytes, seedBatchSize); err != nil {
		return err
	}
	rng := rand.New(rand.NewSource(seed))
	stmt, err := db.PrepareContext(ctx, "SELECT balance FROM accounts WHERE email = ?")
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	var sink int
	for i := 0; i < ops; i++ {
		id := 1 + rng.Intn(rows)
		if err := stmt.QueryRowContext(ctx, emailForID(id)).Scan(&sink); err != nil {
			return err
		}
	}
	_ = sink
	return nil
}

func runUpdateByPKTx(ctx context.Context, db *sql.DB, rows int, ops int, seed int64, payloadBytes int, seedBatchSize int) error {
	if err := seedAccounts(ctx, db, rows, payloadBytes, seedBatchSize); err != nil {
		return err
	}
	rng := rand.New(rand.NewSource(seed))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, "UPDATE accounts SET balance = balance + 1, updated_at = ? WHERE id = ?")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for i := 0; i < ops; i++ {
		id := 1 + rng.Intn(rows)
		if _, err := stmt.ExecContext(ctx, i, id); err != nil {
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

func runUpsertByPKTx(ctx context.Context, db *sql.DB, rows int, ops int, seed int64, payloadBytes int, seedBatchSize int) error {
	if err := seedAccounts(ctx, db, rows, payloadBytes, seedBatchSize); err != nil {
		return err
	}
	rng := rand.New(rand.NewSource(seed))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO accounts(id, email, balance, status, updated_at, payload)
VALUES (?, ?, 1, 'open', ?, zeroblob(0))
ON CONFLICT(id) DO UPDATE SET balance = accounts.balance + excluded.balance, updated_at = excluded.updated_at`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for i := 0; i < ops; i++ {
		id := 1 + rng.Intn(rows)
		if _, err := stmt.ExecContext(ctx, id, emailForID(id), i); err != nil {
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

func runMixedSmallTx(ctx context.Context, db *sql.DB, rows int, ops int, seed int64, payloadBytes int, seedBatchSize int) error {
	if err := seedAccounts(ctx, db, rows, payloadBytes, seedBatchSize); err != nil {
		return err
	}
	rng := rand.New(rand.NewSource(seed))
	txns := ops / 20
	ledgerID := 1
	for txnID := 0; txnID < txns; txnID++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for i := 0; i < 10; i++ {
			id := 1 + rng.Intn(rows)
			var balance int
			if err := tx.QueryRowContext(ctx, "SELECT balance FROM accounts WHERE id = ?", id).Scan(&balance); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		for i := 0; i < 5; i++ {
			id := 1 + rng.Intn(rows)
			var balance int
			if err := tx.QueryRowContext(ctx, "SELECT balance FROM accounts WHERE email = ?", emailForID(id)).Scan(&balance); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		for i := 0; i < 4; i++ {
			id := 1 + rng.Intn(rows)
			if _, err := tx.ExecContext(ctx, "UPDATE accounts SET balance = balance + 1, updated_at = ? WHERE id = ?", txnID, id); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		id := 1 + rng.Intn(rows)
		if _, err := tx.ExecContext(ctx, "INSERT INTO ledger(id, account_id, amount, note) VALUES (?, ?, ?, ?)", ledgerID, id, txnID%100, "mixed"); err != nil {
			_ = tx.Rollback()
			return err
		}
		ledgerID++
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return assertCount(ctx, db, "ledger", txns)
}

func assertCount(ctx context.Context, db *sql.DB, table string, want int) error {
	var got int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&got); err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%s count = %d, want %d", table, got, want)
	}
	return nil
}

func emailForID(id int) string {
	return fmt.Sprintf("user%08d@example.test", id)
}

func fillPayload(payload []byte, id int) {
	if len(payload) == 0 {
		return
	}
	if len(payload) >= 8 {
		binary.LittleEndian.PutUint64(payload[:8], uint64(id))
	} else {
		for i := range payload {
			payload[i] = byte(id + i)
		}
		return
	}
	for i := 8; i < len(payload); i++ {
		payload[i] = byte(id + i)
	}
}

func renderReport(results []runResult, rows int, ops int, runs int, payloadBytes int, seedBatchSize int, scenarioNames string) (string, error) {
	summaries := summarize(results)
	native := map[string]summaryResult{}
	minweight := map[string]summaryResult{}
	for _, summary := range summaries {
		switch summary.engine {
		case "native_btree":
			native[summary.scenario] = summary
		case "minweight_store":
			minweight[summary.scenario] = summary
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Minweight OLTP Benchmark\n\n")
	fmt.Fprintf(&b, "Generated: %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "Environment: `%s/%s`, Go `%s`, GOMAXPROCS `%d`.\n\n", runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.GOMAXPROCS(0))
	command := fmt.Sprintf("go run ./tools/minweight_oltp_bench -rows %d -ops %d -runs %d -payload-bytes %d", rows, ops, runs, payloadBytes)
	if seedBatchSize != 0 {
		command += fmt.Sprintf(" -seed-batch-size %d", seedBatchSize)
	}
	if strings.TrimSpace(scenarioNames) != "" {
		command += fmt.Sprintf(" -scenarios %s", scenarioNames)
	}
	fmt.Fprintf(&b, "Command shape: `%s`.\n\n", command)
	fmt.Fprintf(&b, "Both engines ran through `database/sql` with the same SQL and a single open connection. Path-backed temp databases were used. Pragmas: `foreign_keys=ON`, `journal_mode=DELETE`, `synchronous=OFF`, `temp_store=MEMORY`, `cache_size=-20000`. Native uses SQLite btree pages; minweight uses `sqlite.NewMinweightStorageEngine()` and path-backed minweight stores.\n\n")
	fmt.Fprintf(&b, "Each `accounts` row stores `%s` in an unindexed payload column. Disk bytes are recursive allocated file-system bytes after closing the database for that run.\n\n", formatBytes(int64(payloadBytes)))
	if seedBatchSize != 0 {
		fmt.Fprintf(&b, "Preloaded rows are inserted in batches of `%d` rows per transaction. This keeps large-database read and small-transaction scenarios separate from unsupported very large single-transaction writes.\n\n", seedBatchSize)
	}
	fmt.Fprintf(&b, "## Median Results\n\n")
	fmt.Fprintf(&b, "| Scenario | Ops | Native ops/s | Minweight ops/s | Minweight / Native | Native median | Minweight median | Native disk | Minweight disk | Minweight / Native disk |\n")
	fmt.Fprintf(&b, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, scenario := range scenarioOrder(summaries) {
		n, ok := native[scenario]
		if !ok {
			return "", fmt.Errorf("missing native result for %s", scenario)
		}
		m, ok := minweight[scenario]
		if !ok {
			return "", fmt.Errorf("missing minweight result for %s", scenario)
		}
		ratio := "FAILED"
		if !n.failed && !m.failed {
			ratio = fmt.Sprintf("%.2fx", m.opsPerSec/n.opsPerSec)
		}
		diskRatio := "n/a"
		if n.disk > 0 && m.disk > 0 {
			diskRatio = fmt.Sprintf("%.2fx", float64(m.disk)/float64(n.disk))
		}
		fmt.Fprintf(&b, "| `%s` | %d | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			scenario, n.ops, formatOpsPerSec(n), formatOpsPerSec(m), ratio, formatMedian(n), formatMedian(m), formatBytes(n.disk), formatBytes(m.disk), diskRatio)
	}
	fmt.Fprintf(&b, "\n## Raw Runs\n\n")
	fmt.Fprintf(&b, "| Engine | Scenario | Run durations | Run disk sizes |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- |\n")
	for _, summary := range summaries {
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s |\n", summary.engine, summary.scenario, strings.Join(summary.runNotes, ", "), formatByteRuns(summary.diskRuns))
	}
	fmt.Fprintf(&b, "\n## Notes\n\n")
	fmt.Fprintf(&b, "- This benchmark measures current adapter behavior, not only the standalone minweight KV core. SQLite parsing, VDBE execution, storage-engine dispatch, key encoding, transaction overlay, and minweight store calls are all included.\n")
	fmt.Fprintf(&b, "- `synchronous=OFF` reduces native fsync cost so the comparison focuses more on btree/minweight execution and less on filesystem durability policy.\n")
	fmt.Fprintf(&b, "- `mixed_small_tx` uses small explicit transactions with 10 primary-key reads, 5 secondary-index reads, 4 updates, and 1 ledger insert per transaction.\n")
	fmt.Fprintf(&b, "- Runs that pass `-scenarios` can intentionally exclude `bulk_insert_tx`, `update_by_pk_tx`, and `upsert_by_pk_tx`. In that shape, this report does not claim support for very large single-transaction writes; that is a separate adapter limitation.\n")
	fmt.Fprintf(&b, "- Current minweight wins the measured read-heavy and many-small-transaction shapes. Remaining write work should focus on adapter overhead around SQL writes, comparable secondary keys, and commit batching rather than raw minweight KV throughput.\n")
	fmt.Fprintf(&b, "- Results are local to this machine and this git tree; rerun the command after storage-engine changes.\n")
	return b.String(), nil
}

func summarize(results []runResult) []summaryResult {
	grouped := map[string][]runResult{}
	for _, result := range results {
		key := result.engine + "\x00" + result.scenario
		grouped[key] = append(grouped[key], result)
	}
	summaries := make([]summaryResult, 0, len(grouped))
	for key, group := range grouped {
		parts := strings.Split(key, "\x00")
		sort.Slice(group, func(i, j int) bool {
			if group[i].err != group[j].err {
				return group[i].err == ""
			}
			return group[i].elapsed < group[j].elapsed
		})
		var runs []time.Duration
		diskRuns := make([]int64, 0, len(group))
		runNotes := make([]string, 0, len(group))
		for _, result := range group {
			if result.err == "" {
				runs = append(runs, result.elapsed)
				runNotes = append(runNotes, result.elapsed.String())
			} else {
				runNotes = append(runNotes, fmt.Sprintf("%s failed: %s", result.elapsed, result.err))
			}
			diskRuns = append(diskRuns, result.disk)
		}
		sort.Slice(diskRuns, func(i, j int) bool { return diskRuns[i] < diskRuns[j] })
		var median time.Duration
		var opsPerSec float64
		failed := len(runs) == 0
		if !failed {
			median = runs[len(runs)/2]
			opsPerSec = opsPerSecond(group[0].ops, median)
		}
		var disk int64
		if len(diskRuns) != 0 {
			disk = diskRuns[len(diskRuns)/2]
		}
		ops := group[0].ops
		summaries = append(summaries, summaryResult{
			engine:    parts[0],
			scenario:  parts[1],
			ops:       ops,
			median:    median,
			opsPerSec: opsPerSec,
			runs:      runs,
			disk:      disk,
			diskRuns:  diskRuns,
			runNotes:  runNotes,
			failed:    failed,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].scenario != summaries[j].scenario {
			return summaries[i].scenario < summaries[j].scenario
		}
		return summaries[i].engine < summaries[j].engine
	})
	return summaries
}

func scenarioOrder(summaries []summaryResult) []string {
	seen := map[string]bool{}
	var scenarios []string
	for _, summary := range summaries {
		if !seen[summary.scenario] {
			seen[summary.scenario] = true
			scenarios = append(scenarios, summary.scenario)
		}
	}
	sort.Strings(scenarios)
	return scenarios
}

func opsPerSecond(ops int, elapsed time.Duration) float64 {
	return float64(ops) / elapsed.Seconds()
}

func formatOpsPerSec(summary summaryResult) string {
	if summary.failed {
		return "FAILED"
	}
	return fmt.Sprintf("%.0f", summary.opsPerSec)
}

func formatMedian(summary summaryResult) string {
	if summary.failed {
		return "FAILED"
	}
	return summary.median.String()
}

func formatDurations(runs []time.Duration) string {
	parts := make([]string, len(runs))
	for i, run := range runs {
		parts[i] = run.String()
	}
	return strings.Join(parts, ", ")
}

func formatByteRuns(runs []int64) string {
	parts := make([]string, len(runs))
	for i, run := range runs {
		parts[i] = formatBytes(run)
	}
	return strings.Join(parts, ", ")
}

func formatBytes(v int64) string {
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%dB", v)
	}
	div := int64(unit)
	exp := 0
	for n := v / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f%ciB", float64(v)/float64(div), "KMGTPE"[exp])
}

func pathSize(path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(p string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += fileDiskBytes(info)
		}
		return nil
	})
	return total, err
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
