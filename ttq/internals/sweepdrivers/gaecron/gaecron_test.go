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

package gaecron

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/ttq"
	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"

	"go.chromium.org/luci/ttq/internals"
	ttqtesting "go.chromium.org/luci/ttq/internals/testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSweeper(t *testing.T) {
	t.Parallel()

	Convey("Sweeper works", t, func() {
		epoch := testclock.TestRecentTimeLocal
		ctx := cryptorand.MockForTest(gaetesting.TestingContext(), 111)
		ctx, tclock := testclock.UseTime(ctx, epoch)
		ctx = gologger.StdConfig.Use(ctx)
		ctx = logging.SetLevel(ctx, logging.Debug)

		db := ttqtesting.FakeDB{}
		userCT := ttqtesting.FakeCloudTasks{}
		impl := internals.Impl{DB: &db, TasksClient: &userCT, Options: ttq.Options{
			Shards:              2,
			SecondaryScanShards: 2,
			TasksPerScan:        10,
		}}
		ttqCT := ttqtesting.FakeCloudTasks{}
		sw := NewSweeper(&impl, "/internal/ttq", "projects/example-project/locations/us-central1/queues/ttq", &ttqCT)

		r := router.NewWithRootContext(ctx)
		sw.InstallRoutes(r, router.MiddlewareChain{})
		server := httptest.NewServer(r)
		defer server.Close()

		Convey("noop", func() {
			So(makeCronRequest(server.URL), ShouldEqual, 200)
			reqs := ttqCT.PopCreateTaskReqs()
			So(len(reqs), ShouldEqual, impl.Options.Shards)
			codes := makeTasksRequests(server.URL, reqs...)
			assertAll200(codes)
			// There should be no more followup scans.
			So(len(ttqCT.PopCreateTaskReqs()), ShouldEqual, 0)

			So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 0)
		})

		Convey("Some reminders", func() {
			tclock.Set(epoch)
			_, err := impl.AddTask(ctx, genUserTask(ctx)) // don't call postProcess on this one.
			So(err, ShouldBeNil)
			tclock.Add(time.Hour) // let Reminder for the first user task turn stale.
			pp2, err := impl.AddTask(ctx, genUserTask(ctx))
			So(err, ShouldBeNil)
			So(len(db.AllReminders()), ShouldEqual, 2) // 1 fresh + 1 stale.

			// Sweep.
			So(makeCronRequest(server.URL), ShouldEqual, 200)
			assertAll200(makeTasksRequests(server.URL, ttqCT.PopCreateTaskReqs()...))
			So(len(db.AllReminders()), ShouldEqual, 1) // 1 fresh.
			// There should be no more followup scans.
			So(len(ttqCT.PopCreateTaskReqs()), ShouldEqual, 0)
			// And 1 user task is created.
			So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 1)

			pp2(ctx)
			So(len(db.AllReminders()), ShouldEqual, 0)
			// And 1 user task is also created.
			So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 1)
		})

		Convey("More scans", func() {
			impl.Options.Shards = 1
			impl.Options.TasksPerScan = 2
			for i := 0; i < 3; i++ {
				_, err := impl.AddTask(ctx, genUserTask(ctx)) // don't call postProcess.
				So(err, ShouldBeNil)
			}
			tclock.Add(time.Hour) // turn all Reminders stale.
			So(len(db.AllReminders()), ShouldEqual, 3)

			// Sweep.
			So(makeCronRequest(server.URL), ShouldEqual, 200)
			assertAll200(makeTasksRequests(server.URL, ttqCT.PopCreateTaskReqs()...))
			// First level scan processed 2 reminders only.
			So(len(db.AllReminders()), ShouldEqual, 1)
			// Do second-level scans.
			assertAll200(makeTasksRequests(server.URL, ttqCT.PopCreateTaskReqs()...))

			// All work must be done.
			So(len(db.AllReminders()), ShouldEqual, 0)
			So(len(userCT.PopCreateTaskReqs()), ShouldEqual, 3)
		})
	})
}

func makeCronRequest(serverURL string) int {
	client := http.Client{}
	resp, err := client.Get(serverURL + "/internal/ttq/cron")
	return readResponse(resp, err)
}

func makeTasksRequests(serverURL string, reqs ...*taskspb.CreateTaskRequest) []int {
	respCodes := make([]int, len(reqs))
	wg := sync.WaitGroup{}
	wg.Add(len(reqs))
	for i, _ := range reqs {
		i := i
		go func() {
			defer wg.Done()
			respCodes[i] = makeTaskRequest(serverURL, reqs[i])
		}()
	}
	wg.Wait()
	return respCodes
}

func makeTaskRequest(serverURL string, req *taskspb.CreateTaskRequest) int {
	client := http.Client{}
	gaereq := req.Task.GetAppEngineHttpRequest()
	buf := bytes.NewReader(gaereq.Body)
	resp, err := client.Post(serverURL+gaereq.RelativeUri, "application/json", buf)
	return readResponse(resp, err)
}

func readResponse(resp *http.Response, err error) int {
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	return resp.StatusCode
}

func genUserTask(ctx context.Context) *taskspb.CreateTaskRequest {
	return &taskspb.CreateTaskRequest{
		Parent: "projects/example-project/locations/us-central1/queues/q",
		Task: &taskspb.Task{
			MessageType: &taskspb.Task_HttpRequest{HttpRequest: &taskspb.HttpRequest{
				HttpMethod: taskspb.HttpMethod_GET,
				Url:        "https://example.com/user/exec",
			}},
			ScheduleTime: google.NewTimestamp(clock.Now(ctx).Add(time.Minute)),
		},
	}
}

func assertAll200(codes []int) {
	for _, code := range codes {
		if code != 200 {
			panic(fmt.Errorf("%v has non 200 code", codes))
		}
	}
}
