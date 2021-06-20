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
	"testing"

	"google.golang.org/protobuf/proto"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/common/tree"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSubmissionDuringClosedTree(t *testing.T) {
	t.Parallel()

	Convey("CV submits Full Run if tree is closed only iff No-Tree-Checks: True", t, func() {
		ct := Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "infra"
		const gHost = "g-review"
		const gRepo = "re/po"
		const gRef = "refs/heads/main"
		const gChangeWait = 400
		const gChangeSubmit = 200
		descriptions := map[int]string{
			gChangeWait:   "Just a normal CL",
			gChangeSubmit: "This is a revert.\n\nNo-Tree-Checks: True\nNo-Try: True",
		}

		// TODO(tandrii): remove this once Run creation is not conditional on CV
		// managing Runs for a project.
		ct.EnableCVRunManagement(ctx, lProject)
		ct.TreeFake.ModifyState(ctx, tree.Closed)

		cfg := MakeCfgSingular("cg0", gHost, gRepo, gRef)
		cfg.ConfigGroups[0].Verifiers = &cfgpb.Verifiers{
			TreeStatus: &cfgpb.Verifiers_TreeStatus{
				Url: "https://tree-status.example.com/",
			},
		}
		prjcfgtest.Create(ctx, lProject, cfg)
		So(ct.PMNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)

		for _, gChange := range []int{gChangeWait, gChangeSubmit} {
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
				gChange, gf.Project(gRepo), gf.Ref(gRef),
				gf.Owner("user-1"),
				gf.CQ(+2, ct.Clock.Now(), gf.U("user-2")), gf.Updated(ct.Clock.Now()),
				gf.Desc(descriptions[gChange]),
			)))
		}

		// Start CQDaemon and make it succeed the Run immediately.
		ct.MustCQD(ctx, lProject).SetVerifyClbk(
			func(r *migrationpb.ReportedRun, cvInCharge bool) *migrationpb.ReportedRun {
				r = proto.Clone(r).(*migrationpb.ReportedRun)
				r.Attempt.Status = cvbqpb.AttemptStatus_SUCCESS
				r.Attempt.Substatus = cvbqpb.AttemptSubstatus_NO_SUBSTATUS
				return r
			},
		)
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
		So(ct.GFake.GetChange(gHost, gChangeWait).Info.GetStatus(), ShouldEqual, gerritpb.ChangeStatus_NEW)
		So(ct.GFake.GetChange(gHost, gChangeSubmit).Info.GetStatus(), ShouldEqual, gerritpb.ChangeStatus_MERGED)

		// And the tree must remain closed.
		st, err := ct.TreeFake.Client().FetchLatest(ctx, "whatever")
		So(err, ShouldBeNil)
		So(st.State, ShouldEqual, tree.Closed)
	})
}
