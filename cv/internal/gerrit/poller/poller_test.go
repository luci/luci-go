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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/config"

	. "github.com/smartystreets/goconvey/convey"
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
		const project = "chromium"
		ctx, tclock := testclock.UseTime(context.Background(),
			testclock.TestRecentTimeLocal.Truncate(pollInterval))
		ctx, tqScheduler := tq.TestingContext(ctx, nil)

		Convey("schedule works", func() {
			So(schedule(ctx, project, time.Time{}), ShouldBeNil)
			So(tqScheduler.Tasks().Payloads(), ShouldHaveLength, 1)

			first := tqScheduler.Tasks().Payloads()[0].(*PollGerritTask)
			So(first.GetLuciProject(), ShouldEqual, project)
			firstETA := first.GetEta().AsTime()
			So(firstETA.UnixNano(), ShouldBeBetweenOrEqual,
				tclock.Now().UnixNano(), tclock.Now().Add(pollInterval).UnixNano())

			Convey("idempotency via task deduplication", func() {
				So(schedule(ctx, project, time.Time{}), ShouldBeNil)
				So(tqScheduler.Tasks().Payloads(), ShouldHaveLength, 1)

				Convey("but only for the same project", func() {
					So(schedule(ctx, "another project", time.Time{}), ShouldBeNil)
					So(tqScheduler.Tasks().Payloads(), ShouldHaveLength, 2)
					So(tqScheduler.Tasks().Payloads()[1].(*PollGerritTask).GetLuciProject(), ShouldEqual,
						"another project")
				})
			})

			Convey("schedule next poll", func() {
				So(schedule(ctx, project, firstETA), ShouldBeNil)
				So(tqScheduler.Tasks().Payloads(), ShouldHaveLength, 2)
				So(tqScheduler.Tasks().Payloads()[1].(*PollGerritTask).GetEta().AsTime(),
					ShouldEqual, firstETA.Add(pollInterval))

				Convey("from a delayed prior poll", func() {
					tclock.Set(firstETA.Add(pollInterval).Add(pollInterval / 2))
					So(schedule(ctx, project, firstETA), ShouldBeNil)
					So(tqScheduler.Tasks().Payloads(), ShouldHaveLength, 3)
					So(tqScheduler.Tasks().Payloads()[2].(*PollGerritTask).GetEta().AsTime(),
						ShouldEqual, firstETA.Add(2*pollInterval))
				})
			})
		})
	})
}

func TestPoller(t *testing.T) {
	t.Parallel()

	Convey("Polling & task scheduling works", t, func() {
		// TODO(tandrii): turn this into a re-usable test setup across CV.

		// 10s failsafe for broken tests. Change it to 1ms when debugging.
		topCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer func() {
			So(topCtx.Err(), ShouldBeNil)
			cancel()
		}()

		const lProject = "chromium"
		const gHost = "chromium-review.example.com"
		const gRepo = "infra/inra"
		epoch := testclock.TestRecentTimeLocal
		ctx, tclock := testclock.UseTime(topCtx, epoch)
		tclock.SetTimerCallback(func(dur time.Duration, _ clock.Timer) {
			// Move fake time forward whenever someone's waiting for it.
			tclock.Add(dur)
		})

		ctx = memory.Use(ctx)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		if testing.Verbose() {
			ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
		}
		ctx, tqScheduler := tq.TestingContext(ctx, nil)
		testcfg := config.TestController{}

		mustGetState := func(lProject string) *state {
			st := &state{LuciProject: lProject}
			So(datastore.Get(ctx, st), ShouldBeNil)
			return st
		}

		Convey("without project config, it's a noop", func() {
			So(Poke(ctx, lProject), ShouldBeNil)
			So(tqScheduler.Tasks().Payloads(), ShouldHaveLength, 1)
			tqScheduler.Run(ctx, tqtesting.StopWhenDrained())
			So(tqScheduler.Tasks().Payloads(), ShouldHaveLength, 0)
			So(datastore.Get(ctx, &state{LuciProject: lProject}), ShouldEqual, datastore.ErrNoSuchEntity)
		})

		Convey("with existing project config, establishes task chain", func() {
			testcfg.Create(ctx, lProject, singleRepoConfig(gHost, gRepo))
			So(Poke(ctx, lProject), ShouldBeNil)
			for i := 0; i < 3; i++ {
				// Execute next poll task.
				tqScheduler.Run(ctx, tqtesting.StopAfterTask("poll-gerrit-task"))
				// Ensure follow up task has been created.
				So(tqScheduler.Tasks().Payloads(), ShouldHaveLength, 1)
				So(mustGetState(lProject).EVersion, ShouldEqual, i+1)
				// TODO(tandrii): assert subpollers state changed.
			}

			Convey("disabled project => remove poller state & stop task chain", func() {
				testcfg.Disable(ctx, lProject)
				tqScheduler.Run(ctx, tqtesting.StopAfterTask("poll-gerrit-task"))
				So(tqScheduler.Tasks().Payloads(), ShouldHaveLength, 0)
				So(datastore.Get(ctx, &state{LuciProject: lProject}), ShouldEqual, datastore.ErrNoSuchEntity)
			})

			Convey("notices updated config", func() {
				before := mustGetState(lProject)
				testcfg.Update(ctx, lProject, singleRepoConfig(gHost, gRepo+"/subrepo"))
				tqScheduler.Run(ctx, tqtesting.StopAfterTask("poll-gerrit-task"))
				after := mustGetState(lProject)
				So(after.ConfigHash, ShouldNotEqual, before.ConfigHash)
				// TODO(tandrii): assert subpollers state changed.
			})
		})
	})
}

func singleRepoConfig(gHost, gRepo string) *cfgpb.Config {
	return &cfgpb.Config{
		ConfigGroups: []*cfgpb.ConfigGroup{
			{
				Name: "main",
				Gerrit: []*cfgpb.ConfigGroup_Gerrit{
					{
						Url: "https://" + gHost + "/",
						Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
							{
								Name:      gRepo,
								RefRegexp: []string{"refs/heads/main"},
							},
						},
					},
				},
			},
		},
	}
}
