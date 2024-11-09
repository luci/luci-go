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
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap/gobmaptest"
	"go.chromium.org/luci/cv/internal/gerrit/poller"
	gerritupdater "go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
)

func TestProjectTQLateTasks(t *testing.T) {
	t.Parallel()

	ftt.Run("PM task does nothing if it comes too late", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		pmNotifier := prjmanager.NewNotifier(ct.TQDispatcher)
		runNotifier := runNotifierMock{}
		clMutator := changelist.NewMutator(ct.TQDispatcher, pmNotifier, &runNotifier, &tjMock{})
		clUpdater := changelist.NewUpdater(ct.TQDispatcher, clMutator)
		gerritupdater.RegisterUpdater(clUpdater, ct.GFactory())
		_ = New(pmNotifier, &runNotifier, clMutator, ct.GFactory(), clUpdater)

		const lProject = "infra"
		recipient := prjmanager.EventboxRecipient(ctx, lProject)

		prjcfgtest.Create(ctx, lProject, singleRepoConfig("host", "repo"))

		assert.Loosely(t, pmNotifier.UpdateConfig(ctx, lProject), should.BeNil)
		assert.Loosely(t, pmtest.Projects(ct.TQ.Tasks()), should.Resemble([]string{lProject}))
		events1, err := eventbox.List(ctx, recipient)
		assert.NoErr(t, err)

		// Simulate stuck TQ task, which gets executed with a huge delay.
		ct.Clock.Add(time.Hour)
		ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))
		// It must not modify PM state nor consume events.
		assert.Loosely(t, datastore.Get(ctx, &prjmanager.Project{ID: lProject}), should.Equal(datastore.ErrNoSuchEntity))
		events2, err := eventbox.List(ctx, recipient)
		assert.NoErr(t, err)
		assert.Loosely(t, events2, should.Resemble(events1))
		// But schedules new task instead.
		assert.Loosely(t, pmtest.Projects(ct.TQ.Tasks()), should.Resemble([]string{lProject}))

		// Next task coming ~on time proceeds normally.
		ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))
		assert.Loosely(t, datastore.Get(ctx, &prjmanager.Project{ID: lProject}), should.BeNil)
		events3, err := eventbox.List(ctx, recipient)
		assert.NoErr(t, err)
		assert.Loosely(t, events3, should.BeEmpty)
	})
}

