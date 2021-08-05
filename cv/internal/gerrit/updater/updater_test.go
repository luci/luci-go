// Copyright 2020 The LUCI Authors.
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

package updater

import (
	"context"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap/gobmaptest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSchedule(t *testing.T) {
	t.Parallel()

	Convey("Schedule works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "infra"
		const gHost = "chromium-review.example.com"

		// Each Schedule() moves clock forward by 1ns.
		// This ensures that SortByETA returns tasks in the same order as scheduled,
		// and makes tests deterministic w/o having to somehow sort individual proto
		// messages.
		const scheduleTimeIncrement = time.Nanosecond

		u := New(ct.TQDispatcher, nil, nil)

		do := func(t *RefreshGerritCL) []proto.Message {
			So(u.Schedule(ctx, t), ShouldBeNil)
			ct.Clock.Add(scheduleTimeIncrement)
			return ct.TQ.Tasks().SortByETA().Payloads()
		}

		doTrans := func(t *RefreshGerritCL) []proto.Message {
			err := datastore.RunInTransaction(ctx, func(tctx context.Context) error {
				So(u.Schedule(tctx, t), ShouldBeNil)
				return nil
			}, nil)
			So(err, ShouldBeNil)
			ct.Clock.Add(scheduleTimeIncrement)
			return ct.TQ.Tasks().SortByETA().Payloads()
		}

		doBatch := func(cls []*changelist.CL) []proto.Message {
			err := datastore.RunInTransaction(ctx, func(tctx context.Context) error {
				So(u.ScheduleBatch(tctx, lProject, cls), ShouldBeNil)
				return nil
			}, nil)
			So(err, ShouldBeNil)
			return ct.TQ.Tasks().SortByETA().Payloads()
		}

		taskMinimal := &RefreshGerritCL{
			LuciProject: lProject,
			Host:        gHost,
			Change:      123,
		}

		Convey("Minimal task", func() {
			So(do(taskMinimal), ShouldResembleProto, []proto.Message{taskMinimal})

			Convey("dedup works", func() {
				So(do(taskMinimal), ShouldResembleProto, []proto.Message{taskMinimal})

				Convey("but only within blindRefreshInterval", func() {
					ct.Clock.Add(blindRefreshInterval - time.Second) // still within
					So(do(taskMinimal), ShouldResembleProto, []proto.Message{taskMinimal})
					So(u.ScheduleDelayed(ctx, taskMinimal, time.Hour), ShouldBeNil) // definitely outside
					So(ct.TQ.Tasks().SortByETA().Payloads(), ShouldResembleProto, []proto.Message{taskMinimal, taskMinimal})
				})
			})

			Convey("transactional can't dedup, even with other transactional", func() {
				So(doTrans(taskMinimal), ShouldResembleProto, []proto.Message{taskMinimal, taskMinimal})
				So(doTrans(taskMinimal), ShouldResembleProto, []proto.Message{taskMinimal, taskMinimal, taskMinimal})
			})

			Convey("no dedup if different", func() {
				taskAnother := proto.Clone(taskMinimal).(*RefreshGerritCL)
				Convey("project", func() {
					taskAnother.LuciProject = lProject + "2"
					So(doTrans(taskAnother), ShouldResembleProto, []proto.Message{taskMinimal, taskAnother})
				})
				Convey("change", func() {
					taskAnother.Change++
					So(doTrans(taskAnother), ShouldResembleProto, []proto.Message{taskMinimal, taskAnother})
				})
				Convey("host", func() {
					taskAnother.Host = gHost + "2"
					So(doTrans(taskAnother), ShouldResembleProto, []proto.Message{taskMinimal, taskAnother})
				})
			})
		})

		Convey("CLID hint doesn't effect dedup", func() {
			taskWithHint := proto.Clone(taskMinimal).(*RefreshGerritCL)
			taskWithHint.ClidHint = 321
			do(taskMinimal)
			So(do(taskWithHint), ShouldResembleProto, []proto.Message{taskMinimal})
		})

		Convey("UpdateHint is de-duped with the same UpdatedHint, only", func() {
			// updatedHint logically has no relationship to now, but realistically it's usually
			// quite recent. So, use 1 hour ago.
			updatedHintEpoch := ct.Clock.Now().Add(-time.Hour)
			taskU0 := proto.Clone(taskMinimal).(*RefreshGerritCL)
			taskU0.UpdatedHint = timestamppb.New(updatedHintEpoch)
			taskU1 := proto.Clone(taskMinimal).(*RefreshGerritCL)
			taskU1.UpdatedHint = timestamppb.New(updatedHintEpoch.Add(time.Second))

			Convey("transactionally still no dedup", func() {
				So(doTrans(taskU0), ShouldResembleProto, []proto.Message{taskU0})
				So(doTrans(taskU0), ShouldResembleProto, []proto.Message{taskU0, taskU0})
			})

			Convey("only non-transactionally", func() {
				So(do(taskU0), ShouldResembleProto, []proto.Message{taskU0})
				So(do(taskU0), ShouldResembleProto, []proto.Message{taskU0})
				So(do(taskU1), ShouldResembleProto, []proto.Message{taskU0, taskU1})
				So(do(taskU1), ShouldResembleProto, []proto.Message{taskU0, taskU1})
				So(do(taskMinimal), ShouldResembleProto, []proto.Message{taskU0, taskU1, taskMinimal})
			})

			Convey("only within knownRefreshInterval", func() {
				So(do(taskU0), ShouldResembleProto, []proto.Message{taskU0})
				ct.Clock.Add(knownRefreshInterval)
				So(do(taskU0), ShouldResembleProto, []proto.Message{taskU0, taskU0})
				So(do(taskU0), ShouldResembleProto, []proto.Message{taskU0, taskU0})
				ct.Clock.Add(knownRefreshInterval)
				So(do(taskU0), ShouldResembleProto, []proto.Message{taskU0, taskU0, taskU0})
			})
		})

		Convey("BatchSchedule creates just one task within a transaction", func() {
			cls := []*changelist.CL{
				{ID: 1, ExternalID: changelist.MustGobID(gHost, 11)},
				{ID: 2, ExternalID: changelist.MustGobID(gHost, 12)},
				{ID: 3, ExternalID: changelist.MustGobID(gHost, 13)},
			}
			clMap := map[int64]*changelist.CL{
				11: cls[0],
				12: cls[1],
				13: cls[2],
			}
			So(doBatch(cls), ShouldHaveLength, 1)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(TaskClassBatch))
			So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, len(cls))
			for _, p := range ct.TQ.Tasks().Payloads() {
				t := p.(*RefreshGerritCL)
				cl := clMap[t.GetChange()]
				So(t, ShouldResembleProto, &RefreshGerritCL{
					Change:      t.Change,
					Host:        gHost,
					ClidHint:    int64(cl.ID),
					LuciProject: lProject,
				})
			}
		})

		Convey("BatchSchedule is just Schedule if there is just a single CL ", func() {
			So(doBatch([]*changelist.CL{
				{ID: 5, ExternalID: changelist.MustGobID(gHost, 15)},
			}), ShouldHaveLength, 1)
			So(ct.TQ.Tasks().Payloads(), ShouldResembleProto, []proto.Message{
				&RefreshGerritCL{
					Change:      15,
					Host:        gHost,
					ClidHint:    5,
					LuciProject: lProject,
				},
			})
		})
	})
}

