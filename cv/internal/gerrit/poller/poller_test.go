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
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSchedule(t *testing.T) {
	t.Parallel()

	Convey("Schedule works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		ct.Clock.Set(ct.Clock.Now().Truncate(pollInterval).Add(pollInterval))
		const project = "chromium"

		p := New(ct.TQDispatcher, nil, nil, nil)

		So(p.schedule(ctx, project, time.Time{}), ShouldBeNil)
		payloads := FilterPayloads(ct.TQ.Tasks().SortByETA().Payloads())
		So(payloads, ShouldHaveLength, 1)
		first := payloads[0]
		So(first.GetLuciProject(), ShouldEqual, project)
		firstETA := first.GetEta().AsTime()
		So(firstETA.UnixNano(), ShouldBeBetweenOrEqual,
			ct.Clock.Now().UnixNano(), ct.Clock.Now().Add(pollInterval).UnixNano())

		Convey("idempotency via task deduplication", func() {
			So(p.schedule(ctx, project, time.Time{}), ShouldBeNil)
			So(FilterPayloads(ct.TQ.Tasks().SortByETA().Payloads()), ShouldHaveLength, 1)

			Convey("but only for the same project", func() {
				So(p.schedule(ctx, "another-project", time.Time{}), ShouldBeNil)
				ids := FilterProjects(ct.TQ.Tasks().SortByETA().Payloads())
				sort.Strings(ids)
				So(ids, ShouldResemble, []string{"another-project", project})
			})
		})

		Convey("schedule next poll", func() {
			So(p.schedule(ctx, project, firstETA), ShouldBeNil)
			payloads := FilterPayloads(ct.TQ.Tasks().SortByETA().Payloads())
			So(payloads, ShouldHaveLength, 2)
			So(payloads[1].GetEta().AsTime(), ShouldEqual, firstETA.Add(pollInterval))

			Convey("from a delayed prior poll", func() {
				ct.Clock.Set(firstETA.Add(pollInterval).Add(pollInterval / 2))
				So(p.schedule(ctx, project, firstETA), ShouldBeNil)
				payloads := FilterPayloads(ct.TQ.Tasks().SortByETA().Payloads())
				So(payloads, ShouldHaveLength, 3)
				So(payloads[2].GetEta().AsTime(), ShouldEqual, firstETA.Add(2*pollInterval))
			})
		})
	})
}

func TestObservesProjectLifetime(t *testing.T) {
	t.Parallel()

	Convey("Gerrit Poller observes project lifetime", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "chromium"
		const gHost = "chromium-review.example.com"
		const gRepo = "infra/infra"

		mustLoadState := func() *State {
			st := &State{LuciProject: lProject}
			So(datastore.Get(ctx, st), ShouldBeNil)
			return st
		}

		p := New(ct.TQDispatcher, ct.GFactory(), &clUpdaterMock{}, &pmMock{})

		Convey("Without project config, does nothing", func() {
			So(p.poll(ctx, lProject, ct.Clock.Now()), ShouldBeNil)
			So(ct.TQ.Tasks(), ShouldBeEmpty)
			So(datastore.Get(ctx, &State{LuciProject: lProject}), ShouldEqual, datastore.ErrNoSuchEntity)
		})

		Convey("For an existing project, runs via a task chain", func() {
			prjcfgtest.Create(ctx, lProject, singleRepoConfig(gHost, gRepo))
			So(p.poll(ctx, lProject, ct.Clock.Now()), ShouldBeNil)
			So(mustLoadState().EVersion, ShouldEqual, 1)
			for i := 0; i < 10; i++ {
				So(ct.TQ.Tasks(), ShouldHaveLength, 1)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(taskClassID))
			}
			So(mustLoadState().EVersion, ShouldEqual, 11)
		})

		Convey("On config changes, updates its state", func() {
			prjcfgtest.Create(ctx, lProject, singleRepoConfig(gHost, gRepo))
			So(p.poll(ctx, lProject, ct.Clock.Now()), ShouldBeNil)
			s := mustLoadState()
			So(s.QueryStates.GetStates(), ShouldHaveLength, 1)
			qs0 := s.QueryStates.GetStates()[0]
			So(qs0.GetHost(), ShouldResemble, gHost)
			So(qs0.GetOrProjects(), ShouldResemble, []string{gRepo})

			const gRepo2 = "infra/zzzzz"
			prjcfgtest.Update(ctx, lProject, singleRepoConfig(gHost, gRepo, gRepo2))
			So(p.poll(ctx, lProject, ct.Clock.Now()), ShouldBeNil)
			s = mustLoadState()
			So(s.QueryStates.GetStates(), ShouldHaveLength, 1)
			qs0 = s.QueryStates.GetStates()[0]
			So(qs0.GetOrProjects(), ShouldResemble, []string{gRepo, gRepo2})

			prjcfgtest.Update(ctx, lProject, singleRepoConfig(gHost, gRepo2))
			So(p.poll(ctx, lProject, ct.Clock.Now()), ShouldBeNil)
			s = mustLoadState()
			So(s.QueryStates.GetStates(), ShouldHaveLength, 1)
			qs0 = s.QueryStates.GetStates()[0]
			So(qs0.GetOrProjects(), ShouldResemble, []string{gRepo2})
		})

		Convey("Once project is disabled, deletes state and task chain stops running", func() {
			prjcfgtest.Create(ctx, lProject, singleRepoConfig(gHost, gRepo))
			So(p.poll(ctx, lProject, ct.Clock.Now()), ShouldBeNil)
			So(ct.TQ.Tasks(), ShouldHaveLength, 1)
			So(mustLoadState().EVersion, ShouldEqual, 1)

			prjcfgtest.Disable(ctx, lProject)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(taskClassID))

			So(datastore.Get(ctx, &State{LuciProject: lProject}), ShouldEqual, datastore.ErrNoSuchEntity)
			So(ct.TQ.Tasks(), ShouldBeEmpty)
		})
	})
}

