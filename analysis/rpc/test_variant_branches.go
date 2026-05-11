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
	"time"

	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/changepoints/analyzer"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/changepoints/model"
	"go.chromium.org/luci/analysis/internal/changepoints/sorbet"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/internal/pagination"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testverdicts"
	"go.chromium.org/luci/analysis/internal/tracing"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

type TestVerdictClient interface {
	ReadTestVerdictAfterPosition(ctx context.Context, options testverdicts.ReadVerdictAtOrAfterPositionOptions) (*testverdicts.SourceVerdict, error)
}

// NewTestVariantBranchesServer returns a new pb.TestVariantBranchesServer.
func NewTestVariantBranchesServer(tvc TestVerdictClient, sc sorbet.GenerateClient) pb.TestVariantBranchesServer {
	return &pb.DecoratedTestVariantBranches{
		Prelude: checkAllowedPrelude,
		Service: &testVariantBranchesServer{
			sorbetAnalyzer: sorbet.NewAnalyzer(sc, tvc),
		},
		Postlude: GRPCifyAndLogPostlude,
	}
}

// testVariantBranchesServer implements pb.TestVariantAnalysesServer.
type testVariantBranchesServer struct {
	sorbetAnalyzer *sorbet.Analyzer
}

// Get fetches Spanner for test variant analysis.
func (*testVariantBranchesServer) GetRaw(ctx context.Context, req *pb.GetRawTestVariantBranchRequest) (*pb.TestVariantBranchRaw, error) {
	// This endpoint is designed for use by LUCI Analysis admins only.
	// This check prevents other applications unexpectedly creating a dependency
	// on the RPC.
	if err := checkAllowed(ctx, luciAnalysisAdminGroup); err != nil {
		return nil, err
	}
	project, testID, variantHash, refHash, err := parseTestVariantBranchName(req.Name)
	if err != nil {
		return nil, InvalidArgumentError(errors.Fmt("name: %w", err))
	}
	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetTestVariantBranch); err != nil {
		return nil, err
	}

	// Check ref hash.
	refHashBytes, err := hex.DecodeString(refHash)
	if err != nil {
		return nil, InvalidArgumentError(errors.New("name: ref component must be an encoded hexadecimal string"))
	}
	tvbk := testvariantbranch.Key{
		Project:     project,
		TestID:      testID,
		VariantHash: variantHash,
		RefHash:     testvariantbranch.RefHash(refHashBytes),
	}

	txn, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	tvbs, err := testvariantbranch.Read(txn, []testvariantbranch.Key{tvbk})
	if err != nil {
		return nil, errors.Fmt("read test variant branch: %w", err)
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
	analysis, err := toTestVariantBranchRawProto(tvbs[0])
	if err != nil {
		return nil, errors.Fmt("build proto: %w", err)
	}
	return analysis, nil
}

// BatchGet returns current state of segments for test variant branches.
func (*testVariantBranchesServer) BatchGet(ctx context.Context, req *pb.BatchGetTestVariantBranchRequest) (*pb.BatchGetTestVariantBranchResponse, error) {
	// AIP-211: Perform authorisation checks before validating request proper.
	for i, name := range req.Names {
		project, _, _, _, err := parseTestVariantBranchName(name)
		if err != nil {
			return nil, InvalidArgumentError(errors.Fmt("names[%d]: %w", i, err))
		}
		if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetTestVariantBranch); err != nil {
			return nil, err
		}
	}
	if err := validateBatchGetTestVariantBranchRequest(req); err != nil {
		return nil, InvalidArgumentError(err)
	}

	keys := make([]testvariantbranch.Key, 0, len(req.Names))
	for _, name := range req.Names {
		project, testID, variantHash, refHash, err := parseTestVariantBranchName(name)
		if err != nil {
			return nil, err
		}
		refHashBytes, err := hex.DecodeString(refHash)
		if err != nil {
			// This line is unreachable as ref hash should be validated already.
			return nil, errors.New("ref hash must be an encoded hexadecimal string")
		}
		keys = append(keys, testvariantbranch.Key{
			Project:     project,
			TestID:      testID,
			VariantHash: variantHash,
			RefHash:     testvariantbranch.RefHash(refHashBytes),
		})
	}

	txn, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	tvbs, err := testvariantbranch.Read(txn, keys)
	if err != nil {
		return nil, errors.Fmt("read test variant branch: %w", err)
	}

	return &pb.BatchGetTestVariantBranchResponse{
		TestVariantBranches: toTestVariantBranches(ctx, tvbs),
	}, nil
}

