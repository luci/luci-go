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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFetchActiveRuns(t *testing.T) {
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
					CLs:    []common.CLID{1, 2},
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
			So(runs[0], ShouldResembleProto, &migrationpb.Run{
				Attempt: &cvbqpb.Attempt{LuciProject: "chromium"},
				Id:      rid,
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
	})
}

func TestFinalizeRun(t *testing.T) {
	Convey("FetchActiveRuns", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		Convey("Error if migration run reports non-ended status", func() {
			err := finalizeRun(ctx, &migrationpb.Run{
				Attempt: &cvbqpb.Attempt{
					Status: cvbqpb.AttemptStatus_STARTED,
				},
			})
			So(err, ShouldErrLike, "expected terminal status for attempt")
		})
		Convey("Noop if Run has already ended", func() {
			t := clock.Now(ctx).UTC()
			err := datastore.Put(ctx, &run.Run{
				ID:         common.RunID("chromium/111-2-deadbeef"),
				Status:     run.Status_FAILED,
				EVersion:   3,
				UpdateTime: t,
			})
			So(err, ShouldBeNil)

			ct.Clock.Add(1 * time.Hour)
			err = finalizeRun(ctx, &migrationpb.Run{
				Attempt: &cvbqpb.Attempt{
					Status: cvbqpb.AttemptStatus_SUCCESS,
				},
				Id: "chromium/111-2-deadbeef",
			})
			So(err, ShouldBeNil)

			r := run.Run{ID: common.RunID("chromium/111-2-deadbeef")}
			So(datastore.Get(ctx, &r), ShouldBeNil)
			So(r, ShouldResemble, run.Run{
				ID:         common.RunID("chromium/111-2-deadbeef"),
				Status:     run.Status_FAILED,
				EVersion:   3,
				UpdateTime: t,
			})
		})
		Convey("Set Run Status to terminal", func() {
			err := datastore.Put(ctx, &run.Run{
				ID:         common.RunID("chromium/111-2-deadbeef"),
				Status:     run.Status_RUNNING,
				EVersion:   3,
				UpdateTime: clock.Now(ctx).UTC(),
			})
			So(err, ShouldBeNil)

			ct.Clock.Add(1 * time.Hour)
			now := clock.Now(ctx).UTC()
			err = finalizeRun(ctx, &migrationpb.Run{
				Attempt: &cvbqpb.Attempt{
					Status: cvbqpb.AttemptStatus_SUCCESS,
				},
				Id: "chromium/111-2-deadbeef",
			})
			So(err, ShouldBeNil)

			r := run.Run{ID: common.RunID("chromium/111-2-deadbeef")}
			So(datastore.Get(ctx, &r), ShouldBeNil)
			So(r, ShouldResemble, run.Run{
				ID:         common.RunID("chromium/111-2-deadbeef"),
				Status:     run.Status_SUCCEEDED,
				EVersion:   4,
				UpdateTime: now,
			})
			So(pmtest.Projects(ct.TQ.Tasks()), ShouldResemble, []string{"chromium"})
		})
	})
}
