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
	"math/rand"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/gae/service/taskqueue"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTaskQueue(t *testing.T) {
	Convey(`With a testing configuration`, t, func() {
		c, env := ct.Install(true)
		c = mathrand.Set(c, rand.New(rand.NewSource(1234)))

		// By default, the testing user is a service.
		env.ActAsService()
		svr := New(ServerSettings{NumQueues: 2})

		// The testable TQ object.
		ts := taskqueue.GetTestable(c)
		ts.CreatePullQueue(RawArchiveQueueName(0))
		ts.CreatePullQueue(RawArchiveQueueName(1))

		mustAddTask := func(t *logdog.ArchiveTask) {
			task, err := tqTask(t)
			So(err, ShouldBeNil)

			queueName, _ := svr.(*logdog.DecoratedServices).Service.(*server).getNextArchiveQueueName(c)

			So(taskqueue.Add(c, queueName, task), ShouldBeNil)
		}

		Convey(`Lease a task with empty taskqueue`, func() {
			tasks, err := svr.LeaseArchiveTasks(c, &logdog.LeaseRequest{
				MaxTasks:  10,
				LeaseTime: durationpb.New(10 * time.Minute),
			})
			So(err, ShouldBeNil)
			So(len(tasks.Tasks), ShouldEqual, 0)
		})
		task1 := &logdog.ArchiveTask{Project: "foo", Id: "deadbeef1", Realm: "foo:bar"}
		task2 := &logdog.ArchiveTask{Project: "foo", Id: "deadbeef2", Realm: "foo:bar"}
		task3 := &logdog.ArchiveTask{Project: "foo", Id: "deadbeef3", Realm: "foo:bar"}
		task4 := &logdog.ArchiveTask{Project: "foo", Id: "deadbeef4", Realm: "foo:bar"}

		Convey(`Two tasks`, func() {
			mustAddTask(task1)
			mustAddTask(task2)

			var leasedTasks []*logdog.ArchiveTask
			// We have to retry a couple times to collect all the tasks b/c
			// randomness. This is sensitive to the math.NewSource() value above.
			for i := 0; i < 3; i++ {
				tasks, err := svr.LeaseArchiveTasks(c, &logdog.LeaseRequest{
					MaxTasks:  10,
					LeaseTime: durationpb.New(10 * time.Minute),
				})
				So(err, ShouldBeNil)
				leasedTasks = append(leasedTasks, tasks.Tasks...)
			}

			So(len(leasedTasks), ShouldEqual, 2)
			// No order is guaranteed in the returned slice
			sort.Slice(leasedTasks, func(i, j int) bool {
				return leasedTasks[i].Id < leasedTasks[j].Id
			})
			So(leasedTasks[0].Project, ShouldEqual, "foo")
			So(leasedTasks[0].Id, ShouldEqual, task1.Id)
			So(leasedTasks[1].Id, ShouldEqual, task2.Id)
			Convey(`And delete one of the tasks`, func() {
				_, err := svr.DeleteArchiveTasks(c, &logdog.DeleteRequest{
					// leasedTasks[0] has TaskName filled, but task1 does not.
					Tasks: []*logdog.ArchiveTask{leasedTasks[0]},
				})
				So(err, ShouldBeNil)

				// there should only be one scheduled task remaining.
				numScheduled := 0
				for _, tasks := range ts.GetScheduledTasks() {
					numScheduled += len(tasks)
				}

				So(numScheduled, ShouldEqual, 1)
			})
		})

		Convey(`Many tasks`, func() {
			mustAddTask(task1)
			mustAddTask(task2)
			mustAddTask(task3)
			mustAddTask(task4)

			var leasedTasks []*logdog.ArchiveTask
			// We have to retry a couple times to collect all the tasks b/c
			// randomness. This is sensitive to the math.NewSource() value above.
			for i := 0; i < 3; i++ {
				tasks, err := svr.LeaseArchiveTasks(c, &logdog.LeaseRequest{
					MaxTasks:  1,
					LeaseTime: durationpb.New(10 * time.Minute),
				})
				So(err, ShouldBeNil)
				leasedTasks = append(leasedTasks, tasks.Tasks...)
			}

			So(len(leasedTasks), ShouldEqual, 3)
		})

	})
}
