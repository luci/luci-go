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

	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
	swarming "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
		So(code, ShouldEqual, 1)
		So(stderr, ShouldContainSubstring, errLike)
	}

	Convey(`Make sure that Parse handles when no task ID is given.`, t, func() {
		expectErr(nil, "expecting exactly 1 argument")
	})

	Convey(`Make sure that Parse handles when too many task ID is given.`, t, func() {
		expectErr([]string{"toomany234", "taskids567"}, "expecting exactly 1 argument")
	})
}

func TestCancelTask(t *testing.T) {
	t.Parallel()

	Convey(`Cancel`, t, func() {
		var givenTaskID string
		var givenKillRunning bool
		failTaskID := "failtask"

		service := &swarmingtest.Client{
			CancelTaskMock: func(ctx context.Context, taskID string, killRunning bool) (*swarming.CancelResponse, error) {
				givenTaskID = taskID
				givenKillRunning = killRunning
				res := &swarming.CancelResponse{
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

		Convey(`Cancel task`, func() {
			_, code, _, _ := SubcommandTest(
				context.Background(),
				CmdCancelTask,
				[]string{"-server", "example.com", "task"},
				nil, service,
			)
			So(code, ShouldEqual, 0)
			So(givenTaskID, ShouldEqual, "task")
		})

		Convey(`Cancel running task `, func() {
			_, code, _, _ := SubcommandTest(
				context.Background(),
				CmdCancelTask,
				[]string{"-server", "example.com", "-kill-running", "runningtask"},
				nil, service,
			)
			So(code, ShouldEqual, 0)
			So(givenTaskID, ShouldEqual, "runningtask")
			So(givenKillRunning, ShouldEqual, true)
		})

		Convey(`Cancel task was not OK`, func() {
			err, code, _, _ := SubcommandTest(
				context.Background(),
				CmdCancelTask,
				[]string{"-server", "example.com", failTaskID},
				nil, service,
			)
			So(code, ShouldEqual, 1)
			So(err, ShouldErrLike, "failed to cancel task")
			So(givenTaskID, ShouldEqual, failTaskID)
		})
	})
}
