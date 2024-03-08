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

	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFilter(t *testing.T) {
	t.Parallel()

	Convey("Ok", t, func() {
		f, err := NewFilter([]*apipb.StringPair{
			{Key: "x", Value: "c|b|a"},
			{Key: "x", Value: "y"},
			{Key: "pool", Value: "P1"},
			{Key: "pool", Value: "P1|P2|P3"},
		})
		So(err, ShouldBeNil)
		So(f, ShouldResemble, Filter{
			filters: []perKeyFilter{
				{key: "pool", values: []string{"P1"}},
				{key: "pool", values: []string{"P1", "P2", "P3"}},
				{key: "x", values: []string{"a", "b", "c"}},
				{key: "x", values: []string{"y"}},
			},
		})
		So(f.Pools(), ShouldResemble, []string{"P1", "P2", "P3"})
	})

	Convey("Errors", t, func() {
		call := func(k, v string) error {
			_, err := NewFilter([]*apipb.StringPair{
				{Key: k, Value: v},
			})
			return err
		}
		So(call("", "val"), ShouldErrLike, "bad key")
		So(call("  key", "val"), ShouldErrLike, "bad key")
		So(call("key", ""), ShouldErrLike, "bad value")
		So(call("key", "  val"), ShouldErrLike, "bad value")
	})

	Convey("Empty", t, func() {
		f, err := NewFilter(nil)
		So(err, ShouldBeNil)
		So(f.IsEmpty(), ShouldBeTrue)
	})

	Convey("SplitForQuery", t, func() {
		split := func(q string, mode SplitMode) []string {
			var pairs []*apipb.StringPair
			for _, kv := range strings.Split(q, " ") {
				k, v, _ := strings.Cut(kv, ":")
				pairs = append(pairs, &apipb.StringPair{
					Key:   k,
					Value: v,
				})
			}
			in, err := NewFilter(pairs)
			So(err, ShouldBeNil)

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

		Convey("Empty", func() {
			f, err := NewFilter(nil)
			So(err, ShouldBeNil)
			for _, mode := range []SplitMode{SplitCompletely, SplitOptimally} {
				out := f.SplitForQuery(mode)
				So(out, ShouldHaveLength, 1)
				So(out[0].IsEmpty(), ShouldBeTrue)

				q := datastore.NewQuery("Something")
				split := f.Apply(q, "doesntmatter", mode)
				So(split, ShouldHaveLength, 1)
				So(split[0] == q, ShouldBeTrue) // the exact same original query
			}
		})

		Convey("SplitCompletely", func() {
			// Already simple query.
			So(split("k1:v1 k2:v2", SplitCompletely), ShouldResemble, []string{"k1:v1 k2:v2"})
			// One disjunction.
			So(split("k1:v1|v2 k2:v3", SplitCompletely), ShouldResemble, []string{
				"k1:v1 k2:v3",
				"k1:v2 k2:v3",
			})
			// Two disjunctions.
			So(split("k1:v1|v2 k2:v3 k3:v4|v5", SplitCompletely), ShouldResemble, []string{
				"k1:v1 k2:v3 k3:v4",
				"k1:v1 k2:v3 k3:v5",
				"k1:v2 k2:v3 k3:v4",
				"k1:v2 k2:v3 k3:v5",
			})
			// Repeated keys are OK, but may result in redundant filters.
			So(split("k1:v1|v2 k1:v2|v3 k1:v4", SplitCompletely), ShouldResemble, []string{
				"k1:v1 k1:v2 k1:v4",
				"k1:v1 k1:v3 k1:v4",
				"k1:v2 k1:v2 k1:v4",
				"k1:v2 k1:v3 k1:v4",
			})
		})

		Convey("SplitOptimally", func() {
			// Already simple enough query.
			So(split("k1:v1 k2:v2", SplitOptimally), ShouldResemble, []string{"k1:v1 k2:v2"})
			So(split("k1:v1|v2 k2:v3", SplitOptimally), ShouldResemble, []string{"k1:v1|v2 k2:v3"})
			// Splits on the smallest term.
			So(split("k1:v1|v2|v3 k2:v1|v2 k3:v3", SplitOptimally), ShouldResemble, []string{
				"k1:v1|v2|v3 k2:v1 k3:v3",
				"k1:v1|v2|v3 k2:v2 k3:v3",
			})
			// Leaves at most one disjunction (the largest one).
			So(split("k1:v1|v2|v3 k2:v1|v2|v3|v4 k3:v1|v2 k4:v1", SplitOptimally), ShouldResemble, []string{
				"k1:v1 k2:v1|v2|v3|v4 k3:v1 k4:v1",
				"k1:v2 k2:v1|v2|v3|v4 k3:v1 k4:v1",
				"k1:v3 k2:v1|v2|v3|v4 k3:v1 k4:v1",
				"k1:v1 k2:v1|v2|v3|v4 k3:v2 k4:v1",
				"k1:v2 k2:v1|v2|v3|v4 k3:v2 k4:v1",
				"k1:v3 k2:v1|v2|v3|v4 k3:v2 k4:v1",
			})
		})
	})
}
