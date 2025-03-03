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
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon/distribution"
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
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestTriggerer(t *testing.T) {
	t.Parallel()

	const (
		project        = "chromium"
		gHost          = "x-review"
		gRepo          = "repo"
		cgName         = "cg1"
		owner          = "owner@example.org"
		voter          = "voter@example.org"
		voterAccountID = 9978561

		// These are the Gerrit change nums, not the ID of change.CL{} entities.
		change1 = 12131
		change2 = 22312
		change3 = 32158
	)

	ftt.Run("Triggerer", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
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
				assert.Loosely(t, clUpdater.TestingForceUpdate(ctx, &changelist.UpdateCLTask{
					LuciProject: project,
					ExternalId:  string(changelist.MustGobID(gHost, change)),
				}), should.BeNil)
			}
		}
		loadCL := func(change int64) *changelist.CL {
			cl, err := changelist.MustGobID(gHost, change).Load(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, cl, should.NotBeNil)
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
				ConfigGroupName: cgName,
			}
			for _, dep := range deps {
				cl := loadCL(dep)
				task.TriggeringClDeps.DepClids = append(task.TriggeringClDeps.DepClids, int64(cl.ID))
			}

			assert.Loosely(t, datastore.RunInTransaction(ctx, func(tctx context.Context) error {
				return triggerer.Schedule(tctx, task)
			}, nil), should.BeNil)
			return task.GetTriggeringClDeps().GetOperationId()
		}
		findCQVoteTrigger := func(info *gerritpb.ChangeInfo) *run.Trigger {
			trs := trigger.Find(&trigger.FindInput{ChangeInfo: info})
			if trs == nil {
				return nil
			}
			return trs.CqVoteTrigger
		}
		assertPMNotified := func(t testing.TB, evt *prjpb.TriggeringCLDepsCompleted) {
			t.Helper()
			pmtest.AssertInEventbox(t, ctx, project, &prjpb.Event{
				Event: &prjpb.Event_TriggeringClDepsCompleted{
					TriggeringClDepsCompleted: evt,
				},
			})
		}
		taskCounterMetric := func(fvs ...any) bool {
			_, exist := ct.TSMonSentValue(ctx, metrics.Internal.CLTriggererTaskCompleted, fvs...).(int64)
			return exist
		}
		taskDurationMetric := func(fvs ...any) bool {
			_, exist := ct.TSMonSentValue(ctx, metrics.Internal.CLTriggererTaskDuration, fvs...).(*distribution.Distribution)
			return exist
		}

		mockCI(change1)
		mockCI(change2)
		mockCI(change3, gf.CQ(2, now.Add(-1*time.Minute), voter))
		refreshCL()
		clid1 := int64(loadCL(change1).ID)
		clid2 := int64(loadCL(change2).ID)
		clid3 := int64(loadCL(change3).ID)

		opID := schedule(change3 /* origin */, change1, change2)

		t.Run("votes deps", func(t *ftt.Test) {
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.TriggerProjectCLDepsTaskClass))
			assert.Loosely(t, ct.FailedTQTasks, should.HaveLength(0))

			t.Run("schedules CL update tasks for deps", func(t *ftt.Test) {
				var tasks []*changelist.UpdateCLTask
				tasks = append(tasks, fakeCLUpdater.scheduledTasks...)
				sort.Slice(tasks, func(i, j int) bool {
					return string(tasks[i].ExternalId) < string(tasks[j].ExternalId)
				})

				cl1, cl2 := loadCL(change1), loadCL(change2)
				assert.Loosely(t, tasks, should.HaveLength(2))
				assert.That(t, tasks[0], should.Match(&changelist.UpdateCLTask{
					LuciProject: project,
					ExternalId:  string(cl1.ExternalID),
					Id:          int64(cl1.ID),
					Requester:   changelist.UpdateCLTask_DEP_CL_TRIGGERER,
				}))
				assert.That(t, tasks[1], should.Match(&changelist.UpdateCLTask{
					LuciProject: project,
					ExternalId:  string(cl2.ExternalID),
					Id:          int64(cl2.ID),
					Requester:   changelist.UpdateCLTask_DEP_CL_TRIGGERER,
				}))
			})

			// Verify that change 1 and 2 now have a CQ vote.
			expectedMsg := func(mode string) string {
				return fmt.Sprintf("Triggering %s, because %s is triggered on %s",
					mode, mode, loadCL(change3).ExternalID.MustURL())
			}

			ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
			v1 := findCQVoteTrigger(ci1)
			assert.Loosely(t, v1.GetMode(), should.Equal(string(run.FullRun)))
			assert.Loosely(t, v1.GetGerritAccountId(), should.Equal(voterAccountID))
			assert.Loosely(t, ci1, convey.Adapt(gf.ShouldLastMessageContain)(expectedMsg(v1.GetMode())))
			assert.Loosely(t, loadCL(change1).Snapshot.GetOutdated(), should.NotBeNil)

			// So change2 does.
			ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
			v2 := findCQVoteTrigger(ci2)
			assert.Loosely(t, v2.GetMode(), should.Equal(string(run.FullRun)))
			assert.Loosely(t, v2.GetGerritAccountId(), should.Equal(voterAccountID))
			assert.Loosely(t, ci2, convey.Adapt(gf.ShouldLastMessageContain)(expectedMsg(v2.GetMode())))
			assert.Loosely(t, loadCL(change2).Snapshot.GetOutdated(), should.NotBeNil)

			exist := taskCounterMetric(project, cgName, 2 /* nDeps */, "SUCCEEDED")
			assert.Loosely(t, exist, should.BeTrue)
			exist = taskDurationMetric(project, cgName, 2 /* nDeps */, "SUCCEEDED")
			assert.Loosely(t, exist, should.BeTrue)
		})

		t.Run("skips voting", func(t *ftt.Test) {
			var statusWant string
			check := func(t testing.TB) {
				t.Helper()

				ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.TriggerProjectCLDepsTaskClass))
				assert.Loosely(t, ct.FailedTQTasks, should.HaveLength(0), truth.LineContext())
				ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
				assert.Loosely(t, findCQVoteTrigger(ci1), should.BeNil, truth.LineContext())
				assert.Loosely(t, loadCL(change1).Snapshot.GetOutdated(), should.BeNil, truth.LineContext())
				ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
				assert.Loosely(t, findCQVoteTrigger(ci2), should.BeNil, truth.LineContext())
				assert.Loosely(t, loadCL(change2).Snapshot.GetOutdated(), should.BeNil, truth.LineContext())

				assertPMNotified(t, &prjpb.TriggeringCLDepsCompleted{
					OperationId: opID,
					Origin:      clid3,
					Incompleted: []int64{clid1, clid2},
				})
				assert.Loosely(t, fakeCLUpdater.scheduledTasks, should.HaveLength(0), truth.LineContext())
				exist := taskCounterMetric(project, cgName, 2 /* nDeps */, statusWant)
				assert.Loosely(t, exist, should.BeTrue, truth.LineContext())
				exist = taskDurationMetric(project, cgName, 2 /* nDeps */, statusWant)
				assert.Loosely(t, exist, should.BeTrue, truth.LineContext())
			}

			t.Run("if deadline exceeded", func(t *ftt.Test) {
				ct.Clock.Add(prjpb.MaxTriggeringCLDepsDuration + time.Minute)
				statusWant = "TIMEDOUT"
				check(t)
			})

			t.Run("if origin no longer has CQ+2", func(t *ftt.Test) {
				ct.GFake.MutateChange(gHost, change3, func(c *gf.Change) {
					gf.CQ(0, ct.Clock.Now(), voter)(c.Info)
					gf.Updated(ct.Clock.Now())(c.Info)
				})
				ct.Clock.Add(time.Minute)
				refreshCL()
				statusWant = "CANCELED"
				check(t)
			})

		})

		t.Run("noop if already voted", func(t *ftt.Test) {
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
			assert.Loosely(t, ct.FailedTQTasks, should.HaveLength(0))

			// change1 and change2 should still have a CQ vote, but
			// no SetReviewRequest(s) should have been sent.
			ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
			assert.Loosely(t, findCQVoteTrigger(ci1).GetMode(), should.Equal(string(run.FullRun)))
			assert.Loosely(t, loadCL(change1).Snapshot.GetOutdated(), should.BeNil)
			ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
			assert.Loosely(t, findCQVoteTrigger(ci2).GetMode(), should.Equal(string(run.FullRun)))
			assert.Loosely(t, loadCL(change2).Snapshot.GetOutdated(), should.BeNil)
			assertPMNotified(t, &prjpb.TriggeringCLDepsCompleted{
				OperationId: opID,
				Origin:      clid3,
				Succeeded:   []int64{clid1, clid2},
			})
			for _, req := range ct.GFake.Requests() {
				_, ok := req.(*gerritpb.SetReviewRequest)
				assert.Loosely(t, ok, should.BeFalse)
			}
			assert.Loosely(t, fakeCLUpdater.scheduledTasks, should.HaveLength(0))

			// triggerDepOp{} skips SetReview(), but it'd still report itself
			// as succeeded, and so metric does.
			exist := taskCounterMetric(project, cgName, 2 /* nDeps */, "SUCCEEDED")
			assert.Loosely(t, exist, should.BeTrue)
			exist = taskDurationMetric(project, cgName, 2 /* nDeps */, "SUCCEEDED")
			assert.Loosely(t, exist, should.BeTrue)
		})

		t.Run("overrides CQ+1", func(t *ftt.Test) {
			// if a dep has a CQ+1, overrides it.
			ct.GFake.MutateChange(gHost, change1, func(c *gf.Change) {
				gf.CQ(1, ct.Clock.Now(), voter)(c.Info)
				gf.Updated(ct.Clock.Now())(c.Info)
			})
			ct.Clock.Add(time.Minute)
			refreshCL()
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.TriggerProjectCLDepsTaskClass))
			assert.Loosely(t, ct.FailedTQTasks, should.HaveLength(0))
			ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
			assert.Loosely(t, findCQVoteTrigger(ci1).GetMode(), should.Equal(string(run.FullRun)))
			assert.Loosely(t, loadCL(change1).Snapshot.GetOutdated(), should.NotBeNil)
			ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
			assert.Loosely(t, findCQVoteTrigger(ci2).GetMode(), should.Equal(string(run.FullRun)))
			assert.Loosely(t, loadCL(change2).Snapshot.GetOutdated(), should.NotBeNil)
			assertPMNotified(t, &prjpb.TriggeringCLDepsCompleted{
				OperationId: opID,
				Origin:      clid3,
				Succeeded:   []int64{clid1, clid2},
			})
			exist := taskCounterMetric(project, cgName, 2 /* nDeps */, "SUCCEEDED")
			assert.Loosely(t, exist, should.BeTrue)
			exist = taskDurationMetric(project, cgName, 2 /* nDeps */, "SUCCEEDED")

			assert.Loosely(t, exist, should.BeTrue)

			t.Run("schedules CL update tasks for deps", func(t *ftt.Test) {
				var tasks []*changelist.UpdateCLTask
				tasks = append(tasks, fakeCLUpdater.scheduledTasks...)
				sort.Slice(tasks, func(i, j int) bool {
					return string(tasks[i].ExternalId) < string(tasks[j].ExternalId)
				})

				cl1, cl2 := loadCL(change1), loadCL(change2)
				assert.Loosely(t, tasks, should.HaveLength(2))
				assert.That(t, tasks[0], should.Match(&changelist.UpdateCLTask{
					LuciProject: project,
					ExternalId:  string(cl1.ExternalID),
					Id:          int64(cl1.ID),
					Requester:   changelist.UpdateCLTask_DEP_CL_TRIGGERER,
				}))
				assert.That(t, tasks[1], should.Match(&changelist.UpdateCLTask{
					LuciProject: project,
					ExternalId:  string(cl2.ExternalID),
					Id:          int64(cl2.ID),
					Requester:   changelist.UpdateCLTask_DEP_CL_TRIGGERER,
				}))
			})
		})
		t.Run("handle permanent errors", func(t *ftt.Test) {
			ct.GFake.MutateChange(gHost, change1, func(c *gf.Change) {
				c.ACLs = gf.ACLPublic()
			})
			ct.GFake.MutateChange(gHost, change2, func(c *gf.Change) {
				c.ACLs = gf.ACLPublic()
			})
			ct.Clock.Add(time.Minute)
			refreshCL()
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.TriggerProjectCLDepsTaskClass))
			assert.Loosely(t, ct.FailedTQTasks, should.HaveLength(0))
			ci1 := ct.GFake.GetChange(gHost, int(change1)).Info
			assert.Loosely(t, findCQVoteTrigger(ci1), should.BeNil)
			assert.Loosely(t, loadCL(change1).Snapshot.GetOutdated(), should.BeNil)
			ci2 := ct.GFake.GetChange(gHost, int(change2)).Info
			assert.Loosely(t, findCQVoteTrigger(ci2), should.BeNil)
			assert.Loosely(t, loadCL(change2).Snapshot.GetOutdated(), should.BeNil)
			assertPMNotified(t, &prjpb.TriggeringCLDepsCompleted{
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
			assert.Loosely(t, fakeCLUpdater.scheduledTasks, should.HaveLength(0))
			exist := taskCounterMetric(project, cgName, 2 /* nDeps */, "FAILED")
			assert.Loosely(t, exist, should.BeTrue)
			exist = taskDurationMetric(project, cgName, 2 /* nDeps */, "FAILED")
			assert.Loosely(t, exist, should.BeTrue)
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
	mu             sync.Mutex
}

func (c *clUpdaterMock) Schedule(_ context.Context, task *changelist.UpdateCLTask) error {
	c.mu.Lock()
	c.scheduledTasks = append(c.scheduledTasks, task)
	c.mu.Unlock()
	return nil
}
