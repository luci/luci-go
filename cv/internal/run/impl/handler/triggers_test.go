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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

func TestOnCompletedResetTriggers(t *testing.T) {
	t.Parallel()

	ftt.Run("OnCompletedResetTriggers works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			lProject = "chromium"
			gHost    = "example-review.googlesource.com"
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
							Work: &run.OngoingLongOps_Op_ResetTriggers_{
								ResetTriggers: &run.OngoingLongOps_Op_ResetTriggers{
									Requests: []*run.OngoingLongOps_Op_ResetTriggers_Request{
										{Clid: 1},
									},
									RunStatusIfSucceeded: run.Status_SUCCEEDED,
								},
							},
						},
					},
				},
			},
		}
		result := &eventpb.LongOpCompleted{
			OperationId: opID,
		}
		now := ct.Clock.Now()
		h, _ := makeTestHandler(&ct)

		assertHasLogEntry := func(rs *state.RunState, target *run.LogEntry) {
			for _, le := range rs.LogEntries {
				if proto.Equal(target, le) {
					return
				}
			}
			assert.Loosely(t, fmt.Sprintf("log entry is missing: %s", target), should.BeEmpty)
		}

		t.Run("on expiration", func(t *ftt.Test) {
			result.Status = eventpb.LongOpCompleted_EXPIRED
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_FAILED))
			for _, op := range res.State.OngoingLongOps.GetOps() {
				if op.GetExecutePostAction() == nil {
					assert.Loosely(t, op.GetWork(), should.BeNil, truth.Explain(
						"should not contain any long op other than post action"))
				}
			}
			assertHasLogEntry(res.State, &run.LogEntry{
				Time: timestamppb.New(now),
				Kind: &run.LogEntry_Info_{
					Info: &run.LogEntry_Info{
						Label:   logEntryLabelResetTriggers,
						Message: fmt.Sprintf("failed to reset the triggers of CLs within the %s deadline", maxResetTriggersDuration),
					},
				},
			})
			assert.Loosely(t, res.SideEffectFn, should.NotBeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
		})

		t.Run("on failure", func(t *ftt.Test) {
			result.Status = eventpb.LongOpCompleted_FAILED
			result.Result = &eventpb.LongOpCompleted_ResetTriggers_{
				ResetTriggers: &eventpb.LongOpCompleted_ResetTriggers{
					Results: []*eventpb.LongOpCompleted_ResetTriggers_Result{
						{
							Id:         1,
							ExternalId: string(changelist.MustGobID(gHost, 111)),
							Detail: &eventpb.LongOpCompleted_ResetTriggers_Result_FailureInfo{
								FailureInfo: &eventpb.LongOpCompleted_ResetTriggers_Result_Failure{
									FailureMessage: "no permission to vote",
								},
							},
						},
					},
				},
			}
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_FAILED))
			for _, op := range res.State.OngoingLongOps.GetOps() {
				if op.GetExecutePostAction() == nil {
					assert.Loosely(t, op.GetWork(), should.BeNil, truth.Explain(
						"should not contain any long op other than post action"))
				}
			}
			assertHasLogEntry(res.State, &run.LogEntry{
				Time: timestamppb.New(now),
				Kind: &run.LogEntry_Info_{
					Info: &run.LogEntry_Info{
						Label:   logEntryLabelResetTriggers,
						Message: "failed to reset the trigger of change https://example-review.googlesource.com/c/111. Reason: no permission to vote",
					},
				},
			})
			assert.Loosely(t, res.SideEffectFn, should.NotBeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
		})

		t.Run("on success", func(t *ftt.Test) {
			result.Status = eventpb.LongOpCompleted_SUCCEEDED
			result.Result = &eventpb.LongOpCompleted_ResetTriggers_{
				ResetTriggers: &eventpb.LongOpCompleted_ResetTriggers{
					Results: []*eventpb.LongOpCompleted_ResetTriggers_Result{
						{
							Id:         1,
							ExternalId: string(changelist.MustGobID(gHost, 111)),
							Detail: &eventpb.LongOpCompleted_ResetTriggers_Result_SuccessInfo{
								SuccessInfo: &eventpb.LongOpCompleted_ResetTriggers_Result_Success{
									ResetAt: timestamppb.New(now.Add(-1 * time.Minute)),
								},
							},
						},
					},
				},
			}
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_SUCCEEDED))
			for _, op := range res.State.OngoingLongOps.GetOps() {
				if op.GetExecutePostAction() == nil {
					assert.Loosely(t, op.GetWork(), should.BeNil, truth.Explain(
						"should not contain any long op other than post action"))
				}
			}
			assertHasLogEntry(res.State, &run.LogEntry{
				Time: timestamppb.New(now.Add(-1 * time.Minute)),
				Kind: &run.LogEntry_Info_{
					Info: &run.LogEntry_Info{
						Label:   logEntryLabelResetTriggers,
						Message: "successfully reset the trigger of change https://example-review.googlesource.com/c/111",
					},
				},
			})
			assert.Loosely(t, res.SideEffectFn, should.NotBeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
		})

		t.Run("on partial failure", func(t *ftt.Test) {
			result.Status = eventpb.LongOpCompleted_FAILED
			rs.OngoingLongOps.GetOps()[opID].GetResetTriggers().Requests =
				[]*run.OngoingLongOps_Op_ResetTriggers_Request{
					{Clid: 1},
					{Clid: 2},
				}
			result.Result = &eventpb.LongOpCompleted_ResetTriggers_{
				ResetTriggers: &eventpb.LongOpCompleted_ResetTriggers{
					Results: []*eventpb.LongOpCompleted_ResetTriggers_Result{
						{
							Id:         1,
							ExternalId: string(changelist.MustGobID(gHost, 111)),
							Detail: &eventpb.LongOpCompleted_ResetTriggers_Result_SuccessInfo{
								SuccessInfo: &eventpb.LongOpCompleted_ResetTriggers_Result_Success{
									ResetAt: timestamppb.New(now.Add(-1 * time.Minute)),
								},
							},
						},
						{
							Id:         2,
							ExternalId: string(changelist.MustGobID(gHost, 222)),
							Detail: &eventpb.LongOpCompleted_ResetTriggers_Result_FailureInfo{
								FailureInfo: &eventpb.LongOpCompleted_ResetTriggers_Result_Failure{
									FailureMessage: "no permission to vote",
								},
							},
						},
					},
				},
			}
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_FAILED))
			for _, op := range res.State.OngoingLongOps.GetOps() {
				if op.GetExecutePostAction() == nil {
					assert.Loosely(t, op.GetWork(), should.BeNil, truth.Explain(
						"should not contain any long op other than post action"))
				}
			}
			assertHasLogEntry(res.State, &run.LogEntry{
				Time: timestamppb.New(now.Add(-1 * time.Minute)),
				Kind: &run.LogEntry_Info_{
					Info: &run.LogEntry_Info{
						Label:   logEntryLabelResetTriggers,
						Message: "successfully reset the trigger of change https://example-review.googlesource.com/c/111",
					},
				},
			})
			assertHasLogEntry(res.State, &run.LogEntry{
				Time: timestamppb.New(now),
				Kind: &run.LogEntry_Info_{
					Info: &run.LogEntry_Info{
						Label:   logEntryLabelResetTriggers,
						Message: "failed to reset the trigger of change https://example-review.googlesource.com/c/222. Reason: no permission to vote",
					},
				},
			})
			assert.Loosely(t, res.SideEffectFn, should.NotBeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)
		})
	})
}
