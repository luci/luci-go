// Copyright 2022 The LUCI Authors.
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

package acls

import (
	"context"
	"fmt"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
)

const (
	okButDueToOthers     = "CV cannot continue this run due to errors on the other CL(s) included in this run."
	ownerNotCommitter    = "CV cannot trigger the Run for %q because %q is not a committer."
	ownerNotDryRunner    = "CV cannot trigger the Run for %q because %q is not a dry-runner."
	notOwnerNotCommitter = "CV cannot trigger the Run for %q because %q is neither the CL owner nor a committer."
	noLGTM               = "This CL needs to be approved first to trigger a Run."
)

// runCreateChecker holds the evaluation results of a CL Run, and checks
// if the Run can be created.
type runCreateChecker struct {
	cl                      *changelist.CL
	runMode                 run.Mode
	allowOwnerIfSubmittable cfgpb.Verifiers_GerritCQAbility_CQAction

	// fields to hold the evaluation results of the CL.
	// -----------------------------------------------

	owner     identity.Identity
	triggerer identity.Identity

	isApproved  bool // if the CL has been approved (LGTMed) in Gerrit
	isCommitter bool // if the triggerer is a committer
	isDryRunner bool // if the triggerer is a dry runner
}

func (ck runCreateChecker) canCreateRun() (bool, string) {
	switch ck.runMode {
	case run.FullRun:
		return ck.canCreateFullRun()
	case run.DryRun, run.QuickDryRun:
		return ck.canCreateDryRun()
	default:
		panic(fmt.Errorf("unknown mode %q", ck.runMode))
	}
}

func (ck runCreateChecker) canCreateFullRun() (bool, string) {
	// A committer can run a full run, as long as the CL has been approved.
	if ck.isCommitter {
		if ck.isApproved {
			return true, ""
		}
		return false, noLGTM
	}

	// A non-committer can trigger a full-run,
	// if all of the following conditions are met.
	//
	// 1) triggerer == owner
	// 2) triggerer is a dry-runner OR cg.AllowOwnerIfSubmittable == COMMIT
	// 3) the CL has been approved in Gerrit.
	//
	// Note that a dry-runner can trigger a full-run for own CLs that
	// have been approved in Gerrit.
	//
	// For more context, crbug.com/692611 and go/cq-after-lgtm.
	if ck.triggerer != ck.owner {
		return false, fmt.Sprintf(notOwnerNotCommitter, ck.triggerer, ck.triggerer)
	}
	if !ck.isDryRunner && ck.allowOwnerIfSubmittable != cfgpb.Verifiers_GerritCQAbility_COMMIT {
		return false, fmt.Sprintf(ownerNotCommitter, ck.triggerer, ck.triggerer)
	}
	if !ck.isApproved {
		return false, noLGTM
	}
	return true, ""
}

func (ck runCreateChecker) canCreateDryRun() (bool, string) {
	// A committer can trigger a [Quick]DryRun w/o approval for own CLs.
	if ck.isCommitter {
		if ck.triggerer == ck.owner {
			return true, ""
		}
		// In order for a committer to trigger a dry-run for
		// someone else' CL, all the dependencies, of which owner
		// is not a committer, must be approved in Gerrit.
		//
		// TODO(ddoman): return false if there is an unapproved dependeny
		// of which owner is not a committer.
		return true, ""
	}

	// A non-committer can trigger a dry-run,
	// if all of the following conditions are met.
	//
	// 1) triggerer == owner
	// 2) triggerer is a dry-runner
	//    OR
	//    cg.AllowOwnerIfSubmittable in [COMMIT, DRY_RUN] AND
	//      the CL has been approved in Gerrit.
	// 3) all the deps are trusted.
	//
	// A dep is trusted, if at least one of the following conditions are met.
	// - the dep is one of the CLs included in the Run
	// - the owner of the dep is a committer
	// - the dep has been approved in Gerrit
	//
	// For more context, crbug.com/692611 and go/cq-after-lgtm.
	if ck.triggerer != ck.owner {
		return false, fmt.Sprintf(notOwnerNotCommitter, ck.triggerer, ck.triggerer)
	}
	if !ck.isDryRunner {
		switch ck.allowOwnerIfSubmittable {
		case cfgpb.Verifiers_GerritCQAbility_DRY_RUN:
		case cfgpb.Verifiers_GerritCQAbility_COMMIT:
		default:
			return false, fmt.Sprintf(ownerNotDryRunner, ck.triggerer, ck.triggerer)
		}
		if !ck.isApproved {
			return false, noLGTM
		}
		// TODO(ddoman): return false if there is an unapproved dependeny
		// of which owner is not a committer.
		return true, ""
	}

	// A dry-runner doesn't need an approval to trigger a dry run for own CLs.
	return true, ""
}

// CheckRunCreate verifies that the user(s) who triggered Run are authorized
// to create the Run for the CLs.
func CheckRunCreate(ctx context.Context, cg *prjcfg.ConfigGroup, trs []*run.Trigger, cls []*changelist.CL) (CheckResult, error) {
	res := make(CheckResult, len(cls))
	cks, err := evaluateCLs(ctx, cg, trs, cls)
	if err != nil {
		return nil, err
	}
	for _, ck := range cks {
		if ok, msg := ck.canCreateRun(); !ok {
			res[ck.cl] = msg
			continue
		}
	}
	return res, nil
}

func evaluateCLs(ctx context.Context, cg *prjcfg.ConfigGroup, trs []*run.Trigger, cls []*changelist.CL) ([]*runCreateChecker, error) {
	gVerifier := cg.Content.Verifiers.GetGerritCqAbility()
	commGroups := gVerifier.GetCommitterList()
	dryGroups := gVerifier.GetDryRunAccessList()

	var err error
	cks := make([]*runCreateChecker, len(cls))
	for i, cl := range cls {
		tr := trs[i]
		ck := &runCreateChecker{
			cl:                      cl,
			runMode:                 run.Mode(tr.Mode),
			allowOwnerIfSubmittable: gVerifier.GetAllowOwnerIfSubmittable(),
		}

		ck.triggerer, err = identity.MakeIdentity("user:" + tr.Email)
		if err != nil {
			return nil, errors.Annotate(err, "CL(%d): triggerer %q", cl.ID, tr.Email).Err()
		}
		if len(commGroups) > 0 {
			if ck.isCommitter, err = auth.GetState(ctx).DB().IsMember(ctx, ck.triggerer, commGroups); err != nil {
				return nil, errors.Annotate(err, "CL(%d)", cl.ID).Err()
			}
		}
		if len(dryGroups) > 0 {
			if ck.isDryRunner, err = auth.GetState(ctx).DB().IsMember(ctx, ck.triggerer, dryGroups); err != nil {
				return nil, errors.Annotate(err, "CL(%d)", cl.ID).Err()
			}
		}
		if ck.owner, err = cl.Snapshot.OwnerIdentity(); err != nil {
			return nil, errors.Annotate(err, "CL(%d)", cl.ID).Err()
		}
		if ck.isApproved, err = cl.Snapshot.IsSubmittable(); err != nil {
			return nil, errors.Annotate(err, "CL(%d)", cl.ID).Err()
		}
		cks[i] = ck
	}
	return cks, nil
}
