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

package featureBreaker

import (
	"golang.org/x/net/context"

	mc "go.chromium.org/gae/service/memcache"
)

type mcState struct {
	*state

	c context.Context
	mc.RawInterface
}

func (m *mcState) GetMulti(keys []string, cb mc.RawItemCB) error {
	if len(keys) == 0 {
		return nil
	}
	return m.run(m.c, func() error { return m.RawInterface.GetMulti(keys, cb) })
}

func (m *mcState) AddMulti(items []mc.Item, cb mc.RawCB) error {
	if len(items) == 0 {
		return nil
	}
	return m.run(m.c, func() error { return m.RawInterface.AddMulti(items, cb) })
}

func (m *mcState) SetMulti(items []mc.Item, cb mc.RawCB) error {
	if len(items) == 0 {
		return nil
	}
	return m.run(m.c, func() error { return m.RawInterface.SetMulti(items, cb) })
}

func (m *mcState) DeleteMulti(keys []string, cb mc.RawCB) error {
	if len(keys) == 0 {
		return nil
	}
	return m.run(m.c, func() error { return m.RawInterface.DeleteMulti(keys, cb) })
}

func (m *mcState) CompareAndSwapMulti(items []mc.Item, cb mc.RawCB) error {
	if len(items) == 0 {
		return nil
	}
	return m.run(m.c, func() error { return m.RawInterface.CompareAndSwapMulti(items, cb) })
}

func (m *mcState) Flush() error {
	return m.run(m.c, m.RawInterface.Flush)
}

func (m *mcState) Stats() (ret *mc.Statistics, err error) {
	err = m.run(m.c, func() (err error) {
		ret, err = m.RawInterface.Stats()
		return
	})
	return
}

// FilterMC installs a featureBreaker mc filter in the context.
func FilterMC(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return mc.AddRawFilters(c, func(ic context.Context, rds mc.RawInterface) mc.RawInterface {
		return &mcState{state, ic, rds}
	}), state
}
