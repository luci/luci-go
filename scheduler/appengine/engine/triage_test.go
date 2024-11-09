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
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/scheduler/appengine/engine/policy"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
)

func TestTriageOp(t *testing.T) {
	t.Parallel()

	ftt.Run("with fake env", t, func(t *ftt.Test) {
		c := newTestContext(epoch)
		tb := triageTestBed{maxAllowedTriggers: 1000}

		t.Run("noop triage", func(t *ftt.Test) {
			before := &Job{
				JobID:             "job",
				Enabled:           true,
				ActiveInvocations: []int64{1, 2, 3},
			}
			after, err := tb.runTestTriage(c, before)
			assert.Loosely(t, err, should.BeNil)

			// Only LastTriage timestamp is bumped.
			expected := *before
			expected.LastTriage = epoch
			assert.Loosely(t, after, should.Resemble(&expected))

			// Have some triage log.
			job := JobTriageLog{JobID: "job"}
			assert.Loosely(t, datastore.Get(c, &job), should.BeNil)
			assert.Loosely(t, job, should.Resemble(JobTriageLog{
				JobID:      "job",
				LastTriage: epoch,
				DebugLog: strings.Join([]string{
					"[000 ms] Starting",
					"[000 ms] Pending triggers set:  0 items, 0 garbage",
					"[000 ms] Recently finished set: 0 items, 0 garbage",
					"[000 ms] The preparation is finished",
					"[000 ms] Started the transaction",
					"[000 ms] Number of active invocations: 3",
					"[000 ms] Number of recently finished:  0",
					"[000 ms] Triggers available in this txn: 0",
					"[000 ms] Invoking the triggering policy function",
					"[000 ms] The policy requested 0 new invocations",
					"[000 ms] Removing consumed dsset items",
					"[000 ms] Landing the transaction",
					"[000 ms] Done",
					"",
				}, "\n"),
			}))
		})

		t.Run("pops finished invocations", func(t *ftt.Test) {
			before := &Job{
				JobID:             "job",
				Enabled:           true,
				ActiveInvocations: []int64{1, 2, 3, 4, 5},
			}
			recentlyFinishedSet(c, before.JobID).Add(c, []int64{1, 2, 4})

			// We record the triage time inside FinishedInvocation.Finished, not when
			// the invocations was actually finished. Usually it shouldn't matter,
			// since the difference is only a couple of seconds (due to TQ delays).
			clock.Get(c).(testclock.TestClock).Add(15 * time.Second)
			expectedFinishedTS := epoch.Add(15 * time.Second)

			after, err := tb.runTestTriage(c, before)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, after.ActiveInvocations, should.Resemble([]int64{3, 5}))

			// FinishedInvocationsRaw has the finished invocations.
			finishedRaw := after.FinishedInvocationsRaw
			finished, err := unmarshalFinishedInvs(finishedRaw)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, finished, should.Resemble([]*internal.FinishedInvocation{
				{InvocationId: 1, Finished: timestamppb.New(expectedFinishedTS)},
				{InvocationId: 2, Finished: timestamppb.New(expectedFinishedTS)},
				{InvocationId: 4, Finished: timestamppb.New(expectedFinishedTS)},
			}))

			// Some time later, they are still there.
			clock.Get(c).(testclock.TestClock).Add(FinishedInvocationsHorizon - time.Second)
			after, err = tb.runTestTriage(c, after)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, after.FinishedInvocationsRaw, should.Resemble(finishedRaw))

			// But eventually the get kicked out.
			clock.Get(c).(testclock.TestClock).Add(2 * time.Second)
			after, err = tb.runTestTriage(c, after)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, after.FinishedInvocationsRaw, should.BeNil)
		})

		t.Run("processes triggers", func(t *ftt.Test) {
			triggers := []*internal.Trigger{
				{
					Id:      "t0",
					Created: timestamppb.New(epoch.Add(20 * time.Second)),
				},
				{
					Id:      "t1",
					Created: timestamppb.New(epoch.Add(20 * time.Second)),
				},
				{
					Id:      "t2",
					Created: timestamppb.New(epoch.Add(10 * time.Second)),
				},
				{
					Id:      "a_t3", // to make its ID be before t0 and t1
					Created: timestamppb.New(epoch.Add(20 * time.Second)),
				},
			}

			before := &Job{
				JobID:   "job",
				Enabled: true,
			}
			pendingTriggersSet(c, before.JobID).Add(c, triggers)

			// Cycle 1. Pops triggers, converts them into a bunch of invocations.
			// Triggers are in sorted order!
			after, err := tb.runTestTriage(c, before)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, after.ActiveInvocations, should.Resemble([]int64{1, 2, 3, 4}))
			assert.Loosely(t, tb.requests, should.HaveLength(4))
			assert.Loosely(t, tb.requests[0].IncomingTriggers, should.Resemble([]*internal.Trigger{triggers[2]}))
			assert.Loosely(t, tb.requests[1].IncomingTriggers, should.Resemble([]*internal.Trigger{triggers[3]}))
			assert.Loosely(t, tb.requests[2].IncomingTriggers, should.Resemble([]*internal.Trigger{triggers[0]}))
			assert.Loosely(t, tb.requests[3].IncomingTriggers, should.Resemble([]*internal.Trigger{triggers[1]}))
			tb.requests = nil

			// Cycle 2. Nothing new.
			after, err = tb.runTestTriage(c, after)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, after.ActiveInvocations, should.Resemble([]int64{1, 2, 3, 4}))
			assert.Loosely(t, tb.requests, should.BeNil)
		})

		t.Run("ignores triggers if paused", func(t *ftt.Test) {
			triggers := []*internal.Trigger{
				{
					Id:      "t0",
					Created: timestamppb.New(epoch.Add(20 * time.Second)),
				},
				{
					Id:      "t1",
					Created: timestamppb.New(epoch.Add(20 * time.Second)),
				},
			}

			pendingTriggersSet(c, "job").Add(c, triggers)

			// Just pops and discards triggers without starting anything.
			after, err := tb.runTestTriage(c, &Job{
				JobID:   "job",
				Enabled: true,
				Paused:  true,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, after.ActiveInvocations, should.HaveLength(0))
			assert.Loosely(t, tb.requests, should.HaveLength(0))

			// No triggers there anymore.
			listing, err := pendingTriggersSet(c, "job").List(c)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, listing.Items, should.HaveLength(0))
		})

		t.Run("drops excessive triggers", func(t *ftt.Test) {
			tb.maxAllowedTriggers = 1

			triggers := []*internal.Trigger{
				{
					Id:      "most-recent",
					Created: timestamppb.New(epoch.Add(3 * time.Second)),
				},
				{
					Id:      "dropped-1",
					Created: timestamppb.New(epoch.Add(2 * time.Second)),
				},
				{
					Id:      "dropped-2",
					Created: timestamppb.New(epoch.Add(1 * time.Second)),
				},
			}

			pendingTriggersSet(c, "job").Add(c, triggers)

			// Only the most recent trigger will be kept alive and converted into
			// an invocation.
			after, err := tb.runTestTriage(c, &Job{
				JobID:   "job",
				Enabled: true,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, after.ActiveInvocations, should.HaveLength(1))
			assert.Loosely(t, tb.requests, should.HaveLength(1))
			assert.Loosely(t, tb.requests[0].IncomingTriggers, should.Resemble([]*internal.Trigger{triggers[0]}))

			// No triggers there anymore.
			listing, err := pendingTriggersSet(c, "job").List(c)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, listing.Items, should.HaveLength(0))
		})

		t.Run("doesn't touch ds sets if txn fails to land", func(t *ftt.Test) {
			triggers := []*internal.Trigger{
				{
					Id:      "t0",
					Created: timestamppb.New(epoch.Add(20 * time.Second)),
				},
				{
					Id:      "t1",
					Created: timestamppb.New(epoch.Add(20 * time.Second)),
				},
			}
			pendingTriggersSet(c, "job").Add(c, triggers)

			brokenC, fb := featureBreaker.FilterRDS(c, nil)
			fb.BreakFeatures(nil, "CommitTransaction")

			after, err := tb.runTestTriage(brokenC, &Job{
				JobID:   "job",
				Enabled: true,
			})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, after.ActiveInvocations, should.HaveLength(0))

			// Triggers are still there.
			listing, err := pendingTriggersSet(c, "job").List(c)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, listing.Items, should.HaveLength(2))
		})

		t.Run("discards triggers", func(t *ftt.Test) {
			tb.policyDiscardsTriggers = true

			triggers := []*internal.Trigger{
				{
					Id:      "t0",
					Created: timestamppb.New(epoch.Add(20 * time.Second)),
				},
				{
					Id:      "t1",
					Created: timestamppb.New(epoch.Add(20 * time.Second)),
				},
			}

			before := &Job{
				JobID:    "job",
				Schedule: "with 10m interval", // This needs to be set, since the engine will poke cron.
				Enabled:  true,
			}
			pendingTriggersSet(c, before.JobID).Add(c, triggers)

			// Cycle 1. Pops triggers and discards them.
			after, err := tb.runTestTriage(c, before)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, after.ActiveInvocations, should.HaveLength(0))
			assert.Loosely(t, tb.requests, should.HaveLength(0))

			// Cycle 2. Nothing new.
			after, err = tb.runTestTriage(c, after)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, after.ActiveInvocations, should.HaveLength(0))
			assert.Loosely(t, tb.requests, should.HaveLength(0))
		})
	})
}

