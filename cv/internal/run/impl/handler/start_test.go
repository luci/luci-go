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
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/server/quota"
	"go.chromium.org/luci/server/quota/quotapb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/runtest"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestStart(t *testing.T) {
	t.Parallel()

	ftt.Run("StartRun", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			lProject           = "chromium"
			configGroupName    = "combinable"
			gerritHost         = "chromium-review.googlesource.com"
			committers         = "committer-group"
			dryRunners         = "dry-runner-group"
			stabilizationDelay = time.Minute
			startLatency       = 2 * time.Minute
		)

		builder := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "try",
			Builder: "cool_tester",
		}
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{{
			Name: configGroupName,
			CombineCls: &cfgpb.CombineCLs{
				StabilizationDelay: durationpb.New(stabilizationDelay),
			},
			Verifiers: &cfgpb.Verifiers{
				GerritCqAbility: &cfgpb.Verifiers_GerritCQAbility{
					CommitterList:    []string{committers},
					DryRunAccessList: []string{dryRunners},
				},
				Tryjob: &cfgpb.Verifiers_Tryjob{
					Builders: []*cfgpb.Verifiers_Tryjob_Builder{
						{
							Name: bbutil.FormatBuilderID(builder),
						},
					},
				},
			},
		}}})

		makeIdentity := func(email string) identity.Identity {
			id, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, email))
			assert.NoErr(t, err)
			return id
		}

		const tEmail = "t@example.org"
		rs := &state.RunState{
			Run: run.Run{
				ID:            lProject + "/1111111111111-deadbeef",
				Status:        run.Status_PENDING,
				CreateTime:    clock.Now(ctx).UTC().Add(-startLatency),
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
				Mode:          run.DryRun,
				CreatedBy:     makeIdentity(tEmail),
				BilledTo:      makeIdentity(tEmail),
			},
		}
		h, deps := makeTestHandler(&ct)

		var clid common.CLID
		var accountID int64
		addCL := func(triggerer, owner string) *changelist.CL {
			clid++
			accountID++
			rs.CLs = append(rs.CLs, clid)
			ci := gf.CI(100+int(clid),
				gf.Owner(owner),
				gf.CQ(+1, rs.CreateTime, gf.U(triggerer)))
			cl := &changelist.CL{
				ID:         clid,
				ExternalID: changelist.MustGobID(gerritHost, ci.GetNumber()),
				Snapshot: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gerritHost,
							Info: ci,
						},
					},
				},
			}
			rCL := &run.RunCL{
				ID:         clid,
				Run:        datastore.MakeKey(ctx, common.RunKind, string(rs.ID)),
				ExternalID: changelist.MustGobID(gerritHost, ci.GetNumber()),
				Trigger: &run.Trigger{
					Email:           gf.U(triggerer).Email,
					Time:            timestamppb.New(rs.CreateTime),
					Mode:            string(rs.Mode),
					GerritAccountId: accountID,
				},
			}
			assert.Loosely(t, datastore.Put(ctx, cl, rCL), should.BeNil)
			return cl
		}

		const (
			owner     = "user-1"
			triggerer = owner
		)
		cl := addCL(triggerer, owner)
		ct.AddMember(owner, dryRunners)
		ct.AddMember(owner, committers)
		ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			{Email: fmt.Sprintf("%s@example.com", owner)},
		})

		t.Run("Starts when Run is PENDING", func(t *ftt.Test) {
			deps.qm.runQuotaOp = &quotapb.OpResult{
				Status:          quotapb.OpResult_SUCCESS,
				NewBalance:      5,
				PreviousBalance: 4,
			}

			res, err := h.Start(ctx, rs)
			assert.NoErr(t, err)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			assert.Loosely(t, deps.qm.debitRunQuotaCalls, should.Equal(1))

			assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
			assert.Loosely(t, res.State.StartTime, should.Match(ct.Clock.Now().UTC()))
			assert.Loosely(t, res.State.Tryjobs, should.Resemble(&run.Tryjobs{
				Requirement: &tryjob.Requirement{
					Definitions: []*tryjob.Definition{
						{
							Backend: &tryjob.Definition_Buildbucket_{
								Buildbucket: &tryjob.Definition_Buildbucket{
									Host:    chromeinfra.BuildbucketHost,
									Builder: builder,
								},
							},
							Critical: true,
						},
					},
				},
				RequirementVersion:    1,
				RequirementComputedAt: timestamppb.New(ct.Clock.Now().UTC()),
			}))
			assert.Loosely(t, res.State.LogEntries, should.HaveLength(2))
			assert.Loosely(t, res.State.LogEntries[0].GetInfo().GetMessage(), should.Equal("Run quota debited from t@example.org; balance: 5"))
			assert.Loosely(t, res.State.LogEntries[1].GetStarted(), should.NotBeNil)

			assert.Loosely(t, res.State.NewLongOpIDs, should.HaveLength(2))
			assert.Loosely(t, res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]].GetExecuteTryjobs(), should.NotBeNil)
			assert.Loosely(t, res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[1]].GetPostStartMessage(), should.BeTrue)

			assert.Loosely(t, res.SideEffectFn, should.NotBeNil)
			assert.Loosely(t, datastore.RunInTransaction(ctx, res.SideEffectFn, nil), should.BeNil)
			assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Public.RunStarted, lProject, configGroupName, string(run.DryRun)), should.Equal(1))
			assert.Loosely(t, ct.TSMonSentDistr(ctx, metricPickupLatencyS, lProject).Sum(),
				should.AlmostEqual(startLatency.Seconds()))
			assert.Loosely(t, ct.TSMonSentDistr(ctx, metricPickupLatencyAdjustedS, lProject).Sum(),
				should.AlmostEqual((startLatency - stabilizationDelay).Seconds()))
		})

		t.Run("Does not proceed if run quota is not available", func(t *ftt.Test) {
			deps.qm.runQuotaErr = quota.ErrQuotaApply
			deps.qm.runQuotaOp = &quotapb.OpResult{
				Status: quotapb.OpResult_ERR_UNDERFLOW,
			}
			deps.qm.userLimit = &cfgpb.UserLimit{
				Run: &cfgpb.UserLimit_Run{
					ReachLimitMsg: "foo bar.",
				},
			}

			res, err := h.Start(ctx, rs)
			assert.NoErr(t, err)
			assert.Loosely(t, res.PreserveEvents, should.BeTrue)
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_PENDING))
			ops := res.State.OngoingLongOps.GetOps()
			assert.Loosely(t, len(ops), should.Equal(1))
			assert.Loosely(t, ops, should.ContainKey("1-1"))
			assert.Loosely(t, ops["1-1"].GetPostGerritMessage(), should.Resemble(&run.OngoingLongOps_Op_PostGerritMessage{
				Message: fmt.Sprintf("User %s has exhausted their run quota. This run will start once the quota balance has recovered.\n\nfoo bar.", rs.Run.BilledTo.Email()),
			}))
			assert.Loosely(t, ct.TSMonSentValue(
				ctx,
				metrics.Public.RunQuotaRejection,
				lProject,
				"combinable",
				"chromium/1",
			), should.Equal(1))

			t.Run("Enqueue pending message only once when quota is exhausted", func(t *ftt.Test) {
				res, err := h.Start(ctx, rs)
				assert.NoErr(t, err)
				ops := res.State.OngoingLongOps.GetOps()
				assert.Loosely(t, ops, should.ContainKey("1-1"))
				assert.Loosely(t, ops["1-1"].GetPostGerritMessage(), should.Resemble(&run.OngoingLongOps_Op_PostGerritMessage{
					Message: fmt.Sprintf("User %s has exhausted their run quota. This run will start once the quota balance has recovered.\n\nfoo bar.", rs.Run.BilledTo.Email()),
				}))
			})
		})

		t.Run("Throws error when quota manager fails with an unexpected error", func(t *ftt.Test) {
			deps.qm.runQuotaErr = quota.ErrQuotaApply
			deps.qm.runQuotaOp = &quotapb.OpResult{
				Status: quotapb.OpResult_ERR_UNKNOWN,
			}

			res, err := h.Start(ctx, rs)
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, should.ErrLike("QM.DebitRunQuota: unexpected quotaOp Status ERR_UNKNOWN"))
		})

		t.Run("Does not proceed if any parent RUN is still PENDING", func(t *ftt.Test) {
			const parentRun = common.RunID("parent/1-cow")
			assert.Loosely(t, datastore.Put(ctx,
				&run.Run{
					ID:     parentRun,
					Status: run.Status_PENDING,
				},
			), should.BeNil)
			rs.DepRuns = common.RunIDs{parentRun}
			res, err := h.Start(ctx, rs)
			assert.NoErr(t, err)
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_PENDING))
			ops := res.State.OngoingLongOps.GetOps()
			assert.Loosely(t, len(ops), should.BeZero)
		})

		t.Run("Does not proceed if parent RUN is CANCELLED/FAILED", func(t *ftt.Test) {
			const parentRun = common.RunID("parent/1-cow")
			assert.Loosely(t, datastore.Put(ctx,
				&run.Run{
					ID:     parentRun,
					Status: run.Status_CANCELLED,
				},
			), should.BeNil)
			rs.DepRuns = common.RunIDs{parentRun}
			res, err := h.Start(ctx, rs)
			assert.NoErr(t, err)
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_PENDING))
		})

		t.Run("Emits Start events for PENDING children", func(t *ftt.Test) {
			const child1 = common.RunID("child/1-cow")
			assert.Loosely(t, datastore.Put(
				ctx,
				&run.Run{
					ID:      child1,
					Status:  run.Status_PENDING,
					DepRuns: common.RunIDs{rs.ID},
				},
			), should.BeNil)
			res, err := h.Start(ctx, rs)
			assert.NoErr(t, err)
			assert.Loosely(t, res.SideEffectFn, should.NotBeNil)
			assert.Loosely(t, datastore.RunInTransaction(ctx, res.SideEffectFn, nil), should.BeNil)
			runtest.AssertReceivedStart(t, ctx, child1)
		})

		t.Run("Fail the Run if tryjob computation fails", func(t *ftt.Test) {
			if rs.Options == nil {
				rs.Options = &run.Options{}
			}
			// included a builder that doesn't exist
			rs.Options.IncludedTryjobs = append(rs.Options.IncludedTryjobs, "fooproj/ci:bar_builder")
			anotherCL := addCL(triggerer, owner)
			rs.CLs = common.CLIDs{cl.ID, anotherCL.ID}
			t.Run("Reset triggers on all CLs", func(t *ftt.Test) {
				res, err := h.Start(ctx, rs)
				assert.NoErr(t, err)
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
				assert.Loosely(t, res.State.Status, should.Equal(run.Status_PENDING))
				assert.Loosely(t, res.State.Tryjobs, should.BeNil)
				assert.Loosely(t, res.State.NewLongOpIDs, should.HaveLength(1))
				op := res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]]
				assert.Loosely(t, op.GetResetTriggers(), should.NotBeNil)
				assert.Loosely(t, op.GetResetTriggers().GetRunStatusIfSucceeded(), should.Equal(run.Status_FAILED))
				resetCLs := common.CLIDs{}
				for _, req := range op.GetResetTriggers().GetRequests() {
					resetCLs = append(resetCLs, common.CLID(req.Clid))
				}
				assert.Loosely(t, resetCLs, should.Resemble(res.State.CLs))
				assert.Loosely(t, res.State.LogEntries, should.HaveLength(1))
				assert.Loosely(t, res.State.LogEntries[0].GetInfo(), should.Resemble(&run.LogEntry_Info{
					Label:   "Tryjob Requirement Computation",
					Message: "Failed to compute tryjob requirement. Reason: builder \"fooproj/ci/bar_builder\" is included but not defined in the LUCI project",
				}))
			})
			t.Run("Only reset trigger on root CL", func(t *ftt.Test) {
				rs.RootCL = cl.ID
				res, err := h.Start(ctx, rs)
				assert.NoErr(t, err)
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.State.NewLongOpIDs, should.HaveLength(1))
				op := res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]]
				assert.Loosely(t, op.GetResetTriggers(), should.NotBeNil)
				assert.Loosely(t, op.GetResetTriggers().GetRunStatusIfSucceeded(), should.Equal(run.Status_FAILED))
				resetCLs := common.CLIDs{}
				for _, req := range op.GetResetTriggers().GetRequests() {
					resetCLs = append(resetCLs, common.CLID(req.Clid))
				}
				assert.Loosely(t, resetCLs, should.Resemble(common.CLIDs{rs.RootCL}))
			})
		})

		t.Run("Fail the Run if acls.CheckRunCreate fails", func(t *ftt.Test) {
			ct.ResetMockedAuthDB(ctx)
			res, err := h.Start(ctx, rs)
			assert.NoErr(t, err)
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)

			assert.Loosely(t, res.State.Status, should.Equal(run.Status_PENDING))
			assert.Loosely(t, res.State.LogEntries, should.HaveLength(1))
			assert.Loosely(t, res.State.LogEntries[0].GetInfo(), should.Resemble(&run.LogEntry_Info{
				Label: "Run failed",
				Message: "" +
					"the Run does not pass eligibility checks. See reasons at:" +
					"\n  * " + cl.ExternalID.MustURL(),
			}))

			assert.Loosely(t, res.State.NewLongOpIDs, should.HaveLength(1))
			longOp := res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]]
			resetOp := longOp.GetResetTriggers()
			assert.Loosely(t, resetOp.Requests, should.HaveLength(1))
			assert.Loosely(t, resetOp.Requests[0], should.Resemble(
				&run.OngoingLongOps_Op_ResetTriggers_Request{
					Clid: int64(cl.ID),
					Message: fmt.Sprintf(
						"CV cannot start a Run for `%s` because the user is not a dry-runner.", gf.U(triggerer).Email,
					),
					Notify: gerrit.Whoms{
						gerrit.Whom_OWNER,
						gerrit.Whom_CQ_VOTERS,
					},
					AddToAttention: gerrit.Whoms{
						gerrit.Whom_OWNER,
						gerrit.Whom_CQ_VOTERS,
					},
					AddToAttentionReason: "CQ/CV Run failed",
				},
			))
			assert.Loosely(t, resetOp.RunStatusIfSucceeded, should.Equal(run.Status_FAILED))

			t.Run("Only reset trigger on root CL", func(t *ftt.Test) {
				anotherCL := addCL(triggerer, owner)
				rs.CLs = common.CLIDs{cl.ID, anotherCL.ID}
				rs.RootCL = cl.ID
				res, err := h.Start(ctx, rs)
				assert.NoErr(t, err)
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.State.NewLongOpIDs, should.HaveLength(1))
				op := res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]]
				assert.Loosely(t, op.GetResetTriggers(), should.NotBeNil)
				assert.Loosely(t, op.GetResetTriggers().GetRunStatusIfSucceeded(), should.Equal(run.Status_FAILED))
				resetCLs := common.CLIDs{}
				for _, req := range op.GetResetTriggers().GetRequests() {
					resetCLs = append(resetCLs, common.CLID(req.Clid))
				}
				assert.Loosely(t, resetCLs, should.Resemble(common.CLIDs{rs.RootCL}))
			})
		})

		statuses := []run.Status{
			run.Status_RUNNING,
			run.Status_WAITING_FOR_SUBMISSION,
			run.Status_SUBMITTING,
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			t.Run(fmt.Sprintf("Noop when Run is %s", status), func(t *ftt.Test) {
				rs.Status = status
				res, err := h.Start(ctx, rs)
				assert.NoErr(t, err)
				assert.Loosely(t, res.State, should.Equal(rs))
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			})
		}
	})

	ftt.Run("Starts when Run is PENDING and mode is NewPatchsetRun", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			lProject           = "chromium"
			configGroupName    = "combinable"
			gerritHost         = "chromium-review.googlesource.com"
			committers         = "committer-group"
			dryRunners         = "dry-runner-group"
			stabilizationDelay = time.Minute
			startLatency       = 2 * time.Minute
		)

		builder := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "try",
			Builder: "cool_tester",
		}
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{{
			Name: configGroupName,
			CombineCls: &cfgpb.CombineCLs{
				StabilizationDelay: durationpb.New(stabilizationDelay),
			},
			Verifiers: &cfgpb.Verifiers{
				GerritCqAbility: &cfgpb.Verifiers_GerritCQAbility{
					CommitterList:    []string{committers},
					DryRunAccessList: []string{dryRunners},
				},
				Tryjob: &cfgpb.Verifiers_Tryjob{
					Builders: []*cfgpb.Verifiers_Tryjob_Builder{
						{
							Name: bbutil.FormatBuilderID(builder),
						},
					},
				},
			},
		}}})

		makeIdentity := func(email string) identity.Identity {
			id, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, email))
			assert.NoErr(t, err)
			return id
		}

		const tEmail = "t@example.org"
		rs := &state.RunState{
			Run: run.Run{
				ID:            lProject + "/1111111111111-deadbeef",
				Status:        run.Status_PENDING,
				CreateTime:    clock.Now(ctx).UTC().Add(-startLatency),
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
				Mode:          run.NewPatchsetRun,
				CreatedBy:     makeIdentity(tEmail),
				BilledTo:      makeIdentity(tEmail),
			},
		}
		h, deps := makeTestHandler(&ct)

		const (
			owner     = "user-1"
			triggerer = owner
		)
		ct.AddMember(owner, dryRunners)
		ct.AddMember(owner, committers)
		ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			{Email: fmt.Sprintf("%s@example.com", owner)},
		})

		t.Run("Does not apply quota to on upload runs (NewPatchsetRun)", func(t *ftt.Test) {
			deps.qm.runQuotaOp = &quotapb.OpResult{
				Status:          quotapb.OpResult_SUCCESS,
				NewBalance:      5,
				PreviousBalance: 4,
			}

			res, err := h.Start(ctx, rs)
			assert.NoErr(t, err)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			assert.Loosely(t, deps.qm.debitRunQuotaCalls, should.Equal(0))

			assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
			assert.Loosely(t, res.State.StartTime, should.Match(ct.Clock.Now().UTC()))
			assert.Loosely(t, res.State.Tryjobs, should.Resemble(&run.Tryjobs{
				Requirement:           &tryjob.Requirement{},
				RequirementVersion:    1,
				RequirementComputedAt: timestamppb.New(ct.Clock.Now().UTC()),
			}))
			assert.Loosely(t, res.State.LogEntries, should.HaveLength(1))
			assert.Loosely(t, res.State.LogEntries[0].GetInfo().GetMessage(), should.Equal(""))

			assert.Loosely(t, res.State.NewLongOpIDs, should.HaveLength(1))
			assert.Loosely(t, res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]].GetExecuteTryjobs(), should.NotBeNil)

			assert.Loosely(t, res.SideEffectFn, should.NotBeNil)
			assert.Loosely(t, datastore.RunInTransaction(ctx, res.SideEffectFn, nil), should.BeNil)
			assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Public.RunStarted, lProject, configGroupName, string(run.NewPatchsetRun)), should.Equal(1))
			assert.Loosely(t, ct.TSMonSentDistr(ctx, metricPickupLatencyS, lProject).Sum(),
				should.AlmostEqual(startLatency.Seconds()))
			assert.Loosely(t, ct.TSMonSentDistr(ctx, metricPickupLatencyAdjustedS, lProject).Sum(),
				should.AlmostEqual((startLatency - stabilizationDelay).Seconds()))
		})
	})
}

