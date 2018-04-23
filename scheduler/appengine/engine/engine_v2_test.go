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
	"sort"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/filter/featureBreaker"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/taskqueue"

	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
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
			invs, err = e.enqueueInvocations(c, &job, []task.Request{
				{TriggeredBy: "user:a@example.com"},
				{TriggeredBy: "user:b@example.com"},
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
				JobID:       "project/job-v2",
				Started:     epoch,
				TriggeredBy: "user:a@example.com",
				Status:      task.StatusStarting,
				DebugLog: "[22:42:00.000] New invocation initialized\n" +
					"[22:42:00.000] Triggered by user:a@example.com\n",
			},
			"user:b@example.com": {
				JobID:       "project/job-v2",
				Started:     epoch,
				TriggeredBy: "user:b@example.com",
				Status:      task.StatusStarting,
				DebugLog: "[22:42:00.000] New invocation initialized\n" +
					"[22:42:00.000] Triggered by user:b@example.com\n",
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
			So(subtask.JobId, ShouldEqual, "project/job-v2")
			So(invIDs[subtask.InvId], ShouldBeTrue)
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
			So(e.kickTriageNow(c, "fake/job"), ShouldBeNil)

			tasks := tq.GetScheduledTasks()
			So(len(tasks), ShouldEqual, 1)
			So(tasks[0].Task.ETA.Equal(epoch.Add(2*time.Second)), ShouldBeTrue)
			So(tasks[0].Payload, ShouldResemble, &internal.TriageJobStateTask{JobId: "fake/job"})
		})

		Convey("a bunch of tasks, deduplicated by hitting memcache", func() {
			So(e.kickTriageNow(c, "fake/job"), ShouldBeNil)

			clock.Get(c).(testclock.TestClock).Add(time.Second)
			So(e.kickTriageNow(c, "fake/job"), ShouldBeNil)

			clock.Get(c).(testclock.TestClock).Add(900 * time.Millisecond)
			So(e.kickTriageNow(c, "fake/job"), ShouldBeNil)

			tasks := tq.GetScheduledTasks()
			So(len(tasks), ShouldEqual, 1)
			So(tasks[0].Task.ETA.Equal(epoch.Add(2*time.Second)), ShouldBeTrue)
			So(tasks[0].Payload, ShouldResemble, &internal.TriageJobStateTask{JobId: "fake/job"})
		})

		Convey("a bunch of tasks, deduplicated by hitting task queue", func() {
			c, fb := featureBreaker.FilterMC(c, fmt.Errorf("omg, memcache error"))
			fb.BreakFeatures(nil, "GetMulti", "SetMulti")

			So(e.kickTriageNow(c, "fake/job"), ShouldBeNil)

			clock.Get(c).(testclock.TestClock).Add(time.Second)
			So(e.kickTriageNow(c, "fake/job"), ShouldBeNil)

			clock.Get(c).(testclock.TestClock).Add(900 * time.Millisecond)
			So(e.kickTriageNow(c, "fake/job"), ShouldBeNil)

			tasks := tq.GetScheduledTasks()
			So(len(tasks), ShouldEqual, 1)
			So(tasks[0].Task.ETA.Equal(epoch.Add(2*time.Second)), ShouldBeTrue)
			So(tasks[0].Payload, ShouldResemble, &internal.TriageJobStateTask{JobId: "fake/job"})
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
				Schedule: "triggered",
				Task:     noopTaskBytes(),
				Acls:     aclOne,
			},
		}), ShouldBeNil)

		// Prepare Invocation in Starting state.
		job := Job{JobID: "project/job-v2"}
		So(datastore.Get(c, &job), ShouldBeNil)
		inv, err := e.allocateInvocation(c, &job, task.Request{
			IncomingTriggers: []*internal.Trigger{{Id: "a"}},
		})
		So(err, ShouldBeNil)

		callLaunchInvocation := func(c context.Context, execCount int64) error {
			return tq.ExecuteTask(c, tqtesting.Task{
				Task: &taskqueue.Task{},
				Payload: &internal.LaunchInvocationTask{
					JobId: job.JobID,
					InvId: inv.ID,
				},
			}, &taskqueue.RequestHeaders{TaskExecutionCount: execCount})
		}

		fetchInvocation := func(c context.Context) *Invocation {
			toFetch := Invocation{ID: inv.ID}
			So(datastore.Get(c, &toFetch), ShouldBeNil)
			return &toFetch
		}

		Convey("happy path", func() {
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				So(ctl.InvocationID(), ShouldEqual, inv.ID)
				So(ctl.InvocationNonce(), ShouldEqual, inv.InvocationNonce)
				ctl.DebugLog("Succeeded!")
				ctl.State().Status = task.StatusSucceeded
				return nil
			}
			So(callLaunchInvocation(c, 0), ShouldBeNil)

			updated := fetchInvocation(c)
			triggers, err := updated.IncomingTriggers()
			updated.IncomingTriggersRaw = nil

			So(err, ShouldBeNil)
			So(triggers, ShouldResemble, []*internal.Trigger{{Id: "a"}})
			So(updated, ShouldResemble, &Invocation{
				ID:              inv.ID,
				InvocationNonce: inv.ID,
				JobID:           "project/job-v2",
				IndexedJobID:    "project/job-v2",
				Started:         epoch,
				Finished:        epoch,
				Revision:        job.Revision,
				Task:            job.Task,
				Status:          task.StatusSucceeded,
				MutationsCount:  2,
				DebugLog: "[22:42:00.000] New invocation initialized\n" +
					"[22:42:00.000] Starting the invocation (attempt 1)\n" +
					"[22:42:00.000] Succeeded!\n" +
					"[22:42:00.000] Invocation finished in 0s with status SUCCEEDED\n",
			})
		})

		Convey("already aborted", func() {
			inv.Status = task.StatusAborted
			So(datastore.Put(c, inv), ShouldBeNil)
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				return fmt.Errorf("must not be called")
			}
			So(callLaunchInvocation(c, 0), ShouldBeNil)
			So(fetchInvocation(c).Status, ShouldEqual, task.StatusAborted)
		})

		Convey("retying", func() {
			// Attempt #1.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				return transient.Tag.Apply(fmt.Errorf("oops, failed to start"))
			}
			So(callLaunchInvocation(c, 0), ShouldEqual, errRetryingLaunch)
			So(fetchInvocation(c).Status, ShouldEqual, task.StatusRetrying)

			// Attempt #2.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				ctl.DebugLog("Succeeded!")
				ctl.State().Status = task.StatusSucceeded
				return nil
			}
			So(callLaunchInvocation(c, 1), ShouldBeNil)

			updated := fetchInvocation(c)
			So(updated.Status, ShouldEqual, task.StatusSucceeded)
			So(updated.RetryCount, ShouldEqual, 1)
			So(updated.DebugLog, ShouldEqual, "[22:42:00.000] New invocation initialized\n"+
				"[22:42:00.000] Starting the invocation (attempt 1)\n"+
				"[22:42:00.000] The invocation will be retried\n"+
				"[22:42:00.000] Starting the invocation (attempt 2)\n"+
				"[22:42:00.000] Succeeded!\n"+
				"[22:42:00.000] Invocation finished in 0s with status SUCCEEDED\n")
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
				Schedule: "triggered",
				Task:     noopTaskBytes(),
				Acls:     aclOne,
			},
		}), ShouldBeNil)

		Convey("happy path", func() {
			const expectedInvID int64 = 9200093523825193008

			job, err := e.getJob(c, "project/job-v2")
			So(err, ShouldBeNil)
			futureInv, err := e.ForceInvocation(auth.WithState(c, asUserOne), job)
			So(err, ShouldBeNil)

			// Invocation ID is resolved right away.
			invID, err := futureInv.InvocationID(c)
			So(err, ShouldBeNil)
			So(invID, ShouldEqual, expectedInvID)

			// It is now marked as active in the job state, refetch it to check.
			job, err = e.getJob(c, "project/job-v2")
			So(err, ShouldBeNil)
			So(job.ActiveInvocations, ShouldResemble, []int64{invID})

			// All its fields are good.
			inv, err := e.getInvocation(c, "project/job-v2", invID, true)
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
					"[22:42:00.000] Triggered by user:one@example.com\n",
			})

			// Eventually it runs the task, which then cleans up job state.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				ctl.DebugLog("Started!")
				ctl.State().Status = task.StatusSucceeded
				return nil
			}
			tasks, _, err := tq.RunSimulation(c, nil)
			So(err, ShouldBeNil)

			// The sequence of tasks we've just performed.
			So(tasks.Payloads(), ShouldResemble, []proto.Message{
				&internal.LaunchInvocationsBatchTask{
					Tasks: []*internal.LaunchInvocationTask{{JobId: "project/job-v2", InvId: expectedInvID}},
				},
				&internal.LaunchInvocationTask{
					JobId: "project/job-v2", InvId: expectedInvID,
				},
				&internal.InvocationFinishedTask{
					JobId: "project/job-v2", InvId: expectedInvID,
				},
				&internal.TriageJobStateTask{JobId: "project/job-v2"},
			})

			// The invocation is in finished state.
			inv, err = e.getInvocation(c, "project/job-v2", invID, true)
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
					"[22:42:00.000] Triggered by user:one@example.com\n" +
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
			invs, _, _ := e.ListInvocations(auth.WithState(c, asUserOne), job, ListInvocationsOpts{
				PageSize: 100,
			})
			So(invs, ShouldResemble, []*Invocation{inv})
		})
	})
}

