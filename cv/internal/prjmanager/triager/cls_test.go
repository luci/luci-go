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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func init() {
	// Allow should.Match to look inside clInfo
	registry.RegisterCmpOption(cmp.AllowUnexported(clInfo{}))
	registry.RegisterCmpOption(cmp.AllowUnexported(triagedCL{}))
	registry.RegisterCmpOption(cmp.AllowUnexported(triagedDeps{}))
}

func TestCLsTriage(t *testing.T) {
	t.Parallel()

	ftt.Run("Component's PCL deps triage", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		// Truncate start time point s.t. easy to see diff in test failures.
		epoch := testclock.TestRecentTimeUTC.Truncate(10000 * time.Second)
		dryRun := func(ts time.Time) *run.Trigger {
			return &run.Trigger{Mode: string(run.DryRun), Time: timestamppb.New(ts)}
		}
		fullRun := func(ts time.Time) *run.Trigger {
			return &run.Trigger{Mode: string(run.FullRun), Time: timestamppb.New(ts)}
		}
		newPatchsetTrigger := func(ts time.Time) *run.Trigger {
			return &run.Trigger{Mode: string(run.NewPatchsetRun), Time: timestamppb.New(ts)}
		}

		sup := &simplePMState{
			pb: &prjpb.PState{},
			cgs: []*prjcfg.ConfigGroup{
				{ID: "hash/singular", Content: &cfgpb.ConfigGroup{}},
				{ID: "hash/combinable", Content: &cfgpb.ConfigGroup{CombineCls: &cfgpb.CombineCLs{}}},
				{ID: "hash/another", Content: &cfgpb.ConfigGroup{}},
				{ID: "hash/npr", Content: &cfgpb.ConfigGroup{
					Verifiers: &cfgpb.Verifiers{
						Tryjob: &cfgpb.Verifiers_Tryjob{
							Builders: []*cfgpb.Verifiers_Tryjob_Builder{
								{Name: "nprBuilder", ModeAllowlist: []string{string(run.NewPatchsetRun)}},
							},
						},
					},
				}},
			},
		}
		pm := pmState{sup}
		const singIdx, combIdx, anotherIdx, nprIdx = 0, 1, 2, 3

		do := func(c *prjpb.Component) map[int64]*clInfo {
			sup.pb.Components = []*prjpb.Component{c} // include it in backup
			backup := prjpb.PState{}
			proto.Merge(&backup, sup.pb)

			cls := triageCLs(ctx, c, pm)
			assert.Loosely(t, sup.pb, should.Resemble(&backup)) // must not be modified
			return cls
		}

		t.Run("Typical 1 CL component without deps", func(t *ftt.Test) {
			sup.pb.Pcls = []*prjpb.PCL{{
				Clid:               1,
				ConfigGroupIndexes: []int32{singIdx},
				Status:             prjpb.PCL_OK,
				Triggers:           &run.Triggers{CqVoteTrigger: dryRun(epoch)},
				Submitted:          false,
				Deps:               nil,
			}}

			t.Run("Ready without runs", func(t *ftt.Test) {
				cls := do(&prjpb.Component{Clids: []int64{1}})
				assert.Loosely(t, cls, should.HaveLength(1))
				expected := &clInfo{
					pcl:            pm.MustPCL(1),
					runIndexes:     nil,
					purgingCL:      nil,
					runCountByMode: map[run.Mode]int{},

					triagedCL: triagedCL{
						purgeReasons: nil,
						cqReady:      true,
						deps:         &triagedDeps{},
					},
				}
				assert.Loosely(t, cls[1], should.Match(expected))

				t.Run("ready may also be in 1+ Runs", func(t *ftt.Test) {
					cls := do(&prjpb.Component{
						Clids: []int64{1},
						Pruns: []*prjpb.PRun{{Id: "r1", Clids: []int64{1}, Mode: string(run.DryRun)}},
					})
					assert.Loosely(t, cls, should.HaveLength(1))
					expected.runIndexes = []int32{0}
					expected.runCountByMode["DRY_RUN"] = 1
					assert.Loosely(t, cls[1], should.Match(expected))
				})
			})

			t.Run("CL already with Errors is not ready", func(t *ftt.Test) {
				sup.pb.Pcls[0].PurgeReasons = []*prjpb.PurgeReason{
					{
						ClError: &changelist.CLError{
							Kind: &changelist.CLError_OwnerLacksEmail{OwnerLacksEmail: true},
						},
						ApplyTo: &prjpb.PurgeReason_AllActiveTriggers{AllActiveTriggers: true},
					},
					{
						ClError: &changelist.CLError{
							Kind: &changelist.CLError_UnsupportedMode{UnsupportedMode: "CUSTOM_RUN"},
						},
						ApplyTo: &prjpb.PurgeReason_AllActiveTriggers{AllActiveTriggers: true},
					},
				}
				cls := do(&prjpb.Component{Clids: []int64{1}})
				assert.Loosely(t, cls, should.HaveLength(1))
				expected := &clInfo{
					pcl:            pm.MustPCL(1),
					runIndexes:     nil,
					purgingCL:      nil,
					runCountByMode: map[run.Mode]int{},

					triagedCL: triagedCL{
						purgeReasons: sup.pb.Pcls[0].GetPurgeReasons(),
					},
				}
				assert.Loosely(t, cls[1], should.Match(expected))
			})

			t.Run("Already purged is never ready", func(t *ftt.Test) {
				sup.pb.PurgingCls = []*prjpb.PurgingCL{{
					Clid:    1,
					ApplyTo: &prjpb.PurgingCL_AllActiveTriggers{AllActiveTriggers: true},
				}}
				cls := do(&prjpb.Component{Clids: []int64{1}})
				assert.Loosely(t, cls, should.HaveLength(1))
				expected := &clInfo{
					pcl:            pm.MustPCL(1),
					runIndexes:     nil,
					purgingCL:      pm.PurgingCL(1),
					runCountByMode: map[run.Mode]int{},

					triagedCL: triagedCL{
						purgeReasons: nil,
						cqReady:      false,
						deps:         &triagedDeps{},
					},
				}
				assert.Loosely(t, cls[1], should.Match(expected))

				t.Run("not even if inside 1+ Runs", func(t *ftt.Test) {
					cls := do(&prjpb.Component{
						Clids: []int64{1},
						Pruns: []*prjpb.PRun{{Id: "r1", Clids: []int64{1}, Mode: string(run.DryRun)}},
					})
					assert.Loosely(t, cls, should.HaveLength(1))
					expected.runIndexes = []int32{0}
					expected.runCountByMode["DRY_RUN"] = 1
					assert.Loosely(t, cls[1], should.Match(expected))
				})
			})

			t.Run("CL matching several config groups is never ready", func(t *ftt.Test) {
				sup.PCL(1).ConfigGroupIndexes = []int32{singIdx, anotherIdx}
				cls := do(&prjpb.Component{Clids: []int64{1}})
				assert.Loosely(t, cls, should.HaveLength(1))
				expected := &clInfo{
					pcl:            pm.MustPCL(1),
					runIndexes:     nil,
					purgingCL:      nil,
					runCountByMode: map[run.Mode]int{},

					triagedCL: triagedCL{
						purgeReasons: []*prjpb.PurgeReason{{
							ClError: &changelist.CLError{
								Kind: &changelist.CLError_WatchedByManyConfigGroups_{
									WatchedByManyConfigGroups: &changelist.CLError_WatchedByManyConfigGroups{
										ConfigGroups: []string{"singular", "another"},
									},
								},
							},
							ApplyTo: &prjpb.PurgeReason_Triggers{Triggers: &run.Triggers{CqVoteTrigger: dryRun(epoch)}},
						}},
						cqReady: false,
						deps:    nil, // not checked.
					},
				}
				assert.Loosely(t, cls[1], should.Match(expected))

				t.Run("not even if inside 1+ Runs, but Run protects from purging", func(t *ftt.Test) {
					cls := do(&prjpb.Component{
						Clids: []int64{1},
						Pruns: []*prjpb.PRun{{Id: "r1", Clids: []int64{1}, Mode: string(run.DryRun)}},
					})
					assert.Loosely(t, cls, should.HaveLength(1))
					expected.runIndexes = []int32{0}
					expected.purgeReasons = nil
					expected.runCountByMode["DRY_RUN"] = 1
					assert.Loosely(t, cls[1], should.Match(expected))
				})
			})
		})

		t.Run("Typical 1 CL component with new patchset run enabled", func(t *ftt.Test) {
			sup.pb.Pcls = []*prjpb.PCL{{
				Clid:               1,
				ConfigGroupIndexes: []int32{nprIdx},
				Status:             prjpb.PCL_OK,
				Triggers: &run.Triggers{
					CqVoteTrigger:         dryRun(epoch),
					NewPatchsetRunTrigger: newPatchsetTrigger(epoch),
				},
				Submitted: false,
				Deps:      nil,
			}}
			t.Run("new patchset upload on CL with CQ vote run being purged", func(t *ftt.Test) {
				sup.pb.PurgingCls = append(sup.pb.PurgingCls, &prjpb.PurgingCL{
					Clid: 1,
					ApplyTo: &prjpb.PurgingCL_Triggers{
						Triggers: &run.Triggers{
							CqVoteTrigger: dryRun(epoch),
						},
					},
				})
				expected := &clInfo{
					pcl:        pm.MustPCL(1),
					runIndexes: nil,
					purgingCL: &prjpb.PurgingCL{
						Clid: 1,
						ApplyTo: &prjpb.PurgingCL_Triggers{
							Triggers: &run.Triggers{
								CqVoteTrigger: dryRun(epoch),
							},
						},
					},
					runCountByMode: map[run.Mode]int{},

					triagedCL: triagedCL{
						purgeReasons: nil,
						cqReady:      false,
						nprReady:     true,
						deps:         &triagedDeps{},
					},
				}

				cls := do(&prjpb.Component{Clids: []int64{1}})
				assert.Loosely(t, cls, should.HaveLength(1))
				assert.Loosely(t, cls[1], should.Match(expected))
			})

			t.Run("new patch upload on CL with NPR being purged", func(t *ftt.Test) {
				sup.pb.PurgingCls = append(sup.pb.PurgingCls, &prjpb.PurgingCL{
					Clid: 1,
					ApplyTo: &prjpb.PurgingCL_Triggers{
						Triggers: &run.Triggers{
							NewPatchsetRunTrigger: newPatchsetTrigger(epoch),
						},
					},
				})
				expected := &clInfo{
					pcl:        pm.MustPCL(1),
					runIndexes: nil,
					purgingCL: &prjpb.PurgingCL{
						Clid: 1,
						ApplyTo: &prjpb.PurgingCL_Triggers{
							Triggers: &run.Triggers{
								NewPatchsetRunTrigger: newPatchsetTrigger(epoch),
							},
						},
					},
					runCountByMode: map[run.Mode]int{},

					triagedCL: triagedCL{
						purgeReasons: nil,
						cqReady:      true,
						nprReady:     false,
						deps:         &triagedDeps{},
					},
				}
				cls := do(&prjpb.Component{Clids: []int64{1}})
				assert.Loosely(t, cls, should.HaveLength(1))
				assert.Loosely(t, cls[1], should.Match(expected))

			})
		})
		t.Run("Triage with ongoing New Pachset Run", func(t *ftt.Test) {
			sup.pb.Pcls = []*prjpb.PCL{{
				Clid:               1,
				ConfigGroupIndexes: []int32{nprIdx},
				Status:             prjpb.PCL_OK,
				Triggers: &run.Triggers{
					NewPatchsetRunTrigger: newPatchsetTrigger(epoch),
				},
				Submitted: false,
				Deps:      nil,
			}}
			expected := &clInfo{
				pcl:            pm.MustPCL(1),
				runIndexes:     []int32{0},
				runCountByMode: map[run.Mode]int{"NEW_PATCHSET_RUN": 1},
				triagedCL: triagedCL{
					purgeReasons: nil,
					nprReady:     true,
				},
			}
			cls := do(&prjpb.Component{
				Clids: []int64{1},
				Pruns: []*prjpb.PRun{{Id: "r1", Clids: []int64{1}, Mode: string(run.NewPatchsetRun)}},
			})
			assert.Loosely(t, cls, should.HaveLength(1))
			assert.Loosely(t, cls[1], should.Match(expected))
		})
		t.Run("Single CL Runs: typical CL stack", func(t *ftt.Test) {
			// CL 3 depends on 2, which in turn depends 1.
			// Start configuration is each one is Dry-run triggered.
			sup.pb.Pcls = []*prjpb.PCL{
				{
					Clid:               1,
					ConfigGroupIndexes: []int32{singIdx},
					Status:             prjpb.PCL_OK,
					Triggers:           &run.Triggers{CqVoteTrigger: dryRun(epoch)},
					Submitted:          false,
					Deps:               nil,
				},
				{
					Clid:               2,
					ConfigGroupIndexes: []int32{singIdx},
					Status:             prjpb.PCL_OK,
					Triggers:           &run.Triggers{CqVoteTrigger: dryRun(epoch)},
					Submitted:          false,
					Deps:               []*changelist.Dep{{Clid: 1, Kind: changelist.DepKind_HARD}},
				},
				{
					Clid:               3,
					ConfigGroupIndexes: []int32{singIdx},
					Status:             prjpb.PCL_OK,
					Triggers:           &run.Triggers{CqVoteTrigger: dryRun(epoch)},
					Submitted:          false,
					Deps:               []*changelist.Dep{{Clid: 1, Kind: changelist.DepKind_SOFT}, {Clid: 2, Kind: changelist.DepKind_HARD}},
				},
			}

			t.Run("Dry run everywhere is OK", func(t *ftt.Test) {
				cls := do(&prjpb.Component{Clids: []int64{1, 2, 3}})
				assert.Loosely(t, cls, should.HaveLength(3))
				for _, info := range cls {
					assert.Loosely(t, info.cqReady, should.BeTrue)
					assert.Loosely(t, info.deps.OK(), should.BeTrue)
					assert.Loosely(t, info.lastCQVoteTriggered(), should.Resemble(epoch))
				}
			})

			t.Run("Full run at the bottom (CL1) and dry run elsewhere is also OK", func(t *ftt.Test) {
				sup.PCL(1).Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch)}
				cls := do(&prjpb.Component{Clids: []int64{1, 2, 3}})
				assert.Loosely(t, cls, should.HaveLength(3))
				for _, info := range cls {
					assert.Loosely(t, info.cqReady, should.BeTrue)
					assert.Loosely(t, info.deps.OK(), should.BeTrue)
				}
			})

			t.Run("Full Run on #3 is purged if its deps aren't submitted, but NPR is not affected", func(t *ftt.Test) {
				sup.PCL(1).Triggers = &run.Triggers{CqVoteTrigger: dryRun(epoch), NewPatchsetRunTrigger: newPatchsetTrigger(epoch)}
				sup.PCL(2).Triggers = &run.Triggers{CqVoteTrigger: dryRun(epoch), NewPatchsetRunTrigger: newPatchsetTrigger(epoch)}
				sup.PCL(3).Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch), NewPatchsetRunTrigger: newPatchsetTrigger(epoch)}
				cls := do(&prjpb.Component{Clids: []int64{1, 2, 3}})
				assert.Loosely(t, cls[1].cqReady, should.BeTrue)
				assert.Loosely(t, cls[2].cqReady, should.BeTrue)
				assert.Loosely(t, cls[1].nprReady, should.BeTrue)
				assert.Loosely(t, cls[2].nprReady, should.BeTrue)
				assert.Loosely(t, cls[3].nprReady, should.BeTrue)
				assert.Loosely(t, cls[3], should.Match(&clInfo{
					pcl:            sup.PCL(3),
					runCountByMode: map[run.Mode]int{},
					triagedCL: triagedCL{
						cqReady:  false,
						nprReady: true,
						purgeReasons: []*prjpb.PurgeReason{{
							ClError: &changelist.CLError{
								Kind: &changelist.CLError_InvalidDeps_{
									InvalidDeps: &changelist.CLError_InvalidDeps{
										SingleFullDeps: []*changelist.Dep{
											sup.PCL(3).GetDeps()[0],
										},
									},
								},
							},
							ApplyTo: &prjpb.PurgeReason_Triggers{
								Triggers: &run.Triggers{
									CqVoteTrigger: fullRun(epoch),
								},
							},
						}},
						deps: &triagedDeps{
							lastCQVoteTriggered: epoch,
							invalidDeps: &changelist.CLError_InvalidDeps{
								SingleFullDeps: []*changelist.Dep{
									sup.PCL(3).GetDeps()[0],
								},
							},
							needToTrigger: []*changelist.Dep{
								{Clid: 2, Kind: changelist.DepKind_HARD},
							},
						},
					},
				}))
			})
		})

		t.Run("Multiple CL Runs: 1<->2 and 3 depending on both", func(t *ftt.Test) {
			// CL 3 depends on 1 and 2, while 1 and 2 depend on each other (e.g. via
			// CQ-Depend).  Start configuration is each one is Dry-run triggered.
			sup.pb.Pcls = []*prjpb.PCL{
				{
					Clid:               1,
					ConfigGroupIndexes: []int32{combIdx},
					Status:             prjpb.PCL_OK,
					Triggers:           &run.Triggers{CqVoteTrigger: dryRun(epoch)},
					Submitted:          false,
					Deps:               []*changelist.Dep{{Clid: 2, Kind: changelist.DepKind_SOFT}},
				},
				{
					Clid:               2,
					ConfigGroupIndexes: []int32{combIdx},
					Status:             prjpb.PCL_OK,
					Triggers:           &run.Triggers{CqVoteTrigger: dryRun(epoch)},
					Submitted:          false,
					Deps:               []*changelist.Dep{{Clid: 1, Kind: changelist.DepKind_SOFT}},
				},
				{
					Clid:               3,
					ConfigGroupIndexes: []int32{combIdx},
					Status:             prjpb.PCL_OK,
					Triggers:           &run.Triggers{CqVoteTrigger: dryRun(epoch)},
					Submitted:          false,
					Deps:               []*changelist.Dep{{Clid: 1, Kind: changelist.DepKind_SOFT}, {Clid: 2, Kind: changelist.DepKind_SOFT}},
				},
			}

			t.Run("Happy case: all are ready", func(t *ftt.Test) {
				cls := do(&prjpb.Component{Clids: []int64{1, 2, 3}})
				assert.Loosely(t, cls, should.HaveLength(3))
				for _, info := range cls {
					assert.Loosely(t, info.cqReady, should.BeTrue)
					assert.Loosely(t, info.deps.OK(), should.BeTrue)
				}
			})

			t.Run("Full Run on #1 and #2 can co-exist, but Dry run on #3 is purged", func(t *ftt.Test) {
				// This scenario documents current CQDaemon behavior. This isn't desired
				// long term though.
				sup.PCL(1).Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch)}
				sup.PCL(2).Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch)}
				cls := do(&prjpb.Component{Clids: []int64{1, 2, 3}})
				assert.Loosely(t, cls[1].cqReady, should.BeTrue)
				assert.Loosely(t, cls[2].cqReady, should.BeTrue)
				assert.Loosely(t, cls[3], should.Match(&clInfo{
					pcl:            sup.PCL(3),
					runCountByMode: map[run.Mode]int{},
					triagedCL: triagedCL{
						cqReady: false,
						purgeReasons: []*prjpb.PurgeReason{{
							ClError: &changelist.CLError{
								Kind: &changelist.CLError_InvalidDeps_{
									InvalidDeps: &changelist.CLError_InvalidDeps{
										CombinableMismatchedMode: sup.PCL(3).GetDeps(),
									},
								},
							},
							ApplyTo: &prjpb.PurgeReason_Triggers{
								Triggers: &run.Triggers{
									CqVoteTrigger: dryRun(epoch),
								},
							},
						}},
						deps: &triagedDeps{
							lastCQVoteTriggered: epoch,
							invalidDeps: &changelist.CLError_InvalidDeps{
								CombinableMismatchedMode: sup.PCL(3).GetDeps(),
							},
						},
					},
				}))
			})

			t.Run("Dependencies in diff config groups are not allowed", func(t *ftt.Test) {
				sup.PCL(1).ConfigGroupIndexes = []int32{combIdx}    // depends on 2
				sup.PCL(2).ConfigGroupIndexes = []int32{anotherIdx} // depends on 1
				sup.PCL(3).ConfigGroupIndexes = []int32{combIdx}    // depends on 1(OK) and 2.
				cls := do(&prjpb.Component{Clids: []int64{1, 2, 3}})
				for _, info := range cls {
					assert.Loosely(t, info.cqReady, should.BeFalse)
					assert.Loosely(t, info.purgeReasons, should.Resemble([]*prjpb.PurgeReason{{
						ClError: &changelist.CLError{
							Kind: &changelist.CLError_InvalidDeps_{
								InvalidDeps: info.triagedCL.deps.invalidDeps,
							},
						},
						ApplyTo: &prjpb.PurgeReason_Triggers{
							Triggers: &run.Triggers{
								CqVoteTrigger: dryRun(epoch),
							},
						},
					}}))
				}

				t.Run("unless dependency is already submitted", func(t *ftt.Test) {
					sup.PCL(2).Triggers = nil
					sup.PCL(2).Submitted = true

					cls := do(&prjpb.Component{Clids: []int64{1, 3}})
					for _, info := range cls {
						assert.Loosely(t, info.cqReady, should.BeTrue)
						assert.Loosely(t, info.purgeReasons, should.BeNil)
						assert.Loosely(t, info.deps.submitted, should.Resemble([]*changelist.Dep{{Clid: 2, Kind: changelist.DepKind_SOFT}}))
					}
				})
			})
		})

		t.Run("Ready CLs can have not yet loaded dependencies", func(t *ftt.Test) {
			sup.pb.Pcls = []*prjpb.PCL{
				{
					Clid:   1,
					Status: prjpb.PCL_UNKNOWN,
				},
				{
					Clid:               2,
					ConfigGroupIndexes: []int32{combIdx},
					Status:             prjpb.PCL_OK,
					Triggers:           &run.Triggers{CqVoteTrigger: dryRun(epoch)},
					Deps:               []*changelist.Dep{{Clid: 1, Kind: changelist.DepKind_SOFT}},
				},
			}
			cls := do(&prjpb.Component{Clids: []int64{2}})
			assert.Loosely(t, cls[2], should.Match(&clInfo{
				pcl:            sup.PCL(2),
				runCountByMode: map[run.Mode]int{},
				triagedCL: triagedCL{
					cqReady: true,
					deps:    &triagedDeps{notYetLoaded: sup.PCL(2).GetDeps()},
				},
			}))
		})

		t.Run("Multiple CL Runs with chained CQ votes", func(t *ftt.Test) {
			const clid1, clid2, clid3, clid4 = 1, 2, 3, 4

			newCL := func(clid int64, deps ...*changelist.Dep) *prjpb.PCL {
				return &prjpb.PCL{
					Clid:               clid,
					ConfigGroupIndexes: []int32{singIdx},
					Status:             prjpb.PCL_OK,
					Submitted:          false,
					Deps:               deps,
				}
			}
			Dep := func(clid int64) *changelist.Dep {
				return &changelist.Dep{Clid: clid, Kind: changelist.DepKind_HARD}
			}
			voter := "test@example.org"
			sup.pb.Pcls = []*prjpb.PCL{
				newCL(clid1),
				newCL(clid2, Dep(clid1)),
				newCL(clid3, Dep(clid1), Dep(clid2)),
				newCL(clid4, Dep(clid1), Dep(clid2), Dep(clid3)),
			}

			t.Run("CQ vote on a child CL", func(t *ftt.Test) {
				// Trigger CQ on the CL 3 only.
				sup.pb.Pcls[2].Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch)}
				sup.pb.Pcls[2].Triggers.CqVoteTrigger.Email = voter
				cls := do(&prjpb.Component{Clids: []int64{clid1, clid2, clid3, clid4}})
				assert.Loosely(t, cls, should.HaveLength(4))

				// - all CLs should be not-cq-ready.
				assert.Loosely(t, cls[clid1].cqReady, should.BeFalse)
				assert.Loosely(t, cls[clid2].cqReady, should.BeFalse)
				assert.Loosely(t, cls[clid3].cqReady, should.BeFalse)
				assert.Loosely(t, cls[clid4].cqReady, should.BeFalse)
				// - CL3 should have CL1, and CL2 in needToTrigger, whereas
				// the others shouldn't have any, because only CL3 has
				// the CQ vote. Deps are not triaged, unless a given CL has
				// a CQ vote.
				assert.Loosely(t, cls[clid1].deps, should.BeNil)
				assert.Loosely(t, cls[clid2].deps, should.BeNil)
				assert.Loosely(t, cls[clid3].deps.needToTrigger, should.Resemble([]*changelist.Dep{
					Dep(clid1), Dep(clid2),
				}))
				assert.Loosely(t, cls[clid4].deps, should.BeNil)
			})

			t.Run("CQ vote on multi CLs", func(t *ftt.Test) {
				// Trigger CQ on the CL 2 and 4.
				sup.pb.Pcls[1].Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch)}
				sup.pb.Pcls[1].Triggers.CqVoteTrigger.Email = voter
				sup.pb.Pcls[3].Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch)}
				sup.pb.Pcls[3].Triggers.CqVoteTrigger.Email = voter
				cls := do(&prjpb.Component{Clids: []int64{clid1, clid2, clid3, clid4}})
				assert.Loosely(t, cls, should.HaveLength(4))

				// - all CLs should not be cq-ready.
				assert.Loosely(t, cls[clid1].cqReady, should.BeFalse)
				assert.Loosely(t, cls[clid2].cqReady, should.BeFalse)
				assert.Loosely(t, cls[clid3].cqReady, should.BeFalse)
				assert.Loosely(t, cls[clid4].cqReady, should.BeFalse)
				// - CL3 should have CL1, and CL2 in needToTrigger, whereas
				// the others shouldn't have any, because only CL3 has
				// the CQ vote. Deps are not triaged, unless a given CL has
				// a CQ vote.
				assert.Loosely(t, cls[clid1].deps, should.BeNil)
				assert.Loosely(t, cls[clid2].deps.needToTrigger, should.Resemble([]*changelist.Dep{
					Dep(clid1),
				}))
				assert.Loosely(t, cls[clid3].deps, should.BeNil)
				assert.Loosely(t, cls[clid4].deps.needToTrigger, should.Resemble([]*changelist.Dep{
					// Should NOT have clid4 in needToTrigger, as it is already
					// voted.
					Dep(clid1), Dep(clid3),
				}))
			})

			t.Run("CqReady if all voted", func(t *ftt.Test) {
				// Vote on all the CLs.
				for i := 0; i < 4; i++ {
					sup.pb.Pcls[i].Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch)}
					sup.pb.Pcls[i].Triggers.CqVoteTrigger.Email = voter
				}
				cls := do(&prjpb.Component{Clids: []int64{clid1, clid2, clid3, clid4}})
				assert.Loosely(t, cls, should.HaveLength(4))

				// They all should be cq-ready.
				assert.Loosely(t, cls[clid1].cqReady, should.BeTrue)
				assert.Loosely(t, cls[clid2].cqReady, should.BeTrue)
				assert.Loosely(t, cls[clid3].cqReady, should.BeTrue)
				assert.Loosely(t, cls[clid4].cqReady, should.BeTrue)

				t.Run("unless there is an inflight TriggeringCLDeps{}", func(t *ftt.Test) {
					sup.pb.TriggeringClDeps, _ = sup.pb.COWTriggeringCLDeps(nil, []*prjpb.TriggeringCLDeps{
						{OperationId: "op-1", OriginClid: clid4, DepClids: []int64{1, 2, 3}},
					})
					cls := do(&prjpb.Component{Clids: []int64{clid1, clid2, clid3, clid4}})
					assert.Loosely(t, cls, should.HaveLength(4))

					// They all should not be cq-ready.
					assert.Loosely(t, cls[clid1].cqReady, should.BeFalse)
					assert.Loosely(t, cls[clid2].cqReady, should.BeFalse)
					assert.Loosely(t, cls[clid3].cqReady, should.BeFalse)
					assert.Loosely(t, cls[clid4].cqReady, should.BeFalse)
				})
			})
		})
	})
}
