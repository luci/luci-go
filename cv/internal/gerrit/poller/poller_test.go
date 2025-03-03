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

package poller

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
)

func TestSchedule(t *testing.T) {
	t.Parallel()

	ftt.Run("Schedule works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		ct.Clock.Set(ct.Clock.Now().Truncate(pollInterval).Add(pollInterval))
		const project = "chromium"

		p := New(ct.TQDispatcher, nil, nil, nil)

		assert.Loosely(t, p.schedule(ctx, project, time.Time{}), should.BeNil)
		payloads := FilterPayloads(ct.TQ.Tasks().SortByETA().Payloads())
		assert.Loosely(t, payloads, should.HaveLength(1))
		first := payloads[0]
		assert.Loosely(t, first.GetLuciProject(), should.Equal(project))
		firstETA := first.GetEta().AsTime()
		assert.Loosely(t, firstETA.UnixNano(), should.BeBetweenOrEqual(
			ct.Clock.Now().UnixNano(), ct.Clock.Now().Add(pollInterval).UnixNano()))

		t.Run("idempotency via task deduplication", func(t *ftt.Test) {
			assert.Loosely(t, p.schedule(ctx, project, time.Time{}), should.BeNil)
			assert.Loosely(t, FilterPayloads(ct.TQ.Tasks().SortByETA().Payloads()), should.HaveLength(1))

			t.Run("but only for the same project", func(t *ftt.Test) {
				assert.Loosely(t, p.schedule(ctx, "another-project", time.Time{}), should.BeNil)
				ids := FilterProjects(ct.TQ.Tasks().SortByETA().Payloads())
				sort.Strings(ids)
				assert.That(t, ids, should.Match([]string{"another-project", project}))
			})
		})

		t.Run("schedule next poll", func(t *ftt.Test) {
			assert.Loosely(t, p.schedule(ctx, project, firstETA), should.BeNil)
			payloads := FilterPayloads(ct.TQ.Tasks().SortByETA().Payloads())
			assert.Loosely(t, payloads, should.HaveLength(2))
			assert.That(t, payloads[1].GetEta().AsTime(), should.Match(firstETA.Add(pollInterval)))

			t.Run("from a delayed prior poll", func(t *ftt.Test) {
				ct.Clock.Set(firstETA.Add(pollInterval).Add(pollInterval / 2))
				assert.Loosely(t, p.schedule(ctx, project, firstETA), should.BeNil)
				payloads := FilterPayloads(ct.TQ.Tasks().SortByETA().Payloads())
				assert.Loosely(t, payloads, should.HaveLength(3))
				assert.That(t, payloads[2].GetEta().AsTime(), should.Match(firstETA.Add(2*pollInterval)))
			})
		})
	})
}

