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

func shouldResembleTriagedDeps(actual interface{}, expected ...interface{}) string {
	if len(expected) != 1 {
		return fmt.Sprintf("expected 1 value, got %d", len(expected))
	}
	exp := expected[0] // this may be nil
	a, ok := actual.(*triagedDeps)
	if !ok {
		return fmt.Sprintf("Wrong actual type %T, must be %T", actual, a)
	}
	if err := ShouldHaveSameTypeAs(actual, exp); err != "" {
		return err
	}
	b := exp.(*triagedDeps)
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
		ShouldResemble(a.lastTriggered, b.lastTriggered),
		ShouldResembleProto(a.notYetLoaded, b.notYetLoaded),
		ShouldResembleProto(a.submitted, b.submitted),
		ShouldResembleProto(a.unwatched, b.unwatched),
		ShouldResembleProto(a.wrongConfigGroup, b.wrongConfigGroup),
		ShouldResembleProto(a.incompatMode, b.incompatMode),
	} {
		if err != "" {
			buf.WriteRune(' ')
			buf.WriteString(err)
		}
	}
	return strings.TrimSuffix(buf.String(), " ")
}

func TestShouldResembleTriagedDeps(t *testing.T) {
	t.Parallel()

	Convey("shouldResembleTriagedDeps works", t, func() {
		So(&triagedDeps{}, shouldResembleTriagedDeps, &triagedDeps{})
		So(&triagedDeps{
			wrongConfigGroup: []*changelist.Dep{{Clid: 1}},
			submitted:        []*changelist.Dep{{Clid: 2}},
		}, shouldResembleTriagedDeps, &triagedDeps{
			wrongConfigGroup: []*changelist.Dep{{Clid: 1}},
			submitted:        []*changelist.Dep{{Clid: 2}},
		})
	})
}

