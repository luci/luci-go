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

// Package trigger determines if and how Gerrit CL is triggered.
package trigger

import (
	"fmt"
	"time"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	"go.chromium.org/luci/cv/internal/run"
)

// CQLabel is full Gerrit label name used for triggering LUCI CV, previously
// known as CQ.
const CQLabelName = "Commit-Queue"

// AutoSubmitLabelName is the name of the Gerrit label used for triggering
// auto submission on a given CL.
const AutoSubmitLabelName = "Auto-Submit"

var voteToMode = map[int32]run.Mode{
	1: run.DryRun,
	2: run.FullRun,
}
var modeToVote = map[run.Mode]int32{
	run.DryRun:  1,
	run.FullRun: 2,
}

// FindInput contains the parameters needed to find active triggers related to a
// particular CL.
//
// TODO(crbug/1242951): Use the changelist.CL insteat of gerritpb.ChangeInfo,
//
//	and remove TriggerNewPatchsetRunAfterPS.
type FindInput struct {
	// ChangeInfo is required. It should contain the most recent information
	// about the CL to find the triggers in. E.g. label votes and patchsets.
	ChangeInfo *gerritpb.ChangeInfo

	// ConfigGroup is required. It specifies the configuration that matches the
	// change and should define any additional modes required.
	ConfigGroup *cfgpb.ConfigGroup

	// TriggerNewPatchsetRunAfterPS tells Find to ignore patchsets less or equal
	// to this number.
	TriggerNewPatchsetRunAfterPS int32
}

// Find computes the triggers corresponding to a given CL.
//
// It will return the highest priority trigger based on CQ+1 and CQ+2 votes,
// if the CQ label is non-zero, and an additional trigger for the most recently
// uploaded patchset if new patchset runs are enabled and a run has not yet been
// completed for such a patchset.
//
// CQ+2 a.k.a. Full Run takes priority of CQ+1 a.k.a Dry Run, even if CQ+1 vote
// is newer. Among several equal votes, the earliest is selected as the
// *triggering* CQ vote.
//
// If the applicable ConfigGroup specifies additional modes AND user who voted
// on the *triggering* CQ vote also voted on the additional label, then the
// additional mode is selected instead of the standard Dry/Full Run.
//
// Returns up to one trigger based on a vote on the Commit-Queue label, plus up
// to one trigger for the new patchset.
func Find(input *FindInput) *run.Triggers {
	cqTrigger := findCQTrigger(input)
	nprTrigger := findNewPatchsetRunTrigger(input)

	if cqTrigger == nil && nprTrigger == nil {
		return nil
	}
	return &run.Triggers{
		CqVoteTrigger:         cqTrigger,
		NewPatchsetRunTrigger: nprTrigger,
	}
}

func HasAutoSubmit(ci *gerritpb.ChangeInfo) bool {
	for _, ai := range ci.GetLabels()[AutoSubmitLabelName].GetAll() {
		if ai.GetValue() > 0 {
			return true
		}
	}
	return false
}

func isNewPatchsetRunTriggerable(cg *cfgpb.ConfigGroup) bool {
	for _, b := range cg.GetVerifiers().GetTryjob().GetBuilders() {
		for _, mode := range b.ModeAllowlist {
			if mode == string(run.NewPatchsetRun) {
				return true
			}
		}
	}
	return false
}

func findNewPatchsetRunTrigger(input *FindInput) *run.Trigger {
	switch currentRev := input.ChangeInfo.GetRevisions()[input.ChangeInfo.GetCurrentRevision()]; {
	case !isNewPatchsetRunTriggerable(input.ConfigGroup):
	case input.TriggerNewPatchsetRunAfterPS >= currentRev.GetNumber():
	default:
		return &run.Trigger{
			Mode:  string(run.NewPatchsetRun),
			Email: input.ChangeInfo.Owner.Email,
			Time:  currentRev.GetCreated(),
		}
	}
	return nil
}

