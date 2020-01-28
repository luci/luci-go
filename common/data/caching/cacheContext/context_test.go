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
		for i := 0; i < 5; i++ {
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

func runParallelLookupBenchmark(b *testing.B, depth int, cache bool) {
	c := context.Background()
	for i := 0; i <= depth; i++ {
		c = context.WithValue(c, i, i)
	}
	if cache {
		c = Wrap(c)
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Lookup the value up a few times.
			for i := 0; i < 5; i++ {
				v, ok := c.Value(0).(int)
				if !ok {
					b.Fatal("failed to lookup 0")
				}
				if v != 0 {
					b.Fatalf("lookup mismatch (%d != 0)", v)
				}
			}
		}
	})
}

// Example output:
//
// goos: darwin
// goarch: amd64
// pkg: go.chromium.org/luci/common/data/caching/cacheContext
// BenchmarkStandardLookup1-8              	13106932	        90.8 ns/op
// BenchmarkStandardLookup10-8             	 2363426	       518 ns/op
// BenchmarkStandardLookup50-8             	  391365	      2626 ns/op
// BenchmarkStandardLookup1000-8           	   23338	     52907 ns/op
// BenchmarkCachedLookup1-8                	 5748505	       198 ns/op
// BenchmarkCachedLookup10-8               	 6015274	       197 ns/op
// BenchmarkCachedLookup50-8               	 6097803	       194 ns/op
// BenchmarkCachedLookup1000-8             	 6117986	       194 ns/op
// BenchmarkParallelStandardLookup1-8      	47431329	        23.9 ns/op
// BenchmarkParallelStandardLookup10-8     	 9124604	       124 ns/op
// BenchmarkParallelStandardLookup50-8     	 1930999	       613 ns/op
// BenchmarkParallelStandardLookup1000-8   	   95060	     12045 ns/op
// BenchmarkParallelCachedLookup1-8        	 3323910	       362 ns/op
// BenchmarkParallelCachedLookup10-8       	 3183109	       379 ns/op
// BenchmarkParallelCachedLookup50-8       	 3937646	       313 ns/op
// BenchmarkParallelCachedLookup1000-8     	 3606900	       338 ns/op

func BenchmarkStandardLookup1(b *testing.B)    { runLookupBenchmark(b, 1, false) }
func BenchmarkStandardLookup10(b *testing.B)   { runLookupBenchmark(b, 10, false) }
func BenchmarkStandardLookup50(b *testing.B)   { runLookupBenchmark(b, 50, false) }
func BenchmarkStandardLookup1000(b *testing.B) { runLookupBenchmark(b, 1000, false) }

func BenchmarkCachedLookup1(b *testing.B)    { runLookupBenchmark(b, 1, true) }
func BenchmarkCachedLookup10(b *testing.B)   { runLookupBenchmark(b, 10, true) }
func BenchmarkCachedLookup50(b *testing.B)   { runLookupBenchmark(b, 50, true) }
func BenchmarkCachedLookup1000(b *testing.B) { runLookupBenchmark(b, 1000, true) }

func BenchmarkParallelStandardLookup1(b *testing.B)    { runParallelLookupBenchmark(b, 1, false) }
func BenchmarkParallelStandardLookup10(b *testing.B)   { runParallelLookupBenchmark(b, 10, false) }
func BenchmarkParallelStandardLookup50(b *testing.B)   { runParallelLookupBenchmark(b, 50, false) }
func BenchmarkParallelStandardLookup1000(b *testing.B) { runParallelLookupBenchmark(b, 1000, false) }

func BenchmarkParallelCachedLookup1(b *testing.B)    { runParallelLookupBenchmark(b, 1, true) }
func BenchmarkParallelCachedLookup10(b *testing.B)   { runParallelLookupBenchmark(b, 10, true) }
func BenchmarkParallelCachedLookup50(b *testing.B)   { runParallelLookupBenchmark(b, 50, true) }
func BenchmarkParallelCachedLookup1000(b *testing.B) { runParallelLookupBenchmark(b, 1000, true) }
