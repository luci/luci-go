// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gae

import (
	"time"

	"golang.org/x/net/context"
)

// MCItem is a wrapper around *memcache.Item. Note that the Set* methods all
// return the original MCItem (e.g. they mutate the original), due to
// implementation constraints. They return the original item to allow easy
// chaining, e.g.:
//   itm := gae.MC(c).NewItem("foo").SetValue([]byte("stuff"))
type MCItem interface {
	Key() string
	Value() []byte
	Object() interface{}
	Flags() uint32
	Expiration() time.Duration

	SetKey(string) MCItem
	SetValue([]byte) MCItem
	SetObject(interface{}) MCItem
	SetFlags(uint32) MCItem
	SetExpiration(time.Duration) MCItem
}

// Memcache is the full interface to the memcache service.
type Memcache interface {
	NewItem(key string) MCItem

	Add(item MCItem) error
	Set(item MCItem) error
	Get(key string) (MCItem, error)
	Delete(key string) error
	CompareAndSwap(item MCItem) error

	AddMulti(items []MCItem) error
	SetMulti(items []MCItem) error
	GetMulti(keys []string) (map[string]MCItem, error)
	DeleteMulti(keys []string) error
	CompareAndSwapMulti(items []MCItem) error

	Increment(key string, delta int64, initialValue uint64) (newValue uint64, err error)
	IncrementExisting(key string, delta int64) (newValue uint64, err error)

	Flush() error

	Stats() (*MCStatistics, error)
}

// MCFactory is the function signature for factory methods compatible with
// SetMCFactory.
type MCFactory func(context.Context) Memcache

// GetMC gets the current memcache implementation from the context.
func GetMC(c context.Context) Memcache {
	if f, ok := c.Value(memcacheKey).(MCFactory); ok && f != nil {
		return f(c)
	}
	return nil
}

// SetMCFactory sets the function to produce Memcache instances, as returned by
// the GetMC method.
func SetMCFactory(c context.Context, mcf MCFactory) context.Context {
	return context.WithValue(c, memcacheKey, mcf)
}

// SetMC sets the current Memcache object in the context. Useful for testing
// with a quick mock. This is just a shorthand SetMCFactory invocation to set
// a factory which always returns the same object.
func SetMC(c context.Context, mc Memcache) context.Context {
	return SetMCFactory(c, func(context.Context) Memcache { return mc })
}
