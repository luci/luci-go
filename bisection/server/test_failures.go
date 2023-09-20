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

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestFailureAnalysisToPb converts model.TestFailureAnalysis to pb.TestAnalysis
func TestFailureAnalysisToPb(ctx context.Context, tfa *model.TestFailureAnalysis) (*pb.TestAnalysis, error) {
	result := &pb.TestAnalysis{
		AnalysisId: tfa.ID,
		CreateTime: timestamppb.New(tfa.CreateTime),
		Status:     tfa.Status,
		RunStatus:  tfa.RunStatus,
		Builder: &buildbucketpb.BuilderID{
			Project: tfa.Project,
			Bucket:  tfa.Bucket,
			Builder: tfa.Builder,
		},
		SampleBbid: tfa.FailedBuildID,
	}
	if tfa.HasStarted() {
		result.StartTime = timestamppb.New(tfa.StartTime)
	}
	if tfa.HasEnded() {
		result.EndTime = timestamppb.New(tfa.EndTime)
	}

	// Get test bundle.
	bundle, err := datastoreutil.GetTestFailureBundle(ctx, tfa)
	if err != nil {
		return nil, errors.Annotate(err, "get test failure bundle").Err()
	}
	result.TestFailures = TestFailureBundleToPb(ctx, bundle)
	primary := bundle.Primary()
	result.StartFailureRate = float32(primary.StartPositionFailureRate)
	result.EndFailureRate = float32(primary.EndPositionFailureRate)
	result.StartCommit = &buildbucketpb.GitilesCommit{
		Host:     primary.Ref.GetGitiles().GetHost(),
		Project:  primary.Ref.GetGitiles().GetProject(),
		Ref:      primary.Ref.GetGitiles().GetRef(),
		Id:       tfa.StartCommitHash,
		Position: uint32(primary.RegressionStartPosition),
	}
	result.EndCommit = &buildbucketpb.GitilesCommit{
		Host:     primary.Ref.GetGitiles().GetHost(),
		Project:  primary.Ref.GetGitiles().GetProject(),
		Ref:      primary.Ref.GetGitiles().GetRef(),
		Id:       tfa.EndCommitHash,
		Position: uint32(primary.RegressionEndPosition),
	}

	nsa, err := datastoreutil.GetTestNthSectionForAnalysis(ctx, tfa)
	if err != nil {
		return nil, errors.Annotate(err, "get test nthsection for analysis").Err()
	}
	if nsa != nil {
		nsaResult, err := NthSectionAnalysisToPb(ctx, tfa, nsa)
		if err != nil {
			return nil, errors.Annotate(err, "nthsection analysis to pb").Err()
		}
		result.NthSectionResult = nsaResult
		culprit, err := datastoreutil.GetVerifiedCulpritForTestAnalysis(ctx, tfa)
		if err != nil {
			return nil, errors.Annotate(err, "get verified culprit").Err()
		}
		if culprit != nil {
			result.VerifiedCulprit = CulpritToPb(ctx, tfa, culprit)
		}
	}
	return result, nil
}

func NthSectionAnalysisToPb(ctx context.Context, tfa *model.TestFailureAnalysis, nsa *model.TestNthSectionAnalysis) (*pb.TestNthSectionAnalysisResult, error) {
	result := &pb.TestNthSectionAnalysisResult{
		Status:    nsa.Status,
		RunStatus: nsa.RunStatus,
		BlameList: nsa.BlameList,
		StartTime: timestamppb.New(nsa.StartTime),
	}
	if nsa.HasEnded() {
		result.EndTime = timestamppb.New(nsa.EndTime)
	}
	// TODO (nqmtuan): Populate other fields: Remaining range, reruns, culprit.
	return result, nil
}

func CulpritToPb(ctx context.Context, tfa *model.TestFailureAnalysis, culprit *model.Suspect) *pb.TestCulprit {
	return &pb.TestCulprit{
		Commit: &buildbucketpb.GitilesCommit{
			Host:     culprit.GitilesCommit.Host,
			Project:  culprit.GitilesCommit.Project,
			Ref:      culprit.GitilesCommit.Ref,
			Id:       culprit.GitilesCommit.Id,
			Position: culprit.GitilesCommit.Position,
		},
		ReviewUrl:   culprit.ReviewUrl,
		ReviewTitle: culprit.ReviewTitle,
		// TODO(nqmtuan): Populate action and rerun details.
	}
}

func TestFailureBundleToPb(ctx context.Context, bundle *model.TestFailureBundle) []*pb.TestFailure {
	result := []*pb.TestFailure{}
	// Add primary test failure first, and the rest.
	// Primary should not be nil here, because it is from GetTestFailureBundle.
	primary := bundle.Primary()
	result = append(result, TestFailureToPb(ctx, primary))
	for _, tf := range bundle.Others() {
		result = append(result, TestFailureToPb(ctx, tf))
	}
	return result
}

func TestFailureToPb(ctx context.Context, tf *model.TestFailure) *pb.TestFailure {
	return &pb.TestFailure{
		TestId:      tf.TestID,
		VariantHash: tf.VariantHash,
		RefHash:     tf.RefHash,
		Variant:     tf.Variant,
		IsDiverged:  tf.IsDiverged,
		IsPrimary:   tf.IsPrimary,
		StartHour:   timestamppb.New(tf.StartHour),
	}
}
