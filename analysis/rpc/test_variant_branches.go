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
	"strconv"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/grpc/appstatus"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/changepoints/analyzer"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/changepoints/sorbet"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	"go.chromium.org/luci/analysis/internal/gitiles"
	"go.chromium.org/luci/analysis/internal/pagination"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testverdicts"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

type TestVerdictClient interface {
	ReadTestVerdictsPerSourcePosition(ctx context.Context, options testverdicts.ReadTestVerdictsPerSourcePositionOptions) ([]*testverdicts.CommitWithVerdicts, error)
	ReadTestVerdictAfterPosition(ctx context.Context, options testverdicts.ReadVerdictAtOrAfterPositionOptions) (*testverdicts.SourceVerdict, error)
}

// NewTestVariantBranchesServer returns a new pb.TestVariantBranchesServer.
func NewTestVariantBranchesServer(tvc TestVerdictClient, sc sorbet.GenerateClient) pb.TestVariantBranchesServer {
	return &pb.DecoratedTestVariantBranches{
		Prelude:  checkAllowedPrelude,
		Service:  &testVariantBranchesServer{testVerdictClient: tvc, sorbetAnalyzer: sorbet.NewAnalyzer(sc, tvc)},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// testVariantBranchesServer implements pb.TestVariantAnalysesServer.
type testVariantBranchesServer struct {
	testVerdictClient TestVerdictClient
	sorbetAnalyzer    *sorbet.Analyzer
}

// Get fetches Spanner for test variant analysis.
func (*testVariantBranchesServer) GetRaw(ctx context.Context, req *pb.GetRawTestVariantBranchRequest) (*pb.TestVariantBranchRaw, error) {
	// Currently, we only allow LUCI Analysis admins to use this API.
	// In the future, if this end point is used for the UI, we should
	// have proper ACL check.
	if err := checkAllowed(ctx, luciAnalysisAdminGroup); err != nil {
		return nil, err
	}
	tvbk, err := validateGetRawTestVariantBranchRequest(req)
	if err != nil {
		return nil, invalidArgumentError(err)
	}

	txn, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	tvbs, err := testvariantbranch.Read(txn, []testvariantbranch.Key{tvbk})
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
	analysis, err := toTestVariantBranchRawProto(tvbs[0])
	if err != nil {
		return nil, errors.Annotate(err, "build proto").Err()
	}
	return analysis, nil
}

// BatchGet returns current state of segments for test variant branches.
func (*testVariantBranchesServer) BatchGet(ctx context.Context, req *pb.BatchGetTestVariantBranchRequest) (*pb.BatchGetTestVariantBranchResponse, error) {
	// Currently, we only allow Googlers to use this API.
	// TODO: implement proper ACL check with realms.
	if err := checkAllowed(ctx, googlerOnlyGroup); err != nil {
		return nil, err
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
			return nil, errors.Reason("ref hash must be an encoded hexadecimal string").Err()
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
		return nil, errors.Annotate(err, "read test variant branch").Err()
	}
	tvbpbs := make([]*pb.TestVariantBranch, 0, len(req.Names))
	var analysis analyzer.Analyzer
	for _, tvb := range tvbs {
		if tvb == nil {
			tvbpbs = append(tvbpbs, nil)
			continue
		}
		// Run analysis to get logical segments for the test variant branch.
		segments := analysis.Run(tvb)

		tvbpbs = append(tvbpbs, toTestVariantBranchProto(tvb, segments))
	}
	return &pb.BatchGetTestVariantBranchResponse{
		TestVariantBranches: tvbpbs,
	}, nil
}

func validateGetRawTestVariantBranchRequest(req *pb.GetRawTestVariantBranchRequest) (testvariantbranch.Key, error) {
	project, testID, variantHash, refHash, err := parseTestVariantBranchName(req.Name)
	if err != nil {
		return testvariantbranch.Key{}, errors.Annotate(err, "name").Err()
	}

	// Check ref hash.
	refHashBytes, err := hex.DecodeString(refHash)
	if err != nil {
		return testvariantbranch.Key{}, errors.Reason("ref component must be an encoded hexadecimal string").Err()
	}
	return testvariantbranch.Key{
		Project:     project,
		TestID:      testID,
		VariantHash: variantHash,
		RefHash:     testvariantbranch.RefHash(refHashBytes),
	}, nil
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
		StartHour:                    timestamppb.New(segment.StartHour),
		EndPosition:                  segment.EndPosition,
		EndHour:                      timestamppb.New(segment.EndHour),
		Counts:                       toCountsProto(segment.Counts),
	}
}

func toCountsProto(counts analyzer.Counts) *pb.Segment_Counts {
	return &pb.Segment_Counts{
		TotalVerdicts:      int32(counts.TotalSourceVerdicts),
		UnexpectedVerdicts: int32(counts.UnexpectedSourceVerdicts),
		FlakyVerdicts:      int32(counts.FlakySourceVerdicts),
	}
}

func validateBatchGetTestVariantBranchRequest(req *pb.BatchGetTestVariantBranchRequest) error {
	// MaxTestVariantBranch is the maximum number of test variant branches to be queried in one request.
	const MaxTestVariantBranch = 100

	if len(req.Names) == 0 {
		return errors.Reason("names: unspecified").Err()
	}
	if len(req.Names) > MaxTestVariantBranch {
		return errors.Reason("names: no more than %v may be queried at a time", MaxTestVariantBranch).Err()
	}
	for _, name := range req.Names {
		if _, _, _, _, err := parseTestVariantBranchName(name); err != nil {
			return errors.Annotate(err, "name %s", name).Err()
		}
	}
	return nil
}

// QuerySourcePositions returns commits and the test verdicts at these commits, starting from a source position.
func (s *testVariantBranchesServer) QuerySourcePositions(ctx context.Context, req *pb.QuerySourcePositionsRequest) (*pb.QuerySourcePositionsResponse, error) {
	allowedRealms, err := perms.QueryRealmsNonEmpty(ctx, req.Project, nil, perms.ListTestResultsAndExonerations...)
	if err != nil {
		return nil, err
	}
	if err := validateQuerySourcePositionsRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}
	pageSize := int64(pageSizeLimiter.Adjust(req.PageSize))
	startPosition := req.StartSourcePosition
	if req.PageToken != "" {
		startPosition, err = parseQuerySourcePositionsPageToken(req.PageToken)
		if err != nil {
			return nil, err
		}
	}
	options := testverdicts.ReadTestVerdictsPerSourcePositionOptions{
		Project:       req.Project,
		TestID:        req.TestId,
		VariantHash:   req.VariantHash,
		RefHash:       req.RefHash,
		AllowedRealms: allowedRealms,
		// This query aggregate over source position so that there is at most one row per source position.
		// With the following setting of PositionMustGreater and NumCommits, the largest source position in the query result must
		// greater than startPosition unless no such commit exists.
		PositionMustGreater: startPosition - pageSize,
		NumCommits:          pageSize,
	}
	commitsWithVerdicts, err := s.testVerdictClient.ReadTestVerdictsPerSourcePosition(ctx, options)
	if err != nil {
		return nil, errors.Annotate(err, "read test verdicts from BigQuery").Err()
	}

	// Android test results are not supported.
	if len(commitsWithVerdicts) > 0 && commitsWithVerdicts[0].Realm == "infra-internal:ants-experiment" {
		return nil, appstatus.Errorf(codes.InvalidArgument, "QuerySourcePositions doesn't support Android test results")
	}

	// Gitiles log requires us to supply a commit hash to fetch a commit log.
	// Ideally, if we know the commit hash of the requested startPosition, we can just
	// call gitiles with that, and page (backwards) through the commit history.
	// However, as test verdicts are spare, we may not have the commit hash for the given startPosition.
	// Therefore, we need to find the commit which is the closest to the startPosition
	// but equal to or after startPosition (call it closestAfterCommit), so that we can call gitiles with that commit hash.
	// Commits starts from startPosition will be at offset (closestAfterCommitPosition - startPosition) in the gitiles response.
	closestAfterCommitHash, offset, exist := closestAfterCommit(startPosition, commitsWithVerdicts)
	if !exist {
		return nil, appstatus.Errorf(codes.NotFound, "no commit at or after the requested start position")
	}

	// We want to get (pagesize + offset) commits start from the closestAfterCommit.
	// One gitiles log call returns at most 10000 commits.
	// It is unlikely that (pagesize + offset) will be greater than 10000, we just throw NotFound here if that happens.
	requiredNumCommits := pageSize + offset
	if requiredNumCommits > 10000 {
		return nil, appstatus.Errorf(codes.NotFound, "cannot find source positions because test verdicts is too sparse")
	}
	ref := commitsWithVerdicts[0].Ref
	gitilesClient, err := gitiles.NewClient(ctx, ref.Gitiles.Host.String(), auth.AsCredentialsForwarder)
	if err != nil {
		return nil, errors.Annotate(err, "create gitiles client").Err()
	}
	logReq := &gitilespb.LogRequest{
		Project:    ref.Gitiles.Project.String(),
		Committish: closestAfterCommitHash,
		PageSize:   int32(requiredNumCommits),
		TreeDiff:   true,
	}
	logRes, err := gitilesClient.Log(ctx, logReq)
	if err != nil {
		return nil, errors.Annotate(err, "gitiles log").Err()
	}
	// The response from gitiles contains commits from closestAfterCommit.
	// Commit at the requested start position is at offset.
	logs := logRes.Log[offset:]
	res := []*pb.SourcePosition{}
	for i, commit := range logs {
		pos := startPosition - int64(i)
		commitWithVerdicts := commitWithVerdictsAtSourcePosition(commitsWithVerdicts, pos)
		tvspb := []*pb.TestVerdict{}
		if commitWithVerdicts != nil {
			tvspb = toVerdictsProto(commitWithVerdicts.TestVerdicts)
		}
		cwv := &pb.SourcePosition{
			Commit:   commit,
			Position: pos,
			Verdicts: tvspb,
		}
		res = append(res, cwv)
	}
	// Page token contains the source position where next page should start from.
	nextPageToken := pagination.Token(fmt.Sprintf("%d", startPosition-pageSize))
	return &pb.QuerySourcePositionsResponse{
		SourcePositions: res,
		NextPageToken:   nextPageToken,
	}, nil
}

