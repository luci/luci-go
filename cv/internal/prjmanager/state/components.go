// Copyright 2021 The LUCI Authors.
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

package state

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/runtime/paniccatcher"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/prjmanager/itriager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run/runcreator"
)

const concurrentComponentProcessing = 16

var errCaughtPanic = errors.New("caught panic")

// earliestDecisionTime returns the earliest decision time of all components.
//
// Returns the same time as time.Time and as proto, and boolean indicating that
// earliestDecisionTime is as soon as possible.
//
// Re-uses DecisionTime of one of the components, assuming that components are
// modified copy-on-write.
func earliestDecisionTime(cs []*prjpb.Component) (time.Time, *timestamppb.Timestamp, bool) {
	var ret time.Time
	var retPB *timestamppb.Timestamp
	for _, c := range cs {
		if c.GetTriageRequired() {
			return time.Time{}, nil, true
		}
		if dt := c.GetDecisionTime(); dt != nil {
			if t := dt.AsTime(); ret.IsZero() || ret.After(t) {
				ret = t
				retPB = dt
			}
		}
	}
	return ret, retPB, false
}

// cAction is a component action to be taken during current PM mutation.
//
// An action may involve creating one or more new Runs,
// or removing CQ votes from CL(s) which can't form a Run for some reason.
type cAction struct {
	itriager.Result
	componentIndex int

	// runsFailed is modified during actOnComponents.
	runsFailed int32
}

// triageComponents triages components.
//
// Doesn't modify the state itself.
//
// Returns:
//   - an action per each component that needs acting upon;
//   - indication whether the state should be stored for debugging purpose;
//   - error, if any.
func (h *Handler) triageComponents(ctx context.Context, s *State) ([]*cAction, bool, error) {
	var sup itriager.PMState
	sup, err := s.makeTriageSupporter(ctx)
	if err != nil {
		return nil, false, err
	}

	poolSize := min(concurrentComponentProcessing, len(s.PB.GetComponents()))
	now := clock.Now(ctx)

	var mutex sync.Mutex
	var actions []*cAction
	poolErr := parallel.WorkPool(poolSize, func(work chan<- func() error) {
		for i, oldC := range s.PB.GetComponents() {
			i, oldC := i, oldC
			if !needsTriage(oldC, now) {
				continue
			}
			work <- func() error {
				switch res, err := h.triageOneComponent(ctx, s, oldC, sup); {
				case errors.Is(err, itriager.ErrOutdatedPMState):
					return nil
				case err != nil:
					// Log error here since only total errs count will be propagated up
					// the stack.
					level := logging.Error
					if transient.Tag.In(err) {
						level = logging.Warning
					}
					logging.Logf(ctx, level, "%s while processing component: %s", err, protojson.Format(oldC))
					return err
				default:
					mutex.Lock()
					actions = append(actions, &cAction{componentIndex: i, Result: res})
					mutex.Unlock()
					return nil
				}
			}
		}
	})
	switch merrs, ok := poolErr.(errors.MultiError); {
	case poolErr == nil || (ok && len(merrs) == 0):
		return actions, false, nil
	case !ok:
		panic(fmt.Errorf("unexpected return from parallel.WorkPool: %s", poolErr))
	case len(actions) > 0:
		// Components are independent, so proceed despite errors on some components
		// since partial progress is better than none.
		logging.Warningf(ctx, "triageComponents: %d errors, but proceeding to act on %d components", len(merrs), len(actions))
		return actions, errors.Contains(merrs, errCaughtPanic), nil
	default:
		err := common.MostSevereError(merrs)
		return nil, false, errors.Annotate(err, "failed to triage %d components, keeping the most severe error", len(merrs)).Err()
	}
}

func needsTriage(c *prjpb.Component, now time.Time) bool {
	if c.GetTriageRequired() {
		return true // external event arrived
	}
	next := c.GetDecisionTime()
	if next == nil {
		return false // wait for an external event
	}
	if next.AsTime().After(now) {
		return false // too early
	}
	return true
}

