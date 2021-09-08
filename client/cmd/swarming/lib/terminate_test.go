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

package lib

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	googleapi "google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTerminateBotsParse(t *testing.T) {
	t.Parallel()

	Convey(`Test TerminateBotsParse when there's no input or too many inputs`, t, func() {

		t := terminateRun{}
		t.Init(&testAuthFlags{})

		err := t.GetFlags().Parse([]string{"-server", "http://localhost:9050"})
		So(err, ShouldBeNil)

		Convey(`Test when one bot ID is given.`, func() {
			err = t.parse([]string{"onebotid111"})
			So(err, ShouldBeNil)
		})

		Convey(`Make sure that Parse handles when no bot ID is given.`, func() {
			err = t.parse([]string{})
			So(err, ShouldErrLike, "must specify a")
		})

		Convey(`Make sure that Parse handles when too many bot ID is given.`, func() {
			err = t.parse([]string{"toomany234", "botids567"})
			So(err, ShouldErrLike, "specify only one")
		})
	})
}

func TestTerminateBots(t *testing.T) {
	t.Parallel()

	Convey(`Test terminate`, t, func() {
		ctx := context.Background()
		t := terminateRun{}
		taskStillRunningBotID := "stillrunningbotid123"
		failBotID := "failingbot123"
		errorAtTaskBotID := "errorattaskbot123"
		statusNotCompletedBotID := "errorattaskstatusbot123"
		stillRunningTaskID := "stillrunning456"
		failTaskID := "failingtask456"
		statusNotCompletedTaskID := "failingnoerrortask456"
		terminateTaskID := "itsgonnawork456"
		givenBotID := ""
		givenTaskID := ""
		countLoop := 0

		service := &testService{
			terminateBot: func(ctx context.Context, botID string) (*swarming.SwarmingRpcsTerminateResponse, error) {
				givenBotID = botID
				if botID == failBotID {
					return nil, &googleapi.Error{Code: 404}
				}
				if botID == taskStillRunningBotID {
					return &swarming.SwarmingRpcsTerminateResponse{
						TaskId: stillRunningTaskID,
					}, nil
				}
				if botID == errorAtTaskBotID {
					return &swarming.SwarmingRpcsTerminateResponse{
						TaskId: failTaskID,
					}, nil
				}
				if botID == statusNotCompletedBotID {
					return &swarming.SwarmingRpcsTerminateResponse{
						TaskId: statusNotCompletedTaskID,
					}, nil
				}
				return &swarming.SwarmingRpcsTerminateResponse{
					TaskId: terminateTaskID,
				}, nil
			},
			taskResult: func(ctx context.Context, taskID string, _ bool) (*swarming.SwarmingRpcsTaskResult, error) {
				givenTaskID = taskID
				if taskID == stillRunningTaskID && countLoop < 2 {
					countLoop += 1
					return &swarming.SwarmingRpcsTaskResult{
						State: "RUNNING",
					}, nil
				}
				if taskID == failTaskID {
					return &swarming.SwarmingRpcsTaskResult{
						State: "TIMED_OUT",
					}, errors.New("failed to call GetTaskResult")
				}
				if taskID == statusNotCompletedTaskID {
					return &swarming.SwarmingRpcsTaskResult{
						State: "BOT_DIED",
					}, nil
				}
				return &swarming.SwarmingRpcsTaskResult{
					State: "COMPLETED",
				}, nil
			},
		}

		Convey(`Test terminating bot`, func() {
			err := t.terminateBot(ctx, "testbot123", service)
			So(err, ShouldBeNil)
			So(givenBotID, ShouldEqual, "testbot123")
		})

		Convey(`Test when terminating bot fails`, func() {
			err := t.terminateBot(ctx, failBotID, service)
			So(err, ShouldErrLike, "404")
			So(givenBotID, ShouldEqual, failBotID)
		})

		Convey(`Test terminating bot when wait`, func() {
			t.wait = true
			err := t.terminateBot(ctx, "testbot123", service)
			So(err, ShouldBeNil)
			So(givenBotID, ShouldEqual, "testbot123")
			So(givenTaskID, ShouldEqual, terminateTaskID)
		})

		Convey(`Test terminating bot when wait and bot doesn't stop running immediately`, func() {
			ctx, clk := testclock.UseTime(ctx, time.Now())
			clk.SetTimerCallback(func(d time.Duration, t clock.Timer) {
				clk.Add(d)
			})
			t.wait = true
			err := t.terminateBot(ctx, taskStillRunningBotID, service)
			So(err, ShouldBeNil)
			So(countLoop, ShouldEqual, 2)
			So(givenBotID, ShouldEqual, taskStillRunningBotID)
			So(givenTaskID, ShouldEqual, stillRunningTaskID)
		})

		Convey(`Test terminating bot when wait fails with error`, func() {
			t.wait = true
			err := t.terminateBot(ctx, errorAtTaskBotID, service)
			So(err, ShouldErrLike, "failed to call")
			So(givenBotID, ShouldEqual, errorAtTaskBotID)
			So(givenTaskID, ShouldEqual, failTaskID)
		})

		Convey(`other than status "complete" returned when terminating bot with wait`, func() {
			t.wait = true
			err := t.terminateBot(ctx, statusNotCompletedBotID, service)
			So(err, ShouldErrLike, "failed to terminate bot")
			So(givenBotID, ShouldEqual, statusNotCompletedBotID)
			So(givenTaskID, ShouldEqual, statusNotCompletedTaskID)
		})

	})
}
