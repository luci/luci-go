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

package engine

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/filter/featureBreaker"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/taskqueue"

	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/common/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEnqueueInvocations(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		job := Job{JobID: "project/job-v2"}
		So(datastore.Put(c, &job), ShouldBeNil)

		var invs []*Invocation
		err := runTxn(c, func(c context.Context) error {
			var err error
			invs, err = e.enqueueInvocations(c, &job, []InvocationRequest{
				{
					TriggeredBy:      "user:a@example.com",
					IncomingTriggers: []task.Trigger{{ID: "1"}},
				},
				{
					TriggeredBy:      "user:b@example.com",
					IncomingTriggers: []task.Trigger{{ID: "2"}},
				},
			})
			datastore.Put(c, &job)
			return err
		})
		So(err, ShouldBeNil)

		// The order of new invocations is undefined (including IDs assigned to
		// them), so convert them to map and clear IDs.
		invsByTrigger := map[identity.Identity]Invocation{}
		invIDs := map[int64]bool{}
		for _, inv := range invs {
			invIDs[inv.ID] = true
			cpy := *inv
			So(cpy.ID, ShouldResemble, cpy.InvocationNonce)
			cpy.ID = 0
			cpy.InvocationNonce = 0
			invsByTrigger[inv.TriggeredBy] = cpy
		}
		So(invsByTrigger, ShouldResemble, map[identity.Identity]Invocation{
			"user:a@example.com": {
				JobID:            "project/job-v2",
				Started:          epoch,
				TriggeredBy:      "user:a@example.com",
				IncomingTriggers: []task.Trigger{{ID: "1"}},
				Status:           task.StatusStarting,
				DebugLog: "[22:42:00.000] New invocation initialized\n" +
					"[22:42:00.000] Manually triggered by user:a@example.com\n",
			},
			"user:b@example.com": {
				JobID:            "project/job-v2",
				Started:          epoch,
				TriggeredBy:      "user:b@example.com",
				IncomingTriggers: []task.Trigger{{ID: "2"}},
				Status:           task.StatusStarting,
				DebugLog: "[22:42:00.000] New invocation initialized\n" +
					"[22:42:00.000] Manually triggered by user:b@example.com\n",
			},
		})

		// Both invocations are in ActiveInvocations list of the job.
		So(len(job.ActiveInvocations), ShouldEqual, 2)
		for _, invID := range job.ActiveInvocations {
			So(invIDs[invID], ShouldBeTrue)
		}

		// And we've emitted the launch task.
		tasks := tq.GetScheduledTasks()
		So(tasks[0].Payload, ShouldHaveSameTypeAs, &internal.LaunchInvocationsBatchTask{})
		batch := tasks[0].Payload.(*internal.LaunchInvocationsBatchTask)
		So(len(batch.Tasks), ShouldEqual, 2)
		for _, subtask := range batch.Tasks {
			So(subtask.JobID, ShouldEqual, "project/job-v2")
			So(invIDs[subtask.InvID], ShouldBeTrue)
		}
	})
}

func TestTriageTaskDedup(t *testing.T) {
	t.Parallel()

	Convey("with fake env", t, func() {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		Convey("single task", func() {
			So(e.kickTriageJobStateTask(c, "fake/job"), ShouldBeNil)

			tasks := tq.GetScheduledTasks()
			So(len(tasks), ShouldEqual, 1)
			So(tasks[0].Task.ETA.Equal(epoch.Add(2*time.Second)), ShouldBeTrue)
			So(tasks[0].Payload, ShouldResemble, &internal.TriageJobStateTask{JobID: "fake/job"})
		})

		Convey("a bunch of tasks, deduplicated by hitting memcache", func() {
			So(e.kickTriageJobStateTask(c, "fake/job"), ShouldBeNil)

			clock.Get(c).(testclock.TestClock).Add(time.Second)
			So(e.kickTriageJobStateTask(c, "fake/job"), ShouldBeNil)

			clock.Get(c).(testclock.TestClock).Add(900 * time.Millisecond)
			So(e.kickTriageJobStateTask(c, "fake/job"), ShouldBeNil)

			tasks := tq.GetScheduledTasks()
			So(len(tasks), ShouldEqual, 1)
			So(tasks[0].Task.ETA.Equal(epoch.Add(2*time.Second)), ShouldBeTrue)
			So(tasks[0].Payload, ShouldResemble, &internal.TriageJobStateTask{JobID: "fake/job"})
		})

		Convey("a bunch of tasks, deduplicated by hitting task queue", func() {
			c, fb := featureBreaker.FilterMC(c, fmt.Errorf("omg, memcache error"))
			fb.BreakFeatures(nil, "GetMulti", "SetMulti")

			So(e.kickTriageJobStateTask(c, "fake/job"), ShouldBeNil)

			clock.Get(c).(testclock.TestClock).Add(time.Second)
			So(e.kickTriageJobStateTask(c, "fake/job"), ShouldBeNil)

			clock.Get(c).(testclock.TestClock).Add(900 * time.Millisecond)
			So(e.kickTriageJobStateTask(c, "fake/job"), ShouldBeNil)

			tasks := tq.GetScheduledTasks()
			So(len(tasks), ShouldEqual, 1)
			So(tasks[0].Task.ETA.Equal(epoch.Add(2*time.Second)), ShouldBeTrue)
			So(tasks[0].Payload, ShouldResemble, &internal.TriageJobStateTask{JobID: "fake/job"})
		})
	})
}

