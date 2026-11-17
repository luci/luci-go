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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

func TestComputeRescueCandidates(t *testing.T) {
	t.Parallel()

	ftt.Run("computeRescueCandidates", t, func(t *ftt.Test) {
		ctx := context.Background()
		now := testclock.TestRecentTimeUTC
		payer := gf.U("user-1")
		botA := gf.U("user-50")
		botB := gf.U("user-51")

		vote := func(u *gerritpb.AccountInfo, value int32, minutes int) *gerritpb.ApprovalInfo {
			return &gerritpb.ApprovalInfo{User: u, Value: value, Date: timestamppb.New(now.Add(time.Duration(minutes) * time.Minute))}
		}
		makeCL := func(id common.CLID, votes ...*gerritpb.ApprovalInfo) *changelist.CL {
			ci := gf.CI(int(id))
			ci.Labels = map[string]*gerritpb.LabelInfo{"Commit-Queue": {All: votes}}
			return &changelist.CL{ID: id, Snapshot: &changelist.Snapshot{Kind: &changelist.Snapshot_Gerrit{
				Gerrit: &changelist.Gerrit{Info: ci},
			}}}
		}
		rs := &state.RunState{Run: run.Run{
			Mode:     run.FullRun,
			BilledTo: identity.Identity("user:" + payer.GetEmail()),
		}}
		emails := func(cs []rescueCandidate) []string {
			var ret []string
			for _, c := range cs {
				ret = append(ret, c.id.Email())
			}
			return ret
		}

		t.Run("single CL: excludes payer and non-FULL_RUN votes, earliest first", func(t *ftt.Test) {
			cls := []*changelist.CL{makeCL(1,
				vote(payer, 2, 0),
				vote(botB, 2, 2),
				vote(botA, 2, 1),
				vote(gf.U("user-60"), 1, 0),
			)}
			cs := computeRescueCandidates(ctx, rs, cls)
			assert.That(t, emails(cs), should.Match([]string{botA.GetEmail(), botB.GetEmail()}))
			assert.That(t, cs[0].triggers, should.Match([]*run.Trigger{{
				Mode:            string(run.FullRun),
				GerritAccountId: botA.GetAccountId(),
				Email:           botA.GetEmail(),
				Time:            timestamppb.New(now.Add(time.Minute)),
			}}))
		})

		t.Run("ties are broken by account ID", func(t *ftt.Test) {
			cls := []*changelist.CL{makeCL(1, vote(botB, 2, 1), vote(botA, 2, 1))}
			assert.That(t, emails(computeRescueCandidates(ctx, rs, cls)), should.Match([]string{botA.GetEmail(), botB.GetEmail()}))
		})

		t.Run("multi-CL without root: must vote on all CLs; triggers use each CL's own vote", func(t *ftt.Test) {
			cls := []*changelist.CL{
				makeCL(1, vote(botA, 2, 1), vote(botB, 2, 1)),
				makeCL(2, vote(botA, 2, 3)),
			}
			cs := computeRescueCandidates(ctx, rs, cls)
			assert.That(t, emails(cs), should.Match([]string{botA.GetEmail()}))
			assert.Loosely(t, cs[0].triggers, should.HaveLength(2))
			assert.That(t, cs[0].triggers[0].GetTime(), should.Match(timestamppb.New(now.Add(time.Minute))))
			assert.That(t, cs[0].triggers[1].GetTime(), should.Match(timestamppb.New(now.Add(3*time.Minute))))
		})

		t.Run("root CL: only root votes matter; root trigger used for all CLs", func(t *ftt.Test) {
			cls := []*changelist.CL{
				makeCL(1),
				makeCL(2, vote(botA, 2, 1)),
			}
			rs.RootCL = 2
			cs := computeRescueCandidates(ctx, rs, cls)
			assert.That(t, emails(cs), should.Match([]string{botA.GetEmail()}))
			assert.Loosely(t, cs[0].triggers, should.HaveLength(2))
			assert.That(t, cs[0].triggers[0], should.Match(cs[0].triggers[1]))
		})

		t.Run("no candidates", func(t *ftt.Test) {
			assert.Loosely(t, computeRescueCandidates(ctx, rs, []*changelist.CL{makeCL(1, vote(payer, 2, 0))}), should.BeEmpty)
			assert.Loosely(t, computeRescueCandidates(ctx, rs, nil), should.BeEmpty)
		})
	})
}
