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
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
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
	untrustedDeps        = "" +
		"CV cannot trigger the Run because of the following dependencies. " +
		"They must be approved because their owners are not committers. " +
		"Alternatively, you can ask the owner of this CL to trigger a dry-run."
)

// runCreateChecker holds the evaluation results of a CL Run, and checks
// if the Run can be created.
type runCreateChecker struct {
	cl                      *changelist.CL
	runMode                 run.Mode
	allowOwnerIfSubmittable cfgpb.Verifiers_GerritCQAbility_CQAction
	commGroups              []string // committer groups
	dryGroups               []string // dry-runner groups

	owner         identity.Identity // the CL owner
	triggerer     identity.Identity // the Run triggerer
	isApproved    bool              // if the CL has been approved (LGTMed) in Gerrit
	depsToExamine common.CLIDs      // deps that are possibly untrusted.
	trustedDeps   common.CLIDsSet   // deps that have been proven to be trustable.
}

func (ck runCreateChecker) canTrustDeps(ctx context.Context) (evalResult, error) {
	if len(ck.depsToExamine) == 0 {
		return yes, nil
	}
	deps := make([]*changelist.CL, 0, len(ck.depsToExamine))
	for _, id := range ck.depsToExamine {
		if !ck.trustedDeps.Has(id) {
			deps = append(deps, &changelist.CL{ID: id})
		}
	}
	if len(deps) == 0 {
		return yes, nil
	}

	// fetch the CL entitiy of the deps and examine if they are trustable.
	// CV never removes CL entities. Hence, this handles transient and
	// datastore.ErrNoSuchEntity in the same way.
	if err := changelist.LoadCLs(ctx, deps); err != nil {
		return no, err
	}
	untrusted := deps[:0]
	for _, d := range deps {
		// Dep is trusted, if
		// - it has been approved in Gerrit, OR
		// - the owner is a committer
		switch isApproved, err := d.Snapshot.IsSubmittable(); {
		case err != nil:
			return no, errors.Annotate(err, "dep-CL(%d)", d.ID).Err()
		case isApproved:
			ck.trustedDeps.Add(d.ID)
			continue
		}

		depOwner, err := d.Snapshot.OwnerIdentity()
		if err != nil {
			return no, errors.Annotate(err, "dep-CL(%d)", d.ID).Err()
		}
		switch isCommitter, err := ck.isCommitter(ctx, depOwner); {
		case err != nil:
			return no, errors.Annotate(err,
				"dep-CL(%d): checking if owner %q is a committer", d.ID, depOwner).Err()
		case isCommitter:
			ck.trustedDeps.Add(d.ID)
			continue
		}
		untrusted = append(untrusted, d)
	}
	if len(untrusted) == 0 {
		return yes, nil
	}

	var sb strings.Builder
	sb.WriteString(untrustedDeps)
	for _, d := range untrusted {
		sb.WriteString("\n  - ")
		sb.WriteString(d.ExternalID.MustURL())
	}
	return noWithReason(sb.String()), nil
}

func (ck runCreateChecker) canCreateRun(ctx context.Context) (evalResult, error) {
	switch ck.runMode {
	case run.FullRun:
		return ck.canCreateFullRun(ctx)
	case run.DryRun, run.QuickDryRun:
		return ck.canCreateDryRun(ctx)
	default:
		panic(fmt.Errorf("unknown mode %q", ck.runMode))
	}
}

func (ck runCreateChecker) canCreateFullRun(ctx context.Context) (evalResult, error) {
	// A committer can run a full run, as long as the CL has been approved.
	isCommitter, err := ck.isCommitter(ctx, ck.triggerer)
	if err != nil {
		return no, err
	}
	if isCommitter {
		if ck.isApproved {
			return yes, nil
		}
		return noWithReason(noLGTM), nil
	}

	// A non-committer can trigger a full-run,
	// if all of the following conditions are met.
	//
	// 1) triggerer == owner
	// 2) triggerer is a dry-runner OR cg.AllowOwnerIfSubmittable == COMMIT
	// 3) the CL has been approved in Gerrit.
	//
	// That is, a dry-runner can trigger a full-run for own CLs w/ an approval.
	// For more context, crbug.com/692611 and go/cq-after-lgtm.
	if ck.triggerer != ck.owner {
		return noWithReason(fmt.Sprintf(notOwnerNotCommitter, ck.triggerer, ck.triggerer)), nil
	}
	isDryRunner, err := ck.isDryRunner(ctx, ck.triggerer)
	if err != nil {
		return no, err
	}
	if !isDryRunner && ck.allowOwnerIfSubmittable != cfgpb.Verifiers_GerritCQAbility_COMMIT {
		return noWithReason(fmt.Sprintf(ownerNotCommitter, ck.triggerer, ck.triggerer)), nil
	}
	if !ck.isApproved {
		return noWithReason(noLGTM), nil
	}
	return yes, nil
}

