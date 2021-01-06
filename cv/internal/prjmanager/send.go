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

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/prjmanager/internal"
)

// UpdateConfig tells ProjectManager to read and update to newest ProjectConfig
// by fetching it from Datatstore.
//
// Results in stopping ProjectManager if ProjectConfig got disabled or deleted.
func UpdateConfig(ctx context.Context, luciProject string) error {
	return send(ctx, luciProject, &internal.Event{
		Event: &internal.Event_UpdateConfig{
			UpdateConfig: &internal.UpdateConfig{},
		},
	})
}

// Poke tells ProjectManager to poke all downstream actors and check its own
// state.
func Poke(ctx context.Context, luciProject string) error {
	return send(ctx, luciProject, &internal.Event{
		Event: &internal.Event_Poke{
			Poke: &internal.Poke{},
		},
	})
}

// CLUpdated tells ProjectManager to check latest version of a given CL.
func CLUpdated(ctx context.Context, luciProject string, clid common.CLID, eversion int) error {
	return send(ctx, luciProject, &internal.Event{
		Event: &internal.Event_ClUpdated{
			ClUpdated: &internal.CLUpdated{
				Clid:     int64(clid),
				Eversion: int64(eversion),
			},
		},
	})
}

// runCreated is sent by ProjectManager to itself within a Run creation
// transaction.
//
// Unlike other event-sending funcs, this only creates an event and doesn't
// create a task. This is fine because:
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
func runCreated(ctx context.Context, runID common.RunID) error {
	return sendWithoutDispatch(ctx, runID.LUCIProject(), &internal.Event{
		Event: &internal.Event_RunCreated{
			RunCreated: &internal.RunCreated{
				RunId: string(runID),
			},
		},
	})
}

// RunFinished tells ProjectManager that a run has finalized its state.
func RunFinished(ctx context.Context, runID common.RunID) error {
	return send(ctx, runID.LUCIProject(), &internal.Event{
		Event: &internal.Event_RunFinished{
			RunFinished: &internal.RunFinished{
				RunId: string(runID),
			},
		},
	})
}

func send(ctx context.Context, luciProject string, e *internal.Event) error {
	if err := sendWithoutDispatch(ctx, luciProject, e); err != nil {
		return err
	}
	return internal.Dispatch(ctx, luciProject, time.Time{} /*asap*/)
}

func sendWithoutDispatch(ctx context.Context, luciProject string, e *internal.Event) error {
	value, err := proto.Marshal(e)
	if err != nil {
		return errors.Annotate(err, "failed to marshal").Err()
	}
	to := datastore.MakeKey(ctx, ProjectKind, luciProject)
	return eventbox.Emit(ctx, value, to)
}
