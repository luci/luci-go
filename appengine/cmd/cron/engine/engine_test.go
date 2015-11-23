// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package engine

import (
	"math/rand"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/taskqueue"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/mathrand"

	"github.com/luci/luci-go/appengine/cmd/cron/catalog"
	"github.com/luci/luci-go/appengine/cmd/cron/messages"
	"github.com/luci/luci-go/appengine/cmd/cron/task"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetAllProjects(t *testing.T) {
	Convey("works", t, func() {
		c := newTestContext(epoch)
		e, _ := newTestEngine()
		ds := datastore.Get(c)

		// Empty.
		projects, err := e.GetAllProjects(c)
		So(err, ShouldBeNil)
		So(len(projects), ShouldEqual, 0)

		// Non empty.
		So(ds.PutMulti([]CronJob{
			{JobID: "abc/1", ProjectID: "abc", Enabled: true},
			{JobID: "abc/2", ProjectID: "abc", Enabled: true},
			{JobID: "def/1", ProjectID: "def", Enabled: true},
			{JobID: "xyz/1", ProjectID: "xyz", Enabled: false},
		}), ShouldBeNil)
		ds.Testable().CatchupIndexes()
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
		So(allJobs(c), ShouldResemble, []CronJob{})

		// Adding a new job (ticks every 5 sec).
		So(e.UpdateProjectJobs(c, "abc", []catalog.Definition{
			{
				JobID:    "abc/1",
				Revision: "rev1",
				Schedule: "*/5 * * * * * *",
			}}), ShouldBeNil)
		So(allJobs(c), ShouldResemble, []CronJob{
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
		taskqueue.Get(c).Testable().ResetTasks()

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
		So(allJobs(c), ShouldResemble, []CronJob{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev2",
				Enabled:   true,
				Schedule:  "*/1 * * * * * *",
				State: JobState{
					State:     "SCHEDULED",
					TickNonce: 9111178027324032851,
					TickTime:  epoch.Add(1 * time.Second),
				},
			},
		})
		// Enqueued timer task to launch it.
		task = ensureOneTask(c, "timers-q")
		So(task.Path, ShouldEqual, "/timers")
		So(task.ETA, ShouldResemble, epoch.Add(1*time.Second))
		taskqueue.Get(c).Testable().ResetTasks()

		// Removed -> goes to disabled state.
		So(e.UpdateProjectJobs(c, "abc", []catalog.Definition{}), ShouldBeNil)
		So(allJobs(c), ShouldResemble, []CronJob{
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
		datastore.Get(c).Testable().SetTransactionRetryCount(2)
		So(e.UpdateProjectJobs(c, "abc", []catalog.Definition{
			{
				JobID:    "abc/1",
				Revision: "rev1",
				Schedule: "*/5 * * * * * *",
			}}), ShouldBeNil)
		So(allJobs(c), ShouldResemble, []CronJob{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev1",
				Enabled:   true,
				Schedule:  "*/5 * * * * * *",
				State: JobState{
					State:     "SCHEDULED",
					TickNonce: 1907242367099883828,
					TickTime:  epoch.Add(5 * time.Second),
				},
			},
		})
		// Enqueued timer task to launch it.
		task := ensureOneTask(c, "timers-q")
		So(task.Path, ShouldEqual, "/timers")
		So(task.ETA, ShouldResemble, epoch.Add(5*time.Second))
		taskqueue.Get(c).Testable().ResetTasks()
	})

	Convey("collision is handled", t, func() {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

		// Pretend collision happened in all retries.
		datastore.Get(c).Testable().SetTransactionRetryCount(15)
		err := e.UpdateProjectJobs(c, "abc", []catalog.Definition{
			{
				JobID:    "abc/1",
				Revision: "rev1",
				Schedule: "*/5 * * * * * *",
			}})
		So(errors.IsTransient(err), ShouldBeTrue)
		So(allJobs(c), ShouldResemble, []CronJob{})
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
		So(allJobs(c), ShouldResemble, []CronJob{
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
		So(allJobs(c), ShouldResemble, []CronJob{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev1",
				Enabled:   true,
				Schedule:  "*/5 * * * * * *",
				State: JobState{
					State:     "SCHEDULED",
					TickNonce: 9111178027324032851,
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
		So(allJobs(c), ShouldResemble, []CronJob{
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
		taskqueue.Get(c).Testable().ResetTasks()

		// Tick time comes, the tick task is executed, job is added to queue.
		clock.Get(c).(testclock.TestClock).Add(5 * time.Second)
		So(e.ExecuteSerializedAction(c, tsk.Payload, 0), ShouldBeNil)

		// Job is in queued state now.
		So(allJobs(c), ShouldResemble, []CronJob{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev1",
				Enabled:   true,
				Schedule:  "*/5 * * * * * *",
				Task:      taskBytes,
				State: JobState{
					State:           "QUEUED",
					TickNonce:       9111178027324032851,
					TickTime:        epoch.Add(10 * time.Second),
					InvocationNonce: 631000787647335445,
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
		taskqueue.Get(c).Testable().ResetTasks()

		// Time to run the job and it fails to launch with a transient error.
		mgr.launchTask = func(ctl task.Controller) error {
			ctl.DebugLog("oops, fail")
			return errors.WrapTransient(errors.New("oops"))
		}
		So(errors.IsTransient(e.ExecuteSerializedAction(c, invTask.Payload, 0)), ShouldBeTrue)

		// Still in QUEUED state, but with InvocatioID assigned.
		jobs := allJobs(c)
		So(jobs, ShouldResemble, []CronJob{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev1",
				Enabled:   true,
				Schedule:  "*/5 * * * * * *",
				Task:      taskBytes,
				State: JobState{
					State:              "QUEUED",
					TickNonce:          9111178027324032851,
					TickTime:           epoch.Add(10 * time.Second),
					InvocationNonce:    631000787647335445,
					InvocationTime:     epoch.Add(5 * time.Second),
					InvocationID:       9200093518582666224,
					InvocationStarting: true,
				},
			},
		})
		jobKey := datastore.Get(c).KeyForObj(&jobs[0])

		// Check Invocation fields.
		inv := Invocation{ID: 9200093518582666224, JobKey: jobKey}
		So(datastore.Get(c).Get(&inv), ShouldBeNil)
		inv.JobKey = nil // for easier ShouldResemble below
		So(inv, ShouldResemble, Invocation{
			ID:              9200093518582666224,
			InvocationNonce: 631000787647335445,
			Revision:        "rev1",
			Started:         epoch.Add(5 * time.Second),
			Finished:        epoch.Add(5 * time.Second),
			Task:            taskBytes,
			DebugLog: "[22:42:05.000] Invocation initiated\n" +
				"[22:42:05.000] oops, fail\n" +
				"[22:42:05.000] Failed to run the task: oops\n" +
				"[22:42:05.000] Invocation finished in 0 with status FAILED\n",
			Status: task.StatusFailed,
		})

		// Second attempt. Now starts, hangs midway, they finishes.
		mgr.launchTask = func(ctl task.Controller) error {
			ctl.DebugLog("Starting")
			So(ctl.Save(task.StatusRunning), ShouldBeNil)

			// After first Save the job and the invocation are in running state.
			So(allJobs(c), ShouldResemble, []CronJob{
				{
					JobID:     "abc/1",
					ProjectID: "abc",
					Revision:  "rev1",
					Enabled:   true,
					Schedule:  "*/5 * * * * * *",
					Task:      taskBytes,
					State: JobState{
						State:           "RUNNING",
						TickNonce:       9111178027324032851,
						TickTime:        epoch.Add(10 * time.Second),
						InvocationNonce: 631000787647335445,
						InvocationTime:  epoch.Add(5 * time.Second),
						InvocationID:    9200093518581789696,
					},
				},
			})
			inv := Invocation{ID: 9200093518581789696, JobKey: jobKey}
			So(datastore.Get(c).Get(&inv), ShouldBeNil)
			inv.JobKey = nil // for easier ShouldResemble below
			So(inv, ShouldResemble, Invocation{
				ID:              9200093518581789696,
				InvocationNonce: 631000787647335445,
				Revision:        "rev1",
				Started:         epoch.Add(5 * time.Second),
				Task:            taskBytes,
				DebugLog:        "[22:42:05.000] Invocation initiated\n[22:42:05.000] Starting\n",
				RetryCount:      1,
				Status:          task.StatusRunning,
			})

			// Noop save, just for code coverage.
			So(ctl.Save(task.StatusRunning), ShouldBeNil)

			return ctl.Save(task.StatusSucceeded)
		}
		So(e.ExecuteSerializedAction(c, invTask.Payload, 1), ShouldBeNil)

		// After final save.
		inv = Invocation{ID: 9200093518581789696, JobKey: jobKey}
		So(datastore.Get(c).Get(&inv), ShouldBeNil)
		inv.JobKey = nil // for easier ShouldResemble below
		So(inv, ShouldResemble, Invocation{
			ID:              9200093518581789696,
			InvocationNonce: 631000787647335445,
			Revision:        "rev1",
			Started:         epoch.Add(5 * time.Second),
			Finished:        epoch.Add(5 * time.Second),
			Task:            taskBytes,
			DebugLog: "[22:42:05.000] Invocation initiated\n" +
				"[22:42:05.000] Starting\n" +
				"[22:42:05.000] Invocation finished in 0 with status SUCCEEDED\n",
			RetryCount: 1,
			Status:     task.StatusSucceeded,
		})

		// Previous invocation is canceled.
		inv = Invocation{ID: 9200093518582666224, JobKey: jobKey}
		So(datastore.Get(c).Get(&inv), ShouldBeNil)
		inv.JobKey = nil // for easier ShouldResemble below
		So(inv, ShouldResemble, Invocation{
			ID:              9200093518582666224,
			InvocationNonce: 631000787647335445,
			Revision:        "rev1",
			Started:         epoch.Add(5 * time.Second),
			Finished:        epoch.Add(5 * time.Second),
			Task:            taskBytes,
			DebugLog: "[22:42:05.000] Invocation initiated\n" +
				"[22:42:05.000] oops, fail\n" +
				"[22:42:05.000] Failed to run the task: oops\n" +
				"[22:42:05.000] Invocation finished in 0 with status FAILED\n",
			Status: task.StatusFailed,
		})

		// Job is in scheduled state again.
		So(allJobs(c), ShouldResemble, []CronJob{
			{
				JobID:     "abc/1",
				ProjectID: "abc",
				Revision:  "rev1",
				Enabled:   true,
				Schedule:  "*/5 * * * * * *",
				Task:      taskBytes,
				State: JobState{
					State:     "SCHEDULED",
					TickNonce: 9111178027324032851,
					TickTime:  epoch.Add(10 * time.Second),
				},
			},
		})
	})
}

func TestGenerateInvocationID(t *testing.T) {
	Convey("generateInvocationID does not collide", t, func() {
		c := newTestContext(epoch)
		k := datastore.Get(c).NewKey("CronJob", "", 123, nil)

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
		k := datastore.Get(c).NewKey("CronJob", "", 123, nil)

		older, err := generateInvocationID(c, k)
		So(err, ShouldBeNil)

		clock.Get(c).(testclock.TestClock).Add(5 * time.Second)

		newer, err := generateInvocationID(c, k)
		So(err, ShouldBeNil)

		So(newer, ShouldBeLessThan, older)
	})
}

////

func newTestContext(now time.Time) context.Context {
	c := memory.Use(context.Background())
	c = clock.Set(c, testclock.New(now))
	c = mathrand.Set(c, rand.New(rand.NewSource(1000)))

	ds := datastore.Get(c)
	ds.Testable().AddIndexes(&datastore.IndexDefinition{
		Kind: "CronJob",
		SortBy: []datastore.IndexColumn{
			{Property: "Enabled"},
			{Property: "ProjectID"},
		},
	})
	ds.Testable().CatchupIndexes()

	tq := taskqueue.Get(c)
	tq.Testable().CreateQueue("timers-q")
	tq.Testable().CreateQueue("invs-q")
	return c
}

func newTestEngine() (Engine, *fakeTaskManager) {
	mgr := &fakeTaskManager{}
	cat := catalog.New(nil)
	cat.RegisterTaskManager(mgr)
	return NewEngine(Config{
		Catalog:              cat,
		TimersQueuePath:      "/timers",
		TimersQueueName:      "timers-q",
		InvocationsQueuePath: "/invs",
		InvocationsQueueName: "invs-q",
	}), mgr
}

////

// fakeTaskManager implement task.Manager interface.
type fakeTaskManager struct {
	launchTask func(ctl task.Controller) error
}

func (m *fakeTaskManager) ProtoMessageType() proto.Message {
	return (*messages.NoopTask)(nil)
}

func (m *fakeTaskManager) ValidateProtoMessage(msg proto.Message) error {
	return nil
}

func (m *fakeTaskManager) LaunchTask(c context.Context, msg proto.Message, ctl task.Controller) error {
	return m.launchTask(ctl)
}

////

func noopTaskBytes() []byte {
	buf, _ := proto.Marshal(&messages.Task{Noop: &messages.NoopTask{}})
	return buf
}

func allJobs(c context.Context) []CronJob {
	ds := datastore.Get(c)
	ds.Testable().CatchupIndexes()
	entities := []CronJob{}
	if err := ds.GetAll(datastore.NewQuery("CronJob"), &entities); err != nil {
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
	tqt := taskqueue.Get(c).Testable()
	tasks := tqt.GetScheduledTasks()[q]
	So(tasks == nil || len(tasks) == 0, ShouldBeTrue)
}

func ensureOneTask(c context.Context, q string) *taskqueue.Task {
	tqt := taskqueue.Get(c).Testable()
	tasks := tqt.GetScheduledTasks()[q]
	So(len(tasks), ShouldEqual, 1)
	for _, t := range tasks {
		return t
	}
	return nil
}
