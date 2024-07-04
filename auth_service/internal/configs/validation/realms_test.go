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

package validation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFindCycle(t *testing.T) {
	t.Parallel()

	Convey("Invalid start node", t, func() {
		graph := map[string][]string{
			"A": {},
		}
		_, err := findCycle("B", graph)
		So(err, ShouldErrLike, "unrecognized")
	})

	Convey("No cycles", t, func() {
		graph := map[string][]string{
			"A": {"B", "C"},
			"B": {"C"},
			"C": {},
		}
		cycle, err := findCycle("A", graph)
		So(err, ShouldBeNil)
		So(cycle, ShouldBeEmpty)
	})

	Convey("Trivial cycle", t, func() {
		graph := map[string][]string{
			"A": {"A"},
		}
		cycle, err := findCycle("A", graph)
		So(err, ShouldBeNil)
		So(cycle, ShouldEqual, []string{"A", "A"})
	})

	Convey("Loop", t, func() {
		graph := map[string][]string{
			"A": {"B"},
			"B": {"C"},
			"C": {"A"},
		}
		cycle, err := findCycle("A", graph)
		So(err, ShouldBeNil)
		So(cycle, ShouldEqual, []string{"A", "B", "C", "A"})
	})

	Convey("Irrelevant cycle", t, func() {
		graph := map[string][]string{
			"A": {"B"},
			"B": {"C"},
			"C": {"B"},
		}
		cycle, err := findCycle("A", graph)
		So(err, ShouldBeNil)
		So(cycle, ShouldBeEmpty)
	})

	Convey("Only one relevant cycle", t, func() {
		graph := map[string][]string{
			"A": {"B"},
			"B": {"C"},
			"C": {"B", "D"},
			"D": {"B", "C", "A"},
		}
		cycle, err := findCycle("A", graph)
		So(err, ShouldBeNil)
		So(cycle, ShouldEqual, []string{"A", "B", "C", "D", "A"})
	})

	Convey("Diamond with no cycles", t, func() {
		graph := map[string][]string{
			"A":  {"B1", "B2"},
			"B1": {"C"},
			"B2": {"C"},
			"C":  {},
		}
		cycle, err := findCycle("A", graph)
		So(err, ShouldBeNil)
		So(cycle, ShouldBeEmpty)
	})

	Convey("Diamond with cycles", t, func() {
		graph := map[string][]string{
			"A":  {"B2", "B1"},
			"B1": {"C"},
			"B2": {"C"},
			"C":  {"A"},
		}
		cycle, err := findCycle("A", graph)
		So(err, ShouldBeNil)
		// Note: this graph has two cycles, but the slice order dictates
		// the exploration order.
		So(cycle, ShouldEqual, []string{"A", "B2", "C", "A"})
	})
}
