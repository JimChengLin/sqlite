// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"bytes"
	"math"
)

func minweightCompareIndexKey(ctx BtreeContext, key []byte, pIdxKey uintptr) (int32, int32) {
	if len(key) == 0 {
		return 0, SQLITE_CORRUPT
	}
	buf := _sqlite3MallocZero(ctx.tls, uint64(len(key)+18))
	if buf == 0 {
		return 0, SQLITE_NOMEM
	}
	copy(minweightByteSliceFromPointer(buf, len(key)), key)
	cmp := _sqlite3VdbeRecordCompare(ctx.tls, int32(len(key)), buf, pIdxKey)
	Xsqlite3_free(ctx.tls, buf)
	if minweightUnpackedRecordFromPointer(pIdxKey).FerrCode != uint8(SQLITE_OK) {
		return cmp, SQLITE_CORRUPT
	}
	return cmp, SQLITE_OK
}

func minweightCompareIndexStoreKey(storeKey []byte, probeKey []byte, rec *TUnpackedRecord) (int32, bool) {
	if rec.Fdefault_rc < 0 {
		return 0, false
	}
	if bytes.HasPrefix(storeKey, probeKey) {
		rec.FeqSeen = uint8(1)
		return int32(rec.Fdefault_rc), true
	}
	if bytes.Compare(storeKey, probeKey) > 0 {
		return 1, true
	}
	return 0, false
}

func minweightIndexProbeIsFull(rec *TUnpackedRecord) bool {
	if rec.FpKeyInfo == 0 {
		return false
	}
	return int(rec.FnField) == int(minweightKeyInfoFromPointer(rec.FpKeyInfo).FnAllField)
}

func (e *minweightStorageEngine) BtreeFirst(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	if cur.intKey {
		table, _ := cur.btree.visibleTable(cur.root)
		if table.rowCount == 0 {
			cur.clearCurrent()
			minweightWriteResult(pRes, 1)
			return SQLITE_OK
		}
		payload, ok, err := cur.btree.get(minweightTableKey(cur.root, table.minRowid))
		if err != nil {
			return minweightSQLiteError(err)
		}
		if !ok {
			return SQLITE_CORRUPT
		}
		cur.setCurrent(minweightRow{rowid: table.minRowid, storeKey: minweightTableKey(cur.root, table.minRowid), payload: payload})
		minweightWriteResult(pRes, 0)
		return SQLITE_OK
	}
	row, ok, err := cur.btree.seekIndexGE(cur.root, minweightVersionedIndexLower(cur.root), false)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if ok {
		cur.setCurrent(row)
		minweightWriteResult(pRes, 0)
		return SQLITE_OK
	}
	table, _ := cur.btree.visibleTable(cur.root)
	if table.rowCount == 0 {
		cur.clearCurrent()
		minweightWriteResult(pRes, 1)
		return SQLITE_OK
	}
	return SQLITE_CORRUPT
}

func (e *minweightStorageEngine) BtreeLast(ctx BtreeContext, pCur BtreeCursorHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	if cur.intKey {
		table, _ := cur.btree.visibleTable(cur.root)
		if table.rowCount == 0 {
			cur.clearCurrent()
			minweightWriteResult(pRes, 1)
			return SQLITE_OK
		}
		payload, ok, err := cur.btree.get(minweightTableKey(cur.root, table.maxRowid))
		if err != nil {
			return minweightSQLiteError(err)
		}
		if !ok {
			return SQLITE_CORRUPT
		}
		cur.setCurrent(minweightRow{rowid: table.maxRowid, storeKey: minweightTableKey(cur.root, table.maxRowid), payload: payload})
		minweightWriteResult(pRes, 0)
		return SQLITE_OK
	}
	row, ok, err := cur.btree.seekIndexLE(cur.root, minweightVersionedIndexUpper(cur.root), false)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if ok {
		cur.setCurrent(row)
		minweightWriteResult(pRes, 0)
		return SQLITE_OK
	}
	table, _ := cur.btree.visibleTable(cur.root)
	if table.rowCount == 0 {
		cur.clearCurrent()
		minweightWriteResult(pRes, 1)
		return SQLITE_OK
	}
	return SQLITE_CORRUPT
}

