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
	"sync/atomic"
	"testing"

	"google.golang.org/protobuf/proto"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	commonpb "go.chromium.org/luci/cv/api/common/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHandleLargeCLStack(t *testing.T) {
	t.Parallel()

	Convey("CV full runs and submits a large CL stack.", t, func() {
		ct := Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "infra"
		const gHost = "g-review"
		const gRepo = "re/po"
		const gRef = "refs/heads/main"
		const gChangeFirst = 1001
		// TODO(tandrii): bump max entities limit in datastore in-memory emulation to
		// from 25 to ~500 and then bump this limit to 200.
		// NOTE: current datastore in-memory emulates classic Datastore, not the
		// Firestore used by CV, and as such its limit of 25 counts entity *groups*,
		// thus Run and all its CLs count as 1 such group.
		const N = 19

		cfg := MakeCfgCombinable("cg0", gHost, gRepo, gRef)
		prjcfgtest.Create(ctx, lProject, cfg)
		So(ct.PMNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)

		cis := make([]*gerritpb.ChangeInfo, N)
		for i := range cis {
			cis[i] = gf.CI(
				gChangeFirst+i,
				gf.Project(gRepo), gf.Ref(gRef), gf.Owner("user-1"), gf.PS(1),
				gf.CQ(+2, ct.Clock.Now(), gf.U("user-1")), gf.Updated(ct.Clock.Now()))
		}
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), cis...))
		for i, child := range cis {
			for _, parent := range cis[:i] {
				ct.GFake.SetDependsOn(gHost, child, parent)
			}
		}

		ct.LogPhase(ctx, "CV creates a Run")
		ct.RunUntil(ctx, func() bool {
			return len(ct.LoadRunsOf(ctx, lProject)) > 0
		})
		r := ct.EarliestCreatedRunOf(ctx, lProject)
		So(r.CLs, ShouldHaveLength, N)

		ct.LogPhase(ctx, "CQD marks Run verified")
		verified := atomic.Value{}
		verified.Store(false)
		ct.MustCQD(ctx, lProject).SetVerifyClbk(
			func(r *migrationpb.ReportedRun) *migrationpb.ReportedRun {
				r = proto.Clone(r).(*migrationpb.ReportedRun)
				r.Attempt.Status = cvbqpb.AttemptStatus_SUCCESS
				r.Attempt.Substatus = cvbqpb.AttemptSubstatus_NO_SUBSTATUS
				verified.Store(true)
				return r
			},
		)
		ct.RunUntil(ctx, func() bool { return verified.Load().(bool) })

		ct.LogPhase(ctx, "CV submits all CLs and finishes the Run")
		ct.RunUntil(ctx, func() bool {
			r = ct.LoadRun(ctx, r.ID)
			return run.IsEnded(r.Status)
		})

		So(r.Status, ShouldEqual, commonpb.Run_SUCCEEDED)
		So(r.Submission.GetCls(), ShouldHaveLength, N)
		So(r.Submission.GetSubmittedCls(), ShouldHaveLength, N)
		var actual, expected []int
		for i := range cis {
			gChange := gChangeFirst + i
			expected = append(expected, gChange)
			if ct.GFake.GetChange(gHost, gChangeFirst+i).Info.GetStatus() == gerritpb.ChangeStatus_MERGED {
				actual = append(actual, gChange)
			}
		}
		So(actual, ShouldResemble, expected)
	})
}
