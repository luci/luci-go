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
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/impl/state"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

func init() {
	prjpb.DefaultTaskRefs.ManageProject.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*prjpb.ManageProjectTask)
			err := manageProject(ctx, task.GetLuciProject(), task.GetEta().AsTime())
			return common.TQIfy{KnownFatal: []error{errTaskArrivedTooLate}}.Error(ctx, err)
		},
	)

	prjpb.DefaultTaskRefs.KickManageProject.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*prjpb.KickManageProjectTask)
			var eta time.Time
			if t := task.GetEta(); t != nil {
				eta = t.AsTime()
			}
			err := prjpb.DefaultTaskRefs.Dispatch(ctx, task.GetLuciProject(), eta)
			return common.TQifyError(ctx, err)
		},
	)
}

var errTaskArrivedTooLate = errors.New("task arrived too late", tq.Fatal)

func manageProject(ctx context.Context, luciProject string, taskETA time.Time) error {
	ctx = logging.SetField(ctx, "project", luciProject)
	if delay := clock.Now(ctx).Sub(taskETA); delay > prjpb.MaxAcceptableDelay {
		logging.Warningf(ctx, "task %s arrived %s late; scheduling next task instead", taskETA, delay)
		// Scheduling new task reduces probability of concurrent tasks in extreme
		// events.
		if err := prjpb.DefaultTaskRefs.Dispatch(ctx, luciProject, time.Time{}); err != nil {
			return err
		}
		// Hard-fail this task to avoid retries and get correct monitoring stats.
		return errTaskArrivedTooLate
	}

	recipient := datastore.MakeKey(ctx, prjmanager.ProjectKind, luciProject)
	switch postProcessFns, err := eventbox.ProcessBatch(ctx, recipient, &projectManager{luciProject: luciProject}); {
	case err == nil:
		for _, postProcessFn := range postProcessFns {
			if err := postProcessFn(ctx); err != nil {
				return errors.Annotate(err, "project %q", luciProject).Err()
			}
		}
		return nil
	case eventbox.IsErrConcurretMutation(err):
		// Instead of retrying this task at a later time, which has already
		// overlapped with another task, and risking another overlap for busy
		// project, schedule a new one in the future which will get a chance of
		// deduplication.
		if err2 := prjpb.DefaultTaskRefs.Dispatch(ctx, luciProject, time.Time{}); err2 != nil {
			// This should be rare and retry is the best we can do.
			return errors.Annotate(err2, "project %q", luciProject).Err()
		}
		// Hard-fail this task to avoid retries and get correct monitoring stats.
		return errors.Annotate(err, "project %q", luciProject).Tag(tq.Fatal).Err()
	default:
		return errors.Annotate(err, "project %q", luciProject).Err()
	}
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
func (pm *projectManager) Mutate(ctx context.Context, events eventbox.Events, s eventbox.State) (ts []eventbox.Transition, noops eventbox.Events, err error) {
	ctx, span := trace.StartSpan(ctx, "go.chromium.org/luci/cv/internal/prjmanager/impl/Mutate")
	defer func() { span.End(err) }()

	tr := &triageResult{}
	for _, e := range events {
		tr.triage(ctx, e)
	}
	tr.removeCLUpdateNoops()

	ts, err = pm.mutate(ctx, tr, s.(*state.State))
	return ts, tr.noops, err
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
	// noops are events that can be safely deleted before a transaction
	// because another semantically **superseding** event will remain in
	// eventbox.
	//
	// Safety note: semantically the same event isn't sufficient, since concurrent
	// invocations of a PM must agree on which events can be deleted and which
	// must be kept.
	noops eventbox.Events

	// newConfig stores newConfig event with the largest ID if any.
	newConfig eventbox.Events
	// poke stores Poke event with the largest ID if any.
	poke eventbox.Events

	clsUpdated struct {
		// maps CLID to latest EVersion.
		clEVersions map[int64]int64
		// maps CLID to event ID of CLUpdated or CLsUpdated events.
		clEvents map[int64]string
		// initially, all events. removeCLUpdateNoops() leaves only referenced ones.
		events eventbox.Events
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
	purgesCompleted struct {
		events eventbox.Events
		purges []*prjpb.PurgeCompleted
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
		tr.highestIDWins(item, &tr.newConfig)
	case *prjpb.Event_Poke:
		tr.highestIDWins(item, &tr.poke)

	case *prjpb.Event_ClUpdated:
		tr.clsUpdated.events = append(tr.clsUpdated.events, item)
		tr.triageCLUpdated(v.ClUpdated, item.ID)
	case *prjpb.Event_ClsUpdated:
		tr.clsUpdated.events = append(tr.clsUpdated.events, item)
		for _, cl := range v.ClsUpdated.Cls {
			tr.triageCLUpdated(cl, item.ID)
		}

	case *prjpb.Event_RunCreated:
		tr.runsCreated.events = append(tr.runsCreated.events, item)
		tr.runsCreated.runs = append(tr.runsCreated.runs, common.RunID(v.RunCreated.GetRunId()))
	case *prjpb.Event_RunFinished:
		tr.runsFinished.events = append(tr.runsFinished.events, item)
		tr.runsFinished.runs = append(tr.runsFinished.runs, common.RunID(v.RunFinished.GetRunId()))
	case *prjpb.Event_PurgeCompleted:
		tr.purgesCompleted.events = append(tr.purgesCompleted.events, item)
		tr.purgesCompleted.purges = append(tr.purgesCompleted.purges, v.PurgeCompleted)
	default:
		panic(fmt.Errorf("unknown event: %T [id=%q]", e.GetEvent(), item.ID))
	}
}

func (tr *triageResult) highestIDWins(item eventbox.Event, target *eventbox.Events) {
	if len(*target) == 0 {
		*target = eventbox.Events{item}
		return
	}
	if i := (*target)[0]; i.ID < item.ID {
		tr.noops = append(tr.noops, i)
		(*target)[0] = item
	} else {
		tr.noops = append(tr.noops, item)
	}
}

func (tr *triageResult) triageCLUpdated(v *prjpb.CLUpdated, id string) {
	clid := v.GetClid()
	ev := v.GetEversion()

	cu := &tr.clsUpdated
	if curEV, exists := cu.clEVersions[v.GetClid()]; !exists || curEV < ev {
		if cu.clEVersions == nil {
			cu.clEVersions = make(map[int64]int64, 1)
			cu.clEvents = make(map[int64]string, 1)
		}
		cu.clEVersions[clid] = ev
		cu.clEvents[clid] = id
	}
}

func (tr *triageResult) removeCLUpdateNoops() {
	cu := &tr.clsUpdated
	eventIDs := stringset.New(len(cu.clEvents))
	for _, id := range cu.clEvents {
		eventIDs.Add(id)
	}
	remaining := cu.events[:0]
	for _, e := range cu.events {
		if eventIDs.Has(e.ID) {
			remaining = append(remaining, e)
		} else {
			tr.noops = append(tr.noops, e)
		}
	}
	cu.events = remaining
	cu.clEvents = nil // free memory
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

	if len(tr.clsUpdated.clEVersions) > 0 {
		if s, se, err = s.OnCLsUpdated(ctx, tr.clsUpdated.clEVersions); err != nil {
			return nil, err
		}
		ret = append(ret, eventbox.Transition{
			Events:       tr.clsUpdated.events,
			SideEffectFn: state.SideEffectFn(se),
			TransitionTo: s,
		})
	}

	// OnPurgesCompleted may expire purges even without incoming event.
	if s, se, err = s.OnPurgesCompleted(ctx, tr.purgesCompleted.purges); err != nil {
		return nil, err
	}
	ret = append(ret, eventbox.Transition{
		Events:       tr.purgesCompleted.events,
		SideEffectFn: state.SideEffectFn(se),
		TransitionTo: s,
	})

	if s, se, err = s.ExecDeferred(ctx); err != nil {
		return nil, err
	}
	return append(ret, eventbox.Transition{
		SideEffectFn: state.SideEffectFn(se),
		TransitionTo: s,
	}), nil
}
