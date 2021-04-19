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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run/eventpb"
)

// Notifier notifies RUn Manager.
type Notifier struct {
	// TaskRefs are used to register handlers of RM implementation to
	// avoid circular dependency.
	TaskRefs eventpb.TaskRefs
}

func NewNotifier(tqd *tq.Dispatcher) *Notifier {
	return &Notifier{TaskRefs: eventpb.Register(tqd)}
}

// Start tells RunManager to start the given run.
func (n *Notifier) Start(ctx context.Context, runID common.RunID) error {
	return n.TaskRefs.SendNow(ctx, runID, &eventpb.Event{
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
		return n.TaskRefs.Send(ctx, runID, evt, t)
	}
	return n.TaskRefs.SendNow(ctx, runID, evt)
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
		return n.TaskRefs.Send(ctx, runID, evt, eta)
	}
	return n.TaskRefs.SendNow(ctx, runID, evt)
}

// UpdateConfig tells RunManager to update the given Run to new config.
func (n *Notifier) UpdateConfig(ctx context.Context, runID common.RunID, hash string, eversion int64) error {
	return n.TaskRefs.SendNow(ctx, runID, &eventpb.Event{
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
	return n.TaskRefs.SendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_Cancel{
			Cancel: &eventpb.Cancel{},
		},
	})
}

// NotifyCLUpdated informs RunManager that given CL has a new version available.
func (n *Notifier) NotifyCLUpdated(ctx context.Context, runID common.RunID, clid common.CLID, eVersion int) error {
	return n.TaskRefs.SendNow(ctx, runID, &eventpb.Event{
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
		return n.TaskRefs.SendNow(ctx, runID, evt)
	}
	evt.ProcessAfter = timestamppb.New(eta)
	return n.TaskRefs.Send(ctx, runID, evt, eta)
}

// NotifyCQDVerificationCompleted tells RunManager that CQDaemon has completed
// verifying the provided Run.
//
// TODO(crbug/1141880): Remove this event after migration.
func (n *Notifier) NotifyCQDVerificationCompleted(ctx context.Context, runID common.RunID) error {
	return n.TaskRefs.SendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_CqdVerificationCompleted{
			CqdVerificationCompleted: &eventpb.CQDVerificationCompleted{},
		},
	})
}
