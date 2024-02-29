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
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func shouldResembleTriagedCL(actual any, expected ...any) string {
	if len(expected) != 1 {
		return fmt.Sprintf("expected 1 value, got %d", len(expected))
	}
	exp := expected[0] // this may be nil
	a, ok := actual.(*clInfo)
	if !ok {
		return fmt.Sprintf("Wrong actual type %T, must be %T", actual, a)
	}
	if err := ShouldHaveSameTypeAs(actual, exp); err != "" {
		return err
	}
	b := exp.(*clInfo)
	switch {
	case a == b:
		return ""
	case a == nil:
		return "actual is nil, but non-nil was expected"
	case b == nil:
		return "actual is not-nil, but nil was expected"
	}

	buf := strings.Builder{}
	for _, err := range []string{
		ShouldResemble(a.cqReady, b.cqReady),
		ShouldResemble(a.nprReady, b.nprReady),
		ShouldResemble(a.runIndexes, b.runIndexes),
		cvtesting.SafeShouldResemble(a.deps, b.deps),
		ShouldResembleProto(a.pcl, b.pcl),
		ShouldResembleProto(a.purgingCL, b.purgingCL),
		ShouldResembleProto(a.purgeReasons, b.purgeReasons),
		ShouldResembleProto(a.triggeringCLDeps, b.triggeringCLDeps),
	} {
		if err != "" {
			buf.WriteRune(' ')
			buf.WriteString(err)
		}
	}
	return strings.TrimSpace(buf.String())
}

