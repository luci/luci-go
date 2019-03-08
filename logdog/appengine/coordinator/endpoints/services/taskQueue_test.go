// Copyright 2019 The LUCI Authors.
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

package services

import (
	"testing"
	"time"

	"go.chromium.org/gae/service/taskqueue"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"

	"github.com/golang/protobuf/ptypes"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTaskQueue(t *testing.T) {
	Convey(`With a testing configuration`, t, func() {
		c, env := ct.Install(true)

		// By default, the testing user is a service.
		env.JoinGroup("services")
		svr := New()

		// The testable TQ object.
		ts := taskqueue.GetTestable(c)
		ts.CreatePullQueue(archiveQueueName)

		Convey(`Lease a task with empty taskqueue`, func() {
			tasks, err := svr.LeaseArchiveTasks(c, &logdog.LeaseRequest{
				MaxTasks:  10,
				LeaseTime: ptypes.DurationProto(10 * time.Minute),
			})
			So(err, ShouldBeNil)
			So(len(tasks.Tasks), ShouldEqual, 0)
		})
		task1, _ := tqTask(&logdog.ArchiveTask{Project: "foo", Id: "deadbeef1"})
		task2, _ := tqTask(&logdog.ArchiveTask{Project: "foo", Id: "deadbeef2"})
		task3, _ := tqTask(&logdog.ArchiveTask{Project: "foo", Id: "deadbeef3"})
		task4, _ := tqTask(&logdog.ArchiveTask{Project: "foo", Id: "deadbeef4"})

		Convey(`Two tasks`, func() {
			So(taskqueue.Add(c, archiveQueueName, []*taskqueue.Task{task1, task2}...), ShouldBeNil)
			tasks, err := svr.LeaseArchiveTasks(c, &logdog.LeaseRequest{
				MaxTasks:  10,
				LeaseTime: ptypes.DurationProto(10 * time.Minute),
			})
			So(err, ShouldBeNil)
			So(len(tasks.Tasks), ShouldEqual, 2)
			So(tasks.Tasks[0].Project, ShouldEqual, "foo")
			So(tasks.Tasks[0].Id, ShouldEqual, "deadbeef1")
			Convey(`And delete one of the tasks`, func() {
				_, err := svr.DeleteArchiveTasks(c, &logdog.DeleteRequest{
					Tasks: []*logdog.ArchiveTask{tasks.Tasks[0]},
				})
				So(err, ShouldBeNil)
				So(len(ts.GetScheduledTasks()[archiveQueueName]), ShouldEqual, 1)
			})
		})

		Convey(`Many tasks`, func() {
			taskqueue.Add(c, archiveQueueName, []*taskqueue.Task{task1, task2, task3, task4}...)
			tasks, err := svr.LeaseArchiveTasks(c, &logdog.LeaseRequest{
				MaxTasks:  3,
				LeaseTime: ptypes.DurationProto(10 * time.Minute),
			})
			So(err, ShouldBeNil)
			So(len(tasks.Tasks), ShouldEqual, 3)
		})

	})
}
