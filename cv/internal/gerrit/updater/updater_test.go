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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap/gobmaptest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestUpdaterBackend(t *testing.T) {
	t.Parallel()

	Convey("updaterBackend methods work, except Fetch()", t, func() {
		// Fetch() is covered in TestUpdaterBackendFetch.
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		gu := &updaterBackend{
			clUpdater: changelist.NewUpdater(ct.TQDispatcher, changelist.NewMutator(ct.TQDispatcher, &pmMock{}, &rmMock{})),
			gFactory:  ct.GFactory(),
		}

		Convey("Kind", func() {
			So(gu.Kind(), ShouldEqual, "gerrit")
		})
		Convey("TQErrorSpec", func() {
			tqSpec := gu.TQErrorSpec()
			err := errors.Annotate(gerrit.ErrStaleData, "retry, don't ignore").Err()
			So(tq.Ignore.In(tqSpec.Error(ctx, err)), ShouldBeFalse)
		})
		Convey("LookupApplicableConfig", func() {
			const gHost = "x-review.example.com"
			prjcfgtest.Create(ctx, "luci-project-x", singleRepoConfig(gHost, "x"))
			gobmaptest.Update(ctx, "luci-project-x")
			xConfigGroupID := string(prjcfgtest.MustExist(ctx, "luci-project-x").ConfigGroupIDs[0])

			// Setup valid CL snapshot; it'll be modified in tests below.
			cl := &changelist.CL{
				ID: 11111,
				Snapshot: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: &gerritpb.ChangeInfo{
							Number:  456,
							Ref:     "refs/heads/main",
							Project: "x",
						},
					}},
				},
			}

			Convey("Happy path", func() {
				acfg, err := gu.LookupApplicableConfig(ctx, cl)
				So(err, ShouldBeNil)
				So(acfg, ShouldResembleProto, &changelist.ApplicableConfig{
					Projects: []*changelist.ApplicableConfig_Project{
						{Name: "luci-project-x", ConfigGroupIds: []string{string(xConfigGroupID)}},
					},
				})
			})

			Convey("CL without Gerrit Snapshot can't be decided on", func() {
				cl.Snapshot.Kind = nil
				acfg, err := gu.LookupApplicableConfig(ctx, cl)
				So(err, ShouldBeNil)
				So(acfg, ShouldBeNil)
			})

			Convey("No watching projects", func() {
				cl.Snapshot.GetGerrit().GetInfo().Ref = "ref/un/watched"
				acfg, err := gu.LookupApplicableConfig(ctx, cl)
				So(err, ShouldBeNil)
				// Must be empty, but not nil per updaterBackend interface contract.
				So(acfg, ShouldNotBeNil)
				So(acfg, ShouldResembleProto, &changelist.ApplicableConfig{})
			})

			Convey("Works with >1 LUCI project watching the same CL", func() {
				prjcfgtest.Create(ctx, "luci-project-dupe", singleRepoConfig(gHost, "x"))
				gobmaptest.Update(ctx, "luci-project-dupe")
				acfg, err := gu.LookupApplicableConfig(ctx, cl)
				So(err, ShouldBeNil)
				So(acfg.GetProjects(), ShouldHaveLength, 2)
			})
		})
	})
}

