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

	"go.chromium.org/luci/common/clock"
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
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

func TestOnCLsUpdated(t *testing.T) {
	ftt.Run("OnCLsUpdated", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			lProject   = "chromium"
			gHost      = "x-review.example.com"
			committers = "committer-group"
			dryRunners = "dry-runner-group"
		)

		cfg := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					Verifiers: &cfgpb.Verifiers{
						GerritCqAbility: &cfgpb.Verifiers_GerritCQAbility{
							CommitterList:    []string{committers},
							DryRunAccessList: []string{dryRunners},
						},
					},
				},
			},
		}
		prjcfgtest.Create(ctx, lProject, cfg)
		h, _ := makeTestHandler(&ct)

		// initial state
		triggerTime := clock.Now(ctx).UTC()
		rs := &state.RunState{
			Run: run.Run{
				ID:            common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef")),
				StartTime:     triggerTime.Add(1 * time.Minute),
				Status:        run.Status_RUNNING,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
				CLs:           common.CLIDs{1},
				Mode:          run.DryRun,
			},
		}
		updateCL := func(clID common.CLID, ci *gerritpb.ChangeInfo, ap *changelist.ApplicableConfig, acc *changelist.Access) changelist.CL {
			cl := changelist.CL{
				ID:         clID,
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
				ApplicableConfig: ap,
				Access:           acc,
			}

			assert.Loosely(t, datastore.Put(ctx, &cl), should.BeNil)
			return cl
		}

		verifyHasResetTriggerLongOpScheduled := func(res *Result, expect map[common.CLID]string, endStatus run.Status) {
			// The status should be still RUNNING,
			// because it has not been cancelled yet.
			// It's scheduled to be cancelled.
			assert.Loosely(t, res.State.Status, should.Equal(run.Status_RUNNING))
			assert.Loosely(t, res.SideEffectFn, should.BeNil)
			assert.Loosely(t, res.PreserveEvents, should.BeFalse)

			longOp := res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]]
			cancelOp := longOp.GetResetTriggers()
			assert.Loosely(t, cancelOp.Requests, should.HaveLength(len(expect)))
			for _, req := range cancelOp.Requests {
				clid := common.CLID(req.Clid)
				assert.Loosely(t, expect, should.ContainKey(clid))
				assert.Loosely(t, req.Message, should.ContainSubstring(expect[clid]))
				delete(expect, clid)
			}
			assert.Loosely(t, expect, should.BeEmpty)
			assert.Loosely(t, cancelOp.RunStatusIfSucceeded, should.Equal(endStatus))
		}

		aplConfigOK := &changelist.ApplicableConfig{Projects: []*changelist.ApplicableConfig_Project{
			{Name: lProject, ConfigGroupIds: prjcfgtest.MustExist(ctx, lProject).ConfigGroupNames},
		}}
		accessOK := (*changelist.Access)(nil)

		const gChange1 = 1
		const gPatchSet1 = 5

		ci1 := gf.CI(
			gChange1, gf.PS(gPatchSet1),
			gf.Owner("foo"),
			gf.CQ(+2, triggerTime, gf.U("foo")),
			gf.Approve(),
		)
		ct.AddMember("foo", committers)
		cl1 := updateCL(1, ci1, aplConfigOK, accessOK)
		triggers1 := trigger.Find(&trigger.FindInput{ChangeInfo: ci1, ConfigGroup: cfg.GetConfigGroups()[0]})
		assert.Loosely(t, triggers1.GetCqVoteTrigger(), should.Resemble(&run.Trigger{
			Time:            timestamppb.New(triggerTime),
			Mode:            string(run.FullRun),
			Email:           "foo@example.com",
			GerritAccountId: 1,
		}))
		runCLs := []*run.RunCL{
			{
				ID:      1,
				Run:     datastore.MakeKey(ctx, common.RunKind, string(rs.ID)),
				Detail:  cl1.Snapshot,
				Trigger: triggers1.GetCqVoteTrigger(),
			},
		}
		assert.Loosely(t, runCLs[0].Trigger, should.NotBeNil) // ensure trigger find is working fine.
		assert.Loosely(t, datastore.Put(ctx, runCLs), should.BeNil)

		t.Run("Single CL Run", func(t *ftt.Test) {
			ensureNoop := func() {
				res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1})
				assert.NoErr(t, err)
				assert.Loosely(t, res.State, should.Resemble(rs))
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			}
			t.Run("Noop", func(t *ftt.Test) {
				statuses := []run.Status{
					run.Status_SUCCEEDED,
					run.Status_FAILED,
					run.Status_CANCELLED,
				}
				for _, status := range statuses {
					t.Run(fmt.Sprintf("When Run is %s", status), func(t *ftt.Test) {
						rs.Status = status
						ensureNoop()
					})
				}

				t.Run("When new CL Version", func(t *ftt.Test) {
					t.Run("is a message update", func(t *ftt.Test) {
						newCI1 := proto.Clone(ci1).(*gerritpb.ChangeInfo)
						gf.Messages(&gerritpb.ChangeMessageInfo{
							Message: "This is a message",
						})(newCI1)
						updateCL(1, newCI1, aplConfigOK, accessOK)
						ensureNoop()
					})

					t.Run("is triggered by different user at the exact same time", func(t *ftt.Test) {
						updateCL(1, gf.CI(
							gChange1, gf.PS(gPatchSet1),
							gf.CQ(+2, triggerTime, gf.U("bar")),
							gf.Approve(),
						), aplConfigOK, accessOK)
						ensureNoop()
					})
				})
			})
			t.Run("Preserve events for SUBMITTING Run", func(t *ftt.Test) {
				rs.Status = run.Status_SUBMITTING
				res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1})
				assert.NoErr(t, err)
				assert.Loosely(t, res.State, should.Resemble(rs))
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeTrue)
			})

			t.Run("Preserve events for if trigger reset is ongoing", func(t *ftt.Test) {
				rs.OngoingLongOps = &run.OngoingLongOps{
					Ops: map[string]*run.OngoingLongOps_Op{
						"op_id": {
							Work: &run.OngoingLongOps_Op_ResetTriggers_{
								ResetTriggers: &run.OngoingLongOps_Op_ResetTriggers{
									Requests: []*run.OngoingLongOps_Op_ResetTriggers_Request{
										{
											Clid:    1,
											Message: "no permission to Run",
										},
									},
								},
							},
						},
					},
				}
				res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1})
				assert.NoErr(t, err)
				assert.Loosely(t, res.State, should.Resemble(rs))
				assert.Loosely(t, res.SideEffectFn, should.BeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeTrue)
			})

			runAndVerifyCancelled := func(reason string) {
				res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1})
				assert.NoErr(t, err)
				assert.Loosely(t, res.State.Status, should.Equal(run.Status_CANCELLED))
				assert.Loosely(t, res.State.CancellationReasons, should.Resemble([]string{reason}))
				assert.Loosely(t, res.SideEffectFn, should.NotBeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
			}

			t.Run("Cancels Run on new Patchset", func(t *ftt.Test) {
				updateCL(1, gf.CI(gChange1, gf.PS(gPatchSet1+1), gf.CQ(+2, triggerTime, gf.U("foo"))), aplConfigOK, accessOK)
				runAndVerifyCancelled("the patchset of https://x-review.example.com/c/1 has changed from 5 to 6")
			})
			t.Run("Cancels Run on moved Ref", func(t *ftt.Test) {
				updateCL(1, gf.CI(gChange1, gf.PS(gPatchSet1), gf.CQ(+2, triggerTime, gf.U("foo")), gf.Ref("refs/heads/new")), aplConfigOK, accessOK)
				runAndVerifyCancelled("the ref of https://x-review.example.com/c/1 has moved from refs/heads/main to refs/heads/new")
			})
			t.Run("Cancels Run on removed trigger", func(t *ftt.Test) {
				newCI1 := gf.CI(gChange1, gf.PS(gPatchSet1), gf.CQ(0, triggerTime.Add(1*time.Minute), gf.U("foo")))
				assert.Loosely(t, trigger.Find(&trigger.FindInput{ChangeInfo: newCI1, ConfigGroup: cfg.GetConfigGroups()[0]}), should.BeNil)
				updateCL(1, newCI1, aplConfigOK, accessOK)
				runAndVerifyCancelled("the FULL_RUN trigger on https://x-review.example.com/c/1 has been removed")
			})
			t.Run("Cancels Run on changed mode", func(t *ftt.Test) {
				updateCL(1, gf.CI(gChange1, gf.PS(gPatchSet1), gf.CQ(+1, triggerTime.Add(1*time.Minute), gf.U("foo"))), aplConfigOK, accessOK)
				runAndVerifyCancelled("the triggering vote on https://x-review.example.com/c/1 has requested a different run mode: DRY_RUN")
			})
			t.Run("Cancels Run on change of triggering time", func(t *ftt.Test) {
				updateCL(1, gf.CI(gChange1, gf.PS(gPatchSet1), gf.CQ(+2, triggerTime.Add(2*time.Minute), gf.U("foo"))), aplConfigOK, accessOK)
				runAndVerifyCancelled(fmt.Sprintf("the timestamp of the triggering vote on https://x-review.example.com/c/1 has changed from %s to %s", triggerTime, triggerTime.Add(2*time.Minute)))
			})

			t.Run("Change of access level to the CL", func(t *ftt.Test) {
				t.Run("cancel if another project started watching the same CL", func(t *ftt.Test) {
					ac := proto.Clone(aplConfigOK).(*changelist.ApplicableConfig)
					ac.Projects = append(ac.Projects, &changelist.ApplicableConfig_Project{
						Name: "other-project", ConfigGroupIds: []string{"other-group"},
					})
					updateCL(1, ci1, ac, accessOK)
					runAndVerifyCancelled(fmt.Sprintf("no longer have access to https://x-review.example.com/c/1: watched not only by LUCI Project %q", lProject))
				})
				t.Run("wait if code review access was just lost, potentially due to eventual consistency", func(t *ftt.Test) {
					noAccessAt := ct.Clock.Now().Add(42 * time.Second)
					acc := &changelist.Access{ByProject: map[string]*changelist.Access_Project{
						// Set NoAccessTime to the future, providing some grace period to
						// recover.
						lProject: {NoAccessTime: timestamppb.New(noAccessAt)},
					}}
					updateCL(1, ci1, aplConfigOK, acc)
					res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1})
					assert.NoErr(t, err)
					assert.Loosely(t, res.State, should.Resemble(rs))
					assert.Loosely(t, res.SideEffectFn, should.BeNil)
					// Event must be preserved, s.t. the same CL is re-visited later.
					assert.Loosely(t, res.PreserveEvents, should.BeTrue)
					// And Run Manager must have a task to re-check itself at around
					// NoAccessTime.
					assert.Loosely(t, ct.TQ.Tasks().Payloads(), should.HaveLength(1))
					assert.Loosely(t, ct.TQ.Tasks().Payloads()[0].(*eventpb.ManageRunTask).GetRunId(), should.Resemble(string(rs.ID)))
					assert.Loosely(t, ct.TQ.Tasks()[0].ETA, should.HappenOnOrBetween(noAccessAt, noAccessAt.Add(time.Second)))
				})
				t.Run("cancel if code review access was lost a while ago", func(t *ftt.Test) {
					acc := &changelist.Access{ByProject: map[string]*changelist.Access_Project{
						lProject: {NoAccessTime: timestamppb.New(ct.Clock.Now())},
					}}
					updateCL(1, ci1, aplConfigOK, acc)
					runAndVerifyCancelled("no longer have access to https://x-review.example.com/c/1: code review site denied access")
				})
				t.Run("wait if access level is unknown", func(t *ftt.Test) {
					cl1.Snapshot = nil
					cl1.EVersion++
					assert.Loosely(t, datastore.Put(ctx, &cl1), should.BeNil)
					ensureNoop()
				})
			})

			t.Run("Schedules a ResetTrigger long op if the approval was revoked", func(t *ftt.Test) {
				updateCL(1, gf.CI(
					gChange1, gf.PS(gPatchSet1), gf.CQ(+2, triggerTime, gf.U("foo")), gf.Disapprove(),
				), aplConfigOK, accessOK)
				res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1})
				assert.NoErr(t, err)
				verifyHasResetTriggerLongOpScheduled(res, map[common.CLID]string{
					1: "CV cannot start a Run because this CL is not submittable.",
				}, run.Status_FAILED)
			})
		})

		t.Run("Multi CL Run", func(t *ftt.Test) {
			const gChange2 = 2
			const gPatchSet2 = 7
			ci2 := gf.CI(
				gChange2, gf.PS(gPatchSet2),
				gf.Owner("foo"),
				gf.CQ(+2, triggerTime, gf.U("foo")),
				gf.Approve(),
			)
			cl2 := updateCL(2, ci2, aplConfigOK, accessOK)
			triggers2 := trigger.Find(&trigger.FindInput{ChangeInfo: ci2, ConfigGroup: cfg.GetConfigGroups()[0]})
			assert.Loosely(t, triggers2.GetCqVoteTrigger(), should.Resemble(&run.Trigger{
				Time:            timestamppb.New(triggerTime),
				Mode:            string(run.FullRun),
				Email:           "foo@example.com",
				GerritAccountId: 1,
			}))
			rs.CLs = append(rs.CLs, 2)
			runCLs = append(runCLs, &run.RunCL{
				ID:      2,
				Run:     datastore.MakeKey(ctx, common.RunKind, string(rs.ID)),
				Detail:  cl2.Snapshot,
				Trigger: triggers2.GetCqVoteTrigger(),
			})
			assert.Loosely(t, runCLs[1].Trigger, should.NotBeNil) // ensure trigger find is working fine.
			assert.Loosely(t, datastore.Put(ctx, runCLs), should.BeNil)

			t.Run("Schedules a ResetTrigger long op", func(t *ftt.Test) {
				t.Run("Part of the CLs cause cancellation", func(t *ftt.Test) {
					updateCL(1, gf.CI(gChange1, gf.PS(gPatchSet1+1), gf.CQ(+2, triggerTime, gf.U("foo"))), aplConfigOK, accessOK)
					res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1})
					assert.NoErr(t, err)
					assert.Loosely(t, res.State.CancellationReasons, should.Resemble([]string{"the patchset of https://x-review.example.com/c/1 has changed from 5 to 6"}))
					verifyHasResetTriggerLongOpScheduled(res, map[common.CLID]string{
						2: "Reset the trigger of this CL because the patchset of https://x-review.example.com/c/1 has changed from 5 to 6",
					}, run.Status_CANCELLED)
					t.Run("Cancel directly if it is root CL causing cancellation", func(t *ftt.Test) {
						rs.RootCL = common.CLID(1)
						res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1})
						assert.NoErr(t, err)
						assert.Loosely(t, res.State.Status, should.Equal(run.Status_CANCELLED))
						assert.Loosely(t, isCurrentlyResettingTriggers(rs), should.BeFalse)
					})
				})

				t.Run("Approval was revoked", func(t *ftt.Test) {
					t.Run("Partial", func(t *ftt.Test) {
						updateCL(1, gf.CI(
							gChange1, gf.PS(gPatchSet1), gf.CQ(+2, triggerTime, gf.U("foo")), gf.Disapprove(),
						), aplConfigOK, accessOK)
						res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1})
						assert.NoErr(t, err)
						verifyHasResetTriggerLongOpScheduled(res, map[common.CLID]string{
							1: "CV cannot start a Run because this CL is not submittable.",
							2: "CV cannot start a Run due to errors in the following CL(s).",
						}, run.Status_FAILED)
						t.Run("Only reset trigger on root Cl", func(t *ftt.Test) {
							rs.RootCL = common.CLID(1)
							res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1})
							assert.NoErr(t, err)
							verifyHasResetTriggerLongOpScheduled(res, map[common.CLID]string{
								1: "CV cannot start a Run because this CL is not submittable.",
							}, run.Status_FAILED)
						})
					})
					t.Run("Both", func(t *ftt.Test) {
						updateCL(1, gf.CI(
							gChange1, gf.PS(gPatchSet1), gf.CQ(+2, triggerTime, gf.U("foo")), gf.Disapprove(),
						), aplConfigOK, accessOK)
						updateCL(2, gf.CI(
							gChange2, gf.PS(gPatchSet2), gf.CQ(+2, triggerTime, gf.U("foo")), gf.Disapprove(),
						), aplConfigOK, accessOK)
						res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1, 2})
						assert.NoErr(t, err)
						verifyHasResetTriggerLongOpScheduled(res, map[common.CLID]string{
							1: "CV cannot start a Run because this CL is not submittable.",
							2: "CV cannot start a Run because this CL is not submittable.",
						}, run.Status_FAILED)
					})
				})
			})

			t.Run("All CLs causes cancellation", func(t *ftt.Test) {
				updateCL(1, gf.CI(gChange1, gf.PS(gPatchSet1+1), gf.CQ(+2, triggerTime, gf.U("foo"))), aplConfigOK, accessOK)
				updateCL(2, gf.CI(gChange2, gf.PS(gPatchSet2+1), gf.CQ(+2, triggerTime, gf.U("foo"))), aplConfigOK, accessOK)
				res, err := h.OnCLsUpdated(ctx, rs, common.CLIDs{1, 2})
				assert.NoErr(t, err)
				assert.Loosely(t, res.State.Status, should.Equal(run.Status_CANCELLED))
				assert.Loosely(t, res.State.CancellationReasons, should.Resemble([]string{
					"the patchset of https://x-review.example.com/c/1 has changed from 5 to 6",
					"the patchset of https://x-review.example.com/c/2 has changed from 7 to 8",
				}))
				assert.Loosely(t, res.SideEffectFn, should.NotBeNil)
				assert.Loosely(t, res.PreserveEvents, should.BeFalse)
				assert.Loosely(t, isCurrentlyResettingTriggers(rs), should.BeFalse)
			})
		})
	})
}
