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

package run

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/run/eventpb"
)

// Notifier notifies Run Manager.
type Notifier struct {
	// TasksBinding are used to register handlers of RM implementation to avoid
	// circular dependency.
	TasksBinding eventpb.TasksBinding
}

func NewNotifier(tqd *tq.Dispatcher) *Notifier {
	return &Notifier{TasksBinding: eventpb.Register(tqd)}
}

// Invoke invokes Run Manager to process events at the provided `eta`.
//
// If the provided `eta` is zero, invokes immediately.
func (n *Notifier) Invoke(ctx context.Context, runID common.RunID, eta time.Time) error {
	return n.TasksBinding.Dispatch(ctx, string(runID), eta)
}

// Start tells RunManager to start the given run.
func (n *Notifier) Start(ctx context.Context, runID common.RunID) error {
	return n.SendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_Start{
			Start: &eventpb.Start{},
		},
	})
}

// PokeNow tells RunManager to check its own state immediately.
//
// It's a shorthand of `PokeAfter(ctx, runID, after)` where `after` <= 0 or
// `PokeAt(ctx, runID, eta)` where `eta` is an earlier timestamp.
func (n *Notifier) PokeNow(ctx context.Context, runID common.RunID) error {
	return n.PokeAfter(ctx, runID, 0)
}

// PokeAfter tells RunManager to check its own state after the given duration.
//
// Providing a non-positive duration is equivalent to `PokeNow(...)`.
func (n *Notifier) PokeAfter(ctx context.Context, runID common.RunID, after time.Duration) error {
	evt := &eventpb.Event{
		Event: &eventpb.Event_Poke{
			Poke: &eventpb.Poke{},
		},
	}
	if after > 0 {
		t := clock.Now(ctx).Add(after)
		evt.ProcessAfter = timestamppb.New(t)
		return n.Send(ctx, runID, evt, t)
	}
	return n.SendNow(ctx, runID, evt)
}

// PokeAt tells RunManager to check its own state at around `eta`.
//
// Guarantees no earlier than `eta` but may not be exactly at `eta`.
// Providing an earlier timestamp than the current is equivalent to
// `PokeNow(...)`.
func (n *Notifier) PokeAt(ctx context.Context, runID common.RunID, eta time.Time) error {
	evt := &eventpb.Event{
		Event: &eventpb.Event_Poke{
			Poke: &eventpb.Poke{},
		},
	}
	if eta.After(clock.Now(ctx)) {
		evt.ProcessAfter = timestamppb.New(eta)
		return n.Send(ctx, runID, evt, eta)
	}
	return n.SendNow(ctx, runID, evt)
}

// UpdateConfig tells RunManager to update the given Run to new config.
func (n *Notifier) UpdateConfig(ctx context.Context, runID common.RunID, hash string, eversion int64) error {
	return n.SendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_NewConfig{
			NewConfig: &eventpb.NewConfig{
				Hash:     hash,
				Eversion: eversion,
			},
		},
	})
}

// Cancel tells RunManager to cancel the given Run.
//
// TODO(yiwzhang,tandrii): support reason.
func (n *Notifier) Cancel(ctx context.Context, runID common.RunID) error {
	return n.SendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_Cancel{
			Cancel: &eventpb.Cancel{},
		},
	})
}

// CancelAt tells RunManager to cancel the given Run at `eta`.
//
// TODO(crbug/1141880): Remove this API after migration. This is only needed
// because CV need to delay the cancellation of a Run when waiting for CQD
// report finished Run when CQD is in charge.
func (n *Notifier) CancelAt(ctx context.Context, runID common.RunID, eta time.Time) error {
	evt := &eventpb.Event{
		Event: &eventpb.Event_Cancel{
			Cancel: &eventpb.Cancel{},
		},
	}
	if eta.After(clock.Now(ctx)) {
		evt.ProcessAfter = timestamppb.New(eta)
		return n.Send(ctx, runID, evt, eta)
	}
	return n.SendNow(ctx, runID, evt)
}

