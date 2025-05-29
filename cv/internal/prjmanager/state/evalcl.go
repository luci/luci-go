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
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit/cfgmatcher"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
)

// reevalPCLs re-evaluates PCLs after a project config change.
//
// If any are changed, marks PB.RepartitionRequired.
func (s *State) reevalPCLs(ctx context.Context) error {
	cls, errs, err := s.loadCLsForPCLs(ctx)
	if err != nil {
		return err
	}
	newPCLs := make([]*prjpb.PCL, len(cls))
	mutated := false
	for i, cl := range cls {
		old := s.PB.GetPcls()[i]
		switch pcl, err := s.makePCLFromDS(ctx, cl, errs[i], old); {
		case err != nil:
			return err
		case pcl == nil:
			panic("makePCLFromDS is wrong")
		case pcl != old:
			mutated = true
			fallthrough
		default:
			newPCLs[i] = pcl
		}
	}
	if mutated {
		s.PB.Pcls = newPCLs
		s.PB.RepartitionRequired = true
		s.pclIndex = nil
	}
	return nil
}

// evalUpdatedCLs updates/inserts PCLs, if the PCL doesn't exist or with
// an older eversion than the given eversion.
func (s *State) evalUpdatedCLs(ctx context.Context, clEVersions map[int64]int64) error {
	cls := make([]*changelist.CL, 0, len(clEVersions))
	// Avoid doing anything in cases where all CL updates sent due to recent full
	// poll iff we already know about each CL based on its EVersion.
	for clid, ev := range clEVersions {
		switch pcl := s.PB.GetPCL(clid); {
		case pcl != nil && ev <= pcl.GetEversion():
		default:
			cls = append(cls, &changelist.CL{ID: common.CLID(clid)})
		}
	}
	if len(cls) == 0 {
		return nil
	}
	return s.evalCLsFromDS(ctx, cls)
}

func (s *State) evalCLs(ctx context.Context, clids []int64) error {
	cls := make([]*changelist.CL, len(clids))
	for i, clid := range clids {
		cls[i] = &changelist.CL{ID: common.CLID(clid)}
	}
	return s.evalCLsFromDS(ctx, cls)
}

