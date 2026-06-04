// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	minweight "github.com/JimChengLin/minweight_store"
)

type minweightIntegrityStats struct {
	rowCount int64
	minRowid int64
	maxRowid int64
}

type minweightIntegrityCheck struct {
	bt        *minweightBtree
	state     minweightDBState
	writes    map[string]minweightTxnWrite
	roots     []uint32
	partial   bool
	selected  map[uint32]bool
	rootOrder []uint32
	stats     map[uint32]minweightIntegrityStats
	errors    []string
	mxErr     int32
}

func (e *minweightStorageEngine) BtreeIntegrityCheck(ctx BtreeContext, db SQLiteHandle, p BtreeHandle, aRoot BtreeMemoryHandle, aCnt BtreeMemoryHandle, nRoot int32, mxErr int32, pnErr BtreeMemoryHandle, pzOut BtreeMemoryHandle) (r int32) {
	if !pnErr.IsNil() {
		pnErr.PutInt32(0)
	}
	if !pzOut.IsNil() {
		pzOut.PutUintptr(0)
	}
	if mxErr <= 0 {
		return SQLITE_OK
	}
	check := newMinweightIntegrityCheck(e.btree(p), aRoot, nRoot, mxErr)
	if err := check.run(); err != nil {
		return minweightSQLiteError(err)
	}
	check.writeCounts(ctx, aCnt)
	if len(check.errors) == 0 {
		return SQLITE_OK
	}
	if !pnErr.IsNil() {
		pnErr.PutInt32(int32(len(check.errors)))
	}
	if !pzOut.IsNil() {
		out := minweightAllocCString(ctx, strings.Join(check.errors, "\n"))
		if out == 0 {
			return SQLITE_NOMEM
		}
		pzOut.PutUintptr(out)
	}
	return SQLITE_OK
}

func newMinweightIntegrityCheck(bt *minweightBtree, aRoot BtreeMemoryHandle, nRoot int32, mxErr int32) *minweightIntegrityCheck {
	roots, partial := minweightIntegrityRoots(aRoot, nRoot)
	selected := minweightIntegritySelectedRoots(roots, partial)
	return &minweightIntegrityCheck{
		bt:        bt,
		state:     bt.visibleState(),
		writes:    bt.txnWritesSnapshot(),
		roots:     roots,
		partial:   partial,
		selected:  selected,
		rootOrder: minweightIntegrityRootOrder(roots, partial),
		stats:     map[uint32]minweightIntegrityStats{},
		mxErr:     mxErr,
	}
}

func (c *minweightIntegrityCheck) run() error {
	if err := c.scanVisibleItems(); err != nil {
		return err
	}
	c.validateTableMetadata()
	return nil
}

func (c *minweightIntegrityCheck) scanVisibleItems() error {
	if c.partial {
		for _, root := range c.rootOrder {
			if err := c.scanRoot(root); err != nil {
				return err
			}
			if c.done() {
				return nil
			}
		}
		return nil
	}
	if err := c.scanCommittedAll(); err != nil {
		return err
	}
	if c.done() {
		return nil
	}
	c.scanOverlay(nil, nil)
	return nil
}

func (c *minweightIntegrityCheck) scanRoot(root uint32) error {
	for _, intKey := range []bool{true, false} {
		lower := minweightRootPrefix(root, intKey)
		upper := minweightPrefixUpper(lower)
		if err := c.scanCommittedRange(lower, upper); err != nil {
			return err
		}
		if c.done() {
			return nil
		}
		c.scanOverlay(lower, upper)
		if c.done() {
			return nil
		}
	}
	return nil
}

func (c *minweightIntegrityCheck) scanCommittedAll() error {
	return c.bt.store.Scan(func(item minweight.Item) bool {
		if !c.writeShadows(item.Key) {
			c.visitKey(item.Key)
		}
		return !c.done()
	})
}

func (c *minweightIntegrityCheck) scanCommittedRange(lower []byte, upper []byte) error {
	return c.bt.store.ScanRange(lower, upper, func(item minweight.Item) bool {
		if !c.writeShadows(item.Key) {
			c.visitKey(item.Key)
		}
		return !c.done()
	})
}

