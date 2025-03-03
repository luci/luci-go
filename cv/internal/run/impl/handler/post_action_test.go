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

package handler

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestOnCompletedPostAction(t *testing.T) {
	t.Parallel()

	ftt.Run("onCompletedPostAction", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		const (
			lProject = "chromium"
			opID     = "1-1"
		)
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{{Name: "single"}}})
		opPayload := &run.OngoingLongOps_Op_ExecutePostActionPayload{
			Name: "label-vote",
			Kind: &run.OngoingLongOps_Op_ExecutePostActionPayload_ConfigAction{
				ConfigAction: &cfgpb.ConfigGroup_PostAction{
					Name: "label-vote",
					Action: &cfgpb.ConfigGroup_PostAction_VoteGerritLabels_{
						VoteGerritLabels: &cfgpb.ConfigGroup_PostAction_VoteGerritLabels{},
					},
				},
			},
		}
		opResult := &eventpb.LongOpCompleted{
			OperationId: opID,
			Result: &eventpb.LongOpCompleted_ExecutePostAction{
				ExecutePostAction: &eventpb.LongOpCompleted_ExecutePostActionResult{},
			},
		}
		rs := &state.RunState{
			Run: run.Run{
				ID:            lProject + "/1111111111111-1-deadbeef",
				Status:        run.Status_SUCCEEDED,
				Mode:          run.DryRun,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
				OngoingLongOps: &run.OngoingLongOps{
					Ops: map[string]*run.OngoingLongOps_Op{
						opID: {
							Work: &run.OngoingLongOps_Op_ExecutePostAction{
								ExecutePostAction: opPayload,
							},
						},
					},
				},
			},
		}
		h, _ := makeTestHandler(&ct)
		var err error
		var res *Result

		outerCheck := func(t testing.TB) {
			t.Helper()
			assert.Loosely(t, res.State.OngoingLongOps, should.BeNil, truth.LineContext())
			assert.Loosely(t, res.SideEffectFn, should.BeNil, truth.LineContext())
			assert.Loosely(t, res.PreserveEvents, should.BeFalse, truth.LineContext())
		}

		t.Run("execution summary is set", func(t *ftt.Test) {
			opResult.Status = eventpb.LongOpCompleted_CANCELLED
			opResult.GetExecutePostAction().Summary = "this is a summary"
			res, err = h.OnLongOpCompleted(ctx, rs, opResult)
			assert.NoErr(t, err)
			assert.That(t, res.State.LogEntries[0].GetInfo(), should.Match(&run.LogEntry_Info{
				Label:   "PostAction[label-vote]",
				Message: "this is a summary",
			}))
			outerCheck(t)
		})

		t.Run("execution summary is not set", func(t *ftt.Test) {
			var expected string

			innerCheck := func(t testing.TB) {
				t.Helper()
				res, err = h.OnLongOpCompleted(ctx, rs, opResult)
				assert.Loosely(t, err, should.BeNil, truth.LineContext())
				assert.That(t, res.State.LogEntries[0].GetInfo(), should.Match(&run.LogEntry_Info{
					Label:   "PostAction[label-vote]",
					Message: expected,
				}), truth.LineContext())

				outerCheck(t)
			}

			t.Run("the op succeeded", func(t *ftt.Test) {
				opResult.Status = eventpb.LongOpCompleted_SUCCEEDED
				expected = "the execution succeeded"
				innerCheck(t)
			})
			t.Run("the op expired", func(t *ftt.Test) {
				opResult.Status = eventpb.LongOpCompleted_EXPIRED
				expected = "the execution deadline was exceeded"
				innerCheck(t)
			})
			t.Run("the op cancelled", func(t *ftt.Test) {
				opResult.Status = eventpb.LongOpCompleted_CANCELLED
				expected = "the execution was cancelled"
				innerCheck(t)
			})
			t.Run("the op failed", func(t *ftt.Test) {
				opResult.Status = eventpb.LongOpCompleted_FAILED
				expected = "the execution failed"
				innerCheck(t)
			})
		})
	})
}

func TestShouldCreditRunQuotaOnPostAction(t *testing.T) {
	t.Parallel()

	ftt.Run("shouldCreditRunQuota", t, func(t *ftt.Test) {
		execState := &tryjob.ExecutionState{
			Requirement: &tryjob.Requirement{},
		}
		rs := &state.RunState{
			Run: run.Run{
				ID:     "chromium/1111111111111-1-deadbeef",
				Status: run.Status_FAILED,
				Mode:   run.DryRun,
				Tryjobs: &run.Tryjobs{
					State: execState,
				},
			},
		}

		addExecution := func(isCritical bool, tid common.TryjobID, status tryjob.Status) {
			execState.GetRequirement().Definitions = append(
				execState.GetRequirement().Definitions,
				&tryjob.Definition{Critical: isCritical},
			)
			execState.Executions = append(
				execState.Executions,
				&tryjob.ExecutionState_Execution{
					Attempts: []*tryjob.ExecutionState_Execution_Attempt{
						{
							TryjobId: int64(tid),
							Status:   status,
						},
					},
				},
			)
		}
		addNonCritical := func(tid common.TryjobID, status tryjob.Status) {
			addExecution(false, tid, status)
		}
		addCritical := func(tid common.TryjobID, status tryjob.Status) {
			addExecution(true, tid, status)
		}

		t.Run("returns true if no critical jobs are running", func(t *ftt.Test) {
			rs.Status = run.Status_FAILED
			addNonCritical(1, tryjob.Status_PENDING)
			addNonCritical(2, tryjob.Status_ENDED)
			addCritical(3, tryjob.Status_ENDED)
			addCritical(4, tryjob.Status_ENDED)

			assert.That(t, shouldCreditRunQuota(rs), should.BeTrue)
		})

		t.Run("returns false if the run ended but critical tryjobs are running", func(t *ftt.Test) {
			rs.Status = run.Status_FAILED
			addCritical(1, tryjob.Status_TRIGGERED)
			addCritical(2, tryjob.Status_ENDED)
			assert.That(t, shouldCreditRunQuota(rs), should.BeFalse)
		})

		t.Run("returns false if the run is still running", func(t *ftt.Test) {
			rs.Status = run.Status_RUNNING
			// even if all tryjobs are ended
			addCritical(1, tryjob.Status_ENDED)
			addCritical(2, tryjob.Status_ENDED)
			assert.That(t, shouldCreditRunQuota(rs), should.BeFalse)
		})

		t.Run("returns false if NewPatchsetRun", func(t *ftt.Test) {
			rs.Mode = run.NewPatchsetRun
			rs.Status = run.Status_FAILED
			addCritical(1, tryjob.Status_ENDED)
			addCritical(2, tryjob.Status_ENDED)
			assert.That(t, shouldCreditRunQuota(rs), should.BeFalse)
		})
	})
}
