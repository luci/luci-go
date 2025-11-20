// Copyright 2025 The LUCI Authors.
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

package rpc

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(batchToLaunch{}))
	registry.RegisterCmpOption(cmp.AllowUnexported(model.Build{}))
}

func TestDemoteTurboCIChildrenToNative(t *testing.T) {
	t.Parallel()

	batch := func(kind batchToLaunchKind, ids ...string) *batchToLaunch {
		reqs := make([]*pb.ScheduleBuildRequest, len(ids))
		builds := make([]*model.Build, len(ids))
		for i, id := range ids {
			reqs[i] = &pb.ScheduleBuildRequest{RequestId: id}
			builds[i] = &model.Build{BuilderID: id}
		}
		return &batchToLaunch{
			kind:   kind,
			reqs:   reqs,
			builds: builds,
		}
	}

	t.Run("with native", func(t *testing.T) {
		got := demoteTurboCIChildrenToNative([]*batchToLaunch{
			batch(batchKindTurboCIRoot, "root1"),
			batch(batchKindNative, "native1", "native2"),
			batch(batchKindTurboCIRoot, "root2"),
			batch(batchKindTurboCIChildren, "child1", "child2"),
			batch(batchKindTurboCIChildren, "child3", "child4"),
		})

		assert.That(t, got, should.Match([]*batchToLaunch{
			batch(batchKindNative, "native1", "native2", "child1", "child2", "child3", "child4"),
			batch(batchKindTurboCIRoot, "root1"),
			batch(batchKindTurboCIRoot, "root2"),
		}))
	})

	t.Run("without native", func(t *testing.T) {
		got := demoteTurboCIChildrenToNative([]*batchToLaunch{
			batch(batchKindTurboCIRoot, "root1"),
			batch(batchKindTurboCIRoot, "root2"),
			batch(batchKindTurboCIChildren, "child1", "child2"),
			batch(batchKindTurboCIChildren, "child3", "child4"),
		})

		assert.That(t, got, should.Match([]*batchToLaunch{
			batch(batchKindNative, "child1", "child2", "child3", "child4"),
			batch(batchKindTurboCIRoot, "root1"),
			batch(batchKindTurboCIRoot, "root2"),
		}))
	})

	t.Run("native only", func(t *testing.T) {
		got := demoteTurboCIChildrenToNative([]*batchToLaunch{
			batch(batchKindNative, "native1", "native2"),
		})

		assert.That(t, got, should.Match([]*batchToLaunch{
			batch(batchKindNative, "native1", "native2"),
		}))
	})

	t.Run("roots only", func(t *testing.T) {
		got := demoteTurboCIChildrenToNative([]*batchToLaunch{
			batch(batchKindTurboCIRoot, "root1"),
			batch(batchKindTurboCIRoot, "root2"),
		})

		assert.That(t, got, should.Match([]*batchToLaunch{
			batch(batchKindTurboCIRoot, "root1"),
			batch(batchKindTurboCIRoot, "root2"),
		}))
	})
}

