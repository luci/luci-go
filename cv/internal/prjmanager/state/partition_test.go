// Copyright 2021 The LUCI Authors.
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

package state

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(State{}))
}

func TestRepartition(t *testing.T) {
	t.Parallel()

	ftt.Run("repartition works", t, func(t *ftt.Test) {
		state := &State{PB: &prjpb.PState{
			RepartitionRequired: true,
		}}
		cat := &categorizedCLs{
			active:   common.CLIDsSet{},
			deps:     common.CLIDsSet{},
			unused:   common.CLIDsSet{},
			unloaded: common.CLIDsSet{},
		}

		check := func(t testing.TB) {
			t.Helper()

			// Assert guarantees of repartition()
			assert.Loosely(t, state.PB.GetRepartitionRequired(), should.BeFalse, truth.LineContext())
			assert.Loosely(t, state.PB.GetCreatedPruns(), should.BeNil, truth.LineContext())
			actual := state.pclIndex
			state.pclIndex = nil
			state.ensurePCLIndex()
			assert.Loosely(t, actual, should.Resemble(state.pclIndex), truth.LineContext())
		}

		t.Run("nothing to do, except resetting RepartitionRequired", func(t *ftt.Test) {
			t.Run("totally empty", func(t *ftt.Test) {
				state.repartition(cat)
				assert.Loosely(t, state.PB, should.Resemble(&prjpb.PState{}))
				check(t)
			})
			t.Run("1 active CL in 1 component", func(t *ftt.Test) {
				cat.active.ResetI64(1)
				state.PB.Components = []*prjpb.Component{{Clids: []int64{1}}}
				state.PB.Pcls = []*prjpb.PCL{{Clid: 1}}
				pb := backupPB(state)

				state.repartition(cat)
				pb.RepartitionRequired = false
				assert.Loosely(t, state.PB, should.Resemble(pb))
				check(t)
			})
			t.Run("1 active CL in 1 component needing triage with 1 Run", func(t *ftt.Test) {
				cat.active.ResetI64(1)
				state.PB.Components = []*prjpb.Component{{
					Clids:          []int64{1},
					Pruns:          []*prjpb.PRun{{Clids: []int64{1}, Id: "id"}},
					TriageRequired: true,
				}}
				state.PB.Pcls = []*prjpb.PCL{{Clid: 1}}
				pb := backupPB(state)

				state.repartition(cat)
				pb.RepartitionRequired = false
				assert.Loosely(t, state.PB, should.Resemble(pb))
				check(t)
			})
		})

		t.Run("Compacts out unused PCLs", func(t *ftt.Test) {
			t.Run("no existing components", func(t *ftt.Test) {
				cat.active.ResetI64(1, 3)
				cat.unused.ResetI64(2)
				state.PB.Pcls = []*prjpb.PCL{
					{Clid: 1},
					{Clid: 2},
					{Clid: 3, Deps: []*changelist.Dep{{Clid: 1}}},
				}

				state.repartition(cat)
				assert.Loosely(t, state.PB, should.Resemble(&prjpb.PState{
					Pcls: []*prjpb.PCL{
						{Clid: 1},
						{Clid: 3, Deps: []*changelist.Dep{{Clid: 1}}},
					},
					Components: []*prjpb.Component{{
						Clids:          []int64{1, 3},
						TriageRequired: true,
					}},
				}))
				check(t)
			})
			t.Run("wipes out existing component, too", func(t *ftt.Test) {
				cat.unused.ResetI64(1, 2, 3)
				state.PB.Pcls = []*prjpb.PCL{
					{Clid: 1},
					{Clid: 2},
					{Clid: 3},
				}
				state.PB.Components = []*prjpb.Component{
					{Clids: []int64{1}},
					{Clids: []int64{2, 3}},
				}
				state.repartition(cat)
				assert.Loosely(t, state.PB, should.Resemble(&prjpb.PState{
					Pcls:       nil,
					Components: nil,
				}))
				check(t)
			})
			t.Run("shrinks existing component, too", func(t *ftt.Test) {
				cat.active.ResetI64(1)
				cat.unused.ResetI64(2)
				state.PB.Pcls = []*prjpb.PCL{
					{Clid: 1},
					{Clid: 2},
				}
				state.PB.Components = []*prjpb.Component{{
					Clids: []int64{1, 2},
				}}
				state.repartition(cat)
				assert.Loosely(t, state.PB, should.Resemble(&prjpb.PState{
					Pcls: []*prjpb.PCL{
						{Clid: 1},
					},
					Components: []*prjpb.Component{{
						Clids:          []int64{1},
						TriageRequired: true,
					}},
				}))
				check(t)
			})
		})

		t.Run("Creates new components", func(t *ftt.Test) {
			t.Run("1 active CL converted into 1 new component needing triage", func(t *ftt.Test) {
				cat.active.ResetI64(1)
				state.PB.Pcls = []*prjpb.PCL{{Clid: 1}}

				state.repartition(cat)
				assert.Loosely(t, state.PB, should.Resemble(&prjpb.PState{
					Pcls: []*prjpb.PCL{{Clid: 1}},
					Components: []*prjpb.Component{{
						Clids:          []int64{1},
						TriageRequired: true,
					}},
				}))
				check(t)
			})
			t.Run("Deps respected during conversion", func(t *ftt.Test) {
				cat.active.ResetI64(1, 2, 3)
				state.PB.Pcls = []*prjpb.PCL{
					{Clid: 1},
					{Clid: 2},
					{Clid: 3, Deps: []*changelist.Dep{{Clid: 1}}},
				}
				orig := backupPB(state)

				state.repartition(cat)
				sortByFirstCL(state.PB.Components)
				assert.Loosely(t, state.PB, should.Resemble(&prjpb.PState{
					Pcls: orig.Pcls,
					Components: []*prjpb.Component{
						{
							Clids:          []int64{1, 3},
							TriageRequired: true,
						},
						{
							Clids:          []int64{2},
							TriageRequired: true,
						},
					},
				}))
				check(t)
			})
		})

		t.Run("Components splitting works", func(t *ftt.Test) {
			t.Run("Crossing-over 12, 34 => 13, 24", func(t *ftt.Test) {
				cat.active.ResetI64(1, 2, 3, 4)
				state.PB.Pcls = []*prjpb.PCL{
					{Clid: 1},
					{Clid: 2},
					{Clid: 3, Deps: []*changelist.Dep{{Clid: 1}}},
					{Clid: 4, Deps: []*changelist.Dep{{Clid: 2}}},
				}
				state.PB.Components = []*prjpb.Component{
					{Clids: []int64{1, 2}},
					{Clids: []int64{3, 4}},
				}
				orig := backupPB(state)

				state.repartition(cat)
				sortByFirstCL(state.PB.Components)
				assert.Loosely(t, state.PB, should.Resemble(&prjpb.PState{
					Pcls: orig.Pcls,
					Components: []*prjpb.Component{
						{Clids: []int64{1, 3}, TriageRequired: true},
						{Clids: []int64{2, 4}, TriageRequired: true},
					},
				}))
				check(t)
			})
			t.Run("Loaded and unloaded deps can be shared by several components", func(t *ftt.Test) {
				cat.active.ResetI64(1, 2, 3)
				cat.deps.ResetI64(4, 5)
				cat.unloaded.ResetI64(5)
				state.PB.Pcls = []*prjpb.PCL{
					{Clid: 1, Deps: []*changelist.Dep{{Clid: 3}, {Clid: 4}, {Clid: 5}}},
					{Clid: 2, Deps: []*changelist.Dep{{Clid: 4}, {Clid: 5}}},
					{Clid: 3},
					{Clid: 4},
				}
				orig := backupPB(state)

				state.repartition(cat)
				sortByFirstCL(state.PB.Components)
				assert.Loosely(t, state.PB, should.Resemble(&prjpb.PState{
					Pcls: orig.Pcls,
					Components: []*prjpb.Component{
						{Clids: []int64{1, 3}, TriageRequired: true},
						{Clids: []int64{2}, TriageRequired: true},
					},
				}))
				check(t)
			})
		})

		t.Run("CreatedRuns are moved into components", func(t *ftt.Test) {
			t.Run("Simple", func(t *ftt.Test) {
				cat.active.ResetI64(1, 2)
				state.PB.Pcls = []*prjpb.PCL{
					{Clid: 1},
					{Clid: 2, Deps: []*changelist.Dep{{Clid: 1}}},
				}
				state.PB.CreatedPruns = []*prjpb.PRun{{Clids: []int64{1, 2}, Id: "id"}}
				orig := backupPB(state)

				state.repartition(cat)
				assert.Loosely(t, state.PB, should.Resemble(&prjpb.PState{
					CreatedPruns: nil,
					Pcls:         orig.Pcls,
					Components: []*prjpb.Component{
						{
							Clids:          []int64{1, 2},
							Pruns:          []*prjpb.PRun{{Clids: []int64{1, 2}, Id: "id"}},
							TriageRequired: true,
						},
					},
				}))
				check(t)
			})
			t.Run("Force-merge 2 existing components", func(t *ftt.Test) {
				cat.active.ResetI64(1, 2)
				state.PB.Pcls = []*prjpb.PCL{
					{Clid: 1},
					{Clid: 2},
				}
				state.PB.Components = []*prjpb.Component{
					{Clids: []int64{1}, Pruns: []*prjpb.PRun{{Clids: []int64{1}, Id: "1"}}},
					{Clids: []int64{2}, Pruns: []*prjpb.PRun{{Clids: []int64{2}, Id: "2"}}},
				}
				state.PB.CreatedPruns = []*prjpb.PRun{{Clids: []int64{1, 2}, Id: "12"}}
				orig := backupPB(state)

				state.repartition(cat)
				sortByFirstCL(state.PB.Components)
				assert.Loosely(t, state.PB, should.Resemble(&prjpb.PState{
					CreatedPruns: nil,
					Pcls:         orig.Pcls,
					Components: []*prjpb.Component{
						{
							Clids: []int64{1, 2},
							Pruns: []*prjpb.PRun{ // must be sorted by ID
								{Clids: []int64{1}, Id: "1"},
								{Clids: []int64{1, 2}, Id: "12"},
								{Clids: []int64{2}, Id: "2"},
							},
							TriageRequired: true,
						},
					},
				}))
				check(t)
			})
		})

		t.Run("Does all at once", func(t *ftt.Test) {
			// This test adds more test coverage for a busy project where components
			// are created, split, merged, and CreatedRuns are incorporated during
			// repartition(), especially likely after a config update.
			cat.active.ResetI64(1, 2, 4, 5, 6)
			cat.deps.ResetI64(7)
			cat.unused.ResetI64(3)
			cat.unloaded.ResetI64(7)
			state.PB.Pcls = []*prjpb.PCL{
				{Clid: 1},
				{Clid: 2, Deps: []*changelist.Dep{{Clid: 1}}},
				{Clid: 3, Deps: []*changelist.Dep{{Clid: 1}, {Clid: 2}}}, // but unused
				{Clid: 4},
				{Clid: 5, Deps: []*changelist.Dep{{Clid: 4}}},
				{Clid: 6, Deps: []*changelist.Dep{{Clid: 7}}},
			}
			state.PB.Components = []*prjpb.Component{
				{Clids: []int64{1, 2, 3}, Pruns: []*prjpb.PRun{{Clids: []int64{1}, Id: "1"}}},
				{Clids: []int64{4}, Pruns: []*prjpb.PRun{{Clids: []int64{4}, Id: "4"}}},
				{Clids: []int64{5}, Pruns: []*prjpb.PRun{{Clids: []int64{5}, Id: "5"}}},
			}
			state.PB.CreatedPruns = []*prjpb.PRun{
				{Clids: []int64{4, 5}, Id: "45"}, // so, merge component with {4}, {5}.
				{Clids: []int64{6}, Id: "6"},
			}

			state.repartition(cat)
			sortByFirstCL(state.PB.Components)
			assert.Loosely(t, state.PB, should.Resemble(&prjpb.PState{
				Pcls: []*prjpb.PCL{
					{Clid: 1},
					{Clid: 2, Deps: []*changelist.Dep{{Clid: 1}}},
					// 3 was deleted
					{Clid: 4},
					{Clid: 5, Deps: []*changelist.Dep{{Clid: 4}}},
					{Clid: 6, Deps: []*changelist.Dep{{Clid: 7}}},
				},
				Components: []*prjpb.Component{
					{Clids: []int64{1, 2}, TriageRequired: true, Pruns: []*prjpb.PRun{{Clids: []int64{1}, Id: "1"}}},
					{Clids: []int64{4, 5}, TriageRequired: true, Pruns: []*prjpb.PRun{
						{Clids: []int64{4}, Id: "4"},
						{Clids: []int64{4, 5}, Id: "45"},
						{Clids: []int64{5}, Id: "5"},
					}},
					{Clids: []int64{6}, TriageRequired: true, Pruns: []*prjpb.PRun{{Clids: []int64{6}, Id: "6"}}},
				},
			}))
			check(t)
		})
	})
}

