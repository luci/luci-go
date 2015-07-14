// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wrapper

import (
	"golang.org/x/net/context"

	"appengine/memcache"
)

// MCSingleReadWriter is the interface for interacting with a single
// memcache.Item at a time. See appengine.memcache for more details.
type MCSingleReadWriter interface {
	Add(item *memcache.Item) error
	Set(item *memcache.Item) error
	Get(key string) (*memcache.Item, error)
	Delete(key string) error
	CompareAndSwap(item *memcache.Item) error
}

// MCIncrementer is the interface for atomically incrementing a numeric
// memcache entry.  See appengine.memcache for more details.
type MCIncrementer interface {
	Increment(key string, delta int64, initialValue uint64) (newValue uint64, err error)
	IncrementExisting(key string, delta int64) (newValue uint64, err error)
}

// MCMultiReadWriter is the interface for doing batched memcache
// operations. See appengine.memcache for more details.
type MCMultiReadWriter interface {
	MCSingleReadWriter

	AddMulti(items []*memcache.Item) error
	SetMulti(items []*memcache.Item) error
	GetMulti(keys []string) (map[string]*memcache.Item, error)
	DeleteMulti(keys []string) error
	CompareAndSwapMulti(items []*memcache.Item) error
}

// MCFlusher is the interface for wiping the whole memcache server.
// See appengine.memcache for more details.
type MCFlusher interface {
	Flush() error
}

// MCStatter is the interface for gathering statistics about the
// memcache server. See appengine.memcache for more details.
type MCStatter interface {
	Stats() (*memcache.Statistics, error)
}

// MCCodec is the interface representing what you can do with a memcache.Codec
// after it's been inflated with InflateCodec. It mirrors
// appengine.memcache.Codec.
type MCCodec interface {
	// would use MCSingleReadWriter, but Get has a different signature.
	Add(item *memcache.Item) error
	Set(item *memcache.Item) error
	Get(key string, v interface{}) (*memcache.Item, error)
	CompareAndSwap(item *memcache.Item) error

	AddMulti(items []*memcache.Item) error
	SetMulti(items []*memcache.Item) error
	CompareAndSwapMulti(items []*memcache.Item) error
}

// MCCodecInflater binds a Memcache to a memcache.Codec.
type MCCodecInflater interface {
	InflateCodec(m memcache.Codec) MCCodec
}

// Memcache is the full interface to the memcache service.
type Memcache interface {
	MCMultiReadWriter
	MCIncrementer
	MCFlusher
	MCStatter
	MCCodecInflater
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