func (h *Handler) triageOneComponent(ctx context.Context, s *State, oldC *prjpb.Component, sup itriager.PMState) (res itriager.Result, err error) {
	defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
		p.Log(ctx, "caught panic %s", p.Reason)
		// Log as a separate message under debug level to avoid sending it to Cloud
		// Error.
		logging.Debugf(ctx, "caught panic current state:\n%s", protojson.Format(s.PB))
		err = errCaughtPanic
	})
	res, err = h.ComponentTriage(ctx, oldC, sup)
	if err != nil {
		return
	}
	if res.NewValue != nil && res.NewValue == oldC {
		panic(fmt.Errorf("new value re-uses prior component object, must use copy-on-write instead"))
	}
	if len(res.CLsToPurge) > 0 {
		s.validatePurgeCLTasks(oldC, res.CLsToPurge)
	}
	return
}

// actOnComponents executes actions on components produced by triageComponents.
//
// Expects the state to already be shallow cloned.
func (h *Handler) actOnComponents(ctx context.Context, s *State, actions []*cAction) (SideEffect, error) {
	// First, create Runs in parallel.
	// As Run creation may take considerable time, use an earlier deadline to have
	// enough time to save state for other components.
	ctxRunCreation, cancel := earlierDeadline(ctx, 5*time.Second)
	defer cancel()
	poolSize := min(concurrentComponentProcessing, len(actions))
	runsErr := parallel.WorkPool(poolSize, func(work chan<- func() error) {
		for _, action := range actions {
			for _, rc := range action.RunsToCreate {
				action, rc := action, rc
				c := s.PB.GetComponents()[action.componentIndex]
				work <- func() error {
					err := h.createOneRun(ctxRunCreation, rc, c)
					if err != nil {
						atomic.AddInt32(&action.runsFailed, 1)
						// Log error here since only total errs count will be propagated up
						// the stack.
						level := logging.Error
						if transient.Tag.In(err) {
							level = logging.Warning
						}
						logging.Logf(ctx, level, "Failed to create a Run in component\n%s\ndue to error: %s", protojson.Format(c), err)
					}
					return err
				}
			}
		}
	})

	// Keep runsErr for now and try to make progress on actions/components
	// without errors.

	// Shallow-copy the components slice, as one or more components are highly
	// likely to be modified in a loop below.
	s.PB.Components = append(([]*prjpb.Component)(nil), s.PB.GetComponents()...)
	runsCreated, componentsUpdated := 0, 0
	var purgeTasks []*prjpb.PurgeCLTask
	var triggerTasks []*prjpb.TriggeringCLDepsTask
	for _, action := range actions {
		if action.runsFailed > 0 {
			continue
		}
		runsCreated += len(action.RunsToCreate)
		purgeTasks = append(purgeTasks, action.CLsToPurge...)
		// CLsToTrigger in each action is a series of the deps are chained
		// together, and the TQ handler will vote them sequentially.
		//
		// A separate task should be triggered for each action. Otherwise,
		// The deps of unrelated CLs will be handled by a single task, and
		// the CQ vote processes will be blocked by unrelated chains.
		//
		// Note that there can be more than one inflight TriggeringCLsTask
		// for the same component.
		//
		// Let's say that
		// - there are CL1,2,3,4,5 and CL1 is the bottommost CL.
		// - CQ+2 is triggered on CL3.
		// - CV processed the vote and enqueued a TriggeringCLsTask for CL1/2.
		// - Before or while the TriggeringCLsTask is being processed,
		//   * CQ+2 is triggered on CL5
		//   * CV processed the CQ vote on CL5 and
		//     enqueued another TriggeringCLsTask for CL4
		//
		// There is a very small chance that CL4 may get voted earlier than
		// CL1/2/3. However, it's not a big deal, even if it happens.
		if len(action.CLsToTriggerDeps) > 0 {
			for _, item := range action.CLsToTriggerDeps {
				triggerTasks = append(triggerTasks, &prjpb.TriggeringCLDepsTask{
					TriggeringClDeps: item,
				})
			}
		}
		if action.NewValue != nil {
			s.PB.Components[action.componentIndex] = action.NewValue
			componentsUpdated++
		}
	}
	var purgeSE SideEffect
	curPurgeCLCount := len(s.PB.PurgingCls)
	if len(purgeTasks) > 0 {
		purgeSE = h.addCLsToPurge(ctx, s, purgeTasks)
	}
	var triggerSE SideEffect
	curTriggerCLDepsCount := len(s.PB.TriggeringClDeps)
	if len(triggerTasks) > 0 {
		triggerSE = h.addCLsToTriggerDeps(ctx, s, triggerTasks)
	}
	sideEffect := NewSideEffects(purgeSE, triggerSE)
	proceedMsg := fmt.Sprintf(
		// report # of events processed and the new CLs that will be triggered
		// or purged. i.e., the existing CLs that are in the process of purge
		// or trigger are not counted.
		"proceeding to save %d components, purge %d CLs, and trigger %d CL Deps",
		componentsUpdated,
		len(s.PB.PurgingCls)-curPurgeCLCount,
		len(s.PB.TriggeringClDeps)-curTriggerCLDepsCount,
	)
	// Finally, decide the final result.
	switch merrs, ok := runsErr.(errors.MultiError); {
	case runsErr == nil || (ok && len(merrs) == 0):
		logging.Infof(ctx, "actOnComponents: created %d Runs, %s", runsCreated, proceedMsg)
		return sideEffect, nil
	case !ok:
		panic(fmt.Errorf("unexpected return from parallel.WorkPool"))
	default:
		logging.Warningf(ctx, "actOnComponents: created %d Runs, failed to create %d Runs", runsCreated, len(merrs))
		if componentsUpdated+len(purgeTasks) == 0 {
			err := common.MostSevereError(merrs)
			return nil, errors.Annotate(err, "failed to actOnComponents, most severe error").Err()
		}
		// All actions are independent, so proceed despite the errors since partial
		// progress is better than none.
		logging.Debugf(ctx, "actOnComponents: %s", proceedMsg)
		return sideEffect, nil
	}
}