func TestProjectLifeCycle(t *testing.T) {
	t.Parallel()

	ftt.Run("Project can be created, updated, deleted", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		pmNotifier := prjmanager.NewNotifier(ct.TQDispatcher)
		runNotifier := runNotifierMock{}
		clMutator := changelist.NewMutator(ct.TQDispatcher, pmNotifier, &runNotifier, &tjMock{})
		clUpdater := changelist.NewUpdater(ct.TQDispatcher, clMutator)
		gerritupdater.RegisterUpdater(clUpdater, ct.GFactory())
		_ = New(pmNotifier, &runNotifier, clMutator, ct.GFactory(), clUpdater)

		const lProject = "infra"
		recipient := prjmanager.EventboxRecipient(ctx, lProject)

		t.Run("with new project", func(t *ftt.Test) {
			prjcfgtest.Create(ctx, lProject, singleRepoConfig("host", "repo"))
			assert.Loosely(t, pmNotifier.UpdateConfig(ctx, lProject), should.BeNil)
			// Second event is a noop, but should still be consumed at once.
			assert.Loosely(t, pmNotifier.UpdateConfig(ctx, lProject), should.BeNil)
			assert.Loosely(t, pmtest.Projects(ct.TQ.Tasks()), should.Resemble([]string{lProject}))
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))
			events, err := eventbox.List(ctx, recipient)
			assert.NoErr(t, err)
			assert.Loosely(t, events, should.HaveLength(0))
			p, ps, plog := loadProjectEntities(t, ctx, lProject)
			assert.Loosely(t, p.EVersion, should.Equal(1))
			assert.Loosely(t, ps.Status, should.Equal(prjpb.Status_STARTED))
			assert.Loosely(t, plog, should.NotBeNil)
			assert.Loosely(t, poller.FilterProjects(ct.TQ.Tasks().SortByETA().Payloads()), should.Resemble([]string{lProject}))

			// Ensure first poller task gets executed.
			ct.Clock.Add(time.Hour)

			t.Run("update config with incomplete runs", func(t *ftt.Test) {
				err := datastore.Put(
					ctx,
					&run.Run{ID: common.RunID(lProject + "/111-beef"), CLs: common.CLIDs{111}},
					&run.Run{ID: common.RunID(lProject + "/222-cafe"), CLs: common.CLIDs{222}},
				)
				assert.NoErr(t, err)
				// This is what pmNotifier.notifyRunCreated func does,
				// but because it's private, it can't be called from this package.
				simulateRunCreated := func(suffix string) {
					e := &prjpb.Event{Event: &prjpb.Event_RunCreated{
						RunCreated: &prjpb.RunCreated{
							RunId: lProject + "/" + suffix,
						},
					}}
					value, err := proto.Marshal(e)
					assert.NoErr(t, err)
					assert.Loosely(t, eventbox.Emit(ctx, value, recipient), should.BeNil)
				}
				simulateRunCreated("111-beef")
				simulateRunCreated("222-cafe")

				prjcfgtest.Update(ctx, lProject, singleRepoConfig("host", "repo2"))
				assert.Loosely(t, pmNotifier.UpdateConfig(ctx, lProject), should.BeNil)

				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))

				p, _, plog = loadProjectEntities(t, ctx, lProject)
				assert.Loosely(t, p.IncompleteRuns(), should.Match(common.MakeRunIDs(lProject+"/111-beef", lProject+"/222-cafe")))
				assert.Loosely(t, plog, should.NotBeNil)
				assert.Loosely(t, runNotifier.popUpdateConfig(), should.Resemble(p.IncompleteRuns()))

				t.Run("disable project with incomplete runs", func(t *ftt.Test) {
					prjcfgtest.Disable(ctx, lProject)
					assert.Loosely(t, pmNotifier.UpdateConfig(ctx, lProject), should.BeNil)
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))

					p, ps, plog := loadProjectEntities(t, ctx, lProject)
					assert.Loosely(t, p.EVersion, should.Equal(3))
					assert.Loosely(t, ps.Status, should.Equal(prjpb.Status_STOPPING))
					assert.Loosely(t, plog, should.NotBeNil)
					assert.Loosely(t, poller.FilterProjects(ct.TQ.Tasks().SortByETA().Payloads()), should.Resemble([]string{lProject}))
					// Should ask Runs to cancel themselves.
					reqs := make([]cancellationRequest, len(p.IncompleteRuns()))
					for i, runID := range p.IncompleteRuns() {
						reqs[i] = cancellationRequest{
							id:     runID,
							reason: fmt.Sprintf("CV is disabled for LUCI Project %q", lProject),
						}
					}
					assert.Loosely(t, runNotifier.popCancel(), should.Resemble(reqs))

					t.Run("wait for all IncompleteRuns to finish", func(t *ftt.Test) {
						assert.Loosely(t, pmNotifier.NotifyRunFinished(ctx, common.RunID(lProject+"/111-beef"), run.Status_CANCELLED), should.BeNil)
						ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))

						p, ps, plog := loadProjectEntities(t, ctx, lProject)
						assert.Loosely(t, ps.Status, should.Equal(prjpb.Status_STOPPING))
						assert.Loosely(t, p.IncompleteRuns(), should.Resemble(common.MakeRunIDs(lProject+"/222-cafe")))
						assert.Loosely(t, plog, should.BeNil) // still STOPPING.

						assert.Loosely(t, pmNotifier.NotifyRunFinished(ctx, common.RunID(lProject+"/222-cafe"), run.Status_CANCELLED), should.BeNil)
						ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))

						p, ps, plog = loadProjectEntities(t, ctx, lProject)
						assert.Loosely(t, ps.Status, should.Equal(prjpb.Status_STOPPED))
						assert.Loosely(t, p.IncompleteRuns(), should.BeEmpty)
						assert.Loosely(t, plog, should.NotBeNil)
					})
				})
			})

			t.Run("delete project without incomplete runs", func(t *ftt.Test) {
				// No components means also no runs.
				p.State.Components = nil
				prjcfgtest.Delete(ctx, lProject)
				assert.Loosely(t, pmNotifier.UpdateConfig(ctx, lProject), should.BeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.ManageProjectTaskClass))

				p, ps, plog := loadProjectEntities(t, ctx, lProject)
				assert.Loosely(t, p.EVersion, should.Equal(2))
				assert.Loosely(t, ps.Status, should.Equal(prjpb.Status_STOPPED))
				assert.Loosely(t, plog, should.NotBeNil)
				assert.Loosely(t, poller.FilterProjects(ct.TQ.Tasks().SortByETA().Payloads()), should.Resemble([]string{lProject}))
			})
		})
	})
}

