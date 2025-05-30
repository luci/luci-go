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
	"sort"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
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
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testresults/lowlatency"
	"go.chromium.org/luci/analysis/internal/testverdicts"
	"go.chromium.org/luci/analysis/internal/tracing"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

type TestVerdictClient interface {
	ReadTestVerdictAfterPosition(ctx context.Context, options testverdicts.ReadVerdictAtOrAfterPositionOptions) (*testverdicts.SourceVerdict, error)
}

type TestResultClient interface {
	ReadSourceVerdicts(ctx context.Context, options testresults.ReadSourceVerdictsOptions) ([]testresults.SourceVerdict, error)
}

// NewTestVariantBranchesServer returns a new pb.TestVariantBranchesServer.
func NewTestVariantBranchesServer(tvc TestVerdictClient, trc TestResultClient, sc sorbet.GenerateClient) pb.TestVariantBranchesServer {
	return &pb.DecoratedTestVariantBranches{
		Prelude: checkAllowedPrelude,
		Service: &testVariantBranchesServer{
			testResultClient: trc,
			sorbetAnalyzer:   sorbet.NewAnalyzer(sc, tvc),
		},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// testVariantBranchesServer implements pb.TestVariantAnalysesServer.
type testVariantBranchesServer struct {
	testResultClient TestResultClient
	sorbetAnalyzer   *sorbet.Analyzer
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
		return nil, invalidArgumentError(errors.Fmt("name: %w", err))
	}
	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetTestVariantBranch); err != nil {
		return nil, err
	}

	// Check ref hash.
	refHashBytes, err := hex.DecodeString(refHash)
	if err != nil {
		return nil, invalidArgumentError(errors.New("name: ref component must be an encoded hexadecimal string"))
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
			return nil, invalidArgumentError(errors.Fmt("names[%d]: %w", i, err))
		}
		if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetTestVariantBranch); err != nil {
			return nil, err
		}
	}
	if err := validateBatchGetTestVariantBranchRequest(req); err != nil {
		return nil, invalidArgumentError(err)
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
			CommitPosition: int64(run.CommitPosition),
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
	// Only perform request validation necessary to extract the project, which is
	// necessary to perform the authorisation check.
	project, testID, variantHash, refHash, err := parseTestVariantBranchName(req.Parent)
	if err != nil {
		return nil, invalidArgumentError(errors.Fmt("parent: %w", err))
	}

	// Perform authorization check first as per https://google.aip.dev/211.
	allowedSubRealms, err := perms.QuerySubRealmsNonEmpty(ctx, project, "", nil, perms.ListTestResultsAndExonerations...)
	if err != nil {
		// err is already an appstatus-annotated permission error or internal server error.
		return nil, err
	}

	if err := validateQuerySourceVerdictsRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}

	// Check ref hash.
	refHashBytes, err := hex.DecodeString(refHash)
	if err != nil {
		// Should never happen as ref hash already validated.
		panic(errors.Fmt("decode ref hash: %w", err))
	}

	// Fetch test verdicts (grouped into source verdicts) by combining
	// the last 14 days of data from Spanner with older data from
	// BigQuery.
	//
	// This attempts to deliver the most performant solution that
	// still has access to 510 days of test history, by:
	// - Using data from Spanner where it is available (at time of
	//   writing, Spanner's TestResultsBySourcePosition retains 15
	//   days' worth of data but we use 14 days to avoid queries
	//   racing with deletions); observing Spanner is very fast
	//   for such accesses.
	// - Using BigQuery for older data, observing that BigQuery has
	//   510 days of retention. By using Spanner for newer data,
	//   we avoid reading from the streaming buffer (write-optimised
	//   data stores), which is slow to access. Whereas older data
	//   is in read-optimised data stores, which is fast to access.

	// Determine the split.
	// Partition times >= splitPartitionTime we read from Spanner,
	// whereas partition times < splitPartitionTime we read from
	// BigQuery. This ensures we do not read the same data twice.
	splitPartitionTime := clock.Now(ctx).Add(-14 * 24 * time.Hour)

	var spannerSourceVerdicts []lowlatency.SourceVerdict
	var bigQuerySourceVerdicts []testresults.SourceVerdict
	err = parallel.FanOutIn(func(c chan<- func() error) {
		c <- func() error {
			// Fetch test verdicts (grouped into source verdicts) from Spanner.
			opts := lowlatency.ReadSourceVerdictsOptions{
				Project:             project,
				TestID:              testID,
				VariantHash:         variantHash,
				RefHash:             refHashBytes,
				StartSourcePosition: req.StartSourcePosition,
				EndSourcePosition:   req.EndSourcePosition,
				AllowedSubrealms:    allowedSubRealms,
				StartPartitionTime:  splitPartitionTime,
			}
			var err error
			spannerSourceVerdicts, err = lowlatency.ReadSourceVerdicts(span.Single(ctx), opts)
			if err != nil {
				return errors.Fmt("read source verdicts from Spanner: %w", err)
			}
			return nil
		}
		c <- func() error {
			// Fetch test verdicts (grouped into source verdicts) from BigQuery.
			opts := testresults.ReadSourceVerdictsOptions{
				Project:             project,
				TestID:              testID,
				VariantHash:         variantHash,
				RefHash:             refHash,
				StartSourcePosition: req.StartSourcePosition,
				EndSourcePosition:   req.EndSourcePosition,
				AllowedSubrealms:    allowedSubRealms,
				EndPartitionTime:    splitPartitionTime,
			}
			var err error
			bigQuerySourceVerdicts, err = s.testResultClient.ReadSourceVerdicts(ctx, opts)
			if err != nil {
				return errors.Fmt("read source verdicts from BigQuery: %w", err)
			}
			return nil
		}
	})
	if err != nil {
		return nil, err
	}

	return &pb.QuerySourceVerdictsResponse{
		SourceVerdicts: mergeSourceVerdicts(spannerSourceVerdicts, bigQuerySourceVerdicts),
	}, nil
}

