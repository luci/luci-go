// Copyright 2026 The LUCI Authors.
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

package handler

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"sort"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/quota"
	"go.chromium.org/luci/server/quota/quotapb"

	"go.chromium.org/luci/cv/internal/acls"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	cvquota "go.chromium.org/luci/cv/internal/quota"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// tryRescueWithAlternativePayer is called when the run quota of the Run's
// current payer (BilledTo) has been exhausted.
//
// It searches for another principal who voted Commit-Queue+2 on the Run's CLs
// and who:
//   - satisfies the ACLs required to create the Run, and
//   - has run quota available.
//
// If such a principal is found, their run quota is debited, rs.Run.BilledTo is
// set to them, and true is returned.
//
// Only FULL_RUN Runs are eligible for rescue.
func (impl *Impl) tryRescueWithAlternativePayer(ctx context.Context, rs *state.RunState, cls []*changelist.CL, cg *prjcfg.ConfigGroup) (bool, error) {
	if rs.Mode != run.FullRun {
		return false, nil
	}
	for _, c := range computeRescueCandidates(ctx, rs, cls) {
		charged, op, err := impl.tryChargeAlternativePayer(ctx, rs, cg, cls, c)
		switch {
		case err != nil:
			return false, err
		case !charged:
			continue
		case op == nil:
			rs.LogInfof(ctx, logEntryLabelRunQuotaBalanceMessage,
				"Run quota exhausted for %s; billed to alternative voter %s instead, who has no run limit",
				rs.Run.BilledTo.Email(), c.id.Email())
		default:
			rs.LogInfof(ctx, logEntryLabelRunQuotaBalanceMessage,
				"Run quota exhausted for %s; debited from alternative voter %s instead; balance: %d",
				rs.Run.BilledTo.Email(), c.id.Email(), op.GetNewBalance())
		}
		rs.Run.BilledTo = c.id
		return true, nil
	}
	return false, nil
}

// rescueCandidate is a principal that may pay for a Run instead of its
// current payer.
type rescueCandidate struct {
	id identity.Identity
	// triggers are the candidate's FULL_RUN triggers, aligned with the Run's
	// CLs, suitable for acls.CheckRunCreate.
	triggers []*run.Trigger
}

// computeRescueCandidates returns the principals, other than the current
// payer, that have an active FULL_RUN vote (see trigger.FullRunVoters) on
// every triggering CL of the Run, ordered by their vote time on the first
// triggering CL (earliest first, ties broken by Gerrit account ID).
//
// Triggering CLs are the root CL if the Run has one, otherwise all CLs. For
// Runs with a root CL, the root CL trigger is used for all CLs, like in
// checkRunCreate.
func computeRescueCandidates(ctx context.Context, rs *state.RunState, cls []*changelist.CL) []rescueCandidate {
	// votes[i] maps account ID to the FULL_RUN vote on the i-th triggering CL.
	var votes []map[int64]*gerritpb.ApprovalInfo
	for _, cl := range cls {
		if rs.HasRootCL() && cl.ID != rs.RootCL {
			continue
		}
		m := make(map[int64]*gerritpb.ApprovalInfo)
		for _, ai := range trigger.FullRunVoters(cl.Snapshot.GetGerrit().GetInfo()) {
			if email := ai.GetUser().GetEmail(); email != "" && email != rs.BilledTo.Email() {
				m[ai.GetUser().GetAccountId()] = ai
			}
		}
		votes = append(votes, m)
	}
	if len(votes) == 0 {
		return nil
	}

	// Only principals who voted on every triggering CL are candidates.
	// firstCLVotesOfVotersOnAllCLs holds each such principal's vote on the
	// first triggering CL, which determines the candidate order.
	var firstCLVotesOfVotersOnAllCLs []*gerritpb.ApprovalInfo
	for accountID, ai := range votes[0] {
		votedOnAll := true
		for _, m := range votes[1:] {
			if _, ok := m[accountID]; !ok {
				votedOnAll = false
				break
			}
		}
		if votedOnAll {
			firstCLVotesOfVotersOnAllCLs = append(firstCLVotesOfVotersOnAllCLs, ai)
		} else {
			logging.Debugf(ctx, "alternative voter %s did not vote CQ+%d on all CLs of the Run", ai.GetUser().GetEmail(), trigger.CQVoteByMode(run.FullRun))
		}
	}
	sort.Slice(firstCLVotesOfVotersOnAllCLs, func(i, j int) bool {
		vi, vj := firstCLVotesOfVotersOnAllCLs[i], firstCLVotesOfVotersOnAllCLs[j]
		ti, tj := vi.GetDate().AsTime(), vj.GetDate().AsTime()
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return vi.GetUser().GetAccountId() < vj.GetUser().GetAccountId()
	})

	ret := make([]rescueCandidate, 0, len(firstCLVotesOfVotersOnAllCLs))
	for _, ai := range firstCLVotesOfVotersOnAllCLs {
		user := ai.GetUser()
		id, err := identity.MakeIdentity("user:" + user.GetEmail())
		if err != nil {
			logging.Warningf(ctx, "failed to make identity for alternative voter %s: %s", user.GetEmail(), err)
			continue
		}
		makeTrigger := func(vote *gerritpb.ApprovalInfo) *run.Trigger {
			return &run.Trigger{
				Mode:            string(run.FullRun),
				GerritAccountId: user.GetAccountId(),
				Email:           user.GetEmail(),
				Time:            vote.GetDate(),
			}
		}
		triggers := make([]*run.Trigger, len(cls))
		if rs.HasRootCL() {
			rootTrigger := makeTrigger(ai)
			for i := range cls {
				triggers[i] = rootTrigger
			}
		} else {
			// Without a root CL, votes[i] corresponds to cls[i].
			for i := range cls {
				triggers[i] = makeTrigger(votes[i][user.GetAccountId()])
			}
		}
		ret = append(ret, rescueCandidate{id: id, triggers: triggers})
	}
	return ret
}

