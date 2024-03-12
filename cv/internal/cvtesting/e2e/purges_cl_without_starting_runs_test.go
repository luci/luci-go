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
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/runtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPurgesCLWithoutOwner(t *testing.T) {
	t.Parallel()

	Convey("PM purges CLs without owner's email", t, func() {
		/////////////////////////    Setup   ////////////////////////////////
		ct := Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		const (
			lProject = "infra"
			gHost    = "g-review"
			gRepo    = "re/po"
			gRef     = "refs/heads/main"
			gChange  = 43
		)

		prjcfgtest.Create(ctx, lProject, MakeCfgSingular("cg0", gHost, gRepo, gRef))

		ci := gf.CI(
			gChange, gf.Project(gRepo), gf.Ref(gRef),
			gf.Updated(ct.Now()), gf.CQ(+2, ct.Now(), gf.U("user-1")),
			gf.Owner("user-1"),
		)
		ci.GetOwner().Email = ""
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), ci))
		So(ct.MaxCQVote(ctx, gHost, gChange), ShouldEqual, 2)

		ct.LogPhase(ctx, "Run CV until CQ+2 vote is removed")
		So(ct.PMNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)
		ct.RunUntil(ctx, func() bool {
			return ct.MaxCQVote(ctx, gHost, gChange) == 0
		})
		So(ct.LastMessage(gHost, gChange).GetMessage(), ShouldContainSubstring, "doesn't have a preferred email")

		ct.LogPhase(ctx, "Ensure PM had a chance to react to CLUpdated event")
		ct.RunUntil(ctx, func() bool {
			return len(ct.LoadProject(ctx, lProject).State.GetPcls()) == 0
		})
	})
}

func TestPurgesCLWatchedByTwoConfigGroups(t *testing.T) {
	t.Parallel()

	Convey("PM purges CLs watched by more than 1 Config Group of the same project", t, func() {
		/////////////////////////    Setup   ////////////////////////////////
		ct := Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		const (
			lProject = "infra"
			gHost    = "g-review"
			gRepo    = "re/po"
			gRef     = "refs/heads/main"
			gChange  = 43
		)

		cfg := MakeCfgSingular("cg-ok", gHost, gRepo, gRef)
		cfg.ConfigGroups = append(cfg.ConfigGroups, MakeCfgSingular("cg-dup", gHost, gRepo, gRef).GetConfigGroups()[0])
		prjcfgtest.Create(ctx, lProject, cfg)

		ci := gf.CI(
			gChange, gf.Project(gRepo), gf.Ref(gRef),
			gf.Updated(ct.Now()), gf.CQ(+1, ct.Now(), gf.U("user-1")),
			gf.Owner("user-1"),
		)
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), ci))
		So(ct.MaxCQVote(ctx, gHost, gChange), ShouldEqual, 1)

		ct.LogPhase(ctx, "Run CV until CQ+1 vote is removed")
		So(ct.PMNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)
		ct.RunUntil(ctx, func() bool {
			return ct.MaxCQVote(ctx, gHost, gChange) == 0
		})
		msg := ct.LastMessage(gHost, gChange).GetMessage()
		So(msg, ShouldContainSubstring, "it is watched by more than 1 config group")
		So(msg, ShouldContainSubstring, "cg-ok")
		So(msg, ShouldContainSubstring, "cg-dup")

		ct.LogPhase(ctx, "Ensure PM had a chance to react to CLUpdated event")
		ct.RunUntil(ctx, func() bool {
			return len(ct.LoadProject(ctx, lProject).State.GetPcls()) == 0
		})
	})
}

