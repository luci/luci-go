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
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

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
	"go.chromium.org/luci/cv/internal/tryjob/tjcancel"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPurgeCL(t *testing.T) {
	t.Parallel()

	Convey("PurgeCL works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()
		ctx, pmDispatcher := pmtest.MockDispatch(ctx)

		pmNotifier := prjmanager.NewNotifier(ct.TQDispatcher)
		tjNotifier := tryjob.NewNotifier(ct.TQDispatcher)
		_ = tjcancel.NewCancellator(tjNotifier)
		clMutator := changelist.NewMutator(ct.TQDispatcher, pmNotifier, nil, tjNotifier)
		fakeCLUpdater := clUpdaterMock{}
		purger := New(pmNotifier, ct.GFactory(), &fakeCLUpdater, clMutator)

		const lProject = "lprj"
		const gHost = "x-review"
		const gRepo = "repo"
		const change = 43

		cfg := makeConfig(gHost, gRepo)
		prjcfgtest.Create(ctx, lProject, cfg)
		cfgMeta := prjcfgtest.MustExist(ctx, lProject)
		gobmaptest.Update(ctx, lProject)

		// Fake 1 CL in gerrit & import it to Datastore.
		ci := gf.CI(
			change, gf.PS(2), gf.Project(gRepo), gf.Ref("refs/heads/main"),
			gf.CQ(+2, ct.Clock.Now().Add(-2*time.Minute), gf.U("user-1")),
			gf.Updated(ct.Clock.Now().Add(-1*time.Minute)),
		)
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), ci))

		// The real CL Updater for realistic CL Snapshot in datastore.
		clUpdater := changelist.NewUpdater(ct.TQDispatcher, clMutator)
		gerritupdater.RegisterUpdater(clUpdater, ct.GFactory())
		refreshCL := func() {
			So(clUpdater.TestingForceUpdate(ctx, &changelist.UpdateCLTask{
				LuciProject: lProject,
				ExternalId:  string(changelist.MustGobID(gHost, change)),
			}), ShouldBeNil)
		}
		refreshCL()

		loadCL := func() *changelist.CL {
			cl, err := changelist.MustGobID(gHost, change).Load(ctx)
			So(err, ShouldBeNil)
			So(cl, ShouldNotBeNil)
			return cl
		}
		clBefore := loadCL()

		assertPMNotified := func(purgingCL *prjpb.PurgingCL) {
			pmtest.AssertInEventbox(ctx, lProject, &prjpb.Event{Event: &prjpb.Event_PurgeCompleted{
				PurgeCompleted: &prjpb.PurgeCompleted{
					OperationId: purgingCL.GetOperationId(),
					Clid:        purgingCL.GetClid(),
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
				ApplyTo:     &prjpb.PurgingCL_AllActiveTriggers{AllActiveTriggers: true},
			},
			PurgeReasons: []*prjpb.PurgeReason{{
				ClError: &changelist.CLError{
					Kind: &changelist.CLError_OwnerLacksEmail{OwnerLacksEmail: true},
				},
				ApplyTo: &prjpb.PurgeReason_AllActiveTriggers{AllActiveTriggers: true},
			}},
			ConfigGroups: []string{string(cfgMeta.ConfigGroupIDs[0])},
		}

		schedule := func() error {
			return datastore.RunInTransaction(ctx, func(tCtx context.Context) error {
				return purger.Schedule(tCtx, task)
			}, nil)
		}

		ct.Clock.Add(time.Minute)

		Convey("Purge one trigger, then the other", func() {
			task.PurgeReasons = []*prjpb.PurgeReason{
				{
					ClError: &changelist.CLError{
						Kind: &changelist.CLError_InvalidDeps_{
							InvalidDeps: &changelist.CLError_InvalidDeps{
								Unwatched: []*changelist.Dep{
									{Clid: 1, Kind: changelist.DepKind_HARD},
								},
							},
						},
					},
					ApplyTo: &prjpb.PurgeReason_Triggers{
						Triggers: trigger.Find(&trigger.FindInput{ChangeInfo: ci, ConfigGroup: &cfgpb.ConfigGroup{}}),
					},
				},
			}
			So(schedule(), ShouldBeNil)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))

			ciAfter := ct.GFake.GetChange(gHost, change).Info
			triggersAfter := trigger.Find(&trigger.FindInput{
				ChangeInfo:                   ciAfter,
				ConfigGroup:                  cfg.GetConfigGroups()[0],
				TriggerNewPatchsetRunAfterPS: loadCL().TriggerNewPatchsetRunAfterPS,
			})
			So(triggersAfter, ShouldNotBeNil)
			So(triggersAfter.CqVoteTrigger, ShouldBeNil)
			So(triggersAfter.NewPatchsetRunTrigger, ShouldNotBeNil)
			So(ciAfter, gf.ShouldLastMessageContain, "its deps are not watched")
			So(fakeCLUpdater.scheduledTasks, ShouldHaveLength, 1)
			assertPMNotified(task.PurgingCl)

			task.PurgeReasons = []*prjpb.PurgeReason{
				{
					ClError: &changelist.CLError{
						Kind: &changelist.CLError_UnsupportedMode{
							UnsupportedMode: string(run.NewPatchsetRun)},
					},
					ApplyTo: &prjpb.PurgeReason_Triggers{
						Triggers: &run.Triggers{
							NewPatchsetRunTrigger: triggersAfter.GetNewPatchsetRunTrigger(),
						},
					},
				},
			}
			So(schedule(), ShouldBeNil)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))

			ciAfter = ct.GFake.GetChange(gHost, change).Info
			triggersAfter = trigger.Find(&trigger.FindInput{
				ChangeInfo:                   ciAfter,
				ConfigGroup:                  cfg.GetConfigGroups()[0],
				TriggerNewPatchsetRunAfterPS: loadCL().TriggerNewPatchsetRunAfterPS,
			})
			So(triggersAfter, ShouldBeNil)
			So(ciAfter, gf.ShouldLastMessageContain, "is not supported")
			So(fakeCLUpdater.scheduledTasks, ShouldHaveLength, 2)
			assertPMNotified(task.PurgingCl)
		})
		Convey("Happy path: reset both triggers, schedule CL refresh, and notify PM", func() {
			So(schedule(), ShouldBeNil)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))

			ciAfter := ct.GFake.GetChange(gHost, change).Info
			So(trigger.Find(&trigger.FindInput{
				ChangeInfo:                   ciAfter,
				ConfigGroup:                  cfg.GetConfigGroups()[0],
				TriggerNewPatchsetRunAfterPS: loadCL().TriggerNewPatchsetRunAfterPS,
			}), ShouldBeNil)
			So(ciAfter, gf.ShouldLastMessageContain, "owner doesn't have a preferred email")

			So(fakeCLUpdater.scheduledTasks, ShouldHaveLength, 1)
			assertPMNotified(task.PurgingCl)
			So(loadCL().Snapshot.GetOutdated(), ShouldNotBeNil)

			Convey("Idempotent: if TQ task is retried, just notify PM", func() {
				verifyIdempotency := func() {
					// Use different Operation ID s.t. we can easily assert PM was notified
					// the 2nd time.
					task.PurgingCl.OperationId = "op-2"
					So(schedule(), ShouldBeNil)
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))
					// CL in Gerrit shouldn't be changed.
					ciAfter2 := ct.GFake.GetChange(gHost, change).Info
					So(ciAfter2, ShouldResembleProto, ciAfter)
					// But PM must be notified.
					assertPMNotified(task.PurgingCl)
					So(pmDispatcher.LatestETAof(lProject), ShouldHappenBefore, ct.Clock.Now().Add(2*time.Second))
				}
				// Idempotency must not rely on CL being updated between retries.
				Convey("CL updated between retries", func() {
					verifyIdempotency()
					// should remain with the same value in Outdated.
					So(loadCL().Snapshot.GetOutdated(), ShouldNotBeNil)
				})
				Convey("CL not updated between retries", func() {
					refreshCL()
					// Outdated should be nil after refresh().
					So(loadCL().Snapshot.GetOutdated(), ShouldBeNil)
					verifyIdempotency()
					// Idempotency should not make it outdated, because
					// no purge is performed actually.
					So(loadCL().Snapshot.GetOutdated(), ShouldBeNil)
				})
			})
		})

		Convey("Even if no purging is done, PM is always notified", func() {
			Convey("Task arrives after the deadline", func() {
				task.PurgingCl.Deadline = timestamppb.New(ct.Clock.Now().Add(-time.Minute))
				So(schedule(), ShouldBeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))
				So(loadCL().EVersion, ShouldEqual, clBefore.EVersion) // no changes.
				assertPMNotified(task.PurgingCl)
				So(pmDispatcher.LatestETAof(lProject), ShouldHappenBefore, ct.Clock.Now().Add(2*time.Second))
			})

			Convey("Trigger is no longer matching latest CL Snapshot", func() {
				// Simulate old trigger for CQ+1, while snapshot contains CQ+2.
				gf.CQ(+1, ct.Clock.Now().Add(-time.Hour), gf.U("user-1"))(ci)
				// Simulate NPR finished earlier.
				cl := loadCL()
				cl.TriggerNewPatchsetRunAfterPS = 2
				So(datastore.Put(ctx, cl), ShouldBeNil)

				So(schedule(), ShouldBeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))
				So(loadCL().EVersion, ShouldEqual, clBefore.EVersion+1) // +1 for setting Outdated{}
				assertPMNotified(task.PurgingCl)
				// The PM task should be ASAP.
				So(pmDispatcher.LatestETAof(lProject), ShouldHappenBefore, ct.Clock.Now().Add(2*time.Second))
			})
		})

		Convey("Sets Notify and AddToAttentionSet", func() {
			var reqs []*gerritpb.SetReviewRequest
			findSetReviewReqs := func() {
				for _, req := range ct.GFake.Requests() {
					if r, ok := req.(*gerritpb.SetReviewRequest); ok {
						reqs = append(reqs, r)
					}
				}
			}

			Convey("with the default NotifyTarget", func() {
				task.PurgingCl.Notification = nil
				So(schedule(), ShouldBeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))
				findSetReviewReqs()
				So(reqs, ShouldHaveLength, 2)
				postReq := reqs[0]
				voteReq := reqs[1]
				if reqs[1].GetMessage() != "" {
					postReq, voteReq = reqs[1], reqs[0]
				}
				So(postReq.Notify, ShouldEqual, gerritpb.Notify_NOTIFY_NONE)
				So(voteReq.Notify, ShouldEqual, gerritpb.Notify_NOTIFY_NONE)
				So(postReq.GetNotifyDetails(), ShouldNotBeNil)
				So(voteReq.GetNotifyDetails(), ShouldBeNil)
				So(postReq.GetAddToAttentionSet(), ShouldNotBeNil)
				So(voteReq.GetAddToAttentionSet(), ShouldBeNil)
			})
			Convey("with a custom Notification target", func() {
				// 0 implies nobody
				task.PurgingCl.Notification = NoNotification
				So(schedule(), ShouldBeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))
				findSetReviewReqs()
				So(reqs, ShouldHaveLength, 2)
				postReq := reqs[0]
				voteReq := reqs[1]
				if reqs[1].GetMessage() != "" {
					postReq, voteReq = reqs[1], reqs[0]
				}
				So(postReq.Notify, ShouldEqual, gerritpb.Notify_NOTIFY_NONE)
				So(voteReq.Notify, ShouldEqual, gerritpb.Notify_NOTIFY_NONE)
				So(postReq.GetNotifyDetails(), ShouldBeNil)
				So(voteReq.GetNotifyDetails(), ShouldBeNil)
				So(postReq.GetAddToAttentionSet(), ShouldBeNil)
				So(voteReq.GetAddToAttentionSet(), ShouldBeNil)
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
								Name:          "linter",
								ModeAllowlist: []string{string(run.NewPatchsetRun)},
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
