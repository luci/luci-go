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

	bbpb "go.chromium.org/luci/buildbucket/proto"
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
				Convey("tryjobs result failure", func() {
					tj := tryjob.MustBuildbucketID("example.com", 12345).MustCreateIfNotExists(ctx)
					tj.Result = &tryjob.Result{
						Backend: &tryjob.Result_Buildbucket_{
							Buildbucket: &tryjob.Result_Buildbucket{
								Builder: &bbpb.BuilderID{
									Project: lProject,
									Bucket:  "test",
									Builder: "foo",
								},
								SummaryMarkdown: "this is the summary",
							},
						},
					}
					So(datastore.Put(ctx, tj), ShouldBeNil)

					err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return tryjob.SaveExecutionState(ctx, rs.ID, &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_FAILED,
							Failures: &tryjob.ExecutionState_Failures{
								UnsuccessfulResults: []*tryjob.ExecutionState_Failures_UnsuccessfulResult{
									{TryjobId: int64(tj.ID)},
								},
							},
						}, 0, nil)
					}, nil)
					So(err, ShouldBeNil)
					res, err := h.OnLongOpCompleted(ctx, rs, result)
					So(err, ShouldBeNil)
					So(res.State.Status, ShouldEqual, run.Status_RUNNING)
					So(res.State.Tryjobs, ShouldResembleProto, &run.Tryjobs{
						State: &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_FAILED,
							Failures: &tryjob.ExecutionState_Failures{
								UnsuccessfulResults: []*tryjob.ExecutionState_Failures_UnsuccessfulResult{
									{TryjobId: int64(tj.ID)},
								},
							},
						},
					})
					So(res.State.OngoingLongOps.GetOps(), ShouldHaveLength, 1)
					for _, op := range res.State.OngoingLongOps.GetOps() {
						So(op.GetResetTriggers(), ShouldNotBeNil)
						So(op.GetResetTriggers().GetRequests(), ShouldResembleProto, []*run.OngoingLongOps_Op_ResetTriggers_Request{
							{
								Clid:    int64(rs.CLs[0]),
								Message: "This CL has failed the run. Reason:\n\nTryjob [chromium/test/foo](https://example.com/build/12345) has failed with summary ([view all results](https://example-review.googlesource.com/c/101?checksPatchset=2&tab=checks)):\n\n---\nthis is the summary",
								Notify: gerrit.Whoms{
									gerrit.Whom_OWNER,
									gerrit.Whom_CQ_VOTERS,
								},
								AddToAttention: gerrit.Whoms{
									gerrit.Whom_OWNER,
									gerrit.Whom_CQ_VOTERS,
								},
								AddToAttentionReason: "Tryjobs failed",
							},
						})
						So(op.GetResetTriggers().GetRunStatusIfSucceeded(), ShouldEqual, run.Status_FAILED)
					}
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
				})

				Convey("failed to launch tryjobs", func() {
					def := &tryjob.Definition{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host: "example.com",
								Builder: &bbpb.BuilderID{
									Project: lProject,
									Bucket:  "test",
									Builder: "foo",
								},
							},
						},
					}
					err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return tryjob.SaveExecutionState(ctx, rs.ID, &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_FAILED,
							Failures: &tryjob.ExecutionState_Failures{
								LaunchFailures: []*tryjob.ExecutionState_Failures_LaunchFailure{
									{
										Definition: def,
										Reason:     "permission denied",
									},
								},
							},
						}, 0, nil)
					}, nil)
					So(err, ShouldBeNil)
					res, err := h.OnLongOpCompleted(ctx, rs, result)
					So(err, ShouldBeNil)
					So(res.State.Status, ShouldEqual, run.Status_RUNNING)
					So(res.State.Tryjobs, ShouldResembleProto, &run.Tryjobs{
						State: &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_FAILED,
							Failures: &tryjob.ExecutionState_Failures{
								LaunchFailures: []*tryjob.ExecutionState_Failures_LaunchFailure{
									{
										Definition: def,
										Reason:     "permission denied",
									},
								},
							},
						},
					})
					So(res.State.OngoingLongOps.GetOps(), ShouldHaveLength, 1)
					for _, op := range res.State.OngoingLongOps.GetOps() {
						So(op.GetResetTriggers(), ShouldNotBeNil)
						So(op.GetResetTriggers().GetRequests(), ShouldResembleProto, []*run.OngoingLongOps_Op_ResetTriggers_Request{
							{
								Clid:    int64(rs.CLs[0]),
								Message: "Failed to launch tryjob `chromium/test/foo`. Reason: permission denied",
								Notify: gerrit.Whoms{
									gerrit.Whom_OWNER,
									gerrit.Whom_CQ_VOTERS,
								},
								AddToAttention: gerrit.Whoms{
									gerrit.Whom_OWNER,
									gerrit.Whom_CQ_VOTERS,
								},
								AddToAttentionReason: "Tryjobs failed",
							},
						})
						So(op.GetResetTriggers().GetRunStatusIfSucceeded(), ShouldEqual, run.Status_FAILED)
					}
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
				})

				Convey("Unknown error", func() {
					err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return tryjob.SaveExecutionState(ctx, rs.ID, &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_FAILED,
						}, 0, nil)
					}, nil)
					So(err, ShouldBeNil)
					res, err := h.OnLongOpCompleted(ctx, rs, result)
					So(err, ShouldBeNil)
					So(res.State.Status, ShouldEqual, run.Status_RUNNING)
					So(res.State.Tryjobs, ShouldResembleProto, &run.Tryjobs{
						State: &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_FAILED,
						},
					})
					So(res.State.OngoingLongOps.GetOps(), ShouldHaveLength, 1)
					for _, op := range res.State.OngoingLongOps.GetOps() {
						So(op.GetResetTriggers(), ShouldNotBeNil)
						So(op.GetResetTriggers().GetRequests(), ShouldResembleProto, []*run.OngoingLongOps_Op_ResetTriggers_Request{
							{
								Clid:    int64(rs.CLs[0]),
								Message: "Unexpected error when processing Tryjobs. Please retry. If retry continues to fail, please contact LUCI team.\n\n" + cvBugLink,
								Notify: gerrit.Whoms{
									gerrit.Whom_OWNER,
									gerrit.Whom_CQ_VOTERS,
								},
								AddToAttention: gerrit.Whoms{
									gerrit.Whom_OWNER,
									gerrit.Whom_CQ_VOTERS,
								},
								AddToAttentionReason: "Run failed",
							},
						})
						So(op.GetResetTriggers().GetRunStatusIfSucceeded(), ShouldEqual, run.Status_FAILED)
					}
					So(res.SideEffectFn, ShouldBeNil)
					So(res.PreserveEvents, ShouldBeFalse)
				})
			})
		})
	})
}

