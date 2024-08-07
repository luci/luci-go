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
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap/gobmaptest"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCategorizeAndLoadActiveIntoPCLs(t *testing.T) {
	t.Parallel()

	Convey("loadActiveIntoPCLs and categorizeCLs work", t, func() {
		ct := ctest{
			lProject: "test",
			gHost:    "c-review.example.com",
		}
		ctx := ct.SetUp(t)

		cfg := &cfgpb.Config{}
		So(prototext.Unmarshal([]byte(cfgText1), cfg), ShouldBeNil)
		prjcfgtest.Create(ctx, ct.lProject, cfg)
		meta := prjcfgtest.MustExist(ctx, ct.lProject)
		gobmaptest.Update(ctx, ct.lProject)

		// Simulate existence of "test-b" project watching the same Gerrit host but
		// diff repo.
		const lProjectB = "test-b"
		cfgTextB := strings.ReplaceAll(cfgText1, "repo/a", "repo/b")
		cfgB := &cfgpb.Config{}
		So(prototext.Unmarshal([]byte(cfgTextB), cfgB), ShouldBeNil)
		prjcfgtest.Create(ctx, lProjectB, cfgB)
		gobmaptest.Update(ctx, lProjectB)

		cis := make(map[int]*gerritpb.ChangeInfo, 20)
		makeCI := func(i int, project string, cq int, extra ...gf.CIModifier) {
			mods := []gf.CIModifier{
				gf.Ref("refs/heads/main"),
				gf.Project(project),
				gf.Updated(ct.Clock.Now()),
			}
			if cq > 0 {
				mods = append(mods, gf.CQ(cq, ct.Clock.Now(), gf.U("user-1")))
			}
			mods = append(mods, extra...)
			cis[i] = gf.CI(i, mods...)
			ct.GFake.CreateChange(&gf.Change{Host: ct.gHost, ACLs: gf.ACLPublic(), Info: cis[i]})
		}
		makeStack := func(ids []int, project string, cq int) {
			for i, child := range ids {
				makeCI(child, project, cq)
				for _, parent := range ids[:i] {
					ct.GFake.SetDependsOn(ct.gHost, cis[child], cis[parent])
				}
			}
		}
		// Simulate the following CLs state in Gerrit:
		//   In this project:
		//     CQ+1
		//       1 <- 2       form a stack (2 depends on 1)
		//       3            depends on 2 via Cq-Depend.
		//     CQ+2
		//       4            standalone
		//       5 <- 6       form a stack (6 depends on 5)
		//       7 <- 8 <- 9  form a stack (9 depends on 7,8)
		//       13           CQ-Depend on 11 (diff project) and 12 (not existing).
		//   In another project:
		//     CQ+1
		//       10 <- 11     form a stack (11 depends on 10)
		makeStack([]int{1, 2}, "repo/a", +1)
		makeCI(3, "repo/a", +1, gf.Desc("T\n\nCq-Depend: 2"))
		makeStack([]int{7, 8, 9}, "repo/a", +2)
		makeStack([]int{5, 6}, "repo/a", +2)
		makeCI(4, "repo/a", +2)
		makeCI(13, "repo/a", +2, gf.Desc("T\n\nCq-Depend: 11,12"))
		makeStack([]int{10, 11}, "repo/b", +1)

		// Import into DS all CLs in their respective LUCI projects.
		// Do this in-order such that they match auto-assigned CLIDs by fake
		// Datastore as this helps test readability. Note that importing CL 13 would
		// create CL entity for dep #12 before creating CL 13th own entity.
		cls := make(map[int]*changelist.CL, 20)
		for i := 1; i < 14; i++ {
			if i == 12 {
				continue // skipped. will be done after 13
			}
			pr := ct.lProject
			if i == 10 || i == 11 {
				pr = lProjectB
			}
			cls[i] = ct.runCLUpdaterAs(ctx, int64(i), pr)
		}
		// This will get 404 from Gerrit.
		cls[12] = ct.runCLUpdater(ctx, 12)

		for i := 1; i < 14; i++ {
			// On in-memory DS fake, auto-generated IDs are 1,2, ...,
			// so by construction the following would hold:
			//   cls[i].ID == i
			// On real DS, emit mapping to assist in test debug.
			if cls[i].ID != common.CLID(i) {
				logging.Debugf(ctx, "cls[%d].ID = %d", i, cls[i].ID)
			}
		}

		run4 := &run.Run{
			ID:  common.RunID(ct.lProject + "/1-a"),
			CLs: common.CLIDs{cls[4].ID},
		}
		run56 := &run.Run{
			ID:  common.RunID(ct.lProject + "/56-bb"),
			CLs: common.CLIDs{cls[5].ID, cls[6].ID},
		}
		run789 := &run.Run{
			ID:  common.RunID(ct.lProject + "/789-ccc"),
			CLs: common.CLIDs{cls[9].ID, cls[7].ID, cls[8].ID},
		}
		So(datastore.Put(ctx, run4, run56, run789), ShouldBeNil)

		state := &State{PB: &prjpb.PState{
			LuciProject:         ct.lProject,
			Status:              prjpb.Status_STARTED,
			ConfigHash:          meta.Hash(),
			ConfigGroupNames:    []string{"g0", "g1"},
			RepartitionRequired: true,
		}}

		Convey("just categorization", func() {
			state.PB.Pcls = sortPCLs([]*prjpb.PCL{
				defaultPCL(cls[5]),
				defaultPCL(cls[6]),
				defaultPCL(cls[7]),
				defaultPCL(cls[8]),
				defaultPCL(cls[9]),
				{Clid: int64(cls[12].ID), Eversion: 1, Status: prjpb.PCL_UNKNOWN},
			})
			state.PB.Components = []*prjpb.Component{
				{
					Clids: i64sorted(cls[5].ID, cls[6].ID),
					Pruns: []*prjpb.PRun{prjpb.MakePRun(run56)},
				},
				// Simulate 9 previously not depending on 7, 8.
				{Clids: i64sorted(cls[7].ID, cls[8].ID)},
				{Clids: i64s(cls[9].ID)},
			}
			// 789 doesn't match any 1 component, even though 7,8,9 CLs are in PCLs.
			state.PB.CreatedPruns = []*prjpb.PRun{prjpb.MakePRun(run789)}
			pbBefore := backupPB(state)

			cat := state.categorizeCLs(ctx)
			So(state.loadActiveIntoPCLs(ctx, cat), ShouldBeNil)
			So(cat, ShouldResemble, &categorizedCLs{
				active:   mkClidsSet(cls, 5, 6, 7, 8, 9),
				deps:     common.CLIDsSet{},
				unused:   mkClidsSet(cls, 12),
				unloaded: common.CLIDsSet{},
			})
			So(state.PB, ShouldResembleProto, pbBefore)
		})

		Convey("loads unloaded dependencies and active CLs without recursion", func() {
			state.PB.Pcls = []*prjpb.PCL{
				defaultPCL(cls[3]), // depends on 2, which in turns depends on 1.
			}
			state.PB.CreatedPruns = []*prjpb.PRun{prjpb.MakePRun(run56)}
			pb := backupPB(state)

			cat := state.categorizeCLs(ctx)
			So(cat, ShouldResemble, &categorizedCLs{
				active:   mkClidsSet(cls, 3, 5, 6),
				deps:     mkClidsSet(cls, 2),
				unused:   common.CLIDsSet{},
				unloaded: mkClidsSet(cls, 2, 5, 6),
			})
			So(state.loadActiveIntoPCLs(ctx, cat), ShouldBeNil)
			So(cat, ShouldResemble, &categorizedCLs{
				active:   mkClidsSet(cls, 3, 2, 5, 6),
				deps:     mkClidsSet(cls, 1),
				unused:   common.CLIDsSet{},
				unloaded: mkClidsSet(cls, 1),
			})
			pb.Pcls = sortPCLs([]*prjpb.PCL{
				defaultPCL(cls[2]),
				defaultPCL(cls[3]),
				defaultPCL(cls[5]),
				defaultPCL(cls[6]),
			})
			So(state.PB, ShouldResembleProto, pb)
		})

		Convey("loads incomplete Run with unloaded deps", func() {
			// This case shouldn't normally happen in practice. This case simulates a
			// runStale created a while ago of just (11, 13), presumably when current
			// project had CL #11 in scope.
			// Now, 11 and 13 depend on 10 and 12, respectively, and 10 and 11 are no
			// longer watched by current project.
			runStale := &run.Run{
				ID:  common.RunID(ct.lProject + "/111-s"),
				CLs: common.CLIDs{cls[13].ID, cls[11].ID},
			}
			So(datastore.Put(ctx, runStale), ShouldBeNil)
			state.PB.CreatedPruns = []*prjpb.PRun{prjpb.MakePRun(runStale)}
			pb := backupPB(state)

			cat := state.categorizeCLs(ctx)
			So(cat, ShouldResemble, &categorizedCLs{
				active:   mkClidsSet(cls, 11, 13),
				deps:     common.CLIDsSet{},
				unused:   common.CLIDsSet{},
				unloaded: mkClidsSet(cls, 11, 13),
			})
			So(state.loadActiveIntoPCLs(ctx, cat), ShouldBeNil)
			So(cat, ShouldResemble, &categorizedCLs{
				active: mkClidsSet(cls, 11, 13),
				// 10 isn't in deps because this project has no visibility into CL 11.
				deps:     mkClidsSet(cls, 12),
				unused:   common.CLIDsSet{},
				unloaded: mkClidsSet(cls, 12),
			})
			pb.Pcls = sortPCLs([]*prjpb.PCL{
				defaultPCL(cls[13]),
				{
					Clid:     int64(cls[11].ID),
					Eversion: cls[11].EVersion,
					Status:   prjpb.PCL_UNWATCHED,
					Deps:     nil, // not visible to this project
				},
			})
			So(state.PB, ShouldResembleProto, pb)
		})

		Convey("loads incomplete Run with non-existent CLs", func() {
			// This case shouldn't happen in practice, but it can't be ruled out.
			// In order to incorporate just added .CreatedRun into State,
			// Run's CLs must have PCL entries.
			runStale := &run.Run{
				ID:  common.RunID(ct.lProject + "/404-s"),
				CLs: common.CLIDs{cls[4].ID, 404},
			}
			So(datastore.Put(ctx, runStale), ShouldBeNil)
			state.PB.CreatedPruns = []*prjpb.PRun{prjpb.MakePRun(runStale)}
			pb := backupPB(state)

			cat := state.categorizeCLs(ctx)
			So(cat, ShouldResemble, &categorizedCLs{
				active:   common.CLIDsSet{cls[4].ID: struct{}{}, 404: struct{}{}},
				deps:     common.CLIDsSet{},
				unused:   common.CLIDsSet{},
				unloaded: common.CLIDsSet{cls[4].ID: struct{}{}, 404: struct{}{}},
			})
			So(state.loadActiveIntoPCLs(ctx, cat), ShouldBeNil)
			So(cat, ShouldResemble, &categorizedCLs{
				active:   common.CLIDsSet{cls[4].ID: struct{}{}, 404: struct{}{}},
				deps:     common.CLIDsSet{},
				unused:   common.CLIDsSet{},
				unloaded: common.CLIDsSet{},
			})
			pb.Pcls = sortPCLs([]*prjpb.PCL{
				defaultPCL(cls[4]),
				{
					Clid:     404,
					Eversion: 0,
					Status:   prjpb.PCL_DELETED,
				},
			})
			So(state.PB, ShouldResembleProto, pb)
		})

		Convey("identifies submitted PCLs as unused if possible", func() {
			// Modify 1<-2 stack to have #1 submitted.
			ct.Clock.Add(time.Minute)
			cls[1] = ct.submitCL(ctx, 1)
			cis[1] = cls[1].Snapshot.GetGerrit().GetInfo()
			So(cis[1].Status, ShouldEqual, gerritpb.ChangeStatus_MERGED)

			state.PB.Pcls = []*prjpb.PCL{
				{
					Clid:      int64(cls[1].ID),
					Eversion:  cls[1].EVersion,
					Status:    prjpb.PCL_OK,
					Submitted: true,
				},
			}
			Convey("standalone submitted CL without a Run is unused", func() {
				cat := state.categorizeCLs(ctx)
				exp := &categorizedCLs{
					active:   common.CLIDsSet{},
					deps:     common.CLIDsSet{},
					unloaded: common.CLIDsSet{},
					unused:   mkClidsSet(cls, 1),
				}
				So(cat, ShouldResemble, exp)
				So(state.loadActiveIntoPCLs(ctx, cat), ShouldBeNil)
				So(cat, ShouldResemble, exp)
			})

			Convey("standalone submitted CL with a Run is active", func() {
				state.PB.Components = []*prjpb.Component{
					{
						Clids: i64s(cls[1].ID),
						Pruns: []*prjpb.PRun{
							{Clids: i64s(cls[1].ID), Id: "run1"},
						},
					},
				}
				cat := state.categorizeCLs(ctx)
				exp := &categorizedCLs{
					active:   mkClidsSet(cls, 1),
					deps:     common.CLIDsSet{},
					unloaded: common.CLIDsSet{},
					unused:   common.CLIDsSet{},
				}
				So(cat, ShouldResemble, exp)
				So(state.loadActiveIntoPCLs(ctx, cat), ShouldBeNil)
				So(cat, ShouldResemble, exp)
			})

			Convey("submitted dependent is neither active nor unused, but a dep", func() {
				triggers := trigger.Find(&trigger.FindInput{ChangeInfo: cis[2], ConfigGroup: cfg.ConfigGroups[0]})
				So(triggers.GetCqVoteTrigger(), ShouldNotBeNil)
				state.PB.Pcls = sortPCLs(append(state.PB.Pcls,
					&prjpb.PCL{
						Clid:               int64(cls[2].ID),
						Eversion:           cls[2].EVersion,
						Status:             prjpb.PCL_OK,
						Triggers:           triggers,
						ConfigGroupIndexes: []int32{0},
						Deps:               cls[2].Snapshot.GetDeps(),
					},
				))
				cat := state.categorizeCLs(ctx)
				exp := &categorizedCLs{
					active:   mkClidsSet(cls, 2),
					deps:     mkClidsSet(cls, 1),
					unloaded: common.CLIDsSet{},
					unused:   common.CLIDsSet{},
				}
				So(cat, ShouldResemble, exp)
				So(state.loadActiveIntoPCLs(ctx, cat), ShouldBeNil)
				So(cat, ShouldResemble, exp)
			})
		})

		Convey("prunes PCLs with expired triggers", func() {
			makePCL := func(i int, t time.Time, deps ...*changelist.Dep) *prjpb.PCL {
				return &prjpb.PCL{
					Clid:     int64(cls[i].ID),
					Eversion: 1,
					Status:   prjpb.PCL_OK,
					Deps:     deps,
					Triggers: &run.Triggers{CqVoteTrigger: &run.Trigger{
						GerritAccountId: 1,
						Mode:            string(run.DryRun),
						Time:            timestamppb.New(t),
					}},
				}
			}
			state.PB.Pcls = []*prjpb.PCL{
				makePCL(1, ct.Clock.Now().Add(-time.Minute), &changelist.Dep{Clid: int64(cls[4].ID)}),
				makePCL(2, ct.Clock.Now().Add(-common.MaxTriggerAge+time.Second)),
				makePCL(3, ct.Clock.Now().Add(-common.MaxTriggerAge)),
			}
			cat := state.categorizeCLs(ctx)
			So(cat, ShouldResemble, &categorizedCLs{
				active:   mkClidsSet(cls, 1, 2),
				deps:     mkClidsSet(cls, 4),
				unloaded: mkClidsSet(cls, 4),
				unused:   mkClidsSet(cls, 3),
			})

			Convey("and doesn't promote unloaded to active if trigger has expired", func() {
				// Keep CQ+2 vote, but make it timestamp really old.
				infoRef := cls[4].Snapshot.GetGerrit().GetInfo()
				infoRef.Labels = nil
				gf.CQ(2, ct.Clock.Now().Add(-common.MaxTriggerAge), gf.U("user-1"))(infoRef)
				So(datastore.Put(ctx, cls[4]), ShouldBeNil)

				So(state.loadActiveIntoPCLs(ctx, cat), ShouldBeNil)
				So(cat, ShouldResemble, &categorizedCLs{
					active:   mkClidsSet(cls, 1, 2),
					deps:     mkClidsSet(cls, 4),
					unloaded: common.CLIDsSet{},
					unused:   mkClidsSet(cls, 3),
				})
			})
		})

		Convey("noop", func() {
			cat := state.categorizeCLs(ctx)
			So(state.loadActiveIntoPCLs(ctx, cat), ShouldBeNil)
			So(cat, ShouldResemble, &categorizedCLs{
				active:   common.CLIDsSet{},
				deps:     common.CLIDsSet{},
				unused:   common.CLIDsSet{},
				unloaded: common.CLIDsSet{},
			})
		})
	})
}
