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
	"go.chromium.org/luci/cv/internal/common"
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

// ManageRunTaskRef is used by RunManager implementation to add its handler.
var ManageRunTaskRef tq.TaskClassRef

func init() {
	ManageRunTaskRef = tq.RegisterTaskClass(tq.TaskClass{
		ID:        ManageRunTaskClass,
		Prototype: &ManageRunTask{},
		Queue:     "manage-run",
		Kind:      tq.NonTransactional,
	})

	tq.RegisterTaskClass(tq.TaskClass{
		ID:        fmt.Sprintf("kick-%s", ManageRunTaskClass),
		Prototype: &KickManageRunTask{},
		Queue:     "kick-manage-run",
		Kind:      tq.Transactional,
		Quiet:     true,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*KickManageRunTask)
			var eta time.Time
			if t := task.GetEta(); t != nil {
				eta = t.AsTime()
			}
			err := Dispatch(ctx, task.GetRunId(), eta)
			return common.TQifyError(ctx, err)
		},
	})
}

// Dispatch ensures invocation of RunManager via ManageRunTask.
//
// RunManager will be invoked at approximately no earlier than both:
//  * eta time (if given)
//  * next possible.
//
// To avoid actually dispatching TQ tasks in tests, use runtest.MockDispatch().
func Dispatch(ctx context.Context, runID string, eta time.Time) error {
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
		return tq.AddTask(ctx, &tq.Task{
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
	return tq.AddTask(ctx, &tq.Task{
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
