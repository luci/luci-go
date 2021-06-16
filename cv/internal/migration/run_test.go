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

package migration

import (
	"context"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFetchActiveRuns(t *testing.T) {
	t.Parallel()

	Convey("FetchActiveRuns", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		Convey("Returns migration Runs", func() {
			const rid = "chromium/1111111111111-cafecafe"
			updateTime := timestamppb.New(clock.Now(ctx).Add(-2 * time.Minute))
			ci1 := &gerritpb.ChangeInfo{
				Project:         "infra/infra",
				Number:          11111,
				CurrentRevision: "deadbeef",
				Revisions: map[string]*gerritpb.RevisionInfo{
					"deadbeef": {
						Number:  7,
						Created: timestamppb.New(updateTime.AsTime().Add(-2 * time.Minute)),
					},
				},
				Labels: map[string]*gerritpb.LabelInfo{
					"Commit-Queue": {
						All: []*gerritpb.ApprovalInfo{
							{
								User: &gerritpb.AccountInfo{
									AccountId: 1,
									Email:     "user@example.com",
								},
								Value: 1,
								Date:  timestamppb.New(updateTime.AsTime().Add(-2 * time.Minute)),
							},
						},
					},
				},
			}
			ci2 := &gerritpb.ChangeInfo{
				Project:         "infra/infra",
				Number:          11110,
				CurrentRevision: "deadbeef",
				Revisions: map[string]*gerritpb.RevisionInfo{
					"deadbeef": {
						Number:  5,
						Created: timestamppb.New(updateTime.AsTime().Add(-3 * time.Minute)),
					},
				},
				Labels: map[string]*gerritpb.LabelInfo{
					"Commit-Queue": {
						All: []*gerritpb.ApprovalInfo{
							{
								User: &gerritpb.AccountInfo{
									AccountId: 1,
									Email:     "user@example.com",
								},
								Value: 1,
								Date:  timestamppb.New(updateTime.AsTime().Add(-4 * time.Minute)),
							},
						},
					},
				},
			}
			err := datastore.Put(ctx,
				&run.Run{
					ID:     rid,
					Status: run.Status_RUNNING,
					Mode:   run.DryRun,
					CLs:    common.CLIDs{1, 2},
				},
				&run.RunCL{
					ID:  1,
					Run: datastore.MakeKey(ctx, run.RunKind, rid),
					Detail: &changelist.Snapshot{
						ExternalUpdateTime: updateTime,
						Deps: []*changelist.Dep{
							{Clid: 2, Kind: changelist.DepKind_HARD},
						},
						Patchset:              7,
						MinEquivalentPatchset: 6,
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host:  "https://chromium-review.googlesource.com/",
								Files: []string{"1.cpp"},
								Info:  ci1,
							},
						},
					},
					Trigger: &run.Trigger{
						Time:            timestamppb.New(updateTime.AsTime().Add(-2 * time.Minute)),
						Mode:            string(run.DryRun),
						Email:           "user@example.com",
						GerritAccountId: 1,
					},
				},
				&run.RunCL{
					ID:  2,
					Run: datastore.MakeKey(ctx, run.RunKind, rid),
					Detail: &changelist.Snapshot{
						ExternalUpdateTime:    updateTime,
						Patchset:              5,
						MinEquivalentPatchset: 5,
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host:  "https://chromium-review.googlesource.com/",
								Files: []string{"2.cpp"},
								Info:  ci2,
							},
						},
					},
					Trigger: &run.Trigger{
						Time:            timestamppb.New(updateTime.AsTime().Add(-3 * time.Minute)),
						Mode:            string(run.DryRun),
						Email:           "user@example.com",
						GerritAccountId: 1,
					},
				},
			)
			So(err, ShouldBeNil)
			runs, err := fetchActiveRuns(ctx, "chromium")
			So(err, ShouldBeNil)
			So(runs, ShouldHaveLength, 1)
			So(runs[0], ShouldResembleProto, &migrationpb.ActiveRun{
				Id: rid,
				Cls: []*migrationpb.RunCL{
					{
						Id: 1,
						Gc: &cvbqpb.GerritChange{
							Host:                       "https://chromium-review.googlesource.com/",
							Project:                    "infra/infra",
							Change:                     11111,
							Patchset:                   7,
							EarliestEquivalentPatchset: 6,
							Mode:                       cvbqpb.Mode_DRY_RUN,
						},
						Files: []string{"1.cpp"},
						Info:  ci1,
						Trigger: &migrationpb.RunCL_Trigger{
							Time:      timestamppb.New(updateTime.AsTime().Add(-2 * time.Minute)),
							AccountId: 1,
							Email:     "user@example.com",
						},
						Deps: []*migrationpb.RunCL_Dep{
							{Id: 2, Hard: true},
						},
					},
					{
						Id: 2,
						Gc: &cvbqpb.GerritChange{
							Host:                       "https://chromium-review.googlesource.com/",
							Project:                    "infra/infra",
							Change:                     11110,
							Patchset:                   5,
							EarliestEquivalentPatchset: 5,
							Mode:                       cvbqpb.Mode_DRY_RUN,
						},
						Files: []string{"2.cpp"},
						Info:  ci2,
						Trigger: &migrationpb.RunCL_Trigger{
							Time:      timestamppb.New(updateTime.AsTime().Add(-3 * time.Minute)),
							AccountId: 1,
							Email:     "user@example.com",
						},
					},
				},
			})
			Convey("Excludes runs with corresponding VerifiedCQDRun entities", func() {
				err := saveVerifiedCQDRun(
					ctx,
					&migrationpb.ReportVerifiedRunRequest{Run: &migrationpb.ReportedRun{
						Id:      rid,
						Attempt: &cvbqpb.Attempt{Key: "cafecafe"},
					}},
					func(context.Context) error { return nil },
				)
				So(err, ShouldBeNil)
				runs, err := fetchActiveRuns(ctx, "chromium")
				So(err, ShouldBeNil)
				So(runs, ShouldHaveLength, 0)
			})
		})
		Convey("Excludes non-RUNNING runs", func() {
			err := datastore.Put(ctx,
				&run.Run{
					ID:     "chromium/1111111111111-deadbeef",
					Status: run.Status_SUCCEEDED,
				},
				&run.Run{
					ID:     "chromium/2222222222222-baaaaaad",
					Status: run.Status_FAILED,
				},
			)
			So(err, ShouldBeNil)
			runs, err := fetchActiveRuns(ctx, "chromium")
			So(err, ShouldBeNil)
			So(runs, ShouldHaveLength, 0)
		})

		Convey("Handles FYI deps", func() {
			const rid = "chromium/1111111111111-cafecafe"
			const gHost = "x-review.example.com"

			putCL := func(gChange int64, depCLID common.CLID, runCL bool) common.CLID {
				triggeredAt := ct.Clock.Now().Add(-time.Minute)
				ci := gf.CI(int(gChange), gf.CQ(+1, triggeredAt, gf.U("user-1")), gf.Updated(triggeredAt))
				cl, err := changelist.MustGobID(gHost, gChange).GetOrInsert(ctx, func(cl *changelist.CL) {
					cl.Snapshot = &changelist.Snapshot{
						Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
							Host:  gHost,
							Info:  ci,
							Files: []string{"some/file"},
						}},
						Patchset:              1,
						MinEquivalentPatchset: 1,
						ExternalUpdateTime:    timestamppb.New(triggeredAt),
						LuciProject:           "chromium",
					}
					if depCLID > 0 {
						cl.Snapshot.Deps = []*changelist.Dep{{Clid: int64(depCLID), Kind: changelist.DepKind_HARD}}
					}
				})
				So(err, ShouldBeNil)
				if runCL {
					err := datastore.Put(ctx, &run.RunCL{
						ID:     cl.ID,
						Run:    datastore.MakeKey(ctx, run.RunKind, rid),
						Detail: cl.Snapshot,
						Trigger: &run.Trigger{
							Time:            timestamppb.New(triggeredAt),
							Mode:            string(run.DryRun),
							Email:           "user-1@example.com",
							GerritAccountId: 1,
						},
					})
					So(err, ShouldBeNil)
				}
				return cl.ID
			}
			depCLID := putCL(1, -1, false)
			runCLID := putCL(2, depCLID, true)
			So(datastore.Put(ctx, &run.Run{
				ID:     rid,
				Status: run.Status_RUNNING,
				Mode:   run.DryRun,
				CLs:    common.CLIDs{runCLID},
			}), ShouldBeNil)
			runs, err := fetchActiveRuns(ctx, "chromium")
			So(err, ShouldBeNil)
			So(runs, ShouldHaveLength, 1)
			So(runs[0].Cls, ShouldHaveLength, 1)
			So(runs[0].Cls[0].Id, ShouldResemble, int64(runCLID))
			So(runs[0].FyiDeps, ShouldHaveLength, 1)
			So(runs[0].FyiDeps[0].Id, ShouldResemble, int64(depCLID))
			So(runs[0].FyiDeps[0].Trigger, ShouldBeNil)
			So(runs[0].FyiDeps[0].Deps, ShouldBeNil)
		})
	})
}

