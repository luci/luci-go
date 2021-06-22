// Copyright 2020 The LUCI Authors.
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

package manager

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap/gobmaptest"
	"go.chromium.org/luci/cv/internal/gerrit/poller/pollertest"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/runtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProjectTQLateTasks(t *testing.T) {
	t.Parallel()

	Convey("PM task does nothing if it comes too late", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		ctx, _ = runtest.MockDispatch(ctx)

		pmNotifier := prjmanager.NewNotifier(ct.TQDispatcher)
		runNotifier := run.NewNotifier(ct.TQDispatcher)
		_ = New(pmNotifier, runNotifier, updater.New(ct.TQDispatcher, pmNotifier, runNotifier))

		const lProject = "infra"
		lProjectKey := datastore.MakeKey(ctx, prjmanager.ProjectKind, lProject)

		prjcfgtest.Create(ctx, lProject, singleRepoConfig("host", "repo"))

		So(pmNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)
		So(pmtest.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})
		events1, err := eventbox.List(ctx, lProjectKey)
		So(err, ShouldBeNil)

		// Simulate stuck TQ task, which gets executed with a huge delay.
		ct.Clock.Add(time.Hour)
		ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))
		// It must not modify PM state nor consume events.
		So(datastore.Get(ctx, &prjmanager.Project{ID: lProject}), ShouldEqual, datastore.ErrNoSuchEntity)
		events2, err := eventbox.List(ctx, lProjectKey)
		So(err, ShouldBeNil)
		So(events2, ShouldResemble, events1)
		// But schedules new task instead.
		So(pmtest.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})

		// Next task coming ~on time proceeds normally.
		ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))
		So(datastore.Get(ctx, &prjmanager.Project{ID: lProject}), ShouldBeNil)
		events3, err := eventbox.List(ctx, lProjectKey)
		So(err, ShouldBeNil)
		So(events3, ShouldBeEmpty)
	})
}

func TestProjectLifeCycle(t *testing.T) {
	t.Parallel()

	Convey("Project can be created, updated, deleted", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		ctx, rmDispatcher := runtest.MockDispatch(ctx)

		pmNotifier := prjmanager.NewNotifier(ct.TQDispatcher)
		runNotifier := run.NewNotifier(ct.TQDispatcher)
		_ = New(pmNotifier, runNotifier, updater.New(ct.TQDispatcher, pmNotifier, runNotifier))

		const lProject = "infra"
		lProjectKey := datastore.MakeKey(ctx, prjmanager.ProjectKind, lProject)

		Convey("with new project", func() {
			prjcfgtest.Create(ctx, lProject, singleRepoConfig("host", "repo"))
			So(pmNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)
			// Second event is a noop, but should still be consumed at once.
			So(pmNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)
			So(pmtest.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))
			events, err := eventbox.List(ctx, lProjectKey)
			So(err, ShouldBeNil)
			So(events, ShouldHaveLength, 0)
			p, ps := loadProjectEntities(ctx, lProject)
			So(p.EVersion, ShouldEqual, 1)
			So(ps.Status, ShouldEqual, prjpb.Status_STARTED)
			So(pollertest.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})

			// Ensure first poller task gets executed.
			ct.Clock.Add(time.Hour)

			Convey("update config with incomplete runs", func() {
				err := datastore.Put(
					ctx,
					&run.Run{ID: common.RunID(lProject + "/111-beef"), CLs: common.CLIDs{111}},
					&run.Run{ID: common.RunID(lProject + "/222-cafe"), CLs: common.CLIDs{222}},
				)
				So(err, ShouldBeNil)
				// This is what pmNotifier.notifyRunCreated func does,
				// but because it's private, it can't be called from this package.
				simulateRunCreated := func(suffix string) {
					e := &prjpb.Event{Event: &prjpb.Event_RunCreated{
						RunCreated: &prjpb.RunCreated{
							RunId: lProject + "/" + suffix,
						},
					}}
					value, err := proto.Marshal(e)
					So(err, ShouldBeNil)
					So(eventbox.Emit(ctx, value, lProjectKey), ShouldBeNil)
				}
				simulateRunCreated("111-beef")
				simulateRunCreated("222-cafe")

				prjcfgtest.Update(ctx, lProject, singleRepoConfig("host", "repo2"))
				So(pmNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)

				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))

				p, _ = loadProjectEntities(ctx, lProject)
				So(p.IncompleteRuns(), ShouldEqual, common.MakeRunIDs(lProject+"/111-beef", lProject+"/222-cafe"))
				// Must schedule a task per Run for config updates for each of the
				// started run.
				So(rmDispatcher.PopRuns(), ShouldResemble, p.IncompleteRuns())

				Convey("disable project with incomplete runs", func() {
					prjcfgtest.Disable(ctx, lProject)
					So(pmNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))

					p, ps := loadProjectEntities(ctx, lProject)
					So(p.EVersion, ShouldEqual, 3)
					So(ps.Status, ShouldEqual, prjpb.Status_STOPPING)
					So(pollertest.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})

					// Must schedule a task per Run for cancellation.
					So(rmDispatcher.PopRuns(), ShouldResemble, p.IncompleteRuns())

					Convey("wait for all IncompleteRuns to finish", func() {
						So(pmNotifier.NotifyRunFinished(ctx, common.RunID(lProject+"/111-beef")), ShouldBeNil)
						ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))
						p, ps := loadProjectEntities(ctx, lProject)
						So(ps.Status, ShouldEqual, prjpb.Status_STOPPING)
						So(p.IncompleteRuns(), ShouldResemble, common.MakeRunIDs(lProject+"/222-cafe"))

						So(pmNotifier.NotifyRunFinished(ctx, common.RunID(lProject+"/222-cafe")), ShouldBeNil)
						ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))
						p, ps = loadProjectEntities(ctx, lProject)
						So(ps.Status, ShouldEqual, prjpb.Status_STOPPED)
						So(p.IncompleteRuns(), ShouldBeEmpty)
					})
				})
			})

			Convey("delete project without incomplete runs", func() {
				// No components means also no runs.
				p.State.Components = nil
				prjcfgtest.Delete(ctx, lProject)
				So(pmNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))

				p, ps := loadProjectEntities(ctx, lProject)
				So(p.EVersion, ShouldEqual, 2)
				So(ps.Status, ShouldEqual, prjpb.Status_STOPPED)
				So(pollertest.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})
			})
		})
	})
}

