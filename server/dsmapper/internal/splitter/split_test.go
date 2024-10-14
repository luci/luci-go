// Copyright 2018 The LUCI Authors.
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

package splitter

import (
	"context"
	"math/rand"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

type RootEntity struct {
	ID int `gae:"$id"`
}

type IntEntity struct {
	ID     int            `gae:"$id"`
	Parent *datastore.Key `gae:"$parent"`
}

type StringEntity struct {
	ID     string         `gae:"$id"`
	Parent *datastore.Key `gae:"$parent"`
}

func putIntEntities(ctx context.Context, parent *datastore.Key, ids []int) {
	e := make([]*IntEntity, len(ids))
	for i, id := range ids {
		e[i] = &IntEntity{
			ID:     id,
			Parent: parent,
		}
	}
	if err := datastore.Put(ctx, e); err != nil {
		panic(err)
	}
}

type intRange struct {
	start int
	end   int
	size  int
}

func extractRanges(ranges []Range, max int) []intRange {
	out := make([]intRange, len(ranges))
	for i, rng := range ranges {
		r := intRange{-1, -1, 0}
		if rng.Start != nil {
			r.start = int(rng.Start.IntID())
		}
		if rng.End != nil {
			r.end = int(rng.End.IntID())
		}
		switch {
		case r.start != -1 && r.end != -1:
			r.size = r.end - r.start
		case max != 0 && r.start == -1 && r.end == -1:
			r.size = max
		case r.start == -1:
			r.size = r.end
		case max != 0 && r.end == -1:
			r.size = max - r.start
		}
		out[i] = r
	}
	return out
}

func countRanges(ctx context.Context, q *datastore.Query, ranges []Range) []int {
	out := make([]int, len(ranges))
	for i, r := range ranges {
		count, err := datastore.Count(ctx, r.Apply(q))
		if err != nil {
			panic(err)
		}
		out[i] = int(count)
	}
	return out
}

func TestSplitIntoRanges(t *testing.T) {
	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		rnd := rand.New(rand.NewSource(1))

		datastore.GetTestable(ctx).AutoIndex(true)

		getRanges := func(q *datastore.Query, shards, samples, max int) []intRange {
			ranges, err := SplitIntoRanges(ctx, q, Params{
				Shards:  shards,
				Samples: samples,
			})
			assert.Loosely(t, err, should.BeNil)
			return extractRanges(ranges, max)
		}

		getShardSizes := func(q *datastore.Query, shards, samples int) []int {
			ranges, err := SplitIntoRanges(ctx, q, Params{
				Shards:  shards,
				Samples: samples,
			})
			assert.Loosely(t, err, should.BeNil)
			return countRanges(ctx, q, ranges)
		}

		t.Run("With mostly empty query", func(t *ftt.Test) {
			putIntEntities(ctx, nil, []int{1, 2, 3, 4, 5, 6})
			maxID := 6
			datastore.GetTestable(ctx).CatchupIndexes()

			q := datastore.NewQuery("IntEntity")

			// Special case for 1 shard.
			assert.Loosely(t, getRanges(q, 1, 1.0, maxID), should.Resemble([]intRange{
				{-1, -1, 6},
			}))

			// Not enough split points for 4 shards. Got only 2.
			assert.Loosely(t, getRanges(q, 4, 1.0, maxID), should.Resemble([]intRange{
				{-1, 4, 4},
				{4, -1, 2},
			}))
		})

		t.Run("With evenly distributed int keys", func(t *ftt.Test) {
			ints := []int{}
			maxID := 1024
			for i := 0; i < maxID; i++ {
				ints = append(ints, i)
			}
			putIntEntities(ctx, nil, ints)
			datastore.GetTestable(ctx).CatchupIndexes()

			q := datastore.NewQuery("IntEntity")

			// 2 shards. With oversampling we get better accuracy (the middle point is
			// closer to 512).
			assert.Loosely(t, getRanges(q, 2, 2, maxID), should.Resemble([]intRange{
				{-1, 399, 399},
				{399, -1, 625},
			}))
			assert.Loosely(t, getRanges(q, 2, 64, maxID), should.Resemble([]intRange{
				{-1, 476, 476},
				{476, -1, 548},
			}))
			assert.Loosely(t, getRanges(q, 2, 256, maxID), should.Resemble([]intRange{
				{-1, 489, 489},
				{489, -1, 535},
			}))

			// 3 shards. With oversampling we get better accuracy (shard size is close
			// to 340, shards are more even).
			assert.Loosely(t, getRanges(q, 3, 3, maxID), should.Resemble([]intRange{
				{-1, 265, 265},
				{265, 399, 134},
				{399, -1, 625},
			}))
			assert.Loosely(t, getRanges(q, 3, 96, maxID), should.Resemble([]intRange{
				{-1, 285, 285},
				{285, 696, 411},
				{696, -1, 328},
			}))
			assert.Loosely(t, getRanges(q, 3, 384, maxID), should.Resemble([]intRange{
				{-1, 327, 327},
				{327, 658, 331},
				{658, -1, 366},
			}))
		})

		t.Run("With normally distributed int keys", func(t *ftt.Test) {
			ints := []int{}
			seen := map[int]bool{}
			for i := 0; i < 1000; i++ {
				key := int(rnd.NormFloat64()*2000 + 50000)
				if key > 0 && !seen[key] {
					ints = append(ints, key)
					seen[key] = true
				}
			}
			putIntEntities(ctx, nil, ints)
			datastore.GetTestable(ctx).CatchupIndexes()

			q := datastore.NewQuery("IntEntity")

			// Had fewer distinct points generated (due to rnd giving duplicates).
			// All points are present in the single shard if not really sharding.
			assert.Loosely(t, len(seen), should.Equal(937))
			assert.Loosely(t, getShardSizes(q, 1, 1), should.Resemble([]int{len(seen)}))

			// Shards have ~ even sizes (in terms of number of points there).
			shards := getShardSizes(q, 10, 320)
			assert.Loosely(t, shards, should.Resemble([]int{
				80, 97, 105, 80, 92, 95, 106, 103, 87, 92,
			}))

			// But key ranges are very different, reflecting distribution of points.
			assert.Loosely(t, getRanges(q, 10, 320, 0), should.Resemble([]intRange{
				{-1, 47124, 47124},
				{47124, 48158, 1034},
				{48158, 48925, 767},
				{48925, 49363, 438},
				{49363, 49871, 508},
				{49871, 50352, 481},
				{50352, 50973, 621},
				{50973, 51677, 704},
				{51677, 52573, 896},
				{52573, -1, 0},
			}))

			// All points are counted.
			total := 0
			for _, s := range shards {
				total += s
			}
			assert.Loosely(t, total, should.Equal(len(seen)))
		})

		t.Run("Handles ancestor filter", func(t *ftt.Test) {
			root1 := datastore.KeyForObj(ctx, &RootEntity{ID: 1})
			root2 := datastore.KeyForObj(ctx, &RootEntity{ID: 2})

			putIntEntities(ctx, root1, []int{1, 2, 3, 4})
			putIntEntities(ctx, root2, []int{1, 2, 3, 4, 5, 6, 7, 8})
			datastore.GetTestable(ctx).CatchupIndexes()

			q := datastore.NewQuery("IntEntity")

			// Non-ancestor query discovers all 12 entities.
			assert.Loosely(t, getShardSizes(q, 4, 128), should.Resemble([]int{
				2, 2, 4, 4,
			}))

			// With ancestor query, discovers only entities that match the query.
			assert.Loosely(t, getShardSizes(q.Ancestor(root1), 4, 128), should.Resemble([]int{
				1, 1, 2, 0, // 4 total
			}))
			assert.Loosely(t, getShardSizes(q.Ancestor(root2), 4, 128), should.Resemble([]int{
				4, 1, 2, 1, // 8 total
			}))
		})

		t.Run("Handles arbitrary keys", func(t *ftt.Test) {
			entities := make([]any, 1000)
			for i := 0; i < len(entities); i++ {
				blob := make([]byte, 10)
				_, err := rnd.Read(blob)
				assert.Loosely(t, err, should.BeNil)
				entities[i] = &StringEntity{
					ID:     string(blob),
					Parent: datastore.KeyForObj(ctx, &RootEntity{ID: rnd.Intn(10) + 1}),
				}
			}
			assert.Loosely(t, datastore.Put(ctx, entities), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			q := datastore.NewQuery("StringEntity")

			// Discovers all 1000 entities, split ~ evenly between shards.
			assert.Loosely(t, getShardSizes(q, 8, 256), should.Resemble([]int{
				115, 114, 113, 110, 133, 133, 148, 134,
			}))
		})
	})
}