func TestFetchExcludedCLs(t *testing.T) {
	t.Parallel()

	Convey("FetchExcludedCLs", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const gHost = "example.com"
		const lProject = "infra"

		putCL := func(clid common.CLID) *changelist.CL {
			cl := &changelist.CL{
				ID: clid,
				Snapshot: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: &gerritpb.ChangeInfo{
								Number: 1000 + int64(clid),
							},
						},
					},
				},
			}
			So(datastore.Put(ctx, cl), ShouldBeNil)
			return cl
		}
		putRun := func(s run.Status, clids ...common.CLID) *run.Run {
			clDigest := make([]byte, len(clids))
			for i, clid := range clids {
				clDigest[i] = byte(clid)
				putCL(clid)
			}
			r := &run.Run{
				ID:     common.MakeRunID(lProject, ct.Clock.Now(), 1, clDigest),
				Status: s,
				CLs:    clids,
			}
			So(datastore.Put(ctx, r), ShouldBeNil)
			return r
		}
		putVerified := func(rs ...*run.Run) {
			for _, r := range rs {
				So(datastore.Put(ctx, &VerifiedCQDRun{ID: r.ID}), ShouldBeNil)
			}
		}
		idsOf := func(cls []*cvbqpb.GerritChange) []int {
			out := make([]int, len(cls))
			for i, cl := range cls {
				So(cl.GetHost(), ShouldResemble, gHost)
				out[i] = int(cl.GetChange())
			}
			sort.Ints(out)
			return out
		}

		Convey("no runs, nothing to exclude", func() {
			cls, err := fetchExcludedCLs(ctx, lProject)
			So(err, ShouldBeNil)
			So(cls, ShouldHaveLength, 0)
		})

		Convey("with active Runs", func() {
			r12 := putRun(run.Status_RUNNING, 1, 2)
			r234 := putRun(run.Status_RUNNING, 2, 3, 4)
			r5 := putRun(run.Status_WAITING_FOR_SUBMISSION, 5)
			r6 := putRun(run.Status_SUBMITTING, 6)
			r7 := putRun(run.Status_CANCELLED, 7)
			r8 := putRun(run.Status_SUCCEEDED, 8)
			r9 := putRun(run.Status_FAILED, 9)

			Convey("no VerifiedCQDRun, so nothing to exclude", func() {
				cls, err := fetchExcludedCLs(ctx, lProject)
				So(err, ShouldBeNil)
				So(cls, ShouldHaveLength, 0)
			})
			Convey("the expected case of some running CL having VerifiedCQDRun", func() {
				putVerified(r12)
				cls, err := fetchExcludedCLs(ctx, lProject)
				So(err, ShouldBeNil)
				So(idsOf(cls), ShouldResemble, []int{1001, 1002})
			})
			Convey("only non-ended Runs count", func() {
				putVerified(r12, r234, r5, r6, r7, r8, r9)
				cls, err := fetchExcludedCLs(ctx, lProject)
				So(err, ShouldBeNil)
				So(idsOf(cls), ShouldResemble, []int{1001, 1002, 1003, 1004, 1005, 1006})
			})
		})
	})
}

