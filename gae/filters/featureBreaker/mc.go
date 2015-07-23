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

	mc.Interface
}

func (m *mcState) Get(key string) (ret mc.Item, err error) {
	err = m.run(func() (err error) {
		ret, err = m.Interface.Get(key)
		return
	})
	return
}

func (m *mcState) GetMulti(keys []string) (ret map[string]mc.Item, err error) {
	err = m.run(func() (err error) {
		ret, err = m.Interface.GetMulti(keys)
		return
	})
	return
}

func (m *mcState) Add(item mc.Item) error {
	return m.run(func() error { return m.Interface.Add(item) })
}

func (m *mcState) Set(item mc.Item) error {
	return m.run(func() error { return m.Interface.Set(item) })
}

func (m *mcState) Delete(key string) error {
	return m.run(func() error { return m.Interface.Delete(key) })
}

func (m *mcState) CompareAndSwap(item mc.Item) error {
	return m.run(func() error { return m.Interface.CompareAndSwap(item) })
}

func (m *mcState) AddMulti(items []mc.Item) error {
	return m.run(func() error { return m.Interface.AddMulti(items) })
}

func (m *mcState) SetMulti(items []mc.Item) error {
	return m.run(func() error { return m.Interface.SetMulti(items) })
}

func (m *mcState) DeleteMulti(keys []string) error {
	return m.run(func() error { return m.Interface.DeleteMulti(keys) })
}

func (m *mcState) Flush() error {
	return m.run(func() error { return m.Interface.Flush() })
}

func (m *mcState) CompareAndSwapMulti(items []mc.Item) error {
	return m.run(func() error { return m.Interface.CompareAndSwapMulti(items) })
}

func (m *mcState) Increment(key string, delta int64, initialValue uint64) (newValue uint64, err error) {
	err = m.run(func() (err error) {
		newValue, err = m.Interface.Increment(key, delta, initialValue)
		return
	})
	return
}

func (m *mcState) IncrementExisting(key string, delta int64) (newValue uint64, err error) {
	err = m.run(func() (err error) {
		newValue, err = m.Interface.IncrementExisting(key, delta)
		return
	})
	return
}

func (m *mcState) Stats() (ret *mc.Statistics, err error) {
	err = m.run(func() (err error) {
		ret, err = m.Interface.Stats()
		return
	})
	return
}

// FilterMC installs a counter mc filter in the context.
func FilterMC(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return mc.AddFilters(c, func(ic context.Context, rds mc.Interface) mc.Interface {
		return &mcState{state, rds}
	}), state
}
