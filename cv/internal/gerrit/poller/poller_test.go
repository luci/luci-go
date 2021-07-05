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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
	pt "go.chromium.org/luci/cv/internal/gerrit/poller/pollertest"
	"go.chromium.org/luci/cv/internal/gerrit/poller/task"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/gerrit/updater/updatertest"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSchedule(t *testing.T) {
	t.Parallel()

	Convey("schedule works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		ct.Clock.Set(ct.Clock.Now().Truncate(pollInterval).Add(pollInterval))
		const project = "chromium"

		p := New(ct.TQDispatcher, nil, nil, nil)

		Convey("schedule works", func() {
			So(p.schedule(ctx, project, time.Time{}), ShouldBeNil)
			payloads := pt.PFilter(ct.TQ.Tasks())
			So(payloads, ShouldHaveLength, 1)
			first := payloads[0]
			So(first.GetLuciProject(), ShouldEqual, project)
			firstETA := first.GetEta().AsTime()
			So(firstETA.UnixNano(), ShouldBeBetweenOrEqual,
				ct.Clock.Now().UnixNano(), ct.Clock.Now().Add(pollInterval).UnixNano())

			Convey("idempotency via task deduplication", func() {
				So(p.schedule(ctx, project, time.Time{}), ShouldBeNil)
				So(pt.PFilter(ct.TQ.Tasks()), ShouldHaveLength, 1)

				Convey("but only for the same project", func() {
					So(p.schedule(ctx, "another-project", time.Time{}), ShouldBeNil)
					ids := pt.Projects(ct.TQ.Tasks())
					sort.Strings(ids)
					So(ids, ShouldResemble, []string{"another-project", project})
				})
			})

			Convey("schedule next poll", func() {
				So(p.schedule(ctx, project, firstETA), ShouldBeNil)
				payloads := pt.PFilter(ct.TQ.Tasks().SortByETA())
				So(payloads, ShouldHaveLength, 2)
				So(payloads[1].GetEta().AsTime(), ShouldEqual, firstETA.Add(pollInterval))

				Convey("from a delayed prior poll", func() {
					ct.Clock.Set(firstETA.Add(pollInterval).Add(pollInterval / 2))
					So(p.schedule(ctx, project, firstETA), ShouldBeNil)
					payloads := pt.PFilter(ct.TQ.Tasks().SortByETA())
					So(payloads, ShouldHaveLength, 3)
					So(payloads[2].GetEta().AsTime(), ShouldEqual, firstETA.Add(2*pollInterval))
				})
			})
		})
	})
}

func TestScheduleRefreshTasks(t *testing.T) {
	t.Parallel()

	Convey("scheduleRefreshTasks works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "chromium"
		const gHost = "chromium-review.example.com"
		const gRepo = "infra/infra"

		pm := pmMock{}
		p := New(ct.TQDispatcher, ct.GFake.Factory(), updater.New(ct.TQDispatcher, ct.GFake.Factory(), &pm, nil), &pm)

		changes := []int64{1, 2, 3, 4, 5}
		const notYetSaved = 4

		var knownIDs common.CLIDs
		for _, i := range changes {
			if i == notYetSaved {
				continue
			}
			cl, err := changelist.MustGobID(gHost, i).GetOrInsert(ctx, func(cl *changelist.CL) {
				// in practice, cl.Snapshot would be populated, but for this test it
				// doesn't matter.
			})
			So(err, ShouldBeNil)
			knownIDs = append(knownIDs, cl.ID)
		}
		sort.Sort(knownIDs)

		err := p.scheduleRefreshTasks(ctx, lProject, gHost, changes)
		So(err, ShouldBeNil)

		// PM must be notified immediately on CLs already saved.
		ids := pm.projects[lProject]
		sort.Sort(ids)
		So(ids, ShouldResemble, knownIDs)

		// CL Updater must have scheduled tasks.
		tasks := ct.TQ.Tasks().SortByETA()
		So(tasks, ShouldHaveLength, len(changes))
		// Tasks must be somewhat distributed in time.
		mid := ct.Clock.Now().Add(fullPollInterval / 2)
		So(tasks[1].ETA, ShouldHappenBefore, mid)
		So(tasks[len(tasks)-2].ETA, ShouldHappenAfter, mid)
		// For not yet saved CL, PM must be forcefully notified.
		var forced []int64
		for _, task := range tasks {
			p := task.Payload.(*updater.RefreshGerritCL)
			if p.GetForceNotify() {
				forced = append(forced, p.GetChange())
			}
		}
		So(forced, ShouldResemble, []int64{notYetSaved})
	})
}