func validateQuerySourceVerdictsRequest(req *pb.QuerySourceVerdictsRequest) error {
	// Do not need to validate .Parent field, that is validated in the caller.

	if req.StartSourcePosition <= 0 {
		return errors.New("start_source_position: must be a positive number")
	}
	// EndSourcePosition is an exclusive bound.
	// We should accept `0` otherwise source verdicts at CP#1 cannot be queried.
	if req.EndSourcePosition < 0 {
		return errors.New("end_source_position: must be a non-negative number")
	}
	if req.EndSourcePosition >= req.StartSourcePosition {
		return errors.New("end_source_position: must be less than start_source_position")
	}
	if req.StartSourcePosition-req.EndSourcePosition > 1000 {
		return errors.New("end_source_position: must not query more than 1000 source positions from start_source_position")
	}
	return nil
}

func mergeSourceVerdicts(spanner []lowlatency.SourceVerdict, bq []testresults.SourceVerdict) []*pb.QuerySourceVerdictsResponse_SourceVerdict {
	// verdictsBySourcePosition collects all test verdicts at a source position.
	// At each source position, verdicts are sorted latest partition time first.
	verdictsBySourcePosition := make(map[int64][]*pb.QuerySourceVerdictsResponse_TestVerdict)

	// Test verdicts retrieved from BigQuery have strictly older partition times
	// than those from Spanner due to the query split, so are appended before
	// those from Spanner. Within each source position, verdicts are
	// sorted oldest partition time first.
	for _, row := range bq {
		verdictsBySourcePosition[row.Position] = toTestVerdictsBigQuery(row.Verdicts)
	}
	for _, row := range spanner {
		verdictsBySourcePosition[row.Position] = append(verdictsBySourcePosition[row.Position], toTestVerdictsSpanner(row.Verdicts)...)
	}

	results := make([]*pb.QuerySourceVerdictsResponse_SourceVerdict, 0, len(verdictsBySourcePosition))
	for sourcePosition, verdicts := range verdictsBySourcePosition {
		// Limit to 20 most recent verdicts (by partition time)
		// per source position.
		if len(verdicts) > 20 {
			verdicts = verdicts[:20]
		}

		results = append(results, &pb.QuerySourceVerdictsResponse_SourceVerdict{
			Position: sourcePosition,
			// Compute status only based on the returned verdicts, for consistency.
			Status:   aggregateStatus(verdicts),
			Verdicts: verdicts,
		})
	}

	// Sort source verdicts in descending position order, i.e. largest
	// position first.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Position > results[j].Position
	})
	return results
}

