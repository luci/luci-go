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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
)

// EventboxRecipient returns eventbox.Recipient for a given LUCI project.
func EventboxRecipient(ctx context.Context, luciProject string) eventbox.Recipient {
	return eventbox.Recipient{
		Key:              datastore.MakeKey(ctx, ProjectKind, luciProject),
		MonitoringString: "Project/" + luciProject,
	}
}

// Notifier notifies Project Manager.
type Notifier struct {
	// TasksBinding are used to register handlers of Project Manager Implementation & CL Purger to
	// avoid circular dependency.
	TasksBinding prjpb.TasksBinding
}

// NewNotifier creates a new Project Manager notifier and registers it in the
// provided tq.Dispatcher.
func NewNotifier(tqd *tq.Dispatcher) *Notifier {
	return &Notifier{TasksBinding: prjpb.Register(tqd)}
}

// UpdateConfig tells Project Manager to read and update to newest ProjectConfig
// by fetching it from Datatstore.
//
// Results in stopping Project Manager if ProjectConfig got disabled or deleted.
func (n *Notifier) UpdateConfig(ctx context.Context, luciProject string) error {
	return n.SendNow(ctx, luciProject, &prjpb.Event{
		Event: &prjpb.Event_NewConfig{
			NewConfig: &prjpb.NewConfig{},
		},
	})
}

// Poke tells Project Manager to poke all downstream actors and check its own
// state.
func (n *Notifier) Poke(ctx context.Context, luciProject string) error {
	return n.SendNow(ctx, luciProject, &prjpb.Event{
		Event: &prjpb.Event_Poke{
			Poke: &prjpb.Poke{},
		},
	})
}

// NotifyCLsUpdated tells Project Manager to check latest versions of the given
// CLs.
func (n *Notifier) NotifyCLsUpdated(ctx context.Context, luciProject string, cls *changelist.CLUpdatedEvents) error {
	return n.SendNow(ctx, luciProject, &prjpb.Event{
		Event: &prjpb.Event_ClsUpdated{
			ClsUpdated: cls,
		},
	})
}

// NotifyPurgeCompleted tells Project Manager that a CL purge has completed.
//
// The ultimate result of CL purge is the updated state of a CL itself, thus no
// information is provided here.
func (n *Notifier) NotifyPurgeCompleted(ctx context.Context, luciProject string, purgingCL *prjpb.PurgingCL) error {
	return n.SendNow(ctx, luciProject, &prjpb.Event{
		Event: &prjpb.Event_PurgeCompleted{
			PurgeCompleted: &prjpb.PurgeCompleted{
				OperationId: purgingCL.GetOperationId(),
				Clid:        purgingCL.GetClid(),
			},
		},
	})
}

// NotifyRunCreated is sent by Project Manager to itself within a Run creation
// transaction.
//
// Unlike other event-sending of Notifier, this one only creates an event and
// doesn't create a task. This is fine because:
//   - if Run creation transaction fails, then this event isn't actually
//     created anyways.
//   - if Project Manager observes the Run creation success, then it'll act as if
//     this event was received in the upcoming state transition. Yes, it won't
//     process this event immediately, but at this point the event is a noop,
//     so it'll be cleared out from the eventbox upon next invocation of
//     Project Manager. So there is no need to create a TQ task.
//   - else, namely Run creation succeeds but Project Manager sees it as a
//     failure OR Project Manager fails at any point before it can act on
//     RunCreation, then the existing TQ task running Project Manager will be
//     retried. So once again there is no need to create a TQ task.
func (n *Notifier) NotifyRunCreated(ctx context.Context, runID common.RunID) error {
	return n.sendWithoutDispatch(ctx, runID.LUCIProject(), &prjpb.Event{
		Event: &prjpb.Event_RunCreated{
			RunCreated: &prjpb.RunCreated{
				RunId: string(runID),
			},
		},
	})
}

// NotifyRunFinished tells Project Manager that a run has finalized its state.
func (n *Notifier) NotifyRunFinished(ctx context.Context, runID common.RunID, status run.Status) error {
	return n.SendNow(ctx, runID.LUCIProject(), &prjpb.Event{
		Event: &prjpb.Event_RunFinished{
			RunFinished: &prjpb.RunFinished{
				RunId:  string(runID),
				Status: status,
			},
		},
	})
}

// SendNow sends the event to Project's eventbox and invokes Project Manager
// immediately.
func (n *Notifier) SendNow(ctx context.Context, luciProject string, e *prjpb.Event) error {
	if err := n.sendWithoutDispatch(ctx, luciProject, e); err != nil {
		return err
	}
	return n.TasksBinding.Dispatch(ctx, luciProject, time.Time{} /*asap*/)
}

// sendWithoutDispatch sends the event to Project's eventbox without invoking a
// PM.
func (n *Notifier) sendWithoutDispatch(ctx context.Context, luciProject string, e *prjpb.Event) error {
	value, err := proto.Marshal(e)
	if err != nil {
		return errors.Annotate(err, "failed to marshal").Err()
	}
	return eventbox.Emit(ctx, value, EventboxRecipient(ctx, luciProject))
}

// NotifyTriggeringCLDepsCompleted tells Project Manager CL deps trigger completion.
func (n *Notifier) NotifyTriggeringCLDepsCompleted(ctx context.Context, luciProject string, event *prjpb.TriggeringCLDepsCompleted) error {
	return n.SendNow(ctx, luciProject, &prjpb.Event{
		Event: &prjpb.Event_TriggeringClDepsCompleted{
			TriggeringClDepsCompleted: event,
		},
	})
}
