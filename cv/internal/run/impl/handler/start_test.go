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

package handler

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	"go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStart(t *testing.T) {
	t.Parallel()

	Convey("StartRun", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const (
			lProject           = "chromium"
			stabilizationDelay = time.Minute
			startLatency       = 2 * time.Minute
		)

		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{{
			Name: "combinable",
			CombineCls: &cfgpb.CombineCLs{
				StabilizationDelay: durationpb.New(stabilizationDelay),
			},
		}}})

		rs := &state.RunState{
			Run: run.Run{
				ID:            lProject + "/1111111111111-deadbeef",
				Status:        run.Status_PENDING,
				CreateTime:    clock.Now(ctx).UTC().Add(-startLatency),
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
			},
		}
		h, _ := makeTestHandler(&ct)

		Convey("Starts when Run is PENDING", func() {
			rs.Run.Status = run.Status_PENDING
			res, err := h.Start(ctx, rs)
			So(err, ShouldBeNil)
			So(res.State.Run.Status, ShouldEqual, run.Status_RUNNING)
			So(res.State.Run.StartTime, ShouldResemble, clock.Now(ctx).UTC())
			So(res.State.LogEntries, ShouldHaveLength, 1)
			So(res.State.LogEntries[0].GetStarted(), ShouldNotBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)

			So(ct.TSMonSentDistr(ctx, metricPickupLatencyS, lProject).Sum(),
				ShouldAlmostEqual, startLatency.Seconds())
			So(ct.TSMonSentDistr(ctx, metricPickupLatencyAdjustedS, lProject).Sum(),
				ShouldAlmostEqual, (startLatency - stabilizationDelay).Seconds())
		})

		statuses := []run.Status{
			run.Status_RUNNING,
			run.Status_WAITING_FOR_SUBMISSION,
			run.Status_SUBMITTING,
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				rs.Run.Status = status
				res, err := h.Start(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
			})
		}
	})
}

func TestStartMessageFactory(t *testing.T) {
	t.Parallel()

	Convey("startMessageFactory works", t, func() {
		epoch := testclock.TestRecentTimeUTC.Truncate(time.Second)
		makeCL := func(id common.CLID, gHost string, gNumber int, triggerDelay time.Duration) *run.RunCL {
			return &run.RunCL{
				ID: id,
				Detail: &changelist.Snapshot{Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
					Host: gHost,
					Info: gerritfake.CI(gNumber),
				}}},
				Trigger: &run.Trigger{
					Time: timestamppb.New(epoch.Add(triggerDelay)),
				},
			}
		}
		r := &run.Run{Mode: run.FullRun, CLs: common.CLIDs{1, 2, 3}}
		cls := []*run.RunCL{
			makeCL(1, "x-review.example.com", 24, 10*time.Second),
			makeCL(2, "y-review.example.com", 25, 20*time.Second),
			makeCL(3, "z-review.example.com", 26, 30*time.Second),
		}
		factory := startMessageFactory(r, cls)

		expCLs := []botdata.ChangeID{
			{Host: "x-review.example.com", Number: 24},
			{Host: "y-review.example.com", Number: 25},
			{Host: "z-review.example.com", Number: 26},
		}

		test := func(clid common.CLID) botdata.BotData {
			s, err := factory(clid)
			So(err, ShouldBeNil)
			bd, ok := botdata.Parse(&gerritpb.ChangeMessageInfo{Message: s})
			So(ok, ShouldBeTrue)
			So(bd.Action, ShouldEqual, botdata.Start)
			So(bd.CLs, ShouldResemble, expCLs)
			return bd
		}

		Convey("panics on unknown CLID", func() {
			So(func() { _, _ = factory(42) }, ShouldPanic)
		})
		Convey("sets CL-specific triggeredAt", func() {
			So(test(1).TriggeredAt, ShouldResemble, epoch.Add(10*time.Second))
			So(test(2).TriggeredAt, ShouldResemble, epoch.Add(20*time.Second))
		})
		Convey("sets CL-specific revision", func() {
			So(test(2).Revision, ShouldResemble, "rev-000025-001")
			So(test(3).Revision, ShouldResemble, "rev-000026-001")
		})
	})
}
