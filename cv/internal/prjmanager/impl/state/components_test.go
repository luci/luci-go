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
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestComponentsActions(t *testing.T) {
	t.Parallel()

	Convey("Component actions logic work in the abstract", t, func() {
		ctx := context.Background()

		state := NewExisting(&prjpb.PState{
			Pcls: []*prjpb.PCL{
				{Clid: 1},
				{Clid: 2},
				{Clid: 3},
			},
			Components: []*prjpb.Component{
				{Clids: []int64{1}, Dirty: true},
				{Clids: []int64{2}, Dirty: true},
				{Clids: []int64{3}, Dirty: true},
			},
		})
		pb := backupPB(state)

		Convey("noop at preevaluation", func() {
			state.testComponentPreevaluator = &testCPreevaluator{skip: []int64{1, 2, 3}}
			as, err := state.prepareComponentActions(ctx)
			So(err, ShouldBeNil)
			So(as, ShouldBeEmpty)
			So(state.PB, ShouldResembleProto, pb)
		})

		Convey("act on #1 and #3, but no modification", func() {
			state.testComponentPreevaluator = &testCPreevaluator{should: []int64{1, 3}}
			as, err := state.prepareComponentActions(ctx)
			So(err, ShouldBeNil)
			So(as, ShouldHaveLength, 2)
			sort.Slice(as, func(i, j int) bool { return as[i].componentIndex < as[j].componentIndex })
			So(as[0].componentIndex, ShouldEqual, 0)
			So(as[0].actor.(*testCActor).c, ShouldEqual, state.PB.GetComponents()[0])
			So(as[1].componentIndex, ShouldEqual, 2)
			So(as[1].actor.(*testCActor).c, ShouldEqual, state.PB.GetComponents()[2])

			So(state.execComponentActions(ctx, as), ShouldBeNil)
			So(state.PB, ShouldResembleProto, pb)
		})

		Convey("act on #1 and #3, but only #3 modified", func() {
			state.testComponentPreevaluator = &testCPreevaluator{should: []int64{1, 3}, modifyAct: []int64{3}}
			as, err := state.prepareComponentActions(ctx)
			So(err, ShouldBeNil)
			So(state.execComponentActions(ctx, as), ShouldBeNil)
			pb.GetComponents()[2].Dirty = false // the only chagne
			So(state.PB, ShouldResembleProto, pb)
		})

		Convey("partial error still leads to partial progress", func() {
			state.testComponentPreevaluator = &testCPreevaluator{
				should:    []int64{1, 3},
				errShould: []int64{2},

				errAct:    []int64{1},
				modifyAct: []int64{3},
			}
			as, err := state.prepareComponentActions(ctx)
			So(err, ShouldBeNil)
			So(as, ShouldHaveLength, 2)
			So(state.execComponentActions(ctx, as), ShouldBeNil)
			pb.GetComponents()[2].Dirty = false // the only chagne
			So(state.PB, ShouldResembleProto, pb)
		})

		Convey("100% error on shouldAct results in error regardless of skipped", func() {
			state.testComponentPreevaluator = &testCPreevaluator{errShould: []int64{2, 3}} // 1 is skipped
			_, err := state.prepareComponentActions(ctx)
			So(err, ShouldErrLike, "shouldAct on 0, failed to check 2, keeping the most severe error: failed to shouldAct on")
			So(state.PB, ShouldResembleProto, pb)
		})

		Convey("100% error on act results in error", func() {
			state.testComponentPreevaluator = &testCPreevaluator{should: []int64{2, 3}, errAct: []int64{2, 3}}
			as, err := state.prepareComponentActions(ctx)
			So(err, ShouldBeNil)
			So(as, ShouldHaveLength, 2)
			So(state.execComponentActions(ctx, as), ShouldErrLike,
				"acted on components: succeded 0 (modified 0), failed 2, keeping the most severe error:")
			So(state.PB, ShouldResembleProto, pb)
		})
	})
}

type testCPreevaluator struct {
	// If any CL in the list equals first CL in component,
	// then evaluator or actor responds appropriately.
	skip, errShould, should, errAct, modifyAct []int64
}

func matchesFirstComponentCL(c *prjpb.Component, clids []int64) bool {
	for _, id := range clids {
		if id == c.GetClids()[0] {
			return true
		}
	}
	return false
}

func (t *testCPreevaluator) shouldEvaluate(_ time.Time, _ pclGetter, c *prjpb.Component) (ce componentActor, skip bool) {
	if matchesFirstComponentCL(c, t.skip) {
		return nil, true
	}
	return &testCActor{t, c}, false
}

type testCActor struct {
	parent *testCPreevaluator
	c      *prjpb.Component
}

func (t *testCActor) shouldAct(context.Context) (bool, error) {
	if matchesFirstComponentCL(t.c, t.parent.errShould) {
		return false, errors.Reason("failed to shouldAct on %v", t.c).Err()
	}
	return matchesFirstComponentCL(t.c, t.parent.should), nil
}

func (t *testCActor) act(context.Context) (*prjpb.Component, error) {
	if matchesFirstComponentCL(t.c, t.parent.errAct) {
		return nil, errors.Reason("failed to act on %v", t.c).Err()
	}
	if matchesFirstComponentCL(t.c, t.parent.modifyAct) {
		return &prjpb.Component{
			Clids: t.c.GetClids(),
			Dirty: false,
		}, nil
	}
	return t.c, nil
}