func TestEmitTriggers(t *testing.T) {
	t.Parallel()

	Convey("with fake env", t, func() {
		const testingJob = "project/job-v2"

		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		So(e.UpdateProjectJobs(c, "project", []catalog.Definition{
			{
				JobID:    testingJob,
				Revision: "rev1",
				Schedule: "triggered",
				Task:     noopTaskBytes(),
				Acls:     aclOne, // owned by one@example.com
			},
		}), ShouldBeNil)

		Convey("happy path", func() {
			var incomingTriggers []*internal.Trigger
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				incomingTriggers = ctl.Request().IncomingTriggers
				ctl.State().Status = task.StatusSucceeded
				return nil
			}

			job, err := e.getJob(c, testingJob)
			So(err, ShouldBeNil)

			// Simulate EmitTriggers call from an owner.
			emittedTriggers := []*internal.Trigger{
				{
					Id:           "t1",
					OrderInBatch: 1,
					Payload: &internal.Trigger_Buildbucket{
						Buildbucket: &api.BuildbucketTrigger{
							Tags: []string{"a:b"},
						},
					},
				},
				{
					Id:           "t2",
					OrderInBatch: 2,
				},
			}
			asOne := auth.WithState(c, &authtest.FakeState{Identity: "user:one@example.com"})
			err = e.EmitTriggers(asOne, map[*Job][]*internal.Trigger{job: emittedTriggers})
			So(err, ShouldBeNil)

			// Run TQ until all activities stop.
			tasks, _, err := tq.RunSimulation(c, nil)
			So(err, ShouldBeNil)

			// We expect a triage invoked by EmitTrigger, and one full invocation.
			expect := expectedTasks{JobID: testingJob, Epoch: epoch}
			expect.triage()
			expect.invocationSequence(9200093521727759856)
			expect.triage()
			So(tasks.Payloads(), ShouldResemble, expect.Tasks)

			// The task manager received all triggers (though maybe out of order, it
			// is not defined).
			sort.Slice(incomingTriggers, func(i, j int) bool {
				return incomingTriggers[i].Id < incomingTriggers[j].Id
			})
			So(incomingTriggers, ShouldResemble, emittedTriggers)
		})

		Convey("no TRIGGERER permission", func() {
			job, err := e.getJob(c, testingJob)
			So(err, ShouldBeNil)

			asTwo := auth.WithState(c, &authtest.FakeState{Identity: "user:two@example.com"})
			err = e.EmitTriggers(asTwo, map[*Job][]*internal.Trigger{
				job: {
					{Id: "t1"},
				},
			})
			So(err, ShouldEqual, ErrNoPermission)
		})

		Convey("paused job ignores triggers", func() {
			job, err := e.getJob(c, testingJob)
			So(err, ShouldBeNil)
			So(e.setJobPausedFlag(c, job, true, ""), ShouldBeNil)

			// The pause emits a triage, get over it now.
			tasks, _, err := tq.RunSimulation(c, nil)
			So(err, ShouldBeNil)
			expect := expectedTasks{JobID: testingJob, Epoch: epoch}
			expect.kickTriage()
			expect.triage()
			So(tasks.Payloads(), ShouldResemble, expect.Tasks)

			// Make the RPC, which succeeds.
			asOne := auth.WithState(c, &authtest.FakeState{Identity: "user:one@example.com"})
			err = e.EmitTriggers(asOne, map[*Job][]*internal.Trigger{
				job: {
					{Id: "t1"},
				},
			})
			So(err, ShouldBeNil)

			// But nothing really happens.
			tasks, _, err = tq.RunSimulation(c, nil)
			So(err, ShouldBeNil)
			So(len(tasks.Payloads()), ShouldEqual, 0)
		})
	})
}