func TestLaunchInvocationTask(t *testing.T) {
	t.Parallel()

	Convey("with fake env", t, func() {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		// Add the job.
		So(e.UpdateProjectJobs(c, "project", []catalog.Definition{
			{
				JobID:    "project/job-v2",
				Revision: "rev1",
				Schedule: "*/5 * * * * * *",
				Task:     noopTaskBytes(),
				Acls:     aclOne,
			},
		}), ShouldBeNil)

		// Prepare Invocation in Starting state.
		job := Job{JobID: "project/job-v2"}
		So(datastore.Get(c, &job), ShouldBeNil)
		inv, err := e.allocateInvocation(c, &job, InvocationRequest{
			IncomingTriggers: []task.Trigger{{ID: "a"}},
		})
		So(err, ShouldBeNil)

		callLaunchInvocation := func(c context.Context, execCount int64) error {
			return tq.ExecuteTask(c, tqtesting.Task{
				Task: &taskqueue.Task{},
				Payload: &internal.LaunchInvocationTask{
					JobID: job.JobID,
					InvID: inv.ID,
				},
			}, &taskqueue.RequestHeaders{TaskExecutionCount: execCount})
		}

		fetchInvocation := func(c context.Context) *Invocation {
			toFetch := Invocation{ID: inv.ID}
			So(datastore.Get(c, &toFetch), ShouldBeNil)
			return &toFetch
		}

		Convey("happy path", func() {
			mgr.launchTask = func(ctx context.Context, ctl task.Controller, triggers []task.Trigger) error {
				So(ctl.InvocationID(), ShouldEqual, inv.ID)
				So(ctl.InvocationNonce(), ShouldEqual, inv.InvocationNonce)
				ctl.DebugLog("Succeeded!")
				ctl.State().Status = task.StatusSucceeded
				return nil
			}
			So(callLaunchInvocation(c, 0), ShouldBeNil)
			So(fetchInvocation(c), ShouldResemble, &Invocation{
				ID:               inv.ID,
				InvocationNonce:  inv.ID,
				JobID:            "project/job-v2",
				IndexedJobID:     "project/job-v2",
				Started:          epoch,
				Finished:         epoch,
				IncomingTriggers: []task.Trigger{{ID: "a"}},
				Revision:         job.Revision,
				Task:             job.Task,
				Status:           task.StatusSucceeded,
				MutationsCount:   2,
				DebugLog: "[22:42:00.000] New invocation initialized\n" +
					"[22:42:00.000] Starting the invocation (attempt 1)\n" +
					"[22:42:00.000] Succeeded!\n" +
					"[22:42:00.000] Invocation finished in 0s with status SUCCEEDED\n",
			})
		})

		Convey("already aborted", func() {
			inv.Status = task.StatusAborted
			So(datastore.Put(c, inv), ShouldBeNil)
			mgr.launchTask = func(ctx context.Context, ctl task.Controller, triggers []task.Trigger) error {
				return fmt.Errorf("must not be called")
			}
			So(callLaunchInvocation(c, 0), ShouldBeNil)
			So(fetchInvocation(c).Status, ShouldEqual, task.StatusAborted)
		})

		Convey("retying", func() {
			// Attempt #1.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller, triggers []task.Trigger) error {
				return transient.Tag.Apply(fmt.Errorf("oops, failed to start"))
			}
			So(callLaunchInvocation(c, 0), ShouldEqual, errRetryingLaunch)
			So(fetchInvocation(c).Status, ShouldEqual, task.StatusRetrying)

			// Attempt #2.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller, triggers []task.Trigger) error {
				ctl.DebugLog("Succeeded!")
				ctl.State().Status = task.StatusSucceeded
				return nil
			}
			So(callLaunchInvocation(c, 1), ShouldBeNil)
			So(fetchInvocation(c), ShouldResemble, &Invocation{
				ID:               inv.ID,
				InvocationNonce:  inv.ID,
				JobID:            "project/job-v2",
				IndexedJobID:     "project/job-v2",
				Started:          epoch,
				Finished:         epoch,
				IncomingTriggers: []task.Trigger{{ID: "a"}},
				Revision:         job.Revision,
				Task:             job.Task,
				Status:           task.StatusSucceeded,
				MutationsCount:   4,
				RetryCount:       1,
				DebugLog: "[22:42:00.000] New invocation initialized\n" +
					"[22:42:00.000] Starting the invocation (attempt 1)\n" +
					"[22:42:00.000] The invocation will be retried\n" +
					"[22:42:00.000] Starting the invocation (attempt 2)\n" +
					"[22:42:00.000] Succeeded!\n" +
					"[22:42:00.000] Invocation finished in 0s with status SUCCEEDED\n",
			})
		})
	})
}

