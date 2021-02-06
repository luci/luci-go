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
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestEarliestDecisionTime(t *testing.T) {
	t.Parallel()

	Convey("earliestDecisionTime works", t, func() {
		earliest := func(cs []*prjpb.Component) time.Time {
			t, tPB := earliestDecisionTime(cs)
			if t.IsZero() {
				So(tPB, ShouldBeNil)
			} else {
				So(tPB.AsTime(), ShouldResemble, t)
			}
			return t
		}

		t0 := testclock.TestRecentTimeUTC
		cs := []*prjpb.Component{
			{DecisionTime: nil},
		}
		So(earliest(cs), ShouldResemble, time.Time{})

		cs = append(cs, &prjpb.Component{DecisionTime: timestamppb.New(t0.Add(time.Second))})
		So(earliest(cs), ShouldResemble, t0.Add(time.Second))

		cs = append(cs, &prjpb.Component{})
		So(earliest(cs), ShouldResemble, t0.Add(time.Second))

		cs = append(cs, &prjpb.Component{DecisionTime: timestamppb.New(t0.Add(time.Hour))})
		So(earliest(cs), ShouldResemble, t0.Add(time.Second))

		cs = append(cs, &prjpb.Component{DecisionTime: timestamppb.New(t0)})
		So(earliest(cs), ShouldResemble, t0)
	})
}

