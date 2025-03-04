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

package handler

import (
	"context"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/quota/quotapb"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	apipb "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/tree"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/bq"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/postaction"
	"go.chromium.org/luci/cv/internal/run/pubsub"
	"go.chromium.org/luci/cv/internal/run/rdb"
	"go.chromium.org/luci/cv/internal/run/runtest"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestEndRun(t *testing.T) {
	t.Parallel()

	ftt.Run("EndRun", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			clid     = 1
			lProject = "infra"
		)
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					PostActions: []*cfgpb.ConfigGroup_PostAction{
						{
							Name: "run-verification-label",
							Conditions: []*cfgpb.ConfigGroup_PostAction_TriggeringCondition{
								{
									Mode:     string(run.DryRun),
									Statuses: []apipb.Run_Status{apipb.Run_FAILED},
								},
							},
						},
					},
				},
			},
		})
		cgs, err := prjcfgtest.MustExist(ctx, lProject).GetConfigGroups(ctx)
		assert.NoErr(t, err)
		cg := cgs[0]

		// mock a CL with two onoging Runs.
		rids := common.RunIDs{
			common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef")),
			common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("cafecafe")),
		}
		sort.Sort(rids)
		cl := changelist.CL{
			ID:             clid,
			EVersion:       3,
			IncompleteRuns: rids,
			UpdateTime:     ct.Clock.Now().UTC(),
		}
		assert.NoErr(t, datastore.Put(ctx, &cl))

		// mock some child runs of rids[0]
		childRunID := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("child"))
		childRun := run.Run{
			ID:      childRunID,
			DepRuns: common.RunIDs{rids[0]},
		}
		finChildRunID := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("finchild"))
		finChildRun := run.Run{
			ID:      finChildRunID,
			DepRuns: common.RunIDs{rids[0]},
			Status:  run.Status_FAILED,
		}
		assert.NoErr(t, datastore.Put(ctx, &childRun, &finChildRun))

		rs := &state.RunState{
			Run: run.Run{
				ID:            rids[0],
				Status:        run.Status_RUNNING,
				ConfigGroupID: cg.ID,
				CreateTime:    ct.Clock.Now().Add(-2 * time.Minute),
				StartTime:     ct.Clock.Now().Add(-1 * time.Minute),
				Mode:          run.DryRun,
				CLs:           common.CLIDs{1},
				OngoingLongOps: &run.OngoingLongOps{
					Ops: map[string]*run.OngoingLongOps_Op{
						"11-22": {
							CancelRequested: false,
							Work:            &run.OngoingLongOps_Op_PostStartMessage{PostStartMessage: true},
						},
					},
				},
			},
		}

		impl, deps := makeImpl(&ct)
		se := impl.endRun(ctx, rs, run.Status_FAILED, cg, []*run.Run{&childRun, &finChildRun})
		assert.Loosely(t, rs.Status, should.Equal(run.Status_FAILED))
		assert.That(t, rs.EndTime, should.Match(ct.Clock.Now()))
		assert.NoErr(t, datastore.RunInTransaction(ctx, se, nil))

		t.Run("removeRunFromCLs", func(t *ftt.Test) {
			// fetch the updated CL entity.
			cl = changelist.CL{ID: clid}
			assert.NoErr(t, datastore.Get(ctx, &cl))

			// it should have removed the ended Run, but not the other
			// ongoing Run from the CL entity.
			assert.That(t, cl.IncompleteRuns, should.Match(common.RunIDs{rids[1]}))
			t.Run("schedule CLUpdate for the removed Run", func(t *ftt.Test) {
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(changelist.BatchOnCLUpdatedTaskClass))
				pmtest.AssertReceivedRunFinished(t, ctx, rids[0], rs.Status)
				pmtest.AssertReceivedCLsNotified(t, ctx, rids[0].LUCIProject(), []*changelist.CL{&cl})
				assert.That(t, deps.clUpdater.refreshedCLs, should.Match(common.MakeCLIDs(clid)))
			})
		})

		t.Run("child runs get ParentRunCompleted events.", func(t *ftt.Test) {
			runtest.AssertReceivedParentRunCompleted(t, ctx, childRunID)
			runtest.AssertNotReceivedParentRunCompleted(t, ctx, finChildRunID)
		})

		t.Run("cancel ongoing LongOps", func(t *ftt.Test) {
			assert.Loosely(t, rs.OngoingLongOps.GetOps()["11-22"].GetCancelRequested(), should.BeTrue)
		})

		t.Run("populate metrics for run events", func(t *ftt.Test) {
			fset1 := []any{
				lProject, "main", string(run.DryRun),
				apipb.Run_FAILED.String(), true,
			}
			fset2 := fset1[0 : len(fset1)-1]
			assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Public.RunEnded, fset1...), should.Equal(1))
			assert.Loosely(t, ct.TSMonSentDistr(ctx, metrics.Public.RunDuration, fset2...).Sum(),
				should.AlmostEqual((1 * time.Minute).Seconds()))
			assert.Loosely(t, ct.TSMonSentDistr(ctx, metrics.Public.RunTotalDuration, fset1...).Sum(),
				should.AlmostEqual((2 * time.Minute).Seconds()))
		})

		t.Run("publish RunEnded event", func(t *ftt.Test) {
			var task *pubsub.PublishRunEndedTask
			for _, tsk := range ct.TQ.Tasks() {
				if p, ok := tsk.Payload.(*pubsub.PublishRunEndedTask); ok {
					task = p
					break
				}
			}
			assert.That(t, task, should.Match(&pubsub.PublishRunEndedTask{
				PublicId:    rs.ID.PublicID(),
				LuciProject: rs.ID.LUCIProject(),
				Status:      rs.Status,
				Eversion:    int64(rs.EVersion + 1),
			}))
		})

		t.Run("enqueue long-ops for PostAction", func(t *ftt.Test) {
			postActions := make([]*run.OngoingLongOps_Op_ExecutePostActionPayload, 0, len(rs.OngoingLongOps.GetOps()))
			for _, op := range rs.OngoingLongOps.GetOps() {
				if act := op.GetExecutePostAction(); act != nil {
					d := timestamppb.New(ct.Clock.Now().UTC().Add(maxPostActionExecutionDuration))
					assert.That(t, op.GetDeadline(), should.Match(d))
					assert.Loosely(t, op.GetCancelRequested(), should.BeFalse)
					postActions = append(postActions, act)
				}
			}
			sort.Slice(postActions, func(i, j int) bool {
				return strings.Compare(postActions[i].GetName(), postActions[j].GetName()) < 0
			})

			assert.That(t, postActions, should.Match([]*run.OngoingLongOps_Op_ExecutePostActionPayload{
				{
					Name: postaction.CreditRunQuotaPostActionName,
					Kind: &run.OngoingLongOps_Op_ExecutePostActionPayload_CreditRunQuota_{
						CreditRunQuota: &run.OngoingLongOps_Op_ExecutePostActionPayload_CreditRunQuota{},
					},
				},
				{
					Name: "run-verification-label",
					Kind: &run.OngoingLongOps_Op_ExecutePostActionPayload_ConfigAction{
						ConfigAction: &cfgpb.ConfigGroup_PostAction{
							Name: "run-verification-label",
							Conditions: []*cfgpb.ConfigGroup_PostAction_TriggeringCondition{
								{
									Mode:     string(run.DryRun),
									Statuses: []apipb.Run_Status{apipb.Run_FAILED},
								},
							},
						},
					},
				},
			}))
		})
	})

	ftt.Run("EndRun when mode is NewPatchsetRun", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			clid     = 1
			lProject = "infra"
		)
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					PostActions: []*cfgpb.ConfigGroup_PostAction{
						{
							Name: "run-verification-label",
							Conditions: []*cfgpb.ConfigGroup_PostAction_TriggeringCondition{
								{
									Mode:     string(run.DryRun),
									Statuses: []apipb.Run_Status{apipb.Run_FAILED},
								},
							},
						},
					},
				},
			},
		})
		cgs, err := prjcfgtest.MustExist(ctx, lProject).GetConfigGroups(ctx)
		assert.NoErr(t, err)
		cg := cgs[0]

		// mock a CL with two onoging Runs.
		rids := common.RunIDs{
			common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef")),
			common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("cafecafe")),
		}
		sort.Sort(rids)
		cl := changelist.CL{
			ID:             clid,
			EVersion:       3,
			IncompleteRuns: rids,
			UpdateTime:     ct.Clock.Now().UTC(),
		}
		assert.NoErr(t, datastore.Put(ctx, &cl))

		// mock some child runs of rids[0]
		childRunID := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("child"))
		childRun := run.Run{
			ID:      childRunID,
			DepRuns: common.RunIDs{rids[0]},
		}
		finChildRunID := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("finchild"))
		finChildRun := run.Run{
			ID:      finChildRunID,
			DepRuns: common.RunIDs{rids[0]},
			Status:  run.Status_FAILED,
		}
		assert.NoErr(t, datastore.Put(ctx, &childRun, &finChildRun))

		rs := &state.RunState{
			Run: run.Run{
				ID:            rids[0],
				Status:        run.Status_RUNNING,
				ConfigGroupID: cg.ID,
				CreateTime:    ct.Clock.Now().Add(-2 * time.Minute),
				StartTime:     ct.Clock.Now().Add(-1 * time.Minute),
				Mode:          run.NewPatchsetRun,
				CLs:           common.CLIDs{1},
				OngoingLongOps: &run.OngoingLongOps{
					Ops: map[string]*run.OngoingLongOps_Op{
						"11-22": {
							CancelRequested: false,
							Work:            &run.OngoingLongOps_Op_PostStartMessage{PostStartMessage: true},
						},
					},
				},
			},
		}

		impl, _ := makeImpl(&ct)
		se := impl.endRun(ctx, rs, run.Status_FAILED, cg, []*run.Run{&childRun, &finChildRun})
		assert.Loosely(t, rs.Status, should.Equal(run.Status_FAILED))
		assert.That(t, rs.EndTime, should.Match(ct.Clock.Now()))
		assert.NoErr(t, datastore.RunInTransaction(ctx, se, nil))

		t.Run("Does not credit quota for on upload runs (NewPatchsetRun)", func(t *ftt.Test) {
			postActions := make([]*run.OngoingLongOps_Op_ExecutePostActionPayload, 0, len(rs.OngoingLongOps.GetOps()))
			for _, op := range rs.OngoingLongOps.GetOps() {
				if act := op.GetExecutePostAction(); act != nil {
					d := timestamppb.New(ct.Clock.Now().UTC().Add(maxPostActionExecutionDuration))
					assert.That(t, op.GetDeadline(), should.Match(d))
					assert.Loosely(t, op.GetCancelRequested(), should.BeFalse)
					postActions = append(postActions, act)
				}
			}
			sort.Slice(postActions, func(i, j int) bool {
				return strings.Compare(postActions[i].GetName(), postActions[j].GetName()) < 0
			})

			assert.That(t, postActions, should.Match([]*run.OngoingLongOps_Op_ExecutePostActionPayload{}))
		})
	})
}