func TestOnCompletedPostStartMessage(t *testing.T) {
	t.Parallel()

	ftt.Run("onCompletedPostStartMessage works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			lProject = "chromium"
			opID     = "1-1"
		)

		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{{Name: "single"}}})

		rs := &state.RunState{
			Run: run.Run{
				ID:            lProject + "/1111111111111-1-deadbeef",
				Status:        run.Status_RUNNING,
				Mode:          run.DryRun,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
				OngoingLongOps: &run.OngoingLongOps{
					Ops: map[string]*run.OngoingLongOps_Op{
						opID: {
							Work: &run.OngoingLongOps_Op_PostStartMessage{
								PostStartMessage: true,
							},
						},
					},
				},
			},
		}
		result := &eventpb.LongOpCompleted{
			OperationId: opID,
		}
		h, _ := makeTestHandler(&ct)

		t.Run("if Run isn't RUNNING, just cleans up the operation", func(t *ftt.Test) {
			// NOTE: This should be rare. And since posting the starting message isn't
			// a critical operation, it's OK to ignore its failures if the Run is
			// already submitting the CL.
			rs.Run.Status = run.Status_SUBMITTING
			result.Status = eventpb.LongOpCompleted_FAILED
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_SUBMITTING))
			assert.Loosely(t, res.State.OngoingLongOps, should.BeNil)
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
		})

		t.Run("on cancellation, cleans up Run's state", func(t *ftt.Test) {
			// NOTE: as of this writing (Oct 2021), the only time posting start
			// message is cancelled is if the Run was already finalized. Therefore,
			// Run can't be in RUNNING state any more.
			// However, this test aims to cover possible future logic change in CV.
			result.Status = eventpb.LongOpCompleted_CANCELLED
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
			assert.Loosely(t, res.State.OngoingLongOps, should.BeNil)
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
		})

		t.Run("on success, cleans Run's state", func(t *ftt.Test) {
			result.Status = eventpb.LongOpCompleted_SUCCEEDED
			postedAt := ct.Clock.Now().Add(-time.Second)
			result.Result = &eventpb.LongOpCompleted_PostStartMessage_{
				PostStartMessage: &eventpb.LongOpCompleted_PostStartMessage{
					Time: timestamppb.New(postedAt),
				},
			}
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
			assert.Loosely(t, res.State.OngoingLongOps, should.BeNil)
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			assert.Loosely(t, res.State.LogEntries[0].GetTime().AsTime(), should.Resemble(postedAt.UTC()))
		})

		t.Run("on failure, cleans Run's state and record reasons", func(t *ftt.Test) {
			result.Status = eventpb.LongOpCompleted_FAILED
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
			assert.Loosely(t, res.State.OngoingLongOps, should.BeNil)
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			assert.Loosely(t, res.State.LogEntries[0].GetInfo().GetMessage(), should.ContainSubstring("Failed to post the starting message"))
		})

		t.Run("on expiration,cleans Run's state and record reasons", func(t *ftt.Test) {
			result.Status = eventpb.LongOpCompleted_EXPIRED
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
			assert.Loosely(t, res.State.OngoingLongOps, should.BeNil)
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			assert.Loosely(t, res.State.LogEntries[0].GetInfo().GetMessage(), should.ContainSubstring("Failed to post the starting message"))
		})
	})
}
