// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"bytes"
	"errors"

	minweight "github.com/JimChengLin/minweight_store"
)

func minweightCopyState(src minweightDBState, dataVer uint32) minweightDBState {
	state := minweightCloneState(src)
	state.dataVer = dataVer
	return state
}

func minweightCopySourceItems(src *minweightBtree, writes map[string]minweightTxnWrite, put func(key, value []byte) error) error {
	var putErr error
	if err := src.store.Scan(func(item minweight.Item) bool {
		if bytes.Equal(item.Key, minweightMetaKey) {
			return true
		}
		if _, ok := writes[string(item.Key)]; ok {
			return true
		}
		putErr = put(item.Key, item.Value)
		return putErr == nil
	}); err != nil {
		return err
	}
	if putErr != nil {
		return putErr
	}
	for _, key := range minweightTxnWriteKeys(writes) {
		write := writes[key]
		if write.deleted || bytes.Equal(write.key, minweightMetaKey) {
			continue
		}
		if err := put(write.key, write.value); err != nil {
			return err
		}
	}
	return nil
}

func (bt *minweightBtree) setTxnWrite(write minweightTxnWrite) error {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	tx := bt.activeTxnLocked()
	if tx == nil {
		return errors.New("minweight sqlite transaction closed during copy")
	}
	tx.writes[string(write.key)] = write
	return nil
}

func (bt *minweightBtree) copyIntoActiveTxn(src *minweightBtree, writes map[string]minweightTxnWrite) error {
	var writeErr error
	if err := bt.store.Scan(func(item minweight.Item) bool {
		writeErr = bt.setTxnWrite(minweightTxnWrite{
			key:     append([]byte(nil), item.Key...),
			deleted: true,
		})
		return writeErr == nil
	}); err != nil {
		return err
	}
	if writeErr != nil {
		return writeErr
	}
	return minweightCopySourceItems(src, writes, func(key, value []byte) error {
		return bt.setTxnWrite(minweightTxnWrite{
			key:   append([]byte(nil), key...),
			value: append([]byte(nil), value...),
		})
	})
}

func (bt *minweightBtree) copyIntoMemoryStore(src *minweightBtree, state minweightDBState, writes map[string]minweightTxnWrite) error {
	store := minweight.New()
	if err := minweightCopySourceItems(src, writes, store.Put); err != nil {
		return err
	}
	bt.mu.Lock()
	bt.store = store
	bt.applyStateLocked(state)
	bt.mu.Unlock()
	return nil
}

func (bt *minweightBtree) copyIntoPathStore(src *minweightBtree, state minweightDBState, writes map[string]minweightTxnWrite) error {
	var batch minweight.WriteBatch
	var batchErr error
	if err := bt.store.Scan(func(item minweight.Item) bool {
		if batchErr = batch.Delete(item.Key); batchErr != nil {
			return false
		}
		return true
	}); err != nil {
		return err
	}
	if batchErr != nil {
		return batchErr
	}
	if err := minweightCopySourceItems(src, writes, batch.Put); err != nil {
		return err
	}
	if err := batch.Put(minweightMetaKey, minweightEncodeDatabaseState(state)); err != nil {
		return err
	}
	if err := bt.store.WriteBatch(batch); err != nil {
		return err
	}
	bt.mu.Lock()
	bt.applyStateLocked(state)
	bt.mu.Unlock()
	return nil
}

func (bt *minweightBtree) copyContentsFrom(src *minweightBtree) error {
	if bt == src {
		return nil
	}
	sourceState := src.visibleState()
	sourceWrites := src.txnWritesSnapshot()
	bt.mu.Lock()
	if tx := bt.activeTxnLocked(); tx != nil {
		state := minweightCopyState(sourceState, bt.visibleStateLocked().dataVer+1)
		tx.state = state
		tx.writes = map[string]minweightTxnWrite{}
		bt.mu.Unlock()
		return bt.copyIntoActiveTxn(src, sourceWrites)
	}
	state := minweightCopyState(sourceState, bt.dataVer+1)
	bt.mu.Unlock()
	if bt.path == "" {
		return bt.copyIntoMemoryStore(src, state, sourceWrites)
	}
	return bt.copyIntoPathStore(src, state, sourceWrites)
}