func TestObservesProjectLifetime(t *testing.T) {
	t.Parallel()

	ftt.Run("Gerrit Poller observes project lifetime", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "chromium"
		const gHost = "chromium-review.example.com"
		const gRepo = "infra/infra"

		mustLoadState := func() *State {
			st := &State{LuciProject: lProject}
			assert.Loosely(t, datastore.Get(ctx, st), should.BeNil)
			return st
		}

		p := New(ct.TQDispatcher, ct.GFactory(), &clUpdaterMock{}, &pmMock{})

		t.Run("Without project config, does nothing", func(t *ftt.Test) {
			assert.Loosely(t, p.poll(ctx, lProject, ct.Clock.Now()), should.BeNil)
			assert.Loosely(t, ct.TQ.Tasks(), should.BeEmpty)
			assert.Loosely(t, datastore.Get(ctx, &State{LuciProject: lProject}), should.Equal(datastore.ErrNoSuchEntity))
		})

		t.Run("For an existing project, runs via a task chain", func(t *ftt.Test) {
			prjcfgtest.Create(ctx, lProject, singleRepoConfig(gHost, gRepo))
			assert.Loosely(t, p.poll(ctx, lProject, ct.Clock.Now()), should.BeNil)
			assert.Loosely(t, mustLoadState().EVersion, should.Equal(1))
			for i := 0; i < 10; i++ {
				assert.Loosely(t, ct.TQ.Tasks(), should.HaveLength(1))
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(taskClassID))
			}
			assert.Loosely(t, mustLoadState().EVersion, should.Equal(11))
		})

		t.Run("On config changes, updates its state", func(t *ftt.Test) {
			prjcfgtest.Create(ctx, lProject, singleRepoConfig(gHost, gRepo))
			assert.Loosely(t, p.poll(ctx, lProject, ct.Clock.Now()), should.BeNil)
			s := mustLoadState()
			assert.Loosely(t, s.QueryStates.GetStates(), should.HaveLength(1))
			qs0 := s.QueryStates.GetStates()[0]
			assert.That(t, qs0.GetHost(), should.Match(gHost))
			assert.That(t, qs0.GetOrProjects(), should.Match([]string{gRepo}))

			const gRepo2 = "infra/zzzzz"
			prjcfgtest.Update(ctx, lProject, singleRepoConfig(gHost, gRepo, gRepo2))
			assert.Loosely(t, p.poll(ctx, lProject, ct.Clock.Now()), should.BeNil)
			s = mustLoadState()
			assert.Loosely(t, s.QueryStates.GetStates(), should.HaveLength(1))
			qs0 = s.QueryStates.GetStates()[0]
			assert.That(t, qs0.GetOrProjects(), should.Match([]string{gRepo, gRepo2}))

			prjcfgtest.Update(ctx, lProject, singleRepoConfig(gHost, gRepo2))
			assert.Loosely(t, p.poll(ctx, lProject, ct.Clock.Now()), should.BeNil)
			s = mustLoadState()
			assert.Loosely(t, s.QueryStates.GetStates(), should.HaveLength(1))
			qs0 = s.QueryStates.GetStates()[0]
			assert.That(t, qs0.GetOrProjects(), should.Match([]string{gRepo2}))
		})

		t.Run("Once project is disabled, deletes state and task chain stops running", func(t *ftt.Test) {
			prjcfgtest.Create(ctx, lProject, singleRepoConfig(gHost, gRepo))
			assert.Loosely(t, p.poll(ctx, lProject, ct.Clock.Now()), should.BeNil)
			assert.Loosely(t, ct.TQ.Tasks(), should.HaveLength(1))
			assert.Loosely(t, mustLoadState().EVersion, should.Equal(1))

			prjcfgtest.Disable(ctx, lProject)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(taskClassID))

			assert.Loosely(t, datastore.Get(ctx, &State{LuciProject: lProject}), should.Equal(datastore.ErrNoSuchEntity))
			assert.Loosely(t, ct.TQ.Tasks(), should.BeEmpty)
		})
	})
}

