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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
)

func TestOnTryjobsUpdated(t *testing.T) {
	t.Parallel()

	ftt.Run("OnTryjobsUpdated", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

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

		t.Run("Enqueue longop", func(t *ftt.Test) {
			res, err := h.OnTryjobsUpdated(ctx, rs, common.MakeTryjobIDs(456, 789, 456))
			assert.NoErr(t, err)
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			assert.Loosely(t, res.State.OngoingLongOps.GetOps(), should.HaveLength(1))
			for _, op := range res.State.OngoingLongOps.GetOps() {
				assert.Loosely(t, op, should.Match(&run.OngoingLongOps_Op{
					Deadline: timestamppb.New(ct.Clock.Now().UTC().Add(maxTryjobExecutorDuration)),
					Work: &run.OngoingLongOps_Op_ExecuteTryjobs{
						ExecuteTryjobs: &tryjob.ExecuteTryjobsPayload{
							TryjobsUpdated: []int64{456, 789}, // Also deduped 456
						},
					},
				}))
			}
		})

		t.Run("Defer if an tryjob execute task is ongoing", func(t *ftt.Test) {
			enqueueTryjobsUpdatedTask(ctx, rs, common.TryjobIDs{123})
			res, err := h.OnTryjobsUpdated(ctx, rs, common.MakeTryjobIDs(456, 789, 456))
			assert.NoErr(t, err)
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeTrue)
		})

		statuses := []run.Status{
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
			run.Status_WAITING_FOR_SUBMISSION,
			run.Status_SUBMITTING,
		}
		for _, status := range statuses {
			t.Run(fmt.Sprintf("Noop when Run is %s", status), func(t *ftt.Test) {
				rs.Status = status
				res, err := h.OnTryjobsUpdated(ctx, rs, common.TryjobIDs{123})
				assert.NoErr(t, err)
				assert.Loosely(t, res.State, should.Equal(rs))
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			})
		}
	})
}

