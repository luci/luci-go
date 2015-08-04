// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featureBreaker

import (
	"golang.org/x/net/context"

	mc "github.com/luci/gae/service/memcache"
)

type mcState struct {
	*state

	mc.RawInterface
}

func (m *mcState) GetMulti(keys []string, cb mc.RawItemCB) error {
	return m.run(func() error { return m.RawInterface.GetMulti(keys, cb) })
}

func (m *mcState) AddMulti(items []mc.Item, cb mc.RawCB) error {
	return m.run(func() error { return m.RawInterface.AddMulti(items, cb) })
}

func (m *mcState) SetMulti(items []mc.Item, cb mc.RawCB) error {
	return m.run(func() error { return m.RawInterface.SetMulti(items, cb) })
}

func (m *mcState) DeleteMulti(keys []string, cb mc.RawCB) error {
	return m.run(func() error { return m.RawInterface.DeleteMulti(keys, cb) })
}

func (m *mcState) CompareAndSwapMulti(items []mc.Item, cb mc.RawCB) error {
	return m.run(func() error { return m.RawInterface.CompareAndSwapMulti(items, cb) })
}

func (m *mcState) Flush() error {
	return m.run(m.RawInterface.Flush)
}

func (m *mcState) Stats() (ret *mc.Statistics, err error) {
	err = m.run(func() (err error) {
		ret, err = m.RawInterface.Stats()
		return
	})
	return
}

// FilterMC installs a counter mc filter in the context.
func FilterMC(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return mc.AddRawFilters(c, func(ic context.Context, rds mc.RawInterface) mc.RawInterface {
		return &mcState{state, rds}
	}), state
}