func TestCheckRunCreate(t *testing.T) {
	t.Parallel()
	ftt.Run("CheckRunCreate", t, func(t *ftt.Test) {
		ct := &cvtesting.Test{}
		ctx := ct.SetUp(t)
		const clid1 = 1
		const clid2 = 2
		const gHost = "x-review.example.com"
		const gRepo = "luci-go"
		const gChange1 = 123
		const gChange2 = 234
		const lProject = "infra"

		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					Gerrit: []*cfgpb.ConfigGroup_Gerrit{{
						Url: "https://" + gHost,
						Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
							{Name: gRepo, RefRegexp: []string{"refs/heads/.+"}},
						},
					}},
					Verifiers: &cfgpb.Verifiers{
						GerritCqAbility: &cfgpb.Verifiers_GerritCQAbility{
							CommitterList: []string{"committer-group"},
						},
					},
				},
			},
		})
		cgs, err := prjcfgtest.MustExist(ctx, lProject).GetConfigGroups(ctx)
		assert.NoErr(t, err)

		cg := cgs[0]

		rid := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("deadbeef"))
		rs := &state.RunState{
			Run: run.Run{
				ID:            rid,
				Status:        run.Status_RUNNING,
				ConfigGroupID: prjcfg.MakeConfigGroupID("deadbeef", "main"),
				CreateTime:    ct.Clock.Now().Add(-2 * time.Minute),
				StartTime:     ct.Clock.Now().Add(-1 * time.Minute),
				CLs:           common.CLIDs{clid1, clid2},
			},
		}
		cls := []*changelist.CL{
			{
				ID:             clid1,
				ExternalID:     changelist.MustGobID(gHost, gChange1),
				IncompleteRuns: common.RunIDs{rid},
				EVersion:       3,
				UpdateTime:     ct.Clock.Now().UTC(),
				Snapshot: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: gf.CI(gChange1,
							gf.Owner("user-1"),
							gf.Approve(),
							gf.CQ(+2, rs.CreateTime, "user-1"),
						),
					}},
					LuciProject:        lProject,
					ExternalUpdateTime: timestamppb.New(ct.Clock.Now()),
				},
			},
			{
				ID:             clid2,
				ExternalID:     changelist.MustGobID(gHost, gChange2),
				IncompleteRuns: common.RunIDs{rid},
				EVersion:       5,
				UpdateTime:     ct.Clock.Now().UTC(),
				Snapshot: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: gf.CI(gChange2,
							gf.Owner("user-1"),
							gf.Approve(),
							gf.CQ(+2, rs.CreateTime, "user-1"),
						),
					}},
					LuciProject:        lProject,
					ExternalUpdateTime: timestamppb.New(ct.Clock.Now()),
				},
			},
		}
		rcls := []*run.RunCL{
			{
				ID:         clid1,
				Run:        datastore.MakeKey(ctx, common.RunKind, string(rid)),
				ExternalID: cls[0].ExternalID,
				Detail:     cls[0].Snapshot,
				Trigger: trigger.Find(&trigger.FindInput{
					ChangeInfo:  cls[0].Snapshot.GetGerrit().GetInfo(),
					ConfigGroup: cg.Content,
				}).GetCqVoteTrigger(),
			},
			{
				ID:         clid2,
				Run:        datastore.MakeKey(ctx, common.RunKind, string(rid)),
				ExternalID: cls[1].ExternalID,
				Detail:     cls[1].Snapshot,
				Trigger: trigger.Find(&trigger.FindInput{
					ChangeInfo:  cls[1].Snapshot.GetGerrit().GetInfo(),
					ConfigGroup: cg.Content,
				}).GetCqVoteTrigger(),
			},
		}
		assert.NoErr(t, datastore.Put(ctx, cls, rcls))
		ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			{Email: "user-1@example.com"},
		})

		t.Run("Returns empty metas for new patchset run", func(t *ftt.Test) {
			rs.Mode = run.NewPatchsetRun
			ok, err := checkRunCreate(ctx, ct.GFake, rs, cg, rcls, cls)
			assert.NoErr(t, err)
			assert.Loosely(t, ok, should.BeFalse)
			assert.Loosely(t, rs.OngoingLongOps.Ops, should.HaveLength(1))
			for _, op := range rs.OngoingLongOps.Ops {
				reqs := op.GetResetTriggers().GetRequests()
				assert.Loosely(t, reqs, should.HaveLength(2))
				assert.Loosely(t, reqs[0].Clid, should.Equal(clid1))
				assert.Loosely(t, reqs[0].Message, should.BeEmpty)
				assert.Loosely(t, reqs[0].AddToAttention, should.BeEmpty)
				assert.Loosely(t, reqs[1].Clid, should.Equal(clid2))
				assert.Loosely(t, reqs[1].Message, should.BeEmpty)
				assert.Loosely(t, reqs[1].AddToAttention, should.BeEmpty)
			}
		})
		t.Run("Populates metas for other modes", func(t *ftt.Test) {
			rs.Mode = run.FullRun
			ok, err := checkRunCreate(ctx, ct.GFake, rs, cg, rcls, cls)
			assert.NoErr(t, err)
			assert.Loosely(t, ok, should.BeFalse)
			assert.Loosely(t, rs.OngoingLongOps.Ops, should.HaveLength(1))
			for _, op := range rs.OngoingLongOps.Ops {
				reqs := op.GetResetTriggers().GetRequests()
				assert.Loosely(t, reqs, should.HaveLength(2))
				assert.Loosely(t, reqs[0].Clid, should.Equal(clid1))
				assert.Loosely(t, reqs[0].Message, should.Equal("CV cannot start a Run for `user-1@example.com` because the user is not a committer."))
				assert.That(t, reqs[0].AddToAttention, should.Match([]gerrit.Whom{
					gerrit.Whom_OWNER,
					gerrit.Whom_CQ_VOTERS}))
				assert.Loosely(t, reqs[1].Clid, should.Equal(clid2))
				assert.Loosely(t, reqs[1].Message, should.Equal("CV cannot start a Run for `user-1@example.com` because the user is not a committer."))
				assert.That(t, reqs[1].AddToAttention, should.Match([]gerrit.Whom{
					gerrit.Whom_OWNER,
					gerrit.Whom_CQ_VOTERS}))
			}
		})

		t.Run("Populates metas if run has root CL", func(t *ftt.Test) {
			rs.Mode = run.FullRun
			rs.RootCL = clid1
			// make rootCL not submittable
			ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
				{Email: "user-1@example.com"},
			})
			cls[0].Snapshot.GetGerrit().Info = gf.CI(gChange1,
				gf.Owner("user-1"),
				gf.Disapprove(),
				gf.CQ(+2, rs.CreateTime, "user-1"),
			)
			// Remove the trigger on second CL
			cls[1].Snapshot.GetGerrit().Info = gf.CI(gChange2,
				gf.Owner("user-1"),
				gf.Approve(),
			)
			rcls[0].Detail = cls[0].Snapshot
			rcls[1].Detail = cls[1].Snapshot
			assert.NoErr(t, datastore.Put(ctx, cls, rcls))

			t.Run("Only root CL fails the ACL check", func(t *ftt.Test) {
				ct.AddMember("user-1", "committer-group") // can create a full run on cl2 now
				ok, err := checkRunCreate(ctx, ct.GFake, rs, cg, rcls, cls)
				assert.NoErr(t, err)
				assert.Loosely(t, ok, should.BeFalse)
				assert.Loosely(t, rs.OngoingLongOps.Ops, should.HaveLength(1))
				for _, op := range rs.OngoingLongOps.Ops {
					reqs := op.GetResetTriggers().GetRequests()
					assert.Loosely(t, reqs, should.HaveLength(1))
					assert.Loosely(t, reqs[0].Clid, should.Equal(clid1))
					assert.Loosely(t, reqs[0].Message, should.ContainSubstring("CV cannot start a Run because this CL is not submittable"))
					assert.That(t, reqs[0].AddToAttention, should.Match([]gerrit.Whom{
						gerrit.Whom_OWNER,
						gerrit.Whom_CQ_VOTERS}))
				}
			})

			t.Run("Non root CL also fails the ACL check", func(t *ftt.Test) {
				ok, err := checkRunCreate(ctx, ct.GFake, rs, cg, rcls, cls)
				assert.NoErr(t, err)
				assert.Loosely(t, ok, should.BeFalse)
				assert.Loosely(t, rs.OngoingLongOps.Ops, should.HaveLength(1))
				for _, op := range rs.OngoingLongOps.Ops {
					reqs := op.GetResetTriggers().GetRequests()
					assert.Loosely(t, reqs, should.HaveLength(1))
					assert.Loosely(t, reqs[0].Clid, should.Equal(clid1))
					assert.Loosely(t, reqs[0].Message, should.ContainSubstring("can not start the Run due to following errors"))
					assert.That(t, reqs[0].AddToAttention, should.Match([]gerrit.Whom{
						gerrit.Whom_OWNER,
						gerrit.Whom_CQ_VOTERS}))
				}
			})
		})
	})
}