func TestPurgesCLWatchedByTwoProjects(t *testing.T) {
	t.Parallel()

	Convey("PM purges CLs watched by more than 1 LUCI Projects", t, func() {
		/////////////////////////    Setup   ////////////////////////////////
		ct := Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		const (
			lProject1 = "project-1"
			lProject2 = "project-2"
			gHost     = "g-review"
			gRepo     = "re/po"
			gRef      = "refs/heads/main"
			gChange   = 43
		)

		ct.LogPhase(ctx, "Fully ingest overlapping configs")
		prjcfgtest.Create(ctx, lProject1, MakeCfgSingular("cg1", gHost, gRepo, gRef))
		prjcfgtest.Create(ctx, lProject2, MakeCfgSingular("cg2", gHost, gRepo, gRef))
		So(ct.PMNotifier.UpdateConfig(ctx, lProject1), ShouldBeNil)
		So(ct.PMNotifier.UpdateConfig(ctx, lProject2), ShouldBeNil)
		ct.RunUntil(ctx, func() bool {
			res, err := gobmap.Lookup(ctx, gHost, gRepo, gRef)
			if err != nil {
				panic(err)
			}
			return len(res.GetProjects()) == 2
		})

		ct.LogPhase(ctx, "Add Gerrit CL watched by both projects")
		ci := gf.CI(
			gChange, gf.Project(gRepo), gf.Ref(gRef),
			gf.Updated(ct.Now()), gf.CQ(+1, ct.Now(), gf.U("user-1")),
			gf.Owner("user-1"),
		)
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject1, lProject2), ci))
		So(ct.MaxCQVote(ctx, gHost, gChange), ShouldEqual, 1)

		ct.LogPhase(ctx, "Run CV until CQ+1 vote is removed")
		ct.RunUntil(ctx, func() bool {
			return ct.MaxCQVote(ctx, gHost, gChange) == 0
		})
		// There is a race between projects, but due to CL leases, only one should
		// succeed in posting a message.
		msg := ct.LastMessage(gHost, gChange).GetMessage()
		So(msg, ShouldContainSubstring, "is watched by more than 1 LUCI project")
		So(msg, ShouldContainSubstring, "project-1")
		So(msg, ShouldContainSubstring, "project-2")

		ct.LogPhase(ctx, "Ensure both PMs no longer track the CL")
		ct.RunUntil(ctx, func() bool {
			return len(ct.LoadProject(ctx, lProject1).State.GetPcls())+len(ct.LoadProject(ctx, lProject2).State.GetPcls()) == 0
		})
	})
}

func TestPurgesCLWithUnwatchedDeps_DiffGerritHost(t *testing.T) {
	t.Parallel()
	testPurgesCLWithUnwatchedDeps(t, "different Gerrit host", func(ctx context.Context, ct *Test) (string, int64) {
		// Use case: user has a typo in the `Cq-Depend:`,
		// which allows CV to detect the mistake w/o having to query Gerrit against a
		// mis-typed host.
		ct.GFake.AddFrom(gf.WithCIs("chrome-internal-typo-review.example.com", gf.ACLPublic(), gf.CI(
			44, gf.Project("chrome/src"),
			gf.Updated(ct.Now()), gf.CQ(+2, ct.Now(), gf.U("user-1")),
			gf.Owner("user-1"),
		)))
		return "chrome-internal-typo", 44
	})
}

func TestPurgesCLWithUnwatchedDeps_DiffGerritRepo(t *testing.T) {
	t.Parallel()
	testPurgesCLWithUnwatchedDeps(t, "different Gerrit repo", func(ctx context.Context, ct *Test) (string, int64) {
		// Use case: typical project misconfiguration, whereby user expects a repo to
		// be watched, but it isn't.
		ct.GFake.AddFrom(gf.WithCIs("chromium-review.example.com", gf.ACLPublic(), gf.CI(
			44, gf.Project("v8"),
			gf.Updated(ct.Now()), gf.CQ(+2, ct.Now(), gf.U("user-1")),
			gf.Owner("user-1"),
		)))
		return "chromium", 44
	})
}