func TestCLsTriage(t *testing.T) {
	t.Parallel()

	Convey("Component's PCL deps triage", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		// Truncate start time point s.t. easy to see diff in test failures.
		epoch := testclock.TestRecentTimeUTC.Truncate(10000 * time.Second)
		dryRun := func(t time.Time) *run.Trigger {
			return &run.Trigger{Mode: string(run.DryRun), Time: timestamppb.New(t)}
		}
		fullRun := func(t time.Time) *run.Trigger {
			return &run.Trigger{Mode: string(run.FullRun), Time: timestamppb.New(t)}
		}
		newPatchsetTrigger := func(t time.Time) *run.Trigger {
			return &run.Trigger{Mode: string(run.NewPatchsetRun), Time: timestamppb.New(t)}
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
			So(sup.pb, ShouldResembleProto, &backup) // must not be modified
			return cls
		}

		Convey("Typical 1 CL component without deps", func() {
			sup.pb.Pcls = []*prjpb.PCL{{
				Clid:               1,
				ConfigGroupIndexes: []int32{singIdx},
				Status:             prjpb.PCL_OK,
				Triggers:           &run.Triggers{CqVoteTrigger: dryRun(epoch)},
				Submitted:          false,
				Deps:               nil,
			}}

			Convey("Ready without runs", func() {
				cls := do(&prjpb.Component{Clids: []int64{1}})
				So(cls, ShouldHaveLength, 1)
				expected := &clInfo{
					pcl:        pm.MustPCL(1),
					runIndexes: nil,
					purgingCL:  nil,

					triagedCL: triagedCL{
						purgeReasons: nil,
						cqReady:      true,
						deps:         &triagedDeps{},
					},
				}
				So(cls[1], shouldResembleTriagedCL, expected)

				Convey("ready may also be in 1+ Runs", func() {
					cls := do(&prjpb.Component{
						Clids: []int64{1},
						Pruns: []*prjpb.PRun{{Id: "r1", Clids: []int64{1}, Mode: string(run.DryRun)}},
					})
					So(cls, ShouldHaveLength, 1)
					expected.runIndexes = []int32{0}
					So(cls[1], shouldResembleTriagedCL, expected)
				})
			})

			Convey("CL already with Errors is not ready", func() {
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
				So(cls, ShouldHaveLength, 1)
				expected := &clInfo{
					pcl:        pm.MustPCL(1),
					runIndexes: nil,
					purgingCL:  nil,

					triagedCL: triagedCL{
						purgeReasons: sup.pb.Pcls[0].GetPurgeReasons(),
					},
				}
				So(cls[1], shouldResembleTriagedCL, expected)
			})

			Convey("Already purged is never ready", func() {
				sup.pb.PurgingCls = []*prjpb.PurgingCL{{
					Clid:    1,
					ApplyTo: &prjpb.PurgingCL_AllActiveTriggers{AllActiveTriggers: true},
				}}
				cls := do(&prjpb.Component{Clids: []int64{1}})
				So(cls, ShouldHaveLength, 1)
				expected := &clInfo{
					pcl:        pm.MustPCL(1),
					runIndexes: nil,
					purgingCL:  pm.PurgingCL(1),

					triagedCL: triagedCL{
						purgeReasons: nil,
						cqReady:      false,
						deps:         &triagedDeps{},
					},
				}
				So(cls[1], shouldResembleTriagedCL, expected)

				Convey("not even if inside 1+ Runs", func() {
					cls := do(&prjpb.Component{
						Clids: []int64{1},
						Pruns: []*prjpb.PRun{{Id: "r1", Clids: []int64{1}, Mode: string(run.DryRun)}},
					})
					So(cls, ShouldHaveLength, 1)
					expected.runIndexes = []int32{0}
					So(cls[1], shouldResembleTriagedCL, expected)
				})
			})

			Convey("CL matching several config groups is never ready", func() {
				sup.PCL(1).ConfigGroupIndexes = []int32{singIdx, anotherIdx}
				cls := do(&prjpb.Component{Clids: []int64{1}})
				So(cls, ShouldHaveLength, 1)
				expected := &clInfo{
					pcl:        pm.MustPCL(1),
					runIndexes: nil,
					purgingCL:  nil,

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
				So(cls[1], shouldResembleTriagedCL, expected)

				Convey("not even if inside 1+ Runs, but Run protects from purging", func() {
					cls := do(&prjpb.Component{
						Clids: []int64{1},
						Pruns: []*prjpb.PRun{{Id: "r1", Clids: []int64{1}, Mode: string(run.DryRun)}},
					})
					So(cls, ShouldHaveLength, 1)
					expected.runIndexes = []int32{0}
					expected.purgeReasons = nil
					So(cls[1], shouldResembleTriagedCL, expected)
				})
			})
		})

		Convey("Typical 1 CL component with new patchset run enabled", func() {
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
			Convey("new patchset upload on CL with CQ vote run being purged", func() {
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

					triagedCL: triagedCL{
						purgeReasons: nil,
						cqReady:      false,
						nprReady:     true,
						deps:         &triagedDeps{},
					},
				}

				cls := do(&prjpb.Component{Clids: []int64{1}})
				So(cls, ShouldHaveLength, 1)
				So(cls[1], shouldResembleTriagedCL, expected)
			})

			Convey("new patch upload on CL with NPR being purged", func() {
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
					triagedCL: triagedCL{
						purgeReasons: nil,
						cqReady:      true,
						nprReady:     false,
						deps:         &triagedDeps{},
					},
				}
				cls := do(&prjpb.Component{Clids: []int64{1}})
				So(cls, ShouldHaveLength, 1)
				So(cls[1], shouldResembleTriagedCL, expected)

			})
		})
		Convey("Triage with ongoing New Pachset Run", func() {
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
				pcl:        pm.MustPCL(1),
				runIndexes: []int32{0},
				triagedCL: triagedCL{
					purgeReasons: nil,
					nprReady:     true,
				},
			}
			cls := do(&prjpb.Component{
				Clids: []int64{1},
				Pruns: []*prjpb.PRun{{Id: "r1", Clids: []int64{1}, Mode: string(run.NewPatchsetRun)}},
			})
			So(cls, ShouldHaveLength, 1)
			So(cls[1], shouldResembleTriagedCL, expected)
		})
		Convey("Single CL Runs: typical CL stack", func() {
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

			Convey("Dry run everywhere is OK", func() {
				cls := do(&prjpb.Component{Clids: []int64{1, 2, 3}})
				So(cls, ShouldHaveLength, 3)
				for _, info := range cls {
					So(info.cqReady, ShouldBeTrue)
					So(info.deps.OK(), ShouldBeTrue)
					So(info.lastCQVoteTriggered(), ShouldResemble, epoch)
				}
			})

			Convey("Full run at the bottom (CL1) and dry run elsewhere is also OK", func() {
				sup.PCL(1).Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch)}
				cls := do(&prjpb.Component{Clids: []int64{1, 2, 3}})
				So(cls, ShouldHaveLength, 3)
				for _, info := range cls {
					So(info.cqReady, ShouldBeTrue)
					So(info.deps.OK(), ShouldBeTrue)
				}
			})

			Convey("Full Run on #3 is purged if its deps aren't submitted, but NPR is not affected", func() {
				sup.PCL(1).Triggers = &run.Triggers{CqVoteTrigger: dryRun(epoch), NewPatchsetRunTrigger: newPatchsetTrigger(epoch)}
				sup.PCL(2).Triggers = &run.Triggers{CqVoteTrigger: dryRun(epoch), NewPatchsetRunTrigger: newPatchsetTrigger(epoch)}
				sup.PCL(3).Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch), NewPatchsetRunTrigger: newPatchsetTrigger(epoch)}
				cls := do(&prjpb.Component{Clids: []int64{1, 2, 3}})
				So(cls[1].cqReady, ShouldBeTrue)
				So(cls[2].cqReady, ShouldBeTrue)
				So(cls[1].nprReady, ShouldBeTrue)
				So(cls[2].nprReady, ShouldBeTrue)
				So(cls[3].nprReady, ShouldBeTrue)
				So(cls[3], shouldResembleTriagedCL, &clInfo{
					pcl: sup.PCL(3),
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
				})
			})
		})

		Convey("Multiple CL Runs: 1<->2 and 3 depending on both", func() {
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

			Convey("Happy case: all are ready", func() {
				cls := do(&prjpb.Component{Clids: []int64{1, 2, 3}})
				So(cls, ShouldHaveLength, 3)
				for _, info := range cls {
					So(info.cqReady, ShouldBeTrue)
					So(info.deps.OK(), ShouldBeTrue)
				}
			})

			Convey("Full Run on #1 and #2 can co-exist, but Dry run on #3 is purged", func() {
				// This scenario documents current CQDaemon behavior. This isn't desired
				// long term though.
				sup.PCL(1).Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch)}
				sup.PCL(2).Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch)}
				cls := do(&prjpb.Component{Clids: []int64{1, 2, 3}})
				So(cls[1].cqReady, ShouldBeTrue)
				So(cls[2].cqReady, ShouldBeTrue)
				So(cls[3], shouldResembleTriagedCL, &clInfo{
					pcl: sup.PCL(3),
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
				})
			})

			Convey("Dependencies in diff config groups are not allowed", func() {
				sup.PCL(1).ConfigGroupIndexes = []int32{combIdx}    // depends on 2
				sup.PCL(2).ConfigGroupIndexes = []int32{anotherIdx} // depends on 1
				sup.PCL(3).ConfigGroupIndexes = []int32{combIdx}    // depends on 1(OK) and 2.
				cls := do(&prjpb.Component{Clids: []int64{1, 2, 3}})
				for _, info := range cls {
					So(info.cqReady, ShouldBeFalse)
					So(info.purgeReasons, ShouldResembleProto, []*prjpb.PurgeReason{{
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
					}})
				}

				Convey("unless dependency is already submitted", func() {
					sup.PCL(2).Triggers = nil
					sup.PCL(2).Submitted = true

					cls := do(&prjpb.Component{Clids: []int64{1, 3}})
					for _, info := range cls {
						So(info.cqReady, ShouldBeTrue)
						So(info.purgeReasons, ShouldBeNil)
						So(info.deps.submitted, ShouldResembleProto, []*changelist.Dep{{Clid: 2, Kind: changelist.DepKind_SOFT}})
					}
				})
			})
		})

		Convey("Ready CLs can have not yet loaded dependencies", func() {
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
			So(cls[2], shouldResembleTriagedCL, &clInfo{
				pcl: sup.PCL(2),
				triagedCL: triagedCL{
					cqReady: true,
					deps:    &triagedDeps{notYetLoaded: sup.PCL(2).GetDeps()},
				},
			})
		})

		Convey("Multiple CL Runs with chained CQ votes", func() {
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

			Convey("CQ vote on a child CL", func() {
				// Trigger CQ on the CL 3 only.
				sup.pb.Pcls[2].Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch)}
				sup.pb.Pcls[2].Triggers.CqVoteTrigger.Email = voter
				cls := do(&prjpb.Component{Clids: []int64{clid1, clid2, clid3, clid4}})
				So(cls, ShouldHaveLength, 4)

				// - all CLs should be not-cq-ready.
				So(cls[clid1].cqReady, ShouldBeFalse)
				So(cls[clid2].cqReady, ShouldBeFalse)
				So(cls[clid3].cqReady, ShouldBeFalse)
				So(cls[clid4].cqReady, ShouldBeFalse)
				// - CL3 should have CL1, and CL2 in needToTrigger, whereas
				// the others shouldn't have any, because only CL3 has
				// the CQ vote. Deps are not triaged, unless a given CL has
				// a CQ vote.
				So(cls[clid1].deps, ShouldBeNil)
				So(cls[clid2].deps, ShouldBeNil)
				So(cls[clid3].deps.needToTrigger, ShouldResembleProto, []*changelist.Dep{
					Dep(clid1), Dep(clid2),
				})
				So(cls[clid4].deps, ShouldBeNil)
			})

			Convey("CQ vote on multi CLs", func() {
				// Trigger CQ on the CL 2 and 4.
				sup.pb.Pcls[1].Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch)}
				sup.pb.Pcls[1].Triggers.CqVoteTrigger.Email = voter
				sup.pb.Pcls[3].Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch)}
				sup.pb.Pcls[3].Triggers.CqVoteTrigger.Email = voter
				cls := do(&prjpb.Component{Clids: []int64{clid1, clid2, clid3, clid4}})
				So(cls, ShouldHaveLength, 4)

				// - all CLs should not be cq-ready.
				So(cls[clid1].cqReady, ShouldBeFalse)
				So(cls[clid2].cqReady, ShouldBeFalse)
				So(cls[clid3].cqReady, ShouldBeFalse)
				So(cls[clid4].cqReady, ShouldBeFalse)
				// - CL3 should have CL1, and CL2 in needToTrigger, whereas
				// the others shouldn't have any, because only CL3 has
				// the CQ vote. Deps are not triaged, unless a given CL has
				// a CQ vote.
				So(cls[clid1].deps, ShouldBeNil)
				So(cls[clid2].deps.needToTrigger, ShouldResembleProto, []*changelist.Dep{
					Dep(clid1),
				})
				So(cls[clid3].deps, ShouldBeNil)
				So(cls[clid4].deps.needToTrigger, ShouldResembleProto, []*changelist.Dep{
					// Should NOT have clid4 in needToTrigger, as it is already
					// voted.
					Dep(clid1), Dep(clid3),
				})
			})

			Convey("CqReady if all voted", func() {
				// Vote on all the CLs.
				for i := 0; i < 4; i++ {
					sup.pb.Pcls[i].Triggers = &run.Triggers{CqVoteTrigger: fullRun(epoch)}
					sup.pb.Pcls[i].Triggers.CqVoteTrigger.Email = voter
				}
				cls := do(&prjpb.Component{Clids: []int64{clid1, clid2, clid3, clid4}})
				So(cls, ShouldHaveLength, 4)

				// They all should be cq-ready.
				So(cls[clid1].cqReady, ShouldBeTrue)
				So(cls[clid2].cqReady, ShouldBeTrue)
				So(cls[clid3].cqReady, ShouldBeTrue)
				So(cls[clid4].cqReady, ShouldBeTrue)

				Convey("unless there is an inflight TriggeringCLDeps{}", func() {
					sup.pb.TriggeringClDeps, _ = sup.pb.COWTriggeringCLDeps(nil, []*prjpb.TriggeringCLDeps{
						{OperationId: "op-1", OriginClid: clid4, DepClids: []int64{1, 2, 3}},
					})
					cls := do(&prjpb.Component{Clids: []int64{clid1, clid2, clid3, clid4}})
					So(cls, ShouldHaveLength, 4)

					// They all should not be cq-ready.
					So(cls[clid1].cqReady, ShouldBeFalse)
					So(cls[clid2].cqReady, ShouldBeFalse)
					So(cls[clid3].cqReady, ShouldBeFalse)
					So(cls[clid4].cqReady, ShouldBeFalse)
				})
			})
		})
	})
}