func TestOneJobTriggersAnother(t *testing.T) {
	t.Parallel()

	Convey("with fake env", t, func() {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		triggeringJob := "project/triggering-job-v2"
		triggeredJob := "project/triggered-job-v2"

		So(e.UpdateProjectJobs(c, "project", []catalog.Definition{
			{
				JobID:           triggeringJob,
				TriggeredJobIDs: []string{triggeredJob},
				Revision:        "rev1",
				Schedule:        "triggered",
				Task:            noopTaskBytes(),
				Acls:            aclOne,
			},
			{
				JobID:    triggeredJob,
				Revision: "rev1",
				Schedule: "triggered",
				Task:     noopTaskBytes(),
				Acls:     aclOne,
			},
		}), ShouldBeNil)

		Convey("happy path", func() {
			const triggeringInvID int64 = 9200093523824911856
			const triggeredInvID int64 = 9200093521728457040

			// Force launch the triggering job.
			job, err := e.getJob(c, triggeringJob)
			So(err, ShouldBeNil)
			_, err = e.ForceInvocation(auth.WithState(c, asUserOne), job)
			So(err, ShouldBeNil)

			// Eventually it runs the task which emits a bunch of triggers, which
			// causes the triggered job triage. We stop right before it and examine
			// what we see.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				ctl.EmitTrigger(ctx, &internal.Trigger{Id: "t1"})
				So(ctl.Save(ctx), ShouldBeNil)
				ctl.EmitTrigger(ctx, &internal.Trigger{Id: "t2"})
				ctl.State().Status = task.StatusSucceeded
				return nil
			}
			tasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
				ShouldStopBefore: func(t tqtesting.Task) bool {
					_, ok := t.Payload.(*internal.TriageJobStateTask)
					return ok
				},
			})
			So(err, ShouldBeNil)

			// How these triggers are seen from outside the task.
			expectedTrigger1 := &internal.Trigger{
				Id:           "t1",
				JobId:        triggeringJob,
				InvocationId: triggeringInvID,
				Created:      google.NewTimestamp(epoch.Add(1 * time.Second)),
			}
			expectedTrigger2 := &internal.Trigger{
				Id:           "t2",
				JobId:        triggeringJob,
				InvocationId: triggeringInvID,
				Created:      google.NewTimestamp(epoch.Add(1 * time.Second)),
				OrderInBatch: 1, // second call to EmitTrigger done by the invocation
			}

			// All the tasks we've just executed.
			So(tasks.Payloads(), ShouldResemble, []proto.Message{
				// Triggering job begins execution.
				&internal.LaunchInvocationsBatchTask{
					Tasks: []*internal.LaunchInvocationTask{{JobId: triggeringJob, InvId: triggeringInvID}},
				},
				&internal.LaunchInvocationTask{
					JobId: triggeringJob, InvId: triggeringInvID,
				},

				// It emits a trigger in the middle.
				&internal.FanOutTriggersTask{
					JobIds:   []string{triggeredJob},
					Triggers: []*internal.Trigger{expectedTrigger1},
				},
				&internal.EnqueueTriggersTask{
					JobId:    triggeredJob,
					Triggers: []*internal.Trigger{expectedTrigger1},
				},

				// Triggering job finishes execution, emitting another trigger.
				&internal.InvocationFinishedTask{
					JobId: triggeringJob,
					InvId: triggeringInvID,
					Triggers: &internal.FanOutTriggersTask{
						JobIds:   []string{triggeredJob},
						Triggers: []*internal.Trigger{expectedTrigger2},
					},
				},
				&internal.EnqueueTriggersTask{
					JobId:    triggeredJob,
					Triggers: []*internal.Trigger{expectedTrigger2},
				},
			})

			// Triggering invocation has finished (with triggers recorded).
			triggeringInv, err := e.getInvocation(c, triggeringJob, triggeringInvID, true)
			So(err, ShouldBeNil)
			So(triggeringInv.Status, ShouldEqual, task.StatusSucceeded)
			outgoing, err := triggeringInv.OutgoingTriggers()
			So(err, ShouldBeNil)
			So(outgoing, ShouldResemble, []*internal.Trigger{expectedTrigger1, expectedTrigger2})

			// At this point triggered job's triage is about to start. Before it does,
			// verify emitted trigger (sitting in the pending triggers set) is
			// discoverable through ListTriggers.
			tj, _ := e.getJob(c, triggeredJob)
			triggers, err := e.ListTriggers(c, tj)
			So(err, ShouldBeNil)
			So(triggers, ShouldResemble, []*internal.Trigger{expectedTrigger1, expectedTrigger2})

			// Resume the simulation to do the triages and start the triggered
			// invocation.
			var seen []*internal.Trigger
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				seen = ctl.Request().IncomingTriggers
				ctl.State().Status = task.StatusSucceeded
				return nil
			}
			tasks, _, err = tq.RunSimulation(c, nil)
			So(err, ShouldBeNil)

			// All the tasks we've just executed.
			So(tasks.Payloads(), ShouldResemble, []proto.Message{
				// Triggered job is getting triaged (because pending triggers).
				&internal.TriageJobStateTask{
					JobId: triggeredJob,
				},
				// Triggering job is getting triaged (because it has just finished).
				&internal.TriageJobStateTask{
					JobId: triggeringJob,
				},
				// The triggered job begins execution.
				&internal.LaunchInvocationsBatchTask{
					Tasks: []*internal.LaunchInvocationTask{{JobId: triggeredJob, InvId: triggeredInvID}},
				},
				&internal.LaunchInvocationTask{
					JobId: triggeredJob, InvId: triggeredInvID,
				},
				// ...and finishes. Note that the triage doesn't launch new invocation.
				&internal.InvocationFinishedTask{
					JobId: triggeredJob, InvId: triggeredInvID,
				},
				&internal.TriageJobStateTask{JobId: triggeredJob},
			})

			// Verify LaunchTask callback saw the triggers.
			So(seen, ShouldResemble, []*internal.Trigger{expectedTrigger1, expectedTrigger2})

			// And they are recoded in IncomingTriggers set.
			triggeredInv, err := e.getInvocation(c, triggeredJob, triggeredInvID, true)
			So(err, ShouldBeNil)
			So(triggeredInv.Status, ShouldEqual, task.StatusSucceeded)
			incoming, err := triggeredInv.IncomingTriggers()
			So(err, ShouldBeNil)
			So(incoming, ShouldResemble, []*internal.Trigger{expectedTrigger1, expectedTrigger2})
		})
	})
}

