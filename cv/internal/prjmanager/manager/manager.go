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

package manager

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
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/poller"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/clpurger"
	"go.chromium.org/luci/cv/internal/prjmanager/cltriggerer"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/prjmanager/state"
	"go.chromium.org/luci/cv/internal/prjmanager/triager"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/runcreator"
	"go.chromium.org/luci/cv/internal/tracing"
)

const (
	// maxEventsPerBatch limits the number of incoming events the PM will process at
	// once.
	//
	// This shouldn't be hit in practice under normal operation. This is chosen such
	// that PM can read these events and make some progress in 1 minute.
	maxEventsPerBatch = 10000

	// logProjectStateFrequency forces saving ProjectLog entity iff
	// Project.EVersion is divisible by logProjectStateFrequency.
	//
	// In practice, the busiest projects sustain at most ~1 QPS of updates.
	// Thus, value of 60 limits ProjectLog to at most 1/minute or 1.5k/day.
	logProjectStateFrequency = 60
)

var errTaskArrivedTooLate = errors.New("task arrived too late")

// ProjectManager implements managing projects.
type ProjectManager struct {
	tasksBinding prjpb.TasksBinding
	handler      state.Handler
}

// New creates a new ProjectManager and registers it for handling tasks created
// by the given TQ Notifier.
func New(n *prjmanager.Notifier, rn state.RunNotifier, c *changelist.Mutator, g gerrit.Factory, u *changelist.Updater) *ProjectManager {
	pm := &ProjectManager{
		tasksBinding: n.TasksBinding,
		handler: state.Handler{
			CLMutator:       c,
			PMNotifier:      n,
			RunNotifier:     rn,
			CLPurger:        clpurger.New(n, g, u, c),
			CLTriggerer:     cltriggerer.New(n, g),
			CLPoller:        poller.New(n.TasksBinding.TQDispatcher, g, u, n),
			ComponentTriage: triager.Triage,
		},
	}
	n.TasksBinding.ManageProject.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*prjpb.ManageProjectTask)
			ctx = logging.SetField(ctx, "project", task.GetLuciProject())
			err := pm.manageProject(ctx, task.GetLuciProject(), task.GetEta().AsTime())
			return common.TQIfy{
				KnownIgnore:     []error{errTaskArrivedTooLate},
				KnownIgnoreTags: []errors.BoolTag{common.DSContentionTag},
				KnownRetryTags:  []errors.BoolTag{runcreator.StateChangedTag},
			}.Error(ctx, err)
		},
	)

	n.TasksBinding.KickManageProject.AttachHandler(
		func(ctx context.Context, payload proto.Message) error {
			task := payload.(*prjpb.KickManageProjectTask)
			var eta time.Time
			if t := task.GetEta(); t != nil {
				eta = t.AsTime()
			}
			err := n.TasksBinding.Dispatch(ctx, task.GetLuciProject(), eta)
			return common.TQifyError(ctx, err)
		},
	)
	return pm
}

func (pm *ProjectManager) manageProject(ctx context.Context, luciProject string, taskETA time.Time) error {
	retryViaNewTask := false
	var processErr error
	if delay := clock.Now(ctx).Sub(taskETA); delay > prjpb.MaxAcceptableDelay {
		logging.Warningf(ctx, "task %s arrived %s late; scheduling next task instead", taskETA, delay)
		retryViaNewTask = true
		processErr = errTaskArrivedTooLate
	} else {
		processErr = pm.processBatch(ctx, luciProject)
		if common.DSContentionTag.In(processErr) {
			logging.Warningf(ctx, "Datastore contention; scheduling next task instead")
			retryViaNewTask = true
		}
	}

	if retryViaNewTask {
		// Scheduling new task reduces probability of concurrent tasks in extreme
		// events.
		if err := pm.tasksBinding.Dispatch(ctx, luciProject, time.Time{}); err != nil {
			// This should be rare and retry is the best we can do.
			return err
		}
	}
	return processErr
}