func TestDiscoversCLs(t *testing.T) {
	t.Parallel()

	ftt.Run("Gerrit Poller discovers CLs", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "chromium"
		const gHost = "chromium-review.example.com"
		const gRepo = "infra/infra"

		mustLoadState := func() *State {
			st := &State{LuciProject: lProject}
			assert.Loosely(t, datastore.Get(ctx, st), should.BeNil)
			return st
		}
		ensureCLEntity := func(change int64) *changelist.CL {
			return changelist.MustGobID(gHost, change).MustCreateIfNotExists(ctx)
		}

		pm := pmMock{}
		clUpdater := clUpdaterMock{}
		p := New(ct.TQDispatcher, ct.GFactory(), &clUpdater, &pm)

		prjcfgtest.Create(ctx, lProject, singleRepoConfig(gHost, gRepo))
		// Initialize Poller state for ease of modifications in test later.
		assert.Loosely(t, p.poll(ctx, lProject, ct.Clock.Now()), should.BeNil)
		ct.Clock.Add(10 * fullPollInterval)

		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(),
			// These CLs ordered from oldest to newest by .Updated.
			gf.CI(31, gf.CQ(+2), gf.Project(gRepo), gf.Updated(ct.Clock.Now().Add(-2*fullPollInterval))),
			gf.CI(32, gf.CQ(+1), gf.Project(gRepo), gf.Updated(ct.Clock.Now().Add(-1*fullPollInterval))),
			gf.CI(33, gf.CQ(+2), gf.Project(gRepo), gf.Updated(ct.Clock.Now().Add(-2*incrementalPollOverlap))),
			gf.CI(34, gf.CQ(+1), gf.Project(gRepo), gf.Updated(ct.Clock.Now().Add(-1*incrementalPollOverlap))),
			gf.CI(35, gf.CQ(+1), gf.Project(gRepo), gf.Updated(ct.Clock.Now().Add(-1*time.Millisecond))),
			// No CQ vote. This will not show up on a full query, but only on an incremental.
			gf.CI(36, gf.Project(gRepo), gf.Updated(ct.Clock.Now().Add(-time.Minute))),

			// These must not be matched.
			gf.CI(40, gf.CQ(+2), gf.Project(gRepo), gf.Updated(ct.Clock.Now().Add(-common.MaxTriggerAge-time.Second))),
			gf.CI(41, gf.CQ(+2), gf.Project("another/project"), gf.Updated(ct.Clock.Now())),
			// TODO(tandrii): when poller becomes ref-ware, add this to the test.
			// gf.CI(42, gf.CQ(+2), gf.Project(gRepo), gf.Ref("refs/not/matched"), gf.Updated(ct.Clock.Now())),
		))

		t.Run("Discover all CLs in case of a full query", func(t *ftt.Test) {
			s := mustLoadState()
			s.QueryStates.GetStates()[0].LastFullTime = nil // Force "full" fetch
			assert.Loosely(t, datastore.Put(ctx, s), should.BeNil)

			postFullQueryVerify := func() {
				qs := mustLoadState().QueryStates.GetStates()[0]
				assert.That(t, qs.GetLastFullTime().AsTime(), should.Match(ct.Clock.Now().UTC()))
				assert.Loosely(t, qs.GetLastIncrTime(), should.BeNil)
				assert.That(t, qs.GetChanges(), should.Match([]int64{31, 32, 33, 34, 35}))
			}

			t.Run("On project start, just creates CLUpdater tasks with forceNotify", func(t *ftt.Test) {
				assert.Loosely(t, p.poll(ctx, lProject, ct.Clock.Now()), should.BeNil)
				assert.That(t, clUpdater.peekScheduledChanges(), should.Match([]int{31, 32, 33, 34, 35}))
				postFullQueryVerify()
			})

			t.Run("In a typical case, uses forceNotify judiciously", func(t *ftt.Test) {
				// In a typical case, CV has been polling before and so is already aware
				// of every CL except 35.
				s.QueryStates.GetStates()[0].Changes = []int64{31, 32, 33, 34}
				assert.Loosely(t, datastore.Put(ctx, s), should.BeNil)
				// However, 34 may not yet have an CL entity.
				knownCLIDs := common.CLIDs{
					ensureCLEntity(31).ID,
					ensureCLEntity(32).ID,
					ensureCLEntity(33).ID,
				}

				assert.Loosely(t, p.poll(ctx, lProject, ct.Clock.Now()), should.BeNil)

				// PM must be notified in "bulk".
				assert.That(t, pm.popNotifiedCLs(lProject), should.Match(sortedCLIDs(knownCLIDs...)))
				// All CLs must have clUpdater tasks.
				assert.That(t, clUpdater.peekScheduledChanges(), should.Match([]int{31, 32, 33, 34, 35}))
				postFullQueryVerify()
			})

			t.Run("When previously known changes are no longer found, forces their refresh, too", func(t *ftt.Test) {
				// Test common occurrence of CL no longer appearing in query
				// results due to user or even CV action.
				ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(),
					// No CQ vote.
					gf.CI(25, gf.Project(gRepo), gf.Updated(ct.Clock.Now().Add(-time.Minute))),
					// Abandoned.
					gf.CI(26, gf.Status(gerritpb.ChangeStatus_ABANDONED), gf.CQ(+2), gf.Project(gRepo), gf.Updated(ct.Clock.Now().Add(-time.Minute))),
					// Submitted.
					gf.CI(27, gf.Status(gerritpb.ChangeStatus_ABANDONED), gf.CQ(+2), gf.Project(gRepo), gf.Updated(ct.Clock.Now().Add(-time.Minute))),
				))
				s.QueryStates.GetStates()[0].Changes = []int64{25, 26, 27, 31, 32, 33, 34}
				assert.Loosely(t, datastore.Put(ctx, s), should.BeNil)
				var knownCLIDs common.CLIDs
				for _, c := range s.QueryStates.GetStates()[0].Changes {
					knownCLIDs = append(knownCLIDs, ensureCLEntity(c).ID)
				}

				assert.Loosely(t, p.poll(ctx, lProject, ct.Clock.Now()), should.BeNil)

				// PM must be notified in "bulk" for all previously known CLs.
				assert.That(t, pm.popNotifiedCLs(lProject), should.Match(sortedCLIDs(knownCLIDs...)))
				// All current and prior CLs must have clUpdater tasks.
				assert.That(t, clUpdater.peekScheduledChanges(), should.Match([]int{25, 26, 27, 31, 32, 33, 34, 35}))
				postFullQueryVerify()
			})

			t.Run("On config change, force notifies PM and runs a full query", func(t *ftt.Test) {
				// Strictly speaking, this test isn't just a full poll, but also a
				// config change.

				// Simulate prior full fetch has just happened.
				s.QueryStates.GetStates()[0].LastFullTime = timestamppb.New(ct.Clock.Now().Add(-pollInterval))
				// But with a different query.
				s.QueryStates.GetStates()[0].OrProjects = []string{gRepo, "repo/which/had/cl30"}
				// And all CLs except but 35 are already known but also CL 30.
				s.QueryStates.GetStates()[0].Changes = []int64{30, 31, 32, 33, 34}
				s.ConfigHash = "some/other/hash"
				assert.Loosely(t, datastore.Put(ctx, s), should.BeNil)
				var knownCLIDs common.CLIDs
				for _, c := range s.QueryStates.GetStates()[0].Changes {
					knownCLIDs = append(knownCLIDs, ensureCLEntity(c).ID)
				}

				assert.Loosely(t, p.poll(ctx, lProject, ct.Clock.Now()), should.BeNil)

				// PM must be notified about all prior CLs.
				assert.That(t, pm.popNotifiedCLs(lProject), should.Match(sortedCLIDs(knownCLIDs...)))
				// All CLs must have clUpdater tasks.
				// NOTE: the code isn't optimized for this use case, so there will be
				// multiple tasks for changes 31..34. While this is unfortunte, it's
				// rare enough that it doesn't really matter.
				assert.That(t, clUpdater.peekScheduledChanges(), should.Match([]int{30, 31, 31, 32, 32, 33, 33, 34, 34, 35}))
				postFullQueryVerify()

				qs := mustLoadState().QueryStates.GetStates()[0]
				assert.That(t, qs.GetOrProjects(), should.Match([]string{gRepo}))
			})
		})

		t.Run("Discover most recently modified CLs only in case of an incremental query", func(t *ftt.Test) {
			s := mustLoadState()
			s.QueryStates.GetStates()[0].LastFullTime = timestamppb.New(ct.Clock.Now()) // Force incremental fetch
			assert.Loosely(t, datastore.Put(ctx, s), should.BeNil)

			t.Run("Unless the pubsub is enabled", func(t *ftt.Test) {
				assert.Loosely(t, p.poll(ctx, lProject, ct.Clock.Now()), should.BeNil)
				assert.Loosely(t, clUpdater.peekScheduledChanges(), should.BeEmpty)
				qs := mustLoadState().QueryStates.GetStates()[0]
				assert.Loosely(t, qs.GetLastIncrTime(), should.BeNil)
			})

			ct.DisableProjectInGerritListener(ctx, lProject)

			t.Run("In a typical case, schedules update tasks for new CLs", func(t *ftt.Test) {
				s.QueryStates.GetStates()[0].Changes = []int64{31, 32, 33}
				assert.Loosely(t, datastore.Put(ctx, s), should.BeNil)
				assert.Loosely(t, p.poll(ctx, lProject, ct.Clock.Now()), should.BeNil)

				assert.That(t, clUpdater.peekScheduledChanges(), should.Match([]int{34, 35, 36}))

				qs := mustLoadState().QueryStates.GetStates()[0]
				assert.That(t, qs.GetLastIncrTime().AsTime(), should.Match(ct.Clock.Now().UTC()))
				assert.That(t, qs.GetChanges(), should.Match([]int64{31, 32, 33, 34, 35, 36}))
			})

			t.Run("Even if CL is already known, schedules update tasks", func(t *ftt.Test) {
				s.QueryStates.GetStates()[0].Changes = []int64{31, 32, 33, 34, 35, 36}
				assert.Loosely(t, datastore.Put(ctx, s), should.BeNil)
				assert.Loosely(t, p.poll(ctx, lProject, ct.Clock.Now()), should.BeNil)

				qs := mustLoadState().QueryStates.GetStates()[0]
				assert.That(t, qs.GetLastIncrTime().AsTime(), should.Match(ct.Clock.Now().UTC()))
				assert.That(t, qs.GetChanges(), should.Match([]int64{31, 32, 33, 34, 35, 36}))
			})

		})
	})
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