func TestOnCompletedExecuteTryjobs(t *testing.T) {
	t.Parallel()

	ftt.Run("OnCompletedExecuteTryjobs works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

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
			assert.Loosely(t, datastore.Put(ctx,
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
			), should.BeNil)
		}

		result := &eventpb.LongOpCompleted{
			OperationId: opID,
		}
		h, _ := makeTestHandler(&ct)

		for _, longOpStatus := range []eventpb.LongOpCompleted_Status{
			eventpb.LongOpCompleted_EXPIRED,
			eventpb.LongOpCompleted_FAILED,
		} {
			t.Run(fmt.Sprintf("on long op %s", longOpStatus), func(t *ftt.Test) {
				result.Status = longOpStatus
				res, err := h.OnLongOpCompleted(ctx, rs, result)
				assert.NoErr(t, err)
				assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
				assert.Loosely(t, res.State.OngoingLongOps.GetOps(), should.HaveLength(1))
				for _, op := range res.State.OngoingLongOps.GetOps() {
					assert.Loosely(t, op.GetResetTriggers(), should.NotBeNil)
					assert.Loosely(t, op.GetResetTriggers().GetRunStatusIfSucceeded(), should.Equal(run.Status_FAILED))
				}
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			})
		}

		t.Run("on long op success", func(t *ftt.Test) {
			result.Status = eventpb.LongOpCompleted_SUCCEEDED

			t.Run("Tryjob execution still running", func(t *ftt.Test) {
				err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return tryjob.SaveExecutionState(ctx, rs.ID, &tryjob.ExecutionState{
						Status: tryjob.ExecutionState_RUNNING,
					}, 0, nil)
				}, nil)
				assert.NoErr(t, err)
				res, err := h.OnLongOpCompleted(ctx, rs, result)
				assert.NoErr(t, err)
				assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
				assert.Loosely(t, res.State.Tryjobs, should.Match(&run.Tryjobs{
					State: &tryjob.ExecutionState{
						Status: tryjob.ExecutionState_RUNNING,
					},
				}))
				assert.Loosely(t, res.State.OngoingLongOps, should.BeNil)
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			})

			t.Run("Tryjob execution succeeds", func(t *ftt.Test) {
				err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return tryjob.SaveExecutionState(ctx, rs.ID, &tryjob.ExecutionState{
						Status: tryjob.ExecutionState_SUCCEEDED,
					}, 0, nil)
				}, nil)
				assert.NoErr(t, err)
				t.Run("Full Run", func(t *ftt.Test) {
					rs.Mode = run.FullRun
					ctx = context.WithValue(ctx, &fakeTaskIDKey, "task-foo")
					res, err := h.OnLongOpCompleted(ctx, rs, result)
					assert.NoErr(t, err)
					assert.Loosely(t, res.State.Status, should.Equal(run.Status_SUBMITTING))
					assert.Loosely(t, res.State.Tryjobs, should.Match(&run.Tryjobs{
						State: &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_SUCCEEDED,
						},
					}))
					assert.Loosely(t, res.State.OngoingLongOps, should.BeNil)
					assert.Loosely(t, res.State.Submission, should.NotBeNil)
					assert.Loosely(t, submit.MustCurrentRun(ctx, lProject), should.Equal(rs.ID))
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
				})
				t.Run("Dry Run", func(t *ftt.Test) {
					rs.Mode = run.DryRun
					res, err := h.OnLongOpCompleted(ctx, rs, result)
					assert.NoErr(t, err)
					assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
					assert.Loosely(t, res.State.Tryjobs, should.Match(&run.Tryjobs{
						State: &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_SUCCEEDED,
						},
					}))
					assert.Loosely(t, res.State.OngoingLongOps.GetOps(), should.HaveLength(1))
					for _, op := range res.State.OngoingLongOps.GetOps() {
						assert.Loosely(t, op.GetResetTriggers(), should.NotBeNil)
						assert.Loosely(t, op.GetResetTriggers().GetRequests(), should.Match([]*run.OngoingLongOps_Op_ResetTriggers_Request{
							{
								Clid:    int64(rs.CLs[0]),
								Message: "This CL has passed the run",
								Notify: gerrit.Whoms{
									gerrit.Whom_OWNER,
									gerrit.Whom_CQ_VOTERS,
								},
							},
						}))
						assert.Loosely(t, op.GetResetTriggers().GetRunStatusIfSucceeded(), should.Equal(run.Status_SUCCEEDED))
					}
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
				})
				t.Run("New Patchset Run, no message is posted", func(t *ftt.Test) {
					rs.Mode = run.NewPatchsetRun
					res, err := h.OnLongOpCompleted(ctx, rs, result)
					assert.NoErr(t, err)
					assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
					assert.Loosely(t, res.State.Tryjobs, should.Match(&run.Tryjobs{
						State: &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_SUCCEEDED,
						},
					}))
					assert.Loosely(t, res.State.OngoingLongOps.GetOps(), should.HaveLength(1))
					for _, op := range res.State.OngoingLongOps.GetOps() {
						assert.Loosely(t, op.GetResetTriggers(), should.NotBeNil)
						assert.Loosely(t, op.GetResetTriggers().GetRequests(), should.Match([]*run.OngoingLongOps_Op_ResetTriggers_Request{
							{Clid: int64(rs.CLs[0])},
						}))
						assert.Loosely(t, op.GetResetTriggers().GetRunStatusIfSucceeded(), should.Equal(run.Status_SUCCEEDED))
					}
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
				})
			})

			t.Run("Tryjob execution failed", func(t *ftt.Test) {
				t.Run("tryjobs result failure", func(t *ftt.Test) {
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
					assert.Loosely(t, datastore.Put(ctx, tj), should.BeNil)

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
					assert.NoErr(t, err)
					res, err := h.OnLongOpCompleted(ctx, rs, result)
					assert.NoErr(t, err)
					assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
					assert.Loosely(t, res.State.Tryjobs, should.Match(&run.Tryjobs{
						State: &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_FAILED,
							Failures: &tryjob.ExecutionState_Failures{
								UnsuccessfulResults: []*tryjob.ExecutionState_Failures_UnsuccessfulResult{
									{TryjobId: int64(tj.ID)},
								},
							},
						},
					}))
					assert.Loosely(t, res.State.OngoingLongOps.GetOps(), should.HaveLength(1))
					for _, op := range res.State.OngoingLongOps.GetOps() {
						assert.Loosely(t, op.GetResetTriggers(), should.NotBeNil)
						assert.Loosely(t, op.GetResetTriggers().GetRequests(), should.Match([]*run.OngoingLongOps_Op_ResetTriggers_Request{
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
						}))
						assert.Loosely(t, op.GetResetTriggers().GetRunStatusIfSucceeded(), should.Equal(run.Status_FAILED))
					}
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
				})

				t.Run("failed to launch tryjobs", func(t *ftt.Test) {
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
					assert.NoErr(t, err)
					res, err := h.OnLongOpCompleted(ctx, rs, result)
					assert.NoErr(t, err)
					assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
					assert.Loosely(t, res.State.Tryjobs, should.Match(&run.Tryjobs{
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
					}))
					assert.Loosely(t, res.State.OngoingLongOps.GetOps(), should.HaveLength(1))
					for _, op := range res.State.OngoingLongOps.GetOps() {
						assert.Loosely(t, op.GetResetTriggers(), should.NotBeNil)
						assert.Loosely(t, op.GetResetTriggers().GetRequests(), should.Match([]*run.OngoingLongOps_Op_ResetTriggers_Request{
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
						}))
						assert.Loosely(t, op.GetResetTriggers().GetRunStatusIfSucceeded(), should.Equal(run.Status_FAILED))
					}
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
				})

				t.Run("Unknown error", func(t *ftt.Test) {
					err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return tryjob.SaveExecutionState(ctx, rs.ID, &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_FAILED,
						}, 0, nil)
					}, nil)
					assert.NoErr(t, err)
					res, err := h.OnLongOpCompleted(ctx, rs, result)
					assert.NoErr(t, err)
					assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
					assert.Loosely(t, res.State.Tryjobs, should.Match(&run.Tryjobs{
						State: &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_FAILED,
						},
					}))
					assert.Loosely(t, res.State.OngoingLongOps.GetOps(), should.HaveLength(1))
					for _, op := range res.State.OngoingLongOps.GetOps() {
						assert.Loosely(t, op.GetResetTriggers(), should.NotBeNil)
						assert.Loosely(t, op.GetResetTriggers().GetRequests(), should.Match([]*run.OngoingLongOps_Op_ResetTriggers_Request{
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
						}))
						assert.Loosely(t, op.GetResetTriggers().GetRunStatusIfSucceeded(), should.Equal(run.Status_FAILED))
					}
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					assert.Loosely(t, res.PreserveEvents, should.BeFalse)
				})

				t.Run("Only reset trigger on root CL", func(t *ftt.Test) {
					err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return tryjob.SaveExecutionState(ctx, rs.ID, &tryjob.ExecutionState{
							Status: tryjob.ExecutionState_FAILED,
						}, 0, nil)
					}, nil)
					assert.NoErr(t, err)
					anotherCL := &run.RunCL{
						ID:         1002,
						Run:        datastore.MakeKey(ctx, common.RunKind, string(rs.ID)),
						ExternalID: changelist.MustGobID(gHost, 1002),
						Detail: &changelist.Snapshot{
							LuciProject: lProject,
							Kind: &changelist.Snapshot_Gerrit{
								Gerrit: &changelist.Gerrit{
									Host: gHost,
									Info: gf.CI(1002),
								},
							},
							Patchset: 2,
						},
					}
					assert.Loosely(t, datastore.Put(ctx, anotherCL), should.BeNil)
					rs.CLs = append(rs.CLs, anotherCL.ID)

					rs.RootCL = anotherCL.ID
					res, err := h.OnLongOpCompleted(ctx, rs, result)
					assert.NoErr(t, err)
					assert.Loosely(t, res.State.OngoingLongOps.GetOps(), should.HaveLength(1))
					for _, op := range res.State.OngoingLongOps.GetOps() {
						assert.Loosely(t, op.GetResetTriggers(), should.NotBeNil)
						assert.Loosely(t, op.GetResetTriggers().GetRequests(), should.Match([]*run.OngoingLongOps_Op_ResetTriggers_Request{
							{
								Clid:    int64(anotherCL.ID),
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
						}))
						assert.Loosely(t, op.GetResetTriggers().GetRunStatusIfSucceeded(), should.Equal(run.Status_FAILED))
					}
				})
			})
		})
	})
}