func toVerdictsProto(bqVerdicts []*testverdicts.TestVerdict) []*pb.TestVerdict {
	res := make([]*pb.TestVerdict, 0, len(bqVerdicts))
	for _, tv := range bqVerdicts {
		if !tv.HasAccess {
			// The caller doesn't have access to this test verdict.
			continue
		}
		tvpb := &pb.TestVerdict{
			TestId:        tv.TestID,
			VariantHash:   tv.VariantHash,
			InvocationId:  tv.InvocationID,
			Status:        pb.TestVerdictStatus(pb.TestVerdictStatus_value[tv.Status]),
			PartitionTime: timestamppb.New(tv.PartitionTime),
			Changelists:   []*pb.Changelist{},
		}
		if tv.PassedAvgDurationUsec.Valid {
			tvpb.PassedAvgDuration = durationpb.New(time.Duration(int(tv.PassedAvgDurationUsec.Float64*1000)) * time.Millisecond)
		}
		for _, cl := range tv.Changelists {
			tvpb.Changelists = append(tvpb.Changelists, &pb.Changelist{
				Host:      cl.Host.String(),
				Change:    cl.Change.Int64,
				Patchset:  int32(cl.Patchset.Int64),
				OwnerKind: pb.ChangelistOwnerKind(pb.ChangelistOwnerKind_value[cl.OwnerKind.String()]),
			})
		}
		res = append(res, tvpb)
	}
	return res
}

