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

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"

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
			active:   clidsSet{},
			deps:     clidsSet{},
			unused:   clidsSet{},
			unloaded: clidsSet{},
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
				cat.active.resetI64(1)
				state.PB.Components = []*prjpb.Component{{Clids: []int64{1}}}
				state.PB.Pcls = []*prjpb.PCL{{Clid: 1}}
				pb := backupPB(state)

				state.repartition(cat)
				pb.RepartitionRequired = false
				So(state.PB, ShouldResembleProto, pb)
			})
			Convey("1 active CL in 1 component needing triage with 1 Run", func() {
				cat.active.resetI64(1)
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
				cat.active.resetI64(1, 3)
				cat.unused.resetI64(2)
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
				cat.unused.resetI64(1, 2, 3)
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
				cat.active.resetI64(1)
				cat.unused.resetI64(2)
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
				cat.active.resetI64(1)
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
				cat.active.resetI64(1, 2, 3)
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
				cat.active.resetI64(1, 2, 3, 4)
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
				cat.active.resetI64(1, 2, 3)
				cat.deps.resetI64(4, 5)
				cat.unloaded.resetI64(5)
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
				cat.active.resetI64(1, 2)
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
				cat.active.resetI64(1, 2)
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
			cat.active.resetI64(1, 2, 4, 5, 6)
			cat.deps.resetI64(7)
			cat.unused.resetI64(3)
			cat.unloaded.resetI64(7)
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