func sharedPrefixRepos(prefix string, n int) []string {
	rs := make([]string, n)
	for i := range rs {
		rs[i] = fmt.Sprintf("%s/%03d", prefix, i)
	}
	return rs
}

type pmMock struct {
	projects map[string]common.CLIDs
}

func (p *pmMock) NotifyCLsUpdated(ctx context.Context, project string, cls *changelist.CLUpdatedEvents) error {
	if p.projects == nil {
		p.projects = make(map[string]common.CLIDs, len(cls.GetEvents()))
	}
	for _, e := range cls.GetEvents() {
		p.projects[project] = append(p.projects[project], common.CLID(e.GetClid()))
	}
	return nil
}

func (p *pmMock) popNotifiedCLs(luciProject string) common.CLIDs {
	if p.projects == nil {
		return nil
	}
	res := p.projects[luciProject]
	delete(p.projects, luciProject)
	return sortedCLIDs(res...)
}

func sortedCLIDs(ids ...common.CLID) common.CLIDs {
	res := common.CLIDs(ids)
	res.Dedupe() // it also sorts as a by-product.
	return res
}

type clUpdaterMock struct {
	m     sync.Mutex
	tasks []struct {
		payload *changelist.UpdateCLTask
		eta     time.Time
	}
}

func (c *clUpdaterMock) Schedule(ctx context.Context, tsk *changelist.UpdateCLTask) error {
	return c.ScheduleDelayed(ctx, tsk, 0)
}