func TestPurgesCLWithUnwatchedDeps_DiffGerritRef(t *testing.T) {
	t.Parallel()
	testPurgesCLWithUnwatchedDeps(t, "different Gerrit ref", func(ctx context.Context, ct *Test) (string, int64) {
		// Use case: typical project misconfiguration, whereby user expects a ref to
		// be watched, but it isn't, even though some other refs on the repo are
		// watched.
		ct.GFake.AddFrom(gf.WithCIs("chromium-review.example.com", gf.ACLPublic(), gf.CI(
			44, gf.Project("chromium/src"), gf.Ref("refs/branch-heads/not-watched"),
			gf.Updated(ct.Now()), gf.CQ(+2, ct.Now(), gf.U("user-1")),
			gf.Owner("user-1"),
		)))
		return "chromium", 44
	})
}

func testPurgesCLWithUnwatchedDeps(
	t *testing.T,
	name string,
	setupDep func(ctx context.Context, ct *Test) (depSubHost string, depChange int64),
) {
	Convey("PM purges CL with dep outside the project after waiting stabilization_delay: "+name, t, func() {
		ct := Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		const (
			lProject = "chromium"
			gHost    = "chromium-review.example.com"
			gRepo    = "chromium/src"
			gRef     = "refs/heads/main"
			gChange  = 33
		)

		const stabilizationDelay = 2 * time.Minute
		cfg := MakeCfgSingular("cg0", gHost, gRepo, gRef)
		cfg.GetConfigGroups()[0].CombineCls = &cfgpb.CombineCLs{
			StabilizationDelay: durationpb.New(stabilizationDelay),
		}
		prjcfgtest.Create(ctx, lProject, cfg)

		tStart := ct.Now()
		depSubHost, depChange := setupDep(ctx, &ct)
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
			gChange, gf.Project(gRepo), gf.Ref(gRef),
			gf.Updated(tStart), gf.CQ(+2, tStart, gf.U("user-1")),
			gf.Owner("user-1"),
			gf.Desc(fmt.Sprintf("T\n\nCq-Depend: %s:%d", depSubHost, depChange)),
		)))

		ct.LogPhase(ctx, "Run CV until CQ+2 vote is removed")
		So(ct.PMNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)
		ct.RunUntil(ctx, func() bool {
			return ct.MaxCQVote(ctx, gHost, gChange) == 0
		})
		m := ct.LastMessage(gHost, gChange)
		So(m, ShouldNotBeNil)
		So(m.GetDate().AsTime(), ShouldHappenAfter, tStart.Add(stabilizationDelay))
		So(m.GetMessage(), ShouldContainSubstring, "its deps are not watched by the same LUCI project")
		So(m.GetMessage(), ShouldContainSubstring, fmt.Sprintf("https://%s-review.example.com/c/%d", depSubHost, depChange))

		ct.LogPhase(ctx, "Ensure PM had a chance to react to CLUpdated event")
		ct.RunUntil(ctx, func() bool {
			return len(ct.LoadProject(ctx, lProject).State.GetPcls()) == 0
		})
	})
}

