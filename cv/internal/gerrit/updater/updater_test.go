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

	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
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
		ctx := ct.SetUp(t)

		gu := &updaterBackend{
			clUpdater: changelist.NewUpdater(ct.TQDispatcher, changelist.NewMutator(ct.TQDispatcher, &pmMock{}, &rmMock{}, &tjMock{})),
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

		Convey("HasChanged", func() {
			ciInCV := gf.CI(1, gf.Updated(ct.Clock.Now()), gf.MetaRevID("deadbeef"))
			ciInGerrit := proto.Clone(ciInCV).(*gerritpb.ChangeInfo)
			snapshotInCV := &changelist.Snapshot{
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Info: ciInCV,
					},
				},
			}
			snapshotInGerrit := &changelist.Snapshot{
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Info: ciInGerrit,
					},
				},
			}
			Convey("Returns false for exact same snapshot", func() {
				So(gu.HasChanged(snapshotInCV, snapshotInGerrit), ShouldBeFalse)
			})
			Convey("Returns true for greater update time at backend", func() {
				ct.Clock.Add(1 * time.Minute)
				gf.Updated(ct.Clock.Now())(ciInGerrit)
				So(gu.HasChanged(snapshotInCV, snapshotInGerrit), ShouldBeTrue)
			})
			Convey("Returns false for greater update time in CV", func() {
				ct.Clock.Add(1 * time.Minute)
				gf.Updated(ct.Clock.Now())(ciInCV)
				So(gu.HasChanged(snapshotInCV, snapshotInGerrit), ShouldBeFalse)
			})
			Convey("Returns true if meta rev id is different", func() {
				gf.MetaRevID("cafecafe")(ciInGerrit)
				So(gu.HasChanged(snapshotInCV, snapshotInGerrit), ShouldBeTrue)
			})
		})
	})
}

