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

package eventpb

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
)

const (
	// ManageRunTaskClass is the ID of ManageRunTask Class.
	ManageRunTaskClass = "manage-run"
	// taskInterval is target frequency of executions of ManageRunTask.
	//
	// See Dispatch() for details.
	taskInterval = time.Second
)

// TasksBinding binds Run Manager tasks to a TQ Dispatcher.
//
// This struct exists to separate task creation and handling,
// which in turns avoids circular dependency.
type TasksBinding struct {
	ManageRun    tq.TaskClassRef
	KickManage   tq.TaskClassRef
	TQDispatcher *tq.Dispatcher
}

// Register registers tasks with the given TQ Dispatcher.
func Register(tqd *tq.Dispatcher) TasksBinding {
	t := TasksBinding{
		ManageRun: tqd.RegisterTaskClass(tq.TaskClass{
			ID:        ManageRunTaskClass,
			Prototype: &ManageRunTask{},
			Queue:     "manage-run",
			Kind:      tq.NonTransactional,
			Quiet:     true,
		}),
		KickManage: tqd.RegisterTaskClass(tq.TaskClass{
			ID:        fmt.Sprintf("kick-%s", ManageRunTaskClass),
			Prototype: &KickManageRunTask{},
			Queue:     "kick-manage-run",
			Kind:      tq.Transactional,
			Quiet:     true,
		}),
		TQDispatcher: tqd,
	}
	t.KickManage.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		task := payload.(*KickManageRunTask)
		var eta time.Time
		if t := task.GetEta(); t != nil {
			eta = t.AsTime()
		}
		err := t.Dispatch(ctx, task.GetRunId(), eta)
		return common.TQifyError(ctx, err)
	})
	return t
}

// Dispatch ensures invocation of RunManager via ManageRunTask.
//
// RunManager will be invoked at approximately no earlier than both:
//  * eta time (if given)
//  * next possible.
//
// To avoid actually dispatching TQ tasks in tests, use runtest.MockDispatch().
func (tr TasksBinding) Dispatch(ctx context.Context, runID string, eta time.Time) error {
	mock, mocked := ctx.Value(&mockDispatcherContextKey).(func(string, time.Time))

	if datastore.CurrentTransaction(ctx) != nil {
		payload := &KickManageRunTask{RunId: runID}
		if !eta.IsZero() {
			payload.Eta = timestamppb.New(eta)
		}

		if mocked {
			mock(runID, eta)
			return nil
		}
		return tr.TQDispatcher.AddTask(ctx, &tq.Task{
			DeduplicationKey: "", // not allowed in a transaction
			Payload:          payload,
		})
	}

	// If actual local clock is more than `clockDrift` behind, the "next" computed
	// ManageRunTask moment might be already executing, meaning task dedup will
	// ensure no new task will be scheduled AND the already executing run
	// might not have read the Event that was just written.
	// Thus, for safety, this should be large, however, will also leads to higher
	// latency of event processing of non-busy RunManager.
	// TODO(tandrii/yiwzhang): this can be reduced significantly once safety
	// "ping" events are originated from Config import cron tasks.
	const clockDrift = 100 * time.Millisecond
	now := clock.Now(ctx).Add(clockDrift) // Use the worst possible time.
	if eta.IsZero() || eta.Before(now) {
		eta = now
	}
	eta = eta.Truncate(taskInterval).Add(taskInterval)

	if mocked {
		mock(runID, eta)
		return nil
	}
	return tr.TQDispatcher.AddTask(ctx, &tq.Task{
		Title:            runID,
		DeduplicationKey: fmt.Sprintf("%s\n%d", runID, eta.UnixNano()),
		ETA:              eta,
		Payload:          &ManageRunTask{RunId: runID},
	})
}

var mockDispatcherContextKey = "eventpb.mockDispatcher"

// InstallMockDispatcher is used in test to run tests emitting RM events without
// actually dispatching RM tasks.
//
// See runtest.MockDispatch().
func InstallMockDispatcher(ctx context.Context, f func(runID string, eta time.Time)) context.Context {
	return context.WithValue(ctx, &mockDispatcherContextKey, f)
}

// SendNow sends the event to Run's eventbox and invokes RunManager immediately.
func (tr TasksBinding) SendNow(ctx context.Context, runID common.RunID, evt *Event) error {
	return tr.Send(ctx, runID, evt, time.Time{})
}

// Send sends the event to Run's eventbox and invokes RunManager at `eta`.
func (tr TasksBinding) Send(ctx context.Context, runID common.RunID, evt *Event, eta time.Time) error {
	if err := SendWithoutDispatch(ctx, runID, evt); err != nil {
		return err
	}
	return tr.Dispatch(ctx, string(runID), eta)
}

// SendWithoutDispatch sends the event to Run's eventbox without invoking RM.
func SendWithoutDispatch(ctx context.Context, runID common.RunID, evt *Event) error {
	value, err := proto.Marshal(evt)
	if err != nil {
		return errors.Annotate(err, "failed to marshal").Err()
	}
	to := datastore.MakeKey(ctx, "Run", string(runID))
	return eventbox.Emit(ctx, value, to)
}
