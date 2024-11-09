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

	"google.golang.org/protobuf/proto"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/runtest"
)

func TestConfigChangeStartsAndStopsRuns(t *testing.T) {
	t.Parallel()

	ftt.Run("CV starts new and stops old Runs on config change as needed", t, func(t *ftt.Test) {
		ct := Test{}
		ctx := ct.SetUp(t)

		const (
			lProject            = "infra"
			gHost               = "g-review.example.com"
			gRepoFirst          = "repo/first"
			gRepoSecond         = "repo/second"
			gChangeFirstSingle  = 10
			gChangeFirstCombo   = 15
			gChangeSecondCombo  = 25
			gChangeSecondSingle = 20
		)
		builder := &cfgpb.Verifiers_Tryjob_Builder{
			Host: buildbucketHost,
			Name: fmt.Sprintf("%s/try/test-builder", lProject),
		}
		cfgFirst := MakeCfgCombinable("main", gHost, gRepoFirst, "refs/heads/.+", builder)
		ct.BuildbucketFake.EnsureBuilders(cfgFirst)
		now := ct.Clock.Now()
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject),
			// One CL in each repo can run standalone.
			gf.CI(
				gChangeFirstSingle, gf.Project(gRepoFirst),
				gf.Owner("user-1"),
				gf.CQ(+1, now, gf.U("user-1")),
				gf.Updated(now),
			),
			gf.CI(
				gChangeSecondSingle, gf.Project(gRepoSecond),
				gf.Owner("user-2"),
				gf.CQ(+1, now, gf.U("user-2")),
				gf.Updated(now),
			),

			// First combo CL, when CV isn't watching gRepoSecond, can run standalone,
			// but not the second combo which explicitly depends on the first.
			gf.CI(
				gChangeFirstCombo, gf.Project(gRepoFirst),
				gf.Owner("user-12"),
				gf.CQ(+1, now, gf.U("user-12")),
				gf.Updated(now),
			),
			gf.CI(
				gChangeSecondCombo, gf.Project(gRepoSecond),
				gf.Owner("user-12"),
				gf.CQ(+1, now, gf.U("user-12")),
				gf.Updated(now),
				gf.Desc(fmt.Sprintf("Second Combo\n\nCq-Depend: %d", gChangeFirstCombo)),
			),
		))
		ct.AddDryRunner("user-1")
		ct.AddDryRunner("user-2")
		ct.AddDryRunner("user-12")

		ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			{Email: "user-1@example.com"},
		})
		ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			{Email: "user-2@example.com"},
		})
		ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			{Email: "user-12@example.com"},
		})

		ct.LogPhase(ctx, "CV starts 2 runs while watching first repo only")
		prjcfgtest.Create(ctx, lProject, cfgFirst)
		assert.Loosely(t, ct.PMNotifier.UpdateConfig(ctx, lProject), should.BeNil)

		var runFirstSingle, runFirstCombo *run.Run
		ct.RunUntil(ctx, func() bool {
			runFirstSingle = ct.LatestRunWithGerritCL(ctx, gHost, gChangeFirstSingle)
			runFirstCombo = ct.LatestRunWithGerritCL(ctx, gHost, gChangeFirstCombo)
			return runtest.AreRunning(runFirstSingle, runFirstCombo)
		})
		// Project must have no other runs.
		assert.Loosely(t, ct.LoadRunsOf(ctx, lProject), should.HaveLength(2))
		// And combo Run must have just 1 CL.
		assert.Loosely(t, runFirstCombo.CLs, should.HaveLength(1))

		ct.LogPhase(ctx, "CV watches both repos")
		cfgBoth := proto.Clone(cfgFirst).(*cfgpb.Config)
		g0 := cfgBoth.ConfigGroups[0].Gerrit[0]
		g0.Projects = append(g0.Projects, &cfgpb.ConfigGroup_Gerrit_Project{
			Name:      gRepoSecond,
			RefRegexp: []string{"refs/heads/.+"},
		})
		prjcfgtest.Update(ctx, lProject, cfgBoth)
		assert.Loosely(t, ct.PMNotifier.UpdateConfig(ctx, lProject), should.BeNil)

		var runSecondSingle *run.Run
		ct.RunUntil(ctx, func() bool {
			runSecondSingle = ct.LatestRunWithGerritCL(ctx, gHost, gChangeSecondSingle)
			return runtest.AreRunning(runSecondSingle)
		})
		// TODO(crbug/1221535): CV should not ignore gChangeSecondCombo while
		// runFirstCombo is running. It should either stop runFirstCombo or start a
		// new Run.
		assert.Loosely(t, ct.LatestRunWithGerritCL(ctx, gHost, gChangeSecondCombo), should.BeNil)
		runFirstSingle = ct.LoadRun(ctx, runFirstSingle.ID)
		runFirstCombo = ct.LoadRun(ctx, runFirstCombo.ID)
		assert.Loosely(t, runtest.AreRunning(runFirstSingle, runFirstCombo, runSecondSingle), should.BeTrue)

		ct.LogPhase(ctx, "CV watches only the second repo, stops Runs on CLs from the first repo, and purges second combo CL")
		cfgSecond := MakeCfgCombinable("main", gHost, gRepoSecond, "refs/heads/.+", builder)
		prjcfgtest.Update(ctx, lProject, cfgSecond)
		assert.Loosely(t, ct.PMNotifier.UpdateConfig(ctx, lProject), should.BeNil)
		ct.RunUntil(ctx, func() bool {
			runFirstSingle = ct.LoadRun(ctx, runFirstSingle.ID)
			runFirstCombo = ct.LoadRun(ctx, runFirstCombo.ID)
			return runtest.AreEnded(runFirstSingle, runFirstCombo) && ct.MaxCQVote(ctx, gHost, gChangeSecondCombo) == 0
		})
		assert.Loosely(t, ct.LastMessage(gHost, gChangeSecondCombo).GetMessage(), should.ContainSubstring(
			"CQ can't process the CL because its deps are not watched by the same LUCI project"))
	})
}