// NotifyCLUpdated informs RunManager that given CL has a new version available.
func (n *Notifier) NotifyCLUpdated(ctx context.Context, runID common.RunID, clid common.CLID, eVersion int) error {
	return n.SendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_ClUpdated{
			ClUpdated: &eventpb.CLUpdated{
				Clid:     int64(clid),
				EVersion: int64(eVersion),
			},
		},
	})
}

// NotifyReadyForSubmission informs RunManager that the provided Run will be
// ready for submission at `eta`.
func (n *Notifier) NotifyReadyForSubmission(ctx context.Context, runID common.RunID, eta time.Time) error {
	evt := &eventpb.Event{
		Event: &eventpb.Event_ReadyForSubmission{
			ReadyForSubmission: &eventpb.ReadyForSubmission{},
		},
	}
	if eta.IsZero() {
		return n.SendNow(ctx, runID, evt)
	}
	evt.ProcessAfter = timestamppb.New(eta)
	return n.Send(ctx, runID, evt, eta)
}

// NotifyCLSubmitted informs RunManager that the provided CL is submitted.
//
// Unlike other event-sending funcs, this function only delivers the event
// to Run's eventbox, but does not dispatch the task. This is because it is
// okay to process all events of this kind together to record the submission
// result for each individual CLs after submission completes.
// Waking up RM unnecessarily may increase the contention of Run entity.
func (n *Notifier) NotifyCLSubmitted(ctx context.Context, runID common.RunID, clid common.CLID) error {
	return n.sendWithoutDispatch(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_ClSubmitted{
			ClSubmitted: &eventpb.CLSubmitted{
				Clid: int64(clid),
			},
		},
	})
}

// NotifySubmissionCompleted informs RunManager that the submission of the
// provided Run has completed.
func (n *Notifier) NotifySubmissionCompleted(ctx context.Context, runID common.RunID, sc *eventpb.SubmissionCompleted, invokeRM bool) error {
	evt := &eventpb.Event{
		Event: &eventpb.Event_SubmissionCompleted{
			SubmissionCompleted: sc,
		},
	}
	if invokeRM {
		return n.SendNow(ctx, runID, evt)
	}
	return n.sendWithoutDispatch(ctx, runID, evt)
}

// NotifyCQDVerificationCompleted tells RunManager that CQDaemon has completed
// verifying the provided Run.
//
// TODO(crbug/1141880): Remove this event after migration.
func (n *Notifier) NotifyCQDVerificationCompleted(ctx context.Context, runID common.RunID) error {
	return n.SendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_CqdVerificationCompleted{
			CqdVerificationCompleted: &eventpb.CQDVerificationCompleted{},
		},
	})
}

// NotifyCQDFinished tells RunManager that CQDaemon has finished the provided
// Run.
//
// TODO(crbug/1224170): Remove this event after migration.
func (n *Notifier) NotifyCQDFinished(ctx context.Context, runID common.RunID) error {
	return n.SendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_CqdFinished{
			CqdFinished: &eventpb.CQDFinished{},
		},
	})
}

// SendNow sends the event to Run's eventbox and invokes RunManager immediately.
func (n *Notifier) SendNow(ctx context.Context, runID common.RunID, evt *eventpb.Event) error {
	return n.Send(ctx, runID, evt, time.Time{})
}

// Send sends the event to Run's eventbox and invokes RunManager at `eta`.
func (n *Notifier) Send(ctx context.Context, runID common.RunID, evt *eventpb.Event, eta time.Time) error {
	if err := n.sendWithoutDispatch(ctx, runID, evt); err != nil {
		return err
	}
	return n.TasksBinding.Dispatch(ctx, string(runID), eta)
}

// sendWithoutDispatch sends the event to Run's eventbox without invoking RM.
func (n *Notifier) sendWithoutDispatch(ctx context.Context, runID common.RunID, evt *eventpb.Event) error {
	value, err := proto.Marshal(evt)
	if err != nil {
		return errors.Annotate(err, "failed to marshal").Err()
	}
	to := datastore.MakeKey(ctx, RunKind, string(runID))
	return eventbox.Emit(ctx, value, to)
}