func TestSplitByLaunchStrategy(t *testing.T) {
	t.Parallel()

	knownParents := map[string]*model.Build{}
	addParent := func(id string, turboCI bool) {
		b := &model.Build{
			ID:        int64(len(knownParents) + 1),
			BuilderID: id,
		}
		if turboCI {
			b.StageAttemptID = "some-id"
		}
		knownParents[id] = b
	}

	addParent("native_parent1", false)
	addParent("native_parent2", false)
	addParent("turboci_parent1", true)
	addParent("turboci_parent2", true)

	type fakeReq struct {
		id      string
		turboCI bool
		parent  string
	}

	prepOp := func(req []fakeReq) *scheduleBuildOp {
		reqs := make([]*pb.ScheduleBuildRequest, len(req))
		builds := make([]*model.Build, len(req))
		errs := make(errors.MultiError, len(req))
		parents := map[int64]*parent{}

		for idx, r := range req {
			var parentBuildID int64
			if r.parent != "" {
				par := knownParents[r.parent]
				if par == nil {
					panic(fmt.Sprintf("unknown parent %q", r.parent))
				}
				parentBuildID = par.ID
				parents[parentBuildID] = &parent{
					bld:       par,
					ancestors: []int64{parentBuildID},
				}
			}
			reqs[idx] = &pb.ScheduleBuildRequest{
				RequestId:     r.id,
				ParentBuildId: parentBuildID,
			}

			var exps []string
			if r.turboCI {
				exps = []string{buildbucket.ExperimentRunInTurboCI}
			}
			builds[idx] = &model.Build{
				BuilderID: r.id,
				Proto: &pb.Build{
					Input: &pb.Build_Input{
						Experiments: exps,
					},
				},
			}

			errs = append(errs, nil)
		}
		op := &scheduleBuildOp{
			Reqs:    reqs,
			Builds:  builds,
			Errs:    errs,
			Parents: &parentsMap{fromRequests: parents},
		}
		op.UpdateReqMap()
		return op
	}

	batchesToStr := func(t *testing.T, batches []*batchToLaunch) []string {
		t.Helper()
		var out []string
		for _, b := range batches {
			assert.That(t, len(b.reqs), should.Equal(len(b.builds)))
			parts := []string{""}
			switch b.kind {
			case batchKindNative:
				parts[0] = "native:"
			case batchKindTurboCIRoot:
				parts[0] = "root:"
			case batchKindTurboCIChildren:
				parts[0] = fmt.Sprintf("children(%s):", b.parent.BuilderID)
			default:
				panic("impossible")
			}
			for idx, req := range b.reqs {
				assert.That(t, req.RequestId, should.Equal(b.builds[idx].BuilderID))
				parts = append(parts, req.RequestId)
			}
			out = append(out, strings.Join(parts, " "))
		}
		slices.Sort(out)
		return out
	}

	cases := []struct {
		name string
		reqs []fakeReq
		want []string
	}{
		{
			name: "one native root",
			reqs: []fakeReq{
				{id: "root"},
			},
			want: []string{
				"native: root",
			},
		},

		{
			name: "one native child",
			reqs: []fakeReq{
				{id: "child", parent: "native_parent1"},
			},
			want: []string{
				"native: child",
			},
		},

		{
			name: "one turboci root",
			reqs: []fakeReq{
				{id: "root", turboCI: true},
			},
			want: []string{
				"root: root",
			},
		},

		{
			name: "one turboci child of turboci",
			reqs: []fakeReq{
				{id: "child", turboCI: true, parent: "turboci_parent1"},
			},
			want: []string{
				"children(turboci_parent1): child",
			},
		},

		{
			name: "one turboci child of native",
			reqs: []fakeReq{
				{id: "child", turboCI: true, parent: "native_parent1"},
			},
			want: []string{
				"native: child",
			},
		},

		{
			name: "native only",
			reqs: []fakeReq{
				{id: "native_root_1"},
				{id: "native_root_2"},
				{id: "native_child_1", parent: "native_parent1"},
				{id: "native_child_2", parent: "native_parent1"},
				{id: "native_child_3", parent: "native_parent2"},
			},
			want: []string{
				"native: native_root_1 native_root_2 native_child_1 native_child_2 native_child_3",
			},
		},

		{
			name: "turboci roots",
			reqs: []fakeReq{
				{id: "root_1", turboCI: true},
				{id: "root_2", turboCI: true},
			},
			want: []string{
				"root: root_1",
				"root: root_2",
			},
		},

		{
			name: "turboci children of turboci root",
			reqs: []fakeReq{
				{id: "child_1", turboCI: true, parent: "turboci_parent1"},
				{id: "child_2", turboCI: true, parent: "turboci_parent1"},
				{id: "another", turboCI: true, parent: "turboci_parent2"},
			},
			want: []string{
				"children(turboci_parent1): child_1 child_2",
				"children(turboci_parent2): another",
			},
		},

		{
			name: "turboci children of native root",
			reqs: []fakeReq{
				{id: "child_1", turboCI: true, parent: "native_parent1"},
				{id: "child_2", turboCI: true, parent: "native_parent1"},
				{id: "another", turboCI: true, parent: "native_parent2"},
			},
			want: []string{
				"native: child_1 child_2 another",
			},
		},

		{
			name: "everything",
			reqs: []fakeReq{
				{id: "native_root_1"},
				{id: "native_root_2"},
				{id: "native_child_1", parent: "native_parent1"},
				{id: "native_child_2", parent: "native_parent1"},
				{id: "native_child_3", parent: "native_parent2"},
				{id: "root_1", turboCI: true},
				{id: "root_2", turboCI: true},
				{id: "child_1", turboCI: true, parent: "turboci_parent1"},
				{id: "child_2", turboCI: true, parent: "turboci_parent1"},
				{id: "child_3", turboCI: true, parent: "turboci_parent2"},
				{id: "child_4", turboCI: true, parent: "native_parent1"},
				{id: "child_5", turboCI: true, parent: "native_parent1"},
				{id: "child_6", turboCI: true, parent: "native_parent2"},
			},
			want: []string{
				"children(turboci_parent1): child_1 child_2",
				"children(turboci_parent2): child_3",
				"native: native_root_1 native_root_2 native_child_1 native_child_2 native_child_3 child_4 child_5 child_6",
				"root: root_1",
				"root: root_2",
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			batches := splitByLaunchStrategyImpl(prepOp(cs.reqs))
			assert.That(t, batchesToStr(t, batches), should.Match(cs.want))
		})
	}
}