func TestForceInvocationV2(t *testing.T) {
	t.Parallel()

	Convey("with fake env", t, func() {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		So(e.UpdateProjectJobs(c, "project", []catalog.Definition{
			{
				JobID:    "project/job-v2",
				Revision: "rev1",
				Schedule: "*/5 * * * * * *",
				Task:     noopTaskBytes(),
				Acls:     aclOne,
			},
		}), ShouldBeNil)

		Convey("happy path", func() {
			const expectedInvID int64 = 9200093523825174512

			futureInv, err := e.ForceInvocation(auth.WithState(c, asUserOne), "project/job-v2")
			So(err, ShouldBeNil)

			// Invocation ID is resolved right away.
			invID, err := futureInv.InvocationID(c)
			So(err, ShouldBeNil)
			So(invID, ShouldEqual, expectedInvID)

			// It is marked as active in the job state.
			job, err := e.getJob(c, "project/job-v2")
			So(err, ShouldBeNil)
			So(job.ActiveInvocations, ShouldResemble, []int64{invID})

			// All its fields are good.
			inv, err := e.getInvocation(c, "project/job-v2", invID)
			So(err, ShouldBeNil)
			So(inv, ShouldResemble, &Invocation{
				ID:              expectedInvID,
				JobID:           "project/job-v2",
				InvocationNonce: expectedInvID,
				Started:         epoch,
				TriggeredBy:     "user:one@example.com",
				Revision:        "rev1",
				Task:            noopTaskBytes(),
				Status:          task.StatusStarting,
				DebugLog: "[22:42:00.000] New invocation initialized\n" +
					"[22:42:00.000] Manually triggered by user:one@example.com\n",
			})

			// Eventually it runs the task, which then cleans up job state.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller, triggers []task.Trigger) error {
				ctl.DebugLog("Started!")
				ctl.State().Status = task.StatusSucceeded
				return nil
			}
			tasks, _, err := tq.RunSimulation(c, nil)
			So(err, ShouldBeNil)

			// The sequence of tasks we've just performed.
			So(len(tasks), ShouldEqual, 4)
			So(tasks[0].Payload, ShouldResemble, &internal.LaunchInvocationsBatchTask{
				Tasks: []*internal.LaunchInvocationTask{{JobID: "project/job-v2", InvID: expectedInvID}},
			})
			So(tasks[1].Payload, ShouldResemble, &internal.LaunchInvocationTask{
				JobID: "project/job-v2", InvID: expectedInvID,
			})
			So(tasks[2].Payload, ShouldResemble, &internal.InvocationFinishedTask{
				JobID: "project/job-v2", InvID: expectedInvID,
			})
			So(tasks[3].Payload, ShouldResemble, &internal.TriageJobStateTask{JobID: "project/job-v2"})

			// The invocation is in finished state.
			inv, err = e.getInvocation(c, "project/job-v2", invID)
			So(err, ShouldBeNil)
			So(inv, ShouldResemble, &Invocation{
				ID:              expectedInvID,
				JobID:           "project/job-v2",
				IndexedJobID:    "project/job-v2", // set for finished tasks!
				InvocationNonce: expectedInvID,
				Started:         epoch,
				Finished:        epoch.Add(time.Second),
				TriggeredBy:     "user:one@example.com",
				Revision:        "rev1",
				Task:            noopTaskBytes(),
				Status:          task.StatusSucceeded,
				MutationsCount:  2,
				DebugLog: "[22:42:00.000] New invocation initialized\n" +
					"[22:42:00.000] Manually triggered by user:one@example.com\n" +
					"[22:42:01.000] Starting the invocation (attempt 1)\n" +
					"[22:42:01.000] Started!\n" +
					"[22:42:01.000] Invocation finished in 1s with status SUCCEEDED\n",
			})

			// The job state is updated (the invocation is no longer active).
			job, err = e.getJob(c, "project/job-v2")
			So(err, ShouldBeNil)
			So(job.ActiveInvocations, ShouldBeNil)

			// The invocation is now in the list of finish invocations.
			datastore.GetTestable(c).CatchupIndexes()
			invs, _, _ := e.ListVisibleInvocations(auth.WithState(c, asUserOne), "project/job-v2", 100, "")
			So(invs, ShouldResemble, []*Invocation{inv})
		})
	})
}
