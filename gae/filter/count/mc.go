// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package count

import (
	"golang.org/x/net/context"

	mc "github.com/luci/gae/service/memcache"
)

// MCCounter is the counter object for the Memcache service.
type MCCounter struct {
	NewItem             Entry
	AddMulti            Entry
	SetMulti            Entry
	GetMulti            Entry
	DeleteMulti         Entry
	CompareAndSwapMulti Entry
	Increment           Entry
	Flush               Entry
	Stats               Entry
}

type mcCounter struct {
	c *MCCounter

	mc mc.RawInterface
}

var _ mc.RawInterface = (*mcCounter)(nil)

func (m *mcCounter) NewItem(key string) mc.Item {
	_ = m.c.NewItem.up()
	return m.mc.NewItem(key)
}

func (m *mcCounter) GetMulti(keys []string, cb mc.RawItemCB) error {
	return m.c.GetMulti.up(m.mc.GetMulti(keys, cb))
}

func (m *mcCounter) AddMulti(items []mc.Item, cb mc.RawCB) error {
	return m.c.AddMulti.up(m.mc.AddMulti(items, cb))
}

func (m *mcCounter) SetMulti(items []mc.Item, cb mc.RawCB) error {
	return m.c.SetMulti.up(m.mc.SetMulti(items, cb))
}

func (m *mcCounter) DeleteMulti(keys []string, cb mc.RawCB) error {
	return m.c.DeleteMulti.up(m.mc.DeleteMulti(keys, cb))
}

func (m *mcCounter) CompareAndSwapMulti(items []mc.Item, cb mc.RawCB) error {
	return m.c.CompareAndSwapMulti.up(m.mc.CompareAndSwapMulti(items, cb))
}

func (m *mcCounter) Flush() error { return m.c.Flush.up(m.mc.Flush()) }

func (m *mcCounter) Increment(key string, delta int64, initialValue *uint64) (newValue uint64, err error) {
	ret, err := m.mc.Increment(key, delta, initialValue)
	return ret, m.c.Increment.up(err)
}

func (m *mcCounter) Stats() (*mc.Statistics, error) {
	ret, err := m.mc.Stats()
	return ret, m.c.Stats.up(err)
}

// FilterMC installs a counter Memcache filter in the context.
func FilterMC(c context.Context) (context.Context, *MCCounter) {
	state := &MCCounter{}
	return mc.AddRawFilters(c, func(ic context.Context, mc mc.RawInterface) mc.RawInterface {
		return &mcCounter{state, mc}
	}), state
}
