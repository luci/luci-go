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

	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCreatesSingularRun(t *testing.T) {
	t.Parallel()

	Convey("CV creates 1 CL Run, which gets canceled by the user", t, func() {
		/////////////////////////    Setup   ////////////////////////////////
		ct := Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "infra"
		const gHost = "g-review"
		const gRepo = "re/po"
		const gRef = "refs/heads/main"
		const gChange = 33

		// TODO(tandrii): remove this once Run creation is not conditional on CV
		// managing Runs for a project.
		ct.EnableCVRunManagement(ctx, lProject)
		cfg := MakeCfgSingular("cg0", gHost, gRepo, gRef)
		ct.Cfg.Create(ctx, lProject, cfg)

		tStart := ct.Clock.Now()

		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
			gChange, gf.Project(gRepo), gf.Ref(gRef),
			gf.Owner("user-1"),
			gf.Updated(tStart), gf.CQ(+2, tStart, gf.U("user-2")),
		)))

		/////////////////////////    Run CV   ////////////////////////////////
		ct.LogPhase(ctx, "CV notices CL and starts the Run")
		So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
		var r *run.Run
		ct.RunUntil(ctx, func() bool {
			r = ct.EarliestCreatedRunOf(ctx, lProject)
			return r != nil && r.Status == run.Status_RUNNING
		})

		ct.LogPhase(ctx, "User cancels the Run")
		ct.GFake.MutateChange(gHost, gChange, func(c *gf.Change) {
			gf.CQ(0, ct.Clock.Now(), "user-2")(c.Info)
			gf.Updated(ct.Clock.Now())(c.Info)
		})

		ct.LogPhase(ctx, "CV cancels the Run")
		ct.RunUntil(ctx, func() bool {
			r2 := ct.LoadRun(ctx, r.ID)
			p := ct.LoadProject(ctx, lProject)
			return r2.Status == run.Status_CANCELLED && len(p.State.GetComponents()) == 0
		})

		/////////////////////////    Verify    ////////////////////////////////
		p := ct.LoadProject(ctx, lProject)
		So(p.State.GetPcls(), ShouldBeEmpty)
	})
}