type dependencies struct {
	pm         *prjmanager.Notifier
	rm         *run.Notifier
	qm         *quotaManagerMock
	tjNotifier *tryjobNotifierMock
	clUpdater  *clUpdaterMock
}

type testHandler struct {
	t     testing.TB
	inner Handler
}

func validateStateMutation(t testing.TB, passed, initialCopy, result *state.RunState) {
	t.Helper()

	switch {
	case should.Match(initialCopy)(result) == nil:
		// No state change; doesn't matter whether shallow copy is created or not.
		return
	case passed == result:
		assert.Loosely(t, errors.New("handler mutated the input state but doesn't create a shallow copy before mutation"), should.BeNil, truth.LineContext())
	case should.Match(passed)(initialCopy) != nil:
		assert.Loosely(t, errors.New("handler created a shallow copy but modified addressable property in place; forgot to clone a proto?"), should.BeNil, truth.LineContext())
	}
}

func (t *testHandler) Start(ctx context.Context, rs *state.RunState) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.Start(ctx, rs)
	if err != nil {
		return nil, err
	}
	validateStateMutation(t.t, rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) Cancel(ctx context.Context, rs *state.RunState, reasons []string) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.Cancel(ctx, rs, reasons)
	if err != nil {
		return nil, err
	}
	validateStateMutation(t.t, rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) OnCLsUpdated(ctx context.Context, rs *state.RunState, cls common.CLIDs) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.OnCLsUpdated(ctx, rs, cls)
	if err != nil {
		return nil, err
	}
	validateStateMutation(t.t, rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) UpdateConfig(ctx context.Context, rs *state.RunState, ver string) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.UpdateConfig(ctx, rs, ver)
	if err != nil {
		return nil, err
	}
	validateStateMutation(t.t, rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) OnReadyForSubmission(ctx context.Context, rs *state.RunState) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.OnReadyForSubmission(ctx, rs)
	if err != nil {
		return nil, err
	}
	validateStateMutation(t.t, rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) OnCLsSubmitted(ctx context.Context, rs *state.RunState, cls common.CLIDs) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.OnCLsSubmitted(ctx, rs, cls)
	if err != nil {
		return nil, err
	}
	validateStateMutation(t.t, rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) OnSubmissionCompleted(ctx context.Context, rs *state.RunState, sc *eventpb.SubmissionCompleted) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.OnSubmissionCompleted(ctx, rs, sc)
	if err != nil {
		return nil, err
	}
	validateStateMutation(t.t, rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) OnLongOpCompleted(ctx context.Context, rs *state.RunState, result *eventpb.LongOpCompleted) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.OnLongOpCompleted(ctx, rs, result)
	if err != nil {
		return nil, err
	}
	validateStateMutation(t.t, rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) OnTryjobsUpdated(ctx context.Context, rs *state.RunState, tryjobs common.TryjobIDs) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.OnTryjobsUpdated(ctx, rs, tryjobs)
	if err != nil {
		return nil, err
	}
	validateStateMutation(t.t, rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) TryResumeSubmission(ctx context.Context, rs *state.RunState) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.TryResumeSubmission(ctx, rs)
	if err != nil {
		return nil, err
	}
	validateStateMutation(t.t, rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) Poke(ctx context.Context, rs *state.RunState) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.Poke(ctx, rs)
	if err != nil {
		return nil, err
	}
	validateStateMutation(t.t, rs, initialCopy, res.State)
	return res, err
}