func TestInvocationTimers(t *testing.T) {
	t.Parallel()

	Convey("with fake env", t, func() {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		const testJobID = "project/job-v2"
		So(e.UpdateProjectJobs(c, "project", []catalog.Definition{
			{
				JobID:    testJobID,
				Revision: "rev1",
				Schedule: "triggered",
				Task:     noopTaskBytes(),
				Acls:     aclOne,
			},
		}), ShouldBeNil)

		Convey("happy path", func() {
			const testInvID int64 = 9200093523825193008

			// Force launch the job.
			job, err := e.getJob(c, testJobID)
			So(err, ShouldBeNil)
			_, err = e.ForceInvocation(auth.WithState(c, asUserOne), job)
			So(err, ShouldBeNil)

			// See handelTimer. Name of the timer => time since epoch.
			callTimes := map[string]time.Duration{}

			// Eventually it runs the task which emits a bunch of timers and then
			// some more, and then stops.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				ctl.AddTimer(ctx, time.Minute, "1 min", []byte{1})
				ctl.AddTimer(ctx, 2*time.Minute, "2 min", []byte{2})
				ctl.State().Status = task.StatusRunning
				return nil
			}
			mgr.handleTimer = func(ctx context.Context, ctl task.Controller, name string, payload []byte) error {
				callTimes[name] = clock.Now(ctx).Sub(epoch)
				switch name {
				case "1 min": // ignore
				case "2 min":
					// Call us again later.
					ctl.AddTimer(ctx, time.Minute, "stop", []byte{3})
				case "stop":
					ctl.AddTimer(ctx, time.Minute, "ignored-timer", nil)
					ctl.State().Status = task.StatusSucceeded
				}
				return nil
			}
			tasks, _, err := tq.RunSimulation(c, nil)
			So(err, ShouldBeNil)

			timerMsg := func(idSuffix string, created, eta time.Duration, title string, payload []byte) *internal.Timer {
				return &internal.Timer{
					Id:      fmt.Sprintf("%s:%d:%s", testJobID, testInvID, idSuffix),
					Created: google.NewTimestamp(epoch.Add(created)),
					Eta:     google.NewTimestamp(epoch.Add(eta)),
					Title:   title,
					Payload: payload,
				}
			}

			// Individual timers emitted by the test. Note that 1 extra sec comes from
			// the delay added by kickLaunchInvocationsBatchTask.
			timer1 := timerMsg("1:0", time.Second, time.Second+time.Minute, "1 min", []byte{1})
			timer2 := timerMsg("1:1", time.Second, time.Second+2*time.Minute, "2 min", []byte{2})
			timer3 := timerMsg("3:0", time.Second+2*time.Minute, time.Second+3*time.Minute, "stop", []byte{3})

			// All 'handleTimer' ticks happened at expected moments in time.
			So(callTimes, ShouldResemble, map[string]time.Duration{
				"1 min": time.Second + time.Minute,
				"2 min": time.Second + 2*time.Minute,
				"stop":  time.Second + 3*time.Minute,
			})

			// All the tasks we've just executed.
			So(tasks.Payloads(), ShouldResemble, []proto.Message{
				// Triggering job begins execution.
				&internal.LaunchInvocationsBatchTask{
					Tasks: []*internal.LaunchInvocationTask{{JobId: testJobID, InvId: testInvID}},
				},
				&internal.LaunchInvocationTask{
					JobId: testJobID, InvId: testInvID,
				},

				// Request to schedule a bunch of timers.
				&internal.ScheduleTimersTask{
					JobId:  testJobID,
					InvId:  testInvID,
					Timers: []*internal.Timer{timer1, timer2},
				},

				// Actual individual timers.
				&internal.TimerTask{
					JobId: testJobID,
					InvId: testInvID,
					Timer: timer1,
				},
				&internal.TimerTask{
					JobId: testJobID,
					InvId: testInvID,
					Timer: timer2,
				},

				// One more, scheduled from handleTimer.
				&internal.TimerTask{
					JobId: testJobID,
					InvId: testInvID,
					Timer: timer3,
				},

				// End of the invocation.
				&internal.InvocationFinishedTask{
					JobId: testJobID, InvId: testInvID,
				},
				&internal.TriageJobStateTask{JobId: testJobID},
			})
		})
	})
}

