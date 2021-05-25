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
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager/impl/state/itriager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/retry/transient"
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
		ct.Cfg.Create(ctx, lProject, cfg)
		pm := &simplePMState{pb: &prjpb.PState{}}
		var err error
		pm.cgs, err = ct.Cfg.MustExist(ctx, lProject).GetConfigGroups(ctx)
		So(err, ShouldBeNil)

		dryRun := func(t time.Time) *run.Trigger {
			return &run.Trigger{Mode: string(run.DryRun), Time: timestamppb.New(t)}
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

		undirty := func(c *prjpb.Component) *prjpb.Component {
			c = c.CloneShallow()
			c.Dirty = false
			return c
		}

		Convey("Noops", func() {
			pm.pb.Pcls = []*prjpb.PCL{
				{Clid: 33, ConfigGroupIndexes: []int32{singIdx}, Trigger: dryRun(ct.Clock.Now())},
			}
			oldC := &prjpb.Component{
				Clids: []int64{33},
				// Component already has a Run, so no action required.
				Pruns: []*prjpb.PRun{{Id: "id", Clids: []int64{33}}},
				Dirty: true,
			}
			res := mustTriage(oldC)
			So(res.NewValue, ShouldResembleProto, undirty(oldC))
			So(res.RunsToCreate, ShouldBeEmpty)
			So(res.CLsToPurge, ShouldBeEmpty)
		})

		Convey("Prunes CLs", func() {
			pm.pb.Pcls = []*prjpb.PCL{
				{
					Clid:               33,
					ConfigGroupIndexes: nil, // modified below.
					Trigger:            dryRun(ct.Clock.Now()),
					Errors: []*changelist.CLError{ // => must purge.
						{Kind: &changelist.CLError_OwnerLacksEmail{OwnerLacksEmail: true}},
					},
				},
			}
			oldC := &prjpb.Component{Clids: []int64{33}}

			Convey("singular group -- no delay", func() {
				pm.pb.Pcls[0].ConfigGroupIndexes = []int32{singIdx}
				res := mustTriage(oldC)
				So(res.NewValue, ShouldResembleProto, undirty(oldC))
				So(res.CLsToPurge, ShouldHaveLength, 1)
				So(res.RunsToCreate, ShouldBeEmpty)
			})
			Convey("combinable group -- obey stabilization_delay", func() {
				pm.pb.Pcls[0].ConfigGroupIndexes = []int32{combIdx}

				res := mustTriage(oldC)
				c := undirty(oldC)
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
				pm.pb.Pcls[0].OwnerLacksEmail = false // many groups is an error itself
				pm.pb.Pcls[0].ConfigGroupIndexes = []int32{singIdx, combIdx, anotherIdx}
				res := mustTriage(oldC)
				So(res.NewValue, ShouldResembleProto, undirty(oldC))
				So(res.CLsToPurge, ShouldHaveLength, 1)
				So(res.RunsToCreate, ShouldBeEmpty)
			})
		})

		Convey("Creates Runs", func() {
			putPCL := func(clid int, grpIndex int32, mode run.Mode, triggerTime time.Time, depsCLIDs ...int) (*changelist.CL, *prjpb.PCL) {
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
				tr := trigger.Find(ci, nil)
				So(tr.GetMode(), ShouldResemble, string(mode))
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
				return cl, &prjpb.PCL{
					Clid:               int64(clid),
					Eversion:           1,
					Status:             prjpb.PCL_OK,
					ConfigGroupIndexes: []int32{grpIndex},
					Trigger:            tr,
					Deps:               cl.Snapshot.GetDeps(),
				}
			}

			Convey("Singular", func() {
				Convey("OK", func() {
					_, pcl := putPCL(33, singIdx, run.DryRun, ct.Clock.Now())
					pm.pb.Pcls = []*prjpb.PCL{pcl}
					oldC := &prjpb.Component{Clids: []int64{33}, Dirty: true}
					res := mustTriage(oldC)
					So(res.NewValue, ShouldResembleProto, undirty(oldC))
					So(res.CLsToPurge, ShouldBeEmpty)
					So(res.RunsToCreate, ShouldHaveLength, 1)
					rc := res.RunsToCreate[0]
					So(rc.ConfigGroupID.Name(), ShouldResemble, "singular")
					So(rc.Mode, ShouldResemble, run.DryRun)
					So(rc.InputCLs, ShouldHaveLength, 1)
					So(rc.InputCLs[0].ID, ShouldEqual, 33)
				})
				Convey("EVersion mismatch is a transient error", func() {
					cl, pcl := putPCL(33, singIdx, run.DryRun, ct.Clock.Now())
					cl.EVersion = 2
					So(datastore.Put(ctx, cl), ShouldBeNil)
					pm.pb.Pcls = []*prjpb.PCL{pcl}
					err := failTriage(&prjpb.Component{Clids: []int64{33}, Dirty: true})
					So(transient.Tag.In(err), ShouldBeTrue)
					So(err, ShouldErrLike, "EVersion changed 1 => 2")
				})
				Convey("OK with resolved deps", func() {
					_, pcl32 := putPCL(32, singIdx, run.FullRun, ct.Clock.Now())
					_, pcl33 := putPCL(33, singIdx, run.DryRun, ct.Clock.Now(), 32)
					pm.pb.Pcls = []*prjpb.PCL{pcl32, pcl33}
					oldC := &prjpb.Component{Clids: []int64{32, 33}, Dirty: true}
					res := mustTriage(oldC)
					So(res.NewValue, ShouldResembleProto, undirty(oldC))
					So(res.CLsToPurge, ShouldBeEmpty)
					So(res.RunsToCreate, ShouldHaveLength, 2)
					sortRunsToCreateByFirstCL(&res)
					So(res.RunsToCreate[0].InputCLs[0].ID, ShouldEqual, 32)
					So(res.RunsToCreate[0].Mode, ShouldResemble, run.FullRun)
					So(res.RunsToCreate[1].InputCLs[0].ID, ShouldEqual, 33)
					So(res.RunsToCreate[1].Mode, ShouldResemble, run.DryRun)
				})
				Convey("Waits for unresolved dep without an error", func() {
					pcl32 := &prjpb.PCL{Clid: 32, Eversion: 1, Status: prjpb.PCL_UNKNOWN}
					_, pcl33 := putPCL(33, singIdx, run.DryRun, ct.Clock.Now(), 32)
					pm.pb.Pcls = []*prjpb.PCL{pcl32, pcl33}
					oldC := &prjpb.Component{Clids: []int64{33}, Dirty: true}
					res := mustTriage(oldC)
					So(res.NewValue, ShouldResembleProto, undirty(oldC))
					// TODO(crbug/1211576): this waiting can last forever. Component needs
					// to record how long it has been waiting and abort with clear message
					// to the user.
					So(res.NewValue.GetDecisionTime(), ShouldBeNil) // wait for external event of loading a dep
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
