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
	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMakeAttempt(t *testing.T) {
	Convey("makeAttempt", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()
		epoch := ct.Clock.Now().UTC()
		const (
			lProject      = "infra"
			bbHost        = "cr-buildbucket.appspot.com"
			gHost         = "foo-review.googlesource.com"
			gRepo         = "test/repo"
			gRef          = "refs/head/main"
			gChange       = 101
			gPatchset     = 47
			gEquiPatchset = 42
			gBuildID1     = 100001
			gBuildID2     = 100002
			gBuildID3     = 100003
			gBuildID4     = 100004
		)

		plainBuilder := &buildbucketpb.BuilderID{
			Project: lProject,
			Bucket:  "try",
			Builder: "plain",
		}
		reuseDisabledBuilder := &buildbucketpb.BuilderID{
			Project: lProject,
			Bucket:  "try",
			Builder: "disable-reuse",
		}
		optionalBuilder := &buildbucketpb.BuilderID{
			Project: lProject,
			Bucket:  "try",
			Builder: "optional",
		}
		runID := common.MakeRunID(lProject, epoch, 1, []byte("aaa"))
		cfg := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					Verifiers: &cfgpb.Verifiers{
						Tryjob: &cfgpb.Verifiers_Tryjob{
							Builders: []*cfgpb.Verifiers_Tryjob_Builder{
								{
									Name: bbutil.FormatBuilderID(plainBuilder),
								},
								{
									Name:         bbutil.FormatBuilderID(reuseDisabledBuilder),
									DisableReuse: true,
								},
								{
									Name:                 bbutil.FormatBuilderID(optionalBuilder),
									ExperimentPercentage: 100,
								},
							},
						},
					},
				},
			},
		}
		prjcfgtest.Create(ctx, lProject, cfg)

		cl := &run.RunCL{
			ID:         gChange + 1000,
			Run:        datastore.MakeKey(ctx, common.RunKind, string(runID)),
			ExternalID: changelist.MustGobID(gHost, gChange),
			Detail: &changelist.Snapshot{
				LuciProject:           lProject,
				Patchset:              gPatchset,
				MinEquivalentPatchset: gEquiPatchset,
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: &gerritpb.ChangeInfo{
							Number:  gChange,
							Project: gRepo,
							Ref:     gRef,
							Owner: &gerritpb.AccountInfo{
								Name:  "Foo Bar",
								Email: "foobar@example.com",
							},
						},
					},
				},
			},
			Trigger: &run.Trigger{Time: timestamppb.New(epoch)},
		}

		r := &run.Run{
			ID:            common.RunID(runID),
			Status:        run.Status_SUCCEEDED,
			ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
			CreateTime:    epoch,
			StartTime:     epoch.Add(time.Minute * 2),
			EndTime:       epoch.Add(time.Minute * 25),
			CLs:           common.CLIDs{cl.ID},
			Mode:          run.FullRun,
			Submission: &run.Submission{
				Cls:          []int64{int64(cl.ID)},
				SubmittedCls: []int64{int64(cl.ID)},
			},
			Tryjobs: &run.Tryjobs{
				State: &tryjob.ExecutionState{
					Requirement: &tryjob.Requirement{
						Definitions: []*tryjob.Definition{
							{
								Backend: &tryjob.Definition_Buildbucket_{
									Buildbucket: &tryjob.Definition_Buildbucket{
										Host:    bbHost,
										Builder: plainBuilder,
									},
								},
								Critical: true,
							},
							{
								Backend: &tryjob.Definition_Buildbucket_{
									Buildbucket: &tryjob.Definition_Buildbucket{
										Host:    bbHost,
										Builder: reuseDisabledBuilder,
									},
								},
								DisableReuse: true,
								Critical:     true,
							},
							{
								Backend: &tryjob.Definition_Buildbucket_{
									Buildbucket: &tryjob.Definition_Buildbucket{
										Host:    bbHost,
										Builder: optionalBuilder,
									},
								},
								Optional: true,
								Critical: false,
							},
						},
					},
					Executions: []*tryjob.ExecutionState_Execution{
						{
							Attempts: []*tryjob.ExecutionState_Execution_Attempt{
								{
									ExternalId: string(tryjob.MustBuildbucketID(bbHost, gBuildID4)),
									Status:     tryjob.Status_ENDED,
									Reused:     true,
								},
								{
									ExternalId: string(tryjob.MustBuildbucketID(bbHost, gBuildID1)),
									Status:     tryjob.Status_ENDED,
								},
							},
						},
						{
							Attempts: []*tryjob.ExecutionState_Execution_Attempt{
								{
									ExternalId: string(tryjob.MustBuildbucketID(bbHost, gBuildID2)),
									Status:     tryjob.Status_ENDED,
								},
							},
						},
						{
							Attempts: []*tryjob.ExecutionState_Execution_Attempt{
								{
									// tryjob not triggered so external id is missing.
									ExternalId: "",
									Status:     tryjob.Status_UNTRIGGERED,
								},
								{
									ExternalId: string(tryjob.MustBuildbucketID(bbHost, gBuildID3)),
									Status:     tryjob.Status_ENDED,
								},
							},
						},
					},
					Status: tryjob.ExecutionState_SUCCEEDED,
				},
			},
		}

		Convey("All fields", func() {
			a, err := makeAttempt(ctx, r, []*run.RunCL{cl})
			So(err, ShouldBeNil)
			So(a, ShouldResembleProto, &cvbqpb.Attempt{
				Key:                  runID.AttemptKey(),
				LuciProject:          lProject,
				ConfigGroup:          cfg.GetConfigGroups()[0].GetName(),
				ClGroupKey:           "2fb6f02ce54ceef7",
				EquivalentClGroupKey: "b5aefc068a978ddc",
				StartTime:            timestamppb.New(epoch),
				ActualStartTime:      timestamppb.New(epoch.Add(2 * time.Minute)),
				EndTime:              timestamppb.New(epoch.Add(25 * time.Minute)),
				Status:               cvbqpb.AttemptStatus_SUCCESS,
				Substatus:            cvbqpb.AttemptSubstatus_NO_SUBSTATUS,
				GerritChanges: []*cvbqpb.GerritChange{
					{
						Host:                       gHost,
						Project:                    gRepo,
						Change:                     gChange,
						Patchset:                   gPatchset,
						EarliestEquivalentPatchset: gEquiPatchset,
						TriggerTime:                timestamppb.New(epoch),
						Mode:                       cvbqpb.Mode_FULL_RUN,
						SubmitStatus:               cvbqpb.GerritChange_SUCCESS,
						IsOwnerBot:                 false,
					},
				},
				Builds: []*cvbqpb.Build{
					{
						Host:     bbHost,
						Id:       gBuildID1,
						Origin:   cvbqpb.Build_NOT_REUSED,
						Critical: true,
					},
					{
						Host:     bbHost,
						Id:       gBuildID2,
						Origin:   cvbqpb.Build_NOT_REUSABLE,
						Critical: true,
					},
					{
						Host:     bbHost,
						Id:       gBuildID3,
						Origin:   cvbqpb.Build_NOT_REUSED,
						Critical: false,
					},
					{
						Host:     bbHost,
						Id:       gBuildID4,
						Origin:   cvbqpb.Build_REUSED,
						Critical: true,
					},
				},
			})
		})

		Convey("Partial submission", func() {
			clSubmitted := &run.RunCL{
				ID:         1,
				Run:        datastore.MakeKey(ctx, common.RunKind, string(runID)),
				ExternalID: changelist.MustGobID(gHost, 1),
				Detail: &changelist.Snapshot{
					LuciProject:           lProject,
					Patchset:              11,
					MinEquivalentPatchset: 11,
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: &gerritpb.ChangeInfo{
								Number:  1,
								Project: gRepo,
								Ref:     gRef,
							},
						},
					},
				},
				Trigger: &run.Trigger{Time: timestamppb.New(epoch)},
			}
			clFailedToSubmit := &run.RunCL{
				ID:         2,
				Run:        datastore.MakeKey(ctx, common.RunKind, string(runID)),
				ExternalID: changelist.MustGobID(gHost, 2),
				Detail: &changelist.Snapshot{
					LuciProject:           lProject,
					Patchset:              22,
					MinEquivalentPatchset: 22,
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: &gerritpb.ChangeInfo{
								Number:  2,
								Project: gRepo,
								Ref:     gRef,
							},
						},
					},
				},
				Trigger: &run.Trigger{Time: timestamppb.New(epoch)},
			}
			clPendingToSubmit := &run.RunCL{
				ID:         3,
				Run:        datastore.MakeKey(ctx, common.RunKind, string(runID)),
				ExternalID: changelist.MustGobID(gHost, 3),
				Detail: &changelist.Snapshot{
					LuciProject:           lProject,
					Patchset:              33,
					MinEquivalentPatchset: 33,
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: &gerritpb.ChangeInfo{
								Number:  3,
								Project: gRepo,
								Ref:     gRef,
							},
						},
					},
				},
				Trigger: &run.Trigger{Time: timestamppb.New(epoch)},
			}
			r.CLs = common.CLIDs{clSubmitted.ID, clFailedToSubmit.ID, clPendingToSubmit.ID}
			r.Status = run.Status_FAILED
			r.Submission = &run.Submission{
				Cls: []int64{
					int64(clSubmitted.ID),
					int64(clFailedToSubmit.ID),
					int64(clPendingToSubmit.ID),
				},
				SubmittedCls: []int64{int64(clSubmitted.ID)},
				FailedCls:    []int64{int64(clFailedToSubmit.ID)},
			}

			a, err := makeAttempt(ctx, r, []*run.RunCL{
				clSubmitted, clFailedToSubmit, clPendingToSubmit,
			})
			So(err, ShouldBeNil)
			So(a.GetGerritChanges(), ShouldResembleProto, []*cvbqpb.GerritChange{
				{
					Host:                       gHost,
					Project:                    gRepo,
					Change:                     1,
					Patchset:                   11,
					EarliestEquivalentPatchset: 11,
					TriggerTime:                timestamppb.New(epoch),
					Mode:                       cvbqpb.Mode_FULL_RUN,
					SubmitStatus:               cvbqpb.GerritChange_SUCCESS,
				},
				{
					Host:                       gHost,
					Project:                    gRepo,
					Change:                     2,
					Patchset:                   22,
					EarliestEquivalentPatchset: 22,
					TriggerTime:                timestamppb.New(epoch),
					Mode:                       cvbqpb.Mode_FULL_RUN,
					SubmitStatus:               cvbqpb.GerritChange_FAILURE,
				},
				{
					Host:                       gHost,
					Project:                    gRepo,
					Change:                     3,
					Patchset:                   33,
					EarliestEquivalentPatchset: 33,
					TriggerTime:                timestamppb.New(epoch),
					Mode:                       cvbqpb.Mode_FULL_RUN,
					SubmitStatus:               cvbqpb.GerritChange_PENDING,
				},
			})
			// In the case of submit failure for one or more CLs,
			// the Attempt value is still SUCCESS, for backwards
			// compatibility.
			So(a.Status, ShouldEqual, cvbqpb.AttemptStatus_SUCCESS)
			So(a.Substatus, ShouldEqual, cvbqpb.AttemptSubstatus_NO_SUBSTATUS)
		})

		Convey("Failed Tryjob", func() {
			r.Tryjobs.GetState().Status = tryjob.ExecutionState_FAILED
			r.Status = run.Status_FAILED
			a, err := makeAttempt(ctx, r, []*run.RunCL{cl})
			So(err, ShouldBeNil)
			So(a.Status, ShouldEqual, cvbqpb.AttemptStatus_FAILURE)
			So(a.Substatus, ShouldEqual, cvbqpb.AttemptSubstatus_FAILED_TRYJOBS)
		})

		Convey("Failed due to missing approval", func() {
			// TODO(crbug/1342810): Populate run failure reason
			r.Status = run.Status_FAILED
			a, err := makeAttempt(ctx, r, []*run.RunCL{cl})
			So(err, ShouldBeNil)
			So(a.Status, ShouldEqual, cvbqpb.AttemptStatus_FAILURE)
			So(a.Substatus, ShouldEqual, cvbqpb.AttemptSubstatus_UNAPPROVED)
		})

		Convey("Cancelled", func() {
			// TODO(crbug/1342810): Populate run failure reason
			r.Status = run.Status_CANCELLED
			a, err := makeAttempt(ctx, r, []*run.RunCL{cl})
			So(err, ShouldBeNil)
			So(a.Status, ShouldEqual, cvbqpb.AttemptStatus_ABORTED)
			So(a.Substatus, ShouldEqual, cvbqpb.AttemptSubstatus_MANUAL_CANCEL)
		})

		Convey("Empty actual start time", func() {
			r.StartTime = time.Time{}
			a, err := makeAttempt(ctx, r, []*run.RunCL{cl})
			So(err, ShouldBeNil)
			So(a.GetActualStartTime(), ShouldBeNil)
		})

		Convey("HasCustomRequirement", func() {
			r.Options = &run.Options{
				IncludedTryjobs: []string{fmt.Sprintf("%s/try: cool-builder", lProject)},
			}
			a, err := makeAttempt(ctx, r, []*run.RunCL{cl})
			So(err, ShouldBeNil)
			So(a.GetHasCustomRequirement(), ShouldBeTrue)
		})

		Convey("Owner is bot", func() {
			Convey("tagged with service user", func() {
				cl.Detail.GetGerrit().GetInfo().GetOwner().Tags = []string{"SERVICE_USER"}
				a, err := makeAttempt(ctx, r, []*run.RunCL{cl})
				So(err, ShouldBeNil)
				So(a.GerritChanges[0].IsOwnerBot, ShouldBeTrue)
			})
			Convey("domain is prod.google.com", func() {
				cl.Detail.GetGerrit().GetInfo().GetOwner().Email = "abc@prod.google.com"
				a, err := makeAttempt(ctx, r, []*run.RunCL{cl})
				So(err, ShouldBeNil)
				So(a.GerritChanges[0].IsOwnerBot, ShouldBeTrue)
			})
			Convey("domain is gserviceaccount.com", func() {
				cl.Detail.GetGerrit().GetInfo().GetOwner().Email = "xyz@proj-foo.iam.gserviceaccount.com"
				a, err := makeAttempt(ctx, r, []*run.RunCL{cl})
				So(err, ShouldBeNil)
				So(a.GerritChanges[0].IsOwnerBot, ShouldBeTrue)
			})
		})

		Convey("Multi CL Run with root CL", func() {
			anotherCL := &run.RunCL{
				ID:         cl.ID + 1,
				Run:        datastore.MakeKey(ctx, common.RunKind, string(runID)),
				ExternalID: changelist.MustGobID(gHost, gChange+1),
				Detail: &changelist.Snapshot{
					LuciProject:           lProject,
					Patchset:              gPatchset + 1,
					MinEquivalentPatchset: gEquiPatchset + 1,
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: &gerritpb.ChangeInfo{
								Number:  gChange + 1,
								Project: gRepo,
								Ref:     gRef,
								Owner: &gerritpb.AccountInfo{
									Name:  "Foo Bar",
									Email: "foobar@example.com",
								},
							},
						},
					},
				},
				Trigger: nil, // empty trigger for readability
			}
			r.CLs = append(r.CLs, anotherCL.ID)
			r.RootCL = cl.ID
			r.Submission.Cls = append(r.Submission.Cls, int64(anotherCL.ID))
			r.Submission.SubmittedCls = append(r.Submission.SubmittedCls, int64(anotherCL.ID))
			So(datastore.Put(ctx, anotherCL, r), ShouldBeNil)
			a, err := makeAttempt(ctx, r, []*run.RunCL{cl, anotherCL})
			So(err, ShouldBeNil)
			So(a.GetGerritChanges(), ShouldResembleProto, []*cvbqpb.GerritChange{
				{
					Host:                       gHost,
					Project:                    gRepo,
					Change:                     gChange,
					Patchset:                   gPatchset,
					EarliestEquivalentPatchset: gEquiPatchset,
					TriggerTime:                timestamppb.New(epoch),
					Mode:                       cvbqpb.Mode_FULL_RUN,
					SubmitStatus:               cvbqpb.GerritChange_SUCCESS,
				},
				{
					Host:                       gHost,
					Project:                    gRepo,
					Change:                     gChange + 1,
					Patchset:                   gPatchset + 1,
					EarliestEquivalentPatchset: gEquiPatchset + 1,
					TriggerTime:                timestamppb.New(epoch),
					Mode:                       cvbqpb.Mode_FULL_RUN,
					SubmitStatus:               cvbqpb.GerritChange_SUCCESS,
				},
			})
		})
	})
}
