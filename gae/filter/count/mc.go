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

package count

import (
	"golang.org/x/net/context"

	mc "go.chromium.org/gae/service/memcache"
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
