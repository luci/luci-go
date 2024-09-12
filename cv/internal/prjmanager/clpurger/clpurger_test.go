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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
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
)

func TestPurgeCL(t *testing.T) {
	t.Parallel()

	ftt.Run("PurgeCL works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
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
			assert.Loosely(t, clUpdater.TestingForceUpdate(ctx, &changelist.UpdateCLTask{
				LuciProject: lProject,
				ExternalId:  string(changelist.MustGobID(gHost, change)),
			}), should.BeNil)
		}
		refreshCL()

		loadCL := func() *changelist.CL {
			cl, err := changelist.MustGobID(gHost, change).Load(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cl, should.NotBeNil)
			return cl
		}
		clBefore := loadCL()

		assertPMNotified := func(t testing.TB, purgingCL *prjpb.PurgingCL) {
			t.Helper()
			pmtest.AssertInEventbox(t, ctx, lProject, &prjpb.Event{Event: &prjpb.Event_PurgeCompleted{
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

		t.Run("Purge one trigger, then the other", func(t *ftt.Test) {
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
			assert.Loosely(t, schedule(), should.BeNil)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))

			ciAfter := ct.GFake.GetChange(gHost, change).Info
			triggersAfter := trigger.Find(&trigger.FindInput{
				ChangeInfo:                   ciAfter,
				ConfigGroup:                  cfg.GetConfigGroups()[0],
				TriggerNewPatchsetRunAfterPS: loadCL().TriggerNewPatchsetRunAfterPS,
			})
			assert.Loosely(t, triggersAfter, should.NotBeNil)
			assert.Loosely(t, triggersAfter.CqVoteTrigger, should.BeNil)
			assert.Loosely(t, triggersAfter.NewPatchsetRunTrigger, should.NotBeNil)
			assert.Loosely(t, ciAfter, convey.Adapt(gf.ShouldLastMessageContain)("its deps are not watched"))
			assert.Loosely(t, fakeCLUpdater.scheduledTasks, should.HaveLength(1))
			assertPMNotified(t, task.PurgingCl)

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
			assert.Loosely(t, schedule(), should.BeNil)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))

			ciAfter = ct.GFake.GetChange(gHost, change).Info
			triggersAfter = trigger.Find(&trigger.FindInput{
				ChangeInfo:                   ciAfter,
				ConfigGroup:                  cfg.GetConfigGroups()[0],
				TriggerNewPatchsetRunAfterPS: loadCL().TriggerNewPatchsetRunAfterPS,
			})
			assert.Loosely(t, triggersAfter, should.BeNil)
			assert.Loosely(t, ciAfter, convey.Adapt(gf.ShouldLastMessageContain)("is not supported"))
			assert.Loosely(t, fakeCLUpdater.scheduledTasks, should.HaveLength(2))
			assertPMNotified(t, task.PurgingCl)
		})
		t.Run("Happy path: reset both triggers, schedule CL refresh, and notify PM", func(t *ftt.Test) {
			assert.Loosely(t, schedule(), should.BeNil)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))

			ciAfter := ct.GFake.GetChange(gHost, change).Info
			assert.Loosely(t, trigger.Find(&trigger.FindInput{
				ChangeInfo:                   ciAfter,
				ConfigGroup:                  cfg.GetConfigGroups()[0],
				TriggerNewPatchsetRunAfterPS: loadCL().TriggerNewPatchsetRunAfterPS,
			}), should.BeNil)
			assert.Loosely(t, ciAfter, convey.Adapt(gf.ShouldLastMessageContain)("owner doesn't have a preferred email"))

			assert.Loosely(t, fakeCLUpdater.scheduledTasks, should.HaveLength(1))
			assertPMNotified(t, task.PurgingCl)
			assert.Loosely(t, loadCL().Snapshot.GetOutdated(), should.NotBeNil)

			t.Run("Idempotent: if TQ task is retried, just notify PM", func(t *ftt.Test) {
				verifyIdempotency := func() {
					// Use different Operation ID s.t. we can easily assert PM was notified
					// the 2nd time.
					task.PurgingCl.OperationId = "op-2"
					assert.Loosely(t, schedule(), should.BeNil)
					ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))
					// CL in Gerrit shouldn't be changed.
					ciAfter2 := ct.GFake.GetChange(gHost, change).Info
					assert.Loosely(t, ciAfter2, should.Resemble(ciAfter))
					// But PM must be notified.
					assertPMNotified(t, task.PurgingCl)
					assert.Loosely(t, pmDispatcher.LatestETAof(lProject), should.HappenBefore(ct.Clock.Now().Add(2*time.Second)))
				}
				// Idempotency must not rely on CL being updated between retries.
				t.Run("CL updated between retries", func(t *ftt.Test) {
					verifyIdempotency()
					// should remain with the same value in Outdated.
					assert.Loosely(t, loadCL().Snapshot.GetOutdated(), should.NotBeNil)
				})
				t.Run("CL not updated between retries", func(t *ftt.Test) {
					refreshCL()
					// Outdated should be nil after refresh().
					assert.Loosely(t, loadCL().Snapshot.GetOutdated(), should.BeNil)
					verifyIdempotency()
					// Idempotency should not make it outdated, because
					// no purge is performed actually.
					assert.Loosely(t, loadCL().Snapshot.GetOutdated(), should.BeNil)
				})
			})
		})

		t.Run("Even if no purging is done, PM is always notified", func(t *ftt.Test) {
			t.Run("Task arrives after the deadline", func(t *ftt.Test) {
				task.PurgingCl.Deadline = timestamppb.New(ct.Clock.Now().Add(-time.Minute))
				assert.Loosely(t, schedule(), should.BeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))
				assert.Loosely(t, loadCL().EVersion, should.Equal(clBefore.EVersion)) // no changes.
				assertPMNotified(t, task.PurgingCl)
				assert.Loosely(t, pmDispatcher.LatestETAof(lProject), should.HappenBefore(ct.Clock.Now().Add(2*time.Second)))
			})

			t.Run("Trigger is no longer matching latest CL Snapshot", func(t *ftt.Test) {
				// Simulate old trigger for CQ+1, while snapshot contains CQ+2.
				gf.CQ(+1, ct.Clock.Now().Add(-time.Hour), gf.U("user-1"))(ci)
				// Simulate NPR finished earlier.
				cl := loadCL()
				cl.TriggerNewPatchsetRunAfterPS = 2
				assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)

				assert.Loosely(t, schedule(), should.BeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))
				assert.Loosely(t, loadCL().EVersion, should.Equal(clBefore.EVersion+1)) // +1 for setting Outdated{}
				assertPMNotified(t, task.PurgingCl)
				// The PM task should be ASAP.
				assert.Loosely(t, pmDispatcher.LatestETAof(lProject), should.HappenBefore(ct.Clock.Now().Add(2*time.Second)))
			})
		})

		t.Run("Sets Notify and AddToAttentionSet", func(t *ftt.Test) {
			var reqs []*gerritpb.SetReviewRequest
			findSetReviewReqs := func() {
				for _, req := range ct.GFake.Requests() {
					if r, ok := req.(*gerritpb.SetReviewRequest); ok {
						reqs = append(reqs, r)
					}
				}
			}

			t.Run("with the default NotifyTarget", func(t *ftt.Test) {
				task.PurgingCl.Notification = nil
				assert.Loosely(t, schedule(), should.BeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))
				findSetReviewReqs()
				assert.Loosely(t, reqs, should.HaveLength(2))
				postReq := reqs[0]
				voteReq := reqs[1]
				if reqs[1].GetMessage() != "" {
					postReq, voteReq = reqs[1], reqs[0]
				}
				assert.Loosely(t, postReq.Notify, should.Equal(gerritpb.Notify_NOTIFY_NONE))
				assert.Loosely(t, voteReq.Notify, should.Equal(gerritpb.Notify_NOTIFY_NONE))
				assert.Loosely(t, postReq.GetNotifyDetails(), should.NotBeNil)
				assert.Loosely(t, voteReq.GetNotifyDetails(), should.BeNil)
				assert.Loosely(t, postReq.GetAddToAttentionSet(), should.NotBeNil)
				assert.Loosely(t, voteReq.GetAddToAttentionSet(), should.BeNil)
			})
			t.Run("with a custom Notification target", func(t *ftt.Test) {
				// 0 implies nobody
				task.PurgingCl.Notification = NoNotification
				assert.Loosely(t, schedule(), should.BeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))
				findSetReviewReqs()
				assert.Loosely(t, reqs, should.HaveLength(2))
				postReq := reqs[0]
				voteReq := reqs[1]
				if reqs[1].GetMessage() != "" {
					postReq, voteReq = reqs[1], reqs[0]
				}
				assert.Loosely(t, postReq.Notify, should.Equal(gerritpb.Notify_NOTIFY_NONE))
				assert.Loosely(t, voteReq.Notify, should.Equal(gerritpb.Notify_NOTIFY_NONE))
				assert.Loosely(t, postReq.GetNotifyDetails(), should.BeNil)
				assert.Loosely(t, voteReq.GetNotifyDetails(), should.BeNil)
				assert.Loosely(t, postReq.GetAddToAttentionSet(), should.BeNil)
				assert.Loosely(t, voteReq.GetAddToAttentionSet(), should.BeNil)
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
