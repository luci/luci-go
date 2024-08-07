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

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
	"google.golang.org/protobuf/types/known/timestamppb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRepartition(t *testing.T) {
	t.Parallel()

	Convey("repartition works", t, func() {
		state := &State{PB: &prjpb.PState{
			RepartitionRequired: true,
		}}
		cat := &categorizedCLs{
			active:   common.CLIDsSet{},
			deps:     common.CLIDsSet{},
			unused:   common.CLIDsSet{},
			unloaded: common.CLIDsSet{},
		}

		defer func() {
			// Assert guarantees of repartition()
			So(state.PB.GetRepartitionRequired(), ShouldBeFalse)
			So(state.PB.GetCreatedPruns(), ShouldBeNil)
			actual := state.pclIndex
			state.pclIndex = nil
			state.ensurePCLIndex()
			So(actual, ShouldResemble, state.pclIndex)
		}()

		Convey("nothing to do, except resetting RepartitionRequired", func() {
			Convey("totally empty", func() {
				state.repartition(cat)
				So(state.PB, ShouldResembleProto, &prjpb.PState{})
			})
			Convey("1 active CL in 1 component", func() {
				cat.active.ResetI64(1)
				state.PB.Components = []*prjpb.Component{{Clids: []int64{1}}}
				state.PB.Pcls = []*prjpb.PCL{{Clid: 1}}
				pb := backupPB(state)

				state.repartition(cat)
				pb.RepartitionRequired = false
				So(state.PB, ShouldResembleProto, pb)
			})
			Convey("1 active CL in 1 component needing triage with 1 Run", func() {
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
				So(state.PB, ShouldResembleProto, pb)
			})
		})

		Convey("Compacts out unused PCLs", func() {
			Convey("no existing components", func() {
				cat.active.ResetI64(1, 3)
				cat.unused.ResetI64(2)
				state.PB.Pcls = []*prjpb.PCL{
					{Clid: 1},
					{Clid: 2},
					{Clid: 3, Deps: []*changelist.Dep{{Clid: 1}}},
				}

				state.repartition(cat)
				So(state.PB, ShouldResembleProto, &prjpb.PState{
					Pcls: []*prjpb.PCL{
						{Clid: 1},
						{Clid: 3, Deps: []*changelist.Dep{{Clid: 1}}},
					},
					Components: []*prjpb.Component{{
						Clids:          []int64{1, 3},
						TriageRequired: true,
					}},
				})
			})
			Convey("wipes out existing component, too", func() {
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
				So(state.PB, ShouldResembleProto, &prjpb.PState{
					Pcls:       nil,
					Components: nil,
				})
			})
			Convey("shrinks existing component, too", func() {
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
				So(state.PB, ShouldResembleProto, &prjpb.PState{
					Pcls: []*prjpb.PCL{
						{Clid: 1},
					},
					Components: []*prjpb.Component{{
						Clids:          []int64{1},
						TriageRequired: true,
					}},
				})
			})
		})

		Convey("Creates new components", func() {
			Convey("1 active CL converted into 1 new component needing triage", func() {
				cat.active.ResetI64(1)
				state.PB.Pcls = []*prjpb.PCL{{Clid: 1}}

				state.repartition(cat)
				So(state.PB, ShouldResembleProto, &prjpb.PState{
					Pcls: []*prjpb.PCL{{Clid: 1}},
					Components: []*prjpb.Component{{
						Clids:          []int64{1},
						TriageRequired: true,
					}},
				})
			})
			Convey("Deps respected during conversion", func() {
				cat.active.ResetI64(1, 2, 3)
				state.PB.Pcls = []*prjpb.PCL{
					{Clid: 1},
					{Clid: 2},
					{Clid: 3, Deps: []*changelist.Dep{{Clid: 1}}},
				}
				orig := backupPB(state)

				state.repartition(cat)
				sortByFirstCL(state.PB.Components)
				So(state.PB, ShouldResembleProto, &prjpb.PState{
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
				})
			})
		})

		Convey("Components splitting works", func() {
			Convey("Crossing-over 12, 34 => 13, 24", func() {
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
				So(state.PB, ShouldResembleProto, &prjpb.PState{
					Pcls: orig.Pcls,
					Components: []*prjpb.Component{
						{Clids: []int64{1, 3}, TriageRequired: true},
						{Clids: []int64{2, 4}, TriageRequired: true},
					},
				})
			})
			Convey("Loaded and unloaded deps can be shared by several components", func() {
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
				So(state.PB, ShouldResembleProto, &prjpb.PState{
					Pcls: orig.Pcls,
					Components: []*prjpb.Component{
						{Clids: []int64{1, 3}, TriageRequired: true},
						{Clids: []int64{2}, TriageRequired: true},
					},
				})
			})
		})

		Convey("CreatedRuns are moved into components", func() {
			Convey("Simple", func() {
				cat.active.ResetI64(1, 2)
				state.PB.Pcls = []*prjpb.PCL{
					{Clid: 1},
					{Clid: 2, Deps: []*changelist.Dep{{Clid: 1}}},
				}
				state.PB.CreatedPruns = []*prjpb.PRun{{Clids: []int64{1, 2}, Id: "id"}}
				orig := backupPB(state)

				state.repartition(cat)
				So(state.PB, ShouldResembleProto, &prjpb.PState{
					CreatedPruns: nil,
					Pcls:         orig.Pcls,
					Components: []*prjpb.Component{
						{
							Clids:          []int64{1, 2},
							Pruns:          []*prjpb.PRun{{Clids: []int64{1, 2}, Id: "id"}},
							TriageRequired: true,
						},
					},
				})
			})
			Convey("Force-merge 2 existing components", func() {
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
				So(state.PB, ShouldResembleProto, &prjpb.PState{
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
				})
			})
		})

		Convey("Does all at once", func() {
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
			So(state.PB, ShouldResembleProto, &prjpb.PState{
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
			})
		})
	})
}

func TestPartitionSpecialCases(t *testing.T) {
	t.Parallel()

	Convey("Special cases of partitioning", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		epoch := ct.Clock.Now().Truncate(time.Hour)

		Convey("crbug/1217775", func() {
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
			So(cat.active, ShouldResemble, common.CLIDsSet{10: {}, 12: {}})
			So(cat.deps, ShouldResemble, common.CLIDsSet{11: {}})
			So(cat.unused, ShouldBeEmpty)
			So(cat.unloaded, ShouldBeEmpty)

			s1 := s0.cloneShallow()
			s1.repartition(cat)

			// All PCLs are still used.
			So(s1.PB.GetPcls(), ShouldResembleProto, s0.PB.GetPcls())
			// But CreatedPruns must be moved into components.
			So(s1.PB.GetCreatedPruns(), ShouldBeEmpty)
			// Because CLs are related, there should be just 1 component remaining with
			// both Runs.
			So(s1.PB.GetComponents(), ShouldResembleProto, []*prjpb.Component{
				{
					Clids: []int64{10, 12},
					Pruns: []*prjpb.PRun{
						{Id: "chromium/8999-1-aa10", Clids: []int64{10}},
						{Id: "chromium/8999-1-aa12", Clids: []int64{12}},
					},
					TriageRequired: true,
				},
			})
		})
	})
}
