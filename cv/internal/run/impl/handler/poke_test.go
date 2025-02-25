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
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	tspb "go.chromium.org/luci/tree_status/proto/v1"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/runtest"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestPoke(t *testing.T) {
	t.Parallel()

	ftt.Run("Poke", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			lProject   = "infra"
			gHost      = "x-review.example.com"
			dryRunners = "dry-runner-group"
			gChange    = 1
			gPatchSet  = 5
		)

		cfg := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					Verifiers: &cfgpb.Verifiers{
						TreeStatus: &cfgpb.Verifiers_TreeStatus{
							TreeName: lProject,
						},
						GerritCqAbility: &cfgpb.Verifiers_GerritCQAbility{
							DryRunAccessList: []string{dryRunners},
						},
					},
				},
			},
		}
		prjcfgtest.Create(ctx, lProject, cfg)
		h, deps := makeTestHandler(&ct)

		rid := common.MakeRunID(lProject, ct.Clock.Now(), gChange, []byte("deadbeef"))
		rs := &state.RunState{
			Run: run.Run{
				ID:            rid,
				CreateTime:    ct.Clock.Now().UTC().Add(-2 * time.Minute),
				StartTime:     ct.Clock.Now().UTC().Add(-1 * time.Minute),
				CLs:           common.CLIDs{gChange},
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
				Status:        run.Status_RUNNING,
				Mode:          run.DryRun,
			},
		}

		ci := gf.CI(
			gChange, gf.PS(gPatchSet),
			gf.Owner("foo"),
			gf.CQ(+1, clock.Now(ctx).UTC(), gf.U("foo")),
		)
		ct.AddMember("foo", dryRunners)
		ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			{Email: "foo@example.com"},
		})
		cl := &changelist.CL{
			ID:         gChange,
			ExternalID: changelist.MustGobID(gHost, ci.GetNumber()),
			Snapshot: &changelist.Snapshot{
				LuciProject: lProject,
				Patchset:    ci.GetRevisions()[ci.GetCurrentRevision()].GetNumber(),
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: ci,
					},
				},
			},
		}
		triggers := trigger.Find(&trigger.FindInput{ChangeInfo: ci, ConfigGroup: cfg.GetConfigGroups()[0]})
		assert.Loosely(t, triggers.GetCqVoteTrigger(), should.Match(&run.Trigger{
			Time:            timestamppb.New(clock.Now(ctx).UTC()),
			Mode:            string(run.DryRun),
			Email:           "foo@example.com",
			GerritAccountId: 1,
		}))
		rcl := &run.RunCL{
			ID:      gChange,
			Run:     datastore.MakeKey(ctx, common.RunKind, string(rid)),
			Detail:  cl.Snapshot,
			Trigger: triggers.GetCqVoteTrigger(),
		}
		assert.Loosely(t, datastore.Put(ctx, cl, rcl), should.BeNil)

		now := ct.Clock.Now()
		ctx = context.WithValue(ctx, &fakeTaskIDKey, "task-foo")

		verifyNoOp := func() {
			res, err := h.Poke(ctx, rs)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State, should.Match(rs))
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			assert.Loosely(t, res.PostProcessFn, should.BeNil)
			assert.Loosely(t, deps.clUpdater.refreshedCLs, should.BeEmpty)
		}

		t.Run("Cancels run exceeding max duration", func(t *ftt.Test) {
			ct.Clock.Add(2 * common.MaxRunTotalDuration)
			res, err := h.Poke(ctx, rs)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_CANCELLED))
		})

		t.Run("Tree checks", func(t *ftt.Test) {
			t.Run("Check Tree if condition matches", func(t *ftt.Test) {
				// WAITING_FOR_SUBMISSION makes sense only for FullRun.
				// It's an error condition,
				// if run == DryRun, but status == WAITING_FOR_SUBMISSION.
				rs.Mode = run.FullRun
				rs.Status = run.Status_WAITING_FOR_SUBMISSION
				rs.Submission = &run.Submission{
					TreeOpen:          false,
					LastTreeCheckTime: timestamppb.New(now.Add(-1 * time.Minute)),
				}

				t.Run("Open", func(t *ftt.Test) {
					ct.TreeFakeSrv.ModifyState(lProject, tspb.GeneralState_OPEN)
					res, err := h.Poke(ctx, rs)
					assert.NoErr(t, err)
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
					assert.Loosely(t, res.PostProcessFn, should.NotBeNil)
					// proceed to submission right away
					assert.Loosely(t, res.State.Status, should.Equal(run.Status_SUBMITTING))
					assert.Loosely(t, res.State.Submission, should.Match(&run.Submission{
						Deadline:          timestamppb.New(now.Add(defaultSubmissionDuration)),
						Cls:               []int64{gChange},
						TaskId:            "task-foo",
						TreeOpen:          true,
						LastTreeCheckTime: timestamppb.New(now),
					}))
				})

				t.Run("Close", func(t *ftt.Test) {
					ct.TreeFakeSrv.ModifyState(lProject, tspb.GeneralState_CLOSED)
					res, err := h.Poke(ctx, rs)
					assert.NoErr(t, err)
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
					assert.Loosely(t, res.PostProcessFn, should.BeNil)
					assert.Loosely(t, res.State.Status, should.Equal(run.Status_WAITING_FOR_SUBMISSION))
					// record the result and check again after 1 minute.
					assert.Loosely(t, res.State.Submission, should.Match(&run.Submission{
						TreeOpen:          false,
						LastTreeCheckTime: timestamppb.New(now),
					}))
					runtest.AssertReceivedPoke(t, ctx, rid, now.Add(1*time.Minute))
				})

				t.Run("Failed", func(t *ftt.Test) {
					ct.TreeFakeSrv.InjectErr(lProject, errors.New("error retrieving tree status"))
					t.Run("Not too long", func(t *ftt.Test) {
						res, err := h.Poke(ctx, rs)
						assert.NoErr(t, err)
						assert.That(t, res.State.Status, should.Equal(run.Status_WAITING_FOR_SUBMISSION))
					})

					t.Run("Too long", func(t *ftt.Test) {
						rs.Submission.TreeErrorSince = timestamppb.New(now.Add(-11 * time.Minute))
						res, err := h.Poke(ctx, rs)
						assert.NoErr(t, err)
						assert.That(t, res.State, should.NotEqual(rs))
						assert.Loosely(t, res.SideEffectFn, should.BeNil)
						assert.That(t, res.PreserveEvents, should.BeFalse)
						assert.Loosely(t, res.PostProcessFn, should.BeNil)
						assert.Loosely(t, res.State.NewLongOpIDs, should.HaveLength(1))
						ct := res.State.OngoingLongOps.Ops[res.State.NewLongOpIDs[0]].GetResetTriggers()
						assert.That(t, ct.RunStatusIfSucceeded, should.Equal(run.Status_FAILED))
						assert.Loosely(t, ct.Requests, should.HaveLength(1))
						assert.That(t, ct.Requests[0].Message, should.ContainSubstring("Could not submit this CL because the tree infra repeatedly returned failures"))
						assert.That(t, res.State.Status, should.Equal(run.Status_WAITING_FOR_SUBMISSION))
						t.Run("Reset trigger on root CL only", func(t *ftt.Test) {
							rs.CLs = append(rs.CLs, cl.ID+1000)
							rs.RootCL = cl.ID
							res, err := h.Poke(ctx, rs)
							assert.NoErr(t, err)
							assert.Loosely(t, res.State.NewLongOpIDs, should.HaveLength(1))
							ct := res.State.OngoingLongOps.Ops[res.State.NewLongOpIDs[0]].GetResetTriggers()
							assert.Loosely(t, ct.Requests, should.HaveLength(1))
							assert.Loosely(t, ct.Requests[0].Clid, should.Equal(rs.RootCL))
						})
					})
				})
			})

			t.Run("No-op if condition doesn't match", func(t *ftt.Test) {
				t.Run("Not in WAITING_FOR_SUBMISSION status", func(t *ftt.Test) {
					rs.Status = run.Status_RUNNING
					verifyNoOp()
				})

				t.Run("Tree is open in the previous check", func(t *ftt.Test) {
					rs.Status = run.Status_WAITING_FOR_SUBMISSION
					rs.Submission = &run.Submission{
						TreeOpen:          true,
						LastTreeCheckTime: timestamppb.New(now.Add(-2 * time.Minute)),
					}
					verifyNoOp()
				})

				t.Run("Last Tree check is too recent", func(t *ftt.Test) {
					rs.Status = run.Status_WAITING_FOR_SUBMISSION
					rs.Submission = &run.Submission{
						TreeOpen:          false,
						LastTreeCheckTime: timestamppb.New(now.Add(-1 * time.Second)),
					}
					verifyNoOp()
				})
			})
		})

		t.Run("CLs Refresh", func(t *ftt.Test) {
			t.Run("No-op if finalized", func(t *ftt.Test) {
				rs.Status = run.Status_CANCELLED
				verifyNoOp()
			})
			t.Run("No-op if recently created", func(t *ftt.Test) {
				rs.CreateTime = ct.Clock.Now()
				rs.LatestCLsRefresh = time.Time{}
				verifyNoOp()
			})
			t.Run("No-op if recently refreshed", func(t *ftt.Test) {
				rs.LatestCLsRefresh = ct.Clock.Now().Add(-clRefreshInterval / 2)
				verifyNoOp()
			})
			t.Run("Schedule refresh", func(t *ftt.Test) {
				verifyScheduled := func() {
					res, err := h.Poke(ctx, rs)
					assert.NoErr(t, err)
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
					assert.Loosely(t, res.PostProcessFn, should.BeNil)
					assert.Loosely(t, res.State, should.NotEqual(rs))
					assert.Loosely(t, res.State.LatestCLsRefresh, should.Match(datastore.RoundTime(ct.Clock.Now().UTC())))
					assert.Loosely(t, deps.clUpdater.refreshedCLs.Contains(1), should.BeTrue)
				}
				t.Run("For the first time", func(t *ftt.Test) {
					rs.CreateTime = ct.Clock.Now().Add(-clRefreshInterval - time.Second)
					rs.LatestCLsRefresh = time.Time{}
					verifyScheduled()
				})
				t.Run("For the second (and later) time", func(t *ftt.Test) {
					rs.LatestCLsRefresh = ct.Clock.Now().Add(-clRefreshInterval - time.Second)
					verifyScheduled()
				})
			})
			t.Run("Run fails if no longer eligible", func(t *ftt.Test) {
				rs.LatestCLsRefresh = ct.Clock.Now().Add(-clRefreshInterval - time.Second)
				ct.ResetMockedAuthDB(ctx)

				// verify that it did not schedule refresh but reset triggers.
				res, err := h.Poke(ctx, rs)
				assert.NoErr(t, err)
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
				assert.Loosely(t, res.PostProcessFn, should.BeNil)
				assert.Loosely(t, res.State.Status, should.Equal(rs.Status))
				assert.Loosely(t, deps.clUpdater.refreshedCLs, should.BeEmpty)

				longOp := res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]]
				resetOp := longOp.GetResetTriggers()
				assert.Loosely(t, resetOp.Requests, should.HaveLength(1))
				assert.Loosely(t, resetOp.Requests[0], should.Match(
					&run.OngoingLongOps_Op_ResetTriggers_Request{
						Clid:    int64(gChange),
						Message: "CV cannot start a Run for `foo@example.com` because the user is not a dry-runner.",
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
			})
		})

		t.Run("Tryjobs Refresh", func(t *ftt.Test) {
			reqmt := &tryjob.Requirement{
				Definitions: []*tryjob.Definition{
					{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Builder: &buildbucketpb.BuilderID{
									Project: "test_proj",
									Bucket:  "test_bucket",
									Builder: "test_builder",
								},
							},
						},
					},
				},
			}
			rs.Tryjobs = &run.Tryjobs{
				Requirement: reqmt,
				State: &tryjob.ExecutionState{
					Requirement: reqmt,
					Executions: []*tryjob.ExecutionState_Execution{
						{
							Attempts: []*tryjob.ExecutionState_Execution_Attempt{
								{
									TryjobId:   1,
									ExternalId: string(tryjob.MustBuildbucketID("bb.example.com", 456)),
									Status:     tryjob.Status_ENDED,
								},
								{
									TryjobId:   2,
									ExternalId: string(tryjob.MustBuildbucketID("bb.example.com", 123)),
									Status:     tryjob.Status_TRIGGERED,
								},
							},
						},
					},
				},
			}
			t.Run("No-op if finalized", func(t *ftt.Test) {
				rs.Status = run.Status_CANCELLED
				verifyNoOp()
			})
			t.Run("No-op if recently created", func(t *ftt.Test) {
				rs.CreateTime = ct.Clock.Now()
				rs.LatestTryjobsRefresh = time.Time{}
				verifyNoOp()
			})
			t.Run("No-op if recently refreshed", func(t *ftt.Test) {
				rs.LatestTryjobsRefresh = ct.Clock.Now().Add(-tryjobRefreshInterval / 2)
				verifyNoOp()
			})
			t.Run("Schedule refresh", func(t *ftt.Test) {
				verifyScheduled := func() {
					res, err := h.Poke(ctx, rs)
					assert.NoErr(t, err)
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
					assert.Loosely(t, res.PostProcessFn, should.BeNil)
					assert.Loosely(t, res.State, should.NotEqual(rs))
					assert.Loosely(t, res.State.LatestTryjobsRefresh, should.Match(datastore.RoundTime(ct.Clock.Now().UTC())))
					assert.Loosely(t, deps.tjNotifier.updateScheduled, should.Match(common.TryjobIDs{2}))
				}
				t.Run("For the first time", func(t *ftt.Test) {
					rs.CreateTime = ct.Clock.Now().Add(-tryjobRefreshInterval - time.Second)
					rs.LatestTryjobsRefresh = time.Time{}
					verifyScheduled()
				})
				t.Run("For the second (and later) time", func(t *ftt.Test) {
					rs.LatestTryjobsRefresh = ct.Clock.Now().Add(-tryjobRefreshInterval - time.Second)
					verifyScheduled()
				})

				t.Run("Skip if external id is not present", func(t *ftt.Test) {
					execution := rs.Tryjobs.GetState().GetExecutions()[0]
					tryjob.LatestAttempt(execution).ExternalId = ""
					_, err := h.Poke(ctx, rs)
					assert.NoErr(t, err)
					assert.Loosely(t, deps.tjNotifier.updateScheduled, should.BeEmpty)
				})

				t.Run("Skip if tryjob is not in Triggered status", func(t *ftt.Test) {
					execution := rs.Tryjobs.GetState().GetExecutions()[0]
					tryjob.LatestAttempt(execution).Status = tryjob.Status_ENDED
					_, err := h.Poke(ctx, rs)
					assert.NoErr(t, err)
					assert.Loosely(t, deps.tjNotifier.updateScheduled, should.BeEmpty)
				})
			})
		})
	})
}
