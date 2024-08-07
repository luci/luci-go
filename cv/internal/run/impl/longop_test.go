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
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/longops"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/runtest"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/retry/transient"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestLongOps(t *testing.T) {
	t.Parallel()

	Convey("Manager handles long ops", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		const runID = "chromium/222-1-deadbeef"
		const initialEVersion = 10
		So(datastore.Put(ctx, &run.Run{
			ID:       runID,
			Status:   run.Status_RUNNING,
			EVersion: initialEVersion,
		}), ShouldBeNil)

		loadRun := func(ctx context.Context) *run.Run {
			ret := &run.Run{ID: runID}
			So(datastore.Get(ctx, ret), ShouldBeNil)
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
		So(rs.OngoingLongOps.GetOps(), ShouldHaveLength, 1)

		// Simulate what happens when Run state is transactionally updated.
		So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			return proc.SaveState(ctx, eventbox.State(rs), eventbox.EVersion(rs.EVersion+1))
		}, nil), ShouldBeNil)

		// Verify that Run's state records new Long operation.
		r := loadRun(ctx)
		So(r.OngoingLongOps.GetOps(), ShouldHaveLength, 1)
		op := r.OngoingLongOps.GetOps()[opID]
		So(op, ShouldResembleProto, &run.OngoingLongOps_Op{
			CancelRequested: false,
			Deadline:        timestamppb.New(ct.Clock.Now().Add(time.Minute)),
			Work: &run.OngoingLongOps_Op_PostStartMessage{
				PostStartMessage: true,
			},
		})
		// Verify that long op task was enqueued.
		So(ct.TQ.Tasks().Payloads(), ShouldResembleProto, []proto.Message{
			&eventpb.ManageRunLongOpTask{
				OperationId: opID,
				RunId:       runID,
			},
		})

		Convey("manager handles Long Operation TQ task", func() {
			manager := New(notifier, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ct.Env)

			Convey("OK", func() {
				called := false
				manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *longops.Base) (*eventpb.LongOpCompleted, error) {
					called = true
					d, ok := ctx.Deadline()
					So(ok, ShouldBeTrue)
					So(d.UTC(), ShouldResemble, op.GetDeadline().AsTime())
					return &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_SUCCEEDED}, nil
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
				So(called, ShouldBeTrue)
				runtest.AssertReceivedLongOpCompleted(ctx, runID, &eventpb.LongOpCompleted{
					OperationId: opID,
					Status:      eventpb.LongOpCompleted_SUCCEEDED,
				})
			})

			Convey("CancelRequested handling", func() {
				rs := &state.RunState{Run: *loadRun(ctx)}
				rs.RequestLongOpCancellation(opID)
				rs.EVersion++
				So(datastore.Put(ctx, &rs.Run), ShouldBeNil)

				called := false
				manager.testDoLongOperationWithDeadline = func(ctx context.Context, opBase *longops.Base) (*eventpb.LongOpCompleted, error) {
					called = true
					So(opBase.IsCancelRequested(), ShouldBeTrue)
					return &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_CANCELLED}, nil
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
				So(called, ShouldBeTrue)
				runtest.AssertReceivedLongOpCompleted(ctx, runID, &eventpb.LongOpCompleted{
					OperationId: opID,
					Status:      eventpb.LongOpCompleted_CANCELLED,
				})
			})

			Convey("Expired long op must not be executed, but Run Manager should be notified", func() {
				ct.Clock.Add(time.Hour)
				called := false
				manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *longops.Base) (*eventpb.LongOpCompleted, error) {
					called = true
					return &eventpb.LongOpCompleted{}, nil
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
				So(called, ShouldBeFalse)
				runtest.AssertReceivedLongOpCompleted(ctx, runID, &eventpb.LongOpCompleted{
					OperationId: opID,
					Status:      eventpb.LongOpCompleted_EXPIRED,
				})
			})

			Convey("Expired while executing", func() {
				called := false
				manager.testDoLongOperationWithDeadline = func(dctx context.Context, _ *longops.Base) (*eventpb.LongOpCompleted, error) {
					called = true
					ct.Clock.Add(time.Hour) // expire the `dctx`
					// NOTE: it's unclear why the following sometimes fails:
					for dctx.Err() == nil {
						clock.Sleep(dctx, time.Second)
					}
					So(dctx.Err(), ShouldNotBeNil)
					return nil, errors.Annotate(dctx.Err(), "somehow treating as permanent failure").Err()
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
				So(called, ShouldBeTrue)
				runtest.AssertReceivedLongOpCompleted(ctx, runID, &eventpb.LongOpCompleted{
					OperationId: opID,
					Status:      eventpb.LongOpCompleted_EXPIRED,
				})
			})

			Convey("Transient failure is retried", func() {
				called := 0
				manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *longops.Base) (*eventpb.LongOpCompleted, error) {
					called++
					if called == 1 {
						return nil, errors.New("troops", transient.Tag)
					}
					return &eventpb.LongOpCompleted{Status: eventpb.LongOpCompleted_SUCCEEDED}, nil
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
				So(called, ShouldEqual, 2)
				runtest.AssertReceivedLongOpCompleted(ctx, runID, &eventpb.LongOpCompleted{
					OperationId: opID,
					Status:      eventpb.LongOpCompleted_SUCCEEDED,
				})
			})

			Convey("Non-transient failure is fatal", func() {
				called := 0
				manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *longops.Base) (*eventpb.LongOpCompleted, error) {
					called++
					if called == 1 {
						return nil, errors.New("foops")
					}
					return &eventpb.LongOpCompleted{}, nil
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
				So(called, ShouldEqual, 1)
				runtest.AssertReceivedLongOpCompleted(ctx, runID, &eventpb.LongOpCompleted{
					OperationId: opID,
					Status:      eventpb.LongOpCompleted_FAILED,
				})
			})

			Convey("Doesn't execute in weird cases", func() {
				Convey("Run deleted", func() {
					So(datastore.Delete(ctx, &run.Run{ID: runID}), ShouldBeNil)
					called := false
					manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *longops.Base) (*eventpb.LongOpCompleted, error) {
						called = true
						return &eventpb.LongOpCompleted{}, nil
					}
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
					So(called, ShouldBeFalse)
					runtest.AssertEventboxEmpty(ctx, runID)
				})
				Convey("Long op is no longer known", func() {
					r := loadRun(ctx)
					r.OngoingLongOps = nil
					So(datastore.Put(ctx, r), ShouldBeNil)
					called := false
					manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *longops.Base) (*eventpb.LongOpCompleted, error) {
						called = true
						return &eventpb.LongOpCompleted{}, nil
					}
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.ManageRunLongOpTaskClass))
					So(called, ShouldBeFalse)
					runtest.AssertEventboxEmpty(ctx, runID)
				})
			})
		})
	})
}

