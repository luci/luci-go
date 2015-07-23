// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/impl/dummy"
	mc "github.com/luci/gae/service/memcache"
	"github.com/luci/luci-go/common/clock"
)

type mcItem struct {
	key        string
	value      []byte
	object     interface{}
	flags      uint32
	expiration time.Duration

	CasID uint64
}

var _ mc.Item = (*mcItem)(nil)

func (m *mcItem) Key() string               { return m.key }
func (m *mcItem) Value() []byte             { return m.value }
func (m *mcItem) Object() interface{}       { return m.object }
func (m *mcItem) Flags() uint32             { return m.flags }
func (m *mcItem) Expiration() time.Duration { return m.expiration }

func (m *mcItem) SetKey(key string) mc.Item {
	m.key = key
	return m
}
func (m *mcItem) SetValue(val []byte) mc.Item {
	m.value = val
	return m
}
func (m *mcItem) SetObject(obj interface{}) mc.Item {
	m.object = obj
	return m
}
func (m *mcItem) SetFlags(flg uint32) mc.Item {
	m.flags = flg
	return m
}
func (m *mcItem) SetExpiration(exp time.Duration) mc.Item {
	m.expiration = exp
	return m
}

func (m *mcItem) duplicate() *mcItem {
	ret := mcItem{}
	ret = *m
	ret.value = make([]byte, len(m.value))
	copy(ret.value, m.value)
	return &ret
}

type memcacheData struct {
	lock  sync.Mutex
	items map[string]*mcItem
	casID uint64
}

// memcacheImpl binds the current connection's memcache data to an
// implementation of {gae.Memcache, gae.Testable}.
type memcacheImpl struct {
	mc.Interface

	data *memcacheData
	ctx  context.Context
}

var _ mc.Interface = (*memcacheImpl)(nil)

// useMC adds a gae.Memcache implementation to context, accessible
// by gae.GetMC(c)
func useMC(c context.Context) context.Context {
	lck := sync.Mutex{}
	mcdMap := map[string]*memcacheData{}

	return mc.SetFactory(c, func(ic context.Context) mc.Interface {
		lck.Lock()
		defer lck.Unlock()

		ns := curGID(ic).namespace
		mcd, ok := mcdMap[ns]
		if !ok {
			mcd = &memcacheData{items: map[string]*mcItem{}}
			mcdMap[ns] = mcd
		}

		return &memcacheImpl{
			dummy.Memcache(),
			mcd,
			ic,
		}
	})
}

func (m *memcacheImpl) mkItemLocked(i mc.Item) (ret *mcItem) {
	m.data.casID++

	var exp time.Duration
	if i.Expiration() != 0 {
		exp = time.Duration(clock.Now(m.ctx).Add(i.Expiration()).UnixNano())
	}
	newItem := mcItem{
		key:        i.Key(),
		flags:      i.Flags(),
		expiration: exp,
		value:      i.Value(),
		CasID:      m.data.casID,
	}
	return newItem.duplicate()
}

func (m *memcacheImpl) NewItem(key string) mc.Item {
	return &mcItem{key: key}
}

// Add implements context.MCSingleReadWriter.Add.
func (m *memcacheImpl) Add(i mc.Item) error {
	m.data.lock.Lock()
	defer m.data.lock.Unlock()

	if _, ok := m.retrieveLocked(i.Key()); !ok {
		m.data.items[i.Key()] = m.mkItemLocked(i)
		return nil
	}
	return mc.ErrNotStored
}

// CompareAndSwap implements context.MCSingleReadWriter.CompareAndSwap.
func (m *memcacheImpl) CompareAndSwap(item mc.Item) error {
	m.data.lock.Lock()
	defer m.data.lock.Unlock()

	if cur, ok := m.retrieveLocked(item.Key()); ok {
		casid := uint64(0)
		if mi, ok := item.(*mcItem); ok && mi != nil {
			casid = mi.CasID
		}

		if cur.CasID == casid {
			m.data.items[item.Key()] = m.mkItemLocked(item)
		} else {
			return mc.ErrCASConflict
		}
	} else {
		return mc.ErrNotStored
	}
	return nil
}

// Set implements context.MCSingleReadWriter.Set.
func (m *memcacheImpl) Set(i mc.Item) error {
	m.data.lock.Lock()
	defer m.data.lock.Unlock()
	m.data.items[i.Key()] = m.mkItemLocked(i)
	return nil
}

// Get implements context.MCSingleReadWriter.Get.
func (m *memcacheImpl) Get(key string) (itm mc.Item, err error) {
	m.data.lock.Lock()
	defer m.data.lock.Unlock()
	if val, ok := m.retrieveLocked(key); ok {
		itm = val.duplicate().SetExpiration(0)
	} else {
		err = mc.ErrCacheMiss
	}
	return
}

// Delete implements context.MCSingleReadWriter.Delete.
func (m *memcacheImpl) Delete(key string) error {
	m.data.lock.Lock()
	defer m.data.lock.Unlock()

	if _, ok := m.retrieveLocked(key); ok {
		delete(m.data.items, key)
		return nil
	}
	return mc.ErrCacheMiss
}

func (m *memcacheImpl) retrieveLocked(key string) (*mcItem, bool) {
	ret, ok := m.data.items[key]
	if ok && ret.Expiration() != 0 && ret.Expiration() < time.Duration(clock.Now(m.ctx).UnixNano()) {
		ret = nil
		ok = false
		delete(m.data.items, key)
	}
	return ret, ok
}