func (c *minweightIntegrityCheck) scanOverlay(lower []byte, upper []byte) {
	for _, key := range minweightTxnWriteKeys(c.writes) {
		write := c.writes[key]
		if write.deleted || !minweightKeyInRange(write.key, lower, upper) {
			continue
		}
		c.visitKey(write.key)
		if c.done() {
			return
		}
	}
}

func (c *minweightIntegrityCheck) visitKey(key []byte) {
	if len(key) == 0 {
		if !c.partial {
			c.addError("minweight malformed empty key")
		}
		return
	}
	switch key[0] {
	case minweightTablePrefix:
		c.visitTableKey(key)
	case minweightIndexPrefix:
		c.visitIndexKey(key)
	default:
		if !c.partial {
			c.addError("minweight unknown key prefix 0x%02x", key[0])
		}
	}
}

func (c *minweightIntegrityCheck) visitTableKey(key []byte) {
	if len(key) < 5 {
		if !c.partial {
			c.addError("minweight malformed table key length %d", len(key))
		}
		return
	}
	root := binary.BigEndian.Uint32(key[1:5])
	if !c.rootChecked(root) {
		return
	}
	if len(key) != 13 {
		c.addError("minweight malformed table key length %d", len(key))
		return
	}
	table, ok := c.state.tables[root]
	if !ok {
		c.addError("minweight table key references unknown root %d", root)
		return
	}
	if !table.intKey {
		c.addError("minweight root %d has table key in index btree", root)
		return
	}
	u := binary.BigEndian.Uint64(key[5:13]) ^ (1 << 63)
	minweightAddIntegrityRowid(c.stats, root, int64(u))
}

func (c *minweightIntegrityCheck) visitIndexKey(key []byte) {
	if len(key) < 5 {
		if !c.partial {
			c.addError("minweight malformed index key length %d", len(key))
		}
		return
	}
	root := binary.BigEndian.Uint32(key[1:5])
	if !c.rootChecked(root) {
		return
	}
	if !minweightIndexKeyInVersionedRange(root, key) {
		c.addError("minweight unsupported raw index key in root %d", root)
		return
	}
	table, ok := c.state.tables[root]
	if !ok {
		c.addError("minweight index key references unknown root %d", root)
		return
	}
	if table.intKey {
		c.addError("minweight root %d has index key in table btree", root)
		return
	}
	minweightAddIntegrityIndexRow(c.stats, root)
}

func (c *minweightIntegrityCheck) validateTableMetadata() {
	for _, root := range minweightIntegrityTableRoots(c.state.tables) {
		if c.done() || !c.rootChecked(root) {
			continue
		}
		c.validateTableRoot(root, c.state.tables[root])
	}
}

func (c *minweightIntegrityCheck) validateTableRoot(root uint32, table minweightTable) {
	stat := c.stats[root]
	switch {
	case root == 0:
		c.addError("minweight metadata contains root 0")
	case root > c.state.next:
		c.addError("minweight root %d is greater than largest root %d", root, c.state.next)
	case table.rowCount < 0:
		c.addError("minweight root %d has negative row count %d", root, table.rowCount)
	case table.rowCount != stat.rowCount:
		c.addError("minweight root %d row count metadata %d != actual %d", root, table.rowCount, stat.rowCount)
	case table.intKey && stat.rowCount == 0 && (table.minRowid != 0 || table.maxRowid != 0):
		c.addError("minweight root %d empty table has rowid bounds %d..%d", root, table.minRowid, table.maxRowid)
	case table.intKey && stat.rowCount != 0 && (table.minRowid != stat.minRowid || table.maxRowid != stat.maxRowid):
		c.addError("minweight root %d rowid bounds metadata %d..%d != actual %d..%d", root, table.minRowid, table.maxRowid, stat.minRowid, stat.maxRowid)
	}
}

