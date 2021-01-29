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

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/run/eventpb"
)

// Start tells RunManager to start the given run.
func Start(ctx context.Context, runID common.RunID) error {
	return sendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_Start{
			Start: &eventpb.Start{},
		},
	})
}

// PokeNow tells RunManager to check its own state immediately.
func PokeNow(ctx context.Context, runID common.RunID) error {
	return Poke(ctx, runID, 0)
}

// Poke tells RunManager to check its own state.
func Poke(ctx context.Context, runID common.RunID, after time.Duration) error {
	evt := &eventpb.Event{
		Event: &eventpb.Event_Poke{
			Poke: &eventpb.Poke{},
		},
	}
	if after > 0 {
		t := clock.Now(ctx).Add(after)
		evt.ProcessAfter = timestamppb.New(t)
		return send(ctx, runID, evt, t)

	}
	return sendNow(ctx, runID, evt)
}

// UpdateConfig tells RunManager to update the given Run to new config.
func UpdateConfig(ctx context.Context, runID common.RunID, hash string, eversion int64) error {
	return sendNow(ctx, runID, &eventpb.Event{
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
func Cancel(ctx context.Context, runID common.RunID) error {
	return sendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_Cancel{
			Cancel: &eventpb.Cancel{},
		},
	})
}

// NotifyCLUpdated informs RunManager that given CL has a new version available.
func NotifyCLUpdated(ctx context.Context, runID common.RunID, clid common.CLID, eVersion int) error {
	return sendNow(ctx, runID, &eventpb.Event{
		Event: &eventpb.Event_ClUpdated{
			ClUpdated: &eventpb.CLUpdated{
				Clid:     int64(clid),
				EVersion: int64(eVersion),
			},
		},
	})
}

// NotifyFinished tells RunManager that Run has finished in CQDaemon.
//
// TODO(crbug/1141880): Remove this event after migration.
func NotifyFinished(ctx context.Context, runID common.RunID, after time.Duration) error {
	evt := &eventpb.Event{
		Event: &eventpb.Event_Finished{
			Finished: &eventpb.Finished{},
		},
	}
	if after > 0 {
		t := clock.Now(ctx).Add(after)
		evt.ProcessAfter = timestamppb.New(t)
		return send(ctx, runID, evt, t)

	}
	return sendNow(ctx, runID, evt)
}

func sendNow(ctx context.Context, runID common.RunID, evt *eventpb.Event) error {
	return send(ctx, runID, evt, time.Time{})
}

func send(ctx context.Context, runID common.RunID, evt *eventpb.Event, eta time.Time) error {
	value, err := proto.Marshal(evt)
	if err != nil {
		return errors.Annotate(err, "failed to marshal").Err()
	}
	rid := string(runID)
	to := datastore.MakeKey(ctx, RunKind, rid)
	if err := eventbox.Emit(ctx, value, to); err != nil {
		return err
	}
	return eventpb.Dispatch(ctx, rid, eta)
}
