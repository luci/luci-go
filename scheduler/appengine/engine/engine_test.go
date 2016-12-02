// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package engine

import (
	"encoding/json"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/api/pubsub/v1"

	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	tq "github.com/luci/gae/service/taskqueue"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/common/data/stringset"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/server/secrets/testsecrets"

	"github.com/luci/luci-go/scheduler/appengine/catalog"
	"github.com/luci/luci-go/scheduler/appengine/messages"
	"github.com/luci/luci-go/scheduler/appengine/task"
	"github.com/luci/luci-go/scheduler/appengine/task/noop"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
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
		So(errors.IsTransient(err), ShouldBeTrue)
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

		// Adding a new job (ticks every 5 sec).
		So(e.UpdateProjectJobs(c, "abc", []catalog.Definition{
			{
				JobID:    "abc/1",
				Revision: "rev1",
				Schedule: "*/5 * * * * * *",
				Task:     taskBytes,
			}}), ShouldBeNil)
		So(allJobs(c), ShouldResemble, []Job{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev1",
				Enabled:   true,
				Schedule:  "*/5 * * * * * *",
				Task:      taskBytes,
				State: JobState{
					State:     "SCHEDULED",
					TickNonce: 6278013164014963328,
					TickTime:  epoch.Add(5 * time.Second),
				},
			},
		})
		// Enqueued timer task to launch it.
		tsk := ensureOneTask(c, "timers-q")
		So(tsk.Path, ShouldEqual, "/timers")
		So(tsk.ETA, ShouldResemble, epoch.Add(5*time.Second))
		tq.GetTestable(c).ResetTasks()

		// Tick time comes, the tick task is executed, job is added to queue.
		clock.Get(c).(testclock.TestClock).Add(5 * time.Second)
		So(e.ExecuteSerializedAction(c, tsk.Payload, 0), ShouldBeNil)

		// Job is in queued state now.
		So(allJobs(c), ShouldResemble, []Job{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev1",
				Enabled:   true,
				Schedule:  "*/5 * * * * * *",
				Task:      taskBytes,
				State: JobState{
					State:           "QUEUED",
					TickNonce:       886585524575582446,
					TickTime:        epoch.Add(10 * time.Second),
					InvocationNonce: 928953616732700780,
					InvocationTime:  epoch.Add(5 * time.Second),
				},
			},
		})

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
		mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
			// Check data provided via the controller.
			So(ctl.JobID(), ShouldEqual, "abc/1")
			So(ctl.InvocationID(), ShouldEqual, int64(9200093518582198800))
			So(ctl.InvocationNonce(), ShouldEqual, int64(928953616732700780))
			So(ctl.Task(), ShouldResemble, &messages.NoopTask{})

			ctl.DebugLog("oops, fail")
			return errors.WrapTransient(errors.New("oops"))
		}
		So(errors.IsTransient(e.ExecuteSerializedAction(c, invTask.Payload, 0)), ShouldBeTrue)

		// Still in QUEUED state, but with InvocatioID assigned.
		jobs := allJobs(c)
		So(jobs, ShouldResemble, []Job{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev1",
				Enabled:   true,
				Schedule:  "*/5 * * * * * *",
				Task:      taskBytes,
				State: JobState{
					State:           "QUEUED",
					TickNonce:       886585524575582446,
					TickTime:        epoch.Add(10 * time.Second),
					InvocationNonce: 928953616732700780,
					InvocationTime:  epoch.Add(5 * time.Second),
					InvocationID:    9200093518582198800,
				},
			},
		})
		jobKey := ds.KeyForObj(c, &jobs[0])

		// Check Invocation fields.
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
			Finished:        epoch.Add(5 * time.Second),
			Task:            taskBytes,
			DebugLog:        "",
			Status:          task.StatusFailed,
			MutationsCount:  1,
		})
		So(debugLog, ShouldContainSubstring, "[22:42:05.000] Invocation initiated (attempt 1)")
		So(debugLog, ShouldContainSubstring, "[22:42:05.000] oops, fail")
		So(debugLog, ShouldContainSubstring, "with status FAILED")
		So(debugLog, ShouldContainSubstring, "[22:42:05.000] It will probably be retried")

		// Second attempt. Now starts, hangs midway, they finishes.
		mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
			// Make sure Save() checkpoints the progress.
			ctl.DebugLog("Starting")
			ctl.State().Status = task.StatusRunning
			So(ctl.Save(ctx), ShouldBeNil)

			// After first Save the job and the invocation are in running state.
			So(allJobs(c), ShouldResemble, []Job{
				{
					JobID:     "abc/1",
					ProjectID: "abc",
					Revision:  "rev1",
					Enabled:   true,
					Schedule:  "*/5 * * * * * *",
					Task:      taskBytes,
					State: JobState{
						State:                "RUNNING",
						TickNonce:            886585524575582446,
						TickTime:             epoch.Add(10 * time.Second),
						InvocationNonce:      928953616732700780,
						InvocationRetryCount: 1,
						InvocationTime:       epoch.Add(5 * time.Second),
						InvocationID:         9200093518582296192,
					},
				},
			})
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

		// Previous invocation is canceled.
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
			MutationsCount:  1,
		})
		So(debugLog, ShouldContainSubstring, "[22:42:05.000] Invocation initiated (attempt 1)")
		So(debugLog, ShouldContainSubstring, "[22:42:05.000] oops, fail")
		So(debugLog, ShouldContainSubstring, "with status FAILED")
		So(debugLog, ShouldContainSubstring, "[22:42:05.000] It will probably be retried")

		// Job is in scheduled state again.
		So(allJobs(c), ShouldResemble, []Job{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev1",
				Enabled:   true,
				Schedule:  "*/5 * * * * * *",
				Task:      taskBytes,
				State: JobState{
					State:     "SCHEDULED",
					TickNonce: 886585524575582446,
					TickTime:  epoch.Add(10 * time.Second),
					PrevTime:  epoch.Add(5 * time.Second),
				},
			},
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

		So(ds.Put(c,
			&Job{JobID: "abc/1", ProjectID: "abc", Enabled: true},
			&Job{JobID: "abc/2", ProjectID: "abc", Enabled: true},
			&Job{JobID: "def/1", ProjectID: "def", Enabled: true},
			&Job{JobID: "def/2", ProjectID: "def", Enabled: false},
		), ShouldBeNil)

		job1 := ds.NewKey(c, "Job", "abc/1", 0, nil)
		job2 := ds.NewKey(c, "Job", "abc/2", 0, nil)
		So(ds.Put(c,
			&Invocation{ID: 1, JobKey: job1, InvocationNonce: 123},
			&Invocation{ID: 2, JobKey: job1, InvocationNonce: 123},
			&Invocation{ID: 3, JobKey: job1},
			&Invocation{ID: 1, JobKey: job2},
			&Invocation{ID: 2, JobKey: job2},
			&Invocation{ID: 3, JobKey: job2},
		), ShouldBeNil)

		ds.GetTestable(c).CatchupIndexes()

		Convey("GetAllJobs works", func() {
			jobs, err := e.GetAllJobs(c)
			So(err, ShouldBeNil)
			ids := stringset.New(0)
			for _, j := range jobs {
				ids.Add(j.JobID)
			}
			asSlice := ids.ToSlice()
			sort.Strings(asSlice)
			So(asSlice, ShouldResemble, []string{"abc/1", "abc/2", "def/1"}) // only enabled
		})

		Convey("GetProjectJobs works", func() {
			jobs, err := e.GetProjectJobs(c, "def")
			So(err, ShouldBeNil)
			So(len(jobs), ShouldEqual, 1)
			So(jobs[0].JobID, ShouldEqual, "def/1")
		})

		Convey("GetJob works", func() {
			job, err := e.GetJob(c, "missing/job")
			So(job, ShouldBeNil)
			So(err, ShouldBeNil)

			job, err = e.GetJob(c, "abc/1")
			So(job, ShouldNotBeNil)
			So(err, ShouldBeNil)
		})

		Convey("ListInvocations works", func() {
			invs, cursor, err := e.ListInvocations(c, "abc/1", 2, "")
			So(err, ShouldBeNil)
			So(len(invs), ShouldEqual, 2)
			So(invs[0].ID, ShouldEqual, 1)
			So(invs[1].ID, ShouldEqual, 2)
			So(cursor, ShouldNotEqual, "")

			invs, cursor, err = e.ListInvocations(c, "abc/1", 2, cursor)
			So(err, ShouldBeNil)
			So(len(invs), ShouldEqual, 1)
			So(invs[0].ID, ShouldEqual, 3)
			So(cursor, ShouldEqual, "")
		})

		Convey("GetInvocation works", func() {
			inv, err := e.GetInvocation(c, "missing/job", 1)
			So(inv, ShouldBeNil)
			So(err, ShouldBeNil)

			inv, err = e.GetInvocation(c, "abc/1", 1)
			So(inv, ShouldNotBeNil)
			So(err, ShouldBeNil)
		})

		Convey("GetInvocationsByNonce works", func() {
			inv, err := e.GetInvocationsByNonce(c, 11111) // unknown
			So(len(inv), ShouldEqual, 0)
			So(err, ShouldBeNil)

			inv, err = e.GetInvocationsByNonce(c, 123)
			So(len(inv), ShouldEqual, 2)
			So(err, ShouldBeNil)
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

		ctl, err := e.controllerForInvocation(c, &inv)
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
			So(errors.IsTransient(e.ProcessPubSubPush(c, blob)), ShouldBeFalse)
		})
	})
}

