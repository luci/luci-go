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

	"go.chromium.org/luci/common/errors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/impl/submit"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOnTryjobsUpdated(t *testing.T) {
	t.Parallel()

	Convey("OnTryjobsUpdated", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		const lProject = "infra"

		now := ct.Clock.Now().UTC()
		rid := common.MakeRunID(lProject, now, 1, []byte("deadbeef"))
		rs := &state.RunState{
			Run: run.Run{
				ID:         rid,
				Status:     run.Status_RUNNING,
				CreateTime: now.Add(-2 * time.Minute),
				StartTime:  now.Add(-1 * time.Minute),
				CLs:        common.CLIDs{1},
			},
		}
		h, _ := makeTestHandler(&ct)

		Convey("Enqueue longop", func() {
			res, err := h.OnTryjobsUpdated(ctx, rs, common.MakeTryjobIDs(456, 789, 456))
			So(err, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			So(res.State.OngoingLongOps.GetOps(), ShouldHaveLength, 1)
			for _, op := range res.State.OngoingLongOps.GetOps() {
				So(op, ShouldResembleProto, &run.OngoingLongOps_Op{
					Deadline: timestamppb.New(ct.Clock.Now().UTC().Add(maxTryjobExecutorDuration)),
					Work: &run.OngoingLongOps_Op_ExecuteTryjobs{
						ExecuteTryjobs: &tryjob.ExecuteTryjobsPayload{
							TryjobsUpdated: []int64{456, 789}, // Also deduped 456
						},
					},
				})
			}
		})

		Convey("Defer if an tryjob execute task is ongoing", func() {
			enqueueTryjobsUpdatedTask(ctx, rs, common.TryjobIDs{123})
			res, err := h.OnTryjobsUpdated(ctx, rs, common.MakeTryjobIDs(456, 789, 456))
			So(err, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeTrue)
		})

		statuses := []run.Status{
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
			run.Status_WAITING_FOR_SUBMISSION,
			run.Status_SUBMITTING,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				rs.Status = status
				res, err := h.OnTryjobsUpdated(ctx, rs, common.TryjobIDs{123})
				So(err, ShouldBeNil)
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
			})
		}
	})
}

