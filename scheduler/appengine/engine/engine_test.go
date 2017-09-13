// Copyright 2015 The LUCI Authors.
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
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/api/pubsub/v1"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	tq "go.chromium.org/gae/service/taskqueue"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets/testsecrets"

	"go.chromium.org/luci/scheduler/appengine/acl"
	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/noop"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetAllProjects(t *testing.T) {
	Convey("works", t, func() {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

		// Empty.
		projects, err := e.GetAllProjects(c)
		So(err, ShouldBeNil)
		So(len(projects), ShouldEqual, 0)

		// Non empty.
		So(ds.Put(c,
			&Job{JobID: "abc/1", ProjectID: "abc", Enabled: true},
			&Job{JobID: "abc/2", ProjectID: "abc", Enabled: true},
			&Job{JobID: "def/1", ProjectID: "def", Enabled: true},
			&Job{JobID: "xyz/1", ProjectID: "xyz", Enabled: false},
		), ShouldBeNil)
		ds.GetTestable(c).CatchupIndexes()
		projects, err = e.GetAllProjects(c)
		So(err, ShouldBeNil)
		So(projects, ShouldResemble, []string{"abc", "def"})
	})
}

func TestUpdateProjectJobs(t *testing.T) {
	Convey("works", t, func() {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

		// Doing nothing.
		So(e.UpdateProjectJobs(c, "abc", []catalog.Definition{}), ShouldBeNil)
		So(allJobs(c), ShouldResemble, []Job{})

		// Adding a new job (ticks every 5 sec).
		So(e.UpdateProjectJobs(c, "abc", []catalog.Definition{
			{
				JobID:    "abc/1",
				Revision: "rev1",
				Schedule: "*/5 * * * * * *",
				Acls:     acl.GrantsByRole{Readers: []string{"group:r"}, Owners: []string{"groups:o"}},
			}}), ShouldBeNil)
		So(allJobs(c), ShouldResemble, []Job{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev1",
				Enabled:   true,
				Acls:      acl.GrantsByRole{Readers: []string{"group:r"}, Owners: []string{"groups:o"}},
				Schedule:  "*/5 * * * * * *",
				State: JobState{
					State:     "SCHEDULED",
					TickNonce: 6278013164014963328,
					TickTime:  epoch.Add(5 * time.Second),
				},
			},
		})

		// Enqueued timer task to launch it.
		task := ensureOneTask(c, "timers-q")
		So(task.Path, ShouldEqual, "/timers")
		So(task.ETA, ShouldResemble, epoch.Add(5*time.Second))
		tq.GetTestable(c).ResetTasks()

		// Readding same job in with exact same config revision -> noop.
		So(e.UpdateProjectJobs(c, "abc", []catalog.Definition{
			{
				JobID:    "abc/1",
				Revision: "rev1",
				Schedule: "*/5 * * * * * *",
			}}), ShouldBeNil)
		ensureZeroTasks(c, "timers-q")
		ensureZeroTasks(c, "invs-q")

		// Changing schedule to tick earlier -> rescheduled.
		So(e.UpdateProjectJobs(c, "abc", []catalog.Definition{
			{
				JobID:    "abc/1",
				Revision: "rev2",
				Schedule: "*/1 * * * * * *",
			}}), ShouldBeNil)
		So(allJobs(c), ShouldResemble, []Job{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev2",
				Enabled:   true,
				Schedule:  "*/1 * * * * * *",
				State: JobState{
					State:     "SCHEDULED",
					TickNonce: 886585524575582446,
					TickTime:  epoch.Add(1 * time.Second),
				},
			},
		})
		// Enqueued timer task to launch it.
		task = ensureOneTask(c, "timers-q")
		So(task.Path, ShouldEqual, "/timers")
		So(task.ETA, ShouldResemble, epoch.Add(1*time.Second))
		tq.GetTestable(c).ResetTasks()

		// Removed -> goes to disabled state.
		So(e.UpdateProjectJobs(c, "abc", []catalog.Definition{}), ShouldBeNil)
		So(allJobs(c), ShouldResemble, []Job{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev2",
				Enabled:   false,
				Schedule:  "*/1 * * * * * *",
				State: JobState{
					State: "DISABLED",
				},
			},
		})
		ensureZeroTasks(c, "timers-q")
		ensureZeroTasks(c, "invs-q")
	})
}

func TestTransactionRetries(t *testing.T) {
	Convey("retry works", t, func() {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

		// Adding a new job with transaction retry, should enqueue one task.
		ds.GetTestable(c).SetTransactionRetryCount(2)
		So(e.UpdateProjectJobs(c, "abc", []catalog.Definition{
			{
				JobID:    "abc/1",
				Revision: "rev1",
				Schedule: "*/5 * * * * * *",
			}}), ShouldBeNil)
		So(allJobs(c), ShouldResemble, []Job{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev1",
				Enabled:   true,
				Schedule:  "*/5 * * * * * *",
				State: JobState{
					State:     "SCHEDULED",
					TickNonce: 928953616732700780,
					TickTime:  epoch.Add(5 * time.Second),
				},
			},
		})
		// Enqueued timer task to launch it.
		task := ensureOneTask(c, "timers-q")
		So(task.Path, ShouldEqual, "/timers")
		So(task.ETA, ShouldResemble, epoch.Add(5*time.Second))
		tq.GetTestable(c).ResetTasks()
	})

	Convey("collision is handled", t, func() {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

		// Pretend collision happened in all retries.
		ds.GetTestable(c).SetTransactionRetryCount(15)
		err := e.UpdateProjectJobs(c, "abc", []catalog.Definition{
			{
				JobID:    "abc/1",
				Revision: "rev1",
				Schedule: "*/5 * * * * * *",
			}})
		So(transient.Tag.In(err), ShouldBeTrue)
		So(allJobs(c), ShouldResemble, []Job{})
		ensureZeroTasks(c, "timers-q")
		ensureZeroTasks(c, "invs-q")
	})
}

