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

package triager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
)

// maxAllowedDeps limits how many non-submitted deps a CL may have for CV to
// consider it.
//
// This applies to both singular and combinable modes.
// See also https://crbug.com/1217100.
const maxAllowedDeps = 240

// triagedDeps categorizes deps of a CL, referred to below as the "dependent"
// CL.
//
// Categories are exclusive. Non-submitted OK deps are not recorded here to
// avoid unnecessary allocations in the most common case, but they do affect
// the lastTriggered time.
type triagedDeps struct {
	// lastCQVoteTriggered among *all* deps which are triggered. Can be Zero
	//time if no dep is triggered.
	lastCQVoteTriggered time.Time

	// submitted are already submitted deps watched by this project, though not
	// necessarily the same config group as the dependent CL. These deps are OK.
	submitted []*changelist.Dep

	// notYetLoaded means that more specific category isn't yet known.
	notYetLoaded []*changelist.Dep

	// needToTrigger is a list of the deps that should be triggered with CQ
	// votes.
	needToTrigger []*changelist.Dep

	invalidDeps *changelist.CLError_InvalidDeps
}

// triageDeps triages deps of a PCL. See triagedDeps for documentation.
func triageDeps(ctx context.Context, pcl *prjpb.PCL, cgIndex int32, pm pmState) *triagedDeps {
	cg := pm.ConfigGroup(cgIndex).Content
	res := &triagedDeps{}
	for _, dep := range pcl.GetDeps() {
		dPCL := pm.PCL(dep.GetClid())
		res.categorize(ctx, pcl, cgIndex, cg, dPCL, dep)
		cqTrigger := dPCL.GetTriggers().GetCqVoteTrigger()
		if cqTrigger != nil {
			if tPB := cqTrigger.GetTime(); tPB != nil {
				if t := tPB.AsTime(); res.lastCQVoteTriggered.IsZero() || res.lastCQVoteTriggered.Before(t) {
					res.lastCQVoteTriggered = t
				}
			}
		}
	}
	if okDeps := len(pcl.GetDeps()) - len(res.submitted); okDeps > maxAllowedDeps {
		// Only declare this invalid if every non-submitted DEP is OK.
		if res.invalidDeps == nil && len(res.notYetLoaded) == 0 {
			res.ensureInvalidDeps()
			res.invalidDeps.TooMany = &changelist.CLError_InvalidDeps_TooMany{
				Actual:     int32(okDeps),
				MaxAllowed: maxAllowedDeps,
			}
		}
	}
	return res
}

// OK is true if triagedDeps doesn't have any not-OK deps.
func (t *triagedDeps) OK() bool {
	return t.invalidDeps == nil
}

func (t *triagedDeps) makePurgeReason() *changelist.CLError {
	if t.OK() {
		panic("makePurgeReason must be called only iff !OK")
	}
	return &changelist.CLError{Kind: &changelist.CLError_InvalidDeps_{InvalidDeps: t.invalidDeps}}
}

// categorize adds dep to the applicable slice (if any).
//
// pcl is dependent PCL, which must be triggered.
// Its dep is represented by dPCL.
func (t *triagedDeps) categorize(ctx context.Context, pcl *prjpb.PCL, cgIndex int32, cg *cfgpb.ConfigGroup, dPCL *prjpb.PCL, dep *changelist.Dep) {
	if dPCL == nil {
		t.notYetLoaded = append(t.notYetLoaded, dep)
		return
	}

	switch s := dPCL.GetStatus(); s {
	case prjpb.PCL_UNKNOWN:
		t.notYetLoaded = append(t.notYetLoaded, dep)
		return

	case prjpb.PCL_UNWATCHED, prjpb.PCL_DELETED:
		// PCL deleted from Datastore should not happen outside of project
		// re-enablement, so it's OK to treat the same as PCL_UNWATCHED for
		// simplicity.
		t.ensureInvalidDeps()
		t.invalidDeps.Unwatched = append(t.invalidDeps.Unwatched, dep)
		return

	case prjpb.PCL_OK:
		// Happy path; continue after the switch.
	default:
		panic(fmt.Errorf("unrecognized CL %d dep %d status %s", pcl.GetClid(), dPCL.GetClid(), s))
	}
	// CL is watched by this LUCI project.

	if dPCL.GetSubmitted() {
		// Submitted CL may no longer be in the expected ConfigGroup,
		// but since it's in the same project, it's OK to refer to it as it
		// doesn't create an information leak.
		t.submitted = append(t.submitted, dep)
		return
	}

	switch cgIndexes := dPCL.GetConfigGroupIndexes(); len(cgIndexes) {
	case 0:
		panic(fmt.Errorf("at least one ConfigGroup index required for watched dep PCL %d", dPCL.GetClid()))
	case 1:
		if cgIndexes[0] != cgIndex {
			t.ensureInvalidDeps()
			t.invalidDeps.WrongConfigGroup = append(t.invalidDeps.WrongConfigGroup, dep)
			return
		}
		// Happy path; continue after the switch.
	default:
		// Strictly speaking, it may be OK iff dependentCGIndex is matched among
		// other config groups. However, there is no compelling use-case for
		// depending on a CL which matches several config groups. So, for
		// compatibility with CQDaemon, be strict.
		t.ensureInvalidDeps()
		t.invalidDeps.WrongConfigGroup = append(t.invalidDeps.WrongConfigGroup, dep)
		return
	}

	tr := pcl.GetTriggers().GetCqVoteTrigger()
	dtr := dPCL.GetTriggers().GetCqVoteTrigger()
	if cg.GetCombineCls() == nil {
		t.categorizeSingle(ctx, tr, dtr, dep, cg)
	} else {
		t.categorizeCombinable(tr, dtr, dep)
	}
}