func TestLongOpCancellationChecker(t *testing.T) {
	t.Parallel()

	Convey("longOpCancellationChecker works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		const runID = "chromium/222-1-deadbeef"
		const opID = "op-1"

		So(datastore.Put(ctx, &run.Run{
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
		}), ShouldBeNil)

		loadRun := func() *run.Run {
			ret := &run.Run{ID: runID}
			So(datastore.Get(ctx, ret), ShouldBeNil)
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
				So("context expired before background goroutine was done", ShouldBeFalse)
			}
		}
		assertNotDone := func() {
			select {
			case <-done:
				So("background goroutine is done", ShouldBeFalse)
			default:
			}
		}

		l := longOpCancellationChecker{
			interval: time.Second,
			testChan: done,
		}

		Convey("Normal operation without long op cancellation", func() {
			stop := l.start(ctx, loadRun(), opID)
			defer stop()
			So(l.check(), ShouldBeFalse)
			ct.Clock.Add(time.Minute)
			So(l.check(), ShouldBeFalse)
			assertNotDone()
		})

		Convey("Initial Run state with cancellation request is noticed immediately", func() {
			r := loadRun()
			r.OngoingLongOps.GetOps()[opID].CancelRequested = true
			So(datastore.Put(ctx, r), ShouldBeNil)

			stop := l.start(ctx, loadRun(), opID)
			defer stop()

			So(l.check(), ShouldBeTrue)
			// Background goroutine shouldn't even be started, hence the `done`
			// channel should remain open.
			assertNotDone()
		})

		Convey("Notices cancellation request eventually", func() {
			stop := l.start(ctx, loadRun(), opID)
			defer stop()

			// Store request to cancel.
			r := loadRun()
			r.OngoingLongOps.GetOps()[opID].CancelRequested = true
			So(datastore.Put(ctx, r), ShouldBeNil)

			ct.Clock.Add(time.Minute)
			// Must be done soon.
			assertDone()
			// Now, the cancellation request must be noticed.
			So(l.check(), ShouldBeTrue)
		})

		Convey("Robust in case of edge cases which should not happen in practice", func() {
			Convey("Notices Run losing track of this long operation", func() {
				stop := l.start(ctx, loadRun(), opID)
				defer stop()

				r := loadRun()
				r.OngoingLongOps = nil
				So(datastore.Put(ctx, r), ShouldBeNil)

				ct.Clock.Add(time.Minute)
				// Must be done soon.
				assertDone()
				// Treat it as if the long op was requested to be cancelled.
				So(l.check(), ShouldBeTrue)
			})

			Convey("Notices Run deletion", func() {
				stop := l.start(ctx, loadRun(), opID)
				defer stop()

				So(datastore.Delete(ctx, loadRun()), ShouldBeNil)

				ct.Clock.Add(time.Minute)
				// Must be done soon.
				assertDone()
				// Treat it as if the long op was requested to be cancelled.
				So(l.check(), ShouldBeTrue)
			})
		})

		Convey("Background goroutine lifetime is bounded", func() {
			Convey("by calling stop()", func() {
				stop := l.start(ctx, loadRun(), opID)
				assertNotDone()
				stop()
				assertDone()
				So(l.check(), ShouldBeFalse) // the long op is still not cancelled
			})

			Convey("by context", func() {
				Convey("when context expires", func() {
					innerCtx, ctxCancel := clock.WithTimeout(ctx, time.Minute)
					defer ctxCancel() // to cleanup test resources, not actually relevant to the test
					stop := l.start(innerCtx, loadRun(), opID)
					defer stop()
					ct.Clock.Add(time.Hour) // expire the innerCtx
					assertDone()
					So(l.check(), ShouldBeFalse) // the long op is still not cancelled
				})

				Convey("context is cancelled", func() {
					innerCtx, ctxCancel := clock.WithTimeout(ctx, time.Minute)
					stop := l.start(innerCtx, loadRun(), opID)
					defer stop()
					ctxCancel()
					assertDone()
					So(l.check(), ShouldBeFalse) // the long op is still not cancelled
				})
			})
		})
	})
}