func TestResetAllJobsOnDevServer(t *testing.T) {
	Convey("works", t, func() {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

		So(e.UpdateProjectJobs(c, "abc", []catalog.Definition{
			{
				JobID:    "abc/1",
				Revision: "rev1",
				Schedule: "*/5 * * * * * *",
			}}), ShouldBeNil)
		So(allJobs(c), ShouldResemble, []Job{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev1",
				Enabled:   true,
				Schedule:  "*/5 * * * * * *",
				State: JobState{
					State:     "SCHEDULED",
					TickNonce: 6278013164014963328,
					TickTime:  epoch.Add(5 * time.Second),
				},
			},
		})

		clock.Get(c).(testclock.TestClock).Add(1 * time.Minute)

		// ResetAllJobsOnDevServer should reschedule the job.
		So(e.ResetAllJobsOnDevServer(c), ShouldBeNil)
		So(allJobs(c), ShouldResemble, []Job{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev1",
				Enabled:   true,
				Schedule:  "*/5 * * * * * *",
				State: JobState{
					State:     "SCHEDULED",
					TickNonce: 886585524575582446,
					TickTime:  epoch.Add(65 * time.Second),
				},
			},
		})
	})
}

func TestFullFlow(t *testing.T) {
	Convey("full flow", t, func() {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()
		taskBytes := noopTaskBytes()

		expectedJobs := func(state JobState) []Job {
			return []Job{
				{
					JobID:     "abc/1",
					ProjectID: "abc",
					Revision:  "rev1",
					Enabled:   true,
					Schedule:  "*/5 * * * * * *",
					Task:      taskBytes,
					State:     state,
				},
			}
		}

		// Adding a new job (ticks every 5 sec).
		So(e.UpdateProjectJobs(c, "abc", []catalog.Definition{
			{
				JobID:    "abc/1",
				Revision: "rev1",
				Schedule: "*/5 * * * * * *",
				Task:     taskBytes,
			}}), ShouldBeNil)
		So(allJobs(c), ShouldResemble, expectedJobs(JobState{
			State:     "SCHEDULED",
			TickNonce: 6278013164014963328,
			TickTime:  epoch.Add(5 * time.Second),
		}))

		// Enqueued timer task to launch it.
		tsk := ensureOneTask(c, "timers-q")
		So(tsk.Path, ShouldEqual, "/timers")
		So(tsk.ETA, ShouldResemble, epoch.Add(5*time.Second))
		tq.GetTestable(c).ResetTasks()

		// Tick time comes, the tick task is executed, job is added to queue.
		clock.Get(c).(testclock.TestClock).Add(5 * time.Second)
		So(e.ExecuteSerializedAction(c, tsk.Payload, 0), ShouldBeNil)

		// Job is in queued state now.
		So(allJobs(c), ShouldResemble, expectedJobs(JobState{
			State:           "QUEUED",
			TickNonce:       886585524575582446,
			TickTime:        epoch.Add(10 * time.Second),
			InvocationNonce: 928953616732700780,
			InvocationTime:  epoch.Add(5 * time.Second),
		}))

		// Next tick task is added.
		tickTask := ensureOneTask(c, "timers-q")
		So(tickTask.Path, ShouldEqual, "/timers")
		So(tickTask.ETA, ShouldResemble, epoch.Add(10*time.Second))

		// Invocation task (ETA is 1 sec in the future).
		invTask := ensureOneTask(c, "invs-q")
		So(invTask.Path, ShouldEqual, "/invs")
		So(invTask.ETA, ShouldResemble, epoch.Add(6*time.Second))
		tq.GetTestable(c).ResetTasks()

		// Time to run the job and it fails to launch with a transient error.
		mgr.launchTask = func(ctx context.Context, ctl task.Controller, triggers []task.Trigger) error {
			// Check data provided via the controller.
			So(ctl.JobID(), ShouldEqual, "abc/1")
			So(ctl.InvocationID(), ShouldEqual, int64(9200093518582198800))
			So(ctl.InvocationNonce(), ShouldEqual, int64(928953616732700780))
			So(ctl.Task(), ShouldResemble, &messages.NoopTask{})

			ctl.DebugLog("oops, fail")
			return errors.New("oops", transient.Tag)
		}
		So(transient.Tag.In(e.ExecuteSerializedAction(c, invTask.Payload, 0)), ShouldBeTrue)

		// Still in QUEUED state, but with InvocatioID assigned.
		jobs := allJobs(c)
		So(jobs, ShouldResemble, expectedJobs(JobState{
			State:           "QUEUED",
			TickNonce:       886585524575582446,
			TickTime:        epoch.Add(10 * time.Second),
			InvocationNonce: 928953616732700780,
			InvocationTime:  epoch.Add(5 * time.Second),
			InvocationID:    9200093518582198800,
		}))
		jobKey := ds.KeyForObj(c, &jobs[0])

		// Check Invocation fields. It indicates that the attempt has failed and
		// will be retried.
		inv := Invocation{ID: 9200093518582198800, JobKey: jobKey}
		So(ds.Get(c, &inv), ShouldBeNil)
		inv.JobKey = nil // for easier ShouldResemble below
		debugLog := inv.DebugLog
		inv.DebugLog = ""
		So(inv, ShouldResemble, Invocation{
			ID:              9200093518582198800,
			InvocationNonce: 928953616732700780,
			Revision:        "rev1",
			Started:         epoch.Add(5 * time.Second),
			Task:            taskBytes,
			DebugLog:        "",
			Status:          task.StatusRetrying,
			MutationsCount:  1,
		})
		So(debugLog, ShouldContainSubstring, "[22:42:05.000] Invocation initiated (attempt 1)")
		So(debugLog, ShouldContainSubstring, "[22:42:05.000] oops, fail")
		So(debugLog, ShouldContainSubstring, "[22:42:05.000] The invocation will be retried")

		// The job is still in QUEUED state.
		So(allJobs(c), ShouldResemble, expectedJobs(JobState{
			State:           "QUEUED",
			TickNonce:       886585524575582446,
			TickTime:        epoch.Add(10 * time.Second),
			InvocationNonce: 928953616732700780,
			InvocationTime:  epoch.Add(5 * time.Second),
			InvocationID:    9200093518582198800,
		}))

		// Second attempt. Now starts, hangs midway, they finishes.
		mgr.launchTask = func(ctx context.Context, ctl task.Controller, triggers []task.Trigger) error {
			// Make sure Save() checkpoints the progress.
			ctl.DebugLog("Starting")
			ctl.State().Status = task.StatusRunning
			So(ctl.Save(ctx), ShouldBeNil)

			// After first Save the job and the invocation are in running state.
			So(allJobs(c), ShouldResemble, expectedJobs(JobState{
				State:                "RUNNING",
				TickNonce:            886585524575582446,
				TickTime:             epoch.Add(10 * time.Second),
				InvocationNonce:      928953616732700780,
				InvocationRetryCount: 1,
				InvocationTime:       epoch.Add(5 * time.Second),
				InvocationID:         9200093518582296192,
			}))
			inv := Invocation{ID: 9200093518582296192, JobKey: jobKey}
			So(ds.Get(c, &inv), ShouldBeNil)
			inv.JobKey = nil // for easier ShouldResemble below
			So(inv, ShouldResemble, Invocation{
				ID:              9200093518582296192,
				InvocationNonce: 928953616732700780,
				Revision:        "rev1",
				Started:         epoch.Add(5 * time.Second),
				Task:            taskBytes,
				DebugLog:        "[22:42:05.000] Invocation initiated (attempt 2)\n[22:42:05.000] Starting\n",
				RetryCount:      1,
				Status:          task.StatusRunning,
				MutationsCount:  1,
			})

			// Noop save, just for the code coverage.
			So(ctl.Save(ctx), ShouldBeNil)

			// Change state to the final one.
			ctl.State().Status = task.StatusSucceeded
			ctl.State().ViewURL = "http://view_url"
			ctl.State().TaskData = []byte("blah")
			return nil
		}
		So(e.ExecuteSerializedAction(c, invTask.Payload, 1), ShouldBeNil)

		// After final save.
		inv = Invocation{ID: 9200093518582296192, JobKey: jobKey}
		So(ds.Get(c, &inv), ShouldBeNil)
		inv.JobKey = nil // for easier ShouldResemble below
		debugLog = inv.DebugLog
		inv.DebugLog = ""
		So(inv, ShouldResemble, Invocation{
			ID:              9200093518582296192,
			InvocationNonce: 928953616732700780,
			Revision:        "rev1",
			Started:         epoch.Add(5 * time.Second),
			Finished:        epoch.Add(5 * time.Second),
			Task:            taskBytes,
			DebugLog:        "",
			RetryCount:      1,
			Status:          task.StatusSucceeded,
			ViewURL:         "http://view_url",
			TaskData:        []byte("blah"),
			MutationsCount:  2,
		})
		So(debugLog, ShouldContainSubstring, "[22:42:05.000] Invocation initiated (attempt 2)")
		So(debugLog, ShouldContainSubstring, "[22:42:05.000] Starting")
		So(debugLog, ShouldContainSubstring, "with status SUCCEEDED")

		// Previous invocation is aborted now (in Failed state).
		inv = Invocation{ID: 9200093518582198800, JobKey: jobKey}
		So(ds.Get(c, &inv), ShouldBeNil)
		inv.JobKey = nil // for easier ShouldResemble below
		debugLog = inv.DebugLog
		inv.DebugLog = ""
		So(inv, ShouldResemble, Invocation{
			ID:              9200093518582198800,
			InvocationNonce: 928953616732700780,
			Revision:        "rev1",
			Started:         epoch.Add(5 * time.Second),
			Finished:        epoch.Add(5 * time.Second),
			Task:            taskBytes,
			DebugLog:        "",
			Status:          task.StatusFailed,
			MutationsCount:  2,
		})
		So(debugLog, ShouldContainSubstring,
			"[22:42:05.000] New invocation is starting (9200093518582296192), marking this one as failed")

		// Job is in scheduled state again.
		So(allJobs(c), ShouldResemble, expectedJobs(JobState{
			State:     "SCHEDULED",
			TickNonce: 886585524575582446,
			TickTime:  epoch.Add(10 * time.Second),
			PrevTime:  epoch.Add(5 * time.Second),
		}))
	})
}

