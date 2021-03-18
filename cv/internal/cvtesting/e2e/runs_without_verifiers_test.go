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
	"time"

	"go.chromium.org/luci/cv/internal/common"
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
		So(ct.LoadProject(ctx, lProject).State.GetComponents(), ShouldHaveLength, 1)
		So(ct.LoadProject(ctx, lProject).State.GetComponents()[0].GetPruns(), ShouldHaveLength, 1)
		So(ct.LoadGerritCL(ctx, gHost, gChange).IncompleteRuns.ContainsSorted(r.ID), ShouldBeTrue)

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
		So(ct.LoadGerritCL(ctx, gHost, gChange).IncompleteRuns.ContainsSorted(r.ID), ShouldBeFalse)
		So(ct.LoadProject(ctx, lProject).State.GetPcls(), ShouldBeEmpty)
		So(ct.LoadProject(ctx, lProject).State.GetComponents(), ShouldBeEmpty)
	})
}

func TestCreatesSingularRunWithDeps(t *testing.T) {
	Convey("CV creates Run in singular config in presence of Git dependencies.", t, func() {
		/////////////////////////    Setup   ////////////////////////////////
		ct := Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "infra"
		const gHost = "g-review"
		const gRepo = "re/po"
		const gRef = "refs/heads/main"

		// TODO(tandrii): remove this once Run creation is not conditional on CV
		// managing Runs for a project.
		ct.EnableCVRunManagement(ctx, lProject)
		cfg := MakeCfgSingular("cg0", gHost, gRepo, gRef)
		ct.Cfg.Create(ctx, lProject, cfg)

		tStart := ct.Clock.Now()

		ct.LogPhase(ctx, "Set Git chain of 13 depends on 12, and vote CQ+1 on 13")
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
			12, gf.Project(gRepo), gf.Ref(gRef), gf.PS(2),
		)))
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
			13, gf.Project(gRepo), gf.Ref(gRef), gf.PS(3),
			gf.Updated(tStart),
			gf.CQ(+1, tStart, gf.U("user-1")),
		)))
		ct.GFake.SetDependsOn(gHost, "13_3", "12_2")

		ct.LogPhase(ctx, "CV starts dry Run on just the 13")
		So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
		var r13 *run.Run
		ct.RunUntil(ctx, func() bool {
			r13 = ct.EarliestCreatedRunOf(ctx, lProject)
			return r13 != nil && r13.Status == run.Status_RUNNING
		})
		So(r13.Mode, ShouldResemble, run.DryRun)
		So(r13.CLs, ShouldResemble, common.CLIDs{ct.LoadGerritCL(ctx, gHost, 13).ID})

		ct.LogPhase(ctx, "User votes CQ+2 on 12, CV starts Run on just 12 and doesn't touch 13")
		ct.Clock.Add(time.Minute)
		ct.GFake.MutateChange(gHost, 12, func(c *gf.Change) {
			gf.CQ(+2, ct.Clock.Now(), "user-1")(c.Info)
			gf.Updated(ct.Clock.Now())(c.Info)
		})
		ct.RunUntil(ctx, func() bool {
			return len(ct.LoadGerritCL(ctx, gHost, 12).IncompleteRuns) > 0
		})
		r12 := ct.LoadRun(ctx, ct.LoadGerritCL(ctx, gHost, 12).IncompleteRuns[0])
		So(r12.Mode, ShouldResemble, run.FullRun)
		So(r12.CLs, ShouldResemble, common.CLIDs{ct.LoadGerritCL(ctx, gHost, 12).ID})
		So(ct.LoadRun(ctx, r13.ID).Status, ShouldEqual, run.Status_RUNNING)
		So(r12.CreateTime, ShouldHappenAfter, r13.CreateTime)

		ct.LogPhase(ctx, "User upgrades to CQ+2 on 13, which terminates old Run.\n"+
			"Due to 13 depending on not yet submitted 12, CV purges 13 by removing CQ+2 vote")
		ct.Clock.Add(time.Minute)
		ct.GFake.MutateChange(gHost, 13, func(c *gf.Change) {
			gf.CQ(+2, ct.Clock.Now(), "user-1")(c.Info)
			gf.Updated(ct.Clock.Now())(c.Info)
		})
		// TODO(crbug/1188763): fix Run Manager to cancel since trigger has changed.
		// ct.RunUntil(ctx, func() bool {
		// 	// CQ+2 vote removed.
		// 	return trigger.Find(ct.GFake.GetChange(gHost, 13).Info) == nil
		// })
	})
}
