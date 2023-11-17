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

package triager

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type testCLInfo clInfo

func (ci *testCLInfo) Deps(deps ...*testCLInfo) *testCLInfo {
	for _, dep := range deps {
		ci.pcl.Deps = append(ci.pcl.Deps, &changelist.Dep{
			Clid: dep.Clid(),
			Kind: changelist.DepKind_HARD,
		})
	}
	return ci
}

func (ci *testCLInfo) SoftDeps(deps ...*testCLInfo) *testCLInfo {
	for _, dep := range deps {
		ci.pcl.Deps = append(ci.pcl.Deps, &changelist.Dep{
			Clid: dep.Clid(),
			Kind: changelist.DepKind_SOFT,
		})
	}
	return ci
}

func (ci *testCLInfo) CQ(val int) *testCLInfo {
	switch val {
	case 0:
		ci.pcl.Triggers = nil
	case 1:
		ci.pcl.Triggers = &run.Triggers{
			CqVoteTrigger: &run.Trigger{
				Mode: string(run.DryRun),
			},
		}
	case 2:
		ci.pcl.Triggers = &run.Triggers{
			CqVoteTrigger: &run.Trigger{
				Mode: string(run.FullRun),
			},
		}
	default:
		panic(fmt.Errorf("unsupported CQ value"))
	}
	return ci
}

func (ci *testCLInfo) triageDeps(cls map[int64]*clInfo) {
	mode := ci.pcl.GetTriggers().GetCqVoteTrigger().GetMode()
	if mode != string(run.FullRun) {
		return
	}
	ci.deps.needToTrigger = ci.deps.needToTrigger[:0]
	for _, dep := range ci.pcl.GetDeps() {
		dci, ok := cls[dep.GetClid()]
		if !ok {
			ci.deps.notYetLoaded = append(ci.deps.notYetLoaded, &changelist.Dep{
				Clid: dep.GetClid(),
				Kind: changelist.DepKind_HARD,
			})
			continue
		}
		depMode := dci.pcl.GetTriggers().GetCqVoteTrigger().GetMode()
		if mode != depMode {
			ci.deps.needToTrigger = append(ci.deps.needToTrigger, &changelist.Dep{
				Clid: dep.GetClid(),
				Kind: changelist.DepKind_HARD,
			})
		}
	}
}

func (ci *testCLInfo) Clid() int64 {
	return ci.pcl.GetClid()
}

func (ci *testCLInfo) NeedToTrigger() []int64 {
	var ret []int64
	if ci.deps == nil {
		return ret
	}
	for _, dep := range ci.deps.needToTrigger {
		ret = append(ret, dep.GetClid())
	}
	return ret
}

func (ci *testCLInfo) SetPurgingCL() *testCLInfo {
	ci.purgingCL = &prjpb.PurgingCL{}
	return ci
}

func (ci *testCLInfo) SetPurgeReasons() *testCLInfo {
	ci.purgeReasons = []*prjpb.PurgeReason{{}}
	return ci
}

func (ci *testCLInfo) SetIncompleteRun(m run.Mode) *testCLInfo {
	ci.runIndexes = []int32{1}
	ci.runCountByMode[m]++
	return ci
}

func (ci *testCLInfo) SetTriggeringCLDeps() *testCLInfo {
	ci.triggeringCLDeps = &prjpb.TriggeringCLDeps{}
	return ci
}

func (ci *testCLInfo) Outdated() *testCLInfo {
	ci.pcl.Outdated = &changelist.Snapshot_Outdated{}
	return ci
}