func TestSaveFinishedCQDRun(t *testing.T) {
	t.Parallel()

	Convey("saveFinishedCQDRun", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		mr := &migrationpb.ReportedRun{
			Attempt: &cvbqpb.Attempt{
				Key: "samekeyhash",
				// Other fields are populated in practice, but this test ignores that.
			},
		}

		Convey("unknown CV ID", func() {
			noopNotify := func(context.Context) error { return nil }
			So(saveFinishedCQDRun(ctx, mr, noopNotify), ShouldBeNil)
			f := FinishedCQDRun{AttemptKey: "samekeyhash"}
			So(datastore.Get(ctx, &f), ShouldBeNil)
			So(f.Payload, ShouldResembleProto, mr)
			So(f.RunID == "", ShouldBeTrue)
			_, err := LoadFinishedCQDRun(ctx, "some/run-1-id")
			So(err, ShouldErrLike, "no FinishedCQDRun")

			Convey("with known CV ID", func() {
				mr.Id = "known-cv-id/123123-samekeyhash"
				So(saveFinishedCQDRun(ctx, mr, noopNotify), ShouldBeNil)
				f := FinishedCQDRun{AttemptKey: "samekeyhash"}
				So(datastore.Get(ctx, &f), ShouldBeNil)
				So(f.Payload, ShouldResembleProto, mr)
				So(f.RunID, ShouldResemble, common.RunID(mr.Id))

				f2, err := LoadFinishedCQDRun(ctx, f.RunID)
				So(err, ShouldBeNil)
				So(f2.AttemptKey, ShouldResemble, f.AttemptKey)
			})
		})
	})
}