func (h *Handler) createOneRun(ctx context.Context, rc *runcreator.Creator, c *prjpb.Component) (err error) {
	defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
		p.Log(ctx, "caught panic while creating a Run %s", p.Reason)
		err = errCaughtPanic
	})

	switch _, err = rc.Create(ctx, h.CLMutator, h.PMNotifier, h.RunNotifier); {
	case err == nil:
		return nil
	case runcreator.StateChangedTag.In(err):
		// This is a transient error at component action level: on retry, the Triage()
		// function will re-evaulate the state.
		return transient.Tag.Apply(err)
	default:
		return err
	}
}

// validatePurgeCLTasks verifies correctness of tasks from Triage.
//
// Modifies given tasks in place.
// Panics in case of problems.
func (s *State) validatePurgeCLTasks(c *prjpb.Component, ts []*prjpb.PurgeCLTask) {
	// First, verify individual tasks have expected fields set.
	m := make(common.CLIDsSet, len(ts))
	for _, t := range ts {
		id := t.GetPurgingCl().GetClid()
		switch {
		case id == 0:
			panic(fmt.Errorf("clid must be set"))
		case m.HasI64(id):
			panic(fmt.Errorf("duplicated clid %d", id))
		case len(t.GetPurgeReasons()) == 0:
			panic(fmt.Errorf("at least 1 reason must be given"))
		}
		for i, r := range t.GetPurgeReasons() {
			if r.GetClError().GetKind() == nil {
				panic(fmt.Errorf("PurgeReason #%d is nil", i))
			}
			if r.GetApplyTo() == nil {
				panic(fmt.Errorf("which trigger(s) the PurgeReason applies to must be specified"))
			}
		}

		m.AddI64(id)
	}
	// Verify only CLs not yet purged are being purged.
	// NOTE: This iterates all CLs currently being purged, but there should be
	// very few such CLs compared to the total number of tracked CLs.
	for _, p := range s.PB.GetPurgingCls() {
		if m.HasI64(p.GetClid()) {
			panic(fmt.Errorf("can't purge %d CL which is already being purged", p.GetClid()))
		}
	}
	// Verify only CLs from the component are being purged.
	for _, clid := range c.GetClids() {
		m.DelI64(clid)
	}
	if len(m) > 0 {
		panic(fmt.Errorf("purging %v CLs outside the component", m))
	}
}

