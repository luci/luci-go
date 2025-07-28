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

package triager

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

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
	"go.chromium.org/luci/cv/internal/prjmanager/itriager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/runcreator"
)

func TestTriage(t *testing.T) {
	t.Parallel()

	ftt.Run("Triage works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		// Truncate start time point s.t. easy to see diff in test failures.
		ct.RoundTestClock(10000 * time.Second)

		const gHost = "g-review.example.com"
		const lProject = "v8"

		const stabilizationDelay = 5 * time.Minute
		const singIdx, combIdx, anotherIdx, nprIdx, nprCombIdx = 0, 1, 2, 3, 4
		cfg := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{Name: "singular"},
				{Name: "combinable", CombineCls: &cfgpb.CombineCLs{
					StabilizationDelay: durationpb.New(stabilizationDelay),
				}},
				{Name: "another"},
				{Name: "newPatchsetRun",
					Verifiers: &cfgpb.Verifiers{
						Tryjob: &cfgpb.Verifiers_Tryjob{
							Builders: []*cfgpb.Verifiers_Tryjob_Builder{
								{Name: "nprBuilder", ModeAllowlist: []string{string(run.NewPatchsetRun)}},
							},
						},
					},
				},
				{Name: "newPatchsetRunCombi",
					Verifiers: &cfgpb.Verifiers{
						Tryjob: &cfgpb.Verifiers_Tryjob{
							Builders: []*cfgpb.Verifiers_Tryjob_Builder{
								{Name: "nprBuilder", ModeAllowlist: []string{string(run.NewPatchsetRun)}},
							},
						},
					},
					CombineCls: &cfgpb.CombineCLs{StabilizationDelay: durationpb.New(stabilizationDelay)},
				},
			},
		}
		prjcfgtest.Create(ctx, lProject, cfg)
		pm := &simplePMState{pb: &prjpb.PState{}}
		var err error
		pm.cgs, err = prjcfgtest.MustExist(ctx, lProject).GetConfigGroups(ctx)
		assert.NoErr(t, err)

		dryRun := func(ts time.Time) *run.Triggers {
			return &run.Triggers{CqVoteTrigger: &run.Trigger{Mode: string(run.DryRun), Time: timestamppb.New(ts)}}
		}

		triage := func(c *prjpb.Component) (itriager.Result, error) {
			backup := prjpb.PState{}
			proto.Merge(&backup, pm.pb)
			res, err := Triage(ctx, c, pm)
			// Regardless of result, PM's state must be not be modified.
			assert.That(t, pm.pb, should.Match(&backup))
			return res, err
		}
		mustTriage := func(c *prjpb.Component) itriager.Result {
			res, err := triage(c)
			assert.NoErr(t, err)
			return res
		}
		failTriage := func(c *prjpb.Component) error {
			_, err := triage(c)
			assert.Loosely(t, err, should.NotBeNil)
			return err
		}

		markTriaged := func(c *prjpb.Component) *prjpb.Component {
			c = c.CloneShallow()
			c.TriageRequired = false
			return c
		}
		owner := gf.U("user-1")
		voter := gf.U("user-2")
		depKind := changelist.DepKind_SOFT
		putPCL := func(clid int, grpIndex int32, mode run.Mode, triggerTime time.Time, depsCLIDs ...int) (*changelist.CL, *prjpb.PCL) {
			mods := []gf.CIModifier{gf.PS(1), gf.Owner("user-1"), gf.Updated(triggerTime)}
			switch mode {
			case run.FullRun:
				mods = append(mods, gf.CQ(+2, triggerTime, voter))
			case run.DryRun:
				mods = append(mods, gf.CQ(+1, triggerTime, voter))
			case run.NewPatchsetRun:
			case "":
				// skip
			default:
				panic(fmt.Errorf("unsupported %s", mode))
			}
			ci := gf.CI(clid, mods...)
			trs := trigger.Find(&trigger.FindInput{ChangeInfo: ci, ConfigGroup: cfg.GetConfigGroups()[grpIndex]})
			switch mode {
			case run.NewPatchsetRun:
				assert.Loosely(t, grpIndex, should.BeGreaterThanOrEqual(nprIdx))
				assert.Loosely(t, trs.GetNewPatchsetRunTrigger(), should.NotBeNil)
			case "":
				// skip
			default:
				assert.Loosely(t, trs.GetCqVoteTrigger(), should.NotBeNil)
				assert.That(t, trs.GetCqVoteTrigger().GetMode(), should.Match(string(mode)))
			}
			cl := &changelist.CL{
				ID:         common.CLID(clid),
				ExternalID: changelist.MustGobID(gHost, int64(clid)),
				EVersion:   1,
				Snapshot: &changelist.Snapshot{Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
					Host: gHost,
					Info: ci,
				}}},
			}
			for _, d := range depsCLIDs {
				cl.Snapshot.Deps = append(cl.Snapshot.Deps, &changelist.Dep{
					Clid: int64(d),
					Kind: depKind,
				})
			}
			assert.NoErr(t, datastore.Put(ctx, cl))
			pclTriggers := proto.Clone(trs).(*run.Triggers)
			if pclTriggers.GetNewPatchsetRunTrigger() != nil {
				pclTriggers.NewPatchsetRunTrigger.Email = owner.GetEmail()
				pclTriggers.NewPatchsetRunTrigger.GerritAccountId = owner.GetAccountId()
			}
			if pclTriggers.GetCqVoteTrigger() != nil {
				pclTriggers.CqVoteTrigger.Email = voter.GetEmail()
				pclTriggers.CqVoteTrigger.GerritAccountId = voter.GetAccountId()
			}
			return cl, &prjpb.PCL{
				Clid:               int64(clid),
				Eversion:           1,
				Status:             prjpb.PCL_OK,
				ConfigGroupIndexes: []int32{grpIndex},
				Triggers:           pclTriggers,
				Deps:               cl.Snapshot.GetDeps(),
			}
		}

		t.Run("Noops", func(t *ftt.Test) {
			pcl := &prjpb.PCL{Clid: 33, ConfigGroupIndexes: []int32{singIdx}, Triggers: dryRun(ct.Clock.Now())}
			pm.pb.Pcls = []*prjpb.PCL{pcl}
			oldC := &prjpb.Component{
				Clids: []int64{33},
				// Component already has a Run, so no action required.
				Pruns:          []*prjpb.PRun{{Id: "id", Clids: []int64{33}, Mode: string(run.DryRun)}},
				TriageRequired: true,
			}
			res := mustTriage(oldC)
			assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
			assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
			assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
		})

		t.Run("Chained CQ votes", func(t *ftt.Test) {
			now := ct.Clock.Now()
			depKind = changelist.DepKind_HARD
			const cl31, cl32, cl33, cl34 = 31, 32, 33, 34

			t.Run("with CQ vote on a child CL", func(t *ftt.Test) {
				_, pcl31 := putPCL(cl31, singIdx, "", now.Add(-time.Minute))
				_, pcl32 := putPCL(cl32, singIdx, "", now.Add(-time.Second), cl31)
				_, pcl33 := putPCL(cl33, singIdx, run.FullRun, now, cl31, cl32)
				pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl32, pcl33}
				var res itriager.Result

				t.Run("no deps have CQ+2", func(t *ftt.Test) {
					oldC := &prjpb.Component{Clids: []int64{cl31, cl32, cl33}, TriageRequired: true}
					res = mustTriage(oldC)
					assert.That(t, res.CLsToTriggerDeps, should.Match([]*prjpb.TriggeringCLDeps{
						{
							OriginClid:      cl33,
							DepClids:        []int64{cl31, cl32},
							Trigger:         pcl33.Triggers.GetCqVoteTrigger(),
							ConfigGroupName: "singular",
						},
					}))
				})

				t.Run("a dep has TriggeringCLDeps already", func(t *ftt.Test) {
					pm.pb.TriggeringClDeps, _ = pm.pb.COWTriggeringCLDeps(nil, []*prjpb.TriggeringCLDeps{
						{
							OriginClid: cl32,
							DepClids:   []int64{cl31},
							Trigger:    pcl32.Triggers.GetCqVoteTrigger(),
						},
					})
					oldC := &prjpb.Component{Clids: []int64{cl31, cl32, cl33}, TriageRequired: true}
					res = mustTriage(oldC)
					// should skip triaging cl33, because its dep already has
					// an inflight TriggeringCLDeps.
					assert.Loosely(t, res.CLsToTriggerDeps, should.BeNil)
				})
				// No triage required, as the Component will be waken up
				// by the vote completion event.
				assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
				assert.Loosely(t, res.NewValue.GetDecisionTime(), should.BeNil)
				// No runs should be created.
				// No CLs should be purged.
				// CL31 and CL32 should be in CLsToTriggerDeps.
				assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
				assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
			})
			t.Run("with CQ vote on some dep CLs, but not all", func(t *ftt.Test) {
				_, pcl31 := putPCL(cl31, singIdx, run.FullRun, now.Add(-time.Second))
				_, pcl32 := putPCL(cl32, singIdx, "", now.Add(-time.Second), cl31)
				_, pcl33 := putPCL(cl33, singIdx, run.FullRun, now.Add(-time.Minute), cl31, cl32)
				_, pcl34 := putPCL(cl34, singIdx, run.FullRun, now.Add(-time.Minute), cl31, cl32, cl33)
				pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl32, pcl33, pcl34}
				oldC := &prjpb.Component{
					Clids: []int64{cl31, cl32, cl33, cl34}, TriageRequired: true}
				res := mustTriage(oldC)

				// No triage required, as the Component will be waken up
				// by the vote completion event.
				assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
				assert.Loosely(t, res.NewValue.GetDecisionTime(), should.BeNil)

				// run for CL31 should be created, as it meets all the run
				// creation requirements.
				assert.Loosely(t, res.RunsToCreate, should.HaveLength(1))
				assert.Loosely(t, res.RunsToCreate[0].InputCLs, should.HaveLength(1))
				assert.Loosely(t, res.RunsToCreate[0].InputCLs[0].ID, should.Equal(cl31))
				assert.Loosely(t, res.RunsToCreate[0].DepRuns, should.BeNil)

				// a single TriggeringCLDeps{} should be created for CL34 to
				// trigger CL32.
				assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
				assert.That(t, res.CLsToTriggerDeps, should.Match([]*prjpb.TriggeringCLDeps{
					{
						OriginClid:      cl34,
						DepClids:        []int64{cl32},
						Trigger:         pcl34.Triggers.GetCqVoteTrigger(),
						ConfigGroupName: "singular",
					},
				}))
			})
			t.Run("with CQ votes on all CLs", func(t *ftt.Test) {
				_, pcl31 := putPCL(cl31, singIdx, run.FullRun, now.Add(-time.Second))
				_, pcl32 := putPCL(cl32, singIdx, run.FullRun, now.Add(-time.Second), cl31)
				pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl32}
				component := &prjpb.Component{Clids: []int64{cl31, cl32}, TriageRequired: true}
				mockRunCreation := func(rc *runcreator.Creator) common.RunID {
					component.Pruns = makePrunsWithMode(
						run.FullRun, rc.ExpectedRunID(), rc.InputCLs[0].ID)
					assert.Loosely(t, datastore.Put(ctx, &run.Run{
						ID:     rc.ExpectedRunID(),
						Status: run.Status_RUNNING}), should.BeNil)
					return rc.ExpectedRunID()
				}

				// The first triage should create runcreator for pcl31 only.
				res := mustTriage(component)
				assert.Loosely(t, res.RunsToCreate, should.HaveLength(1))
				assert.Loosely(t, res.RunsToCreate[0].InputCLs, should.HaveLength(1))
				assert.Loosely(t, res.RunsToCreate[0].InputCLs[0].ID, should.Equal(cl31))
				assert.Loosely(t, res.RunsToCreate[0].DepRuns, should.BeNil)
				assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
				assert.Loosely(t, res.CLsToTriggerDeps, should.BeEmpty)
				rid31 := mockRunCreation(res.RunsToCreate[0])

				t.Run("with a dep that has an incomplete Run", func(t *ftt.Test) {
					// pcl32, next, but with DepRuns this time.
					res = mustTriage(component)
					assert.Loosely(t, res.RunsToCreate, should.HaveLength(1))
					assert.Loosely(t, res.RunsToCreate[0].InputCLs, should.HaveLength(1))
					assert.Loosely(t, res.RunsToCreate[0].InputCLs[0].ID, should.Equal(cl32))
					assert.That(t, res.RunsToCreate[0].DepRuns, should.Match(common.RunIDs{rid31}))
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.CLsToTriggerDeps, should.BeEmpty)
					mockRunCreation(res.RunsToCreate[0])

					// the next triage should produce nothing.
					res = mustTriage(component)
					assert.Loosely(t, res.RunsToCreate, should.HaveLength(0))
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.CLsToTriggerDeps, should.BeEmpty)
				})

				t.Run("with a dep of which run has been submitted", func(t *ftt.Test) {
					pcl31.Submitted = true
					// Actually, run submitter doesn't remove the CQ vote,
					// but State.makePCL() skips setting the trigger in
					// the PCL if the CL is submitted,.
					//
					// triageNewTriggers() panics, if the assumption is
					// wrong.
					pcl31.Triggers = nil
					assert.Loosely(t, datastore.Put(ctx, &run.Run{
						ID:     rid31,
						Status: run.Status_SUCCEEDED}), should.BeNil)
					component.Pruns = nil

					res = mustTriage(component)
					assert.Loosely(t, res.RunsToCreate, should.HaveLength(1))
					assert.Loosely(t, res.RunsToCreate[0].InputCLs, should.HaveLength(1))
					assert.Loosely(t, res.RunsToCreate[0].InputCLs[0].ID, should.Equal(cl32))
					// DepRuns should be nil, as pcl31 has been submitted.
					assert.Loosely(t, res.RunsToCreate[0].DepRuns, should.BeNil)
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.CLsToTriggerDeps, should.BeEmpty)
					mockRunCreation(res.RunsToCreate[0])

					// the next triage should produce nothing.
					res = mustTriage(component)
					assert.Loosely(t, res.RunsToCreate, should.HaveLength(0))
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.CLsToTriggerDeps, should.BeEmpty)
				})
			})
		})

		t.Run("Purges CLs", func(t *ftt.Test) {
			pm.pb.Pcls = []*prjpb.PCL{{
				Clid:               33,
				ConfigGroupIndexes: nil, // modified below.
				Triggers:           dryRun(ct.Clock.Now()),
				PurgeReasons: []*prjpb.PurgeReason{{
					ClError: &changelist.CLError{ // => must purge.
						Kind: &changelist.CLError_OwnerLacksEmail{OwnerLacksEmail: true},
					},
					ApplyTo: &prjpb.PurgeReason_Triggers{Triggers: dryRun(ct.Clock.Now())},
				}},
			}}
			oldC := &prjpb.Component{Clids: []int64{33}}

			t.Run("singular group -- no delay", func(t *ftt.Test) {
				pm.pb.Pcls[0].ConfigGroupIndexes = []int32{singIdx}
				res := mustTriage(oldC)
				assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
				assert.Loosely(t, res.CLsToPurge, should.HaveLength(1))
				assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
			})
			t.Run("singular group, does not affect NPR", func(t *ftt.Test) {
				_, pcl := putPCL(33, nprIdx, run.DryRun, ct.Clock.Now())
				pm.pb.Pcls[0] = pcl
				pcl.PurgeReasons = []*prjpb.PurgeReason{{
					ClError: &changelist.CLError{
						Kind: &changelist.CLError_SelfCqDepend{},
					},
					ApplyTo: &prjpb.PurgeReason_Triggers{Triggers: dryRun(ct.Clock.Now())},
				}}
				res := mustTriage(oldC)
				assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
				assert.Loosely(t, res.CLsToPurge, should.HaveLength(1))
				assert.Loosely(t, res.RunsToCreate, should.HaveLength(1))
				assert.Loosely(t, res.RunsToCreate[0].Mode, should.Equal(run.NewPatchsetRun))
			})
			t.Run("NPR trigger only", func(t *ftt.Test) {
				_, pcl := putPCL(33, nprIdx, run.NewPatchsetRun, ct.Clock.Now())
				pm.pb.Pcls[0] = pcl
				pcl.PurgeReasons = []*prjpb.PurgeReason{{
					ClError: &changelist.CLError{
						Kind: &changelist.CLError_UnsupportedMode{},
					},
					ApplyTo: &prjpb.PurgeReason_Triggers{
						Triggers: &run.Triggers{
							NewPatchsetRunTrigger: &run.Trigger{
								Mode: string(run.NewPatchsetRun),
								Time: timestamppb.New(ct.Clock.Now()),
							},
						},
					},
				}}
				res := mustTriage(oldC)
				assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
				assert.Loosely(t, res.CLsToPurge, should.HaveLength(1))
				assert.Loosely(t, res.RunsToCreate, should.HaveLength(0))
			})
			t.Run("singular group, purge NPR trigger and let dry run continue", func(t *ftt.Test) {
				_, pcl := putPCL(33, nprIdx, run.DryRun, ct.Clock.Now())
				pm.pb.Pcls[0] = pcl
				pcl.PurgeReasons = []*prjpb.PurgeReason{{
					ClError: &changelist.CLError{
						Kind: &changelist.CLError_OwnerLacksEmail{},
					},
					ApplyTo: &prjpb.PurgeReason_Triggers{
						Triggers: &run.Triggers{
							NewPatchsetRunTrigger: pcl.GetTriggers().GetNewPatchsetRunTrigger(),
						},
					},
				}}
				res := mustTriage(oldC)
				assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
				assert.Loosely(t, res.CLsToPurge, should.HaveLength(1))
				assert.Loosely(t, res.RunsToCreate, should.HaveLength(1))
				assert.Loosely(t, res.RunsToCreate[0].Mode, should.Equal(run.DryRun))
			})
			t.Run("combinable group -- obey stabilization_delay", func(t *ftt.Test) {
				pm.pb.Pcls[0].ConfigGroupIndexes = []int32{combIdx}

				res := mustTriage(oldC)
				c := markTriaged(oldC)
				c.DecisionTime = timestamppb.New(ct.Clock.Now().Add(stabilizationDelay))
				assert.That(t, res.NewValue, should.Match(c))
				assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
				assert.Loosely(t, res.RunsToCreate, should.BeEmpty)

				ct.Clock.Add(stabilizationDelay * 2)
				res = mustTriage(oldC)
				c.DecisionTime = nil
				assert.That(t, res.NewValue, should.Match(c))
				assert.Loosely(t, res.CLsToPurge, should.HaveLength(1))
				assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
			})
			t.Run("NewPatchsetRun combinable group -- ignore stabilization_delay", func(t *ftt.Test) {
				_, pcl := putPCL(33, nprCombIdx, run.NewPatchsetRun, ct.Clock.Now())
				pm.pb.Pcls[0] = pcl
				pcl.PurgeReasons = []*prjpb.PurgeReason{{
					ClError: &changelist.CLError{
						Kind: &changelist.CLError_UnsupportedMode{},
					},
					ApplyTo: &prjpb.PurgeReason_Triggers{
						Triggers: &run.Triggers{
							NewPatchsetRunTrigger: &run.Trigger{
								Mode: string(run.NewPatchsetRun),
								Time: timestamppb.New(ct.Clock.Now()),
							},
						},
					},
				}}
				res := mustTriage(oldC)
				assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
				assert.Loosely(t, res.CLsToPurge, should.HaveLength(1))
				assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
			})
			t.Run("many groups -- no delay", func(t *ftt.Test) {
				// many groups is an error itself
				pm.pb.Pcls[0].ConfigGroupIndexes = []int32{singIdx, combIdx, anotherIdx}
				res := mustTriage(oldC)
				assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
				assert.Loosely(t, res.CLsToPurge, should.HaveLength(1))
				assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
			})

			t.Run("with already existing Run:", func(t *ftt.Test) {
				_, pcl32 := putPCL(32, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Second))
				_, pcl33 := putPCL(33, combIdx, run.FullRun, ct.Clock.Now(), 32)

				pm.pb.Pcls = []*prjpb.PCL{pcl32, pcl33}
				oldC := &prjpb.Component{Clids: []int64{32, 33}, TriageRequired: true}
				ct.Clock.Add(stabilizationDelay)
				const expectedRunID = "v8/9042327596854-1-42e0a59099a0f673"

				t.Run("wait a bit if Run is RUNNING", func(t *ftt.Test) {
					assert.NoErr(t, datastore.Put(ctx, &run.Run{ID: expectedRunID, Status: run.Status_RUNNING}))
					res := mustTriage(oldC)
					assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
					assert.That(t, res.NewValue.GetDecisionTime().AsTime(), should.Match(ct.Clock.Now().Add(5*time.Second).UTC()))
					assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
				})

				r := &run.Run{
					ID:      expectedRunID,
					Status:  run.Status_CANCELLED,
					EndTime: datastore.RoundTime(ct.Clock.Now().UTC()),
				}
				assert.NoErr(t, datastore.Put(ctx, r))

				t.Run("wait a bit if Run was just finalized", func(t *ftt.Test) {
					ct.Clock.Add(5 * time.Second)
					res := mustTriage(oldC)
					assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
					assert.That(t, res.NewValue.GetDecisionTime().AsTime(), should.Match(r.EndTime.Add(time.Minute).UTC()))
					assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
				})

				t.Run("purge CLs due to trigger re-use after long-ago finished Run", func(t *ftt.Test) {
					ct.Clock.Add(2 * time.Minute)
					res := mustTriage(oldC)
					assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
					assert.Loosely(t, res.NewValue.GetDecisionTime(), should.BeNil)
					assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
					assert.Loosely(t, res.CLsToPurge, should.HaveLength(2))
					for _, p := range res.CLsToPurge {
						assert.Loosely(t, p.GetPurgeReasons(), should.HaveLength(1))
						assert.That(t, p.GetPurgeReasons()[0].GetClError().GetReusedTrigger().GetRun(), should.Match(expectedRunID))
					}
				})
			})
		})

		t.Run("Creates Runs", func(t *ftt.Test) {
			t.Run("CQVote and NPR triggers", func(t *ftt.Test) {
				_, pcl := putPCL(33, nprIdx, run.DryRun, ct.Clock.Now())
				pm.pb.Pcls = []*prjpb.PCL{pcl}
				oldC := &prjpb.Component{Clids: []int64{33}, TriageRequired: true}
				res := mustTriage(oldC)
				assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
				assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
				assert.Loosely(t, res.RunsToCreate, should.HaveLength(2))
				rc := res.RunsToCreate[0]
				assert.That(t, rc.ConfigGroupID.Name(), should.Match("newPatchsetRun"))
				assert.That(t, rc.Mode, should.Match(run.DryRun))
				assert.Loosely(t, rc.InputCLs, should.HaveLength(1))
				rc = res.RunsToCreate[1]
				assert.Loosely(t, rc.InputCLs[0].ID, should.Equal(33))
				assert.That(t, rc.ConfigGroupID.Name(), should.Match("newPatchsetRun"))
				assert.That(t, rc.Mode, should.Match(run.NewPatchsetRun))
				assert.Loosely(t, rc.InputCLs, should.HaveLength(1))
				assert.Loosely(t, rc.InputCLs[0].ID, should.Equal(33))
			})
			t.Run("NPR Only", func(t *ftt.Test) {
				t.Run("OK", func(t *ftt.Test) {
					var pcl *prjpb.PCL
					_, pcl = putPCL(33, nprIdx, run.NewPatchsetRun, ct.Clock.Now())

					pm.pb.Pcls = []*prjpb.PCL{pcl}
					oldC := &prjpb.Component{Clids: []int64{33}, TriageRequired: true}
					res := mustTriage(oldC)
					assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.RunsToCreate, should.HaveLength(1))
					rc := res.RunsToCreate[0]
					assert.That(t, rc.ConfigGroupID.Name(), should.Match("newPatchsetRun"))
					assert.That(t, rc.Mode, should.Match(run.NewPatchsetRun))
					assert.Loosely(t, rc.InputCLs, should.HaveLength(1))
					assert.Loosely(t, rc.InputCLs[0].ID, should.Equal(33))
				})

				t.Run("Noop when Run already exists", func(t *ftt.Test) {
					var pcl *prjpb.PCL
					_, pcl = putPCL(33, nprIdx, run.NewPatchsetRun, ct.Clock.Now())

					pm.pb.Pcls = []*prjpb.PCL{pcl}
					oldC := &prjpb.Component{Clids: []int64{33}, TriageRequired: true, Pruns: makePrunsWithMode(run.NewPatchsetRun, "run-id", 33)}
					res := mustTriage(oldC)
					assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
				})

				t.Run("EVersion mismatch is an ErrOutdatedPMState", func(t *ftt.Test) {
					cl, pcl := putPCL(33, nprIdx, run.NewPatchsetRun, ct.Clock.Now())
					cl.EVersion = 2
					assert.NoErr(t, datastore.Put(ctx, cl))
					pm.pb.Pcls = []*prjpb.PCL{pcl}
					err := failTriage(&prjpb.Component{Clids: []int64{33}, TriageRequired: true})
					assert.ErrIsLike(t, err, itriager.ErrOutdatedPMState)
					assert.ErrIsLike(t, err, "EVersion changed 1 => 2")
				})
			})
			t.Run("Singular", func(t *ftt.Test) {
				t.Run("OK", func(t *ftt.Test) {
					_, pcl := putPCL(33, singIdx, run.DryRun, ct.Clock.Now())
					pm.pb.Pcls = []*prjpb.PCL{pcl}
					oldC := &prjpb.Component{Clids: []int64{33}, TriageRequired: true}
					res := mustTriage(oldC)
					assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.RunsToCreate, should.HaveLength(1))
					rc := res.RunsToCreate[0]
					assert.That(t, rc.ConfigGroupID.Name(), should.Match("singular"))
					assert.That(t, rc.Mode, should.Match(run.DryRun))
					assert.Loosely(t, rc.InputCLs, should.HaveLength(1))
					assert.Loosely(t, rc.InputCLs[0].ID, should.Equal(33))
				})

				t.Run("Noop when Run already exists", func(t *ftt.Test) {
					_, pcl := putPCL(33, singIdx, run.DryRun, ct.Clock.Now())
					pm.pb.Pcls = []*prjpb.PCL{pcl}
					oldC := &prjpb.Component{Clids: []int64{33}, TriageRequired: true, Pruns: makePruns("run-id", 33)}
					res := mustTriage(oldC)
					assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
				})

				t.Run("EVersion mismatch is an ErrOutdatedPMState", func(t *ftt.Test) {
					cl, pcl := putPCL(33, singIdx, run.DryRun, ct.Clock.Now())
					cl.EVersion = 2
					assert.NoErr(t, datastore.Put(ctx, cl))
					pm.pb.Pcls = []*prjpb.PCL{pcl}
					err := failTriage(&prjpb.Component{Clids: []int64{33}, TriageRequired: true})
					assert.ErrIsLike(t, err, itriager.ErrOutdatedPMState)
					assert.ErrIsLike(t, err, "EVersion changed 1 => 2")
				})

				t.Run("OK with resolved deps", func(t *ftt.Test) {
					_, pcl32 := putPCL(32, singIdx, run.FullRun, ct.Clock.Now())
					_, pcl33 := putPCL(33, singIdx, run.DryRun, ct.Clock.Now(), 32)
					pm.pb.Pcls = []*prjpb.PCL{pcl32, pcl33}
					oldC := &prjpb.Component{Clids: []int64{32, 33}, TriageRequired: true}
					res := mustTriage(oldC)
					assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.RunsToCreate, should.HaveLength(2))
					sortRunsToCreateByFirstCL(&res)
					assert.Loosely(t, res.RunsToCreate[0].InputCLs[0].ID, should.Equal(32))
					assert.That(t, res.RunsToCreate[0].Mode, should.Match(run.FullRun))
					assert.Loosely(t, res.RunsToCreate[1].InputCLs[0].ID, should.Equal(33))
					assert.That(t, res.RunsToCreate[1].Mode, should.Match(run.DryRun))
				})

				t.Run("OK with existing Runs but on different CLs", func(t *ftt.Test) {
					_, pcl31 := putPCL(31, singIdx, run.FullRun, ct.Clock.Now())
					_, pcl32 := putPCL(32, singIdx, run.DryRun, ct.Clock.Now(), 31)
					_, pcl33 := putPCL(33, singIdx, run.DryRun, ct.Clock.Now(), 32)
					pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl32, pcl33}
					oldC := &prjpb.Component{
						Clids:          []int64{31, 32, 33},
						Pruns:          makePruns("first", 31, "third", 33),
						TriageRequired: true,
					}
					res := mustTriage(oldC)
					assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.RunsToCreate, should.HaveLength(1))
					assert.Loosely(t, res.RunsToCreate[0].InputCLs[0].ID, should.Equal(32))
					assert.That(t, res.RunsToCreate[0].Mode, should.Match(run.DryRun))
				})

				t.Run("Waits for unresolved dep without an error", func(t *ftt.Test) {
					pcl32 := &prjpb.PCL{Clid: 32, Eversion: 1, Status: prjpb.PCL_UNKNOWN}
					_, pcl33 := putPCL(33, singIdx, run.DryRun, ct.Clock.Now(), 32)
					pm.pb.Pcls = []*prjpb.PCL{pcl32, pcl33}
					oldC := &prjpb.Component{Clids: []int64{33}, TriageRequired: true}
					res := mustTriage(oldC)
					assert.That(t, res.NewValue, should.Match(markTriaged(oldC)))
					// TODO(crbug/1211576): this waiting can last forever. Component needs
					// to record how long it has been waiting and abort with clear message
					// to the user.
					assert.Loosely(t, res.NewValue.GetDecisionTime(), should.BeNil) // wait for external event of loading a dep
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
				})
			})

			t.Run("Combinable", func(t *ftt.Test) {
				t.Run("OK after obeying stabilization delay", func(t *ftt.Test) {
					// Simulate a CL stack <base> -> 31 -> 32 -> 33, which user wants
					// to land at the same time by making 31 depend on 33.
					_, pcl31 := putPCL(31, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Minute), 33)
					_, pcl32 := putPCL(32, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Second), 31)
					_, pcl33 := putPCL(33, combIdx, run.FullRun, ct.Clock.Now(), 32, 31)
					pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl32, pcl33}
					oldC := &prjpb.Component{Clids: []int64{31, 32, 33}, TriageRequired: true}
					res := mustTriage(oldC)
					assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
					assert.That(t, res.NewValue.GetDecisionTime().AsTime(), should.Match(ct.Clock.Now().Add(stabilizationDelay).UTC()))
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.RunsToCreate, should.BeEmpty)

					ct.Clock.Add(stabilizationDelay)

					oldC = res.NewValue
					res = mustTriage(oldC)
					assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
					assert.Loosely(t, res.NewValue.GetDecisionTime(), should.BeNil)
					rc := res.RunsToCreate[0]
					assert.That(t, rc.ConfigGroupID.Name(), should.Match("combinable"))
					assert.That(t, rc.Mode, should.Match(run.FullRun))
					trig := pcl33.GetTriggers().GetCqVoteTrigger()
					assert.That(t, rc.CreateTime, should.Match(trig.GetTime().AsTime()))
					assert.Loosely(t, rc.InputCLs, should.HaveLength(3))
				})

				t.Run("Even a single CL should wait for stabilization delay", func(t *ftt.Test) {
					_, pcl := putPCL(33, combIdx, run.FullRun, ct.Clock.Now())
					pm.pb.Pcls = []*prjpb.PCL{pcl}
					oldC := &prjpb.Component{Clids: []int64{33}, TriageRequired: true}
					res := mustTriage(oldC)
					assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
					assert.That(t, res.NewValue.GetDecisionTime().AsTime(), should.Match(ct.Clock.Now().Add(stabilizationDelay).UTC()))
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.RunsToCreate, should.BeEmpty)

					ct.Clock.Add(stabilizationDelay)

					oldC = res.NewValue
					res = mustTriage(oldC)
					assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
					assert.Loosely(t, res.NewValue.GetDecisionTime(), should.BeNil)
					rc := res.RunsToCreate[0]
					assert.That(t, rc.ConfigGroupID.Name(), should.Match("combinable"))
					assert.That(t, rc.Mode, should.Match(run.FullRun))
					assert.Loosely(t, rc.InputCLs, should.HaveLength(1))
					assert.Loosely(t, rc.InputCLs[0].ID, should.Equal(33))
				})

				t.Run("Waits for unresolved dep, even after stabilization delay", func(t *ftt.Test) {
					pcl32 := &prjpb.PCL{Clid: 32, Eversion: 1, Status: prjpb.PCL_UNKNOWN}
					_, pcl33 := putPCL(33, combIdx, run.FullRun, ct.Clock.Now(), 32)
					pm.pb.Pcls = []*prjpb.PCL{pcl32, pcl33}
					oldC := &prjpb.Component{Clids: []int64{33}, TriageRequired: true}
					res := mustTriage(oldC)
					assert.That(t, res.NewValue.GetDecisionTime().AsTime(), should.Match(ct.Clock.Now().Add(stabilizationDelay).UTC()))
					assert.Loosely(t, res.RunsToCreate, should.BeEmpty)

					ct.Clock.Add(stabilizationDelay)

					oldC = res.NewValue
					res = mustTriage(oldC)
					assert.Loosely(t, res.NewValue.GetDecisionTime(), should.BeNil) // wait for external event of loading a dep
					// TODO(crbug/1211576): this waiting can last forever. Component needs
					// to record how long it has been waiting and abort with clear message
					// to the user.
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
				})

				t.Run("Noop if there is existing Run encompassing all the CLs", func(t *ftt.Test) {
					_, pcl31 := putPCL(31, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Hour), 32)
					_, pcl32 := putPCL(32, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Hour), 31)

					pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl32}
					oldC := &prjpb.Component{Clids: []int64{31, 32}, TriageRequired: true, Pruns: makePruns("runID", 31, 32)}
					res := mustTriage(oldC)
					assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.RunsToCreate, should.BeEmpty)

					t.Run("even if some CLs are no longer triggered", func(t *ftt.Test) {
						// Happens during Run abort due to, say, tryjob failure.
						pcl31.Triggers = nil
						assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
						assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
						assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
					})

					t.Run("even if some CLs are already submitted", func(t *ftt.Test) {
						// Happens during Run submission.
						pcl32.Triggers = nil
						pcl32.Submitted = true
						assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
						assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
						assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
					})
				})

				t.Run("Component growing to N+1 CLs that could form a Run while Run on N CLs is already running", func(t *ftt.Test) {
					// Simulate scenario of user first uploading 31<-41 and CQing two
					// CLs, then much later uploading 51 depending on 31 and CQing 51
					// while (31,41) Run is still running.
					//
					// This may change once postponeExpandingExistingRunScope() function is
					// implemented, but for now test documents that CV will just wait
					// until (31, 41) Run completes.
					_, pcl31 := putPCL(31, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Hour))
					_, pcl41 := putPCL(41, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Hour), 31)
					_, pcl51 := putPCL(51, combIdx, run.FullRun, ct.Clock.Now(), 31)
					ct.Clock.Add(2 * stabilizationDelay)
					pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl41, pcl51}
					oldC := &prjpb.Component{Clids: []int64{31, 41, 51}, TriageRequired: true, Pruns: makePruns("41-31", 31, 41)}
					res := mustTriage(oldC)
					assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
					assert.Loosely(t, res.NewValue.GetDecisionTime(), should.BeNil)
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
				})

				t.Run("Doesn't react to updates that must be handled first by the Run Manager", func(t *ftt.Test) {
					_, pcl31 := putPCL(31, combIdx, run.DryRun, ct.Clock.Now().Add(-time.Hour), 31)
					_, pcl41 := putPCL(41, combIdx, run.DryRun, ct.Clock.Now().Add(-time.Hour), 31)
					_, pcl51 := putPCL(51, combIdx, run.DryRun, ct.Clock.Now().Add(-time.Hour), 31, 41)

					pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl41, pcl51}
					oldC := &prjpb.Component{
						Clids:          []int64{31, 41, 51},
						TriageRequired: true,
						Pruns:          makePruns("sub/mitting", 31, 41, 51),
					}
					mustWaitForRM := func() {
						res := mustTriage(oldC)
						assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
						assert.Loosely(t, res.NewValue.GetDecisionTime(), should.BeNil)
						assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
						assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
					}

					t.Run("multi-CL Run is being submitted", func(t *ftt.Test) {
						pcl31.Submitted = true
						pcl31.Triggers = nil
						mustWaitForRM()
					})
					t.Run("multi-CL Run is being canceled", func(t *ftt.Test) {
						pcl31.Triggers = nil
						mustWaitForRM()
					})
					t.Run("multi-CL Run is no longer in the same ConfigGroup", func(t *ftt.Test) {
						pcl31.ConfigGroupIndexes = []int32{anotherIdx}
						mustWaitForRM()
					})
					t.Run("multi-CL Run is no longer in the same LUCI project", func(t *ftt.Test) {
						pcl31.Status = prjpb.PCL_UNWATCHED
						pcl31.ConfigGroupIndexes = nil
						pcl31.Triggers = nil
						mustWaitForRM()
					})
				})

				t.Run("Handles races between CL purging and Gerrit -> CLUpdater -> PM state propagation", func(t *ftt.Test) {
					// Due to delays / races between purging a CL and PM state,
					// it's possible that CL Purger hasn't yet responded with purge end
					// result yet PM's view of CL state has changed to look valid, and
					// ready to trigger a Run.
					// Then, PM must wait for the purge to complete.
					_, pcl31 := putPCL(31, combIdx, run.DryRun, ct.Clock.Now())
					_, pcl32 := putPCL(32, combIdx, run.DryRun, ct.Clock.Now(), 31)
					_, pcl33 := putPCL(33, combIdx, run.DryRun, ct.Clock.Now(), 31)
					ct.Clock.Add(2 * stabilizationDelay)
					pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl32, pcl33}
					pm.pb.PurgingCls = []*prjpb.PurgingCL{
						{
							Clid: 31, OperationId: "purge-op-31", Deadline: timestamppb.New(ct.Clock.Now().Add(time.Minute)),
							ApplyTo: &prjpb.PurgingCL_AllActiveTriggers{AllActiveTriggers: true},
						},
					}
					oldC := &prjpb.Component{Clids: []int64{31, 32, 33}, TriageRequired: true}
					res := mustTriage(oldC)
					assert.Loosely(t, res.NewValue.GetTriageRequired(), should.BeFalse)
					assert.Loosely(t, res.NewValue.GetDecisionTime(), should.BeNil)
					assert.Loosely(t, res.CLsToPurge, should.BeEmpty)
					assert.Loosely(t, res.RunsToCreate, should.BeEmpty)
				})
			})
		})
	})
}

