// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import (
	"bytes"
	"fmt"
	"sort"

	minweight "github.com/JimChengLin/minweight_store"
)

func (bt *minweightBtree) readGeneration() uint64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return bt.readGenerationLocked()
}

func (bt *minweightBtree) valueForKeyAtGeneration(key []byte, generation uint64) ([]byte, bool, error) {
	value, ok, err := bt.store.Get(key)
	if err != nil {
		return nil, false, err
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if tx := bt.activeTxnLocked(); tx != nil {
		if _, ok := tx.writes[string(key)]; ok {
			return nil, false, nil
		}
	}
	value, ok = bt.valueAtGenerationLocked(key, value, ok, generation)
	return value, ok, nil
}

func (bt *minweightBtree) valueForStoreItemAtGeneration(item minweight.Item, generation uint64) ([]byte, bool) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if tx := bt.activeTxnLocked(); tx != nil {
		if _, ok := tx.writes[string(item.Key)]; ok {
			return nil, false
		}
	}
	return bt.valueAtGenerationLocked(item.Key, item.Value, true, generation)
}

func (bt *minweightBtree) changedKeysAfterGeneration(generation uint64) [][]byte {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	var txWrites map[string]minweightTxnWrite
	if tx := bt.activeTxnLocked(); tx != nil {
		txWrites = tx.writes
	}
	seen := map[string]struct{}{}
	var keys [][]byte
	for _, change := range bt.changes {
		if change.generation <= generation {
			continue
		}
		for key, keyChange := range change.keys {
			if _, ok := seen[key]; ok {
				continue
			}
			if _, ok := txWrites[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			keys = append(keys, append([]byte(nil), keyChange.key...))
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i], keys[j]) < 0
	})
	return keys
}

func minweightIndexKeyMatchesTarget(key []byte, target []byte, ge bool, strict bool) bool {
	cmp := bytes.Compare(key, target)
	if ge {
		return cmp > 0 || !strict && cmp == 0
	}
	return cmp < 0 || !strict && cmp == 0
}

func (bt *minweightBtree) indexGenerationCandidate(root uint32, target []byte, ge bool, strict bool, generation uint64) (minweightRow, bool, error) {
	var best minweightRow
	found := false
	for _, key := range bt.changedKeysAfterGeneration(generation) {
		if !minweightIndexKeyInVersionedRange(root, key) || !minweightIndexKeyMatchesTarget(key, target, ge, strict) {
			continue
		}
		value, ok, err := bt.valueForKeyAtGeneration(key, generation)
		if err != nil {
			return minweightRow{}, false, err
		}
		if !ok {
			continue
		}
		row, ok := minweightIndexRowFromItem(root, minweight.Item{Key: key, Value: value})
		if !ok {
			return minweightRow{}, false, fmt.Errorf("minweight sqlite index key: corrupt versioned index key")
		}
		if ge {
			best, found = minweightBetterIndexGERow(row, true, best, found)
		} else {
			best, found = minweightBetterIndexLERow(row, true, best, found)
		}
	}
	return best, found, nil
}

func minweightTableKeyMatchesTarget(rowid int64, target int64, ge bool) bool {
	if ge {
		return rowid >= target
	}
	return rowid <= target
}

func (bt *minweightBtree) tableGenerationCandidate(root uint32, target int64, ge bool, generation uint64) (minweightRow, bool, error) {
	var best minweightRow
	found := false
	for _, key := range bt.changedKeysAfterGeneration(generation) {
		itemRoot, rowid, ok := minweightTableRootRowid(key)
		if !ok || itemRoot != root || !minweightTableKeyMatchesTarget(rowid, target, ge) {
			continue
		}
		value, ok, err := bt.valueForKeyAtGeneration(key, generation)
		if err != nil {
			return minweightRow{}, false, err
		}
		if !ok {
			continue
		}
		row := minweightRow{
			rowid:    rowid,
			storeKey: append([]byte(nil), key...),
			payload:  append([]byte(nil), value...),
		}
		if ge {
			best, found = minweightBetterTableGERow(row, true, best, found)
		} else {
			best, found = minweightBetterTableLERow(row, true, best, found)
		}
	}
	return best, found, nil
}
