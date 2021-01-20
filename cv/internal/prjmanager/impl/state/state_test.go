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

package state

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/cfgmatcher"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/internal"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type ctest struct {
	cvtesting.Test

	lProject string
	gHost    string
}

func (ct ctest) runCLUpdater(ctx context.Context, change int64) *changelist.CL {
	So(updater.Schedule(ctx, &updater.RefreshGerritCL{
		LuciProject: ct.lProject,
		Host:        ct.gHost,
		Change:      change,
	}), ShouldBeNil)
	ct.TQ.Run(ctx, tqtesting.StopAfterTask(updater.TaskClassID))
	eid, err := changelist.GobID(ct.gHost, change)
	So(err, ShouldBeNil)
	cl, err := eid.Get(ctx)
	So(err, ShouldBeNil)
	So(cl, ShouldNotBeNil)
	return cl
}

const cfgText1 = `
  config_groups {
    name: "g0"
    gerrit {
      url: "https://c-review.example.com"  # Must match gHost.
      projects {
        name: "repo/a"
        ref_regexp: "refs/heads/main"
      }
    }
  }
  config_groups {
    name: "g1"
    fallback: YES
    gerrit {
      url: "https://c-review.example.com"  # Must match gHost.
      projects {
        name: "repo/a"
        ref_regexp: "refs/heads/.+"
      }
    }
  }
`

func updateConfigToNoFallabck(ctx context.Context, ct *ctest) config.Meta {
	cfgText2 := strings.ReplaceAll(cfgText1, "fallback: YES", "fallback: NO")
	cfg2 := &cfgpb.Config{}
	So(prototext.Unmarshal([]byte(cfgText2), cfg2), ShouldBeNil)
	ct.Cfg.Update(ctx, ct.lProject, cfg2)
	gobmap.Update(ctx, ct.lProject)
	return ct.Cfg.MustExist(ctx, ct.lProject)
}

