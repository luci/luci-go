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
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/cfgmatcher"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap/gobmaptest"
	"go.chromium.org/luci/cv/internal/gerrit/poller"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type ctest struct {
	cvtesting.Test

	lProject  string
	gHost     string
	pm        *prjmanager.Notifier
	clUpdater *updater.Updater
}

func (ct *ctest) SetUp() (context.Context, func()) {
	ctx, cancel := ct.Test.SetUp()
	ct.pm = prjmanager.NewNotifier(ct.TQDispatcher)
	ct.clUpdater = updater.New(ct.TQDispatcher, ct.GFactory(), changelist.NewMutator(ct.TQDispatcher, ct.pm, nil))
	return ctx, cancel
}

func (ct ctest) runCLUpdater(ctx context.Context, change int64) *changelist.CL {
	return ct.runCLUpdaterAs(ctx, change, ct.lProject)
}

func (ct ctest) runCLUpdaterAs(ctx context.Context, change int64, lProject string) *changelist.CL {
	So(ct.clUpdater.Refresh(ctx, &updater.RefreshGerritCL{
		LuciProject: lProject,
		Host:        ct.gHost,
		Change:      change,
	}), ShouldBeNil)
	eid, err := changelist.GobID(ct.gHost, change)
	So(err, ShouldBeNil)
	cl, err := eid.Get(ctx)
	So(err, ShouldBeNil)
	So(cl, ShouldNotBeNil)
	return cl
}

