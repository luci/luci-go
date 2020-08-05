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
)

func TestOptimizeOrders(t *testing.T) {
	t.Parallel()

	Convey("OptimizeOrders", t, func() {
		Convey("One", func() {
			So(OptimizeOrders([]CL{{Key: "foo"}}), ShouldResemble, []CL{{Key: "foo"}})
		})

		Convey("Bogus deps", func() {
			clHard := CL{
				Key:  "withHardDep",
				Deps: []Dep{{Key: "BogusHard", Requirement: Hard}},
			}
			clSoft := CL{
				Key:  "withSoftDep",
				Deps: []Dep{{Key: "BogusSoft", Requirement: Soft}},
			}
			So(OptimizeOrders([]CL{clHard, clSoft}), ShouldResemble,
				[]CL{clHard, clSoft})
		})

		Convey("Panic on duplicate keys", func() {
			So(func() { OptimizeOrders([]CL{{Key: "foo"}, {Key: "foo"}}) }, ShouldPanic)
		})

		Convey("Disjoint", func() {
			So(OptimizeOrders([]CL{{Key: "foo"}, {Key: "bar"}}), ShouldResemble,
				[]CL{{Key: "bar"}, {Key: "foo"}})
		})

		Convey("Simple deps", func() {
			cl1 := CL{
				Key:  "1",
				Deps: []Dep{{Key: "2"}},
			}
			cl2 := CL{Key: "2"}
			So(OptimizeOrders([]CL{cl1, cl2}), ShouldResemble, []CL{cl2, cl1})

			Convey("With one hard dep", func() {
				cl1.Deps = []Dep{{Key: "2", Requirement: Hard}}
				So(OptimizeOrders([]CL{cl1, cl2}), ShouldResemble, []CL{cl2, cl1})
			})
		})

		Convey("Break soft dependencies if cycle", func() {
			cl1 := CL{
				Key:  "1",
				Deps: []Dep{{Key: "2", Requirement: Hard}},
			}
			cl2 := CL{
				Key:  "2",
				Deps: []Dep{{Key: "1", Requirement: Soft}},
			}
			So(OptimizeOrders([]CL{cl1, cl2}), ShouldResemble, []CL{cl2, cl1})
		})

		Convey("Panic if hard dependencies form a cycle", func() {
			cl1 := CL{
				Key:  "1",
				Deps: []Dep{{Key: "2", Requirement: Hard}},
			}
			cl2 := CL{
				Key:  "2",
				Deps: []Dep{{Key: "1", Requirement: Hard}},
			}
			So(func() { OptimizeOrders([]CL{cl1, cl2}) }, ShouldPanic)
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
			So(OptimizeOrders([]CL{cl1, cl2, cl3}), ShouldResemble,
				[]CL{cl3, cl2, cl1})

			Convey("Satisfy soft dep", func() {
				cl1.Deps = []Dep{
					{Key: "2", Requirement: Soft},
					{Key: "3", Requirement: Soft},
				}
				cl2.Deps = []Dep{{Key: "3", Requirement: Soft}}

				So(OptimizeOrders([]CL{cl1, cl2, cl3}), ShouldResemble,
					[]CL{cl3, cl2, cl1})
			})

			Convey("Ignore soft dep", func() {
				cl3.Deps = []Dep{{Key: "1"}}
				So(OptimizeOrders([]CL{cl1, cl2, cl3}), ShouldResemble,
					[]CL{cl3, cl2, cl1})
				Convey("Input order doesn't matter", func() {
					So(OptimizeOrders([]CL{cl3, cl2, cl1}), ShouldResemble,
						[]CL{cl3, cl2, cl1})
					So(OptimizeOrders([]CL{cl2, cl1, cl3}), ShouldResemble,
						[]CL{cl3, cl2, cl1})
				})
			})
		})

		Convey("Pathologic", func() {
			N := 30
			cls := genFullGraph(N)
			o := &optimizer{CLs: cls}
			So(o.optimize(), ShouldResemble, cls)
			So(len(o.brokenDeps), ShouldEqual, (N-1)*(N-1))
		})

	})
}

// simulates a complete graph with N CLs s.t. Hard Requirement Deps
// forms a linked chain {0 <- 1 <- 2 .. <- n-1}.
// Total edges: n*(n-1), of which chain is length of (n-1).
func genFullGraph(numOfCLs int) []CL {
	cls := make([]CL, numOfCLs)
	for i := range cls {
		cls[i] = CL{
			Key:  strconv.Itoa(i),
			Deps: make([]Dep, 0, numOfCLs-1),
		}
		for j := 0; j < numOfCLs; j++ {
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

func BenchmarkOptimizeOrders(b *testing.B) {
	for _, numOfCLs := range []int{10, 30, 100} {
		cls := genFullGraph(numOfCLs)
		b.Run(fmt.Sprintf("%d CLs", numOfCLs), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				OptimizeOrders(cls)
			}
		})
	}
}
