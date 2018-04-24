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
	"go.chromium.org/gae/service/taskqueue"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets/testsecrets"

	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/acl"
	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/noop"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var (
	aclPublic = acl.GrantsByRole{Readers: []string{"group:all"}, Owners: []string{"group:administrators"}}
	aclSome   = acl.GrantsByRole{Readers: []string{"group:some"}}
	aclOne    = acl.GrantsByRole{Owners: []string{"one@example.com"}}
	aclAdmin  = acl.GrantsByRole{Readers: []string{"group:administrators"}, Owners: []string{"group:administrators"}}

	asUserOne = &authtest.FakeState{
		Identity:       "user:one@example.com",
		IdentityGroups: []string{"all"},
	}
)

func TestGetAllProjects(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
		taskqueue.GetTestable(c).ResetTasks()

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
					TickNonce: 2673062197574995716,
					TickTime:  epoch.Add(1 * time.Second),
				},
			},
		})
		// Enqueued timer task to launch it.
		task = ensureOneTask(c, "timers-q")
		So(task.Path, ShouldEqual, "/timers")
		So(task.ETA, ShouldResemble, epoch.Add(1*time.Second))
		taskqueue.GetTestable(c).ResetTasks()

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
	t.Parallel()

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
					TickNonce: 4941627882652650080,
					TickTime:  epoch.Add(5 * time.Second),
				},
			},
		})
		// Enqueued timer task to launch it.
		task := ensureOneTask(c, "timers-q")
		So(task.Path, ShouldEqual, "/timers")
		So(task.ETA, ShouldResemble, epoch.Add(5*time.Second))
		taskqueue.GetTestable(c).ResetTasks()
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
	t.Parallel()

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
					TickNonce: 2673062197574995716,
					TickTime:  epoch.Add(65 * time.Second),
				},
			},
		})
	})
}

