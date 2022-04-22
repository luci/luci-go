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
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMakeAttempt(t *testing.T) {
	Convey("makeAttempt", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		epoch := ct.Clock.Now().UTC()
		const lProject = "infra"
		const bbHost = "cr-buildbucket.appspot.com"
		const gHost = "foo-review.googlesource.com"
		runID := common.MakeRunID(lProject, epoch, 1, []byte("aaa"))
		cfg := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					Verifiers: &cfgpb.Verifiers{
						Tryjob: &cfgpb.Verifiers_Tryjob{
							Builders: []*cfgpb.Verifiers_Tryjob_Builder{
								{
									Name:         fmt.Sprintf("%s/try/reused", lProject),
									DisableReuse: false,
								},
								{
									Name:         fmt.Sprintf("%s/try/reusable", lProject),
									DisableReuse: false,
								},
								{
									Name: fmt.Sprintf("%s/try/not-reusable", lProject),
									EquivalentTo: &cfgpb.Verifiers_Tryjob_EquivalentBuilder{
										Name: fmt.Sprintf("%s/try/equi-builder", lProject),
									},
									DisableReuse: true,
								},
							},
						},
					},
				},
			},
		}
		prjcfgtest.Create(ctx, lProject, cfg)

		makeRun := func(mode run.Mode, cls common.CLIDs, submission *run.Submission, tryjobs []*run.Tryjob) *run.Run {
			r := &run.Run{
				ID:            common.RunID(runID),
				Status:        run.Status_SUCCEEDED,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
				CreateTime:    epoch,
				StartTime:     epoch.Add(time.Minute * 2),
				EndTime:       epoch.Add(time.Minute * 25),
				CLs:           cls,
				Tryjobs: &run.Tryjobs{
					Tryjobs: tryjobs,
				},
				Submission: submission,
				Mode:       mode,
			}
			So(datastore.Put(ctx, r), ShouldBeNil)

			for i, clid := range cls {
				changeNum := i + 100
				So(datastore.Put(ctx, &run.RunCL{
					ID:         clid,
					Run:        datastore.MakeKey(ctx, run.RunKind, string(runID)),
					ExternalID: changelist.MustGobID(gHost, int64(changeNum)),
					Detail: &changelist.Snapshot{
						LuciProject:           lProject,
						Patchset:              int32(i + 1),
						MinEquivalentPatchset: int32(i + 1),
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host: gHost,
								Info: &gerritpb.ChangeInfo{
									Number:  int64(changeNum),
									Project: "gproject",
									Ref:     "refs/heads/main",
								},
							},
						},
					},
					Trigger: &run.Trigger{Time: timestamppb.New(epoch)},
				}), ShouldBeNil)
			}
			return r
		}

		Convey("with 1 CL", func() {
			r := makeRun(run.DryRun, common.CLIDs{1}, nil, nil)
			a, err := makeAttempt(ctx, r)
			So(err, ShouldBeNil)
			So(a, ShouldResembleProto, &cvbqpb.Attempt{
				Key:                  runID.AttemptKey(),
				LuciProject:          lProject,
				ConfigGroup:          "main",
				ClGroupKey:           "5f12962fb6ae6239",
				EquivalentClGroupKey: "2f286ef1dcce9458",
				StartTime:            timestamppb.New(epoch),
				EndTime:              timestamppb.New(epoch.Add(25 * time.Minute)),
				GerritChanges: []*cvbqpb.GerritChange{
					{
						Host:                       "foo-review.googlesource.com",
						Project:                    "gproject",
						Change:                     100,
						Patchset:                   1,
						EarliestEquivalentPatchset: 1,
						TriggerTime:                timestamppb.New(epoch),
						Mode:                       cvbqpb.Mode_DRY_RUN,
						SubmitStatus:               cvbqpb.GerritChange_PENDING,
					},
				},
				Status:    cvbqpb.AttemptStatus_SUCCESS,
				Substatus: cvbqpb.AttemptSubstatus_NO_SUBSTATUS,
			})
		})

		Convey("run with partially submitted CLs", func() {
			sub := &run.Submission{
				// CL 3 was submitted, CL 5 failed to submit and CL 7 was never
				// attempted.
				Cls:          []int64{3, 5, 7},
				SubmittedCls: []int64{3},
				FailedCls:    []int64{5},
			}
			r := makeRun(run.FullRun, common.CLIDs{3, 5, 7}, sub, nil)
			a, err := makeAttempt(ctx, r)
			So(err, ShouldBeNil)
			So(a, ShouldResembleProto, &cvbqpb.Attempt{
				Key:                  runID.AttemptKey(),
				LuciProject:          lProject,
				ConfigGroup:          r.ConfigGroupID.Name(),
				ClGroupKey:           "9792d0b416c84c38",
				EquivalentClGroupKey: "59dca6566c96aec8",
				StartTime:            timestamppb.New(r.CreateTime),
				EndTime:              timestamppb.New(r.EndTime),
				GerritChanges: []*cvbqpb.GerritChange{
					{
						Host:                       gHost,
						Project:                    "gproject",
						Change:                     100,
						Patchset:                   1,
						EarliestEquivalentPatchset: 1,
						TriggerTime:                timestamppb.New(epoch),
						Mode:                       cvbqpb.Mode_FULL_RUN,
						SubmitStatus:               cvbqpb.GerritChange_SUCCESS,
					},
					{
						Host:                       gHost,
						Project:                    "gproject",
						Change:                     101,
						Patchset:                   2,
						EarliestEquivalentPatchset: 2,
						TriggerTime:                timestamppb.New(epoch),
						Mode:                       cvbqpb.Mode_FULL_RUN,
						SubmitStatus:               cvbqpb.GerritChange_FAILURE,
					},
					{
						Host:                       gHost,
						Project:                    "gproject",
						Change:                     102,
						Patchset:                   3,
						EarliestEquivalentPatchset: 3,
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

		Convey("run with tryjobs", func() {
			tryjobs := []*run.Tryjob{
				{
					Definition: &tryjob.Definition{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host: bbHost,
								Builder: &buildbucketpb.BuilderID{
									Project: lProject,
									Bucket:  "try",
									Builder: "reused",
								},
							},
						},
					},
					ExternalId: string(tryjob.MustBuildbucketID(bbHost, 456)),
					Reused:     true,
					Critical:   true,
				},
				{
					Definition: &tryjob.Definition{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host: bbHost,
								Builder: &buildbucketpb.BuilderID{
									Project: lProject,
									Bucket:  "try",
									Builder: "reusable",
								},
							},
						},
					},
					ExternalId: string(tryjob.MustBuildbucketID(bbHost, 567)),
					Reused:     false,
					Critical:   false,
				},
				{
					Definition: &tryjob.Definition{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host: bbHost,
								Builder: &buildbucketpb.BuilderID{
									Project: lProject,
									Bucket:  "try",
									Builder: "not-reusable",
								},
							},
						},
					},
					ExternalId: string(tryjob.MustBuildbucketID(bbHost, 678)),
					Reused:     false,
					Critical:   true,
				},
				{
					Definition: &tryjob.Definition{
						Backend: &tryjob.Definition_Buildbucket_{
							Buildbucket: &tryjob.Definition_Buildbucket{
								Host: bbHost,
								Builder: &buildbucketpb.BuilderID{
									Project: lProject,
									Bucket:  "try",
									Builder: "equi-builder",
								},
							},
						},
					},
					ExternalId: string(tryjob.MustBuildbucketID(bbHost, 789)),
					Reused:     false,
					Critical:   false,
				},
			}
			r := makeRun(run.DryRun, nil, nil, tryjobs)
			a, err := makeAttempt(ctx, r)
			So(err, ShouldBeNil)
			So(a, ShouldResembleProto, &cvbqpb.Attempt{
				Key:                  runID.AttemptKey(),
				LuciProject:          lProject,
				ConfigGroup:          "main",
				ClGroupKey:           "e3b0c44298fc1c14",
				EquivalentClGroupKey: "c5bd4eaa8690c8f8",
				StartTime:            timestamppb.New(epoch),
				EndTime:              timestamppb.New(epoch.Add(25 * time.Minute)),
				Builds: []*cvbqpb.Build{
					{
						Id:       456,
						Host:     bbHost,
						Origin:   cvbqpb.Build_REUSED,
						Critical: true,
					},
					{
						Id:       567,
						Host:     bbHost,
						Origin:   cvbqpb.Build_NOT_REUSED,
						Critical: false,
					},
					{
						Id:       678,
						Host:     bbHost,
						Origin:   cvbqpb.Build_NOT_REUSABLE,
						Critical: true,
					},
					{
						Id:       789,
						Host:     bbHost,
						Origin:   cvbqpb.Build_NOT_REUSABLE,
						Critical: false,
					},
				},
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
