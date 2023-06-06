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

package rpc

import (
	"context"
	"encoding/hex"
	"fmt"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	tvbr "go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	pb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// NewTestVariantBranchesServer returns a new pb.TestVariantBranchesServer.
func NewTestVariantBranchesServer() pb.TestVariantBranchesServer {
	return &pb.DecoratedTestVariantBranches{
		// Currently, we only allow LUCI Analysis admins to use this API.
		// In the future, if this end point is used for the UI, we should
		// have proper ACL check.
		Prelude:  checkAllowedAdminPrelude,
		Service:  &testVariantBranchesServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// testVariantBranchesServer implements pb.TestVariantAnalysesServer.
type testVariantBranchesServer struct {
}

// Get fetches Spanner for test variant analysis.
func (*testVariantBranchesServer) Get(ctx context.Context, req *pb.GetTestVariantBranchRequest) (*pb.TestVariantBranch, error) {
	tvbk, err := validateGetTestVariantBranchRequest(req)
	if err != nil {
		return nil, invalidArgumentError(err)
	}

	txn, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	tvbs, err := tvbr.ReadTestVariantBranches(txn, []tvbr.TestVariantBranchKey{tvbk})
	if err != nil {
		return nil, errors.Annotate(err, "read test variant branch").Err()
	}
	// Should not happen.
	if len(tvbs) != 1 {
		return nil, fmt.Errorf("expected to find only 1 test variant branch. Got %d", len(tvbs))
	}
	// Not found.
	if tvbs[0] == nil {
		return nil, appstatus.Error(codes.NotFound, "analysis not found")
	}
	// Convert to proto.
	analysis, err := testVariantBranchToProto(tvbs[0])
	if err != nil {
		return nil, errors.Annotate(err, "build proto").Err()
	}
	return analysis, nil
}

func validateGetTestVariantBranchRequest(req *pb.GetTestVariantBranchRequest) (tvbr.TestVariantBranchKey, error) {
	project, testID, variantHash, refHash, err := parseTestVariantBranchName(req.Name)
	if err != nil {
		return tvbr.TestVariantBranchKey{}, errors.Annotate(err, "name").Err()
	}

	// Check ref hash.
	refHashBytes, err := hex.DecodeString(refHash)
	if err != nil {
		return tvbr.TestVariantBranchKey{}, errors.Reason("ref component must be an encoded hexadecimal string").Err()
	}
	return tvbr.TestVariantBranchKey{
		Project:     project,
		TestID:      testID,
		VariantHash: variantHash,
		RefHash:     tvbr.RefHash(refHashBytes),
	}, nil
}

func testVariantBranchToProto(tvb *tvbr.TestVariantBranch) (*pb.TestVariantBranch, error) {
	var finalizedSegments *anypb.Any
	var finalizingSegment *anypb.Any
	var statistics *anypb.Any

	// Hide the internal Spanner proto from our clients, as they
	// must not depend on it. If this API is used for anything
	// other than debug purposes in future, we will need to define
	// a wire proto and use it instead.
	if tvb.FinalizedSegments.GetSegments() != nil {
		var err error
		finalizedSegments, err = anypb.New(tvb.FinalizedSegments)
		if err != nil {
			return nil, err
		}
	}
	if tvb.FinalizingSegment != nil {
		var err error
		finalizingSegment, err = anypb.New(tvb.FinalizingSegment)
		if err != nil {
			return nil, err
		}
	}
	if tvb.Statistics != nil {
		var err error
		statistics, err = anypb.New(tvb.Statistics)
		if err != nil {
			return nil, err
		}
	}

	refHash := hex.EncodeToString(tvb.RefHash)
	result := &pb.TestVariantBranch{
		Name:              testVariantBranchName(tvb.Project, tvb.TestID, tvb.VariantHash, refHash),
		Project:           tvb.Project,
		TestId:            tvb.TestID,
		VariantHash:       tvb.VariantHash,
		RefHash:           refHash,
		Variant:           tvb.Variant,
		Ref:               tvb.SourceRef,
		FinalizedSegments: finalizedSegments,
		FinalizingSegment: finalizingSegment,
		Statistics:        statistics,
		HotBuffer:         toInputBufferProto(tvb.InputBuffer.HotBuffer),
		ColdBuffer:        toInputBufferProto(tvb.InputBuffer.ColdBuffer),
	}
	return result, nil
}

func toInputBufferProto(history inputbuffer.History) *pb.InputBuffer {
	result := &pb.InputBuffer{
		Length:   int64(len(history.Verdicts)),
		Verdicts: []*pb.PositionVerdict{},
	}
	for _, verdict := range history.Verdicts {
		pv := &pb.PositionVerdict{
			CommitPosition: int64(verdict.CommitPosition),
			Hour:           timestamppb.New(verdict.Hour),
			Runs:           []*pb.PositionVerdict_Run{},
		}
		if verdict.IsSimpleExpected {
			pv.Runs = []*pb.PositionVerdict_Run{
				{
					ExpectedResultCount: 1,
				},
			}
		} else {
			pv.IsExonerated = verdict.Details.IsExonerated
			for _, r := range verdict.Details.Runs {
				pv.Runs = append(pv.Runs, &pb.PositionVerdict_Run{
					ExpectedResultCount:   int64(r.ExpectedResultCount),
					UnexpectedResultCount: int64(r.UnexpectedResultCount),
					IsDuplicate:           r.IsDuplicate,
				})
			}
		}
		result.Verdicts = append(result.Verdicts, pv)
	}
	return result
}
