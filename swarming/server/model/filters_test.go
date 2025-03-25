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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(Filter{}))
	registry.RegisterCmpOption(cmp.AllowUnexported(perKeyFilter{}))
}

func TestFilter(t *testing.T) {
	t.Parallel()

	ftt.Run("Ok", t, func(t *ftt.Test) {
		f, err := NewFilter([]*apipb.StringPair{
			{Key: "x", Value: "c|b|a"},
			{Key: "x", Value: "y"},
			{Key: "pool", Value: "P1"},
			{Key: "pool", Value: "P1|P2|P3"},
		}, ValidateAsTags, false)
		assert.NoErr(t, err)
		assert.That(t, f, should.Match(Filter{
			filters: []perKeyFilter{
				{key: "pool", values: []string{"P1"}},
				{key: "pool", values: []string{"P1", "P2", "P3"}},
				{key: "x", values: []string{"a", "b", "c"}},
				{key: "x", values: []string{"y"}},
			},
		}))
		assert.That(t, f.Pools(), should.Match([]string{"P1", "P2", "P3"}))
	})

	ftt.Run("NewFilterFromTags", t, func(t *ftt.Test) {
		f, err := NewFilterFromTags([]string{
			"k1:v1",
			"k1:v1|v1", // skipped
			"k1:v2|v1",
			"k2:v1",
		})
		assert.NoErr(t, err)
		assert.That(t, f, should.Match(Filter{
			filters: []perKeyFilter{
				{key: "k1", values: []string{"v1"}},
				{key: "k1", values: []string{"v1", "v2"}},
				{key: "k2", values: []string{"v1"}},
			},
		}))
	})

	ftt.Run("NewFilterFromTaskDimensions", t, func(t *ftt.Test) {
		f, err := NewFilterFromTaskDimensions(TaskDimensions{
			"k1": {"v1", "v2|v1"},
			"k2": {"v1"},
		})
		assert.NoErr(t, err)
		assert.That(t, f, should.Match(Filter{
			filters: []perKeyFilter{
				{key: "k1", values: []string{"v1"}},
				{key: "k1", values: []string{"v1", "v2"}},
				{key: "k2", values: []string{"v1"}},
			},
		}))
	})

	ftt.Run("ValidateAsTags errors", t, func(t *ftt.Test) {
		call := func(k, v string) error {
			_, err := NewFilter([]*apipb.StringPair{
				{Key: k, Value: v},
			}, ValidateAsTags, false)
			return err
		}
		assert.That(t, call("", "val"), should.ErrLike("bad key"))
		assert.That(t, call("  key", "val"), should.ErrLike("bad key"))
		assert.That(t, call("key", ""), should.ErrLike("cannot be empty"))
		assert.That(t, call("key", "  val"), should.ErrLike("should have no leading or trailing spaces"))
	})

	ftt.Run("ValidateAsDimensions errors", t, func(t *ftt.Test) {
		call := func(k, v string) error {
			_, err := NewFilter([]*apipb.StringPair{
				{Key: k, Value: v},
			}, ValidateAsDimensions, false)
			return err
		}
		assert.That(t, call(strings.Repeat("k", 100), "val"), should.ErrLike("bad key"))
		assert.That(t, call("key", strings.Repeat("v", 300)), should.ErrLike("should be no longer"))
		assert.That(t, call("id", "bad:id"), should.ErrLike(`bot ID is not allowed to contain ":"`))
	})

	ftt.Run("Duplicate value in a predicate", t, func(t *ftt.Test) {
		pairs := []*apipb.StringPair{
			{Key: "key", Value: "v3|v1|v2|v1|v2"},
		}

		// With allowDups.
		f, err := NewFilter(pairs, ValidateAsDimensions, true)
		assert.NoErr(t, err)
		assert.That(t, f.filters[0].values, should.Match([]string{"v1", "v2", "v3"}))

		// Without allowDups.
		_, err = NewFilter(pairs, ValidateAsDimensions, false)
		assert.That(t, err, should.ErrLike("key \"key\" has repeated values"))
	})

	ftt.Run("Duplicate predicates", t, func(t *ftt.Test) {
		pairs := []*apipb.StringPair{
			{Key: "key", Value: "v2|v1"},
			{Key: "key", Value: "v1|v2"},
		}

		// With allowDups.
		f, err := NewFilter(pairs, ValidateAsDimensions, true)
		assert.NoErr(t, err)
		assert.That(t, len(f.filters), should.Equal(1))

		// Without allowDups.
		_, err = NewFilter(pairs, ValidateAsDimensions, false)
		assert.That(t, err, should.ErrLike("has duplicate constraints"))
	})

	ftt.Run("Empty", t, func(t *ftt.Test) {
		f, err := NewFilter(nil, ValidateAsTags, false)
		assert.NoErr(t, err)
		assert.That(t, f.IsEmpty(), should.BeTrue)
	})

	ftt.Run("SplitForQuery", t, func(t *ftt.Test) {
		split := func(q string, mode SplitMode) []string {
			in, err := NewFilterFromTags(strings.Split(q, " "))
			assert.NoErr(t, err)

			parts := in.SplitForQuery(mode)

			var out []string
			for _, part := range parts {
				var elems []string
				for _, f := range part.filters {
					elems = append(elems, fmt.Sprintf("%s:%s", f.key, strings.Join(f.values, "|")))
				}
				out = append(out, strings.Join(elems, " "))
			}
			return out
		}

		t.Run("Empty", func(t *ftt.Test) {
			f, err := NewFilter(nil, ValidateAsTags, false)
			assert.NoErr(t, err)
			for _, mode := range []SplitMode{SplitCompletely, SplitOptimally} {
				out := f.SplitForQuery(mode)
				assert.That(t, len(out), should.Equal(1))
				assert.That(t, out[0].IsEmpty(), should.BeTrue)

				q := datastore.NewQuery("Something")
				split := f.Apply(q, "doesntmatter", mode)
				assert.That(t, len(split), should.Equal(1))
				assert.That(t, split[0] == q, should.BeTrue) // the exact same original query
			}
		})

		t.Run("SplitCompletely", func(t *ftt.Test) {
			// Already simple query.
			assert.That(t, split("k1:v1 k2:v2", SplitCompletely), should.Match([]string{"k1:v1 k2:v2"}))
			// One disjunction.
			assert.That(t, split("k1:v1|v2 k2:v3", SplitCompletely), should.Match([]string{
				"k1:v1 k2:v3",
				"k1:v2 k2:v3",
			}))
			// Two disjunctions.
			assert.That(t, split("k1:v1|v2 k2:v3 k3:v4|v5", SplitCompletely), should.Match([]string{
				"k1:v1 k2:v3 k3:v4",
				"k1:v1 k2:v3 k3:v5",
				"k1:v2 k2:v3 k3:v4",
				"k1:v2 k2:v3 k3:v5",
			}))
			// Repeated keys are OK, but may result in redundant filters.
			assert.That(t, split("k1:v1|v2 k1:v2|v3 k1:v4", SplitCompletely), should.Match([]string{
				"k1:v1 k1:v2 k1:v4",
				"k1:v1 k1:v3 k1:v4",
				"k1:v2 k1:v2 k1:v4",
				"k1:v2 k1:v3 k1:v4",
			}))
		})

		t.Run("SplitOptimally", func(t *ftt.Test) {
			// Already simple enough query.
			assert.That(t, split("k1:v1 k2:v2", SplitOptimally), should.Match([]string{"k1:v1 k2:v2"}))
			assert.That(t, split("k1:v1|v2 k2:v3", SplitOptimally), should.Match([]string{"k1:v1|v2 k2:v3"}))
			// Splits on the smallest term.
			assert.That(t, split("k1:v1|v2|v3 k2:v1|v2 k3:v3", SplitOptimally), should.Match([]string{
				"k1:v1|v2|v3 k2:v1 k3:v3",
				"k1:v1|v2|v3 k2:v2 k3:v3",
			}))
			// Leaves at most one disjunction (the largest one).
			assert.That(t, split("k1:v1|v2|v3 k2:v1|v2|v3|v4 k3:v1|v2 k4:v1", SplitOptimally), should.Match([]string{
				"k1:v1 k2:v1|v2|v3|v4 k3:v1 k4:v1",
				"k1:v2 k2:v1|v2|v3|v4 k3:v1 k4:v1",
				"k1:v3 k2:v1|v2|v3|v4 k3:v1 k4:v1",
				"k1:v1 k2:v1|v2|v3|v4 k3:v2 k4:v1",
				"k1:v2 k2:v1|v2|v3|v4 k3:v2 k4:v1",
				"k1:v3 k2:v1|v2|v3|v4 k3:v2 k4:v1",
			}))
		})
	})

	ftt.Run("ValidateComplexity", t, func(t *ftt.Test) {
		t.Run("MaxDimensionChecks", func(t *ftt.Test) {
			var pairs []*apipb.StringPair
			for i := range MaxDimensionChecks / 16 {
				var vals []string
				for j := range 16 {
					vals = append(vals, fmt.Sprintf("v%d", j))
				}
				pairs = append(pairs, &apipb.StringPair{
					Key:   fmt.Sprintf("k%d", i),
					Value: strings.Join(vals, "|"),
				})
			}
			pairs = append(pairs, &apipb.StringPair{
				Key:   "two-more",
				Value: "v1|v2",
			})

			f, err := NewFilter(pairs, ValidateAsDimensions, false)
			assert.NoErr(t, err)

			assert.That(t, f.ValidateComplexity(), should.ErrLike("too many dimension constraints 514 (max is 512)"))
		})

		t.Run("MaxCombinatorialAlternatives", func(t *ftt.Test) {
			f, err := NewFilter([]*apipb.StringPair{
				{Key: "k1", Value: "v1|v2|v3"},
				{Key: "k2", Value: "v1|v2|v3"},
			}, ValidateAsDimensions, false)
			assert.NoErr(t, err)

			assert.That(t, f.ValidateComplexity(), should.ErrLike("too many combinations of dimensions 9 (max is 8), reduce usage of \"|\""))
		})
	})

	ftt.Run("MatchesBot", t, func(t *ftt.Test) {
		f, err := NewFilterFromTaskDimensions(TaskDimensions{
			"k1": {"v1", "v2|v3"},
			"k2": {"v4"},
		})
		assert.NoErr(t, err)

		cases := []struct {
			dims    []string
			outcome bool
		}{
			// Matches.
			{[]string{"k1:v1", "k1:v2", "k2:v4"}, true},
			{[]string{"k1:v1", "k1:v3", "k2:v4"}, true},
			{[]string{"k1:v1", "k1:v2", "k2:v4", "k2:extra"}, true},
			// Doesn't match.
			{[]string{}, false},
			{[]string{"k1:v1"}, false},
			{[]string{"k1:v1", "k1:v2"}, false},
			{[]string{"k1:v1", "k2:v4"}, false},
			{[]string{"k1:v1", "k1:v2", "k2:unknown"}, false},
		}
		for _, c := range cases {
			assert.That(t, f.MatchesBot(c.dims), should.Equal(c.outcome))
		}
	})
}