func TestUpdateCLWorks(t *testing.T) {
	t.Parallel()

	Convey("Updating CL works", t, func() {
		// TODO(tandrii): simplify this test drastically by focusing on the
		// functionality that the `updaterBackend` implements, as the rest is
		// already covered by changelist.Updater tests.
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		const lProject = "proj-1"
		const gHost = "chromium-review.example.com"
		const gHostInternal = "internal-review.example.com"
		const gRepo = "depot_tools"

		prjcfgtest.Create(ctx, lProject, singleRepoConfig(gHost, gRepo))
		gobmaptest.Update(ctx, lProject)

		gu := &updaterBackend{
			clUpdater: changelist.NewUpdater(ct.TQDispatcher, changelist.NewMutator(ct.TQDispatcher, &pmMock{}, &rmMock{})),
			gFactory:  ct.GFactory(),
		}
		gu.clUpdater.RegisterBackend(gu)
		task := &changelist.UpdateCLTask{
			LuciProject: lProject,
			// Other fields set in individual tests below.
		}

		assertScheduled := func(expected ...int) {
			var actual []int
			for _, p := range ct.TQ.Tasks().Payloads() {
				if t, ok := p.(*changelist.UpdateCLTask); ok {
					_, changeNumber, err := changelist.ExternalID(t.GetExternalId()).ParseGobID()
					So(err, ShouldBeNil)
					actual = append(actual, int(changeNumber))
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
						So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)

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
					task.ExternalId = string(changelist.MustGobID(gHost, 404))
					So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)
					assertAccessDeniedTemporary(404)
				})
				Convey("HTTP 403", func() {
					task.ExternalId = string(changelist.MustGobID(gHost, 403))
					So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)
					assertAccessDeniedTemporary(403)
				})
			})

			Convey("because CL isn't watched by the LUCI project", func() {
				verifyNoAccess := func() {
					task.ExternalId = string(changelist.MustGobID(gHost, 1))
					So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)
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
				task.ExternalId = string(changelist.MustGobID(gHost, 500))
				So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldErrLike, "boo")
				cl := getCL(ctx, gHost, 500)
				So(cl, ShouldBeNil)
			})

			Convey("fail to get filelist", func() {
				ct.GFake.AddFrom(gf.WithCIs(gHost, okThenErr5xx(), ci500))
				task.ExternalId = string(changelist.MustGobID(gHost, 500))
				So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldErrLike, "boo")
				cl := getCL(ctx, gHost, 500)
				So(cl, ShouldBeNil)
			})
		})

		Convey("CL hint must actually exist", func() {
			task.ExternalId = string(changelist.MustGobID(gHost, 123))
			task.Id = 848484881
			So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldErrLike, "doesn't exist")
		})

		Convey("Fetch for the first time", func() {
			ci := gf.CI(
				123, gf.Project(gRepo), gf.Ref("refs/heads/main"),
				gf.Files("a.cpp", "c/b.py"),
				gf.Desc(strings.TrimSpace(`
Title.

Second paragraph.
May have.
Several lines.
AND_A_TAG=with some value
AND_A_TAG=with the second value

Gerrit-Or-Git: footers are here
Gerrit-or-Git: can be repeated
Cq-Depend: 101
				`)),
			)
			ciParent := gf.CI(122, gf.Desc("Z\n\nCq-Depend: must-be-ignored:47"))
			ciGrandpa := gf.CI(121, gf.Desc("Z\n\nCq-Depend: must-be-ignored:46"))
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), ci, ciParent, ciGrandpa))
			ct.GFake.SetDependsOn(gHost, ci, ciParent)
			ct.GFake.SetDependsOn(gHost, ciParent, ciGrandpa)

			task.ExternalId = string(changelist.MustGobID(gHost, 123))
			So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)
			cl := getCL(ctx, gHost, 123)
			So(cl.AccessKind(ctx, lProject), ShouldEqual, changelist.AccessGranted)
			So(cl.Snapshot.GetGerrit().GetHost(), ShouldEqual, gHost)
			So(cl.Snapshot.GetGerrit().Info.GetProject(), ShouldEqual, gRepo)
			So(cl.Snapshot.GetGerrit().Info.GetRef(), ShouldEqual, "refs/heads/main")
			So(cl.Snapshot.GetGerrit().GetFiles(), ShouldResemble, []string{"a.cpp", "c/b.py"})
			So(cl.Snapshot.GetLuciProject(), ShouldEqual, lProject)
			So(cl.Snapshot.GetExternalUpdateTime(), ShouldResembleProto, ci.GetUpdated())
			So(cl.Snapshot.GetMetadata(), ShouldResembleProto, []*changelist.StringPair{
				{Key: "AND_A_TAG", Value: "with the second value"},
				{Key: "AND_A_TAG", Value: "with some value"},
				{Key: "Cq-Depend", Value: "101"},
				{Key: "Gerrit-Or-Git", Value: "can be repeated"},
				{Key: "Gerrit-Or-Git", Value: "footers are here"},
			})
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
				depKind := changelist.DepKind_HARD
				if gChange == 101 {
					depKind = changelist.DepKind_SOFT
				}
				expectedDeps = append(expectedDeps, &changelist.Dep{Clid: int64(dep.ID), Kind: depKind})
			}
			sort.Slice(expectedDeps, func(i, j int) bool {
				return expectedDeps[i].GetClid() < expectedDeps[j].GetClid()
			})
			So(cl.Snapshot.GetDeps(), ShouldResembleProto, expectedDeps)
			expectedTasks := []*changelist.UpdateCLTask{
				{
					LuciProject: lProject,
					ExternalId:  string(changelist.MustGobID(gHost, 101)),
					Id:          int64(getCL(ctx, gHost, 101).ID),
				},
				{
					LuciProject: lProject,
					ExternalId:  string(changelist.MustGobID(gHost, 121)),
					Id:          int64(getCL(ctx, gHost, 121).ID),
				},
				{
					LuciProject: lProject,
					ExternalId:  string(changelist.MustGobID(gHost, 122)),
					Id:          int64(getCL(ctx, gHost, 122).ID),
				},
			}
			So(sortedRefreshTasks(ct), ShouldResembleProto, expectedTasks)

			// Simulate Gerrit change being updated with +1s timestamp.
			ct.GFake.MutateChange(gHost, 123, func(c *gf.Change) {
				c.Info.Updated.Seconds++
			})

			Convey("Skips update with updatedHint", func() {
				task.UpdatedHint = cl.Snapshot.GetExternalUpdateTime()
				So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)
				So(getCL(ctx, gHost, 123).EVersion, ShouldEqual, cl.EVersion)
			})

			Convey("Updates snapshots explicitly marked outdated", func() {
				task.UpdatedHint = cl.Snapshot.GetExternalUpdateTime()
				cl.Snapshot.Outdated = &changelist.Snapshot_Outdated{}
				So(datastore.Put(ctx, cl), ShouldBeNil)
				So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)
				So(getCL(ctx, gHost, 123).EVersion, ShouldEqual, cl.EVersion+1)
			})

			Convey("Don't update iff fetched less recent than updatedHint ", func() {
				// Set expectation that Gerrit serves change with >=+1m timestamp.
				task.UpdatedHint = timestamppb.New(
					cl.Snapshot.GetExternalUpdateTime().AsTime().Add(time.Minute),
				)
				err := gu.clUpdater.TestingForceUpdate(ctx, task)
				So(errors.Contains(err, gerrit.ErrStaleData), ShouldBeTrue)
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
				So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)
				cl2 := getCL(ctx, gHost, 123)
				So(cl2.EVersion, ShouldEqual, cl.EVersion+1)
				So(cl2.Snapshot.GetExternalUpdateTime().AsTime(), ShouldResemble,
					cl.Snapshot.GetExternalUpdateTime().AsTime().Add(time.Second))

				Convey("New revision doesn't re-use files & related changes", func() {
					// Stay within the same blindRefreshInterval for de-duping refresh
					// tasks of dependencies.
					ct.Clock.Add(changelist.BlindRefreshInterval - 2*time.Second)
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
					So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)
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
						&changelist.UpdateCLTask{
							LuciProject: lProject,
							ExternalId:  string(changelist.MustGobID(gHostInternal, 477)),
							Id:          int64(getCL(ctx, gHostInternal, 477).ID),
						},
					))
				})
			})

			Convey("No longer watched", func() {
				ct.Clock.Add(time.Second)
				prjcfgtest.Update(ctx, lProject, singleRepoConfig(gHost, "another/repo"))
				gobmaptest.Update(ctx, lProject)
				So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)
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
					So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)
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
					So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)
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
			task.ExternalId = string(changelist.MustGobID(gHost, 101))
			task.Id = int64(cl.ID)
			So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)

			cl2 := getCL(ctx, gHost, 101)
			So(cl2.EVersion, ShouldEqual, 2)
			changelist.RemoveUnusedGerritInfo(ci)
			So(cl2.Snapshot.GetGerrit().GetInfo(), ShouldResembleProto, ci)
		})

		Convey("Handles New -> Abandon -> Restored transitions correctly", func() {
			task.ExternalId = string(changelist.MustGobID(gHost, 123))

			// Start with a NEW Gerrit change.
			ci := gf.CI(
				123, gf.Project(gRepo), gf.Ref("refs/heads/main"),
				gf.Files("a.cpp", "c/b.py"),
				gf.Desc("T.\n\nCq-Depend: 101"))
			ciParent := gf.CI(122)
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), ci, ciParent))
			ct.GFake.SetDependsOn(gHost, ci, ciParent)

			So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)
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
			So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)
			v2 := getCL(ctx, gHost, 123)
			So(v2.Snapshot.GetGerrit().GetInfo().GetStatus(), ShouldEqual, gerritpb.ChangeStatus_ABANDONED)
			// Files and deps don't have to be set as CV doesn't work with abandoned such CLs.

			// Restore the Gerrit change.
			ct.Clock.Add(time.Minute)
			ct.GFake.MutateChange(gHost, 123, func(c *gf.Change) {
				c.Info.Status = gerritpb.ChangeStatus_NEW
				c.Info.Updated = timestamppb.New(ct.Clock.Now())
			})
			So(gu.clUpdater.TestingForceUpdate(ctx, task), ShouldBeNil)
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
	cl, err := eid.Load(ctx)
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

func sortedRefreshTasks(ct cvtesting.Test) []*changelist.UpdateCLTask {
	ret := make([]*changelist.UpdateCLTask, 0, len(ct.TQ.Tasks().Payloads()))
	for _, m := range ct.TQ.Tasks().Payloads() {
		v, ok := m.(*changelist.UpdateCLTask)
		if ok {
			ret = append(ret, v)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		switch a, b := ret[i], ret[j]; {
		case a.GetExternalId() < b.GetExternalId():
			return true
		case a.GetExternalId() > b.GetExternalId():
			return false
		case a.GetLuciProject() < b.GetLuciProject():
			return true
		case a.GetLuciProject() > b.GetLuciProject():
			return false
		default:
			return a.GetUpdatedHint().AsTime().Before(b.GetUpdatedHint().AsTime())
		}
	})
	return ret
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