func (ck runCreateChecker) canCreateDryRun(ctx context.Context) (evalResult, error) {
	// A committer can trigger a [Quick]DryRun w/o approval for own CLs.
	isCommitter, err := ck.isCommitter(ctx, ck.triggerer)
	if err != nil {
		return no, err
	}
	if isCommitter {
		if ck.triggerer == ck.owner {
			return yes, nil
		}
		// In order for a committer to trigger a dry-run for
		// someone else' CL, all the dependencies, of which owner
		// is not a committer, must be approved in Gerrit.
		return ck.canTrustDeps(ctx)
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
		return noWithReason(fmt.Sprintf(notOwnerNotCommitter, ck.triggerer, ck.triggerer)), nil
	}
	isDryRunner, err := ck.isDryRunner(ctx, ck.triggerer)
	if err != nil {
		return no, err
	}
	if !isDryRunner {
		switch ck.allowOwnerIfSubmittable {
		case cfgpb.Verifiers_GerritCQAbility_DRY_RUN:
		case cfgpb.Verifiers_GerritCQAbility_COMMIT:
		default:
			return noWithReason(fmt.Sprintf(ownerNotDryRunner, ck.triggerer, ck.triggerer)), nil
		}
		if !ck.isApproved {
			return noWithReason(noLGTM), nil
		}
		return ck.canTrustDeps(ctx)
	}
	return yes, nil
}

func (ck runCreateChecker) isDryRunner(ctx context.Context, id identity.Identity) (bool, error) {
	if len(ck.dryGroups) == 0 {
		return false, nil
	}
	return auth.GetState(ctx).DB().IsMember(ctx, id, ck.dryGroups)
}

func (ck runCreateChecker) isCommitter(ctx context.Context, id identity.Identity) (bool, error) {
	if len(ck.commGroups) == 0 {
		return false, nil
	}
	return auth.GetState(ctx).DB().IsMember(ctx, id, ck.commGroups)
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
		switch result, err := ck.canCreateRun(ctx); {
		case err != nil:
			return nil, err
		case !result.ok:
			res[ck.cl] = result.reason
		}
	}
	return res, nil
}

func evaluateCLs(ctx context.Context, cg *prjcfg.ConfigGroup, trs []*run.Trigger, cls []*changelist.CL) ([]*runCreateChecker, error) {
	gVerifier := cg.Content.Verifiers.GetGerritCqAbility()

	cks := make([]*runCreateChecker, len(cls))
	trustedDeps := make(common.CLIDsSet, len(cls))
	for i, cl := range cls {
		tr := trs[i]
		triggerer, err := identity.MakeIdentity("user:" + tr.Email)
		if err != nil {
			return nil, errors.Annotate(err, "CL(%d): triggerer %q", cl.ID, tr.Email).Err()
		}
		owner, err := cl.Snapshot.OwnerIdentity()
		if err != nil {
			return nil, errors.Annotate(err, "CL(%d)", cl.ID).Err()
		}
		isApproved, err := cl.Snapshot.IsSubmittable()
		if err != nil {
			return nil, errors.Annotate(err, "CL(%d)", cl.ID).Err()
		}
		// by default, all deps are untrusted, unless they are part of the Run.
		var depsToExamine common.CLIDs
		if len(cl.Snapshot.Deps) > 0 {
			depsToExamine = make(common.CLIDs, len(cl.Snapshot.Deps))
			for i, d := range cl.Snapshot.Deps {
				depsToExamine[i] = common.CLID(d.Clid)
			}
		}
		trustedDeps.Add(cl.ID)
		cks[i] = &runCreateChecker{
			cl:                      cl,
			runMode:                 run.Mode(tr.Mode),
			allowOwnerIfSubmittable: gVerifier.GetAllowOwnerIfSubmittable(),
			commGroups:              gVerifier.GetCommitterList(),
			dryGroups:               gVerifier.GetDryRunAccessList(),

			owner:         owner,
			triggerer:     triggerer,
			isApproved:    isApproved,
			depsToExamine: depsToExamine,
			trustedDeps:   trustedDeps,
		}
	}
	return cks, nil
}