func TestFetchAttempt(t *testing.T) {
	t.Parallel()

	Convey("fetchAttempt works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		mkRun := func(t time.Time, clDigest []byte) *run.Run {
			id := common.MakeRunID("project", t, 1, clDigest)
			r := &run.Run{
				ID:            id,
				CQDAttemptKey: id.AttemptKey(),
				CreateTime:    datastore.RoundTime(t.UTC()),
			}
			So(datastore.Put(ctx, r), ShouldBeNil)
			return r
		}

		deadbeefBytes := []byte{222, 173, 190, 239}

		Convey("None", func() {
			out, err := fetchAttempt(ctx, "deadbeef")
			So(err, ShouldBeNil)
			So(out, ShouldBeNil)
		})
		Convey("One", func() {
			r := mkRun(ct.Clock.Now(), deadbeefBytes)
			So(r.CQDAttemptKey, ShouldResemble, "deadbeef")
			out, err := fetchAttempt(ctx, "deadbeef")
			So(err, ShouldBeNil)
			So(out.ID, ShouldResemble, r.ID)
			So(out.CQDAttemptKey, ShouldResemble, "deadbeef")
		})
		Convey("Several", func() {
			mkRun(ct.Clock.Now().Add(time.Minute), deadbeefBytes)
			mkRun(ct.Clock.Now().Add(time.Hour), deadbeefBytes)
			mkRun(ct.Clock.Now().Add(time.Second), deadbeefBytes)
			out, err := fetchAttempt(ctx, "deadbeef")
			So(err, ShouldBeNil)
			So(out.CQDAttemptKey, ShouldResemble, "deadbeef")
			So(out.CreateTime, ShouldResemble, datastore.RoundTime(ct.Clock.Now().Add(time.Hour).UTC()))
		})
	})
}
