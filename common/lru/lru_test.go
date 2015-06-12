// Copyright (c) 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lru

import (
	"fmt"
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// A sortable slice of interface{} that are all strings.
type stringInterfaceSlice []interface{}

// Implements sort.Interface
func (s stringInterfaceSlice) Len() int {
	return len(s)
}

// Implements sort.Interface
func (s stringInterfaceSlice) Less(i, j int) bool {
	return s[i].(string) < s[j].(string)
}

// Implements sort.Interface
func (s stringInterfaceSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// If put is true, test the Put command; if false, test the PutIfMissing
// command.
func TestCache(t *testing.T) {
	t.Parallel()

	Convey(`An empty LRU cache with size 3`, t, func() {
		cache := New(3)

		Convey(`A Get() returns nil.`, func() {
			So(cache.Get("test"), ShouldBeNil)
		})

		// Adds values to the cache sequentially, blocking on the values being
		// processed.
		addCacheValues := func(values ...string) {
			for _, v := range values {
				isPresent := (cache.Peek(v) != nil)
				So(cache.Put(v, v), ShouldEqual, isPresent)
			}
		}

		shouldHaveValues := func(actual interface{}, expected ...interface{}) string {
			cache := actual.(*cacheImpl)

			// Take a snapshot.
			ss := cache.snapshot()
			keys := stringInterfaceSlice(cache.keys())
			sort.Sort(keys)

			// A snapshot should have all keys.
			if len(ss) != len(keys) {
				return fmt.Sprintf("Snapshot size (%d) doesn't match keys size (%d).", len(ss), len(keys))
			}
			for _, k := range keys {
				v, ok := ss[k]
				if !ok {
					return fmt.Sprintf("Snapshot missing key '%s'", k)
				}
				if k != v {
					return fmt.Sprintf("Entry %s=%s should match.", k, v)
				}
			}

			// Maps should match.
			expectedMap := make(snapshot)
			for _, exp := range expected {
				value := exp.(string)
				expectedMap[value] = value
			}
			return ShouldResemble(ss, expectedMap)
		}

		Convey(`Has a size of 3.`, func() {
			So(cache.Size(), ShouldEqual, 3)
		})

		Convey(`With three values, {a, b, c}`, func() {
			addCacheValues("a", "b", "c")
			So(cache.Len(), ShouldEqual, 3)

			Convey(`Is empty after a purge.`, func() {
				cache.Purge()
				So(cache.Len(), ShouldEqual, 0)
			})

			Convey(`Can retrieve each of those values.`, func() {
				So(cache.Get("a"), ShouldEqual, "a")
				So(cache.Get("b"), ShouldEqual, "b")
				So(cache.Get("c"), ShouldEqual, "c")
			})

			Convey(`Get()ting "a", then adding "d" will cause "b" to be evicted.`, func() {
				So(cache.Get("a"), ShouldEqual, "a")
				addCacheValues("d")
				So(cache, shouldHaveValues, "a", "c", "d")
			})

			Convey(`Peek()ing "a", then adding "d" will cause "a" to be evicted.`, func() {
				So(cache.Peek("a"), ShouldEqual, "a")
				addCacheValues("d")
				So(cache, shouldHaveValues, "b", "c", "d")
			})
		})

		Convey(`When adding {a, b, c, d}, "a" will be evicted.`, func() {
			addCacheValues("a", "b", "c", "d")
			So(cache.Len(), ShouldEqual, 3)

			So(cache, shouldHaveValues, "b", "c", "d")

			Convey(`Requests for "a" will be nil.`, func() {
				So(cache.Get("a"), ShouldBeNil)
			})
		})

		Convey(`When adding {a, b, c, a, d}, "b" will be evicted.`, func() {
			addCacheValues("a", "b", "c", "a", "d")
			So(cache.Len(), ShouldEqual, 3)

			So(cache, shouldHaveValues, "a", "c", "d")

			Convey(`When removing "c", will contain {a, d}.`, func() {
				So(cache.Remove("c"), ShouldEqual, "c")
				So(cache, shouldHaveValues, "a", "d")

				Convey(`When adding {e, f}, "a" will be evicted.`, func() {
					addCacheValues("e", "f")
					So(cache, shouldHaveValues, "d", "e", "f")
				})
			})
		})

		Convey(`When removing a value that isn't there, returns nil.`, func() {
			So(cache.Remove("foo"), ShouldBeNil)
		})
	})
}