func TestRelatedChangeProcessing(t *testing.T) {
	t.Parallel()

	Convey("setGitDeps works", t, func() {
		ctx := context.Background()
		f := fetcher{
			change: 111,
			host:   "host",
			toUpdate: updateFields{
				Snapshot: &changelist.Snapshot{Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{}}},
			},
		}

		Convey("No related changes", func() {
			f.setGitDeps(ctx, nil)
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldBeNil)

			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldBeNil)
		})

		Convey("Just itself", func() {
			// This isn't happening today, but CV shouldn't choke if Gerrit changes.
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(111, 3, 3), // No parents.
			})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldBeNil)

			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(111, 3, 3, "107_2"),
			})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldBeNil)
		})

		Convey("Has related, but no deps", func() {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(111, 3, 3, "107_2"),
				gf.RelatedChange(114, 1, 3, "111_3"),
				gf.RelatedChange(117, 2, 2, "114_1"),
			})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldBeNil)
		})

		Convey("Has related, but lacking this change crbug/1199471", func() {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(114, 1, 3, "111_3"),
				gf.RelatedChange(117, 2, 2, "114_1"),
			})
			So(f.toUpdate.Snapshot.GetErrors(), ShouldHaveLength, 1)
			So(f.toUpdate.Snapshot.GetErrors()[0].GetCorruptGerritMetadata(), ShouldContainSubstring, "https://crbug.com/1199471")
		})
		Convey("Has related, and several times itself", func() {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(111, 2, 2, "107_2"),
				gf.RelatedChange(111, 3, 3, "107_2"),
				gf.RelatedChange(114, 1, 3, "111_3"),
			})
			So(f.toUpdate.Snapshot.GetErrors()[0].GetCorruptGerritMetadata(), ShouldContainSubstring, "https://crbug.com/1199471")
		})

		Convey("1 parent", func() {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(107, 1, 3, "104_2"),
				gf.RelatedChange(111, 3, 3, "107_1"),
				gf.RelatedChange(117, 2, 2, "114_1"),
			})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldResembleProto, []*changelist.GerritGitDep{
				{Change: 107, Immediate: true},
			})
		})

		Convey("Diamond", func() {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(103, 2, 2),
				gf.RelatedChange(104, 2, 2, "103_2"),
				gf.RelatedChange(107, 1, 3, "104_2"),
				gf.RelatedChange(108, 1, 3, "104_2"),
				gf.RelatedChange(111, 3, 3, "107_1", "108_1"),
				gf.RelatedChange(114, 1, 3, "111_3"),
				gf.RelatedChange(117, 2, 2, "114_1"),
			})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldResembleProto, []*changelist.GerritGitDep{
				{Change: 107, Immediate: true},
				{Change: 108, Immediate: true},
				{Change: 104, Immediate: false},
				{Change: 103, Immediate: false},
			})
		})

		Convey("Same revision, different changes", func() {
			c104 := gf.RelatedChange(104, 1, 1, "103_2")
			c105 := gf.RelatedChange(105, 1, 1, "103_2")
			c105.GetCommit().Id = c104.GetCommit().GetId()
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(103, 2, 2),
				c104,
				c105, // should be ignored, somewhat arbitrarily.
				gf.RelatedChange(111, 3, 3, "104_1"),
			})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldResembleProto, []*changelist.GerritGitDep{
				{Change: 104, Immediate: true},
				{Change: 103, Immediate: false},
			})
		})

		Convey("2 parents which are the same change at different revisions", func() {
			// Actually happened, see https://crbug.com/988309.
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(104, 1, 2, "long-ago-merged1"),
				gf.RelatedChange(107, 1, 1, "long-ago-merged2"),
				gf.RelatedChange(104, 2, 2, "107_1"),
				gf.RelatedChange(111, 3, 3, "104_1", "104_2"),
			})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldResembleProto, []*changelist.GerritGitDep{
				{Change: 104, Immediate: true},
				{Change: 107, Immediate: false},
			})
		})
	})
}

