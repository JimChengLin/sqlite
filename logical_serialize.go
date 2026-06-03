// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const logicalSerializeMagic = "modernc.org/sqlite logical serialize v1\n"

type logicalSerializedDatabase struct {
	Settings *logicalSerializedSettings `json:"settings,omitempty"`
	Schema   []logicalSerializedSchema  `json:"schema"`
	Tables   []logicalSerializedTable   `json:"tables"`
}

type logicalSerializedSettings struct {
	PageSize     int64 `json:"pageSize"`
	AutoVacuum   int64 `json:"autoVacuum"`
	SecureDelete int64 `json:"secureDelete"`
	MaxPageCount int64 `json:"maxPageCount"`
}

type logicalSerializedSchema struct {
	Type string `json:"type"`
	Name string `json:"name"`
	SQL  string `json:"sql"`
}

type logicalSerializedTable struct {
	Name        string                     `json:"name"`
	Columns     []string                   `json:"columns"`
	RowidColumn string                     `json:"rowidColumn,omitempty"`
	Rows        [][]logicalSerializedValue `json:"rows"`
}

type logicalSerializedValue struct {
	Type  string  `json:"type"`
	Int   int64   `json:"int,omitempty"`
	Float float64 `json:"float,omitempty"`
	Text  string  `json:"text,omitempty"`
	Blob  []byte  `json:"blob,omitempty"`
	Bool  bool    `json:"bool,omitempty"`
}

func logicalSerialize(c *conn) ([]byte, error) {
	db := logicalSerializedDatabase{}
	settings, err := logicalSerializeSettings(c)
	if err != nil {
		return nil, err
	}
	db.Settings = settings
	schema, err := logicalSerializeSchema(c)
	if err != nil {
		return nil, err
	}
	db.Schema = schema
	for _, object := range schema {
		if object.Type != "table" {
			continue
		}
		table, err := logicalSerializeTable(c, object.Name)
		if err != nil {
			return nil, err
		}
		db.Tables = append(db.Tables, table)
	}
	payload, err := json.Marshal(db)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(logicalSerializeMagic)+len(payload))
	copy(out, logicalSerializeMagic)
	copy(out[len(logicalSerializeMagic):], payload)
	return out, nil
}

func logicalDeserialize(c *conn, buf []byte) error {
	if !bytes.HasPrefix(buf, []byte(logicalSerializeMagic)) {
		return fmt.Errorf("sqlite: Deserialize requires a minweight logical serialization")
	}
	var db logicalSerializedDatabase
	if err := json.Unmarshal(buf[len(logicalSerializeMagic):], &db); err != nil {
		return err
	}
	if err := logicalBackupCheckDestination(c); err != nil {
		return err
	}
	if err := logicalBackupExec(c, "PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	if db.Settings != nil {
		if err := logicalDeserializeSettings(c, *db.Settings); err != nil {
			return err
		}
	}
	if err := logicalBackupClearDestination(c); err != nil {
		return err
	}
	for _, object := range db.Schema {
		if object.Type == "table" {
			if err := logicalBackupExec(c, object.SQL); err != nil {
				return err
			}
		}
	}
	for _, table := range db.Tables {
		if err := logicalDeserializeTable(c, table); err != nil {
			return err
		}
	}
	for _, object := range db.Schema {
		if object.Type != "table" {
			if err := logicalBackupExec(c, object.SQL); err != nil {
				return err
			}
		}
	}
	return nil
}

func logicalSerializeSettings(c *conn) (*logicalSerializedSettings, error) {
	pageSize, err := logicalSerializePragmaInt(c, "PRAGMA page_size")
	if err != nil {
		return nil, err
	}
	autoVacuum, err := logicalSerializePragmaInt(c, "PRAGMA auto_vacuum")
	if err != nil {
		return nil, err
	}
	secureDelete, err := logicalSerializePragmaInt(c, "PRAGMA secure_delete")
	if err != nil {
		return nil, err
	}
	maxPageCount, err := logicalSerializePragmaInt(c, "PRAGMA max_page_count")
	if err != nil {
		return nil, err
	}
	return &logicalSerializedSettings{
		PageSize:     pageSize,
		AutoVacuum:   autoVacuum,
		SecureDelete: secureDelete,
		MaxPageCount: maxPageCount,
	}, nil
}