func (c *clUpdaterMock) ScheduleDelayed(ctx context.Context, tsk *changelist.UpdateCLTask, d time.Duration) error {
	c.m.Lock()
	defer c.m.Unlock()
	c.tasks = append(c.tasks, struct {
		payload *changelist.UpdateCLTask
		eta     time.Time
	}{tsk, clock.Now(ctx).Add(d)})
	return nil
}

func (c *clUpdaterMock) sortTasksByETAlocked() {
	sort.Slice(c.tasks, func(i, j int) bool { return c.tasks[i].eta.Before(c.tasks[j].eta) })
}

func (c *clUpdaterMock) peekETAs() []time.Time {
	c.m.Lock()
	defer c.m.Unlock()
	c.sortTasksByETAlocked()
	out := make([]time.Time, len(c.tasks))
	for i, tsk := range c.tasks {
		out[i] = tsk.eta
	}
	return out
}

func (c *clUpdaterMock) popPayloadsByETA() []*changelist.UpdateCLTask {
	c.m.Lock()
	c.sortTasksByETAlocked()
	tasks := c.tasks
	c.tasks = nil
	c.m.Unlock()

	out := make([]*changelist.UpdateCLTask, len(tasks))
	for i, tsk := range tasks {
		out[i] = tsk.payload
	}
	return out
}

func (c *clUpdaterMock) peekScheduledChanges() []int {
	c.m.Lock()
	defer c.m.Unlock()
	out := make([]int, len(c.tasks))
	for i, tsk := range c.tasks {
		_, change, err := changelist.ExternalID(tsk.payload.GetExternalId()).ParseGobID()
		if err != nil {
			panic(err)
		}
		out[i] = int(change)
	}
	sort.Ints(out)
	return out
}
