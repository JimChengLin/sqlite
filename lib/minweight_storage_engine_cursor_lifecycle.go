// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"unsafe"

	"modernc.org/libc"
)

func minweightWriteResult(pRes BtreeMemoryHandle, v int32) {
	if !pRes.IsNil() {
		pRes.PutInt32(v)
	}
}

func (c *minweightCursor) closePayload(ctx BtreeContext) {
	if c.payloadBuf != 0 {
		Xsqlite3_free(ctx.tls, c.payloadBuf)
		c.payloadBuf = 0
	}
}

func (c *minweightCursor) current() (minweightRow, bool) {
	if !c.valid {
		return minweightRow{}, false
	}
	return c.row, true
}

func (c *minweightCursor) setCurrent(row minweightRow) {
	c.row = row
	c.valid = true
	c.dataVer = c.btree.visibleDataVer()
	c.hasLastRow = false
	c.incrblobInvalid = false
}

func (c *minweightCursor) clearCurrent() {
	c.valid = false
	c.row = minweightRow{}
	c.hasLastRow = false
}

func (c *minweightCursor) markCorrupt() {
	c.valid = false
	c.row = minweightRow{}
	c.hasLastRow = false
	c.faultCode = SQLITE_CORRUPT
}

func (e *minweightStorageEngine) invalidateIncrblobCursors(bt *minweightBtree, root uint32, rowid int64, clearTable bool) {
	if e.incrblobCursors.Load() == 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.incrblobCursors.Load() == 0 {
		return
	}
	for _, cur := range e.cursors {
		if !cur.incrblob || cur.incrblobInvalid || !cur.intKey {
			continue
		}
		if cur.btree.minweightDatabase != bt.minweightDatabase || cur.root != root {
			continue
		}
		if !clearTable {
			row, ok := cur.current()
			if !ok || row.rowid != rowid {
				continue
			}
		}
		cur.valid = false
		cur.hasLastRow = false
		cur.incrblobInvalid = true
	}
}

func (e *minweightStorageEngine) tripCursors(ctx BtreeContext, bt *minweightBtree, errCode int32, writeOnly int32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for ptr, cur := range e.cursors {
		if cur.btree != bt {
			continue
		}
		if writeOnly != 0 && !cur.writable {
			continue
		}
		cur.closePayload(ctx)
		cur.valid = false
		cur.hasLastRow = false
		cur.incrblobInvalid = false
		cur.faultCode = errCode
		raw := minweightBtCursorFromPointer(ptr)
		raw.FeState = uint8(CURSOR_FAULT)
		raw.FskipNext = errCode
	}
}

func (e *minweightStorageEngine) BtreeClearCursor(ctx BtreeContext, pCur BtreeCursorHandle) {
	cur := e.cursor(pCur)
	cur.clearCurrent()
}

func (e *minweightStorageEngine) BtreeCursorHasMoved(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	if pCur.IsNil() {
		return 0
	}
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return 1
	}
	if cur.incrblobInvalid {
		return 1
	}
	if cur.valid && cur.dataVer != cur.btree.visibleDataVer() {
		return 1
	}
	return 0
}

func (e *minweightStorageEngine) BtreeFakeValidCursor(ctx BtreeContext) (r BtreeCursorHandle) {
	return BtreeCursorHandle{}
}

func minweightRestoreIndexCursor(cur *minweightCursor, row minweightRow) (int32, error) {
	if !minweightIndexKeyVersionedForRoot(cur.root, row.storeKey) {
		return 0, minweightCorruptMetadata("index cursor restore missing versioned store key")
	}
	payload, ok, err := cur.btree.get(row.storeKey)
	if err != nil {
		return 0, err
	}
	if ok {
		cur.setCurrent(minweightRow{
			key:      append([]byte(nil), payload...),
			storeKey: append([]byte(nil), row.storeKey...),
			payload:  append([]byte(nil), payload...),
		})
		return 0, nil
	}
	next, ok, err := cur.btree.seekIndexGE(cur.root, row.storeKey, false)
	if err != nil {
		return 0, err
	}
	if ok {
		cur.setCurrent(next)
	} else {
		cur.clearCurrent()
		cur.lastRow = row
		cur.hasLastRow = true
	}
	return 1, nil
}

