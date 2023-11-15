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

package prjpb

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
)

const (
	// PMTaskInterval is target frequency of executions of ManageProjectTask.
	//
	// See Dispatch() for details.
	PMTaskInterval = time.Second

	// MaxAcceptableDelay prevents TQ tasks which arrive too late from invoking PM.
	//
	// MaxAcceptableDelay / PMTaskInterval effectively limits # concurrent
	// invocations of PM on the same project that may happen due to task retries,
	// delays, and queue throttling.
	//
	// Do not set too low, as this may prevent actual PM invoking from happening at
	// all if the TQ is overloaded.
	MaxAcceptableDelay = 60 * time.Second

	ManageProjectTaskClass        = "manage-project"
	KickManageProjectTaskClass    = "kick-" + ManageProjectTaskClass
	PurgeProjectCLTaskClass       = "purge-project-cl"
	TriggerProjectCLDepsTaskClass = "trigger-project-cl-deps"
)

// TasksBinding binds Project Manager tasks to a TQ Dispatcher.
//
// This struct exists to separate task creation and handling,
// which in turns avoids circular dependency.
type TasksBinding struct {
	ManageProject        tq.TaskClassRef
	KickManageProject    tq.TaskClassRef
	PurgeProjectCL       tq.TaskClassRef
	TriggerProjectCLs    tq.TaskClassRef
	TriggerProjectCLDeps tq.TaskClassRef
	TQDispatcher         *tq.Dispatcher
}

func Register(tqd *tq.Dispatcher) TasksBinding {
	return TasksBinding{
		TQDispatcher: tqd,

		ManageProject: tqd.RegisterTaskClass(tq.TaskClass{
			ID:           ManageProjectTaskClass,
			Prototype:    &ManageProjectTask{},
			Queue:        "manage-project",
			Kind:         tq.NonTransactional,
			Quiet:        true,
			QuietOnError: true,
		}),
		KickManageProject: tqd.RegisterTaskClass(tq.TaskClass{
			ID:           KickManageProjectTaskClass,
			Prototype:    &KickManageProjectTask{},
			Queue:        "kick-manage-project",
			Kind:         tq.Transactional,
			Quiet:        true,
			QuietOnError: true,
		}),
		PurgeProjectCL: tqd.RegisterTaskClass(tq.TaskClass{
			ID:           PurgeProjectCLTaskClass,
			Prototype:    &PurgeCLTask{},
			Queue:        "purge-project-cl",
			Kind:         tq.Transactional,
			Quiet:        false, // these tasks are rare enough that verbosity only helps.
			QuietOnError: true,
		}),
		TriggerProjectCLDeps: tqd.RegisterTaskClass(tq.TaskClass{
			ID:           TriggerProjectCLDepsTaskClass,
			Prototype:    &TriggeringCLDepsTask{},
			Queue:        "trigger-project-cl-deps",
			Kind:         tq.Transactional,
			Quiet:        true,
			QuietOnError: true,
		}),
	}
}

// Dispatch ensures invocation of ProjectManager via ManageProjectTask.
//
// ProjectManager will be invoked at approximately no earlier than both:
// * eta time
// * next possible.
//
// To avoid actually dispatching TQ tasks in tests, use pmtest.MockDispatch().
func (tr TasksBinding) Dispatch(ctx context.Context, luciProject string, eta time.Time) error {
	mock, mocked := ctx.Value(&mockDispatcherContextKey).(func(string, time.Time))

	if datastore.CurrentTransaction(ctx) != nil {
		// TODO(tandrii): use txndefer to immediately trigger a ManageProjectTask after
		// transaction completes to reduce latency in *most* circumstances.
		// The KickManageProjectTask is still required for correctness.
		payload := &KickManageProjectTask{LuciProject: luciProject}
		if !eta.IsZero() {
			payload.Eta = timestamppb.New(eta)
		}

		if mocked {
			mock(luciProject, eta)
			return nil
		}
		return tr.TQDispatcher.AddTask(ctx, &tq.Task{
			Title:            luciProject,
			DeduplicationKey: "", // not allowed in a transaction
			Payload:          payload,
		})
	}

	// If actual local clock is more than `clockDrift` behind, the "next" computed
	// ManageProjectTask moment might be already executing, meaning task dedup
	// will ensure no new task will be scheduled AND the already executing run
	// might not have read the Event that was just written.
	// Thus, this should be large for safety. However, large value leads to higher
	// latency of event processing of non-busy ProjectManagers.
	// TODO(tandrii): this can be reduced significantly once safety "ping" events
	// are originated from Config import cron tasks.
	const clockDrift = 100 * time.Millisecond
	now := clock.Now(ctx).Add(clockDrift) // Use the worst possible time.
	if eta.IsZero() || eta.Before(now) {
		eta = now
	}
	eta = eta.Truncate(PMTaskInterval).Add(PMTaskInterval)

	if mocked {
		mock(luciProject, eta)
		return nil
	}
	return tr.TQDispatcher.AddTask(ctx, &tq.Task{
		Title:            luciProject,
		DeduplicationKey: fmt.Sprintf("%s\n%d", luciProject, eta.UnixNano()),
		ETA:              eta,
		Payload:          &ManageProjectTask{LuciProject: luciProject, Eta: timestamppb.New(eta)},
	})
}

var mockDispatcherContextKey = "prjpb.mockDispatcher"

// InstallMockDispatcher is used in test to run tests emitting PM events without
// actually dispatching PM tasks.
//
// See pmtest.MockDispatch().
func InstallMockDispatcher(ctx context.Context, f func(luciProject string, eta time.Time)) context.Context {
	return context.WithValue(ctx, &mockDispatcherContextKey, f)
}
