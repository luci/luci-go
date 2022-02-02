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

	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
)

const (
	okButDueToOthers     = "CV cannot continue this run due to errors on the other CL(s) included in this run."
	notOwnerNotCommitter = "CV cannot trigger the Run for %q because %q is neither the CL owner nor a member of the committer groups."
)

// CheckResult tells the result of an ACL check performed.
type CheckResult map[*run.RunCL]string

// OK returns true if the result indicates no failures. False, otherwise.
func (res CheckResult) OK() bool {
	return len(res) == 0
}

// Failure returns a failure message for a given RunCL.
//
// Returns an empty string, if the result was ok.
func (res CheckResult) Failure(rcl *run.RunCL) string {
	if res.OK() {
		return ""
	}
	msg, ok := res[rcl]
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
	eids := make([]string, 0, len(res))
	for cl := range res {
		eids = append(eids, cl.ExternalID.MustURL())
	}
	sort.Strings(eids)

	var sb strings.Builder
	isFirst := true
	for cl, msg := range res {
		if !isFirst {
			sb.WriteString("\n\n")
		}
		isFirst = false
		sb.WriteString("* ")
		sb.WriteString(cl.ExternalID.MustURL())
		sb.WriteString("\n")
		sb.WriteString(msg)
	}
	return sb.String()
}

// CheckRunCreate verifies that the user(s) who triggered Run are authorized
// to create the Run for the CLs.
func CheckRunCreate(ctx context.Context, cg *prjcfg.ConfigGroup, cls []*run.RunCL, mode run.Mode) (CheckResult, error) {
	res := make(CheckResult, len(cls))
	for _, cl := range cls {
		triggerer, err := identity.MakeIdentity("user:" + cl.Trigger.Email)
		if err != nil {
			return nil, errors.Annotate(
				err, "the triggerer identity %q of CL %q is invalid", cl.Trigger.Email, cl.ID).Err()
		}

		switch yes, err := isCommitter(ctx, triggerer, cg.Content.Verifiers); {
		case err != nil:
			return nil, errors.Annotate(err, "failed to check committer").Err()
		case !yes:
			// Non-committer must be CL owner.
			owner, err := cl.Detail.OwnerIdentity()
			if err != nil {
				return nil, errors.Annotate(
					err, "the owner identity of CL %q is invalid", cl.ID).Err()
			}
			if triggerer != owner {
				res[cl] = fmt.Sprintf(notOwnerNotCommitter, triggerer, triggerer)
				continue
			}
		}
	}
	return res, nil
}

func isCommitter(ctx context.Context, one identity.Identity, v *cfgpb.Verifiers) (bool, error) {
	if groups := v.GetGerritCqAbility().GetCommitterList(); len(groups) > 0 {
		return auth.GetState(ctx).DB().IsMember(ctx, one, groups)
	}
	return false, nil
}
