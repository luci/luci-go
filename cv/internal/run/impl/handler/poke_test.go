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
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/tree"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/runtest"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPoke(t *testing.T) {
	t.Parallel()

	Convey("Poke", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

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
							Url: "tree.example.com",
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
				Mode:          run.DryRun,
			},
		}

		ci := gf.CI(
			gChange, gf.PS(gPatchSet),
			gf.Owner("foo"),
			gf.CQ(+1, clock.Now(ctx).UTC(), gf.U("foo")),
		)
		ct.AddMember("foo", dryRunners)
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
		So(triggers.GetCqVoteTrigger(), ShouldResembleProto, &run.Trigger{
			Time:            timestamppb.New(clock.Now(ctx).UTC()),
			Mode:            string(run.DryRun),
			Email:           "foo@example.com",
			GerritAccountId: 1,
		})
		rcl := &run.RunCL{
			ID:      gChange,
			Run:     datastore.MakeKey(ctx, common.RunKind, string(rid)),
			Detail:  cl.Snapshot,
			Trigger: triggers.GetCqVoteTrigger(),
		}
		So(datastore.Put(ctx, cl, rcl), ShouldBeNil)

		now := ct.Clock.Now()
		ctx = context.WithValue(ctx, &fakeTaskIDKey, "task-foo")

		verifyNoOp := func() {
			res, err := h.Poke(ctx, rs)
			So(err, ShouldBeNil)
			So(res.State, cvtesting.SafeShouldResemble, rs)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			So(res.PostProcessFn, ShouldBeNil)
			So(deps.clUpdater.refreshedCLs, ShouldBeEmpty)
		}

		Convey("Tree checks", func() {
			Convey("Check Tree if condition matches", func() {
				// WAITING_FOR_SUBMISSION makes sense only for FullRun.
				// It's an error condition,
				// if run == DryRun, but status == WAITING_FOR_SUBMISSION.
				rs.Mode = run.FullRun
				rs.Status = run.Status_WAITING_FOR_SUBMISSION
				rs.Submission = &run.Submission{
					TreeOpen:          false,
					LastTreeCheckTime: timestamppb.New(now.Add(-1 * time.Minute)),
				}

				Convey("Open", func() {
					res, err := h.Poke(ctx, rs)
					So(err, ShouldBeNil)
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldNotBeNil)
					// proceed to submission right away
					So(res.State.Status, ShouldEqual, run.Status_SUBMITTING)
					So(res.State.Submission, ShouldResembleProto, &run.Submission{
						Deadline:          timestamppb.New(now.Add(defaultSubmissionDuration)),
						Cls:               []int64{gChange},
						TaskId:            "task-foo",
						TreeOpen:          true,
						LastTreeCheckTime: timestamppb.New(now),
					})
				})

				Convey("Close", func() {
					ct.TreeFake.ModifyState(ctx, tree.Closed)
					res, err := h.Poke(ctx, rs)
					So(err, ShouldBeNil)
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldBeNil)
					So(res.State.Status, ShouldEqual, run.Status_WAITING_FOR_SUBMISSION)
					// record the result and check again after 1 minute.
					So(res.State.Submission, ShouldResembleProto, &run.Submission{
						TreeOpen:          false,
						LastTreeCheckTime: timestamppb.New(now),
					})
					runtest.AssertReceivedPoke(ctx, rid, now.Add(1*time.Minute))
				})

				Convey("Failed", func() {
					ct.TreeFake.ModifyState(ctx, tree.StateUnknown)
					ct.TreeFake.InjectErr(fmt.Errorf("error retrieving tree status"))
					Convey("Not too long", func() {
						res, err := h.Poke(ctx, rs)
						So(err, ShouldBeNil)
						So(res.State.Status, ShouldEqual, run.Status_WAITING_FOR_SUBMISSION)
					})

					Convey("Too long", func() {
						rs.Submission.TreeErrorSince = timestamppb.New(now.Add(-11 * time.Minute))
						res, err := h.Poke(ctx, rs)
						So(err, ShouldBeNil)
						So(res.State, ShouldNotPointTo, rs)
						So(res.SideEffectFn, ShouldBeNil)
						So(res.PreserveEvents, ShouldBeFalse)
						So(res.PostProcessFn, ShouldBeNil)
						So(res.State.NewLongOpIDs, ShouldHaveLength, 1)
						ct := res.State.OngoingLongOps.Ops[res.State.NewLongOpIDs[0]].GetResetTriggers()
						So(ct.RunStatusIfSucceeded, ShouldEqual, run.Status_FAILED)
						So(ct.Requests, ShouldHaveLength, 1)
						So(ct.Requests[0].Message, ShouldContainSubstring, "Could not submit this CL because the tree status app at tree.example.com repeatedly returned failures")
						So(res.State.Status, ShouldEqual, run.Status_WAITING_FOR_SUBMISSION)
					})
				})
			})

			Convey("No-op if condition doesn't match", func() {
				Convey("Not in WAITING_FOR_SUBMISSION status", func() {
					rs.Status = run.Status_RUNNING
					verifyNoOp()
				})

				Convey("Tree is open in the previous check", func() {
					rs.Status = run.Status_WAITING_FOR_SUBMISSION
					rs.Submission = &run.Submission{
						TreeOpen:          true,
						LastTreeCheckTime: timestamppb.New(now.Add(-2 * time.Minute)),
					}
					verifyNoOp()
				})

				Convey("Last Tree check is too recent", func() {
					rs.Status = run.Status_WAITING_FOR_SUBMISSION
					rs.Submission = &run.Submission{
						TreeOpen:          false,
						LastTreeCheckTime: timestamppb.New(now.Add(-1 * time.Second)),
					}
					verifyNoOp()
				})
			})
		})

		Convey("CLs Refresh", func() {
			Convey("No-op if finalized", func() {
				rs.Status = run.Status_CANCELLED
				verifyNoOp()
			})
			Convey("No-op if recently created", func() {
				rs.CreateTime = ct.Clock.Now()
				rs.LatestCLsRefresh = time.Time{}
				verifyNoOp()
			})
			Convey("No-op if recently refreshed", func() {
				rs.LatestCLsRefresh = ct.Clock.Now().Add(-clRefreshInterval / 2)
				verifyNoOp()
			})
			Convey("Schedule refresh", func() {
				verifyScheduled := func() {
					res, err := h.Poke(ctx, rs)
					So(err, ShouldBeNil)
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldBeNil)
					So(res.State, ShouldNotPointTo, rs)
					So(res.State.LatestCLsRefresh, ShouldResemble, datastore.RoundTime(ct.Clock.Now().UTC()))
					So(deps.clUpdater.refreshedCLs.Contains(1), ShouldBeTrue)
				}
				Convey("For the first time", func() {
					rs.CreateTime = ct.Clock.Now().Add(-clRefreshInterval - time.Second)
					rs.LatestCLsRefresh = time.Time{}
					verifyScheduled()
				})
				Convey("For the second (and later) time", func() {
					rs.LatestCLsRefresh = ct.Clock.Now().Add(-clRefreshInterval - time.Second)
					verifyScheduled()
				})
			})
			Convey("Run fails if no longer eligible", func() {
				rs.LatestCLsRefresh = ct.Clock.Now().Add(-clRefreshInterval - time.Second)
				ct.ResetMockedAuthDB(ctx)

				// verify that it did not schedule refresh but reset triggers.
				res, err := h.Poke(ctx, rs)
				So(err, ShouldBeNil)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldBeNil)
				So(res.State.Status, ShouldEqual, rs.Status)
				So(deps.clUpdater.refreshedCLs, ShouldBeEmpty)

				longOp := res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]]
				resetOp := longOp.GetResetTriggers()
				So(resetOp.Requests, ShouldHaveLength, 1)
				So(resetOp.Requests[0], ShouldResembleProto,
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
				)
				So(resetOp.RunStatusIfSucceeded, ShouldEqual, run.Status_FAILED)
			})
		})

		Convey("Tryjobs Refresh", func() {
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
			Convey("No-op if finalized", func() {
				rs.Status = run.Status_CANCELLED
				verifyNoOp()
			})
			Convey("No-op if recently created", func() {
				rs.CreateTime = ct.Clock.Now()
				rs.LatestTryjobsRefresh = time.Time{}
				verifyNoOp()
			})
			Convey("No-op if recently refreshed", func() {
				rs.LatestTryjobsRefresh = ct.Clock.Now().Add(-tryjobRefreshInterval / 2)
				verifyNoOp()
			})
			Convey("Schedule refresh", func() {
				verifyScheduled := func() {
					res, err := h.Poke(ctx, rs)
					So(err, ShouldBeNil)
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
					So(res.PostProcessFn, ShouldBeNil)
					So(res.State, ShouldNotPointTo, rs)
					So(res.State.LatestTryjobsRefresh, ShouldEqual, datastore.RoundTime(ct.Clock.Now().UTC()))
					So(deps.tjNotifier.updateScheduled, ShouldResemble, common.TryjobIDs{2})
				}
				Convey("For the first time", func() {
					rs.CreateTime = ct.Clock.Now().Add(-tryjobRefreshInterval - time.Second)
					rs.LatestTryjobsRefresh = time.Time{}
					verifyScheduled()
				})
				Convey("For the second (and later) time", func() {
					rs.LatestTryjobsRefresh = ct.Clock.Now().Add(-tryjobRefreshInterval - time.Second)
					verifyScheduled()
				})

				Convey("Skip if external id is not present", func() {
					execution := rs.Tryjobs.GetState().GetExecutions()[0]
					tryjob.LatestAttempt(execution).ExternalId = ""
					_, err := h.Poke(ctx, rs)
					So(err, ShouldBeNil)
					So(deps.tjNotifier.updateScheduled, ShouldBeEmpty)
				})

				Convey("Skip if tryjob is not in Triggered status", func() {
					execution := rs.Tryjobs.GetState().GetExecutions()[0]
					tryjob.LatestAttempt(execution).Status = tryjob.Status_ENDED
					_, err := h.Poke(ctx, rs)
					So(err, ShouldBeNil)
					So(deps.tjNotifier.updateScheduled, ShouldBeEmpty)
				})
			})
		})
	})
}
