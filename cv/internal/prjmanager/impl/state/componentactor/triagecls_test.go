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

package componentactor

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func shouldResembleTriagedCL(actual interface{}, expected ...interface{}) string {
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
		ShouldResemble(a.ready, b.ready),
		ShouldResemble(a.runIndexes, b.runIndexes),
		shouldResembleTriagedDeps(a.deps, b.deps),
		ShouldResembleProto(a.pcl, b.pcl),
		ShouldResembleProto(a.purgingCL, b.purgingCL),
		ShouldResembleProto(a.purgeReasons, b.purgeReasons),
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
		// Truncate start time point s.t. easy to see diff in test failures.
		epoch := testclock.TestRecentTimeUTC.Truncate(10000 * time.Second)
		dryRun := func(t time.Time) *run.Trigger {
			return &run.Trigger{Mode: string(run.DryRun), Time: timestamppb.New(t)}
		}
		fullRun := func(t time.Time) *run.Trigger {
			return &run.Trigger{Mode: string(run.FullRun), Time: timestamppb.New(t)}
		}

		sup := &simplePMState{
			pb: &prjpb.PState{},
			cgs: []*config.ConfigGroup{
				{ID: "hash/singular", Content: &cfgpb.ConfigGroup{}},
				{ID: "hash/combinable", Content: &cfgpb.ConfigGroup{CombineCls: &cfgpb.CombineCLs{}}},
				{ID: "hash/another", Content: &cfgpb.ConfigGroup{}},
			},
		}
		const singIdx, combIdx, anotherIdx = 0, 1, 2

		triageCLs := func(c *prjpb.Component) *Actor {
			sup.pb.Components = []*prjpb.Component{c} // include it in backup
			backup := prjpb.PState{}
			proto.Merge(&backup, sup.pb)

			a := newActor(c, sup)
			a.triageCLs()
			So(sup.pb, ShouldResembleProto, &backup) // must not be modified
			return a
		}

		Convey("Typical 1 CL component without deps", func() {
			sup.pb.Pcls = []*prjpb.PCL{{
				Clid:               1,
				ConfigGroupIndexes: []int32{singIdx},
				Status:             prjpb.PCL_OK,
				Trigger:            dryRun(epoch),
				Submitted:          false,
				Deps:               nil,
			}}

			Convey("Ready without runs", func() {
				a := triageCLs(&prjpb.Component{Clids: []int64{1}})
				So(a.reverseDeps, ShouldBeEmpty)
				So(a.cls, ShouldHaveLength, 1)
				expected := &clInfo{
					pcl:        a.s.MustPCL(1),
					runIndexes: nil,
					purgingCL:  nil,

					triagedCL: triagedCL{
						purgeReasons: nil,
						ready:        true,
						deps:         &triagedDeps{},
					},
				}
				So(a.cls[1], shouldResembleTriagedCL, expected)

				Convey("ready may also be in 1+ Runs", func() {
					a := triageCLs(&prjpb.Component{
						Clids: []int64{1},
						Pruns: []*prjpb.PRun{{Id: "r1", Clids: []int64{1}}},
					})
					So(a.cls, ShouldHaveLength, 1)
					expected.runIndexes = []int32{0}
					So(a.cls[1], shouldResembleTriagedCL, expected)
				})
			})

			Convey("CL already with Errors is not ready", func() {
				sup.pb.Pcls[0].Errors = []*changelist.CLError{
					{Kind: &changelist.CLError_OwnerLacksEmail{OwnerLacksEmail: true}},
					{Kind: &changelist.CLError_UnsupportedMode{UnsupportedMode: "CUSTOM_RUN"}},
				}
				a := triageCLs(&prjpb.Component{Clids: []int64{1}})
				So(a.cls, ShouldHaveLength, 1)
				expected := &clInfo{
					pcl:        a.s.MustPCL(1),
					runIndexes: nil,
					purgingCL:  nil,

					triagedCL: triagedCL{
						purgeReasons: sup.pb.Pcls[0].Errors,
					},
				}
				So(a.cls[1], shouldResembleTriagedCL, expected)
			})

			Convey("Already purged is never ready", func() {
				sup.pb.PurgingCls = []*prjpb.PurgingCL{{Clid: 1}}
				a := triageCLs(&prjpb.Component{Clids: []int64{1}})
				So(a.cls, ShouldHaveLength, 1)
				expected := &clInfo{
					pcl:        a.s.MustPCL(1),
					runIndexes: nil,
					purgingCL:  a.s.PurgingCL(1),

					triagedCL: triagedCL{
						purgeReasons: nil,
						ready:        false,
						deps:         &triagedDeps{},
					},
				}
				So(a.cls[1], shouldResembleTriagedCL, expected)

				Convey("not even if inside 1+ Runs", func() {
					a := triageCLs(&prjpb.Component{
						Clids: []int64{1},
						Pruns: []*prjpb.PRun{{Id: "r1", Clids: []int64{1}}},
					})
					So(a.cls, ShouldHaveLength, 1)
					expected.runIndexes = []int32{0}
					So(a.cls[1], shouldResembleTriagedCL, expected)
				})
			})

			Convey("CL matching several config groups is never ready", func() {
				sup.PCL(1).ConfigGroupIndexes = []int32{singIdx, anotherIdx}
				a := triageCLs(&prjpb.Component{Clids: []int64{1}})
				So(a.cls, ShouldHaveLength, 1)
				expected := &clInfo{
					pcl:        a.s.MustPCL(1),
					runIndexes: nil,
					purgingCL:  nil,

					triagedCL: triagedCL{
						purgeReasons: []*changelist.CLError{
							{
								Kind: &changelist.CLError_WatchedByManyConfigGroups_{
									WatchedByManyConfigGroups: &changelist.CLError_WatchedByManyConfigGroups{
										ConfigGroups: []string{"singular", "another"},
									},
								},
							},
						},
						ready: false,
						deps:  nil, // not checked.
					},
				}
				So(a.cls[1], shouldResembleTriagedCL, expected)

				Convey("not even if inside 1+ Runs, but Run protects from purging", func() {
					a := triageCLs(&prjpb.Component{
						Clids: []int64{1},
						Pruns: []*prjpb.PRun{{Id: "r1", Clids: []int64{1}}},
					})
					So(a.cls, ShouldHaveLength, 1)
					expected.runIndexes = []int32{0}
					expected.purgeReasons = nil
					So(a.cls[1], shouldResembleTriagedCL, expected)
				})
			})
		})

		Convey("Single CL Runs: typical CL stack", func() {
			// CL 3 depends on 2, which in turn depends 1.
			// Start configuraiton is each one is Dry-run triggered.
			sup.pb.Pcls = []*prjpb.PCL{
				{
					Clid:               1,
					ConfigGroupIndexes: []int32{singIdx},
					Status:             prjpb.PCL_OK,
					Trigger:            dryRun(epoch),
					Submitted:          false,
					Deps:               nil,
				},
				{
					Clid:               2,
					ConfigGroupIndexes: []int32{singIdx},
					Status:             prjpb.PCL_OK,
					Trigger:            dryRun(epoch),
					Submitted:          false,
					Deps:               []*changelist.Dep{{Clid: 1, Kind: changelist.DepKind_HARD}},
				},
				{
					Clid:               3,
					ConfigGroupIndexes: []int32{singIdx},
					Status:             prjpb.PCL_OK,
					Trigger:            dryRun(epoch),
					Submitted:          false,
					Deps:               []*changelist.Dep{{Clid: 1, Kind: changelist.DepKind_SOFT}, {Clid: 2, Kind: changelist.DepKind_HARD}},
				},
			}

			Convey("Dry run everywhere is OK, and reverseDeps should be computed", func() {
				a := triageCLs(&prjpb.Component{Clids: []int64{1, 2, 3}})
				So(sortedRevDeps(a.reverseDeps), ShouldResemble, map[int64][]int64{1: {2, 3}, 2: {3}})
				So(a.cls, ShouldHaveLength, 3)
				for _, info := range a.cls {
					So(info.ready, ShouldBeTrue)
					So(info.deps.OK(), ShouldBeTrue)
					So(info.lastTriggered(), ShouldResemble, epoch)
				}
			})

			Convey("Full run at the bottom (CL1) and dry run elsewhere is also OK", func() {
				sup.PCL(1).Trigger = fullRun(epoch)
				a := triageCLs(&prjpb.Component{Clids: []int64{1, 2, 3}})
				So(sortedRevDeps(a.reverseDeps), ShouldResemble, map[int64][]int64{1: {2, 3}, 2: {3}})
				So(a.cls, ShouldHaveLength, 3)
				for _, info := range a.cls {
					So(info.ready, ShouldBeTrue)
					So(info.deps.OK(), ShouldBeTrue)
				}
			})

			Convey("Full Run on #3 is purged if its deps aren't submitted", func() {
				sup.PCL(3).Trigger = fullRun(epoch)
				a := triageCLs(&prjpb.Component{Clids: []int64{1, 2, 3}})
				// reverseDeps can lead to ready CLs only.
				So(a.reverseDeps, ShouldResemble, map[int64][]int64{1: {2}})
				So(a.cls[1].ready, ShouldBeTrue)
				So(a.cls[2].ready, ShouldBeTrue)
				So(a.cls[3], shouldResembleTriagedCL, &clInfo{
					pcl: sup.PCL(3),
					triagedCL: triagedCL{
						ready: false,
						purgeReasons: []*changelist.CLError{
							{
								Kind: &changelist.CLError_InvalidDeps_{
									InvalidDeps: &changelist.CLError_InvalidDeps{
										IncompatMode: sup.PCL(3).GetDeps(),
									},
								},
							},
						},
						deps: &triagedDeps{
							lastTriggered: epoch,
							incompatMode:  sup.PCL(3).GetDeps(),
						},
					},
				})
			})

			Convey("CL1 submitted but still with Run, CL2 CQ+1 is OK, CL3 CQ+2 is purged", func() {
				sup.PCL(1).Trigger = nil
				sup.PCL(1).Submitted = true
				// PCL(2) is still not submitted.
				sup.PCL(3).Trigger = fullRun(epoch)
				a := triageCLs(&prjpb.Component{
					Clids: []int64{1, 2, 3},
					Pruns: []*prjpb.PRun{{Id: "r1", Clids: []int64{1}}},
				})
				// reverseDeps can't have 2->3 because 3 is invalid, and 1->2 because 1
				// is submitted.
				So(a.reverseDeps, ShouldBeEmpty)
				So(a.cls[2].ready, ShouldBeTrue)
				So(a.cls[2].deps, shouldResembleTriagedDeps, &triagedDeps{
					submitted: []*changelist.Dep{{Clid: 1, Kind: changelist.DepKind_HARD}},
				})
				So(a.cls[3], shouldResembleTriagedCL, &clInfo{
					pcl: sup.PCL(3),
					triagedCL: triagedCL{
						ready: false,
						purgeReasons: []*changelist.CLError{
							{
								Kind: &changelist.CLError_InvalidDeps_{
									InvalidDeps: &changelist.CLError_InvalidDeps{
										IncompatMode: []*changelist.Dep{{Clid: 2, Kind: changelist.DepKind_HARD}},
									},
								},
							},
						},
						deps: &triagedDeps{
							lastTriggered: epoch.UTC(),
							submitted:     []*changelist.Dep{{Clid: 1, Kind: changelist.DepKind_SOFT}},
							incompatMode:  []*changelist.Dep{{Clid: 2, Kind: changelist.DepKind_HARD}},
						},
					},
				})
			})
		})

		Convey("Multiple CL Runs: 1<->2 and 3 depending on both", func() {
			// CL 3 depends on 1 and 2, while 1 and 2 depend on each other (e.g. via
			// CQ-Depend).  Start configuraiton is each one is Dry-run triggered.
			sup.pb.Pcls = []*prjpb.PCL{
				{
					Clid:               1,
					ConfigGroupIndexes: []int32{combIdx},
					Status:             prjpb.PCL_OK,
					Trigger:            dryRun(epoch),
					Submitted:          false,
					Deps:               []*changelist.Dep{{Clid: 2, Kind: changelist.DepKind_SOFT}},
				},
				{
					Clid:               2,
					ConfigGroupIndexes: []int32{combIdx},
					Status:             prjpb.PCL_OK,
					Trigger:            dryRun(epoch),
					Submitted:          false,
					Deps:               []*changelist.Dep{{Clid: 1, Kind: changelist.DepKind_SOFT}},
				},
				{
					Clid:               3,
					ConfigGroupIndexes: []int32{combIdx},
					Status:             prjpb.PCL_OK,
					Trigger:            dryRun(epoch),
					Submitted:          false,
					Deps:               []*changelist.Dep{{Clid: 1, Kind: changelist.DepKind_SOFT}, {Clid: 2, Kind: changelist.DepKind_SOFT}},
				},
			}

			Convey("Happy case: all are ready with reverseDeps computed", func() {
				a := triageCLs(&prjpb.Component{Clids: []int64{1, 2, 3}})
				So(sortedRevDeps(a.reverseDeps), ShouldResemble, map[int64][]int64{1: {2, 3}, 2: {1, 3}})
				So(a.cls, ShouldHaveLength, 3)
				for _, info := range a.cls {
					So(info.ready, ShouldBeTrue)
					So(info.deps.OK(), ShouldBeTrue)
				}
			})

			Convey("Full Run on #1 and #2 can co-exist, but Dry run on #3 is purged", func() {
				// This scenario documents current CQDaemon behavior. This isn't desired
				// long term though.
				sup.PCL(1).Trigger = fullRun(epoch)
				sup.PCL(2).Trigger = fullRun(epoch)
				a := triageCLs(&prjpb.Component{Clids: []int64{1, 2, 3}})
				// reverseDeps can lead to ready CLs only.
				So(a.reverseDeps, ShouldResemble, map[int64][]int64{1: {2}, 2: {1}})
				So(a.cls[1].ready, ShouldBeTrue)
				So(a.cls[2].ready, ShouldBeTrue)
				So(a.cls[3], shouldResembleTriagedCL, &clInfo{
					pcl: sup.PCL(3),
					triagedCL: triagedCL{
						ready: false,
						purgeReasons: []*changelist.CLError{
							{
								Kind: &changelist.CLError_InvalidDeps_{
									InvalidDeps: &changelist.CLError_InvalidDeps{
										IncompatMode: sup.PCL(3).GetDeps(),
									},
								},
							},
						},
						deps: &triagedDeps{
							lastTriggered: epoch,
							incompatMode:  sup.PCL(3).GetDeps(),
						},
					},
				})
			})

			Convey("Dependencies in diff config groups are not allowed", func() {
				sup.PCL(1).ConfigGroupIndexes = []int32{combIdx}    // depends on 2
				sup.PCL(2).ConfigGroupIndexes = []int32{anotherIdx} // depends on 1
				sup.PCL(3).ConfigGroupIndexes = []int32{combIdx}    // depends on 1(OK) and 2.
				a := triageCLs(&prjpb.Component{Clids: []int64{1, 2, 3}})
				So(a.reverseDeps, ShouldBeEmpty)
				for _, info := range a.cls {
					So(info.ready, ShouldBeFalse)
					So(info.purgeReasons, ShouldResembleProto, []*changelist.CLError{
						{
							Kind: &changelist.CLError_InvalidDeps_{
								InvalidDeps: &changelist.CLError_InvalidDeps{
									WrongConfigGroup: info.triagedCL.deps.wrongConfigGroup,
								},
							},
						},
					})
				}

				Convey("unless dependency is already submitted", func() {
					sup.PCL(2).Trigger = nil
					sup.PCL(2).Submitted = true

					a := triageCLs(&prjpb.Component{Clids: []int64{1, 3}})
					for _, info := range a.cls {
						So(info.ready, ShouldBeTrue)
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
					Trigger:            dryRun(epoch),
					Deps:               []*changelist.Dep{{Clid: 1, Kind: changelist.DepKind_SOFT}},
				},
			}
			a := triageCLs(&prjpb.Component{Clids: []int64{2}})
			So(a.reverseDeps, ShouldResemble, map[int64][]int64{1: {2}})
			So(a.cls[2], shouldResembleTriagedCL, &clInfo{
				pcl: sup.PCL(2),
				triagedCL: triagedCL{
					ready: true,
					deps:  &triagedDeps{notYetLoaded: sup.PCL(2).GetDeps()},
				},
			})
		})
	})
}

// sorts in place and returns reverseDeps map.
func sortedRevDeps(m map[int64][]int64) map[int64][]int64 {
	for _, vs := range m {
		sort.Slice(vs, func(i, j int) bool { return vs[i] < vs[j] })
	}
	return m
}
