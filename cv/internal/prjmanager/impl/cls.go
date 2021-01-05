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
	"sort"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/gerrit/cfgmatcher"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/internal"
)

// pendingCLs handles mutations to the PendingCLs part of Project Manager state.
//
// The pendingCLs are modified via copy-on-write a.k.a. COW
// (https://en.wikipedia.org/wiki/Copy-on-write).
type pendingCLs struct {
	*prjmanager.PendingCLs // the only serialized field.

	// configHash duplicates that of state.configHash to avoid having to pass it to each
	// method all the time.
	configHash string
	// luciProject is immutable field, stored for convinience to avoid passing it
	// through args all the time.
	luciProject string
	// cfgmatcher matches Gerrit CLs to config groups. Cached. Tied to specific
	// configHash.
	cfgmatcher *cfgmatcher.Matcher
}

func (p *pendingCLs) cloneShallow() *pendingCLs {
	r := &pendingCLs{}
	*r = *p
	orig := p.PendingCLs
	if orig != nil {
		r.PendingCLs = &prjmanager.PendingCLs{
			Cls:              orig.GetCls(),
			Components:       orig.GetComponents(),
			ConfigGroupNames: orig.GetConfigGroupNames(),
			DirtyComponents:  orig.GetDirtyComponents(),
		}
	}
	return r
}

func (p *pendingCLs) clonePCLsForGrowth(expectedNewCLs int) []*prjmanager.PendingCL {
	old := p.PendingCLs.GetCls()
	dst := make([]*prjmanager.PendingCL, len(old), len(old)+expectedNewCLs)
	copy(dst, old)
	return dst
}

func (p *pendingCLs) cloneComponents() []*prjmanager.Component {
	old := p.PendingCLs.GetComponents()
	dst := make([]*prjmanager.Component, len(old))
	copy(dst, old)
	return dst
}

////////////////////////////////////////////////////////////////////////////////
// High level mutations.

// updateConfig re-evaluates currently pending CLs and their components
// according to the new config.
//
// This evaluation doesn't affect incomplete Runs, which are owned by their
// respective Run Managers, which are in turn responsible for canceling these
// Runs if their Run isn't viable in the new config.
func (p *pendingCLs) updateConfig(ctx context.Context, meta config.Meta) (*pendingCLs, eventbox.SideEffectFn, error) {
	p = p.cloneShallow()
	p.configHash = meta.Hash()
	// reset cfgmatcher, since this is a new config version.
	p.cfgmatcher = nil

	// This is very inefficient and doesn't preserve state of components,
	// and so may need to be revised for more complicatd Run creation algorithms.
	p.PendingCLs.Components = nil
	p.DirtyComponents = true

	p.PendingCLs.ConfigGroupNames = make([]string, len(meta.ConfigGroupIDs))
	for i, id := range meta.ConfigGroupIDs {
		p.PendingCLs.ConfigGroupNames[i] = id.Name()
	}

	cls := make([]*changelist.CL, len(p.PendingCLs.GetCls()))
	for i, pcl := range p.PendingCLs.GetCls() {
		cls[i] = &changelist.CL{ID: common.CLID(pcl.GetClid())}
	}
	errs, err := loadMultiCLs(ctx, cls)
	if err != nil {
		return nil, nil, err
	}

	newPCLs := make([]*prjmanager.PendingCL, 0, len(cls))
	for i, old := range p.PendingCLs.GetCls() {
		switch err := errs[i]; {
		case err == datastore.ErrNoSuchEntity:
			// This must not normally happen.
			// However, if this does occur, it's better to avoid getting stuck.
			logging.Errorf(ctx, "CL %d no longer in Datastore, dropping it", old.GetClid())
		case err != nil:
			return nil, nil, errors.Annotate(err, "failed to load CL %d", old.GetClid()).Tag(transient.Tag).Err()
		default:
			pcl, err := p.makePendingCL(ctx, cls[i])
			if err != nil {
				return nil, nil, errors.Annotate(err, "failed to create PendingCL %d", old.GetClid()).Err()
			}
			newPCLs = append(newPCLs, pcl)
		}
	}
	// Since old PCLs were sorted by CLID and the new ones were created in exact
	// same order, possibly dropping a few CLs, the sorted order is preserved.
	p.PendingCLs.Cls = newPCLs
	return p, nil, nil
}

// poke checks and defers if any action is overdue.
func (p *pendingCLs) poke(ctx context.Context) (*pendingCLs, eventbox.SideEffectFn, error) {
	// TODO
	return p, nil, nil
}