func (e *minweightStorageEngine) BtreeTableMoveto(ctx BtreeContext, pCur BtreeCursorHandle, intKey int64, biasRight int32, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	if !cur.readTracked {
		table, dataVer, ok := cur.btree.visibleTableAndDataVer(cur.root)
		if ok && table.intKey && (table.rowCount == 0 || intKey > table.maxRowid) {
			cur.clearCurrent()
			cur.lastRow = minweightRow{rowid: intKey}
			cur.hasLastRow = true
			cur.dataVer = dataVer
			minweightWriteResult(pRes, -1)
			return SQLITE_OK
		}
		var key [13]byte
		minweightTableKeyInto(key[:], cur.root, intKey)
		payload, exact, err := cur.btree.get(key[:])
		if err != nil {
			return minweightSQLiteError(err)
		}
		if exact {
			cur.setCurrent(minweightRow{rowid: intKey, payload: payload})
			minweightWriteResult(pRes, 0)
			return SQLITE_OK
		}
	}
	row, ok, err := cur.btree.seekTableGE(cur.root, intKey)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if ok {
		cur.setCurrent(row)
		if row.rowid == intKey {
			minweightWriteResult(pRes, 0)
		} else {
			minweightWriteResult(pRes, 1)
		}
		return SQLITE_OK
	}
	cur.clearCurrent()
	cur.lastRow = minweightRow{rowid: intKey}
	cur.hasLastRow = true
	cur.dataVer = cur.btree.visibleDataVer()
	minweightWriteResult(pRes, -1)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeIndexMoveto(ctx BtreeContext, pCur BtreeCursorHandle, pIdxKey BtreeIndexKeyHandle, pRes BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	rec := minweightUnpackedRecordFromPointer(pIdxKey.ptr)
	rec.FerrCode = uint8(SQLITE_OK)
	rec.FeqSeen = uint8(0)
	probeKey, err := minweightComparableIndexProbeKey(ctx, cur.root, pIdxKey.ptr)
	if err != nil {
		return minweightSQLiteError(err)
	}
	beyondMax := cur.btree.indexProbeBeyondMax(cur.root, probeKey)
	if minweightIndexProbeIsFull(rec) && rec.Fdefault_rc >= 0 {
		payload, exact, err := cur.btree.get(probeKey)
		if err != nil {
			return minweightSQLiteError(err)
		}
		if exact {
			rec.FeqSeen = uint8(1)
			cmp := int32(rec.Fdefault_rc)
			cur.setCurrent(minweightRow{
				key:      payload,
				storeKey: probeKey,
				payload:  payload,
			})
			minweightWriteResult(pRes, cmp)
			return SQLITE_OK
		}
	}
	if beyondMax {
		cur.clearCurrent()
		minweightWriteResult(pRes, -1)
		return SQLITE_OK
	}
	for {
		row, ok, err := cur.btree.seekIndexGE(cur.root, probeKey, false)
		if err != nil {
			return minweightSQLiteError(err)
		}
		if !ok {
			cur.clearCurrent()
			minweightWriteResult(pRes, -1)
			return SQLITE_OK
		}
		cmp, fast := minweightCompareIndexStoreKey(row.storeKey, probeKey, rec)
		if !fast {
			var rc int32
			cmp, rc = minweightCompareIndexKey(ctx, row.key, pIdxKey.ptr)
			if rc != SQLITE_OK {
				return rc
			}
		}
		if cmp >= 0 {
			cur.setCurrent(row)
			minweightWriteResult(pRes, cmp)
			return SQLITE_OK
		}
		probeKey = minweightIndexSeekAfter(row.storeKey)
	}
}

func (e *minweightStorageEngine) BtreeEof(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return 1
	}
	if cur.valid {
		return 0
	}
	if cur.intKey {
		if cur.hasLastRow && cur.dataVer != cur.btree.visibleDataVer() && cur.lastRow.rowid != math.MaxInt64 {
			row, ok, err := cur.btree.seekTableGE(cur.root, cur.lastRow.rowid+1)
			if err != nil {
				return 1
			}
			if ok {
				cur.setCurrent(row)
				return 0
			}
		}
		return 1
	}
	if cur.hasLastRow {
		if !minweightIndexKeyVersionedForRoot(cur.root, cur.lastRow.storeKey) {
			cur.markCorrupt()
			return 1
		}
		row, ok, err := cur.btree.seekIndexGE(cur.root, cur.lastRow.storeKey, true)
		if err != nil {
			cur.markCorrupt()
			return 1
		}
		if ok {
			cur.setCurrent(row)
			return 0
		}
		return 1
	}
	return 1
}

func (e *minweightStorageEngine) BtreeRowCountEst(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	cur := e.cursor(pCur)
	table, _ := cur.btree.visibleTable(cur.root)
	rowCount := table.rowCount
	return rowCount
}

func (e *minweightStorageEngine) BtreeNext(ctx BtreeContext, pCur BtreeCursorHandle, flags int32) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	if cur.intKey {
		return minweightCursorNextTable(cur)
	}
	return minweightCursorNextIndex(cur)
}