func (e *minweightStorageEngine) BtreeCursorRestore(ctx BtreeContext, pCur BtreeCursorHandle, pDifferentRow BtreeMemoryHandle) (r int32) {
	if pCur.IsNil() {
		minweightWriteResult(pDifferentRow, 0)
		return SQLITE_OK
	}
	cur := e.cursor(pCur)
	differentRow := int32(0)
	if cur.faultCode != SQLITE_OK {
		minweightWriteResult(pDifferentRow, 1)
		return cur.faultCode
	}
	if cur.incrblobInvalid {
		minweightWriteResult(pDifferentRow, 1)
		return SQLITE_OK
	}
	if cur.valid && cur.dataVer != cur.btree.visibleDataVer() {
		row, ok := cur.current()
		if !ok {
			cur.valid = false
			differentRow = 1
		} else if cur.intKey {
			payload, ok, err := cur.btree.get(minweightTableKey(cur.root, row.rowid))
			if err != nil {
				return minweightSQLiteError(err)
			}
			if ok {
				cur.setCurrent(minweightRow{rowid: row.rowid, storeKey: minweightTableKey(cur.root, row.rowid), payload: payload})
			} else {
				next, ok, err := cur.btree.seekTableGE(cur.root, row.rowid)
				if err != nil {
					return minweightSQLiteError(err)
				}
				if ok {
					cur.setCurrent(next)
				} else {
					cur.clearCurrent()
					cur.lastRow = row
					cur.hasLastRow = true
				}
				differentRow = 1
			}
		} else {
			moved, err := minweightRestoreIndexCursor(cur, row)
			if err != nil {
				return minweightSQLiteError(err)
			}
			differentRow = moved
		}
	}
	minweightWriteResult(pDifferentRow, differentRow)
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCursorHintFlags(ctx BtreeContext, pCur BtreeCursorHandle, x uint32) {
	minweightBtCursorFromPointer(pCur.ptr).Fhints = uint8(x)
}

func (e *minweightStorageEngine) BtreeCursor(ctx BtreeContext, p BtreeHandle, iTable uint32, wrFlag int32, pKeyInfo BtreeKeyInfoHandle, pCur BtreeCursorHandle) (r int32) {
	bt := e.btree(p)
	if wrFlag != 0 {
		if rc := bt.ensureWritable(); rc != SQLITE_OK {
			return rc
		}
	}
	readTracked := wrFlag == 0
	if readTracked {
		bt.retainReader()
	}
	table, ok := bt.visibleTable(iTable)
	if !ok {
		table = minweightTable{intKey: pKeyInfo.IsNil()}
		if wrFlag != 0 {
			bt.mu.Lock()
			bt.updateStateLocked(func(state *minweightDBState) {
				state.tables[iTable] = table
			})
			bt.mu.Unlock()
		}
	}
	rawCursor := minweightBtCursorFromPointer(pCur.ptr)
	*rawCursor = BtCursor{}
	rawCursor.FpBtree = p.ptr
	rawCursor.FpgnoRoot = TPgno(iTable)
	rawCursor.FcurIntKey = libc.Uint8FromInt32(libc.BoolInt32(table.intKey))
	rawCursor.FpKeyInfo = pKeyInfo.ptr
	rawCursor.FiPage = -1
	if wrFlag != 0 {
		rawCursor.FcurFlags |= uint8(BTCF_WriteFlag)
	}
	cur := &minweightCursor{
		btree:       bt,
		root:        iTable,
		intKey:      table.intKey,
		writable:    wrFlag != 0,
		readTracked: readTracked,
	}
	e.mu.Lock()
	rawCursor.FpBt = e.registerCursorLocked(pCur.ptr, cur)
	e.mu.Unlock()
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCursorSize(ctx BtreeContext) (r int32) {
	return int32(unsafe.Sizeof(BtCursor{}))
}

func (e *minweightStorageEngine) BtreeCursorZero(ctx BtreeContext, p BtreeCursorHandle) {
	*minweightBtCursorFromPointer(p.ptr) = BtCursor{}
}

func (e *minweightStorageEngine) BtreeCloseCursor(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	rawCursor := minweightBtCursorFromPointer(pCur.ptr)
	e.mu.Lock()
	cur := e.unregisterCursorLocked(pCur.ptr, rawCursor.FpBt)
	if cur != nil && cur.incrblob {
		e.incrblobCursors.Add(-1)
	}
	e.mu.Unlock()
	rawCursor.FpBt = 0
	if cur != nil {
		if cur.readTracked {
			cur.btree.releaseReader()
			cur.readTracked = false
		}
		cur.closePayload(ctx)
	}
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeCursorIsValidNN(ctx BtreeContext, pCur BtreeCursorHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode == SQLITE_OK && cur.valid {
		return 1
	}
	return 0
}

func (e *minweightStorageEngine) BtreeIntegerKey(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	row, ok := e.cursor(pCur).current()
	if !ok {
		return 0
	}
	return row.rowid
}

func (e *minweightStorageEngine) BtreeCursorPin(ctx BtreeContext, pCur BtreeCursorHandle) {
	minweightBtCursorFromPointer(pCur.ptr).FcurFlags |= uint8(BTCF_Pinned)
}

func (e *minweightStorageEngine) BtreeCursorUnpin(ctx BtreeContext, pCur BtreeCursorHandle) {
	minweightBtCursorFromPointer(pCur.ptr).FcurFlags &^= uint8(BTCF_Pinned)
}

func (e *minweightStorageEngine) BtreeOffset(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	return 0
}

func (e *minweightStorageEngine) BtreePayloadSize(ctx BtreeContext, pCur BtreeCursorHandle) (r uint32) {
	row, ok := e.cursor(pCur).current()
	if !ok {
		return 0
	}
	if e.cursor(pCur).intKey {
		return uint32(len(row.payload))
	}
	return uint32(len(row.key))
}

func (e *minweightStorageEngine) BtreeMaxRecordSize(ctx BtreeContext, pCur BtreeCursorHandle) (r int64) {
	return int64(e.BtreePayloadSize(ctx, pCur))
}

func (e *minweightStorageEngine) BtreePayload(ctx BtreeContext, pCur BtreeCursorHandle, offset uint32, amt uint32, pBuf BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	row, ok := cur.current()
	if !ok {
		return SQLITE_ERROR
	}
	data := row.payload
	if !cur.intKey {
		data = row.key
	}
	end := int(offset + amt)
	if int(offset) > len(data) || end > len(data) {
		return SQLITE_CORRUPT
	}
	pBuf.WriteBytes(data[offset:end])
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreePayloadChecked(ctx BtreeContext, pCur BtreeCursorHandle, offset uint32, amt uint32, pBuf BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	if cur.incrblobInvalid {
		return SQLITE_ABORT
	}
	return e.BtreePayload(ctx, pCur, offset, amt, pBuf)
}

func (e *minweightStorageEngine) BtreePayloadFetch(ctx BtreeContext, pCur BtreeCursorHandle, pAmt BtreeMemoryHandle) (r BtreeMemoryHandle) {
	cur := e.cursor(pCur)
	if cur.faultCode != SQLITE_OK {
		pAmt.PutInt32(0)
		return BtreeMemoryHandle{}
	}
	row, ok := cur.current()
	if !ok {
		pAmt.PutInt32(0)
		return BtreeMemoryHandle{}
	}
	data := row.payload
	if !cur.intKey {
		data = row.key
	}
	cur.closePayload(ctx)
	cur.payloadBuf = _sqlite3MallocZero(ctx.tls, uint64(len(data)+18))
	if len(data) != 0 {
		copy(minweightByteSliceFromPointer(cur.payloadBuf, len(data)), data)
	}
	pAmt.PutInt32(int32(len(data)))
	return btreeMemoryHandle(ctx.tls, cur.payloadBuf)
}

func (e *minweightStorageEngine) BtreePutData(ctx BtreeContext, pCsr BtreeCursorHandle, offset uint32, amt uint32, z BtreeMemoryHandle) (r int32) {
	cur := e.cursor(pCsr)
	if rc := cur.btree.ensureWritable(); rc != SQLITE_OK {
		return rc
	}
	if cur.faultCode != SQLITE_OK {
		return cur.faultCode
	}
	if cur.incrblobInvalid {
		return SQLITE_ABORT
	}
	row, ok := cur.current()
	if !ok || !cur.intKey {
		return SQLITE_ERROR
	}
	if !cur.writable {
		return SQLITE_READONLY
	}
	payload := append([]byte(nil), row.payload...)
	end := uint64(offset) + uint64(amt)
	if end > uint64(len(payload)) {
		return SQLITE_CORRUPT
	}
	copy(payload[int(offset):int(end)], z.ReadBytes(int(amt)))
	if err := cur.btree.put(minweightTableKey(cur.root, row.rowid), payload); err != nil {
		return minweightSQLiteError(err)
	}
	cur.btree.bumpDataVer()
	return SQLITE_OK
}

func (e *minweightStorageEngine) BtreeIncrblobCursor(ctx BtreeContext, pCur BtreeCursorHandle) {
	rawCursor := minweightBtCursorFromPointer(pCur.ptr)
	e.mu.Lock()
	var cur *minweightCursor
	if rawCursor.FpBt != 0 && int(rawCursor.FpBt) < len(e.cursorSlots) {
		cur = e.cursorSlots[rawCursor.FpBt]
	}
	if cur == nil {
		cur = e.cursors[pCur.ptr]
	}
	if cur == nil {
		e.mu.Unlock()
		panic("sqlite minweight storage engine: unknown cursor handle")
	}
	if !cur.incrblob {
		cur.incrblob = true
		e.incrblobCursors.Add(1)
	}
	e.mu.Unlock()
	rawCursor.FcurFlags |= uint8(BTCF_Incrblob)
}

func (e *minweightStorageEngine) BtreeCursorHasHint(ctx BtreeContext, pCsr BtreeCursorHandle, mask uint32) (r int32) {
	if uint32(minweightBtCursorFromPointer(pCsr.ptr).Fhints)&mask != 0 {
		return 1
	}
	return 0
}