func toTestVariantBranches(ctx context.Context, tvbs []*testvariantbranch.Entry) []*pb.TestVariantBranch {
	// Stores the total time performing analysis (across all test variant branches).
	var analysisTime time.Duration

	_, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/rpc.toTestVariantBranches")
	defer func() {
		tracing.End(s, nil, attribute.Int64("analysis_nanos", analysisTime.Nanoseconds()))
	}()

	results := make([]*pb.TestVariantBranch, 0, len(tvbs))
	var analysis analyzer.Analyzer
	for _, tvb := range tvbs {
		if tvb == nil {
			results = append(results, nil)
			continue
		}

		startTime := time.Now()
		// Run analysis to get logical segments for the test variant branch.
		segments := analysis.Run(tvb)
		analysisTime += time.Since(startTime)

		results = append(results, toTestVariantBranchProto(tvb, segments))
	}
	return results
}

func toTestVariantBranchProto(tvb *testvariantbranch.Entry, segments []analyzer.Segment) *pb.TestVariantBranch {
	refHash := hex.EncodeToString(tvb.RefHash)
	return &pb.TestVariantBranch{
		Name:        testVariantBranchName(tvb.Project, tvb.TestID, tvb.VariantHash, refHash),
		Project:     tvb.Project,
		TestId:      tvb.TestID,
		VariantHash: tvb.VariantHash,
		RefHash:     refHash,
		Variant:     tvb.Variant,
		Ref:         tvb.SourceRef,
		Segments:    toSegmentsProto(segments),
	}
}

func toTestVariantBranchRawProto(tvb *testvariantbranch.Entry) (*pb.TestVariantBranchRaw, error) {
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
	result := &pb.TestVariantBranchRaw{
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
		Length: int64(len(history.Runs)),
		Runs:   []*pb.InputBuffer_Run{},
	}
	for _, run := range history.Runs {
		runpb := &pb.InputBuffer_Run{
			CommitPosition: int64(run.SourcePosition),
			Hour:           timestamppb.New(run.Hour),
		}
		runpb.Counts = &pb.InputBuffer_Run_Counts{
			ExpectedPassCount:    int64(run.Expected.PassCount),
			ExpectedFailCount:    int64(run.Expected.FailCount),
			ExpectedCrashCount:   int64(run.Expected.CrashCount),
			ExpectedAbortCount:   int64(run.Expected.AbortCount),
			UnexpectedPassCount:  int64(run.Unexpected.PassCount),
			UnexpectedFailCount:  int64(run.Unexpected.FailCount),
			UnexpectedCrashCount: int64(run.Unexpected.CrashCount),
			UnexpectedAbortCount: int64(run.Unexpected.AbortCount),
		}
		result.Runs = append(result.Runs, runpb)
	}
	return result
}

// toSegmentsProto converts the segments to the proto segments.
func toSegmentsProto(segments []analyzer.Segment) []*pb.Segment {
	results := make([]*pb.Segment, 0, len(segments))
	for _, segment := range segments {
		results = append(results, toSegmentProto(segment))
	}
	return results
}

