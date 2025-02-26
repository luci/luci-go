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
	"iter"
	"slices"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/validate"
)

const (
	// MaxDimensionChecks is how many elementary dimension predicates are allowed
	// in total across all keys in task dimensions.
	//
	// This is a measure of size of the datastore index required for indexing
	// tasks with these dimensions.
	MaxDimensionChecks = 512

	// MaxCombinatorialAlternatives is how many terms are allowed in the
	// disjunctive normal form of the dimensions constraint predicate.
	//
	// E.g. predicate `(k1 in [v1, v2]) && (k2 in [v3, v4])` has 4 terms in
	// its DNF:
	//	(k1 == v1 && k2 == v3) ||
	//	(k1 == v1 && k2 == v4) ||
	//	(k1 == v2 && k2 == v3) ||
	//	(k1 == v2 && k2 == v4)
	//
	// This is a measure of complexity of the datastore query necessary to fetch
	// all tasks matching a particular filter.
	MaxCombinatorialAlternatives = 8
)

// FilterValidation defines how strictly to validated a filter.
type FilterValidation int

const (
	// ValidateAsTags indicates to use relaxed validation rules.
	//
	// Used for filters on arbitrary task tags.
	ValidateAsTags FilterValidation = 0

	// ValidateAsDimensions indicates to use more strict validation rules.
	//
	// Used for filters representing task dimensions.
	ValidateAsDimensions FilterValidation = 1

	// ValidateMinimally indicates to use almost no validation.
	//
	// Useful if it's known the filter was validated already.
	ValidateMinimally FilterValidation = 2
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
// Empty filter is possible (if `tags` are empty). If `allowDups` is true, will
// silently deduplicate redundant values. Otherwise their presence will result
// in an error.
func NewFilter(pairs []*apipb.StringPair, validation FilterValidation, allowDups bool) (Filter, error) {
	filter := Filter{filters: make([]perKeyFilter, 0, len(pairs))}
	err := initFilter(&filter, validation, allowDups, slices.Values(pairs))
	if err != nil {
		return filter, err
	}
	return filter, nil
}

// NewFilterFromTags is a variant of NewFilter that takes "k:v" pairs as input
// and parses them as task tags.
func NewFilterFromTags(tags []string) (Filter, error) {
	filter := Filter{filters: make([]perKeyFilter, 0, len(tags))}
	err := initFilter(&filter, ValidateAsTags, true, func(yield func(*apipb.StringPair) bool) {
		for _, tag := range tags {
			parts := strings.SplitN(tag, ":", 2)
			if !yield(&apipb.StringPair{Key: parts[0], Value: parts[1]}) {
				return
			}
		}
	})
	if err != nil {
		return filter, err
	}
	return filter, nil
}

// NewFilterFromTaskDimensions is a variant of NewFilter that parses given task
// dimensions.
func NewFilterFromTaskDimensions(dims TaskDimensions) (Filter, error) {
	filter := Filter{filters: make([]perKeyFilter, 0, len(dims))}
	err := initFilter(&filter, ValidateAsDimensions, false, func(yield func(*apipb.StringPair) bool) {
		for key, vals := range dims {
			for _, val := range vals {
				if !yield(&apipb.StringPair{Key: key, Value: val}) {
					return
				}
			}
		}
	})
	if err != nil {
		return filter, err
	}
	return filter, nil
}

// initFilter is actual implementation of NewFilter.
func initFilter(filter *Filter, validation FilterValidation, allowDups bool, pairs iter.Seq[*apipb.StringPair]) error {
	var validateKey func(key string) error
	var validateVal func(val string) error
	switch validation {
	case ValidateAsTags:
		validateKey = func(key string) error {
			if key == "" {
				return errors.Reason("cannot be empty").Err()
			}
			if strings.TrimSpace(key) != key {
				return errors.Reason("should have no leading or trailing spaces").Err()
			}
			return nil
		}
		validateVal = validateKey // the same rules

	case ValidateAsDimensions:
		validateKey = validate.DimensionKey
		validateVal = validate.DimensionValue

	case ValidateMinimally:
		validateKey = func(key string) error { return nil }
		validateVal = func(val string) error { return nil }

	default:
		panic("unknown FilterValidation mode")
	}

	for pair := range pairs {
		if err := validateKey(pair.Key); err != nil {
			return errors.Annotate(err, "bad key %q", pair.Key).Err()
		}

		vals := strings.Split(pair.Value, "|")
		deduped := stringset.New(len(vals))
		for _, val := range vals {
			if err := validateVal(val); err != nil {
				return errors.Annotate(err, "key %q: value %q", pair.Key, val).Err()
			}
			deduped.Add(val)
		}

		if len(deduped) != len(vals) && !allowDups {
			return errors.Reason("key %q has repeated values", pair.Key).Err()
		}

		filter.filters = append(filter.filters, perKeyFilter{
			key:    pair.Key,
			values: deduped.ToSortedSlice(),
		})
	}

	slices.SortFunc(filter.filters, func(a, b perKeyFilter) int {
		if n := strings.Compare(a.key, b.key); n != 0 {
			return n
		}
		return slices.Compare(a.values, b.values)
	})

	dedupped := slices.CompactFunc(filter.filters, func(a, b perKeyFilter) bool {
		return a.key == b.key && slices.Equal(a.values, b.values)
	})
	if len(dedupped) != len(filter.filters) && !allowDups {
		return errors.Reason("has duplicate constraints").Err()
	}
	filter.filters = dedupped

	return nil
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

// NarrowToKey returns a subset of the filter that applies to the given key.
func (f Filter) NarrowToKey(key string) Filter {
	var out []perKeyFilter
	for _, f := range f.filters {
		if f.key == key {
			out = append(out, f)
		}
	}
	return Filter{filters: out}
}

// Pairs enumerates all "k:v" pairs used by the filter.
//
// May have duplicates if the same pair appears in two different predicates.
//
// Note this looses some information: two different filters `("k:v1|v2")` and
// `("k:v1", "k:v2")` will result in the same list of pairs.
func (f Filter) Pairs() iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, f := range f.filters {
			for _, v := range f.values {
				if !yield(f.key + ":" + v) {
					return
				}
			}
		}
	}
}

// PairCount is how many "k:v" pairs are present in the filter.
func (f Filter) PairCount() int {
	count := 0
	for _, f := range f.filters {
		count += len(f.values)
	}
	return count
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

// ValidateComplexity returns an error if this filter is too complex to be used
// as task dimensions.
//
// This is a precaution against submitting tasks that end up creating a lot of
// datastore indexes or put a strain on the RBE scheduler.
func (f Filter) ValidateComplexity() error {
	total := 0
	combinatorialAlternatives := 1
	for _, kf := range f.filters {
		total += len(kf.values)
		combinatorialAlternatives *= len(kf.values)
	}
	if total > MaxDimensionChecks {
		return errors.Reason("too many dimension constraints %d (max is %d)", total, MaxDimensionChecks).Err()
	}
	if combinatorialAlternatives > MaxCombinatorialAlternatives {
		return errors.Reason("too many combinations of dimensions %d (max is %d), reduce usage of \"|\"", combinatorialAlternatives, MaxCombinatorialAlternatives).Err()
	}
	return nil
}
