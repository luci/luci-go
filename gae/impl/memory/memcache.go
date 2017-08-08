// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memory

import (
	"encoding/binary"
	"sync"
	"time"

	"go.chromium.org/gae/service/info"
	mc "go.chromium.org/gae/service/memcache"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	"golang.org/x/net/context"
)

type mcItem struct {
	key        string
	value      []byte
	flags      uint32
	expiration time.Duration

	CasID uint64
}

var _ mc.Item = (*mcItem)(nil)

func (m *mcItem) Key() string               { return m.key }
func (m *mcItem) Value() []byte             { return m.value }
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
func (m *mcItem) SetFlags(flg uint32) mc.Item {
	m.flags = flg
	return m
}
func (m *mcItem) SetExpiration(exp time.Duration) mc.Item {
	m.expiration = exp
	return m
}

func (m *mcItem) SetAll(other mc.Item) {
	if other == nil {
		*m = mcItem{key: m.key}
	} else {
		k := m.key
		*m = *other.(*mcItem)
		m.key = k
	}
}

type mcDataItem struct {
	value      []byte
	flags      uint32
	expiration time.Time
	casID      uint64
}

func (m *mcDataItem) toUserItem(key string) *mcItem {
	value := make([]byte, len(m.value))
	copy(value, m.value)
	// Expiration is defined to be 0 when retrieving items from memcache.
	//   https://cloud.google.com/appengine/docs/go/memcache/reference#Item
	// ¯\_(ツ)_/¯
	return &mcItem{key, value, m.flags, 0, m.casID}
}

type memcacheData struct {
	lock  sync.Mutex
	items map[string]*mcDataItem
	casID uint64

	stats mc.Statistics
}

func (m *memcacheData) mkDataItemLocked(now time.Time, i mc.Item) (ret *mcDataItem) {
	m.casID++

	exp := time.Time{}
	if i.Expiration() != 0 {
		exp = now.Add(i.Expiration()).Truncate(time.Second)
	}
	value := make([]byte, len(i.Value()))
	copy(value, i.Value())
	return &mcDataItem{
		flags:      i.Flags(),
		expiration: exp,
		value:      value,
		casID:      m.casID,
	}
}

func (m *memcacheData) setItemLocked(now time.Time, i mc.Item) {
	if cur, ok := m.items[i.Key()]; ok {
		m.stats.Items--
		m.stats.Bytes -= uint64(len(cur.value))
	}
	m.stats.Items++
	m.stats.Bytes += uint64(len(i.Value()))
	m.items[i.Key()] = m.mkDataItemLocked(now, i)
}

func (m *memcacheData) delItemLocked(k string) {
	if itm, ok := m.items[k]; ok {
		m.stats.Items--
		m.stats.Bytes -= uint64(len(itm.value))
		delete(m.items, k)
	}
}

func (m *memcacheData) reset() {
	m.stats = mc.Statistics{}
	m.items = map[string]*mcDataItem{}
}

func (m *memcacheData) hasItemLocked(now time.Time, key string) bool {
	ret, ok := m.items[key]
	if ok && !ret.expiration.IsZero() && ret.expiration.Before(now) {
		m.delItemLocked(key)
		return false
	}
	return ok
}

func (m *memcacheData) retrieveLocked(now time.Time, key string) (*mcDataItem, error) {
	if !m.hasItemLocked(now, key) {
		m.stats.Misses++
		return nil, mc.ErrCacheMiss
	}

	ret := m.items[key]
	m.stats.Hits++
	m.stats.ByteHits += uint64(len(ret.value))
	return ret, nil
}

// memcacheImpl binds the current connection's memcache data to an
// implementation of {gae.Memcache, gae.Testable}.
type memcacheImpl struct {
	data *memcacheData
	ctx  context.Context
}

var _ mc.RawInterface = (*memcacheImpl)(nil)

// useMC adds a gae.Memcache implementation to context, accessible
// by gae.GetMC(c)
func useMC(c context.Context) context.Context {
	lck := sync.Mutex{}
	// TODO(riannucci): just use namespace for automatic key prefixing. Flush
	// actually wipes the ENTIRE memcache, regardless of namespace.
	mcdMap := map[string]*memcacheData{}

	return mc.SetRawFactory(c, func(ic context.Context) mc.RawInterface {
		lck.Lock()
		defer lck.Unlock()

		ns := info.GetNamespace(ic)
		mcd, ok := mcdMap[ns]
		if !ok {
			mcd = &memcacheData{items: map[string]*mcDataItem{}}
			mcdMap[ns] = mcd
		}

		return &memcacheImpl{
			mcd,
			ic,
		}
	})
}