// updateCLs updates pendingCLs given new version of some CL(s).
func (p *pendingCLs) updateCLs(ctx context.Context, events []*internal.CLUpdated) (*pendingCLs, eventbox.SideEffectFn, error) {
	toUpdateIndexes, newEvents := p.clsForUpdate(events)
	if len(toUpdateIndexes)+len(newEvents) == 0 {
		return p, nil, nil
	}
	p = p.cloneShallow()
	p.PendingCLs.Cls = p.clonePCLsForGrowth(len(newEvents))

	// Load all updated + new CLs together.
	cls := make([]*changelist.CL, len(toUpdateIndexes)+len(newEvents))
	for i, index := range toUpdateIndexes {
		pcl := p.PendingCLs.GetCls()[index]
		cls[i] = &changelist.CL{ID: common.CLID(pcl.GetClid())}
	}
	for i, e := range newEvents {
		cls[len(toUpdateIndexes)+i] = &changelist.CL{ID: common.CLID(e.GetClid())}
	}
	errs, err := loadMultiCLs(ctx, cls)
	if err != nil {
		return nil, nil, err
	}

	// Work through CLs, recording indexes of CLIDs which are either no
	// longer tracked or were updated.
	updatedCLIDs := make(map[common.CLID]struct{}, len(cls))
	newAdded := false
	for i, cl := range cls {
		isNew := i >= len(toUpdateIndexes)
		switch err := errs[i]; {
		case err == datastore.ErrNoSuchEntity:
			// This must not normally happen.
			if isNew {
				logging.Errorf(ctx, "CL %d from CLUpdate event no longer in Datastore", cl.ID)
			} else {
				logging.Errorf(ctx, "CL %d no longer in Datastore, dropping it", cl.ID)
				p.PendingCLs.GetCls()[i] = nil
				updatedCLIDs[cl.ID] = struct{}{}
			}
		case err != nil:
			return nil, nil, errors.Annotate(err, "failed to load CL %d", cl.ID).Tag(transient.Tag).Err()
		default:
			pcl, err := p.makePendingCL(ctx, cl)
			if err != nil {
				return nil, nil, errors.Annotate(err, "failed to create PendingCL %d", cl.ID).Err()
			}
			if isNew {
				newAdded = true
				p.PendingCLs.Cls = append(p.PendingCLs.GetCls(), pcl)
			} else {
				// TODO(tandrii): compare new and old pcl since most CL updates are noops.
				p.PendingCLs.GetCls()[i] = pcl
				updatedCLIDs[cl.ID] = struct{}{}
			}
		}
	}
	p.sortAndCompactPCLs()

	if newAdded {
		// Just re-compute all components.
		p.DirtyComponents = true
	} else {
		p.PendingCLs.Components = p.cloneComponents()
		for _, c := range p.PendingCLs.GetComponents() {
			for _, id := range c.GetPendingIds() {
				if _, ok := updatedCLIDs[common.CLID(id)]; ok {
					c.Dirty = true
					break
				}
			}
		}
	}
	return p, nil, nil
}

// onRunsCreated updates pendingCL after successful creation of new Run(s).
func (p *pendingCLs) onRunsCreated(ctx context.Context, created common.RunIDs) (*pendingCLs, eventbox.SideEffectFn, error) {
	// TODO
	return p, nil, nil
}

// onRunsFinished updates pendingCL after completion of some Run(s).
func (p *pendingCLs) onRunsFinished(ctx context.Context, finished common.RunIDs) (*pendingCLs, eventbox.SideEffectFn, error) {
	// Noop for compatibility with CQDaemon.
	// In CQDaemon, once a CL component with CQ votes (CQ "Candidate") differs
	// from current Run (CQ "Attempt"), CQDaemon immediately stops working on the
	// current Run and start working on the new component as the new Run.
	// TODO(tandrii): revisit this once CV handles all possible causes of Run
	// finalization (notably, submits).
	return p, nil, nil
}

// execDeferred performs any actions (e.g. create new Runs) deferred previously.
//
// This is done at the end s.t. all other mutations have already been recorded
// by the time important decisions are done.
func (p *pendingCLs) execDeferred(ctx context.Context) (*pendingCLs, eventbox.SideEffectFn, error) {
	// TODO
	return p, nil, nil
}

////////////////////////////////////////////////////////////////////////////////
// Helpers for creation of a new PendingCL.

// unknownConfigGroupIndex is value of config_group_index of pendingCL proto
// message indicating that applicable config group(s) is(are) not known yet.
const unknownConfigGroupIndex = -1

