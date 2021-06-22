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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMakeAttempt(t *testing.T) {

	Convey("makeAttempt", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		epoch := ct.Clock.Now().UTC()

		Convey("given run with one CL", func() {
			runID := common.MakeRunID("lproject", epoch, 1, []byte("aaa"))
			r := &run.Run{
				ID:            common.RunID(runID),
				Status:        run.Status_SUCCEEDED,
				ConfigGroupID: "sha256:deadbeefdeadbeef/cgroup",
				CreateTime:    epoch,
				StartTime:     epoch.Add(time.Minute * 2),
				EndTime:       epoch.Add(time.Minute * 25),
				CLs:           common.CLIDs{1},
				Submission:    nil,
				Mode:          run.DryRun,
			}
			So(datastore.Put(ctx, r), ShouldBeNil)
			So(datastore.Put(ctx, &run.RunCL{
				ID:         1,
				Run:        datastore.MakeKey(ctx, run.RunKind, string(runID)),
				ExternalID: "gerrit/foo-review.googlesource.com/111",
				Detail: &changelist.Snapshot{
					LuciProject:           "lproject",
					Patchset:              2,
					MinEquivalentPatchset: 2,
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: "foo-review.googlesource.com",
							Info: &gerrit.ChangeInfo{
								Number:  111,
								Project: "gproject",
								Ref:     "refs/heads/main",
							},
						},
					},
				},
				Trigger: &run.Trigger{Time: timestamppb.New(epoch)},
			}), ShouldBeNil)
			So(nil, ShouldBeNil)

			a, err := makeAttempt(ctx, r)
			So(err, ShouldBeNil)
			So(a, ShouldResembleProto, &cvbqpb.Attempt{
				Key:                  runID.AttemptKey(),
				LuciProject:          "lproject",
				ConfigGroup:          "cgroup",
				ClGroupKey:           "331ea2a6a5d5f3b3",
				EquivalentClGroupKey: "47337d4707144297",
				StartTime:            timestamppb.New(epoch),
				EndTime:              timestamppb.New(epoch.Add(25 * time.Minute)),
				GerritChanges: []*cvbqpb.GerritChange{
					{
						Host:                       "foo-review.googlesource.com",
						Project:                    "gproject",
						Change:                     111,
						Patchset:                   2,
						EarliestEquivalentPatchset: 2,
						TriggerTime:                timestamppb.New(epoch),
						Mode:                       cvbqpb.Mode_DRY_RUN,
						SubmitStatus:               cvbqpb.GerritChange_PENDING,
					},
				},
				Status:    cvbqpb.AttemptStatus_SUCCESS,
				Substatus: cvbqpb.AttemptSubstatus_NO_SUBSTATUS,
			})
		})

		Convey("given run with partially submitted CLs", func() {
			runID := common.MakeRunID("lproject", epoch, 1, []byte("f00"))
			r := &run.Run{
				ID:            runID,
				Status:        run.Status_FAILED,
				ConfigGroupID: "sha256:deadbeefdeadbeef/cgroup",
				CreateTime:    epoch,
				StartTime:     epoch.Add(time.Minute * 2),
				EndTime:       epoch.Add(time.Minute * 30),
				CLs:           common.CLIDs{3, 5, 7},
				Submission: &run.Submission{
					// CL 3 was submitted, CL 5 failed to submit and CL 7 was never
					// attempted.
					Cls:          []int64{3, 5, 7},
					SubmittedCls: []int64{3},
					FailedCls:    []int64{5},
				},
				Mode: run.FullRun,
			}
			So(datastore.Put(ctx, r), ShouldBeNil)
			So(datastore.Put(ctx, &run.RunCL{
				ID:         3,
				Run:        datastore.MakeKey(ctx, run.RunKind, string(runID)),
				ExternalID: "gerrit/foo-review.googlesource.com/333",
				Detail: &changelist.Snapshot{
					LuciProject:           "lproject",
					Patchset:              12,
					MinEquivalentPatchset: 11,
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: "foo-review.googlesource.com",
							Info: &gerrit.ChangeInfo{
								Number:  333,
								Project: "gproject",
								Ref:     "refs/heads/main",
							},
						},
					},
				},
				Trigger: &run.Trigger{Time: timestamppb.New(epoch)},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &run.RunCL{
				ID:         5,
				Run:        datastore.MakeKey(ctx, run.RunKind, string(runID)),
				ExternalID: "gerrit/foo-review.googlesource.com/555",
				Detail: &changelist.Snapshot{
					LuciProject:           "lproject",
					Patchset:              6,
					MinEquivalentPatchset: 4,
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: "foo-review.googlesource.com",
							Info: &gerrit.ChangeInfo{
								Number:  555,
								Project: "gproject",
								Ref:     "refs/heads/main",
							},
						},
					},
				},
				Trigger: &run.Trigger{Time: timestamppb.New(epoch)},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &run.RunCL{
				ID:         7,
				Run:        datastore.MakeKey(ctx, run.RunKind, string(runID)),
				ExternalID: "gerrit/foo-review.googlesource.com/777",
				Detail: &changelist.Snapshot{
					LuciProject:           "lproject",
					Patchset:              3,
					MinEquivalentPatchset: 2,
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: "foo-review.googlesource.com",
							Info: &gerrit.ChangeInfo{
								Number:  777,
								Project: "gproject",
								Ref:     "refs/heads/main",
							},
						},
					},
				},
				Trigger: &run.Trigger{Time: timestamppb.New(epoch)},
			}), ShouldBeNil)

			a, err := makeAttempt(ctx, r)
			So(err, ShouldBeNil)
			So(a, ShouldResembleProto, &cvbqpb.Attempt{
				Key:                  runID.AttemptKey(),
				LuciProject:          "lproject",
				ConfigGroup:          r.ConfigGroupID.Name(),
				ClGroupKey:           "54090427bcf39a01",
				EquivalentClGroupKey: "07b4731d9f201da5",
				StartTime:            timestamppb.New(r.CreateTime),
				EndTime:              timestamppb.New(r.EndTime),
				GerritChanges: []*cvbqpb.GerritChange{
					{
						Host:                       "foo-review.googlesource.com",
						Project:                    "gproject",
						Change:                     333,
						Patchset:                   12,
						EarliestEquivalentPatchset: 11,
						TriggerTime:                timestamppb.New(epoch),
						Mode:                       cvbqpb.Mode_FULL_RUN,
						SubmitStatus:               cvbqpb.GerritChange_SUCCESS,
					},
					{
						Host:                       "foo-review.googlesource.com",
						Project:                    "gproject",
						Change:                     555,
						Patchset:                   6,
						EarliestEquivalentPatchset: 4,
						TriggerTime:                timestamppb.New(epoch),
						Mode:                       cvbqpb.Mode_FULL_RUN,
						SubmitStatus:               cvbqpb.GerritChange_FAILURE,
					},
					{
						Host:                       "foo-review.googlesource.com",
						Project:                    "gproject",
						Change:                     777,
						Patchset:                   3,
						EarliestEquivalentPatchset: 2,
						TriggerTime:                timestamppb.New(epoch),
						Mode:                       cvbqpb.Mode_FULL_RUN,
						SubmitStatus:               cvbqpb.GerritChange_PENDING,
					},
				},
				// In the case of submit failure for one or more CLs,
				// the Attempt value is still SUCCESS, for backwards
				// compatibility.
				Status:    cvbqpb.AttemptStatus_SUCCESS,
				Substatus: cvbqpb.AttemptSubstatus_NO_SUBSTATUS,
			})
		})
	})
}

