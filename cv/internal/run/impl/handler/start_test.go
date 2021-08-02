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
	"context"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"
	"google.golang.org/protobuf/types/known/durationpb"

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
				Status:        run.Status_PENDING,
				CreateTime:    clock.Now(ctx).UTC().Add(-startLatency),
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
			},
		}
		h, _, _, _ := makeTestImpl(&ct)

		Convey("Starts when Run is PENDING", func() {
			// TODO(tandrii): move this to cvtesting.Test setup.
			ctx, _, _ = tsmon.WithFakes(ctx)
			tsmon.GetState(ctx).SetStore(store.NewInMemory(&target.Task{}))

			rs.Run.Status = run.Status_PENDING
			res, err := h.Start(ctx, rs)
			So(err, ShouldBeNil)
			So(res.State.Run.Status, ShouldEqual, run.Status_RUNNING)
			So(res.State.Run.StartTime, ShouldResemble, clock.Now(ctx).UTC())
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)

			So(tsmonSentDistr(ctx, metricPickupLatencyS, lProject).Sum(),
				ShouldAlmostEqual, startLatency.Seconds())
			So(tsmonSentDistr(ctx, metricPickupLatencyAdjustedS, lProject).Sum(),
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

func tsmonSentDistr(ctx context.Context, m types.Metric, fieldVals ...interface{}) *distribution.Distribution {
	resetTime := time.Time{}
	return tsmon.GetState(ctx).Store().Get(ctx, m, resetTime, fieldVals).(*distribution.Distribution)
}
