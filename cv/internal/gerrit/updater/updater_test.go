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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/tq"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap/gobmaptest"
)

func TestUpdaterBackend(t *testing.T) {
	t.Parallel()

	ftt.Run("updaterBackend methods work, except Fetch()", t, func(t *ftt.Test) {
		// Fetch() is covered in TestUpdaterBackendFetch.
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		gu := &updaterBackend{
			clUpdater: changelist.NewUpdater(ct.TQDispatcher, changelist.NewMutator(ct.TQDispatcher, &pmMock{}, &rmMock{}, &tjMock{})),
			gFactory:  ct.GFactory(),
		}

		t.Run("Kind", func(t *ftt.Test) {
			assert.Loosely(t, gu.Kind(), should.Equal("gerrit"))
		})
		t.Run("TQErrorSpec", func(t *ftt.Test) {
			tqSpec := gu.TQErrorSpec()
			err := errors.Fmt("retry, don't ignore: %w", gerrit.ErrStaleData)
			assert.Loosely(t, tq.Ignore.In(tqSpec.Error(ctx, err)), should.BeFalse)
		})
		t.Run("LookupApplicableConfig", func(t *ftt.Test) {
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

			t.Run("Happy path", func(t *ftt.Test) {
				acfg, err := gu.LookupApplicableConfig(ctx, cl)
				assert.NoErr(t, err)
				assert.That(t, acfg, should.Match(&changelist.ApplicableConfig{
					Projects: []*changelist.ApplicableConfig_Project{
						{Name: "luci-project-x", ConfigGroupIds: []string{string(xConfigGroupID)}},
					},
				}))
			})

			t.Run("CL without Gerrit Snapshot can't be decided on", func(t *ftt.Test) {
				cl.Snapshot.Kind = nil
				acfg, err := gu.LookupApplicableConfig(ctx, cl)
				assert.NoErr(t, err)
				assert.Loosely(t, acfg, should.BeNil)
			})

			t.Run("No watching projects", func(t *ftt.Test) {
				cl.Snapshot.GetGerrit().GetInfo().Ref = "ref/un/watched"
				acfg, err := gu.LookupApplicableConfig(ctx, cl)
				assert.NoErr(t, err)
				// Must be empty, but not nil per updaterBackend interface contract.
				assert.Loosely(t, acfg, should.NotBeNil)
				assert.That(t, acfg, should.Match(&changelist.ApplicableConfig{}))
			})

			t.Run("Works with >1 LUCI project watching the same CL", func(t *ftt.Test) {
				prjcfgtest.Create(ctx, "luci-project-dupe", singleRepoConfig(gHost, "x"))
				gobmaptest.Update(ctx, "luci-project-dupe")
				acfg, err := gu.LookupApplicableConfig(ctx, cl)
				assert.NoErr(t, err)
				assert.Loosely(t, acfg.GetProjects(), should.HaveLength(2))
			})
		})

		t.Run("HasChanged", func(t *ftt.Test) {
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
			t.Run("Returns false for exact same snapshot", func(t *ftt.Test) {
				assert.Loosely(t, gu.HasChanged(snapshotInCV, snapshotInGerrit), should.BeFalse)
			})
			t.Run("Returns true for greater update time at backend", func(t *ftt.Test) {
				ct.Clock.Add(1 * time.Minute)
				gf.Updated(ct.Clock.Now())(ciInGerrit)
				assert.Loosely(t, gu.HasChanged(snapshotInCV, snapshotInGerrit), should.BeTrue)
			})
			t.Run("Returns false for greater update time in CV", func(t *ftt.Test) {
				ct.Clock.Add(1 * time.Minute)
				gf.Updated(ct.Clock.Now())(ciInCV)
				assert.Loosely(t, gu.HasChanged(snapshotInCV, snapshotInGerrit), should.BeFalse)
			})
			t.Run("Returns true if meta rev id is different", func(t *ftt.Test) {
				gf.MetaRevID("cafecafe")(ciInGerrit)
				assert.Loosely(t, gu.HasChanged(snapshotInCV, snapshotInGerrit), should.BeTrue)
			})
		})
	})
}