func TestPoller(t *testing.T) {
	t.Parallel()

	Convey("Polling & task scheduling works", t, func() {
		// TODO(tandrii); de-couple this test from how CL updater works.
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "chromium"
		const gHost = "chromium-review.example.com"
		const gRepo = "infra/infra"

		mustGetState := func(lProject string) *State {
			st := &State{LuciProject: lProject}
			So(datastore.Get(ctx, st), ShouldBeNil)
			return st
		}
		execTooLatePoll := func(ctx context.Context) {
			beforeReqs := ct.GFake.Requests()
			beforeState := mustGetState(lProject)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
			afterReqs := ct.GFake.Requests()
			afterState := mustGetState(lProject)

			So(afterReqs, ShouldHaveLength, len(beforeReqs))
			So(afterState.EVersion, ShouldEqual, beforeState.EVersion)
		}

		pm := pmMock{}
		p := New(ct.TQDispatcher, ct.GFake.Factory(), updater.New(ct.TQDispatcher, ct.GFake.Factory(), &pm, nil), &pm)

		Convey("without project config, it's a noop", func() {
			So(p.Poke(ctx, lProject), ShouldBeNil)
			So(pt.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})
			ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
			So(ct.TQ.Tasks().Payloads(), ShouldBeEmpty)
			So(datastore.Get(ctx, &State{LuciProject: lProject}), ShouldEqual, datastore.ErrNoSuchEntity)
		})

		Convey("with existing project config, establishes task chain", func() {
			prjcfgtest.Create(ctx, lProject, singleRepoConfig(gHost, gRepo))
			So(p.Poke(ctx, lProject), ShouldBeNil)
			// Execute next poll task, which should result in full poll.
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
			st := mustGetState(lProject)
			So(st.EVersion, ShouldEqual, 1)
			fullPollStamp := timestamppb.New(ct.Clock.Now())
			So(st.SubPollers.GetSubPollers(), ShouldResembleProto, []*SubPoller{{
				Host:         gHost,
				OrProjects:   []string{gRepo},
				LastFullTime: fullPollStamp,
			}})
			So(watchedBy(ctx, gHost, gRepo, "refs/heads/main").HasOnlyProject(lProject), ShouldBeTrue)

			// Ensure follow up task has been created.
			So(pt.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})

			Convey("with CLs", func() {
				getCL := func(host string, change int) *changelist.CL {
					return getCL(ctx, host, change)
				}
				ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(),
					gf.CI(31, gf.CQ(+2), gf.Project(gRepo), gf.Updated(ct.Clock.Now())),
					// This CL was updated about the same time as last poll. It should be
					// discovered during next incremental poll.
					gf.CI(32, gf.CQ(+1), gf.Project(gRepo), gf.Updated(ct.Clock.Now().Add(time.Second))),
					// This suddently appearing CL won't be discovered until next full poll.
					gf.CI(33, gf.CQ(+2), gf.Project(gRepo), gf.Updated(ct.Clock.Now().Add(-time.Hour))),

					// No CQ+1 or CQ+2 vote yet.
					gf.CI(34, gf.Project(gRepo)),

					// Wrong repo.
					gf.CI(41, gf.CQ(+2), gf.Project("not-matched"), gf.Updated(ct.Clock.Now())),
				))

				Convey("performs incremental polls", func() {
					// Execute next poll task, it should be incremental.
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
					st = mustGetState(lProject)
					So(st.SubPollers.GetSubPollers(), ShouldResembleProto, []*SubPoller{{
						Host:         gHost,
						OrProjects:   []string{gRepo},
						LastFullTime: fullPollStamp,
						LastIncrTime: timestamppb.New(ct.Clock.Now()),
						Changes:      []int64{31, 32},
					}})
					// TQ tasks.
					So(pt.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})
					So(updatertest.ChangeNumbers(ct.TQ.Tasks()), ShouldResemble, []int64{31, 32})

					// Run all tasks.
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
					// Due to CL update task de-dup, regardless of what next incremental
					// poll discovered, there shouldn't be more CL update tasks.
					So(pt.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})
					So(updatertest.PFilter(ct.TQ.Tasks()), ShouldBeEmpty)
					// But there should be 2 CLs populated in Datastore.
					So(getCL(gHost, 31), ShouldNotBeNil)
					So(getCL(gHost, 32), ShouldNotBeNil)
					So(getCL(gHost, 33), ShouldBeNil)
					So(getCL(gHost, 34), ShouldBeNil)

					Convey("and the full poll happens every fullPollInterval and notifies PM on all changes", func() {
						So(fullPollInterval, ShouldBeGreaterThan, 2*pollInterval)
						ct.Clock.Add(fullPollInterval - 2*pollInterval)
						execTooLatePoll(ctx)

						// Update 2 changes in the mean time.
						ct.GFake.MutateChange(gHost, 34, func(c *gf.Change) {
							gf.Updated(ct.Clock.Now())(c.Info)
							gf.CQ(+2)(c.Info)
						})
						ct.GFake.MutateChange(gHost, 32, func(c *gf.Change) {
							gf.Updated(ct.Clock.Now())(c.Info)
						})

						ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
						st = mustGetState(lProject)
						So(st.SubPollers.GetSubPollers(), ShouldResembleProto, []*SubPoller{{
							Host:         gHost,
							OrProjects:   []string{gRepo},
							LastFullTime: timestamppb.New(ct.Clock.Now()),
							Changes:      []int64{31, 32, 33, 34},
						}})

						So(pt.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})
						So(updatertest.ChangeNumbers(ct.TQ.Tasks()), ShouldResemble, []int64{31, 32, 33, 34})
						// 33 has ForceNotifyPM=true. Such task isn't de-dupe-able.
						uTask33 := updatertest.PFilter(ct.TQ.Tasks()).SortByChangeNumber()[2]
						So(uTask33.GetForceNotify(), ShouldBeTrue)

						// And PM is notified only 31 and 32 for now.
						So(pm.popNotifiedCLs(lProject), ShouldResemble, sortedCLIDsOf(ctx, gHost, 31, 32))

						ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
						So(getCL(gHost, 33), ShouldNotBeNil)
						So(getCL(gHost, 34), ShouldNotBeNil)
						So(getCL(gHost, 32).EVersion, ShouldEqual, 2) // was updated.

						Convey("next full poll also notifies PM", func() {
							ct.Clock.Add(fullPollInterval)
							execTooLatePoll(ctx)
							ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
							So(mustGetState(lProject).SubPollers.GetSubPollers()[0].GetLastFullTime().AsTime(), ShouldResemble, ct.Clock.Now().UTC())

							So(pt.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})
							// PM must be notified on all 31..34.
							So(pm.popNotifiedCLs(lProject), ShouldResemble, sortedCLIDsOf(ctx, gHost, 31, 32, 33, 34))
							So(updatertest.ChangeNumbers(ct.TQ.Tasks()), ShouldResemble, []int64{31, 32, 33, 34})
							So(updatertest.PFilter(ct.TQ.Tasks())[0].GetForceNotify(), ShouldBeFalse)
						})

						Convey("full poll schedules tasks for no longer found changes", func() {
							ct.Clock.Add(fullPollInterval)
							execTooLatePoll(ctx)

							ct.GFake.MutateChange(gHost, 33, func(c *gf.Change) {
								gf.Updated(ct.Clock.Now())(c.Info)
								c.Info.Labels = nil // no more CQ vote.
							})
							ct.GFake.MutateChange(gHost, 34, func(c *gf.Change) {
								gf.Updated(ct.Clock.Now())(c.Info)
								gf.Status(gerritpb.ChangeStatus_MERGED)(c.Info)
							})

							ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
							So(mustGetState(lProject).SubPollers.GetSubPollers(), ShouldResembleProto, []*SubPoller{
								{
									Host:         gHost,
									OrProjects:   []string{gRepo},
									LastFullTime: timestamppb.New(ct.Clock.Now()),
									LastIncrTime: nil,
									Changes:      []int64{31, 32},
								},
							})

							// 1x Poll + 1x PM task + 4x CL refresh.
							So(pt.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})
							// PM must be still notified on all 31..34.
							So(pm.popNotifiedCLs(lProject), ShouldResemble, sortedCLIDsOf(ctx, gHost, 31, 32, 33, 34))
							So(updatertest.ChangeNumbers(ct.TQ.Tasks()), ShouldResemble, []int64{31, 32, 33, 34})
							// 33 and 34 have 1 final refresh task without updateHint.
							for _, p := range updatertest.PFilter(ct.TQ.Tasks())[2:] {
								So(p.GetForceNotify(), ShouldBeFalse)
								So(p.GetUpdatedHint(), ShouldBeNil)
							}
						})
					})
				})
			})

			Convey("notices updated config, updates SubPollers state", func() {
				before := mustGetState(lProject)
				before.SubPollers.SubPollers[0].Changes = []int64{31, 32}
				So(datastore.Put(ctx, before), ShouldBeNil)

				repos := append(sharedPrefixRepos("shared", minReposPerPrefixQuery+10), gRepo)
				prjcfgtest.Update(ctx, lProject, singleRepoConfig(gHost, repos...))
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
				after := mustGetState(lProject)
				So(after.ConfigHash, ShouldNotEqual, before.ConfigHash)
				So(after.SubPollers.GetSubPollers(), ShouldResembleProto, []*SubPoller{
					{
						Host:                gHost,
						CommonProjectPrefix: "shared",
						LastFullTime:        timestamppb.New(ct.Clock.Now()),
					},
					{
						// Re-used SubPoller state.
						Host:         gHost,
						OrProjects:   []string{gRepo},
						LastFullTime: fullPollStamp,                   // same as before
						LastIncrTime: timestamppb.New(ct.Clock.Now()), // new incremental poll
						Changes:      []int64{31, 32},                 // same as before
					},
				})
				So(watchedBy(ctx, gHost, "shared/001", "refs/heads/main").
					HasOnlyProject(lProject), ShouldBeTrue)

				Convey("if SubPollers state can't be re-used, schedules CL update events", func() {
					prjcfgtest.Update(ctx, lProject, singleRepoConfig(gHost, "another/repo"))
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
					after := mustGetState(lProject)
					So(after.SubPollers.GetSubPollers(), ShouldResembleProto, []*SubPoller{
						{
							Host:         gHost,
							OrProjects:   []string{"another/repo"},
							LastFullTime: timestamppb.New(ct.Clock.Now()),
						},
					})

					So(pt.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})
					// PM can't be notified directly, because CL31 and CL32 are not in
					// Datastore yet (unlikely, but possible if Gerrit updater TQ is
					// stalled).
					So(pm.projects, ShouldBeEmpty)
					So(updatertest.ChangeNumbers(ct.TQ.Tasks()), ShouldResemble, []int64{31, 32})
					for _, p := range updatertest.PFilter(ct.TQ.Tasks()).SortByChangeNumber() {
						So(p.GetForceNotify(), ShouldBeTrue)
					}
				})
			})

			Convey("disabled project => remove poller state & stop task chain", func() {
				prjcfgtest.Disable(ctx, lProject)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
				So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 0)
				So(datastore.Get(ctx, &State{LuciProject: lProject}), ShouldEqual, datastore.ErrNoSuchEntity)
				ensureNotWatched(ctx, gHost, gRepo, "refs/heads/main")
				ensureNotWatched(ctx, gHost, "shared/001", "refs/heads/main")
			})

			Convey("deleted => remove poller state & stop task chain", func() {
				prjcfgtest.Delete(ctx, lProject)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
				So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 0)
				So(datastore.Get(ctx, &State{LuciProject: lProject}), ShouldEqual, datastore.ErrNoSuchEntity)
				ensureNotWatched(ctx, gHost, gRepo, "refs/heads/main")
				ensureNotWatched(ctx, gHost, "shared/001", "refs/heads/main")
			})
		})
	})
}