func (t *testHandler) OnParentRunCompleted(ctx context.Context, rs *state.RunState) (*Result, error) {
	initialCopy := rs.DeepCopy()
	res, err := t.inner.OnParentRunCompleted(ctx, rs)
	if err != nil {
		return nil, err
	}
	validateStateMutation(t.t, rs, initialCopy, res.State)
	return res, err
}

func makeTestHandler(ct *cvtesting.Test) (Handler, dependencies) {
	handler, dependencies := makeImpl(ct)
	return &testHandler{t: ct.TB, inner: handler}, dependencies
}

// makeImpl should only be used to test common functions. For testing handler,
// please use makeTestHandler instead.
func makeImpl(ct *cvtesting.Test) (*Impl, dependencies) {
	deps := dependencies{
		pm:         prjmanager.NewNotifier(ct.TQDispatcher),
		rm:         run.NewNotifier(ct.TQDispatcher),
		qm:         &quotaManagerMock{},
		tjNotifier: &tryjobNotifierMock{},
		clUpdater:  &clUpdaterMock{},
	}
	cf := rdb.NewMockRecorderClientFactory(ct.GoMockCtl)
	impl := &Impl{
		PM:          deps.pm,
		RM:          deps.rm,
		TN:          deps.tjNotifier,
		CLMutator:   changelist.NewMutator(ct.TQDispatcher, deps.pm, deps.rm, nil),
		CLUpdater:   deps.clUpdater,
		TreeFactory: tree.NewClientFactory(ct.TreeFakeSrv.Host()),
		GFactory:    ct.GFactory(),
		BQExporter:  bq.NewExporter(ct.TQDispatcher, ct.BQFake, ct.Env),
		RdbNotifier: rdb.NewNotifier(ct.TQDispatcher, cf),
		Publisher:   pubsub.NewPublisher(ct.TQDispatcher, ct.Env),
		QM:          deps.qm,
		Env:         ct.Env,
	}
	return impl, deps
}