func TestDiscoversCLs(t *testing.T) {
	t.Parallel()

	Convey("Gerrit Poller discovers CLs", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "chromium"
		const gHost = "chromium-review.example.com"
		const gRepo = "infra/infra"

		mustLoadState := func() *State {
			st := &State{LuciProject: lProject}
			So(datastore.Get(ctx, st), ShouldBeNil)
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
		So(p.poll(ctx, lProject, ct.Clock.Now()), ShouldBeNil)
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

		Convey("Discover all CLs in case of a full query", func() {
			s := mustLoadState()
			s.QueryStates.GetStates()[0].LastFullTime = nil // Force "full" fetch
			So(datastore.Put(ctx, s), ShouldBeNil)

			postFullQueryVerify := func() {
				qs := mustLoadState().QueryStates.GetStates()[0]
				So(qs.GetLastFullTime().AsTime(), ShouldResemble, ct.Clock.Now().UTC())
				So(qs.GetLastIncrTime(), ShouldBeNil)
				So(qs.GetChanges(), ShouldResemble, []int64{31, 32, 33, 34, 35})
			}

			Convey("On project start, just creates CLUpdater tasks with forceNotify", func() {
				So(p.poll(ctx, lProject, ct.Clock.Now()), ShouldBeNil)
				So(clUpdater.peekScheduledChanges(), ShouldResemble, []int{31, 32, 33, 34, 35})
				postFullQueryVerify()
			})

			Convey("In a typical case, uses forceNotify judiciously", func() {
				// In a typical case, CV has been polling before and so is already aware
				// of every CL except 35.
				s.QueryStates.GetStates()[0].Changes = []int64{31, 32, 33, 34}
				So(datastore.Put(ctx, s), ShouldBeNil)
				// However, 34 may not yet have an CL entity.
				knownCLIDs := common.CLIDs{
					ensureCLEntity(31).ID,
					ensureCLEntity(32).ID,
					ensureCLEntity(33).ID,
				}

				So(p.poll(ctx, lProject, ct.Clock.Now()), ShouldBeNil)

				// PM must be notified in "bulk".
				So(pm.popNotifiedCLs(lProject), ShouldResemble, sortedCLIDs(knownCLIDs...))
				// All CLs must have clUpdater tasks.
				So(clUpdater.peekScheduledChanges(), ShouldResemble, []int{31, 32, 33, 34, 35})
				postFullQueryVerify()
			})

			Convey("When previously known changes are no longer found, forces their refresh, too", func() {
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
				So(datastore.Put(ctx, s), ShouldBeNil)
				var knownCLIDs common.CLIDs
				for _, c := range s.QueryStates.GetStates()[0].Changes {
					knownCLIDs = append(knownCLIDs, ensureCLEntity(c).ID)
				}

				So(p.poll(ctx, lProject, ct.Clock.Now()), ShouldBeNil)

				// PM must be notified in "bulk" for all previously known CLs.
				So(pm.popNotifiedCLs(lProject), ShouldResemble, sortedCLIDs(knownCLIDs...))
				// All current and prior CLs must have clUpdater tasks.
				So(clUpdater.peekScheduledChanges(), ShouldResemble, []int{25, 26, 27, 31, 32, 33, 34, 35})
				postFullQueryVerify()
			})

			Convey("On config change, force notifies PM and runs a full query", func() {
				// Strictly speaking, this test isn't just a full poll, but also a
				// config change.

				// Simulate prior full fetch has just happened.
				s.QueryStates.GetStates()[0].LastFullTime = timestamppb.New(ct.Clock.Now().Add(-pollInterval))
				// But with a different query.
				s.QueryStates.GetStates()[0].OrProjects = []string{gRepo, "repo/which/had/cl30"}
				// And all CLs except but 35 are already known but also CL 30.
				s.QueryStates.GetStates()[0].Changes = []int64{30, 31, 32, 33, 34}
				s.ConfigHash = "some/other/hash"
				So(datastore.Put(ctx, s), ShouldBeNil)
				var knownCLIDs common.CLIDs
				for _, c := range s.QueryStates.GetStates()[0].Changes {
					knownCLIDs = append(knownCLIDs, ensureCLEntity(c).ID)
				}

				So(p.poll(ctx, lProject, ct.Clock.Now()), ShouldBeNil)

				// PM must be notified about all prior CLs.
				So(pm.popNotifiedCLs(lProject), ShouldResemble, sortedCLIDs(knownCLIDs...))
				// All CLs must have clUpdater tasks.
				// NOTE: the code isn't optimized for this use case, so there will be
				// multiple tasks for changes 31..34. While this is unfortunte, it's
				// rare enough that it doesn't really matter.
				So(clUpdater.peekScheduledChanges(), ShouldResemble, []int{30, 31, 31, 32, 32, 33, 33, 34, 34, 35})
				postFullQueryVerify()

				qs := mustLoadState().QueryStates.GetStates()[0]
				So(qs.GetOrProjects(), ShouldResemble, []string{gRepo})
			})
		})

		Convey("Discover most recently modified CLs only in case of an incremental query", func() {
			s := mustLoadState()
			s.QueryStates.GetStates()[0].LastFullTime = timestamppb.New(ct.Clock.Now()) // Force incremental fetch
			So(datastore.Put(ctx, s), ShouldBeNil)

			Convey("Unless the pubsub is enabled", func() {
				So(p.poll(ctx, lProject, ct.Clock.Now()), ShouldBeNil)
				So(clUpdater.peekScheduledChanges(), ShouldBeEmpty)
				qs := mustLoadState().QueryStates.GetStates()[0]
				So(qs.GetLastIncrTime(), ShouldBeNil)
			})

			ct.DisableProjectInGerritListener(ctx, lProject)

			Convey("In a typical case, schedules update tasks for new CLs", func() {
				s.QueryStates.GetStates()[0].Changes = []int64{31, 32, 33}
				So(datastore.Put(ctx, s), ShouldBeNil)
				So(p.poll(ctx, lProject, ct.Clock.Now()), ShouldBeNil)

				So(clUpdater.peekScheduledChanges(), ShouldResemble, []int{34, 35, 36})

				qs := mustLoadState().QueryStates.GetStates()[0]
				So(qs.GetLastIncrTime().AsTime(), ShouldResemble, ct.Clock.Now().UTC())
				So(qs.GetChanges(), ShouldResemble, []int64{31, 32, 33, 34, 35, 36})
			})

			Convey("Even if CL is already known, schedules update tasks", func() {
				s.QueryStates.GetStates()[0].Changes = []int64{31, 32, 33, 34, 35, 36}
				So(datastore.Put(ctx, s), ShouldBeNil)
				So(p.poll(ctx, lProject, ct.Clock.Now()), ShouldBeNil)

				qs := mustLoadState().QueryStates.GetStates()[0]
				So(qs.GetLastIncrTime().AsTime(), ShouldResemble, ct.Clock.Now().UTC())
				So(qs.GetChanges(), ShouldResemble, []int64{31, 32, 33, 34, 35, 36})
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
