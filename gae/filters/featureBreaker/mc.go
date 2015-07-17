// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featureBreaker

import (
	"golang.org/x/net/context"

	"infra/gae/libs/gae"
)

type mcState struct {
	*state

	gae.Memcache
}

func (m *mcState) Get(key string) (ret gae.MCItem, err error) {
	err = m.run(func() (err error) {
		ret, err = m.Memcache.Get(key)
		return
	})
	return
}

func (m *mcState) GetMulti(keys []string) (ret map[string]gae.MCItem, err error) {
	err = m.run(func() (err error) {
		ret, err = m.Memcache.GetMulti(keys)
		return
	})
	return
}

func (m *mcState) Add(item gae.MCItem) error {
	return m.run(func() error { return m.Memcache.Add(item) })
}

func (m *mcState) Set(item gae.MCItem) error {
	return m.run(func() error { return m.Memcache.Set(item) })
}

func (m *mcState) Delete(key string) error {
	return m.run(func() error { return m.Memcache.Delete(key) })
}

func (m *mcState) CompareAndSwap(item gae.MCItem) error {
	return m.run(func() error { return m.Memcache.CompareAndSwap(item) })
}

func (m *mcState) AddMulti(items []gae.MCItem) error {
	return m.run(func() error { return m.Memcache.AddMulti(items) })
}

func (m *mcState) SetMulti(items []gae.MCItem) error {
	return m.run(func() error { return m.Memcache.SetMulti(items) })
}

func (m *mcState) DeleteMulti(keys []string) error {
	return m.run(func() error { return m.Memcache.DeleteMulti(keys) })
}

func (m *mcState) Flush() error {
	return m.run(func() error { return m.Memcache.Flush() })
}

func (m *mcState) CompareAndSwapMulti(items []gae.MCItem) error {
	return m.run(func() error { return m.Memcache.CompareAndSwapMulti(items) })
}

func (m *mcState) Increment(key string, delta int64, initialValue uint64) (newValue uint64, err error) {
	err = m.run(func() (err error) {
		newValue, err = m.Memcache.Increment(key, delta, initialValue)
		return
	})
	return
}

func (m *mcState) IncrementExisting(key string, delta int64) (newValue uint64, err error) {
	err = m.run(func() (err error) {
		newValue, err = m.Memcache.IncrementExisting(key, delta)
		return
	})
	return
}

func (m *mcState) Stats() (ret *gae.MCStatistics, err error) {
	err = m.run(func() (err error) {
		ret, err = m.Memcache.Stats()
		return
	})
	return
}

// FilterMC installs a counter Memcache filter in the context.
func FilterMC(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return gae.AddMCFilters(c, func(ic context.Context, rds gae.Memcache) gae.Memcache {
		return &mcState{state, rds}
	}), state
}
