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

package impl

import (
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCancel(t *testing.T) {
	t.Parallel()

	Convey("Cancel", t, func() {
		ct := cvtesting.Test{}
		ctx, close := ct.SetUp()
		defer close()
		s := &state{
			Run: run.Run{
				ID:         "chromium/1111111111111-deadbeef",
				CreateTime: clock.Now(ctx).UTC().Add(-2 * time.Minute),
			},
		}

		Convey("Cancels PENDING Run", func() {
			s.Run.Status = run.Status_PENDING
			se, ns, err := cancel(ctx, s)
			So(err, ShouldBeNil)
			So(ns.Run.Status, ShouldEqual, run.Status_CANCELLED)
			now := clock.Now(ctx).UTC()
			So(ns.Run.StartTime, ShouldResemble, now)
			So(ns.Run.EndTime, ShouldResemble, now)
			So(se, ShouldBeNil)
		})

		Convey("Cancels RUNNING Run", func() {
			s.Run.Status = run.Status_RUNNING
			s.Run.StartTime = clock.Now(ctx).UTC().Add(-1 * time.Minute)
			se, ns, err := cancel(ctx, s)
			So(err, ShouldBeNil)
			So(ns.Run.Status, ShouldEqual, run.Status_CANCELLED)
			now := clock.Now(ctx).UTC()
			So(ns.Run.StartTime, ShouldResemble, now.Add(-1*time.Minute))
			So(ns.Run.EndTime, ShouldResemble, now)
			So(se, ShouldBeNil)
		})

		statuses := []run.Status{
			run.Status_FINALIZING,
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				s.Run.Status = status
				s.Run.StartTime = clock.Now(ctx).UTC().Add(-1 * time.Minute)
				s.Run.EndTime = clock.Now(ctx).UTC().Add(-30 * time.Second)
				se, ns, err := cancel(ctx, s)
				So(err, ShouldBeNil)
				So(ns, ShouldEqual, s)
				So(se, ShouldBeNil)
			})
		}
	})
}