func watchedBy(ctx context.Context, gHost, gRepo, ref string) *changelist.ApplicableConfig {
	a, err := gobmap.Lookup(ctx, gHost, gRepo, ref)
	So(err, ShouldBeNil)
	return a
}

func ensureNotWatched(ctx context.Context, gHost, gRepo, ref string) {
	a := watchedBy(ctx, gHost, gRepo, ref)
	So(a.GetProjects(), ShouldBeEmpty)
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

// NotifyCLUpdated implements updater.PM.
func (p *pmMock) NotifyCLUpdated(ctx context.Context, project string, cl common.CLID, eversion int) error {
	if p.projects == nil {
		p.projects = make(map[string]common.CLIDs, 1)
	}
	p.projects[project] = append(p.projects[project], cl)
	return nil
}

// NotifyCLsUpdated implements PM.
func (p *pmMock) NotifyCLsUpdated(ctx context.Context, luciProject string, cls []*changelist.CL) error {
	for _, cl := range cls {
		p.NotifyCLUpdated(ctx, luciProject, cl.ID, cl.EVersion)
	}
	return nil
}

func (p *pmMock) popNotifiedCLs(luciProject string) common.CLIDs {
	res := p.projects[luciProject]
	delete(p.projects, luciProject)
	return sortedCLIDs(res...)
}

func getCL(ctx context.Context, gHost string, change int) *changelist.CL {
	eid, err := changelist.GobID(gHost, int64(change))
	So(err, ShouldBeNil)
	cl, err := eid.Get(ctx)
	if err == datastore.ErrNoSuchEntity {
		return nil
	}
	So(err, ShouldBeNil)
	return cl
}

func sortedCLIDsOf(ctx context.Context, gHost string, changes ...int) common.CLIDs {
	ids := make([]common.CLID, len(changes))
	for i, c := range changes {
		ids[i] = getCL(ctx, gHost, c).ID
	}
	return sortedCLIDs(ids...)
}

func sortedCLIDs(ids ...common.CLID) common.CLIDs {
	res := common.CLIDs(ids)
	res.Dedupe() // it also sorts as a by-product.
	return res
}