func TestFullTriggeredFlow(t *testing.T) {
	Convey("full triggered flow", t, func() {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()
		taskBytes := noopTaskBytes()

		// Create a new triggering noop job (ticks every 5 sec).
		jobsDefinitions := []catalog.Definition{
			{
				JobID:           "abc/1",
				Revision:        "rev1",
				Schedule:        "*/5 * * * * * *",
				Task:            taskBytes,
				Flavor:          catalog.JobFlavorTrigger,
				TriggeredJobIDs: []string{"abc/2-triggered", "abc/3-triggered"},
			},
		}
		// And also jobs 2, 3 to be triggered by job 1.
		for i := 2; i <= 3; i++ {
			jobsDefinitions = append(jobsDefinitions, catalog.Definition{
				JobID:    fmt.Sprintf("abc/%d-triggered", i),
				Revision: "rev1",
				Schedule: "triggered",
				Task:     taskBytes,
				Flavor:   catalog.JobFlavorTriggered,
			})
		}
		So(e.UpdateProjectJobs(c, "abc", jobsDefinitions), ShouldBeNil)
		// Enqueued timer task to launch it.
		tsk := ensureOneTask(c, "timers-q")
		So(tsk.Path, ShouldEqual, "/timers")
		So(tsk.ETA, ShouldResemble, epoch.Add(5*time.Second))
		tq.GetTestable(c).ResetTasks()

		// Tick time comes, the tick task is executed, job is added to queue.
		clock.Get(c).(testclock.TestClock).Add(5 * time.Second)
		So(e.ExecuteSerializedAction(c, tsk.Payload, 0), ShouldBeNil)

		// Job1 is in queued state now.
		job1 := getJob(c, "abc/1")
		So(job1.Flavor, ShouldEqual, catalog.JobFlavorTrigger)
		So(job1.TriggeredJobIDs, ShouldResemble, []string{"abc/2-triggered", "abc/3-triggered"})
		So(job1.State.State, ShouldEqual, JobStateQueued)

		// Next tick task is added.
		tickTask := ensureOneTask(c, "timers-q")
		So(tickTask.Path, ShouldEqual, "/timers")
		So(tickTask.ETA, ShouldResemble, epoch.Add(10*time.Second))

		// Invocation task (ETA is 1 sec in the future).
		invTask := ensureOneTask(c, "invs-q")
		So(invTask.Path, ShouldEqual, "/invs")
		So(invTask.ETA, ShouldResemble, epoch.Add(6*time.Second))
		tq.GetTestable(c).ResetTasks()

		var invID int64 // set inside launchTask once invocation is known.

		mgr.launchTask = func(ctx context.Context, ctl task.Controller, _ []task.Trigger) error {
			// Make sure Save() checkpoints the progress.
			ctl.DebugLog("Starting")
			ctl.State().Status = task.StatusRunning
			So(ctl.Save(ctx), ShouldBeNil)

			// After first Save the job and the invocation are in running state.
			j1 := getJob(c, "abc/1")
			So(j1.State.State, ShouldEqual, JobStateRunning)
			invID = j1.State.InvocationID
			inv, err := e.getInvocation(c, "abc/1", invID)
			So(err, ShouldBeNil)
			So(inv.TriggeredJobIDs, ShouldResemble, []string{"abc/2-triggered", "abc/3-triggered"})
			So(inv.DebugLog, ShouldEqual, "[22:42:05.000] Invocation initiated (attempt 1)\n[22:42:05.000] Starting\n")
			So(inv.Status, ShouldEqual, task.StatusRunning)
			So(inv.MutationsCount, ShouldEqual, 1)

			ctl.EmitTrigger(ctx, task.Trigger{ID: "trg", Payload: []byte("note the trigger id")})
			ctl.EmitTrigger(ctx, task.Trigger{ID: "trg", Payload: []byte("different payload")})

			// Change state to the final one.
			ctl.State().Status = task.StatusSucceeded
			ctl.State().ViewURL = "http://view_url"
			ctl.State().TaskData = []byte("blah")
			return nil
		}
		So(e.ExecuteSerializedAction(c, invTask.Payload, 0), ShouldBeNil)

		// After final save.
		inv, err := e.getInvocation(c, "abc/1", invID)
		So(err, ShouldBeNil)
		So(inv.Status, ShouldEqual, task.StatusSucceeded)
		So(inv.MutationsCount, ShouldEqual, 2)
		So(inv.DebugLog, ShouldContainSubstring, "[22:42:05.000] Emitting a trigger trg") // twice.

		for _, triggerTask := range popAllTasks(c, "invs-q") {
			So(e.ExecuteSerializedAction(c, triggerTask.Payload, 0), ShouldBeNil)
		}
		// Triggers should result in new invocations for previously suspended jobs.
		So(getJob(c, "abc/2-triggered").State.State, ShouldEqual, JobStateQueued)
		So(getJob(c, "abc/3-triggered").State.State, ShouldEqual, JobStateQueued)

		// Prepare to track triggers passed to task launchers.
		deliveredTriggers := map[string][]string{}
		mgr.launchTask = func(ctx context.Context, ctl task.Controller, triggers []task.Trigger) error {
			So(deliveredTriggers, ShouldNotContainKey, ctl.JobID())
			ids := make([]string, 0, len(triggers))
			for _, t := range triggers {
				ids = append(ids, t.ID)
			}
			sort.Strings(ids) // For deterministic tests.
			deliveredTriggers[ctl.JobID()] = ids
			ctl.State().Status = task.StatusSucceeded
			return nil
		}
		// Actually execute task launching.
		for _, t := range popAllTasks(c, "invs-q") {
			So(e.ExecuteSerializedAction(c, t.Payload, 0), ShouldBeNil)
		}
		So(deliveredTriggers, ShouldResemble, map[string][]string{
			"abc/2-triggered": {"trg"}, "abc/3-triggered": {"trg"},
		})
	})
}
func TestGenerateInvocationID(t *testing.T) {
	Convey("generateInvocationID does not collide", t, func() {
		c := newTestContext(epoch)
		k := ds.NewKey(c, "Job", "", 123, nil)

		// Bunch of ids generated at the exact same moment in time do not collide.
		ids := map[int64]struct{}{}
		for i := 0; i < 20; i++ {
			id, err := generateInvocationID(c, k)
			So(err, ShouldBeNil)
			ids[id] = struct{}{}
		}
		So(len(ids), ShouldEqual, 20)
	})

	Convey("generateInvocationID gen IDs with most recent first", t, func() {
		c := newTestContext(epoch)
		k := ds.NewKey(c, "Job", "", 123, nil)

		older, err := generateInvocationID(c, k)
		So(err, ShouldBeNil)

		clock.Get(c).(testclock.TestClock).Add(5 * time.Second)

		newer, err := generateInvocationID(c, k)
		So(err, ShouldBeNil)

		So(newer, ShouldBeLessThan, older)
	})
}

