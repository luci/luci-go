// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package count

import (
	"golang.org/x/net/context"

	"infra/gae/libs/gae"
)

// MCCounter is the counter object for the Memcache service.
type MCCounter struct {
	NewItem             Entry
	Add                 Entry
	Set                 Entry
	Get                 Entry
	Delete              Entry
	CompareAndSwap      Entry
	AddMulti            Entry
	SetMulti            Entry
	GetMulti            Entry
	DeleteMulti         Entry
	CompareAndSwapMulti Entry
	Increment           Entry
	IncrementExisting   Entry
	Flush               Entry
	Stats               Entry
}

type mcCounter struct {
	c *MCCounter

	mc gae.Memcache
}

var _ gae.Memcache = (*mcCounter)(nil)

func (m *mcCounter) NewItem(key string) gae.MCItem {
	m.c.NewItem.up()
	return m.mc.NewItem(key)
}

func (m *mcCounter) Get(key string) (gae.MCItem, error) {
	ret, err := m.mc.Get(key)
	return ret, m.c.Get.up(err)
}

func (m *mcCounter) GetMulti(keys []string) (map[string]gae.MCItem, error) {
	ret, err := m.mc.GetMulti(keys)
	return ret, m.c.GetMulti.up(err)
}

func (m *mcCounter) Add(item gae.MCItem) error { return m.c.Add.up(m.mc.Add(item)) }
func (m *mcCounter) Set(item gae.MCItem) error { return m.c.Set.up(m.mc.Set(item)) }
func (m *mcCounter) Delete(key string) error   { return m.c.Delete.up(m.mc.Delete(key)) }
func (m *mcCounter) CompareAndSwap(item gae.MCItem) error {
	return m.c.CompareAndSwap.up(m.mc.CompareAndSwap(item))
}
func (m *mcCounter) AddMulti(items []gae.MCItem) error { return m.c.AddMulti.up(m.mc.AddMulti(items)) }
func (m *mcCounter) SetMulti(items []gae.MCItem) error { return m.c.SetMulti.up(m.mc.SetMulti(items)) }
func (m *mcCounter) DeleteMulti(keys []string) error {
	return m.c.DeleteMulti.up(m.mc.DeleteMulti(keys))
}
func (m *mcCounter) Flush() error { return m.c.Flush.up(m.mc.Flush()) }

func (m *mcCounter) CompareAndSwapMulti(items []gae.MCItem) error {
	return m.c.CompareAndSwapMulti.up(m.mc.CompareAndSwapMulti(items))
}

func (m *mcCounter) Increment(key string, delta int64, initialValue uint64) (newValue uint64, err error) {
	ret, err := m.mc.Increment(key, delta, initialValue)
	return ret, m.c.Increment.up(err)
}

func (m *mcCounter) IncrementExisting(key string, delta int64) (newValue uint64, err error) {
	ret, err := m.mc.IncrementExisting(key, delta)
	return ret, m.c.IncrementExisting.up(err)
}

func (m *mcCounter) Stats() (*gae.MCStatistics, error) {
	ret, err := m.mc.Stats()
	return ret, m.c.Stats.up(err)
}

// FilterMC installs a counter Memcache filter in the context.
func FilterMC(c context.Context) (context.Context, *MCCounter) {
	state := &MCCounter{}
	return gae.AddMCFilters(c, func(ic context.Context, mc gae.Memcache) gae.Memcache {
		return &mcCounter{state, mc}
	}), state
}