func (t *triagedDeps) categorizeCombinable(tr, dtr *run.Trigger, dep *changelist.Dep) {
	// During the `combine_cls.stabilization_delay` since the last triggered CL
	// in a group, a user can change their mind. Since the full group of CLs
	// isn't known here, categorization decision may or may not be final.
	switch {
	case dtr.GetMode() == tr.GetMode():
		// Happy path.
		return
	case dtr == nil:
		t.ensureInvalidDeps()
		t.invalidDeps.CombinableUntriggered = append(t.invalidDeps.CombinableUntriggered, dep)
		return
	default:
		// TODO(tandrii): support dry run on dependent and full Run on dep.
		// For example, on a CL stack:
		//      CL  | Mode
		//       D    CQ+1
		//       C    CQ+1
		//       B    CQ+2
		//       A    CQ+2
		//      (base)  -
		// D+C+B+A are can be dry-run-ed and B+A can be CQ+2ed at the same time
		t.ensureInvalidDeps()
		t.invalidDeps.CombinableMismatchedMode = append(t.invalidDeps.CombinableMismatchedMode, dep)
		return
	}
}

func (t *triagedDeps) categorizeSingle(ctx context.Context, tr, dtr *run.Trigger, dep *changelist.Dep, cg *cfgpb.ConfigGroup) {
	// TODO(crbug/1470341) once the dogfood process is done,
	// enable chained cq votes by default, and remove all the unnecessary
	// param "ctx".
	var isMCEDogfooder bool
	switch triggerer, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, tr.Email)); {
	case err != nil:
		// Log the error w/o handling it. Chained CQ votes will be turned on
		// by default.
		logging.Errorf(ctx, "categorizeSingle: MakeIdentity: %s", err)
	default:
		isMCEDogfooder = common.IsMCEDogfooder(ctx, triggerer)
	}
	// dependent is guaranteed non-nil.
	switch mode := run.Mode(tr.GetMode()); {
	case mode == run.FullRun && isMCEDogfooder && dep.GetKind() == changelist.DepKind_HARD:
		// If a dep has no or different (prob CQ+1) CQ vote, then schedule
		// a trigger for CQ+2 on the dep, and postpone a run creation for
		// this CL.
		if tr.GetMode() != dtr.GetMode() {
			t.needToTrigger = append(t.needToTrigger, dep)
			return
		}
	case mode == run.FullRun:
		if cg.GetVerifiers().GetGerritCqAbility().GetAllowSubmitWithOpenDeps() && dep.GetKind() == changelist.DepKind_HARD {
			// If configured, allow CV to submit the entire stack (HARD deps
			// only) of changes.
			return
		}
		t.ensureInvalidDeps()
		t.invalidDeps.SingleFullDeps = append(t.invalidDeps.SingleFullDeps, dep)
	}
}

// ensureInvalidDeps initializes if necessary and returns .invalidDeps.
func (t *triagedDeps) ensureInvalidDeps() *changelist.CLError_InvalidDeps {
	if t.invalidDeps == nil {
		t.invalidDeps = &changelist.CLError_InvalidDeps{}
	}
	return t.invalidDeps
}

// iterateNotSubmitted calls clbk per each dep which isn't submitted.
//
// Must be called with the same PCL as was used to construct the triagedDeps.
func (t *triagedDeps) iterateNotSubmitted(pcl *prjpb.PCL, clbk func(dep *changelist.Dep)) {
	// Because construction of triagedDeps is in order of PCL's Deps, the
	// submitted must be a sub-sequence of Deps and we can compare just Dep
	// pointers.
	all, subs := pcl.GetDeps(), t.submitted
	for {
		switch {
		case len(subs) == 0:
			for _, dep := range all {
				clbk(dep)
			}
			return
		case len(all) == 0:
			panic(errors.New("must not happen because submitted must be a subset of all deps (wrong PCL?)"))
		default:
			if all[0] == subs[0] {
				subs = subs[1:]
			} else {
				clbk(all[0])
			}
			all = all[1:]
		}
	}
}
