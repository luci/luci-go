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

package state

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/gerrit/cfgmatcher"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// reevalPCLs re-evaluates PCLs after a project config change.
//
// If components have to be re-evaluated, only marks PB.DirtyComponents.
func (s *State) reevalPCLs(ctx context.Context) error {
	cls, errs, err := s.loadCLsForPCLs(ctx)
	if err != nil {
		return err
	}
	newPCLs := make([]*prjpb.PCL, len(cls))
	for i, cl := range cls {
		old := s.PB.GetPcls()[i]
		switch pcl, err := s.makePCLFromDS(ctx, cl, errs[i], old); {
		case err != nil:
			return err
		case pcl == nil:
			panic("makePCLFromDS is wrong")
		default:
			// TODO(tandrii): avoid updating components if the new PCL is exact same
			// as the old one.
			newPCLs[i] = pcl
		}
	}
	s.PB.Pcls = newPCLs
	s.PB.DirtyComponents = true
	s.pclIndex = nil
	return nil
}

// evalUpdatedCLs updates/inserts PCLs.
func (s *State) evalUpdatedCLs(ctx context.Context, clmap map[common.CLID]int64) error {
	cls := make([]*changelist.CL, 0, len(clmap))
	for id := range clmap {
		cls = append(cls, &changelist.CL{ID: id})
	}
	return s.evalCLsFromDS(ctx, cls)
}