// evalCLsFromDS adds, updates, and marks for deletion PCLs based on CLs in
// Datastore.
//
// Sorts passed cls slice and updates it with loaded from DS info.
func (s *State) evalCLsFromDS(ctx context.Context, cls []*changelist.CL) error {
	if s.cfgMatcher == nil {
		meta, err := prjcfg.GetHashMeta(ctx, s.PB.GetLuciProject(), s.PB.GetConfigHash())
		if err != nil {
			return err
		}
		if s.configGroups, err = meta.GetConfigGroups(ctx); err != nil {
			return err
		}
		s.cfgMatcher = cfgmatcher.LoadMatcherFromConfigGroups(ctx, s.configGroups, &meta)
	}

	// Sort new/updated CLs in the way as PCLs already are, namely by CL ID. Do it
	// before loading from Datastore because `errs` must correspond to `cls`.
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
	changed := common.CLIDsSet{}
	for i, cl := range cls {
		// Copy all old PCLs before this CL.
		for len(oldPCLs) > 0 && common.CLID(oldPCLs[0].GetClid()) < cl.ID {
			newPCLs = append(newPCLs, oldPCLs[0])
			oldPCLs = oldPCLs[1:]
		}
		// If CL is updated, pop old.
		var old *prjpb.PCL
		if len(oldPCLs) > 0 && common.CLID(oldPCLs[0].GetClid()) == cl.ID {
			old = oldPCLs[0]
			oldPCLs = oldPCLs[1:]
		}
		// Compute new PCL.
		switch pcl, err := s.makePCLFromDS(ctx, cl, errs[i], old); {
		case err != nil:
			return err
		case pcl == nil && old != nil:
			panic(fmt.Errorf("makePCLFromDS is wrong"))
		case pcl == nil:
			// New CL, but not in datastore. Don't add anything to newPCLs.
			// This weird case was logged by makePCLFromDS already.
		case pcl != old:
			changed.Add(cl.ID)
			fallthrough
		default:
			newPCLs = append(newPCLs, pcl)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	// Copy remaining oldPCLs.
	for len(oldPCLs) > 0 {
		newPCLs = append(newPCLs, oldPCLs[0])
		oldPCLs = oldPCLs[1:]
	}
	s.PB.Pcls = newPCLs
	s.PB.RepartitionRequired = true
	s.PB.Components = markForTriageOnChangedPCLs(s.PB.GetComponents(), s.PB.GetPcls(), changed)
	s.pclIndex = nil
	return nil
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
		return nil, transient.Tag.Apply(errors.Fmt("failed to load CL %d: %w", cl.ID, err))
	default:
		pcl := s.makePCL(ctx, cl)
		if proto.Equal(pcl, old) {
			return old, nil
		}
		return pcl, nil
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

	var ap *changelist.ApplicableConfig_Project
	switch kind, reason := cl.AccessKindWithReason(ctx, s.PB.GetLuciProject()); kind {
	case changelist.AccessUnknown:
		// Need more time to fetch this.
		logging.Debugf(ctx, "CL %d %s %s", cl.ID, cl.ExternalID, reason)
		return pcl
	case changelist.AccessDeniedProbably:
		// PM should not create a new Run in such cases, but PM won't terminate
		// existing Run when Run Manager can and should do it on its own.
		fallthrough
	case changelist.AccessDenied:
		logging.Infof(ctx, "This project has no access to CL(%d %s): %s", cl.ID, cl.ExternalID, reason)
		var watchedByMultiple bool
		ap, watchedByMultiple = cl.IsWatchedByThisAndOtherProjects(s.PB.GetLuciProject())
		if !watchedByMultiple {
			pcl.Status = prjpb.PCL_UNWATCHED
			return pcl
		}
		// Special case if the CL is watched by more than one project.
		err := newMultiProjectWatchError(cl)
		pcl.PurgeReasons = append(pcl.PurgeReasons,
			&prjpb.PurgeReason{
				ClError: err,
				ApplyTo: &prjpb.PurgeReason_AllActiveTriggers{AllActiveTriggers: true},
			})
		pcl.Errors = append(pcl.Errors, err)
		pcl.Status = prjpb.PCL_OK
	case changelist.AccessGranted:
		switch {
		case cl.Snapshot.GetOutdated() != nil:
			// Need more time to fetch this.
			logging.Debugf(ctx, "CL %d %s Snapshot is outdated", cl.ID, cl.ExternalID)
			pcl.Outdated = cl.Snapshot.GetOutdated()
			return pcl
		case cl.Snapshot.GetGerrit() == nil:
			panic(fmt.Errorf("only Gerrit CLs supported for now"))
		case len(cl.ApplicableConfig.GetProjects()) != 1:
			panic(fmt.Errorf("AccessGranted but %d projects in ApplicableConfig", len(cl.ApplicableConfig.GetProjects())))
		}
		ap = cl.ApplicableConfig.GetProjects()[0]
		pcl.Status = prjpb.PCL_OK
	default:
		panic(fmt.Errorf("unknown access kind %d", kind))
	}

	s.setApplicableConfigGroups(ap, cl.Snapshot, pcl)
	if errs := cl.Snapshot.GetErrors(); len(errs) > 0 {
		for _, err := range errs {
			pcl.PurgeReasons = append(pcl.PurgeReasons, &prjpb.PurgeReason{
				ClError: err,
				ApplyTo: &prjpb.PurgeReason_AllActiveTriggers{AllActiveTriggers: true},
			})
			pcl.Errors = append(pcl.Errors, err)
		}
	}

	pcl.Outdated = cl.Snapshot.GetOutdated()
	pcl.Deps = cl.Snapshot.GetDeps()
	for _, d := range pcl.GetDeps() {
		if d.GetClid() == pcl.GetClid() {
			if d.GetKind() != changelist.DepKind_SOFT {
				logging.Errorf(ctx, "BUG: self-referential %s dep: CL %d with Snapshot\n%s", d.GetKind(), cl.ID, cl.Snapshot)
				// If this actually happens, better to proceed with bad error message to
				// the user than crash later while processing the CL.
			}
			// TODO(robertocn): allow new patchset runs to continue even in the
			// case of this error, since new patchset runs should not be subject
			// to dependencies.
			err := &changelist.CLError{
				Kind: &changelist.CLError_SelfCqDepend{SelfCqDepend: true},
			}
			pcl.PurgeReasons = append(pcl.PurgeReasons, &prjpb.PurgeReason{
				ClError: err,
				ApplyTo: &prjpb.PurgeReason_AllActiveTriggers{AllActiveTriggers: true},
			})
			pcl.Errors = append(pcl.Errors, err)
		}
	}

	// cl.Snapshot.IsSubmittable() returns an error if it's not a Gerrit CL.
	// If so, log the error, and mark the PCL as not submittable.
	var err error
	if pcl.Submittable, err = cl.Snapshot.IsSubmittable(); err != nil {
		logging.Errorf(ctx, "prjmanager.State.makePCL: IsSubmittable failed with %s", err)
	}
	ci := cl.Snapshot.GetGerrit().GetInfo()
	if ci.GetStatus() == gerritpb.ChangeStatus_MERGED {
		pcl.Submitted = true
		return pcl
	}

	if ci.GetOwner().GetEmail() == "" {
		err := &changelist.CLError{
			Kind: &changelist.CLError_OwnerLacksEmail{OwnerLacksEmail: true},
		}
		pcl.PurgeReasons = append(pcl.PurgeReasons, &prjpb.PurgeReason{
			ClError: err,
			ApplyTo: &prjpb.PurgeReason_AllActiveTriggers{AllActiveTriggers: true},
		})
		pcl.Errors = append(pcl.Errors, err)
	}

	s.setTriggers(ci, pcl, cl.TriggerNewPatchsetRunAfterPS)

	// Check for "Commit: false" footer after setting Trigger, because this should
	// only have an effect in the case of an attempted full run.
	if hasCommitFalseFlag(cl.Snapshot.GetMetadata()) && pcl.GetTriggers().GetCqVoteTrigger().GetMode() == string(run.FullRun) {
		err := &changelist.CLError{
			Kind: &changelist.CLError_CommitBlocked{CommitBlocked: true},
		}
		pcl.PurgeReasons = append(pcl.PurgeReasons, &prjpb.PurgeReason{
			ClError: err,
			ApplyTo: &prjpb.PurgeReason_Triggers{
				Triggers: &run.Triggers{
					CqVoteTrigger: pcl.GetTriggers().GetCqVoteTrigger(),
				},
			},
		})
		pcl.Errors = append(pcl.Errors, err)
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
	if upToDate := s.tryUsingApplicableConfigGroups(ap, pcl); upToDate {
		return
	}
	// Project's config has been updated after CL snapshot was made.
	g := snapshot.GetGerrit()
	ci := g.GetInfo()
	for _, id := range s.cfgMatcher.Match(g.GetHost(), ci.GetProject(), ci.GetRef()) {
		index := s.indexOfConfigGroup(prjcfg.ConfigGroupID(id))
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
		if prjcfg.ConfigGroupID(id).Hash() != expectedConfigHash {
			return false
		}
	}
	for _, id := range ap.GetConfigGroupIds() {
		index := s.indexOfConfigGroup(prjcfg.ConfigGroupID(id))
		pcl.ConfigGroupIndexes = append(pcl.ConfigGroupIndexes, index)
	}
	return true
}

// setTriggers populates a PCL's .Triggers field with the triggers present in
// the given ChangeInfo.
//
// It also validates that the trigger mode is allowed, and strips the
// information from the triggerer.
func (s *State) setTriggers(ci *gerritpb.ChangeInfo, pcl *prjpb.PCL, latestPSRun int32) {
	// Triggers are a function of a CL and applicable ConfigGroup, which may
	// define additional modes.
	// In case of misconfiguration, there may be 0 or 2+ applicable
	// ConfigGroups, in which case we use empty ConfigGroup{}.
	// This doesn't matter much, since such CLs will be soon purged.
	// In the very worst case, CL purger will remove just the CQ vote and not
	// the additional label's vote defined in actually intended ConfigGroup,
	// which isn't a big deal.
	var cg *cfgpb.ConfigGroup
	if idxs := pcl.GetConfigGroupIndexes(); len(idxs) == 1 {
		cg = s.configGroups[idxs[0]].Content
	} else {
		cg = &cfgpb.ConfigGroup{}
	}
	ts := trigger.Find(&trigger.FindInput{ChangeInfo: ci, ConfigGroup: cg, TriggerNewPatchsetRunAfterPS: latestPSRun})
	if ts == nil {
		return
	}

	allowedRunModes := stringset.NewFromSlice(
		"",
		string(run.DryRun),
		string(run.FullRun),
		string(run.NewPatchsetRun),
	)
	for _, am := range cg.GetAdditionalModes() {
		allowedRunModes.Add(am.GetName())
	}

	for _, modeString := range []string{ts.GetCqVoteTrigger().GetMode(), ts.GetNewPatchsetRunTrigger().GetMode()} {
		if !allowedRunModes.Has(modeString) {
			err := &changelist.CLError{
				Kind: &changelist.CLError_UnsupportedMode{UnsupportedMode: string(modeString)},
			}
			pcl.PurgeReasons = append(pcl.PurgeReasons, &prjpb.PurgeReason{
				ClError: err,
				ApplyTo: &prjpb.PurgeReason_Triggers{
					Triggers: ts,
				},
			})
			pcl.Errors = append(pcl.Errors, err)
			return
		}
	}

	pcl.Triggers = ts
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
	// datastore.GetMulti splits it into concurrently queried batches.
	// To support more, CLs would need to be loaded and processed in batches,
	// or CL snapshot size reduced.
	err := datastore.Get(ctx, cls)
	switch merr, ok := err.(errors.MultiError); {
	case err == nil:
		return cls, make(errors.MultiError, len(cls)), nil
	case ok:
		return cls, merr, nil
	default:
		return nil, nil, transient.Tag.Apply(errors.Fmt("failed to load %d CLs: %w", len(cls), err))
	}
}

func newMultiProjectWatchError(cl *changelist.CL) *changelist.CLError {
	projects := make([]string, len(cl.ApplicableConfig.GetProjects()))
	for i, p := range cl.ApplicableConfig.GetProjects() {
		projects[i] = p.GetName()
	}
	return &changelist.CLError{
		Kind: &changelist.CLError_WatchedByManyProjects_{
			WatchedByManyProjects: &changelist.CLError_WatchedByManyProjects{
				Projects: projects,
			},
		},
	}
}

func hasCommitFalseFlag(metadata []*changelist.StringPair) bool {
	for _, p := range metadata {
		// The values stored in the CL Snapshot Metadata are not necessarily normalized,
		// and could have come from "Commit: false", "COMMIT=FALSE" or some other style.
		// Other possible values like "Commit: no" are not recognized.
		if strings.ToLower(p.Key) == "commit" && strings.ToLower(p.Value) == "false" {
			return true
		}
	}
	return false
}
