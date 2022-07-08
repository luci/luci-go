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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/tree"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/runtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPoke(t *testing.T) {
	t.Parallel()

	Convey("Poke", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
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
		So(srvcfg.SetTestMigrationConfig(ctx, &migrationpb.Settings{
			ApiHosts: []*migrationpb.Settings_ApiHost{
				{
					Host:          ct.Env.LogicalHostname,
					Prod:          true,
					ProjectRegexp: []string{".*"},
				},
			},
			UseCvTryjobExecutor: &migrationpb.Settings_UseCVTryjobExecutor{
				ProjectRegexp: []string{lProject},
			},
		}), ShouldBeNil)
		h, deps := makeTestHandler(&ct)

		rid := common.MakeRunID(lProject, ct.Clock.Now(), gChange, []byte("deadbeef"))
		rs := &state.RunState{
			Run: run.Run{
				ID:                  rid,
				CreateTime:          ct.Clock.Now().UTC().Add(-2 * time.Minute),
				StartTime:           ct.Clock.Now().UTC().Add(-1 * time.Minute),
				CLs:                 common.CLIDs{gChange},
				ConfigGroupID:       prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
				UseCVTryjobExecutor: true,
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
		rcl := &run.RunCL{
			ID:      gChange,
			Run:     datastore.MakeKey(ctx, common.RunKind, string(rid)),
			Detail:  cl.Snapshot,
			Trigger: trigger.Find(ci, cfg.GetConfigGroups()[0]),
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
						Deadline:          timestamppb.New(now.Add(submissionDuration)),
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
					So(res.State, ShouldNotEqual, rs)
					So(res.State.LatestCLsRefresh, ShouldResemble, datastore.RoundTime(ct.Clock.Now().UTC()))
					So(deps.clUpdater.refreshedCLs.Contains(1), ShouldBeTrue)
				}
				Convey("For the first time", func() {
					rs.CreateTime = ct.Clock.Now().Add(-clRefreshInterval - time.Second)
					rs.LatestCLsRefresh = time.Time{}
					verifyScheduled()
				})
				Convey("For the (n+1)-th time", func() {
					rs.LatestCLsRefresh = ct.Clock.Now().Add(-clRefreshInterval - time.Second)
					verifyScheduled()
				})
			})
			Convey("Run fails if no longer eligible", func() {
				rs.LatestCLsRefresh = ct.Clock.Now().Add(-clRefreshInterval - time.Second)
				ct.ResetMockedAuthDB(ctx)

				// verify that it did not schedule refresh but CancelTrigger.
				res, err := h.Poke(ctx, rs)
				So(err, ShouldBeNil)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldBeNil)
				So(res.State.Status, ShouldEqual, rs.Status)
				So(deps.clUpdater.refreshedCLs, ShouldBeEmpty)

				longOp := res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]]
				cancelOp := longOp.GetCancelTriggers()
				So(cancelOp.Requests, ShouldHaveLength, 1)
				So(cancelOp.Requests[0], ShouldResembleProto,
					&run.OngoingLongOps_Op_TriggersCancellation_Request{
						Clid:    int64(gChange),
						Message: "CV cannot start a Run for `foo@example.com` because the user is not a dry-runner.",
						Notify: []run.OngoingLongOps_Op_TriggersCancellation_Whom{
							run.OngoingLongOps_Op_TriggersCancellation_OWNER,
							run.OngoingLongOps_Op_TriggersCancellation_CQ_VOTERS,
						},
						AddToAttention: []run.OngoingLongOps_Op_TriggersCancellation_Whom{
							run.OngoingLongOps_Op_TriggersCancellation_OWNER,
							run.OngoingLongOps_Op_TriggersCancellation_CQ_VOTERS,
						},
						AddToAttentionReason: "CQ/CV Run failed",
					},
				)
				So(cancelOp.RunStatusIfSucceeded, ShouldEqual, run.Status_FAILED)
			})
		})

		Convey("Check UseCVTryjobExecutor", func() {
			Convey("Skip if UseCVTryjobExecutor is false", func() {
				rs.UseCVTryjobExecutor = false
				res, err := h.Poke(ctx, rs)
				So(err, ShouldBeNil)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldBeNil)
				So(res.State.UseCVTryjobExecutor, ShouldBeFalse)
			})

			Convey("Skip if has ExecuteTryjobs long op", func() {
				enqueueTryjobsUpdatedTask(ctx, rs, common.TryjobIDs{123})
				So(srvcfg.SetTestMigrationConfig(ctx, &migrationpb.Settings{
					ApiHosts: []*migrationpb.Settings_ApiHost{
						{
							Host:          ct.Env.LogicalHostname,
							Prod:          true,
							ProjectRegexp: []string{".*"},
						},
					},
					UseCvTryjobExecutor: &migrationpb.Settings_UseCVTryjobExecutor{
						ProjectRegexpExclude: []string{lProject},
					},
				}), ShouldBeNil)
				res, err := h.Poke(ctx, rs)
				So(err, ShouldBeNil)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldBeNil)
				So(res.State.UseCVTryjobExecutor, ShouldBeTrue)
			})

			Convey("Change UseCVTryjobExecutor to false", func() {
				So(srvcfg.SetTestMigrationConfig(ctx, &migrationpb.Settings{
					ApiHosts: []*migrationpb.Settings_ApiHost{
						{
							Host:          ct.Env.LogicalHostname,
							Prod:          true,
							ProjectRegexp: []string{".*"},
						},
					},
					UseCvTryjobExecutor: &migrationpb.Settings_UseCVTryjobExecutor{
						ProjectRegexpExclude: []string{lProject},
					},
				}), ShouldBeNil)
				res, err := h.Poke(ctx, rs)
				So(err, ShouldBeNil)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldBeNil)
				So(res.State.UseCVTryjobExecutor, ShouldBeFalse)
			})
		})
	})
}
