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
	"testing"
	"time"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/utils/tasktest"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

var _ task.Manager = (*TaskManager)(nil)

func TestFullFlow(t *testing.T) {
	Convey("Noop", t, func() {
		c := memory.Use(context.Background())
		mgr := TaskManager{sleepDuration: time.Nanosecond}
		ctl := &tasktest.TestController{
			TaskMessage:  &messages.NoopTask{},
			SaveCallback: func() error { return nil },
		}
		So(mgr.LaunchTask(c, ctl, nil), ShouldBeNil)
		So(ctl.TaskState, ShouldResemble, task.State{
			Status: task.StatusSucceeded,
		})
	})
}