func TestUpdaterBackendFetch(t *testing.T) {
	t.Parallel()

	ftt.Run("updaterBackend.Fetch() works", t, func(t *ftt.Test) {
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
				if ts, ok := p.(*changelist.UpdateCLTask); ok {
					_, changeNumber, err := changelist.ExternalID(ts.GetExternalId()).ParseGobID()
					assert.NoErr(t, err)
					actual = append(actual, int(changeNumber))
				}
			}
			sort.Ints(actual)
			sort.Ints(expectedChanges)
			assert.That(t, actual, should.Match(expectedChanges))
		}

		// Most of the code doesn't care if CL exists, so for simplicity we test it
		// with non yet existing CL input.
		newCL := changelist.CL{ExternalID: externalID}

		t.Run("happy path: fetches CL which has no deps and produces correct Snapshot", func(t *ftt.Test) {
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

			t.Run("works if parent commits are empty", func(t *ftt.Test) {
				ct.GFake.MutateChange(gHost, gChange, func(c *gf.Change) {
					gf.ParentCommits(nil)(c.Info)
				})
			})

			res, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
			assert.NoErr(t, err)

			assert.Loosely(t, res.AddDependentMeta, should.BeNil)
			assert.That(t, res.ApplicableConfig, should.Match(&changelist.ApplicableConfig{
				Projects: []*changelist.ApplicableConfig_Project{
					{
						Name: lProject,
						ConfigGroupIds: []string{
							string(prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0]),
						},
					},
				},
			}))
			// Verify Snapshot in two separate steps for easier debugging: high level
			// fields and the noisy Gerrit Change Info.
			// But first, backup Snapshot for later use.
			assert.Loosely(t, res.Snapshot, should.NotBeNil)
			backedupSnapshot := proto.Clone(res.Snapshot).(*changelist.Snapshot)
			// save Gerrit CI portion for later check and check high-level fields first.
			ci = res.Snapshot.GetGerrit().GetInfo()
			res.Snapshot.GetGerrit().Info = nil
			assert.That(t, res.Snapshot, should.Match(&changelist.Snapshot{
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
			}))
			expectedCI := ct.GFake.GetChange(gHost, gChange).Info
			changelist.RemoveUnusedGerritInfo(expectedCI)
			assert.That(t, ci, should.Match(expectedCI))

			t.Run("may re-uses files & related changes of the existing Snapshot", func(t *ftt.Test) {
				// Simulate previously saved CL.
				existingCL := changelist.CL{
					ID:               123123213,
					ExternalID:       newCL.ExternalID,
					ApplicableConfig: res.ApplicableConfig,
					Snapshot:         backedupSnapshot,
				}
				ct.Clock.Add(time.Minute)

				t.Run("possible", func(t *ftt.Test) {
					// Simulate a new message posted to the Gerrit CL.
					ct.GFake.MutateChange(gHost, gChange, func(c *gf.Change) {
						c.Info.Messages = append(c.Info.Messages, &gerritpb.ChangeMessageInfo{
							Message: "this message is ignored by CV",
						})
						c.Info.Updated = timestamppb.New(ct.Clock.Now())
					})

					res2, err := gu.Fetch(ctx, changelist.NewFetchInput(&existingCL, task))
					assert.NoErr(t, err)
					// Only the ChangeInfo & ExternalUpdateTime must change.
					expectedCI := ct.GFake.GetChange(gHost, gChange).Info
					changelist.RemoveUnusedGerritInfo(expectedCI)
					assert.That(t, res2.Snapshot.GetGerrit().GetInfo(), should.Match(expectedCI))
					// NOTE: the prior result Snapshot already has nil ChangeInfo due to
					// the assertions done above. Now modify it to match what we expect to
					// get in res2.
					res.Snapshot.ExternalUpdateTime = timestamppb.New(ct.Clock.Now())
					res2.Snapshot.GetGerrit().Info = nil
					assert.That(t, res2.Snapshot, should.Match(res.Snapshot))
				})

				t.Run("not possible", func(t *ftt.Test) {
					// Simulate a new revision with new file set.
					ct.GFake.MutateChange(gHost, gChange, func(c *gf.Change) {
						gf.PS(4)(c.Info)
						gf.Files("new.file")(c.Info)
						gf.Desc("See footer.\n\nCq-Depend: 444")(c.Info)
						gf.Updated(ct.Clock.Now())(c.Info)
					})

					res2, err := gu.Fetch(ctx, changelist.NewFetchInput(&existingCL, task))
					assert.NoErr(t, err)
					assert.That(t, res2.Snapshot.GetGerrit().GetFiles(), should.Match([]string{"new.file"}))
					assert.That(t, res2.Snapshot.GetGerrit().GetSoftDeps(), should.Match([]*changelist.GerritSoftDep{{Change: 444, Host: gHost}}))
				})
			})
		})

		t.Run("happy path: fetches CL with deps", func(t *ftt.Test) {
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
			assert.NoErr(t, err)

			assert.That(t, res.Snapshot.GetGerrit().GetGitDeps(), should.Match([]*changelist.GerritGitDep{
				{Change: 55, Immediate: true},
				{Change: 54, Immediate: false},
			}))
			assert.That(t, res.Snapshot.GetGerrit().GetSoftDeps(), should.Match([]*changelist.GerritSoftDep{
				{Change: 55, Host: gHost},
				{Change: 444, Host: gHostInternal},
			}))
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
			assert.That(t, res.Snapshot.GetDeps(), should.Match(expected))
		})

		t.Run("happy path: fetches CL in ignorable state", func(t *ftt.Test) {
			for _, s := range []gerritpb.ChangeStatus{gerritpb.ChangeStatus_ABANDONED, gerritpb.ChangeStatus_MERGED} {
				t.Run(s.String(), func(t *ftt.Test) {
					ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(),
						gf.CI(gChange, gf.Project(gRepo), gf.Ref("refs/heads/main"), gf.Status(s),
							gf.Desc("All deps and files are ignored for such CL.\n\nCq-Depend: 44"),
						)))
					res, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
					assert.NoErr(t, err)
					assert.That(t, res.Snapshot.GetGerrit().GetInfo().GetStatus(), should.Match(s))
					assert.Loosely(t, res.Snapshot.GetGerrit().GetFiles(), should.BeNil)
					assert.Loosely(t, res.Snapshot.GetDeps(), should.BeNil)

				})
			}
			t.Run("regression: NEW -> ABANDON -> NEW transitions don't lose file list", func(t *ftt.Test) {
				ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), gf.CI(
					gChange, gf.Project(gRepo), gf.Ref("refs/heads/main"), gf.Files("a.txt"),
					gf.Status(gerritpb.ChangeStatus_NEW),
				)))
				res, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
				assert.NoErr(t, err)
				assert.That(t, res.Snapshot.GetGerrit().GetFiles(), should.Match([]string{"a.txt"}))

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
				assert.NoErr(t, err)
				// CV doesn't care about files of ABANDONED CLs.
				assert.Loosely(t, res.Snapshot.GetGerrit().GetFiles(), should.BeEmpty)

				// Back to NEW => files must be restored.
				savedCL.Snapshot = res.Snapshot
				ct.Clock.Add(time.Minute)
				ct.GFake.MutateChange(gHost, gChange, func(c *gf.Change) {
					gf.Status(gerritpb.ChangeStatus_NEW)(c.Info)
					gf.Updated(ct.Clock.Now())(c.Info)
				})
				res, err = gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
				assert.NoErr(t, err)
				assert.That(t, res.Snapshot.GetGerrit().GetFiles(), should.Match([]string{"a.txt"}))
			})
		})

		t.Run("stale data", func(t *ftt.Test) {
			staleUpdateTime := ct.Clock.Now().Add(-time.Hour)
			ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), gf.CI(
				gChange, gf.Project(gRepo), gf.Ref("refs/heads/main"),
				gf.Updated(staleUpdateTime),
			)))

			t.Run("CL's updated timestamp is before updatedHint", func(t *ftt.Test) {
				task.Hint.ExternalUpdateTime = timestamppb.New(staleUpdateTime.Add(time.Minute))
				_, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
				assert.ErrIsLike(t, err, gerrit.ErrStaleData)
			})

			t.Run("CL's existing Snapshot is more recent", func(t *ftt.Test) {
				existingCL := changelist.CL{
					ID:         1231231232,
					ExternalID: newCL.ExternalID,
					Snapshot: &changelist.Snapshot{
						ExternalUpdateTime: timestamppb.New(staleUpdateTime.Add(time.Minute)),
					},
				}

				_, err := gu.Fetch(ctx, changelist.NewFetchInput(&existingCL, task))
				assert.ErrIsLike(t, err, gerrit.ErrStaleData)

				t.Run("if MetaRevId was set, skip updating Snapshot", func(t *ftt.Test) {
					task.Hint.MetaRevId = "deadbeef"
					res, err := gu.Fetch(ctx, changelist.NewFetchInput(&existingCL, task))
					// The Fetch() should succeed with nil in toUpdate.Snapshot.
					assert.NoErr(t, err)
					assert.Loosely(t, res.Snapshot, should.BeNil)
				})
			})
		})

		t.Run("Uncertain lack of access", func(t *ftt.Test) {
			// Simulate Gerrit responding with 404.
			gfResponse := status.New(codes.NotFound, "not found or no access")
			gfACLmock := func(_ gf.Operation, luciProject string) *status.Status {
				return gfResponse
			}
			ct.GFake.AddFrom(gf.WithCIs(gHost, gfACLmock, gf.CI(gChange, gf.Ref("refs/heads/main"), gf.Project(gRepo))))

			// First time 404 isn't certain.
			res, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
			assert.NoErr(t, err)
			assert.Loosely(t, res.Snapshot, should.BeNil)
			assert.Loosely(t, res.ApplicableConfig, should.BeNil)
			assert.That(t, res.AddDependentMeta.GetByProject()[lProject], should.Match(&changelist.Access_Project{
				NoAccess:     true,
				NoAccessTime: timestamppb.New(ct.Clock.Now().Add(noAccessGraceDuration)),
				UpdateTime:   timestamppb.New(ct.Clock.Now()),
			}))
			assertUpdateCLScheduledFor(gChange)

			// Simulate intermediate save to Datastore.
			cl := changelist.CL{ID: 1123123, ExternalID: externalID, Access: res.AddDependentMeta}
			// For now, access denied isn't certain.
			assert.Loosely(t, cl.AccessKind(ctx, lProject), should.Equal(changelist.AccessDeniedProbably))

			t.Run("403 is treated same as 404, ie potentially stale", func(t *ftt.Test) {
				ct.Clock.Add(noAccessGraceRetryDelay)
				// Because 403 can be caused due to stale ACLs on a stale mirror.
				gfResponse = status.New(codes.PermissionDenied, "403")
				res2, err := gu.Fetch(ctx, changelist.NewFetchInput(&cl, task))
				assert.NoErr(t, err)
				// res2 must be the same, except for the .UpdateTime.
				assert.That(t, res2.AddDependentMeta.GetByProject()[lProject].UpdateTime, should.Match(timestamppb.New(ct.Clock.Now())))
				res.AddDependentMeta.GetByProject()[lProject].UpdateTime = nil
				res2.AddDependentMeta.GetByProject()[lProject].UpdateTime = nil
				assert.That(t, res2, should.Match(res))
				// And thus, lack of access is still uncertain.
				assert.Loosely(t, cl.AccessKind(ctx, lProject), should.Equal(changelist.AccessDeniedProbably))
			})

			t.Run("still no access after grace duration", func(t *ftt.Test) {
				ct.Clock.Add(noAccessGraceDuration + time.Second)
				res2, err := gu.Fetch(ctx, changelist.NewFetchInput(&cl, task))
				assert.NoErr(t, err)
				// res2 must be the same, except for the .UpdateTime.
				assert.That(t, res2.AddDependentMeta.GetByProject()[lProject].UpdateTime, should.Match(timestamppb.New(ct.Clock.Now())))
				res.AddDependentMeta.GetByProject()[lProject].UpdateTime = nil
				res2.AddDependentMeta.GetByProject()[lProject].UpdateTime = nil
				assert.That(t, res2, should.Match(res))
				// Nothing new should be scheduled (on top of the existing task).
				assertUpdateCLScheduledFor(gChange)
				// Finally, certainty is reached.
				assert.Loosely(t, cl.AccessKind(ctx, lProject), should.Equal(changelist.AccessDenied))
			})

			t.Run("access not found is forgotten on a successful fetch", func(t *ftt.Test) {
				// At a later time, the CL "magically" appears, e.g. if ACLs are fixed.
				gfResponse = status.New(codes.OK, "OK")
				ct.Clock.Add(time.Minute)
				res2, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
				assert.NoErr(t, err)
				// The previous record of lack of Access must be expunged.
				assert.Loosely(t, res2.AddDependentMeta, should.BeNil)
				assert.That(t, res2.DelAccess, should.Match([]string{lProject}))
				// Exact value of Snapshot and ApplicableConfig is tested in happy path,
				// here we only care that both are set.
				assert.Loosely(t, res2.Snapshot, should.NotBeNil)
				assert.Loosely(t, res2.ApplicableConfig, should.NotBeNil)

				t.Run("can lose access again, which should not erase the snapshot saved before", func(t *ftt.Test) {
					// Simulate saved CL.
					cl.Snapshot = res.Snapshot
					cl.ApplicableConfig = res.ApplicableConfig

					gfResponse = status.New(codes.NotFound, "not found, again")
					ct.Clock.Add(time.Minute)
					res3, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
					assert.NoErr(t, err)

					assert.Loosely(t, res3.Snapshot, should.BeNil)         // nothing to update
					assert.Loosely(t, res3.ApplicableConfig, should.BeNil) // nothing to update
					assert.That(t, res3.AddDependentMeta.GetByProject()[lProject], should.Match(&changelist.Access_Project{
						NoAccess:     true,
						NoAccessTime: timestamppb.New(ct.Clock.Now().Add(noAccessGraceDuration)),
						UpdateTime:   timestamppb.New(ct.Clock.Now()),
					}))
				})
			})
		})

		t.Run("Not watched CL", func(t *ftt.Test) {
			t.Run("Gerrit host is not watched", func(t *ftt.Test) {
				bogusCL := changelist.CL{ExternalID: changelist.MustGobID("404.example.com", 404)}
				res, err := gu.Fetch(ctx, changelist.NewFetchInput(&bogusCL, task))
				assert.NoErr(t, err)
				assert.Loosely(t, res.Snapshot, should.BeNil)
				assert.Loosely(t, res.ApplicableConfig, should.BeNil)
				assert.That(t, res.AddDependentMeta.GetByProject()[lProject], should.Match(&changelist.Access_Project{
					NoAccess:     true,
					NoAccessTime: timestamppb.New(ct.Clock.Now()), // immediate no access.
					UpdateTime:   timestamppb.New(ct.Clock.Now()),
				}))
				bogusCL.Access = res.AddDependentMeta
				assert.Loosely(t, bogusCL.AccessKind(ctx, lProject), should.Equal(changelist.AccessDenied))
			})
			t.Run("Only the ref isn't watched", func(t *ftt.Test) {
				ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLPublic(), gf.CI(gChange, gf.Ref("refs/un/watched"), gf.Project(gRepo))))
				res, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
				assert.NoErr(t, err)
				// Although technically, LUCI project currently has access,
				// we mark it as lacking access from CV's PoV.
				// TODO(tandrii): this is weird, and ought to be refactored together
				// with weird "AddDependentMeta" field.
				assert.Loosely(t, res.AddDependentMeta.GetByProject()[lProject], should.NotBeNil)
				assert.That(t, res.ApplicableConfig, should.Match(&changelist.ApplicableConfig{
					// No watching projects.
				}))
			})
		})

		t.Run("Gerrit errors are propagated", func(t *ftt.Test) {
			t.Run("GetChange fails", func(t *ftt.Test) {
				fakeResponseStatus := func(_ gf.Operation, _ string) *status.Status {
					return status.New(codes.ResourceExhausted, "doesn't matter")
				}
				ct.GFake.AddFrom(gf.WithCIs(gHost, fakeResponseStatus, gf.CI(gChange, gf.Ref("refs/heads/main"), gf.Project(gRepo))))
				_, err := gu.Fetch(ctx, changelist.NewFetchInput(&newCL, task))
				assert.ErrIsLike(t, err, gerrit.ErrOutOfQuota)
			})
			t.Run("ListFiles or GetRelatedChanges fails", func(t *ftt.Test) {
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
				assert.ErrIsLike(t, err, "2nd call failed")
			})
		})

		t.Run("MetaRevID", func(t *ftt.Test) {
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