func TestCron(t *testing.T) {
	t.Parallel()

	Convey("with fake env", t, func() {
		const testJobID = "project/job-v2"

		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		updateJob := func(schedule string) {
			So(e.UpdateProjectJobs(c, "project", []catalog.Definition{
				{
					JobID:    testJobID,
					Revision: "rev1",
					Schedule: schedule,
					Task:     noopTaskBytes(),
					Acls:     aclOne,
				},
			}), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()
		}

		mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
			ctl.State().Status = task.StatusSucceeded
			return nil
		}

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		Convey("relative schedule", func() {
			updateJob("with 10s interval")

			Convey("happy path", func() {
				// Let the TQ spin for two full cycles.
				tasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					Deadline: epoch.Add(30 * time.Second),
				})
				So(err, ShouldBeNil)

				// Collect the list of TQ tasks we expect to be executed.
				expect := expectedTasks{JobID: testJobID, Epoch: epoch}
				// 10 sec after the job is created, a tick comes and emits a trigger.
				expect.cronTickSequence(6278013164014963328, 3, 10*time.Second)
				// It causes an invocation.
				expect.invocationSequence(9200093511241999856)
				expect.triage()
				// 10 sec after it finishes, new tick comes (4 extra seconds are from
				// 2 sec delays induced by 2 triages).
				expect.cronTickSequence(928953616732700780, 6, 24*time.Second)
				// It causes an invocation.
				expect.invocationSequence(9200093496562633040)
				expect.triage()
				// ... and so on

				So(tasks.Payloads(), ShouldResemble, expect.Tasks)
			})

			Convey("schedule changes", func() {
				// Let the TQ spin until it hits the task execution.
				tasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					ShouldStopBefore: func(t tqtesting.Task) bool {
						_, ok := t.Payload.(*internal.LaunchInvocationTask)
						return ok
					},
				})
				So(err, ShouldBeNil)

				// At this point the job's schedule changes.
				updateJob("with 30s interval")

				// We let the TQ spin some more.
				moreTasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					Deadline: epoch.Add(50 * time.Second),
				})
				So(err, ShouldBeNil)

				// Here's what we expect to execute.
				expect := expectedTasks{JobID: testJobID, Epoch: epoch}
				// 10 sec after the job is created, a tick comes and emits a trigger.
				expect.cronTickSequence(6278013164014963328, 3, 10*time.Second)
				// It causes an invocation. We changed the schedule to 30s after that.
				expect.invocationSequence(9200093511241999856)
				expect.triage()
				// 30 sec after it finishes, new tick comes (4 extra seconds are from
				// 2 sec delays induced by 2 triages).
				expect.cronTickSequence(928953616732700780, 6, 44*time.Second)
				// It causes an invocation.
				expect.invocationSequence(9200093475591113040)
				expect.triage()

				// Got it?
				So(append(tasks, moreTasks...).Payloads(), ShouldResemble, expect.Tasks)
			})

			Convey("pause/unpause", func() {
				// Let the TQ spin until it hits the end of task execution.
				tasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					ShouldStopAfter: func(t tqtesting.Task) bool {
						_, ok := t.Payload.(*internal.InvocationFinishedTask)
						return ok
					},
				})
				So(err, ShouldBeNil)

				// At this point we pause the job.
				j, err := e.getJob(c, testJobID)
				So(err, ShouldBeNil)
				So(e.setJobPausedFlag(c, j, true, ""), ShouldBeNil)

				// We let the TQ spin some more.
				moreTasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					Deadline: epoch.Add(time.Hour),
				})
				So(err, ShouldBeNil)

				// Here's what we expect to execute.
				expect := expectedTasks{JobID: testJobID, Epoch: epoch}
				// 10 sec after the job is created, a tick comes and emits a trigger.
				expect.cronTickSequence(6278013164014963328, 3, 10*time.Second)
				// It causes an invocation. We pause the job after that.
				expect.invocationSequence(9200093511241999856)
				// The pause kicks the triage, which later executes.
				expect.kickTriage()
				expect.triage()
				// and nothing else happens ...

				// Got it?
				So(append(tasks, moreTasks...).Payloads(), ShouldResemble, expect.Tasks)

				// Some time later we unpause the job, it starts again immediately.
				clock.Get(c).(testclock.TestClock).Set(epoch.Add(time.Hour))
				So(e.setJobPausedFlag(c, j, false, ""), ShouldBeNil)

				tasks, _, err = tq.RunSimulation(c, &tqtesting.SimulationParams{
					Deadline: epoch.Add(time.Hour + 10*time.Second),
				})
				So(err, ShouldBeNil)

				// Did it?
				expect.clear()
				expect.cronTickSequence(325298467681248558, 7, time.Hour)
				expect.invocationSequence(9200089746854060480)
				expect.triage()
				So(tasks.Payloads(), ShouldResemble, expect.Tasks)
			})

			Convey("disabling", func() {
				// Let the TQ spin until it hits the end of task execution.
				tasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					ShouldStopAfter: func(t tqtesting.Task) bool {
						_, ok := t.Payload.(*internal.InvocationFinishedTask)
						return ok
					},
				})
				So(err, ShouldBeNil)

				// At this point we disable the job.
				So(e.UpdateProjectJobs(c, "project", nil), ShouldBeNil)

				// We let the TQ spin some more...
				moreTasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					Deadline: epoch.Add(time.Hour),
				})
				So(err, ShouldBeNil)

				// Here's what we expect to execute.
				expect := expectedTasks{JobID: testJobID, Epoch: epoch}
				// 10 sec after the job is created, a tick comes and emits a trigger.
				expect.cronTickSequence(6278013164014963328, 3, 10*time.Second)
				// It causes an invocation. We disable the job after that.
				expect.invocationSequence(9200093511241999856)
				// Disabling kicks the triage, which later executes.
				expect.kickTriage()
				expect.triage()
				// and nothing else happens ...

				// Got it?
				So(append(tasks, moreTasks...).Payloads(), ShouldResemble, expect.Tasks)
			})
		})

		Convey("absolute schedule", func() {
			updateJob("5,10 * * * * * *") // on 5th and 10th sec

			Convey("happy path", func() {
				// Let the TQ spin for two full cycles.
				tasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					Deadline: epoch.Add(14 * time.Second),
				})
				So(err, ShouldBeNil)

				// Collect the list of TQ tasks we expect to be executed.
				expect := expectedTasks{JobID: testJobID, Epoch: epoch}
				// 5 sec after the job is created, a tick comes and emits a trigger.
				expect.cronTickSequence(6278013164014963328, 3, 5*time.Second)
				// It causes an invocation.
				expect.invocationSequence(9200093517533908688)
				expect.triage()
				// Next tick comes right on schedule, 10 sec after the start.
				expect.cronTickSequence(2673062197574995716, 6, 10*time.Second)
				// It causes an invocation.
				expect.invocationSequence(9200093511241900480)
				expect.triage()
				// ... and so on

				So(tasks.Payloads(), ShouldResemble, expect.Tasks)
			})

			Convey("overrun", func() {
				// Currently cron just keeps submitting triggers that ends up in the
				// pending triggers queue if there's some invocation currently running.
				// Once the invocation finishes, the next one start right away (just
				// like with any other kind of trigger). Overruns are not recorded.

				// Simulate "long" task with timers.
				mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
					ctl.AddTimer(ctx, 5*time.Second, "5 sec", nil)
					ctl.State().Status = task.StatusRunning
					return nil
				}
				mgr.handleTimer = func(ctx context.Context, ctl task.Controller, name string, payload []byte) error {
					ctl.State().Status = task.StatusSucceeded
					return nil
				}

				// Let the TQ spin for two full cycles.
				tasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					Deadline: epoch.Add(15 * time.Second),
				})
				So(err, ShouldBeNil)

				// Collect the list of TQ tasks we expect to be executed.
				expect := expectedTasks{JobID: testJobID, Epoch: epoch}
				// 5 sec after the job is created, a tick comes and emits a trigger.
				expect.cronTickSequence(6278013164014963328, 3, 5*time.Second)
				// It causes an invocation to start (and keep running).
				expect.invocationStart(9200093517533908688)
				// The next tick comes right on the schedule, but it doesn't start an
				// invocation yet, just submits a trigger.
				expect.cronTickSequence(2673062197574995716, 6, 10*time.Second)
				// A scheduled invocation timer arrives and finishes the invocation.
				expect.invocationTimer(9200093517533908688, 1, "5 sec", 7*time.Second, 12*time.Second)
				expect.invocationEnd(9200093517533908688)
				expect.triage()
				// The triage detects pending cron trigger and launches a new invocation
				// right away.
				expect.invocationStart(9200093509144748480)

				So(tasks.Payloads(), ShouldResemble, expect.Tasks)
			})
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

