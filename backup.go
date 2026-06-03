// Copyright 2025 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite // import "modernc.org/sqlite"

import (
	"context"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"

	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

// Backup object is used to manage progress and cleanup an online backup. It
// is returned by NewBackup or NewRestore.
type Backup struct {
	srcConn *conn // source database connection
	dstConn *conn // destination database connection

	pBackup uintptr // sqlite3_backup object pointer
	logical *logicalBackup
}

type logicalBackup struct {
	src       *conn
	dst       *conn
	pageCount int
	remaining int
	started   bool
	done      bool
	released  bool
	err       error
}

// Step will copy up to n pages between the source and destination databases
// specified by the backup object. If n is negative, all remaining source
// pages are copied.
// If it successfully copies n pages and there are still more pages to be
// copied, then the function returns true with no error. If it successfully
// finishes copying all pages from source to destination, then it returns
// false with no error. If an error occurs while running, then an error is
// returned.
func (b *Backup) Step(n int32) (bool, error) {
	if b.logical != nil {
		return b.logical.step(n)
	}
	rc := sqlite3.Xsqlite3_backup_step(b.srcConn.tls, b.pBackup, n)
	if rc == sqlite3.SQLITE_OK {
		return true, nil
	} else if rc == sqlite3.SQLITE_DONE {
		return false, nil
	} else {
		return false, b.srcConn.errstr(rc)
	}
}

// Finish releases all resources associated with the Backup object. The Backup
// object is invalid and may not be used following a call to Finish.
func (b *Backup) Finish() error {
	if b.logical != nil {
		b.dstConn.Close()
		return b.logical.release()
	}
	rc := sqlite3.Xsqlite3_backup_finish(b.srcConn.tls, b.pBackup)
	b.dstConn.Close()
	if rc == sqlite3.SQLITE_OK {
		return nil
	} else {
		return b.srcConn.errstr(rc)
	}
}

// Remaining returns the number of source-database pages still to be backed
// up at the conclusion of the most recent [Backup.Step] call. The value is
// useful for driving progress UIs that need to estimate how much work is
// left.
//
// If Step has not yet been called on this Backup, or if the most recent
// Step returned false (SQLITE_DONE), Remaining returns 0.
//
// See https://www.sqlite.org/c3ref/backup_finish.html.
func (b *Backup) Remaining() int {
	if b.logical != nil {
		return b.logical.Remaining()
	}
	return int(sqlite3.Xsqlite3_backup_remaining(b.srcConn.tls, b.pBackup))
}

// PageCount returns the total number of pages in the source database at the
// conclusion of the most recent [Backup.Step] call. Pair with [Backup.Remaining]
// to compute progress as a fraction (PageCount - Remaining) / PageCount.
//
// See https://www.sqlite.org/c3ref/backup_finish.html.
func (b *Backup) PageCount() int {
	if b.logical != nil {
		return b.logical.PageCount()
	}
	return int(sqlite3.Xsqlite3_backup_pagecount(b.srcConn.tls, b.pBackup))
}

// Commit releases all resources associated with the Backup object but does not
// close the destination database connection.
//
// The destination database connection is returned to the caller or an error if raised.
// It is the responsibility of the caller to handle the connection closure.
func (b *Backup) Commit() (driver.Conn, error) {
	if b.logical != nil {
		if !b.logical.done && b.logical.err == nil {
			_, _ = b.logical.step(-1)
		}
		if err := b.logical.release(); err != nil {
			b.dstConn.Close()
			return nil, err
		}
		if b.logical.err == nil {
			return b.dstConn, nil
		}
		b.dstConn.Close()
		return nil, b.logical.err
	}
	rc := sqlite3.Xsqlite3_backup_finish(b.srcConn.tls, b.pBackup)

	if rc == sqlite3.SQLITE_OK {
		return b.dstConn, nil
	} else {
		b.dstConn.Close()
		return nil, b.srcConn.errstr(rc)
	}
}

func newLogicalBackup(src *conn, dst *conn) (*logicalBackup, error) {
	if err := logicalBackupCheckDestination(dst); err != nil {
		return nil, err
	}
	pageCount, err := logicalBackupPageCount(src)
	if err != nil {
		return nil, err
	}
	if rc := sqlite3.StorageEngineBeginLogicalBackup(src.tls, src.db); rc != sqlite3.SQLITE_OK {
		return nil, errstrForDB(src.tls, rc, src.db)
	}
	return &logicalBackup{src: src, dst: dst, pageCount: pageCount, remaining: pageCount}, nil
}

func logicalBackupCheckDestination(dst *conn) error {
	schema, err := libc.CString("main")
	if err != nil {
		return err
	}
	defer libc.Xfree(dst.tls, schema)
	if sqlite3.Xsqlite3_txn_state(dst.tls, dst.db, schema) != sqlite3.SQLITE_TXN_NONE {
		return &Error{msg: "SQL logic error: destination database is in use (1)", code: int(sqlite3.SQLITE_ERROR)}
	}
	return nil
}

func (b *logicalBackup) step(n int32) (bool, error) {
	if b.err != nil {
		return false, b.err
	}
	if b.done {
		return false, nil
	}
	if !b.started {
		b.started = true
	}
	if n >= 0 && int(n) < b.remaining {
		b.remaining -= int(n)
		return true, nil
	}
	b.err = logicalBackupCopy(b.src, b.dst)
	if b.err != nil {
		return false, b.err
	}
	b.remaining = 0
	b.done = true
	return false, nil
}

func (b *logicalBackup) release() error {
	if b.released {
		return b.err
	}
	b.released = true
	if rc := sqlite3.StorageEngineFinishLogicalBackup(b.src.tls, b.src.db); rc != sqlite3.SQLITE_OK {
		err := errstrForDB(b.src.tls, rc, b.src.db)
		if b.err == nil {
			b.err = err
		}
		return err
	}
	return b.err
}

func (b *logicalBackup) Remaining() int {
	if !b.started || b.done {
		return 0
	}
	return b.remaining
}

func (b *logicalBackup) PageCount() int {
	if !b.started {
		return 0
	}
	return b.pageCount
}

func logicalBackupPageCount(src *conn) (int, error) {
	count := 2
	tables, err := logicalBackupQuery(src, "SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY rowid")
	if err != nil {
		return 0, err
	}
	for _, table := range tables {
		name, ok := table[0].(string)
		if !ok {
			return 0, fmt.Errorf("sqlite: backup table name is %T", table[0])
		}
		rows, err := logicalBackupQuery(src, "SELECT COUNT(*) FROM "+quoteIdent(name))
		if err != nil {
			return 0, err
		}
		if len(rows) != 1 {
			return 0, fmt.Errorf("sqlite: backup count for %s returned %d rows", name, len(rows))
		}
		switch v := rows[0][0].(type) {
		case int64:
			count += int(v/32) + 1
		default:
			return 0, fmt.Errorf("sqlite: backup count for %s is %T", name, rows[0][0])
		}
	}
	if count < 2 {
		count = 2
	}
	return count, nil
}

func logicalBackupCopy(src *conn, dst *conn) (err error) {
	if err := logicalBackupExec(dst, "PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	if err := logicalBackupClearDestination(dst); err != nil {
		return err
	}
	if err := logicalBackupCreateSchema(src, dst); err != nil {
		return err
	}
	if err := logicalBackupEnsureTables(src, dst); err != nil {
		return err
	}
	if err := logicalBackupCopyRows(src, dst); err != nil {
		return err
	}
	return nil
}

func logicalBackupClearDestination(dst *conn) error {
	rows, err := logicalBackupQuery(dst, `
		SELECT type, name
		FROM sqlite_schema
		WHERE name NOT LIKE 'sqlite_%'
		  AND type IN ('trigger', 'view', 'index', 'table')
		ORDER BY CASE type
			WHEN 'trigger' THEN 0
			WHEN 'view' THEN 1
			WHEN 'index' THEN 2
			WHEN 'table' THEN 3
			ELSE 4
		END`)
	if err != nil {
		return err
	}
	for _, row := range rows {
		typ, ok := row[0].(string)
		if !ok {
			return fmt.Errorf("sqlite: backup schema type is %T", row[0])
		}
		name, ok := row[1].(string)
		if !ok {
			return fmt.Errorf("sqlite: backup schema name is %T", row[1])
		}
		var sql string
		switch typ {
		case "table":
			sql = "DROP TABLE IF EXISTS " + quoteIdent(name)
		case "index":
			sql = "DROP INDEX IF EXISTS " + quoteIdent(name)
		case "trigger":
			sql = "DROP TRIGGER IF EXISTS " + quoteIdent(name)
		case "view":
			sql = "DROP VIEW IF EXISTS " + quoteIdent(name)
		default:
			return fmt.Errorf("sqlite: backup unknown schema type %q", typ)
		}
		if err := logicalBackupExec(dst, sql); err != nil {
			return err
		}
	}
	return nil
}

func logicalBackupCreateSchema(src *conn, dst *conn) error {
	rows, err := logicalBackupQuery(src, `
		SELECT sql
		FROM sqlite_schema
		WHERE sql IS NOT NULL
		  AND name NOT LIKE 'sqlite_%'
		  AND type IN ('table', 'index', 'trigger', 'view')
		ORDER BY CASE type
			WHEN 'table' THEN 0
			WHEN 'index' THEN 1
			WHEN 'trigger' THEN 2
			WHEN 'view' THEN 3
			ELSE 4
		END, rowid`)
	if err != nil {
		return err
	}
	for _, row := range rows {
		sql, ok := row[0].(string)
		if !ok {
			return fmt.Errorf("sqlite: backup schema SQL is %T", row[0])
		}
		if err := logicalBackupExec(dst, sql); err != nil {
			return err
		}
	}
	return nil
}

func logicalBackupCopyRows(src *conn, dst *conn) error {
	tables, err := logicalBackupQuery(src, "SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY rowid")
	if err != nil {
		return err
	}
	for _, table := range tables {
		name, ok := table[0].(string)
		if !ok {
			return fmt.Errorf("sqlite: backup table name is %T", table[0])
		}
		if err := logicalBackupCopyTable(src, dst, name); err != nil {
			return err
		}
	}
	return nil
}

func logicalBackupEnsureTables(src *conn, dst *conn) error {
	tables, err := logicalBackupQuery(src, "SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY rowid")
	if err != nil {
		return err
	}
	for _, table := range tables {
		name, ok := table[0].(string)
		if !ok {
			return fmt.Errorf("sqlite: backup table name is %T", table[0])
		}
		exists, err := logicalBackupTableExists(dst, name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		sql, err := logicalBackupSynthCreateTable(src, name)
		if err != nil {
			return err
		}
		if err := logicalBackupExec(dst, sql); err != nil {
			return err
		}
	}
	return nil
}

func logicalBackupTableExists(c *conn, table string) (bool, error) {
	rows, err := logicalBackupQuery(c, "SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name="+quoteLiteral(table))
	if err != nil {
		return false, err
	}
	if len(rows) != 1 {
		return false, fmt.Errorf("sqlite: backup table lookup for %s returned %d rows", table, len(rows))
	}
	n, ok := rows[0][0].(int64)
	if !ok {
		return false, fmt.Errorf("sqlite: backup table lookup for %s is %T", table, rows[0][0])
	}
	return n != 0, nil
}

func logicalBackupSynthCreateTable(src *conn, table string) (string, error) {
	rows, err := logicalBackupQuery(src, "PRAGMA table_info("+quoteIdent(table)+")")
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("sqlite: backup table %s has no columns", table)
	}
	cols := make([]string, 0, len(rows))
	var pk []string
	for _, row := range rows {
		name, ok := row[1].(string)
		if !ok {
			return "", fmt.Errorf("sqlite: backup column name is %T", row[1])
		}
		typ := ""
		if row[2] != nil {
			var ok bool
			typ, ok = row[2].(string)
			if !ok {
				return "", fmt.Errorf("sqlite: backup column type for %s is %T", name, row[2])
			}
		}
		notNull, ok := row[3].(int64)
		if !ok {
			return "", fmt.Errorf("sqlite: backup column notnull for %s is %T", name, row[3])
		}
		pkRank, ok := row[5].(int64)
		if !ok {
			return "", fmt.Errorf("sqlite: backup column pk for %s is %T", name, row[5])
		}
		col := quoteIdent(name)
		if typ != "" {
			col += " " + typ
		}
		if notNull != 0 {
			col += " NOT NULL"
		}
		if row[4] != nil {
			dflt, ok := row[4].(string)
			if !ok {
				return "", fmt.Errorf("sqlite: backup column default for %s is %T", name, row[4])
			}
			col += " DEFAULT " + dflt
		}
		cols = append(cols, col)
		if pkRank != 0 {
			pk = append(pk, quoteIdent(name))
		}
	}
	if len(pk) == 1 {
		for i, col := range cols {
			if strings.HasPrefix(col, pk[0]+" ") || col == pk[0] {
				cols[i] = col + " PRIMARY KEY"
				pk = nil
				break
			}
		}
	}
	if len(pk) != 0 {
		cols = append(cols, "PRIMARY KEY ("+strings.Join(pk, ", ")+")")
	}
	return "CREATE TABLE " + quoteIdent(table) + " (" + strings.Join(cols, ", ") + ")", nil
}

func logicalBackupCopyTable(src *conn, dst *conn, table string) error {
	r, err := src.query(context.Background(), "SELECT * FROM "+quoteIdent(table), nil)
	if err != nil {
		return err
	}
	defer r.Close()
	cols := r.Columns()
	if len(cols) == 0 {
		return nil
	}
	insert := "INSERT INTO " + quoteIdent(table) + " VALUES (" + strings.TrimRight(strings.Repeat("?,", len(cols)), ",") + ")"
	values := make([]driver.Value, len(cols))
	for {
		if err := r.Next(values); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		args := make([]driver.NamedValue, len(values))
		for i, value := range values {
			args[i] = driver.NamedValue{Ordinal: i + 1, Value: value}
		}
		if _, err := dst.exec(context.Background(), insert, args); err != nil {
			return err
		}
	}
}

func logicalBackupQuery(c *conn, query string) ([][]driver.Value, error) {
	r, err := c.query(context.Background(), query, nil)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	values := make([]driver.Value, len(r.Columns()))
	var out [][]driver.Value
	for {
		if err := r.Next(values); err != nil {
			if err == io.EOF {
				return out, nil
			}
			return nil, err
		}
		row := make([]driver.Value, len(values))
		copy(row, values)
		out = append(out, row)
	}
}

func logicalBackupExec(c *conn, query string) error {
	_, err := c.exec(context.Background(), query, nil)
	return err
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func quoteLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}
