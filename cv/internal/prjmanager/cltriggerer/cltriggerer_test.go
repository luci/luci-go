// Copyright 2023 The LUCI Authors.
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

package cltriggerer

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"
	"google.golang.org/protobuf/types/known/timestamppb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap/gobmaptest"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	gerritupdater "go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTriggerer(t *testing.T) {
	t.Parallel()

	const (
		project        = "chromium"
		gHost          = "x-review"
		gRepo          = "repo"
		owner          = "owner@example.org"
		voter          = "voter@example.org"
		voterAccountID = 9978561

		// These are the Gerrit change nums, not the ID of change.CL{} entities.
		change1 = 12131
		change2 = 22312
		change3 = 32158
	)

	Convey("Triggerer", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()
		now := ct.Clock.Now()

		// Set mockup data with CLs and Project.
		mockCI := func(change int, mods ...gf.CIModifier) {
			mods = append(mods,
				gf.PS(1), gf.Project(gRepo),
				gf.Ref("refs/heads/main"), gf.Owner(owner),
			)
			ci := gf.CI(change, mods...)
			ct.GFake.CreateChange(&gf.Change{
				Host: gHost,
				ACLs: gf.ACLRestricted(project),
				Info: ci,
			})
		}

		cfg := makeConfig(gHost, gRepo)
		prjcfgtest.Create(ctx, project, cfg)
		_ = prjcfgtest.MustExist(ctx, project)
		gobmaptest.Update(ctx, project)

		// Run the real CL Updater for realistic CL Snapshot in datastore.
		pmNotifier := prjmanager.NewNotifier(ct.TQDispatcher)
		tjNotifier := tryjob.NewNotifier(ct.TQDispatcher)
		clMutator := changelist.NewMutator(ct.TQDispatcher, pmNotifier, nil, tjNotifier)

		// The real CL Updater for realistic CL Snapshot in datastore.
		clUpdater := changelist.NewUpdater(ct.TQDispatcher, clMutator)
		gerritupdater.RegisterUpdater(clUpdater, ct.GFactory())

		// triggerer with fakeCLUpdater so that tests can examine the scheduled
		// tasks.
		fakeCLUpdater := &clUpdaterMock{}
		triggerer := New(pmNotifier, ct.GFactory(), fakeCLUpdater, clMutator)

		// helper functions
		refreshCL := func() {
			for _, change := range []int64{change1, change2, change3} {
				So(clUpdater.TestingForceUpdate(ctx, &changelist.UpdateCLTask{
					LuciProject: project,
					ExternalId:  string(changelist.MustGobID(gHost, change)),
				}), ShouldBeNil)
			}
		}
		loadCL := func(change int64) *changelist.CL {
			cl, err := changelist.MustGobID(gHost, change).Load(ctx)
			So(err, ShouldBeNil)
			So(cl, ShouldNotBeNil)
			return cl
		}
		schedule := func(origin int64, deps ...int64) string {
			task := &prjpb.TriggeringCLDepsTask{LuciProject: project}
			dl := timestamppb.New(now.Add(prjpb.MaxTriggeringCLDepsDuration))
			originCL := loadCL(origin)
			task.TriggeringClDeps = &prjpb.TriggeringCLDeps{
				OriginClid:  int64(originCL.ID),
				OperationId: fmt.Sprintf("cl-%d-123", origin),
				Deadline:    dl,
				Trigger: &run.Trigger{
					Email:           voter,
					Mode:            string(run.FullRun),
					GerritAccountId: voterAccountID,
				},
			}
			for _, dep := range deps {
				cl := loadCL(dep)
				task.TriggeringClDeps.DepClids = append(task.TriggeringClDeps.DepClids, int64(cl.ID))
			}

			So(datastore.RunInTransaction(ctx, func(tctx context.Context) error {
				return triggerer.Schedule(tctx, task)
			}, nil), ShouldBeNil)
			return task.GetTriggeringClDeps().GetOperationId()
		}
		findCQVoteTrigger := func(info *gerritpb.ChangeInfo) *run.Trigger {
			trs := trigger.Find(&trigger.FindInput{ChangeInfo: info})
			if trs == nil {
				return nil
			}
			return trs.CqVoteTrigger
		}
		assertPMNotified := func(evt *prjpb.TriggeringCLDepsCompleted) {
			pmtest.AssertInEventbox(ctx, project, &prjpb.Event{
				Event: &prjpb.Event_TriggeringClDepsCompleted{
					TriggeringClDepsCompleted: evt,
				},
			})
		}

		mockCI(change1)
		mockCI(change2)
		mockCI(change3, gf.CQ(2, now.Add(-1*time.Minute), voter))
		refreshCL()
		clid1 := int64(loadCL(change1).ID)
		clid2 := int64(loadCL(change2).ID)
		clid3 := int64(loadCL(change3).ID)

		opID := schedule(change3 /* origin */, change1, change2)

		Convey("votes deps", func() {
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.TriggerProjectCLDepsTaskClass))
			So(ct.FailedTQTasks, ShouldHaveLength, 0)

			Convey("schedules CL update tasks for deps", func() {
				var tasks []*changelist.UpdateCLTask
				tasks = append(tasks, fakeCLUpdater.scheduledTasks...)
				sort.Slice(tasks, func(i, j int) bool {
					return string(tasks[i].ExternalId) < string(tasks[j].ExternalId)
				})

				cl1, cl2 := loadCL(change1), loadCL(change2)
				So(tasks, ShouldHaveLength, 2)
				So(tasks[0], ShouldResembleProto, &changelist.UpdateCLTask{
					LuciProject: project,
					ExternalId:  string(cl1.ExternalID),
					Id:          int64(cl1.ID),
					Requester:   changelist.UpdateCLTask_DEP_CL_TRIGGERER,
				})
				So(tasks[1], ShouldResembleProto, &changelist.UpdateCLTask{
					LuciProject: project,
					ExternalId:  string(cl2.ExternalID),
					Id:          int64(cl2.ID),
					Requester:   changelist.UpdateCLTask_DEP_CL_TRIGGERER,
				})
			})

			// Verify that change 1 and 2 now have a CQ vote.
			expectedMsg := func(mode string) string {
				return fmt.Sprintf("Triggering %s, because %s is triggered on %s",
					mode, mode, loadCL(change3).ExternalID.MustURL())
			}

			ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
			v1 := findCQVoteTrigger(ci1)
			So(v1.GetMode(), ShouldEqual, string(run.FullRun))
			So(v1.GetGerritAccountId(), ShouldEqual, voterAccountID)
			So(ci1, gf.ShouldLastMessageContain, expectedMsg(v1.GetMode()))
			So(loadCL(change1).Snapshot.GetOutdated(), ShouldNotBeNil)

			// So change2 does.
			ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
			v2 := findCQVoteTrigger(ci2)
			So(v2.GetMode(), ShouldEqual, string(run.FullRun))
			So(v2.GetGerritAccountId(), ShouldEqual, voterAccountID)
			So(ci2, gf.ShouldLastMessageContain, expectedMsg(v2.GetMode()))
			So(loadCL(change2).Snapshot.GetOutdated(), ShouldNotBeNil)
		})

		Convey("skips voting, if deadline exceeded", func() {
			Convey("if deadline exceeded", func() {
				ct.Clock.Add(prjpb.MaxTriggeringCLDepsDuration + time.Minute)
			})
			Convey("if origin no longer has CQ+2", func() {
				ct.GFake.MutateChange(gHost, change3, func(c *gf.Change) {
					gf.CQ(0, ct.Clock.Now(), voter)(c.Info)
					gf.Updated(ct.Clock.Now())(c.Info)
				})
				ct.Clock.Add(time.Minute)
				refreshCL()
			})

			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.TriggerProjectCLDepsTaskClass))
			So(ct.FailedTQTasks, ShouldHaveLength, 0)
			ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
			So(findCQVoteTrigger(ci1), ShouldBeNil)
			So(loadCL(change1).Snapshot.GetOutdated(), ShouldBeNil)
			ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
			So(findCQVoteTrigger(ci2), ShouldBeNil)
			So(loadCL(change2).Snapshot.GetOutdated(), ShouldBeNil)

			assertPMNotified(&prjpb.TriggeringCLDepsCompleted{
				OperationId: opID,
				Origin:      clid3,
				Incompleted: []int64{clid1, clid2},
			})
			So(fakeCLUpdater.scheduledTasks, ShouldHaveLength, 0)
		})

		Convey("noop if already voted", func() {
			// This is the case where
			// - the deps didn't have CQ+2 at the time of triageDeps(), but
			// - the deps were triggered (manually?) before cltriggerer
			// processes the task.
			ct.GFake.MutateChange(gHost, change1, func(c *gf.Change) {
				gf.CQ(2, ct.Clock.Now(), voter)(c.Info)
				gf.Updated(ct.Clock.Now())(c.Info)
			})
			ct.GFake.MutateChange(gHost, change2, func(c *gf.Change) {
				gf.CQ(2, ct.Clock.Now(), voter)(c.Info)
				gf.Updated(ct.Clock.Now())(c.Info)
			})
			ct.Clock.Add(time.Minute)
			refreshCL()
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.TriggerProjectCLDepsTaskClass))
			So(ct.FailedTQTasks, ShouldHaveLength, 0)

			// change1 and change2 should still have a CQ vote, but
			// no SetReviewRequest(s) should have been sent.
			ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
			So(findCQVoteTrigger(ci1).GetMode(), ShouldEqual, string(run.FullRun))
			So(loadCL(change1).Snapshot.GetOutdated(), ShouldBeNil)
			ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
			So(findCQVoteTrigger(ci2).GetMode(), ShouldEqual, string(run.FullRun))
			So(loadCL(change2).Snapshot.GetOutdated(), ShouldBeNil)
			assertPMNotified(&prjpb.TriggeringCLDepsCompleted{
				OperationId: opID,
				Origin:      clid3,
				Succeeded:   []int64{clid1, clid2},
			})
			for _, req := range ct.GFake.Requests() {
				_, ok := req.(*gerritpb.SetReviewRequest)
				So(ok, ShouldBeFalse)
			}
			So(fakeCLUpdater.scheduledTasks, ShouldHaveLength, 0)
		})

		Convey("overrides CQ+1", func() {
			// if a dep has a CQ+1, overrides it.
			ct.GFake.MutateChange(gHost, change1, func(c *gf.Change) {
				gf.CQ(1, ct.Clock.Now(), voter)(c.Info)
				gf.Updated(ct.Clock.Now())(c.Info)
			})
			ct.Clock.Add(time.Minute)
			refreshCL()
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.TriggerProjectCLDepsTaskClass))
			So(ct.FailedTQTasks, ShouldHaveLength, 0)
			ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
			So(findCQVoteTrigger(ci1).GetMode(), ShouldEqual, string(run.FullRun))
			So(loadCL(change1).Snapshot.GetOutdated(), ShouldNotBeNil)
			ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
			So(findCQVoteTrigger(ci2).GetMode(), ShouldEqual, string(run.FullRun))
			So(loadCL(change2).Snapshot.GetOutdated(), ShouldNotBeNil)
			assertPMNotified(&prjpb.TriggeringCLDepsCompleted{
				OperationId: opID,
				Origin:      clid3,
				Succeeded:   []int64{clid1, clid2},
			})

			Convey("schedules CL update tasks for deps", func() {
				var tasks []*changelist.UpdateCLTask
				tasks = append(tasks, fakeCLUpdater.scheduledTasks...)
				sort.Slice(tasks, func(i, j int) bool {
					return string(tasks[i].ExternalId) < string(tasks[j].ExternalId)
				})

				cl1, cl2 := loadCL(change1), loadCL(change2)
				So(tasks, ShouldHaveLength, 2)
				So(tasks[0], ShouldResembleProto, &changelist.UpdateCLTask{
					LuciProject: project,
					ExternalId:  string(cl1.ExternalID),
					Id:          int64(cl1.ID),
					Requester:   changelist.UpdateCLTask_DEP_CL_TRIGGERER,
				})
				So(tasks[1], ShouldResembleProto, &changelist.UpdateCLTask{
					LuciProject: project,
					ExternalId:  string(cl2.ExternalID),
					Id:          int64(cl2.ID),
					Requester:   changelist.UpdateCLTask_DEP_CL_TRIGGERER,
				})
			})
		})
		Convey("handle permanent errors", func() {
			ct.GFake.MutateChange(gHost, change1, func(c *gf.Change) {
				c.ACLs = gf.ACLPublic()
			})
			ct.GFake.MutateChange(gHost, change2, func(c *gf.Change) {
				c.ACLs = gf.ACLPublic()
			})
			ct.Clock.Add(time.Minute)
			refreshCL()
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.TriggerProjectCLDepsTaskClass))
			So(ct.FailedTQTasks, ShouldHaveLength, 0)
			ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
			So(findCQVoteTrigger(ci1), ShouldBeNil)
			So(loadCL(change1).Snapshot.GetOutdated(), ShouldBeNil)
			ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
			So(findCQVoteTrigger(ci2), ShouldBeNil)
			So(loadCL(change2).Snapshot.GetOutdated(), ShouldBeNil)
			assertPMNotified(&prjpb.TriggeringCLDepsCompleted{
				OperationId: opID,
				Origin:      clid3,
				Failed: []*changelist.CLError_TriggerDeps{
					{
						PermissionDenied: []*changelist.CLError_TriggerDeps_PermissionDenied{{
							Clid:  clid1,
							Email: voter,
						}},
					},
					{
						PermissionDenied: []*changelist.CLError_TriggerDeps_PermissionDenied{{
							Clid:  clid2,
							Email: voter,
						}},
					},
				},
			})
			So(fakeCLUpdater.scheduledTasks, ShouldHaveLength, 0)
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
				Verifiers: &cfgpb.Verifiers{
					Tryjob: &cfgpb.Verifiers_Tryjob{
						Builders: []*cfgpb.Verifiers_Tryjob_Builder{
							{
								Name: "builder",
							},
						},
					},
				},
			},
		},
	}
}

type clUpdaterMock struct {
	scheduledTasks []*changelist.UpdateCLTask
}

func (c *clUpdaterMock) Schedule(_ context.Context, task *changelist.UpdateCLTask) error {
	c.scheduledTasks = append(c.scheduledTasks, task)
	return nil
}