// addCLsToPurge changes PB.PurgingCLs and prepares for atomic creation of TQ
// tasks to do actual purge.
//
// Expects given tasks to be correct (see validatePurgeCLTasks).
func (h *Handler) addCLsToPurge(ctx context.Context, s *State, ts []*prjpb.PurgeCLTask) SideEffect {
	if len(ts) == 0 {
		return nil
	}
	s.populatePurgeCLTasks(ctx, ts)
	purgingCLs := make([]*prjpb.PurgingCL, len(ts))
	for i, t := range ts {
		purgingCLs[i] = t.GetPurgingCl()
	}
	s.PB.PurgingCls, _ = s.PB.COWPurgingCLs(nil, purgingCLs)
	return &TriggerPurgeCLTasks{payloads: ts, clPurger: h.CLPurger}
}

// addCLsTriggerDeps updates PB.TriggerCLDeps and prepares TQ tasks for the CLs
// to trigger.
func (h *Handler) addCLsToTriggerDeps(ctx context.Context, s *State, ts []*prjpb.TriggeringCLDepsTask) SideEffect {
	if len(ts) == 0 {
		return nil
	}
	deadline := timestamppb.New(clock.Now(ctx).Add(prjpb.MaxTriggeringCLDepsDuration))
	opInt := deadline.AsTime().Unix()
	payloads := make([]*prjpb.TriggeringCLDeps, len(ts))
	for i, t := range ts {
		payload := t.GetTriggeringClDeps()
		payload.OperationId = fmt.Sprintf("%d-%d", opInt, payload.GetOriginClid())
		payload.Deadline = deadline
		payloads[i] = payload
		t.LuciProject = s.PB.GetLuciProject()
	}
	s.PB.TriggeringClDeps, _ = s.PB.COWTriggeringCLDeps(nil, payloads)
	return &ScheduleTriggeringCLDepsTasks{payloads: ts, clTriggerer: h.CLTriggerer}
}

// maxPurgingCLDuration limits the time that a TQ task has to execute
// PurgeCLTask.
const maxPurgingCLDuration = 10 * time.Minute

// populatePurgeCLTasks populates all remaining fields in PurgeCLsTasks created
// by Triage.
//
// Modifies given tasks in place.
func (s *State) populatePurgeCLTasks(ctx context.Context, ts []*prjpb.PurgeCLTask) {
	deadline := timestamppb.New(clock.Now(ctx).Add(maxPurgingCLDuration))
	opInt := deadline.AsTime().Unix()
	for _, t := range ts {
		id := t.GetPurgingCl().GetClid()
		pcl := s.PB.GetPcls()[s.pclIndex[common.CLID(id)]]

		t.LuciProject = s.PB.GetLuciProject()
		t.PurgingCl.Deadline = deadline
		t.PurgingCl.OperationId = fmt.Sprintf("%d-%d", opInt, id)
		t.ConfigGroups = make([]string, len(pcl.GetConfigGroupIndexes()))
		for i, idx := range pcl.GetConfigGroupIndexes() {
			id := prjcfg.MakeConfigGroupID(s.PB.GetConfigHash(), s.PB.ConfigGroupNames[idx])
			t.ConfigGroups[i] = string(id)
		}
	}
}

