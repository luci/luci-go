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

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// EventboxRecipient returns eventbox.Recipient for a given Run.
func EventboxRecipient(ctx context.Context, runID common.RunID) eventbox.Recipient {
	return eventbox.Recipient{
		Key: datastore.MakeKey(ctx, common.RunKind, string(runID)),
		// There are lots of Runs, so aggregate all their metrics behind their LUCI
		// project.
		MonitoringString: "Run/" + runID.LUCIProject(),
	}
}

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
func (n *Notifier) Cancel(ctx context.Context, runID common.RunID, reason string) error {
	return n.SendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_Cancel{
			Cancel: &eventpb.Cancel{
				Reason: reason,
			},
		},
	})
}

// NotifyCLsUpdated informs RunManager that given CLs have new versions
// available.
func (n *Notifier) NotifyCLsUpdated(ctx context.Context, runID common.RunID, cls *changelist.CLUpdatedEvents) error {
	return n.SendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_ClsUpdated{
			ClsUpdated: cls,
		},
	})
}

// NotifyTryjobsUpdated tells RunManager that tryjobs entities were updated.
func (n *Notifier) NotifyTryjobsUpdated(ctx context.Context, runID common.RunID, tryjobs *tryjob.TryjobUpdatedEvents) error {
	return n.SendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_TryjobsUpdated{
			TryjobsUpdated: tryjobs,
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

// NotifyCLsSubmitted informs RunManager that the provided CLs are submitted.
//
// Unlike other event-sending funcs, this function only delivers the event
// to Run's eventbox, but does not dispatch the task. This is because it is
// okay to process all events of this kind together to record the submission
// result for each individual CLs after submission completes.
// Waking up RM unnecessarily may increase the contention of Run entity.
func (n *Notifier) NotifyCLsSubmitted(ctx context.Context, runID common.RunID, clids common.CLIDs) error {
	return n.sendWithoutDispatch(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_ClsSubmitted{
			ClsSubmitted: &eventpb.CLsSubmitted{
				Clids: common.CLIDsAsInt64s(clids),
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

// NotifyLongOpCompleted tells RunManager that a long operation has completed.
func (n *Notifier) NotifyLongOpCompleted(ctx context.Context, runID common.RunID, res *eventpb.LongOpCompleted) error {
	return n.SendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_LongOpCompleted{
			LongOpCompleted: res,
		},
	})
}

// NotifyParentRunCompleted tells RunManager that a parent run has completed.
func (n *Notifier) NotifyParentRunCompleted(ctx context.Context, runID common.RunID) error {
	evt := &eventpb.Event{
		Event: &eventpb.Event_ParentRunCompleted{
			ParentRunCompleted: &eventpb.ParentRunCompleted{},
		},
	}
	return n.SendNow(ctx, runID, evt)
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
	return eventbox.Emit(ctx, value, EventboxRecipient(ctx, runID))
}