func TestComposeTryjobsFailureReason(t *testing.T) {
	ftt.Run("ComposeTryjobsFailureReason", t, func(t *ftt.Test) {
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
		t.Run("panics", func(t *ftt.Test) {
			assert.Loosely(t, func() {
				_ = composeTryjobsResultFailureReason(cl, nil)
			}, should.PanicLike("called without tryjobs"))
		})
		const bbHost = "test.com"
		builder := &bbpb.BuilderID{
			Project: "test_proj",
			Bucket:  "test_bucket",
			Builder: "test_builder",
		}
		t.Run("works", func(t *ftt.Test) {
			t.Run("single", func(t *ftt.Test) {
				t.Run("restricted", func(t *ftt.Test) {
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
					assert.Loosely(t, r, should.Equal("[Tryjob](https://test.com/build/123456790) has failed"))
				})
				t.Run("not restricted", func(t *ftt.Test) {
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
					assert.Loosely(t, r, should.Equal("Tryjob [test_proj/test_bucket/test_builder](https://test.com/build/123456790) has failed with summary ([view all results](https://example.review.com/c/123?checksPatchset=2&tab=checks)):\n\n---\nA couple\nof lines\nwith public details"))
				})
			})

			t.Run("multiple tryjobs", func(t *ftt.Test) {
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

				t.Run("all public visibility", func(t *ftt.Test) {
					r := composeTryjobsResultFailureReason(cl, tjs[1:])
					assert.Loosely(t, r, should.Equal("Failed Tryjobs:\n* [test_proj/test_bucket/test_builder](https://test.com/build/123456791)\n* [test_proj/test_bucket/test_builder](https://test.com/build/123456792). Summary ([view all results](https://example.review.com/c/123?checksPatchset=2&tab=checks)):\n\n---\nA couple\nof lines\nwith public details\n\n---"))
				})

				t.Run("with one restricted visibility", func(t *ftt.Test) {
					r := composeTryjobsResultFailureReason(cl, tjs)
					assert.Loosely(t, r, should.Equal("Failed Tryjobs:\n* https://test.com/build/123456790\n* https://test.com/build/123456791\n* https://test.com/build/123456792"))
				})
			})
		})
	})
}

