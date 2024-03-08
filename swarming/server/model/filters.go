// Copyright 2024 The LUCI Authors.
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

package model

import (
	"fmt"
	"sort"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

// SplitMode is a parameter for SplitForQuery and Apply methods.
type SplitMode int

const (
	// SplitOptimally indicates to make as few split as possible.
	//
	// Some queries may end up using "OR" filters, but no more than one such
	// filter per query. Such queries are still accepted by the datastore.
	SplitOptimally SplitMode = 0

	// SplitCompletely indicates to split a filter into elementary filters.
	//
	// Elementary filters do not have "OR" in them. This is used in testing to
	// cover code paths that merge results of multiple queries. This is needed
	// because the local testing environment current (as of Jan 2024) doesn't
	// actually support OR queries at all.
	SplitCompletely SplitMode = 1
)

// Filter represents a filter over the space of ["key:value"] tags.
//
// Conceptually it is a list of AND'ed together checks on values of tags. Each
// such check compares each value of some particular tag to a set of allowed
// values (often just one). The same tag key is allowed to show up more than
// once. In that case there will be more than one filter on values of this tag
// (see the example below).
//
// In API this filter is encoded by a list of `key:val1|val2|val3` pairs, where
// keys are allowed to be repeated.
//
// For example, this filter:
//
//	["os:Linux", "os:Ubuntu", "zone:us-central|us-east"]
//
// Will match entities with following tags:
//
//	["os:Linux", "os:Ubuntu", "os:Ubuntu-20", "zone:us-central"]
//	["os:Linux", "os:Ubuntu", "os:Ubuntu-22", "zone:us-easy"]
//
// But it will not match these entities:
//
//	["os:Linux", "os:Debian", "zone:us-central"]
//	["os:Linux", "os:Ubuntu", "os:Ubuntu-22", "zone:us-west"]
type Filter struct {
	filters []perKeyFilter // sorted by key
}

// perKeyFilter is a filter that checks the value of a single tag key.
type perKeyFilter struct {
	key    string   // the tag key to check
	values []string // allowed values (no dups, sorted)
}

// NewFilter parses a list of `("key", "val1|val2|val2")` pairs.
//
// Empty filter is possible (if `tags` are empty).
func NewFilter(tags []*apipb.StringPair) (Filter, error) {
	filter := Filter{
		filters: make([]perKeyFilter, 0, len(tags)),
	}

	for _, tag := range tags {
		if strings.TrimSpace(tag.Key) != tag.Key || tag.Key == "" {
			return filter, errors.Reason("bad key %q", tag.Key).Err()
		}

		vals := strings.Split(tag.Value, "|")
		deduped := stringset.New(len(vals))
		for _, val := range vals {
			if strings.TrimSpace(val) != val || val == "" {
				return filter, errors.Reason("bad value for key %q: %q", tag.Key, tag.Value).Err()
			}
			deduped.Add(val)
		}

		filter.filters = append(filter.filters, perKeyFilter{
			key:    tag.Key,
			values: deduped.ToSortedSlice(),
		})
	}

	sort.SliceStable(filter.filters, func(i, j int) bool {
		return filter.filters[i].key < filter.filters[j].key
	})

	return filter, nil
}

// Pools is a list of all pools mentioned in the filter (if any).
func (f Filter) Pools() []string {
	pools := stringset.New(1) // there's usually only 1 pool
	for _, f := range f.filters {
		if f.key == "pool" {
			pools.AddAll(f.values)
		}
	}
	return pools.ToSortedSlice()
}

// IsEmpty is true if this filter doesn't filter anything.
func (f Filter) IsEmpty() bool {
	return len(f.filters) == 0
}

// SplitForQuery splits this filter into several simpler filters that can be
// used in datastore queries, with their results merged.
//
// The unsplit filter is generally too complex for the datastore query planner
// to handle using existing indexes (e.g. an index on `dimensions_flat` and
// a composite index on `(dimensions_flat, composite)` pair when used for
// BotInfo queries).
//
// Unfortunately due to datastore limits we can't just add all necessary
// composite indexes (like `(dimensions_flat, dimensions_flat, composite)` one).
// Since `dimensions_flat` is a repeated property, this results in too many
// indexed permutations of values, blowing up this index. Possible workarounds
// require changing the layout of BotInfo entities in datastore, but that would
// require imposing limits on public Swarming API (basically, we'll need to
// predefine what dimension keys are worth indexing and what are not; currently
// all are indexed).
//
// Instead we split the query into N subqueries, run them in parallel and merge
// results locally. This is relatively expensive and scales poorly, but we need
// to do that only for complex queries that use multiple OR property filters.
// They are relatively rare.
//
// If the original filter is empty, returns one empty filter as the output.
func (f Filter) SplitForQuery(mode SplitMode) []Filter {
	// Count how many OR-ed property filters we have, find the smallest one. We'll
	// use it as a "pivot" for splitting the original filter into smaller filters.
	// That way we'll have the smallest number of splits.
	multiValCount := 0
	pivotIdx := 0
	for idx, filter := range f.filters {
		if vals := len(filter.values); vals > 1 {
			multiValCount += 1
			if multiValCount == 1 || vals < len(f.filters[pivotIdx].values) {
				pivotIdx = idx
			}
		}
	}

	var maxMultiVal int
	switch mode {
	case SplitOptimally:
		maxMultiVal = 1 // support at most one OR property filter
	case SplitCompletely:
		maxMultiVal = 0 // support no OR property filters at all
	default:
		panic(fmt.Sprintf("unknown split mode %d", mode))
	}
	if multiValCount <= maxMultiVal {
		return []Filter{f}
	}

	// Split into simpler filters around the pivot eliminating this particular OR.
	// Keep simplifying the result recursively until we get a list of filters
	// where each one can be handled by the datastore natively.
	pivotVals := f.filters[pivotIdx].values
	simplified := make([]Filter, 0, len(pivotVals))
	for _, pivotVal := range pivotVals {
		subfilter := Filter{
			filters: make([]perKeyFilter, 0, len(f.filters)),
		}
		for idx, filter := range f.filters {
			if idx == pivotIdx {
				// Pivot! Pivot!
				subfilter.filters = append(subfilter.filters, perKeyFilter{
					key:    filter.key,
					values: []string{pivotVal},
				})
			} else {
				subfilter.filters = append(subfilter.filters, filter)
			}
		}
		simplified = append(simplified, subfilter.SplitForQuery(mode)...)
	}

	return simplified
}

// Apply applies this filter to a query, returning (potentially) multiple
// queries.
//
// Results of these queries must be merged locally (e.g. via datastore.RunMulti)
// to get the final filtered result.
//
// `field` is the datastore entity field to apply the filter on. It should be
// a multi-valued field with values of form "key:value".
//
// If the filter is empty, returns a list with the original query as is.
func (f Filter) Apply(q *datastore.Query, field string, mode SplitMode) []*datastore.Query {
	split := f.SplitForQuery(mode)
	out := make([]*datastore.Query, 0, len(split))
	for _, simpleFilter := range split {
		simpleQ := q
		for _, f := range simpleFilter.filters {
			if len(f.values) == 1 {
				simpleQ = simpleQ.Eq(field, fmt.Sprintf("%s:%s", f.key, f.values[0]))
			} else {
				pairs := make([]any, len(f.values))
				for i, v := range f.values {
					pairs[i] = fmt.Sprintf("%s:%s", f.key, v)
				}
				simpleQ = simpleQ.In(field, pairs...)
			}
		}
		out = append(out, simpleQ)
	}
	return out
}
