// Copyright 2023 The LUCI Authors.
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

package protoutil

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestConvertTestFailureAnalysisToPb(t *testing.T) {
	t.Parallel()
	ftt.Run("TestConvertTestFailureAnalysisToPb", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		testutil.UpdateIndices(ctx)
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:              int64(100),
			Project:         "chromium",
			Bucket:          "ci",
			Builder:         "linux-rel",
			StartCommitHash: "start_commit_hash",
			EndCommitHash:   "end_commit_hash",
			FailedBuildID:   8000,
			CreateTime:      time.Unix(int64(100), 0).UTC(),
			StartTime:       time.Unix(int64(110), 0).UTC(),
			EndTime:         time.Unix(int64(120), 0).UTC(),
			TestFailureKey:  datastore.MakeKey(ctx, "TestFailure", 100),
			Status:          pb.AnalysisStatus_FOUND,
			RunStatus:       pb.AnalysisRunStatus_ENDED,
		})
		// non-primary test failure.
		testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
			ID:        int64(99),
			Analysis:  tfa,
			TestID:    "testID2",
			StartHour: time.Unix(int64(100), 0).UTC(),
			Ref: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Ref:     "ref",
					},
				},
			},
			Project: "chromium",
			Variant: map[string]string{
				"key2": "val2",
			},
			VariantHash:      "vhash2",
			RefHash:          "refhash",
			StartPosition:    100,
			EndPosition:      199,
			StartFailureRate: 0,
			EndFailureRate:   1.0,
			IsDiverged:       true,
		})
		primaryTf := testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
			ID:        int64(100),
			Analysis:  tfa,
			IsPrimary: true,
			TestID:    "testID1",
			StartHour: time.Unix(int64(100), 0).UTC(),
			Ref: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Ref:     "ref",
					},
				},
			},
			Project: "chromium",
			Variant: map[string]string{
				"key1": "val1",
			},
			VariantHash:      "vhash1",
			RefHash:          "refhash",
			StartPosition:    100,
			EndPosition:      199,
			StartFailureRate: 0,
			EndFailureRate:   1.0,
		})
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
			ID:                200,
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
			BlameList:         testutil.CreateBlamelist(4),
			Status:            pb.AnalysisStatus_SUSPECTFOUND,
			RunStatus:         pb.AnalysisRunStatus_ENDED,
			StartTime:         time.Unix(int64(100), 0).UTC(),
			EndTime:           time.Unix(int64(109), 0).UTC(),
		})
		culprit := testutil.CreateSuspect(ctx, t, &testutil.SuspectCreationOption{
			ID:                 500,
			ParentKey:          datastore.KeyForObj(ctx, nsa),
			CommitID:           "culprit_commit_id",
			ReviewURL:          "review_url",
			ReviewTitle:        "review_title",
			SuspectRerunKey:    datastore.MakeKey(ctx, "Suspect", 3000),
			ParentRerunKey:     datastore.MakeKey(ctx, "Suspect", 3001),
			VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
			ActionDetails: model.ActionDetails{
				IsRevertCreated:  true,
				RevertURL:        "http://revert",
				RevertCreateTime: time.Unix(120, 0).UTC(),
			},
		})

		// Suspect verification rerun.
		testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
			ID:          3000,
			AnalysisKey: datastore.KeyForObj(ctx, tfa),
			CulpritKey:  datastore.KeyForObj(ctx, culprit),
			Type:        model.RerunBuildType_CulpritVerification,
			CreateTime:  time.Unix(103, 0).UTC(),
			StartTime:   time.Unix(104, 0).UTC(),
			ReportTime:  time.Unix(105, 0).UTC(),
			EndTime:     time.Unix(106, 0).UTC(),
			Status:      pb.RerunStatus_RERUN_STATUS_FAILED,
			BuildStatus: buildbucketpb.Status_SUCCESS,
			TestResult: model.RerunTestResults{
				IsFinalized: true,
				Results: []model.RerunSingleTestResult{
					{
						TestFailureKey:  datastore.KeyForObj(ctx, primaryTf),
						UnexpectedCount: 1,
					},
				},
			},
			GitilesCommit: &buildbucketpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "ref",
				Id:      "commit2",
			},
		})

		// Parent verification rerun.
		testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
			ID:          3001,
			AnalysisKey: datastore.KeyForObj(ctx, tfa),
			CulpritKey:  datastore.KeyForObj(ctx, culprit),
			Type:        model.RerunBuildType_CulpritVerification,
			CreateTime:  time.Unix(103, 0).UTC(),
			StartTime:   time.Unix(104, 0).UTC(),
			ReportTime:  time.Unix(105, 0).UTC(),
			EndTime:     time.Unix(106, 0).UTC(),
			Status:      pb.RerunStatus_RERUN_STATUS_PASSED,
			BuildStatus: buildbucketpb.Status_SUCCESS,
			TestResult: model.RerunTestResults{
				IsFinalized: true,
				Results: []model.RerunSingleTestResult{
					{
						TestFailureKey: datastore.KeyForObj(ctx, primaryTf),
						ExpectedCount:  1,
					},
				},
			},
			GitilesCommit: &buildbucketpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "ref",
				Id:      "commit3",
			},
		})

		// Nthsection rerun1.
		testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
			ID:                    2998,
			AnalysisKey:           datastore.KeyForObj(ctx, tfa),
			NthSectionAnalysisKey: datastore.KeyForObj(ctx, nsa),
			Type:                  model.RerunBuildType_NthSection,
			CreateTime:            time.Unix(103, 0).UTC(),
			StartTime:             time.Unix(104, 0).UTC(),
			ReportTime:            time.Unix(105, 0).UTC(),
			EndTime:               time.Unix(106, 0).UTC(),
			Status:                pb.RerunStatus_RERUN_STATUS_FAILED,
			BuildStatus:           buildbucketpb.Status_SUCCESS,
			TestResult: model.RerunTestResults{
				IsFinalized: true,
				Results: []model.RerunSingleTestResult{
					{
						TestFailureKey:  datastore.KeyForObj(ctx, primaryTf),
						UnexpectedCount: 1,
					},
				},
			},
			GitilesCommit: &buildbucketpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "ref",
				Id:      "commit2",
			},
		})

		// Nthsection rerun2.
		testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
			ID:                    2999,
			AnalysisKey:           datastore.KeyForObj(ctx, tfa),
			NthSectionAnalysisKey: datastore.KeyForObj(ctx, nsa),
			Type:                  model.RerunBuildType_NthSection,
			CreateTime:            time.Unix(104, 0).UTC(),
			StartTime:             time.Unix(105, 0).UTC(),
			ReportTime:            time.Unix(106, 0).UTC(),
			EndTime:               time.Unix(107, 0).UTC(),
			Status:                pb.RerunStatus_RERUN_STATUS_PASSED,
			BuildStatus:           buildbucketpb.Status_SUCCESS,
			TestResult: model.RerunTestResults{
				IsFinalized: true,
				Results: []model.RerunSingleTestResult{
					{
						TestFailureKey: datastore.KeyForObj(ctx, primaryTf),
						ExpectedCount:  1,
					},
				},
			},
			GitilesCommit: &buildbucketpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "ref",
				Id:      "commit3",
			},
		})

		tfa.VerifiedCulpritKey = datastore.KeyForObj(ctx, culprit)
		assert.Loosely(t, datastore.Put(ctx, tfa), should.BeNil)
		nsa.CulpritKey = datastore.KeyForObj(ctx, culprit)
		assert.Loosely(t, datastore.Put(ctx, nsa), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		t.Run("TestFailureAnalysisToPb", func(t *ftt.Test) {
			fieldMask := &fieldmaskpb.FieldMask{
				Paths: []string{"*"},
			}
			mask, err := mask.FromFieldMask(fieldMask, &pb.TestAnalysis{}, mask.AdvancedSemantics())
			assert.Loosely(t, err, should.BeNil)

			tfaProto, err := TestFailureAnalysisToPb(ctx, tfa, mask)
			assert.Loosely(t, err, should.BeNil)

			pbSuspectRerun := &pb.TestSingleRerun{
				Bbid:       3000,
				CreateTime: timestamppb.New(time.Unix(103, 0).UTC()),
				StartTime:  timestamppb.New(time.Unix(104, 0).UTC()),
				ReportTime: timestamppb.New(time.Unix(105, 0).UTC()),
				EndTime:    timestamppb.New(time.Unix(106, 0).UTC()),
				Index:      "2",
				Commit: &buildbucketpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit2",
				},
				RerunResult: &pb.RerunTestResults{
					RerunStatus: pb.RerunStatus_RERUN_STATUS_FAILED,
					Results: []*pb.RerunTestSingleResult{
						{
							TestId:          "testID1",
							VariantHash:     "vhash1",
							UnexpectedCount: 1,
						},
					},
				},
			}

			pbParentRerun := &pb.TestSingleRerun{
				Bbid:       3001,
				CreateTime: timestamppb.New(time.Unix(103, 0).UTC()),
				StartTime:  timestamppb.New(time.Unix(104, 0).UTC()),
				ReportTime: timestamppb.New(time.Unix(105, 0).UTC()),
				EndTime:    timestamppb.New(time.Unix(106, 0).UTC()),
				Index:      "3",
				Commit: &buildbucketpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit3",
				},
				RerunResult: &pb.RerunTestResults{
					RerunStatus: pb.RerunStatus_RERUN_STATUS_PASSED,
					Results: []*pb.RerunTestSingleResult{
						{
							TestId:        "testID1",
							VariantHash:   "vhash1",
							ExpectedCount: 1,
						},
					},
				},
			}

			pbNthsectionRerun1 := &pb.TestSingleRerun{
				Bbid:       2998,
				CreateTime: timestamppb.New(time.Unix(103, 0).UTC()),
				StartTime:  timestamppb.New(time.Unix(104, 0).UTC()),
				ReportTime: timestamppb.New(time.Unix(105, 0).UTC()),
				EndTime:    timestamppb.New(time.Unix(106, 0).UTC()),
				Index:      "2",
				Commit: &buildbucketpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit2",
				},
				RerunResult: &pb.RerunTestResults{
					RerunStatus: pb.RerunStatus_RERUN_STATUS_FAILED,
					Results: []*pb.RerunTestSingleResult{
						{
							TestId:          "testID1",
							VariantHash:     "vhash1",
							UnexpectedCount: 1,
						},
					},
				},
			}

			pbNthsectionRerun2 := &pb.TestSingleRerun{
				Bbid:       2999,
				CreateTime: timestamppb.New(time.Unix(104, 0).UTC()),
				StartTime:  timestamppb.New(time.Unix(105, 0).UTC()),
				ReportTime: timestamppb.New(time.Unix(106, 0).UTC()),
				EndTime:    timestamppb.New(time.Unix(107, 0).UTC()),
				Index:      "3",
				Commit: &buildbucketpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit3",
				},
				RerunResult: &pb.RerunTestResults{
					RerunStatus: pb.RerunStatus_RERUN_STATUS_PASSED,
					Results: []*pb.RerunTestSingleResult{
						{
							TestId:        "testID1",
							VariantHash:   "vhash1",
							ExpectedCount: 1,
						},
					},
				},
			}

			culpritPb := &pb.TestCulprit{
				ReviewUrl:   "review_url",
				ReviewTitle: "review_title",
				Commit: &buildbucketpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "culprit_commit_id",
				},
				CulpritAction: []*pb.CulpritAction{
					{
						ActionType:  pb.CulpritActionType_REVERT_CL_CREATED,
						RevertClUrl: "http://revert",
						ActionTime:  timestamppb.New(time.Unix(120, 0)),
					},
				},
				VerificationDetails: &pb.TestSuspectVerificationDetails{
					Status:       pb.SuspectVerificationStatus_CONFIRMED_CULPRIT,
					SuspectRerun: pbSuspectRerun,
					ParentRerun:  pbParentRerun,
				},
			}

			assert.Loosely(t, tfaProto, should.Match(&pb.TestAnalysis{
				AnalysisId: 100,
				Builder: &buildbucketpb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "linux-rel",
				},
				CreatedTime: timestamppb.New(time.Unix(int64(100), 0).UTC()),
				StartTime:   timestamppb.New(time.Unix(int64(110), 0).UTC()),
				EndTime:     timestamppb.New(time.Unix(int64(120), 0).UTC()),
				Status:      pb.AnalysisStatus_FOUND,
				RunStatus:   pb.AnalysisRunStatus_ENDED,
				StartCommit: &buildbucketpb.GitilesCommit{
					Host:     "chromium.googlesource.com",
					Project:  "chromium/src",
					Ref:      "ref",
					Id:       "start_commit_hash",
					Position: 100,
				},
				EndCommit: &buildbucketpb.GitilesCommit{
					Host:     "chromium.googlesource.com",
					Project:  "chromium/src",
					Ref:      "ref",
					Id:       "end_commit_hash",
					Position: 199,
				},
				SampleBbid: 8000,
				TestFailures: []*pb.TestFailure{
					{
						TestId:      "testID1",
						VariantHash: "vhash1",
						RefHash:     "refhash",
						Variant: &pb.Variant{
							Def: map[string]string{
								"key1": "val1",
							},
						},
						IsPrimary:                 true,
						StartHour:                 timestamppb.New(time.Unix(int64(100), 0).UTC()),
						StartUnexpectedResultRate: 0,
						EndUnexpectedResultRate:   1,
					},
					{
						TestId:      "testID2",
						VariantHash: "vhash2",
						RefHash:     "refhash",
						Variant: &pb.Variant{
							Def: map[string]string{
								"key2": "val2",
							},
						},
						IsDiverged:                true,
						StartHour:                 timestamppb.New(time.Unix(int64(100), 0).UTC()),
						StartUnexpectedResultRate: 0,
						EndUnexpectedResultRate:   1,
					},
				},
				NthSectionResult: &pb.TestNthSectionAnalysisResult{
					Status:    pb.AnalysisStatus_SUSPECTFOUND,
					RunStatus: pb.AnalysisRunStatus_ENDED,
					StartTime: timestamppb.New(time.Unix(int64(100), 0).UTC()),
					EndTime:   timestamppb.New(time.Unix(int64(109), 0).UTC()),
					BlameList: testutil.CreateBlamelist(4),
					Suspect:   culpritPb,
					Reruns: []*pb.TestSingleRerun{
						pbNthsectionRerun1, pbNthsectionRerun2,
					},
				},
				Culprit: culpritPb,
			}))
		})

		t.Run("Bisection overview field mask", func(t *ftt.Test) {
			fieldMask := &fieldmaskpb.FieldMask{
				Paths: []string{
					"analysis_id",
					"created_time",
					"start_time",
					"end_time",
					"status",
					"run_status",
					"builder",
					"culprit.commit",
					"culprit.review_url",
					"culprit.review_title",
					"culprit.culprit_action",
					"test_failures.*.is_primary",
					"test_failures.*.start_hour",
				},
			}
			mask, err := mask.FromFieldMask(fieldMask, &pb.TestAnalysis{}, mask.AdvancedSemantics())
			assert.Loosely(t, err, should.BeNil)

			tfaProto, err := TestFailureAnalysisToPb(ctx, tfa, mask)
			assert.Loosely(t, err, should.BeNil)

			culpritPb := &pb.TestCulprit{
				ReviewUrl:   "review_url",
				ReviewTitle: "review_title",
				Commit: &buildbucketpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "culprit_commit_id",
				},
				CulpritAction: []*pb.CulpritAction{
					{
						ActionType:  pb.CulpritActionType_REVERT_CL_CREATED,
						RevertClUrl: "http://revert",
						ActionTime:  timestamppb.New(time.Unix(120, 0)),
					},
				},
			}

			assert.Loosely(t, tfaProto, should.Match(&pb.TestAnalysis{
				AnalysisId: 100,
				Builder: &buildbucketpb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "linux-rel",
				},
				CreatedTime: timestamppb.New(time.Unix(int64(100), 0).UTC()),
				StartTime:   timestamppb.New(time.Unix(int64(110), 0).UTC()),
				EndTime:     timestamppb.New(time.Unix(int64(120), 0).UTC()),
				Status:      pb.AnalysisStatus_FOUND,
				RunStatus:   pb.AnalysisRunStatus_ENDED,
				TestFailures: []*pb.TestFailure{
					{
						IsPrimary: true,
						StartHour: timestamppb.New(time.Unix(int64(100), 0).UTC()),
					},
					{
						StartHour: timestamppb.New(time.Unix(int64(100), 0).UTC()),
					},
				},
				Culprit: culpritPb,
			}))
		})
	})
}

