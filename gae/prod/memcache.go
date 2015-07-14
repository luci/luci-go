// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"golang.org/x/net/context"
	"time"

	"infra/gae/libs/gae"

	"google.golang.org/appengine/memcache"
)

// useMC adds a gae.Memcache implementation to context, accessible
// by gae.GetMC(c)
func useMC(c context.Context) context.Context {
	return gae.SetMCFactory(c, func(ci context.Context) gae.Memcache {
		return mcImpl{ci}
	})
}

type mcImpl struct{ context.Context }

type mcItem struct {
	i *memcache.Item
}

var _ gae.MCItem = mcItem{}

func (i mcItem) Key() string               { return i.i.Key }
func (i mcItem) Value() []byte             { return i.i.Value }
func (i mcItem) Object() interface{}       { return i.i.Object }
func (i mcItem) Flags() uint32             { return i.i.Flags }
func (i mcItem) Expiration() time.Duration { return i.i.Expiration }

func (i mcItem) SetKey(k string) gae.MCItem {
	i.i.Key = k
	return i
}
func (i mcItem) SetValue(v []byte) gae.MCItem {
	i.i.Value = v
	return i
}
func (i mcItem) SetObject(o interface{}) gae.MCItem {
	i.i.Object = o
	return i
}
func (i mcItem) SetFlags(f uint32) gae.MCItem {
	i.i.Flags = f
	return i
}
func (i mcItem) SetExpiration(d time.Duration) gae.MCItem {
	i.i.Expiration = d
	return i
}

// mcR2FErr (MC real-to-fake w/ error) converts a *memcache.Item to a gae.MCItem,
// and passes along an error.
func mcR2FErr(i *memcache.Item, err error) (gae.MCItem, error) {
	if err != nil {
		return nil, err
	}
	return mcItem{i}, nil
}

// mcF2R (MC fake-to-real) converts a gae.MCItem. i must originate from inside
// this package for this function to work (see the panic message for why).
func mcF2R(i gae.MCItem) *memcache.Item {
	if mci, ok := i.(mcItem); ok {
		return mci.i
	}
	panic(
		"you may not use other gae.MCItem implementations with this " +
			"implementation of gae.Memcache, since it will cause all CompareAndSwap " +
			"operations to fail. Please use the NewItem api instead.")
}

// mcMF2R (MC multi-fake-to-real) converts a slice of gae.MCItem to a slice of
// *memcache.Item.
func mcMF2R(items []gae.MCItem) []*memcache.Item {
	realItems := make([]*memcache.Item, len(items))
	for i, itm := range items {
		realItems[i] = mcF2R(itm)
	}
	return realItems
}

func (m mcImpl) NewItem(key string) gae.MCItem {
	return mcItem{&memcache.Item{Key: key}}
}

//////// MCSingleReadWriter
func (m mcImpl) Add(item gae.MCItem) error {
	return memcache.Add(m.Context, mcF2R(item))
}
func (m mcImpl) Set(item gae.MCItem) error {
	return memcache.Set(m.Context, mcF2R(item))
}
func (m mcImpl) Delete(key string) error {
	return memcache.Delete(m.Context, key)
}
func (m mcImpl) Get(key string) (gae.MCItem, error) {
	return mcR2FErr(memcache.Get(m.Context, key))
}
func (m mcImpl) CompareAndSwap(item gae.MCItem) error {
	return memcache.CompareAndSwap(m.Context, mcF2R(item))
}

//////// MCMultiReadWriter
func (m mcImpl) DeleteMulti(keys []string) error {
	return gae.FixError(memcache.DeleteMulti(m.Context, keys))
}
func (m mcImpl) AddMulti(items []gae.MCItem) error {
	return gae.FixError(memcache.AddMulti(m.Context, mcMF2R(items)))
}
func (m mcImpl) SetMulti(items []gae.MCItem) error {
	return gae.FixError(memcache.SetMulti(m.Context, mcMF2R(items)))
}
func (m mcImpl) GetMulti(keys []string) (map[string]gae.MCItem, error) {
	realItems, err := memcache.GetMulti(m.Context, keys)
	if err != nil {
		return nil, gae.FixError(err)
	}
	items := make(map[string]gae.MCItem, len(realItems))
	for k, itm := range realItems {
		items[k] = mcItem{itm}
	}
	return items, err
}
func (m mcImpl) CompareAndSwapMulti(items []gae.MCItem) error {
	return gae.FixError(memcache.CompareAndSwapMulti(m.Context, mcMF2R(items)))
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
func (m mcImpl) Stats() (*gae.MCStatistics, error) {
	stats, err := memcache.Stats(m.Context)
	if err != nil {
		return nil, err
	}
	return (*gae.MCStatistics)(stats), nil
}