func (c *minweightIntegrityCheck) writeCounts(ctx BtreeContext, aCnt BtreeMemoryHandle) {
	if aCnt.IsNil() {
		return
	}
	for i, root := range c.roots {
		var rowCount int64
		if root != 0 {
			rowCount = c.stats[root].rowCount
		}
		_sqlite3MemSetArrayInt64(ctx.tls, aCnt.ptr, int32(i), Ti64(rowCount))
	}
}

func (c *minweightIntegrityCheck) rootChecked(root uint32) bool {
	return minweightIntegrityRootChecked(root, c.partial, c.selected)
}

func (c *minweightIntegrityCheck) writeShadows(key []byte) bool {
	if len(c.writes) == 0 {
		return false
	}
	_, ok := c.writes[string(key)]
	return ok
}

func (c *minweightIntegrityCheck) addError(format string, args ...any) {
	minweightAddIntegrityError(&c.errors, c.mxErr, format, args...)
}

func (c *minweightIntegrityCheck) done() bool {
	return int32(len(c.errors)) >= c.mxErr
}

func minweightIntegrityRoots(aRoot BtreeMemoryHandle, nRoot int32) ([]uint32, bool) {
	if aRoot.IsNil() || nRoot <= 0 {
		return nil, false
	}
	roots := make([]uint32, nRoot)
	rootBytes := aRoot.ReadBytes(int(nRoot) * 4)
	for i := range roots {
		roots[i] = binary.NativeEndian.Uint32(rootBytes[i*4 : i*4+4])
	}
	return roots, roots[0] == 0
}

func minweightIntegritySelectedRoots(roots []uint32, partial bool) map[uint32]bool {
	if len(roots) == 0 {
		return nil
	}
	selected := make(map[uint32]bool, len(roots))
	start := 0
	if partial {
		start = 1
	}
	for _, root := range roots[start:] {
		if root != 0 {
			selected[root] = true
		}
	}
	return selected
}

func minweightIntegrityRootOrder(roots []uint32, partial bool) []uint32 {
	if !partial || len(roots) <= 1 {
		return nil
	}
	seen := map[uint32]bool{}
	order := make([]uint32, 0, len(roots)-1)
	for _, root := range roots[1:] {
		if root == 0 || seen[root] {
			continue
		}
		seen[root] = true
		order = append(order, root)
	}
	return order
}

func minweightIntegrityRootChecked(root uint32, partial bool, selected map[uint32]bool) bool {
	if !partial {
		return true
	}
	return selected[root]
}

func minweightIntegrityTableRoots(tables map[uint32]minweightTable) []uint32 {
	roots := make([]uint32, 0, len(tables))
	for root := range tables {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i] < roots[j] })
	return roots
}

func minweightAddIntegrityError(errors *[]string, mxErr int32, format string, args ...any) bool {
	if mxErr <= 0 || int32(len(*errors)) >= mxErr {
		return false
	}
	*errors = append(*errors, fmt.Sprintf(format, args...))
	return int32(len(*errors)) < mxErr
}

func minweightAddIntegrityRowid(stats map[uint32]minweightIntegrityStats, root uint32, rowid int64) {
	stat := stats[root]
	if stat.rowCount == 0 {
		stat.minRowid = rowid
		stat.maxRowid = rowid
	} else {
		if rowid < stat.minRowid {
			stat.minRowid = rowid
		}
		if rowid > stat.maxRowid {
			stat.maxRowid = rowid
		}
	}
	stat.rowCount++
	stats[root] = stat
}

func minweightAddIntegrityIndexRow(stats map[uint32]minweightIntegrityStats, root uint32) {
	stat := stats[root]
	stat.rowCount++
	stats[root] = stat
}

func minweightPrefixUpper(prefix []byte) []byte {
	upper := append([]byte(nil), prefix...)
	for i := len(upper) - 1; i >= 0; i-- {
		if upper[i] != 0xff {
			upper[i]++
			return upper[:i+1]
		}
	}
	return nil
}

func minweightKeyInRange(key []byte, lower []byte, upper []byte) bool {
	if lower != nil && bytes.Compare(key, lower) < 0 {
		return false
	}
	return upper == nil || bytes.Compare(key, upper) < 0
}