// evalCLsFromDS adds, updates, and marks for deletion PCLs based on CLs in
// Datastore.
//
// Sorts passed cls slice and updates it with loaded from DS info.
func (s *State) evalCLsFromDS(ctx context.Context, cls []*changelist.CL) error {
	if s.cfgMatcher == nil {
		meta, err := config.GetHashMeta(ctx, s.PB.GetLuciProject(), s.PB.GetConfigHash())
		if err != nil {
			return err
		}
		if s.configGroups, err = meta.GetConfigGroups(ctx); err != nil {
			return err
		}
		s.cfgMatcher = cfgmatcher.LoadMatcherFromConfigGroups(ctx, s.configGroups, &meta)
	}

	// Sort new/updated CLs in the way as PCLs already are, namely by CL ID. Do it
	// before loading from Datastore beacuse `errs` must correspond to `cls`.
	changelist.Sort(cls)
	cls, errs, err := loadCLs(ctx, cls)
	if err != nil {
		return err
	}

	// Now, while copying old PCLs slice into new PCLs slice,
	// insert new PCL objects for new CLs and replace PCL objects for updated CLs.
	// This preserves the property of PCLs being sorted.
	// Since we have to re-create PCLs slice anyways, it's fastest to merge two
	// sorted in the same way PCLs and CLs slices in  O(len(PCLs) + len(cls)).
	oldPCLs := s.PB.GetPcls()
	newPCLs := make([]*prjpb.PCL, 0, len(oldPCLs)+len(cls))
	changed := false
	for i, cl := range cls {
		// Copy all old PCLs before this CL.
		for len(oldPCLs) > 0 && common.CLID(oldPCLs[0].GetClid()) < cl.ID {
			newPCLs = append(newPCLs, oldPCLs[0])
			oldPCLs = oldPCLs[1:]
		}
		// If CL is updated, pop oldPCL.
		var oldPCL *prjpb.PCL
		if len(oldPCLs) > 0 && common.CLID(oldPCLs[0].GetClid()) == cl.ID {
			oldPCL = oldPCLs[0]
			oldPCLs = oldPCLs[1:]
		}
		// Compute new PCL.
		switch pcl, err := s.makePCLFromDS(ctx, cl, errs[i], oldPCL); {
		case err != nil:
			return err
		case pcl == nil && oldPCL != nil:
			panic("makePCLFromDS is wrong")
		case pcl == nil:
			// New CL, but not in datastore. Don't add anything to newPCLs.
			// This weird case was logged by makePCLFromDS already.
		default:
			// TODO(tandrii): avoid updating components if the new PCL is exact same
			// as the old one.
			newPCLs = append(newPCLs, pcl)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	// Copy remaining oldPCLs.
	for len(oldPCLs) > 0 {
		newPCLs = append(newPCLs, oldPCLs[0])
		oldPCLs = oldPCLs[1:]
	}
	s.PB.Pcls = newPCLs
	s.PB.DirtyComponents = true
	s.pclIndex = nil
	return nil
}

// filterOutUpToDate returns map[clid]eversion for which PCL is out of date or
// doens't exist.
func (s *State) filterOutUpToDate(events []*prjpb.CLUpdated) map[common.CLID]int64 {
	res := make(map[common.CLID]int64, len(events))
	for _, e := range events {
		id := common.CLID(e.GetClid())
		if ev, exists := res[id]; !exists || ev < e.GetEversion() {
			res[id] = e.GetEversion()
		}
	}
	// This isn't the most efficient way when P=len(PCLs) >> E=len(events)
	// (e.g. O(E*log(P)) is possible by iterating sorted CLID events
	// and doing binary search on PCLs at each step), but it mostly works.
	for _, pcl := range s.PB.GetPcls() {
		id := common.CLID(pcl.GetClid())
		if ev, exists := res[id]; exists && ev <= pcl.GetEversion() {
			delete(res, id)
		}
	}
	return res
}

func (s *State) makePCLFromDS(ctx context.Context, cl *changelist.CL, err error, old *prjpb.PCL) (*prjpb.PCL, error) {
	switch {
	case err == datastore.ErrNoSuchEntity:
		oldEversion := int64(0)
		if old == nil {
			logging.Errorf(ctx, "New CL %d not in Datastore", cl.ID)
		} else {
			// Must not happen outside of extremely rare un-deletion of a project
			// whose PM state references long ago wiped out CLs.
			logging.Errorf(ctx, "Old CL %d no longer in Datastore", cl.ID)
			oldEversion = old.GetEversion()
		}
		return &prjpb.PCL{
			Clid:     int64(cl.ID),
			Eversion: oldEversion,
			Status:   prjpb.PCL_DELETED,
		}, nil
	case err != nil:
		return nil, errors.Annotate(err, "failed to load CL %d", cl.ID).Tag(transient.Tag).Err()
	default:
		// TODO(tandrii): return pointer to oldPCL if pcl has the same contents.
		return s.makePCL(ctx, cl), nil
	}
}

// makePCL creates new PCL based on Datastore CL entity and current config.
func (s *State) makePCL(ctx context.Context, cl *changelist.CL) *prjpb.PCL {
	if s.cfgMatcher == nil {
		panic("cfgMather must be initialized")
	}
	pcl := &prjpb.PCL{
		Clid:     int64(cl.ID),
		Eversion: int64(cl.EVersion),
		Status:   prjpb.PCL_UNKNOWN,
	}
	if cl.Snapshot == nil {
		return pcl
	}
	if cl.Snapshot.GetGerrit() == nil {
		panic("only Gerrit CLs supported for now")
	}

	// Check ApplicableConfig as recorded by CL updater. This may be stale if one
	// LUCI project transfered responsibility for a CL to another LUCI project,
	// but eventually it'll catch up.
	var ap *changelist.ApplicableConfig_Project
	for _, proj := range cl.ApplicableConfig.GetProjects() {
		if proj.GetName() == s.PB.GetLuciProject() {
			ap = proj
			break
		}
	}
	switch t := cl.ApplicableConfig.GetUpdateTime().AsTime(); {
	case ap == nil:
		logging.Warningf(ctx, "CL(%d) is not watched by us as of %s", cl.ID, t)
		pcl.Status = prjpb.PCL_UNWATCHED
		return pcl
	case len(cl.ApplicableConfig.GetProjects()) > 1:
		logging.Warningf(ctx, "CL(%d) is watched by more than 1 project as of %s", cl.ID, t)
		pcl.Status = prjpb.PCL_UNWATCHED
		return pcl
	}

	if proj := cl.Snapshot.GetLuciProject(); proj != s.PB.GetLuciProject() {
		// Typically, this shouldn't happen: CL are known to Project Manager (PM)
		// based on notifications of CLUpdater, which normally notifies PM only
		// after taking snapshot in the context of the applicable project.
		// So, wait for snapshot to be re-taken in the context of this project.
		logging.Warningf(ctx, "CL(%d) is snapshotted by %q, not %q", cl.ID, proj, s.PB.GetLuciProject())
		return pcl
	}

	pcl.Status = prjpb.PCL_OK
	s.setApplicableConfigGroups(ap, cl.Snapshot, pcl)
	pcl.Deps = cl.Snapshot.GetDeps()
	ci := cl.Snapshot.GetGerrit().GetInfo()
	if ci.GetStatus() == gerritpb.ChangeStatus_MERGED {
		pcl.Submitted = true
	} else {
		// TODO(tandrii): stop storing triggering user's email
		pcl.Trigger = trigger.Find(cl.Snapshot.GetGerrit().GetInfo())
	}
	return pcl
}

// setApplicableConfigGroups sets ConfigGroup indexes of PCL.
//
// If provided ApplicableConfig_Project is up to date, uses config groups from
// it. Otherwise, matches against project config directly using s.cfgMatcher.
//
// Expects s.cfgMatcher to be set.
//
// Modifies the passed PCL.
func (s *State) setApplicableConfigGroups(ap *changelist.ApplicableConfig_Project, snapshot *changelist.Snapshot, pcl *prjpb.PCL) {
	// Most likely, ApplicableConfig stored in a CL entity is still up-to-date.
	if uptodate := s.tryUsingApplicableConfigGroups(ap, pcl); uptodate {
		return
	}
	// Project's config has been updated after CL snapshot was made.
	g := snapshot.GetGerrit()
	ci := g.GetInfo()
	for _, id := range s.cfgMatcher.Match(g.GetHost(), ci.GetProject(), ci.GetRef()) {
		index := s.indexOfConfigGroup(config.ConfigGroupID(id))
		pcl.ConfigGroupIndexes = append(pcl.ConfigGroupIndexes, index)
	}
}

// tryUsingApplicableConfigGroups sets ConfigGroup indexes of a PCL based on
// ApplicableConfig_Project if ApplicableConfig_Project references the State's
// config hash.
//
// Modifies the passed PCL.
// Returns whether config hash matched.
func (s *State) tryUsingApplicableConfigGroups(ap *changelist.ApplicableConfig_Project, pcl *prjpb.PCL) bool {
	expectedConfigHash := s.PB.GetConfigHash()
	// At least 1 ID is guaranteed in ApplicableConfig_Project by gerrit.gobmap.
	for _, id := range ap.GetConfigGroupIds() {
		if config.ConfigGroupID(id).Hash() != expectedConfigHash {
			return false
		}
	}
	for _, id := range ap.GetConfigGroupIds() {
		index := s.indexOfConfigGroup(config.ConfigGroupID(id))
		pcl.ConfigGroupIndexes = append(pcl.ConfigGroupIndexes, index)
	}
	return true
}

// loadCLsForPCLs loads CLs from Datastore corresponding to PCLs.
//
// Returns slice of CLs, error.MultiError slice corresponding to
// per-CL errors *(always exists and has same length as CLs)*, and a
// top level error if it can't be attributed to any CL.
//
// *each error in per-CL errors* is not annotated and is nil if CL was loaded
// successfully.
func (s *State) loadCLsForPCLs(ctx context.Context) ([]*changelist.CL, errors.MultiError, error) {
	cls := make([]*changelist.CL, len(s.PB.GetPcls()))
	for i, pcl := range s.PB.GetPcls() {
		cls[i] = &changelist.CL{ID: common.CLID(pcl.GetClid())}
	}
	return loadCLs(ctx, cls)
}

func loadCLs(ctx context.Context, cls []*changelist.CL) ([]*changelist.CL, errors.MultiError, error) {
	// At 0.007 KiB (serialized) per CL as of Jan 2021, this should scale 2000 CLs
	// with reasonable RAM footprint and well within 10s because
	// changelist.LoadMulti splits it into concurrently queried batches.
	// To support more, CLs would need to be loaded and processed in batches,
	// or CL snapshot size reduced.
	err := changelist.LoadMulti(ctx, cls)
	switch merr, ok := err.(errors.MultiError); {
	case err == nil:
		return cls, make(errors.MultiError, len(cls)), nil
	case ok:
		return cls, merr, nil
	default:
		return nil, nil, errors.Annotate(err, "failed to load %d CLs", len(cls)).Tag(transient.Tag).Err()
	}
}
