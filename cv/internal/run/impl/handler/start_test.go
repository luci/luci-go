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
		rs := &state.RunState{
			Run: run.Run{
				ID:         "chromium/1111111111111-deadbeef",
				Status:     run.Status_PENDING,
				CreateTime: clock.Now(ctx).UTC().Add(-1 * time.Minute),
			},
		}
		h := &Impl{}

		Convey("Starts when Run is PENDING", func() {
			rs.Run.Status = run.Status_PENDING
			res, err := h.Start(ctx, rs)
			So(err, ShouldBeNil)
			So(res.State.Run.Status, ShouldEqual, run.Status_RUNNING)
			So(res.State.Run.StartTime, ShouldResemble, clock.Now(ctx).UTC())
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
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
