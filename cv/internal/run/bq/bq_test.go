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

package bq

import (
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/proto/gerrit"
	. "go.chromium.org/luci/common/testing/assertions"
	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
)

func TestMakeAttempt(t *testing.T) {
	Convey("toGerritChange", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		t := ct.Clock.Now()
		ts, err := ptypes.TimestampProto(ct.Clock.Now())
		So(err, ShouldBeNil)

		runID := "lproject/222-1-deadbeef"
		r := &run.Run{
			ID:         common.RunID(runID),
			Status:     run.Status_SUCCEEDED,
			CreateTime: t,
			StartTime:  t.Add(time.Minute * 1),
			EndTime:    t.Add(time.Minute * 30),
			CLs:        common.CLIDs{3, 5},
			Submission: &run.Submission{
				Cls:          []int64{3, 5},
				SubmittedCls: []int64{3},
			},
			Mode: run.FullRun,
		}

		Convey("submitted CL", func() {
			cl := &run.RunCL{
				ID:  3, // This ID is found in the Run's submitted CLs list.
				Run: datastore.MakeKey(ctx, run.RunKind, runID),
				Detail: &changelist.Snapshot{
					LuciProject:           "lproject",
					Patchset:              12,
					MinEquivalentPatchset: 11,
					Kind: &changelist.Snapshot_Gerrit{
						&changelist.Gerrit{
							Host: "foo-review.googlesource.com",
							Info: &gerrit.ChangeInfo{
								Number:  333,
								Project: "gproject",
								Ref:     "refs/heads/main",
							},
						},
					},
				},
				Trigger: &run.Trigger{
					Time: ts,
					Mode: "FULL_RUN",
				},
			}
			So(toGerritChange(cl, r), ShouldResemble, &cvbqpb.GerritChange{
				Host:                       "foo-review.googlesource.com",
				Project:                    "gproject",
				Change:                     333,
				Patchset:                   12,
				EarliestEquivalentPatchset: 11,
				TriggerTime:                ts,
				Mode:                       cvbqpb.Mode_FULL_RUN,
				SubmitStatus:               cvbqpb.GerritChange_SUCCESS,
			})
		})

		Convey("unsubmitted CL", func() {
			cl := &run.RunCL{
				ID:  5,
				Run: datastore.MakeKey(ctx, run.RunKind, runID),
				Detail: &changelist.Snapshot{
					LuciProject:           "lproject",
					Patchset:              6,
					MinEquivalentPatchset: 4,
					Kind: &changelist.Snapshot_Gerrit{
						&changelist.Gerrit{
							Host: "foo-review.googlesource.com",
							Info: &gerrit.ChangeInfo{
								Number:  555,
								Project: "gproject",
								Ref:     "refs/heads/main",
							},
						},
					},
				},
				Trigger: &run.Trigger{
					Time: ts,
					Mode: "FULL_RUN",
				},
			}
			So(toGerritChange(cl, r), ShouldResemble, &cvbqpb.GerritChange{
				Host:                       "foo-review.googlesource.com",
				Project:                    "gproject",
				Change:                     555,
				Patchset:                   6,
				EarliestEquivalentPatchset: 4,
				TriggerTime:                ts,
				Mode:                       cvbqpb.Mode_FULL_RUN,
				SubmitStatus:               cvbqpb.GerritChange_FAILURE,
			})
		})
	})
}

func TestAttemptFetching(t *testing.T) {
	Convey("fetchCQDAttempt", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		id := common.MakeRunID(
			"chromium", ct.Clock.Now().UTC(), 1, []byte("cafecafe"))

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

		vr := &migration.VerifiedCQDRun{
			ID: id,
			Payload: &migrationpb.ReportVerifiedRunRequest{
				Run: &migrationpb.Run{
					Attempt: attempt,
					Id:      string(id),
					Cls:     []*migrationpb.RunCL{},
				},
			},
		}
		So(datastore.Put(ctx, vr), ShouldBeNil)

		Convey("returns attempt from datastore", func() {
			a, err := fetchCQDAttempt(ctx, id)
			So(err, ShouldBeNil)
			So(a, ShouldResembleProto, attempt)
		})

		Convey("returns nil when not in datastore", func() {
			id := common.MakeRunID(
				"x", ct.Clock.Now().UTC(), 1, []byte("aaa"))
			a, err := fetchCQDAttempt(ctx, id)
			So(err, ShouldBeNil)
			So(a, ShouldBeNil)
		})
	})
}