func toSegmentProto(segment analyzer.Segment) *pb.Segment {
	return &pb.Segment{
		HasStartChangepoint:          segment.HasStartChangepoint,
		StartPosition:                segment.StartPosition,
		StartPositionLowerBound_99Th: segment.StartPositionLowerBound99Th,
		StartPositionUpperBound_99Th: segment.StartPositionUpperBound99Th,
		StartPositionDistribution:    toPositionDistribution(segment.StartPositionDistribution),
		StartHour:                    timestamppb.New(segment.StartHour),
		EndPosition:                  segment.EndPosition,
		EndHour:                      timestamppb.New(segment.EndHour),
		Counts:                       toCountsProto(segment.Counts),
	}
}

func toPositionDistribution(d *model.PositionDistribution) *pb.Segment_PositionDistribution {
	if d == nil {
		// Not all segments have start position distributions, either because
		// they have no starting changepoint or because they were detected
		// prior to the start position distributions being captured.
		return nil
	}
	// Copy to avoid aliasing the original constants/distributions.
	cdfs := model.TailLikelihoods
	positions := *d
	return &pb.Segment_PositionDistribution{
		Cdfs:            cdfs[:],
		SourcePositions: positions[:],
	}
}

func toCountsProto(counts analyzer.Counts) *pb.Segment_Counts {
	return &pb.Segment_Counts{
		UnexpectedResults:        int32(counts.UnexpectedResults),
		TotalResults:             int32(counts.TotalResults),
		ExpectedPassedResults:    int32(counts.ExpectedPassedResults),
		ExpectedFailedResults:    int32(counts.ExpectedFailedResults),
		ExpectedCrashedResults:   int32(counts.ExpectedCrashedResults),
		ExpectedAbortedResults:   int32(counts.ExpectedAbortedResults),
		UnexpectedPassedResults:  int32(counts.UnexpectedPassedResults),
		UnexpectedFailedResults:  int32(counts.UnexpectedFailedResults),
		UnexpectedCrashedResults: int32(counts.UnexpectedCrashedResults),
		UnexpectedAbortedResults: int32(counts.UnexpectedAbortedResults),

		UnexpectedUnretriedRuns:  int32(counts.UnexpectedUnretriedRuns),
		UnexpectedAfterRetryRuns: int32(counts.UnexpectedAfterRetryRuns),
		FlakyRuns:                int32(counts.FlakyRuns),
		TotalRuns:                int32(counts.TotalRuns),

		TotalVerdicts:      int32(counts.TotalSourceVerdicts),
		UnexpectedVerdicts: int32(counts.UnexpectedSourceVerdicts),
		FlakyVerdicts:      int32(counts.FlakySourceVerdicts),
	}
}

func validateBatchGetTestVariantBranchRequest(req *pb.BatchGetTestVariantBranchRequest) error {
	// MaxTestVariantBranch is the maximum number of test variant branches to be queried in one request.
	const MaxTestVariantBranch = 100

	if len(req.Names) == 0 {
		return errors.New("names: unspecified")
	}
	if len(req.Names) > MaxTestVariantBranch {
		return errors.Fmt("names: no more than %v may be queried at a time", MaxTestVariantBranch)
	}
	// Contents of names collection already validated by caller.
	return nil
}

// QuerySourceVerdicts lists source verdicts for a test variant branch.
func (s *testVariantBranchesServer) QuerySourceVerdicts(ctx context.Context, req *pb.QuerySourceVerdictsRequest) (*pb.QuerySourceVerdictsResponse, error) {
	return nil, appstatus.Error(codes.Unimplemented, "RPC has been decomissioned")
}