type expectedTasks struct {
	JobID string
	Epoch time.Time
	Tasks []proto.Message
}

func (e *expectedTasks) clear() {
	e.Tasks = nil
}

func (e *expectedTasks) triage() {
	e.Tasks = append(e.Tasks, &internal.TriageJobStateTask{JobId: e.JobID})
}

func (e *expectedTasks) kickTriage() {
	e.Tasks = append(e.Tasks, &internal.KickTriageTask{JobId: e.JobID})
}

func (e *expectedTasks) cronTickSequence(nonce, gen int64, when time.Duration) {
	e.Tasks = append(e.Tasks,
		&internal.CronTickTask{JobId: e.JobID, TickNonce: nonce},
		&internal.EnqueueTriggersTask{
			JobId: e.JobID,
			Triggers: []*internal.Trigger{
				{
					Id:      fmt.Sprintf("cron:v1:%d", gen),
					Created: google.NewTimestamp(e.Epoch.Add(when)),
					Payload: &internal.Trigger_Cron{
						Cron: &api.CronTrigger{Generation: gen},
					},
				},
			},
		},
	)
	e.triage()
}

func (e *expectedTasks) invocationSequence(invID int64) {
	e.invocationStart(invID)
	e.invocationEnd(invID)
}

func (e *expectedTasks) invocationStart(invID int64) {
	e.Tasks = append(e.Tasks,
		&internal.LaunchInvocationsBatchTask{
			Tasks: []*internal.LaunchInvocationTask{
				{JobId: e.JobID, InvId: invID},
			},
		},
		&internal.LaunchInvocationTask{JobId: e.JobID, InvId: invID},
	)
}

func (e *expectedTasks) invocationEnd(invID int64) {
	e.Tasks = append(e.Tasks, &internal.InvocationFinishedTask{JobId: e.JobID, InvId: invID})
}

func (e *expectedTasks) invocationTimer(invID, seq int64, title string, created, eta time.Duration) {
	e.Tasks = append(e.Tasks, &internal.TimerTask{
		JobId: e.JobID,
		InvId: invID,
		Timer: &internal.Timer{
			Id:      fmt.Sprintf("%s:%d:%d:0", e.JobID, invID, seq),
			Title:   title,
			Created: google.NewTimestamp(e.Epoch.Add(created)),
			Eta:     google.NewTimestamp(e.Epoch.Add(eta)),
		},
	})
}
