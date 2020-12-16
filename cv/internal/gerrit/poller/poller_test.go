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

	timestamppb "google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/config"

	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
	pt "go.chromium.org/luci/cv/internal/gerrit/poller/pollertest"
	"go.chromium.org/luci/cv/internal/gerrit/poller/task"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestProjectOffset(t *testing.T) {
	t.Parallel()

	Convey("projectOffset forms uniformish distribution", t, func() {

		testIntervalOf100x := func(d time.Duration) {
			Convey((100 * d).String(), func() {
				offsets := make([]time.Duration, 101)
				for i := 0; i < 101; i++ {
					project := fmt.Sprintf("project-%d", i*i)
					offsets[i] = projectOffset(project, 100*d)
				}
				sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
				So(offsets[0], ShouldBeGreaterThanOrEqualTo, time.Duration(0))
				for i, o := range offsets {
					min := time.Duration(i-10) * d
					max := time.Duration(i+10) * d
					So(o, ShouldBeBetweenOrEqual, min, max)
				}
				So(offsets[100], ShouldBeLessThan, 100*d)
			})
		}

		testIntervalOf100x(time.Nanosecond)
		testIntervalOf100x(time.Millisecond)
		testIntervalOf100x(10 * time.Millisecond)
		testIntervalOf100x(100 * time.Millisecond)
		testIntervalOf100x(time.Second)
		testIntervalOf100x(time.Minute)
		testIntervalOf100x(time.Hour)
		testIntervalOf100x(7 * 24 * time.Hour)
	})
}

func TestSchedule(t *testing.T) {
	t.Parallel()

	Convey("schedule works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		ct.Clock.Set(ct.Clock.Now().Truncate(pollInterval).Add(pollInterval))
		const project = "chromium"

		Convey("schedule works", func() {
			So(schedule(ctx, project, time.Time{}), ShouldBeNil)
			payloads := pt.PFilter(ct.TQ.Tasks())
			So(payloads, ShouldHaveLength, 1)
			first := payloads[0]
			So(first.GetLuciProject(), ShouldEqual, project)
			firstETA := first.GetEta().AsTime()
			So(firstETA.UnixNano(), ShouldBeBetweenOrEqual,
				ct.Clock.Now().UnixNano(), ct.Clock.Now().Add(pollInterval).UnixNano())

			Convey("idempotency via task deduplication", func() {
				So(schedule(ctx, project, time.Time{}), ShouldBeNil)
				So(pt.PFilter(ct.TQ.Tasks()), ShouldHaveLength, 1)

				Convey("but only for the same project", func() {
					So(schedule(ctx, "another project", time.Time{}), ShouldBeNil)
					So(pt.Projects(ct.TQ.Tasks()), ShouldResemble, []string{
						project, "another project"})
				})
			})

			Convey("schedule next poll", func() {
				So(schedule(ctx, project, firstETA), ShouldBeNil)
				payloads := pt.PFilter(ct.TQ.Tasks().SortByETA())
				So(payloads, ShouldHaveLength, 2)
				So(payloads[1].GetEta().AsTime(), ShouldEqual, firstETA.Add(pollInterval))

				Convey("from a delayed prior poll", func() {
					ct.Clock.Set(firstETA.Add(pollInterval).Add(pollInterval / 2))
					So(schedule(ctx, project, firstETA), ShouldBeNil)
					payloads := pt.PFilter(ct.TQ.Tasks().SortByETA())
					So(payloads, ShouldHaveLength, 3)
					So(payloads[2].GetEta().AsTime(), ShouldEqual, firstETA.Add(2*pollInterval))
				})
			})
		})
	})
}

func TestPartitionConfig(t *testing.T) {
	t.Parallel()

	Convey("partitionConfig works", t, func() {

		Convey("groups by prefix if possible", func() {
			// makeCfgs merges several projects configs into one just to re-use
			// singleRepoConfig.
			makeCfgs := func(cfgs ...*cfgpb.Config) (ret []*config.ConfigGroup) {
				for _, cfg := range cfgs {
					for _, cg := range cfg.GetConfigGroups() {
						ret = append(ret, &config.ConfigGroup{Content: cg})
					}
				}
				return
			}
			cgs := makeCfgs(singleRepoConfig("h1", "infra/222", "infra/111"))
			So(partitionConfig(cgs), ShouldResembleProto, []*SubPoller{
				{Host: "h1", OrProjects: []string{"infra/111", "infra/222"}},
			})

			cgs = append(cgs, makeCfgs(singleRepoConfig("h1", sharedPrefixRepos("infra", 30)...))...)
			So(partitionConfig(cgs), ShouldResembleProto, []*SubPoller{
				{Host: "h1", CommonProjectPrefix: "infra"},
			})
			cgs = append(cgs, makeCfgs(singleRepoConfig("h2", "infra/499", "infra/132"))...)
			So(partitionConfig(cgs), ShouldResembleProto, []*SubPoller{
				{Host: "h1", CommonProjectPrefix: "infra"},
				{Host: "h2", OrProjects: []string{"infra/132", "infra/499"}},
			})
		})

		Convey("evenly distributes repos among SubPollers", func() {
			So(minReposPerPrefixQuery, ShouldBeGreaterThan, 5)
			repos := stringset.New(23)
			repos.AddAll(sharedPrefixRepos("a", 5))
			repos.AddAll(sharedPrefixRepos("b", 5))
			repos.AddAll(sharedPrefixRepos("c", 3))
			repos.AddAll(sharedPrefixRepos("d", 5))
			repos.AddAll(sharedPrefixRepos("e", 5))
			subpollers := partitionHostRepos(
				"host",
				repos.ToSlice(), // effectively shuffles repos
				7,               // at most 7 per query.
			)
			So(subpollers, ShouldHaveLength, 4) // 7*3 < 23 < 7*4

			for _, sp := range subpollers {
				// Ensure each has 5..6 repos instead max of 7.
				So(len(sp.GetOrProjects()), ShouldBeBetweenOrEqual, 5, 6)
				So(sort.StringsAreSorted(sp.GetOrProjects()), ShouldBeTrue)
				repos.DelAll(sp.GetOrProjects())
			}

			// Ensure no overlaps or missed repos.
			So(repos.ToSortedSlice(), ShouldResemble, []string{})
		})
	})
}