func TestUsingAttemptInfoFromCQDaemon(t *testing.T) {
	Convey("fetchCQDAttempt", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		epoch := ct.Clock.Now().UTC()

		runID := common.MakeRunID("chromium", epoch, 1, []byte("cafecafe"))
		r := &run.Run{
			ID:            common.RunID(runID),
			Status:        run.Status_SUCCEEDED,
			ConfigGroupID: "sha256:deadbeefdeadbeef/cgroup",
			CreateTime:    epoch,
			StartTime:     epoch.Add(time.Minute * 2),
			EndTime:       epoch.Add(time.Minute * 25),
			CLs:           common.CLIDs{1},
			Mode:          run.DryRun,
		}
		So(datastore.Put(ctx, r), ShouldBeNil)

		attempt := &cvbqpb.Attempt{
			Key:                  "f001234",
			ConfigGroup:          "maingroup",
			LuciProject:          "chromium",
			ClGroupKey:           "b004321",
			EquivalentClGroupKey: "c003333",
			StartTime:            timestamppb.New(epoch.Add(-2 * time.Minute)),
			EndTime:              timestamppb.New(epoch.Add(-1 * time.Minute)),
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

		Convey("finalized by CV", func() {
			Convey("returns nil when not in datastore", func() {
				a, err := fetchCQDAttempt(ctx, r)
				So(err, ShouldBeNil)
				So(a, ShouldBeNil)
			})

			vr := &migration.VerifiedCQDRun{
				ID: runID,
				Payload: &migrationpb.ReportVerifiedRunRequest{
					Run: &migrationpb.ReportedRun{
						Attempt: attempt,
						Id:      string(runID),
					},
				},
			}
			So(datastore.Put(ctx, vr), ShouldBeNil)

			Convey("returns Attempt from datastore", func() {
				a, err := fetchCQDAttempt(ctx, r)
				So(err, ShouldBeNil)
				So(a, ShouldResembleProto, attempt)
			})

		})

		Convey("finalized by CQDaemon", func() {
			r.FinalizedByCQD = true
			Convey("error if no finished run found in datastore", func() {
				_, err := fetchCQDAttempt(ctx, r)
				So(err, ShouldErrLike, "no FinishedCQDRun for Run")
			})

			fr := &migration.FinishedCQDRun{
				AttemptKey: r.CQDAttemptKey,
				RunID:      runID,
				Payload: &migrationpb.ReportedRun{
					Id:      string(runID),
					Attempt: attempt,
				},
			}
			So(datastore.Put(ctx, fr), ShouldBeNil)

			Convey("returns Attempt from datastore", func() {
				a, err := fetchCQDAttempt(ctx, r)
				So(err, ShouldBeNil)
				So(a, ShouldResembleProto, attempt)
			})
		})
	})

	Convey("reconcileAttempts", t, func() {
		epoch := testclock.TestRecentTimeUTC

		Convey("overwrites fields using values from CQD", func() {
			runID := common.MakeRunID("chromium", epoch, 1, []byte("cafec0ffee"))
			a := &cvbqpb.Attempt{
				Key:                  runID.AttemptKey(),
				LuciProject:          "chromium",
				ConfigGroup:          "cgroup",
				ClGroupKey:           "11115555aaaa5555",
				EquivalentClGroupKey: "6666dddd9999aaaa",
				StartTime:            timestamppb.New(epoch),
				EndTime:              timestamppb.New(epoch.Add(5 * time.Minute)),
				Status:               cvbqpb.AttemptStatus_FAILURE,
				Substatus:            cvbqpb.AttemptSubstatus_NO_SUBSTATUS,
			}
			cqda := &cvbqpb.Attempt{
				Key:                  "f001234",
				ConfigGroup:          "maingroup",
				LuciProject:          "chromium",
				ClGroupKey:           "b004321",
				EquivalentClGroupKey: "c003333",
				StartTime:            timestamppb.New(epoch),
				EndTime:              timestamppb.New(epoch.Add(5 * time.Minute)),
				Builds: []*cvbqpb.Build{
					{
						Id:       423143214321,
						Host:     "cr-buildbucket.appspot.com",
						Origin:   cvbqpb.Build_NOT_REUSED,
						Critical: false,
					},
				},
				Status:               cvbqpb.AttemptStatus_FAILURE,
				Substatus:            cvbqpb.AttemptSubstatus_FAILED_TRYJOBS,
				HasCustomRequirement: true,
			}
			So(reconcileAttempts(a, cqda), ShouldResembleProto, &cvbqpb.Attempt{
				Key:                  runID.AttemptKey(),
				LuciProject:          "chromium",
				ConfigGroup:          "cgroup",
				ClGroupKey:           "11115555aaaa5555",
				EquivalentClGroupKey: "6666dddd9999aaaa",
				StartTime:            timestamppb.New(epoch),
				EndTime:              timestamppb.New(epoch.Add(5 * time.Minute)),
				Builds: []*cvbqpb.Build{
					{
						Id:       423143214321,
						Host:     "cr-buildbucket.appspot.com",
						Origin:   cvbqpb.Build_NOT_REUSED,
						Critical: false,
					},
				},
				Status:               cvbqpb.AttemptStatus_FAILURE,
				Substatus:            cvbqpb.AttemptSubstatus_FAILED_TRYJOBS,
				HasCustomRequirement: true,
			})
		})
	})
}