func TestOnCompletedExecuteTryjobs(t *testing.T) {
	t.Parallel()

	Convey("OnCompletedExecuteTryjobs works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		const (
			lProject = "chromium"
			gHost    = "example-review.googlesource.com"
			opID     = "1-1"
		)
		now := ct.Clock.Now().UTC()

		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{{Name: "single"}}})
		rs := &state.RunState{
			Run: run.Run{
				ID:            common.MakeRunID(lProject, now, 1, []byte("deadbeef")),
				Status:        run.Status_RUNNING,
				CreateTime:    now.Add(-2 * time.Minute),
				StartTime:     now.Add(-1 * time.Minute),
				Mode:          run.DryRun,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
				CLs:           common.CLIDs{1},
				OngoingLongOps: &run.OngoingLongOps{
					Ops: map[string]*run.OngoingLongOps_Op{
						opID: {
							Work: &run.OngoingLongOps_Op_ExecuteTryjobs{
								ExecuteTryjobs: &tryjob.ExecuteTryjobsPayload{
									TryjobsUpdated: []int64{123},
								},
							},
						},
					},
				},
			},
		}

		for _, clid := range rs.CLs {
			ci := gf.CI(100 + int(clid))
			So(datastore.Put(ctx,
				&run.RunCL{
					ID:         clid,
					Run:        datastore.MakeKey(ctx, common.RunKind, string(rs.ID)),
					ExternalID: changelist.MustGobID(gHost, ci.GetNumber()),
					Detail: &changelist.Snapshot{
						LuciProject: lProject,
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host: gHost,
								Info: proto.Clone(ci).(*gerritpb.ChangeInfo),
							},
						},
						Patchset: 2,
					},
				},
			), ShouldBeNil)
		}

		result := &eventpb.LongOpCompleted{
			OperationId: opID,
		}
		h, _ := makeTestHandler(&ct)

		for _, longOpStatus := range []eventpb.LongOpCompleted_Status{
			eventpb.LongOpCompleted_EXPIRED,
			eventpb.LongOpCompleted_FAILED,
		} {
			Convey(fmt.Sprintf("on long op %s", longOpStatus), func() {
				result.Status = longOpStatus
				res, err := h.OnLongOpCompleted(ctx, rs, result)
				So(err, ShouldBeNil)
				So(res.State.Status, ShouldEqual, run.Status_RUNNING)
				So(res.State.OngoingLongOps.GetOps(), ShouldHaveLength, 1)
				for _, op := range res.State.OngoingLongOps.GetOps() {
					So(op.GetResetTriggers(), ShouldNotBeNil)
					So(op.GetResetTriggers().GetRunStatusIfSucceeded(), ShouldEqual, run.Status_FAILED)
				}
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
			})
		}

		Convey("on long op success", func() {
			result.Status = eventpb.LongOpCompleted_SUCCEEDED

			Convey("Tryjob execution still running", func() {
				err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return tryjob.SaveExecutionState(ctx, rs.ID, &tryjob.ExecutionState{
						Status: tryjob.ExecutionState_RUNNING,
					}, 0, nil)
				}, nil)
				So(err, ShouldBeNil)
				res, err := h.OnLongOpCompleted(ctx, rs, result)
				So(err, ShouldBeNil)
				So(res.State.Status, ShouldEqual, run.Status_RUNNING)
				So(res.State.Tryjobs, ShouldResembleProto, &run.Tryjobs{
					State: &tryjob.ExecutionState{
						Status: tryjob.ExecutionState_RUNNING,
					},
				})
				So(res.State.OngoingLongOps, ShouldBeNil)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
			})

			Convey("Tryjob execution succeeds", func() {
				err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return tryjob.SaveExecutionState(ctx, rs.ID, &tryjob.ExecutionState{
						Status: tryjob.ExecutionState_SUCCEEDED,
					}, 0, nil)
				}, nil)
				So(err, ShouldBeNil)
				Convey("Full Run", func() {
					rs.Mode = run.FullRun
					ctx = context.WithValue(ctx, &fakeTaskIDKey, "task-foo")
					res, err := h.OnLongOpCompleted(ctx, rs, result)
					So(err, ShouldBeNil)
					So(res.State.Status, ShouldEqual, run.Status_SUBMITTING)
					So(res.State.Tryjobs, ShouldResembleProto, &run.Tryjobs{
						State: &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_SUCCEEDED,
						},
					})
					So(res.State.OngoingLongOps, ShouldBeNil)
					So(res.State.Submission, ShouldNotBeNil)
					So(submit.MustCurrentRun(ctx, lProject), ShouldEqual, rs.ID)
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
				})
				Convey("Dry Run", func() {
					rs.Mode = run.DryRun
					res, err := h.OnLongOpCompleted(ctx, rs, result)
					So(err, ShouldBeNil)
					So(res.State.Status, ShouldEqual, run.Status_RUNNING)
					So(res.State.Tryjobs, ShouldResembleProto, &run.Tryjobs{
						State: &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_SUCCEEDED,
						},
					})
					So(res.State.OngoingLongOps.GetOps(), ShouldHaveLength, 1)
					for _, op := range res.State.OngoingLongOps.GetOps() {
						So(op.GetResetTriggers(), ShouldNotBeNil)
						So(op.GetResetTriggers().GetRequests(), ShouldResembleProto, []*run.OngoingLongOps_Op_ResetTriggers_Request{
							{
								Clid:    int64(rs.CLs[0]),
								Message: "This CL has passed the run",
								Notify: gerrit.Whoms{
									gerrit.Whom_OWNER,
									gerrit.Whom_CQ_VOTERS,
								},
							},
						})
						So(op.GetResetTriggers().GetRunStatusIfSucceeded(), ShouldEqual, run.Status_SUCCEEDED)
					}
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
				})
				Convey("New Patchset Run, no message is posted", func() {
					rs.Mode = run.NewPatchsetRun
					res, err := h.OnLongOpCompleted(ctx, rs, result)
					So(err, ShouldBeNil)
					So(res.State.Status, ShouldEqual, run.Status_RUNNING)
					So(res.State.Tryjobs, ShouldResembleProto, &run.Tryjobs{
						State: &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_SUCCEEDED,
						},
					})
					So(res.State.OngoingLongOps.GetOps(), ShouldHaveLength, 1)
					for _, op := range res.State.OngoingLongOps.GetOps() {
						So(op.GetResetTriggers(), ShouldNotBeNil)
						So(op.GetResetTriggers().GetRequests(), ShouldResembleProto, []*run.OngoingLongOps_Op_ResetTriggers_Request{
							{Clid: int64(rs.CLs[0])},
						})
						So(op.GetResetTriggers().GetRunStatusIfSucceeded(), ShouldEqual, run.Status_SUCCEEDED)
					}
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
				})
			})

			Convey("Tryjob execution failed", func() {
				var whoms gerrit.Whoms
				Convey("With no failure reason placeholders", func() {
					Convey("On CQ Vote Run", func() {
						rs.Mode = run.DryRun
						whoms = gerrit.Whoms{
							gerrit.Whom_OWNER,
							gerrit.Whom_CQ_VOTERS,
						}
					})
					Convey("On New Patchset Run", func() {
						rs.Mode = run.NewPatchsetRun
						whoms = gerrit.Whoms{
							gerrit.Whom_OWNER,
							gerrit.Whom_PS_UPLOADER,
						}
					})
					err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return tryjob.SaveExecutionState(ctx, rs.ID, &tryjob.ExecutionState{
							Status:            tryjob.ExecutionState_FAILED,
							FailureReasonTmpl: "build 12345 failed",
						}, 0, nil)
					}, nil)
					So(err, ShouldBeNil)
					res, err := h.OnLongOpCompleted(ctx, rs, result)
					So(err, ShouldBeNil)
					So(res.State.Status, ShouldEqual, run.Status_RUNNING)
					So(res.State.Tryjobs, ShouldResembleProto, &run.Tryjobs{
						State: &tryjob.ExecutionState{
							Status:            tryjob.ExecutionState_FAILED,
							FailureReasonTmpl: "build 12345 failed",
						},
					})
					So(res.State.OngoingLongOps.GetOps(), ShouldHaveLength, 1)
					for _, op := range res.State.OngoingLongOps.GetOps() {
						So(op.GetResetTriggers(), ShouldNotBeNil)
						So(op.GetResetTriggers().GetRequests(), ShouldResembleProto, []*run.OngoingLongOps_Op_ResetTriggers_Request{
							{
								Clid:                 int64(rs.CLs[0]),
								Message:              "This CL has failed the run. Reason:\n\nbuild 12345 failed",
								Notify:               whoms,
								AddToAttention:       whoms,
								AddToAttentionReason: "Tryjobs failed",
							},
						})
						So(op.GetResetTriggers().GetRunStatusIfSucceeded(), ShouldEqual, run.Status_FAILED)
					}
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
				})

				Convey("With TryJob results URL failure reason placeholder", func() {
					Convey("With LoadRunCLs success", func() {
						Convey("On CQ Vote Run", func() {
							rs.Mode = run.DryRun
							whoms = gerrit.Whoms{
								gerrit.Whom_OWNER,
								gerrit.Whom_CQ_VOTERS,
							}
						})
						Convey("On New Patchset Run", func() {
							rs.Mode = run.NewPatchsetRun
							whoms = gerrit.Whoms{
								gerrit.Whom_OWNER,
								gerrit.Whom_PS_UPLOADER,
							}
						})
						err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
							return tryjob.SaveExecutionState(ctx, rs.ID, &tryjob.ExecutionState{
								Status:            tryjob.ExecutionState_FAILED,
								FailureReasonTmpl: "build 12345 failed{{if .IsGerritCL}} {{.GerritChecksTabMDLink}}{{end}}",
							}, 0, nil)
						}, nil)
						So(err, ShouldBeNil)
						res, err := h.OnLongOpCompleted(ctx, rs, result)
						So(err, ShouldBeNil)
						So(res.State.Status, ShouldEqual, run.Status_RUNNING)
						So(res.State.Tryjobs, ShouldResembleProto, &run.Tryjobs{
							State: &tryjob.ExecutionState{
								Status:            tryjob.ExecutionState_FAILED,
								FailureReasonTmpl: "build 12345 failed{{if .IsGerritCL}} {{.GerritChecksTabMDLink}}{{end}}",
							},
						})
						So(res.State.OngoingLongOps.GetOps(), ShouldHaveLength, 1)
						for _, op := range res.State.OngoingLongOps.GetOps() {
							So(op.GetResetTriggers(), ShouldNotBeNil)
							So(op.GetResetTriggers().GetRequests(), ShouldResembleProto, []*run.OngoingLongOps_Op_ResetTriggers_Request{
								{
									Clid:                 int64(rs.CLs[0]),
									Message:              "This CL has failed the run. Reason:\n\nbuild 12345 failed [(view all results)](https://example-review.googlesource.com/c/101?checksPatchset=2&tab=checks)",
									Notify:               whoms,
									AddToAttention:       whoms,
									AddToAttentionReason: "Tryjobs failed",
								},
							})
							So(op.GetResetTriggers().GetRunStatusIfSucceeded(), ShouldEqual, run.Status_FAILED)
						}
						So(res.SideEffectFn, ShouldBeNil)
						So(res.PreserveEvents, ShouldBeFalse)
					})
					Convey("With LoadRunCLs error", func() {
						LoadCLs = func(ctx context.Context, runID common.RunID, clids common.CLIDs) ([]*run.RunCL, error) {
							return nil, errors.New("Failed")
						}
						Convey("On CQ Vote Run", func() {
							rs.Mode = run.DryRun
							whoms = gerrit.Whoms{
								gerrit.Whom_OWNER,
								gerrit.Whom_CQ_VOTERS,
							}
						})
						Convey("On New Patchset Run", func() {
							rs.Mode = run.NewPatchsetRun
							whoms = gerrit.Whoms{
								gerrit.Whom_OWNER,
								gerrit.Whom_PS_UPLOADER,
							}
						})
						err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
							return tryjob.SaveExecutionState(ctx, rs.ID, &tryjob.ExecutionState{
								Status:            tryjob.ExecutionState_FAILED,
								FailureReasonTmpl: "build 12345 failed{{if .IsGerritCL}} {{.GerritChecksTabMDLink}}{{end}}",
							}, 0, nil)
						}, nil)
						So(err, ShouldBeNil)
						res, err := h.OnLongOpCompleted(ctx, rs, result)
						So(res, ShouldBeNil)
						So(err, ShouldNotBeNil)
					})
				})
			})
		})
	})
}