func TestComposeLaunchFailureReason(t *testing.T) {
	ftt.Run("Compose Launch Failure Reason", t, func(t *ftt.Test) {
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
		t.Run("Single", func(t *ftt.Test) {
			t.Run("restricted", func(t *ftt.Test) {
				defFoo.ResultVisibility = cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED
				reason := composeLaunchFailureReason(
					[]*tryjob.ExecutionState_Failures_LaunchFailure{
						{Definition: defFoo, Reason: "permission denied"},
					})
				assert.Loosely(t, reason, should.Equal("Failed to launch one tryjob. The tryjob name can't be shown due to configuration. Please contact your Project admin for help."))
			})
			t.Run("public", func(t *ftt.Test) {
				reason := composeLaunchFailureReason(
					[]*tryjob.ExecutionState_Failures_LaunchFailure{
						{Definition: defFoo, Reason: "permission denied"},
					})
				assert.Loosely(t, reason, should.Equal("Failed to launch tryjob `ProjectFoo/BucketFoo/BuilderFoo`. Reason: permission denied"))
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
		t.Run("Multiple", func(t *ftt.Test) {
			t.Run("All restricted", func(t *ftt.Test) {
				defFoo.ResultVisibility = cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED
				defBar.ResultVisibility = cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED
				reason := composeLaunchFailureReason(
					[]*tryjob.ExecutionState_Failures_LaunchFailure{
						{Definition: defFoo, Reason: "permission denied"},
						{Definition: defBar, Reason: "builder not found"},
					})
				assert.Loosely(t, reason, should.Equal("Failed to launch 2 tryjobs. The tryjob names can't be shown due to configuration. Please contact your Project admin for help."))
			})
			t.Run("Partial restricted", func(t *ftt.Test) {
				defBar.ResultVisibility = cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED
				reason := composeLaunchFailureReason(
					[]*tryjob.ExecutionState_Failures_LaunchFailure{
						{Definition: defFoo, Reason: "permission denied"},
						{Definition: defBar, Reason: "builder not found"},
					})
				assert.Loosely(t, reason, should.Equal("Failed to launch the following tryjobs:\n* `ProjectFoo/BucketFoo/BuilderFoo`; Failure reason: permission denied\n\nIn addition to the tryjobs above, failed to launch 1 tryjob. But the tryjob names can't be shown due to configuration. Please contact your Project admin for help."))
			})
			t.Run("All public", func(t *ftt.Test) {
				reason := composeLaunchFailureReason(
					[]*tryjob.ExecutionState_Failures_LaunchFailure{
						{Definition: defFoo, Reason: "permission denied"},
						{Definition: defBar, Reason: "builder not found"},
					})
				assert.Loosely(t, reason, should.Equal("Failed to launch the following tryjobs:\n* `ProjectBar/BucketBar/BuilderBar`; Failure reason: builder not found\n* `ProjectFoo/BucketFoo/BuilderFoo`; Failure reason: permission denied"))
			})
		})
	})
}
