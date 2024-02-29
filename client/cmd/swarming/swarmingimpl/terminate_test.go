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

package swarmingimpl

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/swarming/client/swarming"
	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTerminateBotsParse(t *testing.T) {
	t.Parallel()

	expectErr := func(argv []string, errLike string) {
		_, code, _, stderr := SubcommandTest(
			context.Background(),
			CmdTerminateBot,
			append([]string{"-server", "example.com"}, argv...),
			nil, nil,
		)
		So(code, ShouldEqual, 1)
		So(stderr, ShouldContainSubstring, errLike)
	}

	Convey(`Wants a bot ID`, t, func() {
		expectErr(nil, "expecting exactly 1 argument")
	})

	Convey(`Wants only one bot ID`, t, func() {
		expectErr([]string{"b1", "b2"}, "expecting exactly 1 argument")
	})
}

func TestTerminateBots(t *testing.T) {
	t.Parallel()

	Convey(`Test terminate`, t, func() {
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

		ctx, clk := testclock.UseTime(context.Background(), time.Now())
		clk.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			clk.Add(d)
		})

		service := &swarmingtest.Client{
			TerminateBotMock: func(ctx context.Context, botID string, reason string) (*swarmingv2.TerminateResponse, error) {
				givenBotID = botID
				if botID == failBotID {
					return nil, status.Errorf(codes.NotFound, "no such bot")
				}
				if botID == taskStillRunningBotID {
					return &swarmingv2.TerminateResponse{
						TaskId: stillRunningTaskID,
					}, nil
				}
				if botID == errorAtTaskBotID {
					return &swarmingv2.TerminateResponse{
						TaskId: failTaskID,
					}, nil
				}
				if botID == statusNotCompletedBotID {
					return &swarmingv2.TerminateResponse{
						TaskId: statusNotCompletedTaskID,
					}, nil
				}
				return &swarmingv2.TerminateResponse{
					TaskId: terminateTaskID,
				}, nil
			},
			TaskResultMock: func(ctx context.Context, taskID string, _ *swarming.TaskResultFields) (*swarmingv2.TaskResultResponse, error) {
				givenTaskID = taskID
				if taskID == stillRunningTaskID && countLoop < 2 {
					countLoop += 1
					return &swarmingv2.TaskResultResponse{
						TaskId: taskID,
						State:  swarmingv2.TaskState_RUNNING,
					}, nil
				}
				if taskID == failTaskID {
					return nil, status.Errorf(codes.PermissionDenied, "failed to call GetTaskResult")
				}
				if taskID == statusNotCompletedTaskID {
					return &swarmingv2.TaskResultResponse{
						TaskId: taskID,
						State:  swarmingv2.TaskState_BOT_DIED,
					}, nil
				}
				return &swarmingv2.TaskResultResponse{
					TaskId: taskID,
					State:  swarmingv2.TaskState_COMPLETED,
				}, nil
			},
		}

		Convey(`Test terminating bot`, func() {
			_, code, _, _ := SubcommandTest(
				ctx,
				CmdTerminateBot,
				[]string{"-server", "example.com", "testbot123"},
				nil, service,
			)
			So(code, ShouldEqual, 0)
			So(givenBotID, ShouldEqual, "testbot123")
		})

		Convey(`Test when terminating bot fails`, func() {
			err, code, _, _ := SubcommandTest(
				ctx,
				CmdTerminateBot,
				[]string{"-server", "example.com", failBotID},
				nil, service,
			)
			So(code, ShouldEqual, 1)
			So(err, ShouldErrLike, "no such bot")
			So(givenBotID, ShouldEqual, failBotID)
		})

		Convey(`Test terminating bot with waiting`, func() {
			_, code, _, _ := SubcommandTest(
				ctx,
				CmdTerminateBot,
				[]string{"-server", "example.com", "-wait", "testbot123"},
				nil, service,
			)
			So(code, ShouldEqual, 0)
			So(givenBotID, ShouldEqual, "testbot123")
			So(givenTaskID, ShouldEqual, terminateTaskID)
		})

		Convey(`Test terminating bot with waiting and bot doesn't stop running immediately`, func() {
			_, code, _, _ := SubcommandTest(
				ctx,
				CmdTerminateBot,
				[]string{"-server", "example.com", "-wait", taskStillRunningBotID},
				nil, service,
			)
			So(code, ShouldEqual, 0)
			So(countLoop, ShouldEqual, 2)
			So(givenBotID, ShouldEqual, taskStillRunningBotID)
			So(givenTaskID, ShouldEqual, stillRunningTaskID)
		})

		Convey(`Test terminating bot when wait fails with error`, func() {
			err, code, _, _ := SubcommandTest(
				ctx,
				CmdTerminateBot,
				[]string{"-server", "example.com", "-wait", errorAtTaskBotID},
				nil, service,
			)
			So(code, ShouldEqual, 1)
			So(err, ShouldErrLike, "failed to call")
			So(givenBotID, ShouldEqual, errorAtTaskBotID)
			So(givenTaskID, ShouldEqual, failTaskID)
		})

		Convey(`Other than status "complete" returned when terminating bot with wait`, func() {
			err, code, _, _ := SubcommandTest(
				ctx,
				CmdTerminateBot,
				[]string{"-server", "example.com", "-wait", statusNotCompletedBotID},
				nil, service,
			)
			So(code, ShouldEqual, 1)
			So(err, ShouldErrLike, "failed to terminate bot")
			So(givenBotID, ShouldEqual, statusNotCompletedBotID)
			So(givenTaskID, ShouldEqual, statusNotCompletedTaskID)
		})
	})
}