func TestComponentsActions(t *testing.T) {
	t.Parallel()

	Convey("Component actions logic work in the abstract", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		now := ct.Clock.Now()

		const lProject = "luci-project"

		// scanComponents needs config to exist, but this test doesn't actually care
		// about what's inside due to mock componentActor.
		ct.Cfg.Create(ctx, lProject, &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{
			{
				Name: "main",
				Gerrit: []*cfgpb.ConfigGroup_Gerrit{
					{
						Url: "https://example.com",
						Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
							{Name: "re/po"},
						},
					},
				},
			},
		}})
		meta := ct.Cfg.MustExist(ctx, lProject)

		state := NewExisting(&prjpb.PState{
			LuciProject: lProject,
			Status:      prjpb.Status_STARTED,
			ConfigHash:  meta.Hash(),
			Pcls: []*prjpb.PCL{
				{Clid: 1},
				{Clid: 2},
				{Clid: 3},
				{Clid: 999},
			},
			Components: []*prjpb.Component{
				{Clids: []int64{999}}, // never sees any action.
				{Clids: []int64{1}},
				{Clids: []int64{2}},
				{Clids: []int64{3}, DecisionTime: timestamppb.New(now.Add(3 * time.Minute))},
			},
			NextEvalTime: timestamppb.New(now.Add(3 * time.Minute)),
		})

		pb := backupPB(state)

		makeDirtySetup := func(indexes ...int) {
			for _, i := range indexes {
				state.PB.GetComponents()[i].Dirty = true
			}
			pb = backupPB(state)
		}

		unDirty := func(c *prjpb.Component) *prjpb.Component {
			So(c.GetDirty(), ShouldBeTrue)
			o := cloneComponent(c)
			o.Dirty = false
			return o
		}

		Convey("noop at preevaluation", func() {
			state.testComponentActorFactory = (&componentActorSetup{}).factory
			as, cs, err := state.scanComponents(ctx)
			So(err, ShouldBeNil)
			So(as, ShouldBeNil)
			So(cs, ShouldBeNil)
			So(state.PB, ShouldResembleProto, pb)

			Convey("ExecDeferred", func() {
				state2, sideEffect, err := state.ExecDeferred(ctx)
				So(err, ShouldBeNil)
				So(state2, ShouldEqual, state) // pointer comparison
				So(sideEffect, ShouldBeNil)
				So(pmtest.ETAsOF(ct.TQ.Tasks(), lProject), ShouldBeEmpty)
			})
		})

		Convey("updates future DecisionTime in scan", func() {
			makeDirtySetup(1, 2, 3)
			state.testComponentActorFactory = (&componentActorSetup{
				nextAction: func(cl int64, now time.Time) (time.Time, error) {
					switch cl {
					case 1:
						return time.Time{}, nil
					case 2:
						return now.Add(2 * time.Minute), nil
					case 3:
						return state.PB.Components[3].GetDecisionTime().AsTime(), nil // same
					}
					panic("unrechable")
				},
			}).factory
			actions, components, err := state.scanComponents(ctx)
			So(err, ShouldBeNil)
			So(actions, ShouldBeNil)
			So(components, ShouldResembleProto, []*prjpb.Component{
				pb.GetComponents()[0], // #999 unchanged
				unDirty(pb.GetComponents()[1]),
				{Clids: []int64{2}, DecisionTime: timestamppb.New(now.Add(2 * time.Minute))},
				unDirty(pb.GetComponents()[3]),
			})
			So(state.PB, ShouldResembleProto, pb)

			Convey("ExecDeferred", func() {
				state2, sideEffect, err := state.ExecDeferred(ctx)
				So(err, ShouldBeNil)
				So(sideEffect, ShouldBeNil)
				pb.Components = components
				pb.NextEvalTime = timestamppb.New(now.Add(2 * time.Minute))
				So(state2.PB, ShouldResembleProto, pb)
				So(pmtest.ETAsWithin(ct.TQ.Tasks(), lProject, time.Second, now.Add(2*time.Minute)), ShouldNotBeEmpty)
			})
		})

		Convey("partial failure in scan", func() {
			makeDirtySetup(1, 2, 3)
			state.testComponentActorFactory = (&componentActorSetup{
				nextAction: func(cl int64, now time.Time) (time.Time, error) {
					switch cl {
					case 1:
						return time.Time{}, errors.New("oops1")
					case 2, 3:
						return now, nil
					}
					panic("unrechable")
				},
			}).factory
			actions, components, err := state.scanComponents(ctx)
			So(err, ShouldBeNil)
			So(components, ShouldResembleProto, []*prjpb.Component{
				pb.GetComponents()[0], // #999 unchanged
				{Clids: []int64{1}, Dirty: true, DecisionTime: timestamppb.New(now)},
				pb.GetComponents()[2], // #2 unchanged
				pb.GetComponents()[3], // #3 unchanged
			})
			So(state.PB, ShouldResembleProto, pb)

			So(state.execComponentActions(ctx, actions, components), ShouldBeNil)
			// Must modify passed components only.
			So(state.PB, ShouldResembleProto, pb)
			So(components, ShouldResembleProto, []*prjpb.Component{
				pb.GetComponents()[0],
				{Clids: []int64{1}, Dirty: true, DecisionTime: timestamppb.New(now)}, // errored on
				{Clids: []int64{2}}, // acted upon
				{Clids: []int64{3}}, // acted upon
			})

			Convey("ExecDeferred", func() {
				state2, sideEffect, err := state.ExecDeferred(ctx)
				So(err, ShouldBeNil)
				So(sideEffect, ShouldBeNil)
				pb.Components = components
				pb.NextEvalTime = timestamppb.New(now)
				So(state2.PB, ShouldResembleProto, pb)
				// Self-poke task must be scheduled for earliest possible from now.
				So(pmtest.ETAsWithin(ct.TQ.Tasks(), lProject, time.Second, now.Add(prjpb.PokeInterval)), ShouldNotBeEmpty)
			})
		})

		Convey("100% failure in scan", func() {
			makeDirtySetup(1, 2)
			state.testComponentActorFactory = (&componentActorSetup{
				nextAction: func(cl int64, now time.Time) (time.Time, error) {
					switch cl {
					case 1, 2:
						return time.Time{}, errors.New("oops")
					}
					panic("unrechable")
				},
			}).factory
			_, _, err := state.scanComponents(ctx)
			So(err, ShouldErrLike, "oops")
			So(state.PB, ShouldResembleProto, pb)

			Convey("ExecDeferred", func() {
				state2, sideEffect, err := state.ExecDeferred(ctx)
				So(err, ShouldNotBeNil)
				So(sideEffect, ShouldBeNil)
				So(state2, ShouldBeNil)
			})
		})

		Convey("partial failure in exec", func() {
			makeDirtySetup(1, 2, 3)
			state.testComponentActorFactory = (&componentActorSetup{
				nextAction:  func(cl int64, now time.Time) (time.Time, error) { return now, nil },
				actErrOnCLs: []int64{1, 2},
			}).factory
			actions, components, err := state.scanComponents(ctx)
			So(err, ShouldBeNil)
			err = state.execComponentActions(ctx, actions, components)
			So(err, ShouldBeNil)
			// Must modify passed components only.
			So(state.PB, ShouldResembleProto, pb)
			So(components, ShouldResembleProto, []*prjpb.Component{
				pb.GetComponents()[0], // #999 unchanged
				{Clids: []int64{1}, Dirty: true, DecisionTime: timestamppb.New(now)}, // errored on
				{Clids: []int64{2}, Dirty: true, DecisionTime: timestamppb.New(now)}, // errored on
				{Clids: []int64{3}}, // acted upon
			})

			Convey("ExecDeferred", func() {
				state2, sideEffect, err := state.ExecDeferred(ctx)
				So(err, ShouldBeNil)
				So(sideEffect, ShouldBeNil)
				pb.Components = components
				pb.NextEvalTime = timestamppb.New(now)
				So(state2.PB, ShouldResembleProto, pb)
				// Self-poke task must be scheduled for earliest possible from now.
				So(pmtest.ETAsWithin(ct.TQ.Tasks(), lProject, time.Second, now.Add(prjpb.PokeInterval)), ShouldNotBeEmpty)
			})
		})

		Convey("100% failure in exec", func() {
			makeDirtySetup(1, 2, 3)
			state.testComponentActorFactory = (&componentActorSetup{
				nextAction:  func(cl int64, now time.Time) (time.Time, error) { return now, nil },
				actErrOnCLs: []int64{1, 2, 3},
			}).factory
			actions, components, err := state.scanComponents(ctx)
			So(err, ShouldBeNil)
			err = state.execComponentActions(ctx, actions, components)
			So(err, ShouldErrLike, "act-oops")
			So(state.PB, ShouldResembleProto, pb)

			Convey("ExecDeferred", func() {
				state2, sideEffect, err := state.ExecDeferred(ctx)
				So(err, ShouldNotBeNil)
				So(sideEffect, ShouldBeNil)
				So(state2, ShouldBeNil)
			})
		})
	})
}

