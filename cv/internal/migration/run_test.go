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
	"go.chromium.org/luci/gae/service/datastore"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
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

func TestHelperFunctions(t *testing.T) {
	Convey("saveFinishedRun", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		attempt := &cvbqpb.Attempt{
			Key:                  "f001234",
			ConfigGroup:          "maingroup",
			LuciProject:          "chromium",
			ClGroupKey:           "b004321",
			EquivalentClGroupKey: "c003333",
			StartTime:            timestamppb.New(clock.Now(ctx).Add(-2 * time.Minute)),
			EndTime:              timestamppb.New(clock.Now(ctx).Add(-1 * time.Minute)),
			Builds: []*cvbqpb.Build{
				{
					Id:       423143214321,
					Host:     "cr-buildbucket.appspot.com",
					Origin:   cvbqpb.Build_NOT_REUSED,
					Critical: false,
				},
			},
			GerritChanges: []*cvbqpb.GerritChange{
				{
					Host:                       "https://chromium-review.googlesource.com/",
					Project:                    "chromium/src",
					Change:                     11111,
					Patchset:                   7,
					EarliestEquivalentPatchset: 6,
					Mode:                       cvbqpb.Mode_FULL_RUN,
					SubmitStatus:               cvbqpb.GerritChange_PENDING,
				},
			},
			Status:               cvbqpb.AttemptStatus_SUCCESS,
			Substatus:            cvbqpb.AttemptSubstatus_NO_SUBSTATUS,
			HasCustomRequirement: false,
		}
		mr := &migrationpb.Run{
			Attempt: attempt,
			Id:      "chromium/1111111111111-cafecafe",
			Cls:     []*migrationpb.RunCL{},
		}

		Convey("saves Attempt proto", func() {
			So(saveFinishedRun(ctx, mr), ShouldBeNil)
			fr := &FinishedRun{ID: "chromium/1111111111111-cafecafe"}
			So(datastore.Get(ctx, fr), ShouldBeNil)
			So(fr.Attempt, ShouldResembleProto, attempt)
		})
	})
}