func TestStageTriggerCLDeps(t *testing.T) {
	t.Parallel()

	Convey("stargeTriggerCLDeps", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		cq2 := &run.Trigger{Mode: string(run.FullRun)}
		cls := make(map[int64]*clInfo)
		nextCLID := int64(1)
		newCL := func() *testCLInfo {
			defer func() { nextCLID++ }()
			tci := &testCLInfo{
				pcl: &prjpb.PCL{
					Clid: nextCLID,
				},
				triagedCL: triagedCL{
					deps: &triagedDeps{},
				},
				runCountByMode: make(map[run.Mode]int),
			}
			cls[nextCLID] = (*clInfo)(tci)
			return tci
		}
		triageDeps := func(cis ...*testCLInfo) {
			for _, ci := range cis {
				ci.triageDeps(cls)
			}
		}

		Convey("CLs without deps", func() {
			cl1 := newCL().CQ(0)
			cl2 := newCL().CQ(+2)
			triageDeps(cl1, cl2)
			So(cl1.NeedToTrigger(), ShouldBeNil)
			So(cl2.NeedToTrigger(), ShouldBeNil)
			So(stageTriggerCLDeps(ctx, cls), ShouldHaveLength, 0)
		})

		Convey("CL with deps", func() {
			cl1 := newCL()
			cl2 := newCL().Deps(cl1)
			cl3 := newCL().Deps(cl1, cl2)

			Convey("no deps have CQ vote", func() {
				cl3 = cl3.CQ(+2)
				triageDeps(cl1, cl2, cl3)
				So(cl3.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid(), cl2.Clid()})
				So(stageTriggerCLDeps(ctx, cls), ShouldResembleProto, []*prjpb.TriggeringCLDeps{
					{
						OriginClid: cl3.Clid(),
						DepClids:   []int64{cl1.Clid(), cl2.Clid()},
						Trigger:    cq2,
					},
				})

				Convey("unless outdated", func() {
					Convey("the origin CL", func() {
						cl3 = cl3.Outdated()
					})
					Convey("a dep CL", func() {
						cl2 = cl2.Outdated()
					})
					triageDeps(cl1, cl2, cl3)
					So(cl3.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid(), cl2.Clid()})
					So(stageTriggerCLDeps(ctx, cls), ShouldBeNil)
				})

				Convey("retriaging it should be noop", func() {
					// Now, cl3 has TriggeringCLDeps, created by the previous
					// stageTriggerCLDeps(), and let's say that cl2 has voted.
					cl2 = cl2.CQ(+2)
					cl3 = cl3.SetTriggeringCLDeps()
					triageDeps(cl1, cl2, cl3)

					// Now, triageDeps declares that both cl2 and cl3 have
					// unvoted deps, but none of them should schedule a new
					// task.
					So(cl2.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid()})
					So(cl3.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid()})
					So(stageTriggerCLDeps(ctx, cls), ShouldBeNil)
				})

				Convey("unless a dep was not loaded yet", func() {
					delete(cls, cl1.pcl.GetClid())
					triageDeps(cl1, cl2, cl3)
					So(stageTriggerCLDeps(ctx, cls), ShouldHaveLength, 0)
				})
			})
			Convey("all deps have CQ vote", func() {
				cl1 = cl1.CQ(+2)
				cl2 = cl2.CQ(+2)
				cl3 = cl3.CQ(+2)
				triageDeps(cl1, cl2, cl3)
				So(cl3.NeedToTrigger(), ShouldBeNil)
				So(stageTriggerCLDeps(ctx, cls), ShouldHaveLength, 0)
			})
			Convey("some deps have and some others don't have CQ votes", func() {
				cl2 = cl2.CQ(+2)
				cl3 = cl3.CQ(+2)
				triageDeps(cl1, cl2, cl3)
				// Both cl2.deps and cl3.deps have cl1 in needToTrigger, but
				// TriggerCLDeps{} should be created for cl3 only.
				So(cl2.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid()})
				So(cl3.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid()})
				So(stageTriggerCLDeps(ctx, cls), ShouldResembleProto, []*prjpb.TriggeringCLDeps{
					{
						OriginClid: cl3.Clid(),
						DepClids:   []int64{cl1.Clid()},
						Trigger:    cq2,
					},
				})
			})
		})

		Convey("with inflight purges", func() {
			cl1 := newCL()
			cl2 := newCL().Deps(cl1)
			cl3 := newCL().Deps(cl1, cl2)

			Convey("PurgingCL on the originating CL", func() {
				cl3 = cl3.CQ(+2).SetPurgingCL()
				triageDeps(cl1, cl2, cl3)
				So(cl2.NeedToTrigger(), ShouldBeNil)
				So(cl3.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid(), cl2.Clid()})
			})
			Convey("PurgingCL on a parent CL", func() {
				cl2 = cl2.CQ(+2).SetPurgingCL()
				cl3 = cl3.CQ(+2)
				triageDeps(cl1, cl2, cl3)
				So(cl2.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid()})
				So(cl3.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid()})
			})
			Convey("purgeReasons on the originating CL", func() {
				cl3 = cl3.CQ(+2).SetPurgeReasons()
				triageDeps(cl1, cl2, cl3)
				So(cl2.NeedToTrigger(), ShouldBeNil)
				So(cl3.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid(), cl2.Clid()})
			})
			Convey("purgeReasons on a parent CL", func() {
				cl2 = cl2.CQ(+2).SetPurgeReasons()
				cl3 = cl3.CQ(+2)
				triageDeps(cl1, cl2, cl3)
				So(cl2.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid()})
				So(cl3.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid()})
			})
			So(stageTriggerCLDeps(ctx, cls), ShouldHaveLength, 0)
		})

		Convey("with inflight TriggeringCLDeps", func() {
			cl1 := newCL()
			cl2 := newCL().Deps(cl1)
			cl3 := newCL().Deps(cl1, cl2)

			Convey("TriggeringCLDeps on the originating CL", func() {
				cl3 = cl3.CQ(+2).SetTriggeringCLDeps()
				triageDeps(cl1, cl2, cl3)
				So(cl2.NeedToTrigger(), ShouldBeNil)
				So(cl3.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid(), cl2.Clid()})
			})
			Convey("TriggeringCLDeps on a parent CL", func() {
				cl2 = cl2.CQ(+2).SetTriggeringCLDeps()
				cl3 = cl3.CQ(+2)
				triageDeps(cl1, cl2, cl3)
				So(cl2.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid()})
				So(cl3.NeedToTrigger(), ShouldEqual, []int64{cl1.Clid()})
			})
			So(stageTriggerCLDeps(ctx, cls), ShouldHaveLength, 0)
		})

		Convey("with incomplete run", func() {
			cl1 := newCL()
			cl2 := newCL().Deps(cl1)
			cl3 := newCL().Deps(cl1, cl2)
			cl4 := newCL().Deps(cl1, cl2, cl3)

			Convey("incomplete run with the same CQ vote in all the CLs", func() {
				cl1 = cl1.CQ(+2).SetIncompleteRun(run.FullRun)
				cl2 = cl2.CQ(+2).SetIncompleteRun(run.FullRun)
				cl3 = cl3.CQ(+2).SetIncompleteRun(run.FullRun)

				triageDeps(cl1, cl2, cl3, cl4)
				So(cl1.NeedToTrigger(), ShouldBeNil)
				So(cl2.NeedToTrigger(), ShouldBeNil)
				So(cl3.NeedToTrigger(), ShouldBeNil)
				So(cl4.NeedToTrigger(), ShouldBeNil)
				So(stageTriggerCLDeps(ctx, cls), ShouldHaveLength, 0)
			})

			Convey("incomplete run with different CQVotes in deps", func() {
				cl1 = cl1.CQ(+0).SetIncompleteRun(run.NewPatchsetRun)
				cl2 = cl2.CQ(+1).SetIncompleteRun(run.DryRun)
				cl3 = cl3.CQ(+2).SetIncompleteRun(run.FullRun)

				triageDeps(cl1, cl2, cl3, cl4)
				So(cl1.NeedToTrigger(), ShouldBeNil)
				So(cl2.NeedToTrigger(), ShouldBeNil)
				So(cl3.NeedToTrigger(), ShouldEqual, []int64{cl1.pcl.GetClid(), cl2.pcl.GetClid()})
				So(cl4.NeedToTrigger(), ShouldBeNil)
				So(stageTriggerCLDeps(ctx, cls), ShouldHaveLength, 0)
			})

			Convey("incomplete run on parent CLs", func() {
				// This happen, where a child CL receives CQ+2, while its
				// parents are running.
				cl1 = cl1.CQ(+2).SetIncompleteRun(run.FullRun)
				cl2 = cl2.CQ(+2).SetIncompleteRun(run.FullRun)
				cl3 = cl3.CQ(+2)
				cl4 = cl3.CQ(0)

				triageDeps(cl1, cl2, cl3, cl4)
				So(cl1.NeedToTrigger(), ShouldBeNil)
				So(cl2.NeedToTrigger(), ShouldBeNil)
				So(cl3.NeedToTrigger(), ShouldBeNil)
				So(cl4.NeedToTrigger(), ShouldBeNil)
				So(stageTriggerCLDeps(ctx, cls), ShouldHaveLength, 0)
			})

			Convey("MCE over MCE", func() {
				// Similar to "incomplete run on parent CLs", but with another
				// CL between.
				cl1 = cl1.CQ(+2).SetIncompleteRun(run.FullRun)
				cl2 = cl2.CQ(+2).SetIncompleteRun(run.FullRun)
				cl3 = cl3.CQ(0)
				cl4 = cl4.CQ(+2)

				triageDeps(cl1, cl2, cl3, cl4)
				So(cl1.NeedToTrigger(), ShouldBeNil)
				So(cl2.NeedToTrigger(), ShouldBeNil)
				So(cl3.NeedToTrigger(), ShouldBeNil)
				So(cl4.NeedToTrigger(), ShouldEqual, []int64{cl3.Clid()})
				So(stageTriggerCLDeps(ctx, cls), ShouldResembleProto, []*prjpb.TriggeringCLDeps{
					{
						OriginClid: cl4.Clid(),
						DepClids:   []int64{cl3.Clid()},
						Trigger:    cq2,
					},
				})
			})

			Convey("MCE over MCE with a mix of incomplete and complete runs", func() {
				// Similar to "incomplete run on parent CLs", but with another
				// CL between.
				cl1 = cl1.CQ(+2)
				cl2 = cl2.CQ(+2).SetIncompleteRun(run.FullRun)
				cl3 = cl3.CQ(0)
				cl4 = cl4.CQ(+2)

				triageDeps(cl1, cl2, cl3, cl4)
				So(cl1.NeedToTrigger(), ShouldBeNil)
				So(cl2.NeedToTrigger(), ShouldBeNil)
				So(cl3.NeedToTrigger(), ShouldBeNil)
				So(cl4.NeedToTrigger(), ShouldEqual, []int64{cl3.Clid()})
				So(stageTriggerCLDeps(ctx, cls), ShouldResembleProto, []*prjpb.TriggeringCLDeps{
					{
						OriginClid: cl4.Clid(),
						DepClids:   []int64{cl3.Clid()},
						Trigger:    cq2,
					},
				})
			})
		})
	})
}