func logicalSerializePragmaInt(c *conn, query string) (int64, error) {
	rows, err := logicalBackupQuery(c, query)
	if err != nil {
		return 0, err
	}
	if len(rows) != 1 || len(rows[0]) != 1 {
		return 0, fmt.Errorf("sqlite: serialize %s returned %d rows", query, len(rows))
	}
	v, ok := rows[0][0].(int64)
	if !ok {
		return 0, fmt.Errorf("sqlite: serialize %s returned %T", query, rows[0][0])
	}
	return v, nil
}

func logicalDeserializeSettings(c *conn, settings logicalSerializedSettings) error {
	if settings.PageSize != 0 {
		if err := logicalBackupExec(c, "PRAGMA page_size="+strconv.FormatInt(settings.PageSize, 10)); err != nil {
			return err
		}
	}
	if err := logicalBackupExec(c, "PRAGMA auto_vacuum="+strconv.FormatInt(settings.AutoVacuum, 10)); err != nil {
		return err
	}
	if err := logicalBackupExec(c, "PRAGMA secure_delete="+logicalSecureDeletePragmaValue(settings.SecureDelete)); err != nil {
		return err
	}
	if settings.MaxPageCount != 0 {
		if err := logicalBackupExec(c, "PRAGMA max_page_count="+strconv.FormatInt(settings.MaxPageCount, 10)); err != nil {
			return err
		}
	}
	return nil
}

func logicalSecureDeletePragmaValue(v int64) string {
	if v == 2 {
		return "FAST"
	}
	if v != 0 {
		return "ON"
	}
	return "OFF"
}

