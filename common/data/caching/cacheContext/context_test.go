// Copyright 2016 The LUCI Authors.
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

package cacheContext

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCacheContext(t *testing.T) {
	t.Parallel()

	Convey(`A caching Context populated with values`, t, func() {
		const count = 3
		c := context.Background()
		for i := 0; i < count; i++ {
			c = context.WithValue(c, i, i)
		}
		c = Wrap(c)

		Convey(`Successfully caches values.`, func() {
			// Perform initial lookup to cache.
			for i := 0; i < count; i++ {
				_ = c.Value(i)
			}

			// Cache absence of value.
			c.Value("missing")

			// Clear the Context. Any passthrough calls will now panic.
			c.(*cacheContext).Context = nil
			So(func() { c.Value("not cached") }, ShouldPanic)

			// Confirm that lookups happen from cache.
			for i := 0; i < count; i++ {
				So(c.Value(i), ShouldEqual, i)
			}
			So(c.Value("missing"), ShouldBeNil)
		})

		Convey(`Will not double-wrap.`, func() {
			So(Wrap(c), ShouldEqual, c)
		})
	})
}

func runLookupBenchmark(b *testing.B, depth int, cache bool) {
	c := context.Background()
	for i := 0; i <= depth; i++ {
		c = context.WithValue(c, i, i)
	}
	if cache {
		c = Wrap(c)
	}

	for round := 0; round < b.N; round++ {
		// Lookup the value up a few times.
		for i := 0; i < 10; i++ {
			v, ok := c.Value(0).(int)
			if !ok {
				b.Fatal("failed to lookup 0")
			}
			if v != 0 {
				b.Fatalf("lookup mismatch (%d != 0)", v)
			}
		}
	}
}

func BenchmarkStandardLookup1(b *testing.B)    { runLookupBenchmark(b, 1, false) }
func BenchmarkStandardLookup10(b *testing.B)   { runLookupBenchmark(b, 10, false) }
func BenchmarkStandardLookup50(b *testing.B)   { runLookupBenchmark(b, 50, false) }
func BenchmarkStandardLookup1000(b *testing.B) { runLookupBenchmark(b, 1000, false) }

func BenchmarkCachedLookup1(b *testing.B)    { runLookupBenchmark(b, 1, true) }
func BenchmarkCachedLookup10(b *testing.B)   { runLookupBenchmark(b, 10, true) }
func BenchmarkCachedLookup50(b *testing.B)   { runLookupBenchmark(b, 50, true) }
func BenchmarkCachedLookup1000(b *testing.B) { runLookupBenchmark(b, 1000, true) }
