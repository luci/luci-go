// Copyright 2021 The LUCI Authors.
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

package impl

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/longops"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/runtest"
)

func TestLongOps(t *testing.T) {
	t.Parallel()

	ftt.Run("Manager handles long ops", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		const runID = "chromium/222-1-deadbeef"
		const initialEVersion = 10
		assert.Loosely(t, datastore.Put(ctx, &run.Run{
			ID:       runID,
			Status:   run.Status_RUNNING,
			EVersion: initialEVersion,
		}), should.BeNil)

		loadRun := func(ctx context.Context) *run.Run {
			ret := &run.Run{ID: runID}
			assert.Loosely(t, datastore.Get(ctx, ret), should.BeNil)
			return ret
		}

		notifier := run.NewNotifier(ct.TQDispatcher)
		proc := &runProcessor{
			runID:        runID,
			runNotifier:  notifier,
			tqDispatcher: ct.TQDispatcher,
		}

		// Create a new request.
		rs := &state.RunState{Run: *loadRun(ctx)}
		opID := rs.EnqueueLongOp(&run.OngoingLongOps_Op{
			Deadline: timestamppb.New(ct.Clock.Now().Add(time.Minute)),
			Work: &run.OngoingLongOps_Op_PostStartMessage{
				PostStartMessage: true,
			},
		})
		assert.Loosely(t, rs.OngoingLongOps.GetOps(), should.HaveLength(1))

		// Simulate what happens when Run state is transactionally updated.
		assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			return proc.SaveState(ctx, eventbox.State(rs), eventbox.EVersion(rs.EVersion+1))
		}, nil), should.BeNil)

		// Verify that Run's state records new Long operation.
		r := loadRun(ctx)
		assert.Loosely(t, r.OngoingLongOps.GetOps(), should.HaveLength(1))
		op := r.OngoingLongOps.GetOps()[opID]
		assert.Loosely(t, op, should.Match(&run.OngoingLongOps_Op{
			CancelRequested: false,
			Deadline:        timestamppb.New(ct.Clock.Now().Add(time.Minute)),
			Work: &run.OngoingLongOps_Op_PostStartMessage{
				PostStartMessage: true,
			},
		}))
		// Verify that long op task was enqueued.
		assert.Loosely(t, ct.TQ.Tasks().Payloads(), should.Match([]proto.Message{
			&eventpb.ManageRunLongOpTask{
				OperationId: opID,
				RunId:       runID,
			},
		}))

		t.Run("manager handles Long Operation TQ task", func(t *ftt.Test) {
			manager := New(notifier, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ct.Env)

			t.Run("OK", func(t *ftt.Test) {
				called := false
				manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *longops.Base) (*eventpb.LongOpCompleted, error) {
					called = true
					d, ok := ctx.Deadline()
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, d.UTC(), should.Match(op.GetDeadline().AsTime()))
					return &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_SUCCEEDED}, nil
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
				assert.Loosely(t, called, should.BeTrue)
				runtest.AssertReceivedLongOpCompleted(t, ctx, runID, &eventpb.LongOpCompleted{
					OperationId: opID,
					Status:      eventpb.LongOpCompleted_SUCCEEDED,
				})
			})

			t.Run("CancelRequested handling", func(t *ftt.Test) {
				rs := &state.RunState{Run: *loadRun(ctx)}
				rs.RequestLongOpCancellation(opID)
				rs.EVersion++
				assert.Loosely(t, datastore.Put(ctx, &rs.Run), should.BeNil)

				called := false
				manager.testDoLongOperationWithDeadline = func(ctx context.Context, opBase *longops.Base) (*eventpb.LongOpCompleted, error) {
					called = true
					assert.Loosely(t, opBase.IsCancelRequested(), should.BeTrue)
					return &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_CANCELLED}, nil
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
				assert.Loosely(t, called, should.BeTrue)
				runtest.AssertReceivedLongOpCompleted(t, ctx, runID, &eventpb.LongOpCompleted{
					OperationId: opID,
					Status:      eventpb.LongOpCompleted_CANCELLED,
				})
			})

			t.Run("Expired long op must not be executed, but Run Manager should be notified", func(t *ftt.Test) {
				ct.Clock.Add(time.Hour)
				called := false
				manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *longops.Base) (*eventpb.LongOpCompleted, error) {
					called = true
					return &eventpb.LongOpCompleted{}, nil
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
				assert.Loosely(t, called, should.BeFalse)
				runtest.AssertReceivedLongOpCompleted(t, ctx, runID, &eventpb.LongOpCompleted{
					OperationId: opID,
					Status:      eventpb.LongOpCompleted_EXPIRED,
				})
			})

			t.Run("Expired while executing", func(t *ftt.Test) {
				called := false
				manager.testDoLongOperationWithDeadline = func(dctx context.Context, _ *longops.Base) (*eventpb.LongOpCompleted, error) {
					called = true
					ct.Clock.Add(time.Hour) // expire the `dctx`
					// NOTE: it's unclear why the following sometimes fails:
					for dctx.Err() == nil {
						clock.Sleep(dctx, time.Second)
					}
					assert.Loosely(t, dctx.Err(), should.ErrLike(context.DeadlineExceeded))
					return nil, errors.Annotate(dctx.Err(), "somehow treating as permanent failure").Err()
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
				assert.Loosely(t, called, should.BeTrue)
				runtest.AssertReceivedLongOpCompleted(t, ctx, runID, &eventpb.LongOpCompleted{
					OperationId: opID,
					Status:      eventpb.LongOpCompleted_EXPIRED,
				})
			})

			t.Run("Transient failure is retried", func(t *ftt.Test) {
				called := 0
				manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *longops.Base) (*eventpb.LongOpCompleted, error) {
					called++
					if called == 1 {
						return nil, errors.New("troops", transient.Tag)
					}
					return &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_SUCCEEDED}, nil
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
				assert.Loosely(t, called, should.Equal(2))
				runtest.AssertReceivedLongOpCompleted(t, ctx, runID, &eventpb.LongOpCompleted{
					OperationId: opID,
					Status:      eventpb.LongOpCompleted_SUCCEEDED,
				})
			})

			t.Run("Non-transient failure is fatal", func(t *ftt.Test) {
				called := 0
				manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *longops.Base) (*eventpb.LongOpCompleted, error) {
					called++
					if called == 1 {
						return nil, errors.New("foops")
					}
					return &eventpb.LongOpCompleted{}, nil
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
				assert.Loosely(t, called, should.Equal(1))
				runtest.AssertReceivedLongOpCompleted(t, ctx, runID, &eventpb.LongOpCompleted{
					OperationId: opID,
					Status:      eventpb.LongOpCompleted_FAILED,
				})
			})

			t.Run("Doesn't execute in weird cases", func(t *ftt.Test) {
				t.Run("Run deleted", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Delete(ctx, &run.Run{ID: runID}), should.BeNil)
					called := false
					manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *longops.Base) (*eventpb.LongOpCompleted, error) {
						called = true
						return &eventpb.LongOpCompleted{}, nil
					}
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
					assert.Loosely(t, called, should.BeFalse)
					runtest.AssertEventboxEmpty(t, ctx, runID)
				})
				t.Run("Long op is no longer known", func(t *ftt.Test) {
					r := loadRun(ctx)
					r.OngoingLongOps = nil
					assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)
					called := false
					manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *longops.Base) (*eventpb.LongOpCompleted, error) {
						called = true
						return &eventpb.LongOpCompleted{}, nil
					}
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
					assert.Loosely(t, called, should.BeFalse)
					runtest.AssertEventboxEmpty(t, ctx, runID)
				})
			})
		})
	})
}

