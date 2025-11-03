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

// Package bqutil contains utility functions for BigQuery.
package bqutil

import (
	"context"

	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/mask"

	"go.chromium.org/luci/bisection/model"
	bqpb "go.chromium.org/luci/bisection/proto/bq"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/protoutil"
)

// CompileFailureAnalysisToBqRow returns a CompileAnalysisRow for a CompileFailureAnalysis.
func CompileFailureAnalysisToBqRow(ctx context.Context, cfa *model.CompileFailureAnalysis) (*bqpb.CompileAnalysisRow, error) {
	// Make sure that the analysis has ended.
	if cfa.RunStatus != pb.AnalysisRunStatus_ENDED {
		return nil, errors.Fmt("analysis %d has not ended", cfa.Id)
	}
	compileFailure, err := datastoreutil.GetCompileFailureForAnalysis(ctx, cfa)
	if err != nil {
		return nil, errors.Fmt("get compile failure for analysis: %w", err)
	}

	fb, err := datastoreutil.GetBuild(ctx, cfa.CompileFailure.Parent().IntID())
	if err != nil {
		return nil, errors.Fmt("getBuild: %w", err)
	}
	if fb == nil {
		return nil, errors.Fmt("couldn't find build for analysis %d", cfa.CompileFailure.Parent().IntID())
	}

	result := &bqpb.CompileAnalysisRow{
		Project:     fb.Project,
		AnalysisId:  cfa.Id,
		CreatedTime: timestamppb.New(cfa.CreateTime),
		StartTime:   timestamppb.New(cfa.StartTime),
		EndTime:     timestamppb.New(cfa.EndTime),
		Status:      cfa.Status,
		RunStatus:   cfa.RunStatus,
		Builder: &buildbucketpb.BuilderID{
			Project: fb.Project,
			Bucket:  fb.Bucket,
			Builder: fb.Builder,
		},
		BuildFailure: protoutil.CompileFailureToPb(compileFailure),
		SampleBbid:   fb.Id,
	}
	// TODO(jiameil): Add nthsection result to the result.
	culpritKeys := cfa.VerifiedCulprits
	if len(culpritKeys) == 0 {
		logging.Errorf(ctx, "no verified culprits for analysis %d: %v", cfa.Id, err)
	} else {
		pbCulprits := make([]*pb.Culprit, 0, len(culpritKeys))
		for _, key := range culpritKeys {
			culprit, err := datastoreutil.GetSuspect(ctx, key.IntID(), key.Parent())
			if err != nil {
				// Log the error and continue, so that one bad culprit doesn't fail the whole process
				logging.Errorf(ctx, "could not get suspect for key %s: %v", key.String(), err)
				continue
			}
			pbCulprits = append(pbCulprits, protoutil.SuspectToCulpritPb(culprit))
		}
		result.Culprits = pbCulprits
	}

	ga, err := datastoreutil.GetGenAIAnalysis(ctx, cfa)
	if err != nil {
		return nil, errors.Annotate(err, "get genai result for analysis %d", cfa.Id).Err()
	}
	if ga != nil {
		result.GenaiResult = protoutil.CompileGenAIAnalysisToPb(ga)
	}

	return result, nil
}

// TestFailureAnalysisToBqRow returns a TestAnalysisRow for a TestFailureAnalysis.
// This is very similar to TestFailureAnalysisToPb in protoutil, but we intentionally
// keep them separate because they are for 2 different purposes.
// It gives us the flexibility to change one without affecting the others.
func TestFailureAnalysisToBqRow(ctx context.Context, tfa *model.TestFailureAnalysis) (*bqpb.TestAnalysisRow, error) {
	// Make sure that the analysis has ended.
	if !tfa.HasEnded() {
		return nil, errors.Fmt("analysis %d has not ended", tfa.ID)
	}
	result := &bqpb.TestAnalysisRow{
		Project:     tfa.Project,
		AnalysisId:  tfa.ID,
		CreatedTime: timestamppb.New(tfa.CreateTime),
		StartTime:   timestamppb.New(tfa.StartTime),
		EndTime:     timestamppb.New(tfa.EndTime),
		Status:      tfa.Status,
		RunStatus:   tfa.RunStatus,
		Builder: &buildbucketpb.BuilderID{
			Project: tfa.Project,
			Bucket:  tfa.Bucket,
			Builder: tfa.Builder,
		},
		SampleBbid: tfa.FailedBuildID,
	}

	// Get test bundle.
	bundle, err := datastoreutil.GetTestFailureBundle(ctx, tfa)
	if err != nil {
		return nil, errors.Fmt("get test failure bundle: %w", err)
	}
	tfFieldMask := fieldmaskpb.FieldMask{
		Paths: []string{"*"},
	}
	tfMask, err := mask.FromFieldMask(&tfFieldMask, &pb.TestFailure{}, mask.AdvancedSemantics())
	if err != nil {
		return nil, errors.Fmt("from field mask: %w", err)
	}

	result.TestFailures = protoutil.TestFailureBundleToPb(ctx, bundle, tfMask)
	primary := bundle.Primary()
	result.StartGitilesCommit = &buildbucketpb.GitilesCommit{
		Host:     primary.Ref.GetGitiles().GetHost(),
		Project:  primary.Ref.GetGitiles().GetProject(),
		Ref:      primary.Ref.GetGitiles().GetRef(),
		Id:       tfa.StartCommitHash,
		Position: uint32(primary.RegressionStartPosition),
	}
	result.EndGitilesCommit = &buildbucketpb.GitilesCommit{
		Host:     primary.Ref.GetGitiles().GetHost(),
		Project:  primary.Ref.GetGitiles().GetProject(),
		Ref:      primary.Ref.GetGitiles().GetRef(),
		Id:       tfa.EndCommitHash,
		Position: uint32(primary.RegressionEndPosition),
	}

	nsa, err := datastoreutil.GetTestNthSectionForAnalysis(ctx, tfa)
	if err != nil {
		return nil, errors.Fmt("get test nthsection for analysis: %w", err)
	}
	if nsa != nil {
		nsaFieldMask := fieldmaskpb.FieldMask{
			Paths: []string{"*"},
		}
		nsaMask, err := mask.FromFieldMask(&nsaFieldMask, &pb.TestNthSectionAnalysisResult{}, mask.AdvancedSemantics())
		if err != nil {
			return nil, errors.Fmt("from field mask: %w", err)
		}

		nsaResult, err := protoutil.NthSectionAnalysisToPb(ctx, tfa, nsa, primary.Ref, nsaMask)
		if err != nil {
			return nil, errors.Fmt("nthsection analysis to pb: %w", err)
		}
		result.NthSectionResult = nsaResult
		culprit, err := datastoreutil.GetVerifiedCulpritForTestAnalysis(ctx, tfa)
		if err != nil {
			return nil, errors.Fmt("get verified culprit: %w", err)
		}
		if culprit != nil {
			culpritFieldMask := fieldmaskpb.FieldMask{
				Paths: []string{"*"},
			}
			culpritMask, err := mask.FromFieldMask(&culpritFieldMask, &pb.TestCulprit{}, mask.AdvancedSemantics())
			if err != nil {
				return nil, errors.Fmt("from field mask: %w", err)
			}

			culpritPb, err := protoutil.CulpritToPb(ctx, culprit, nsa, culpritMask)
			if err != nil {
				return nil, errors.Fmt("culprit to pb: %w", err)
			}
			result.Culprit = culpritPb
		}
	}
	return result, nil
}