func (pm *ProjectManager) processBatch(ctx context.Context, luciProject string) error {
	proc := &pmProcessor{
		luciProject: luciProject,
		handler:     &pm.handler,
	}
	recipient := prjmanager.EventboxRecipient(ctx, luciProject)
	postProcessFns, err := eventbox.ProcessBatch(ctx, recipient, proc, maxEventsPerBatch)
	if err != nil {
		return err
	}
	if l := len(postProcessFns); l > 0 {
		panic(fmt.Errorf("postProcessFns is not supported in PM; got %d", l))
	}
	return nil
}

// pmProcessor implements eventbox.Processor.
type pmProcessor struct {
	luciProject string
	handler     *state.Handler
	// loadedPState is set by LoadState and read by SaveState.
	loadedPState *prjpb.PState
}

// LoadState is called to load the state before a transaction.
func (proc *pmProcessor) LoadState(ctx context.Context) (eventbox.State, eventbox.EVersion, error) {
	s := &state.State{}
	switch p, err := prjmanager.Load(ctx, proc.luciProject); {
	case err != nil:
		return nil, 0, err
	case p == nil:
		s.PB = &prjpb.PState{LuciProject: proc.luciProject}
		return s, 0, nil
	default:
		p.State.LuciProject = proc.luciProject
		proc.loadedPState = p.State
		s.PB = p.State
		return s, eventbox.EVersion(p.EVersion), nil
	}
}

// PrepareMutation is called before a transaction to compute transitions.
//
// All actions that must be done atomically with updating state must be
// encapsulated inside Transition.SideEffectFn callback.
func (proc *pmProcessor) PrepareMutation(ctx context.Context, events eventbox.Events, s eventbox.State) (ts []eventbox.Transition, noops eventbox.Events, err error) {
	ctx, span := tracing.Start(ctx, "go.chromium.org/luci/cv/internal/prjmanager/impl/Mutate")
	defer func() { tracing.End(span, err) }()

	tr := &triageResult{}
	for _, e := range events {
		tr.triage(ctx, e)
	}
	tr.removeCLUpdateNoops()

	ts, err = proc.mutate(ctx, tr, s.(*state.State))
	return ts, tr.noops, err
}

// FetchEVersion is called at the beginning of a transaction.
//
// The returned EVersion is compared against the one associated with a state
// loaded via GetState. If different, the transaction is aborted and new state
// isn't saved.
func (proc *pmProcessor) FetchEVersion(ctx context.Context) (eventbox.EVersion, error) {
	p := &prjmanager.Project{ID: proc.luciProject}
	switch err := datastore.Get(ctx, p); {
	case err == datastore.ErrNoSuchEntity:
		return 0, nil
	case err != nil:
		return 0, errors.Annotate(err, "failed to get %q", proc.luciProject).Tag(transient.Tag).Err()
	default:
		return eventbox.EVersion(p.EVersion), nil
	}
}

