// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"infra/gae/libs/wrapper"
	"infra/gae/libs/wrapper/gae/commonErrors"
	"infra/gae/libs/wrapper/unsafe"
	"infra/libs/clock"
	"sync"
	"time"

	"golang.org/x/net/context"

	"appengine/memcache"
)

type memcacheData struct {
	wrapper.BrokenFeatures

	lock  sync.Mutex
	items map[string]*unsafe.Item
	casID uint64
}

// memcacheImpl binds the current connection's memcache data to an
// implementation of {wrapper.Memcache, wrapper.Testable}.
type memcacheImpl struct {
	wrapper.Memcache

	data *memcacheData
	ctx  context.Context
}

var (
	_ = wrapper.Memcache((*memcacheImpl)(nil))
	_ = wrapper.Testable((*memcacheImpl)(nil))
)

// useMC adds a wrapper.Memcache implementation to context, accessible
// by wrapper.GetMC(c)
func useMC(c context.Context) context.Context {
	lck := sync.Mutex{}
	mcdMap := map[string]*memcacheData{}

	return wrapper.SetMCFactory(c, func(ic context.Context) wrapper.Memcache {
		lck.Lock()
		defer lck.Unlock()

		ns := curGID(ic).namespace
		mcd, ok := mcdMap[ns]
		if !ok {
			mcd = &memcacheData{
				BrokenFeatures: wrapper.BrokenFeatures{
					DefaultError: commonErrors.ErrServerErrorMC},
				items: map[string]*unsafe.Item{}}
			mcdMap[ns] = mcd
		}

		return &memcacheImpl{
			wrapper.DummyMC(),
			mcd,
			ic,
		}
	})
}

func (m *memcacheImpl) mkItemLocked(i *memcache.Item) *unsafe.Item {
	m.data.casID++
	var exp time.Duration
	if i.Expiration != 0 {
		exp = time.Duration(clock.Now(m.ctx).Add(i.Expiration).UnixNano())
	}
	newItem := unsafe.Item{
		Key:        i.Key,
		Value:      make([]byte, len(i.Value)),
		Flags:      i.Flags,
		Expiration: exp,
		CasID:      m.data.casID,
	}
	copy(newItem.Value, i.Value)
	return &newItem
}

func copyBack(i *unsafe.Item) *memcache.Item {
	ret := &memcache.Item{
		Key:   i.Key,
		Value: make([]byte, len(i.Value)),
		Flags: i.Flags,
	}
	copy(ret.Value, i.Value)
	unsafe.MCSetCasID(ret, i.CasID)

	return ret
}

func (m *memcacheImpl) retrieve(key string) (*unsafe.Item, bool) {
	ret, ok := m.data.items[key]
	if ok && ret.Expiration != 0 && ret.Expiration < time.Duration(clock.Now(m.ctx).UnixNano()) {
		ret = nil
		ok = false
		delete(m.data.items, key)
	}
	return ret, ok
}

func (m *memcacheImpl) BreakFeatures(err error, features ...string) {
	m.data.BreakFeatures(err, features...)
}

func (m *memcacheImpl) UnbreakFeatures(features ...string) {
	m.data.UnbreakFeatures(features...)
}

// Add implements context.MCSingleReadWriter.Add.
func (m *memcacheImpl) Add(i *memcache.Item) error {
	if err := m.data.IsBroken(); err != nil {
		return err
	}

	m.data.lock.Lock()
	defer m.data.lock.Unlock()

	if _, ok := m.retrieve(i.Key); !ok {
		m.data.items[i.Key] = m.mkItemLocked(i)
		return nil
	}
	return memcache.ErrNotStored
}

// CompareAndSwap implements context.MCSingleReadWriter.CompareAndSwap.
func (m *memcacheImpl) CompareAndSwap(item *memcache.Item) error {
	if err := m.data.IsBroken(); err != nil {
		return err
	}

	m.data.lock.Lock()
	defer m.data.lock.Unlock()

	if cur, ok := m.retrieve(item.Key); ok {
		if cur.CasID == unsafe.MCGetCasID(item) {
			m.data.items[item.Key] = m.mkItemLocked(item)
		} else {
			return memcache.ErrCASConflict
		}
	} else {
		return memcache.ErrNotStored
	}
	return nil
}

// Set implements context.MCSingleReadWriter.Set.
func (m *memcacheImpl) Set(i *memcache.Item) error {
	if err := m.data.IsBroken(); err != nil {
		return err
	}

	m.data.lock.Lock()
	defer m.data.lock.Unlock()

	m.data.items[i.Key] = m.mkItemLocked(i)
	return nil
}

// Get implements context.MCSingleReadWriter.Get.
func (m *memcacheImpl) Get(key string) (*memcache.Item, error) {
	if err := m.data.IsBroken(); err != nil {
		return nil, err
	}

	m.data.lock.Lock()
	defer m.data.lock.Unlock()

	if val, ok := m.retrieve(key); ok {
		return copyBack(val), nil
	}
	return nil, memcache.ErrCacheMiss
}

// Delete implements context.MCSingleReadWriter.Delete.
func (m *memcacheImpl) Delete(key string) error {
	if err := m.data.IsBroken(); err != nil {
		return err
	}

	m.data.lock.Lock()
	defer m.data.lock.Unlock()

	if _, ok := m.retrieve(key); ok {
		delete(m.data.items, key)
		return nil
	}
	return memcache.ErrCacheMiss
}
