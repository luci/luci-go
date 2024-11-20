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
	"go.chromium.org/luci/common/proto/mask"

	"go.chromium.org/luci/bisection/model"
	bqpb "go.chromium.org/luci/bisection/proto/bq"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/protoutil"
)

// TestFailureAnalysisToBqRow returns a TestAnalysisRow for a TestFailureAnalysis.
// This is very similar to TestFailureAnalysisToPb in protoutil, but we intentionally
// keep them separate because they are for 2 different purposes.
// It gives us the flexibility to change one without affecting the others.
func TestFailureAnalysisToBqRow(ctx context.Context, tfa *model.TestFailureAnalysis) (*bqpb.TestAnalysisRow, error) {
	// Make sure that the analysis has ended.
	if !tfa.HasEnded() {
		return nil, errors.Reason("analysis %d has not ended", tfa.ID).Err()
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
		return nil, errors.Annotate(err, "get test failure bundle").Err()
	}
	tfFieldMask := fieldmaskpb.FieldMask{
		Paths: []string{"*"},
	}
	tfMask, err := mask.FromFieldMask(&tfFieldMask, &pb.TestFailure{}, mask.AdvancedSemantics())
	if err != nil {
		return nil, errors.Annotate(err, "from field mask").Err()
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
		return nil, errors.Annotate(err, "get test nthsection for analysis").Err()
	}
	if nsa != nil {
		nsaFieldMask := fieldmaskpb.FieldMask{
			Paths: []string{"*"},
		}
		nsaMask, err := mask.FromFieldMask(&nsaFieldMask, &pb.TestNthSectionAnalysisResult{}, mask.AdvancedSemantics())
		if err != nil {
			return nil, errors.Annotate(err, "from field mask").Err()
		}

		nsaResult, err := protoutil.NthSectionAnalysisToPb(ctx, tfa, nsa, primary.Ref, nsaMask)
		if err != nil {
			return nil, errors.Annotate(err, "nthsection analysis to pb").Err()
		}
		result.NthSectionResult = nsaResult
		culprit, err := datastoreutil.GetVerifiedCulpritForTestAnalysis(ctx, tfa)
		if err != nil {
			return nil, errors.Annotate(err, "get verified culprit").Err()
		}
		if culprit != nil {
			culpritFieldMask := fieldmaskpb.FieldMask{
				Paths: []string{"*"},
			}
			culpritMask, err := mask.FromFieldMask(&culpritFieldMask, &pb.TestCulprit{}, mask.AdvancedSemantics())
			if err != nil {
				return nil, errors.Annotate(err, "from field mask").Err()
			}

			culpritPb, err := protoutil.CulpritToPb(ctx, culprit, nsa, culpritMask)
			if err != nil {
				return nil, errors.Annotate(err, "culprit to pb").Err()
			}
			result.Culprit = culpritPb
		}
	}
	return result, nil
}