// commitWithVerdictsAtSourcePosition find the testverdicts.CommitWithVerdicts at a certain source position.
// It returns nil if not found.
func commitWithVerdictsAtSourcePosition(commits []*testverdicts.CommitWithVerdicts, sourcePosition int64) *testverdicts.CommitWithVerdicts {
	for _, commit := range commits {
		if sourcePosition == commit.Position {
			return commit
		}
	}
	return nil
}

// closestAfterCommit returns the commit hash belongs to the commit in rows which is closest (or equal) to commit at pos and after pos.
// This function assumes rows are sorted by source position ASC.
func closestAfterCommit(pos int64, rows []*testverdicts.CommitWithVerdicts) (commitHash string, offset int64, exist bool) {
	for _, r := range rows {
		if r.Position >= pos {
			return r.CommitHash, r.Position - pos, true
		}
	}
	return "", 0, false
}

func parseQuerySourcePositionsPageToken(pageToken string) (int64, error) {
	tokens, err := pagination.ParseToken(pageToken)
	if err != nil {
		return 0, invalidArgumentError(err)
	}
	if len(tokens) != 1 {
		return 0, invalidArgumentError(err)
	}
	pos, err := strconv.Atoi(tokens[0])
	if err != nil {
		return 0, invalidArgumentError(err)
	}
	return int64(pos), nil
}