type clUpdaterMock struct {
	m            sync.Mutex
	refreshedCLs common.CLIDs
}

func (c *clUpdaterMock) ScheduleBatch(ctx context.Context, luciProject string, cls []*changelist.CL, requester changelist.UpdateCLTask_Requester) error {
	c.m.Lock()
	for _, cl := range cls {
		c.refreshedCLs = append(c.refreshedCLs, cl.ID)
	}
	c.m.Unlock()
	return nil
}

type tryjobNotifierMock struct {
	m               sync.Mutex
	updateScheduled common.TryjobIDs
}

func (t *tryjobNotifierMock) ScheduleUpdate(ctx context.Context, id common.TryjobID, _ tryjob.ExternalID) error {
	t.m.Lock()
	t.updateScheduled = append(t.updateScheduled, id)
	t.m.Unlock()
	return nil
}

type quotaManagerMock struct {
	runQuotaOp  *quotapb.OpResult
	userLimit   *cfgpb.UserLimit
	runQuotaErr error

	debitRunQuotaCalls int
}

func (qm *quotaManagerMock) DebitRunQuota(ctx context.Context, r *run.Run) (*quotapb.OpResult, *cfgpb.UserLimit, error) {
	qm.debitRunQuotaCalls++
	return qm.runQuotaOp, qm.userLimit, qm.runQuotaErr
}