func (ct ctest) submitCL(ctx context.Context, change int64) *changelist.CL {
	ct.GFake.MutateChange(ct.gHost, int(change), func(c *gf.Change) {
		gf.Status(gerritpb.ChangeStatus_MERGED)(c.Info)
		gf.Updated(ct.Clock.Now())(c.Info)
	})
	cl := ct.runCLUpdater(ctx, change)

	// If this fails, you forgot to change fake time.
	So(cl.Snapshot.GetGerrit().GetInfo().GetStatus(), ShouldEqual, gerritpb.ChangeStatus_MERGED)
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

func updateConfigToNoFallabck(ctx context.Context, ct *ctest) prjcfg.Meta {
	cfgText2 := strings.ReplaceAll(cfgText1, "fallback: YES", "fallback: NO")
	cfg2 := &cfgpb.Config{}
	So(prototext.Unmarshal([]byte(cfgText2), cfg2), ShouldBeNil)
	prjcfgtest.Update(ctx, ct.lProject, cfg2)
	gobmaptest.Update(ctx, ct.lProject)
	return prjcfgtest.MustExist(ctx, ct.lProject)
}

func updateConfigRenameG1toG11(ctx context.Context, ct *ctest) prjcfg.Meta {
	cfgText2 := strings.ReplaceAll(cfgText1, `"g1"`, `"g11"`)
	cfg2 := &cfgpb.Config{}
	So(prototext.Unmarshal([]byte(cfgText2), cfg2), ShouldBeNil)
	prjcfgtest.Update(ctx, ct.lProject, cfg2)
	gobmaptest.Update(ctx, ct.lProject)
	return prjcfgtest.MustExist(ctx, ct.lProject)
}

func TestUpdateConfig(t *testing.T) {
	t.Parallel()

	Convey("updateConfig works", t, func() {
		ct := ctest{
			lProject: "test",
			gHost:    "c-review.example.com",
			Test:     cvtesting.Test{},
		}
		ctx, cancel := ct.SetUp()
		defer cancel()

		cfg1 := &cfgpb.Config{}
		So(prototext.Unmarshal([]byte(cfgText1), cfg1), ShouldBeNil)

		prjcfgtest.Create(ctx, ct.lProject, cfg1)
		meta := prjcfgtest.MustExist(ctx, ct.lProject)
		gobmaptest.Update(ctx, ct.lProject)

		clPoller := poller.New(ct.TQDispatcher, nil, nil, nil)

		Convey("initializes newly started project", func() {
			// Newly started project doesn't have any CLs, yet, regardless of what CL
			// snapshots are stored in Datastore.
			s0 := &State{
				PB:       &prjpb.PState{LuciProject: ct.lProject},
				CLPoller: clPoller,
			}
			pb0 := backupPB(s0)
			s1, sideEffect, err := s0.UpdateConfig(ctx)
			So(err, ShouldBeNil)
			So(s0.PB, ShouldResembleProto, pb0) // s0 must not change.
			So(sideEffect, ShouldResemble, &UpdateIncompleteRunsConfig{
				Hash:     meta.Hash(),
				EVersion: meta.EVersion,
				RunIDs:   nil,
			})
			So(s1.PB, ShouldResembleProto, &prjpb.PState{
				LuciProject:         ct.lProject,
				Status:              prjpb.Status_STARTED,
				ConfigHash:          meta.Hash(),
				ConfigGroupNames:    []string{"g0", "g1"},
				Components:          nil,
				Pcls:                nil,
				RepartitionRequired: false,
			})
			So(s1.LogReasons, ShouldResemble, []prjpb.LogReason{prjpb.LogReason_CONFIG_CHANGED, prjpb.LogReason_STATUS_CHANGED})
		})

		// Add 3 CLs: 101 standalone and 202<-203 as a stack.
		triggerTS := timestamppb.New(ct.Clock.Now())
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

		s1 := &State{
			PB: &prjpb.PState{
				LuciProject:      ct.lProject,
				Status:           prjpb.Status_STARTED,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: []*prjpb.PCL{
					{
						Clid:               int64(cl101.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{0}, // g0
						Status:             prjpb.PCL_OK,
						Trigger: &run.Trigger{
							Email:           "user-1@example.com",
							GerritAccountId: 1,
							Mode:            string(run.FullRun),
							Time:            triggerTS,
						},
					},
					{
						Clid:               int64(cl202.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             prjpb.PCL_OK,
						Trigger: &run.Trigger{
							Email:           "user-2@example.com",
							GerritAccountId: 2,
							Mode:            string(run.DryRun),
							Time:            triggerTS,
						},
					},
					{
						Clid:               int64(cl203.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             prjpb.PCL_OK,
						Trigger: &run.Trigger{
							Email:           "user-2@example.com",
							GerritAccountId: 2,
							Mode:            string(run.DryRun),
							Time:            triggerTS,
						},
						Deps: []*changelist.Dep{{Clid: int64(cl202.ID), Kind: changelist.DepKind_HARD}},
					},
				},
				Components: []*prjpb.Component{
					{
						Clids: []int64{int64(cl101.ID)},
						Pruns: []*prjpb.PRun{
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
			},
			CLPoller: clPoller,
		}
		pb1 := backupPB(s1)

		Convey("noop update is quick", func() {
			s2, sideEffect, err := s1.UpdateConfig(ctx)
			So(err, ShouldBeNil)
			So(s2, ShouldEqual, s1) // pointer comparison only.
			So(sideEffect, ShouldBeNil)
		})

		Convey("existing project", func() {
			Convey("updated without touching components", func() {
				meta2 := updateConfigToNoFallabck(ctx, &ct)
				s2, sideEffect, err := s1.UpdateConfig(ctx)
				So(err, ShouldBeNil)
				So(s1.PB, ShouldResembleProto, pb1) // s1 must not change.
				So(sideEffect, ShouldResemble, &UpdateIncompleteRunsConfig{
					Hash:     meta2.Hash(),
					EVersion: meta2.EVersion,
					RunIDs:   common.MakeRunIDs(ct.lProject + "/" + "1111-v1-beef"),
				})
				So(s2.PB, ShouldResembleProto, &prjpb.PState{
					LuciProject:      ct.lProject,
					Status:           prjpb.Status_STARTED,
					ConfigHash:       meta2.Hash(), // changed
					ConfigGroupNames: []string{"g0", "g1"},
					Pcls: []*prjpb.PCL{
						{
							Clid:               int64(cl101.ID),
							Eversion:           1,
							ConfigGroupIndexes: []int32{0, 1}, // +g1, because g1 is no longer "fallback: YES"
							Status:             prjpb.PCL_OK,
							Trigger: &run.Trigger{
								Email:           "user-1@example.com",
								GerritAccountId: 1,
								Mode:            string(run.FullRun),
								Time:            triggerTS,
							},
						},
						pb1.Pcls[1], // #202 didn't change.
						pb1.Pcls[2], // #203 didn't change.
					},
					Components:          markForTriage(pb1.Components),
					RepartitionRequired: true,
				})
				So(s2.LogReasons, ShouldResemble, []prjpb.LogReason{prjpb.LogReason_CONFIG_CHANGED})
			})

			Convey("If PCLs stay same, RepartitionRequired must be false", func() {
				meta2 := updateConfigRenameG1toG11(ctx, &ct)
				s2, sideEffect, err := s1.UpdateConfig(ctx)
				So(err, ShouldBeNil)
				So(s1.PB, ShouldResembleProto, pb1) // s1 must not change.
				So(sideEffect, ShouldResemble, &UpdateIncompleteRunsConfig{
					Hash:     meta2.Hash(),
					EVersion: meta2.EVersion,
					RunIDs:   common.MakeRunIDs(ct.lProject + "/" + "1111-v1-beef"),
				})
				So(s2.PB, ShouldResembleProto, &prjpb.PState{
					LuciProject:         ct.lProject,
					Status:              prjpb.Status_STARTED,
					ConfigHash:          meta2.Hash(),
					ConfigGroupNames:    []string{"g0", "g11"}, // g1 -> g11.
					Pcls:                pb1.GetPcls(),
					Components:          markForTriage(pb1.Components),
					RepartitionRequired: false,
				})
			})
		})

		Convey("disabled project updated with long ago deleted CL", func() {
			s1.PB.Status = prjpb.Status_STOPPED
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
			So(s2.PB, ShouldResembleProto, &prjpb.PState{
				LuciProject:      ct.lProject,
				Status:           prjpb.Status_STARTED,
				ConfigHash:       meta2.Hash(), // changed
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: []*prjpb.PCL{
					{
						Clid:     int64(cl101.ID),
						Eversion: 1,
						Status:   prjpb.PCL_DELETED,
					},
					pb1.Pcls[1], // #202 didn't change.
					pb1.Pcls[2], // #203 didn't change.
				},
				Components:          markForTriage(pb1.Components),
				RepartitionRequired: true,
			})
			So(s2.LogReasons, ShouldResemble, []prjpb.LogReason{prjpb.LogReason_CONFIG_CHANGED, prjpb.LogReason_STATUS_CHANGED})
		})

		Convey("disabled project waits for incomplete Runs", func() {
			prjcfgtest.Disable(ctx, ct.lProject)
			s2, sideEffect, err := s1.UpdateConfig(ctx)
			So(err, ShouldBeNil)
			pb := backupPB(s1)
			pb.Status = prjpb.Status_STOPPING
			So(s2.PB, ShouldResembleProto, pb)
			So(sideEffect, ShouldResemble, &CancelIncompleteRuns{
				RunIDs: common.MakeRunIDs(ct.lProject + "/" + "1111-v1-beef"),
			})
			So(s2.LogReasons, ShouldResemble, []prjpb.LogReason{prjpb.LogReason_STATUS_CHANGED})
		})

		Convey("disabled project stops iff there are no incomplete Runs", func() {
			for _, c := range s1.PB.GetComponents() {
				c.Pruns = nil
			}
			prjcfgtest.Disable(ctx, ct.lProject)
			s2, sideEffect, err := s1.UpdateConfig(ctx)
			So(err, ShouldBeNil)
			So(sideEffect, ShouldBeNil)
			pb := backupPB(s1)
			pb.Status = prjpb.Status_STOPPED
			So(s2.PB, ShouldResembleProto, pb)
			So(prjpb.SortAndDedupeLogReasons(s2.LogReasons), ShouldResemble, []prjpb.LogReason{prjpb.LogReason_STATUS_CHANGED})
		})

		// The rest of the test coverage of UpdateConfig is achieved by testing code
		// of makePCL.

		Convey("makePCL with full snapshot works", func() {
			var err error
			s1.configGroups, err = meta.GetConfigGroups(ctx)
			So(err, ShouldBeNil)
			s1.cfgMatcher = cfgmatcher.LoadMatcherFromConfigGroups(ctx, s1.configGroups, &meta)

			Convey("Status == OK", func() {
				expected := &prjpb.PCL{
					Clid:               int64(cl101.ID),
					Eversion:           int64(cl101.EVersion),
					ConfigGroupIndexes: []int32{0}, // g0
					Trigger: &run.Trigger{
						Email:           "user-1@example.com",
						GerritAccountId: 1,
						Mode:            string(run.FullRun),
						Time:            triggerTS,
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
				Convey("abandoned CL is not triggered even if it has CQ vote", func() {
					cl101.Snapshot.GetGerrit().GetInfo().Status = gerritpb.ChangeStatus_ABANDONED
					expected.Trigger = nil
					So(s1.makePCL(ctx, cl101), ShouldResembleProto, expected)
				})
				Convey("Submitted CL is also not triggered even if it has CQ vote", func() {
					cl101.Snapshot.GetGerrit().GetInfo().Status = gerritpb.ChangeStatus_MERGED
					expected.Trigger = nil
					expected.Submitted = true
					So(s1.makePCL(ctx, cl101), ShouldResembleProto, expected)
				})
			})

			Convey("outdated snapshot requires waiting", func() {
				cl101.Snapshot.Outdated = &changelist.Snapshot_Outdated{}
				So(s1.makePCL(ctx, cl101), ShouldResembleProto, &prjpb.PCL{
					Clid:     int64(cl101.ID),
					Eversion: int64(cl101.EVersion),
					Status:   prjpb.PCL_UNKNOWN,
				})
			})

			Convey("snapshot from diff project requires waiting", func() {
				cl101.Snapshot.LuciProject = "another"
				So(s1.makePCL(ctx, cl101), ShouldResembleProto, &prjpb.PCL{
					Clid:     int64(cl101.ID),
					Eversion: int64(cl101.EVersion),
					Status:   prjpb.PCL_UNKNOWN,
				})
			})

			Convey("CL from diff project is unwatched", func() {
				s1.PB.LuciProject = "another"
				So(s1.makePCL(ctx, cl101), ShouldResembleProto, &prjpb.PCL{
					Clid:     int64(cl101.ID),
					Eversion: int64(cl101.EVersion),
					Status:   prjpb.PCL_UNWATCHED,
				})
			})

			Convey("CL watched by several projects is unwatched but with an error", func() {
				cl101.ApplicableConfig.Projects = append(
					cl101.ApplicableConfig.GetProjects(),
					&changelist.ApplicableConfig_Project{
						ConfigGroupIds: []string{"g"},
						Name:           "another",
					})
				So(s1.makePCL(ctx, cl101), ShouldResembleProto, &prjpb.PCL{
					Clid:               int64(cl101.ID),
					Eversion:           int64(cl101.EVersion),
					Status:             prjpb.PCL_OK,
					ConfigGroupIndexes: []int32{0}, // g0
					Trigger: &run.Trigger{
						Email:           "user-1@example.com",
						GerritAccountId: 1,
						Mode:            string(run.FullRun),
						Time:            triggerTS,
					},
					Errors: []*changelist.CLError{
						{
							Kind: &changelist.CLError_WatchedByManyProjects_{
								WatchedByManyProjects: &changelist.CLError_WatchedByManyProjects{
									Projects: []string{s1.PB.GetLuciProject(), "another"},
								},
							},
						},
					},
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

		prjcfgtest.Create(ctx, ct.lProject, cfg1)
		meta := prjcfgtest.MustExist(ctx, ct.lProject)
		gobmaptest.Update(ctx, ct.lProject)

		// Add 3 CLs: 101 standalone and 202<-203 as a stack.
		triggerTS := timestamppb.New(ct.Clock.Now())
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

		s0 := &State{PB: &prjpb.PState{
			LuciProject:      ct.lProject,
			Status:           prjpb.Status_STARTED,
			ConfigHash:       meta.Hash(),
			ConfigGroupNames: []string{"g0", "g1"},
		}}
		pb0 := backupPB(s0)

		// NOTE: conversion of individual CL to PCL is in TestUpdateConfig.

		Convey("One simple CL", func() {
			s1, sideEffect, err := s0.OnCLsUpdated(ctx, map[int64]int64{
				int64(cl101.ID): int64(cl101.EVersion),
			})
			So(err, ShouldBeNil)
			So(s0.PB, ShouldResembleProto, pb0)
			So(sideEffect, ShouldBeNil)
			So(s1.PB, ShouldResembleProto, &prjpb.PState{
				LuciProject:      ct.lProject,
				Status:           prjpb.Status_STARTED,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: []*prjpb.PCL{
					{
						Clid:               int64(cl101.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{0}, // g0
						Status:             prjpb.PCL_OK,
						Trigger: &run.Trigger{
							Email:           "user-1@example.com",
							GerritAccountId: 1,
							Mode:            string(run.FullRun),
							Time:            triggerTS,
						},
					},
				},
				RepartitionRequired: true,
			})
			Convey("Noop based on EVersion", func() {
				s2, sideEffect, err := s1.OnCLsUpdated(ctx, map[int64]int64{
					int64(cl101.ID): 1, // already known
				})
				So(err, ShouldBeNil)
				So(sideEffect, ShouldBeNil)
				So(s1, ShouldEqual, s2) // pointer comparison only.
			})
		})

		Convey("One CL with a yet unknown dep", func() {
			s1, sideEffect, err := s0.OnCLsUpdated(ctx, map[int64]int64{
				int64(cl203.ID): 1,
			})
			So(err, ShouldBeNil)
			So(s0.PB, ShouldResembleProto, pb0)
			So(sideEffect, ShouldBeNil)
			So(s1.PB, ShouldResembleProto, &prjpb.PState{
				LuciProject:      ct.lProject,
				Status:           prjpb.Status_STARTED,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: []*prjpb.PCL{
					{
						Clid:               int64(cl203.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             prjpb.PCL_OK,
						Trigger: &run.Trigger{
							Email:           "user-2@example.com",
							GerritAccountId: 2,
							Mode:            string(run.DryRun),
							Time:            triggerTS,
						},
						Deps: []*changelist.Dep{{Clid: int64(cl202.ID), Kind: changelist.DepKind_HARD}},
					},
				},
				RepartitionRequired: true,
			})
		})

		Convey("PCLs must remain sorted", func() {
			pcl101 := &prjpb.PCL{
				Clid:               int64(cl101.ID),
				Eversion:           1,
				ConfigGroupIndexes: []int32{0}, // g0
				Status:             prjpb.PCL_OK,
				Trigger: &run.Trigger{
					Email:           "user-1@example.com",
					GerritAccountId: 1,
					Mode:            string(run.FullRun),
					Time:            triggerTS,
				},
			}
			s1 := &State{PB: &prjpb.PState{
				LuciProject:      ct.lProject,
				Status:           prjpb.Status_STARTED,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: sortPCLs([]*prjpb.PCL{
					pcl101,
					{
						Clid:               int64(cl203.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             prjpb.PCL_OK,
						Trigger: &run.Trigger{
							Email:           "user-2@example.com",
							GerritAccountId: 2,
							Mode:            string(run.DryRun),
							Time:            triggerTS,
						},
						Deps: []*changelist.Dep{{Clid: int64(cl202.ID), Kind: changelist.DepKind_HARD}},
					},
				}),
			}}
			pb1 := backupPB(s1)
			bumpEVersion(ctx, cl203, 3)
			s2, sideEffect, err := s1.OnCLsUpdated(ctx, map[int64]int64{
				404:             404,                   // doesn't even exist
				int64(cl202.ID): int64(cl202.EVersion), // new
				int64(cl101.ID): int64(cl101.EVersion), // unchanged
				int64(cl203.ID): 3,                     // updated
			})
			So(err, ShouldBeNil)
			So(s1.PB, ShouldResembleProto, pb1)
			So(sideEffect, ShouldBeNil)
			So(s2.PB, ShouldResembleProto, &prjpb.PState{
				LuciProject:      ct.lProject,
				Status:           prjpb.Status_STARTED,
				ConfigHash:       meta.Hash(),
				ConfigGroupNames: []string{"g0", "g1"},
				Pcls: sortPCLs([]*prjpb.PCL{
					{
						Clid:     404,
						Eversion: 0,
						Status:   prjpb.PCL_DELETED,
					},
					pcl101, // 101 is unchanged
					{ // new
						Clid:               int64(cl202.ID),
						Eversion:           1,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             prjpb.PCL_OK,
						Trigger: &run.Trigger{
							Email:           "user-2@example.com",
							GerritAccountId: 2,
							Mode:            string(run.DryRun),
							Time:            triggerTS,
						},
					},
					{ // updated
						Clid:               int64(cl203.ID),
						Eversion:           3,
						ConfigGroupIndexes: []int32{1}, // g1
						Status:             prjpb.PCL_OK,
						Trigger: &run.Trigger{
							Email:           "user-2@example.com",
							GerritAccountId: 2,
							Mode:            string(run.DryRun),
							Time:            triggerTS,
						},
						Deps: []*changelist.Dep{{Clid: int64(cl202.ID), Kind: changelist.DepKind_HARD}},
					},
				}),
				RepartitionRequired: true,
			})
		})

		Convey("Invalid dep of some other CL must be marked as unwatched", func() {
			// For example, if user made a typo in `CQ-Depend`, e.g.:
			//    `CQ-Depend: chromiAm:123`
			// then CL Updater will create an entity for such CL anyway,
			// but eventually fill it with DependentMeta stating that this LUCI
			// project has no access to it.
			// Note that such typos may be malicious, so PM must treat such CLs as not
			// found regardless of whether they actually exist in Gerrit.
			cl404 := ct.runCLUpdater(ctx, 404)
			So(cl404.Snapshot, ShouldBeNil)
			So(cl404.ApplicableConfig, ShouldBeNil)
			So(cl404.Access.GetByProject(), ShouldContainKey, ct.lProject)
			s1, sideEffect, err := s0.OnCLsUpdated(ctx, map[int64]int64{
				int64(cl404.ID): 1,
			})
			So(err, ShouldBeNil)
			So(s0.PB, ShouldResembleProto, pb0)
			So(sideEffect, ShouldBeNil)
			pb1 := proto.Clone(pb0).(*prjpb.PState)
			pb1.Pcls = append(pb0.Pcls, &prjpb.PCL{
				Clid:               int64(cl404.ID),
				Eversion:           1,
				ConfigGroupIndexes: []int32{},
				Status:             prjpb.PCL_UNWATCHED,
			})
			pb1.RepartitionRequired = true
			So(s1.PB, ShouldResembleProto, pb1)
		})

		Convey("non-STARTED project ignores all CL events", func() {
			s0.PB.Status = prjpb.Status_STOPPING
			s1, sideEffect, err := s0.OnCLsUpdated(ctx, map[int64]int64{
				int64(cl101.ID): int64(cl101.EVersion),
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
		prjcfgtest.Create(ctx, ct.lProject, cfg1)
		meta := prjcfgtest.MustExist(ctx, ct.lProject)

		run1 := &run.Run{ID: common.RunID(ct.lProject + "/101-new"), CLs: common.CLIDs{101}}
		run789 := &run.Run{ID: common.RunID(ct.lProject + "/789-efg"), CLs: common.CLIDs{709, 707, 708}}
		run1finished := &run.Run{ID: common.RunID(ct.lProject + "/101-done"), CLs: common.CLIDs{101}, Status: run.Status_FAILED}
		So(datastore.Put(ctx, run1finished, run1, run789), ShouldBeNil)
		So(run.IsEnded(run1finished.Status), ShouldBeTrue)

		s1 := &State{PB: &prjpb.PState{
			LuciProject:      ct.lProject,
			Status:           prjpb.Status_STARTED,
			ConfigHash:       meta.Hash(),
			ConfigGroupNames: []string{"g0", "g1"},
			// For OnRunsFinished / OnRunsCreated PCLs don't matter, so omit them from
			// the test for brevity, even though valid State must have PCLs covering
			// all components.
			Pcls: nil,
			Components: []*prjpb.Component{
				{
					Clids: []int64{101},
					Pruns: []*prjpb.PRun{{Id: ct.lProject + "/101-aaa", Clids: []int64{1}}},
				},
				{
					Clids: []int64{202, 203, 204},
				},
			},
			CreatedPruns: []*prjpb.PRun{
				{Id: ct.lProject + "/789-efg", Clids: []int64{707, 708, 709}},
			},
		}}
		pb1 := backupPB(s1)

		Convey("Noops", func() {
			Convey("OnRunsFinished on not tracked Run", func() {
				s2, sideEffect, err := s1.OnRunsFinished(ctx, common.RunIDs{run1finished.ID})
				So(err, ShouldBeNil)
				So(sideEffect, ShouldBeNil)
				// although s2 is cloned, it must be exact same as s1.
				So(s2.PB, ShouldResembleProto, pb1)
			})
			Convey("OnRunsCreated on already finished run", func() {
				s2, sideEffect, err := s1.OnRunsCreated(ctx, common.RunIDs{run1finished.ID})
				So(err, ShouldBeNil)
				So(sideEffect, ShouldBeNil)
				// although s2 is cloned, it must be exact same as s1.
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
				So(s2.PB, ShouldResembleProto, pb1)
			})
		})

		Convey("OnRunsCreated", func() {
			Convey("when PM is started", func() {
				runX := &run.Run{ // Run involving all of CLs and more.
					ID: common.RunID(ct.lProject + "/000-xxx"),
					// The order doesn't have to and is intentionally not sorted here.
					CLs: common.CLIDs{404, 101, 202, 204, 203},
				}
				run2 := &run.Run{ID: common.RunID(ct.lProject + "/202-bbb"), CLs: common.CLIDs{202}}
				run3 := &run.Run{ID: common.RunID(ct.lProject + "/203-ccc"), CLs: common.CLIDs{203}}
				run23 := &run.Run{ID: common.RunID(ct.lProject + "/232-bcb"), CLs: common.CLIDs{203, 202}}
				run234 := &run.Run{ID: common.RunID(ct.lProject + "/234-bcd"), CLs: common.CLIDs{203, 204, 202}}
				So(datastore.Put(ctx, run2, run3, run23, run234, runX), ShouldBeNil)

				s2, sideEffect, err := s1.OnRunsCreated(ctx, common.RunIDs{
					run2.ID, run3.ID, run23.ID, run234.ID, runX.ID,
					// non-existing Run shouldn't derail others.
					common.RunID(ct.lProject + "/404-nnn"),
				})
				So(err, ShouldBeNil)
				So(pb1, ShouldResembleProto, s1.PB)
				So(sideEffect, ShouldBeNil)
				So(s2.PB, ShouldResembleProto, &prjpb.PState{
					LuciProject:      ct.lProject,
					Status:           prjpb.Status_STARTED,
					ConfigHash:       meta.Hash(),
					ConfigGroupNames: []string{"g0", "g1"},
					Components: []*prjpb.Component{
						s1.PB.GetComponents()[0], // 101 is unchanged
						{
							Clids: []int64{202, 203, 204},
							Pruns: []*prjpb.PRun{
								// Runs & CLs must be sorted by their respective IDs.
								{Id: string(run2.ID), Clids: []int64{202}},
								{Id: string(run3.ID), Clids: []int64{203}},
								{Id: string(run23.ID), Clids: []int64{202, 203}},
								{Id: string(run234.ID), Clids: []int64{202, 203, 204}},
							},
							TriageRequired: true,
						},
					},
					RepartitionRequired: true,
					CreatedPruns: []*prjpb.PRun{
						{Id: string(runX.ID), Clids: []int64{101, 202, 203, 204, 404}},
						{Id: ct.lProject + "/789-efg", Clids: []int64{707, 708, 709}}, // unchanged
					},
				})
			})
			Convey("when PM is stopping", func() {
				s1.PB.Status = prjpb.Status_STOPPING
				pb1 := backupPB(s1)
				Convey("cancels incomplete Runs", func() {
					s2, sideEffect, err := s1.OnRunsCreated(ctx, common.RunIDs{run1.ID, run1finished.ID})
					So(err, ShouldBeNil)
					So(pb1, ShouldResembleProto, s1.PB)
					So(sideEffect, ShouldResemble, &CancelIncompleteRuns{
						RunIDs: common.RunIDs{run1.ID},
					})
					So(s2, ShouldEqual, s1)
				})
			})
		})

		Convey("OnRunsFinished", func() {
			s1.PB.Status = prjpb.Status_STOPPING
			pb1 := backupPB(s1)

			Convey("deletes from Components", func() {
				pb1 := backupPB(s1)
				s2, sideEffect, err := s1.OnRunsFinished(ctx, common.MakeRunIDs(ct.lProject+"/101-aaa"))
				So(err, ShouldBeNil)
				So(pb1, ShouldResembleProto, s1.PB)
				So(sideEffect, ShouldBeNil)
				So(s2.PB, ShouldResembleProto, &prjpb.PState{
					LuciProject:      ct.lProject,
					Status:           prjpb.Status_STOPPING,
					ConfigHash:       meta.Hash(),
					ConfigGroupNames: []string{"g0", "g1"},
					Components: []*prjpb.Component{
						{
							Clids:          []int64{101},
							Pruns:          nil, // removed
							TriageRequired: true,
						},
						s1.PB.GetComponents()[1], // unchanged
					},
					CreatedPruns:        s1.PB.GetCreatedPruns(), // unchanged
					RepartitionRequired: true,
				})
			})

			Convey("deletes from CreatedPruns", func() {
				s2, sideEffect, err := s1.OnRunsFinished(ctx, common.MakeRunIDs(ct.lProject+"/789-efg"))
				So(err, ShouldBeNil)
				So(pb1, ShouldResembleProto, s1.PB)
				So(sideEffect, ShouldBeNil)
				So(s2.PB, ShouldResembleProto, &prjpb.PState{
					LuciProject:      ct.lProject,
					Status:           prjpb.Status_STOPPING,
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
				So(s2.PB, ShouldResembleProto, &prjpb.PState{
					LuciProject:      ct.lProject,
					Status:           prjpb.Status_STOPPED,
					ConfigHash:       meta.Hash(),
					ConfigGroupNames: []string{"g0", "g1"},
					Pcls:             s1.PB.GetPcls(),
					Components: []*prjpb.Component{
						{Clids: []int64{101}, TriageRequired: true},
						s1.PB.GetComponents()[1], // unchanged.
					},
					CreatedPruns:        nil, // removed
					RepartitionRequired: true,
				})
				So(s2.LogReasons, ShouldResemble, []prjpb.LogReason{prjpb.LogReason_STATUS_CHANGED})
			})
		})
	})
}

func TestOnPurgesCompleted(t *testing.T) {
	t.Parallel()

	Convey("OnPurgesCompleted works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		Convey("Empty", func() {
			s1 := &State{PB: &prjpb.PState{}}
			s2, sideEffect, err := s1.OnPurgesCompleted(ctx, []*prjpb.PurgeCompleted{{OperationId: "op1"}})
			So(err, ShouldBeNil)
			So(sideEffect, ShouldBeNil)
			So(s1, ShouldEqual, s2)
		})

		Convey("With existing", func() {
			now := testclock.TestRecentTimeUTC
			ctx, _ := testclock.UseTime(ctx, now)
			s1 := &State{PB: &prjpb.PState{
				PurgingCls: []*prjpb.PurgingCL{
					// expires later
					{Clid: 1, OperationId: "1", Deadline: timestamppb.New(now.Add(time.Minute))},
					// expires now, but due to grace period it'll stay here.
					{Clid: 2, OperationId: "2", Deadline: timestamppb.New(now)},
					// definitely expired.
					{Clid: 3, OperationId: "3", Deadline: timestamppb.New(now.Add(-time.Hour))},
				},
				// Components require PCLs, but in this test it doesn't matter.
				Components: []*prjpb.Component{
					{Clids: []int64{9}}, // for unconfusing indexes below.
					{Clids: []int64{1}},
					{Clids: []int64{2}, TriageRequired: true},
					{Clids: []int64{3}},
				},
			}}
			pb := backupPB(s1)

			Convey("Expires and removed", func() {
				s2, sideEffect, err := s1.OnPurgesCompleted(ctx, []*prjpb.PurgeCompleted{{OperationId: "1"}})
				So(err, ShouldBeNil)
				So(sideEffect, ShouldBeNil)
				So(s1.PB, ShouldResembleProto, pb)

				pb.PurgingCls = []*prjpb.PurgingCL{
					{Clid: 2, OperationId: "2", Deadline: timestamppb.New(now)},
				}
				pb.Components = []*prjpb.Component{
					pb.Components[0],
					{Clids: []int64{1}, TriageRequired: true},
					pb.Components[2],
					{Clids: []int64{3}, TriageRequired: true},
				}
				So(s2.PB, ShouldResembleProto, pb)
			})

			Convey("All removed", func() {
				s2, sideEffect, err := s1.OnPurgesCompleted(ctx, []*prjpb.PurgeCompleted{
					{OperationId: "3"},
					{OperationId: "1"},
					{OperationId: "5"},
					{OperationId: "2"},
				})
				So(err, ShouldBeNil)
				So(sideEffect, ShouldBeNil)
				So(s1.PB, ShouldResembleProto, pb)

				pb.PurgingCls = nil
				pb.Components = []*prjpb.Component{
					pb.Components[0],
					{Clids: []int64{1}, TriageRequired: true},
					pb.Components[2], // it was waiting for triage already
					{Clids: []int64{3}, TriageRequired: true},
				}
				So(s2.PB, ShouldResembleProto, pb)
			})

			Convey("Doesn't modify components if they are due re-repartition anyway", func() {
				s1.PB.RepartitionRequired = true
				pb := backupPB(s1)
				s2, sideEffect, err := s1.OnPurgesCompleted(ctx, []*prjpb.PurgeCompleted{
					{OperationId: "1"},
					{OperationId: "2"},
					{OperationId: "3"},
				})
				So(err, ShouldBeNil)
				So(sideEffect, ShouldBeNil)
				So(s1.PB, ShouldResembleProto, pb)

				pb.PurgingCls = nil
				So(s2.PB, ShouldResembleProto, pb)
			})
		})
	})
}

// backupPB returns a deep copy of State.PB for future assertion that State
// wasn't modified.
func backupPB(s *State) *prjpb.PState {
	ret := &prjpb.PState{}
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

func defaultPCL(cl *changelist.CL) *prjpb.PCL {
	p := &prjpb.PCL{
		Clid:               int64(cl.ID),
		Eversion:           int64(cl.EVersion),
		ConfigGroupIndexes: []int32{0},
		Status:             prjpb.PCL_OK,
		Deps:               cl.Snapshot.GetDeps(),
	}
	ci := cl.Snapshot.GetGerrit().GetInfo()
	if ci != nil {
		p.Trigger = trigger.Find(ci, &cfgpb.ConfigGroup{})
	}
	return p
}

func i64s(vs ...interface{}) []int64 {
	res := make([]int64, len(vs))
	for i, v := range vs {
		switch x := v.(type) {
		case int64:
			res[i] = x
		case common.CLID:
			res[i] = int64(x)
		case int:
			res[i] = int64(x)
		default:
			panic(fmt.Errorf("unknown type: %T %v", v, v))
		}
	}
	return res
}

func i64sorted(vs ...interface{}) []int64 {
	res := i64s(vs...)
	sort.Slice(res, func(i, j int) bool { return res[i] < res[j] })
	return res
}

func sortPCLs(vs []*prjpb.PCL) []*prjpb.PCL {
	sort.Slice(vs, func(i, j int) bool { return vs[i].GetClid() < vs[j].GetClid() })
	return vs
}

func mkClidsSet(cls map[int]*changelist.CL, ids ...int) clidsSet {
	res := make(clidsSet, len(ids))
	for _, id := range ids {
		res[cls[id].ID] = struct{}{}
	}
	return res
}

func sortByFirstCL(cs []*prjpb.Component) []*prjpb.Component {
	sort.Slice(cs, func(i, j int) bool { return cs[i].GetClids()[0] < cs[j].GetClids()[0] })
	return cs
}
