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

package impl

import (
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/server/tq/tqtesting"
	"google.golang.org/protobuf/types/known/durationpb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	pollertask "go.chromium.org/luci/cv/internal/gerrit/poller/task"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/servicecfg"

	. "github.com/smartystreets/goconvey/convey"
)

func TestE2ECLPurgingWithoutOwner(t *testing.T) {
	t.Parallel()

	Convey("PM purges CLs without owner's email", t, func() {
		/////////////////////////    Setup   ////////////////////////////////
		ct := cvtesting.Test{AppID: "cv"}
		ctx, cancel := ct.SetUp()
		defer cancel()

		// Enable CV management of Runs for all projects.
		settings := &migrationpb.Settings{
			ApiHosts: []*migrationpb.Settings_ApiHost{
				{
					Host:          "cv.appspot.com",
					Prod:          true,
					ProjectRegexp: []string{".+"},
				},
			},
			UseCvRuns: &migrationpb.Settings_UseCVRuns{
				ProjectRegexp: []string{".+"},
			},
		}
		So(servicecfg.SetTestMigrationConfig(ctx, settings), ShouldBeNil)

		const lProject = "infra"
		const gHost = "g-review"
		const gRepo = "re/po"

		ci := gf.CI(
			43, gf.Project(gRepo), gf.Ref("refs/heads/main"),
			gf.Updated(ct.Clock.Now()), gf.CQ(+2, ct.Clock.Now(), gf.U("user-1")),
			gf.Owner("user-1"),
		)
		ci.GetOwner().Email = ""
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), ci))
		So(trigger.Find(ct.GFake.GetChange(gHost, 43).Info), ShouldNotBeNil)

		ct.Cfg.Create(ctx, lProject, singleRepoConfig(gHost, gRepo))

		/////////////////////////    Run CV   ////////////////////////////////
		// Let CV do its work, but don't wait forever. Use ever recurring pollers
		// for indication of progress.
		So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
		ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))
		for i := 0; i < 10; i++ {
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(pollertask.ClassID))
			if trigger.Find(ct.GFake.GetChange(gHost, 43).Info) == nil {
				break
			}
		}
		// Ensure PM had a chance to react to CLUpdated event.
		if len(pmtest.Projects(ct.TQ.Tasks())) > 0 {
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))
		}

		/////////////////////////    Verify   ////////////////////////////////
		So(trigger.Find(ct.GFake.GetChange(gHost, 43).Info), ShouldBeNil)
		p, err := prjmanager.Load(ctx, lProject)
		So(err, ShouldBeNil)
		So(p.State.GetPcls(), ShouldBeEmpty)
		So(p.State.GetComponents(), ShouldBeEmpty)
	})
}

func TestE2ECLPurgingWithUnwatchedDeps(t *testing.T) {
	t.Parallel()

	Convey("PM purges CL with dep outside the project after waiting stabilization_delay", t, func() {
		/////////////////////////    Setup   ////////////////////////////////
		ct := cvtesting.Test{AppID: "cv"}
		ctx, cancel := ct.SetUp()
		defer cancel()

		// Enable CV management of Runs for all projects.
		settings := &migrationpb.Settings{
			ApiHosts: []*migrationpb.Settings_ApiHost{
				{
					Host:          "cv.appspot.com",
					Prod:          true,
					ProjectRegexp: []string{".+"},
				},
			},
			UseCvRuns: &migrationpb.Settings_UseCVRuns{
				ProjectRegexp: []string{".+"},
			},
		}
		So(servicecfg.SetTestMigrationConfig(ctx, settings), ShouldBeNil)

		const (
			lProject = "chromium"
			gHost    = "chromium-review.example.com"
			gRepo    = "chromium/src"
			gChange  = 33

			lProject2 = "webrtc"
			gHost2    = "webrtc-review.example.com"
			gRepo2    = "src"
			gChange2  = 22
		)
		tStart := ct.Clock.Now()

		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
			gChange, gf.Project(gRepo), gf.Ref("refs/heads/main"),
			gf.Updated(tStart), gf.CQ(+2, tStart, gf.U("user-1")),
			gf.Owner("user-1"),
			gf.Desc(fmt.Sprintf("T\n\nCq-Depend: webrtc:%d", gChange2)),
		)))
		ct.GFake.AddFrom(gf.WithCIs(gHost2, gf.ACLRestricted(lProject2), gf.CI(
			gChange2, gf.Project(gRepo2), gf.Ref("refs/heads/main"),
			gf.Updated(tStart), gf.CQ(+2, tStart, gf.U("user-1")),
			gf.Owner("user-1"),
		)))

		const stabilizationDelay = 2 * time.Minute
		cfg1 := singleRepoConfig(gHost, gRepo)
		cfg1.GetConfigGroups()[0].CombineCls = &cfgpb.CombineCLs{
			StabilizationDelay: durationpb.New(stabilizationDelay),
		}
		ct.Cfg.Create(ctx, lProject, cfg1)
		ct.Cfg.Create(ctx, lProject2, singleRepoConfig(gHost2, gRepo2))

		/////////////////////////    Run CV   ////////////////////////////////
		// Let CV do its work, but don't wait forever. Use ever recurring pollers
		// for indication of progress.
		So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
		ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))
		for i := 0; i < 20; i++ {
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(pollertask.ClassID))
			if trigger.Find(ct.GFake.GetChange(gHost, gChange).Info) == nil {
				break
			}
		}
		// Ensure PM had a chance to react to CLUpdated event.
		if len(pmtest.Projects(ct.TQ.Tasks())) > 0 {
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))
		}

		/////////////////////////    Verify   ////////////////////////////////
		ci := ct.GFake.GetChange(gHost, gChange).Info
		So(trigger.Find(ci), ShouldBeNil)
		So(ci.GetMessages(), ShouldHaveLength, 1)
		So(
			ci.GetMessages()[0].GetDate().AsTime(),
			ShouldHappenAfter,
			tStart.Add(stabilizationDelay),
		)

		p, err := prjmanager.Load(ctx, lProject)
		So(err, ShouldBeNil)
		So(p.State.GetPcls(), ShouldBeEmpty)
		So(p.State.GetComponents(), ShouldBeEmpty)
	})
}
