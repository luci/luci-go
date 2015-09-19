// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memcache

import (
	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"
)

type memcacheImpl struct{ RawInterface }

var _ Interface = (*memcacheImpl)(nil)

func (m *memcacheImpl) Add(item Item) error {
	return errors.SingleError(m.AddMulti([]Item{item}))
}

func (m *memcacheImpl) Set(item Item) error {
	return errors.SingleError(m.SetMulti([]Item{item}))
}

func (m *memcacheImpl) Get(key string) (Item, error) {
	ret := m.NewItem(key)
	err := errors.SingleError(m.GetMulti([]Item{ret}))
	return ret, err
}

func (m *memcacheImpl) Delete(key string) error {
	return errors.SingleError(m.DeleteMulti([]string{key}))
}

func (m *memcacheImpl) CompareAndSwap(item Item) error {
	return errors.SingleError(m.CompareAndSwapMulti([]Item{item}))
}

func filterItems(lme errors.LazyMultiError, items []Item, nilErr error) ([]Item, []int) {
	idxMap := make([]int, 0, len(items))
	retItems := make([]Item, 0, len(items))
	for i, itm := range items {
		if itm != nil {
			idxMap = append(idxMap, i)
			retItems = append(retItems, itm)
		} else {
			lme.Assign(i, nilErr)
		}
	}
	return retItems, idxMap
}

func multiCall(items []Item, nilErr error, inner func(items []Item, cb RawCB) error) error {
	lme := errors.NewLazyMultiError(len(items))
	realItems, idxMap := filterItems(lme, items, nilErr)
	j := 0
	err := inner(realItems, func(err error) {
		lme.Assign(idxMap[j], err)
		j++
	})
	if err == nil {
		err = lme.Get()
	}
	return err
}

func (m *memcacheImpl) AddMulti(items []Item) error {
	return multiCall(items, ErrNotStored, m.RawInterface.AddMulti)
}

func (m *memcacheImpl) SetMulti(items []Item) error {
	return multiCall(items, ErrNotStored, m.RawInterface.SetMulti)
}

func (m *memcacheImpl) CompareAndSwapMulti(items []Item) error {
	return multiCall(items, ErrNotStored, m.RawInterface.CompareAndSwapMulti)
}

func (m *memcacheImpl) DeleteMulti(keys []string) error {
	lme := errors.NewLazyMultiError(len(keys))
	i := 0
	err := m.RawInterface.DeleteMulti(keys, func(err error) {
		lme.Assign(i, err)
		i++
	})
	if err == nil {
		err = lme.Get()
	}
	return err
}

func (m *memcacheImpl) GetMulti(items []Item) error {
	lme := errors.NewLazyMultiError(len(items))
	realItems, idxMap := filterItems(lme, items, ErrCacheMiss)
	if len(realItems) == 0 {
		return lme.Get()
	}

	keys := make([]string, len(realItems))
	for i, itm := range realItems {
		keys[i] = itm.Key()
	}

	j := 0
	err := m.RawInterface.GetMulti(keys, func(item Item, err error) {
		i := idxMap[j]
		if !lme.Assign(i, err) {
			items[i].SetAll(item)
		}
		j++
	})
	if err == nil {
		err = lme.Get()
	}
	return err
}

func (m *memcacheImpl) Increment(key string, delta int64, initialValue uint64) (newValue uint64, err error) {
	return m.RawInterface.Increment(key, delta, &initialValue)
}

func (m *memcacheImpl) IncrementExisting(key string, delta int64) (newValue uint64, err error) {
	return m.RawInterface.Increment(key, delta, nil)
}

func (m *memcacheImpl) Raw() RawInterface { return m.RawInterface }

// Get gets the current memcache implementation from the context.
func Get(c context.Context) Interface {
	return &memcacheImpl{GetRaw(c)}
}