func TestDepsTriage(t *testing.T) {
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

		do := func(pcl *prjpb.PCL, cgIdx int32) *triagedDeps {
			backup := prjpb.PState{}
			proto.Merge(&backup, sup.pb)

			// Actual component doesn't matter in this test.
			td := triageDeps(pcl, cgIdx, pmState{sup})
			So(sup.pb, ShouldResembleProto, &backup) // must not be modified
			return td
		}

		Convey("Singluar and Combinable behave the same", func() {
			sameTests := func(name string, cgIdx int32) {
				Convey(name, func() {
					Convey("no deps", func() {
						sup.pb.Pcls = []*prjpb.PCL{
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}},
						}
						td := do(sup.pb.Pcls[0], cgIdx)
						So(td, shouldResembleTriagedDeps, &triagedDeps{})
						So(td.OK(), ShouldBeTrue)
					})

					Convey("Valid CL stack CQ+1", func() {
						sup.pb.Pcls = []*prjpb.PCL{
							{Clid: 31, ConfigGroupIndexes: []int32{cgIdx}, Trigger: dryRun(epoch.Add(3 * time.Second))},
							{Clid: 32, ConfigGroupIndexes: []int32{cgIdx}, Trigger: dryRun(epoch.Add(2 * time.Second))},
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}, Trigger: dryRun(epoch.Add(1 * time.Second)),
								Deps: []*changelist.Dep{
									{Clid: 31, Kind: changelist.DepKind_SOFT},
									{Clid: 32, Kind: changelist.DepKind_HARD},
								}},
						}
						td := do(sup.PCL(33), cgIdx)
						So(td, shouldResembleTriagedDeps, &triagedDeps{
							lastTriggered: epoch.Add(3 * time.Second),
						})
						So(td.OK(), ShouldBeTrue)
					})

					Convey("Not yet loaded deps", func() {
						sup.pb.Pcls = []*prjpb.PCL{
							// 31 isn't in PCLs yet
							{Clid: 32, Status: prjpb.PCL_UNKNOWN},
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}, Trigger: dryRun(epoch.Add(1 * time.Second)),
								Deps: []*changelist.Dep{
									{Clid: 31, Kind: changelist.DepKind_SOFT},
									{Clid: 32, Kind: changelist.DepKind_HARD},
								}},
						}
						pcl33 := sup.PCL(33)
						td := do(pcl33, cgIdx)
						So(td, shouldResembleTriagedDeps, &triagedDeps{notYetLoaded: pcl33.GetDeps()})
						So(td.OK(), ShouldBeTrue)
					})

					Convey("Unwatched", func() {
						sup.pb.Pcls = []*prjpb.PCL{
							{Clid: 31, Status: prjpb.PCL_UNWATCHED},
							{Clid: 32, Status: prjpb.PCL_DELETED},
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}, Trigger: dryRun(epoch.Add(1 * time.Second)),
								Deps: []*changelist.Dep{
									{Clid: 31, Kind: changelist.DepKind_SOFT},
									{Clid: 32, Kind: changelist.DepKind_HARD},
								}},
						}
						pcl33 := sup.PCL(33)
						td := do(pcl33, cgIdx)
						So(td, shouldResembleTriagedDeps, &triagedDeps{unwatched: pcl33.GetDeps()})
						So(td.OK(), ShouldBeFalse)
					})

					Convey("Submitted can be in any config group and they are OK deps", func() {
						sup.pb.Pcls = []*prjpb.PCL{
							{Clid: 32, ConfigGroupIndexes: []int32{anotherIdx}, Submitted: true},
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}, Trigger: dryRun(epoch.Add(1 * time.Second)),
								Deps: []*changelist.Dep{{Clid: 32, Kind: changelist.DepKind_HARD}}},
						}
						pcl33 := sup.PCL(33)
						td := do(pcl33, cgIdx)
						So(td, shouldResembleTriagedDeps, &triagedDeps{submitted: pcl33.GetDeps()})
						So(td.OK(), ShouldBeTrue)
					})

					Convey("Wrong config group", func() {
						sup.pb.Pcls = []*prjpb.PCL{
							{Clid: 31, Trigger: dryRun(epoch.Add(3 * time.Second)), ConfigGroupIndexes: []int32{anotherIdx}},
							{Clid: 32, Trigger: dryRun(epoch.Add(2 * time.Second)), ConfigGroupIndexes: []int32{anotherIdx, cgIdx}},
							{Clid: 33, Trigger: dryRun(epoch.Add(1 * time.Second)), ConfigGroupIndexes: []int32{cgIdx},
								Deps: []*changelist.Dep{
									{Clid: 31, Kind: changelist.DepKind_SOFT},
									{Clid: 32, Kind: changelist.DepKind_HARD},
								}},
						}
						pcl33 := sup.PCL(33)
						td := do(pcl33, cgIdx)
						So(td, shouldResembleTriagedDeps, &triagedDeps{
							lastTriggered:    epoch.Add(3 * time.Second),
							wrongConfigGroup: pcl33.GetDeps(),
						})
						So(td.OK(), ShouldBeFalse)
					})
				})
			}
			sameTests("singular", singIdx)
			sameTests("combinable", combIdx)
		})

		Convey("Singular speciality", func() {
			sup.pb.Pcls = []*prjpb.PCL{
				{
					Clid: 31, ConfigGroupIndexes: []int32{singIdx},
					Trigger: dryRun(epoch.Add(3 * time.Second)),
				},
				{
					Clid: 32, ConfigGroupIndexes: []int32{singIdx},
					Trigger: fullRun(epoch.Add(2 * time.Second)), // not happy about its dep.
					Deps:    []*changelist.Dep{{Clid: 31, Kind: changelist.DepKind_HARD}},
				},
				{
					Clid: 33, ConfigGroupIndexes: []int32{singIdx},
					Trigger: dryRun(epoch.Add(3 * time.Second)), // doesn't care about deps.
					Deps: []*changelist.Dep{
						{Clid: 31, Kind: changelist.DepKind_SOFT},
						{Clid: 32, Kind: changelist.DepKind_HARD},
					},
				},
			}
			Convey("dry run doesn't care about deps' triggers", func() {
				pcl33 := sup.PCL(33)
				td := do(pcl33, singIdx)
				So(td, shouldResembleTriagedDeps, &triagedDeps{
					lastTriggered: epoch.Add(3 * time.Second),
				})
			})
			Convey("quick dry run doesn't care about deps' triggers", func() {
				pcl33 := sup.PCL(33)
				pcl33.Trigger.Mode = string(run.QuickDryRun)
				td := do(pcl33, singIdx)
				So(td, shouldResembleTriagedDeps, &triagedDeps{
					lastTriggered: epoch.Add(3 * time.Second),
				})
			})
			Convey("full run considers any dep incompatible", func() {
				pcl32 := sup.PCL(32)
				td := do(pcl32, singIdx)
				So(td, shouldResembleTriagedDeps, &triagedDeps{
					lastTriggered: epoch.Add(3 * time.Second),
					incompatMode:  pcl32.GetDeps(),
				})
				So(td.OK(), ShouldBeFalse)
			})
		})

		Convey("Combinable speciality", func() {
			// Setup valid deps; sub-tests will mutate this to become invalid.
			sup.pb.Pcls = []*prjpb.PCL{
				{
					Clid: 31, ConfigGroupIndexes: []int32{combIdx},
					Trigger: dryRun(epoch.Add(3 * time.Second)),
				},
				{
					Clid: 32, ConfigGroupIndexes: []int32{combIdx},
					Trigger: dryRun(epoch.Add(2 * time.Second)),
					Deps:    []*changelist.Dep{{Clid: 31, Kind: changelist.DepKind_HARD}},
				},
				{
					Clid: 33, ConfigGroupIndexes: []int32{combIdx},
					Trigger: dryRun(epoch.Add(1 * time.Second)),
					Deps: []*changelist.Dep{
						{Clid: 31, Kind: changelist.DepKind_SOFT},
						{Clid: 32, Kind: changelist.DepKind_HARD},
					},
				},
			}
			Convey("dry run expects all deps to be dry", func() {
				pcl32 := sup.PCL(32)
				Convey("ok", func() {
					td := do(pcl32, combIdx)
					So(td, shouldResembleTriagedDeps, &triagedDeps{lastTriggered: epoch.Add(3 * time.Second)})
				})

				Convey("... not full runs", func() {
					// TODO(tandrii): this can and should be supported.
					sup.PCL(31).Trigger.Mode = string(run.FullRun)
					td := do(pcl32, combIdx)
					So(td, shouldResembleTriagedDeps, &triagedDeps{
						lastTriggered: epoch.Add(3 * time.Second),
						incompatMode:  pcl32.GetDeps(),
					})
				})
			})
			Convey("full run considers any dep incompatible", func() {
				pcl33 := sup.PCL(33)
				Convey("ok", func() {
					for _, pcl := range sup.pb.GetPcls() {
						pcl.Trigger.Mode = string(run.FullRun)
					}
					td := do(pcl33, combIdx)
					So(td, shouldResembleTriagedDeps, &triagedDeps{lastTriggered: epoch.Add(3 * time.Second)})
				})
				Convey("... not dry runs", func() {
					sup.PCL(32).Trigger.Mode = string(run.FullRun)
					td := do(pcl33, combIdx)
					So(td, shouldResembleTriagedDeps, &triagedDeps{
						lastTriggered: epoch.Add(3 * time.Second),
						incompatMode:  []*changelist.Dep{{Clid: 32, Kind: changelist.DepKind_HARD}},
					})
					So(td.OK(), ShouldBeFalse)
				})
			})
		})

		Convey("iterateNotSubmitted works", func() {
			d1 := &changelist.Dep{Clid: 1}
			d2 := &changelist.Dep{Clid: 2}
			d3 := &changelist.Dep{Clid: 3}
			pcl := &prjpb.PCL{}
			td := &triagedDeps{}

			iterate := func() (out []*changelist.Dep) {
				td.iterateNotSubmitted(pcl, func(dep *changelist.Dep) { out = append(out, dep) })
				return
			}

			Convey("no deps", func() {
				So(iterate(), ShouldBeEmpty)
			})
			Convey("only submitted", func() {
				td.submitted = []*changelist.Dep{d3, d1, d2}
				pcl.Deps = []*changelist.Dep{d3, d1, d2} // order must be the same
				So(iterate(), ShouldBeEmpty)
			})
			Convey("some submitted", func() {
				pcl.Deps = []*changelist.Dep{d3, d1, d2}
				td.submitted = []*changelist.Dep{d3}
				So(iterate(), ShouldResembleProto, []*changelist.Dep{d1, d2})
				td.submitted = []*changelist.Dep{d1}
				So(iterate(), ShouldResembleProto, []*changelist.Dep{d3, d2})
				td.submitted = []*changelist.Dep{d2}
				So(iterate(), ShouldResembleProto, []*changelist.Dep{d3, d1})
			})
			Convey("none submitted", func() {
				pcl.Deps = []*changelist.Dep{d3, d1, d2}
				So(iterate(), ShouldResembleProto, []*changelist.Dep{d3, d1, d2})
			})
			Convey("notYetLoaded deps are iterated over, too", func() {
				pcl.Deps = []*changelist.Dep{d3, d1, d2}
				td.notYetLoaded = []*changelist.Dep{d3}
				td.submitted = []*changelist.Dep{d2}
				So(iterate(), ShouldResembleProto, []*changelist.Dep{d3, d1})
			})
			Convey("panic on invalid usage", func() {
				Convey("wrong PCL", func() {
					pcl.Deps = []*changelist.Dep{d3, d1, d2}
					td.submitted = []*changelist.Dep{d1, d2, d3} // wrong order
					So(func() { iterate() }, ShouldPanicLike, fmt.Errorf("(wrong PCL?)"))
				})
				Convey("non-OK triagedDeps", func() {
					td.incompatMode = []*changelist.Dep{d1}
					So(func() { iterate() }, ShouldPanicLike, fmt.Errorf("non-OK triagedDeps"))
				})
			})
		})
	})
}