func TestUpdaterBackendFetch(t *testing.T) {
	t.Parallel()

	Convey("updaterBackend.Fetch() works", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			lProject      = "proj-1"
			gHost         = "chromium-review.example.com"
			gHostInternal = "internal-review.example.com"
			gRepo         = "depot_tools"
			gChange       = 123
		)
		task := &changelist.UpdateCLTask{
			LuciProject: lProject,
			Hint:        &changelist.UpdateCLTask_Hint{ExternalUpdateTime: timestamppb.New(time.Time{})},
			Requester:   changelist.UpdateCLTask_RUN_POKE,
		}

		prjcfgtest.Create(ctx, lProject, singleRepoConfig(gHost, gRepo))
		gobmaptest.Update(ctx, lProject)
		externalID := changelist.MustGobID(gHost, gChange)

		// NOTE: this test doesn't actually care about pmMock/rmMock, but they are
		// provided because they are used by the changelist.Updater to process deps
		// on our (backend's) behalf.
		gu := &updaterBackend{
			clUpdater: changelist.NewUpdater(ct.TQDispatcher, changelist.NewMutator(ct.TQDispatcher, &pmMock{}, &rmMock{}, &tjMock{})),
			gFactory:  ct.GFactory(),
		}
		gu.clUpdater.RegisterBackend(gu)

		assertUpdateCLScheduledFor := func(expectedChanges ...int) {
			var actual []int
			for _, p := range ct.TQ.Tasks().Payloads() {
				if t, ok := p.(*changelist.UpdateCLTask); ok {
					_, changeNumber, err := changelist.ExternalID(t.GetExternalId()).ParseGobID()
					So(err, ShouldBeNil)
					actual = append(actual, int(changeNumber))
				}
			}
			sort.Ints(actual)
			sort.Ints(expectedChanges)
			So(actual, ShouldResemble, expectedChanges)
		}

		// Most of the code doesn't care if CL exists, so for simplicity we test it
		// with non yet existing CL input.
		newCL := changelist.CL{ExternalID: externalID}

		Convey("happy path: fetches CL which has no deps and produces correct Snapshot", func() {
			expUpdateTime := ct.Clock.Now().Add(-time.Minute)
			ci := gf.CI(
				gChange,
				gf.Project(gRepo),
				gf.Ref("refs/heads/main"),
				gf.PS(2),
				gf.Updated(expUpdateTime),
				gf.Desc("Title\n\nMeta: data\nNo-Try: true\nChange-Id: Ideadbeef"),
				gf.Files("z.cpp", "dir/a.py"),
			)
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), ci))

			Convey("works if parent commits are empty", func() {
				ct.GFake.MutateChange(gHost, gChange, func(c *gf.Change) {
					gf.ParentCommits(nil)(c.Info)
				})
			})

			res, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
			So(err, ShouldBeNil)

			So(res.AddDependentMeta, ShouldBeNil)
			So(res.ApplicableConfig, ShouldResembleProto, &changelist.ApplicableConfig{
				Projects: []*changelist.ApplicableConfig_Project{
					{
						Name: lProject,
						ConfigGroupIds: []string{
							string(prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0]),
						},
					},
				},
			})
			// Verify Snapshot in two separate steps for easier debugging: high level
			// fields and the noisy Gerrit Change Info.
			// But first, backup Snapshot for later use.
			So(res.Snapshot, ShouldNotBeNil)
			backedupSnapshot := proto.Clone(res.Snapshot).(*changelist.Snapshot)
			// save Gerrit CI portion for later check and check high-level fields first.
			ci = res.Snapshot.GetGerrit().GetInfo()
			res.Snapshot.GetGerrit().Info = nil
			So(res.Snapshot, ShouldResembleProto, &changelist.Snapshot{
				Deps:                  nil,
				ExternalUpdateTime:    timestamppb.New(expUpdateTime),
				LuciProject:           lProject,
				MinEquivalentPatchset: 2,
				Patchset:              2,
				Metadata: []*changelist.StringPair{
					{Key: "Change-Id", Value: "Ideadbeef"},
					{Key: "Meta", Value: "data"},
					{Key: "No-Try", Value: "true"},
				},
				Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
					Host:  gHost,
					Files: []string{"dir/a.py", "z.cpp"},
				}},
			})
			expectedCI := ct.GFake.GetChange(gHost, gChange).Info
			changelist.RemoveUnusedGerritInfo(expectedCI)
			So(ci, ShouldResembleProto, expectedCI)

			Convey("may re-uses files & related changes of the existing Snapshot", func() {
				// Simulate previously saved CL.
				existingCL := changelist.CL{
					ID:               123123213,
					ExternalID:       newCL.ExternalID,
					ApplicableConfig: res.ApplicableConfig,
					Snapshot:         backedupSnapshot,
				}
				ct.Clock.Add(time.Minute)

				Convey("possible", func() {
					// Simulate a new message posted to the Gerrit CL.
					ct.GFake.MutateChange(gHost, gChange, func(c *gf.Change) {
						c.Info.Messages = append(c.Info.Messages, &gerritpb.ChangeMessageInfo{
							Message: "this message is ignored by CV",
						})
						c.Info.Updated = timestamppb.New(ct.Clock.Now())
					})

					res2, err := gu.Fetch(ctx, changelist.NewFetchInput(&existingCL, task))
					So(err, ShouldBeNil)
					// Only the ChangeInfo & ExternalUpdateTime must change.
					expectedCI := ct.GFake.GetChange(gHost, gChange).Info
					changelist.RemoveUnusedGerritInfo(expectedCI)
					So(res2.Snapshot.GetGerrit().GetInfo(), ShouldResembleProto, expectedCI)
					// NOTE: the prior result Snapshot already has nil ChangeInfo due to
					// the assertions done above. Now modify it to match what we expect to
					// get in res2.
					res.Snapshot.ExternalUpdateTime = timestamppb.New(ct.Clock.Now())
					res2.Snapshot.GetGerrit().Info = nil
					So(res2.Snapshot, ShouldResembleProto, res.Snapshot)
				})

				Convey("not possible", func() {
					// Simulate a new revision with new file set.
					ct.GFake.MutateChange(gHost, gChange, func(c *gf.Change) {
						gf.PS(4)(c.Info)
						gf.Files("new.file")(c.Info)
						gf.Desc("See footer.\n\nCq-Depend: 444")(c.Info)
						gf.Updated(ct.Clock.Now())(c.Info)
					})

					res2, err := gu.Fetch(ctx, changelist.NewFetchInput(&existingCL, task))
					So(err, ShouldBeNil)
					So(res2.Snapshot.GetGerrit().GetFiles(), ShouldResemble, []string{"new.file"})
					So(res2.Snapshot.GetGerrit().GetSoftDeps(), ShouldResemble, []*changelist.GerritSoftDep{{Change: 444, Host: gHost}})
				})
			})
		})

		Convey("happy path: fetches CL with deps", func() {
			// This test focuses on deps only. The rest is tested by the no deps case.

			// Simulate a CL with 2 deps via Cq-Depend -- 444 on internal host and 55,
			// and with 2 parents -- 54, 55 (yes, 55 can be both).
			ci := gf.CI(
				gChange, gf.Project(gRepo), gf.Ref("refs/heads/main"), gf.PS(2),
				gf.Desc("Title\n\nCq-Depend: 55,internal:444"),
				gf.Files("a.cpp"),
			)
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(),
				ci, // this CL
				gf.CI(55, gf.Project(gRepo), gf.Ref("refs/heads/main"), gf.PS(1)),
				gf.CI(54, gf.Project(gRepo), gf.Ref("refs/heads/main"), gf.PS(3)),
			))
			// Make this CL depend on 55 ps#1, which in turn depends on 54 ps#3.
			ct.GFake.SetDependsOn(gHost, ci, "55_1")
			ct.GFake.SetDependsOn(gHost, "55_1", "54_3")

			res, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
			So(err, ShouldBeNil)

			So(res.Snapshot.GetGerrit().GetGitDeps(), ShouldResembleProto, []*changelist.GerritGitDep{
				{Change: 55, Immediate: true},
				{Change: 54, Immediate: false},
			})
			So(res.Snapshot.GetGerrit().GetSoftDeps(), ShouldResembleProto, []*changelist.GerritSoftDep{
				{Change: 55, Host: gHost},
				{Change: 444, Host: gHostInternal},
			})
			// The Snapshot.Deps must use internal CV CL IDs.
			cl54 := changelist.MustGobID(gHost, 54).MustCreateIfNotExists(ctx)
			cl55 := changelist.MustGobID(gHost, 55).MustCreateIfNotExists(ctx)
			cl444 := changelist.MustGobID(gHostInternal, 444).MustCreateIfNotExists(ctx)
			expected := []*changelist.Dep{
				{Clid: int64(cl54.ID), Kind: changelist.DepKind_HARD},
				{Clid: int64(cl55.ID), Kind: changelist.DepKind_HARD},
				{Clid: int64(cl444.ID), Kind: changelist.DepKind_SOFT},
			}
			sort.Slice(expected, func(i, j int) bool { return expected[i].GetClid() < expected[j].GetClid() })
			So(res.Snapshot.GetDeps(), ShouldResembleProto, expected)
		})

		Convey("happy path: fetches CL in ignorable state", func() {
			for _, s := range []gerritpb.ChangeStatus{gerritpb.ChangeStatus_ABANDONED, gerritpb.ChangeStatus_MERGED} {
				s := s
				Convey(s.String(), func() {
					ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(),
						gf.CI(gChange, gf.Project(gRepo), gf.Ref("refs/heads/main"), gf.Status(s),
							gf.Desc("All deps and files are ignored for such CL.\n\nCq-Depend: 44"),
						)))
					res, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
					So(err, ShouldBeNil)
					So(res.Snapshot.GetGerrit().GetInfo().GetStatus(), ShouldResemble, s)
					So(res.Snapshot.GetGerrit().GetFiles(), ShouldBeNil)
					So(res.Snapshot.GetDeps(), ShouldBeNil)

				})
			}
			Convey("regression: NEW -> ABANDON -> NEW transitions don't lose file list", func() {
				ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), gf.CI(
					gChange, gf.Project(gRepo), gf.Ref("refs/heads/main"), gf.Files("a.txt"),
					gf.Status(gerritpb.ChangeStatus_NEW),
				)))
				res, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
				So(err, ShouldBeNil)
				So(res.Snapshot.GetGerrit().GetFiles(), ShouldResemble, []string{"a.txt"})

				savedCL := changelist.CL{
					ID:               123123213,
					ExternalID:       newCL.ExternalID,
					Snapshot:         res.Snapshot,
					ApplicableConfig: res.ApplicableConfig,
				}
				ct.Clock.Add(time.Minute)
				ct.GFake.MutateChange(gHost, gChange, func(c *gf.Change) {
					gf.Status(gerritpb.ChangeStatus_ABANDONED)(c.Info)
					gf.Updated(ct.Clock.Now())(c.Info)
				})
				res, err = gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
				So(err, ShouldBeNil)
				// CV doesn't care about files of ABANDONED CLs.
				So(res.Snapshot.GetGerrit().GetFiles(), ShouldBeEmpty)

				// Back to NEW => files must be restored.
				savedCL.Snapshot = res.Snapshot
				ct.Clock.Add(time.Minute)
				ct.GFake.MutateChange(gHost, gChange, func(c *gf.Change) {
					gf.Status(gerritpb.ChangeStatus_NEW)(c.Info)
					gf.Updated(ct.Clock.Now())(c.Info)
				})
				res, err = gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
				So(err, ShouldBeNil)
				So(res.Snapshot.GetGerrit().GetFiles(), ShouldResemble, []string{"a.txt"})
			})
		})

		Convey("stale data", func() {
			staleUpdateTime := ct.Clock.Now().Add(-time.Hour)
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), gf.CI(
				gChange, gf.Project(gRepo), gf.Ref("refs/heads/main"),
				gf.Updated(staleUpdateTime),
			)))

			Convey("CL's updated timestamp is before updatedHint", func() {
				task.Hint.ExternalUpdateTime = timestamppb.New(staleUpdateTime.Add(time.Minute))
				_, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
				So(err, ShouldErrLike, gerrit.ErrStaleData)
			})

			Convey("CL's existing Snapshot is more recent", func() {
				existingCL := changelist.CL{
					ID:         1231231232,
					ExternalID: newCL.ExternalID,
					Snapshot: &changelist.Snapshot{
						ExternalUpdateTime: timestamppb.New(staleUpdateTime.Add(time.Minute)),
					},
				}

				_, err := gu.Fetch(ctx, changelist.NewFetchInput(&existingCL, task))
				So(err, ShouldErrLike, gerrit.ErrStaleData)

				Convey("if MetaRevId was set, skip updating Snapshot", func() {
					task.Hint.MetaRevId = "deadbeef"
					res, err := gu.Fetch(ctx, changelist.NewFetchInput(&existingCL, task))
					// The Fetch() should succeed with nil in toUpdate.Snapshot.
					So(err, ShouldBeNil)
					So(res.Snapshot, ShouldBeNil)
				})
			})
		})

		Convey("Uncertain lack of access", func() {
			// Simulate Gerrit responding with 404.
			gfResponse := status.New(codes.NotFound, "not found or no access")
			gfACLmock := func(_ gf.Operation, luciProject string) *status.Status {
				return gfResponse
			}
			ct.GFake.AddFrom(gf.WithCIs(gHost, gfACLmock, gf.CI(gChange, gf.Ref("refs/heads/main"), gf.Project(gRepo))))

			// First time 404 isn't certain.
			res, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
			So(err, ShouldBeNil)
			So(res.Snapshot, ShouldBeNil)
			So(res.ApplicableConfig, ShouldBeNil)
			So(res.AddDependentMeta.GetByProject()[lProject], ShouldResembleProto, &changelist.Access_Project{
				NoAccess:     true,
				NoAccessTime: timestamppb.New(ct.Clock.Now().Add(noAccessGraceDuration)),
				UpdateTime:   timestamppb.New(ct.Clock.Now()),
			})
			assertUpdateCLScheduledFor(gChange)

			// Simulate intermediate save to Datastore.
			cl := changelist.CL{ID: 1123123, ExternalID: externalID, Access: res.AddDependentMeta}
			// For now, access denied isn't certain.
			So(cl.AccessKind(ctx, lProject), ShouldEqual, changelist.AccessDeniedProbably)

			Convey("403 is treated same as 404, ie potentially stale", func() {
				ct.Clock.Add(noAccessGraceRetryDelay)
				// Because 403 can be caused due to stale ACLs on a stale mirror.
				gfResponse = status.New(codes.PermissionDenied, "403")
				res2, err := gu.Fetch(ctx, changelist.NewFetchInput(&cl, task))
				So(err, ShouldBeNil)
				// res2 must be the same, except for the .UpdateTime.
				So(res2.AddDependentMeta.GetByProject()[lProject].UpdateTime, ShouldResembleProto, timestamppb.New(ct.Clock.Now()))
				res.AddDependentMeta.GetByProject()[lProject].UpdateTime = nil
				res2.AddDependentMeta.GetByProject()[lProject].UpdateTime = nil
				So(res2, cvtesting.SafeShouldResemble, res)
				// And thus, lack of access is still uncertain.
				So(cl.AccessKind(ctx, lProject), ShouldEqual, changelist.AccessDeniedProbably)
			})

			Convey("still no access after grace duration", func() {
				ct.Clock.Add(noAccessGraceDuration + time.Second)
				res2, err := gu.Fetch(ctx, changelist.NewFetchInput(&cl, task))
				So(err, ShouldBeNil)
				// res2 must be the same, except for the .UpdateTime.
				So(res2.AddDependentMeta.GetByProject()[lProject].UpdateTime, ShouldResembleProto, timestamppb.New(ct.Clock.Now()))
				res.AddDependentMeta.GetByProject()[lProject].UpdateTime = nil
				res2.AddDependentMeta.GetByProject()[lProject].UpdateTime = nil
				So(res2, cvtesting.SafeShouldResemble, res)
				// Nothing new should be scheduled (on top of the existing task).
				assertUpdateCLScheduledFor(gChange)
				// Finally, certainty is reached.
				So(cl.AccessKind(ctx, lProject), ShouldEqual, changelist.AccessDenied)
			})

			Convey("access not found is forgotten on a successful fetch", func() {
				// At a later time, the CL "magically" appears, e.g. if ACLs are fixed.
				gfResponse = status.New(codes.OK, "OK")
				ct.Clock.Add(time.Minute)
				res2, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
				So(err, ShouldBeNil)
				// The previous record of lack of Access must be expunged.
				So(res2.AddDependentMeta, ShouldBeNil)
				So(res2.DelAccess, ShouldResemble, []string{lProject})
				// Exact value of Snapshot and ApplicableConfig is tested in happy path,
				// here we only care that both are set.
				So(res2.Snapshot, ShouldNotBeNil)
				So(res2.ApplicableConfig, ShouldNotBeNil)

				Convey("can lose access again, which should not erase the snapshot saved before", func() {
					// Simulate saved CL.
					cl.Snapshot = res.Snapshot
					cl.ApplicableConfig = res.ApplicableConfig

					gfResponse = status.New(codes.NotFound, "not found, again")
					ct.Clock.Add(time.Minute)
					res3, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
					So(err, ShouldBeNil)

					So(res3.Snapshot, ShouldBeNil)         // nothing to update
					So(res3.ApplicableConfig, ShouldBeNil) // nothing to update
					So(res3.AddDependentMeta.GetByProject()[lProject], ShouldResembleProto, &changelist.Access_Project{
						NoAccess:     true,
						NoAccessTime: timestamppb.New(ct.Clock.Now().Add(noAccessGraceDuration)),
						UpdateTime:   timestamppb.New(ct.Clock.Now()),
					})
				})
			})
		})

		Convey("Not watched CL", func() {
			Convey("Gerrit host is not watched", func() {
				bogusCL := changelist.CL{ExternalID: changelist.MustGobID("404.example.com", 404)}
				res, err := gu.Fetch(ctx, changelist.NewFetchInput(&bogusCL, task))
				So(err, ShouldBeNil)
				So(res.Snapshot, ShouldBeNil)
				So(res.ApplicableConfig, ShouldBeNil)
				So(res.AddDependentMeta.GetByProject()[lProject], ShouldResembleProto, &changelist.Access_Project{
					NoAccess:     true,
					NoAccessTime: timestamppb.New(ct.Clock.Now()), // immediate no access.
					UpdateTime:   timestamppb.New(ct.Clock.Now()),
				})
				bogusCL.Access = res.AddDependentMeta
				So(bogusCL.AccessKind(ctx, lProject), ShouldEqual, changelist.AccessDenied)
			})
			Convey("Only the ref isn't watched", func() {
				ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), gf.CI(gChange, gf.Ref("refs/un/watched"), gf.Project(gRepo))))
				res, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
				So(err, ShouldBeNil)
				// Although technically, LUCI project currently has access,
				// we mark it as lacking access from CV's PoV.
				// TODO(tandrii): this is weird, and ought to be refactored together
				// with weird "AddDependentMeta" field.
				So(res.AddDependentMeta.GetByProject()[lProject], ShouldNotBeNil)
				So(res.ApplicableConfig, ShouldResembleProto, &changelist.ApplicableConfig{
					// No watching projects.
				})
			})
		})

		Convey("Gerrit errors are propagated", func() {
			Convey("GetChange fails", func() {
				fakeResponseStatus := func(_ gf.Operation, _ string) *status.Status {
					return status.New(codes.ResourceExhausted, "doesn't matter")
				}
				ct.GFake.AddFrom(gf.WithCIs(gHost, fakeResponseStatus, gf.CI(gChange, gf.Ref("refs/heads/main"), gf.Project(gRepo))))
				_, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
				So(err, ShouldErrLike, gerrit.ErrOutOfQuota)
			})
			Convey("ListFiles or GetRelatedChanges fails", func() {
				// ListFiles and GetRelatedChanges are done in parallel but after the
				// GetChange call.
				cnt := int32(0)
				fakeResponseStatus := func(_ gf.Operation, _ string) *status.Status {
					if atomic.AddInt32(&cnt, 1) == 2 {
						return status.New(codes.Unavailable, "2nd call failed")
					}
					return status.New(codes.OK, "ok")
				}
				ct.GFake.AddFrom(gf.WithCIs(gHost, fakeResponseStatus, gf.CI(gChange, gf.Ref("refs/heads/main"), gf.Project(gRepo))))
				_, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
				So(err, ShouldErrLike, "2nd call failed")
			})
		})

		Convey("MetaRevID", func() {
			expUpdateTime := ct.Clock.Now().Add(-time.Minute)
			ci := gf.CI(
				gChange,
				gf.Project(gRepo),
				gf.Ref("refs/heads/main"),
				gf.PS(2),
				gf.Updated(expUpdateTime),
				gf.MetaRevID("deadbeef"),
			)
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), ci))
			task.Hint.MetaRevId = "deadbeef"
		})
	})
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

type tjMock struct{}

func (t *tjMock) ScheduleCancelStale(ctx context.Context, clid common.CLID, prevMinEquivalentPatchset, currentMinEquivalentPatchset int32, eta time.Time) error {
	return nil
}
