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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestCancelTaskParse(t *testing.T) {
	t.Parallel()

	expectErr := func(argv []string, errLike string) {
		_, code, _, stderr := SubcommandTest(
			context.Background(),
			CmdCancelTask,
			append([]string{"-server", "example.com"}, argv...),
			nil, nil,
		)
		assert.Loosely(t, code, should.Equal(1))
		assert.Loosely(t, stderr, should.ContainSubstring(errLike))
	}

	ftt.Run(`Make sure that Parse handles when no task ID is given.`, t, func(t *ftt.Test) {
		expectErr(nil, "expecting exactly 1 argument")
	})

	ftt.Run(`Make sure that Parse handles when too many task ID is given.`, t, func(t *ftt.Test) {
		expectErr([]string{"toomany234", "taskids567"}, "expecting exactly 1 argument")
	})
}

func TestCancelTask(t *testing.T) {
	t.Parallel()

	ftt.Run(`Cancel`, t, func(t *ftt.Test) {
		var givenTaskID string
		var givenKillRunning bool
		failTaskID := "failtask"

		service := &swarmingtest.Client{
			CancelTaskMock: func(ctx context.Context, taskID string, killRunning bool) (*swarmingpb.CancelResponse, error) {
				givenTaskID = taskID
				givenKillRunning = killRunning
				res := &swarmingpb.CancelResponse{
					Canceled:   true,
					WasRunning: givenKillRunning,
				}
				if givenTaskID == failTaskID {
					res.Canceled = false
					return res, nil
				}
				return res, nil
			},
		}

		t.Run(`Cancel task`, func(t *ftt.Test) {
			_, code, _, _ := SubcommandTest(
				context.Background(),
				CmdCancelTask,
				[]string{"-server", "example.com", "task"},
				nil, service,
			)
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, givenTaskID, should.Equal("task"))
		})

		t.Run(`Cancel running task `, func(t *ftt.Test) {
			_, code, _, _ := SubcommandTest(
				context.Background(),
				CmdCancelTask,
				[]string{"-server", "example.com", "-kill-running", "runningtask"},
				nil, service,
			)
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, givenTaskID, should.Equal("runningtask"))
			assert.Loosely(t, givenKillRunning, should.Equal(true))
		})

		t.Run(`Cancel task was not OK`, func(t *ftt.Test) {
			err, code, _, _ := SubcommandTest(
				context.Background(),
				CmdCancelTask,
				[]string{"-server", "example.com", failTaskID},
				nil, service,
			)
			assert.Loosely(t, code, should.Equal(1))
			assert.Loosely(t, err, should.ErrLike("failed to cancel task"))
			assert.Loosely(t, givenTaskID, should.Equal(failTaskID))
		})
	})
}