func TestUpdateCLWorks(t *testing.T) {
	t.Parallel()

	Convey("Updating CL works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		const lProject = "proj-1"
		const gHost = "chromium-review.example.com"
		const gHostInternal = "internal-review.example.com"
		const gRepo = "depot_tools"

		prjcfgtest.Create(ctx, lProject, singleRepoConfig(gHost, gRepo))
		gobmaptest.Update(ctx, lProject)

		task := &RefreshGerritCL{
			LuciProject: lProject,
			Host:        gHost,
		}
		u := New(ct.TQDispatcher, ct.GFactory(), changelist.NewMutator(ct.TQDispatcher, &pmMock{}, &rmMock{}))

		assertScheduled := func(expected ...int) {
			var actual []int
			for _, p := range ct.TQ.Tasks().Payloads() {
				if t, ok := p.(*RefreshGerritCL); ok {
					actual = append(actual, int(t.GetChange()))
				}
			}
			sort.Ints(actual)
			sort.Ints(expected)
			So(actual, ShouldResemble, expected)
		}

		Convey("No access or permission denied", func() {
			Convey("after getting error from Gerrit", func() {
				assertAccessDeniedTemporary := func(change int) {
					cl := getCL(ctx, gHost, change)
					So(cl.Snapshot, ShouldBeNil)
					So(cl.ApplicableConfig, ShouldBeNil)
					So(cl.Access.GetByProject()[lProject], ShouldResembleProto, &changelist.Access_Project{
						NoAccess:     true,
						NoAccessTime: timestamppb.New(ct.Clock.Now().Add(noAccessGraceDuration)),
						UpdateTime:   timestamppb.New(ct.Clock.Now()),
					})
					So(cl.AccessKind(ctx, lProject), ShouldEqual, changelist.AccessDeniedProbably)
					assertScheduled(change)

					Convey("finalizes status after the grace duration", func() {
						ct.Clock.Add(noAccessGraceDuration + time.Second)
						So(u.Refresh(ctx, task), ShouldBeNil)

						clAfter := getCL(ctx, gHost, change)
						// NoAccessTime must remain unchanged.
						So(clAfter.Access.GetByProject()[lProject].GetNoAccessTime(), ShouldResembleProto,
							cl.Access.GetByProject()[lProject].GetNoAccessTime())
						// Hence, AccessDenied is now certain.
						So(clAfter.AccessKind(ctx, lProject), ShouldEqual, changelist.AccessDenied)
						// No new refresh tasks should be scheduled.
						assertScheduled(change) // same as before.
					})
				}
				Convey("HTTP 404", func() {
					task.Change = 404
					So(u.Refresh(ctx, task), ShouldBeNil)
					assertAccessDeniedTemporary(404)
				})
				Convey("HTTP 403", func() {
					task.Change = 403
					So(u.Refresh(ctx, task), ShouldBeNil)
					assertAccessDeniedTemporary(403)
				})
			})

			Convey("because CL isn't watched by the LUCI project", func() {
				verifyNoAccess := func() {
					task.Change = 1
					So(u.Refresh(ctx, task), ShouldBeNil)
					cl := getCL(ctx, gHost, 1)
					So(cl, ShouldNotBeNil)
					So(cl.Snapshot, ShouldBeNil)
					So(cl.ApplicableConfig, ShouldBeNil)
					So(cl.Access.GetByProject()[lProject], ShouldResembleProto, &changelist.Access_Project{
						NoAccess:     true,
						NoAccessTime: timestamppb.New(ct.Clock.Now()),
						UpdateTime:   timestamppb.New(ct.Clock.Now()),
					})
					So(cl.AccessKind(ctx, lProject), ShouldEqual, changelist.AccessDenied)
				}

				Convey("due to entirely unwatched Gerrit host", func() {
					// Add a CL readable to current LUCI project.
					ci := gf.CI(1, gf.Project(gRepo), gf.Ref("refs/heads/main"))
					ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), ci))
					client, err := ct.GFactory().MakeClient(ctx, gHost, lProject)
					So(err, ShouldBeNil)
					_, err = client.GetChange(ctx, &gerritpb.GetChangeRequest{Number: 1})
					So(err, ShouldBeNil)

					// But update LUCI project config to stop watching entire host.
					prjcfgtest.Update(ctx, lProject, singleRepoConfig("other-"+gHost, gRepo))
					gobmaptest.Update(ctx, lProject)

					verifyNoAccess()
				})

				Convey("due to unwatched repo", func() {
					ci := gf.CI(1, gf.Project("unwatched"), gf.Ref("refs/heads/main"))
					ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), ci))
					verifyNoAccess()
				})

				Convey("due to unwatched ref", func() {
					ci := gf.CI(1, gf.Project(gRepo), gf.Ref("refs/other/unwatched"))
					ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), ci))
					verifyNoAccess()
				})
			})
		})

		Convey("Unhandled Gerrit error results in no CL update", func() {
			ci500 := gf.CI(500, gf.Project(gRepo), gf.Ref("refs/heads/main"))
			Convey("fail to fetch change details", func() {
				ct.GFake.AddFrom(gf.WithCIs(gHost, err5xx, ci500))
				task.Change = 500
				So(u.Refresh(ctx, task), ShouldErrLike, "boo")
				cl := getCL(ctx, gHost, 500)
				So(cl, ShouldBeNil)
			})

			Convey("fail to get filelist", func() {
				ct.GFake.AddFrom(gf.WithCIs(gHost, okThenErr5xx(), ci500))
				task.Change = 500
				So(u.Refresh(ctx, task), ShouldErrLike, "boo")
				cl := getCL(ctx, gHost, 500)
				So(cl, ShouldBeNil)
			})
		})

		Convey("CL hint must actually exist", func() {
			task.Change = 123
			task.ClidHint = 848484881
			So(u.Refresh(ctx, task), ShouldErrLike, "clidHint 848484881 doesn't refer to an existing CL")
		})

		Convey("Fetch for the first time", func() {
			ci := gf.CI(123, gf.Project(gRepo), gf.Ref("refs/heads/main"),
				gf.Files("a.cpp", "c/b.py"), gf.Desc("T.\n\nCq-Depend: 101"))
			ciParent := gf.CI(122, gf.Desc("Z\n\nCq-Depend: must-be-ignored:47"))
			ciGrandpa := gf.CI(121, gf.Desc("Z\n\nCq-Depend: must-be-ignored:46"))
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), ci, ciParent, ciGrandpa))
			ct.GFake.SetDependsOn(gHost, ci, ciParent)
			ct.GFake.SetDependsOn(gHost, ciParent, ciGrandpa)

			task.Change = 123
			So(u.Refresh(ctx, task), ShouldBeNil)
			cl := getCL(ctx, gHost, 123)
			So(cl.AccessKind(ctx, lProject), ShouldEqual, changelist.AccessGranted)
			So(cl.Snapshot.GetGerrit().GetHost(), ShouldEqual, gHost)
			So(cl.Snapshot.GetGerrit().Info.GetProject(), ShouldEqual, gRepo)
			So(cl.Snapshot.GetGerrit().Info.GetRef(), ShouldEqual, "refs/heads/main")
			So(cl.Snapshot.GetGerrit().GetFiles(), ShouldResemble, []string{"a.cpp", "c/b.py"})
			So(cl.Snapshot.GetLuciProject(), ShouldEqual, lProject)
			So(cl.Snapshot.GetExternalUpdateTime(), ShouldResembleProto, ci.GetUpdated())
			So(cl.Snapshot.GetGerrit().GetGitDeps(), ShouldResembleProto,
				[]*changelist.GerritGitDep{
					{Change: 122, Immediate: true},
					{Change: 121},
				})
			So(cl.Snapshot.GetGerrit().GetSoftDeps(), ShouldResembleProto,
				[]*changelist.GerritSoftDep{
					{Change: 101, Host: gHost},
				})

			// Each of the dep should have an existing CL + a task schedule.
			expectedDeps := make([]*changelist.Dep, 0, 3)
			for _, gChange := range []int{122, 121, 101} {
				dep := getCL(ctx, gHost, gChange)
				So(dep, ShouldNotBeNil)
				So(dep.AccessKind(ctx, lProject), ShouldEqual, changelist.AccessUnknown)
				depKind := changelist.DepKind_SOFT
				if gChange == 122 {
					depKind = changelist.DepKind_HARD
				}
				expectedDeps = append(expectedDeps, &changelist.Dep{Clid: int64(dep.ID), Kind: depKind})
			}
			sort.Slice(expectedDeps, func(i, j int) bool {
				return expectedDeps[i].GetClid() < expectedDeps[j].GetClid()
			})
			So(cl.Snapshot.GetDeps(), ShouldResembleProto, expectedDeps)
			expectedTasks := []*RefreshGerritCL{
				{
					LuciProject: lProject,
					Host:        gHost,
					Change:      101,
					ClidHint:    int64(getCL(ctx, gHost, 101).ID),
				},
				{
					LuciProject: lProject,
					Host:        gHost,
					Change:      121,
					ClidHint:    int64(getCL(ctx, gHost, 121).ID),
				},
				{
					LuciProject: lProject,
					Host:        gHost,
					Change:      122,
					ClidHint:    int64(getCL(ctx, gHost, 122).ID),
				},
			}
			So(sortedRefreshTasks(ct), ShouldResembleProto, expectedTasks)

			// Simulate Gerrit change being updated with +1s timestamp.
			ct.GFake.MutateChange(gHost, 123, func(c *gf.Change) {
				c.Info.Updated.Seconds++
			})

			Convey("Skips update with updatedHint", func() {
				task.UpdatedHint = cl.Snapshot.GetExternalUpdateTime()
				So(u.Refresh(ctx, task), ShouldBeNil)
				So(getCL(ctx, gHost, 123).EVersion, ShouldEqual, cl.EVersion)
			})

			Convey("Updates snapshots explicitly marked outdated", func() {
				task.UpdatedHint = cl.Snapshot.GetExternalUpdateTime()
				cl.Snapshot.Outdated = &changelist.Snapshot_Outdated{}
				So(datastore.Put(ctx, cl), ShouldBeNil)
				So(u.Refresh(ctx, task), ShouldBeNil)
				So(getCL(ctx, gHost, 123).EVersion, ShouldEqual, cl.EVersion+1)
			})

			Convey("Don't update iff fetched less recent than updatedHint ", func() {
				// Set expectation that Gerrit serves change with >=+1m timestamp.
				task.UpdatedHint = timestamppb.New(
					cl.Snapshot.GetExternalUpdateTime().AsTime().Add(time.Minute),
				)
				err := u.Refresh(ctx, task)
				So(err, ShouldErrLike, "stale Gerrit data")
				So(transient.Tag.In(err), ShouldBeTrue)
				So(getCL(ctx, gHost, 123).EVersion, ShouldEqual, cl.EVersion)
			})

			Convey("Heeds updatedHint and updates the CL", func() {
				// Set expectation that Gerrit serves change with >=+1ms timestamp.
				task.UpdatedHint = timestamppb.New(
					cl.Snapshot.GetExternalUpdateTime().AsTime().Add(time.Millisecond),
				)
				ct.GFake.MutateChange(gHost, 123, func(c *gf.Change) {
					// Only ChangeInfo but not ListFiles and GetRelatedChanges RPCs should
					// be called. So, ensure 2+ RPCs return 5xx.
					// TODO(crbug/1227384): re-enable okThenErr5xx and remove file change.
					// c.ACLs = okThenErr5xx()
					gf.Files("crbug/1227384/detected.diff")(c.Info)
				})
				So(u.Refresh(ctx, task), ShouldBeNil)
				cl2 := getCL(ctx, gHost, 123)
				So(cl2.EVersion, ShouldEqual, cl.EVersion+1)
				So(cl2.Snapshot.GetExternalUpdateTime().AsTime(), ShouldResemble,
					cl.Snapshot.GetExternalUpdateTime().AsTime().Add(time.Second))

				Convey("New revision doesn't re-use files & related changes", func() {
					// Stay within the same blindRefreshInterval for de-duping refresh
					// tasks of dependencies.
					ct.Clock.Add(blindRefreshInterval - 2*time.Second)
					ct.GFake.MutateChange(gHost, 123, func(c *gf.Change) {
						c.ACLs = gf.ACLPublic()
						// Simulate new patchset which no longer has GerritGitDeps.
						gf.PS(10)(c.Info)
						gf.Files("z.zz")(c.Info)
						// 101 is from before, internal:477 is new.
						gf.Desc("T\n\nCq-Depend: 101,internal:477")(c.Info)
						gf.Updated(ct.Clock.Now())(c.Info)
					})

					task.UpdatedHint = nil
					So(u.Refresh(ctx, task), ShouldBeNil)
					cl3 := getCL(ctx, gHost, 123)
					So(cl3.EVersion, ShouldEqual, cl2.EVersion+1)
					So(cl3.Snapshot.GetExternalUpdateTime().AsTime(), ShouldResemble, ct.Clock.Now().UTC())
					So(cl3.Snapshot.GetGerrit().GetFiles(), ShouldResemble, []string{"z.zz"})
					So(cl3.Snapshot.GetGerrit().GetGitDeps(), ShouldBeNil)
					So(cl3.Snapshot.GetGerrit().GetSoftDeps(), ShouldResembleProto,
						[]*changelist.GerritSoftDep{
							{Change: 101, Host: gHost},
							{Change: 477, Host: gHostInternal},
						})
					// For each dep, a task should have been created, but 101 should have
					// been de-duped with an earlier one. So, only 1 new task for 477:
					So(sortedRefreshTasks(ct), ShouldResembleProto, append(expectedTasks,
						&RefreshGerritCL{
							LuciProject: lProject,
							Host:        gHostInternal,
							Change:      477,
							ClidHint:    int64(getCL(ctx, gHostInternal, 477).ID),
						},
					))
				})
			})

			Convey("No longer watched", func() {
				ct.Clock.Add(time.Second)
				prjcfgtest.Update(ctx, lProject, singleRepoConfig(gHost, "another/repo"))
				gobmaptest.Update(ctx, lProject)
				So(u.Refresh(ctx, task), ShouldBeNil)
				cl2 := getCL(ctx, gHost, 123)
				So(cl2.AccessKind(ctx, lProject), ShouldEqual, changelist.AccessDenied)
				So(cl2.EVersion, ShouldEqual, cl.EVersion+1)
				// Snapshot is preserved in case this is temporal misconfiguration.
				So(cl2.Snapshot, ShouldResembleProto, cl.Snapshot)
			})

			Convey("Watched by a diff project", func() {
				ct.Clock.Add(time.Second)
				const lProject2 = "proj-2"
				prjcfgtest.Update(ctx, lProject, singleRepoConfig(gHost, "another repo"))
				prjcfgtest.Create(ctx, lProject2, singleRepoConfig(gHost, gRepo))
				gobmaptest.Update(ctx, lProject)
				gobmaptest.Update(ctx, lProject2)

				// Use a hint that'd normally prevent an update.
				task.UpdatedHint = cl.Snapshot.GetExternalUpdateTime()
				task.LuciProject = lProject2

				Convey("with access", func() {
					So(u.Refresh(ctx, task), ShouldBeNil)
					cl2 := getCL(ctx, gHost, 123)
					So(cl2.EVersion, ShouldEqual, cl.EVersion+1)
					So(cl2.Snapshot.GetLuciProject(), ShouldEqual, lProject2)
					So(cl2.Snapshot.GetExternalUpdateTime(), ShouldResemble, ct.GFake.GetChange(gHost, 123).Info.GetUpdated())
					So(cl2.AccessKind(ctx, lProject), ShouldEqual, changelist.AccessDenied)
					So(cl2.AccessKind(ctx, lProject2), ShouldEqual, changelist.AccessGranted)
				})

				Convey("without access", func() {
					ct.GFake.MutateChange(gHost, 123, func(c *gf.Change) {
						c.ACLs = gf.ACLRestricted("neither-lProject-nor-lProject2")
					})
					So(u.Refresh(ctx, task), ShouldBeNil)
					cl2 := getCL(ctx, gHost, 123)
					So(cl2.EVersion, ShouldEqual, cl.EVersion+1)
					// Snapshot is kept as is, incl. binding to old project and its ExternalUpdateTime.
					So(cl2.Snapshot, ShouldResembleProto, cl.Snapshot)
					So(cl2.Snapshot.GetLuciProject(), ShouldResemble, lProject)
					// TODO(tandrii): refactor CL access info to ensure that 403/404 for
					// the first time (in a context of a specific LUCI project) is
					// AccessDeniedProbably.
					So(cl2.AccessKind(ctx, lProject2), ShouldEqual, changelist.AccessDenied)
				})
			})
		})

		Convey("Fetch dep after bare CL was created", func() {
			eid, err := changelist.GobID(gHost, 101)
			So(err, ShouldBeNil)
			cl := eid.MustCreateIfNotExists(ctx)
			So(cl.EVersion, ShouldEqual, 1)

			ci := gf.CI(101, gf.Project(gRepo), gf.Ref("refs/heads/main"))
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), ci))
			task.Change = 101
			task.ClidHint = int64(cl.ID)
			So(u.Refresh(ctx, task), ShouldBeNil)

			cl2 := getCL(ctx, gHost, 101)
			So(cl2.EVersion, ShouldEqual, 2)
			changelist.RemoveUnusedGerritInfo(ci)
			So(cl2.Snapshot.GetGerrit().GetInfo(), ShouldResembleProto, ci)
		})

		Convey("Handles New -> Abandon -> Restored transitions correctly", func() {
			task.Change = 123

			// Start with a NEW Gerrit change.
			ci := gf.CI(
				123, gf.Project(gRepo), gf.Ref("refs/heads/main"),
				gf.Files("a.cpp", "c/b.py"),
				gf.Desc("T.\n\nCq-Depend: 101"))
			ciParent := gf.CI(122)
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), ci, ciParent))
			ct.GFake.SetDependsOn(gHost, ci, ciParent)

			So(u.Refresh(ctx, task), ShouldBeNil)
			v1 := getCL(ctx, gHost, 123)
			So(v1.Snapshot.GetGerrit().GetInfo().GetStatus(), ShouldEqual, gerritpb.ChangeStatus_NEW)
			So(v1.Snapshot.GetGerrit().GetFiles(), ShouldResemble, []string{"a.cpp", "c/b.py"})
			So(v1.Snapshot.GetGerrit().GetSoftDeps(), ShouldResembleProto,
				[]*changelist.GerritSoftDep{{Change: 101, Host: gHost}})
			So(v1.Snapshot.GetGerrit().GetGitDeps(), ShouldResembleProto,
				[]*changelist.GerritGitDep{{Change: 122, Immediate: true}})

			// Abandon the Gerrit change.
			ct.Clock.Add(time.Minute)
			ct.GFake.MutateChange(gHost, 123, func(c *gf.Change) {
				c.Info.Status = gerritpb.ChangeStatus_ABANDONED
				c.Info.Updated = timestamppb.New(ct.Clock.Now())
			})
			So(u.Refresh(ctx, task), ShouldBeNil)
			v2 := getCL(ctx, gHost, 123)
			So(v2.Snapshot.GetGerrit().GetInfo().GetStatus(), ShouldEqual, gerritpb.ChangeStatus_ABANDONED)
			// Files and deps don't have to be set as CV doesn't work with abandoned such CLs.

			// Restore the Gerrit change.
			ct.Clock.Add(time.Minute)
			ct.GFake.MutateChange(gHost, 123, func(c *gf.Change) {
				c.Info.Status = gerritpb.ChangeStatus_NEW
				c.Info.Updated = timestamppb.New(ct.Clock.Now())
			})
			So(u.Refresh(ctx, task), ShouldBeNil)
			v3 := getCL(ctx, gHost, 123)
			So(v3.Snapshot.GetGerrit().GetInfo().GetStatus(), ShouldEqual, gerritpb.ChangeStatus_NEW)
			So(v3.Snapshot.GetGerrit().GetFiles(), ShouldResemble, v1.Snapshot.GetGerrit().GetFiles())
			So(v3.Snapshot.GetGerrit().GetSoftDeps(), ShouldResembleProto, v1.Snapshot.GetGerrit().GetSoftDeps())
			So(v3.Snapshot.GetGerrit().GetGitDeps(), ShouldResembleProto, v1.Snapshot.GetGerrit().GetGitDeps())
		})
	})
}