// SaveState is called in a transaction to save the state if it has changed.
//
// The passed EVersion is the incremented value of EVersion of what GetState
// returned before.
func (proc *pmProcessor) SaveState(ctx context.Context, st eventbox.State, ev eventbox.EVersion) error {
	s := st.(*state.State)
	// Erase PB.LuciProject as it's already stored as Project{ID:...}.
	s.PB.LuciProject = ""

	new := &prjmanager.Project{
		ID:         proc.luciProject,
		EVersion:   int64(ev),
		UpdateTime: datastore.RoundTime(clock.Now(ctx).UTC()),
		State:      s.PB,
	}
	entities := make([]any, 1, 3)
	entities[0] = new

	old := proc.loadedPState
	if s.PB.GetConfigHash() != old.GetConfigHash() || s.PB.GetStatus() != old.GetStatus() {
		entities = append(entities, &prjmanager.ProjectStateOffload{
			Project:    datastore.MakeKey(ctx, prjmanager.ProjectKind, proc.luciProject),
			Status:     s.PB.GetStatus(),
			ConfigHash: s.PB.GetConfigHash(),
			UpdateTime: clock.Now(ctx).UTC(),
		})
	}

	switch reasons := s.LogReasons; {
	case new.EVersion%logProjectStateFrequency == 0:
		reasons = append(s.LogReasons, prjpb.LogReason_FYI_PERIODIC)
		fallthrough
	case len(reasons) > 0:
		deduped := prjpb.SortAndDedupeLogReasons(reasons)
		txndefer.Defer(ctx, func(ctx context.Context) {
			logging.Debugf(ctx, "Saved ProjectLog @ %d due to %s", new.EVersion, prjpb.FormatLogReasons(deduped))
		})
		entities = append(entities, &prjmanager.ProjectLog{
			Project:    datastore.MakeKey(ctx, prjmanager.ProjectKind, proc.luciProject),
			EVersion:   new.EVersion,
			Status:     s.PB.GetStatus(),
			ConfigHash: s.PB.GetConfigHash(),
			State:      new.State,
			UpdateTime: new.UpdateTime,
			Reasons:    deduped,
		})
	}

	if err := datastore.Put(ctx, entities...); err != nil {
		return errors.Annotate(err, "failed to put Project").Tag(transient.Tag).Err()
	}
	return nil
}