func TestProjectHandlesManyEvents(t *testing.T) {
	t.Parallel()

	Convey("PM handles many events", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "infra"
		const gHost = "host"
		const gRepo = "repo"

		pmNotifier := prjmanager.NewNotifier(ct.TQDispatcher)
		runNotifier := run.NewNotifier(ct.TQDispatcher)
		clUpdater := updater.New(ct.TQDispatcher, pmNotifier, runNotifier)
		pm := New(pmNotifier, runNotifier, clUpdater)

		refreshCLAndNotifyPM := func(c int64) {
			So(clUpdater.Refresh(ctx, &updater.RefreshGerritCL{
				LuciProject: lProject,
				Host:        "host",
				Change:      c,
			}), ShouldBeNil) // this notifies PM once.
		}

		cfg := singleRepoConfig(gHost, gRepo)
		cfg.ConfigGroups[0].CombineCls = &cfgpb.CombineCLs{
			// Postpone creation of Runs, which isn't important in this test.
			StabilizationDelay: durationpb.New(time.Hour),
		}
		prjcfgtest.Create(ctx, lProject, cfg)
		gobmaptest.Update(ctx, lProject)

		// Put #43 CL directly w/o notifying the PM.
		cl43, err := changelist.MustGobID(gHost, 43).GetOrInsert(ctx, func(cl *changelist.CL) {
			cl.Snapshot = &changelist.Snapshot{
				ExternalUpdateTime:    timestamppb.New(ct.Clock.Now()),
				LuciProject:           lProject,
				MinEquivalentPatchset: 1,
				Patchset:              1,
				Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
					Host: gHost,
					Info: gf.CI(43,
						gf.Project(gRepo), gf.Ref("refs/heads/main"),
						gf.CQ(+2, ct.Clock.Now(), gf.U("user-1"))),
				}},
			}
			meta := prjcfgtest.MustExist(ctx, lProject)
			cl.ApplicableConfig = &changelist.ApplicableConfig{
				Projects: []*changelist.ApplicableConfig_Project{
					{Name: lProject, ConfigGroupIds: []string{string(meta.ConfigGroupIDs[0])}},
				},
			}
		})
		So(err, ShouldBeNil)

		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), gf.CI(
			44, gf.Project(gRepo), gf.Ref("refs/heads/main"),
			gf.CQ(+2, ct.Clock.Now(), gf.U("user-1")))))
		refreshCLAndNotifyPM(44)
		cl44, err := changelist.MustGobID(gHost, 44).Get(ctx)
		So(err, ShouldBeNil)

		// This event is the only event notifying PM about CL#43.
		So(pmNotifier.NotifyCLsUpdated(ctx, lProject, []*changelist.CL{cl43, cl44}), ShouldBeNil)

		const n = 10
		for i := 0; i < n; i++ {
			So(pmNotifier.UpdateConfig(ctx, lProject), ShouldBeNil)
			So(pmNotifier.Poke(ctx, lProject), ShouldBeNil)

			ct.Clock.Add(time.Second)
			ct.GFake.MutateChange(gHost, 44, func(c *gf.Change) { gf.Updated(ct.Clock.Now())(c.Info) })
			refreshCLAndNotifyPM(44)
		}
		// Get the latest cl44 from Datastore.
		cl44, err = changelist.MustGobID(gHost, 44).Get(ctx)
		So(err, ShouldBeNil)

		lProjectKey := datastore.MakeKey(ctx, prjmanager.ProjectKind, lProject)
		events, err := eventbox.List(ctx, lProjectKey)
		So(err, ShouldBeNil)
		// 1 refreshCLAndNotifyPM(44), 1 from NotifyCLsUpdated, 3*n from loop.
		So(events, ShouldHaveLength, 3*n+2)

		// Run `w` concurrent PMs.
		const w = 20
		now := ct.Clock.Now()
		errs := make(errors.MultiError, w)
		wg := sync.WaitGroup{}
		wg.Add(w)
		for i := 0; i < w; i++ {
			i := i
			go func() {
				defer wg.Done()
				errs[i] = pm.manageProject(ctx, lProject, now)
			}()
		}
		wg.Wait()

		// Exactly 1 of the workers must create PM entity, consume events and
		// poke the poller.
		p := prjmanager.Project{ID: lProject}
		So(datastore.Get(ctx, &p), ShouldBeNil)
		// Both cl43 and cl44 must have corresponding PCLs with latest EVersions.
		So(p.State.GetPcls(), ShouldHaveLength, 2)
		for _, pcl := range p.State.GetPcls() {
			switch common.CLID(pcl.GetClid()) {
			case cl43.ID:
				So(pcl.GetEversion(), ShouldEqual, int64(cl43.EVersion))
			case cl44.ID:
				So(pcl.GetEversion(), ShouldEqual, int64(cl44.EVersion))
			default:
				So("must not happen", ShouldBeTrue)
			}
		}

		events, err = eventbox.List(ctx, lProjectKey)
		So(err, ShouldBeNil)
		So(events, ShouldBeEmpty)
		So(pollertest.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})

		// At least 1 worker must finish successfully.
		errCnt, _ := errs.Summary()
		t.Logf("%d/%d workers failed", errCnt, w)
		So(errCnt, ShouldBeLessThan, w)
	})
}

