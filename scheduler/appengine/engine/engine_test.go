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
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/pubsub/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoiface"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/taskqueue"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine/cron"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/schedule"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/noop"
)

func TestGetAllProjects(t *testing.T) {
	t.Parallel()

	ftt.Run("works", t, func(t *ftt.Test) {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

		// Empty.
		projects, err := e.GetAllProjects(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(projects), should.BeZero)

		// Non empty.
		assert.Loosely(t, datastore.Put(c,
			&Job{JobID: "abc/1", ProjectID: "abc", Enabled: true},
			&Job{JobID: "abc/2", ProjectID: "abc", Enabled: true},
			&Job{JobID: "def/1", ProjectID: "def", Enabled: true},
			&Job{JobID: "xyz/1", ProjectID: "xyz", Enabled: false},
		), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		projects, err = e.GetAllProjects(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, projects, should.Resemble([]string{"abc", "def"}))
	})
}

func TestUpdateProjectJobs(t *testing.T) {
	t.Parallel()

	ftt.Run("with test context", t, func(t *ftt.Test) {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		jobDef := catalog.Definition{
			JobID:            "proj/1",
			RealmID:          "proj:testing",
			Revision:         "rev1",
			Schedule:         "*/5 * * * * * *",
			Task:             []uint8{1, 2, 3}, // note: this is actually gibberish, but we don't care here
			TriggeringPolicy: []uint8{4, 5, 6}, // same
		}

		t.Run("noop", func(t *ftt.Test) {
			assert.Loosely(t, e.UpdateProjectJobs(c, "proj", nil), should.BeNil)
			assert.Loosely(t, allJobs(c), should.Resemble([]Job{}))
		})

		t.Run("adding a job", func(t *ftt.Test) {
			// Adding a job that ticks each 5 sec.
			assert.Loosely(t, e.UpdateProjectJobs(c, "proj", []catalog.Definition{jobDef}), should.BeNil)

			// Added.
			assert.Loosely(t, allJobs(c), should.Resemble([]Job{
				{
					JobID:     "proj/1",
					ProjectID: "proj",
					RealmID:   "proj:testing",
					Revision:  "rev1",
					Enabled:   true,
					Schedule:  "*/5 * * * * * *",
					Cron: cron.State{
						Enabled:    true,
						Generation: 2,
						LastRewind: epoch,
						LastTick: cron.TickLaterAction{
							When:      epoch.Add(5 * time.Second),
							TickNonce: 6278013164014963328,
						},
					},
					Task:                jobDef.Task,
					TriggeringPolicyRaw: jobDef.TriggeringPolicy,
				},
			}))

			// The first tick is scheduled.
			assert.Loosely(t, tq.GetScheduledTasks().Payloads(), should.Resemble([]protoiface.MessageV1{
				&internal.CronTickTask{JobId: "proj/1", TickNonce: 6278013164014963328},
			}))

			// Again, should be noop.
			assert.Loosely(t, e.UpdateProjectJobs(c, "proj", []catalog.Definition{jobDef}), should.BeNil)
			assert.Loosely(t, len(tq.GetScheduledTasks()), should.Equal(1)) // no new tasks
		})

		t.Run("adding a very infrequent job", func(t *ftt.Test) {
			jobDef.Schedule = "59 23 31 12 *" // every Dec 31st.
			nextTick := time.Date(epoch.Year(), time.December, 31, 23, 59, 00, 0, time.UTC)
			assert.Loosely(t, nextTick.Sub(epoch), should.BeGreaterThan(30*24*time.Hour))

			assert.Loosely(t, e.UpdateProjectJobs(c, "proj", []catalog.Definition{jobDef}), should.BeNil)
			// Added.
			assert.Loosely(t, allJobs(c), should.Resemble([]Job{
				{
					JobID:     "proj/1",
					ProjectID: "proj",
					RealmID:   "proj:testing",
					Revision:  "rev1",
					Enabled:   true,
					Schedule:  "59 23 31 12 *",
					Cron: cron.State{
						Enabled:    true,
						Generation: 2,
						LastRewind: epoch,
						LastTick: cron.TickLaterAction{
							When:      nextTick,
							TickNonce: 6278013164014963328,
						},
					},
					Task:                jobDef.Task,
					TriggeringPolicyRaw: jobDef.TriggeringPolicy,
				},
			}))

			// The first tick is scheduled with ETA substantially before actual next
			// tick.
			assert.Loosely(t, tq.GetScheduledTasks().Payloads(), should.Resemble([]protoiface.MessageV1{
				&internal.CronTickTask{JobId: "proj/1", TickNonce: 6278013164014963328},
			}))
			assert.Loosely(t, tq.GetScheduledTasks()[0].Task.Delay, should.BeLessThan(30*24*time.Hour))
		})

		t.Run("adding a job with txn retry", func(t *ftt.Test) {
			// Simulate the transaction retry.
			datastore.GetTestable(c).SetTransactionRetryCount(2)
			// Add a job.
			assert.Loosely(t, e.UpdateProjectJobs(c, "proj", []catalog.Definition{jobDef}), should.BeNil)
			// Added only one task, even though we had 2 retries.
			assert.Loosely(t, len(tq.GetScheduledTasks()), should.Equal(1))
		})

		t.Run("adding a job with txn collision", func(t *ftt.Test) {
			// Simulate the transaction refusing to land even after many tries.
			datastore.GetTestable(c).SetTransactionRetryCount(15)
			// Attempt to add a job.
			err := e.UpdateProjectJobs(c, "proj", []catalog.Definition{jobDef})
			// Failed transiently, nothing in the datastore or in TQ.
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
			assert.Loosely(t, allJobs(c), should.Resemble([]Job{}))
			assert.Loosely(t, len(tq.GetScheduledTasks()), should.BeZero)
		})

		t.Run("updating job's schedule", func(t *ftt.Test) {
			// Adding a job that ticks every 5 sec. Make sure its tick is scheduled.
			assert.Loosely(t, e.UpdateProjectJobs(c, "proj", []catalog.Definition{jobDef}), should.BeNil)
			assert.Loosely(t, tq.GetScheduledTasks().Payloads(), should.Resemble([]protoiface.MessageV1{
				&internal.CronTickTask{JobId: "proj/1", TickNonce: 6278013164014963328},
			}))

			// Changing it to tick every 30 sec.
			newDef := jobDef
			newDef.Schedule = "*/30 * * * * * *"
			assert.Loosely(t, e.UpdateProjectJobs(c, "proj", []catalog.Definition{newDef}), should.BeNil)

			// The job is updated now.
			assert.Loosely(t, allJobs(c), should.Resemble([]Job{
				{
					JobID:     "proj/1",
					ProjectID: "proj",
					RealmID:   "proj:testing",
					Revision:  "rev1",
					Enabled:   true,
					Schedule:  "*/30 * * * * * *",
					Cron: cron.State{
						Enabled:    true,
						Generation: 3,
						LastRewind: epoch,
						LastTick: cron.TickLaterAction{
							When:      epoch.Add(30 * time.Second), // new tick time
							TickNonce: 2673062197574995716,
						},
					},
					Task:                jobDef.Task,
					TriggeringPolicyRaw: jobDef.TriggeringPolicy,
				},
			}))

			// The new tick is scheduled now too.
			assert.Loosely(t, tq.GetScheduledTasks().Payloads(), should.Resemble([]protoiface.MessageV1{
				&internal.CronTickTask{JobId: "proj/1", TickNonce: 6278013164014963328},
				&internal.CronTickTask{JobId: "proj/1", TickNonce: 2673062197574995716},
			}))
		})

		t.Run("updating job's triggering policy", func(t *ftt.Test) {
			// Adding a job that is triggered externally (doesn't tick). Schedules no
			// tasks.
			job := jobDef
			job.Schedule = "triggered"
			assert.Loosely(t, e.UpdateProjectJobs(c, "proj", []catalog.Definition{job}), should.BeNil)
			assert.Loosely(t, tq.GetScheduledTasks().Payloads(), should.HaveLength(0))

			// Update its triggering policy. It should emit a triage to evaluate
			// the state of the job using this new policy.
			job.TriggeringPolicy = []uint8{1, 1, 1, 1}
			assert.Loosely(t, e.UpdateProjectJobs(c, "proj", []catalog.Definition{job}), should.BeNil)

			// The job is updated now.
			assert.Loosely(t, allJobs(c), should.Resemble([]Job{
				{
					JobID:     "proj/1",
					ProjectID: "proj",
					RealmID:   "proj:testing",
					Revision:  "rev1",
					Enabled:   true,
					Schedule:  "triggered",
					Cron: cron.State{
						Enabled:    true,
						Generation: 2,
						LastRewind: epoch,
						LastTick: cron.TickLaterAction{
							When:      schedule.DistantFuture,
							TickNonce: 6278013164014963328,
						},
					},
					Task:                jobDef.Task,
					TriggeringPolicyRaw: job.TriggeringPolicy,
				},
			}))

			// Kicked the triage indeed.
			assert.Loosely(t, tq.GetScheduledTasks().Payloads(), should.Resemble([]protoiface.MessageV1{
				&internal.KickTriageTask{JobId: "proj/1"},
			}))
		})

		t.Run("removing a job", func(t *ftt.Test) {
			// Adding a job first.
			assert.Loosely(t, e.UpdateProjectJobs(c, "proj", []catalog.Definition{jobDef}), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// And now removing it.
			assert.Loosely(t, e.UpdateProjectJobs(c, "proj", nil), should.BeNil)

			// Switched to disabled state.
			assert.Loosely(t, allJobs(c), should.Resemble([]Job{
				{
					JobID:     "proj/1",
					ProjectID: "proj",
					RealmID:   "proj:testing",
					Revision:  "rev1",
					Enabled:   false,
					Schedule:  "*/5 * * * * * *",
					Cron: cron.State{
						Enabled:    false,
						Generation: 3,
					},
					Task:                jobDef.Task,
					TriggeringPolicyRaw: jobDef.TriggeringPolicy,
				},
			}))
		})
	})
}

func TestGenerateInvocationID(t *testing.T) {
	t.Parallel()

	ftt.Run("generateInvocationID does not collide", t, func(t *ftt.Test) {
		c := newTestContext(epoch)

		// Bunch of ids generated at the exact same moment in time do not collide.
		ids := map[int64]struct{}{}
		for i := 0; i < 20; i++ {
			id, err := generateInvocationID(c)
			assert.Loosely(t, err, should.BeNil)
			ids[id] = struct{}{}
		}
		assert.Loosely(t, len(ids), should.Equal(20))
	})

	ftt.Run("generateInvocationID gen IDs with most recent first", t, func(t *ftt.Test) {
		c := newTestContext(epoch)

		older, err := generateInvocationID(c)
		assert.Loosely(t, err, should.BeNil)

		clock.Get(c).(testclock.TestClock).Add(5 * time.Second)

		newer, err := generateInvocationID(c)
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, newer, should.BeLessThan(older))
	})
}

func TestQueries(t *testing.T) {
	t.Parallel()

	ftt.Run("with mock data", t, func(t *ftt.Test) {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

		db := authtest.NewFakeDB(
			authtest.MockMembership("user:admin@example.com", adminGroup),

			authtest.MockPermission("user:one@example.com", "abc:one", PermJobsGet),
			authtest.MockPermission("user:some@example.com", "abc:some", PermJobsGet),

			authtest.MockPermission("anonymous:anonymous", "abc:public", PermJobsGet),
			authtest.MockPermission("user:one@example.com", "abc:public", PermJobsGet),
			authtest.MockPermission("user:some@example.com", "abc:public", PermJobsGet),
			authtest.MockPermission("user:admin@example.com", "abc:public", PermJobsGet),

			authtest.MockPermission(
				"user:some@example.com",
				"abc:secret",
				PermJobsGet,
				authtest.RestrictAttribute("scheduler.job.name", "restricted"),
			),
		)

		ctxAnon := auth.WithState(c, &authtest.FakeState{
			Identity: "anonymous:anonymous",
			FakeDB:   db,
		})
		ctxOne := auth.WithState(c, &authtest.FakeState{
			Identity: "user:one@example.com",
			FakeDB:   db,
		})
		ctxSome := auth.WithState(c, &authtest.FakeState{
			Identity: "user:some@example.com",
			FakeDB:   db,
		})
		ctxAdmin := auth.WithState(c, &authtest.FakeState{
			Identity: "user:admin@example.com",
			FakeDB:   db,
		})

		job1 := "abc/1"
		job2 := "abc/2"
		job3 := "abc/3"

		assert.Loosely(t, datastore.Put(c,
			&Job{JobID: job1, ProjectID: "abc", Enabled: true, RealmID: "abc:one"},
			&Job{JobID: job2, ProjectID: "abc", Enabled: true, RealmID: "abc:some"},
			&Job{JobID: job3, ProjectID: "abc", Enabled: true, RealmID: "abc:public"},
			&Job{JobID: "def/1", ProjectID: "def", Enabled: true, RealmID: "abc:public"},
			&Job{JobID: "def/2", ProjectID: "def", Enabled: false, RealmID: "abc:public"},
			&Job{JobID: "secret/admin-only", ProjectID: "secret", Enabled: true, RealmID: "abc:secret"},
			&Job{JobID: "secret/restricted", ProjectID: "secret", Enabled: true, RealmID: "abc:secret"},
		), should.BeNil)

		// Mocked invocations, all in finished state (IndexedJobID set).
		assert.Loosely(t, datastore.Put(c,
			&Invocation{ID: 1, JobID: job1, IndexedJobID: job1},
			&Invocation{ID: 2, JobID: job1, IndexedJobID: job1},
			&Invocation{ID: 3, JobID: job1, IndexedJobID: job1},
			&Invocation{ID: 4, JobID: job2, IndexedJobID: job2},
			&Invocation{ID: 5, JobID: job2, IndexedJobID: job2},
			&Invocation{ID: 6, JobID: job2, IndexedJobID: job2},
			&Invocation{ID: 7, JobID: job3, IndexedJobID: job3},
		), should.BeNil)

		datastore.GetTestable(c).CatchupIndexes()

		t.Run("GetAllProjects ignores ACLs and CurrentIdentity", func(t *ftt.Test) {
			test := func(ctx context.Context) {
				r, err := e.GetAllProjects(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, r, should.Resemble([]string{"abc", "def", "secret"}))
			}
			test(c)
			test(ctxAnon)
			test(ctxAdmin)
		})

		t.Run("GetVisibleJobs works", func(t *ftt.Test) {
			get := func(ctx context.Context) []string {
				jobs, err := e.GetVisibleJobs(ctx)
				assert.Loosely(t, err, should.BeNil)
				return sortedJobIds(jobs)
			}

			t.Run("Anonymous users see only public jobs", func(t *ftt.Test) {
				// Only 3 jobs with default ACLs granting read access to everyone, but
				// def/2 is disabled and so shouldn't be returned.
				assert.Loosely(t, get(ctxAnon), should.Resemble([]string{"abc/3", "def/1"}))
			})
			t.Run("Owners can see their own jobs + public jobs", func(t *ftt.Test) {
				// abc/1 is owned by one@example.com.
				assert.Loosely(t, get(ctxOne), should.Resemble([]string{"abc/1", "abc/3", "def/1"}))
			})
			t.Run("Explicit readers", func(t *ftt.Test) {
				assert.Loosely(t, get(ctxSome), should.Resemble([]string{
					"abc/2",
					"abc/3",
					"def/1",
					"secret/restricted", // via the conditional permission
				}))
			})
			t.Run("Admins have implicit read access to all jobs", func(t *ftt.Test) {
				assert.Loosely(t, get(ctxAdmin), should.Resemble([]string{
					"abc/1",
					"abc/2",
					"abc/3",
					"def/1",
					"secret/admin-only",
					"secret/restricted",
				}))
			})
		})

		t.Run("GetProjectJobsRA works", func(t *ftt.Test) {
			get := func(ctx context.Context, project string) []string {
				jobs, err := e.GetVisibleProjectJobs(ctx, project)
				assert.Loosely(t, err, should.BeNil)
				return sortedJobIds(jobs)
			}
			t.Run("Anonymous can still see public jobs", func(t *ftt.Test) {
				assert.Loosely(t, get(ctxAnon, "def"), should.Resemble([]string{"def/1"}))
			})
			t.Run("Admin have implicit read access to all jobs", func(t *ftt.Test) {
				assert.Loosely(t, get(ctxAdmin, "abc"), should.Resemble([]string{"abc/1", "abc/2", "abc/3"}))
			})
			t.Run("Owners can still see their jobs", func(t *ftt.Test) {
				assert.Loosely(t, get(ctxOne, "abc"), should.Resemble([]string{"abc/1", "abc/3"}))
			})
			t.Run("Readers can see their jobs", func(t *ftt.Test) {
				assert.Loosely(t, get(ctxSome, "abc"), should.Resemble([]string{"abc/2", "abc/3"}))
			})
		})

		t.Run("GetVisibleJob works", func(t *ftt.Test) {
			_, err := e.GetVisibleJob(ctxAdmin, "missing/job")
			assert.Loosely(t, err, should.Equal(ErrNoSuchJob))

			_, err = e.GetVisibleJob(ctxAnon, "abc/1") // no "scheduler.jobs.get" permission.
			assert.Loosely(t, err, should.Equal(ErrNoSuchJob))

			_, err = e.GetVisibleJob(ctxAnon, "def/2") // not enabled, hence not visible.
			assert.Loosely(t, err, should.Equal(ErrNoSuchJob))

			job, err := e.GetVisibleJob(ctxAnon, "def/1") // OK.
			assert.Loosely(t, job, should.NotBeNil)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("ListInvocations works", func(t *ftt.Test) {
			job, err := e.GetVisibleJob(ctxOne, "abc/1")
			assert.Loosely(t, err, should.BeNil)

			t.Run("With paging", func(t *ftt.Test) {
				invs, cursor, err := e.ListInvocations(ctxOne, job, ListInvocationsOpts{
					PageSize: 2,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(invs), should.Equal(2))
				assert.Loosely(t, invs[0].ID, should.Equal(1))
				assert.Loosely(t, invs[1].ID, should.Equal(2))
				assert.Loosely(t, cursor, should.NotEqual(""))

				invs, cursor, err = e.ListInvocations(ctxOne, job, ListInvocationsOpts{
					PageSize: 2,
					Cursor:   cursor,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(invs), should.Equal(1))
				assert.Loosely(t, invs[0].ID, should.Equal(3))
				assert.Loosely(t, cursor, should.BeEmpty)
			})
		})

		t.Run("GetInvocation works", func(t *ftt.Test) {
			job, err := e.GetVisibleJob(ctxOne, "abc/1")
			assert.Loosely(t, err, should.BeNil)

			t.Run("NoSuchInvocation", func(t *ftt.Test) {
				_, err = e.GetInvocation(ctxOne, job, 666) // Missing invocation.
				assert.Loosely(t, err, should.ErrLike(ErrNoSuchInvocation))
			})

			t.Run("Existing invocation", func(t *ftt.Test) {
				inv, err := e.GetInvocation(ctxOne, job, 1)
				assert.Loosely(t, inv, should.NotBeNil)
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}

func TestPrepareTopic(t *testing.T) {
	t.Parallel()

	ftt.Run("PrepareTopic works", t, func(ctx *ftt.Test) {
		c := newTestContext(epoch)

		e, _ := newTestEngine()

		pubSubCalls := 0
		e.configureTopic = func(c context.Context, topic, sub, pushURL, publisher string) error {
			pubSubCalls++
			assert.Loosely(ctx, topic, should.Equal(fmt.Sprintf("projects/%s/topics/dev-scheduler.noop.some~publisher.com", fakeAppID)))
			assert.Loosely(ctx, sub, should.Equal(fmt.Sprintf("projects/%s/subscriptions/dev-scheduler.noop.some~publisher.com", fakeAppID)))
			assert.Loosely(ctx, pushURL, should.BeEmpty) // pull on dev server
			assert.Loosely(ctx, publisher, should.Equal("some@publisher.com"))
			return nil
		}

		ctl := &taskController{
			ctx:     c,
			eng:     e,
			manager: &noop.TaskManager{},
			saved:   Invocation{ID: 123456},
		}
		ctl.populateState()

		// Once.
		topic, token, err := ctl.PrepareTopic(c, "some@publisher.com")
		assert.Loosely(ctx, err, should.BeNil)
		assert.Loosely(ctx, topic, should.Equal(fmt.Sprintf("projects/%s/topics/dev-scheduler.noop.some~publisher.com", fakeAppID)))
		assert.Loosely(ctx, token, should.NotEqual(""))
		assert.Loosely(ctx, pubSubCalls, should.Equal(1))

		// Again. 'configureTopic' should not be called anymore.
		_, _, err = ctl.PrepareTopic(c, "some@publisher.com")
		assert.Loosely(ctx, err, should.BeNil)
		assert.Loosely(ctx, pubSubCalls, should.Equal(1))
	})
}

func TestProcessPubSubPush(t *testing.T) {
	t.Parallel()

	ftt.Run("with mock invocation", t, func(t *ftt.Test) {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		tc := clock.Get(c).(testclock.TestClock)
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			tc.Add(d)
		})

		assert.Loosely(t, datastore.Put(c, &Job{
			JobID:     "abc/1",
			ProjectID: "abc",
			Enabled:   true,
		}), should.BeNil)

		task, err := proto.Marshal(&messages.TaskDefWrapper{
			Noop: &messages.NoopTask{},
		})
		assert.Loosely(t, err, should.BeNil)

		inv := Invocation{
			ID:    1,
			JobID: "abc/1",
			Task:  task,
		}
		assert.Loosely(t, datastore.Put(c, &inv), should.BeNil)

		// Skip talking to PubSub for real.
		e.configureTopic = func(c context.Context, topic, sub, pushURL, publisher string) error {
			return nil
		}

		ctl, err := controllerForInvocation(c, e, &inv)
		assert.Loosely(t, err, should.BeNil)

		// Grab the working auth token.
		_, token, err := ctl.PrepareTopic(c, "some@publisher.com")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, token, should.NotEqual(""))

		prepMessage := func(body, token string) []byte {
			msg := struct {
				Message pubsub.PubsubMessage `json:"message"`
			}{
				Message: pubsub.PubsubMessage{
					Attributes: map[string]string{"auth_token": token},
					Data:       body,
				},
			}
			blob, _ := json.Marshal(&msg)
			return blob
		}

		// Prep url.Values the same way PrepareTopic does.
		urlValues := e.pushSubscriptionURLValues(ctl.manager, "some@publisher.com")

		// Extract the token from attributes where we put it below.
		mgr.examineNotification = func(_ context.Context, msg *pubsub.PubsubMessage) string {
			return msg.Attributes["auth_token"]
		}

		t.Run("ProcessPubSubPush works and retries tq.Retry errors", func(t *ftt.Test) {
			calls := 0
			mgr.handleNotification = func(ctx context.Context, msg *pubsub.PubsubMessage) error {
				assert.Loosely(t, msg.Data, should.Equal("blah"))
				calls++
				if calls == 1 {
					return errors.New("should be retried", tq.Retry)
				}
				return nil
			}
			assert.Loosely(t, e.ProcessPubSubPush(c, prepMessage("blah", token), urlValues), should.BeNil)
			assert.Loosely(t, calls, should.Equal(2)) // executed the retry
		})

		t.Run("ProcessPubSubPush handles bad token", func(t *ftt.Test) {
			err := e.ProcessPubSubPush(c, prepMessage("blah", token+"blah"), urlValues)
			assert.Loosely(t, err, should.ErrLike("bad token"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run("ProcessPubSubPush handles missing invocation", func(t *ftt.Test) {
			datastore.Delete(c, datastore.KeyForObj(c, &inv))

			err := e.ProcessPubSubPush(c, prepMessage("blah", token), urlValues)
			assert.Loosely(t, err, should.ErrLike("doesn't exist"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run("ProcessPubSubPush handles unknown task manager", func(t *ftt.Test) {
			// Pass `nil` instead of urlValue, so that the engine can't figure out
			// what task manager to use.
			err := e.ProcessPubSubPush(c, prepMessage("blah", token), nil)
			assert.Loosely(t, err, should.ErrLike("unknown task manager"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run("ProcessPubSubPush can't find the auth token", func(t *ftt.Test) {
			mgr.examineNotification = func(context.Context, *pubsub.PubsubMessage) string {
				return ""
			}
			err := e.ProcessPubSubPush(c, prepMessage("blah", token), urlValues)
			assert.Loosely(t, err, should.ErrLike("failed to extract"))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
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

	ftt.Run("small log is not trimmed", t, func(t *ftt.Test) {
		inv := Invocation{
			DebugLog: genLines(0, 100),
		}
		inv.trimDebugLog()
		assert.Loosely(t, inv.DebugLog, should.Equal(genLines(0, 100)))
	})

	ftt.Run("huge log is trimmed", t, func(t *ftt.Test) {
		inv := Invocation{
			DebugLog: genLines(0, 500),
		}
		inv.trimDebugLog()
		assert.Loosely(t, inv.DebugLog, should.Equal(
			genLines(0, 94)+"--- the log has been cut here ---\n"+genLines(400, 500)))
	})

	ftt.Run("writing lines to huge log and trimming", t, func(t *ftt.Test) {
		inv := Invocation{
			DebugLog: genLines(0, 500),
		}
		inv.trimDebugLog()
		for i := 0; i < 10; i++ {
			inv.debugLog(ctx, "Line %d - %s", i, junk)
			inv.trimDebugLog()
		}
		// Still single cut only. New 10 lines are at the end.
		assert.Loosely(t, inv.DebugLog, should.Equal(
			genLines(0, 94)+"--- the log has been cut here ---\n"+genLines(410, 500)+genLines(0, 10)))
	})

	ftt.Run("one huge line", t, func(t *ftt.Test) {
		inv := Invocation{
			DebugLog: strings.Repeat("z", 300000),
		}
		inv.trimDebugLog()
		const msg = "\n--- the log has been cut here ---\n"
		assert.Loosely(t, inv.DebugLog, should.Equal(strings.Repeat("z", debugLogSizeLimit-len(msg))+msg))
	})
}

func TestEnqueueInvocations(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		job := Job{JobID: "project/job"}
		assert.Loosely(t, datastore.Put(c, &job), should.BeNil)

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
		assert.Loosely(t, err, should.BeNil)

		// The order of new invocations is undefined (including IDs assigned to
		// them), so convert them to map and clear IDs.
		invsByTrigger := map[identity.Identity]Invocation{}
		invIDs := map[int64]bool{}
		for _, inv := range invs {
			invIDs[inv.ID] = true
			cpy := *inv
			cpy.ID = 0
			invsByTrigger[inv.TriggeredBy] = cpy
		}
		assert.Loosely(t, invsByTrigger, should.Resemble(map[identity.Identity]Invocation{
			"user:a@example.com": {
				JobID:       "project/job",
				Started:     epoch,
				TriggeredBy: "user:a@example.com",
				Status:      task.StatusStarting,
				DebugLog: "[22:42:00.000] New invocation is queued and will start shortly\n" +
					"[22:42:00.000] Triggered by user:a@example.com\n",
			},
			"user:b@example.com": {
				JobID:       "project/job",
				Started:     epoch,
				TriggeredBy: "user:b@example.com",
				Status:      task.StatusStarting,
				DebugLog: "[22:42:00.000] New invocation is queued and will start shortly\n" +
					"[22:42:00.000] Triggered by user:b@example.com\n",
			},
		}))

		// Both invocations are in ActiveInvocations list of the job.
		assert.Loosely(t, len(job.ActiveInvocations), should.Equal(2))
		for _, invID := range job.ActiveInvocations {
			assert.Loosely(t, invIDs[invID], should.BeTrue)
		}

		// And we've emitted the launch task.
		tasks := tq.GetScheduledTasks()
		assert.Loosely(t, tasks[0].Payload, should.HaveType[*internal.LaunchInvocationsBatchTask])
		batch := tasks[0].Payload.(*internal.LaunchInvocationsBatchTask)
		assert.Loosely(t, len(batch.Tasks), should.Equal(2))
		for _, subtask := range batch.Tasks {
			assert.Loosely(t, subtask.JobId, should.Equal("project/job"))
			assert.Loosely(t, invIDs[subtask.InvId], should.BeTrue)
		}
	})
}

func TestTriageTaskDedup(t *testing.T) {
	t.Parallel()

	ftt.Run("with fake env", t, func(t *ftt.Test) {
		c := newTestContext(epoch)
		e, _ := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		t.Run("single task", func(t *ftt.Test) {
			assert.Loosely(t, e.kickTriageNow(c, "fake/job"), should.BeNil)

			tasks := tq.GetScheduledTasks()
			assert.Loosely(t, len(tasks), should.Equal(1))
			assert.Loosely(t, tasks[0].Task.ETA.Equal(epoch.Add(2*time.Second)), should.BeTrue)
			assert.Loosely(t, tasks[0].Payload, should.Resemble(&internal.TriageJobStateTask{JobId: "fake/job"}))
		})

		t.Run("a bunch of tasks, deduplicated by hitting memcache", func(t *ftt.Test) {
			assert.Loosely(t, e.kickTriageNow(c, "fake/job"), should.BeNil)

			clock.Get(c).(testclock.TestClock).Add(time.Second)
			assert.Loosely(t, e.kickTriageNow(c, "fake/job"), should.BeNil)

			clock.Get(c).(testclock.TestClock).Add(900 * time.Millisecond)
			assert.Loosely(t, e.kickTriageNow(c, "fake/job"), should.BeNil)

			tasks := tq.GetScheduledTasks()
			assert.Loosely(t, len(tasks), should.Equal(1))
			assert.Loosely(t, tasks[0].Task.ETA.Equal(epoch.Add(2*time.Second)), should.BeTrue)
			assert.Loosely(t, tasks[0].Payload, should.Resemble(&internal.TriageJobStateTask{JobId: "fake/job"}))
		})

		t.Run("a bunch of tasks, deduplicated by hitting task queue", func(t *ftt.Test) {
			c, fb := featureBreaker.FilterMC(c, fmt.Errorf("omg, memcache error"))
			fb.BreakFeatures(nil, "GetMulti", "SetMulti")

			assert.Loosely(t, e.kickTriageNow(c, "fake/job"), should.BeNil)

			clock.Get(c).(testclock.TestClock).Add(time.Second)
			assert.Loosely(t, e.kickTriageNow(c, "fake/job"), should.BeNil)

			clock.Get(c).(testclock.TestClock).Add(900 * time.Millisecond)
			assert.Loosely(t, e.kickTriageNow(c, "fake/job"), should.BeNil)

			tasks := tq.GetScheduledTasks()
			assert.Loosely(t, len(tasks), should.Equal(1))
			assert.Loosely(t, tasks[0].Task.ETA.Equal(epoch.Add(2*time.Second)), should.BeTrue)
			assert.Loosely(t, tasks[0].Payload, should.Resemble(&internal.TriageJobStateTask{JobId: "fake/job"}))
		})
	})
}

func TestLaunchInvocationTask(t *testing.T) {
	t.Parallel()

	ftt.Run("with fake env", t, func(t *ftt.Test) {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		// Add the job.
		assert.Loosely(t, e.UpdateProjectJobs(c, "project", []catalog.Definition{
			{
				JobID:    "project/job",
				RealmID:  "project:testing",
				Revision: "rev1",
				Schedule: "triggered",
				Task:     noopTaskBytes(),
			},
		}), should.BeNil)

		// Prepare Invocation in Starting state.
		job := Job{JobID: "project/job"}
		assert.Loosely(t, datastore.Get(c, &job), should.BeNil)
		inv, err := e.allocateInvocation(c, &job, task.Request{
			IncomingTriggers: []*internal.Trigger{{Id: "a"}},
		})
		assert.Loosely(t, err, should.BeNil)

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
			assert.Loosely(t, datastore.Get(c, &toFetch), should.BeNil)
			return &toFetch
		}

		t.Run("happy path", func(t *ftt.Test) {
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				assert.Loosely(t, ctl.InvocationID(), should.Equal(inv.ID))
				assert.Loosely(t, ctl.RealmID(), should.Equal("project:testing"))
				ctl.DebugLog("Succeeded!")
				ctl.State().Status = task.StatusSucceeded
				return nil
			}
			assert.Loosely(t, callLaunchInvocation(c, 0), should.BeNil)

			updated := fetchInvocation(c)
			triggers, err := updated.IncomingTriggers()
			updated.IncomingTriggersRaw = nil

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, triggers, should.Resemble([]*internal.Trigger{{Id: "a"}}))
			assert.Loosely(t, updated, should.Resemble(&Invocation{
				ID:             inv.ID,
				JobID:          "project/job",
				IndexedJobID:   "project/job",
				RealmID:        "project:testing",
				Started:        epoch,
				Finished:       epoch,
				Revision:       job.Revision,
				Task:           job.Task,
				Status:         task.StatusSucceeded,
				MutationsCount: 2,
				DebugLog: "[22:42:00.000] New invocation is queued and will start shortly\n" +
					"[22:42:00.000] Starting the invocation (attempt 1)\n" +
					"[22:42:00.000] Succeeded!\n" +
					"[22:42:00.000] Invocation finished in 0s with status SUCCEEDED\n",
			}))
		})

		t.Run("already aborted", func(t *ftt.Test) {
			inv.Status = task.StatusAborted
			assert.Loosely(t, datastore.Put(c, inv), should.BeNil)
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				return fmt.Errorf("must not be called")
			}
			assert.Loosely(t, callLaunchInvocation(c, 0), should.BeNil)
			assert.Loosely(t, fetchInvocation(c).Status, should.Equal(task.StatusAborted))
		})

		t.Run("successful retry", func(t *ftt.Test) {
			// Attempt #1.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				return transient.Tag.Apply(fmt.Errorf("oops, failed to start"))
			}
			assert.Loosely(t, callLaunchInvocation(c, 0), should.Equal(errRetryingLaunch))
			assert.Loosely(t, fetchInvocation(c).Status, should.Equal(task.StatusRetrying))

			// Attempt #2.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				ctl.DebugLog("Succeeded!")
				ctl.State().Status = task.StatusSucceeded
				return nil
			}
			assert.Loosely(t, callLaunchInvocation(c, 1), should.BeNil)

			updated := fetchInvocation(c)
			assert.Loosely(t, updated.Status, should.Equal(task.StatusSucceeded))
			assert.Loosely(t, updated.RetryCount, should.Equal(1))
			assert.Loosely(t, updated.DebugLog, should.Equal("[22:42:00.000] New invocation is queued and will start shortly\n"+
				"[22:42:00.000] Starting the invocation (attempt 1)\n"+
				"[22:42:00.000] The invocation will be retried\n"+
				"[22:42:00.000] Starting the invocation (attempt 2)\n"+
				"[22:42:00.000] Succeeded!\n"+
				"[22:42:00.000] Invocation finished in 0s with status SUCCEEDED\n"))
		})

		t.Run("failed retry", func(t *ftt.Test) {
			// Attempt #1.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				return transient.Tag.Apply(fmt.Errorf("oops, failed to start"))
			}
			assert.Loosely(t, callLaunchInvocation(c, 0), should.Equal(errRetryingLaunch))
			assert.Loosely(t, fetchInvocation(c).Status, should.Equal(task.StatusRetrying))

			// Attempt #2.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				return fmt.Errorf("boom, fatal shot")
			}
			assert.Loosely(t, callLaunchInvocation(c, 1), should.BeNil) // didn't ask for a retry

			updated := fetchInvocation(c)
			assert.Loosely(t, updated.Status, should.Equal(task.StatusFailed))
			assert.Loosely(t, updated.RetryCount, should.Equal(1))
			assert.Loosely(t, updated.DebugLog, should.Equal("[22:42:00.000] New invocation is queued and will start shortly\n"+
				"[22:42:00.000] Starting the invocation (attempt 1)\n"+
				"[22:42:00.000] The invocation will be retried\n"+
				"[22:42:00.000] Starting the invocation (attempt 2)\n"+
				"[22:42:00.000] Invocation finished in 0s with status FAILED\n"))
		})
	})
}

func TestAbortJob(t *testing.T) {
	t.Parallel()

	ftt.Run("with fake env", t, func(t *ftt.Test) {
		const jobID = "project/job"
		const realmID = "project:testing"
		const expectedInvID int64 = 9200093523825193008

		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		assert.Loosely(t, e.UpdateProjectJobs(c, "project", []catalog.Definition{
			{
				JobID:    jobID,
				RealmID:  realmID,
				Revision: "rev1",
				Schedule: "triggered",
				Task:     noopTaskBytes(),
			},
		}), should.BeNil)

		// Launch a new invocation.
		job, err := e.getJob(c, jobID)
		assert.Loosely(t, err, should.BeNil)
		inv := forceInvocation(c, e, jobID)
		invID := inv.ID
		assert.Loosely(t, invID, should.Equal(expectedInvID))

		t.Run("inv aborted before it starts", func(t *ftt.Test) {
			// Kill it right away before it had a chance to start.
			assert.Loosely(t, e.AbortJob(mockOwnerCtx(c, realmID), job), should.BeNil)

			// It is dead right away.
			inv, err := e.getInvocation(c, jobID, invID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv, should.Resemble(&Invocation{
				ID:             expectedInvID,
				JobID:          jobID,
				RealmID:        realmID,
				IndexedJobID:   jobID,
				Started:        epoch,
				Finished:       epoch,
				Revision:       "rev1",
				Task:           noopTaskBytes(),
				Status:         task.StatusAborted,
				MutationsCount: 1,
				DebugLog: "[22:42:00.000] New invocation is queued and will start shortly\n" +
					"[22:42:00.000] Invocation is manually aborted by user:owner@example.com\n" +
					"[22:42:00.000] Invocation finished in 0s with status ABORTED\n",
			}))

			// Unpause the task queue to confirm the new invocation doesn't actually
			// start.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				panic("must not be called")
			}
			tasks, _, err := tq.RunSimulation(c, nil)
			assert.Loosely(t, err, should.BeNil)

			// The sequence of tasks we've just performed.
			assert.Loosely(t, tasks.Payloads(), should.Resemble([]protoiface.MessageV1{
				// The delayed triage directly from AbortJob.
				&internal.KickTriageTask{JobId: jobID},
				// The invocation finalization from AbortInvocation.
				&internal.InvocationFinishedTask{JobId: jobID, InvId: expectedInvID},

				// Noop launch. We can't undo the already posted tasks. Note that the
				// actual launch didn't happen, as checked by mgr.launchTask.
				&internal.LaunchInvocationsBatchTask{
					Tasks: []*internal.LaunchInvocationTask{{JobId: jobID, InvId: expectedInvID}},
				},
				&internal.LaunchInvocationTask{JobId: jobID, InvId: expectedInvID},

				// The triage from KickTriageTask and from InvocationFinishedTask
				// finally arrives.
				&internal.TriageJobStateTask{JobId: jobID},
			}))

			// The job state is updated (the invocation is no longer active).
			job, err = e.getJob(c, jobID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, job.ActiveInvocations, should.BeNil)

			// The invocation is now in the list of finished invocations.
			datastore.GetTestable(c).CatchupIndexes()
			invs, _, _ := e.ListInvocations(mockOwnerCtx(c, realmID), job, ListInvocationsOpts{
				PageSize:     100,
				FinishedOnly: true,
			})
			assert.Loosely(t, invs, should.Resemble([]*Invocation{inv}))
		})

		t.Run("inv aborted while it is running", func(t *ftt.Test) {
			// Let the invocation start and set a timer. Abort the simulation before
			// the timer ticks.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				ctl.State().Status = task.StatusRunning
				ctl.AddTimer(ctx, time.Minute, "1 min", nil)
				return nil
			}
			tasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
				ShouldStopBefore: func(t tqtesting.Task) bool {
					_, ok := t.Payload.(*internal.TimerTask)
					return ok
				},
			})
			assert.Loosely(t, err, should.BeNil)

			// The sequence of tasks we've just performed.
			assert.Loosely(t, tasks.Payloads(), should.Resemble([]protoiface.MessageV1{
				&internal.LaunchInvocationsBatchTask{
					Tasks: []*internal.LaunchInvocationTask{{JobId: jobID, InvId: expectedInvID}},
				},
				&internal.LaunchInvocationTask{
					JobId: jobID, InvId: expectedInvID,
				},
			}))

			// At this point the timer tick is scheduled to happen 1 min from now, but
			// we abort the job.
			mgr.abortTask = func(ctx context.Context, ctl task.Controller) error {
				ctl.DebugLog("Really aborted!")
				return nil
			}
			assert.Loosely(t, e.AbortJob(mockOwnerCtx(c, realmID), job), should.BeNil)

			// It is dead right away.
			inv, err := e.getInvocation(c, jobID, invID)
			assert.Loosely(t, inv.Status, should.Equal(task.StatusAborted))

			// And AbortTask callback was really called.
			assert.Loosely(t, inv.DebugLog, should.ContainSubstring("Really aborted!"))

			// Run all processes to completion.
			mgr.handleTimer = func(ctx context.Context, ctl task.Controller, name string, payload []byte) error {
				panic("must not be called")
			}
			tasks, _, err = tq.RunSimulation(c, nil)
			assert.Loosely(t, err, should.BeNil)

			// The sequence of tasks we've just performed.
			assert.Loosely(t, tasks.Payloads(), should.Resemble([]protoiface.MessageV1{
				// The delayed triage directly from AbortJob.
				&internal.KickTriageTask{JobId: jobID},
				// The invocation finalization from AbortInvocation.
				&internal.InvocationFinishedTask{JobId: jobID, InvId: expectedInvID},

				// The triage from KickTriageTask and from InvocationFinishedTask
				// finally arrives.
				&internal.TriageJobStateTask{JobId: jobID},

				// And delayed TimerTask arrives and gets skipped, as confirmed by
				// mgr.handleTimer.
				&internal.TimerTask{
					JobId: jobID,
					InvId: expectedInvID,
					Timer: &internal.Timer{
						Id:      "project/job:9200093523825193008:1:0",
						Created: timestamppb.New(epoch.Add(time.Second)),
						Eta:     timestamppb.New(epoch.Add(time.Minute + time.Second)),
						Title:   "1 min",
					},
				},
			}))
		})
	})
}

func TestEmitTriggers(t *testing.T) {
	t.Parallel()

	ftt.Run("with fake env", t, func(t *ftt.Test) {
		const testingJob = "project/job"
		const realmID = "project:testing"

		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		assert.Loosely(t, e.UpdateProjectJobs(c, "project", []catalog.Definition{
			{
				JobID:    testingJob,
				RealmID:  realmID,
				Revision: "rev1",
				Schedule: "triggered",
				Task:     noopTaskBytes(),
			},
		}), should.BeNil)

		t.Run("happy path", func(t *ftt.Test) {
			var incomingTriggers []*internal.Trigger
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				incomingTriggers = ctl.Request().IncomingTriggers
				ctl.State().Status = task.StatusSucceeded
				return nil
			}

			job, err := e.getJob(c, testingJob)
			assert.Loosely(t, err, should.BeNil)

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
			err = e.EmitTriggers(mockOwnerCtx(c, realmID), map[*Job][]*internal.Trigger{job: emittedTriggers})
			assert.Loosely(t, err, should.BeNil)

			// Run TQ until all activities stop.
			tasks, _, err := tq.RunSimulation(c, nil)
			assert.Loosely(t, err, should.BeNil)

			// We expect a triage invoked by EmitTrigger, and one full invocation.
			expect := expectedTasks{JobID: testingJob, Epoch: epoch}
			expect.triage()
			expect.invocationSequence(9200093521727759856)
			expect.triage()
			assert.Loosely(t, tasks.Payloads(), should.Resemble(expect.Tasks))

			// The task manager received all triggers (though maybe out of order, it
			// is not defined).
			sort.Slice(incomingTriggers, func(i, j int) bool {
				return incomingTriggers[i].Id < incomingTriggers[j].Id
			})
			assert.Loosely(t, incomingTriggers, should.Resemble(emittedTriggers))
		})

		t.Run("no scheduler.jobs.trigger permission", func(t *ftt.Test) {
			job, err := e.getJob(c, testingJob)
			assert.Loosely(t, err, should.BeNil)

			err = e.EmitTriggers(mockReaderCtx(c, realmID), map[*Job][]*internal.Trigger{
				job: {
					{Id: "t1"},
				},
			})
			assert.Loosely(t, err, should.Equal(ErrNoPermission))
		})

		t.Run("paused job ignores triggers", func(t *ftt.Test) {
			job, err := e.getJob(c, testingJob)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, e.setJobPausedFlag(c, job, true, "", ""), should.BeNil)

			// The pause emits a triage, get over it now.
			tasks, _, err := tq.RunSimulation(c, nil)
			assert.Loosely(t, err, should.BeNil)
			expect := expectedTasks{JobID: testingJob, Epoch: epoch}
			expect.kickTriage()
			expect.triage()
			assert.Loosely(t, tasks.Payloads(), should.Resemble(expect.Tasks))

			// Make the RPC, which succeeds.
			err = e.EmitTriggers(mockOwnerCtx(c, realmID), map[*Job][]*internal.Trigger{
				job: {
					{Id: "t1"},
				},
			})
			assert.Loosely(t, err, should.BeNil)

			// But nothing really happens.
			tasks, _, err = tq.RunSimulation(c, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tasks.Payloads(), should.HaveLength(0))
		})
	})
}

func TestOneJobTriggersAnother(t *testing.T) {
	t.Parallel()

	ftt.Run("with fake env", t, func(t *ftt.Test) {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		triggeringJob := "project/triggering-job"
		triggeredJob := "project/triggered-job"

		assert.Loosely(t, e.UpdateProjectJobs(c, "project", []catalog.Definition{
			{
				JobID:           triggeringJob,
				RealmID:         "project:testing",
				TriggeredJobIDs: []string{triggeredJob},
				Revision:        "rev1",
				Schedule:        "triggered",
				Task:            noopTaskBytes(),
			},
			{
				JobID:    triggeredJob,
				RealmID:  "project:testing",
				Revision: "rev1",
				Schedule: "triggered",
				Task:     noopTaskBytes(),
			},
		}), should.BeNil)

		t.Run("happy path", func(t *ftt.Test) {
			const triggeringInvID int64 = 9200093523824911856
			const triggeredInvID int64 = 9200093521728457040

			// Force launch the triggering job.
			forceInvocation(c, e, triggeringJob)

			// Eventually it runs the task which emits a bunch of triggers, which
			// causes the triggered job triage. We stop right before it and examine
			// what we see.
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				ctl.EmitTrigger(ctx, &internal.Trigger{Id: "t1"})
				assert.Loosely(t, ctl.Save(ctx), should.BeNil)
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
			assert.Loosely(t, err, should.BeNil)

			// How these triggers are seen from outside the task.
			expectedTrigger1 := &internal.Trigger{
				Id:           "t1",
				JobId:        triggeringJob,
				InvocationId: triggeringInvID,
				Created:      timestamppb.New(epoch.Add(1 * time.Second)),
			}
			expectedTrigger2 := &internal.Trigger{
				Id:           "t2",
				JobId:        triggeringJob,
				InvocationId: triggeringInvID,
				Created:      timestamppb.New(epoch.Add(1 * time.Second)),
				OrderInBatch: 1, // second call to EmitTrigger done by the invocation
			}

			// All the tasks we've just executed.
			assert.Loosely(t, tasks.Payloads(), should.Resemble([]protoiface.MessageV1{
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
			}))

			// Triggering invocation has finished (with triggers recorded).
			triggeringInv, err := e.getInvocation(c, triggeringJob, triggeringInvID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, triggeringInv.Status, should.Equal(task.StatusSucceeded))
			outgoing, err := triggeringInv.OutgoingTriggers()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, outgoing, should.Resemble([]*internal.Trigger{expectedTrigger1, expectedTrigger2}))

			// At this point triggered job's triage is about to start. Before it does,
			// verify emitted trigger (sitting in the pending triggers set) is
			// discoverable through ListTriggers.
			tj, _ := e.getJob(c, triggeredJob)
			triggers, err := e.ListTriggers(c, tj)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, triggers, should.Resemble([]*internal.Trigger{expectedTrigger1, expectedTrigger2}))

			// Resume the simulation to do the triages and start the triggered
			// invocation.
			var seen []*internal.Trigger
			mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
				seen = ctl.Request().IncomingTriggers
				ctl.State().Status = task.StatusSucceeded
				return nil
			}
			tasks, _, err = tq.RunSimulation(c, nil)
			assert.Loosely(t, err, should.BeNil)

			// All the tasks we've just executed.
			assert.Loosely(t, tasks.Payloads(), should.Resemble([]protoiface.MessageV1{
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
			}))

			// Verify LaunchTask callback saw the triggers.
			assert.Loosely(t, seen, should.Resemble([]*internal.Trigger{expectedTrigger1, expectedTrigger2}))

			// And they are recoded in IncomingTriggers set.
			triggeredInv, err := e.getInvocation(c, triggeredJob, triggeredInvID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, triggeredInv.Status, should.Equal(task.StatusSucceeded))
			incoming, err := triggeredInv.IncomingTriggers()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, incoming, should.Resemble([]*internal.Trigger{expectedTrigger1, expectedTrigger2}))
		})
	})
}

func TestInvocationTimers(t *testing.T) {
	t.Parallel()

	ftt.Run("with fake env", t, func(t *ftt.Test) {
		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		const testJobID = "project/job"
		assert.Loosely(t, e.UpdateProjectJobs(c, "project", []catalog.Definition{
			{
				JobID:    testJobID,
				RealmID:  "project:testing",
				Revision: "rev1",
				Schedule: "triggered",
				Task:     noopTaskBytes(),
			},
		}), should.BeNil)

		t.Run("happy path", func(t *ftt.Test) {
			const testInvID int64 = 9200093523825193008

			// Force launch the job.
			forceInvocation(c, e, testJobID)

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
			assert.Loosely(t, err, should.BeNil)

			timerMsg := func(idSuffix string, created, eta time.Duration, title string, payload []byte) *internal.Timer {
				return &internal.Timer{
					Id:      fmt.Sprintf("%s:%d:%s", testJobID, testInvID, idSuffix),
					Created: timestamppb.New(epoch.Add(created)),
					Eta:     timestamppb.New(epoch.Add(eta)),
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
			assert.Loosely(t, callTimes, should.Resemble(map[string]time.Duration{
				"1 min": time.Second + time.Minute,
				"2 min": time.Second + 2*time.Minute,
				"stop":  time.Second + 3*time.Minute,
			}))

			// All the tasks we've just executed.
			assert.Loosely(t, tasks.Payloads(), should.Resemble([]protoiface.MessageV1{
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
			}))
		})
	})
}

func TestCron(t *testing.T) {
	t.Parallel()

	ftt.Run("with fake env", t, func(t *ftt.Test) {
		const testJobID = "project/job"

		c := newTestContext(epoch)
		e, mgr := newTestEngine()

		updateJob := func(schedule string) {
			assert.Loosely(t, e.UpdateProjectJobs(c, "project", []catalog.Definition{
				{
					JobID:    testJobID,
					RealmID:  "project:testing",
					Revision: "rev1",
					Schedule: schedule,
					Task:     noopTaskBytes(),
				},
			}), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()
		}

		mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
			ctl.State().Status = task.StatusSucceeded
			return nil
		}

		tq := tqtesting.GetTestable(c, e.cfg.Dispatcher)
		tq.CreateQueues()

		t.Run("relative schedule", func(t *ftt.Test) {
			updateJob("with 10s interval")

			t.Run("happy path", func(t *ftt.Test) {
				// Let the TQ spin for two full cycles.
				tasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					Deadline: epoch.Add(30 * time.Second),
				})
				assert.Loosely(t, err, should.BeNil)

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

				assert.Loosely(t, tasks.Payloads(), should.Resemble(expect.Tasks))
			})

			t.Run("schedule changes", func(t *ftt.Test) {
				// Let the TQ spin until it hits the task execution.
				tasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					ShouldStopBefore: func(t tqtesting.Task) bool {
						_, ok := t.Payload.(*internal.LaunchInvocationTask)
						return ok
					},
				})
				assert.Loosely(t, err, should.BeNil)

				// At this point the job's schedule changes.
				updateJob("with 30s interval")

				// We let the TQ spin some more.
				moreTasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					Deadline: epoch.Add(50 * time.Second),
				})
				assert.Loosely(t, err, should.BeNil)

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
				assert.Loosely(t, append(tasks, moreTasks...).Payloads(), should.Resemble(expect.Tasks))
			})

			t.Run("pause/unpause", func(t *ftt.Test) {
				// Let the TQ spin until it hits the end of task execution.
				tasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					ShouldStopAfter: func(t tqtesting.Task) bool {
						_, ok := t.Payload.(*internal.InvocationFinishedTask)
						return ok
					},
				})
				assert.Loosely(t, err, should.BeNil)

				// At this point we pause the job.
				j, err := e.getJob(c, testJobID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, e.setJobPausedFlag(c, j, true, "user:someone@example.com", "pause reason"), should.BeNil)

				// The information about the pause was recorded in the entity.
				job, err := e.getJob(c, testJobID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, job.Paused, should.BeTrue)
				assert.Loosely(t, job.PausedOrResumedWhen, should.Match(clock.Now(c).UTC()))
				assert.Loosely(t, job.PausedOrResumedBy, should.Equal(identity.Identity("user:someone@example.com")))
				assert.Loosely(t, job.PausedOrResumedReason, should.Equal("pause reason"))

				// We let the TQ spin some more.
				moreTasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					Deadline: epoch.Add(time.Hour),
				})
				assert.Loosely(t, err, should.BeNil)

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
				assert.Loosely(t, append(tasks, moreTasks...).Payloads(), should.Resemble(expect.Tasks))

				// Some time later we unpause the job, it starts again immediately.
				clock.Get(c).(testclock.TestClock).Set(epoch.Add(time.Hour))
				assert.Loosely(t, e.setJobPausedFlag(c, j, false, "user:someone@example.com", "resume reason"), should.BeNil)

				tasks, _, err = tq.RunSimulation(c, &tqtesting.SimulationParams{
					Deadline: epoch.Add(time.Hour + 10*time.Second),
				})
				assert.Loosely(t, err, should.BeNil)

				// Did it?
				expect.clear()
				expect.cronTickSequence(325298467681248558, 7, time.Hour)
				expect.invocationSequence(9200089746854060480)
				expect.triage()
				assert.Loosely(t, tasks.Payloads(), should.Resemble(expect.Tasks))
			})

			t.Run("disabling", func(t *ftt.Test) {
				// Let the TQ spin until it hits the end of task execution.
				tasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					ShouldStopAfter: func(t tqtesting.Task) bool {
						_, ok := t.Payload.(*internal.InvocationFinishedTask)
						return ok
					},
				})
				assert.Loosely(t, err, should.BeNil)

				// At this point we disable the job.
				assert.Loosely(t, e.UpdateProjectJobs(c, "project", nil), should.BeNil)

				// We let the TQ spin some more...
				moreTasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					Deadline: epoch.Add(time.Hour),
				})
				assert.Loosely(t, err, should.BeNil)

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
				assert.Loosely(t, append(tasks, moreTasks...).Payloads(), should.Resemble(expect.Tasks))
			})
		})

		t.Run("absolute schedule", func(t *ftt.Test) {
			updateJob("5,10 * * * * * *") // on 5th and 10th sec

			t.Run("happy path", func(t *ftt.Test) {
				// Let the TQ spin for two full cycles.
				tasks, _, err := tq.RunSimulation(c, &tqtesting.SimulationParams{
					Deadline: epoch.Add(14 * time.Second),
				})
				assert.Loosely(t, err, should.BeNil)

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

				assert.Loosely(t, tasks.Payloads(), should.Resemble(expect.Tasks))
			})

			t.Run("overrun", func(t *ftt.Test) {
				// Currently cron just keeps submitting triggers that ends up in the
				// pending triggers queue if there's some invocation currently running.
				// Once the invocation finishes, the next one start right away (just
				// like with any other kind of trigger). Overruns are not recorded.

				// Simulate "long" task with timers.
				mgr.launchTask = func(ctx context.Context, ctl task.Controller) error {
					ctl.AddTimer(ctx, 6*time.Second, "6 sec", nil)
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
				assert.Loosely(t, err, should.BeNil)

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
				expect.invocationTimer(9200093517533908688, 1, "6 sec", 7*time.Second, 13*time.Second)
				expect.invocationEnd(9200093517533908688)
				expect.triage()
				// The triage detects pending cron trigger and launches a new invocation
				// right away.
				expect.invocationStart(9200093509144748480)

				assert.Loosely(t, tasks.Payloads(), should.Resemble(expect.Tasks))
			})
		})
	})
}

func noopTaskBytes() []byte {
	buf, _ := proto.Marshal(&messages.TaskDefWrapper{Noop: &messages.NoopTask{}})
	return buf
}

////////////////////////////////////////////////////////////////////////////////

func forceInvocation(c context.Context, e *engineImpl, jobID string) (inv *Invocation) {
	err := e.jobTxn(c, jobID, func(c context.Context, job *Job, isNew bool) (err error) {
		invs, err := e.enqueueInvocations(c, job, []task.Request{
			{},
		})
		if err != nil {
			return err
		}
		inv = invs[0]
		return nil
	})
	if err != nil {
		panic(err)
	}
	return
}

////////////////////////////////////////////////////////////////////////////////

type expectedTasks struct {
	JobID string
	Epoch time.Time
	Tasks []protoiface.MessageV1
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
					Created: timestamppb.New(e.Epoch.Add(when)),
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
			Created: timestamppb.New(e.Epoch.Add(created)),
			Eta:     timestamppb.New(e.Epoch.Add(eta)),
		},
	})
}