func TestComposeTryjobsFailureReason(t *testing.T) {
	Convey("ComposeTryjobsFailureReason", t, func() {
		cl := &run.RunCL{
			ID:         1111,
			ExternalID: changelist.MustGobID("example.review.com", 123),
			Detail: &changelist.Snapshot{
				LuciProject: "test",
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Host: "example.review.com",
					},
				},
				Patchset: 2,
			},
		}
		Convey("panics", func() {
			So(func() {
				_ = composeTryjobsResultFailureReason(cl, nil)
			}, ShouldPanicLike, "called without tryjobs")
		})
		const bbHost = "test.com"
		builder := &bbpb.BuilderID{
			Project: "test_proj",
			Bucket:  "test_bucket",
			Builder: "test_builder",
		}
		Convey("works", func() {
			Convey("single", func() {
				Convey("restricted", func() {
					r := composeTryjobsResultFailureReason(cl, []*tryjob.Tryjob{
						{
							ExternalID: tryjob.MustBuildbucketID(bbHost, 123456790),
							Definition: &tryjob.Definition{
								Backend: &tryjob.Definition_Buildbucket_{
									Buildbucket: &tryjob.Definition_Buildbucket{
										Builder: builder,
										Host:    bbHost,
									},
								},
								ResultVisibility: cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED,
							},
							Result: &tryjob.Result{
								Backend: &tryjob.Result_Buildbucket_{
									Buildbucket: &tryjob.Result_Buildbucket{
										Builder:         builder,
										SummaryMarkdown: "A couple\nof lines\nwith secret details",
									},
								},
							},
						},
					})
					So(r, ShouldEqual, "[Tryjob](https://test.com/build/123456790) has failed")
				})
				Convey("not restricted", func() {
					r := composeTryjobsResultFailureReason(cl, []*tryjob.Tryjob{
						{
							ExternalID: tryjob.MustBuildbucketID(bbHost, 123456790),
							Definition: &tryjob.Definition{
								Backend: &tryjob.Definition_Buildbucket_{
									Buildbucket: &tryjob.Definition_Buildbucket{
										Builder: builder,
										Host:    bbHost,
									},
								},
								ResultVisibility: cfgpb.CommentLevel_COMMENT_LEVEL_FULL,
							},
							Result: &tryjob.Result{
								Backend: &tryjob.Result_Buildbucket_{
									Buildbucket: &tryjob.Result_Buildbucket{
										Builder:         builder,
										SummaryMarkdown: "A couple\nof lines\nwith public details",
									},
								},
							},
						},
					})
					So(r, ShouldEqual, "Tryjob [test_proj/test_bucket/test_builder](https://test.com/build/123456790) has failed with summary ([view all results](https://example.review.com/c/123?checksPatchset=2&tab=checks)):\n\n---\nA couple\nof lines\nwith public details")
				})
			})

			Convey("multiple tryjobs", func() {
				tjs := []*tryjob.Tryjob{
					// restricted.
					{
						ExternalID: tryjob.MustBuildbucketID("test.com", 123456790),
						Definition: &tryjob.Definition{
							Backend: &tryjob.Definition_Buildbucket_{
								Buildbucket: &tryjob.Definition_Buildbucket{
									Builder: builder,
									Host:    bbHost,
								},
							},
							ResultVisibility: cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED,
						},
						Result: &tryjob.Result{
							Backend: &tryjob.Result_Buildbucket_{
								Buildbucket: &tryjob.Result_Buildbucket{
									Builder:         builder,
									SummaryMarkdown: "A couple\nof lines\nwith secret details",
								},
							},
						},
					},
					// un-restricted but empty summary markdown.
					{
						ExternalID: tryjob.MustBuildbucketID("test.com", 123456791),
						Definition: &tryjob.Definition{
							Backend: &tryjob.Definition_Buildbucket_{
								Buildbucket: &tryjob.Definition_Buildbucket{
									Builder: builder,
									Host:    bbHost,
								},
							},
							ResultVisibility: cfgpb.CommentLevel_COMMENT_LEVEL_FULL,
						},
						Result: &tryjob.Result{
							Backend: &tryjob.Result_Buildbucket_{
								Buildbucket: &tryjob.Result_Buildbucket{
									Builder: builder,
								},
							},
						},
					},
					// un-restricted.
					{
						ExternalID: tryjob.MustBuildbucketID("test.com", 123456792),
						Definition: &tryjob.Definition{
							Backend: &tryjob.Definition_Buildbucket_{
								Buildbucket: &tryjob.Definition_Buildbucket{
									Builder: builder,
									Host:    bbHost,
								},
							},
							ResultVisibility: cfgpb.CommentLevel_COMMENT_LEVEL_FULL,
						},
						Result: &tryjob.Result{
							Backend: &tryjob.Result_Buildbucket_{
								Buildbucket: &tryjob.Result_Buildbucket{
									SummaryMarkdown: "A couple\nof lines\nwith public details",
								},
							},
						},
					},
				}

				Convey("all public visibility", func() {
					r := composeTryjobsResultFailureReason(cl, tjs[1:])
					So(r, ShouldEqual, "Failed Tryjobs:\n* [test_proj/test_bucket/test_builder](https://test.com/build/123456791)\n* [test_proj/test_bucket/test_builder](https://test.com/build/123456792). Summary ([view all results](https://example.review.com/c/123?checksPatchset=2&tab=checks)):\n\n---\nA couple\nof lines\nwith public details\n\n---")
				})

				Convey("with one restricted visibility", func() {
					r := composeTryjobsResultFailureReason(cl, tjs)
					So(r, ShouldEqual, "Failed Tryjobs:\n* https://test.com/build/123456790\n* https://test.com/build/123456791\n* https://test.com/build/123456792")
				})
			})
		})
	})
}