// triageResult is the result of the triage of the incoming events.
type triageResult struct {
	// Noops are events that can be safely deleted before a transaction
	// because another semantically **superseding** event will remain in
	// eventbox.
	//
	// Safety note: semantically the same event isn't sufficient, since
	// concurrent invocations of a PM must agree on which events can be deleted
	// and which must be kept.
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
		events eventbox.Events
		runs   map[common.RunID]run.Status
	}
	purgesCompleted struct {
		events eventbox.Events
		purges []*prjpb.PurgeCompleted
	}
	triggeringCLsCompleted struct {
		events    eventbox.Events
		succeeded []*prjpb.TriggeringCLsCompleted_OpResult
		failed    []*prjpb.TriggeringCLsCompleted_OpResult
		skipped   []*prjpb.TriggeringCLsCompleted_OpResult
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

	case *prjpb.Event_ClsUpdated:
		tr.clsUpdated.events = append(tr.clsUpdated.events, item)
		for _, cl := range v.ClsUpdated.GetEvents() {
			tr.triageCLUpdated(cl, item.ID)
		}

	case *prjpb.Event_RunCreated:
		tr.runsCreated.events = append(tr.runsCreated.events, item)
		tr.runsCreated.runs = append(tr.runsCreated.runs, common.RunID(v.RunCreated.GetRunId()))
	case *prjpb.Event_RunFinished:
		tr.runsFinished.events = append(tr.runsFinished.events, item)
		if tr.runsFinished.runs == nil {
			tr.runsFinished.runs = make(map[common.RunID]run.Status)
		}
		tr.runsFinished.runs[common.RunID(v.RunFinished.GetRunId())] = v.RunFinished.GetStatus()
	case *prjpb.Event_PurgeCompleted:
		tr.purgesCompleted.events = append(tr.purgesCompleted.events, item)
		tr.purgesCompleted.purges = append(tr.purgesCompleted.purges, v.PurgeCompleted)
	case *prjpb.Event_TriggeringClDepsCompleted:
	case *prjpb.Event_TriggeringClsCompleted:
		tr.triggeringCLsCompleted.events = append(tr.triggeringCLsCompleted.events, item)
		tr.triggeringCLsCompleted.succeeded = append(
			tr.triggeringCLsCompleted.succeeded, v.TriggeringClsCompleted.GetSucceeded()...)
		tr.triggeringCLsCompleted.failed = append(
			tr.triggeringCLsCompleted.failed, v.TriggeringClsCompleted.GetFailed()...)
		tr.triggeringCLsCompleted.skipped = append(
			tr.triggeringCLsCompleted.skipped, v.TriggeringClsCompleted.GetSkipped()...)
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

func (tr *triageResult) triageCLUpdated(v *changelist.CLUpdatedEvent, id string) {
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

func (proc *pmProcessor) mutate(ctx context.Context, tr *triageResult, s *state.State) ([]eventbox.Transition, error) {
	var err error
	var se state.SideEffect
	ret := make([]eventbox.Transition, 0, 7)

	if upgraded := s.UpgradeIfNecessary(); upgraded != s {
		ret = append(ret, eventbox.Transition{TransitionTo: upgraded})
		s = upgraded
	}

	// Visit all non-empty fields of triageResult and emit Transitions.
	// The order of visits matters.

	// Even though OnRunCreated event is sent before OnRunFinished event,
	// under rare conditions it's possible that OnRunsFinished will be read first,
	// and OnRunsCreated will be read only in the next PM invocation
	// (see https://crbug.com/1218681 for a concrete example).
	if len(tr.runsCreated.runs) > 0 {
		if s, se, err = proc.handler.OnRunsCreated(ctx, s, tr.runsCreated.runs); err != nil {
			return nil, err
		}
		ret = append(ret, eventbox.Transition{
			Events:       tr.runsCreated.events,
			SideEffectFn: state.SideEffectFn(se),
			TransitionTo: s,
		})
	}

	if len(tr.runsFinished.runs) > 0 {
		if s, se, err = proc.handler.OnRunsFinished(ctx, s, tr.runsFinished.runs); err != nil {
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
		if s, se, err = proc.handler.UpdateConfig(ctx, s); err != nil {
			return nil, err
		}
		ret = append(ret, eventbox.Transition{
			Events:       tr.newConfig,
			SideEffectFn: state.SideEffectFn(se),
			TransitionTo: s,
		})
	}

	if len(tr.poke) > 0 {
		if s, se, err = proc.handler.Poke(ctx, s); err != nil {
			return nil, err
		}
		ret = append(ret, eventbox.Transition{
			Events:       tr.poke,
			SideEffectFn: state.SideEffectFn(se),
			TransitionTo: s,
		})
	}

	if len(tr.clsUpdated.clEVersions) > 0 {
		if s, se, err = proc.handler.OnCLsUpdated(ctx, s, tr.clsUpdated.clEVersions); err != nil {
			return nil, err
		}
		ret = append(ret, eventbox.Transition{
			Events:       tr.clsUpdated.events,
			SideEffectFn: state.SideEffectFn(se),
			TransitionTo: s,
		})
	}

	// OnPurgesCompleted may expire purges even without incoming event.
	if s, se, err = proc.handler.OnPurgesCompleted(ctx, s, tr.purgesCompleted.purges); err != nil {
		return nil, err
	}
	ret = append(ret, eventbox.Transition{
		Events:       tr.purgesCompleted.events,
		SideEffectFn: state.SideEffectFn(se),
		TransitionTo: s,
	})

	// OnTriggeringCLsCompleted may expire triggers even without incoming event.
	s, se, err = proc.handler.OnTriggeringCLsCompleted(ctx, s,
		tr.triggeringCLsCompleted.succeeded,
		tr.triggeringCLsCompleted.failed,
		tr.triggeringCLsCompleted.skipped,
	)
	if err != nil {
		return nil, err
	}
	ret = append(ret, eventbox.Transition{
		Events:       tr.triggeringCLsCompleted.events,
		SideEffectFn: state.SideEffectFn(se),
		TransitionTo: s,
	})

	if s, se, err = proc.handler.ExecDeferred(ctx, s); err != nil {
		return nil, err
	}
	return append(ret, eventbox.Transition{
		SideEffectFn: state.SideEffectFn(se),
		TransitionTo: s,
	}), nil
}