func TestUpdateConfig(t *testing.T) {
	t.Parallel()

	Convey("updateConfig works", t, func() {
		ct := ctest{
			lProject: "test",
			gHost:    "c-review.example.com",
		}
		ctx, cancel := ct.SetUp()
		defer cancel()

		cfg1 := &cfgpb.Config{}
		So(prototext.Unmarshal([]byte(cfgText1), cfg1), ShouldBeNil)

		ct.Cfg.Create(ctx, ct.lProject, cfg1)
		meta := ct.Cfg.MustExist(ctx, ct.lProject)
		So(gobmap.Update(ctx, ct.lProject), ShouldBeNil)

		Convey("initializes newly started project", func() {
			// Newly started project doesn't have any CLs, yet, regardless of what CL
			// snapshots are stored in Datastore.
			s0 := NewInitial(ct.lProject)
			pb0 := backupPB(s0)
			s1, sideEffect, err := s0.UpdateConfig(ctx)
			So(err, ShouldBeNil)
			So(s0.PB, ShouldResembleProto, pb0) // s0 must not change.
			So(sideEffect, ShouldResemble, &UpdateIncompleteRunsConfig{
				Hash:     meta.Hash(),
				EVersion: meta.EVersion,
				RunIDs:   nil,
			})
			So(s1.Status, ShouldEqual, prjmanager.Status_STARTED)
			So(s1.PB, ShouldResembleProto, &internal.PState{
				LuciProject:      ct.lProject,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Components:       nil,
				Pcls:             nil,
				DirtyComponents:  true,
			})
		})

		// Add 3 CLs: 101 standalone and 202<-203 as a stack.
		ci101 := gf.CI(
			101, gf.PS(1), gf.Ref("refs/heads/main"), gf.Project("repo/a"),
			gf.CQ(+2, ct.Clock.Now(), gf.U("user-1")), gf.Updated(ct.Clock.Now()),
		)
		ci202 := gf.CI(
			202, gf.PS(3), gf.Ref("refs/heads/other"), gf.Project("repo/a"), gf.AllRevs(),
			gf.CQ(+1, ct.Clock.Now(), gf.U("user-2")), gf.Updated(ct.Clock.Now()),
		)
		ci203 := gf.CI(
			203, gf.PS(3), gf.Ref("refs/heads/other"), gf.Project("repo/a"), gf.AllRevs(),
			gf.CQ(+1, ct.Clock.Now(), gf.U("user-2")), gf.Updated(ct.Clock.Now()),
		)
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci101})
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci202})
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci203})
		ct.GFake.SetDependsOn(ct.gHost, "203_3" /* child */, "202_2" /*parent*/)
		cl101 := ct.runCLUpdater(ctx, 101)
		cl202 := ct.runCLUpdater(ctx, 202)
		cl203 := ct.runCLUpdater(ctx, 203)

		s1 := NewExisting(prjmanager.Status_STARTED, &internal.PState{
			LuciProject:      ct.lProject,
			ConfigHash:       meta.Hash(),
			ConfigGroupNames: []string{"g0", "g1"},
			Pcls: []*internal.PCL{
				{
					Clid:               int64(cl101.ID),
					Eversion:           1,
					ConfigGroupIndexes: []int32{0}, // g0
					Status:             internal.PCL_OK,
					Trigger:            trigger.Find(ci101),
				},
				{
					Clid:               int64(cl202.ID),
					Eversion:           1,
					ConfigGroupIndexes: []int32{1}, // g1
					Status:             internal.PCL_OK,
					Trigger:            trigger.Find(ci202),
				},
				{
					Clid:               int64(cl203.ID),
					Eversion:           1,
					ConfigGroupIndexes: []int32{1}, // g1
					Status:             internal.PCL_OK,
					Trigger:            trigger.Find(ci203),
					Deps:               []*changelist.Dep{{Clid: int64(cl202.ID), Kind: changelist.DepKind_HARD}},
				},
			},
			Components: []*internal.Component{
				{
					Clids: []int64{int64(cl101.ID)},
					Pruns: []*internal.PRun{
						{
							Id:    ct.lProject + "/" + "1111-v1-beef",
							Clids: []int64{int64(cl101.ID)},
						},
					},
				},
				{
					Clids: []int64{404},
				},
			},
		})
		pb1 := backupPB(s1)

		Convey("noop update is quick", func() {
			s2, sideEffect, err := s1.UpdateConfig(ctx)
			So(err, ShouldBeNil)
			So(s2, ShouldEqual, s1) // pointer comparison only.
			So(sideEffect, ShouldBeNil)
		})

		Convey("existing projects is updated without touching components", func() {
			meta2 := updateConfigToNoFallabck(ctx, &ct)
			s2, sideEffect, err := s1.UpdateConfig(ctx)
			So(err, ShouldBeNil)
			So(s1.PB, ShouldResembleProto, pb1) // s1 must not change.
			So(sideEffect, ShouldResemble, &UpdateIncompleteRunsConfig{
				Hash:     meta2.Hash(),
				EVersion: meta2.EVersion,
				RunIDs:   common.MakeRunIDs(ct.lProject + "/" + "1111-v1-beef"),
			})
			So(s2.Status, ShouldEqual, prjmanager.Status_STARTED)
			So(s2.PB, ShouldResembleProto, &internal.PState{
				LuciProject:      ct.lProject,
				ConfigHash:       meta2.Hash(), // changed
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: []*internal.PCL{
					{
						Clid:               int64(cl101.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{0, 1}, // +g1, because g1 is no longer "fallback: YES"
						Status:             internal.PCL_OK,
						Trigger:            trigger.Find(ci101),
					},
					pb1.Pcls[1], // #202 didn't change.
					pb1.Pcls[2], // #203 didn't change.
				},
				Components:      pb1.Components, // no changes here.
				DirtyComponents: true,           // set to re-eval components
			})
		})

		Convey("disabled project updated with long ago deleted CL", func() {
			s1.Status = prjmanager.Status_STOPPED
			for _, c := range s1.PB.GetComponents() {
				c.Pruns = nil // disabled projects don't have incomplete runs.
			}
			pb1 = backupPB(s1)
			changelist.Delete(ctx, cl101.ID)

			meta2 := updateConfigToNoFallabck(ctx, &ct)
			s2, sideEffect, err := s1.UpdateConfig(ctx)
			So(err, ShouldBeNil)
			So(s1.PB, ShouldResembleProto, pb1) // s1 must not change.
			So(sideEffect, ShouldResemble, &UpdateIncompleteRunsConfig{
				Hash:     meta2.Hash(),
				EVersion: meta2.EVersion,
				// No runs to notify.
			})
			So(s2.Status, ShouldEqual, prjmanager.Status_STARTED)
			So(s2.PB, ShouldResembleProto, &internal.PState{
				LuciProject:      ct.lProject,
				ConfigHash:       meta2.Hash(), // changed
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: []*internal.PCL{
					{
						Clid:     int64(cl101.ID),
						Eversion: 1,
						Status:   internal.PCL_DELETED,
					},
					pb1.Pcls[1], // #202 didn't change.
					pb1.Pcls[2], // #203 didn't change.
				},
				Components:      pb1.Components, // no changes here.
				DirtyComponents: true,           // set to re-eval components
			})
		})

		Convey("disabled project waits for incomplete Runs", func() {
			ct.Cfg.Disable(ctx, ct.lProject)
			s2, sideEffect, err := s1.UpdateConfig(ctx)
			So(err, ShouldBeNil)
			So(s2.Status, ShouldEqual, prjmanager.Status_STOPPING)
			So(s2.PB, ShouldResembleProto, s1.PB)
			So(sideEffect, ShouldResemble, &CancelIncompleteRuns{
				RunIDs: common.MakeRunIDs(ct.lProject + "/" + "1111-v1-beef"),
			})

		})

		Convey("disabled project stops iff there are no incomplete Runs", func() {
			for _, c := range s1.PB.GetComponents() {
				c.Pruns = nil
			}
			ct.Cfg.Disable(ctx, ct.lProject)
			s2, sideEffect, err := s1.UpdateConfig(ctx)
			So(err, ShouldBeNil)
			So(s2.Status, ShouldEqual, prjmanager.Status_STOPPED)
			So(s2.PB, ShouldResembleProto, s1.PB)
			So(sideEffect, ShouldBeNil)
		})

		// The rest of the test coverage of UpdateConfig is achieved by testing code
		// of makePCL.

		Convey("makePCL with full snapshot works", func() {
			var err error
			s1.cfgMatcher, err = cfgmatcher.LoadMatcherFrom(ctx, meta)
			So(err, ShouldBeNil)

			Convey("Status == OK", func() {
				expected := &internal.PCL{
					Clid:               int64(cl101.ID),
					Eversion:           int64(cl101.EVersion),
					ConfigGroupIndexes: []int32{0}, // g0
					Trigger: &run.Trigger{
						Email:           "user-1@example.com",
						GerritAccountId: 1,
						Mode:            string(run.FullRun),
						Time:            timestamppb.New(ct.Clock.Now()),
					},
				}
				Convey("CL snapshotted with current config", func() {
					So(s1.makePCL(ctx, cl101), ShouldResembleProto, expected)
				})
				Convey("CL snapshotted with an older config", func() {
					cl101.ApplicableConfig.GetProjects()[0].ConfigGroupIds = []string{"oldhash/g0"}
					So(s1.makePCL(ctx, cl101), ShouldResembleProto, expected)
				})
				Convey("not triggered CL", func() {
					delete(cl101.Snapshot.GetGerrit().GetInfo().GetLabels(), trigger.CQLabelName)
					expected.Trigger = nil
					So(s1.makePCL(ctx, cl101), ShouldResembleProto, expected)
				})
			})

			Convey("snapshot from diff project requires waiting", func() {
				cl101.Snapshot.LuciProject = "another"
				So(s1.makePCL(ctx, cl101), ShouldResembleProto, &internal.PCL{
					Clid:     int64(cl101.ID),
					Eversion: int64(cl101.EVersion),
					Status:   internal.PCL_UNKNOWN,
				})
			})

			Convey("CL from diff project is unwatched", func() {
				s1.PB.LuciProject = "another"
				So(s1.makePCL(ctx, cl101), ShouldResembleProto, &internal.PCL{
					Clid:     int64(cl101.ID),
					Eversion: int64(cl101.EVersion),
					Status:   internal.PCL_UNWATCHED,
				})
			})

			Convey("CL watched by several projects is unwatched", func() {
				cl101.ApplicableConfig.Projects = append(
					cl101.ApplicableConfig.GetProjects(),
					&changelist.ApplicableConfig_Project{
						ConfigGroupIds: []string{"g"},
						Name:           "another",
					})
				So(s1.makePCL(ctx, cl101), ShouldResembleProto, &internal.PCL{
					Clid:     int64(cl101.ID),
					Eversion: int64(cl101.EVersion),
					Status:   internal.PCL_UNWATCHED,
				})
			})
		})
	})
}

