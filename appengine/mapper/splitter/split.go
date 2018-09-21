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

// Package splitter implements SplitIntoRanges function useful when splitting
// large datastore queries into a bunch of smaller queries with approximately
// evenly-sized result sets.
//
// It is based on __scatter__ magical property. For more info see:
// https://github.com/GoogleCloudPlatform/appengine-mapreduce/wiki/ScatterPropertyImplementation
package splitter

import (
	"math"
	"sort"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
)

// Params are passed to SplitIntoRanges.
//
// See the doc for SplitIntoRanges for more info.
type Params struct {
	Shards             int     // maximal number of key ranges to return
	OversamplingFactor float64 // will fetch OversamplingFactor*Shards random keys
}

// Range represents a range of datastore keys (Start, End].
type Range struct {
	Start *datastore.Key // if nil, then the range represents (0x000..., End]
	End   *datastore.Key // if nil, then the range represents (Start, 0xfff...)
}

// Apply adds >Start and <=End filters to the query and returns the resulting
// query.
func (r Range) Apply(q *datastore.Query) *datastore.Query {
	if r.Start != nil {
		q = q.Gt("__key__", r.Start)
	}
	if r.End != nil {
		q = q.Lte("__key__", r.End)
	}
	return q
}

// SplitIntoRanges returns a list of key ranges (up to 'shards') that together
// cover the results of the provided query.
//
// When all query results are fetched and split between returned ranges, sizes
// of resulting buckets are approximately even.
//
// Internally uses magical entity property __scatter__. It is set on ~0.8% of
// datastore entities. Querying a bunch of entities ordered by __scatter__
// returns a pseudorandom sample of entities that match the query. To improve
// chances of a more even split, we query Shards*OversamplingFactor entities,
// and then pick the split points evenly among them.
//
// If the given query has filters, SplitIntoRanges may need a corresponding
// composite index that includes __scatter__ field.
//
// May return fewer ranges than requested if it detects there are too few
// entities. In extreme case may return single a range (000..., fff...)
// represented by Range with 'Start' and 'End' both set to nil.
func SplitIntoRanges(c context.Context, q *datastore.Query, p Params) ([]Range, error) {
	switch {
	case p.Shards < 1:
		panic("number of shards should be >=1")
	case p.OversamplingFactor < 1.0:
		panic("oversampling factor should be >= 1.0")
	}

	// Don't even bother if requested 1 shard. Return (-inf, +inf).
	if p.Shards == 1 {
		return []Range{{}}, nil
	}

	limit := int32(float64(p.Shards) * p.OversamplingFactor)
	keys := make([]*datastore.Key, 0, limit)

	byScat := q.ClearOrder().
		Order("__scatter__").
		Limit(limit).
		KeysOnly(true)
	if err := datastore.GetAll(c, byScat, &keys); err != nil {
		return nil, err
	}

	// Here keys are ordered by __scatter__ (which is basically random). Reorder
	// keys by, well, key: smallest first.
	sort.Slice(keys, func(i, j int) bool { return keys[i].Less(keys[j]) })

	var splitPoints []*datastore.Key
	if len(keys) < p.Shards {
		// If number of results is less than number of shards, just use one entity
		// per shard (and returns fewer than 'shards' results). In extreme case of
		// empty query, this will return one (-inf, +inf) shard.
		splitPoints = keys
	} else {
		// Otherwise evenly pick the split points among 'keys'. For N shards, there
		// will be N-1 split points. For example, for 6 keys, and 3 shards:
		//
		// * * | * * | * *
		//
		// Since ranges include right boundary, the split points would be:
		//
		// * [*] * [*] * *
		//
		// This we'll pick the split point left to the float location of where
		// the split should be.
		//
		// When calculating 'stride' we use len(keys)-1/shards because we want the
		// (float) split location to be "between" points. E.g for the case of 6
		// points and 2 shards, the split location should be 2.5:
		//
		// *   *   *   |   *   *   *
		// 0   1   2  2.5  3   4   5
		splitPoints = make([]*datastore.Key, p.Shards-1)
		stride := float64(len(keys)-1) / float64(p.Shards)
		for i := 0; i < len(splitPoints); i++ {
			idx := int(math.Floor(stride*float64(i) + stride))
			splitPoints[i] = keys[idx]
		}
	}

	// Use the calculated points to divides 'keys' into non-intersecting ranges
	// that also cover (-inf, ...) and (, +inf). In extreme keys of 0 split
	// points, the result would be single (-inf, +inf) range.
	ranges := make([]Range, 0, len(splitPoints)+1)
	var prev *datastore.Key
	for _, k := range splitPoints {
		ranges = append(ranges, Range{
			Start: prev,
			End:   k,
		})
		prev = k
	}
	ranges = append(ranges, Range{
		Start: prev,
		End:   nil,
	})
	return ranges, nil
}
