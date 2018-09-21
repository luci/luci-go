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
	"math/rand"
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
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

func putIntEntities(c context.Context, parent *datastore.Key, ids []int) {
	e := make([]*IntEntity, len(ids))
	for i, id := range ids {
		e[i] = &IntEntity{
			ID:     id,
			Parent: parent,
		}
	}
	if err := datastore.Put(c, e); err != nil {
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

func countRanges(c context.Context, q *datastore.Query, ranges []Range) []int {
	out := make([]int, len(ranges))
	for i, r := range ranges {
		count, err := datastore.Count(c, r.Apply(q))
		if err != nil {
			panic(err)
		}
		out[i] = int(count)
	}
	return out
}

func TestSplitIntoRanges(t *testing.T) {
	Convey("Works", t, func() {
		c := memory.Use(context.Background())
		rnd := rand.New(rand.NewSource(1))

		datastore.GetTestable(c).AutoIndex(true)

		getRanges := func(q *datastore.Query, shards, samples, max int) []intRange {
			ranges, err := SplitIntoRanges(c, q, Params{
				Shards:  shards,
				Samples: samples,
			})
			So(err, ShouldBeNil)
			return extractRanges(ranges, max)
		}

		getShardSizes := func(q *datastore.Query, shards, samples int) []int {
			ranges, err := SplitIntoRanges(c, q, Params{
				Shards:  shards,
				Samples: samples,
			})
			So(err, ShouldBeNil)
			return countRanges(c, q, ranges)
		}

		Convey("With mostly empty query", func() {
			putIntEntities(c, nil, []int{1, 2, 3, 4, 5, 6})
			maxID := 6
			datastore.GetTestable(c).CatchupIndexes()

			q := datastore.NewQuery("IntEntity")

			// Special case for 1 shard.
			So(getRanges(q, 1, 1.0, maxID), ShouldResemble, []intRange{
				{-1, -1, 6},
			})

			// Not enough split points for 4 shards. Got only 2.
			So(getRanges(q, 4, 1.0, maxID), ShouldResemble, []intRange{
				{-1, 4, 4},
				{4, -1, 2},
			})
		})

		Convey("With evenly distributed int keys", func() {
			ints := []int{}
			maxID := 1024
			for i := 0; i < maxID; i++ {
				ints = append(ints, i)
			}
			putIntEntities(c, nil, ints)
			datastore.GetTestable(c).CatchupIndexes()

			q := datastore.NewQuery("IntEntity")

			// 2 shards. With oversampling we get better accuracy (the middle point is
			// closer to 512).
			So(getRanges(q, 2, 2, maxID), ShouldResemble, []intRange{
				{-1, 399, 399},
				{399, -1, 625},
			})
			So(getRanges(q, 2, 64, maxID), ShouldResemble, []intRange{
				{-1, 476, 476},
				{476, -1, 548},
			})
			So(getRanges(q, 2, 256, maxID), ShouldResemble, []intRange{
				{-1, 489, 489},
				{489, -1, 535},
			})

			// 3 shards. With oversampling we get better accuracy (shard size is close
			// to 340, shards are more even).
			So(getRanges(q, 3, 3, maxID), ShouldResemble, []intRange{
				{-1, 265, 265},
				{265, 399, 134},
				{399, -1, 625},
			})
			So(getRanges(q, 3, 96, maxID), ShouldResemble, []intRange{
				{-1, 285, 285},
				{285, 696, 411},
				{696, -1, 328},
			})
			So(getRanges(q, 3, 384, maxID), ShouldResemble, []intRange{
				{-1, 327, 327},
				{327, 658, 331},
				{658, -1, 366},
			})
		})

		Convey("With normally distributed int keys", func() {
			ints := []int{}
			seen := map[int]bool{}
			for i := 0; i < 1000; i++ {
				key := int(rnd.NormFloat64()*2000 + 50000)
				if key > 0 && !seen[key] {
					ints = append(ints, key)
					seen[key] = true
				}
			}
			putIntEntities(c, nil, ints)
			datastore.GetTestable(c).CatchupIndexes()

			q := datastore.NewQuery("IntEntity")

			// Had fewer distinct points generated (due to rnd giving duplicates).
			// All points are present in the single shard if not really sharding.
			So(len(seen), ShouldEqual, 937)
			So(getShardSizes(q, 1, 1), ShouldResemble, []int{len(seen)})

			// Shards have ~ even sizes (in terms of number of points there).
			shards := getShardSizes(q, 10, 320)
			So(shards, ShouldResemble, []int{
				80, 97, 105, 80, 92, 95, 106, 103, 87, 92,
			})

			// But key ranges are very different, reflecting distribution of points.
			So(getRanges(q, 10, 320, 0), ShouldResemble, []intRange{
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
			})

			// All points are counted.
			total := 0
			for _, s := range shards {
				total += s
			}
			So(total, ShouldEqual, len(seen))
		})

		Convey("Handles ancestor filter", func() {
			root1 := datastore.KeyForObj(c, &RootEntity{ID: 1})
			root2 := datastore.KeyForObj(c, &RootEntity{ID: 2})

			putIntEntities(c, root1, []int{1, 2, 3, 4})
			putIntEntities(c, root2, []int{1, 2, 3, 4, 5, 6, 7, 8})
			datastore.GetTestable(c).CatchupIndexes()

			q := datastore.NewQuery("IntEntity")

			// Non-ancestor query discovers all 12 entities.
			So(getShardSizes(q, 4, 128), ShouldResemble, []int{
				2, 2, 4, 4,
			})

			// With ancestor query, discovers only entities that match the query.
			So(getShardSizes(q.Ancestor(root1), 4, 128), ShouldResemble, []int{
				1, 1, 2, 0, // 4 total
			})
			So(getShardSizes(q.Ancestor(root2), 4, 128), ShouldResemble, []int{
				4, 1, 2, 1, // 8 total
			})
		})

		Convey("Handles arbitrary keys", func() {
			entities := make([]interface{}, 1000)
			for i := 0; i < len(entities); i++ {
				blob := make([]byte, 10)
				_, err := rnd.Read(blob)
				So(err, ShouldBeNil)
				entities[i] = &StringEntity{
					ID:     string(blob),
					Parent: datastore.KeyForObj(c, &RootEntity{ID: rnd.Intn(10) + 1}),
				}
			}
			So(datastore.Put(c, entities), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			q := datastore.NewQuery("StringEntity")

			// Discovers all 1000 entities, split ~ evenly between shards.
			So(getShardSizes(q, 8, 256), ShouldResemble, []int{
				115, 114, 113, 110, 133, 133, 148, 134,
			})
		})
	})
}
