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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"

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