// Query queries test variant branches with a given test id and ref.
func (s *testVariantBranchesServer) Query(ctx context.Context, req *pb.QueryTestVariantBranchRequest) (*pb.QueryTestVariantBranchResponse, error) {
	if err := pbutil.ValidateProject(req.GetProject()); err != nil {
		return nil, InvalidArgumentError(errors.Fmt("project: %w", err))
	}
	if err := perms.VerifyProjectPermissions(ctx, req.Project, perms.PermListTestVariantBranches); err != nil {
		return nil, err
	}
	if err := validateQueryTestVariantBranchRequest(req); err != nil {
		return nil, InvalidArgumentError(err)
	}
	pageSize := int(PageSizeLimiter.Adjust(req.PageSize))
	txn, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	opts := testvariantbranch.QueryVariantBranchesOptions{
		PageSize:  pageSize,
		PageToken: req.PageToken,
	}
	refHash := pbutil.SourceRefHash(req.Ref)
	tvbs, nextPageToken, err := testvariantbranch.QueryVariantBranches(txn, req.Project, req.TestId, refHash, opts)
	if err != nil {
		return nil, errors.Fmt("query variant branches: %w", err)
	}
	tvbpbs := make([]*pb.TestVariantBranch, 0, len(tvbs))
	var analysis analyzer.Analyzer
	for _, tvb := range tvbs {
		segments := analysis.Run(tvb)
		tvbpbs = append(tvbpbs, toTestVariantBranchProto(tvb, segments))
	}
	return &pb.QueryTestVariantBranchResponse{
		TestVariantBranch: tvbpbs,
		NextPageToken:     nextPageToken,
	}, nil
}

func validateQueryTestVariantBranchRequest(req *pb.QueryTestVariantBranchRequest) error {
	// Project already validated.
	if err := rdbpbutil.ValidateTestID(req.TestId, rdbpbutil.QuerySideTestIDLimitCallback); err != nil {
		return errors.Fmt("test_id: %w", err)
	}
	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Fmt("page_size: %w", err)
	}
	if err := pbutil.ValidateSourceRef(req.Ref); err != nil {
		return errors.Fmt("ref: %w", err)
	}
	return nil
}

func (s *testVariantBranchesServer) QueryChangepointAIAnalysis(ctx context.Context, req *pb.QueryChangepointAIAnalysisRequest) (*pb.QueryChangepointAIAnalysisResponse, error) {
	if err := checkAllowed(ctx, googlerOnlyGroup); err != nil {
		return nil, err
	}
	if err := validateQueryChangepointAIAnalysisRequest(req); err != nil {
		return nil, InvalidArgumentError(err)
	}
	allowedRealms, err := perms.QueryRealmsNonEmpty(ctx, req.Project, nil, perms.ListTestResultsAndExonerations...)
	if err != nil {
		return nil, err
	}

	refHashBytes, err := hex.DecodeString(req.RefHash)
	if err != nil {
		// This line is unreachable as ref hash should be validated already.
		return nil, InvalidArgumentError(errors.New("ref hash must be an encoded hexadecimal string"))
	}

	request := sorbet.AnalysisRequest{
		Project:             req.Project,
		TestID:              req.TestId,
		VariantHash:         req.VariantHash,
		RefHash:             testvariantbranch.RefHash(refHashBytes),
		StartSourcePosition: req.StartSourcePosition,
		AllowedRealms:       allowedRealms,
	}
	response, err := s.sorbetAnalyzer.Analyze(ctx, request)
	if err != nil {
		return nil, errors.Fmt("analyze: %w", err)
	}
	return &pb.QueryChangepointAIAnalysisResponse{
		AnalysisMarkdown: response.Response,
		Prompt:           response.Prompt,
	}, nil
}

func validateQueryChangepointAIAnalysisRequest(req *pb.QueryChangepointAIAnalysisRequest) error {
	if err := pbutil.ValidateProject(req.GetProject()); err != nil {
		return errors.Fmt("project: %w", err)
	}
	if err := rdbpbutil.ValidateTestID(req.TestId, rdbpbutil.QuerySideTestIDLimitCallback); err != nil {
		return errors.Fmt("test_id: %w", err)
	}
	if err := pbutil.ValidateVariantHash(req.VariantHash); err != nil {
		return errors.Fmt("variant_hash: %w", err)
	}
	if err := ValidateRefHash(req.RefHash); err != nil {
		return errors.Fmt("ref_hash: %w", err)
	}
	if req.StartSourcePosition <= 0 {
		return errors.New("start_source_position: must be a positive number")
	}
	return nil
}
