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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTriage(t *testing.T) {
	t.Parallel()

	Convey("Triage works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		// Truncate start time point s.t. easy to see diff in test failures.
		ct.RoundTestClock(10000 * time.Second)

		const gHost = "g-review.example.com"
		const lProject = "v8"

		const stabilizationDelay = 5 * time.Minute
		const singIdx, combIdx, anotherIdx = 0, 1, 2
		cfg := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{Name: "singular"},
				{Name: "combinable", CombineCls: &cfgpb.CombineCLs{
					StabilizationDelay: durationpb.New(stabilizationDelay),
				}},
				{Name: "another"},
			},
		}
		prjcfgtest.Create(ctx, lProject, cfg)
		pm := &simplePMState{pb: &prjpb.PState{}}
		var err error
		pm.cgs, err = prjcfgtest.MustExist(ctx, lProject).GetConfigGroups(ctx)
		So(err, ShouldBeNil)

		dryRun := func(t time.Time) *run.Triggers {
			return &run.Triggers{CqVoteTrigger: &run.Trigger{Mode: string(run.DryRun), Time: timestamppb.New(t)}}
		}

		triage := func(c *prjpb.Component) (itriager.Result, error) {
			backup := prjpb.PState{}
			proto.Merge(&backup, pm.pb)
			res, err := Triage(ctx, c, pm)
			// Regardless of result, PM's state must be not be modified.
			So(pm.pb, ShouldResembleProto, &backup)
			return res, err
		}
		mustTriage := func(c *prjpb.Component) itriager.Result {
			res, err := triage(c)
			So(err, ShouldBeNil)
			return res
		}
		failTriage := func(c *prjpb.Component) error {
			_, err := triage(c)
			So(err, ShouldNotBeNil)
			return err
		}

		markTriaged := func(c *prjpb.Component) *prjpb.Component {
			c = c.CloneShallow()
			c.TriageRequired = false
			return c
		}
		putPCLBoth := func(clid int, grpIndex int32, mode run.Mode, triggerTime time.Time, depsCLIDs ...int) (*changelist.CL, *prjpb.PCL) {
			mods := []gf.CIModifier{gf.PS(1), gf.Updated(triggerTime)}
			u := gf.U("user-1")
			switch mode {
			case run.FullRun:
				mods = append(mods, gf.CQ(+2, triggerTime, u))
			case run.DryRun:
				mods = append(mods, gf.CQ(+1, triggerTime, u))
			default:
				panic(fmt.Errorf("unsupported %s", mode))
			}
			ci := gf.CI(clid, mods...)
			trs := trigger.Find(&trigger.FindInput{ChangeInfo: ci, ConfigGroup: nil})
			So(trs.GetCqVoteTrigger(), ShouldNotBeNil)
			So(trs.GetCqVoteTrigger().GetMode(), ShouldResemble, string(mode))
			cl := &changelist.CL{
				ID:       common.CLID(clid),
				EVersion: 1,
				Snapshot: &changelist.Snapshot{Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
					Host: gHost,
					Info: ci,
				}}},
			}
			for _, d := range depsCLIDs {
				cl.Snapshot.Deps = append(cl.Snapshot.Deps, &changelist.Dep{
					Clid: int64(d),
					Kind: changelist.DepKind_SOFT,
				})
			}
			So(datastore.Put(ctx, cl), ShouldBeNil)
			pclTriggers := proto.Clone(trs).(*run.Triggers)
			pclTriggers.CqVoteTrigger.Email = ""
			pclTriggers.CqVoteTrigger.GerritAccountId = 0
			return cl, &prjpb.PCL{
				Clid:               int64(clid),
				Eversion:           1,
				Status:             prjpb.PCL_OK,
				ConfigGroupIndexes: []int32{grpIndex},
				Triggers:           pclTriggers,
				Trigger:            pclTriggers.GetCqVoteTrigger(),
				Deps:               cl.Snapshot.GetDeps(),
			}
		}
		putPCLOld := func(clid int, grpIndex int32, mode run.Mode, triggerTime time.Time, depsCLIDs ...int) (*changelist.CL, *prjpb.PCL) {
			cl, pcl := putPCLBoth(clid, grpIndex, mode, triggerTime, depsCLIDs...)
			pcl.Triggers = nil
			return cl, pcl
		}
		putPCLNew := func(clid int, grpIndex int32, mode run.Mode, triggerTime time.Time, depsCLIDs ...int) (*changelist.CL, *prjpb.PCL) {
			cl, pcl := putPCLBoth(clid, grpIndex, mode, triggerTime, depsCLIDs...)
			pcl.Trigger = nil
			return cl, pcl
		}
		resetTriggers := func(pcl *prjpb.PCL) {
			pcl.Trigger = nil
			pcl.Triggers = nil
		}

		Convey("Noops", func() {
			pcl := &prjpb.PCL{Clid: 33, ConfigGroupIndexes: []int32{singIdx}, Triggers: dryRun(ct.Clock.Now())}
			pm.pb.Pcls = []*prjpb.PCL{pcl}
			oldC := &prjpb.Component{
				Clids: []int64{33},
				// Component already has a Run, so no action required.
				Pruns:          []*prjpb.PRun{{Id: "id", Clids: []int64{33}}},
				TriageRequired: true,
			}
			res := mustTriage(oldC)
			So(res.NewValue, ShouldResembleProto, markTriaged(oldC))
			So(res.RunsToCreate, ShouldBeEmpty)
			So(res.CLsToPurge, ShouldBeEmpty)
		})

		Convey("Purges CLs", func() {
			pm.pb.Pcls = []*prjpb.PCL{
				{
					Clid:               33,
					ConfigGroupIndexes: nil, // modified below.
					Triggers:           dryRun(ct.Clock.Now()),
					Errors: []*changelist.CLError{ // => must purge.
						{Kind: &changelist.CLError_OwnerLacksEmail{OwnerLacksEmail: true}},
					},
				},
			}
			oldC := &prjpb.Component{Clids: []int64{33}}

			Convey("singular group -- no delay", func() {
				pm.pb.Pcls[0].ConfigGroupIndexes = []int32{singIdx}
				res := mustTriage(oldC)
				So(res.NewValue, ShouldResembleProto, markTriaged(oldC))
				So(res.CLsToPurge, ShouldHaveLength, 1)
				So(res.RunsToCreate, ShouldBeEmpty)
			})
			Convey("combinable group -- obey stabilization_delay", func() {
				pm.pb.Pcls[0].ConfigGroupIndexes = []int32{combIdx}

				res := mustTriage(oldC)
				c := markTriaged(oldC)
				c.DecisionTime = timestamppb.New(ct.Clock.Now().Add(stabilizationDelay))
				So(res.NewValue, ShouldResembleProto, c)
				So(res.CLsToPurge, ShouldBeEmpty)
				So(res.RunsToCreate, ShouldBeEmpty)

				ct.Clock.Add(stabilizationDelay * 2)
				res = mustTriage(oldC)
				c.DecisionTime = nil
				So(res.NewValue, ShouldResembleProto, c)
				So(res.CLsToPurge, ShouldHaveLength, 1)
				So(res.RunsToCreate, ShouldBeEmpty)
			})
			Convey("many groups -- no delay", func() {
				// many groups is an error itself
				pm.pb.Pcls[0].ConfigGroupIndexes = []int32{singIdx, combIdx, anotherIdx}
				res := mustTriage(oldC)
				So(res.NewValue, ShouldResembleProto, markTriaged(oldC))
				So(res.CLsToPurge, ShouldHaveLength, 1)
				So(res.RunsToCreate, ShouldBeEmpty)
			})

			Convey("with already existing Run:", func() {
				Convey("with pre-transition pcls", func() {
					_, pcl32 := putPCLOld(32, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Second))
					_, pcl33 := putPCLOld(33, combIdx, run.FullRun, ct.Clock.Now(), 32)

					pm.pb.Pcls = []*prjpb.PCL{pcl32, pcl33}
					oldC := &prjpb.Component{Clids: []int64{32, 33}, TriageRequired: true}
					ct.Clock.Add(stabilizationDelay)
					const expectedRunID = "v8/9042327596854-1-690d9e2cc74b34aa"

					Convey("wait a bit if Run is RUNNING", func() {
						So(datastore.Put(ctx, &run.Run{ID: expectedRunID, Status: run.Status_RUNNING}), ShouldBeNil)
						res := mustTriage(oldC)
						So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
						So(res.NewValue.GetDecisionTime().AsTime(), ShouldResemble, ct.Clock.Now().Add(5*time.Second).UTC())
						So(res.RunsToCreate, ShouldBeEmpty)
						So(res.CLsToPurge, ShouldBeEmpty)
					})

					r := &run.Run{
						ID:      expectedRunID,
						Status:  run.Status_CANCELLED,
						EndTime: datastore.RoundTime(ct.Clock.Now().UTC()),
					}
					So(datastore.Put(ctx, r), ShouldBeNil)

					Convey("wait a bit if Run was just finalized", func() {
						ct.Clock.Add(5 * time.Second)
						res := mustTriage(oldC)
						So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
						So(res.NewValue.GetDecisionTime().AsTime(), ShouldResemble, r.EndTime.Add(time.Minute).UTC())
						So(res.RunsToCreate, ShouldBeEmpty)
						So(res.CLsToPurge, ShouldBeEmpty)
					})

					Convey("purge CLs due to trigger re-use after long-ago finished Run", func() {
						ct.Clock.Add(2 * time.Minute)
						res := mustTriage(oldC)
						So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
						So(res.NewValue.GetDecisionTime(), ShouldBeNil)
						So(res.RunsToCreate, ShouldBeEmpty)
						So(res.CLsToPurge, ShouldHaveLength, 2)
						for _, p := range res.CLsToPurge {
							So(p.GetReasons(), ShouldHaveLength, 1)
							So(p.GetReasons()[0].GetReusedTrigger().GetRun(), ShouldResemble, expectedRunID)
						}
					})
				})
				Convey("with post-transition PCLs ", func() {
					_, pcl32 := putPCLNew(32, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Second))
					_, pcl33 := putPCLNew(33, combIdx, run.FullRun, ct.Clock.Now(), 32)

					pm.pb.Pcls = []*prjpb.PCL{pcl32, pcl33}
					oldC := &prjpb.Component{Clids: []int64{32, 33}, TriageRequired: true}
					ct.Clock.Add(stabilizationDelay)
					const expectedRunID = "v8/9042327596854-1-690d9e2cc74b34aa"

					Convey("wait a bit if Run is RUNNING", func() {
						So(datastore.Put(ctx, &run.Run{ID: expectedRunID, Status: run.Status_RUNNING}), ShouldBeNil)
						res := mustTriage(oldC)
						So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
						So(res.NewValue.GetDecisionTime().AsTime(), ShouldResemble, ct.Clock.Now().Add(5*time.Second).UTC())
						So(res.RunsToCreate, ShouldBeEmpty)
						So(res.CLsToPurge, ShouldBeEmpty)
					})

					r := &run.Run{
						ID:      expectedRunID,
						Status:  run.Status_CANCELLED,
						EndTime: datastore.RoundTime(ct.Clock.Now().UTC()),
					}
					So(datastore.Put(ctx, r), ShouldBeNil)

					Convey("wait a bit if Run was just finalized", func() {
						ct.Clock.Add(5 * time.Second)
						res := mustTriage(oldC)
						So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
						So(res.NewValue.GetDecisionTime().AsTime(), ShouldResemble, r.EndTime.Add(time.Minute).UTC())
						So(res.RunsToCreate, ShouldBeEmpty)
						So(res.CLsToPurge, ShouldBeEmpty)
					})

					Convey("purge CLs due to trigger re-use after long-ago finished Run", func() {
						ct.Clock.Add(2 * time.Minute)
						res := mustTriage(oldC)
						So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
						So(res.NewValue.GetDecisionTime(), ShouldBeNil)
						So(res.RunsToCreate, ShouldBeEmpty)
						So(res.CLsToPurge, ShouldHaveLength, 2)
						for _, p := range res.CLsToPurge {
							So(p.GetReasons(), ShouldHaveLength, 1)
							So(p.GetReasons()[0].GetReusedTrigger().GetRun(), ShouldResemble, expectedRunID)
						}
					})
				})
			})
		})

		Convey("Creates Runs", func() {
			Convey("Singular", func() {
				Convey("OK", func() {
					var pcl *prjpb.PCL
					Convey("New", func() {
						_, pcl = putPCLNew(33, singIdx, run.DryRun, ct.Clock.Now())
					})
					Convey("Old", func() {
						_, pcl = putPCLOld(33, singIdx, run.DryRun, ct.Clock.Now())
					})
					pm.pb.Pcls = []*prjpb.PCL{pcl}
					oldC := &prjpb.Component{Clids: []int64{33}, TriageRequired: true}
					res := mustTriage(oldC)
					So(res.NewValue, ShouldResembleProto, markTriaged(oldC))
					So(res.CLsToPurge, ShouldBeEmpty)
					So(res.RunsToCreate, ShouldHaveLength, 1)
					rc := res.RunsToCreate[0]
					So(rc.ConfigGroupID.Name(), ShouldResemble, "singular")
					So(rc.Mode, ShouldResemble, run.DryRun)
					So(rc.InputCLs, ShouldHaveLength, 1)
					So(rc.InputCLs[0].ID, ShouldEqual, 33)
				})

				Convey("Noop when Run already exists", func() {
					var pcl *prjpb.PCL
					Convey("New", func() {
						_, pcl = putPCLNew(33, singIdx, run.DryRun, ct.Clock.Now())
					})
					Convey("Old", func() {
						_, pcl = putPCLOld(33, singIdx, run.DryRun, ct.Clock.Now())
					})
					pm.pb.Pcls = []*prjpb.PCL{pcl}
					oldC := &prjpb.Component{Clids: []int64{33}, TriageRequired: true, Pruns: makePruns("run-id", 33)}
					res := mustTriage(oldC)
					So(res.NewValue, ShouldResembleProto, markTriaged(oldC))
					So(res.CLsToPurge, ShouldBeEmpty)
					So(res.RunsToCreate, ShouldBeEmpty)
				})

				Convey("EVersion mismatch is an ErrOutdatedPMState", func() {
					var pcl *prjpb.PCL
					var cl *changelist.CL
					Convey("New", func() {
						cl, pcl = putPCLNew(33, singIdx, run.DryRun, ct.Clock.Now())
					})
					Convey("Old", func() {
						cl, pcl = putPCLOld(33, singIdx, run.DryRun, ct.Clock.Now())
					})
					cl.EVersion = 2
					So(datastore.Put(ctx, cl), ShouldBeNil)
					pm.pb.Pcls = []*prjpb.PCL{pcl}
					err := failTriage(&prjpb.Component{Clids: []int64{33}, TriageRequired: true})
					So(itriager.IsErrOutdatedPMState(err), ShouldBeTrue)
					So(err, ShouldErrLike, "EVersion changed 1 => 2")
				})

				Convey("OK with resolved deps", func() {
					var pcl32, pcl33 *prjpb.PCL
					Convey("New", func() {
						_, pcl32 = putPCLNew(32, singIdx, run.FullRun, ct.Clock.Now())
						_, pcl33 = putPCLNew(33, singIdx, run.DryRun, ct.Clock.Now(), 32)
					})
					Convey("Old", func() {
						_, pcl32 = putPCLOld(32, singIdx, run.FullRun, ct.Clock.Now())
						_, pcl33 = putPCLOld(33, singIdx, run.DryRun, ct.Clock.Now(), 32)
					})
					pm.pb.Pcls = []*prjpb.PCL{pcl32, pcl33}
					oldC := &prjpb.Component{Clids: []int64{32, 33}, TriageRequired: true}
					res := mustTriage(oldC)
					So(res.NewValue, ShouldResembleProto, markTriaged(oldC))
					So(res.CLsToPurge, ShouldBeEmpty)
					So(res.RunsToCreate, ShouldHaveLength, 2)
					sortRunsToCreateByFirstCL(&res)
					So(res.RunsToCreate[0].InputCLs[0].ID, ShouldEqual, 32)
					So(res.RunsToCreate[0].Mode, ShouldResemble, run.FullRun)
					So(res.RunsToCreate[1].InputCLs[0].ID, ShouldEqual, 33)
					So(res.RunsToCreate[1].Mode, ShouldResemble, run.DryRun)
				})

				Convey("OK with existing Runs but on different CLs", func() {
					var pcl31, pcl32, pcl33 *prjpb.PCL
					Convey("New", func() {
						_, pcl31 = putPCLNew(31, singIdx, run.FullRun, ct.Clock.Now())
						_, pcl32 = putPCLNew(32, singIdx, run.DryRun, ct.Clock.Now(), 31)
						_, pcl33 = putPCLNew(33, singIdx, run.DryRun, ct.Clock.Now(), 32)
					})
					Convey("Old", func() {
						_, pcl31 = putPCLOld(31, singIdx, run.FullRun, ct.Clock.Now())
						_, pcl32 = putPCLOld(32, singIdx, run.DryRun, ct.Clock.Now(), 31)
						_, pcl33 = putPCLOld(33, singIdx, run.DryRun, ct.Clock.Now(), 32)
					})
					pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl32, pcl33}
					oldC := &prjpb.Component{
						Clids:          []int64{31, 32, 33},
						Pruns:          makePruns("first", 31, "third", 33),
						TriageRequired: true,
					}
					res := mustTriage(oldC)
					So(res.NewValue, ShouldResembleProto, markTriaged(oldC))
					So(res.CLsToPurge, ShouldBeEmpty)
					So(res.RunsToCreate, ShouldHaveLength, 1)
					So(res.RunsToCreate[0].InputCLs[0].ID, ShouldEqual, 32)
					So(res.RunsToCreate[0].Mode, ShouldResemble, run.DryRun)
				})

				Convey("Waits for unresolved dep without an error", func() {
					pcl32 := &prjpb.PCL{Clid: 32, Eversion: 1, Status: prjpb.PCL_UNKNOWN}
					var pcl33 *prjpb.PCL
					Convey("New", func() {
						_, pcl33 = putPCLNew(33, singIdx, run.DryRun, ct.Clock.Now(), 32)
					})
					Convey("Old", func() {
						_, pcl33 = putPCLOld(33, singIdx, run.DryRun, ct.Clock.Now(), 32)
					})
					pm.pb.Pcls = []*prjpb.PCL{pcl32, pcl33}
					oldC := &prjpb.Component{Clids: []int64{33}, TriageRequired: true}
					res := mustTriage(oldC)
					So(res.NewValue, ShouldResembleProto, markTriaged(oldC))
					// TODO(crbug/1211576): this waiting can last forever. Component needs
					// to record how long it has been waiting and abort with clear message
					// to the user.
					So(res.NewValue.GetDecisionTime(), ShouldBeNil) // wait for external event of loading a dep
					So(res.CLsToPurge, ShouldBeEmpty)
					So(res.RunsToCreate, ShouldBeEmpty)
				})
			})

			Convey("Combinable", func() {
				Convey("OK after obeying stabilization delay", func() {
					// Simulate a CL stack <base> -> 31 -> 32 -> 33, which user wants
					// to land at the same time by making 31 depend on 33.
					var pcl31, pcl32, pcl33 *prjpb.PCL
					Convey("New", func() {
						_, pcl31 = putPCLNew(31, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Minute), 33)
						_, pcl32 = putPCLNew(32, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Second), 31)
						_, pcl33 = putPCLNew(33, combIdx, run.FullRun, ct.Clock.Now(), 32, 31)
					})
					Convey("Old", func() {
						_, pcl31 = putPCLOld(31, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Minute), 33)
						_, pcl32 = putPCLOld(32, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Second), 31)
						_, pcl33 = putPCLOld(33, combIdx, run.FullRun, ct.Clock.Now(), 32, 31)
					})
					pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl32, pcl33}
					oldC := &prjpb.Component{Clids: []int64{31, 32, 33}, TriageRequired: true}
					res := mustTriage(oldC)
					So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
					So(res.NewValue.GetDecisionTime().AsTime(), ShouldResemble, ct.Clock.Now().Add(stabilizationDelay).UTC())
					So(res.CLsToPurge, ShouldBeEmpty)
					So(res.RunsToCreate, ShouldBeEmpty)

					ct.Clock.Add(stabilizationDelay)

					oldC = res.NewValue
					res = mustTriage(oldC)
					So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
					So(res.NewValue.GetDecisionTime(), ShouldBeNil)
					rc := res.RunsToCreate[0]
					So(rc.ConfigGroupID.Name(), ShouldResemble, "combinable")
					So(rc.Mode, ShouldResemble, run.FullRun)
					t := pcl33.GetTriggers().GetCqVoteTrigger()
					if t == nil {
						t = pcl33.GetTrigger()
					}
					So(rc.CreateTime, ShouldEqual, t.GetTime().AsTime())
					So(rc.InputCLs, ShouldHaveLength, 3)
				})

				Convey("Even a single CL should wait for stabilization delay", func() {
					var pcl *prjpb.PCL
					Convey("New", func() {
						_, pcl = putPCLNew(33, combIdx, run.FullRun, ct.Clock.Now())
					})
					Convey("Old", func() {
						_, pcl = putPCLOld(33, combIdx, run.FullRun, ct.Clock.Now())
					})
					pm.pb.Pcls = []*prjpb.PCL{pcl}
					oldC := &prjpb.Component{Clids: []int64{33}, TriageRequired: true}
					res := mustTriage(oldC)
					So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
					So(res.NewValue.GetDecisionTime().AsTime(), ShouldResemble, ct.Clock.Now().Add(stabilizationDelay).UTC())
					So(res.CLsToPurge, ShouldBeEmpty)
					So(res.RunsToCreate, ShouldBeEmpty)

					ct.Clock.Add(stabilizationDelay)

					oldC = res.NewValue
					res = mustTriage(oldC)
					So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
					So(res.NewValue.GetDecisionTime(), ShouldBeNil)
					rc := res.RunsToCreate[0]
					So(rc.ConfigGroupID.Name(), ShouldResemble, "combinable")
					So(rc.Mode, ShouldResemble, run.FullRun)
					So(rc.InputCLs, ShouldHaveLength, 1)
					So(rc.InputCLs[0].ID, ShouldEqual, 33)
				})

				Convey("Waits for unresolved dep, even after stabilization delay", func() {
					pcl32 := &prjpb.PCL{Clid: 32, Eversion: 1, Status: prjpb.PCL_UNKNOWN}
					var pcl33 *prjpb.PCL
					Convey("New", func() {
						_, pcl33 = putPCLNew(33, combIdx, run.FullRun, ct.Clock.Now(), 32)
					})
					Convey("Old", func() {
						_, pcl33 = putPCLOld(33, combIdx, run.FullRun, ct.Clock.Now(), 32)
					})
					pm.pb.Pcls = []*prjpb.PCL{pcl32, pcl33}
					oldC := &prjpb.Component{Clids: []int64{33}, TriageRequired: true}
					res := mustTriage(oldC)
					So(res.NewValue.GetDecisionTime().AsTime(), ShouldResemble, ct.Clock.Now().Add(stabilizationDelay).UTC())
					So(res.RunsToCreate, ShouldBeEmpty)

					ct.Clock.Add(stabilizationDelay)

					oldC = res.NewValue
					res = mustTriage(oldC)
					So(res.NewValue.GetDecisionTime(), ShouldBeNil) // wait for external event of loading a dep
					// TODO(crbug/1211576): this waiting can last forever. Component needs
					// to record how long it has been waiting and abort with clear message
					// to the user.
					So(res.CLsToPurge, ShouldBeEmpty)
					So(res.RunsToCreate, ShouldBeEmpty)
				})

				Convey("Noop if there is existing Run encompassing all the CLs", func() {

					Convey("with pre-transition pcls", func() {
						_, pcl31 := putPCLOld(31, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Hour), 32)
						_, pcl32 := putPCLOld(32, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Hour), 31)

						pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl32}
						oldC := &prjpb.Component{Clids: []int64{31, 32}, TriageRequired: true, Pruns: makePruns("runID", 31, 32)}
						res := mustTriage(oldC)
						So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
						So(res.CLsToPurge, ShouldBeEmpty)
						So(res.RunsToCreate, ShouldBeEmpty)

						Convey("even if some CLs are no longer triggered", func() {
							// Happens during Run abort due to, say, tryjob failure.
							resetTriggers(pcl31)
							So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
							So(res.CLsToPurge, ShouldBeEmpty)
							So(res.RunsToCreate, ShouldBeEmpty)
						})

						Convey("even if some CLs are already submitted", func() {
							// Happens during Run submission.
							resetTriggers(pcl32)
							pcl32.Submitted = true
							So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
							So(res.CLsToPurge, ShouldBeEmpty)
							So(res.RunsToCreate, ShouldBeEmpty)
						})
					})
					Convey("with post-transition pcls", func() {
						_, pcl31 := putPCLNew(31, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Hour), 32)
						_, pcl32 := putPCLNew(32, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Hour), 31)

						pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl32}
						oldC := &prjpb.Component{Clids: []int64{31, 32}, TriageRequired: true, Pruns: makePruns("runID", 31, 32)}
						res := mustTriage(oldC)
						So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
						So(res.CLsToPurge, ShouldBeEmpty)
						So(res.RunsToCreate, ShouldBeEmpty)

						Convey("even if some CLs are no longer triggered", func() {
							// Happens during Run abort due to, say, tryjob failure.
							resetTriggers(pcl31)
							So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
							So(res.CLsToPurge, ShouldBeEmpty)
							So(res.RunsToCreate, ShouldBeEmpty)
						})

						Convey("even if some CLs are already submitted", func() {
							// Happens during Run submission.
							resetTriggers(pcl32)
							pcl32.Submitted = true
							So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
							So(res.CLsToPurge, ShouldBeEmpty)
							So(res.RunsToCreate, ShouldBeEmpty)
						})
					})
				})

				Convey("Component growing to N+1 CLs that could form a Run while Run on N CLs is already running", func() {
					// Simulate scenario of user first uploading 31<-41 and CQing two
					// CLs, then much later uploading 51 depending on 31 and CQing 51
					// while (31,41) Run is still running.
					//
					// This may change once postponeExpandingExistingRunScope() function is
					// implemented, but for now test documents that CV will just wait
					// until (31, 41) Run completes.
					var pcl31, pcl41, pcl51 *prjpb.PCL
					Convey("New", func() {
						_, pcl31 = putPCLNew(31, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Hour))
						_, pcl41 = putPCLNew(41, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Hour), 31)
						_, pcl51 = putPCLNew(51, combIdx, run.FullRun, ct.Clock.Now(), 31)
					})
					Convey("Old", func() {
						_, pcl31 = putPCLOld(31, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Hour))
						_, pcl41 = putPCLOld(41, combIdx, run.FullRun, ct.Clock.Now().Add(-time.Hour), 31)
						_, pcl51 = putPCLOld(51, combIdx, run.FullRun, ct.Clock.Now(), 31)
					})
					ct.Clock.Add(2 * stabilizationDelay)
					pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl41, pcl51}
					oldC := &prjpb.Component{Clids: []int64{31, 41, 51}, TriageRequired: true, Pruns: makePruns("41-31", 31, 41)}
					res := mustTriage(oldC)
					So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
					So(res.NewValue.GetDecisionTime(), ShouldBeNil)
					So(res.CLsToPurge, ShouldBeEmpty)
					So(res.RunsToCreate, ShouldBeEmpty)
				})

				Convey("Doesn't react to updates that must be handled first by the Run Manager", func() {
					Convey("with pre-transition pcls", func() {
						_, pcl31 := putPCLOld(31, combIdx, run.DryRun, ct.Clock.Now().Add(-time.Hour), 31)
						_, pcl41 := putPCLOld(41, combIdx, run.DryRun, ct.Clock.Now().Add(-time.Hour), 31)
						_, pcl51 := putPCLOld(51, combIdx, run.DryRun, ct.Clock.Now().Add(-time.Hour), 31, 41)

						pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl41, pcl51}
						oldC := &prjpb.Component{
							Clids:          []int64{31, 41, 51},
							TriageRequired: true,
							Pruns:          makePruns("sub/mitting", 31, 41, 51),
						}
						mustWaitForRM := func() {
							res := mustTriage(oldC)
							So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
							So(res.NewValue.GetDecisionTime(), ShouldBeNil)
							So(res.CLsToPurge, ShouldBeEmpty)
							So(res.RunsToCreate, ShouldBeEmpty)
						}

						Convey("multi-CL Run is being submitted", func() {
							pcl31.Submitted = true
							resetTriggers(pcl31)
							mustWaitForRM()
						})
						Convey("multi-CL Run is being canceled", func() {
							resetTriggers(pcl31)
							mustWaitForRM()
						})
						Convey("multi-CL Run is no longer in the same ConfigGroup", func() {
							pcl31.ConfigGroupIndexes = []int32{anotherIdx}
							mustWaitForRM()
						})
						Convey("multi-CL Run is no longer in the same LUCI project", func() {
							pcl31.Status = prjpb.PCL_UNWATCHED
							pcl31.ConfigGroupIndexes = nil
							resetTriggers(pcl31)
							mustWaitForRM()
						})
					})
					Convey("with post-transition pcls", func() {
						_, pcl31 := putPCLNew(31, combIdx, run.DryRun, ct.Clock.Now().Add(-time.Hour), 31)
						_, pcl41 := putPCLNew(41, combIdx, run.DryRun, ct.Clock.Now().Add(-time.Hour), 31)
						_, pcl51 := putPCLNew(51, combIdx, run.DryRun, ct.Clock.Now().Add(-time.Hour), 31, 41)

						pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl41, pcl51}
						oldC := &prjpb.Component{
							Clids:          []int64{31, 41, 51},
							TriageRequired: true,
							Pruns:          makePruns("sub/mitting", 31, 41, 51),
						}
						mustWaitForRM := func() {
							res := mustTriage(oldC)
							So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
							So(res.NewValue.GetDecisionTime(), ShouldBeNil)
							So(res.CLsToPurge, ShouldBeEmpty)
							So(res.RunsToCreate, ShouldBeEmpty)
						}

						Convey("multi-CL Run is being submitted", func() {
							pcl31.Submitted = true
							resetTriggers(pcl31)
							mustWaitForRM()
						})
						Convey("multi-CL Run is being canceled", func() {
							resetTriggers(pcl31)
							mustWaitForRM()
						})
						Convey("multi-CL Run is no longer in the same ConfigGroup", func() {
							pcl31.ConfigGroupIndexes = []int32{anotherIdx}
							mustWaitForRM()
						})
						Convey("multi-CL Run is no longer in the same LUCI project", func() {
							pcl31.Status = prjpb.PCL_UNWATCHED
							pcl31.ConfigGroupIndexes = nil
							resetTriggers(pcl31)
							mustWaitForRM()
						})
					})
				})

				Convey("Handles races between CL purging and Gerrit -> CLUpdater -> PM state propagation", func() {
					// Due to delays / races between purging a CL and PM state,
					// it's possible that CL Purger hasn't yet responded with purge end
					// result yet PM's view of CL state has changed to look valid, and
					// ready to trigger a Run.
					// Then, PM must wait for the purge to complete.
					var pcl31, pcl32, pcl33 *prjpb.PCL
					Convey("New", func() {
						_, pcl31 = putPCLNew(31, combIdx, run.DryRun, ct.Clock.Now())
						_, pcl32 = putPCLNew(32, combIdx, run.DryRun, ct.Clock.Now(), 31)
						_, pcl33 = putPCLNew(33, combIdx, run.DryRun, ct.Clock.Now(), 31)
					})
					Convey("Old", func() {
						_, pcl31 = putPCLOld(31, combIdx, run.DryRun, ct.Clock.Now())
						_, pcl32 = putPCLOld(32, combIdx, run.DryRun, ct.Clock.Now(), 31)
						_, pcl33 = putPCLOld(33, combIdx, run.DryRun, ct.Clock.Now(), 31)
					})
					ct.Clock.Add(2 * stabilizationDelay)
					pm.pb.Pcls = []*prjpb.PCL{pcl31, pcl32, pcl33}
					pm.pb.PurgingCls = []*prjpb.PurgingCL{
						{Clid: 31, OperationId: "purge-op-31", Deadline: timestamppb.New(ct.Clock.Now().Add(time.Minute))},
					}
					oldC := &prjpb.Component{Clids: []int64{31, 32, 33}, TriageRequired: true}
					res := mustTriage(oldC)
					So(res.NewValue.GetTriageRequired(), ShouldBeFalse)
					So(res.NewValue.GetDecisionTime(), ShouldBeNil)
					So(res.CLsToPurge, ShouldBeEmpty)
					So(res.RunsToCreate, ShouldBeEmpty)
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
func makePruns(runIDthenCLIDs ...interface{}) []*prjpb.PRun {
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
			cur = &prjpb.PRun{Id: string(v)}
		case int64:
			if cur == nil {
				panic("CLIDs must follow a string RunID")
			}
			cur.Clids = append(cur.Clids, v)
		}
	}
	panic("not reachable")
}