func aggregateStatus(verdicts []*pb.QuerySourceVerdictsResponse_TestVerdict) pb.QuerySourceVerdictsResponse_VerdictStatus {
	hasExpected := false
	hasUnexpected := false
	for _, v := range verdicts {
		switch v.Status {
		case pb.QuerySourceVerdictsResponse_EXPECTED:
			hasExpected = true
		case pb.QuerySourceVerdictsResponse_UNEXPECTED:
			hasUnexpected = true
		case pb.QuerySourceVerdictsResponse_FLAKY:
			hasExpected = true
			hasUnexpected = true
		case pb.QuerySourceVerdictsResponse_SKIPPED:
		}
	}
	if hasExpected && hasUnexpected {
		return pb.QuerySourceVerdictsResponse_FLAKY
	} else if hasExpected {
		return pb.QuerySourceVerdictsResponse_EXPECTED
	} else if hasUnexpected {
		return pb.QuerySourceVerdictsResponse_UNEXPECTED
	}
	return pb.QuerySourceVerdictsResponse_SKIPPED
}

func toTestVerdictsSpanner(tvs []lowlatency.SourceVerdictTestVerdict) []*pb.QuerySourceVerdictsResponse_TestVerdict {
	result := make([]*pb.QuerySourceVerdictsResponse_TestVerdict, 0, len(tvs))
	for _, tv := range tvs {
		result = append(result, &pb.QuerySourceVerdictsResponse_TestVerdict{
			InvocationId:  tv.RootInvocationID,
			PartitionTime: timestamppb.New(tv.PartitionTime),
			Status:        tv.Status,
			Changelists:   toChangelistSpanner(tv.Changelists),
		})

	}
	return result
}

func toChangelistSpanner(changelists []testresults.Changelist) []*pb.Changelist {
	result := make([]*pb.Changelist, 0, len(changelists))
	for _, c := range changelists {
		result = append(result, &pb.Changelist{
			Host:      c.Host,
			Change:    c.Change,
			Patchset:  int32(c.Patchset),
			OwnerKind: c.OwnerKind,
		})
	}
	return result
}

func toTestVerdictsBigQuery(tvs []testresults.SourceVerdictTestVerdict) []*pb.QuerySourceVerdictsResponse_TestVerdict {
	result := make([]*pb.QuerySourceVerdictsResponse_TestVerdict, 0, len(tvs))
	for _, tv := range tvs {
		result = append(result, &pb.QuerySourceVerdictsResponse_TestVerdict{
			InvocationId:  tv.InvocationID,
			PartitionTime: timestamppb.New(tv.PartitionTime),
			Status:        pb.QuerySourceVerdictsResponse_VerdictStatus(pb.QuerySourceVerdictsResponse_VerdictStatus_value[tv.Status]),
			Changelists:   toChangelistBigQuery(tv.Changelists),
		})

	}
	return result
}

func toChangelistBigQuery(cls []testresults.BQChangelist) []*pb.Changelist {
	result := make([]*pb.Changelist, 0, len(cls))
	for _, cl := range cls {
		result = append(result, &pb.Changelist{
			Host:      cl.Host.String(),
			Change:    cl.Change.Int64,
			Patchset:  int32(cl.Patchset.Int64),
			OwnerKind: pb.ChangelistOwnerKind(pb.ChangelistOwnerKind_value[cl.OwnerKind.String()]),
		})
	}
	return result
}

// Query queries test variant branches with a given test id and ref.
func (s *testVariantBranchesServer) Query(ctx context.Context, req *pb.QueryTestVariantBranchRequest) (*pb.QueryTestVariantBranchResponse, error) {
	if err := pbutil.ValidateProject(req.GetProject()); err != nil {
		return nil, invalidArgumentError(errors.Fmt("project: %w", err))
	}
	if err := perms.VerifyProjectPermissions(ctx, req.Project, perms.PermListTestVariantBranches); err != nil {
		return nil, err
	}
	if err := validateQueryTestVariantBranchRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}
	pageSize := int(pageSizeLimiter.Adjust(req.PageSize))
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
	if err := rdbpbutil.ValidateTestID(req.TestId); err != nil {
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
		return nil, invalidArgumentError(err)
	}
	allowedRealms, err := perms.QueryRealmsNonEmpty(ctx, req.Project, nil, perms.ListTestResultsAndExonerations...)
	if err != nil {
		return nil, err
	}

	refHashBytes, err := hex.DecodeString(req.RefHash)
	if err != nil {
		// This line is unreachable as ref hash should be validated already.
		return nil, invalidArgumentError(errors.New("ref hash must be an encoded hexadecimal string"))
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
	if err := rdbpbutil.ValidateTestID(req.TestId); err != nil {
		return errors.Fmt("test_id: %w", err)
	}
	if err := ValidateVariantHash(req.VariantHash); err != nil {
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
