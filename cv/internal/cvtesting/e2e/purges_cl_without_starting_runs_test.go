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
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPurgesCLWithoutOwner(t *testing.T) {
	t.Parallel()

	Convey("PM purges CLs without owner's email", t, func() {
		/////////////////////////    Setup   ////////////////////////////////
		ct := Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const (
			lProject = "infra"
			gHost    = "g-review"
			gRepo    = "re/po"
			gRef     = "refs/heads/main"
			gChange  = 43
		)

		ct.EnableCVRunManagement(ctx, lProject)
		ct.Cfg.Create(ctx, lProject, MakeCfgSingular("cg0", gHost, gRepo, gRef))

		ci := gf.CI(
			gChange, gf.Project(gRepo), gf.Ref(gRef),
			gf.Updated(ct.Now()), gf.CQ(+2, ct.Now(), gf.U("user-1")),
			gf.Owner("user-1"),
		)
		ci.GetOwner().Email = ""
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), ci))
		So(trigger.Find(ct.GFake.GetChange(gHost, gChange).Info), ShouldNotBeNil)

		ct.LogPhase(ctx, "Run CV until CQ+2 vote is removed")
		So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
		ct.RunUntil(ctx, func() bool {
			return trigger.Find(ct.GFake.GetChange(gHost, gChange).Info) == nil
		})

		ct.LogPhase(ctx, "Ensure PM had a chance to react to CLUpdated event")
		ct.RunUntil(ctx, func() bool {
			return len(ct.LoadProject(ctx, lProject).State.GetPcls()) == 0
		})

		ct.LogPhase(ctx, "Verify")
		So(trigger.Find(ct.GFake.GetChange(gHost, gChange).Info), ShouldBeNil)
		p := ct.LoadProject(ctx, lProject)
		So(p.State.GetPcls(), ShouldBeEmpty)
		So(p.State.GetComponents(), ShouldBeEmpty)
	})
}

func TestPurgesCLWithUnwatchedDeps(t *testing.T) {
	t.Parallel()

	Convey("PM purges CL with dep outside the project after waiting stabilization_delay", t, func() {
		/////////////////////////    Setup   ////////////////////////////////
		ct := Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const (
			lProject = "chromium"
			gHost    = "chromium-review.example.com"
			gRepo    = "chromium/src"
			gRef     = "refs/heads/main"
			gChange  = 33

			lProject2 = "webrtc"
			gHost2    = "webrtc-review.example.com"
			gRepo2    = "src"
			gChange2  = 22
		)
		// Enable CV management of both projects.
		ct.EnableCVRunManagement(ctx, lProject)
		ct.EnableCVRunManagement(ctx, lProject2)

		const stabilizationDelay = 2 * time.Minute
		cfg1 := MakeCfgSingular("cg0", gHost, gRepo, gRef)
		cfg1.GetConfigGroups()[0].CombineCls = &cfgpb.CombineCLs{
			StabilizationDelay: durationpb.New(stabilizationDelay),
		}
		ct.Cfg.Create(ctx, lProject, cfg1)
		ct.Cfg.Create(ctx, lProject2, MakeCfgSingular("cg0", gHost2, gRepo2, gRef))

		tStart := ct.Now()

		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), gf.CI(
			gChange, gf.Project(gRepo), gf.Ref(gRef),
			gf.Updated(tStart), gf.CQ(+2, tStart, gf.U("user-1")),
			gf.Owner("user-1"),
			gf.Desc(fmt.Sprintf("T\n\nCq-Depend: webrtc:%d", gChange2)),
		)))
		ct.GFake.AddFrom(gf.WithCIs(gHost2, gf.ACLRestricted(lProject2), gf.CI(
			gChange2, gf.Project(gRepo2), gf.Ref(gRef),
			gf.Updated(tStart), gf.CQ(+2, tStart, gf.U("user-1")),
			gf.Owner("user-1"),
		)))

		ct.LogPhase(ctx, "Run CV until CQ+2 vote is removed")
		So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
		ct.RunUntil(ctx, func() bool {
			return trigger.Find(ct.GFake.GetChange(gHost, gChange).Info) == nil
		})

		ct.LogPhase(ctx, "Ensure PM had a chance to react to CLUpdated event")
		ct.RunUntil(ctx, func() bool {
			return len(ct.LoadProject(ctx, lProject).State.GetPcls()) == 0
		})

		ct.LogPhase(ctx, "Verify")
		ci := ct.GFake.GetChange(gHost, gChange).Info
		So(trigger.Find(ci), ShouldBeNil)
		So(ci.GetMessages(), ShouldHaveLength, 1)
		So(gf.LastMessage(ci).GetDate().AsTime(), ShouldHappenAfter, tStart.Add(stabilizationDelay))

		p := ct.LoadProject(ctx, lProject)
		So(p.State.GetPcls(), ShouldBeEmpty)
		So(p.State.GetComponents(), ShouldBeEmpty)
	})
}