type componentActorSetup struct {
	nextAction  func(clid int64, now time.Time) (time.Time, error)
	actErrOnCLs []int64
}

func (s *componentActorSetup) factory(c *prjpb.Component, _ actorSupporter) componentActor {
	return &testCActor{s, c}
}

type testCActor struct {
	parent *componentActorSetup
	c      *prjpb.Component
}

func (t *testCActor) nextActionTime(_ context.Context, now time.Time) (time.Time, error) {
	return t.parent.nextAction(t.c.GetClids()[0], now)
}

func (t *testCActor) act(context.Context) (*prjpb.Component, error) {
	for _, clid := range t.parent.actErrOnCLs {
		if t.c.GetClids()[0] == clid {
			return nil, errors.Reason("act-oops %v", t.c).Err()
		}
	}
	c := cloneComponent(t.c)
	c.Dirty = false
	c.DecisionTime = nil
	return c, nil
}

func TestDepsTriage(t *testing.T) {
	// This test may hang with infinite recursion due to proto comparison.
	// If this fails, run it with -test.timeout=100ms while debugging.

	t.Parallel()

	Convey("Component's PCL deps triage", t, func() {
		// Truncate start time point s.t. easy to see diff in test failures.
		epoch := testclock.TestRecentTimeUTC.Truncate(10000 * time.Second)
		dryRun := func(t time.Time) *run.Trigger {
			return &run.Trigger{Mode: string(run.DryRun), Time: timestamppb.New(t)}
		}
		fullRun := func(t time.Time) *run.Trigger {
			return &run.Trigger{Mode: string(run.FullRun), Time: timestamppb.New(t)}
		}
		state := NewExisting(&prjpb.PState{})

		configGroups := []*config.ConfigGroup{
			{ID: "hash/singular", Content: &cfgpb.ConfigGroup{}},
			{ID: "hash/combinable", Content: &cfgpb.ConfigGroup{CombineCls: &cfgpb.CombineCLs{}}},
			{ID: "hash/another", Content: &cfgpb.ConfigGroup{}},
		}
		const singIdx, combIdx, anotherIdx = 0, 1, 2

		triage := func(pcl *prjpb.PCL, cgIdx int32) *triagedDeps {
			state.pclIndex = nil
			state.ensurePCLIndex()
			a := componentActorImpl{s: &actorSupporterImpl{
				pclIndex:     state.pclIndex,
				pcls:         state.PB.GetPcls(),
				configGroups: configGroups,
			}}
			pb := backupPB(state)
			td := a.triageDeps(pcl, cgIdx)
			So(state.PB, ShouldResembleProto, pb)
			return td
		}

		assertEqual := func(a, b *triagedDeps) {
			So(a.lastTriggered, ShouldResemble, b.lastTriggered)

			So(a.notYetLoaded, ShouldResembleProto, b.notYetLoaded)
			So(a.submitted, ShouldResembleProto, b.submitted)

			So(a.unwatched, ShouldResembleProto, b.unwatched)
			So(a.wrongConfigGroup, ShouldResembleProto, b.wrongConfigGroup)
			So(a.incompatMode, ShouldResembleProto, b.incompatMode)
		}

		mustPCL := func(id int64) *prjpb.PCL {
			for _, pcl := range state.PB.GetPcls() {
				if pcl.GetClid() == id {
					return pcl
				}
			}
			panic(fmt.Errorf("wrong ID: %d", id))
		}

		Convey("Singluar and Combinable behave the same", func() {
			sameTests := func(name string, cgIdx int32) {
				Convey(name, func() {
					Convey("no deps", func() {
						state.PB.Pcls = []*prjpb.PCL{
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}},
						}
						td := triage(state.PB.Pcls[0], cgIdx)
						assertEqual(td, &triagedDeps{})
						So(td.OK(), ShouldBeTrue)
					})

					Convey("Valid CL stack CQ+1", func() {
						state.PB.Pcls = []*prjpb.PCL{
							{Clid: 31, ConfigGroupIndexes: []int32{cgIdx}, Trigger: dryRun(epoch.Add(3 * time.Second))},
							{Clid: 32, ConfigGroupIndexes: []int32{cgIdx}, Trigger: dryRun(epoch.Add(2 * time.Second))},
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}, Trigger: dryRun(epoch.Add(1 * time.Second)),
								Deps: []*changelist.Dep{
									{Clid: 31, Kind: changelist.DepKind_SOFT},
									{Clid: 32, Kind: changelist.DepKind_HARD},
								}},
						}
						td := triage(mustPCL(33), cgIdx)
						assertEqual(td, &triagedDeps{
							lastTriggered: epoch.Add(3 * time.Second),
						})
						So(td.OK(), ShouldBeTrue)
					})

					Convey("Not yet loaded deps", func() {
						state.PB.Pcls = []*prjpb.PCL{
							// 31 isn't in PCLs yet
							{Clid: 32, Status: prjpb.PCL_UNKNOWN},
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}, Trigger: dryRun(epoch.Add(1 * time.Second)),
								Deps: []*changelist.Dep{
									{Clid: 31, Kind: changelist.DepKind_SOFT},
									{Clid: 32, Kind: changelist.DepKind_HARD},
								}},
						}
						pcl33 := mustPCL(33)
						td := triage(pcl33, cgIdx)
						assertEqual(td, &triagedDeps{notYetLoaded: pcl33.GetDeps()})
						So(td.OK(), ShouldBeTrue)
					})

					Convey("Unwatched", func() {
						state.PB.Pcls = []*prjpb.PCL{
							{Clid: 31, Status: prjpb.PCL_UNWATCHED},
							{Clid: 32, Status: prjpb.PCL_DELETED},
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}, Trigger: dryRun(epoch.Add(1 * time.Second)),
								Deps: []*changelist.Dep{
									{Clid: 31, Kind: changelist.DepKind_SOFT},
									{Clid: 32, Kind: changelist.DepKind_HARD},
								}},
						}
						pcl33 := mustPCL(33)
						td := triage(pcl33, cgIdx)
						assertEqual(td, &triagedDeps{unwatched: pcl33.GetDeps()})
						So(td.OK(), ShouldBeFalse)
					})

					Convey("Submitted can be in any config group and they are OK deps", func() {
						state.PB.Pcls = []*prjpb.PCL{
							{Clid: 32, ConfigGroupIndexes: []int32{anotherIdx}, Submitted: true},
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}, Trigger: dryRun(epoch.Add(1 * time.Second)),
								Deps: []*changelist.Dep{{Clid: 32, Kind: changelist.DepKind_HARD}}},
						}
						pcl33 := mustPCL(33)
						td := triage(pcl33, cgIdx)
						assertEqual(td, &triagedDeps{submitted: pcl33.GetDeps()})
						So(td.OK(), ShouldBeTrue)
					})

					Convey("wrong config group", func() {
						state.PB.Pcls = []*prjpb.PCL{
							{Clid: 31, Trigger: dryRun(epoch.Add(3 * time.Second)), ConfigGroupIndexes: []int32{anotherIdx}},
							{Clid: 32, Trigger: dryRun(epoch.Add(2 * time.Second)), ConfigGroupIndexes: []int32{anotherIdx, cgIdx}},
							{Clid: 33, Trigger: dryRun(epoch.Add(1 * time.Second)), ConfigGroupIndexes: []int32{cgIdx},
								Deps: []*changelist.Dep{
									{Clid: 31, Kind: changelist.DepKind_SOFT},
									{Clid: 32, Kind: changelist.DepKind_HARD},
								}},
						}
						pcl33 := mustPCL(33)
						td := triage(pcl33, cgIdx)
						assertEqual(td, &triagedDeps{
							lastTriggered:    epoch.Add(3 * time.Second),
							wrongConfigGroup: pcl33.GetDeps(),
						})
						So(td.OK(), ShouldBeFalse)
					})
				})
			}
			sameTests("singular", singIdx)
			sameTests("combinable", combIdx)
		})

		Convey("Singular speciality", func() {
			state.PB.Pcls = []*prjpb.PCL{
				{
					Clid: 31, ConfigGroupIndexes: []int32{singIdx},
					Trigger: dryRun(epoch.Add(3 * time.Second)),
				},
				{
					Clid: 32, ConfigGroupIndexes: []int32{singIdx},
					Trigger: fullRun(epoch.Add(2 * time.Second)), // not happy about its dep.
					Deps:    []*changelist.Dep{{Clid: 31, Kind: changelist.DepKind_HARD}},
				},
				{
					Clid: 33, ConfigGroupIndexes: []int32{singIdx},
					Trigger: dryRun(epoch.Add(3 * time.Second)), // doesn't care about deps.
					Deps: []*changelist.Dep{
						{Clid: 31, Kind: changelist.DepKind_SOFT},
						{Clid: 32, Kind: changelist.DepKind_HARD},
					},
				},
			}
			Convey("dry run doesn't care about deps' triggers", func() {
				pcl33 := mustPCL(33)
				td := triage(pcl33, singIdx)
				assertEqual(td, &triagedDeps{
					lastTriggered: epoch.Add(3 * time.Second),
				})
			})
			Convey("full run considers any dep incompatible", func() {
				pcl32 := mustPCL(32)
				td := triage(pcl32, singIdx)
				assertEqual(td, &triagedDeps{
					lastTriggered: epoch.Add(3 * time.Second),
					incompatMode:  pcl32.GetDeps(),
				})
				So(td.OK(), ShouldBeFalse)
			})
		})

		Convey("Combinable speciality", func() {
			// Setup valid deps; sub-tests will mutate this to become invalid.
			state.PB.Pcls = []*prjpb.PCL{
				{
					Clid: 31, ConfigGroupIndexes: []int32{combIdx},
					Trigger: dryRun(epoch.Add(3 * time.Second)),
				},
				{
					Clid: 32, ConfigGroupIndexes: []int32{combIdx},
					Trigger: dryRun(epoch.Add(2 * time.Second)),
					Deps:    []*changelist.Dep{{Clid: 31, Kind: changelist.DepKind_HARD}},
				},
				{
					Clid: 33, ConfigGroupIndexes: []int32{combIdx},
					Trigger: dryRun(epoch.Add(1 * time.Second)),
					Deps: []*changelist.Dep{
						{Clid: 31, Kind: changelist.DepKind_SOFT},
						{Clid: 32, Kind: changelist.DepKind_HARD},
					},
				},
			}
			Convey("dry run expects all deps to be dry", func() {
				pcl32 := mustPCL(32)
				Convey("ok", func() {
					td := triage(pcl32, combIdx)
					assertEqual(td, &triagedDeps{lastTriggered: epoch.Add(3 * time.Second)})
				})

				Convey("... not full runs", func() {
					// TODO(tandrii): this can and should be supported.
					mustPCL(31).Trigger.Mode = string(run.FullRun)
					td := triage(pcl32, combIdx)
					assertEqual(td, &triagedDeps{
						lastTriggered: epoch.Add(3 * time.Second),
						incompatMode:  pcl32.GetDeps(),
					})
				})
			})
			Convey("full run considers any dep incompatible", func() {
				pcl33 := mustPCL(33)
				Convey("ok", func() {
					for _, pcl := range state.PB.GetPcls() {
						pcl.Trigger.Mode = string(run.FullRun)
					}
					td := triage(pcl33, combIdx)
					assertEqual(td, &triagedDeps{lastTriggered: epoch.Add(3 * time.Second)})
				})
				Convey("... not dry runs", func() {
					mustPCL(32).Trigger.Mode = string(run.FullRun)
					td := triage(pcl33, combIdx)
					assertEqual(td, &triagedDeps{
						lastTriggered: epoch.Add(3 * time.Second),
						incompatMode:  []*changelist.Dep{{Clid: 32, Kind: changelist.DepKind_HARD}},
					})
					So(td.OK(), ShouldBeFalse)
				})
			})
		})
	})
}