func TestProjectHandlesManyEvents(t *testing.T) {
	t.Parallel()

	ftt.Run("PM handles many events", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "infra"
		const gHost = "host"
		const gRepo = "repo"

		recipient := prjmanager.EventboxRecipient(ctx, lProject)
		pmNotifier := prjmanager.NewNotifier(ct.TQDispatcher)
		runNotifier := runNotifierMock{}
		clMutator := changelist.NewMutator(ct.TQDispatcher, pmNotifier, &runNotifier, &tjMock{})
		clUpdater := changelist.NewUpdater(ct.TQDispatcher, clMutator)
		gerritupdater.RegisterUpdater(clUpdater, ct.GFactory())
		pm := New(pmNotifier, &runNotifier, clMutator, ct.GFactory(), clUpdater)

		cfg := singleRepoConfig(gHost, gRepo)
		cfg.ConfigGroups[0].CombineCls = &cfgpb.CombineCLs{
			// Postpone creation of Runs, which isn't important in this test.
			StabilizationDelay: durationpb.New(time.Hour),
		}
		prjcfgtest.Create(ctx, lProject, cfg)
		gobmaptest.Update(ctx, lProject)

		// Put #43 CL directly w/o notifying the PM.
		cl43 := changelist.MustGobID(gHost, 43).MustCreateIfNotExists(ctx)
		cl43.Snapshot = &changelist.Snapshot{
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
		cl43.ApplicableConfig = &changelist.ApplicableConfig{
			Projects: []*changelist.ApplicableConfig_Project{
				{Name: lProject, ConfigGroupIds: []string{string(meta.ConfigGroupIDs[0])}},
			},
		}
		assert.Loosely(t, datastore.Put(ctx, cl43), should.BeNil)

		cl44 := changelist.MustGobID(gHost, 44).MustCreateIfNotExists(ctx)
		cl44.Snapshot = &changelist.Snapshot{
			ExternalUpdateTime:    timestamppb.New(ct.Clock.Now()),
			LuciProject:           lProject,
			MinEquivalentPatchset: 1,
			Patchset:              1,
			Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
				Info: gf.CI(
					44, gf.Project(gRepo), gf.Ref("refs/heads/main"),
					gf.CQ(+2, ct.Clock.Now(), gf.U("user-1"))),
			}},
		}
		cl44.ApplicableConfig = &changelist.ApplicableConfig{
			Projects: []*changelist.ApplicableConfig_Project{
				{Name: lProject, ConfigGroupIds: []string{string(meta.ConfigGroupIDs[0])}},
			},
		}
		assert.Loosely(t, datastore.Put(ctx, cl44), should.BeNil)

		// This event is the only event notifying PM about CL#43.
		assert.Loosely(t, pmNotifier.NotifyCLsUpdated(ctx, lProject, changelist.ToUpdatedEvents(cl43, cl44)), should.BeNil)

		const n = 20
		for i := 0; i < n; i++ {
			assert.Loosely(t, pmNotifier.UpdateConfig(ctx, lProject), should.BeNil)
			assert.Loosely(t, pmNotifier.Poke(ctx, lProject), should.BeNil)
			// Simulate updating a CL.
			cl44.EVersion++
			assert.Loosely(t, datastore.Put(ctx, cl44), should.BeNil)
			assert.Loosely(t, pmNotifier.NotifyCLsUpdated(ctx, lProject, changelist.ToUpdatedEvents(cl44)), should.BeNil)
		}

		events, err := eventbox.List(ctx, recipient)
		assert.NoErr(t, err)
		// Expect the following events:
		// +1 from NotifyCLsUpdated on cl43 and cl44,
		// +3*n from loop.
		assert.Loosely(t, events, should.HaveLength(3*n+1))

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
		assert.Loosely(t, datastore.Get(ctx, &p), should.BeNil)
		// Both cl43 and cl44 must have corresponding PCLs with latest EVersions.
		assert.Loosely(t, p.State.GetPcls(), should.HaveLength(2))
		for _, pcl := range p.State.GetPcls() {
			switch common.CLID(pcl.GetClid()) {
			case cl43.ID:
				assert.Loosely(t, pcl.GetEversion(), should.Equal(cl43.EVersion))
			case cl44.ID:
				assert.Loosely(t, pcl.GetEversion(), should.Equal(cl44.EVersion))
			default:
				assert.Loosely(t, "must not happen", should.BeTrue)
			}
		}

		events, err = eventbox.List(ctx, recipient)
		assert.NoErr(t, err)
		assert.Loosely(t, events, should.BeEmpty)
		assert.Loosely(t, poller.FilterProjects(ct.TQ.Tasks().SortByETA().Payloads()), should.Resemble([]string{lProject}))

		// At least 1 worker must finish successfully.
		errCnt, _ := errs.Summary()
		t.Logf("%d/%d workers failed", errCnt, w)
		assert.Loosely(t, errCnt, should.BeLessThan(w))
	})
}

