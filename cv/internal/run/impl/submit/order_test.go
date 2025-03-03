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

package submit

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

func init() {
	registry.RegisterCmpOption(cmpopts.IgnoreUnexported(run.RunCL{}))
}

func TestComputeOrder(t *testing.T) {
	t.Parallel()

	ftt.Run("ComputeOrder", t, func(t *ftt.Test) {
		mustComputeOrder := func(input []*run.RunCL) []*run.RunCL {
			res, err := ComputeOrder(input)
			if err != nil {
				panic(err)
			}
			return res
		}
		t.Run("Single CL", func(t *ftt.Test) {
			assert.Loosely(t, mustComputeOrder([]*run.RunCL{{ID: 1}}),
				should.Match(
					[]*run.RunCL{{ID: 1}},
				))
		})

		t.Run("Ignore non-existing Deps", func(t *ftt.Test) {
			clHard := &run.RunCL{
				ID: 1,
				Detail: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 11, Kind: changelist.DepKind_HARD}, // Bogus Dep
					},
				},
			}
			clSoft := &run.RunCL{
				ID: 2,
				Detail: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 12, Kind: changelist.DepKind_SOFT}, // Bogus Dep
					},
				},
			}
			assert.Loosely(t, mustComputeOrder([]*run.RunCL{clHard, clSoft}),
				should.Match(
					[]*run.RunCL{clHard, clSoft},
				))
		})

		t.Run("Error when duplicated CLs provided", func(t *ftt.Test) {
			res, err := ComputeOrder([]*run.RunCL{{ID: 1}, {ID: 1}})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, should.ErrLike("duplicate cl: 1"))
		})

		t.Run("Disjoint CLs", func(t *ftt.Test) {
			assert.Loosely(t, mustComputeOrder([]*run.RunCL{{ID: 2}, {ID: 1}}),
				should.Match(
					[]*run.RunCL{{ID: 1}, {ID: 2}},
				))
		})

		t.Run("Simple deps", func(t *ftt.Test) {
			cl1 := &run.RunCL{
				ID: common.CLID(1),
				Detail: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_SOFT},
					},
				},
			}
			cl2 := &run.RunCL{ID: 2}
			assert.Loosely(t,
				mustComputeOrder([]*run.RunCL{cl1, cl2}),
				should.Match(
					[]*run.RunCL{cl2, cl1},
				))

			t.Run("With one hard dep", func(t *ftt.Test) {
				cl1.Detail = &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_HARD},
					},
				}
				assert.Loosely(t,
					mustComputeOrder([]*run.RunCL{cl1, cl2}),
					should.Match(
						[]*run.RunCL{cl2, cl1},
					))
			})
		})

		t.Run("Break soft dependencies if there's a cycle", func(t *ftt.Test) {
			cl1 := &run.RunCL{
				ID: common.CLID(1),
				Detail: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_HARD},
					},
				},
			}
			cl2 := &run.RunCL{
				ID: 2,
				Detail: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 1, Kind: changelist.DepKind_SOFT},
					},
				},
			}
			assert.Loosely(t,
				mustComputeOrder([]*run.RunCL{cl1, cl2}),
				should.Match(
					[]*run.RunCL{cl2, cl1},
				))
		})

		t.Run("Return error if hard dependencies form a cycle", func(t *ftt.Test) {
			cl1 := &run.RunCL{
				ID: common.CLID(1),
				Detail: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_HARD},
					},
				},
			}
			cl2 := &run.RunCL{
				ID: 2,
				Detail: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 1, Kind: changelist.DepKind_HARD},
					},
				},
			}
			res, err := ComputeOrder([]*run.RunCL{cl1, cl2})
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, should.ErrLike("cycle detected for cl: 1"))
		})

		t.Run("Chain of 3", func(t *ftt.Test) {
			cl1 := &run.RunCL{
				ID: common.CLID(1),
				Detail: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_HARD},
						{Clid: 3, Kind: changelist.DepKind_HARD},
					},
				},
			}
			cl2 := &run.RunCL{
				ID: 2,
				Detail: &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 3, Kind: changelist.DepKind_HARD},
					},
				},
			}
			cl3 := &run.RunCL{ID: 3}
			assert.Loosely(t,
				mustComputeOrder([]*run.RunCL{cl1, cl2, cl3}),
				should.Match(
					[]*run.RunCL{cl3, cl2, cl1},
				))

			t.Run("Satisfy soft dep", func(t *ftt.Test) {
				cl1.Detail = &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_SOFT},
						{Clid: 3, Kind: changelist.DepKind_SOFT},
					},
				}
				cl2.Detail = &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 3, Kind: changelist.DepKind_SOFT},
					},
				}
				assert.Loosely(t,
					mustComputeOrder([]*run.RunCL{cl1, cl2, cl3}),
					should.Match(
						[]*run.RunCL{cl3, cl2, cl1},
					))
			})

			t.Run("Ignore soft dep when cycle is present", func(t *ftt.Test) {
				cl3.Detail = &changelist.Snapshot{
					Deps: []*changelist.Dep{
						{Clid: 1, Kind: changelist.DepKind_SOFT},
					},
				}
				assert.Loosely(t,
					mustComputeOrder([]*run.RunCL{cl1, cl2, cl3}),
					should.Match(
						[]*run.RunCL{cl3, cl2, cl1},
					))
				t.Run("Input order doesn't matter", func(t *ftt.Test) {
					assert.Loosely(t,
						mustComputeOrder([]*run.RunCL{cl3, cl2, cl1}),
						should.Match(
							[]*run.RunCL{cl3, cl2, cl1},
						))
					assert.Loosely(t,
						mustComputeOrder([]*run.RunCL{cl2, cl1, cl3}),
						should.Match(
							[]*run.RunCL{cl3, cl2, cl1},
						))
				})
			})
		})

		t.Run("Pathologic", func(t *ftt.Test) {
			N := 30
			cls := genFullGraph(N)
			actual, numBrokenDeps, err := compute(cls)
			assert.NoErr(t, err)
			assert.That(t, actual, should.Match(cls))
			assert.Loosely(t, numBrokenDeps, should.Equal((N-1)*(N-1)))
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
func genFullGraph(numCLs int) []*run.RunCL {
	cls := make([]*run.RunCL, numCLs)
	for i := range cls {
		cls[i] = &run.RunCL{
			ID: common.CLID(i),
			Detail: &changelist.Snapshot{
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
				cls[i].Detail.Deps = append(cls[i].Detail.Deps, dep)
			}
		}
	}
	return cls
}
