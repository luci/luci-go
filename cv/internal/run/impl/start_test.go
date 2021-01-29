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

func TestStart(t *testing.T) {
	t.Parallel()

	Convey("StartRun", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		s := &state{
			Run: run.Run{
				ID:         "chromium/1111111111111-deadbeef",
				Status:     run.Status_PENDING,
				CreateTime: clock.Now(ctx).UTC().Add(-1 * time.Minute),
			},
		}

		Convey("Starts when Run is PENDING", func() {
			s.Run.Status = run.Status_PENDING
			se, ns, err := start(ctx, s)
			So(err, ShouldBeNil)
			So(ns.Run.Status, ShouldEqual, run.Status_RUNNING)
			So(ns.Run.StartTime, ShouldResemble, clock.Now(ctx).UTC())
			So(se, ShouldBeNil)
		})

		statuses := []run.Status{
			run.Status_RUNNING,
			run.Status_FINALIZING,
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				s.Run.Status = status
				se, ns, err := start(ctx, s)
				So(err, ShouldBeNil)
				So(ns, ShouldEqual, s)
				So(se, ShouldBeNil)
			})
		}
	})
}
