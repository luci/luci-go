// Copyright 2017 The LUCI Authors.
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

package noop

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/utils/tasktest"
)

var _ task.Manager = (*TaskManager)(nil)

func TestFullFlow(t *testing.T) {
	t.Parallel()

	ftt.Run("Noop", t, func(t *ftt.Test) {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeUTC)
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			tc.Add(d)
		})

		mgr := TaskManager{}
		ctl := &tasktest.TestController{
			TaskMessage: &messages.NoopTask{
				SleepMs:       100,
				TriggersCount: 2,
			},
			SaveCallback: func() error { return nil },
		}

		assert.Loosely(t, mgr.LaunchTask(c, ctl), should.BeNil)
		assert.Loosely(t, ctl.TaskState, should.Match(task.State{
			Status: task.StatusSucceeded,
		}))
		assert.Loosely(t, ctl.Triggers, should.Match([]*internal.Trigger{
			{Id: "noop:1:0", Payload: &internal.Trigger_Noop{Noop: &api.NoopTrigger{}}},
			{Id: "noop:1:1", Payload: &internal.Trigger_Noop{Noop: &api.NoopTrigger{}}},
		}))
	})
}
