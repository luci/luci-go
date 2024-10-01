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

package e2e

import (
	"fmt"
	"testing"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common/tree"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"
)

func TestSubmissionDuringClosedTree(t *testing.T) {
	//	t.Parallel()
	ftt.Run("Test closed tree", t, func(t *ftt.Test) {
		ct := Test{}
		ctx := ct.SetUp(t)

		const lProject = "infra"
		const gHost = "g-review"
		const gRepo = "re/po"
		const gRef = "refs/heads/main"
		const gChange = 400
		const gChangeSubmit = 200

		t.Run("CV fails Full Run if tree status app is down", func(t *ftt.Test) {
			ct.TreeFake.InjectErr(fmt.Errorf("tree status app is down"))

			cfg := MakeCfgSingular("cg0", gHost, gRepo, gRef)
			cfg.ConfigGroups[0].Verifiers.TreeStatus = &cfgpb.Verifiers_TreeStatus{
				Url: "https://tree-status.example.com/",
			}
			prjcfgtest.Create(ctx, lProject, cfg)
			assert.Loosely(t, ct.PMNotifier.UpdateConfig(ctx, lProject), should.BeNil)

			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
				gChange, gf.Project(gRepo), gf.Ref(gRef),
				gf.Owner("user-1"),
				gf.CQ(+2, ct.Clock.Now(), gf.U("user-2")),
				gf.Approve(),
				gf.Updated(ct.Clock.Now()),
				gf.Desc("Just a normal CL"),
			)))

			// Only a committer can trigger a FullRun for someone else's CL.
			ct.AddCommitter("user-2")

			ct.LogPhase(ctx, "CV starts Runs")
			var r *run.Run
			ct.RunUntil(ctx, func() bool {
				rs := ct.LoadRunsOf(ctx, lProject)
				if len(rs) == 0 {
					return false
				}
				r = rs[0]
				return true
			})

			ct.LogPhase(ctx, "CV fails to submit the CL")
			ct.RunUntil(ctx, func() bool {
				r = ct.LoadRun(ctx, r.ID)
				return r.Status == run.Status_FAILED
			})
			info := ct.GFake.GetChange(gHost, gChange).Info
			assert.Loosely(t, info.GetStatus(), should.Equal(gerritpb.ChangeStatus_NEW))
			assert.Loosely(t, info.Messages[len(info.Messages)-1].Message, should.ContainSubstring("Could not submit this CL because the tree status app"))
			assert.Loosely(t, info.Messages[len(info.Messages)-1].Message, should.ContainSubstring("repeatedly returned failures"))
			assert.Loosely(t, info.GetLabels()["Commit-Queue"].Value, should.BeZero)
		})

		t.Run("CV submits Full Run only iff No-Tree-Checks: True", func(t *ftt.Test) {
			descriptions := map[int]string{
				gChange:       "Just a normal CL",
				gChangeSubmit: "This is a revert.\n\nNo-Tree-Checks: True\nNo-Try: True",
			}
			var expectedErr error

			check := func(t testing.TB) {
				cfg := MakeCfgSingular("cg0", gHost, gRepo, gRef)
				cfg.ConfigGroups[0].Verifiers.TreeStatus = &cfgpb.Verifiers_TreeStatus{
					Url: "https://tree-status.example.com/",
				}
				prjcfgtest.Create(ctx, lProject, cfg)
				assert.Loosely(t, ct.PMNotifier.UpdateConfig(ctx, lProject), should.BeNil)

				for _, gChange := range []int{gChange, gChangeSubmit} {
					ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
						gChange, gf.Project(gRepo), gf.Ref(gRef),
						gf.Owner("user-1"),
						gf.CQ(+2, ct.Clock.Now(), gf.U("user-2")),
						gf.Approve(),
						gf.Updated(ct.Clock.Now()),
						gf.Desc(descriptions[gChange]),
					)))
				}
				// Only a committer can trigger a FullRun for someone else' CL.
				ct.AddCommitter("user-2")

				ct.LogPhase(ctx, "CV starts Runs")
				var rWait, rSubmit *run.Run
				ct.RunUntil(ctx, func() bool {
					rs := ct.LoadRunsOf(ctx, lProject)
					if len(rs) != 2 {
						return false
					}
					if rs[0].CLs[0] == ct.LoadGerritCL(ctx, gHost, gChangeSubmit).ID {
						rSubmit, rWait = rs[0], rs[1]
					} else {
						rSubmit, rWait = rs[1], rs[0]
					}
					return true
				})

				ct.LogPhase(ctx, "CV submits CL with No-Tree-Checks and waits with the other CL")
				ct.RunUntil(ctx, func() bool {
					rSubmit = ct.LoadRun(ctx, rSubmit.ID)
					rWait = ct.LoadRun(ctx, rWait.ID)
					return rWait.Status == run.Status_WAITING_FOR_SUBMISSION && rSubmit.Status == run.Status_SUCCEEDED
				})
				assert.Loosely(t, ct.GFake.GetChange(gHost, gChange).Info.GetStatus(), should.Equal(gerritpb.ChangeStatus_NEW))
				assert.Loosely(t, ct.GFake.GetChange(gHost, gChangeSubmit).Info.GetStatus(), should.Equal(gerritpb.ChangeStatus_MERGED))

				// And the tree must not open.
				st, err := ct.TreeFake.Client().FetchLatest(ctx, "whatever")
				assert.Loosely(t, err, should.Equal(expectedErr))
				assert.Loosely(t, st.State, should.NotEqual(tree.Open))

			}

			t.Run("when tree is closed", func(t *ftt.Test) {
				ct.TreeFake.ModifyState(ctx, tree.Closed)
				check(t)
			})

			t.Run("when tree status app is failing", func(t *ftt.Test) {
				expectedErr = fmt.Errorf("tree status app is down")
				ct.TreeFake.InjectErr(expectedErr)
				check(t)
			})
		})
	})
}
