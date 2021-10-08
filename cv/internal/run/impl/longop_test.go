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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
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
		ctx, cancel := ct.SetUp()
		defer cancel()
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
			&eventpb.DoLongOpTask{
				OperationId: opID,
				RunId:       runID,
			},
		})

		Convey("manager handles Long Operation TQ task", func() {
			manager := New(notifier, nil, nil, nil, nil, nil, nil, ct.Env)

			Convey("OK", func() {
				called := false
				manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *run.Run, _ *run.OngoingLongOps_Op) (*eventpb.LongOpCompleted, error) {
					called = true
					d, ok := ctx.Deadline()
					So(ok, ShouldBeTrue)
					So(d.UTC(), ShouldResemble, op.GetDeadline().AsTime())
					return &eventpb.LongOpCompleted{}, nil
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.DoLongOpTaskClass))
				So(called, ShouldBeTrue)
				runtest.AssertReceivedLongOpCompleted(ctx, runID, &eventpb.LongOpCompleted{OperationId: opID})
			})

			Convey("Expired long op must not be executed, but Run Manager should be notified", func() {
				ct.Clock.Add(time.Hour)
				called := false
				manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *run.Run, _ *run.OngoingLongOps_Op) (*eventpb.LongOpCompleted, error) {
					called = true
					return &eventpb.LongOpCompleted{}, nil
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.DoLongOpTaskClass))
				So(called, ShouldBeFalse)
				runtest.AssertReceivedLongOpCompleted(ctx, runID, &eventpb.LongOpCompleted{OperationId: opID})
			})

			Convey("Transient failure is retried", func() {
				called := 0
				manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *run.Run, _ *run.OngoingLongOps_Op) (*eventpb.LongOpCompleted, error) {
					called++
					if called == 1 {
						return nil, errors.New("troops", transient.Tag)
					}
					return &eventpb.LongOpCompleted{}, nil
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.DoLongOpTaskClass))
				So(called, ShouldEqual, 2)
				runtest.AssertReceivedLongOpCompleted(ctx, runID, &eventpb.LongOpCompleted{OperationId: opID})
			})

			Convey("Non-transient failure is fatal", func() {
				called := 0
				manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *run.Run, _ *run.OngoingLongOps_Op) (*eventpb.LongOpCompleted, error) {
					called++
					if called == 1 {
						return nil, errors.New("foops")
					}
					return &eventpb.LongOpCompleted{}, nil
				}
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.DoLongOpTaskClass))
				So(called, ShouldEqual, 1)
				runtest.AssertReceivedLongOpCompleted(ctx, runID, &eventpb.LongOpCompleted{OperationId: opID})
			})

			Convey("Doesn't execute in weird cases", func() {
				Convey("Run deleted", func() {
					So(datastore.Delete(ctx, &run.Run{ID: runID}), ShouldBeNil)
					called := false
					manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *run.Run, _ *run.OngoingLongOps_Op) (*eventpb.LongOpCompleted, error) {
						called = true
						return &eventpb.LongOpCompleted{}, nil
					}
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.DoLongOpTaskClass))
					So(called, ShouldBeFalse)
					runtest.AssertEventboxEmpty(ctx, runID)
				})
				Convey("Long op is no longer known", func() {
					r := loadRun(ctx)
					r.OngoingLongOps = nil
					So(datastore.Put(ctx, r), ShouldBeNil)
					called := false
					manager.testDoLongOperationWithDeadline = func(ctx context.Context, _ *run.Run, _ *run.OngoingLongOps_Op) (*eventpb.LongOpCompleted, error) {
						called = true
						return &eventpb.LongOpCompleted{}, nil
					}
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(eventpb.DoLongOpTaskClass))
					So(called, ShouldBeFalse)
					runtest.AssertEventboxEmpty(ctx, runID)
				})
			})
		})
	})
}