func TestLongOpCancellationChecker(t *testing.T) {
	t.Parallel()

	ftt.Run("longOpCancellationChecker works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		const runID = "chromium/222-1-deadbeef"
		const opID = "op-1"

		assert.Loosely(t, datastore.Put(ctx, &run.Run{
			ID:       runID,
			Status:   run.Status_RUNNING,
			EVersion: 1,
			OngoingLongOps: &run.OngoingLongOps{
				Ops: map[string]*run.OngoingLongOps_Op{
					opID: {
						CancelRequested: false, // changed in tests below
						// Other fields aren't relevant to this test.
					},
				},
			},
		}), should.BeNil)

		loadRun := func() *run.Run {
			ret := &run.Run{ID: runID}
			assert.Loosely(t, datastore.Get(ctx, ret), should.BeNil)
			return ret
		}

		ct.Clock.SetTimerCallback(func(dur time.Duration, _ clock.Timer) {
			// Whenever background goroutine sleeps, awake it immediately.
			ct.Clock.Add(dur)
		})

		done := make(chan struct{})

		assertDone := func() {
			select {
			case <-done:
			case <-ctx.Done():
				assert.Loosely(t, "context expired before background goroutine was done", should.BeFalse)
			}
		}
		assertNotDone := func() {
			select {
			case <-done:
				assert.Loosely(t, "background goroutine is done", should.BeFalse)
			default:
			}
		}

		l := longOpCancellationChecker{
			interval: time.Second,
			testChan: done,
		}

		t.Run("Normal operation without long op cancellation", func(t *ftt.Test) {
			stop := l.start(ctx, loadRun(), opID)
			defer stop()
			assert.Loosely(t, l.check(), should.BeFalse)
			ct.Clock.Add(time.Minute)
			assert.Loosely(t, l.check(), should.BeFalse)
			assertNotDone()
		})

		t.Run("Initial Run state with cancellation request is noticed immediately", func(t *ftt.Test) {
			r := loadRun()
			r.OngoingLongOps.GetOps()[opID].CancelRequested = true
			assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)

			stop := l.start(ctx, loadRun(), opID)
			defer stop()

			assert.Loosely(t, l.check(), should.BeTrue)
			// Background goroutine shouldn't even be started, hence the `done`
			// channel should remain open.
			assertNotDone()
		})

		t.Run("Notices cancellation request eventually", func(t *ftt.Test) {
			stop := l.start(ctx, loadRun(), opID)
			defer stop()

			// Store request to cancel.
			r := loadRun()
			r.OngoingLongOps.GetOps()[opID].CancelRequested = true
			assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)

			ct.Clock.Add(time.Minute)
			// Must be done soon.
			assertDone()
			// Now, the cancellation request must be noticed.
			assert.Loosely(t, l.check(), should.BeTrue)
		})

		t.Run("Robust in case of edge cases which should not happen in practice", func(t *ftt.Test) {
			t.Run("Notices Run losing track of this long operation", func(t *ftt.Test) {
				stop := l.start(ctx, loadRun(), opID)
				defer stop()

				r := loadRun()
				r.OngoingLongOps = nil
				assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)

				ct.Clock.Add(time.Minute)
				// Must be done soon.
				assertDone()
				// Treat it as if the long op was requested to be cancelled.
				assert.Loosely(t, l.check(), should.BeTrue)
			})

			t.Run("Notices Run deletion", func(t *ftt.Test) {
				stop := l.start(ctx, loadRun(), opID)
				defer stop()

				assert.Loosely(t, datastore.Delete(ctx, loadRun()), should.BeNil)

				ct.Clock.Add(time.Minute)
				// Must be done soon.
				assertDone()
				// Treat it as if the long op was requested to be cancelled.
				assert.Loosely(t, l.check(), should.BeTrue)
			})
		})

		t.Run("Background goroutine lifetime is bounded", func(t *ftt.Test) {
			t.Run("by calling stop()", func(t *ftt.Test) {
				stop := l.start(ctx, loadRun(), opID)
				assertNotDone()
				stop()
				assertDone()
				assert.Loosely(t, l.check(), should.BeFalse) // the long op is still not cancelled
			})

			t.Run("by context", func(t *ftt.Test) {
				t.Run("when context expires", func(t *ftt.Test) {
					innerCtx, ctxCancel := clock.WithTimeout(ctx, time.Minute)
					defer ctxCancel() // to cleanup test resources, not actually relevant to the test
					stop := l.start(innerCtx, loadRun(), opID)
					defer stop()
					ct.Clock.Add(time.Hour) // expire the innerCtx
					assertDone()
					assert.Loosely(t, l.check(), should.BeFalse) // the long op is still not cancelled
				})

				t.Run("context is cancelled", func(t *ftt.Test) {
					innerCtx, ctxCancel := clock.WithTimeout(ctx, time.Minute)
					stop := l.start(innerCtx, loadRun(), opID)
					defer stop()
					ctxCancel()
					assertDone()
					assert.Loosely(t, l.check(), should.BeFalse) // the long op is still not cancelled
				})
			})
		})
	})
}
