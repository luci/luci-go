// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"time"

	mc "github.com/luci/gae/service/memcache"
	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"
	"google.golang.org/appengine/memcache"
)

// useMC adds a gae.Memcache implementation to context, accessible
// by gae.GetMC(c)
func useMC(c context.Context) context.Context {
	return mc.SetFactory(c, func(ci context.Context) mc.Interface {
		return mcImpl{ci}
	})
}

type mcImpl struct{ context.Context }

type mcItem struct {
	i *memcache.Item
}

var _ mc.Item = mcItem{}

func (i mcItem) Key() string               { return i.i.Key }
func (i mcItem) Value() []byte             { return i.i.Value }
func (i mcItem) Object() interface{}       { return i.i.Object }
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
func (i mcItem) SetObject(o interface{}) mc.Item {
	i.i.Object = o
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
func mcF2R(i mc.Item) *memcache.Item {
	if mci, ok := i.(mcItem); ok {
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

//////// MCSingleReadWriter
func (m mcImpl) Add(item mc.Item) error {
	return memcache.Add(m.Context, mcF2R(item))
}
func (m mcImpl) Set(item mc.Item) error {
	return memcache.Set(m.Context, mcF2R(item))
}
func (m mcImpl) Delete(key string) error {
	return memcache.Delete(m.Context, key)
}
func (m mcImpl) Get(key string) (mc.Item, error) {
	return mcR2FErr(memcache.Get(m.Context, key))
}
func (m mcImpl) CompareAndSwap(item mc.Item) error {
	return memcache.CompareAndSwap(m.Context, mcF2R(item))
}

//////// MCMultiReadWriter
func (m mcImpl) DeleteMulti(keys []string) error {
	return errors.Fix(memcache.DeleteMulti(m.Context, keys))
}
func (m mcImpl) AddMulti(items []mc.Item) error {
	return errors.Fix(memcache.AddMulti(m.Context, mcMF2R(items)))
}
func (m mcImpl) SetMulti(items []mc.Item) error {
	return errors.Fix(memcache.SetMulti(m.Context, mcMF2R(items)))
}
func (m mcImpl) GetMulti(keys []string) (map[string]mc.Item, error) {
	realItems, err := memcache.GetMulti(m.Context, keys)
	if err != nil {
		return nil, errors.Fix(err)
	}
	items := make(map[string]mc.Item, len(realItems))
	for k, itm := range realItems {
		items[k] = mcItem{itm}
	}
	return items, err
}
func (m mcImpl) CompareAndSwapMulti(items []mc.Item) error {
	return errors.Fix(memcache.CompareAndSwapMulti(m.Context, mcMF2R(items)))
}

//////// MCIncrementer
func (m mcImpl) Increment(key string, delta int64, initialValue uint64) (uint64, error) {
	return memcache.Increment(m.Context, key, delta, initialValue)
}
func (m mcImpl) IncrementExisting(key string, delta int64) (uint64, error) {
	return memcache.IncrementExisting(m.Context, key, delta)
}

//////// MCFlusher
func (m mcImpl) Flush() error {
	return memcache.Flush(m.Context)
}

//////// MCStatter
func (m mcImpl) Stats() (*mc.Statistics, error) {
	stats, err := memcache.Stats(m.Context)
	if err != nil {
		return nil, err
	}
	return (*mc.Statistics)(stats), nil
}