func TestPoller(t *testing.T) {
	t.Parallel()

	Convey("Polling & task scheduling works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "chromium"
		const gHost = "chromium-review.example.com"
		const gRepo = "infra/infra"

		mustGetState := func(lProject string) *state {
			st := &state{LuciProject: lProject}
			So(datastore.Get(ctx, st), ShouldBeNil)
			return st
		}

		Convey("without project config, it's a noop", func() {
			So(Poke(ctx, lProject), ShouldBeNil)
			So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 1)
			ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
			So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 0)
			So(datastore.Get(ctx, &state{LuciProject: lProject}), ShouldEqual, datastore.ErrNoSuchEntity)
		})

		Convey("with existing project config, establishes task chain", func() {
			ct.Cfg.Create(ctx, lProject, singleRepoConfig(gHost, gRepo))
			So(Poke(ctx, lProject), ShouldBeNil)
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
			// Ensure follow up task has been created.
			So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 1)
			So(watchedBy(ctx, gHost, gRepo, "refs/heads/main").
				HasOnlyProject(lProject), ShouldBeTrue)

			Convey("with CLs", func() {
				getCL := func(host string, change int) *changelist.CL {
					eid, err := changelist.GobID(host, int64(change))
					So(err, ShouldBeNil)
					cl, err := eid.Get(ctx)
					if err == datastore.ErrNoSuchEntity {
						return nil
					}
					So(err, ShouldBeNil)
					return cl
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
					}})
					// 1 for the future poll + 2 immediate to update CLs 31, 32.
					So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 1+2)
					// Run all tasks.
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
					// Due to CL update task de-dup, regardless of what next incremental
					// poll discovered, there shouldn't more CL update tasks.
					So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 1)
					// But there should be 2 CLs.
					So(getCL(gHost, 31), ShouldNotBeNil)
					So(getCL(gHost, 32), ShouldNotBeNil)
					So(getCL(gHost, 33), ShouldBeNil)
					So(getCL(gHost, 34), ShouldBeNil)

					Convey("and full polls every fullPollInterval", func() {
						So(fullPollInterval, ShouldBeGreaterThan, 2*pollInterval)
						ct.Clock.Add(fullPollInterval - 2*pollInterval)
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
						}})
						// 1 for the future poll + 3 for CL for 33 and 34 and again 32.
						So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 4)
						ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
						So(getCL(gHost, 33), ShouldNotBeNil)
						So(getCL(gHost, 34), ShouldNotBeNil)
						So(getCL(gHost, 32).EVersion, ShouldEqual, 2) // was updated.
					})
				})
			})

			Convey("notices updated config and resets SubPollers state", func() {
				before := mustGetState(lProject)
				repos := append(sharedPrefixRepos("shared", minReposPerPrefixQuery+10), gRepo)
				ct.Cfg.Update(ctx, lProject, singleRepoConfig(gHost, repos...))
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
						Host:         gHost,
						OrProjects:   []string{gRepo},
						LastFullTime: timestamppb.New(ct.Clock.Now()),
					},
				})
				So(watchedBy(ctx, gHost, "shared/001", "refs/heads/main").
					HasOnlyProject(lProject), ShouldBeTrue)
			})

			Convey("disabled project => remove poller state & stop task chain", func() {
				ct.Cfg.Disable(ctx, lProject)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
				So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 0)
				So(datastore.Get(ctx, &state{LuciProject: lProject}), ShouldEqual, datastore.ErrNoSuchEntity)
				ensureNotWatched(ctx, gHost, gRepo, "refs/heads/main")
				ensureNotWatched(ctx, gHost, "shared/001", "refs/heads/main")
			})

			Convey("deleted => remove poller state & stop task chain", func() {
				ct.Cfg.Delete(ctx, lProject)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(task.ClassID))
				So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 0)
				So(datastore.Get(ctx, &state{LuciProject: lProject}), ShouldEqual, datastore.ErrNoSuchEntity)
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