func minweightCursorNextTable(cur *minweightCursor) int32 {
	if !cur.valid {
		return minweightCursorNextTableFromLastRow(cur)
	}
	row, ok := cur.current()
	if !ok {
		return SQLITE_DONE
	}
	if row.rowid == math.MaxInt64 {
		cur.valid = false
		cur.lastRow = row
		cur.hasLastRow = true
		return SQLITE_DONE
	}
	next, ok, err := cur.btree.seekTableGE(cur.root, row.rowid+1)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if !ok {
		cur.valid = false
		cur.lastRow = row
		cur.hasLastRow = true
		return SQLITE_DONE
	}
	cur.setCurrent(next)
	return SQLITE_OK
}

func minweightCursorNextTableFromLastRow(cur *minweightCursor) int32 {
	if !cur.hasLastRow || cur.lastRow.rowid == math.MaxInt64 {
		return SQLITE_DONE
	}
	row, ok, err := cur.btree.seekTableGE(cur.root, cur.lastRow.rowid+1)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if !ok {
		return SQLITE_DONE
	}
	cur.setCurrent(row)
	return SQLITE_OK
}

func minweightCursorNextIndex(cur *minweightCursor) int32 {
	if !cur.valid {
		return minweightCursorNextIndexFromLastRow(cur)
	}
	row, ok := cur.current()
	if ok && minweightIndexKeyVersionedForRoot(cur.root, row.storeKey) {
		next, nextOK, err := cur.btree.seekIndexGE(cur.root, row.storeKey, true)
		if err != nil {
			return minweightSQLiteError(err)
		}
		if !nextOK {
			cur.valid = false
			cur.lastRow = row
			cur.hasLastRow = true
			return SQLITE_DONE
		}
		cur.setCurrent(next)
		return SQLITE_OK
	}
	cur.markCorrupt()
	return SQLITE_CORRUPT
}

func minweightCursorNextIndexFromLastRow(cur *minweightCursor) int32 {
	if !cur.hasLastRow {
		return SQLITE_DONE
	}
	if !minweightIndexKeyVersionedForRoot(cur.root, cur.lastRow.storeKey) {
		cur.markCorrupt()
		return SQLITE_CORRUPT
	}
	row, ok, err := cur.btree.seekIndexGE(cur.root, cur.lastRow.storeKey, true)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if !ok {
		return SQLITE_DONE
	}
	cur.setCurrent(row)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreePrevious(ctx BtreeContext, pCur BtreeCursorHandle, flags int32) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	if cur.intKey {
		return minweightCursorPreviousTable(cur)
	}
	return minweightCursorPreviousIndex(cur)
}

func minweightCursorPreviousTable(cur *minweightCursor) int32 {
	if !cur.valid {
		return minweightCursorPreviousTableFromLastRow(cur)
	}
	row, ok := cur.current()
	if !ok {
		return SQLITE_DONE
	}
	if row.rowid == math.MinInt64 {
		cur.valid = false
		cur.lastRow = row
		cur.hasLastRow = true
		return SQLITE_DONE
	}
	prev, ok, err := cur.btree.seekTableLE(cur.root, row.rowid-1)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if !ok {
		cur.valid = false
		cur.lastRow = row
		cur.hasLastRow = true
		return SQLITE_DONE
	}
	cur.setCurrent(prev)
	return SQLITE_OK
}

func minweightCursorPreviousTableFromLastRow(cur *minweightCursor) int32 {
	if !cur.hasLastRow || cur.lastRow.rowid == math.MinInt64 {
		return SQLITE_DONE
	}
	row, ok, err := cur.btree.seekTableLE(cur.root, cur.lastRow.rowid-1)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if !ok {
		return SQLITE_DONE
	}
	cur.setCurrent(row)
	return SQLITE_OK
}

func minweightCursorPreviousIndex(cur *minweightCursor) int32 {
	if !cur.valid {
		return minweightCursorPreviousIndexFromLastRow(cur)
	}
	row, ok := cur.current()
	if ok && minweightIndexKeyVersionedForRoot(cur.root, row.storeKey) {
		prev, prevOK, err := cur.btree.seekIndexLE(cur.root, row.storeKey, true)
		if err != nil {
			return minweightSQLiteError(err)
		}
		if !prevOK {
			cur.valid = false
			cur.lastRow = row
			cur.hasLastRow = true
			return SQLITE_DONE
		}
		cur.setCurrent(prev)
		return SQLITE_OK
	}
	cur.markCorrupt()
	return SQLITE_CORRUPT
}

func minweightCursorPreviousIndexFromLastRow(cur *minweightCursor) int32 {
	if !cur.hasLastRow {
		return SQLITE_DONE
	}
	if !minweightIndexKeyVersionedForRoot(cur.root, cur.lastRow.storeKey) {
		cur.markCorrupt()
		return SQLITE_CORRUPT
	}
	row, ok, err := cur.btree.seekIndexLE(cur.root, cur.lastRow.storeKey, true)
	if err != nil {
		return minweightSQLiteError(err)
	}
	if !ok {
		return SQLITE_DONE
	}
	cur.setCurrent(row)
	return SQLITE_OK
}
