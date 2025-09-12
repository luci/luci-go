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
	"fmt"
	"net/url"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/bisection/analysis"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
)

// mockCheckAccessAnalyses is a test ACL function that allows all requests
func mockCheckAccessAnalyses(ctx context.Context, methodName string, req proto.Message) (context.Context, error) {
	return ctx, nil // Allow all requests in tests
}

func TestQueryAnalysis(t *testing.T) {
	t.Parallel()
	server := &AnalysesServer{ACL: mockCheckAccessAnalyses}
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	datastore.GetTestable(c).AutoIndex(true)

	ftt.Run("No BuildFailure Info", t, func(t *ftt.Test) {
		req := &pb.QueryAnalysisRequest{}
		_, err := server.QueryAnalysis(c, req)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.InvalidArgument))
	})

	ftt.Run("No bbid", t, func(t *ftt.Test) {
		req := &pb.QueryAnalysisRequest{BuildFailure: &pb.BuildFailure{}}
		_, err := server.QueryAnalysis(c, req)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.InvalidArgument))
	})

	ftt.Run("Unsupported step", t, func(t *ftt.Test) {
		req := &pb.QueryAnalysisRequest{
			BuildFailure: &pb.BuildFailure{
				FailedStepName: "some step",
				Bbid:           123,
			},
		}
		_, err := server.QueryAnalysis(c, req)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.Unimplemented))
	})

	ftt.Run("No analysis found", t, func(t *ftt.Test) {
		req := &pb.QueryAnalysisRequest{
			BuildFailure: &pb.BuildFailure{
				FailedStepName: "compile",
				Bbid:           123,
			},
		}
		_, err := server.QueryAnalysis(c, req)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.NotFound))
	})

	ftt.Run("Analysis found", t, func(t *ftt.Test) {
		// Prepares datastore
		failedBuild := &model.LuciFailedBuild{
			Id: 123,
			LuciBuild: model.LuciBuild{
				Project: "chromium/test",
				Bucket:  "ci",
				Builder: "android",
			},
			BuildFailureType: pb.BuildFailureType_COMPILE,
		}
		assert.Loosely(t, datastore.Put(c, failedBuild), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compileFailure := &model.CompileFailure{
			Id:    123,
			Build: datastore.KeyForObj(c, failedBuild),
		}
		assert.Loosely(t, datastore.Put(c, compileFailure), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect := &model.Suspect{
			GitilesCommit: buildbucketpb.GitilesCommit{
				Host:    "host1",
				Project: "proj1",
				Id:      "commit5",
			},
			ReviewUrl:   "http://this/is/review/url",
			ReviewTitle: "This is review title",
			ActionDetails: model.ActionDetails{
				RevertURL:         "https://this/is/revert/review/url",
				IsRevertCreated:   true,
				RevertCreateTime:  (&timestamppb.Timestamp{Seconds: 100}).AsTime(),
				IsRevertCommitted: true,
				RevertCommitTime:  (&timestamppb.Timestamp{Seconds: 200}).AsTime(),
			},
			VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
			Score:              100,
			Justification:      "Justification",
		}
		assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compileFailureAnalysis := &model.CompileFailureAnalysis{
			CompileFailure:     datastore.KeyForObj(c, compileFailure),
			FirstFailedBuildId: 119,
			InitialRegressionRange: &pb.RegressionRange{
				LastPassed: &buildbucketpb.GitilesCommit{
					Host:    "host1",
					Project: "proj1",
					Ref:     "ref",
					Id:      "commit9",
				},
				FirstFailed: &buildbucketpb.GitilesCommit{
					Host:    "host1",
					Project: "proj1",
					Ref:     "ref",
					Id:      "commit0",
				},
			},
		}
		assert.Loosely(t, datastore.Put(c, compileFailureAnalysis), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		heuristicAnalysis := &model.CompileHeuristicAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, compileFailureAnalysis),
		}
		assert.Loosely(t, datastore.Put(c, heuristicAnalysis), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		// Create nth section analysis
		nsa := &model.CompileNthSectionAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, compileFailureAnalysis),
			Status:         pb.AnalysisStatus_FOUND,
			RunStatus:      pb.AnalysisRunStatus_ENDED,
			StartTime:      (&timestamppb.Timestamp{Seconds: 100}).AsTime(),
			EndTime:        (&timestamppb.Timestamp{Seconds: 102}).AsTime(),
			BlameList:      testutil.CreateBlamelist(10),
		}
		assert.Loosely(t, datastore.Put(c, nsa), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		// Create suspect for nthsection
		nthSectionSuspect := &model.Suspect{
			GitilesCommit: buildbucketpb.GitilesCommit{
				Host:    "host1",
				Project: "proj1",
				Id:      "commit6",
			},
			ReviewUrl:          "http://this/is/review/url1",
			ReviewTitle:        "This is review title1",
			VerificationStatus: model.SuspectVerificationStatus_Vindicated,
			ParentAnalysis:     datastore.KeyForObj(c, nsa),
		}
		assert.Loosely(t, datastore.Put(c, nthSectionSuspect), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		// Add culprit verification rerun build for suspect
		suspectRerunBuild := &model.CompileRerunBuild{
			Id: 8877665544332211,
			LuciBuild: model.LuciBuild{
				BuildId:    8877665544332211,
				Project:    "chromium",
				Bucket:     "findit",
				Builder:    "luci-bisection-single-revision",
				CreateTime: (&timestamppb.Timestamp{Seconds: 100}).AsTime(),
				StartTime:  (&timestamppb.Timestamp{Seconds: 101}).AsTime(),
				EndTime:    (&timestamppb.Timestamp{Seconds: 102}).AsTime(),
				Status:     buildbucketpb.Status_FAILURE,
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "host1",
					Project: "proj1",
					Ref:     "ref",
					Id:      "commit5",
				},
			},
		}
		assert.Loosely(t, datastore.Put(c, suspectRerunBuild), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		// Add culprit verification single rerun for suspect
		suspectSingleRerun := &model.SingleRerun{
			Analysis:   datastore.KeyForObj(c, compileFailureAnalysis),
			Suspect:    datastore.KeyForObj(c, suspect),
			RerunBuild: datastore.KeyForObj(c, suspectRerunBuild),
			Status:     pb.RerunStatus_RERUN_STATUS_FAILED,
			GitilesCommit: buildbucketpb.GitilesCommit{
				Host:    "host1",
				Project: "proj1",
				Ref:     "ref",
				Id:      "commit5",
			},
			CreateTime: (&timestamppb.Timestamp{Seconds: 100}).AsTime(),
			StartTime:  (&timestamppb.Timestamp{Seconds: 101}).AsTime(),
			EndTime:    (&timestamppb.Timestamp{Seconds: 102}).AsTime(),
			Type:       model.RerunBuildType_CulpritVerification,
			Priority:   100,
		}
		assert.Loosely(t, datastore.Put(c, suspectSingleRerun), should.BeNil)

		// Add culprit verification rerun build for parent of suspect
		parentRerunBuild := &model.CompileRerunBuild{
			Id: 7766554433221100,
			LuciBuild: model.LuciBuild{
				BuildId:    7766554433221100,
				Project:    "chromium",
				Bucket:     "findit",
				Builder:    "luci-bisection-single-revision",
				CreateTime: (&timestamppb.Timestamp{Seconds: 200}).AsTime(),
				StartTime:  (&timestamppb.Timestamp{Seconds: 201}).AsTime(),
				EndTime:    (&timestamppb.Timestamp{Seconds: 202}).AsTime(),
				Status:     buildbucketpb.Status_SUCCESS,
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "host1",
					Project: "proj1",
					Ref:     "ref",
					Id:      "commit6",
				},
			},
		}
		assert.Loosely(t, datastore.Put(c, parentRerunBuild), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		// Add culprit verification single rerun for parent of suspect
		parentSingleRerun := &model.SingleRerun{
			Analysis:   datastore.KeyForObj(c, compileFailureAnalysis),
			Suspect:    datastore.KeyForObj(c, suspect),
			RerunBuild: datastore.KeyForObj(c, parentRerunBuild),
			Status:     pb.RerunStatus_RERUN_STATUS_PASSED,
			GitilesCommit: buildbucketpb.GitilesCommit{
				Host:    "host1",
				Project: "proj1",
				Ref:     "ref",
				Id:      "commit6",
			},
			CreateTime: (&timestamppb.Timestamp{Seconds: 200}).AsTime(),
			StartTime:  (&timestamppb.Timestamp{Seconds: 201}).AsTime(),
			EndTime:    (&timestamppb.Timestamp{Seconds: 202}).AsTime(),
			Type:       model.RerunBuildType_CulpritVerification,
			Priority:   100,
		}
		assert.Loosely(t, datastore.Put(c, parentSingleRerun), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		// Update suspect's culprit verification results
		suspect.ParentAnalysis = datastore.KeyForObj(c, heuristicAnalysis)
		suspect.VerificationStatus = model.SuspectVerificationStatus_ConfirmedCulprit
		suspect.SuspectRerunBuild = datastore.KeyForObj(c, suspectRerunBuild)
		suspect.ParentRerunBuild = datastore.KeyForObj(c, parentRerunBuild)
		assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compileFailureAnalysis.VerifiedCulprits = []*datastore.Key{
			datastore.KeyForObj(c, suspect),
		}
		assert.Loosely(t, datastore.Put(c, compileFailureAnalysis), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		// nth section rerun
		rrBuild := &model.CompileRerunBuild{
			Id: 800999000,
		}
		assert.Loosely(t, datastore.Put(c, rrBuild), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		nthSectionRerun := &model.SingleRerun{
			Analysis:           datastore.KeyForObj(c, compileFailureAnalysis),
			NthSectionAnalysis: datastore.KeyForObj(c, nsa),
			Status:             pb.RerunStatus_RERUN_STATUS_PASSED,
			GitilesCommit: buildbucketpb.GitilesCommit{
				Host:    "host1",
				Project: "proj1",
				Ref:     "ref",
				Id:      "commit8",
			},
			RerunBuild: datastore.KeyForObj(c, rrBuild),
			CreateTime: (&timestamppb.Timestamp{Seconds: 300}).AsTime(),
			StartTime:  (&timestamppb.Timestamp{Seconds: 301}).AsTime(),
			EndTime:    (&timestamppb.Timestamp{Seconds: 302}).AsTime(),
			Type:       model.RerunBuildType_NthSection,
			Priority:   100,
		}

		assert.Loosely(t, datastore.Put(c, nthSectionRerun), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		req := &pb.QueryAnalysisRequest{
			BuildFailure: &pb.BuildFailure{
				FailedStepName: "compile",
				Bbid:           123,
			},
		}

		res, err := server.QueryAnalysis(c, req)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(res.Analyses), should.Equal(1))

		analysis := res.Analyses[0]
		assert.Loosely(t, analysis.Builder, should.Match(&buildbucketpb.BuilderID{
			Project: "chromium/test",
			Bucket:  "ci",
			Builder: "android",
		}))
		assert.Loosely(t, analysis.BuildFailureType, should.Equal(pb.BuildFailureType_COMPILE))
		assert.Loosely(t, len(analysis.Culprits), should.Equal(1))
		assert.Loosely(t, proto.Equal(analysis.Culprits[0], &pb.Culprit{
			Commit: &buildbucketpb.GitilesCommit{
				Host:    "host1",
				Project: "proj1",
				Id:      "commit5",
			},
			ReviewUrl:   "http://this/is/review/url",
			ReviewTitle: "This is review title",
			CulpritAction: []*pb.CulpritAction{
				{
					ActionType:  pb.CulpritActionType_CULPRIT_AUTO_REVERTED,
					RevertClUrl: "https://this/is/revert/review/url",
					ActionTime:  &timestamppb.Timestamp{Seconds: 200},
				},
			},
			VerificationDetails: &pb.SuspectVerificationDetails{
				Status: string(model.SuspectVerificationStatus_ConfirmedCulprit),
				SuspectRerun: &pb.SingleRerun{
					Bbid:      8877665544332211,
					StartTime: &timestamppb.Timestamp{Seconds: 101},
					EndTime:   &timestamppb.Timestamp{Seconds: 102},
					RerunResult: &pb.RerunResult{
						RerunStatus: pb.RerunStatus_RERUN_STATUS_FAILED,
					},
					Commit: &buildbucketpb.GitilesCommit{
						Host:    "host1",
						Project: "proj1",
						Ref:     "ref",
						Id:      "commit5",
					},
				},
				ParentRerun: &pb.SingleRerun{
					Bbid:      7766554433221100,
					StartTime: &timestamppb.Timestamp{Seconds: 201},
					EndTime:   &timestamppb.Timestamp{Seconds: 202},
					RerunResult: &pb.RerunResult{
						RerunStatus: pb.RerunStatus_RERUN_STATUS_PASSED,
					},
					Commit: &buildbucketpb.GitilesCommit{
						Host:    "host1",
						Project: "proj1",
						Ref:     "ref",
						Id:      "commit6",
					},
				},
			},
		}), should.BeTrue)

		assert.Loosely(t, len(analysis.HeuristicResult.Suspects), should.Equal(1))
		assert.Loosely(t, proto.Equal(analysis.HeuristicResult.Suspects[0], &pb.HeuristicSuspect{
			GitilesCommit: &buildbucketpb.GitilesCommit{
				Host:    "host1",
				Project: "proj1",
				Id:      "commit5",
			},
			ReviewUrl:       "http://this/is/review/url",
			ReviewTitle:     "This is review title",
			Score:           100,
			Justification:   "Justification",
			ConfidenceLevel: pb.SuspectConfidenceLevel_HIGH,
			VerificationDetails: &pb.SuspectVerificationDetails{
				Status: string(model.SuspectVerificationStatus_ConfirmedCulprit),
				SuspectRerun: &pb.SingleRerun{
					Bbid:      8877665544332211,
					StartTime: &timestamppb.Timestamp{Seconds: 101},
					EndTime:   &timestamppb.Timestamp{Seconds: 102},
					RerunResult: &pb.RerunResult{
						RerunStatus: pb.RerunStatus_RERUN_STATUS_FAILED,
					},
					Commit: &buildbucketpb.GitilesCommit{
						Host:    "host1",
						Project: "proj1",
						Ref:     "ref",
						Id:      "commit5",
					},
				},
				ParentRerun: &pb.SingleRerun{
					Bbid:      7766554433221100,
					StartTime: &timestamppb.Timestamp{Seconds: 201},
					EndTime:   &timestamppb.Timestamp{Seconds: 202},
					RerunResult: &pb.RerunResult{
						RerunStatus: pb.RerunStatus_RERUN_STATUS_PASSED,
					},
					Commit: &buildbucketpb.GitilesCommit{
						Host:    "host1",
						Project: "proj1",
						Ref:     "ref",
						Id:      "commit6",
					},
				},
			},
		}), should.BeTrue)

		nthSectionResult := analysis.NthSectionResult
		assert.Loosely(t, nthSectionResult, should.NotBeNil)
		assert.Loosely(t, proto.Equal(nthSectionResult.StartTime, &timestamppb.Timestamp{Seconds: 100}), should.BeTrue)
		assert.Loosely(t, proto.Equal(nthSectionResult.EndTime, &timestamppb.Timestamp{Seconds: 102}), should.BeTrue)
		assert.Loosely(t, nthSectionResult.Status, should.Equal(pb.AnalysisStatus_FOUND))
		assert.Loosely(t, nthSectionResult.Suspect, should.Match(&pb.NthSectionSuspect{
			GitilesCommit: &buildbucketpb.GitilesCommit{
				Host:    "host1",
				Project: "proj1",
				Id:      "commit6",
			},
			ReviewUrl:   "http://this/is/review/url1",
			ReviewTitle: "This is review title1",
			VerificationDetails: &pb.SuspectVerificationDetails{
				Status: string(model.SuspectVerificationStatus_Vindicated),
			},
			Commit: &buildbucketpb.GitilesCommit{
				Host:    "host1",
				Project: "proj1",
				Id:      "commit6",
			},
		}))

		assert.Loosely(t, nthSectionResult.RemainingNthSectionRange, should.Match(&pb.RegressionRange{
			LastPassed: &buildbucketpb.GitilesCommit{
				Host:    "host1",
				Project: "proj1",
				Ref:     "ref",
				Id:      "commit5",
			},
			FirstFailed: &buildbucketpb.GitilesCommit{
				Host:    "host1",
				Project: "proj1",
				Ref:     "ref",
				Id:      "commit5",
			},
		}))

		assert.Loosely(t, len(nthSectionResult.Reruns), should.Equal(3))

		assert.Loosely(t, nthSectionResult.Reruns[0], should.Match(&pb.SingleRerun{
			Bbid:      8877665544332211,
			StartTime: &timestamppb.Timestamp{Seconds: 101},
			EndTime:   &timestamppb.Timestamp{Seconds: 102},
			RerunResult: &pb.RerunResult{
				RerunStatus: pb.RerunStatus_RERUN_STATUS_FAILED,
			},
			Commit: &buildbucketpb.GitilesCommit{
				Host:    "host1",
				Project: "proj1",
				Ref:     "ref",
				Id:      "commit5",
			},
			Index: "5",
			Type:  "Culprit Verification",
		}))

		assert.Loosely(t, nthSectionResult.Reruns[1], should.Match(&pb.SingleRerun{
			Bbid:      7766554433221100,
			StartTime: &timestamppb.Timestamp{Seconds: 201},
			EndTime:   &timestamppb.Timestamp{Seconds: 202},
			RerunResult: &pb.RerunResult{
				RerunStatus: pb.RerunStatus_RERUN_STATUS_PASSED,
			},
			Commit: &buildbucketpb.GitilesCommit{
				Host:    "host1",
				Project: "proj1",
				Ref:     "ref",
				Id:      "commit6",
			},
			Index: "6",
			Type:  "Culprit Verification",
		}))

		assert.Loosely(t, nthSectionResult.Reruns[2], should.Match(&pb.SingleRerun{
			Bbid:      800999000,
			StartTime: &timestamppb.Timestamp{Seconds: 301},
			EndTime:   &timestamppb.Timestamp{Seconds: 302},
			RerunResult: &pb.RerunResult{
				RerunStatus: pb.RerunStatus_RERUN_STATUS_PASSED,
			},
			Commit: &buildbucketpb.GitilesCommit{
				Host:    "host1",
				Project: "proj1",
				Ref:     "ref",
				Id:      "commit8",
			},
			Index: "8",
			Type:  "NthSection",
		}))

		assert.Loosely(t, nthSectionResult.BlameList, should.Match(nsa.BlameList))
	})

	ftt.Run("Analysis found for a similar failure", t, func(t *ftt.Test) {
		// Prepares datastore
		basedFailedBuild := &model.LuciFailedBuild{
			Id: 122,
		}
		assert.Loosely(t, datastore.Put(c, basedFailedBuild), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		basedCompileFailure := &model.CompileFailure{
			Id:    122,
			Build: datastore.KeyForObj(c, basedFailedBuild),
		}
		assert.Loosely(t, datastore.Put(c, basedCompileFailure), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		failedBuild := &model.LuciFailedBuild{
			Id: 123,
		}
		assert.Loosely(t, datastore.Put(c, failedBuild), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compileFailure := &model.CompileFailure{
			Id:               123,
			Build:            datastore.KeyForObj(c, failedBuild),
			MergedFailureKey: datastore.KeyForObj(c, basedCompileFailure),
		}
		assert.Loosely(t, datastore.Put(c, compileFailure), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compileFailureAnalysis := &model.CompileFailureAnalysis{
			CompileFailure: datastore.KeyForObj(c, basedCompileFailure),
		}
		assert.Loosely(t, datastore.Put(c, compileFailureAnalysis), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		req := &pb.QueryAnalysisRequest{
			BuildFailure: &pb.BuildFailure{
				FailedStepName: "compile",
				Bbid:           123,
			},
		}

		res, err := server.QueryAnalysis(c, req)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(res.Analyses), should.Equal(1))
	})
}

func TestListAnalyses(t *testing.T) {
	t.Parallel()
	server := &AnalysesServer{ACL: mockCheckAccessAnalyses}

	ftt.Run("List existing analyses", t, func(t *ftt.Test) {
		// Set up context and AEAD so that page tokens can be generated
		c := memory.Use(context.Background())
		c = secrets.GeneratePrimaryTinkAEADForTest(c)

		// Prepares datastore
		failureAnalysis1 := &model.CompileFailureAnalysis{
			Id:         1,
			CreateTime: (&timestamppb.Timestamp{Seconds: 100}).AsTime(),
		}
		failureAnalysis2 := &model.CompileFailureAnalysis{
			Id:         2,
			CreateTime: (&timestamppb.Timestamp{Seconds: 102}).AsTime(),
		}
		failureAnalysis3 := &model.CompileFailureAnalysis{
			Id:         3,
			CreateTime: (&timestamppb.Timestamp{Seconds: 101}).AsTime(),
		}
		failureAnalysis4 := &model.CompileFailureAnalysis{
			Id:         4,
			CreateTime: (&timestamppb.Timestamp{Seconds: 103}).AsTime(),
		}
		assert.Loosely(t, datastore.Put(c, failureAnalysis1), should.BeNil)
		assert.Loosely(t, datastore.Put(c, failureAnalysis2), should.BeNil)
		assert.Loosely(t, datastore.Put(c, failureAnalysis3), should.BeNil)
		assert.Loosely(t, datastore.Put(c, failureAnalysis4), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		t.Run("Invalid page size", func(t *ftt.Test) {
			req := &pb.ListAnalysesRequest{
				PageSize: -5,
			}
			_, err := server.ListAnalyses(c, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.InvalidArgument))
		})

		t.Run("Specifying page size is optional", func(t *ftt.Test) {
			req := &pb.ListAnalysesRequest{}
			res, err := server.ListAnalyses(c, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(res.Analyses), should.Equal(4))

			t.Run("Next page token is empty if there are no more analyses", func(t *ftt.Test) {
				assert.Loosely(t, res.NextPageToken, should.BeEmpty)
			})
		})

		t.Run("Response is limited by the page size", func(t *ftt.Test) {
			req := &pb.ListAnalysesRequest{
				PageSize: 3,
			}
			res, err := server.ListAnalyses(c, req)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, len(res.Analyses), should.Equal(int(req.PageSize)))
			assert.Loosely(t, res.NextPageToken, should.NotEqual(""))

			t.Run("Returned analyses are sorted correctly", func(t *ftt.Test) {
				assert.Loosely(t, res.Analyses[0].AnalysisId, should.Equal(4))
				assert.Loosely(t, res.Analyses[1].AnalysisId, should.Equal(2))
				assert.Loosely(t, res.Analyses[2].AnalysisId, should.Equal(3))

				t.Run("Page token will get the next page of analyses", func(t *ftt.Test) {
					req = &pb.ListAnalysesRequest{
						PageSize:  3,
						PageToken: res.NextPageToken,
					}
					res, err = server.ListAnalyses(c, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(res.Analyses), should.Equal(1))
					assert.Loosely(t, res.Analyses[0].AnalysisId, should.Equal(1))
				})
			})
		})
	})
}

func TestListTestAnalyses(t *testing.T) {
	t.Parallel()
	server := &AnalysesServer{ACL: mockCheckAccessAnalyses}

	ftt.Run("List existing analyses", t, func(t *ftt.Test) {
		// Set up context and AEAD so that page tokens can be generated
		ctx := memory.Use(context.Background())
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)
		testutil.UpdateIndices(ctx)

		// Prepares datastore
		for i := 1; i < 5; i++ {
			tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
				ID:             int64(i),
				CreateTime:     time.Unix(int64(100+i), 0).UTC(),
				TestFailureKey: datastore.MakeKey(ctx, "TestFailure", i),
			})
			testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
				ID:        int64(i),
				Analysis:  tfa,
				IsPrimary: true,
			})
		}

		t.Run("Empty project", func(t *ftt.Test) {
			req := &pb.ListTestAnalysesRequest{
				PageSize: 3,
			}
			_, err := server.ListTestAnalyses(ctx, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.InvalidArgument))
		})

		t.Run("Invalid page size", func(t *ftt.Test) {
			req := &pb.ListTestAnalysesRequest{
				Project:  "chromium",
				PageSize: -5,
			}
			_, err := server.ListTestAnalyses(ctx, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.InvalidArgument))
		})

		t.Run("Specifying page size is optional", func(t *ftt.Test) {
			req := &pb.ListTestAnalysesRequest{
				Project: "chromium",
			}
			res, err := server.ListTestAnalyses(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(res.Analyses), should.Equal(4))
			assert.Loosely(t, res.NextPageToken, should.BeEmpty)
		})

		t.Run("Response is limited by the page size", func(t *ftt.Test) {
			req := &pb.ListTestAnalysesRequest{
				Project:  "chromium",
				PageSize: 3,
			}
			res, err := server.ListTestAnalyses(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, len(res.Analyses), should.Equal(int(req.PageSize)))
			assert.Loosely(t, res.NextPageToken, should.NotEqual(""))
			assert.Loosely(t, res.Analyses[0].AnalysisId, should.Equal(4))
			assert.Loosely(t, res.Analyses[1].AnalysisId, should.Equal(3))
			assert.Loosely(t, res.Analyses[2].AnalysisId, should.Equal(2))

			// Next page.
			req = &pb.ListTestAnalysesRequest{
				Project:   "chromium",
				PageSize:  3,
				PageToken: res.NextPageToken,
			}
			res, err = server.ListTestAnalyses(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(res.Analyses), should.Equal(1))
			assert.Loosely(t, res.Analyses[0].AnalysisId, should.Equal(1))
		})
	})
}

