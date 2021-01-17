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
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager/internal"
)

// reevalPCLs re-evaluates PCLs after a project config change.
//
// If components have to be re-evaluated, only marks PB.DirtyComponents.
func (s *State) reevalPCLs(ctx context.Context) error {
	cls, errs, err := s.loadCLsForPCLs(ctx)
	if err != nil {
		return err
	}
	newPCLs := make([]*internal.PCL, len(cls))
	for i, cl := range cls {
		switch err := errs[i]; {
		case err == datastore.ErrNoSuchEntity:
			// Must not happen outside of extremely rare un-deletion of a project
			// whose PM state references long ago wiped out CLs.
			logging.Errorf(ctx, "CL %d no longer in Datastore", cl.ID)
			old := s.PB.GetPcls()[i]
			newPCLs[i] = &internal.PCL{
				Clid:     old.GetClid(),
				Eversion: old.GetEversion(),
				Status:   internal.PCL_DELETED,
			}
		case err != nil:
			return errors.Annotate(err, "failed to load CL %d", cl.ID).Tag(transient.Tag).Err()
		default:
			// TODO(tandrii): avoid updating components if the new PCL is exact same
			// as old one.
			newPCLs[i] = s.makePCL(ctx, cls[i])
		}
	}
	s.PB.Pcls = newPCLs
	s.PB.DirtyComponents = true
	return nil
}

// makePCL creates new PCL based on Datastore CL entity and current config.
func (s *State) makePCL(ctx context.Context, cl *changelist.CL) *internal.PCL {
	if s.cfgMatcher == nil {
		panic("cfgMather must be initialized")
	}
	pcl := &internal.PCL{
		Clid:     int64(cl.ID),
		Eversion: int64(cl.EVersion),
		Status:   internal.PCL_UNKNOWN,
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
		pcl.Status = internal.PCL_UNWATCHED
		return pcl
	case len(cl.ApplicableConfig.GetProjects()) > 1:
		logging.Warningf(ctx, "CL(%d) is watched by more than 1 project as of %s", cl.ID, t)
		pcl.Status = internal.PCL_UNWATCHED
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

	pcl.Status = internal.PCL_OK
	s.setApplicableConfigGroups(ap, cl.Snapshot, pcl)
	pcl.Deps = cl.Snapshot.GetDeps()
	// TODO(tandrii): stop storing triggering user's email
	pcl.Trigger = trigger.Find(cl.Snapshot.GetGerrit().GetInfo())
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
func (s *State) setApplicableConfigGroups(ap *changelist.ApplicableConfig_Project, snapshot *changelist.Snapshot, pcl *internal.PCL) {
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
func (s *State) tryUsingApplicableConfigGroups(ap *changelist.ApplicableConfig_Project, pcl *internal.PCL) bool {
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