func TestOnCLsUpdated(t *testing.T) {
	t.Parallel()

	Convey("OnCLsUpdated works", t, func() {
		ct := ctest{
			lProject: "test",
			gHost:    "c-review.example.com",
		}
		ctx, cancel := ct.SetUp()
		defer cancel()

		cfg1 := &cfgpb.Config{}
		So(prototext.Unmarshal([]byte(cfgText1), cfg1), ShouldBeNil)

		ct.Cfg.Create(ctx, ct.lProject, cfg1)
		meta := ct.Cfg.MustExist(ctx, ct.lProject)
		So(gobmap.Update(ctx, ct.lProject), ShouldBeNil)

		// Add 3 CLs: 101 standalone and 202<-203 as a stack.
		ci101 := gf.CI(
			101, gf.PS(1), gf.Ref("refs/heads/main"), gf.Project("repo/a"),
			gf.CQ(+2, ct.Clock.Now(), gf.U("user-1")), gf.Updated(ct.Clock.Now()),
		)
		ci202 := gf.CI(
			202, gf.PS(3), gf.Ref("refs/heads/other"), gf.Project("repo/a"), gf.AllRevs(),
			gf.CQ(+1, ct.Clock.Now(), gf.U("user-2")), gf.Updated(ct.Clock.Now()),
		)
		ci203 := gf.CI(
			203, gf.PS(3), gf.Ref("refs/heads/other"), gf.Project("repo/a"), gf.AllRevs(),
			gf.CQ(+1, ct.Clock.Now(), gf.U("user-2")), gf.Updated(ct.Clock.Now()),
		)
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci101})
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci202})
		ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: ci203})
		ct.GFake.SetDependsOn(ct.gHost, "203_3" /* child */, "202_2" /*parent*/)
		cl101 := ct.runCLUpdater(ctx, 101)
		cl202 := ct.runCLUpdater(ctx, 202)
		cl203 := ct.runCLUpdater(ctx, 203)

		s0 := NewExisting(prjmanager.Status_STARTED, &internal.PState{
			LuciProject:      ct.lProject,
			ConfigHash:       meta.Hash(),
			ConfigGroupNames: []string{"g0", "g1"},
		})
		pb0 := backupPB(s0)

		// NOTE: conversion of individual CL to PCL is in TestUpdateConfig.

		Convey("One simple CL", func() {
			s1, sideEffect, err := s0.OnCLsUpdated(ctx, []*internal.CLUpdated{
				{Clid: int64(cl101.ID), Eversion: int64(cl101.EVersion)},
			})
			So(err, ShouldBeNil)
			So(s0.PB, ShouldResembleProto, pb0)
			So(sideEffect, ShouldBeNil)
			So(s1.PB, ShouldResembleProto, &internal.PState{
				LuciProject:      ct.lProject,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: []*internal.PCL{
					{
						Clid:               int64(cl101.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{0}, // g0
						Status:             internal.PCL_OK,
						Trigger:            trigger.Find(ci101),
					},
				},
				DirtyComponents: true,
			})
			Convey("Noop based on EVersion", func() {
				s2, sideEffect, err := s1.OnCLsUpdated(ctx, []*internal.CLUpdated{
					{Clid: int64(cl101.ID), Eversion: 1}, // already known
				})
				So(err, ShouldBeNil)
				So(sideEffect, ShouldBeNil)
				So(s1, ShouldEqual, s2) // pointer comparison only.
			})

			Convey("Removes duplicates", func() {
				pb := backupPB(s1)
				bumpEVersion(ctx, cl101, 10)
				s2, sideEffect, err := s1.OnCLsUpdated(ctx, []*internal.CLUpdated{
					{Clid: int64(cl101.ID), Eversion: 5},
					{Clid: int64(cl101.ID), Eversion: 7},
				})
				So(err, ShouldBeNil)
				So(sideEffect, ShouldBeNil)
				So(s1.PB, ShouldResembleProto, pb)
				pb.GetPcls()[0].Eversion = int64(cl101.EVersion)
				So(s2.PB, ShouldResembleProto, pb)
			})
		})

		Convey("One CL with a yet unknown dep", func() {
			s1, sideEffect, err := s0.OnCLsUpdated(ctx, []*internal.CLUpdated{
				{Clid: int64(cl203.ID), Eversion: 1},
			})
			So(err, ShouldBeNil)
			So(s0.PB, ShouldResembleProto, pb0)
			So(sideEffect, ShouldBeNil)
			So(s1.PB, ShouldResembleProto, &internal.PState{
				LuciProject:      ct.lProject,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: []*internal.PCL{
					{
						Clid:               int64(cl203.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             internal.PCL_OK,
						Trigger:            trigger.Find(ci203),
						Deps:               []*changelist.Dep{{Clid: int64(cl202.ID), Kind: changelist.DepKind_HARD}},
					},
				},
				DirtyComponents: true,
			})
		})

		Convey("PCLs must remain sorted", func() {
			s1 := NewExisting(prjmanager.Status_STARTED, &internal.PState{
				LuciProject:      ct.lProject,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: []*internal.PCL{
					{
						Clid:               int64(cl101.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{0}, // g0
						Status:             internal.PCL_OK,
						Trigger:            trigger.Find(ci101),
					},
					{
						Clid:               int64(cl203.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             internal.PCL_OK,
						Trigger:            trigger.Find(ci203),
						Deps:               []*changelist.Dep{{Clid: int64(cl202.ID), Kind: changelist.DepKind_HARD}},
					},
				},
			})
			pb1 := backupPB(s1)
			bumpEVersion(ctx, cl203, 3)
			s2, sideEffect, err := s1.OnCLsUpdated(ctx, []*internal.CLUpdated{
				{Clid: 404, Eversion: 404},                               // doesn't even exist
				{Clid: int64(cl202.ID), Eversion: int64(cl202.EVersion)}, // new
				{Clid: int64(cl101.ID), Eversion: int64(cl101.EVersion)}, // unchanged
				{Clid: int64(cl203.ID), Eversion: 3},                     // updated
			})
			So(err, ShouldBeNil)
			So(s1.PB, ShouldResembleProto, pb1)
			So(sideEffect, ShouldBeNil)
			So(s2.PB, ShouldResembleProto, &internal.PState{
				LuciProject:      ct.lProject,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: []*internal.PCL{
					s1.PB.GetPcls()[0], // 101 is unchanged
					{ // new & inserted at the right spot
						Clid:               int64(cl202.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             internal.PCL_OK,
						Trigger:            trigger.Find(ci202),
					},
					{ // updated
						Clid:               int64(cl203.ID),
						Eversion:           3,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             internal.PCL_OK,
						Trigger:            trigger.Find(ci203),
						Deps:               []*changelist.Dep{{Clid: int64(cl202.ID), Kind: changelist.DepKind_HARD}},
					},
				},
				DirtyComponents: true,
			})
		})

		Convey("non-STARTED project ignores all CL events", func() {
			s0.Status = prjmanager.Status_STOPPING
			s1, sideEffect, err := s0.OnCLsUpdated(ctx, []*internal.CLUpdated{
				{Clid: int64(cl101.ID), Eversion: int64(cl101.EVersion)},
			})
			So(err, ShouldBeNil)
			So(sideEffect, ShouldBeNil)
			So(s0, ShouldEqual, s1) // pointer comparison only.
		})
	})
}

func TestRunsCreatedAndFinished(t *testing.T) {
	t.Parallel()

	Convey("OnRunsCreated and OnRunsFinished works", t, func() {
		ct := ctest{
			lProject: "test",
			gHost:    "c-review.example.com",
		}
		ctx, cancel := ct.SetUp()
		defer cancel()

		cfg1 := &cfgpb.Config{}
		So(prototext.Unmarshal([]byte(cfgText1), cfg1), ShouldBeNil)
		ct.Cfg.Create(ctx, ct.lProject, cfg1)
		meta := ct.Cfg.MustExist(ctx, ct.lProject)

		run1 := &run.Run{ID: common.RunID(ct.lProject + "/101-aaa"), CLs: []common.CLID{101}}
		run789 := &run.Run{ID: common.RunID(ct.lProject + "/789-efg"), CLs: []common.CLID{709, 707, 708}}
		So(datastore.Put(ctx, run1, run789), ShouldBeNil)

		s1 := NewExisting(prjmanager.Status_STARTED, &internal.PState{
			LuciProject:      ct.lProject,
			ConfigHash:       meta.Hash(),
			ConfigGroupNames: []string{"g0", "g1"},
			// For OnRunsFinished / OnRunsCreated PCLs don't matter, so omit them from
			// the test for brevity, even though valid State must have PCLs covering
			// all components.
			Pcls: nil,
			Components: []*internal.Component{
				{
					Clids: []int64{101},
					Pruns: []*internal.PRun{{Id: ct.lProject + "/101-aaa", Clids: []int64{1}}},
				},
				{
					Clids: []int64{202, 203, 204},
				},
			},
			CreatedPruns: []*internal.PRun{
				{Id: ct.lProject + "/789-efg", Clids: []int64{707, 708, 709}},
			},
		})
		pb1 := backupPB(s1)

		Convey("Noops", func() {
			Convey("OnRunsFinished on not tracked Run", func() {
				s2, sideEffect, err := s1.OnRunsFinished(ctx, common.MakeRunIDs(ct.lProject+"/999-zzz"))
				So(err, ShouldBeNil)
				So(sideEffect, ShouldBeNil)
				// although s2 is cloned, it must be exact same as s1.
				So(s2.Status, ShouldEqual, s1.Status)
				So(s2.PB, ShouldResembleProto, pb1)
			})
			Convey("OnRunsCreated on already tracked Run", func() {
				s2, sideEffect, err := s1.OnRunsCreated(ctx, common.MakeRunIDs(ct.lProject+"/101-aaa"))
				So(err, ShouldBeNil)
				So(sideEffect, ShouldBeNil)
				So(s2, ShouldEqual, s1)
				So(pb1, ShouldResembleProto, s1.PB)
			})
			Convey("OnRunsCreated on somehow already deleted run", func() {
				s2, sideEffect, err := s1.OnRunsCreated(ctx, common.MakeRunIDs(ct.lProject+"/404-nnn"))
				So(err, ShouldBeNil)
				So(sideEffect, ShouldBeNil)
				// although s2 is cloned, it must be exact same as s1.
				So(s2.Status, ShouldEqual, s1.Status)
				So(s2.PB, ShouldResembleProto, pb1)
			})
		})

		Convey("OnRunsCreated", func() {
			runX := &run.Run{ // Run involving all of CLs and more.
				ID: common.RunID(ct.lProject + "/000-xxx"),
				// The order doesn't have to and is intentionally not sorted here.
				CLs: []common.CLID{404, 101, 202, 204, 203},
			}
			run2 := &run.Run{ID: common.RunID(ct.lProject + "/202-bbb"), CLs: []common.CLID{202}}
			run3 := &run.Run{ID: common.RunID(ct.lProject + "/203-ccc"), CLs: []common.CLID{203}}
			run23 := &run.Run{ID: common.RunID(ct.lProject + "/232-bcb"), CLs: []common.CLID{203, 202}}
			run234 := &run.Run{ID: common.RunID(ct.lProject + "/234-bcd"), CLs: []common.CLID{203, 204, 202}}
			So(datastore.Put(ctx, run2, run3, run23, run234, runX), ShouldBeNil)

			s2, sideEffect, err := s1.OnRunsCreated(ctx, common.RunIDs{
				run2.ID, run3.ID, run23.ID, run234.ID, runX.ID,
				// non-existing Run shouldn't derail others.
				common.RunID(ct.lProject + "/404-nnn"),
			})
			So(err, ShouldBeNil)
			So(pb1, ShouldResembleProto, s1.PB)
			So(sideEffect, ShouldBeNil)
			So(s2.PB, ShouldResembleProto, &internal.PState{
				LuciProject:      ct.lProject,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Components: []*internal.Component{
					s1.PB.GetComponents()[0], // 101 is unchanged
					{
						Clids: []int64{202, 203, 204},
						Pruns: []*internal.PRun{
							// Runs & CLs must be sorted by their respective IDs.
							{Id: string(run2.ID), Clids: []int64{202}},
							{Id: string(run3.ID), Clids: []int64{203}},
							{Id: string(run23.ID), Clids: []int64{202, 203}},
							{Id: string(run234.ID), Clids: []int64{202, 203, 204}},
						},
						Dirty: true,
					},
				},
				CreatedPruns: []*internal.PRun{
					{Id: string(runX.ID), Clids: []int64{101, 202, 203, 204, 404}},
					{Id: ct.lProject + "/789-efg", Clids: []int64{707, 708, 709}}, // unchanged
				},
			})
		})

		Convey("OnRunsFinished", func() {
			s1.Status = prjmanager.Status_STOPPING
			pb1 := backupPB(s1)

			Convey("deletes from Components", func() {
				pb1 := backupPB(s1)
				s2, sideEffect, err := s1.OnRunsFinished(ctx, common.MakeRunIDs(ct.lProject+"/101-aaa"))
				So(err, ShouldBeNil)
				So(pb1, ShouldResembleProto, s1.PB)
				So(sideEffect, ShouldBeNil)
				So(s2.Status, ShouldEqual, prjmanager.Status_STOPPING)
				So(s2.PB, ShouldResembleProto, &internal.PState{
					LuciProject:      ct.lProject,
					ConfigHash:       meta.Hash(),
					ConfigGroupNames: []string{"g0", "g1"},
					Components: []*internal.Component{
						{
							Clids: []int64{101},
							Pruns: nil,  // removed
							Dirty: true, // marked dirty
						},
						s1.PB.GetComponents()[1], // unchanged
					},
					CreatedPruns: s1.PB.GetCreatedPruns(), // unchanged
				})
			})

			Convey("deletes from CreatedPruns", func() {
				s2, sideEffect, err := s1.OnRunsFinished(ctx, common.MakeRunIDs(ct.lProject+"/789-efg"))
				So(err, ShouldBeNil)
				So(pb1, ShouldResembleProto, s1.PB)
				So(sideEffect, ShouldBeNil)
				So(s2.Status, ShouldEqual, prjmanager.Status_STOPPING)
				So(s2.PB, ShouldResembleProto, &internal.PState{
					LuciProject:      ct.lProject,
					ConfigHash:       meta.Hash(),
					ConfigGroupNames: []string{"g0", "g1"},
					Components:       s1.PB.Components, // unchanged
					CreatedPruns:     nil,              // removed
				})
			})

			Convey("stops PM iff all runs finished", func() {
				s2, sideEffect, err := s1.OnRunsFinished(ctx, common.MakeRunIDs(
					ct.lProject+"/101-aaa",
					ct.lProject+"/789-efg",
				))
				So(err, ShouldBeNil)
				So(pb1, ShouldResembleProto, s1.PB)
				So(sideEffect, ShouldBeNil)
				So(s2.Status, ShouldEqual, prjmanager.Status_STOPPED)
				So(s2.PB, ShouldResembleProto, &internal.PState{
					LuciProject:      ct.lProject,
					ConfigHash:       meta.Hash(),
					ConfigGroupNames: []string{"g0", "g1"},
					Pcls:             s1.PB.GetPcls(),
					Components: []*internal.Component{
						{Clids: []int64{101}, Dirty: true},
						s1.PB.GetComponents()[1], // unchanged.
					},
					CreatedPruns: nil, // removed
				})
			})
		})

	})
}

// backupPB returns a deep copy of State.PB for future assertion that State
// wasn't modified.
func backupPB(s *State) *internal.PState {
	ret := &internal.PState{}
	proto.Merge(ret, s.PB)
	return ret
}

func bumpEVersion(ctx context.Context, cl *changelist.CL, desired int) {
	if cl.EVersion >= desired {
		panic(fmt.Errorf("can't go %d to %d", cl.EVersion, desired))
	}
	cl.EVersion = desired
	So(datastore.Put(ctx, cl), ShouldBeNil)
}