func TestGetTestAnalyses(t *testing.T) {
	t.Parallel()
	server := &AnalysesServer{ACL: mockCheckAccessAnalyses}

	ftt.Run("List existing analyses", t, func(t *ftt.Test) {
		// Set up context and AEAD so that page tokens can be generated
		ctx := memory.Use(context.Background())
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)
		testutil.UpdateIndices(ctx)

		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:             int64(100),
			CreateTime:     time.Unix(int64(100), 0).UTC(),
			TestFailureKey: datastore.MakeKey(ctx, "TestFailure", 100),
		})
		testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
			ID:        int64(100),
			Analysis:  tfa,
			IsPrimary: true,
			TestID:    "testID",
			StartHour: time.Unix(int64(100), 0).UTC(),
		})

		t.Run("Not found", func(t *ftt.Test) {
			req := &pb.GetTestAnalysisRequest{
				AnalysisId: 101,
			}
			_, err := server.GetTestAnalysis(ctx, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.NotFound))
		})

		t.Run("Found", func(t *ftt.Test) {
			req := &pb.GetTestAnalysisRequest{
				AnalysisId: 100,
			}
			res, err := server.GetTestAnalysis(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match(&pb.TestAnalysis{
				AnalysisId: 100,
				Builder: &buildbucketpb.BuilderID{
					Project: "chromium",
					Bucket:  "bucket",
					Builder: "builder",
				},
				SampleBbid:  8000,
				CreatedTime: timestamppb.New(time.Unix(100, 0).UTC()),
				StartCommit: &buildbucketpb.GitilesCommit{},
				EndCommit:   &buildbucketpb.GitilesCommit{},
				TestFailures: []*pb.TestFailure{
					{
						TestId:    "testID",
						IsPrimary: true,
						Variant:   &pb.Variant{},
						StartHour: timestamppb.New(time.Unix(100, 0).UTC()),
					},
				},
			}))
		})
	})
}

