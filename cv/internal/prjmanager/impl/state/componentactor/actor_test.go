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

package componentactor

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestActor(t *testing.T) {
	t.Parallel()

	Convey("Component's PCL deps triage", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		// Truncate start time point s.t. easy to see diff in test failures.
		ct.RoundTestClock(10000 * time.Second)

		pmNotifier := prjmanager.NewNotifier(ct.TQDispatcher)
		runNotifier := run.NewNotifier(ct.TQDispatcher)

		dryRun := func(t time.Time) *run.Trigger {
			return &run.Trigger{Mode: string(run.DryRun), Time: timestamppb.New(t)}
		}

		const stabilizationDelay = 5 * time.Minute
		pm := &simplePMState{
			pb: &prjpb.PState{},
			cgs: []*config.ConfigGroup{
				{ID: "hash/singular", Content: &cfgpb.ConfigGroup{}},
				{ID: "hash/combinable", Content: &cfgpb.ConfigGroup{CombineCls: &cfgpb.CombineCLs{
					StabilizationDelay: durationpb.New(stabilizationDelay),
				}}},
				{ID: "hash/another", Content: &cfgpb.ConfigGroup{}},
			},
		}
		const singIdx, combIdx, anotherIdx = 0, 1, 2

		nextActionTime := func(c *prjpb.Component) (*Actor, time.Time) {
			backup := prjpb.PState{}
			proto.Merge(&backup, pm.pb)

			a := newActor(c, pm)
			t, err := a.NextActionTime(ctx, ct.Clock.Now().UTC())
			So(err, ShouldBeNil)
			So(pm.pb, ShouldResembleProto, &backup)
			return a, t
		}

		Convey("Noops", func() {
			pm.pb.Pcls = []*prjpb.PCL{
				{Clid: 33, ConfigGroupIndexes: []int32{singIdx}, Trigger: dryRun(ct.Clock.Now())},
			}
			_, t := nextActionTime(&prjpb.Component{
				Clids: []int64{33},
				// Component already has a Run, so no action required.
				Pruns: []*prjpb.PRun{{Id: "id", Clids: []int64{33}}},
				Dirty: true,
			})
			So(t, ShouldResemble, time.Time{})
		})

		Convey("Prunes CLs", func() {
			pm.pb.Pcls = []*prjpb.PCL{
				{
					Clid:               33,
					ConfigGroupIndexes: nil, // modified below.
					Trigger:            dryRun(ct.Clock.Now()),
					Errors: []*changelist.CLError{ // => must purge.
						{Kind: &changelist.CLError_OwnerLacksEmail{OwnerLacksEmail: true}},
					},
				},
			}
			oldC := &prjpb.Component{Clids: []int64{33}}

			Convey("singular group -- no delay", func() {
				pm.pb.Pcls[0].ConfigGroupIndexes = []int32{singIdx}
				a, t := nextActionTime(oldC)
				So(t, ShouldResemble, ct.Clock.Now().UTC())
				newC, purgeTasks, err := a.Act(ctx, pmNotifier, runNotifier)
				So(err, ShouldBeNil)
				So(purgeTasks, ShouldHaveLength, 1)
				So(newC.GetDirty(), ShouldBeFalse)
			})
			Convey("combinable group -- obey stabilization_delay", func() {
				pm.pb.Pcls[0].ConfigGroupIndexes = []int32{combIdx}

				_, t := nextActionTime(oldC)
				So(t, ShouldResemble, ct.Clock.Now().UTC().Add(stabilizationDelay))

				ct.Clock.Add(stabilizationDelay * 2)
				a, t := nextActionTime(oldC)
				So(t, ShouldResemble, ct.Clock.Now().UTC())
				_, purgeTasks, err := a.Act(ctx, pmNotifier, runNotifier)
				So(err, ShouldBeNil)
				So(purgeTasks, ShouldHaveLength, 1)
			})
			Convey("many groups -- no delay", func() {
				pm.pb.Pcls[0].OwnerLacksEmail = false // many groups is an error itself
				pm.pb.Pcls[0].ConfigGroupIndexes = []int32{singIdx, combIdx, anotherIdx}
				a, t := nextActionTime(oldC)
				So(t, ShouldResemble, ct.Clock.Now().UTC())
				_, purgeTasks, err := a.Act(ctx, pmNotifier, runNotifier)
				So(err, ShouldBeNil)
				So(purgeTasks, ShouldHaveLength, 1)
			})
		})
	})
}
