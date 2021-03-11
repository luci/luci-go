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

package clpurger

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/servicecfg"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPurgeCL(t *testing.T) {
	t.Parallel()

	Convey("PurgeCL works", t, func() {
		ct := cvtesting.Test{AppID: "cv"}
		ctx, cancel := ct.SetUp()
		defer cancel()
		const lProject = "lprj"
		const gHost = "x-review"
		const gRepo = "repo"
		const change = 43

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

		ct.Cfg.Create(ctx, lProject, makeConfig(gHost, gRepo))
		So(gobmap.Update(ctx, lProject), ShouldBeNil)

		// Fake 1 CL in gerrit & import it to Datastore.
		ci := gf.CI(
			change, gf.PS(2), gf.Project(gRepo), gf.Ref("refs/heads/main"),
			gf.CQ(+2, ct.Clock.Now().Add(-2*time.Minute), gf.U("user-1")),
			gf.Updated(ct.Clock.Now().Add(-1*time.Minute)),
		)
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), ci))

		refreshCL := func() {
			So(updater.Refresh(ctx, &updater.RefreshGerritCL{
				LuciProject: lProject,
				Host:        gHost,
				Change:      change,
			}), ShouldBeNil)
		}
		refreshCL()

		loadCL := func() *changelist.CL {
			cl, err := changelist.MustGobID(gHost, change).Get(ctx)
			So(err, ShouldBeNil)
			return cl
		}
		clBefore := loadCL()

		assertPMNotified := func(op string) {
			pmtest.AssertInEventbox(ctx, lProject, &prjpb.Event{Event: &prjpb.Event_PurgeCompleted{
				PurgeCompleted: &prjpb.PurgeCompleted{
					OperationId: op,
				},
			}})
		}

		// Basic task.
		task := &prjpb.PurgeCLTask{
			LuciProject: lProject,
			PurgingCl: &prjpb.PurgingCL{
				OperationId: "op",
				Clid:        int64(clBefore.ID),
				Deadline:    timestamppb.New(ct.Clock.Now().Add(10 * time.Minute)),
			},
			Trigger: trigger.Find(ci),
			Reason: &prjpb.PurgeCLTask_Reason{
				Reason: &prjpb.PurgeCLTask_Reason_OwnerLacksEmail{
					OwnerLacksEmail: true,
				},
			},
		}
		So(task.Trigger, ShouldNotBeNil)

		ct.Clock.Add(time.Minute)

		Convey("Happy path: cancel trigger, refresh CL, and notify PM", func() {
			So(Schedule(ctx, task), ShouldBeNil)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeCLTaskClass))

			clAfter := loadCL()
			So(clAfter.EVersion, ShouldBeGreaterThan, clBefore.EVersion)
			So(trigger.Find(clAfter.Snapshot.GetGerrit().GetInfo()), ShouldBeNil)
			So(lastMessageOf(clAfter), ShouldContainSubstring, "owner doesn't have a preferred email")
			assertPMNotified("op")

			Convey("Idempotent: if TQ task is retried, just notify PM", func() {
				// Use different Operation ID s.t. we can easily assert PM was notified
				// the 2nd time.
				task.PurgingCl.OperationId = "op-2"
				So(Schedule(ctx, task), ShouldBeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeCLTaskClass))
				So(loadCL().EVersion, ShouldEqual, clAfter.EVersion)
				assertPMNotified("op-2")
				So(pmtest.LatestETAof(ct.TQ.Tasks(), lProject), ShouldHappenBefore, ct.Clock.Now().Add(2*time.Second))
			})
		})

		Convey("Even if no purging is done, PM is always notified", func() {
			Convey("Task arrives after the deadline", func() {
				task.PurgingCl.Deadline = timestamppb.New(ct.Clock.Now().Add(-time.Minute))
				So(Schedule(ctx, task), ShouldBeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeCLTaskClass))
				So(loadCL().EVersion, ShouldEqual, clBefore.EVersion) // no changes.
				assertPMNotified("op")
				So(pmtest.LatestETAof(ct.TQ.Tasks(), lProject), ShouldHappenBefore, ct.Clock.Now().Add(2*time.Second))
			})

			Convey("Trigger is no longer matching latest CL Snapshot", func() {
				// Simulate old trigger for CQ+1, while snapshot contains CQ+2.
				gf.CQ(+1, ct.Clock.Now().Add(-time.Hour), gf.U("user-1"))(ci)
				task.Trigger = trigger.Find(ci)

				So(Schedule(ctx, task), ShouldBeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeCLTaskClass))
				So(loadCL().EVersion, ShouldEqual, clBefore.EVersion) // no changes.
				assertPMNotified("op")
				// The PM task should be ASAP.
				So(pmtest.LatestETAof(ct.TQ.Tasks(), lProject), ShouldHappenBefore, ct.Clock.Now().Add(2*time.Second))
			})

			Convey("Doesn't purge if CV isn't managing Runs for the project", func() {
				settings.UseCvRuns.ProjectRegexpExclude = []string{lProject}
				So(servicecfg.SetTestMigrationConfig(ctx, settings), ShouldBeNil)

				So(Schedule(ctx, task), ShouldBeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeCLTaskClass))
				So(loadCL().EVersion, ShouldEqual, clBefore.EVersion) // no changes.
				assertPMNotified("op")
				// Should create PM task with ETA ~1 minute later.
				So(pmtest.LatestETAof(ct.TQ.Tasks(), lProject), ShouldHappenAfter, ct.Clock.Now().Add(time.Minute-2*time.Second))
			})
		})
	})
}

func makeConfig(gHost string, gRepo string) *cfgpb.Config {
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

func lastMessageOf(cl *changelist.CL) string {
	msgs := cl.Snapshot.GetGerrit().GetInfo().GetMessages()
	if len(msgs) == 0 {
		panic("no messages")
	}
	return msgs[0].GetMessage()
}
