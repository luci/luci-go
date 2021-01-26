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

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/impl/state"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

func init() {
	prjpb.PokePMTaskRef.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*prjpb.PokePMTask)
			err := pokePMTask(ctx, task.GetLuciProject())
			// TODO(tandrii): avoid retries iff we know a new task was already
			// scheduled for the next second.
			return common.TQifyError(ctx, err)
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

	// loadedPState is set by LoadState and read by SaveState.
	loadedPState *prjpb.PState
}

// LoadState is called to load the state before a transaction.
func (pm *projectManager) LoadState(ctx context.Context) (eventbox.State, eventbox.EVersion, error) {
	switch p, err := prjmanager.Load(ctx, pm.luciProject); {
	case err != nil:
		return nil, 0, err
	case p == nil:
		return state.NewInitial(pm.luciProject), 0, nil
	default:
		p.State.LuciProject = pm.luciProject
		pm.loadedPState = p.State
		return state.NewExisting(p.State), eventbox.EVersion(p.EVersion), nil
	}
}

// Mutate is called before a transaction to compute transitions.
//
// All actions that must be done atomically with updating state must be
// encapsulated inside Transition.SideEffectFn callback.
func (pm *projectManager) Mutate(ctx context.Context, events eventbox.Events, s eventbox.State) ([]eventbox.Transition, error) {
	tr := &triageResult{}
	for _, e := range events {
		tr.triage(ctx, e)
	}
	return pm.mutate(ctx, tr, s.(*state.State))
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
		return 0, errors.Annotate(err, "failed to get %q", pm.luciProject).Tag(transient.Tag).Err()
	default:
		return eventbox.EVersion(p.EVersion), nil
	}
}

// SaveState is called in a transaction to save the state if it has changed.
//
// The passed EVersion is the incremented value of EVersion of what GetState
// returned before.
func (pm *projectManager) SaveState(ctx context.Context, st eventbox.State, ev eventbox.EVersion) error {
	s := st.(*state.State)
	// Erase PB.LuciProject as it's already stored as Project{ID:...}.
	s.PB.LuciProject = ""
	entities := make([]interface{}, 1, 2)
	entities[0] = &prjmanager.Project{
		ID:         pm.luciProject,
		EVersion:   int(ev),
		UpdateTime: clock.Now(ctx).UTC(),
		State:      s.PB,
	}

	old := pm.loadedPState
	if s.PB.GetConfigHash() != old.GetConfigHash() || s.PB.GetStatus() != old.GetStatus() {
		entities = append(entities, &prjmanager.ProjectStateOffload{
			Project:    datastore.MakeKey(ctx, prjmanager.ProjectKind, pm.luciProject),
			Status:     s.PB.GetStatus(),
			ConfigHash: s.PB.GetConfigHash(),
		})
	}

	if err := datastore.Put(ctx, entities...); err != nil {
		return errors.Annotate(err, "failed to put Project").Tag(transient.Tag).Err()
	}
	return nil
}

// triageResult is the result of the triage of the incoming events.
type triageResult struct {
	newConfig eventbox.Events
	poke      eventbox.Events

	clsUpdated struct {
		// i-th event corresponds to i-th cl.
		events eventbox.Events
		cls    []*prjpb.CLUpdated
	}
	runsCreated struct {
		// events and runs are in random order.
		events eventbox.Events
		runs   common.RunIDs
	}
	runsFinished struct {
		// events and runs are in random order.
		events eventbox.Events
		runs   common.RunIDs
	}
}

func (tr *triageResult) triage(ctx context.Context, item eventbox.Event) {
	e := &prjpb.Event{}
	if err := proto.Unmarshal(item.Value, e); err != nil {
		// This is a bug in code or data corruption.
		// There is no way to recover on its own.
		logging.Errorf(ctx, "CRITICAL: failed to deserialize event %q: %s", item.ID, err)
		panic(err)
	}
	switch v := e.GetEvent().(type) {
	case *prjpb.Event_NewConfig:
		tr.newConfig = append(tr.newConfig, item)
	case *prjpb.Event_Poke:
		tr.poke = append(tr.poke, item)
	case *prjpb.Event_ClUpdated:
		tr.clsUpdated.events = append(tr.clsUpdated.events, item)
		tr.clsUpdated.cls = append(tr.clsUpdated.cls, v.ClUpdated)
	case *prjpb.Event_RunCreated:
		tr.runsCreated.events = append(tr.runsCreated.events, item)
		tr.runsCreated.runs = append(tr.runsCreated.runs, common.RunID(v.RunCreated.GetRunId()))
	case *prjpb.Event_RunFinished:
		tr.runsFinished.events = append(tr.runsFinished.events, item)
		tr.runsFinished.runs = append(tr.runsFinished.runs, common.RunID(v.RunFinished.GetRunId()))
	default:
		panic(fmt.Errorf("unknown event: %T [id=%q]", e.GetEvent(), item.ID))
	}
}

func (pm *projectManager) mutate(ctx context.Context, tr *triageResult, s *state.State) ([]eventbox.Transition, error) {
	var err error
	var se state.SideEffect
	ret := make([]eventbox.Transition, 0, 6)

	// Visit all non-empty fields of triageResult and emit Transitions.
	// The order of visits matters.

	// It's possible that the same Run will be in both runCreated & runFinished,
	// so process created first.
	if len(tr.runsCreated.runs) > 0 {
		if s, se, err = s.OnRunsCreated(ctx, tr.runsCreated.runs); err != nil {
			return nil, err
		}
		ret = append(ret, eventbox.Transition{
			Events:       tr.runsCreated.events,
			SideEffectFn: state.SideEffectFn(se),
			TransitionTo: s,
		})
	}
	if len(tr.runsFinished.runs) > 0 {
		if s, se, err = s.OnRunsFinished(ctx, tr.runsFinished.runs); err != nil {
			return nil, err
		}
		ret = append(ret, eventbox.Transition{
			Events:       tr.runsFinished.events,
			SideEffectFn: state.SideEffectFn(se),
			TransitionTo: s,
		})
	}

	// UpdateConfig event may result in stopping the PM, which requires notifying
	// each of the incomplete Runs to stop. Thus, runsCreated must be processed
	// before to ensure no Run will be missed.
	if len(tr.newConfig) > 0 {
		if s, se, err = s.UpdateConfig(ctx); err != nil {
			return nil, err
		}
		ret = append(ret, eventbox.Transition{
			Events:       tr.newConfig,
			SideEffectFn: state.SideEffectFn(se),
			TransitionTo: s,
		})
	}

	if len(tr.poke) > 0 {
		if s, se, err = s.Poke(ctx); err != nil {
			return nil, err
		}
		ret = append(ret, eventbox.Transition{
			Events:       tr.poke,
			SideEffectFn: state.SideEffectFn(se),
			TransitionTo: s,
		})
	}

	if len(tr.clsUpdated.cls) > 0 {
		if s, se, err = s.OnCLsUpdated(ctx, tr.clsUpdated.cls); err != nil {
			return nil, err
		}
		ret = append(ret, eventbox.Transition{
			Events:       tr.clsUpdated.events,
			SideEffectFn: state.SideEffectFn(se),
			TransitionTo: s,
		})
	}

	if s, se, err = s.ExecDeferred(ctx); err != nil {
		return nil, err
	}
	return append(ret, eventbox.Transition{
		SideEffectFn: state.SideEffectFn(se),
		TransitionTo: s,
	}), nil
}