func TestBatchGetTestAnalyses(t *testing.T) {
	t.Parallel()
	server := &AnalysesServer{ACL: mockCheckAccessAnalyses, LUCIAnalysisHost: "fake-host"}
	ftt.Run("With server", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		testutil.UpdateIndices(ctx)

		t.Run("invalid request", func(t *ftt.Test) {
			t.Run("missing project", func(t *ftt.Test) {
				req := &pb.BatchGetTestAnalysesRequest{
					TestFailures: []*pb.BatchGetTestAnalysesRequest_TestFailureIdentifier{},
				}
				_, err := server.BatchGetTestAnalyses(ctx, req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err.Error(), should.ContainSubstring("project: unspecified"))
			})
			t.Run("missing test failures", func(t *ftt.Test) {
				req := &pb.BatchGetTestAnalysesRequest{
					Project:      "chromium",
					TestFailures: []*pb.BatchGetTestAnalysesRequest_TestFailureIdentifier{},
				}
				_, err := server.BatchGetTestAnalyses(ctx, req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err.Error(), should.ContainSubstring("test_failures: unspecified"))
			})

			t.Run("missing test id", func(t *ftt.Test) {
				req := &pb.BatchGetTestAnalysesRequest{
					Project:      "chromium",
					TestFailures: []*pb.BatchGetTestAnalysesRequest_TestFailureIdentifier{{}},
				}
				_, err := server.BatchGetTestAnalyses(ctx, req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err.Error(), should.ContainSubstring("test_variants[0]: test_id: unspecified"))
			})

			t.Run("invalid variant hash", func(t *ftt.Test) {
				req := &pb.BatchGetTestAnalysesRequest{
					Project: "chromium",
					TestFailures: []*pb.BatchGetTestAnalysesRequest_TestFailureIdentifier{{
						TestId:      "testid",
						VariantHash: "randomstring",
						RefHash:     "randomstring",
					}},
				}
				_, err := server.BatchGetTestAnalyses(ctx, req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err.Error(), should.ContainSubstring("test_variants[0].variant_hash"))
			})

			t.Run("invalid source position", func(t *ftt.Test) {
				req := &pb.BatchGetTestAnalysesRequest{
					Project: "chromium",
					TestFailures: []*pb.BatchGetTestAnalysesRequest_TestFailureIdentifier{{
						TestId:         "testid",
						VariantHash:    "aaaaaaaaaaaaaaa",
						RefHash:        "aaaaaaaaaaaaaaa",
						SourcePosition: -1,
					}},
				}
				_, err := server.BatchGetTestAnalyses(ctx, req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err.Error(), should.ContainSubstring("test_variants[0]: source_position: must not be negative"))
			})
		})

		fakeAnalysisClient := &analysis.FakeTestVariantBranchesClient{}
		ctx = analysis.UseFakeClient(ctx, fakeAnalysisClient)

		t.Run("valid request", func(t *ftt.Test) {
			testFailureInRequest := []*pb.BatchGetTestAnalysesRequest_TestFailureIdentifier{}
			for i := 1; i < 6; i++ {
				tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
					ID:             int64(100 + i),
					CreateTime:     time.Unix(int64(100), 0).UTC(),
					TestFailureKey: datastore.MakeKey(ctx, "TestFailure", 100+i),
				})
				// Create the most recent test failure.
				testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
					ID:            int64(100 + i),
					Analysis:      tfa,
					TestID:        "testid" + fmt.Sprint(i),
					VariantHash:   "aaaaaaaaaaaaaaa" + fmt.Sprint(i),
					RefHash:       "bbbbbbbbbbbbbbb" + fmt.Sprint(i),
					StartPosition: 101,
					EndPosition:   110,
					IsPrimary:     true,
					StartHour:     time.Unix(int64(99), 0).UTC(),
					IsDiverged:    i == 5, // TestFailure 105 is diverged.
				})
				// Create another less recent test failure.
				testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
					ID:            int64(1000 + i),
					TestID:        "testid" + fmt.Sprint(i),
					VariantHash:   "aaaaaaaaaaaaaaa" + fmt.Sprint(i),
					RefHash:       "bbbbbbbbbbbbbbb" + fmt.Sprint(i),
					StartPosition: 90,
					EndPosition:   99,
				})
				testFailureInRequest = append(testFailureInRequest, &pb.BatchGetTestAnalysesRequest_TestFailureIdentifier{
					TestId:      "testid" + fmt.Sprint(i),
					VariantHash: "aaaaaaaaaaaaaaa" + fmt.Sprint(i),
					RefHash:     "bbbbbbbbbbbbbbb" + fmt.Sprint(i),
				})
			}

			fakeAnalysisClient.TestVariantBranches = []*analysispb.TestVariantBranch{
				makeFakeTestVariantBranch(0, nil), // No changepoint data -> return nil.
				makeFakeTestVariantBranch(1, []*analysispb.Segment{{
					StartPosition: 1,
					EndPosition:   100,
					Counts: &analysispb.Segment_Counts{
						TotalResults:      1,
						UnexpectedResults: 1,
					},
				}}), // One segment -> return nil.
				makeFakeTestVariantBranch(1, []*analysispb.Segment{{
					StartPosition: 1,
					EndPosition:   100,
					Counts: &analysispb.Segment_Counts{
						TotalResults:      1,
						UnexpectedResults: 1,
					},
				}}), // One segment -> return nil.

			}

			t.Run("request with multiple test failures", func(t *ftt.Test) {
				req := &pb.BatchGetTestAnalysesRequest{
					Project:      "chromium",
					TestFailures: testFailureInRequest,
				}
				segments := [][]*analysispb.Segment{
					nil, // No changepoint data -> return nil.
					{{
						StartPosition: 1,
						EndPosition:   100,
						Counts: &analysispb.Segment_Counts{
							TotalResults:      1,
							UnexpectedResults: 1,
						},
					}}, // One segment -> return nil.
					{{
						StartPosition: 111,
						EndPosition:   200,
						Counts: &analysispb.Segment_Counts{
							TotalResults:      1,
							UnexpectedResults: 1,
						},
					},
						{
							StartPosition: 1,
							EndPosition:   105,
							Counts: &analysispb.Segment_Counts{
								TotalResults:      1,
								UnexpectedResults: 0,
							},
						}}, // Two segment, failure not ongoing, regression range (105,111]-> return nil.
					{{
						StartPosition: 109,
						EndPosition:   200,
						Counts: &analysispb.Segment_Counts{
							TotalResults:      1,
							UnexpectedResults: 1,
						},
					},
						{
							StartPosition: 1,
							EndPosition:   101,
							Counts: &analysispb.Segment_Counts{
								TotalResults:      1,
								UnexpectedResults: 0,
							},
						}}, // Two segment, failure is ongoing, regression range (101,109] -> return test analysis 104.
					{{
						StartPosition: 109,
						EndPosition:   200,
						Counts: &analysispb.Segment_Counts{
							TotalResults:      1,
							UnexpectedResults: 1,
						},
					},
						{
							StartPosition: 1,
							EndPosition:   101,
							Counts: &analysispb.Segment_Counts{
								TotalResults:      1,
								UnexpectedResults: 0,
							},
						}}, // Two segment, failure is ongoing, regression range (101,109] -> not return test analysis because test failure is diverged.
				}

				var branches []*analysispb.TestVariantBranch
				for i, segs := range segments {
					branches = append(branches, makeFakeTestVariantBranch(i+1, segs))
				}

				fakeAnalysisClient.TestVariantBranches = branches

				resp, err := server.BatchGetTestAnalyses(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Match(&pb.BatchGetTestAnalysesResponse{
					TestAnalyses: []*pb.TestAnalysis{nil, nil, nil, {
						AnalysisId:  104,
						Builder:     &buildbucketpb.BuilderID{Project: "chromium", Bucket: "bucket", Builder: "builder"},
						CreatedTime: timestamppb.New(time.Unix(int64(100), 0).UTC()),
						EndCommit:   &buildbucketpb.GitilesCommit{Position: 110},
						SampleBbid:  8000,
						StartCommit: &buildbucketpb.GitilesCommit{Position: 101},
						TestFailures: []*pb.TestFailure{
							{
								TestId:      "testid4",
								VariantHash: "aaaaaaaaaaaaaaa4",
								RefHash:     "bbbbbbbbbbbbbbb4",
								Variant:     &pb.Variant{},
								IsDiverged:  false,
								IsPrimary:   true,
								StartHour:   timestamppb.New(time.Unix(int64(99), 0).UTC()),
							},
						},
					}, nil},
				}))
			})

			t.Run("request with source position", func(t *ftt.Test) {
				req := &pb.BatchGetTestAnalysesRequest{
					Project: "chromium",
					TestFailures: []*pb.BatchGetTestAnalysesRequest_TestFailureIdentifier{
						{
							TestId:         "testid1",
							VariantHash:    "aaaaaaaaaaaaaaa1",
							RefHash:        "bbbbbbbbbbbbbbb1",
							SourcePosition: 105,
						},
						{
							TestId:         "testid2",
							VariantHash:    "aaaaaaaaaaaaaaa2",
							RefHash:        "bbbbbbbbbbbbbbb2",
							SourcePosition: 112, // Not found
						},
						{
							TestId:         "testid3",
							VariantHash:    "aaaaaaaaaaaaaaa2", // Not found
							RefHash:        "bbbbbbbbbbbbbbb2",
							SourcePosition: 105,
						},
						{
							TestId:         "testid5", // Diverge
							VariantHash:    "aaaaaaaaaaaaaaa5",
							RefHash:        "bbbbbbbbbbbbbbb5",
							SourcePosition: 109,
						},
					},
				}
				resp, err := server.BatchGetTestAnalyses(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Match(&pb.BatchGetTestAnalysesResponse{
					TestAnalyses: []*pb.TestAnalysis{
						{
							AnalysisId:  101,
							Builder:     &buildbucketpb.BuilderID{Project: "chromium", Bucket: "bucket", Builder: "builder"},
							CreatedTime: timestamppb.New(time.Unix(int64(100), 0).UTC()),
							EndCommit:   &buildbucketpb.GitilesCommit{Position: 110},
							SampleBbid:  8000,
							StartCommit: &buildbucketpb.GitilesCommit{Position: 101},
							TestFailures: []*pb.TestFailure{
								{
									TestId:      "testid1",
									VariantHash: "aaaaaaaaaaaaaaa1",
									RefHash:     "bbbbbbbbbbbbbbb1",
									Variant:     &pb.Variant{},
									IsDiverged:  false,
									IsPrimary:   true,
									StartHour:   timestamppb.New(time.Unix(int64(99), 0).UTC()),
								},
							},
						},
						{},
						{},
						{},
					},
				}))
			})
		})
		t.Run("valid request, permission denied obtaining LUCI Analysis test variant branches", func(t *ftt.Test) {
			// When a project does not exist in LUCI, LUCI Analysis returns a permission denied error.
			fakeAnalysisClient.BatchGetErr = status.Error(codes.PermissionDenied, "permission denied")

			req := &pb.BatchGetTestAnalysesRequest{
				Project: "other",
				TestFailures: []*pb.BatchGetTestAnalysesRequest_TestFailureIdentifier{
					{
						TestId:      "aaaa",
						VariantHash: "1234567890abcdef",
						RefHash:     "1234567890abcdef",
					},
				},
			}

			// Expect a response with an empty item for each requested test failure in this case.
			rsp, err := server.BatchGetTestAnalyses(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp.TestAnalyses, should.Match([]*pb.TestAnalysis{nil}))
		})
	})
}

func makeFakeTestVariantBranch(i int, segments []*analysispb.Segment) *analysispb.TestVariantBranch {
	testID := "testid" + fmt.Sprint(i)
	variantHash := "aaaaaaaaaaaaaaa" + fmt.Sprint(i)
	refHash := "bbbbbbbbbbbbbbb" + fmt.Sprint(i)

	return &analysispb.TestVariantBranch{
		Name:        fmt.Sprintf("projects/chromium/tests/%s/variants/%s/refs/%s", url.PathEscape(testID), variantHash, refHash),
		Project:     "chromium",
		TestId:      testID,
		VariantHash: variantHash,
		RefHash:     refHash,
		Segments:    segments,
	}
}