func findCQTrigger(input *FindInput) *run.Trigger {
	if input.ChangeInfo.GetStatus() != gerritpb.ChangeStatus_NEW {
		return nil
	}
	li := input.ChangeInfo.GetLabels()[CQLabelName]
	if li == nil {
		return nil
	}
	curRevision := input.ChangeInfo.GetCurrentRevision()

	// Check if there was a previous attempt that got canceled by means of a
	// comment. Normally, CQDaemon would remove appropriate label, but in case
	// of ACLs misconfiguration preventing CQDaemon from removing votes on
	// behalf of users, CQDaemon will abort attempt by posting a special comment.
	var prevAttemptTS time.Time
	for _, msg := range input.ChangeInfo.GetMessages() {
		if bd, ok := botdata.Parse(msg); ok {
			if bd.Action == botdata.Cancel && bd.Revision == curRevision && bd.TriggeredAt.After(prevAttemptTS) {
				prevAttemptTS = bd.TriggeredAt
			}
		}
	}

	largest := int32(0)
	var earliest time.Time
	var cqTrigger *run.Trigger
	for _, ai := range li.GetAll() {
		val := ai.GetValue()
		switch {
		case val <= 0:
			continue
		case val < largest:
			continue
		case val >= 2:
			// Clamp down vote value for compatibility with CQDaemon.
			val = 2
		}

		switch t := ai.GetDate().AsTime(); {
		case !prevAttemptTS.Before(t):
			continue
		case val > largest:
			largest = val
			earliest = t
		case earliest.Before(t):
			continue
		case earliest.After(t):
			earliest = t
		default:
			// Equal timestamps shouldn't really ever happen.
			continue
		}
		cqTrigger = &run.Trigger{
			Mode:            string(voteToMode[val]),
			GerritAccountId: ai.GetUser().GetAccountId(),
			Time:            ai.GetDate(),
			Email:           ai.GetUser().GetEmail(),
		}
	}
	if cqTrigger == nil {
		return nil
	}
	for _, mode := range input.ConfigGroup.GetAdditionalModes() {
		if applyAdditionalMode(input.ChangeInfo, mode, cqTrigger) {
			// first one wins
			break
		}
	}

	// Gerrit may copy CQ vote(s) to next patchset in some project configurations.
	// In such cases, CQ vote timestamp will be *before* new patchset creation,
	// and yet *this* CQ attempt started only when new patchset was created.
	// So, attempt start is the latest of timestamps (CQ vote, this patchset
	// creation).
	revisionTS := input.ChangeInfo.Revisions[curRevision].GetCreated()
	if cqTrigger.GetTime().AsTime().Before(revisionTS.AsTime()) {
		cqTrigger.Time = revisionTS
	}
	return cqTrigger
}

func applyAdditionalMode(ci *gerritpb.ChangeInfo, mode *cfgpb.Mode, res *run.Trigger) bool {
	labelVotes := ci.GetLabels()[mode.GetTriggeringLabel()]
	switch {
	case labelVotes == nil:
		return false
	case mode.GetCqLabelValue() != modeToVote[run.Mode(res.GetMode())]:
		return false
	}

	matchesVote := func(vote *gerritpb.ApprovalInfo) bool {
		switch {
		case vote.GetValue() != mode.GetTriggeringValue():
			return false
		case vote.GetUser().GetAccountId() != res.GetGerritAccountId():
			return false
		case !vote.GetDate().AsTime().Equal(res.GetTime().AsTime()):
			return false
		default:
			return true
		}
	}

	for _, vote := range labelVotes.GetAll() {
		if matchesVote(vote) {
			res.Mode = mode.GetName()
			res.ModeDefinition = mode
			return true
		}
	}
	return false
}

// CQVoteByMode returns the numeric Commit-Queue value for a given Run mode.
func CQVoteByMode(m run.Mode) int32 {
	val, ok := modeToVote[m]
	if !ok {
		panic(fmt.Errorf("CQVoteByMode: invalid mode %q", m))
	}
	return val
}
