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
	"time"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	"go.chromium.org/luci/cv/internal/run"
)

// CQLabel is full Gerrit label name used for triggering LUCI CV, previously
// known as CQ.
const CQLabelName = "Commit-Queue"

var voteToMode = map[int32]run.Mode{
	1: run.DryRun,
	2: run.FullRun,
}

// Find computes the latest trigger based on CQ+1 and CQ+2 votes.
//
// CQ+2 a.k.a. Full Run takes priority of CQ+1 a.k.a Dry Run,
// even if CQ+1 vote is newer. Among several equal votes, the earliest is
// selected.
//
// Returns nil if CL is not triggered.
func Find(ci *gerritpb.ChangeInfo) *run.Trigger {
	li := ci.GetLabels()[CQLabelName]
	if li == nil {
		return nil
	}
	curRevision := ci.GetCurrentRevision()

	// Check if there was a previous attempt that got canceled by means of a
	// comment. Normally, CQDaemon would remove appropriate label, but in case
	// of ACLs misconfiguration preventing CQDaemon from removing votes on
	// behalf of users, CQDaemon will abort attempt by posting a special comment.
	var prevAttemptTs time.Time
	for _, msg := range ci.GetMessages() {
		if bd, ok := botdata.Parse(msg); ok {
			if bd.Action == botdata.Cancel && bd.Revision == curRevision && bd.TriggeredAt.After(prevAttemptTs) {
				prevAttemptTs = bd.TriggeredAt
			}
		}
	}

	largest := int32(0)
	var earliest time.Time
	var ret *run.Trigger
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
		case !prevAttemptTs.Before(t):
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
		ret = &run.Trigger{
			Mode:            string(voteToMode[val]),
			GerritAccountId: ai.GetUser().GetAccountId(),
			Time:            ai.GetDate(),
			Email:           ai.GetUser().GetEmail(),
		}
	}
	if ret == nil {
		return nil
	}

	// Gerrit may copy CQ vote(s) to next patchset in some project configurations.
	// In such cases, CQ vote timestamp will be *before* new patchset creation,
	// and yet *this* CQ attempt started only when new patchset was created.
	// So, attempt start is the latest of timestamps (CQ vote, this patchset
	// creation).
	revisionTs := ci.Revisions[curRevision].GetCreated()
	if ret.GetTime().AsTime().Before(revisionTs.AsTime()) {
		ret.Time = revisionTs
	}
	return ret
}
