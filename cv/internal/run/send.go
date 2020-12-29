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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/run/internal"
)

// Start tells RunManager to start the given run.
func Start(ctx context.Context, runID ID) error {
	return send(ctx, runID, &internal.Event{
		Event: &internal.Event_Start{
			Start: &internal.Start{},
		},
	})
}

// Poke tells RunManager to check its own state.
func Poke(ctx context.Context, runID ID) error {
	return send(ctx, runID, &internal.Event{
		Event: &internal.Event_Poke{
			Poke: &internal.Poke{},
		},
	})
}

// UpdateConfig tells RunManager to update the given Run to new config.
func UpdateConfig(ctx context.Context, runID ID, hash string, eversion int64) error {
	return send(ctx, runID, &internal.Event{
		Event: &internal.Event_UpdateConfig{
			UpdateConfig: &internal.UpdateConfig{
				Hash:     hash,
				Eversion: eversion,
			},
		},
	})
}

// Cancel tells RunManager to cancel the given run.
//
// TODO(yiwzhang,tandrii): support reason.
func Cancel(ctx context.Context, runID ID) error {
	return send(ctx, runID, &internal.Event{
		Event: &internal.Event_Cancel{
			Cancel: &internal.Cancel{},
		},
	})
}

func send(ctx context.Context, runID ID, evt *internal.Event) error {
	value, err := proto.Marshal(evt)
	if err != nil {
		return errors.Annotate(err, "failed to marshal").Err()
	}
	rid := string(runID)
	to := datastore.MakeKey(ctx, RunKind, rid)
	if err := eventbox.Emit(ctx, value, to); err != nil {
		return err
	}
	return internal.Dispatch(ctx, rid, time.Time{} /*asap*/)
}