func TestFullFlow(t *testing.T) {
	t.Parallel()

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
		taskqueue.GetTestable(c).ResetTasks()

		// Tick time comes, the tick task is executed, job is added to queue.
		clock.Get(c).(testclock.TestClock).Add(5 * time.Second)
		So(e.ExecuteSerializedAction(c, tsk.Payload, 0), ShouldBeNil)

		// Job is in queued state now.
		So(allJobs(c), ShouldResemble, expectedJobs(JobState{
			State:           "QUEUED",
			TickNonce:       2673062197574995716,
			TickTime:        epoch.Add(10 * time.Second),
			InvocationNonce: 4941627882652650080,
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
		taskqueue.GetTestable(c).ResetTasks()

		// Time to run the job and it fails to launch with a transient error.
		mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
			// Check data provided via the controller.
			So(ctl.JobID(), ShouldEqual, "abc/1")
			So(ctl.InvocationID(), ShouldEqual, int64(9200093518582484688))
			So(ctl.InvocationNonce(), ShouldEqual, int64(4941627882652650080))
			So(ctl.Task(), ShouldResemble, &messages.NoopTask{})

			ctl.DebugLog("oops, fail")
			return errors.New("oops", transient.Tag)
		}
		So(transient.Tag.In(e.ExecuteSerializedAction(c, invTask.Payload, 0)), ShouldBeTrue)

		// Still in QUEUED state, but with InvocatioID assigned.
		jobs := allJobs(c)
		So(jobs, ShouldResemble, expectedJobs(JobState{
			State:           "QUEUED",
			TickNonce:       2673062197574995716,
			TickTime:        epoch.Add(10 * time.Second),
			InvocationNonce: 4941627882652650080,
			InvocationTime:  epoch.Add(5 * time.Second),
			InvocationID:    9200093518582484688,
		}))
		jobKey := ds.KeyForObj(c, &jobs[0])

		// Check Invocation fields. It indicates that the attempt has failed and
		// will be retried.
		inv := Invocation{ID: 9200093518582484688, JobKey: jobKey}
		So(ds.Get(c, &inv), ShouldBeNil)
		inv.JobKey = nil // for easier ShouldResemble below
		debugLog := inv.DebugLog
		inv.DebugLog = ""
		So(inv, ShouldResemble, Invocation{
			ID:              9200093518582484688,
			InvocationNonce: 4941627882652650080,
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
			TickNonce:       2673062197574995716,
			TickTime:        epoch.Add(10 * time.Second),
			InvocationNonce: 4941627882652650080,
			InvocationTime:  epoch.Add(5 * time.Second),
			InvocationID:    9200093518582484688,
		}))

		// Second attempt. Now starts, hangs midway, they finishes.
		mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
			// Make sure Save() checkpoints the progress.
			ctl.DebugLog("Starting")
			ctl.State().Status = task.StatusRunning
			So(ctl.Save(ctx), ShouldBeNil)

			// After first Save the job and the invocation are in running state.
			So(allJobs(c), ShouldResemble, expectedJobs(JobState{
				State:                "RUNNING",
				TickNonce:            2673062197574995716,
				TickTime:             epoch.Add(10 * time.Second),
				InvocationNonce:      4941627882652650080,
				InvocationRetryCount: 1,
				InvocationTime:       epoch.Add(5 * time.Second),
				InvocationID:         9200093518582515376,
			}))
			inv := Invocation{ID: 9200093518582515376, JobKey: jobKey}
			So(ds.Get(c, &inv), ShouldBeNil)
			inv.JobKey = nil // for easier ShouldResemble below
			So(inv, ShouldResemble, Invocation{
				ID:              9200093518582515376,
				InvocationNonce: 4941627882652650080,
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
			// Pretend invocation actually took some time.
			clock.Get(c).(testclock.TestClock).Add(1 * time.Second)
			return nil
		}
		So(e.ExecuteSerializedAction(c, invTask.Payload, 1), ShouldBeNil)

		// After final save.
		inv = Invocation{ID: 9200093518582515376, JobKey: jobKey}
		So(ds.Get(c, &inv), ShouldBeNil)
		inv.JobKey = nil // for easier ShouldResemble below
		debugLog = inv.DebugLog
		inv.DebugLog = ""
		So(inv, ShouldResemble, Invocation{
			ID:              9200093518582515376,
			InvocationNonce: 4941627882652650080,
			Revision:        "rev1",
			Started:         epoch.Add(5 * time.Second),
			Finished:        epoch.Add(6 * time.Second),
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
		So(getSentDistrValue(c, metricInvocationsDurations, "abc/1", "SUCCEEDED"), ShouldEqual, 1.0)

		// Previous invocation is aborted now (in Failed state).
		inv = Invocation{ID: 9200093518582484688, JobKey: jobKey}
		So(ds.Get(c, &inv), ShouldBeNil)
		inv.JobKey = nil // for easier ShouldResemble below
		debugLog = inv.DebugLog
		inv.DebugLog = ""
		So(inv, ShouldResemble, Invocation{
			ID:              9200093518582484688,
			InvocationNonce: 4941627882652650080,
			Revision:        "rev1",
			Started:         epoch.Add(5 * time.Second),
			Finished:        epoch.Add(5 * time.Second),
			Task:            taskBytes,
			DebugLog:        "",
			Status:          task.StatusFailed,
			MutationsCount:  2,
		})
		So(debugLog, ShouldContainSubstring,
			"[22:42:05.000] New invocation is starting (9200093518582515376), marking this one as failed")

		// Job is in scheduled state again.
		So(allJobs(c), ShouldResemble, expectedJobs(JobState{
			State:     "SCHEDULED",
			TickNonce: 2673062197574995716,
			TickTime:  epoch.Add(10 * time.Second),
			PrevTime:  epoch.Add(6 * time.Second),
		}))
	})
}

func TestForceInvocation(t *testing.T) {
	t.Parallel()

	Convey("full flow", t, func() {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		job := &Job{
			JobID:     "abc/1",
			ProjectID: "abc",
			Enabled:   true,
			Schedule:  "triggered",
			Task:      noopTaskBytes(),
			State:     JobState{State: JobStateSuspended},
			Acls:      acl.GrantsByRole{Owners: []string{"one@example.com"}},
		}
		So(ds.Put(c, job), ShouldBeNil)

		ctxOne := auth.WithState(c, &authtest.FakeState{Identity: "user:one@example.com"})
		ctxTwo := auth.WithState(c, &authtest.FakeState{Identity: "user:two@example.com"})

		// Only owner can trigger.
		fut, err := e.ForceInvocation(ctxTwo, job)
		So(err, ShouldEqual, ErrNoPermission)

		// Triggers something.
		fut, err = e.ForceInvocation(ctxOne, job)
		So(err, ShouldBeNil)
		So(fut, ShouldNotBeNil)

		// No invocation yet.
		invID, err := fut.InvocationID(ctxOne)
		So(err, ShouldBeNil)
		So(invID, ShouldEqual, 0)

		// But the launch is queued.
		invTask := ensureOneTask(c, "invs-q")
		So(invTask.Path, ShouldEqual, "/invs")
		taskqueue.GetTestable(c).ResetTasks()

		// Launch it.
		var startedInvID int64
		mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
			startedInvID = ctl.InvocationID()
			ctl.State().Status = task.StatusRunning
			return nil
		}
		So(e.ExecuteSerializedAction(c, invTask.Payload, 0), ShouldBeNil)

		ds.GetTestable(c).CatchupIndexes()

		// The invocation ID is now available.
		invID, err = fut.InvocationID(ctxOne)
		So(err, ShouldBeNil)
		So(invID, ShouldEqual, startedInvID)
	})
}

func TestFullTriggeredFlow(t *testing.T) {
	t.Parallel()

	Convey("full triggered flow", t, func() {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()
		taskBytes := noopTaskBytes()

		// Create a new triggering noop job (ticks every 5 sec).
		jobsDefinitions := []catalog.Definition{
			{
				JobID:    "abc/1",
				Revision: "rev1",
				Schedule: "*/5 * * * * * *",
				Task:     taskBytes,
				Flavor:   catalog.JobFlavorTrigger,
				TriggeredJobIDs: []string{
					"abc/2-triggered",
					"abc/3-triggered",
					"abc/4-triggered",
				},
			},
		}
		// And also jobs 2, 3, 4 to be triggered by job 1.
		for i := 2; i <= 4; i++ {
			jobsDefinitions = append(jobsDefinitions, catalog.Definition{
				JobID:    fmt.Sprintf("abc/%d-triggered", i),
				Revision: "rev1",
				Schedule: "triggered",
				Task:     taskBytes,
				Flavor:   catalog.JobFlavorTriggered,
			})
		}
		So(e.UpdateProjectJobs(c, "abc", jobsDefinitions), ShouldBeNil)

		// Pause job #4. It should skip the incoming trigger.
		job4 := getJob(c, "abc/4-triggered")
		So(e.setJobPausedFlag(c, &job4, true, "user:someone@example.com"), ShouldBeNil)
		So(getJob(c, "abc/4-triggered").State.State, ShouldEqual, JobStateSuspended)

		// Enqueued timer task to launch it.
		tsk := ensureOneTask(c, "timers-q")
		So(tsk.Path, ShouldEqual, "/timers")
		So(tsk.ETA, ShouldResemble, epoch.Add(5*time.Second))
		taskqueue.GetTestable(c).ResetTasks()

		// Tick time comes, the tick task is executed, job is added to queue.
		clock.Get(c).(testclock.TestClock).Add(5 * time.Second)
		So(e.ExecuteSerializedAction(c, tsk.Payload, 0), ShouldBeNil)

		// Job1 is in queued state now.
		job1 := getJob(c, "abc/1")
		So(job1.Flavor, ShouldEqual, catalog.JobFlavorTrigger)
		So(job1.TriggeredJobIDs, ShouldResemble, []string{
			"abc/2-triggered",
			"abc/3-triggered",
			"abc/4-triggered",
		})
		So(job1.State.State, ShouldEqual, JobStateQueued)

		// Next tick task is added.
		tickTask := ensureOneTask(c, "timers-q")
		So(tickTask.Path, ShouldEqual, "/timers")
		So(tickTask.ETA, ShouldResemble, epoch.Add(10*time.Second))

		// Invocation task (ETA is 1 sec in the future).
		invTask := ensureOneTask(c, "invs-q")
		So(invTask.Path, ShouldEqual, "/invs")
		So(invTask.ETA, ShouldResemble, epoch.Add(6*time.Second))
		taskqueue.GetTestable(c).ResetTasks()

		var invID int64 // set inside launchTask once invocation is known.

		mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
			// Make sure Save() checkpoints the progress.
			ctl.DebugLog("Starting")
			ctl.State().Status = task.StatusRunning
			So(ctl.Save(ctx), ShouldBeNil)

			// After first Save the job and the invocation are in running state.
			j1 := getJob(c, "abc/1")
			So(j1.State.State, ShouldEqual, JobStateRunning)
			invID = j1.State.InvocationID
			inv, err := e.getInvocation(c, "abc/1", invID, false)
			So(err, ShouldBeNil)
			So(inv.TriggeredJobIDs, ShouldResemble, []string{
				"abc/2-triggered",
				"abc/3-triggered",
				"abc/4-triggered",
			})
			So(inv.DebugLog, ShouldEqual, "[22:42:05.000] Invocation initiated (attempt 1)\n[22:42:05.000] Starting\n")
			So(inv.Status, ShouldEqual, task.StatusRunning)
			So(inv.MutationsCount, ShouldEqual, 1)

			ctl.EmitTrigger(ctx, &internal.Trigger{
				Id: "trg",
				Payload: &internal.Trigger_Noop{
					Noop: &api.NoopTrigger{Data: "note the trigger id"},
				},
			})
			ctl.EmitTrigger(ctx, &internal.Trigger{
				Id: "trg",
				Payload: &internal.Trigger_Noop{
					Noop: &api.NoopTrigger{Data: "different payload"},
				},
			})

			// Change state to the final one.
			ctl.State().Status = task.StatusSucceeded
			ctl.State().ViewURL = "http://view_url"
			ctl.State().TaskData = []byte("blah")
			// Pretend invocation actually took some time.
			clock.Get(c).(testclock.TestClock).Add(15625 * time.Microsecond)
			return nil
		}
		So(e.ExecuteSerializedAction(c, invTask.Payload, 0), ShouldBeNil)

		// After final save.
		inv, err := e.getInvocation(c, "abc/1", invID, false)
		So(err, ShouldBeNil)
		So(inv.Status, ShouldEqual, task.StatusSucceeded)
		So(inv.MutationsCount, ShouldEqual, 2)
		So(inv.DebugLog, ShouldContainSubstring, "[22:42:05.000] Emitting a trigger trg") // twice.
		So(getSentDistrValue(c, metricInvocationsDurations, "abc/1", "SUCCEEDED"), ShouldEqual, 0.015625)

		// All triggers will be batched into just 1 tq task.
		batchTask := ensureOneTask(c, "invs-q")
		taskqueue.GetTestable(c).ResetTasks()
		So(e.ExecuteSerializedAction(c, batchTask.Payload, 0), ShouldBeNil)
		// Now execute each of trigger tasks.
		for _, triggerTask := range popAllTasks(c, "invs-q") {
			So(e.ExecuteSerializedAction(c, triggerTask.Payload, 0), ShouldBeNil)
		}

		// Triggers should result in new invocations for previously suspended jobs.
		So(getJob(c, "abc/2-triggered").State.State, ShouldEqual, JobStateQueued)
		So(getJob(c, "abc/3-triggered").State.State, ShouldEqual, JobStateQueued)

		// Except job 4, which is paused.
		job4 = getJob(c, "abc/4-triggered")
		So(job4.State.State, ShouldEqual, JobStateSuspended)
		So(job4.State.PendingTriggersRaw, ShouldBeNil)

		// Prepare to track triggers passed to task launchers.
		deliveredTriggers := map[string][]string{}
		mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
			So(deliveredTriggers, ShouldNotContainKey, ctl.JobID())
			req := ctl.Request()
			ids := make([]string, 0, len(req.IncomingTriggers))
			for _, t := range req.IncomingTriggers {
				ids = append(ids, t.Id)
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
	t.Parallel()

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
	t.Parallel()

	Convey("with mock data", t, func() {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

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

		Convey("ListInvocations works", func() {
			job, err := e.GetVisibleJob(ctxOne, "abc/1")
			So(err, ShouldBeNil)

			Convey("With paging", func() {
				invs, cursor, err := e.ListInvocations(ctxOne, job, ListInvocationsOpts{
					PageSize: 2,
				})
				So(err, ShouldBeNil)
				So(len(invs), ShouldEqual, 2)
				So(invs[0].ID, ShouldEqual, 1)
				So(invs[1].ID, ShouldEqual, 2)
				So(cursor, ShouldNotEqual, "")

				invs, cursor, err = e.ListInvocations(ctxOne, job, ListInvocationsOpts{
					PageSize: 2,
					Cursor:   cursor,
				})
				So(err, ShouldBeNil)
				So(len(invs), ShouldEqual, 1)
				So(invs[0].ID, ShouldEqual, 3)
				So(cursor, ShouldEqual, "")
			})
		})

		Convey("GetInvocation works", func() {
			job, err := e.GetVisibleJob(ctxOne, "abc/1")
			So(err, ShouldBeNil)

			Convey("NoSuchInvocation", func() {
				_, err = e.GetInvocation(ctxOne, job, 666) // Missing invocation.
				So(err, ShouldResemble, ErrNoSuchInvocation)
			})

			Convey("Existing invocation", func() {
				inv, err := e.GetInvocation(ctxOne, job, 1)
				So(inv, ShouldNotBeNil)
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestRecordOverrun(t *testing.T) {
	t.Parallel()

	Convey("RecordOverrun works", t, func(ctx C) {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

		job := &Job{JobID: "abc/1"}
		So(ds.Put(c, job), ShouldBeNil)
		So(e.recordOverrun(c, "abc/1", 1, 0), ShouldBeNil)

		ds.GetTestable(c).CatchupIndexes()

		q := ds.NewQuery("Invocation").Ancestor(ds.KeyForObj(c, job))
		var all []Invocation
		So(ds.GetAll(c, q, &all), ShouldEqual, nil)
		So(all, ShouldResemble, []Invocation{
			{
				ID:       9200093523825174512,
				JobKey:   ds.KeyForObj(c, job),
				Started:  epoch,
				Finished: epoch,
				Status:   task.StatusOverrun,
				DebugLog: "[22:42:00.000] New invocation should be starting now, but previous one is still starting\n" +
					"[22:42:00.000] Total overruns thus far: 1\n",
			}})
		So(getSentMetric(c, metricInvocationsOverrun, "abc/1"), ShouldEqual, 1)
	})
}

func TestPrepareTopic(t *testing.T) {
	t.Parallel()

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
	})
}

func TestProcessPubSubPush(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	Convey("with mock invocation", t, func() {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()

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
		job := prepareQueuedJob(c, jobID, invNonce)

		launchInv := func() int64 {
			var invID int64
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				invID = ctl.InvocationID()
				ctl.State().Status = task.StatusRunning
				So(ctl.Save(ctx), ShouldBeNil)
				return nil
			}
			So(e.startInvocation(c, jobID, invNonce, "", nil, 0), ShouldBeNil)

			// It is alive and the job entity tracks it.
			inv, err := e.getInvocation(c, jobID, invID, false)
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
			clock.Get(c).(testclock.TestClock).Add(1 * time.Hour)

			// Try to kill it w/o permission.
			So(e.AbortInvocation(ctxReader, job, invID), ShouldResemble, ErrNoPermission)
			// Now kill it.
			So(e.AbortInvocation(ctxOwner, job, invID), ShouldBeNil)

			// It is dead.
			inv, err := e.getInvocation(c, jobID, invID, false)
			So(err, ShouldBeNil)
			So(inv.Status, ShouldEqual, task.StatusAborted)
			So(getSentDistrValue(c, metricInvocationsDurations, jobID, "ABORTED"), ShouldEqual, 3600)

			// The job moved on with its life.
			job, err := e.getJob(c, jobID)
			So(err, ShouldBeNil)
			So(job.State.State, ShouldEqual, JobStateSuspended)
			So(job.State.InvocationID, ShouldEqual, 0)
		})

		Convey("AbortJob kills running invocation", func() {
			// Actually launch the queued invocation.
			invID := launchInv()
			clock.Get(c).(testclock.TestClock).Add(8250 * time.Millisecond)

			// Try to kill it w/o permission.
			So(e.AbortJob(ctxReader, job), ShouldResemble, ErrNoPermission)
			// Kill it.
			So(e.AbortJob(ctxOwner, job), ShouldBeNil)

			// It is dead.
			inv, err := e.getInvocation(c, jobID, invID, false)
			So(err, ShouldBeNil)
			So(inv.Status, ShouldEqual, task.StatusAborted)
			So(getSentDistrValue(c, metricInvocationsDurations, jobID, "ABORTED"), ShouldEqual, 8.25)

			// The job moved on with its life.
			job, err := e.getJob(c, jobID)
			So(err, ShouldBeNil)
			So(job.State.State, ShouldEqual, JobStateSuspended)
			So(job.State.InvocationID, ShouldEqual, 0)
		})

		Convey("AbortJob kills queued invocation", func() {
			So(e.AbortJob(ctxOwner, job), ShouldBeNil)

			// The job moved on with its life.
			job, err := e.getJob(c, jobID)
			So(err, ShouldBeNil)
			So(job.State.State, ShouldEqual, JobStateSuspended)
			So(job.State.InvocationID, ShouldEqual, 0)
		})
	})
}

func TestAddTimer(t *testing.T) {
	t.Parallel()

	Convey("with mock job", t, func() {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		// A job in "QUEUED" state (about to run an invocation).
		const jobID = "abc/1"
		const invNonce = int64(12345)
		prepareQueuedJob(c, jobID, invNonce)

		Convey("AddTimer works", func() {
			// Start an invocation that adds a timer.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
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
			tasks := taskqueue.GetTestable(c).GetScheduledTasks()["timers-q"]
			So(len(tasks), ShouldEqual, 1)
			var tqt *taskqueue.Task
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
			taskqueue.GetTestable(c).ResetTasks()

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
			// Invocation is complete.
			So(getSentDistrValue(c, metricInvocationsDurations, jobID, "SUCCEEDED"), ShouldEqual, 60.0)

			// The job has finished (by timer handler). Moves back to SUSPENDED state.
			job, err = e.getJob(c, jobID)
			So(err, ShouldBeNil)
			So(job.State.State, ShouldEqual, JobStateSuspended)

			// No new timers added for finished job.
			tasks = taskqueue.GetTestable(c).GetScheduledTasks()["timers-q"]
			So(len(tasks), ShouldEqual, 0)
		})
	})
}

func TestTrimDebugLog(t *testing.T) {
	t.Parallel()

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

	c, _, _ = tsmon.WithFakes(c)
	fake := store.NewInMemory(&target.Task{})
	tsmon.GetState(c).SetStore(fake)

	ds.GetTestable(c).AddIndexes(&ds.IndexDefinition{
		Kind: "Job",
		SortBy: []ds.IndexColumn{
			{Property: "Enabled"},
			{Property: "ProjectID"},
		},
	})
	ds.GetTestable(c).CatchupIndexes()

	taskqueue.GetTestable(c).CreateQueue("timers-q")
	taskqueue.GetTestable(c).CreateQueue("invs-q")
	return c
}

// getSentMetric returns sent value or nil if value wasn't sent.
func getSentMetric(c context.Context, m types.Metric, fieldVals ...interface{}) interface{} {
	return tsmon.GetState(c).S.Get(c, m, time.Time{}, fieldVals)
}

// getSentDistrValue returns the value that was added to distribuition after
// ensuring there was exactly 1 value sent.
func getSentDistrValue(c context.Context, m types.Metric, fieldVals ...interface{}) float64 {
	switch d, ok := getSentMetric(c, m, fieldVals...).(*distribution.Distribution); {
	case !ok:
		panic(errors.New("not a distribuition"))
	case d.Count() != 1:
		panic(fmt.Errorf("expected 1 value, but %d values were sent with sum of %f", d.Count(), d.Sum()))
	default:
		return d.Sum()
	}
}

func newTestEngine() (*engineImpl, *fakeTaskManager) {
	mgr := &fakeTaskManager{}
	cat := catalog.New()
	cat.RegisterTaskManager(mgr)
	return NewEngine(Config{
		Catalog:              cat,
		Dispatcher:           &tq.Dispatcher{},
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
	launchTask         func(ctx context.Context, ctl task.Controller) error
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

func (m *fakeTaskManager) ValidateProtoMessage(c *validation.Context, msg proto.Message) {}

func (m *fakeTaskManager) LaunchTask(c context.Context, ctl task.Controller) error {
	return m.launchTask(c, ctl)
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
func prepareQueuedJob(c context.Context, jobID string, invNonce int64) *Job {
	taskBlob, err := proto.Marshal(&messages.TaskDefWrapper{
		Noop: &messages.NoopTask{},
	})
	if err != nil {
		panic(err)
	}
	job := &Job{
		JobID:     jobID,
		ProjectID: strings.Split(jobID, "/")[0],
		Enabled:   true,
		Acls:      acl.GrantsByRole{Owners: []string{"group:owners"}, Readers: []string{"group:readers"}},
		Task:      taskBlob,
		Schedule:  "triggered",
		State: JobState{
			State:           JobStateQueued,
			InvocationNonce: invNonce,
		},
	}
	err = ds.Put(c, job)
	if err != nil {
		panic(err)
	}
	return job
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
	tqt := taskqueue.GetTestable(c)
	tasks := tqt.GetScheduledTasks()[q]
	So(tasks == nil || len(tasks) == 0, ShouldBeTrue)
}

func ensureOneTask(c context.Context, q string) *taskqueue.Task {
	tqt := taskqueue.GetTestable(c)
	tasks := tqt.GetScheduledTasks()[q]
	So(len(tasks), ShouldEqual, 1)
	for _, t := range tasks {
		return t
	}
	return nil
}

func popAllTasks(c context.Context, q string) []*taskqueue.Task {
	tqt := taskqueue.GetTestable(c)
	tasks := make([]*taskqueue.Task, 0, len(tqt.GetScheduledTasks()[q]))
	for _, t := range tqt.GetScheduledTasks()[q] {
		tasks = append(tasks, t)
	}
	tqt.ResetTasks()
	return tasks
}
