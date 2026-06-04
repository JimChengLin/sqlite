// Copyright 2026 The Sqlite Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (darwin && (amd64 || arm64)) || (linux && (amd64 || arm64 || loong64 || ppc64le || riscv64 || s390x))

package sqlite3

import "math"

func (bt *minweightBtree) clearRoot(root uint32, intKey bool) (int, error) {
	if intKey {
		return bt.clearIntKeyRoot(root)
	}
	return bt.clearVersionedIndexRoot(root)
}

func (bt *minweightBtree) clearIntKeyRoot(root uint32) (int, error) {
	var n int
	row, ok, err := bt.seekTableGE(root, math.MinInt64)
	if err != nil {
		return 0, err
	}
	for ok {
		if _, err := bt.delete(minweightTableKey(root, row.rowid)); err != nil {
			return n, err
		}
		n++
		if row.rowid == math.MaxInt64 {
			break
		}
		row, ok, err = bt.seekTableGE(root, row.rowid+1)
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func (bt *minweightBtree) clearVersionedIndexRoot(root uint32) (int, error) {
	var n int
	row, ok, err := bt.seekIndexGE(root, minweightVersionedIndexLower(root), false)
	if err != nil {
		return 0, err
	}
	for ok {
		key := append([]byte(nil), row.storeKey...)
		if _, err := bt.delete(key); err != nil {
			return n, err
		}
		n++
		row, ok, err = bt.seekIndexGE(root, minweightIndexSeekAfter(key), false)
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func (bt *minweightBtree) moveRoot(from uint32, to uint32, intKey bool) error {
	if intKey {
		return bt.moveIntKeyRoot(from, to)
	}
	return bt.moveVersionedIndexRoot(from, to)
}

func (bt *minweightBtree) moveIntKeyRoot(from uint32, to uint32) error {
	row, ok, err := bt.seekTableGE(from, math.MinInt64)
	if err != nil {
		return err
	}
	for ok {
		if err := bt.put(minweightTableKey(to, row.rowid), row.payload); err != nil {
			return err
		}
		if _, err := bt.delete(minweightTableKey(from, row.rowid)); err != nil {
			return err
		}
		if row.rowid == math.MaxInt64 {
			return nil
		}
		row, ok, err = bt.seekTableGE(from, row.rowid+1)
		if err != nil {
			return err
		}
	}
	return nil
}

func (bt *minweightBtree) moveVersionedIndexRoot(from uint32, to uint32) error {
	row, ok, err := bt.seekIndexGE(from, minweightVersionedIndexLower(from), false)
	if err != nil {
		return err
	}
	for ok {
		oldKey := append([]byte(nil), row.storeKey...)
		if err := bt.put(minweightMoveIndexStoreKey(to, row), row.payload); err != nil {
			return err
		}
		if _, err := bt.delete(oldKey); err != nil {
			return err
		}
		row, ok, err = bt.seekIndexGE(from, minweightIndexSeekAfter(oldKey), false)
		if err != nil {
			return err
		}
	}
	return nil
}
