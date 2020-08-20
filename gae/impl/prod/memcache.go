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

package prod

import (
	"time"

	mc "go.chromium.org/gae/service/memcache"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/memcache"
)

// useMC adds a gae.Memcache implementation to context, accessible
// by gae.GetMC(c)
func useMC(c context.Context) context.Context {
	return mc.SetRawFactory(c, func(ci context.Context) mc.RawInterface {
		return mcImpl{getAEContext(ci)}
	})
}

type mcImpl struct {
	aeCtx context.Context
}

type mcItem struct {
	i *memcache.Item
}

var _ mc.Item = mcItem{}

func (i mcItem) Key() string               { return i.i.Key }
func (i mcItem) Value() []byte             { return i.i.Value }
func (i mcItem) Flags() uint32             { return i.i.Flags }
func (i mcItem) Expiration() time.Duration { return i.i.Expiration }

func (i mcItem) SetKey(k string) mc.Item {
	i.i.Key = k
	return i
}
func (i mcItem) SetValue(v []byte) mc.Item {
	i.i.Value = v
	return i
}
func (i mcItem) SetFlags(f uint32) mc.Item {
	i.i.Flags = f
	return i
}
func (i mcItem) SetExpiration(d time.Duration) mc.Item {
	i.i.Expiration = d
	return i
}

func (i mcItem) SetAll(other mc.Item) {
	if other == nil {
		i.i = &memcache.Item{Key: i.i.Key}
	} else {
		k := i.i.Key
		*i.i = *other.(mcItem).i
		i.i.Key = k
	}
}

// mcR2FErr (MC real-to-fake w/ error) converts a *memcache.Item to a mc.Item,
// and passes along an error.
func mcR2FErr(i *memcache.Item, err error) (mc.Item, error) {
	if err != nil {
		return nil, err
	}
	return mcItem{i}, nil
}

// mcF2R (MC fake-to-real) converts a mc.Item. i must originate from inside
// this package for this function to work (see the panic message for why).
//
// If the item's Value == nil, it will be copied and replaced with []byte{}.
func mcF2R(i mc.Item) *memcache.Item {
	if mci, ok := i.(mcItem); ok {
		if mci.i.Value == nil {
			ret := *mci.i
			ret.Value = []byte{}
			return &ret
		}
		return mci.i
	}
	panic(
		"you may not use other mc.Item implementations with this " +
			"implementation of gae.Memcache, since it will cause all CompareAndSwap " +
			"operations to fail. Please use the NewItem api instead.")
}

// mcMF2R (MC multi-fake-to-real) converts a slice of mc.Item to a slice of
// *memcache.Item.
func mcMF2R(items []mc.Item) []*memcache.Item {
	realItems := make([]*memcache.Item, len(items))
	for i, itm := range items {
		realItems[i] = mcF2R(itm)
	}
	return realItems
}

func (m mcImpl) NewItem(key string) mc.Item {
	return mcItem{&memcache.Item{Key: key}}
}

func doCB(err error, cb mc.RawCB) error {
	if me, ok := err.(appengine.MultiError); ok {
		for _, err := range me {
			cb(err)
		}
		err = nil
	}
	return err
}

func (m mcImpl) DeleteMulti(keys []string, cb mc.RawCB) error {
	return doCB(memcache.DeleteMulti(m.aeCtx, keys), cb)
}

func (m mcImpl) AddMulti(items []mc.Item, cb mc.RawCB) error {
	return doCB(memcache.AddMulti(m.aeCtx, mcMF2R(items)), cb)
}

func (m mcImpl) SetMulti(items []mc.Item, cb mc.RawCB) error {
	return doCB(memcache.SetMulti(m.aeCtx, mcMF2R(items)), cb)
}

func (m mcImpl) GetMulti(keys []string, cb mc.RawItemCB) error {
	realItems, err := memcache.GetMulti(m.aeCtx, keys)
	if err != nil {
		return err
	}
	for _, k := range keys {
		itm := realItems[k]
		if itm == nil {
			cb(nil, memcache.ErrCacheMiss)
		} else {
			cb(mcItem{itm}, nil)
		}
	}
	return nil
}

func (m mcImpl) CompareAndSwapMulti(items []mc.Item, cb mc.RawCB) error {
	return doCB(memcache.CompareAndSwapMulti(m.aeCtx, mcMF2R(items)), cb)
}

func (m mcImpl) Increment(key string, delta int64, initialValue *uint64) (uint64, error) {
	if initialValue == nil {
		return memcache.IncrementExisting(m.aeCtx, key, delta)
	}
	return memcache.Increment(m.aeCtx, key, delta, *initialValue)
}

func (m mcImpl) Flush() error {
	return memcache.Flush(m.aeCtx)
}

func (m mcImpl) Stats() (*mc.Statistics, error) {
	stats, err := memcache.Stats(m.aeCtx)
	if err != nil {
		return nil, err
	}
	return (*mc.Statistics)(stats), nil
}
