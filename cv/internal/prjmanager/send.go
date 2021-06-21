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

package prjmanager

import (
	"context"
	"time"

	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// Notifier notifies PM.
type Notifier struct {
	// TasksBinding are used to register handlers of PM Implementation & CL Purger to
	// avoid circular dependency.
	TasksBinding prjpb.TasksBinding
}

// NewNotifier creates a new PM notifier and registers it in the provided
// tq.Dispatcher.
func NewNotifier(tqd *tq.Dispatcher) *Notifier {
	return &Notifier{TasksBinding: prjpb.Register(tqd)}
}

// UpdateConfig tells ProjectManager to read and update to newest ProjectConfig
// by fetching it from Datatstore.
//
// Results in stopping ProjectManager if ProjectConfig got disabled or deleted.
func (n *Notifier) UpdateConfig(ctx context.Context, luciProject string) error {
	return n.TasksBinding.SendNow(ctx, luciProject, &prjpb.Event{
		Event: &prjpb.Event_NewConfig{
			NewConfig: &prjpb.NewConfig{},
		},
	})
}

// Poke tells ProjectManager to poke all downstream actors and check its own
// state.
func (n *Notifier) Poke(ctx context.Context, luciProject string) error {
	return n.TasksBinding.SendNow(ctx, luciProject, &prjpb.Event{
		Event: &prjpb.Event_Poke{
			Poke: &prjpb.Poke{},
		},
	})
}

// NotifyCLUpdated tells ProjectManager to check latest version of a given CL.
func (n *Notifier) NotifyCLUpdated(ctx context.Context, luciProject string, clid common.CLID, eversion int) error {
	return n.TasksBinding.SendNow(ctx, luciProject, &prjpb.Event{
		Event: &prjpb.Event_ClUpdated{
			ClUpdated: &prjpb.CLUpdated{
				Clid:     int64(clid),
				Eversion: int64(eversion),
			},
		},
	})
}

// NotifyCLsUpdated is a batch of NotifyCLUpdated for the same ProjectManager.
//
// In each given CL, .ID and .EVersion must be set.
func (n *Notifier) NotifyCLsUpdated(ctx context.Context, luciProject string, cls []*changelist.CL) error {
	return n.TasksBinding.SendNow(ctx, luciProject, &prjpb.Event{
		Event: &prjpb.Event_ClsUpdated{
			ClsUpdated: prjpb.MakeCLsUpdated(cls),
		},
	})
}

// NotifyPurgeCompleted tells ProjectManager that a CL purge has completed.
//
// The ultimate result of CL purge is the updated state of a CL itself, thus no
// information is provided here.
//
// TODO(tandrii): remove eta parameter once CV does all the purging.
func (n *Notifier) NotifyPurgeCompleted(ctx context.Context, luciProject string, operationID string, eta time.Time) error {
	err := prjpb.Send(ctx, luciProject, &prjpb.Event{
		Event: &prjpb.Event_PurgeCompleted{
			PurgeCompleted: &prjpb.PurgeCompleted{
				OperationId: operationID,
			},
		},
	})
	if err != nil {
		return err
	}
	return n.TasksBinding.Dispatch(ctx, luciProject, eta)
}

// NotifyRunCreated is sent by ProjectManager to itself within a Run creation
// transaction.
//
// Unlike other event-sending of Notifier, this one only creates an event and
// doesn't create a task. This is fine because:
//   * if Run creation transaction fails, then this event isn't actually
//     created anyways.
//   * if ProjectManager observes the Run creation success, then it'll act as if
//     this event was received in the upcoming state transition. Yes, it won't
//     process this event immediately, but at this point the event is a noop,
//     so it'll be cleared out from the eventbox upon next invocation of
//     ProjectManager. So there is no need to create a TQ task.
//   * else, namely Run creation succeeds but ProjectManager sees it as a
//     failure OR ProjectManager fails at any point before it can act on
//     RunCreation, then the existing TQ task running ProjectManager will be
//     retried. So once again there is no need to create a TQ task.
func (n *Notifier) NotifyRunCreated(ctx context.Context, runID common.RunID) error {
	return prjpb.Send(ctx, runID.LUCIProject(), &prjpb.Event{
		Event: &prjpb.Event_RunCreated{
			RunCreated: &prjpb.RunCreated{
				RunId: string(runID),
			},
		},
	})
}

// NotifyRunFinished tells ProjectManager that a run has finalized its state.
func (n *Notifier) NotifyRunFinished(ctx context.Context, runID common.RunID) error {
	return n.TasksBinding.SendNow(ctx, runID.LUCIProject(), &prjpb.Event{
		Event: &prjpb.Event_RunFinished{
			RunFinished: &prjpb.RunFinished{
				RunId: string(runID),
			},
		},
	})
}
