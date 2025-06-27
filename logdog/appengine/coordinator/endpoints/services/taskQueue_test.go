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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/taskqueue"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
)

func TestTaskQueue(t *testing.T) {
	ftt.Run(`With a testing configuration`, t, func(t *ftt.Test) {
		c, env := ct.Install()
		c = mathrand.Set(c, rand.New(rand.NewSource(1234)))

		// By default, the testing user is a service.
		env.ActAsService()
		svr := New(ServerSettings{NumQueues: 2})

		// The testable TQ object.
		ts := taskqueue.GetTestable(c)
		ts.CreatePullQueue(RawArchiveQueueName(0))
		ts.CreatePullQueue(RawArchiveQueueName(1))

		mustAddTask := func(t testing.TB, tsk *logdog.ArchiveTask) {
			t.Helper()
			task, err := tqTask(tsk)
			assert.Loosely(t, err, should.BeNil, truth.LineContext())

			queueName, _ := svr.(*logdog.DecoratedServices).Service.(*server).getNextArchiveQueueName(c)

			assert.Loosely(t, taskqueue.Add(c, queueName, task), should.BeNil, truth.LineContext())
		}

		t.Run(`Lease a task with empty taskqueue`, func(t *ftt.Test) {
			tasks, err := svr.LeaseArchiveTasks(c, &logdog.LeaseRequest{
				MaxTasks:  10,
				LeaseTime: durationpb.New(10 * time.Minute),
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(tasks.Tasks), should.BeZero)
		})
		task1 := &logdog.ArchiveTask{Project: "foo", Id: "deadbeef1", Realm: "foo:bar"}
		task2 := &logdog.ArchiveTask{Project: "foo", Id: "deadbeef2", Realm: "foo:bar"}
		task3 := &logdog.ArchiveTask{Project: "foo", Id: "deadbeef3", Realm: "foo:bar"}
		task4 := &logdog.ArchiveTask{Project: "foo", Id: "deadbeef4", Realm: "foo:bar"}

		t.Run(`Two tasks`, func(t *ftt.Test) {
			mustAddTask(t, task1)
			mustAddTask(t, task2)

			var leasedTasks []*logdog.ArchiveTask
			// We have to retry a couple times to collect all the tasks b/c
			// randomness. This is sensitive to the math.NewSource() value above.
			for range 3 {
				tasks, err := svr.LeaseArchiveTasks(c, &logdog.LeaseRequest{
					MaxTasks:  10,
					LeaseTime: durationpb.New(10 * time.Minute),
				})
				assert.Loosely(t, err, should.BeNil)
				leasedTasks = append(leasedTasks, tasks.Tasks...)
			}

			assert.Loosely(t, len(leasedTasks), should.Equal(2))
			// No order is guaranteed in the returned slice
			sort.Slice(leasedTasks, func(i, j int) bool {
				return leasedTasks[i].Id < leasedTasks[j].Id
			})
			assert.Loosely(t, leasedTasks[0].Project, should.Equal("foo"))
			assert.Loosely(t, leasedTasks[0].Id, should.Equal(task1.Id))
			assert.Loosely(t, leasedTasks[1].Id, should.Equal(task2.Id))
			t.Run(`And delete one of the tasks`, func(t *ftt.Test) {
				_, err := svr.DeleteArchiveTasks(c, &logdog.DeleteRequest{
					// leasedTasks[0] has TaskName filled, but task1 does not.
					Tasks: []*logdog.ArchiveTask{leasedTasks[0]},
				})
				assert.Loosely(t, err, should.BeNil)

				// there should only be one scheduled task remaining.
				numScheduled := 0
				for _, tasks := range ts.GetScheduledTasks() {
					numScheduled += len(tasks)
				}

				assert.Loosely(t, numScheduled, should.Equal(1))
			})
		})

		t.Run(`Many tasks`, func(t *ftt.Test) {
			mustAddTask(t, task1)
			mustAddTask(t, task2)
			mustAddTask(t, task3)
			mustAddTask(t, task4)

			var leasedTasks []*logdog.ArchiveTask
			// We have to retry a couple times to collect all the tasks b/c
			// randomness. This is sensitive to the math.NewSource() value above.
			for range 3 {
				tasks, err := svr.LeaseArchiveTasks(c, &logdog.LeaseRequest{
					MaxTasks:  1,
					LeaseTime: durationpb.New(10 * time.Minute),
				})
				assert.Loosely(t, err, should.BeNil)
				leasedTasks = append(leasedTasks, tasks.Tasks...)
			}

			assert.Loosely(t, len(leasedTasks), should.Equal(3))
		})

	})
}
