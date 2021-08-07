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

	"go.chromium.org/luci/common/clock"
	"google.golang.org/protobuf/types/known/durationpb"

	commonpb "go.chromium.org/luci/cv/api/common/v1"
	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
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
				Status:        commonpb.Run_PENDING,
				CreateTime:    clock.Now(ctx).UTC().Add(-startLatency),
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
			},
		}
		h, _, _, _ := makeTestImpl(&ct)

		Convey("Starts when Run is PENDING", func() {
			rs.Run.Status = commonpb.Run_PENDING
			res, err := h.Start(ctx, rs)
			So(err, ShouldBeNil)
			So(res.State.Run.Status, ShouldEqual, commonpb.Run_RUNNING)
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

		statuses := []commonpb.Run_Status{
			commonpb.Run_RUNNING,
			commonpb.Run_WAITING_FOR_SUBMISSION,
			commonpb.Run_SUBMITTING,
			commonpb.Run_SUCCEEDED,
			commonpb.Run_FAILED,
			commonpb.Run_CANCELLED,
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