func getCL(ctx context.Context, host string, change int) *changelist.CL {
	eid, err := changelist.GobID(host, int64(change))
	So(err, ShouldBeNil)
	cl, err := eid.Get(ctx)
	if err == datastore.ErrNoSuchEntity {
		return nil
	}
	So(err, ShouldBeNil)
	return cl
}

func singleRepoConfig(gHost string, gRepos ...string) *cfgpb.Config {
	projects := make([]*cfgpb.ConfigGroup_Gerrit_Project, len(gRepos))
	for i, gRepo := range gRepos {
		projects[i] = &cfgpb.ConfigGroup_Gerrit_Project{
			Name:      gRepo,
			RefRegexp: []string{"refs/heads/main"},
		}
	}
	return &cfgpb.Config{
		ConfigGroups: []*cfgpb.ConfigGroup{
			{
				Name: "main",
				Gerrit: []*cfgpb.ConfigGroup_Gerrit{
					{
						Url:      "https://" + gHost + "/",
						Projects: projects,
					},
				},
			},
		},
	}
}

func err5xx(gf.Operation, string) *status.Status {
	return status.New(codes.Internal, "boo")
}

func okThenErr5xx() gf.AccessCheck {
	calls := int32(0)
	return func(o gf.Operation, p string) *status.Status {
		if atomic.AddInt32(&calls, 1) == 1 {
			return status.New(codes.OK, "")
		} else {
			return err5xx(o, p)
		}
	}
}

