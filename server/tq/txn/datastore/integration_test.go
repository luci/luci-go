// Copyright 2020 The LUCI Authors.
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

package datastore

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/filter/featureBreaker/flaky"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/tqtesting"
)

func TestDistributedSweeping(t *testing.T) {
	t.Parallel()

	RunTest(t, func(disp *tq.Dispatcher) tq.Sweeper {
		// Use smaller sweep tasks to hit more edge cases.
		return tq.NewDistributedSweeper(disp, tq.DistributedSweeperOptions{
			SweepShards:         4,
			TasksPerScan:        10,
			SecondaryScanShards: 4,
		})
	})
}

func TestInProcSweeping(t *testing.T) {
	t.Parallel()

	RunTest(t, func(disp *tq.Dispatcher) tq.Sweeper {
		// Use smaller sweep tasks to hit more edge cases.
		return tq.NewInProcSweeper(tq.InProcSweeperOptions{
			SweepShards:             4,
			TasksPerScan:            10,
			SecondaryScanShards:     4,
			SubmitBatchSize:         4,
			SubmitConcurrentBatches: 2,
		})
	})
}

// RunTest ensures that transactionally submitted tasks eventually execute,
// and only once, even if the database and Cloud Tasks RPCs fail with high
// chance.
func RunTest(t *testing.T, sweeper func(*tq.Dispatcher) tq.Sweeper) {
	var epoch = testclock.TestRecentTimeUTC
	const sweepSleep = "sweep sleep"

	ctx, tc := testclock.UseTime(context.Background(), epoch)
	tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
		if testclock.HasTags(t, tqtesting.ClockTag) {
			panic("there should be no task retries, they all should fail fatally")
		}
		if testclock.HasTags(t, sweepSleep) {
			tc.Add(d)
		}
	})

	ctx = txndefer.FilterRDS(memory.Use(ctx))
	ctx, fb := featureBreaker.FilterRDS(ctx, nil)
	datastore.GetTestable(ctx).Consistent(true)

	// Note: must install after memory.Use, since it overrides the logger.
	ctx = gologger.StdConfig.Use(ctx)
	ctx = logging.SetLevel(ctx, logging.Debug)

	// Use a single RND for all flaky.Errors(...) instances. Otherwise they
	// repeat the same random pattern each time withBrokenDS is called.
	rnd := rand.NewSource(0)

	withBrokenDS := func(cb func()) {
		// Makes datastore very faulty.
		fb.BreakFeaturesWithCallback(
			flaky.Errors(flaky.Params{
				Rand:                             rnd,
				DeadlineProbability:              0.3,
				ConcurrentTransactionProbability: 0.3,
			}),
			featureBreaker.DatastoreFeatures...,
		)

		cb()

		// "Fixes" datastore, letting us examine it.
		fb.BreakFeaturesWithCallback(
			func(context.Context, string) error { return nil },
			featureBreaker.DatastoreFeatures...,
		)
	}

	disp := &tq.Dispatcher{}
	ctx, sched := tq.TestingContext(ctx, disp)
	disp.Sweeper = sweeper(disp)

	// "Buganize" the submitter.
	ctx = tq.UseSubmitter(ctx, &flakySubmitter{
		Submitter:              sched,
		InternalErrProbability: 0.3,
		Rand:                   rand.New(rand.NewSource(123)),
	})

	// This will collect which tasks were executed and how many times.
	mu := sync.Mutex{}
	execed := map[int]int{}

	disp.RegisterTaskClass(tq.TaskClass{
		ID:        "work",
		Prototype: &durationpb.Duration{}, // use it just as int container
		Kind:      tq.Transactional,
		Queue:     "default",
		Handler: func(ctx context.Context, msg proto.Message) error {
			d := msg.(*durationpb.Duration)
			mu.Lock()
			execed[int(d.Seconds)]++
			mu.Unlock()
			return nil
		},
	})

	type testEntity struct {
		_kind string `gae:"$kind,testEntity"`
		ID    int    `gae:"$id"`
	}

	// Run a bunch of transactions that each add an entity and submit a task.
	// Some of them will fail, this is fine. But eventually each submitted
	// task must execute, and only once.
	withBrokenDS(func() {
		parallel.WorkPool(16, func(work chan<- func() error) {
			for i := 1; i <= 500; i++ {
				i := i
				work <- func() error {
					return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						task := &tq.Task{Payload: &durationpb.Duration{Seconds: int64(i)}}
						if err := disp.AddTask(ctx, task); err != nil {
							return err
						}
						return datastore.Put(ctx, &testEntity{ID: i})
					}, nil)
				}
			}
		})
	})

	// See how many transactions really landed.
	var landed []*testEntity
	assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery("testEntity"), &landed), should.BeNil)
	assert.Loosely(t, len(landed), should.BeGreaterThan(5)) // it is usually much larger (but still random)

	// Run rounds of sweeps until there are no reminders left or we appear to
	// be stuck. Use panics instead of So(...) to avoid spamming goconvey with
	// lots and lots of assertion dots.
	for {
		reminders, err := datastore.Count(ctx, datastore.NewQuery(reminderKind))
		if err != nil {
			panic(err)
		}
		logging.Infof(ctx, "%s: %d reminders left, %d tasks executed", clock.Now(ctx).Sub(epoch), reminders, len(execed))
		if reminders == 0 && len(sched.Tasks()) == 0 {
			break // no pending tasks and no reminders in the datastore, we are done
		}

		// Blow up if it takes too much time to converge. Note that this is fake
		// time. Also the limit is much-much higher than expected mean time to
		// make sure this test doesn't flake.
		//
		// The tree closed anyway. I bumped the time from 360 to 720 fake minutes on
		// 2025-02-26. If this test flakes again in less than two quarters, let's disable
		// or rewrite it.
		if clock.Now(ctx).Sub(epoch) >= 720*time.Minute { // 720 Sweeps.
			panic("Looks like the test is stuck")
		}

		// Submit a bunch of sweep tasks and wait until they (and all their
		// follow ups) are done.
		withBrokenDS(func() {
			disp.Sweep(ctx)
			sched.Run(ctx, tqtesting.StopWhenDrained())
		})

		// Launch the next sweep a bit later. This is the only ticking clock in
		// the simulation. It is needed because we use "real" time when checking
		// freshness of reminders.
		clock.Sleep(clock.Tag(ctx, sweepSleep), time.Minute)
	}

	// All transactionally submitted tasks should have been executed.
	assert.Loosely(t, len(execed), should.Equal(len(landed)))
	// And at most once.
	for k, v := range execed {
		if v != 1 {
			t.Errorf("task %d executed %d times", k, v)
		}
	}
}

type flakySubmitter struct {
	Submitter              tq.Submitter
	Rand                   *rand.Rand
	InternalErrProbability float64

	m sync.Mutex
}

func (f *flakySubmitter) Submit(ctx context.Context, req *reminder.Payload) error {
	f.m.Lock()
	fail := f.Rand.Float64() < f.InternalErrProbability
	f.m.Unlock()
	if fail {
		return status.Errorf(codes.Internal, "Simulated internal error")
	}
	return f.Submitter.Submit(ctx, req)
}
