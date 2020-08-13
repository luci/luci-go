// Copyright 2020 The LUCI Authors.
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

package submission

import (
	"fmt"
	"strconv"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestComputeOrder(t *testing.T) {
	t.Parallel()

	Convey("ComputeOrder", t, func() {
		computeOrder := func(input []CL) []CL {
			res, err := ComputeOrder(input)
			if err != nil {
				panic(err)
			}
			return res
		}
		Convey("Single CL", func() {
			So(computeOrder([]CL{{Key: "foo"}}), ShouldResemble, []CL{{Key: "foo"}})
		})

		Convey("Ignore non-existing Deps", func() {
			clHard := CL{
				Key:  "withHardDep",
				Deps: []Dep{{Key: "BogusDepHard", Requirement: Hard}},
			}
			clSoft := CL{
				Key:  "withSoftDep",
				Deps: []Dep{{Key: "BogusDepSoft", Requirement: Soft}},
			}
			So(computeOrder([]CL{clHard, clSoft}), ShouldResemble,
				[]CL{clHard, clSoft})
		})

		Convey("Error when duplicated CLs provided", func() {
			res, err := ComputeOrder([]CL{{Key: "foo"}, {Key: "foo"}})
			So(res, ShouldBeNil)
			So(err, ShouldErrLike, "duplicate cl: foo")
		})

		Convey("Disjoint CLs", func() {
			So(computeOrder([]CL{{Key: "foo"}, {Key: "bar"}}), ShouldResemble,
				[]CL{{Key: "bar"}, {Key: "foo"}})
		})

		Convey("Simple deps", func() {
			cl1 := CL{
				Key:  "1",
				Deps: []Dep{{Key: "2"}},
			}
			cl2 := CL{Key: "2"}
			So(computeOrder([]CL{cl1, cl2}), ShouldResemble, []CL{cl2, cl1})

			Convey("With one hard dep", func() {
				cl1.Deps = []Dep{{Key: "2", Requirement: Hard}}
				So(computeOrder([]CL{cl1, cl2}), ShouldResemble, []CL{cl2, cl1})
			})
		})

		Convey("Break soft dependencies if there's a cycle", func() {
			cl1 := CL{
				Key:  "1",
				Deps: []Dep{{Key: "2", Requirement: Hard}},
			}
			cl2 := CL{
				Key:  "2",
				Deps: []Dep{{Key: "1", Requirement: Soft}},
			}
			So(computeOrder([]CL{cl1, cl2}), ShouldResemble, []CL{cl2, cl1})
		})

		Convey("Return error if hard dependencies form a cycle", func() {
			cl1 := CL{
				Key:  "1",
				Deps: []Dep{{Key: "2", Requirement: Hard}},
			}
			cl2 := CL{
				Key:  "2",
				Deps: []Dep{{Key: "1", Requirement: Hard}},
			}
			res, err := ComputeOrder([]CL{cl1, cl2})
			So(res, ShouldBeNil)
			So(err, ShouldErrLike, "cycle detected for cl: 1")
		})

		Convey("Chain of 3", func() {
			cl1 := CL{
				Key: "1",
				Deps: []Dep{
					{Key: "2", Requirement: Hard},
					{Key: "3", Requirement: Hard},
				},
			}
			cl2 := CL{
				Key:  "2",
				Deps: []Dep{{Key: "3", Requirement: Hard}},
			}
			cl3 := CL{
				Key: "3",
			}
			So(computeOrder([]CL{cl1, cl2, cl3}), ShouldResemble,
				[]CL{cl3, cl2, cl1})

			Convey("Satisfy soft dep", func() {
				cl1.Deps = []Dep{
					{Key: "2", Requirement: Soft},
					{Key: "3", Requirement: Soft},
				}
				cl2.Deps = []Dep{{Key: "3", Requirement: Soft}}

				So(computeOrder([]CL{cl1, cl2, cl3}), ShouldResemble,
					[]CL{cl3, cl2, cl1})
			})

			Convey("Ignore soft dep when cycle is present", func() {
				cl3.Deps = []Dep{{Key: "1"}}
				So(computeOrder([]CL{cl1, cl2, cl3}), ShouldResemble,
					[]CL{cl3, cl2, cl1})
				Convey("Input order doesn't matter", func() {
					So(computeOrder([]CL{cl3, cl2, cl1}), ShouldResemble,
						[]CL{cl3, cl2, cl1})
					So(computeOrder([]CL{cl2, cl1, cl3}), ShouldResemble,
						[]CL{cl3, cl2, cl1})
				})
			})
		})

		Convey("Pathologic", func() {
			N := 30
			cls := genFullGraph(N)
			actual, numBrokenDeps, err := compute(cls)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, cls)
			So(numBrokenDeps, ShouldEqual, (N-1)*(N-1))
		})
	})
}

func BenchmarkComputeOrder(b *testing.B) {
	for _, numCLs := range []int{10, 30, 100} {
		cls := genFullGraph(numCLs)
		b.Run(fmt.Sprintf("%d CLs", numCLs), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ComputeOrder(cls)
			}
		})
	}
}

// genFullGraph creates a complete graph with N CLs s.t. Hard Requirement
// Deps form a linked chain {0 <- 1 <- 2 .. <- n-1}.
// Total edges: n*(n-1), of which chain is length of (n-1).
func genFullGraph(numCLs int) []CL {
	cls := make([]CL, numCLs)
	for i := range cls {
		cls[i] = CL{
			Key:  strconv.Itoa(i),
			Deps: make([]Dep, 0, numCLs-1),
		}
		for j := 0; j < numCLs; j++ {
			if i != j {
				dep := Dep{
					Key:         strconv.Itoa(j),
					Requirement: Soft,
				}
				if j+1 == i {
					dep.Requirement = Hard
				}
				cls[i].Deps = append(cls[i].Deps, dep)
			}
		}
	}
	return cls
}
