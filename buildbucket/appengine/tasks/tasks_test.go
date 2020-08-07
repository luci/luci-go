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

package tasks

import (
	"context"
	"testing"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/server/tq/tqtesting"

	taskdef "go.chromium.org/luci/buildbucket/appengine/tasks/defs"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTasks(t *testing.T) {
	t.Parallel()

	Convey("CancelSwarmingTask", t, func() {
		base := &tq.Dispatcher{}
		sch := base.SchedulerForTest()
		d := NewDispatcher(base)
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey("invalid", func() {
			sch.TaskSucceeded = func(ctx context.Context, task *tqtesting.Task) {
				panic("task validation didn't reject the task")
			}
			sch.TaskFailed = sch.TaskSucceeded

			Convey("nil", func() {
				So(d.CancelSwarmingTask(ctx, nil), ShouldErrLike, "hostname is required")
				sch.Run(ctx, tqtesting.StopWhenDrained())
			})

			Convey("empty", func() {
				task := &taskdef.CancelSwarmingTask{}
				So(d.CancelSwarmingTask(ctx, task), ShouldErrLike, "hostname is required")
				sch.Run(ctx, tqtesting.StopWhenDrained())
			})

			Convey("hostname", func() {
				task := &taskdef.CancelSwarmingTask{
					TaskId: "id",
				}
				So(d.CancelSwarmingTask(ctx, task), ShouldErrLike, "hostname is required")
				sch.Run(ctx, tqtesting.StopWhenDrained())
			})

			Convey("task id", func() {
				task := &taskdef.CancelSwarmingTask{
					Hostname: "example.com",
				}
				So(d.CancelSwarmingTask(ctx, task), ShouldErrLike, "task_id is required")
				sch.Run(ctx, tqtesting.StopWhenDrained())
			})
		})

		Convey("valid", func() {
			// TODO(crbug/1042991): Tests for CancelSwarmingTask.
		})
	})
}