func TestComposeLaunchFailureReason(t *testing.T) {
	Convey("Compose Launch Failure Reason", t, func() {
		defFoo := &tryjob.Definition{
			Backend: &tryjob.Definition_Buildbucket_{
				Buildbucket: &tryjob.Definition_Buildbucket{
					Host: "buildbucket.example.com",
					Builder: &bbpb.BuilderID{
						Project: "ProjectFoo",
						Bucket:  "BucketFoo",
						Builder: "BuilderFoo",
					},
				},
			},
		}
		Convey("Single", func() {
			Convey("restricted", func() {
				defFoo.ResultVisibility = cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED
				reason := composeLaunchFailureReason(
					[]*tryjob.ExecutionState_Failures_LaunchFailure{
						{Definition: defFoo, Reason: "permission denied"},
					})
				So(reason, ShouldEqual, "Failed to launch one tryjob. The tryjob name can't be shown due to configuration. Please contact your Project admin for help.")
			})
			Convey("public", func() {
				reason := composeLaunchFailureReason(
					[]*tryjob.ExecutionState_Failures_LaunchFailure{
						{Definition: defFoo, Reason: "permission denied"},
					})
				So(reason, ShouldEqual, "Failed to launch tryjob `ProjectFoo/BucketFoo/BuilderFoo`. Reason: permission denied")
			})
		})
		defBar := &tryjob.Definition{
			Backend: &tryjob.Definition_Buildbucket_{
				Buildbucket: &tryjob.Definition_Buildbucket{
					Host: "buildbucket.example.com",
					Builder: &bbpb.BuilderID{
						Project: "ProjectBar",
						Bucket:  "BucketBar",
						Builder: "BuilderBar",
					},
				},
			},
		}
		Convey("Multiple", func() {
			Convey("All restricted", func() {
				defFoo.ResultVisibility = cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED
				defBar.ResultVisibility = cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED
				reason := composeLaunchFailureReason(
					[]*tryjob.ExecutionState_Failures_LaunchFailure{
						{Definition: defFoo, Reason: "permission denied"},
						{Definition: defBar, Reason: "builder not found"},
					})
				So(reason, ShouldEqual, "Failed to launch 2 tryjobs. The tryjob names can't be shown due to configuration. Please contact your Project admin for help.")
			})
			Convey("Partial restricted", func() {
				defBar.ResultVisibility = cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED
				reason := composeLaunchFailureReason(
					[]*tryjob.ExecutionState_Failures_LaunchFailure{
						{Definition: defFoo, Reason: "permission denied"},
						{Definition: defBar, Reason: "builder not found"},
					})
				So(reason, ShouldEqual, "Failed to launch the following tryjobs:\n* `ProjectFoo/BucketFoo/BuilderFoo`; Failure reason: permission denied\n\nIn addition to the tryjobs above, failed to launch 1 tryjob. But the tryjob names can't be shown due to configuration. Please contact your Project admin for help.")
			})
			Convey("All public", func() {
				reason := composeLaunchFailureReason(
					[]*tryjob.ExecutionState_Failures_LaunchFailure{
						{Definition: defFoo, Reason: "permission denied"},
						{Definition: defBar, Reason: "builder not found"},
					})
				So(reason, ShouldEqual, "Failed to launch the following tryjobs:\n* `ProjectBar/BucketBar/BuilderBar`; Failure reason: builder not found\n* `ProjectFoo/BucketFoo/BuilderFoo`; Failure reason: permission denied")
			})
		})
	})
}
