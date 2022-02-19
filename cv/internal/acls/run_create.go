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
	"sort"
	"strings"

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

// CheckResult tells the result of an ACL check performed.
type CheckResult map[*changelist.CL]string

// OK returns true if the result indicates no failures. False, otherwise.
func (res CheckResult) OK() bool {
	return len(res) == 0
}

// Failure returns a failure message for a given RunCL.
//
// Returns an empty string, if the result was ok.
func (res CheckResult) Failure(cl *changelist.CL) string {
	if res.OK() {
		return ""
	}
	msg, ok := res[cl]
	if !ok {
		eids := make([]string, 0, len(res))
		for cl := range res {
			eids = append(eids, cl.ExternalID.MustURL())
		}
		sort.Strings(eids)

		var sb strings.Builder
		sb.WriteString(okButDueToOthers)
		for _, eid := range eids {
			sb.WriteString("\n  - ")
			sb.WriteString(eid)
		}
		return sb.String()
	}
	return msg
}

// FailuresSummary returns a summary of all the failures reported.
//
// Returns an empty string, if the result was ok.
func (res CheckResult) FailuresSummary() string {
	if res.OK() {
		return ""
	}
	msgs := make([]string, 0, len(res))
	for cl, msg := range res {
		msgs = append(msgs, fmt.Sprintf("* %s\n%s", cl.ExternalID.MustURL(), msg))
	}
	sort.Strings(msgs)

	var sb strings.Builder
	sb.WriteString(msgs[0])
	for _, msg := range msgs[1:] {
		sb.WriteString("\n\n")
		sb.WriteString(msg)
	}
	return sb.String()
}

type clInfo struct {
	owner     identity.Identity
	triggerer identity.Identity

	isApproved  bool // if the CL has been approved (LGTMed) in Gerrit
	isCommitter bool // if the triggerer is a committer
	isDryRunner bool // if the triggerer is a dry runner

	allowOwnerIfSubmittable cfgpb.Verifiers_GerritCQAbility_CQAction
}

func evaluateCL(ctx context.Context, cg *prjcfg.ConfigGroup, tr *run.Trigger, cl *changelist.CL) (clInfo, error) {
	var info clInfo
	var err error
	if info.triggerer, err = identity.MakeIdentity("user:" + tr.Email); err != nil {
		return info, errors.Annotate(err, "triggerer %q", tr.Email).Err()
	}
	if info.owner, err = cl.Snapshot.OwnerIdentity(); err != nil {
		return info, errors.Annotate(err, "CL owner identity").Err()
	}
	if info.isApproved, err = cl.Snapshot.IsSubmittable(); err != nil {
		return info, errors.Annotate(err, "checking if CL is submittable").Err()
	}

	gVerifier := cg.Content.Verifiers.GetGerritCqAbility()
	if grps := gVerifier.GetCommitterList(); len(grps) > 0 {
		if info.isCommitter, err = auth.GetState(ctx).DB().IsMember(ctx, info.triggerer, grps); err != nil {
			return info, errors.Annotate(err, "checking if triggerer %q is committer", info.triggerer).Err()
		}
	}
	if grps := gVerifier.GetDryRunAccessList(); len(grps) > 0 {
		if info.isDryRunner, err = auth.GetState(ctx).DB().IsMember(ctx, info.triggerer, grps); err != nil {
			return info, errors.Annotate(err, "checking if triggerer %q is dry-runner", info.triggerer).Err()
		}
	}
	info.allowOwnerIfSubmittable = gVerifier.GetAllowOwnerIfSubmittable()
	return info, nil
}

func canCreateFullRun(info clInfo) (bool, string) {
	// A committer can run a full run, as long as the CL has been approved.
	if info.isCommitter {
		if info.isApproved {
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
	if info.triggerer != info.owner {
		return false, fmt.Sprintf(notOwnerNotCommitter, info.triggerer, info.triggerer)
	}
	if !info.isDryRunner && info.allowOwnerIfSubmittable != cfgpb.Verifiers_GerritCQAbility_COMMIT {
		return false, fmt.Sprintf(ownerNotCommitter, info.triggerer, info.triggerer)
	}
	if !info.isApproved {
		return false, noLGTM
	}
	return true, ""
}

func canCreateDryRun(info clInfo) (bool, string) {
	// A committer can trigger a [Quick]DryRun w/o approval for own CLs.
	if info.isCommitter {
		if info.triggerer == info.owner {
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
	//    the CL has been approved in Gerrit.
	// 3) all the dependencies of which owner is not a committer
	// have beeen approved in Gerrit.
	//
	// Note that AllowOwnerIfSubmittable == COMMIT doesn't allow non-dry-runners
	// to trigger a dry-run for own CLs.
	//
	// For more context, crbug.com/692611 and go/cq-after-lgtm.
	if info.triggerer != info.owner {
		return false, fmt.Sprintf(notOwnerNotCommitter, info.triggerer, info.triggerer)
	}
	if !info.isDryRunner {
		switch info.allowOwnerIfSubmittable {
		case cfgpb.Verifiers_GerritCQAbility_DRY_RUN:
		case cfgpb.Verifiers_GerritCQAbility_COMMIT:
		default:
			return false, fmt.Sprintf(ownerNotDryRunner, info.triggerer, info.triggerer)
		}
		if !info.isApproved {
			return false, noLGTM
		}
	}
	// TODO(ddoman): return false if there is an unapproved dependeny
	// of which owner is not a committer.
	return true, ""
}

// CheckRunCreate verifies that the user(s) who triggered Run are authorized
// to create the Run for the CLs.
func CheckRunCreate(ctx context.Context, cg *prjcfg.ConfigGroup, trs []*run.Trigger, cls []*changelist.CL) (CheckResult, error) {
	res := make(CheckResult, len(cls))
	for i, cl := range cls {
		info, err := evaluateCL(ctx, cg, trs[i], cl)
		if err != nil {
			return nil, errors.Annotate(err, "CL(%d)", cl.ID).Err()
		}

		ok, msg := false, ""
		switch run.Mode(trs[i].Mode) {
		case run.FullRun:
			ok, msg = canCreateFullRun(info)
		case run.DryRun, run.QuickDryRun:
			ok, msg = canCreateDryRun(info)
		default:
			panic(fmt.Errorf("unknown mode %q", trs[i].Mode))
		}
		if !ok {
			res[cl] = msg
			continue
		}
	}
	return res, nil
}