func loadProjectEntities(ctx context.Context, luciProject string) (*prjmanager.Project, *prjmanager.ProjectStateOffload) {
	p := &prjmanager.Project{ID: luciProject}
	ps := &prjmanager.ProjectStateOffload{
		Project: datastore.MakeKey(ctx, prjmanager.ProjectKind, luciProject),
	}
	err := datastore.Get(ctx, p, ps)
	if merr, ok := err.(errors.MultiError); ok && merr[0] == datastore.ErrNoSuchEntity {
		So(merr[1], ShouldEqual, datastore.ErrNoSuchEntity)
		return nil, nil
	}
	So(err, ShouldBeNil)
	return p, ps
}

func singleRepoConfig(gHost string, gRepos ...string) *cfgpb.Config {
	projects := make([]*cfgpb.ConfigGroup_Gerrit_Project, len(gRepos))
	for i, gRepo := range gRepos {
		projects[i] = &cfgpb.ConfigGroup_Gerrit_Project{
			Name:      gRepo,
			RefRegexp: []string{"refs/heads/main"},
		}
	}
	return &cfgpb.Config{
		ConfigGroups: []*cfgpb.ConfigGroup{
			{
				Name: "main",
				Gerrit: []*cfgpb.ConfigGroup_Gerrit{
					{
						Url:      "https://" + gHost + "/",
						Projects: projects,
					},
				},
			},
		},
	}
}