func TestRegressionRange(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	ftt.Run("TestRegressionRange", t, func(t *ftt.Test) {
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{})
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
			ID:                200,
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
			BlameList:         testutil.CreateBlamelist(4),
			Status:            pb.AnalysisStatus_RUNNING,
			RunStatus:         pb.AnalysisRunStatus_STARTED,
		})
		sourceRef := &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
				},
			},
		}
		nsaFieldMask := fieldmaskpb.FieldMask{
			Paths: []string{"*"},
		}
		nsaMask, err := mask.FromFieldMask(&nsaFieldMask, &pb.TestNthSectionAnalysisResult{}, mask.AdvancedSemantics())
		assert.Loosely(t, err, should.BeNil)
		pbNsa, err := NthSectionAnalysisToPb(ctx, tfa, nsa, sourceRef, nsaMask)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pbNsa.RemainingNthSectionRange, should.Match(&pb.RegressionRange{
			LastPassed: &buildbucketpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "ref",
				Id:      "commit4",
			},
			FirstFailed: &buildbucketpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "ref",
				Id:      "commit0",
			},
		}))

		// Add a rerun, regression should update.
		testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
			AnalysisKey:           datastore.KeyForObj(ctx, tfa),
			NthSectionAnalysisKey: datastore.KeyForObj(ctx, nsa),
			Type:                  model.RerunBuildType_NthSection,
			Status:                pb.RerunStatus_RERUN_STATUS_PASSED,
			BuildStatus:           buildbucketpb.Status_SUCCESS,
			GitilesCommit: &buildbucketpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "ref",
				Id:      "commit2",
			},
		})

		pbNsa, err = NthSectionAnalysisToPb(ctx, tfa, nsa, sourceRef, nsaMask)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pbNsa.RemainingNthSectionRange, should.Match(&pb.RegressionRange{
			LastPassed: &buildbucketpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "ref",
				Id:      "commit2",
			},
			FirstFailed: &buildbucketpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "ref",
				Id:      "commit0",
			},
		}))
	})
}
