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

	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCancel(t *testing.T) {
	t.Parallel()

	Convey("Cancel", t, func() {
		ct := cvtesting.Test{}
		ctx, close := ct.SetUp()
		defer close()
		rs := &state.RunState{
			Run: run.Run{
				ID:         "chromium/1111111111111-deadbeef",
				CreateTime: clock.Now(ctx).UTC().Add(-2 * time.Minute),
			},
		}
		h := &Impl{}

		Convey("Cancels PENDING Run", func() {
			rs.Run.Status = run.Status_PENDING
			se, newrs, err := h.Cancel(ctx, rs)
			So(err, ShouldBeNil)
			So(newrs.Run.Status, ShouldEqual, run.Status_CANCELLED)
			now := clock.Now(ctx).UTC()
			So(newrs.Run.StartTime, ShouldResemble, now)
			So(newrs.Run.EndTime, ShouldResemble, now)
			So(se, ShouldNotBeNil)
			So(se(ctx), ShouldBeNil)
			pmtest.AssertReceivedRunFinished(ctx, rs.Run.ID)
		})

		Convey("Cancels RUNNING Run", func() {
			rs.Run.Status = run.Status_RUNNING
			rs.Run.StartTime = clock.Now(ctx).UTC().Add(-1 * time.Minute)
			se, newrs, err := h.Cancel(ctx, rs)
			So(err, ShouldBeNil)
			So(newrs.Run.Status, ShouldEqual, run.Status_CANCELLED)
			now := clock.Now(ctx).UTC()
			So(newrs.Run.StartTime, ShouldResemble, now.Add(-1*time.Minute))
			So(newrs.Run.EndTime, ShouldResemble, now)
			So(se, ShouldNotBeNil)
			So(se(ctx), ShouldBeNil)
			pmtest.AssertReceivedRunFinished(ctx, rs.Run.ID)
		})

		Convey("Cancels SUBMITTING Run", func() {
			rs.Run.Status = run.Status_SUBMITTING
			se, newrs, err := h.Cancel(ctx, rs)
			So(err, ShouldBeNil)
			So(newrs, ShouldEqual, rs)
			So(se, ShouldBeNil)
		})

		statuses := []run.Status{
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				rs.Run.Status = status
				rs.Run.StartTime = clock.Now(ctx).UTC().Add(-1 * time.Minute)
				rs.Run.EndTime = clock.Now(ctx).UTC().Add(-30 * time.Second)
				se, newrs, err := h.Cancel(ctx, rs)
				So(err, ShouldBeNil)
				So(newrs, ShouldEqual, rs)
				So(se, ShouldBeNil)
			})
		}
	})
}
