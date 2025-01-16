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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestFilter(t *testing.T) {
	t.Parallel()

	ftt.Run("Ok", t, func(t *ftt.Test) {
		f, err := NewFilter([]*apipb.StringPair{
			{Key: "x", Value: "c|b|a"},
			{Key: "x", Value: "y"},
			{Key: "pool", Value: "P1"},
			{Key: "pool", Value: "P1|P2|P3"},
		})
		assert.NoErr(t, err)
		assert.Loosely(t, f, should.Resemble(Filter{
			filters: []perKeyFilter{
				{key: "pool", values: []string{"P1"}},
				{key: "pool", values: []string{"P1", "P2", "P3"}},
				{key: "x", values: []string{"a", "b", "c"}},
				{key: "x", values: []string{"y"}},
			},
		}))
		assert.Loosely(t, f.Pools(), should.Resemble([]string{"P1", "P2", "P3"}))
	})

	ftt.Run("Errors", t, func(t *ftt.Test) {
		call := func(k, v string) error {
			_, err := NewFilter([]*apipb.StringPair{
				{Key: k, Value: v},
			})
			return err
		}
		assert.Loosely(t, call("", "val"), should.ErrLike("bad key"))
		assert.Loosely(t, call("  key", "val"), should.ErrLike("bad key"))
		assert.Loosely(t, call("key", ""), should.ErrLike("bad value"))
		assert.Loosely(t, call("key", "  val"), should.ErrLike("bad value"))
	})

	ftt.Run("Empty", t, func(t *ftt.Test) {
		f, err := NewFilter(nil)
		assert.NoErr(t, err)
		assert.Loosely(t, f.IsEmpty(), should.BeTrue)
	})

	ftt.Run("SplitForQuery", t, func(t *ftt.Test) {
		split := func(q string, mode SplitMode) []string {
			in, err := NewFilterFromKV(strings.Split(q, " "))
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
			f, err := NewFilter(nil)
			assert.NoErr(t, err)
			for _, mode := range []SplitMode{SplitCompletely, SplitOptimally} {
				out := f.SplitForQuery(mode)
				assert.Loosely(t, out, should.HaveLength(1))
				assert.Loosely(t, out[0].IsEmpty(), should.BeTrue)

				q := datastore.NewQuery("Something")
				split := f.Apply(q, "doesntmatter", mode)
				assert.Loosely(t, split, should.HaveLength(1))
				assert.Loosely(t, split[0] == q, should.BeTrue) // the exact same original query
			}
		})

		t.Run("SplitCompletely", func(t *ftt.Test) {
			// Already simple query.
			assert.Loosely(t, split("k1:v1 k2:v2", SplitCompletely), should.Resemble([]string{"k1:v1 k2:v2"}))
			// One disjunction.
			assert.Loosely(t, split("k1:v1|v2 k2:v3", SplitCompletely), should.Resemble([]string{
				"k1:v1 k2:v3",
				"k1:v2 k2:v3",
			}))
			// Two disjunctions.
			assert.Loosely(t, split("k1:v1|v2 k2:v3 k3:v4|v5", SplitCompletely), should.Resemble([]string{
				"k1:v1 k2:v3 k3:v4",
				"k1:v1 k2:v3 k3:v5",
				"k1:v2 k2:v3 k3:v4",
				"k1:v2 k2:v3 k3:v5",
			}))
			// Repeated keys are OK, but may result in redundant filters.
			assert.Loosely(t, split("k1:v1|v2 k1:v2|v3 k1:v4", SplitCompletely), should.Resemble([]string{
				"k1:v1 k1:v2 k1:v4",
				"k1:v1 k1:v3 k1:v4",
				"k1:v2 k1:v2 k1:v4",
				"k1:v2 k1:v3 k1:v4",
			}))
		})

		t.Run("SplitOptimally", func(t *ftt.Test) {
			// Already simple enough query.
			assert.Loosely(t, split("k1:v1 k2:v2", SplitOptimally), should.Resemble([]string{"k1:v1 k2:v2"}))
			assert.Loosely(t, split("k1:v1|v2 k2:v3", SplitOptimally), should.Resemble([]string{"k1:v1|v2 k2:v3"}))
			// Splits on the smallest term.
			assert.Loosely(t, split("k1:v1|v2|v3 k2:v1|v2 k3:v3", SplitOptimally), should.Resemble([]string{
				"k1:v1|v2|v3 k2:v1 k3:v3",
				"k1:v1|v2|v3 k2:v2 k3:v3",
			}))
			// Leaves at most one disjunction (the largest one).
			assert.Loosely(t, split("k1:v1|v2|v3 k2:v1|v2|v3|v4 k3:v1|v2 k4:v1", SplitOptimally), should.Resemble([]string{
				"k1:v1 k2:v1|v2|v3|v4 k3:v1 k4:v1",
				"k1:v2 k2:v1|v2|v3|v4 k3:v1 k4:v1",
				"k1:v3 k2:v1|v2|v3|v4 k3:v1 k4:v1",
				"k1:v1 k2:v1|v2|v3|v4 k3:v2 k4:v1",
				"k1:v2 k2:v1|v2|v3|v4 k3:v2 k4:v1",
				"k1:v3 k2:v1|v2|v3|v4 k3:v2 k4:v1",
			}))
		})
	})
}