func (m *memcacheImpl) NewItem(key string) mc.Item {
	return &mcItem{key: key}
}

func doCBs(items []mc.Item, cb mc.RawCB, inner func(mc.Item) error) {
	// This weird construction is so that we:
	//   - don't take the lock for the entire multi operation, since it could imply
	//     false atomicity.
	//   - don't allow cb to block the actual batch operation, since that would
	//     allow binding in ways that aren't possible under the real
	//     implementation (like a recursive deadlock)
	errs := make([]error, len(items))
	for i, itm := range items {
		errs[i] = inner(itm)
	}
	for _, e := range errs {
		cb(e)
	}
}

func (m *memcacheImpl) AddMulti(items []mc.Item, cb mc.RawCB) error {
	now := clock.Now(m.ctx)
	doCBs(items, cb, func(itm mc.Item) error {
		m.data.lock.Lock()
		defer m.data.lock.Unlock()
		if !m.data.hasItemLocked(now, itm.Key()) {
			m.data.setItemLocked(now, itm)
			return nil
		}
		return mc.ErrNotStored
	})
	return nil
}

func (m *memcacheImpl) CompareAndSwapMulti(items []mc.Item, cb mc.RawCB) error {
	now := clock.Now(m.ctx)
	doCBs(items, cb, func(itm mc.Item) error {
		m.data.lock.Lock()
		defer m.data.lock.Unlock()

		if cur, err := m.data.retrieveLocked(now, itm.Key()); err == nil {
			casid := uint64(0)
			if mi, ok := itm.(*mcItem); ok && mi != nil {
				casid = mi.CasID
			}

			if cur.casID == casid {
				m.data.setItemLocked(now, itm)
			} else {
				return mc.ErrCASConflict
			}
			return nil
		}
		return mc.ErrNotStored
	})
	return nil
}

func (m *memcacheImpl) SetMulti(items []mc.Item, cb mc.RawCB) error {
	now := clock.Now(m.ctx)
	doCBs(items, cb, func(itm mc.Item) error {
		m.data.lock.Lock()
		defer m.data.lock.Unlock()
		m.data.setItemLocked(now, itm)
		return nil
	})
	return nil
}

func (m *memcacheImpl) GetMulti(keys []string, cb mc.RawItemCB) error {
	now := clock.Now(m.ctx)

	itms := make([]mc.Item, len(keys))
	errs := make([]error, len(keys))

	for i, k := range keys {
		itms[i], errs[i] = func() (mc.Item, error) {
			m.data.lock.Lock()
			defer m.data.lock.Unlock()
			val, err := m.data.retrieveLocked(now, k)
			if err != nil {
				return nil, err
			}
			return val.toUserItem(k), nil
		}()
	}

	for i, itm := range itms {
		cb(itm, errs[i])
	}

	return nil
}

func (m *memcacheImpl) DeleteMulti(keys []string, cb mc.RawCB) error {
	now := clock.Now(m.ctx)

	errs := make([]error, len(keys))

	for i, k := range keys {
		errs[i] = func() error {
			m.data.lock.Lock()
			defer m.data.lock.Unlock()
			_, err := m.data.retrieveLocked(now, k)
			if err != nil {
				return err
			}
			m.data.delItemLocked(k)
			return nil
		}()
	}

	for _, e := range errs {
		cb(e)
	}

	return nil
}

func (m *memcacheImpl) Flush() error {
	m.data.lock.Lock()
	defer m.data.lock.Unlock()

	m.data.reset()
	return nil
}

func (m *memcacheImpl) Increment(key string, delta int64, initialValue *uint64) (uint64, error) {
	now := clock.Now(m.ctx)

	m.data.lock.Lock()
	defer m.data.lock.Unlock()

	cur := uint64(0)
	if initialValue == nil {
		curItm, err := m.data.retrieveLocked(now, key)
		if err != nil {
			return 0, err
		}
		if len(curItm.value) != 8 {
			return 0, errors.New("memcache Increment: got invalid current value")
		}
		cur = binary.LittleEndian.Uint64(curItm.value)
	} else {
		cur = *initialValue
	}
	if delta < 0 {
		if uint64(-delta) > cur {
			cur = 0
		} else {
			cur -= uint64(-delta)
		}
	} else {
		cur += uint64(delta)
	}

	newval := make([]byte, 8)
	binary.LittleEndian.PutUint64(newval, cur)
	m.data.setItemLocked(now, m.NewItem(key).SetValue(newval))

	return cur, nil
}

func (m *memcacheImpl) Stats() (*mc.Statistics, error) {
	m.data.lock.Lock()
	defer m.data.lock.Unlock()

	ret := m.data.stats
	return &ret, nil
}
