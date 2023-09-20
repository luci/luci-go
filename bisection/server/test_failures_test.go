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

package server

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestConvertTestFailureAnalysisToPb(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	Convey("TestFailureAnalysisToPb", t, func() {
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
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
		testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
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
		testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
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
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
			ID:                200,
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
			BlameList:         testutil.CreateBlamelist(3),
			Status:            pb.AnalysisStatus_SUSPECTFOUND,
			RunStatus:         pb.AnalysisRunStatus_ENDED,
			StartTime:         time.Unix(int64(100), 0).UTC(),
			EndTime:           time.Unix(int64(109), 0).UTC(),
		})
		culprit := testutil.CreateSuspect(ctx, &testutil.SuspectCreationOption{
			ID:          500,
			ParentKey:   datastore.KeyForObj(ctx, nsa),
			CommitID:    "culprit_commit_id",
			ReviewURL:   "review_url",
			ReviewTitle: "review_title",
		})
		tfa.VerifiedCulpritKey = datastore.KeyForObj(ctx, culprit)
		So(datastore.Put(ctx, tfa), ShouldBeNil)
		nsa.CulpritKey = datastore.KeyForObj(ctx, culprit)
		So(datastore.Put(ctx, nsa), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		tfaProto, err := TestFailureAnalysisToPb(ctx, tfa)
		So(err, ShouldBeNil)
		So(tfaProto, ShouldResembleProto, &pb.TestAnalysis{
			AnalysisId: 100,
			Builder: &buildbucketpb.BuilderID{
				Project: "chromium",
				Bucket:  "ci",
				Builder: "linux-rel",
			},
			CreateTime: timestamppb.New(time.Unix(int64(100), 0).UTC()),
			StartTime:  timestamppb.New(time.Unix(int64(110), 0).UTC()),
			EndTime:    timestamppb.New(time.Unix(int64(120), 0).UTC()),
			Status:     pb.AnalysisStatus_FOUND,
			RunStatus:  pb.AnalysisRunStatus_ENDED,
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
			SampleBbid:       8000,
			StartFailureRate: 0,
			EndFailureRate:   1.0,
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
					IsPrimary: true,
					StartHour: timestamppb.New(time.Unix(int64(100), 0).UTC()),
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
					IsDiverged: true,
					StartHour:  timestamppb.New(time.Unix(int64(100), 0).UTC()),
				},
			},
			NthSectionResult: &pb.TestNthSectionAnalysisResult{
				Status:    pb.AnalysisStatus_SUSPECTFOUND,
				RunStatus: pb.AnalysisRunStatus_ENDED,
				StartTime: timestamppb.New(time.Unix(int64(100), 0).UTC()),
				EndTime:   timestamppb.New(time.Unix(int64(109), 0).UTC()),
				BlameList: testutil.CreateBlamelist(3),
			},
			VerifiedCulprit: &pb.TestCulprit{
				ReviewUrl:   "review_url",
				ReviewTitle: "review_title",
				Commit: &buildbucketpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "culprit_commit_id",
				},
			},
		})
	})
}
