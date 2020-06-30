// Copyright 2020 The LUCI Authors.
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

package internal

import (
	"context"
	"testing"
	"time"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/proto/google"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMakeReminder(t *testing.T) {
	t.Parallel()

	Convey("makeReminder", t, func() {
		ctx := cryptorand.MockForTest(context.Background(), 111)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeLocal)
		req := taskspb.CreateTaskRequest{
			Parent: "projects/example-project/locations/us-central1/queues/q",
			Task: &taskspb.Task{
				MessageType: &taskspb.Task_HttpRequest{HttpRequest: &taskspb.HttpRequest{
					HttpMethod: taskspb.HttpMethod_GET,
					Url:        "https://example.com/user/exec",
				}},
				ScheduleTime: google.NewTimestamp(clock.Now(ctx).Add(time.Minute)),
			},
		}

		Convey("typical request with deadline", func() {
			deadline := testclock.TestRecentTimeLocal.Add(5 * time.Second)
			reqCtx, cancel := clock.WithDeadline(ctx, deadline)
			defer cancel()
			r, clonedReq, err := makeReminder(reqCtx, &req)
			So(err, ShouldBeNil)
			So(r.Id, ShouldResemble, "1408d4aaccbdf3f4b03972992f179ce4")
			So(len(r.Id), ShouldEqual, 2*keySpaceBytes)
			So(r.FreshUntil, ShouldEqual, deadline.UTC())
			So(clonedReq.Task.Name, ShouldResemble,
				"projects/example-project/locations/us-central1/queues/q/tasks/1408d4aaccbdf3f4b03972992f179ce4")

			deserialized, err := r.createTaskRequest()
			So(err, ShouldBeNil)
			So(clonedReq, ShouldResembleProto, deserialized)

			clonedReq.Task.Name = ""
			So(clonedReq, ShouldResembleProto, &req)
		})

		Convey("reasonable FreshUntil without deadline", func() {
			r, _, err := makeReminder(ctx, &req)
			So(err, ShouldBeNil)
			So(r.FreshUntil, ShouldEqual, testclock.TestRecentTimeLocal.Add(happyPathMaxDuration).UTC())
		})

		Convey("named tasks not supported", func() {
			req.Task.Name = req.Parent + "/tasks/deadbeef"
			_, _, err := makeReminder(ctx, &req)
			So(err, ShouldErrLike, "Named tasks are not supported")
		})
	})
}