func sortedRefreshTasks(ct cvtesting.Test) []*RefreshGerritCL {
	ret := make([]*RefreshGerritCL, 0, len(ct.TQ.Tasks().Payloads()))
	for _, m := range ct.TQ.Tasks().Payloads() {
		v, ok := m.(*RefreshGerritCL)
		if ok {
			ret = append(ret, v)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].less(ret[j]) })
	return ret
}

func (l *RefreshGerritCL) less(r *RefreshGerritCL) bool {
	switch {
	case l.GetHost() < r.GetHost():
		return true
	case l.GetHost() > r.GetHost():
		return false
	case l.GetChange() < r.GetChange():
		return true
	case l.GetChange() > r.GetChange():
		return false
	case l.GetLuciProject() < r.GetLuciProject():
		return true
	case l.GetLuciProject() > r.GetLuciProject():
		return false
	default:
		return l.GetUpdatedHint().AsTime().Before(r.GetUpdatedHint().AsTime())
	}
}

type pmMock struct {
}

func (*pmMock) NotifyCLsUpdated(ctx context.Context, project string, cls *changelist.CLUpdatedEvents) error {
	return nil
}

type rmMock struct {
}

func (*rmMock) NotifyCLsUpdated(ctx context.Context, rid common.RunID, cls *changelist.CLUpdatedEvents) error {
	return nil
}