// makePendingCL creates and returns a PendingCL to track.
//
// Returns nil pendingCL if it should not be tracked.
func (p *pendingCLs) makePendingCL(ctx context.Context, cl *changelist.CL) (*prjmanager.PendingCL, error) {
	pcl := &prjmanager.PendingCL{
		Clid:     int64(cl.ID),
		Eversion: int64(cl.EVersion),
		// By default, track & wait until details are discovered.
		ConfigGroupIndex: unknownConfigGroupIndex,
	}
	if cl.Snapshot == nil {
		return pcl, nil
	}

	if cl.Snapshot.GetGerrit() == nil {
		panic("only Gerrit CLs supported for now")
	}

	var ap *changelist.ApplicableConfig_Project
	for _, proj := range cl.ApplicableConfig.GetProjects() {
		if proj.GetName() == p.luciProject {
			ap = proj
			break
		}
	}
	switch t := cl.ApplicableConfig.GetUpdateTime().AsTime(); {
	case ap == nil:
		logging.Warningf(ctx, "CL(%d) is not watched by %s as of %s", cl.ID, p.luciProject, t)
		return nil, nil
	case len(cl.ApplicableConfig.GetProjects()) > 1:
		logging.Warningf(ctx, "CL(%d) is watched by more than 1 project as of %s", cl.ID, t)
		return nil, nil
	}

	if proj := cl.Snapshot.GetLuciProject(); proj != p.luciProject {
		// Typically, this shouldn't happen: CL are known to Project Manager (PM)
		// based on notifications of CLUpdater, which normally notifies PM only
		// after taking snapshot in the context of the applicable project.
		// So, wait for snapshot to be re-taken in the context of this project.
		logging.Warningf(ctx, "CL(%d) is snapshotted by %q, not %q", cl.ID, proj, p.luciProject)
		return pcl, nil
	}

	// Most likely, ApplicableConfig stored in a CL entity is still up-to-date.
	if uptodate := p.tryUsingApplicableConfigGroups(ap, pcl); !uptodate {
		// Unless Project's config has just been updated after CL entity was
		// updated.
		if err := p.setApplicableConfigGroups(ctx, cl.Snapshot, pcl); err != nil {
			return nil, err
		}
	}

	pcl.Deps = cl.Snapshot.GetDeps()
	pcl.Trigger = trigger.Find(cl.Snapshot.GetGerrit().GetInfo())
	if pcl.Trigger == nil {
		logging.Debugf(ctx, "CL(%d) is not triggered", cl.ID)
		return nil, nil
	}
	return pcl, nil
}

// tryUsingApplicableConfigGroups sets ConfigGroup indexes of pendingCL based on
// ApplicableConfig_Project, if ApplicableConfig_Project references the expected
// config hash.
//
// Returns whether config hash matched.
func (p *pendingCLs) tryUsingApplicableConfigGroups(ap *changelist.ApplicableConfig_Project, pcl *prjmanager.PendingCL) bool {
	// At least 1 ID is guaranteed in ApplicableConfig_Project by gerrit.gobmap.
	for _, id := range ap.GetConfigGroupIds() {
		if config.ConfigGroupID(id).Hash() != p.configHash {
			return false
		}
	}
	for _, id := range ap.GetConfigGroupIds() {
		p.addConfigGroupName(config.ConfigGroupID(id), pcl)
	}
	return true
}

// setApplicableConfigGroups sets ConfigGroup indexes of pendingCL by matching
// it against project config directly.
func (p *pendingCLs) setApplicableConfigGroups(ctx context.Context, s *changelist.Snapshot, pcl *prjmanager.PendingCL) error {
	if err := p.ensureCfgMatcher(ctx); err != nil {
		return err
	}
	g := s.GetGerrit()
	ci := g.GetInfo()
	for _, id := range p.cfgmatcher.Match(g.GetHost(), ci.GetProject(), ci.GetRef()) {
		p.addConfigGroupName(id, pcl)
	}
	return nil
}

// addConfigGroupName adds applicable ConfigGroup name to the pendingCL.
func (p *pendingCLs) addConfigGroupName(id config.ConfigGroupID, pcl *prjmanager.PendingCL) {
	switch index := p.indexOfConfigGroup(id); {
	case pcl.GetConfigGroupIndex() == unknownConfigGroupIndex:
		pcl.ConfigGroupIndex = index
	default:
		pcl.ExcessConfigGroupIndexes = append(pcl.ExcessConfigGroupIndexes, index)
	}
}

