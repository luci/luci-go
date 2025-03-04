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

package longops

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/lease"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
)

func TestResetTriggers(t *testing.T) {
	t.Parallel()

	ftt.Run("ResetTriggers works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		mutator := changelist.NewMutator(ct.TQDispatcher, nil, nil, nil)

		const (
			lProject = "infra"
			gHost    = "g-review.example.com"
		)
		runCreateTime := clock.Now(ctx)
		runID := common.MakeRunID(lProject, runCreateTime, 1, []byte("deadbeef"))

		cfg := cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{Name: "test"},
			},
		}
		prjcfgtest.Create(ctx, lProject, &cfg)

		initRunAndCLs := func(cis []*gerritpb.ChangeInfo) (*run.Run, common.CLIDs) {
			clids := make(common.CLIDs, len(cis))
			cls := make([]*changelist.CL, len(cis))
			runCLs := make([]*run.RunCL, len(cis))
			for i, ci := range cis {
				assert.Loosely(t, ci.GetNumber(), should.BeGreaterThan(0))
				assert.Loosely(t, ci.GetNumber(), should.BeLessThan(1000))
				triggers := trigger.Find(&trigger.FindInput{ChangeInfo: ci, ConfigGroup: cfg.GetConfigGroups()[0]})
				assert.Loosely(t, ct.GFake.Has(gHost, int(ci.GetNumber())), should.BeFalse)
				ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject), ci))
				cl := changelist.MustGobID(gHost, ci.GetNumber()).MustCreateIfNotExists(ctx)
				cl.Snapshot = &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: ci,
					}},
					LuciProject:        lProject,
					ExternalUpdateTime: timestamppb.New(runCreateTime),
				}
				cl.EVersion++
				clids[i] = cl.ID
				runCLs[i] = &run.RunCL{
					ID:         cl.ID,
					ExternalID: cl.ExternalID,
					IndexedID:  cl.ID,
					Trigger:    triggers.GetCqVoteTrigger(),
					Run:        datastore.MakeKey(ctx, common.RunKind, string(runID)),
					Detail:     cl.Snapshot,
				}
				cls[i] = cl
			}
			r := &run.Run{
				ID:            runID,
				Status:        run.Status_RUNNING,
				CLs:           clids,
				Mode:          run.DryRun,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
			}
			assert.NoErr(t, datastore.Put(ctx, r, cls, runCLs))
			return r, clids
		}

		makeOp := func(r *run.Run) *ResetTriggersOp {
			reqs := make([]*run.OngoingLongOps_Op_ResetTriggers_Request, 0, len(r.CLs))
			for _, clid := range r.CLs {
				if r.HasRootCL() && clid != r.RootCL {
					continue
				}
				req := &run.OngoingLongOps_Op_ResetTriggers_Request{
					Clid:    int64(clid),
					Message: fmt.Sprintf("reset message for CL %d", clid),
					Notify: gerrit.Whoms{
						gerrit.Whom_OWNER,
						gerrit.Whom_REVIEWERS,
					},
					AddToAttention: gerrit.Whoms{
						gerrit.Whom_OWNER,
						gerrit.Whom_CQ_VOTERS,
					},
					AddToAttentionReason: fmt.Sprintf("attention reason for CL %d", clid),
				}
				reqs = append(reqs, req)
			}

			return &ResetTriggersOp{
				Base: &Base{
					Op: &run.OngoingLongOps_Op{
						Deadline:        timestamppb.New(clock.Now(ctx).Add(10000 * time.Hour)), // infinite
						CancelRequested: false,
						Work: &run.OngoingLongOps_Op_ResetTriggers_{
							ResetTriggers: &run.OngoingLongOps_Op_ResetTriggers{
								Requests: reqs,
							},
						},
					},
					IsCancelRequested: func() bool { return false },
					Run:               r,
				},
				GFactory:  ct.GFactory(),
				CLMutator: mutator,
			}
		}

		assertTriggerRemoved := func(eid changelist.ExternalID) {
			host, changeID, err := changelist.ExternalID(eid).ParseGobID()
			assert.NoErr(t, err)
			assert.Loosely(t, host, should.Equal(gHost))
			changeInfo := ct.GFake.GetChange(gHost, int(changeID)).Info
			assert.Loosely(t, trigger.Find(&trigger.FindInput{ChangeInfo: changeInfo, ConfigGroup: cfg.GetConfigGroups()[0]}), should.BeNil)
		}

		testHappyPath := func(prefix string, clCount, concurrency int) {
			t.Run(fmt.Sprintf("%s [%d CLs with concurrency %d]", prefix, clCount, concurrency), func(t *ftt.Test) {
				cis := make([]*gerritpb.ChangeInfo, clCount)
				for i := range cis {
					cis[i] = gf.CI(i+1, gf.CQ(+1), gf.Updated(runCreateTime.Add(-1*time.Minute)))
				}
				r, _ := initRunAndCLs(cis)
				startTime := clock.Now(ctx)
				op := makeOp(r)
				op.Concurrency = concurrency
				res, err := op.Do(ctx)
				assert.NoErr(t, err)
				assert.Loosely(t, res.GetStatus(), should.Equal(eventpb.LongOpCompleted_SUCCEEDED))
				results := res.GetResetTriggers().GetResults()
				assert.Loosely(t, results, should.HaveLength(clCount))
				processedCLIDs := make(common.CLIDsSet, clCount)
				for _, result := range results {
					assert.Loosely(t, processedCLIDs.HasI64(result.Id), should.BeFalse) // duplicate processing
					processedCLIDs.AddI64(result.Id)
					assertTriggerRemoved(changelist.ExternalID(result.ExternalId))
					assert.Loosely(t, result.GetSuccessInfo().GetResetAt().AsTime(), should.HappenOnOrAfter(startTime))
				}
				assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Internal.RunResetTriggerAttempted, lProject, "test", string(run.DryRun), true, "GERRIT_ERROR_NONE"), should.Equal(clCount))
			})
		}

		testHappyPath("single", 1, 1)
		testHappyPath("serial", 4, 1)
		testHappyPath("concurrent", 80, 8)

		t.Run("works when some cl doesn't have trigger", func(t *ftt.Test) {
			cis := []*gerritpb.ChangeInfo{
				gf.CI(1, gf.CQ(+1), gf.Updated(runCreateTime.Add(-1*time.Minute))),
				// 1 is the root CL so no trigger on 2
				gf.CI(2, gf.Updated(runCreateTime.Add(-1*time.Minute))),
			}
			r, clids := initRunAndCLs(cis)
			r.RootCL = clids[0]
			op := makeOp(r)
			res, err := op.Do(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, res.GetStatus(), should.Equal(eventpb.LongOpCompleted_SUCCEEDED))
			results := res.GetResetTriggers().GetResults()
			assert.Loosely(t, results, should.HaveLength(1))
			for _, result := range results {
				assert.Loosely(t, result.GetId(), should.Equal(clids[0]))
				assertTriggerRemoved(changelist.ExternalID(result.ExternalId))
			}
			t.Run("error if requesting to reset CL without trigger", func(t *ftt.Test) {
				op := makeOp(r)
				assert.Loosely(t, op.Op.GetResetTriggers().GetRequests(), should.HaveLength(1))
				// switch to the CL without trigger
				op.Op.GetResetTriggers().GetRequests()[0].Clid = int64(clids[1])
				_, err := op.Do(ctx)
				assert.ErrIsLike(t, err, "requested trigger reset on CL 2 that doesn't have trigger at all")
			})
		})

		t.Run("Retry on alreadyInLease failure", func(t *ftt.Test) {
			t.Skip("TODO(crbug/1297723): re-enable this test after fixing the flake.")

			// Creating changes from 1 to `clCount`, lease the CL with duration ==
			// change number * time.Minute.
			clCount := 6
			cis := make([]*gerritpb.ChangeInfo, clCount)
			for i := 1; i <= clCount; i++ {
				cis[i-1] = gf.CI(i, gf.CQ(+1), gf.Updated(runCreateTime.Add(-1*time.Minute)))
			}
			r, clids := initRunAndCLs(cis)
			for i, clid := range clids {
				_, _, err := lease.ApplyOnCL(ctx, clid, time.Duration(cis[i].GetNumber())*time.Minute, "FooBar")
				assert.NoErr(t, err)
			}
			startTime := clock.Now(ctx)
			op := makeOp(r)
			op.Concurrency = clCount
			op.testAfterTryResetFn = func() {
				// Advance the clock by 1 minute + 1 second so that the lease will
				// be guaranteed to expire in the next attempt.
				ct.Clock.Add(1*time.Minute + 1*time.Second)
			}
			res, err := op.Do(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, res.GetStatus(), should.Equal(eventpb.LongOpCompleted_SUCCEEDED))
			results := res.GetResetTriggers().GetResults()
			assert.Loosely(t, results, should.HaveLength(len(cis)))
			for i, result := range results {
				assert.Loosely(t, result.Id, should.Equal(clids[i]))
				assert.Loosely(t, result.GetSuccessInfo().GetResetAt().AsTime(), should.HappenAfter(startTime.Add(time.Duration(cis[i].GetNumber())*time.Minute)))
				assertTriggerRemoved(changelist.ExternalID(result.ExternalId))
			}
		})

		// TODO(crbug/1199880): test can retry transient failure once Gerrit fake
		// gain the flakiness mode.

		t.Run("Failed permanently for non-transient error", func(t *ftt.Test) {
			cis := []*gerritpb.ChangeInfo{
				gf.CI(1, gf.CQ(+1), gf.Updated(runCreateTime.Add(-1*time.Minute))),
				gf.CI(2, gf.CQ(+1), gf.Updated(runCreateTime.Add(-1*time.Minute))),
			}
			r, clids := initRunAndCLs(cis)
			ct.GFake.MutateChange(gHost, 2, func(c *gf.Change) {
				c.ACLs = gf.ACLReadOnly(lProject) // can't mutate
			})
			op := makeOp(r)
			startTime := clock.Now(ctx)
			res, err := op.Do(ctx)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, res.GetStatus(), should.Equal(eventpb.LongOpCompleted_FAILED))
			results := res.GetResetTriggers().GetResults()
			assert.Loosely(t, results, should.HaveLength(len(cis)))
			for _, result := range results {
				switch common.CLID(result.Id) {
				case clids[0]: // Change 1
					assert.Loosely(t, result.GetSuccessInfo().GetResetAt().AsTime(), should.HappenAfter(startTime))
				case clids[1]: // Change 2
					assert.Loosely(t, result.GetFailureInfo().GetFailureMessage(), should.NotBeEmpty)
				}
				assert.Loosely(t, result.ExternalId, should.NotBeEmpty)
			}
			assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Internal.RunResetTriggerAttempted, lProject, "test", string(run.DryRun), true, "GERRIT_ERROR_NONE"), should.Equal(1))
			assert.Loosely(t, ct.TSMonSentValue(ctx, metrics.Internal.RunResetTriggerAttempted, lProject, "test", string(run.DryRun), false, "PERMISSION_DENIED"), should.Equal(1))
		})

		t.Run("Doesn't obey long op cancellation", func(t *ftt.Test) {
			ci := gf.CI(1, gf.CQ(+1), gf.Updated(runCreateTime.Add(-1*time.Minute)))
			cis := []*gerritpb.ChangeInfo{ci}
			r, clids := initRunAndCLs(cis)
			op := makeOp(r)
			op.IsCancelRequested = func() bool { return true }
			res, err := op.Do(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, res.GetStatus(), should.Equal(eventpb.LongOpCompleted_SUCCEEDED))
			results := res.GetResetTriggers().GetResults()
			assert.Loosely(t, results, should.HaveLength(len(cis)))
			for i, result := range results {
				assert.Loosely(t, result.Id, should.Equal(clids[i]))
				assertTriggerRemoved(changelist.ExternalID(result.ExternalId))
				assert.Loosely(t, result.GetSuccessInfo().GetResetAt(), should.NotBeNil)
			}
		})
	})
}