// tryChargeAlternativePayer debits run quota from the given candidate if they
// pass the ACLs to create the Run.
//
// Returns whether the candidate can pay for the Run. If so, op is the result
// of the quota debit, or nil if no run limit applies to the candidate.
func (impl *Impl) tryChargeAlternativePayer(ctx context.Context, rs *state.RunState, cg *prjcfg.ConfigGroup, cls []*changelist.CL, c rescueCandidate) (charged bool, op *quotapb.OpResult, err error) {
	email := c.id.Email()

	// The candidate must satisfy the ACLs to create the Run on their own.
	switch aclResult, err := acls.CheckRunCreate(ctx, impl.GFactory, cg, c.triggers, cls); {
	case err != nil:
		return false, nil, errors.Fmt("acls.CheckRunCreate for alternative voter %s: %w", email, err)
	case !aclResult.OK():
		logging.Debugf(ctx, "alternative voter %s does not pass ACLs: %s", email, aclResult.FailuresSummary())
		return false, nil, nil
	}

	tempRunState := rs.ShallowCopy()
	tempRunState.Run.BilledTo = c.id
	// Scope the quota operation ID to the candidate, so the debit is distinct
	// from the original payer's for the same Run and stays idempotent across
	// retries of the Run start. The email is hashed to avoid PII in the ID.
	emailHash := md5.Sum([]byte(email))
	switch quotaOp, _, err := impl.QM.DebitRunQuota(ctx, cvquota.DebitRunOptions{
		Run:               &tempRunState.Run,
		OperationIDSuffix: hex.EncodeToString(emailHash[:]),
	}); {
	case err == nil:
		// quotaOp is nil if no run limit applies to the candidate.
		return true, quotaOp, nil
	case errors.Is(err, quota.ErrQuotaApply) && quotaOp.GetStatus() == quotapb.OpResult_ERR_UNDERFLOW:
		logging.Debugf(ctx, "alternative voter %s also has exhausted their run quota", email)
		return false, nil, nil
	case errors.Is(err, quota.ErrQuotaApply):
		return false, nil, transient.Tag.Apply(errors.Fmt("QM.DebitRunQuota for alternative voter %s: unexpected quotaOp Status %s: %w", email, quotaOp.GetStatus(), err))
	default:
		return false, nil, transient.Tag.Apply(errors.Fmt("QM.DebitRunQuota for alternative voter %s: %w", email, err))
	}
}
