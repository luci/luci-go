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
//   itm := memcache.NewItem(c, "foo").SetValue([]byte("stuff"))
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