func TestPurgesCLWithMismatchedDepsMode(t *testing.T) {
	t.Parallel()

	Convey("PM purges CL with dep outside the project after waiting stabilization_delay", t, func() {
		/////////////////////////    Setup   ////////////////////////////////
		ct := Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		const (
			lProject      = "chromiumos"
			gHost         = "chromium-review.example.com"
			gRepo         = "cros/platform"
			gRef          = "refs/heads/main"
			gChange44     = 44
			gChange45     = 45
			customLabel   = "Custom-Label"
			customRunMode = "CUSTOM_RUN"
		)

		ct.LogPhase(ctx, "Set up stack of 2 CLs with active combine_cls setting but differing modes")

		const stabilizationDelay = 5 * time.Minute
		cfg := MakeCfgSingular("cg0", gHost, gRepo, gRef)
		cfg.GetConfigGroups()[0].CombineCls = &cfgpb.CombineCLs{
			StabilizationDelay: durationpb.New(stabilizationDelay),
		}
		cfg.GetConfigGroups()[0].AdditionalModes = []*cfgpb.Mode{{
			CqLabelValue:    1,
			Name:            customRunMode,
			TriggeringLabel: customLabel,
			TriggeringValue: 1,
		}}
		prjcfgtest.Create(ctx, lProject, cfg)

		tStart := ct.Now()
		ci44 := gf.CI(
			gChange44, gf.Project(gRepo), gf.Ref(gRef), gf.Updated(tStart),
			gf.Owner("user-1"),
			gf.CQ(+1, tStart, gf.U("user-1")), // Just DRY_RUN.
			gf.Desc(fmt.Sprintf("T\n\nCq-Depend: %d", gChange45)),
		)
		ci45 := gf.CI(
			gChange45, gf.Project(gRepo), gf.Ref(gRef), gf.Updated(tStart),
			gf.Owner("user-1"),
			// These 2 votes trigger customRunMode.
			gf.CQ(+1, tStart, gf.U("user-1")),
			gf.Vote(customLabel, +1, tStart, gf.U("user-1")),
			// Some other user triggering just the customLabel, which is a noop.
			gf.Vote(customLabel, +1, tStart.Add(-time.Minute), gf.U("user-2")),
		)
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), ci45, ci44))
		// Make ci45 depend on ci44 via Git relationship (ie make it a CL stack).
		ct.GFake.SetDependsOn(gHost, ci45, ci44)
		// Now, ci45 and ci44 must be tested only together.

		ct.LogPhase(ctx, "Run CV until both CLs are purged")
		So(ct.PMNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)
		ct.RunUntil(ctx, func() bool {
			return (ct.MaxCQVote(ctx, gHost, gChange44) == 0 &&
				ct.MaxCQVote(ctx, gHost, gChange45) == 0 &&
				ct.MaxVote(ctx, gHost, gChange45, customLabel) == 0)
		})

		ct.LogPhase(ctx, "Ensure purging happened only after stabilizationDelay")
		So(ct.LastMessage(gHost, gChange44).GetDate().AsTime(), ShouldHappenAfter, tStart.Add(stabilizationDelay))
		So(ct.LastMessage(gHost, gChange45).GetDate().AsTime(), ShouldHappenAfter, tStart.Add(stabilizationDelay))

		ct.LogPhase(ctx, "Ensure CL is no longer active in CV")
		ct.RunUntil(ctx, func() bool {
			return len(ct.LoadProject(ctx, lProject).State.GetPcls()) == 0
		})
	})
}

func TestPurgesCLCQDependingOnItself(t *testing.T) {
	t.Parallel()

	Convey("PM purges CL which CQ-Depends on itself", t, func() {
		/////////////////////////    Setup   ////////////////////////////////
		ct := Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		const (
			lProject  = "chromiumos"
			gHost     = "chromium-review.example.com"
			gRepo     = "cros/platform"
			gRef      = "refs/heads/main"
			gChange44 = 44
		)

		ct.LogPhase(ctx, "Set up a CL depending on itself")
		cfg := MakeCfgSingular("cg0", gHost, gRepo, gRef)
		prjcfgtest.Create(ctx, lProject, cfg)
		tStart := ct.Now()
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
			gChange44, gf.Project(gRepo), gf.Ref(gRef), gf.Updated(tStart),
			gf.Owner("user-1"),
			gf.CQ(+1, tStart, gf.U("user-1")),
			gf.Desc(fmt.Sprintf("T\n\nCq-Depend: %d", gChange44)),
		)))

		ct.LogPhase(ctx, "Run CV until CL is purged")
		So(ct.PMNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)
		ct.RunUntil(ctx, func() bool {
			return ct.MaxCQVote(ctx, gHost, gChange44) == 0
		})
		So(ct.LastMessage(gHost, gChange44).GetMessage(), ShouldContainSubstring, "because it depends on itself")

		ct.LogPhase(ctx, "Ensure CL is no longer active in CV")
		ct.RunUntil(ctx, func() bool {
			return len(ct.LoadProject(ctx, lProject).State.GetPcls()) == 0
		})
	})
}