func sortRunsToCreateByFirstCL(res *itriager.Result) {
	sort.Slice(res.RunsToCreate, func(i, j int) bool {
		return res.RunsToCreate[i].InputCLs[0].ID < res.RunsToCreate[j].InputCLs[0].ID
	})
}

// makePruns is readability sugar to create 0+ pruns slice.
// Example use: makePruns("first", 31, 32, "second", 44, "third", 11).
func makePruns(runIDthenCLIDs ...any) []*prjpb.PRun {
	return makePrunsWithMode(run.DryRun, runIDthenCLIDs...)
}

func makePrunsWithMode(m run.Mode, runIDthenCLIDs ...any) []*prjpb.PRun {
	var out []*prjpb.PRun
	var cur *prjpb.PRun
	const sentinel = "<$sentinel>"
	runIDthenCLIDs = append(runIDthenCLIDs, sentinel)

	for _, arg := range runIDthenCLIDs {
		switch v := arg.(type) {
		case common.RunID:
			arg = string(v)
		case int:
			arg = int64(v)
		case common.CLID:
			arg = int64(v)
		}

		switch v := arg.(type) {
		case string:
			if cur != nil {
				if len(cur.GetClids()) == 0 {
					panic("two consecutive strings not allowed = each run must have at least one CLID")
				}
				out = append(out, cur)
			}
			if v == sentinel {
				return out
			}
			if v == "" {
				panic("empty run ID")
			}
			cur = &prjpb.PRun{Id: string(v), Mode: string(m)}
		case int64:
			if cur == nil {
				panic("CLIDs must follow a string RunID")
			}
			cur.Clids = append(cur.Clids, v)
		}
	}
	panic("not reachable")
}
