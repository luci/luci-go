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
	"testing"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestComputeOrder(t *testing.T) {
	t.Parallel()

	Convey("ComputeOrder", t, func() {
		mustComputeOrder := func(input []*changelist.CL) []*changelist.CL {
			res, err := ComputeOrder(input)
			if err != nil {
				panic(err)
			}
			return res
		}
		Convey("Single CL", func() {
			So(mustComputeOrder([]*changelist.CL{{ID: 1}}),
				ShouldResembleProto,
				[]*changelist.CL{{ID: 1}},
			)
		})

		Convey("Ignore non-existing Deps", func() {
			clHard := &changelist.CL{
				ID: 1,
				Snapshot: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 11, Kind: changelist.DepKind_HARD}, // Bogus Dep
					},
				},
			}
			clSoft := &changelist.CL{
				ID: 2,
				Snapshot: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 12, Kind: changelist.DepKind_SOFT}, // Bogus Dep
					},
				},
			}
			So(mustComputeOrder([]*changelist.CL{clHard, clSoft}),
				ShouldResembleProto,
				[]*changelist.CL{clHard, clSoft},
			)
		})

		Convey("Error when duplicated CLs provided", func() {
			res, err := ComputeOrder([]*changelist.CL{{ID: 1}, {ID: 1}})
			So(res, ShouldBeNil)
			So(err, ShouldErrLike, "duplicate cl: 1")
		})

		Convey("Disjoint CLs", func() {
			So(mustComputeOrder([]*changelist.CL{{ID: 2}, {ID: 1}}),
				ShouldResembleProto,
				[]*changelist.CL{{ID: 1}, {ID: 2}},
			)
		})

		Convey("Simple deps", func() {
			cl1 := &changelist.CL{
				ID: common.CLID(1),
				Snapshot: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_SOFT},
					},
				},
			}
			cl2 := &changelist.CL{ID: 2}
			So(
				mustComputeOrder([]*changelist.CL{cl1, cl2}),
				ShouldResembleProto,
				[]*changelist.CL{cl2, cl1},
			)

			Convey("With one hard dep", func() {
				cl1.Snapshot = &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_HARD},
					},
				}
				So(
					mustComputeOrder([]*changelist.CL{cl1, cl2}),
					ShouldResembleProto,
					[]*changelist.CL{cl2, cl1},
				)
			})
		})

		Convey("Break soft dependencies if there's a cycle", func() {
			cl1 := &changelist.CL{
				ID: common.CLID(1),
				Snapshot: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_HARD},
					},
				},
			}
			cl2 := &changelist.CL{
				ID: 2,
				Snapshot: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 1, Kind: changelist.DepKind_SOFT},
					},
				},
			}
			So(
				mustComputeOrder([]*changelist.CL{cl1, cl2}),
				ShouldResembleProto,
				[]*changelist.CL{cl2, cl1},
			)
		})

		Convey("Return error if hard dependencies form a cycle", func() {
			cl1 := &changelist.CL{
				ID: common.CLID(1),
				Snapshot: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_HARD},
					},
				},
			}
			cl2 := &changelist.CL{
				ID: 2,
				Snapshot: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 1, Kind: changelist.DepKind_HARD},
					},
				},
			}
			res, err := ComputeOrder([]*changelist.CL{cl1, cl2})
			So(res, ShouldBeNil)
			So(err, ShouldErrLike, "cycle detected for cl: 1")
		})

		Convey("Chain of 3", func() {
			cl1 := &changelist.CL{
				ID: common.CLID(1),
				Snapshot: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_HARD},
						{Clid: 3, Kind: changelist.DepKind_HARD},
					},
				},
			}
			cl2 := &changelist.CL{
				ID: 2,
				Snapshot: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 3, Kind: changelist.DepKind_HARD},
					},
				},
			}
			cl3 := &changelist.CL{ID: 3}
			So(
				mustComputeOrder([]*changelist.CL{cl1, cl2, cl3}),
				ShouldResembleProto,
				[]*changelist.CL{cl3, cl2, cl1},
			)

			Convey("Satisfy soft dep", func() {
				cl1.Snapshot = &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_SOFT},
						{Clid: 3, Kind: changelist.DepKind_SOFT},
					},
				}
				cl2.Snapshot = &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 3, Kind: changelist.DepKind_SOFT},
					},
				}
				So(
					mustComputeOrder([]*changelist.CL{cl1, cl2, cl3}),
					ShouldResembleProto,
					[]*changelist.CL{cl3, cl2, cl1},
				)
			})

			Convey("Ignore soft dep when cycle is present", func() {
				cl3.Snapshot = &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 1, Kind: changelist.DepKind_SOFT},
					},
				}
				So(
					mustComputeOrder([]*changelist.CL{cl1, cl2, cl3}),
					ShouldResembleProto,
					[]*changelist.CL{cl3, cl2, cl1},
				)
				Convey("Input order doesn't matter", func() {
					So(
						mustComputeOrder([]*changelist.CL{cl3, cl2, cl1}),
						ShouldResembleProto,
						[]*changelist.CL{cl3, cl2, cl1},
					)
					So(
						mustComputeOrder([]*changelist.CL{cl2, cl1, cl3}),
						ShouldResembleProto,
						[]*changelist.CL{cl3, cl2, cl1},
					)
				})
			})
		})

		Convey("Pathologic", func() {
			N := 30
			cls := genFullGraph(N)
			actual, numBrokenDeps, err := compute(cls)
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, cls)
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
func genFullGraph(numCLs int) []*changelist.CL {
	cls := make([]*changelist.CL, numCLs)
	for i := range cls {
		cls[i] = &changelist.CL{
			ID: common.CLID(i),
			Snapshot: &changelist.Snapshot{
				Deps: make([]*changelist.Dep, 0, numCLs-1),
			},
		}
		for j := 0; j < numCLs; j++ {
			if i != j {
				dep := &changelist.Dep{
					Clid: int64(j),
					Kind: changelist.DepKind_SOFT,
				}
				if j+1 == i {
					dep.Kind = changelist.DepKind_HARD
				}
				cls[i].Snapshot.Deps = append(cls[i].Snapshot.Deps, dep)
			}
		}
	}
	return cls
}