func TestQueries(t *testing.T) {
	Convey("with mock data", t, func() {
		c := newTestContext(epoch)
		e, _ := newTestEngine()
		aclPublic := acl.GrantsByRole{Readers: []string{"group:all"}, Owners: []string{"group:administrators"}}
		aclSome := acl.GrantsByRole{Readers: []string{"group:some"}}
		aclOne := acl.GrantsByRole{Owners: []string{"one@example.com"}}
		aclAdmin := acl.GrantsByRole{Readers: []string{"group:administrators"}, Owners: []string{"group:administrators"}}

		ctxAnon := auth.WithState(c, &authtest.FakeState{
			Identity:       "anonymous:anonymous",
			IdentityGroups: []string{"all"},
		})
		ctxOne := auth.WithState(c, &authtest.FakeState{
			Identity:       "user:one@example.com",
			IdentityGroups: []string{"all"},
		})
		ctxSome := auth.WithState(c, &authtest.FakeState{
			Identity:       "user:some@example.com",
			IdentityGroups: []string{"all", "some"},
		})
		ctxAdmin := auth.WithState(c, &authtest.FakeState{
			Identity:       "user:admin@example.com",
			IdentityGroups: []string{"administrators", "all"},
		})

		So(ds.Put(c,
			&Job{JobID: "abc/1", ProjectID: "abc", Enabled: true, Acls: aclOne},
			&Job{JobID: "abc/2", ProjectID: "abc", Enabled: true, Acls: aclSome},
			&Job{JobID: "abc/3", ProjectID: "abc", Enabled: true, Acls: aclPublic},
			&Job{JobID: "def/1", ProjectID: "def", Enabled: true, Acls: aclPublic},
			&Job{JobID: "def/2", ProjectID: "def", Enabled: false, Acls: aclPublic},
			&Job{JobID: "secret/1", ProjectID: "secret", Enabled: true, Acls: aclAdmin},
		), ShouldBeNil)

		job1 := ds.NewKey(c, "Job", "abc/1", 0, nil)
		job2 := ds.NewKey(c, "Job", "abc/2", 0, nil)
		job3 := ds.NewKey(c, "Job", "abc/3", 0, nil)
		So(ds.Put(c,
			&Invocation{ID: 1, JobKey: job1, InvocationNonce: 123},
			&Invocation{ID: 2, JobKey: job1, InvocationNonce: 123},
			&Invocation{ID: 3, JobKey: job1},
			&Invocation{ID: 1, JobKey: job2},
			&Invocation{ID: 2, JobKey: job2},
			&Invocation{ID: 3, JobKey: job2},
			&Invocation{ID: 1, JobKey: job3},
		), ShouldBeNil)

		ds.GetTestable(c).CatchupIndexes()

		Convey("GetAllProjects ignores ACLs and CurrentIdentity", func() {
			test := func(ctx context.Context) {
				r, err := e.GetAllProjects(c)
				So(err, ShouldBeNil)
				So(r, ShouldResemble, []string{"abc", "def", "secret"})
			}
			test(c)
			test(ctxAnon)
			test(ctxAdmin)
		})

		Convey("GetVisibleJobs works", func() {
			get := func(ctx context.Context) []string {
				jobs, err := e.GetVisibleJobs(ctx)
				So(err, ShouldBeNil)
				return sortedJobIds(jobs)
			}

			Convey("Anonymous users see only public jobs", func() {
				// Only 3 jobs with default ACLs granting READER access to everyone, but
				// def/2 is disabled and so shouldn't be returned.
				So(get(ctxAnon), ShouldResemble, []string{"abc/3", "def/1"})
			})
			Convey("Owners can see their own jobs + public jobs", func() {
				// abc/1 is owned by one@example.com.
				So(get(ctxOne), ShouldResemble, []string{"abc/1", "abc/3", "def/1"})
			})
			Convey("Explicit readers", func() {
				So(get(ctxSome), ShouldResemble, []string{"abc/2", "abc/3", "def/1"})
			})
			Convey("Admins have implicit READER access to all jobs", func() {
				So(get(ctxAdmin), ShouldResemble, []string{"abc/1", "abc/2", "abc/3", "def/1", "secret/1"})
			})
		})

		Convey("GetProjectJobsRA works", func() {
			get := func(ctx context.Context, project string) []string {
				jobs, err := e.GetVisibleProjectJobs(ctx, project)
				So(err, ShouldBeNil)
				return sortedJobIds(jobs)
			}
			Convey("Anonymous can still see public jobs", func() {
				So(get(ctxAnon, "def"), ShouldResemble, []string{"def/1"})
			})
			Convey("Admin have implicit READER access to all jobs", func() {
				So(get(ctxAdmin, "abc"), ShouldResemble, []string{"abc/1", "abc/2", "abc/3"})
			})
			Convey("Owners can still see their jobs", func() {
				So(get(ctxOne, "abc"), ShouldResemble, []string{"abc/1", "abc/3"})
			})
			Convey("Readers can see their jobs", func() {
				So(get(ctxSome, "abc"), ShouldResemble, []string{"abc/2", "abc/3"})
			})
		})

		Convey("GetVisibleJob works", func() {
			_, err := e.GetVisibleJob(ctxAdmin, "missing/job")
			So(err, ShouldEqual, ErrNoSuchJob)

			_, err = e.GetVisibleJob(ctxAnon, "abc/1") // no READER permission.
			So(err, ShouldEqual, ErrNoSuchJob)

			_, err = e.GetVisibleJob(ctxAnon, "def/2") // not enabled, hence not visible.
			So(err, ShouldEqual, ErrNoSuchJob)

			job, err := e.GetVisibleJob(ctxAnon, "def/1") // OK.
			So(job, ShouldNotBeNil)
			So(err, ShouldBeNil)
		})

		Convey("ListVisibleInvocations works", func() {
			Convey("Anonymous can't see non-public job invocations", func() {
				_, _, err := e.ListVisibleInvocations(ctxAnon, "abc/1", 2, "")
				So(err, ShouldResemble, ErrNoSuchJob)
			})

			Convey("With paging", func() {
				invs, cursor, err := e.ListVisibleInvocations(ctxOne, "abc/1", 2, "")
				So(err, ShouldBeNil)
				So(len(invs), ShouldEqual, 2)
				So(invs[0].ID, ShouldEqual, 1)
				So(invs[1].ID, ShouldEqual, 2)
				So(cursor, ShouldNotEqual, "")

				invs, cursor, err = e.ListVisibleInvocations(ctxOne, "abc/1", 2, cursor)
				So(err, ShouldBeNil)
				So(len(invs), ShouldEqual, 1)
				So(invs[0].ID, ShouldEqual, 3)
				So(cursor, ShouldEqual, "")
			})
		})

		Convey("GetInvocation works", func() {
			Convey("Anonymous can't see non-public job invocation", func() {
				_, err := e.GetVisibleInvocation(ctxAnon, "abc/1", 1)
				So(err, ShouldResemble, ErrNoSuchInvocation)
			})

			Convey("NoSuchInvocation", func() {
				_, err := e.GetVisibleInvocation(ctxAdmin, "missing/job", 1)
				So(err, ShouldResemble, ErrNoSuchInvocation)
			})

			Convey("Reader sees", func() {
				inv, err := e.GetVisibleInvocation(ctxOne, "abc/1", 1)
				So(inv, ShouldNotBeNil)
				So(err, ShouldBeNil)
			})
		})

		Convey("GetInvocationsByNonce works", func() {
			Convey("Anonymous can't see non-public job invocations", func() {
				invs, err := e.GetVisibleInvocationsByNonce(ctxAnon, "abc/1", 123)
				So(len(invs), ShouldEqual, 0)
				So(err, ShouldBeNil)
			})

			Convey("NoSuchInvocation", func() {
				invs, err := e.GetVisibleInvocationsByNonce(ctxAdmin, "abc/1", 11111) // unknown
				So(len(invs), ShouldEqual, 0)
				So(err, ShouldBeNil)
			})

			Convey("Wrong job ID", func() {
				invs, err := e.GetVisibleInvocationsByNonce(ctxAdmin, "abc/2", 123) // it is actually "abc/1"
				So(len(invs), ShouldEqual, 0)
				So(err, ShouldBeNil)
			})

			Convey("Reader sees", func() {
				invs, err := e.GetVisibleInvocationsByNonce(ctxOne, "abc/1", 123)
				So(len(invs), ShouldEqual, 2)
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestPrepareTopic(t *testing.T) {
	Convey("PrepareTopic works", t, func(ctx C) {
		c := newTestContext(epoch)

		e, _ := newTestEngine()

		pubSubCalls := 0
		e.configureTopic = func(c context.Context, topic, sub, pushURL, publisher string) error {
			pubSubCalls++
			ctx.So(topic, ShouldEqual, "projects/app/topics/dev-scheduler+noop+some~publisher.com")
			ctx.So(sub, ShouldEqual, "projects/app/subscriptions/dev-scheduler+noop+some~publisher.com")
			ctx.So(pushURL, ShouldEqual, "") // pull on dev server
			ctx.So(publisher, ShouldEqual, "some@publisher.com")
			return nil
		}

		ctl := &taskController{
			ctx:     c,
			eng:     e,
			manager: &noop.TaskManager{},
			saved: Invocation{
				ID:     123456,
				JobKey: ds.NewKey(c, "Job", "job_id", 0, nil),
			},
		}
		ctl.populateState()

		// Once.
		topic, token, err := ctl.PrepareTopic(c, "some@publisher.com")
		So(err, ShouldBeNil)
		So(topic, ShouldEqual, "projects/app/topics/dev-scheduler+noop+some~publisher.com")
		So(token, ShouldNotEqual, "")
		So(pubSubCalls, ShouldEqual, 1)

		// Again. 'configureTopic' should not be called anymore.
		_, _, err = ctl.PrepareTopic(c, "some@publisher.com")
		So(err, ShouldBeNil)
		So(pubSubCalls, ShouldEqual, 1)

		// Make sure memcache-based deduplication also works.
		e.doneFlags = make(map[string]bool)
		_, _, err = ctl.PrepareTopic(c, "some@publisher.com")
		So(err, ShouldBeNil)
		So(pubSubCalls, ShouldEqual, 1)
	})
}

func TestProcessPubSubPush(t *testing.T) {
	Convey("with mock invocation", t, func() {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		So(ds.Put(c, &Job{
			JobID:     "abc/1",
			ProjectID: "abc",
			Enabled:   true,
		}), ShouldBeNil)

		task, err := proto.Marshal(&messages.TaskDefWrapper{
			Noop: &messages.NoopTask{},
		})
		So(err, ShouldBeNil)

		inv := Invocation{
			ID:     1,
			JobKey: ds.NewKey(c, "Job", "abc/1", 0, nil),
			Task:   task,
		}
		So(ds.Put(c, &inv), ShouldBeNil)

		// Skip talking to PubSub for real.
		e.configureTopic = func(c context.Context, topic, sub, pushURL, publisher string) error {
			return nil
		}

		ctl, err := controllerForInvocation(c, e, &inv)
		So(err, ShouldBeNil)

		// Grab the working auth token.
		_, token, err := ctl.PrepareTopic(c, "some@publisher.com")
		So(err, ShouldBeNil)
		So(token, ShouldNotEqual, "")

		Convey("ProcessPubSubPush works", func() {
			msg := struct {
				Message pubsub.PubsubMessage `json:"message"`
			}{
				Message: pubsub.PubsubMessage{
					Attributes: map[string]string{"auth_token": token},
					Data:       "blah",
				},
			}
			blob, err := json.Marshal(&msg)
			So(err, ShouldBeNil)

			handled := false
			mgr.handleNotification = func(ctx context.Context, msg *pubsub.PubsubMessage) error {
				So(msg.Data, ShouldEqual, "blah")
				handled = true
				return nil
			}
			So(e.ProcessPubSubPush(c, blob), ShouldBeNil)
			So(handled, ShouldBeTrue)
		})

		Convey("ProcessPubSubPush handles bad token", func() {
			msg := struct {
				Message pubsub.PubsubMessage `json:"message"`
			}{
				Message: pubsub.PubsubMessage{
					Attributes: map[string]string{"auth_token": token + "blah"},
					Data:       "blah",
				},
			}
			blob, err := json.Marshal(&msg)
			So(err, ShouldBeNil)
			So(e.ProcessPubSubPush(c, blob), ShouldErrLike, "bad token")
		})

		Convey("ProcessPubSubPush handles missing invocation", func() {
			ds.Delete(c, ds.KeyForObj(c, &inv))
			msg := pubsub.PubsubMessage{
				Attributes: map[string]string{"auth_token": token},
			}
			blob, err := json.Marshal(&msg)
			So(err, ShouldBeNil)
			So(transient.Tag.In(e.ProcessPubSubPush(c, blob)), ShouldBeFalse)
		})
	})
}

func TestAborts(t *testing.T) {
	Convey("with mock invocation", t, func() {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()
		ctxAnon := auth.WithState(c, &authtest.FakeState{
			Identity: "anonymous:anonymous",
		})
		ctxReader := auth.WithState(c, &authtest.FakeState{
			Identity:       "user:reader@example.com",
			IdentityGroups: []string{"readers"},
		})
		ctxOwner := auth.WithState(c, &authtest.FakeState{
			Identity:       "user:owner@example.com",
			IdentityGroups: []string{"owners"},
		})

		// A job in "QUEUED" state (about to run an invocation).
		const jobID = "abc/1"
		const invNonce = int64(12345)
		prepareQueuedJob(c, jobID, invNonce)

		launchInv := func() int64 {
			var invID int64
			mgr.launchTask = func(ctx context.Context, ctl task.Controller, triggers []task.Trigger) error {
				invID = ctl.InvocationID()
				ctl.State().Status = task.StatusRunning
				So(ctl.Save(ctx), ShouldBeNil)
				return nil
			}
			So(e.startInvocation(c, jobID, invNonce, "", nil, 0), ShouldBeNil)

			// It is alive and the job entity tracks it.
			inv, err := e.getInvocation(c, jobID, invID)
			So(err, ShouldBeNil)
			So(inv.Status, ShouldEqual, task.StatusRunning)
			job, err := e.getJob(c, jobID)
			So(err, ShouldBeNil)
			So(job.State.State, ShouldEqual, JobStateRunning)
			So(job.State.InvocationID, ShouldEqual, invID)

			return invID
		}

		Convey("AbortInvocation works", func() {
			// Actually launch the queued invocation.
			invID := launchInv()

			// Try to kill it w/o permission.
			So(e.AbortInvocation(c, jobID, invID), ShouldNotBeNil) // No current identity.
			So(e.AbortInvocation(ctxAnon, jobID, invID), ShouldResemble, ErrNoSuchJob)
			So(e.AbortInvocation(ctxReader, jobID, invID), ShouldResemble, ErrNoOwnerPermission)
			// Now kill it.
			So(e.AbortInvocation(ctxOwner, jobID, invID), ShouldBeNil)

			// It is dead.
			inv, err := e.getInvocation(c, jobID, invID)
			So(err, ShouldBeNil)
			So(inv.Status, ShouldEqual, task.StatusAborted)

			// The job moved on with its life.
			job, err := e.getJob(c, jobID)
			So(err, ShouldBeNil)
			So(job.State.State, ShouldEqual, JobStateSuspended)
			So(job.State.InvocationID, ShouldEqual, 0)
		})

		Convey("AbortJob kills running invocation", func() {
			// Actually launch the queued invocation.
			invID := launchInv()

			// Try to kill it w/o permission.
			So(e.AbortJob(c, jobID), ShouldNotBeNil) // No current identity.
			So(e.AbortJob(ctxAnon, jobID), ShouldResemble, ErrNoSuchJob)
			So(e.AbortJob(ctxReader, jobID), ShouldResemble, ErrNoOwnerPermission)
			// Kill it.
			So(e.AbortJob(ctxOwner, jobID), ShouldBeNil)

			// It is dead.
			inv, err := e.getInvocation(c, jobID, invID)
			So(err, ShouldBeNil)
			So(inv.Status, ShouldEqual, task.StatusAborted)

			// The job moved on with its life.
			job, err := e.getJob(c, jobID)
			So(err, ShouldBeNil)
			So(job.State.State, ShouldEqual, JobStateSuspended)
			So(job.State.InvocationID, ShouldEqual, 0)
		})

		Convey("AbortJob kills queued invocation", func() {
			So(e.AbortJob(ctxOwner, jobID), ShouldBeNil)

			// The job moved on with its life.
			job, err := e.getJob(c, jobID)
			So(err, ShouldBeNil)
			So(job.State.State, ShouldEqual, JobStateSuspended)
			So(job.State.InvocationID, ShouldEqual, 0)
		})

		Convey("AbortJob fails on non-existing job", func() {
			So(e.AbortJob(ctxOwner, "not/exists"), ShouldResemble, ErrNoSuchJob)
		})
	})
}

func TestAddTimer(t *testing.T) {
	Convey("with mock job", t, func() {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		// A job in "QUEUED" state (about to run an invocation).
		const jobID = "abc/1"
		const invNonce = int64(12345)
		prepareQueuedJob(c, jobID, invNonce)

		Convey("AddTimer works", func() {
			// Start an invocation that adds a timer.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller, triggers []task.Trigger) error {
				ctl.AddTimer(ctx, time.Minute, "timer-name", []byte{1, 2, 3})
				ctl.State().Status = task.StatusRunning
				return nil
			}
			So(e.startInvocation(c, jobID, invNonce, "", nil, 0), ShouldBeNil)

			// The job is running.
			job, err := e.getJob(c, jobID)
			So(err, ShouldBeNil)
			So(job.State.State, ShouldEqual, JobStateRunning)

			// Added a task to the timers task queue.
			tasks := tq.GetTestable(c).GetScheduledTasks()["timers-q"]
			So(len(tasks), ShouldEqual, 1)
			var tqt *tq.Task
			for _, tqt = range tasks {
			}
			So(tqt.ETA, ShouldResemble, clock.Now(c).Add(time.Minute))

			// Verify task body.
			payload := actionTaskPayload{}
			So(json.Unmarshal(tqt.Payload, &payload), ShouldBeNil)
			So(payload, ShouldResemble, actionTaskPayload{
				JobID: "abc/1",
				InvID: 9200093523825174512,
				InvTimer: &invocationTimer{
					Delay:   time.Minute,
					Name:    "timer-name",
					Payload: []byte{1, 2, 3},
				},
			})

			// Clear the queue.
			tq.GetTestable(c).ResetTasks()

			// Time comes to execute the task.
			mgr.handleTimer = func(ctx context.Context, ctl task.Controller, name string, payload []byte) error {
				So(name, ShouldEqual, "timer-name")
				So(payload, ShouldResemble, []byte{1, 2, 3})
				ctl.AddTimer(ctx, time.Minute, "ignored-timer", nil)
				ctl.State().Status = task.StatusSucceeded
				return nil
			}
			clock.Get(c).(testclock.TestClock).Add(time.Minute)
			So(e.ExecuteSerializedAction(c, tqt.Payload, 0), ShouldBeNil)

			// The job has finished (by timer handler). Moves back to SUSPENDED state.
			job, err = e.getJob(c, jobID)
			So(err, ShouldBeNil)
			So(job.State.State, ShouldEqual, JobStateSuspended)

			// No new timers added for finished job.
			tasks = tq.GetTestable(c).GetScheduledTasks()["timers-q"]
			So(len(tasks), ShouldEqual, 0)
		})
	})
}

func TestTrimDebugLog(t *testing.T) {
	ctx := clock.Set(context.Background(), testclock.New(epoch))
	junk := strings.Repeat("a", 1000)

	genLines := func(start, end int) string {
		inv := Invocation{}
		for i := start; i < end; i++ {
			inv.debugLog(ctx, "Line %d - %s", i, junk)
		}
		return inv.DebugLog
	}

	Convey("small log is not trimmed", t, func() {
		inv := Invocation{
			DebugLog: genLines(0, 100),
		}
		inv.trimDebugLog()
		So(inv.DebugLog, ShouldEqual, genLines(0, 100))
	})

	Convey("huge log is trimmed", t, func() {
		inv := Invocation{
			DebugLog: genLines(0, 500),
		}
		inv.trimDebugLog()
		So(inv.DebugLog, ShouldEqual,
			genLines(0, 94)+"--- the log has been cut here ---\n"+genLines(400, 500))
	})

	Convey("writing lines to huge log and trimming", t, func() {
		inv := Invocation{
			DebugLog: genLines(0, 500),
		}
		inv.trimDebugLog()
		for i := 0; i < 10; i++ {
			inv.debugLog(ctx, "Line %d - %s", i, junk)
			inv.trimDebugLog()
		}
		// Still single cut only. New 10 lines are at the end.
		So(inv.DebugLog, ShouldEqual,
			genLines(0, 94)+"--- the log has been cut here ---\n"+genLines(410, 500)+genLines(0, 10))
	})

	Convey("one huge line", t, func() {
		inv := Invocation{
			DebugLog: strings.Repeat("z", 300000),
		}
		inv.trimDebugLog()
		const msg = "\n--- the log has been cut here ---\n"
		So(inv.DebugLog, ShouldEqual, strings.Repeat("z", debugLogSizeLimit-len(msg))+msg)
	})
}

////

func newTestContext(now time.Time) context.Context {
	c := memory.Use(context.Background())
	c = clock.Set(c, testclock.New(now))
	c = mathrand.Set(c, rand.New(rand.NewSource(1000)))
	c = testsecrets.Use(c)

	ds.GetTestable(c).AddIndexes(&ds.IndexDefinition{
		Kind: "Job",
		SortBy: []ds.IndexColumn{
			{Property: "Enabled"},
			{Property: "ProjectID"},
		},
	})
	ds.GetTestable(c).CatchupIndexes()

	tq.GetTestable(c).CreateQueue("timers-q")
	tq.GetTestable(c).CreateQueue("invs-q")
	return c
}

func newTestEngine() (*engineImpl, *fakeTaskManager) {
	mgr := &fakeTaskManager{}
	cat := catalog.New("scheduler.cfg")
	cat.RegisterTaskManager(mgr)
	return NewEngine(Config{
		Catalog:              cat,
		TimersQueuePath:      "/timers",
		TimersQueueName:      "timers-q",
		InvocationsQueuePath: "/invs",
		InvocationsQueueName: "invs-q",
		PubSubPushPath:       "/push-url",
	}).(*engineImpl), mgr
}

////

// fakeTaskManager implement task.Manager interface.
type fakeTaskManager struct {
	launchTask         func(ctx context.Context, ctl task.Controller, triggers []task.Trigger) error
	handleNotification func(ctx context.Context, msg *pubsub.PubsubMessage) error
	handleTimer        func(ctx context.Context, ctl task.Controller, name string, payload []byte) error
}

func (m *fakeTaskManager) Name() string {
	return "fake"
}

func (m *fakeTaskManager) ProtoMessageType() proto.Message {
	return (*messages.NoopTask)(nil)
}

func (m *fakeTaskManager) Traits() task.Traits {
	return task.Traits{}
}

func (m *fakeTaskManager) ValidateProtoMessage(msg proto.Message) error {
	return nil
}

func (m *fakeTaskManager) LaunchTask(c context.Context, ctl task.Controller, triggers []task.Trigger) error {
	return m.launchTask(c, ctl, triggers)
}

func (m *fakeTaskManager) AbortTask(c context.Context, ctl task.Controller) error {
	return nil
}

func (m *fakeTaskManager) HandleNotification(c context.Context, ctl task.Controller, msg *pubsub.PubsubMessage) error {
	return m.handleNotification(c, msg)
}

func (m fakeTaskManager) HandleTimer(c context.Context, ctl task.Controller, name string, payload []byte) error {
	return m.handleTimer(c, ctl, name, payload)
}

////

func sortedJobIds(jobs []*Job) []string {
	ids := stringset.New(len(jobs))
	for _, j := range jobs {
		ids.Add(j.JobID)
	}
	asSlice := ids.ToSlice()
	sort.Strings(asSlice)
	return asSlice
}

// prepareQueuedJob makes datastore entries for a job in QUEUED state.
func prepareQueuedJob(c context.Context, jobID string, invNonce int64) {
	taskBlob, err := proto.Marshal(&messages.TaskDefWrapper{
		Noop: &messages.NoopTask{},
	})
	if err != nil {
		panic(err)
	}
	chunks := strings.Split(jobID, "/")
	err = ds.Put(c, &Job{
		JobID:     jobID,
		ProjectID: chunks[0],
		Enabled:   true,
		Acls:      acl.GrantsByRole{Owners: []string{"group:owners"}, Readers: []string{"group:readers"}},
		Task:      taskBlob,
		Schedule:  "triggered",
		State: JobState{
			State:           JobStateQueued,
			InvocationNonce: invNonce,
		},
	})
	if err != nil {
		panic(err)
	}
}

func noopTaskBytes() []byte {
	buf, _ := proto.Marshal(&messages.TaskDefWrapper{Noop: &messages.NoopTask{}})
	return buf
}

func allJobs(c context.Context) []Job {
	ds.GetTestable(c).CatchupIndexes()
	entities := []Job{}
	if err := ds.GetAll(c, ds.NewQuery("Job"), &entities); err != nil {
		panic(err)
	}
	// Strip UTC location pointers from zero time.Time{} so that ShouldResemble
	// can compare it to default time.Time{}. nil location is UTC too.
	for i := range entities {
		ent := &entities[i]
		if ent.State.InvocationTime.IsZero() {
			ent.State.InvocationTime = time.Time{}
		}
		if ent.State.TickTime.IsZero() {
			ent.State.TickTime = time.Time{}
		}
	}
	return entities
}

func getJob(c context.Context, jobID string) Job {
	for _, job := range allJobs(c) {
		if job.JobID == jobID {
			return job
		}
	}
	panic(fmt.Errorf("no such jobs %s", jobID))
}

func ensureZeroTasks(c context.Context, q string) {
	tqt := tq.GetTestable(c)
	tasks := tqt.GetScheduledTasks()[q]
	So(tasks == nil || len(tasks) == 0, ShouldBeTrue)
}

func ensureOneTask(c context.Context, q string) *tq.Task {
	tqt := tq.GetTestable(c)
	tasks := tqt.GetScheduledTasks()[q]
	So(len(tasks), ShouldEqual, 1)
	for _, t := range tasks {
		return t
	}
	return nil
}

func popAllTasks(c context.Context, q string) []*tq.Task {
	tqt := tq.GetTestable(c)
	tasks := make([]*tq.Task, 0, len(tqt.GetScheduledTasks()[q]))
	for _, t := range tqt.GetScheduledTasks()[q] {
		tasks = append(tasks, t)
	}
	tqt.ResetTasks()
	return tasks
}
