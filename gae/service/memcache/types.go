// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memcache

import (
	"time"
)

// Statistics represents a set of statistics about the memcache cache.  This
// may include items that have expired but have not yet been removed from the
// cache.
type Statistics struct {
	Hits     uint64 // Counter of cache hits
	Misses   uint64 // Counter of cache misses
	ByteHits uint64 // Counter of bytes transferred for gets

	Items uint64 // Items currently in the cache
	Bytes uint64 // Size of all items currently in the cache

	Oldest int64 // Age of access of the oldest item, in seconds
}

// Item is a wrapper around *memcache.Item. Note that the Set* methods all
// return the original Item (e.g. they mutate the original), due to
// implementation constraints. They return the original item to allow easy
// chaining, e.g.:
//   itm := memcache.Get(c).NewItem("foo").SetValue([]byte("stuff"))
type Item interface {
	Key() string
	Value() []byte
	Flags() uint32
	Expiration() time.Duration

	SetKey(string) Item
	SetValue([]byte) Item
	SetFlags(uint32) Item
	SetExpiration(time.Duration) Item

	// SetAll copies all the values from other, except Key, into this item
	// (including the hidden CasID field). If other is nil, then all non-Key
	// fields are reset.
	SetAll(other Item)
}