type triageTestBed struct {
	// Inputs.
	maxAllowedTriggers     int
	policyDiscardsTriggers bool

	// Outputs.
	nextInvID int64
	requests  []task.Request
}

// runTestTriage runs the triage operation and refetches the job after it.
//
// On triage errors, both 'after' and 'err' are returned.
func (t *triageTestBed) runTestTriage(c context.Context, before *Job) (after *Job, err error) {
	if err = datastore.Put(c, before); err != nil {
		return nil, err
	}

	op := triageOp{
		jobID: before.JobID,
		policyFactory: func(*messages.TriggeringPolicy) (policy.Func, error) {
			return func(env policy.Environment, in policy.In) (out policy.Out) {
				if t.policyDiscardsTriggers {
					out.Discard = append(out.Discard, in.Triggers...)
				} else {
					for _, t := range in.Triggers {
						out.Requests = append(out.Requests, task.Request{
							IncomingTriggers: []*internal.Trigger{t},
						})
					}
				}
				return
			}, nil
		},
		enqueueInvocations: func(c context.Context, job *Job, req []task.Request) error {
			t.requests = append(t.requests, req...)
			for range req {
				t.nextInvID++
				job.ActiveInvocations = append(job.ActiveInvocations, t.nextInvID)
			}
			return nil
		},
		maxAllowedTriggers: t.maxAllowedTriggers,
	}

	if err = op.prepare(c); err == nil {
		err = runTxn(c, func(c context.Context) error {
			job := Job{JobID: op.jobID}
			if err := datastore.Get(c, &job); err != nil {
				return err
			}
			if err := op.transaction(c, &job); err != nil {
				return err
			}
			return datastore.Put(c, &job)
		})
	}
	op.finalize(c, err == nil)

	after = &Job{JobID: before.JobID}
	if getErr := datastore.Get(c, after); getErr != nil {
		panic("must not happen")
	}
	return after, err
}
