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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCacheContext(t *testing.T) {
	t.Parallel()

	ftt.Run(`A caching Context populated with values`, t, func(t *ftt.Test) {
		const count = 3
		c := context.Background()
		for i := 0; i < count; i++ {
			c = context.WithValue(c, i, i)
		}
		c = Wrap(c)

		t.Run(`Successfully caches values.`, func(t *ftt.Test) {
			// Perform initial lookup to cache.
			for i := 0; i < count; i++ {
				_ = c.Value(i)
			}

			// Cache absence of value.
			c.Value("missing")

			// Clear the Context. Any passthrough calls will now panic.
			c.(*cacheContext).Context = nil
			assert.Loosely(t, func() { c.Value("not cached") }, should.Panic)

			// Confirm that lookups happen from cache.
			for i := 0; i < count; i++ {
				assert.Loosely(t, c.Value(i), should.Equal(i))
			}
			assert.Loosely(t, c.Value("missing"), should.BeNil)
		})

		t.Run(`Will not double-wrap.`, func(t *ftt.Test) {
			assert.Loosely(t, Wrap(c), should.Equal(c))
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

// Results of running `go test -bench . -cpu=8`:
//
// BenchmarkStandardLookup1-8              12789495      92.0 ns/op
// BenchmarkStandardLookup10-8              2374756       511 ns/op
// BenchmarkStandardLookup50-8               401910      2552 ns/op
// BenchmarkStandardLookup1000-8              22759     52650 ns/op
// BenchmarkCachedLookup1-8                 5957251       204 ns/op
// BenchmarkCachedLookup10-8                5783949       203 ns/op
// BenchmarkCachedLookup50-8                5679669       205 ns/op
// BenchmarkCachedLookup1000-8              5805810       201 ns/op
// BenchmarkParallelStandardLookup1-8      44769181      23.4 ns/op
// BenchmarkParallelStandardLookup10-8      7837178       137 ns/op
// BenchmarkParallelStandardLookup50-8      1808611       612 ns/op
// BenchmarkParallelStandardLookup1000-8      92068     12511 ns/op
// BenchmarkParallelCachedLookup1-8         4062573       300 ns/op
// BenchmarkParallelCachedLookup10-8        3488845       353 ns/op
// BenchmarkParallelCachedLookup50-8        3603590       323 ns/op
// BenchmarkParallelCachedLookup1000-8      3482008       351 ns/op

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
