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

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCancelTaskParse(t *testing.T) {
	t.Parallel()

	Convey(`Test CancelTaskParse when there's no input or too many inputs`, t, func() {

		t := cancelRun{}
		t.Init(&testAuthFlags{})

		err := t.GetFlags().Parse([]string{"-server", "http://localhost:9050"})
		So(err, ShouldBeNil)

		Convey(`Test when one task ID is given.`, func() {
			err = t.parse([]string{"onetaskid111"})
			So(err, ShouldBeNil)
		})

		Convey(`Make sure that Parse handles when no task ID is given.`, func() {
			err = t.parse([]string{})
			So(err, ShouldErrLike, "must specify a swarming task ID")
		})

		Convey(`Make sure that Parse handles when too many task ID is given.`, func() {
			err = t.parse([]string{"toomany234", "taskids567"})
			So(err, ShouldErrLike, "specify only one")
		})
	})
}

func TestCancelTask(t *testing.T) {
	t.Parallel()

	Convey(`Cancel`, t, func() {
		ctx := context.Background()
		t := cancelRun{}

		var givenTaskID string
		var givenKillRunning bool
		failTaskID := "failtask"

		service := &testService{
			cancelTask: func(ctx context.Context, taskID string, req *swarming.SwarmingRpcsTaskCancelRequest) (*swarming.SwarmingRpcsCancelResponse, error) {
				givenTaskID = taskID
				givenKillRunning = req.KillRunning
				res := &swarming.SwarmingRpcsCancelResponse{
					Ok:         true,
					WasRunning: givenKillRunning,
				}
				if givenTaskID == failTaskID {
					res.Ok = false
					return res, nil
				}
				return res, nil
			},
		}

		Convey(`Cancel task`, func() {
			err := t.cancelTask(ctx, "task", service)
			So(err, ShouldBeNil)
			So(givenTaskID, ShouldEqual, "task")
		})

		Convey(`Cancel running task `, func() {
			t.killRunning = true
			err := t.cancelTask(ctx, "runningtask", service)
			So(err, ShouldBeNil)
			So(givenTaskID, ShouldEqual, "runningtask")
			So(givenKillRunning, ShouldEqual, true)
		})

		Convey(`Cancel task was not OK`, func() {
			err := t.cancelTask(ctx, failTaskID, service)
			So(err, ShouldErrLike, "failed to cancel task")
			So(givenTaskID, ShouldEqual, failTaskID)
		})
	})
}
