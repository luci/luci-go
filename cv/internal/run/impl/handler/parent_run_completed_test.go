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
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	apipb "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/runtest"
)

func TestOnParentRunCompleted(t *testing.T) {
	t.Parallel()

	ftt.Run("OnParentRunCompleted", t, func(t *ftt.Test) {
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

		rid := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef"))

		cl := changelist.CL{
			ID:             clid,
			EVersion:       3,
			IncompleteRuns: common.RunIDs{rid},
			UpdateTime:     ct.Clock.Now().UTC(),
		}
		assert.NoErr(t, datastore.Put(ctx, &cl))

		rs := &state.RunState{
			Run: run.Run{
				ID:            rid,
				Status:        run.Status_RUNNING,
				ConfigGroupID: cg.ID,
				CreateTime:    ct.Clock.Now().Add(-2 * time.Minute),
				StartTime:     ct.Clock.Now().Add(-1 * time.Minute),
				Mode:          run.DryRun,
				CLs:           common.CLIDs{clid},
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
		h, _ := makeTestHandler(&ct)

		success1 := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("parentSuccess"))
		success1Run := run.Run{
			ID:     success1,
			Status: run.Status_SUCCEEDED,
		}

		success2 := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("cafecafe"))
		success2Run := run.Run{
			ID:     success2,
			Status: run.Status_SUCCEEDED,
		}

		failed := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("cow"))
		failedRun := run.Run{
			ID:     failed,
			Status: run.Status_FAILED,
		}

		running := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("running"))
		runningRun := run.Run{
			ID:     running,
			Status: run.Status_RUNNING,
		}

		assert.NoErr(t, datastore.Put(ctx, &success1Run, &success2Run, &failedRun, &runningRun))

		t.Run("All parents successful, should submit", func(t *ftt.Test) {
			rs.DepRuns = common.RunIDs{success1, success2}
			rs.Status = run.Status_WAITING_FOR_SUBMISSION

			res, err := h.OnParentRunCompleted(ctx, rs)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State, should.Equal(rs))
			assert.Loosely(t, res.SideEffectFn, should.NotBeNil)
			assert.Loosely(t, res.SideEffectFn(ctx), should.BeNil)
			runtest.AssertReceivedReadyForSubmission(t, ctx, rs.ID, time.Time{})
		})
		t.Run("All parents successful but run not ready for submission", func(t *ftt.Test) {
			rs.DepRuns = common.RunIDs{success1, success2}
			rs.Status = run.Status_RUNNING

			res, err := h.OnParentRunCompleted(ctx, rs)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State, should.Equal(rs))
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
		})
		t.Run("One parent failed, should cancel", func(t *ftt.Test) {
			rs.DepRuns = common.RunIDs{success1, failed}
			rs.Status = run.Status_WAITING_FOR_SUBMISSION

			res, err := h.OnParentRunCompleted(ctx, rs)
			assert.NoErr(t, err)
			longOp := res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]]
			resetOp := longOp.GetResetTriggers()
			assert.Loosely(t, resetOp.Requests, should.HaveLength(1))
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			t.Run("Reset trigger on root CL only", func(t *ftt.Test) {
				rs.CLs = append(rs.CLs, clid+1000)
				rs.RootCL = clid
				res, err := h.OnParentRunCompleted(ctx, rs)
				assert.NoErr(t, err)
				longOp := res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]]
				resetOp := longOp.GetResetTriggers()
				assert.Loosely(t, resetOp.Requests, should.HaveLength(1))
				assert.Loosely(t, resetOp.Requests[0].Clid, should.Equal(rs.RootCL))
			})
		})
		t.Run("One parent not done, should not submit", func(t *ftt.Test) {
			rs.DepRuns = common.RunIDs{success1, running}
			rs.Status = run.Status_WAITING_FOR_SUBMISSION

			res, err := h.OnParentRunCompleted(ctx, rs)
			assert.NoErr(t, err)
			assert.Loosely(t, res.State, should.Equal(rs))
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
		})
	})
}