func loadProjectEntities(t testing.TB, ctx context.Context, luciProject string) (
	*prjmanager.Project,
	*prjmanager.ProjectStateOffload,
	*prjmanager.ProjectLog,
) {
	t.Helper()

	p := &prjmanager.Project{ID: luciProject}
	switch err := datastore.Get(ctx, p); {
	case err == datastore.ErrNoSuchEntity:
		return nil, nil, nil
	case err != nil:
		panic(err)
	}

	key := datastore.MakeKey(ctx, prjmanager.ProjectKind, luciProject)
	ps := &prjmanager.ProjectStateOffload{Project: key}
	if err := datastore.Get(ctx, ps); err != nil {
		// ProjectStateOffload must exist if Project exists.
		panic(err)
	}

	plog := &prjmanager.ProjectLog{
		Project:  datastore.MakeKey(ctx, prjmanager.ProjectKind, luciProject),
		EVersion: p.EVersion,
	}
	switch err := datastore.Get(ctx, plog); {
	case err == datastore.ErrNoSuchEntity:
		return p, ps, nil
	case err != nil:
		panic(err)
	default:
		// Quick check invariant that plog replicates what's stored in Project &
		// ProjectStateOffload entities at the same EVersion.
		assert.Loosely(t, plog.EVersion, should.Equal(p.EVersion), truth.LineContext())
		assert.Loosely(t, plog.Status, should.Equal(ps.Status), truth.LineContext())
		assert.Loosely(t, plog.ConfigHash, should.Equal(ps.ConfigHash), truth.LineContext())
		assert.Loosely(t, plog.State, should.Resemble(p.State), truth.LineContext())
		assert.Loosely(t, plog.Reasons, should.NotBeEmpty, truth.LineContext())
		return p, ps, plog
	}
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

type runNotifierMock struct {
	m            sync.Mutex
	cancel       []cancellationRequest
	updateConfig common.RunIDs
}

type cancellationRequest struct {
	id     common.RunID
	reason string
}

func (r *runNotifierMock) NotifyCLsUpdated(ctx context.Context, rid common.RunID, cls *changelist.CLUpdatedEvents) error {
	panic("not implemented")
}

func (r *runNotifierMock) Start(ctx context.Context, id common.RunID) error {
	return nil
}

func (r *runNotifierMock) PokeNow(ctx context.Context, id common.RunID) error {
	panic("not implemented")
}

func (r *runNotifierMock) Cancel(ctx context.Context, id common.RunID, reason string) error {
	r.m.Lock()
	r.cancel = append(r.cancel, cancellationRequest{id: id, reason: reason})
	r.m.Unlock()
	return nil
}

func (r *runNotifierMock) UpdateConfig(ctx context.Context, id common.RunID, hash string, eversion int64) error {
	r.m.Lock()
	r.updateConfig = append(r.updateConfig, id)
	r.m.Unlock()
	return nil
}

func (r *runNotifierMock) popUpdateConfig() common.RunIDs {
	r.m.Lock()
	out := r.updateConfig
	r.updateConfig = nil
	r.m.Unlock()
	sort.Sort(out)
	return out
}

func (r *runNotifierMock) popCancel() []cancellationRequest {
	r.m.Lock()
	out := r.cancel
	r.cancel = nil
	r.m.Unlock()
	sort.Slice(out, func(i, j int) bool {
		return out[i].id < out[j].id
	})
	return out
}

type tjMock struct{}

func (t *tjMock) ScheduleCancelStale(ctx context.Context, clid common.CLID, prevMinEquivalentPatchset, currentMinEquivalentPatchset int32, eta time.Time) error {
	return nil
}
