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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/internal"
)

func init() {
	internal.PokePMTaskRef.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*internal.PokePMTask)
			switch err := pokePMTask(ctx, task.GetLuciProject()); {
			case err == nil:
				return nil
			case !transient.Tag.In(err):
				err = tq.Fatal.Apply(err)
				fallthrough
			default:
				errors.Log(ctx, err)
				// TODO(tandrii): avoid retries iff we know a new task was already
				// scheduled for the next second.
				return err
			}
		},
	)
}

func pokePMTask(ctx context.Context, luciProject string) error {
	ctx = logging.SetField(ctx, "project", luciProject)
	recipient := datastore.MakeKey(ctx, prjmanager.ProjectKind, luciProject)
	return eventbox.ProcessBatch(ctx, recipient, &projectManager{luciProject: luciProject})
}

// projectManager implements eventbox.Processor.
type projectManager struct {
	luciProject string
}

// LoadState is called to load the state before a transaction.
func (pm *projectManager) LoadState(ctx context.Context) (eventbox.State, eventbox.EVersion, error) {
	p := &prjmanager.Project{ID: pm.luciProject}
	switch err := datastore.Get(ctx, p); {
	case err == datastore.ErrNoSuchEntity:
		return &state{}, 0, nil
	case err != nil:
		return nil, 0, errors.Annotate(err, "failed to get %q", pm.luciProject).Tag(
			transient.Tag).Err()
	default:
		s := &state{
			Status:         p.Status,
			ConfigHash:     p.ConfigHash,
			IncompleteRuns: p.IncompleteRuns,
		}
		return s, eventbox.EVersion(p.EVersion), nil
	}
}

// Mutate is called before a transaction to compute transitions.
//
// All actions that must be done atomically with updating state must be
// encapsulated inside Transition.SideEffectFn callback.
func (pm *projectManager) Mutate(ctx context.Context, events eventbox.Events, s eventbox.State) (
	[]eventbox.Transition, error) {
	tr := &triageResult{}
	for _, e := range events {
		tr.triage(ctx, e)
	}
	return pm.mutate(ctx, tr, s.(*state))
}

// FetchEVersion is called at the beginning of a transaction.
//
// The returned EVersion is compared against the one associated with a state
// loaded via GetState. If different, the transaction is aborted and new state
// isn't saved.
func (pm *projectManager) FetchEVersion(ctx context.Context) (eventbox.EVersion, error) {
	p := &prjmanager.Project{ID: pm.luciProject}
	switch err := datastore.Get(ctx, p); {
	case err == datastore.ErrNoSuchEntity:
		return 0, nil
	case err != nil:
		return 0, errors.Annotate(err, "failed to get %q", pm.luciProject).Tag(
			transient.Tag).Err()
	default:
		return eventbox.EVersion(p.EVersion), nil
	}
}

// SaveState is called in a transaction to save the state if it has changed.
//
// The passed EVersion is the incremented value of EVersion of what GetState
// returned before.
func (pm *projectManager) SaveState(ctx context.Context, st eventbox.State, ev eventbox.EVersion) error {
	s := st.(*state)
	p := &prjmanager.Project{
		ID:         pm.luciProject,
		EVersion:   int(ev),
		UpdateTime: clock.Now(ctx).UTC(),
	}
	p.Status = s.Status
	p.ConfigHash = s.ConfigHash
	p.IncompleteRuns = s.IncompleteRuns
	if err := datastore.Put(ctx, p); err != nil {
		return errors.Annotate(err, "failed to put Project").Tag(transient.Tag).Err()
	}
	return nil
}

// triageResult is the result of the triage of the incoming events.
type triageResult struct {
	updateConfig eventbox.Events
	poke         eventbox.Events
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
	case *internal.Event_UpdateConfig:
		tr.updateConfig = append(tr.updateConfig, item)
	case *internal.Event_Poke:
		tr.poke = append(tr.poke, item)
	default:
		panic(fmt.Errorf("unknown event: %T [id=%q]", e.GetEvent(), item.ID))
	}
}

func (pm *projectManager) mutate(ctx context.Context, tr *triageResult, s *state) (
	ret []eventbox.Transition, err error) {
	// Visit all non-empty fields of triageResult and emit Transitions.
	if len(tr.updateConfig) > 0 {
		t := eventbox.Transition{Events: tr.updateConfig}
		t.SideEffectFn, s, err = updateConfig(ctx, pm.luciProject, s)
		if err != nil {
			return nil, err
		}
		t.TransitionTo = s
		ret = append(ret, t)
	}
	if len(tr.poke) > 0 {
		t := eventbox.Transition{Events: tr.poke}
		t.SideEffectFn, s, err = poke(ctx, pm.luciProject, s)
		if err != nil {
			return nil, err
		}
		t.TransitionTo = s
		ret = append(ret, t)
	}
	return
}