func TestPurgesOnTriggerReuse(t *testing.T) {
	t.Parallel()

	Convey("PM purges CL which CQ-Depends on itself", t, func() {
		/////////////////////////    Setup   ////////////////////////////////
		ct := Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		const (
			lProject = "chromiumos"
			gHost    = "chromium-review.example.com"
			gRepo    = "cros/platform"
			gRef     = "refs/heads/main"
			gChange  = 44
		)

		ct.LogPhase(ctx, "CV starts CQ Dry Run")
		cfg := MakeCfgSingular("cg0", gHost, gRepo, gRef, &cfgpb.Verifiers_Tryjob_Builder{
			Host: buildbucketHost,
			Name: fmt.Sprintf("%s/try/test-builder", lProject),
		})
		ct.BuildbucketFake.EnsureBuilders(cfg)
		prjcfgtest.Create(ctx, lProject, cfg)
		So(ct.PMNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)

		tStart := ct.Now()
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
			gChange, gf.Project(gRepo), gf.Ref(gRef), gf.Updated(tStart),
			gf.Owner("user-1"),
			gf.CQ(+1, tStart, gf.U("user-1")),
		)))
		ct.AddDryRunner("user-1")

		var first *run.Run
		ct.RunUntil(ctx, func() bool {
			first = ct.EarliestCreatedRunOf(ctx, lProject)
			return runtest.AreRunning(first)
		})

		ct.LogPhase(ctx, "User abandons the CL and CV completes the Run")
		ct.GFake.MutateChange(gHost, gChange, func(c *gf.Change) {
			ct.Clock.Add(time.Minute)
			c.Info.Status = gerritpb.ChangeStatus_MERGED
			c.Info.Updated = timestamppb.New(ct.Clock.Now())
		})
		ct.RunUntil(ctx, func() bool { return runtest.AreEnded(ct.LoadRun(ctx, first.ID)) })

		ct.LogPhase(ctx, "User restores the CL and CV purges the CL")
		ct.GFake.MutateChange(gHost, gChange, func(c *gf.Change) {
			ct.Clock.Add(time.Minute)
			c.Info.Status = gerritpb.ChangeStatus_NEW
			c.Info.Updated = timestamppb.New(ct.Clock.Now())
		})
		ct.RunUntil(ctx, func() bool { return ct.MaxCQVote(ctx, gHost, gChange) == 0 })
		So(ct.LastMessage(gHost, gChange).GetMessage(), ShouldContainSubstring, "triggered by the same vote")
		So(ct.LastMessage(gHost, gChange).GetMessage(), ShouldContainSubstring, string(first.ID))
	})
}

func TestPurgesOnCommitFalseFooter(t *testing.T) {
	t.Parallel()

	Convey("PM purges a CL with a 'Commit: false' footer", t, func() {
		ct := Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		const (
			lProject = "infra"
			gHost    = "g-review"
			gRepo    = "re/po"
			gRef     = "refs/heads/main"
			gChange  = 43
		)

		prjcfgtest.Create(ctx, lProject, MakeCfgSingular("cg0", gHost, gRepo, gRef))

		ci := gf.CI(
			gChange, gf.Project(gRepo), gf.Ref(gRef),
			gf.Updated(ct.Now()), gf.CQ(+2, ct.Now(), gf.U("user-1")),
			gf.Owner("user-1"),
			gf.Desc("Summary\n\nCommit: false"),
		)

		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), ci))
		So(ct.MaxCQVote(ctx, gHost, gChange), ShouldEqual, 2)

		ct.LogPhase(ctx, "Run CV until CQ+2 vote is removed")
		So(ct.PMNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)
		ct.RunUntil(ctx, func() bool {
			return ct.MaxCQVote(ctx, gHost, gChange) == 0
		})
		So(ct.LastMessage(gHost, gChange).GetMessage(), ShouldContainSubstring, "\"Commit: false\" footer")

		ct.LogPhase(ctx, "Ensure PM had a chance to react to CLUpdated event")
		ct.RunUntil(ctx, func() bool {
			return len(ct.LoadProject(ctx, lProject).State.GetPcls()) == 0
		})
	})
}
