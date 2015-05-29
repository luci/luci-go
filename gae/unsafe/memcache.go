// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package unsafe

import (
	"time"

	"unsafe"

	"appengine/memcache"
)

// Item is duplicated from appengine/memcache.
type Item struct {
	Key        string
	Value      []byte
	Object     interface{}
	Flags      uint32
	Expiration time.Duration
	CasID      uint64
}

func init() {
	// we can't actually refer to casID by name, but we can refer to everything
	// else, and ensure that everything lines up. This should catch an api change,
	// but wouldn't catch something like uint64 -> int64.
	if unsafe.Sizeof(memcache.Item{}) != unsafe.Sizeof(Item{}) {
		panic("memcache.Item and Item mismatch sizes")
	}
	if unsafe.Offsetof(memcache.Item{}.Key) != unsafe.Offsetof(Item{}.Key) {
		panic("memcache.Item and Item mismatch Key")
	}
	if unsafe.Offsetof(memcache.Item{}.Value) != unsafe.Offsetof(Item{}.Value) {
		panic("memcache.Item and Item mismatch Value")
	}
	if unsafe.Offsetof(memcache.Item{}.Object) != unsafe.Offsetof(Item{}.Object) {
		panic("memcache.Item and Item mismatch Object")
	}
	if unsafe.Offsetof(memcache.Item{}.Flags) != unsafe.Offsetof(Item{}.Flags) {
		panic("memcache.Item and Item mismatch Flags")
	}
	if unsafe.Offsetof(memcache.Item{}.Expiration) != unsafe.Offsetof(Item{}.Expiration) {
		panic("memcache.Item and Item mismatch Expiration")
	}
}

// MCSetCasID sets the private .casID field of memcache.Item.
func MCSetCasID(i *memcache.Item, val uint64) {
	mci := (*Item)(unsafe.Pointer(i))
	mci.CasID = val
}

// MCGetCasID retrieves the private .casID field of memcache.Item.
func MCGetCasID(i *memcache.Item) uint64 {
	return (*Item)(unsafe.Pointer(i)).CasID
}