func logicalSerializeSchema(c *conn) ([]logicalSerializedSchema, error) {
	rows, err := logicalBackupQuery(c, `
		SELECT type, name, sql
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
		return nil, err
	}
	var out []logicalSerializedSchema
	for _, row := range rows {
		typ, ok := row[0].(string)
		if !ok {
			return nil, fmt.Errorf("sqlite: serialize schema type is %T", row[0])
		}
		name, ok := row[1].(string)
		if !ok {
			return nil, fmt.Errorf("sqlite: serialize schema name is %T", row[1])
		}
		sql, ok := row[2].(string)
		if !ok {
			return nil, fmt.Errorf("sqlite: serialize schema SQL is %T", row[2])
		}
		out = append(out, logicalSerializedSchema{Type: typ, Name: name, SQL: sql})
	}
	return out, nil
}

func logicalSerializeTable(c *conn, name string) (logicalSerializedTable, error) {
	columns, rows, err := logicalSerializeQuery(c, "SELECT * FROM "+quoteIdent(name))
	if err != nil {
		return logicalSerializedTable{}, err
	}
	table := logicalSerializedTable{Name: name, Columns: columns}
	withoutRowid, err := logicalTableWithoutRowid(c, name)
	if err != nil {
		return logicalSerializedTable{}, err
	}
	if !withoutRowid {
		rowidColumn := logicalHiddenRowidColumn(columns)
		if rowidColumn == "" {
			return logicalSerializedTable{}, fmt.Errorf("sqlite: serialize table %s shadows all rowid aliases", name)
		}
		rowidColumns, rowidRows, rowidErr := logicalSerializeQuery(c, "SELECT "+rowidColumn+" AS __minweight_rowid__, * FROM "+quoteIdent(name)+" ORDER BY "+rowidColumn)
		if rowidErr != nil {
			return logicalSerializedTable{}, rowidErr
		}
		if len(rowidColumns) != len(columns)+1 {
			return logicalSerializedTable{}, fmt.Errorf("sqlite: serialize table %s rowid query returned %d columns, want %d", name, len(rowidColumns), len(columns)+1)
		}
		table.RowidColumn = rowidColumn
		rows = rowidRows
	}
	for _, row := range rows {
		var serialized []logicalSerializedValue
		for _, value := range row {
			v, err := logicalValueFromDriver(value)
			if err != nil {
				return logicalSerializedTable{}, err
			}
			serialized = append(serialized, v)
		}
		table.Rows = append(table.Rows, serialized)
	}
	return table, nil
}

func logicalTableWithoutRowid(c *conn, name string) (bool, error) {
	rows, err := logicalBackupQuery(c, "PRAGMA table_list")
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		schema, ok := row[0].(string)
		if !ok {
			return false, fmt.Errorf("sqlite: serialize table_list schema is %T", row[0])
		}
		tableName, ok := row[1].(string)
		if !ok {
			return false, fmt.Errorf("sqlite: serialize table_list name is %T", row[1])
		}
		if schema != "main" || tableName != name {
			continue
		}
		wr, ok := row[4].(int64)
		if !ok {
			return false, fmt.Errorf("sqlite: serialize table_list wr is %T", row[4])
		}
		return wr != 0, nil
	}
	return false, fmt.Errorf("sqlite: serialize table %s not found in pragma_table_list", name)
}

func logicalSerializeQuery(c *conn, query string) ([]string, [][]driver.Value, error) {
	r, err := c.query(context.Background(), query, nil)
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()
	columns := r.Columns()
	values := make([]driver.Value, len(columns))
	var rows [][]driver.Value
	for {
		if err := r.Next(values); err != nil {
			if err == io.EOF {
				return columns, rows, nil
			}
			return nil, nil, err
		}
		row := make([]driver.Value, len(values))
		copy(row, values)
		rows = append(rows, row)
	}
}

func logicalHiddenRowidColumn(columns []string) string {
	for _, candidate := range []string{"_rowid_", "rowid", "oid"} {
		found := false
		for _, column := range columns {
			if strings.EqualFold(column, candidate) {
				found = true
				break
			}
		}
		if !found {
			return candidate
		}
	}
	return ""
}

func logicalDeserializeTable(c *conn, table logicalSerializedTable) error {
	columns := table.Columns
	if table.RowidColumn != "" {
		columns = append([]string{table.RowidColumn}, columns...)
	}
	insert := "INSERT INTO " + quoteIdent(table.Name) + " (" + quoteIdentList(columns) + ") VALUES (" + strings.TrimRight(strings.Repeat("?,", len(columns)), ",") + ")"
	for _, row := range table.Rows {
		if len(row) != len(columns) {
			return fmt.Errorf("sqlite: serialized table %s row has %d values for %d columns", table.Name, len(row), len(columns))
		}
		args := make([]driver.NamedValue, len(row))
		for i, value := range row {
			v, err := logicalValueToDriver(value)
			if err != nil {
				return err
			}
			args[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
		}
		if _, err := c.exec(context.Background(), insert, args); err != nil {
			return err
		}
	}
	return nil
}

func quoteIdentList(columns []string) string {
	out := make([]string, len(columns))
	for i, column := range columns {
		out[i] = quoteIdent(column)
	}
	return strings.Join(out, ",")
}

func logicalValueFromDriver(v driver.Value) (logicalSerializedValue, error) {
	switch x := v.(type) {
	case nil:
		return logicalSerializedValue{Type: "null"}, nil
	case int64:
		return logicalSerializedValue{Type: "int", Int: x}, nil
	case float64:
		return logicalSerializedValue{Type: "float", Float: x}, nil
	case string:
		return logicalSerializedValue{Type: "text", Text: x}, nil
	case []byte:
		return logicalSerializedValue{Type: "blob", Blob: append([]byte(nil), x...)}, nil
	case bool:
		return logicalSerializedValue{Type: "bool", Bool: x}, nil
	case time.Time:
		return logicalSerializedValue{Type: "time", Text: x.Format(time.RFC3339Nano)}, nil
	default:
		return logicalSerializedValue{}, fmt.Errorf("sqlite: serialize unsupported value type %T", v)
	}
}

func logicalValueToDriver(v logicalSerializedValue) (driver.Value, error) {
	switch v.Type {
	case "null":
		return nil, nil
	case "int":
		return v.Int, nil
	case "float":
		return v.Float, nil
	case "text":
		return v.Text, nil
	case "blob":
		return append([]byte(nil), v.Blob...), nil
	case "bool":
		return v.Bool, nil
	case "time":
		t, err := time.Parse(time.RFC3339Nano, v.Text)
		if err != nil {
			return nil, err
		}
		return t, nil
	default:
		return nil, fmt.Errorf("sqlite: deserialize unknown value type %q", v.Type)
	}
}