func validateQuerySourcePositionsRequest(req *pb.QuerySourcePositionsRequest) error {
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return errors.Annotate(err, "project").Err()
	}
	if err := rdbpbutil.ValidateTestID(req.TestId); err != nil {
		return errors.Annotate(err, "test_id").Err()
	}
	if err := ValidateVariantHash(req.VariantHash); err != nil {
		return errors.Annotate(err, "variant_hash").Err()
	}
	if err := ValidateRefHash(req.RefHash); err != nil {
		return errors.Annotate(err, "ref_hash").Err()
	}
	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Annotate(err, "page_size").Err()
	}
	if req.StartSourcePosition <= 0 {
		return errors.Reason("start_source_position: must be a positive number").Err()
	}
	return nil
}

// Query queries test variant branches with a given test id and ref.
func (s *testVariantBranchesServer) Query(ctx context.Context, req *pb.QueryTestVariantBranchRequest) (*pb.QueryTestVariantBranchResponse, error) {
	// Currently, we only allow Googlers to use this API.
	// TODO: implement proper ACL check with realms.
	if err := checkAllowed(ctx, googlerOnlyGroup); err != nil {
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
		return nil, errors.Annotate(err, "query variant branches").Err()
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
	if err := pbutil.ValidateProject(req.GetProject()); err != nil {
		return errors.Annotate(err, "project").Err()
	}
	if err := rdbpbutil.ValidateTestID(req.TestId); err != nil {
		return errors.Annotate(err, "test_id").Err()
	}
	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Annotate(err, "page_size").Err()
	}
	if err := pbutil.ValidateSourceRef(req.Ref); err != nil {
		return errors.Annotate(err, "ref").Err()
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
		return nil, invalidArgumentError(errors.Reason("ref hash must be an encoded hexadecimal string").Err())
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
		return nil, errors.Annotate(err, "analyze").Err()
	}
	return &pb.QueryChangepointAIAnalysisResponse{
		AnalysisMarkdown: response.Response,
		Prompt:           response.Prompt,
	}, nil
}

func validateQueryChangepointAIAnalysisRequest(req *pb.QueryChangepointAIAnalysisRequest) error {
	if err := pbutil.ValidateProject(req.GetProject()); err != nil {
		return errors.Annotate(err, "project").Err()
	}
	if err := rdbpbutil.ValidateTestID(req.TestId); err != nil {
		return errors.Annotate(err, "test_id").Err()
	}
	if err := ValidateVariantHash(req.VariantHash); err != nil {
		return errors.Annotate(err, "variant_hash").Err()
	}
	if err := ValidateRefHash(req.RefHash); err != nil {
		return errors.Annotate(err, "ref_hash").Err()
	}
	if req.StartSourcePosition <= 0 {
		return errors.Reason("start_source_position: must be a positive number").Err()
	}
	return nil
}
