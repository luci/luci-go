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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/swarming/client/swarming"
	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
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
		assert.Loosely(t, code, should.Equal(1))
		assert.Loosely(t, stderr, should.ContainSubstring(errLike))
	}

	ftt.Run(`Wants a bot ID`, t, func(t *ftt.Test) {
		expectErr(nil, "expecting exactly 1 argument")
	})

	ftt.Run(`Wants only one bot ID`, t, func(t *ftt.Test) {
		expectErr([]string{"b1", "b2"}, "expecting exactly 1 argument")
	})
}

func TestTerminateBots(t *testing.T) {
	t.Parallel()

	ftt.Run(`Test terminate`, t, func(t *ftt.Test) {
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
			TerminateBotMock: func(ctx context.Context, botID string, reason string) (*swarmingpb.TerminateResponse, error) {
				givenBotID = botID
				if botID == failBotID {
					return nil, status.Errorf(codes.NotFound, "no such bot")
				}
				if botID == taskStillRunningBotID {
					return &swarmingpb.TerminateResponse{
						TaskId: stillRunningTaskID,
					}, nil
				}
				if botID == errorAtTaskBotID {
					return &swarmingpb.TerminateResponse{
						TaskId: failTaskID,
					}, nil
				}
				if botID == statusNotCompletedBotID {
					return &swarmingpb.TerminateResponse{
						TaskId: statusNotCompletedTaskID,
					}, nil
				}
				return &swarmingpb.TerminateResponse{
					TaskId: terminateTaskID,
				}, nil
			},
			TaskResultMock: func(ctx context.Context, taskID string, _ *swarming.TaskResultFields) (*swarmingpb.TaskResultResponse, error) {
				givenTaskID = taskID
				if taskID == stillRunningTaskID && countLoop < 2 {
					countLoop += 1
					return &swarmingpb.TaskResultResponse{
						TaskId: taskID,
						State:  swarmingpb.TaskState_RUNNING,
					}, nil
				}
				if taskID == failTaskID {
					return nil, status.Errorf(codes.PermissionDenied, "failed to call GetTaskResult")
				}
				if taskID == statusNotCompletedTaskID {
					return &swarmingpb.TaskResultResponse{
						TaskId: taskID,
						State:  swarmingpb.TaskState_BOT_DIED,
					}, nil
				}
				return &swarmingpb.TaskResultResponse{
					TaskId: taskID,
					State:  swarmingpb.TaskState_COMPLETED,
				}, nil
			},
		}

		t.Run(`Test terminating bot`, func(t *ftt.Test) {
			_, code, _, _ := SubcommandTest(
				ctx,
				CmdTerminateBot,
				[]string{"-server", "example.com", "testbot123"},
				nil, service,
			)
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, givenBotID, should.Equal("testbot123"))
		})

		t.Run(`Test when terminating bot fails`, func(t *ftt.Test) {
			err, code, _, _ := SubcommandTest(
				ctx,
				CmdTerminateBot,
				[]string{"-server", "example.com", failBotID},
				nil, service,
			)
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, err, should.ErrLike("no such bot"))
			assert.Loosely(t, givenBotID, should.Equal(failBotID))
		})

		t.Run(`Test terminating bot with waiting`, func(t *ftt.Test) {
			_, code, _, _ := SubcommandTest(
				ctx,
				CmdTerminateBot,
				[]string{"-server", "example.com", "-wait", "testbot123"},
				nil, service,
			)
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, givenBotID, should.Equal("testbot123"))
			assert.Loosely(t, givenTaskID, should.Equal(terminateTaskID))
		})

		t.Run(`Test terminating bot with waiting and bot doesn't stop running immediately`, func(t *ftt.Test) {
			_, code, _, _ := SubcommandTest(
				ctx,
				CmdTerminateBot,
				[]string{"-server", "example.com", "-wait", taskStillRunningBotID},
				nil, service,
			)
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, countLoop, should.Equal(2))
			assert.Loosely(t, givenBotID, should.Equal(taskStillRunningBotID))
			assert.Loosely(t, givenTaskID, should.Equal(stillRunningTaskID))
		})

		t.Run(`Test terminating bot when wait fails with error`, func(t *ftt.Test) {
			err, code, _, _ := SubcommandTest(
				ctx,
				CmdTerminateBot,
				[]string{"-server", "example.com", "-wait", errorAtTaskBotID},
				nil, service,
			)
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, err, should.ErrLike("failed to call"))
			assert.Loosely(t, givenBotID, should.Equal(errorAtTaskBotID))
			assert.Loosely(t, givenTaskID, should.Equal(failTaskID))
		})

		t.Run(`Other than status "complete" returned when terminating bot with wait`, func(t *ftt.Test) {
			err, code, _, _ := SubcommandTest(
				ctx,
				CmdTerminateBot,
				[]string{"-server", "example.com", "-wait", statusNotCompletedBotID},
				nil, service,
			)
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, err, should.ErrLike("failed to terminate bot"))
			assert.Loosely(t, givenBotID, should.Equal(statusNotCompletedBotID))
			assert.Loosely(t, givenTaskID, should.Equal(statusNotCompletedTaskID))
		})
	})
}
