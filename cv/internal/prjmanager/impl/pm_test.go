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

package impl

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"
	"google.golang.org/protobuf/proto"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/gerrit/poller/pollertest"
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

		const lProject = "infra"
		lProjectKey := datastore.MakeKey(ctx, prjmanager.ProjectKind, lProject)

		ct.Cfg.Create(ctx, lProject, singleRepoConfig("host", "repo"))

		So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
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

		const lProject = "infra"
		lProjectKey := datastore.MakeKey(ctx, prjmanager.ProjectKind, lProject)

		Convey("with new project", func() {
			ct.Cfg.Create(ctx, lProject, singleRepoConfig("host", "repo"))
			So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
			// Second event is noop, but should still be consumed at once.
			So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
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
					&run.Run{ID: common.RunID(lProject + "/111-beef"), CLs: []common.CLID{111}},
					&run.Run{ID: common.RunID(lProject + "/222-cafe"), CLs: []common.CLID{222}},
				)
				So(err, ShouldBeNil)
				// This is what prjmanager.notifyRunCreated func does,
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

				ct.Cfg.Update(ctx, lProject, singleRepoConfig("host", "repo2"))
				So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)

				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))

				p, _ = loadProjectEntities(ctx, lProject)
				So(p.IncompleteRuns(), ShouldEqual, common.MakeRunIDs(lProject+"/111-beef", lProject+"/222-cafe"))
				// Must schedule a task per Run for config updates for each of the
				// started run.
				runsWithTasks := runtest.SortedRuns(ct.TQ.Tasks())
				So(runsWithTasks, ShouldResemble, p.IncompleteRuns())

				Convey("disable project with incomplete runs", func() {
					ct.Cfg.Disable(ctx, lProject)
					So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))

					p, ps := loadProjectEntities(ctx, lProject)
					So(p.EVersion, ShouldEqual, 3)
					So(ps.Status, ShouldEqual, prjpb.Status_STOPPING)
					So(pollertest.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})

					// Must schedule a task per Run for cancelation on top of already
					// scheduled ones before for config disabling.
					expected := append(runsWithTasks, p.IncompleteRuns()...)
					sort.Sort(expected)
					So(runtest.SortedRuns(ct.TQ.Tasks()), ShouldResemble, expected)

					Convey("wait for all IncompleteRuns to finish", func() {
						So(prjmanager.NotifyRunFinished(ctx, common.RunID(lProject+"/111-beef")), ShouldBeNil)
						ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))
						p, ps := loadProjectEntities(ctx, lProject)
						So(ps.Status, ShouldEqual, prjpb.Status_STOPPING)
						So(p.IncompleteRuns(), ShouldResemble, common.MakeRunIDs(lProject+"/222-cafe"))

						So(prjmanager.NotifyRunFinished(ctx, common.RunID(lProject+"/222-cafe")), ShouldBeNil)
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
				ct.Cfg.Delete(ctx, lProject)
				So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
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
		lProjectKey := datastore.MakeKey(ctx, prjmanager.ProjectKind, lProject)

		ct.Cfg.Create(ctx, lProject, singleRepoConfig("host", "repo"))

		const n = 10
		for i := 0; i < n; i++ {
			So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
			So(prjmanager.Poke(ctx, lProject), ShouldBeNil)
		}
		events, err := eventbox.List(ctx, lProjectKey)
		So(err, ShouldBeNil)
		So(events, ShouldHaveLength, 2*n)

		const w = 20
		now := ct.Clock.Now()
		errs := make(errors.MultiError, w)
		wg := sync.WaitGroup{}
		wg.Add(w)
		for i := 0; i < w; i++ {
			i := i
			go func() {
				defer wg.Done()
				errs[i] = pokePMTask(ctx, lProject, now)
			}()
		}
		wg.Wait()

		// Exactly 1 of the workers must create PM entity, consume events and
		// poke the poller.
		p := prjmanager.Project{ID: lProject}
		So(datastore.Get(ctx, &p), ShouldBeNil)
		So(p.EVersion, ShouldEqual, 1)
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
