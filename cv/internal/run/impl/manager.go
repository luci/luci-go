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

package impl

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/internal"
)

func init() {
	internal.PokeRunTaskRef.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*internal.PokeRunTask)
			switch err := pokeRunTask(ctx, run.ID(task.GetRunId())); {
			case err == nil:
				return nil
			case !transient.Tag.In(err):
				err = tq.Fatal.Apply(err)
				fallthrough
			default:
				errors.Log(ctx, err)
				// TODO(tandrii/yiwzhang): avoid retries iff we know a new task was
				// already scheduled for the next second.
				return err
			}
		},
	)
}

func pokeRunTask(ctx context.Context, runID run.ID) error {
	ctx = logging.SetField(ctx, "run", runID)
	recipient := datastore.MakeKey(ctx, run.RunKind, runID)
	return eventbox.ProcessBatch(ctx, recipient, &runManager{id: runID})
}

// runManager implements eventbox.Processor.
type runManager struct {
	id run.ID
}

var _ eventbox.Processor = (*runManager)(nil)

// LoadState is called to load the state before a transaction.
func (rm *runManager) LoadState(ctx context.Context) (eventbox.State, eventbox.EVersion, error) {
	panic("not implemented")

}

// Mutate is called before a transaction to compute transitions based on a
// batch of events.
//
// All actions that must be done atomically with updating state must be
// encapsulated inside Transition.SideEffectFn callback.
func (rm *runManager) Mutate(ctx context.Context, events eventbox.Events, s eventbox.State) ([]eventbox.Transition, error) {
	panic("not implemented")
}

// FetchEVersion is called at the beginning of a transaction.
//
// The returned EVersion is compared against the one associated with a state
// loaded via GetState. If different, the transaction is aborted and new state
// isn't saved.
func (rm *runManager) FetchEVersion(ctx context.Context) (eventbox.EVersion, error) {
	panic("not implemented")
}

// SaveState is called in a transaction to save the state if it has changed.
//
// The passed eversion is incremented value of eversion of what GetState
// returned before.
func (rm *runManager) SaveState(ctx context.Context, st eventbox.State, ev eventbox.EVersion) error {
	panic("not implemented")
}

// triageResult is the result of the triage of the incoming events.
type triageResult struct {
	starts  eventbox.Events
	cancels eventbox.Events
}

func (tr *triageResult) triage(ctx context.Context, item eventbox.Event) {
	e := &internal.Event{}
	if err := proto.Unmarshal(item.Value, e); err != nil {
		// This is a bug in code or data corruption.
		// There is no way to recover on its own.
		logging.Errorf(ctx, "CRITICAL: failed to deserialize event %q: %s", item.ID, err)
		panic(err)
	}
	switch e.GetEvent().(type) {
	case *internal.Event_Start:
		tr.starts = append(tr.starts, item)
	case *internal.Event_Cancel:
		tr.cancels = append(tr.cancels, item)
	default:
		panic(fmt.Errorf("unknown event: %T [id=%q]", e.GetEvent(), item.ID))
	}
}