func TestAborts(t *testing.T) {
	Convey("with mock invocation", t, func() {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		// A job in "QUEUED" state (about to run an invocation).
		const jobID = "abc/1"
		const invNonce = int64(12345)
		prepareQueuedJob(c, jobID, invNonce)

		launchInv := func() int64 {
			var invID int64
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				invID = ctl.InvocationID()
				ctl.State().Status = task.StatusRunning
				So(ctl.Save(ctx), ShouldBeNil)
				return nil
			}
			So(e.startInvocation(c, jobID, invNonce, "", 0), ShouldBeNil)

			// It is alive and the job entity tracks it.
			inv, err := e.GetInvocation(c, jobID, invID)
			So(err, ShouldBeNil)
			So(inv.Status, ShouldEqual, task.StatusRunning)
			job, err := e.GetJob(c, jobID)
			So(err, ShouldBeNil)
			So(job.State.State, ShouldEqual, JobStateRunning)
			So(job.State.InvocationID, ShouldEqual, invID)

			return invID
		}

		Convey("AbortInvocation works", func() {
			// Actually launch the queued invocation.
			invID := launchInv()

			// Kill it.
			So(e.AbortInvocation(c, jobID, invID, ""), ShouldBeNil)

			// It is dead.
			inv, err := e.GetInvocation(c, jobID, invID)
			So(err, ShouldBeNil)
			So(inv.Status, ShouldEqual, task.StatusAborted)

			// The job moved on with its life.
			job, err := e.GetJob(c, jobID)
			So(err, ShouldBeNil)
			So(job.State.State, ShouldEqual, JobStateSuspended)
			So(job.State.InvocationID, ShouldEqual, 0)
		})

		Convey("AbortJob kills running invocation", func() {
			// Actually launch the queued invocation.
			invID := launchInv()

			// Kill it.
			So(e.AbortJob(c, jobID, ""), ShouldBeNil)

			// It is dead.
			inv, err := e.GetInvocation(c, jobID, invID)
			So(err, ShouldBeNil)
			So(inv.Status, ShouldEqual, task.StatusAborted)

			// The job moved on with its life.
			job, err := e.GetJob(c, jobID)
			So(err, ShouldBeNil)
			So(job.State.State, ShouldEqual, JobStateSuspended)
			So(job.State.InvocationID, ShouldEqual, 0)
		})

		Convey("AbortJob kills queued invocation", func() {
			So(e.AbortJob(c, jobID, ""), ShouldBeNil)

			// The job moved on with its life.
			job, err := e.GetJob(c, jobID)
			So(err, ShouldBeNil)
			So(job.State.State, ShouldEqual, JobStateSuspended)
			So(job.State.InvocationID, ShouldEqual, 0)
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
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				ctl.AddTimer(ctx, time.Minute, "timer-name", []byte{1, 2, 3})
				ctl.State().Status = task.StatusRunning
				return nil
			}
			So(e.startInvocation(c, jobID, invNonce, "", 0), ShouldBeNil)

			// The job is running.
			job, err := e.GetJob(c, jobID)
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
			job, err = e.GetJob(c, jobID)
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

func (m *fakeTaskManager) ValidateProtoMessage(msg proto.Message) error {
	return nil
}

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
