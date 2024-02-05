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

	apipb "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDimensionsFilter(t *testing.T) {
	t.Parallel()

	Convey("Ok", t, func() {
		f, err := NewDimensionsFilter([]*apipb.StringPair{
			{Key: "x", Value: "c|b|a"},
			{Key: "x", Value: "y"},
			{Key: "pool", Value: "P1"},
			{Key: "pool", Value: "P1|P2|P3"},
		})
		So(err, ShouldBeNil)
		So(f, ShouldResemble, DimensionsFilter{
			filters: []dimensionFilter{
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
			_, err := NewDimensionsFilter([]*apipb.StringPair{
				{Key: k, Value: v},
			})
			return err
		}
		So(call("", "val"), ShouldErrLike, "bad dimension key")
		So(call("  key", "val"), ShouldErrLike, "bad dimension key")
		So(call("key", ""), ShouldErrLike, "invalid value")
		So(call("key", "  val"), ShouldErrLike, "invalid value")
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
			in, err := NewDimensionsFilter(pairs)
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
