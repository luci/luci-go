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
	// . "go.chromium.org/luci/common/testing/assertions"
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
		mockCI(change1)
		mockCI(change2)
		mockCI(change3, gf.CQ(2, now.Add(-1*time.Minute), voter))

		cfg := makeConfig(gHost, gRepo)
		prjcfgtest.Create(ctx, project, cfg)
		_ = prjcfgtest.MustExist(ctx, project)
		gobmaptest.Update(ctx, project)

		// Run the real CL Updater for realistic CL Snapshot in datastore.
		pmNotifier := prjmanager.NewNotifier(ct.TQDispatcher)
		tjNotifier := tryjob.NewNotifier(ct.TQDispatcher)
		clMutator := changelist.NewMutator(ct.TQDispatcher, pmNotifier, nil, tjNotifier)
		clUpdater := changelist.NewUpdater(ct.TQDispatcher, clMutator)
		gerritupdater.RegisterUpdater(clUpdater, ct.GFactory())
		triggerer := New(pmNotifier, ct.GFactory())
		refreshCL := func() {
			for _, change := range []int64{change1, change2, change3} {
				So(clUpdater.TestingForceUpdate(ctx, &changelist.UpdateCLTask{
					LuciProject: project,
					ExternalId:  string(changelist.MustGobID(gHost, change)),
				}), ShouldBeNil)
			}
		}
		refreshCL()
		loadCL := func(change int64) *changelist.CL {
			cl, err := changelist.MustGobID(gHost, change).Load(ctx)
			So(err, ShouldBeNil)
			So(cl, ShouldNotBeNil)
			return cl
		}
		schedule := func(origin int64, changes ...int64) *prjpb.TriggeringCLsTask {
			task := &prjpb.TriggeringCLsTask{LuciProject: project}
			dl := timestamppb.New(now.Add(prjpb.MaxTriggeringCLsDuration))
			task.TriggeringCls = nil
			originCL := loadCL(origin)
			for _, change := range changes {
				cl := loadCL(change)
				task.TriggeringCls = append(task.TriggeringCls, &prjpb.TriggeringCL{
					Clid:        int64(cl.ID),
					OriginClid:  int64(originCL.ID),
					OperationId: fmt.Sprintf("cl-%d-123", change),
					Deadline:    dl,
					Trigger: &run.Trigger{
						Email:           voter,
						Mode:            string(run.FullRun),
						GerritAccountId: voterAccountID,
					},
				})
			}

			So(datastore.RunInTransaction(ctx, func(tctx context.Context) error {
				return triggerer.Schedule(tctx, task)
			}, nil), ShouldBeNil)
			return task
		}
		findCQVoteTrigger := func(info *gerritpb.ChangeInfo) *run.Trigger {
			trs := trigger.Find(&trigger.FindInput{ChangeInfo: info})
			if trs == nil {
				return nil
			}
			return trs.CqVoteTrigger
		}
		assertPMNotified := func(completed *prjpb.TriggeringCLsCompleted) {
			pmtest.AssertInEventbox(ctx, project, &prjpb.Event{
				Event: &prjpb.Event_TriggeringClsCompleted{
					TriggeringClsCompleted: completed,
				},
			})
		}
		task := schedule(
			change3,          // origin
			change1, change2, // changes to vote,
		)

		Convey("Votes", func() {
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.TriggerProjectCLTaskClass))
			So(ct.FailedTQTasks, ShouldHaveLength, 0)

			// change1 should have a CQ vote now by the voter,
			ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
			v1 := findCQVoteTrigger(ci1)
			So(v1.GetMode(), ShouldEqual, string(run.FullRun))
			So(v1.GetGerritAccountId(), ShouldEqual, voterAccountID)
			expectedMsg := "Triggering %s, because %s is triggered on %s"
			So(ci1, gf.ShouldLastMessageContain,
				fmt.Sprintf(expectedMsg, v1.GetMode(), v1.GetMode(), loadCL(change3).ExternalID.MustURL()))

			// So change2 does.
			ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
			v2 := findCQVoteTrigger(ci2)
			So(v2.GetMode(), ShouldEqual, string(run.FullRun))
			So(v2.GetGerritAccountId(), ShouldEqual, voterAccountID)
			So(ci1, gf.ShouldLastMessageContain,
				fmt.Sprintf(expectedMsg, v2.GetMode(), v2.GetMode(), loadCL(change3).ExternalID.MustURL()))
		})
		Convey("skips processing TriggeringCL", func() {
			Convey("if deadline exceeded", func() {
				ct.Clock.Add(prjpb.MaxTriggeringCLsDuration + time.Minute)
			})
			Convey("if already have a CQ+2 vote", func() {
				// remove the CQ vote from CL3.
				ct.GFake.MutateChange(gHost, change3, func(c *gf.Change) {
					gf.CQ(0, ct.Clock.Now(), voter)(c.Info)
					gf.Updated(ct.Clock.Now())(c.Info)
				})
				ct.Clock.Add(time.Minute)
				refreshCL()
			})

			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.TriggerProjectCLTaskClass))
			So(ct.FailedTQTasks, ShouldHaveLength, 0)
			// change1 and change2 should NOT have a CQ vote.
			ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
			So(findCQVoteTrigger(ci1), ShouldBeNil)
			ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
			So(findCQVoteTrigger(ci2), ShouldBeNil)

			assertPMNotified(&prjpb.TriggeringCLsCompleted{
				Succeeded: nil,
				Failed:    nil,
				Skipped: []*prjpb.TriggeringCLsCompleted_OpResult{
					{
						OperationId: task.TriggeringCls[0].GetOperationId(),
						OriginClid:  task.TriggeringCls[0].GetOriginClid(),
					},
					{
						OperationId: task.TriggeringCls[1].GetOperationId(),
						OriginClid:  task.TriggeringCls[1].GetOriginClid(),
					},
				},
			})
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
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.TriggerProjectCLTaskClass))
			So(ct.FailedTQTasks, ShouldHaveLength, 0)

			// change1 and change2 should still have a CQ vote, but
			// no SetReviewRequest(s) should have been sent.
			ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
			So(findCQVoteTrigger(ci1).GetMode(), ShouldEqual, string(run.FullRun))
			ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
			So(findCQVoteTrigger(ci2).GetMode(), ShouldEqual, string(run.FullRun))
			assertPMNotified(&prjpb.TriggeringCLsCompleted{
				Succeeded: []*prjpb.TriggeringCLsCompleted_OpResult{
					{
						OperationId: task.TriggeringCls[0].GetOperationId(),
						OriginClid:  task.TriggeringCls[0].GetOriginClid(),
					},
					{
						OperationId: task.TriggeringCls[1].GetOperationId(),
						OriginClid:  task.TriggeringCls[1].GetOriginClid(),
					},
				},
				Failed:  nil,
				Skipped: nil,
			})
			for _, req := range ct.GFake.Requests() {
				_, ok := req.(*gerritpb.SetReviewRequest)
				So(ok, ShouldBeFalse)
			}
		})
		Convey("overrides CQ+1", func() {
			// if a dep has a CQ+1, overrides it.
			ct.GFake.MutateChange(gHost, change1, func(c *gf.Change) {
				gf.CQ(1, ct.Clock.Now(), voter)(c.Info)
				gf.Updated(ct.Clock.Now())(c.Info)
			})
			ct.Clock.Add(time.Minute)
			refreshCL()
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.TriggerProjectCLTaskClass))
			So(ct.FailedTQTasks, ShouldHaveLength, 0)
			ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
			So(findCQVoteTrigger(ci1).GetMode(), ShouldEqual, string(run.FullRun))
			ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
			So(findCQVoteTrigger(ci2).GetMode(), ShouldEqual, string(run.FullRun))
			assertPMNotified(&prjpb.TriggeringCLsCompleted{
				Succeeded: []*prjpb.TriggeringCLsCompleted_OpResult{
					{
						OperationId: task.TriggeringCls[0].GetOperationId(),
						OriginClid:  task.TriggeringCls[0].GetOriginClid(),
					},
					{
						OperationId: task.TriggeringCls[1].GetOperationId(),
						OriginClid:  task.TriggeringCls[1].GetOriginClid(),
					},
				},
				Failed:  nil,
				Skipped: nil,
			})
		})
		Convey("handle permanent errors", func() {
			ct.GFake.MutateChange(gHost, change1, func(c *gf.Change) {
				c.ACLs = gf.ACLPublic()
			})
			ct.Clock.Add(time.Minute)
			refreshCL()
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.TriggerProjectCLTaskClass))
			So(ct.FailedTQTasks, ShouldHaveLength, 0)
			ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
			So(findCQVoteTrigger(ci1), ShouldBeNil)
			ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
			So(findCQVoteTrigger(ci2), ShouldBeNil)
			assertPMNotified(&prjpb.TriggeringCLsCompleted{
				Succeeded: nil,
				Failed: []*prjpb.TriggeringCLsCompleted_OpResult{
					{
						OperationId: task.TriggeringCls[0].GetOperationId(),
						OriginClid:  task.TriggeringCls[0].GetOriginClid(),
						Reason: &changelist.CLError_TriggerDeps{
							PermissionDenied: []*changelist.CLError_TriggerDeps_PermissionDenied{{
								Clid:  task.TriggeringCls[0].GetClid(),
								Email: voter,
							}},
						},
					},
				},
				Skipped: []*prjpb.TriggeringCLsCompleted_OpResult{
					{
						OperationId: task.TriggeringCls[1].GetOperationId(),
						OriginClid:  task.TriggeringCls[1].GetOriginClid(),
					},
				},
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