func (s *State) makeTriageSupporter(ctx context.Context) (*triageSupporter, error) {
	if s.configGroups == nil {
		meta, err := prjcfg.GetHashMeta(ctx, s.PB.GetLuciProject(), s.PB.GetConfigHash())
		if err != nil {
			return nil, err
		}
		if s.configGroups, err = meta.GetConfigGroups(ctx); err != nil {
			return nil, err
		}
	}
	s.ensurePCLIndex()
	purging := make(map[int64]*prjpb.PurgingCL, len(s.PB.GetPurgingCls()))
	for _, p := range s.PB.GetPurgingCls() {
		purging[p.GetClid()] = p
	}
	triggering := make(map[int64]*prjpb.TriggeringCLDeps, len(s.PB.GetTriggeringClDeps()))
	for _, t := range s.PB.GetTriggeringClDeps() {
		triggering[t.GetOriginClid()] = t
	}

	return &triageSupporter{
		pcls:             s.PB.GetPcls(),
		pclIndex:         s.pclIndex,
		purging:          purging,
		triggeringCLDeps: triggering,
		configGroups:     s.configGroups,
	}, nil
}

// triageSupporter provides limited access to resources of PM state.
//
// Implements itriager.PMState.
type triageSupporter struct {
	pcls             []*prjpb.PCL
	pclIndex         map[common.CLID]int
	purging          map[int64]*prjpb.PurgingCL
	triggeringCLDeps map[int64]*prjpb.TriggeringCLDeps
	configGroups     []*prjcfg.ConfigGroup
}

var _ itriager.PMState = (*triageSupporter)(nil)

func (a *triageSupporter) PCL(clid int64) *prjpb.PCL {
	i, ok := a.pclIndex[common.CLID(clid)]
	if !ok {
		return nil
	}
	return a.pcls[i]
}

func (a *triageSupporter) PurgingCL(clid int64) *prjpb.PurgingCL {
	return a.purging[clid]
}

func (a *triageSupporter) ConfigGroup(index int32) *prjcfg.ConfigGroup {
	return a.configGroups[index]
}

func (a *triageSupporter) TriggeringCLDeps(clid int64) *prjpb.TriggeringCLDeps {
	return a.triggeringCLDeps[clid]
}

func markForTriage(in []*prjpb.Component) []*prjpb.Component {
	out := make([]*prjpb.Component, len(in))
	for i, c := range in {
		if !c.GetTriageRequired() {
			c = c.CloneShallow()
			c.TriageRequired = true
		}
		out[i] = c
	}
	return out
}

func markForTriageOnChangedPCLs(in []*prjpb.Component, pcls []*prjpb.PCL, changed common.CLIDsSet) []*prjpb.Component {
	// For each changed CL `A`, expand changed set to include all CLs `B` such
	// that B depends on A.
	reverseDeps := make(map[int64][]int64, len(pcls)) // `A` -> all such `B` CLs
	for _, p := range pcls {
		for _, dep := range p.GetDeps() {
			reverseDeps[dep.GetClid()] = append(reverseDeps[dep.GetClid()], p.GetClid())
		}
	}
	expanded := make(common.CLIDsSet, len(changed))
	var expand func(int64)
	expand = func(clid int64) {
		if expanded.HasI64(clid) {
			return
		}
		expanded.AddI64(clid)
		for _, revDep := range reverseDeps[clid] {
			expand(revDep)
		}
	}
	for clid := range changed {
		expand(int64(clid))
	}

	out := make([]*prjpb.Component, len(in))
	for i, c := range in {
		if !c.GetTriageRequired() {
			for _, clid := range c.GetClids() {
				if expanded.HasI64(clid) {
					c = c.CloneShallow()
					c.TriageRequired = true
					break
				}
			}
		}
		out[i] = c
	}
	return out
}

func earlierDeadline(ctx context.Context, reserve time.Duration) (context.Context, context.CancelFunc) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return ctx, func() {} // no deadline
	}
	return clock.WithDeadline(ctx, deadline.Add(-reserve))
}