func TestPartitionSpecialCases(t *testing.T) {
	t.Parallel()

	ftt.Run("Special cases of partitioning", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		epoch := ct.Clock.Now().Truncate(time.Hour)

		t.Run("crbug/1217775", func(t *ftt.Test) {
			s0 := &State{PB: &prjpb.PState{
				// PCLs form a stack 11 <- 12 <- 13.
				Pcls: []*prjpb.PCL{
					{
						Clid: 10,
						// ConfigGroupIndexes: []int32{0},
						Status: prjpb.PCL_OK,
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Time: timestamppb.New(epoch.Add(1 * time.Minute)),
							Mode: string(run.DryRun),
						}},
					},
					{
						Clid: 11,
						// ConfigGroupIndexes: []int32{0},
						Deps: []*changelist.Dep{
							{Clid: 10, Kind: changelist.DepKind_HARD},
						},
						Status:   prjpb.PCL_OK,
						Triggers: nil, // no longer triggered, because its DryRun has just finished.
					},
					{
						Clid: 12,
						// ConfigGroupIndexes: []int32{0},
						Deps: []*changelist.Dep{
							{Clid: 10, Kind: changelist.DepKind_SOFT},
							{Clid: 11, Kind: changelist.DepKind_HARD},
						},
						Status: prjpb.PCL_OK,
						Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
							Time: timestamppb.New(epoch.Add(2 * time.Minute)),
							Mode: string(run.DryRun),
						}},
					},
				},

				Components: []*prjpb.Component{
					{
						Clids: []int64{11},
						// Associated DryRun has just been completed and removed and
						// TriageRequired set to true.
						TriageRequired: true,
					},
				},

				CreatedPruns: []*prjpb.PRun{
					{Id: "chromium/8999-1-aa10", Clids: []int64{10}},
					{Id: "chromium/8999-1-aa12", Clids: []int64{12}},
				},
			}}

			cat := s0.categorizeCLs(ctx)
			assert.Loosely(t, cat.active, should.Resemble(common.CLIDsSet{10: {}, 12: {}}))
			assert.Loosely(t, cat.deps, should.Resemble(common.CLIDsSet{11: {}}))
			assert.Loosely(t, cat.unused, should.BeEmpty)
			assert.Loosely(t, cat.unloaded, should.BeEmpty)

			s1 := s0.cloneShallow()
			s1.repartition(cat)

			// All PCLs are still used.
			assert.Loosely(t, s1.PB.GetPcls(), should.Resemble(s0.PB.GetPcls()))
			// But CreatedPruns must be moved into components.
			assert.Loosely(t, s1.PB.GetCreatedPruns(), should.BeEmpty)
			// Because CLs are related, there should be just 1 component remaining with
			// both Runs.
			assert.Loosely(t, s1.PB.GetComponents(), should.Resemble([]*prjpb.Component{
				{
					Clids: []int64{10, 12},
					Pruns: []*prjpb.PRun{
						{Id: "chromium/8999-1-aa10", Clids: []int64{10}},
						{Id: "chromium/8999-1-aa12", Clids: []int64{12}},
					},
					TriageRequired: true,
				},
			}))
		})
	})
}