func (p *pendingCLs) indexOfConfigGroup(id config.ConfigGroupID) int32 {
	if id.Hash() != p.configHash {
		// TODO(tandrii): remove quick sanity check.
		panic(fmt.Errorf("given %s != expected hash %s", id, p.configHash))
	}
	want := id.Name()
	// This can be optimized by lazyly creating and caching a map from
	// ConfigGroupID to its index, but most projects have <10 groups,
	// so iterating a slice is faster & good enough.
	names := p.PendingCLs.GetConfigGroupNames()
	for i, name := range names {
		if name == want {
			return int32(i)
		}
	}
	panic(fmt.Errorf("%s doesn't match any in known %s ConfigGroupNames", id, names))
}

func (p *pendingCLs) ensureCfgMatcher(ctx context.Context) error {
	if p.cfgmatcher != nil {
		return nil
	}
	var err error
	p.cfgmatcher, err = cfgmatcher.LoadMatcher(ctx, p.luciProject, p.configHash)
	return err
}

////////////////////////////////////////////////////////////////////////////////
// Helpers for updateCL.

// clsForUpdate returns indexes of pendingCLs to update and subsequence of
// incoming slice containing just new CLs.
//
// The returned toUpdateIndexes are monotonically increasing.
func (p *pendingCLs) clsForUpdate(events []*internal.CLUpdated) (toUpdateIndexes []int, newEvents []*internal.CLUpdated) {
	// This algo is O(len(cls) + len(PCLs)).
	// Observation: # of CLUpdated events consumed is usually << # of PCLs.
	// TODO(tandrii): since PCLs are sorted by CLID, iterate over cls and use
	// binary search on PCLs to achieve O(log(PCLs)*len(cls)).
	m := make(map[int64]*internal.CLUpdated, len(events))
	for _, e := range events {
		m[e.GetClid()] = e
	}
	for i, pcl := range p.PendingCLs.GetCls() {
		id := pcl.GetClid()
		e, ok := m[id]
		if !ok {
			continue
		}
		delete(m, id)
		if e.GetEversion() <= pcl.GetEversion() {
			continue
		}
		toUpdateIndexes = append(toUpdateIndexes, i)
	}
	if len(m) == 0 {
		return toUpdateIndexes, nil
	}
	newEvents = make([]*internal.CLUpdated, len(m))
	for _, e := range m {
		newEvents = append(newEvents, e)
	}
	return toUpdateIndexes, newEvents
}

// sortAndCompactPCLs removes all nils from p.pendingCLs.Cls and keeps them
// sorted by CL ID.
func (p *pendingCLs) sortAndCompactPCLs() {
	// kicking nils to the end.
	s := p.PendingCLs.GetCls()
	sort.Slice(s, func(i, j int) bool {
		switch a, b := s[i], s[j]; {
		case a == nil:
			return false // kick nils to the end of slice.
		case b == nil:
			return true
		default:
			return a.GetClid() < b.GetClid()
		}
	})
	p.PendingCLs.Cls = s
}

////////////////////////////////////////////////////////////////////////////////
// Garbage Helpers.

// loadMultiCLs loads multiple CLs from Datastore and returns error.MultiError
// slice with an error per CL loaded even if there were no errors.
//
// Returns the usual error only if it is not CL specific.
func loadMultiCLs(ctx context.Context, cls []*changelist.CL) (errors.MultiError, error) {
	err := changelist.LoadMulti(ctx, cls)
	switch merr, ok := err.(errors.MultiError); {
	case err == nil:
		return make(errors.MultiError, len(cls)), nil
	case ok:
		return merr, nil
	default:
		return nil, errors.Annotate(err, "failed to load %d CLs", len(cls)).Tag(transient.Tag).Err()
	}
}

// deferPartition defers re-partitioning of pending CLs into components.
func (p *pendingCLs) deferPartition() *pendingCLs {
	if !p.PendingCLs.GetDirtyComponents() {
		p = p.cloneShallow()
		p.PendingCLs.DirtyComponents = true
	}
	return p
}

// deferComponentEval defers re-evaluating of a specific component.
//
// Mutates incoming indexes slice.
func (p *pendingCLs) deferComponentsEval(indexes ...int) *pendingCLs {
	p = p.cloneShallow()
	orig := p.PendingCLs.Components

	sort.Ints(indexes)
	// Add sentinel s.t. indexes is never empty.
	indexes = append(indexes, len(orig))
	p.PendingCLs.Components = make([]*prjmanager.Component, len(orig))
	for i, c := range orig {
		dirty := false
		if i == indexes[0] {
			dirty = true
			indexes = indexes[1:]
		}
		if c.GetDirty() || !dirty {
			p.PendingCLs.Components[i] = c
		} else {
			p.PendingCLs.Components[i] = &prjmanager.Component{
				PendingIds:   c.GetPendingIds(),
				DecisionTime: c.GetDecisionTime(),
				Dirty:        true,
			}
		}
	}
	return p
}
