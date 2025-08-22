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

// Package server implements the LUCI Bisection servers to handle pRPC requests.
package server

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/pagination"
	"go.chromium.org/luci/common/pagination/dscursor"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"

	"go.chromium.org/luci/bisection/analysis"
	"go.chromium.org/luci/bisection/compilefailureanalysis/heuristic"
	"go.chromium.org/luci/bisection/compilefailureanalysis/nthsection"
	"go.chromium.org/luci/bisection/internal/tracing"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"
	"go.chromium.org/luci/bisection/util/protoutil"
)

var listAnalysesPageTokenVault = dscursor.NewVault([]byte("luci.bisection.v1.ListAnalyses"))
var listTestAnalysesPageTokenVault = dscursor.NewVault([]byte("luci.bisection.v1.ListTestAnalyses"))

// Max and default page sizes for ListAnalyses - the proto should be updated
// to reflect any changes to these values.
var listAnalysesPageSizeLimiter = PageSizeLimiter{
	Max:     200,
	Default: 50,
}

// Max and default page sizes for ListTestAnalyses - the proto should be updated
// to reflect any changes to these values.
var listTestAnalysesPageSizeLimiter = PageSizeLimiter{
	Max:     200,
	Default: 50,
}

// AnalysesServer implements the LUCI Bisection proto service for Analyses.
type AnalysesServer struct {
	// Hostname of the LUCI Analysis pRPC service, e.g. analysis.api.luci.app
	LUCIAnalysisHost string
}

// GetAnalysis returns the analysis given the analysis id
func (server *AnalysesServer) GetAnalysis(c context.Context, req *pb.GetAnalysisRequest) (*pb.Analysis, error) {
	c = loggingutil.SetAnalysisID(c, req.AnalysisId)
	analysis := &model.CompileFailureAnalysis{
		Id: req.AnalysisId,
	}
	switch err := datastore.Get(c, analysis); err {
	case nil:
		//continue
	case datastore.ErrNoSuchEntity:
		return nil, status.Errorf(codes.NotFound, "Analysis %d not found: %v", req.AnalysisId, err)
	default:
		return nil, status.Errorf(codes.Internal, "Error in retrieving analysis: %s", err)
	}
	result, err := GetAnalysisResult(c, analysis)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Error getting analysis result: %s", err)
	}
	return result, nil
}

// QueryAnalysis returns the analysis given a query
func (server *AnalysesServer) QueryAnalysis(c context.Context, req *pb.QueryAnalysisRequest) (*pb.QueryAnalysisResponse, error) {
	if err := validateQueryAnalysisRequest(req); err != nil {
		return nil, err
	}
	if req.BuildFailure.FailedStepName != "compile" {
		return nil, status.Errorf(codes.Unimplemented, "only compile failures are supported")
	}
	bbid := req.BuildFailure.GetBbid()
	c = loggingutil.SetQueryBBID(c, bbid)
	logging.Infof(c, "QueryAnalysis for build %d", bbid)

	analysis, err := datastoreutil.GetAnalysisForBuild(c, bbid)
	if err != nil {
		logging.Errorf(c, "Could not query analysis for build %d: %s", bbid, err)
		return nil, status.Errorf(codes.Internal, "failed to get analysis for build %d: %s", bbid, err)
	}
	if analysis == nil {
		logging.Infof(c, "No analysis for build %d", bbid)
		return nil, status.Errorf(codes.NotFound, "analysis not found for build %d", bbid)
	}
	c = loggingutil.SetAnalysisID(c, analysis.Id)
	analysispb, err := GetAnalysisResult(c, analysis)
	if err != nil {
		logging.Errorf(c, "Could not get analysis data for build %d: %s", bbid, err)
		return nil, status.Errorf(codes.Internal, "failed to get analysis data %s", err)
	}

	res := &pb.QueryAnalysisResponse{
		Analyses: []*pb.Analysis{analysispb},
	}
	return res, nil
}

// TriggerAnalysis triggers an analysis for a failure
func (server *AnalysesServer) TriggerAnalysis(c context.Context, req *pb.TriggerAnalysisRequest) (*pb.TriggerAnalysisResponse, error) {
	// TODO(nqmtuan): Implement this
	return nil, nil
}

// UpdateAnalysis updates the information of an analysis.
// At the mean time, it is only used for update the bugs associated with an
// analysis.
func (server *AnalysesServer) UpdateAnalysis(c context.Context, req *pb.UpdateAnalysisRequest) (*pb.Analysis, error) {
	// TODO(nqmtuan): Implement this
	return nil, nil
}

func (server *AnalysesServer) ListTestAnalyses(ctx context.Context, req *pb.ListTestAnalysesRequest) (*pb.ListTestAnalysesResponse, error) {
	logging.Infof(ctx, "ListTestAnalyses for project %s", req.Project)

	// Validate the request.
	if err := validateListTestAnalysesRequest(req); err != nil {
		return nil, err
	}

	// By default, returning all fields.
	fieldMask := req.Fields
	if fieldMask == nil {
		fieldMask = defaultFieldMask()
	}
	mask, err := mask.FromFieldMask(fieldMask, &pb.TestAnalysis{}, mask.AdvancedSemantics())
	if err != nil {
		return nil, errors.Fmt("from field mask: %w", err)
	}

	// Decode cursor from page token.
	cursor, err := listTestAnalysesPageTokenVault.Cursor(ctx, req.PageToken)
	switch err {
	case pagination.ErrInvalidPageToken:
		return nil, status.Errorf(codes.InvalidArgument, "invalid page token")
	case nil:
		// Continue
	default:
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Override the page size if necessary.
	pageSize := int(listTestAnalysesPageSizeLimiter.Adjust(req.PageSize))

	// Query datastore for test analyses.
	q := datastore.NewQuery("TestFailureAnalysis").Eq("project", req.Project).Order("-create_time").Start(cursor)
	tfas := make([]*model.TestFailureAnalysis, 0, pageSize)
	var nextCursor datastore.Cursor
	err = datastore.Run(ctx, q, func(tfa *model.TestFailureAnalysis, getCursor datastore.CursorCB) error {
		tfas = append(tfas, tfa)

		// Check whether the page size limit has been reached
		if len(tfas) == pageSize {
			nextCursor, err = getCursor()
			if err != nil {
				return err
			}
			return datastore.Stop
		}
		return nil
	})
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Construct the next page token.
	nextPageToken, err := listTestAnalysesPageTokenVault.PageToken(ctx, nextCursor)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Get the result for each test failure analysis.
	analyses := make([]*pb.TestAnalysis, len(tfas))
	err = parallel.FanOutIn(func(workC chan<- func() error) {
		for i, tfa := range tfas {
			// Assign to local variables.
			workC <- func() error {
				analysis, err := protoutil.TestFailureAnalysisToPb(ctx, tfa, mask)
				if err != nil {
					err = errors.Fmt("test failure analysis to pb: %w", err)
					logging.Errorf(ctx, "Could not get analysis data for analysis %d: %s", tfa.ID, err)
					return err
				}
				analyses[i] = analysis
				return nil
			}
		}
	})
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.ListTestAnalysesResponse{
		Analyses:      analyses,
		NextPageToken: nextPageToken,
	}, nil
}

func (server *AnalysesServer) GetTestAnalysis(ctx context.Context, req *pb.GetTestAnalysisRequest) (*pb.TestAnalysis, error) {
	ctx = loggingutil.SetAnalysisID(ctx, req.AnalysisId)

	// By default, returning all fields.
	fieldMask := req.Fields
	if fieldMask == nil {
		fieldMask = defaultFieldMask()
	}
	mask, err := mask.FromFieldMask(fieldMask, &pb.TestAnalysis{}, mask.AdvancedSemantics())
	if err != nil {
		return nil, errors.Fmt("from field mask: %w", err)
	}

	tfa, err := datastoreutil.GetTestFailureAnalysis(ctx, req.AnalysisId)
	if err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			logging.Errorf(ctx, err.Error())
			return nil, status.Errorf(codes.NotFound, "analysis not found: %v", err)
		}
		err = errors.Fmt("get test failure analysis: %w", err)
		logging.Errorf(ctx, err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}
	result, err := protoutil.TestFailureAnalysisToPb(ctx, tfa, mask)
	if err != nil {
		err = errors.Fmt("test failure analysis to pb: %w", err)
		logging.Errorf(ctx, err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}
	return result, nil
}

func (server *AnalysesServer) BatchGetTestAnalyses(ctx context.Context, req *pb.BatchGetTestAnalysesRequest) (res *pb.BatchGetTestAnalysesResponse, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/bisection/server/analyses.BatchGetTestAnalyses")
	defer func() { tracing.End(ts, err) }()

	// Adding a time value to the context when we are logging, so we know
	// what request we are logging, because we may process many requests at the same time.
	// We can also add hash of the request, or other information about the requests,
	// but it is not neccessary for now.
	ctx = logging.SetField(ctx, "request_time_unix_millis", time.Now().UnixMilli())

	// Validate request.
	if err := validateBatchGetTestAnalysesRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// By default, returning all fields.
	fieldMask := req.Fields
	if fieldMask == nil {
		fieldMask = defaultFieldMask()
	}
	tfamask, err := mask.FromFieldMask(fieldMask, &pb.TestAnalysis{}, mask.AdvancedSemantics())
	if err != nil {
		return nil, errors.Fmt("from field mask: %w", err)
	}

	// For test failures that have source position specified, we do not need to query LUCI Analysis.
	// We only need to query LUCI Analysis for test failures that do not have source position specified.
	luciAnalysisRequests := []*pb.BatchGetTestAnalysesRequest_TestFailureIdentifier{}
	for _, tf := range req.TestFailures {
		if tf.SourcePosition == 0 {
			luciAnalysisRequests = append(luciAnalysisRequests, tf)
		}
	}

	// Query Changepoint analysis.
	changePointResults := map[string]*analysispb.TestVariantBranch{}
	if len(luciAnalysisRequests) > 0 {
		logging.Infof(ctx, "Start querying changepoint analysis for %d test failures", len(luciAnalysisRequests))
		client, err := analysis.NewTestVariantBranchesClient(ctx, server.LUCIAnalysisHost, req.Project)
		if err != nil {
			return nil, errors.Fmt("create LUCI Analysis client: %w", err)
		}

		tvbRequest := &analysispb.BatchGetTestVariantBranchRequest{}
		for _, tf := range luciAnalysisRequests {
			tvbRequest.Names = append(tvbRequest.Names, fmt.Sprintf("projects/%s/tests/%s/variants/%s/refs/%s", req.Project, url.PathEscape(tf.TestId), tf.VariantHash, tf.RefHash))
		}
		cpr, err := client.BatchGet(ctx, tvbRequest)
		if err != nil {
			code := status.Code(err)
			if code == codes.PermissionDenied {
				logging.Fields{
					"Project": req.Project,
				}.Warningf(ctx, "User requested test analyses for project %q, but obtained permission denied from LUCI Analysis while trying to read test variant branches (project may not exist).", req.Project)

				// Project does not exist (or this LUCI Bisection deployment
				// does not have access). Return an empty set of results.
				return &pb.BatchGetTestAnalysesResponse{
					TestAnalyses: make([]*pb.TestAnalysis, len(req.TestFailures)),
				}, nil
			} else {
				return nil, status.Errorf(codes.Internal, "read changepoint analysis: %s", err)
			}
		}
		logging.Infof(ctx, "Changepoint analysis returns %d results", len(cpr.TestVariantBranches))
		for i, tf := range luciAnalysisRequests {
			key := fmt.Sprintf("%s-%s-%s", tf.TestId, tf.VariantHash, tf.RefHash)
			changePointResults[key] = cpr.TestVariantBranches[i]
		}
	}

	result := make([]*pb.TestAnalysis, len(req.TestFailures))
	err = parallel.FanOutIn(func(workC chan<- func() error) {
		for i, tf := range req.TestFailures {
			workC <- func() error {
				var cpr *analysispb.TestVariantBranch
				if tf.SourcePosition == 0 {
					key := fmt.Sprintf("%s-%s-%s", tf.TestId, tf.VariantHash, tf.RefHash)
					cpr = changePointResults[key]
				}
				tfaProto, err := retrieveTestAnalysis(ctx, req.Project, tf, cpr, tfamask)
				if err != nil {
					return errors.Fmt("retrieve test analysis: %w", err)
				}
				result[i] = tfaProto
				return nil
			}
		}
	})

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.BatchGetTestAnalysesResponse{
		TestAnalyses: result,
	}, nil
}

func retrieveTestAnalysis(ctx context.Context, project string, tf *pb.BatchGetTestAnalysesRequest_TestFailureIdentifier, changepointResult *analysispb.TestVariantBranch, tfamask *mask.Mask) (analysis *pb.TestAnalysis, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/bisection/server/analyses.retrieveTestAnalysis", attribute.String("test_id", tf.TestId), attribute.String("ref_hash", tf.RefHash), attribute.String("variant_hash", tf.VariantHash))
	defer func() { tracing.End(ts, err) }()
	logging.Infof(ctx, "Start getting test failures for test_id = %q refHash = %q variantHash = %q", tf.TestId, tf.RefHash, tf.VariantHash)
	tfs, err := datastoreutil.GetTestFailures(ctx, project, tf.TestId, tf.RefHash, tf.VariantHash)
	if err != nil {
		return nil, errors.Fmt("get test failures: %w", err)
	}
	if len(tfs) == 0 {
		return nil, nil
	}

	var testFailure *model.TestFailure
	if tf.SourcePosition > 0 {
		// If source position is specified, we find the test failure that has the source position
		// within its regression range.
		for _, t := range tfs {
			if !t.IsDiverged && tf.SourcePosition >= t.RegressionStartPosition && tf.SourcePosition <= t.RegressionEndPosition {
				testFailure = t
				break
			}
		}
	} else {
		// If source position is not specified, we find the latest test failure.
		sort.Slice(tfs, func(i, j int) bool {
			return tfs[i].RegressionStartPosition > tfs[j].RegressionStartPosition
		})
		testFailure = tfs[0]
		if testFailure.IsDiverged {
			// Do not return test analysis if diverged.
			// Because diverged test failure is considered excluded from the test analyses.
			return nil, nil
		}
		ongoing, reason := isTestFailureDeterministicallyOngoing(testFailure, changepointResult)
		// Do not return the test analysis if the failure is not ongoing.
		if !ongoing {
			logging.Infof(ctx, "no bisection returned for test %s %s %s because %s", tf.TestId, tf.VariantHash, tf.RefHash, reason)
			return nil, nil
		}
	}

	if testFailure == nil {
		return nil, nil
	}

	// Return the test analysis that analyze this test failure.
	tfa, err := datastoreutil.GetTestFailureAnalysis(ctx, testFailure.AnalysisKey.IntID())
	if err != nil {
		return nil, errors.Fmt("get test failure analysis: %w", err)
	}
	tfaProto, err := protoutil.TestFailureAnalysisToPb(ctx, tfa, tfamask)
	if err != nil {
		return nil, errors.Fmt("convert test failure analysis to protobuf: %w", err)
	}
	logging.Infof(ctx, "Finished getting test failures for test_id = %q refHash = %q variantHash = %q", tf.TestId, tf.RefHash, tf.VariantHash)
	return tfaProto, nil
}

// IsTestFailureDeterministicallyOngoing returns a boolean which indicate whether
// a test failure is still deterministically failing.
// It also returns a string to explain why it is not deterministically failing.
func isTestFailureDeterministicallyOngoing(tf *model.TestFailure, changepointResult *analysispb.TestVariantBranch) (bool, string) {
	if changepointResult == nil {
		return false, "changepoint is not found"
	}
	segments := changepointResult.Segments
	if len(segments) < 2 {
		return false, "not deterministically failing"
	}
	curSegment := segments[0]
	prevSegment := segments[1]
	// The latest failure is not deterministically failing, return false.
	if curSegment.Counts.TotalResults != curSegment.Counts.UnexpectedResults {
		return false, "not deterministically failing"
	}
	// If the test failure is still ongoing, the regression range of the failure
	// on record should equal or contain the current regression range of
	// the latest segment in changepoint analysis.
	// Because the regression range of deterministic failure obtained from changepoint analysis
	// only shrinks or stays the same over time.
	if (curSegment.StartPosition <= tf.RegressionEndPosition) &&
		(prevSegment.EndPosition >= tf.RegressionStartPosition) {
		return true, ""
	}
	return false, "latest bisected failure is not ongoing"
}

// GetAnalysisResult returns an analysis for pRPC from CompileFailureAnalysis
func GetAnalysisResult(c context.Context, analysis *model.CompileFailureAnalysis) (*pb.Analysis, error) {
	result := &pb.Analysis{
		AnalysisId:      analysis.Id,
		Status:          analysis.Status,
		RunStatus:       analysis.RunStatus,
		CreatedTime:     timestamppb.New(analysis.CreateTime),
		FirstFailedBbid: analysis.FirstFailedBuildId,
		LastPassedBbid:  analysis.LastPassedBuildId,
	}

	if analysis.HasEnded() {
		result.EndTime = timestamppb.New(analysis.EndTime)
	}

	// Populate Builder and BuildFailureType data
	if analysis.CompileFailure != nil && analysis.CompileFailure.Parent() != nil {
		// Add details from associated compile failure
		failedBuild, err := datastoreutil.GetBuild(c, analysis.CompileFailure.Parent().IntID())
		if err != nil {
			return nil, err
		}
		if failedBuild != nil {
			result.Builder = &buildbucketpb.BuilderID{
				Project: failedBuild.Project,
				Bucket:  failedBuild.Bucket,
				Builder: failedBuild.Builder,
			}
			result.BuildFailureType = failedBuild.BuildFailureType
		}
	}

	heuristicAnalysis, err := datastoreutil.GetHeuristicAnalysis(c, analysis)
	if err != nil {
		return nil, err
	}
	if heuristicAnalysis != nil {
		suspects, err := datastoreutil.GetSuspectsForHeuristicAnalysis(c, heuristicAnalysis)
		if err != nil {
			return nil, err
		}

		pbSuspects := make([]*pb.HeuristicSuspect, len(suspects))
		for i, suspect := range suspects {
			pbSuspects[i] = &pb.HeuristicSuspect{
				GitilesCommit:   &suspect.GitilesCommit,
				ReviewUrl:       suspect.ReviewUrl,
				Score:           int32(suspect.Score),
				Justification:   suspect.Justification,
				ConfidenceLevel: heuristic.GetConfidenceLevel(suspect.Score),
			}

			verificationDetails, err := constructSuspectVerificationDetails(c, suspect)
			if err != nil {
				return nil, errors.Fmt("couldn't constructSuspectVerificationDetails: %w", err)
			}
			pbSuspects[i].VerificationDetails = verificationDetails

			// TODO: check access permissions before including the review title.
			//       For now, we will include it by default as LUCI Bisection access
			//       should already be restricted to internal users only.
			pbSuspects[i].ReviewTitle = suspect.ReviewTitle
		}
		heuristicResult := &pb.HeuristicAnalysisResult{
			Status:    heuristicAnalysis.Status,
			StartTime: timestamppb.New(heuristicAnalysis.StartTime),
			Suspects:  pbSuspects,
		}
		if heuristicAnalysis.HasEnded() {
			heuristicResult.EndTime = timestamppb.New(heuristicAnalysis.EndTime)
		}

		result.HeuristicResult = heuristicResult
	}

	// Get culprits
	culprits := make([]*pb.Culprit, len(analysis.VerifiedCulprits))
	for i, culprit := range analysis.VerifiedCulprits {
		suspect := &model.Suspect{
			Id:             culprit.IntID(),
			ParentAnalysis: culprit.Parent(),
		}
		err = datastore.Get(c, suspect)
		if err != nil {
			return nil, err
		}

		pbCulprit := &pb.Culprit{
			Commit:      &suspect.GitilesCommit,
			ReviewUrl:   suspect.ReviewUrl,
			ReviewTitle: suspect.ReviewTitle,
		}

		// Add suspect verification details for the culprit
		verificationDetails, err := constructSuspectVerificationDetails(c, suspect)
		if err != nil {
			return nil, err
		}
		pbCulprit.VerificationDetails = verificationDetails
		pbCulprit.CulpritAction = protoutil.CulpritActionsForSuspect(suspect)
		culprits[i] = pbCulprit
	}
	result.Culprits = culprits

	nthSectionResult, err := getNthSectionResult(c, analysis)
	if err != nil {
		// If fetching nthSection analysis result failed for some reasons, print
		// out the error, but we still continue.
		err = errors.Fmt("getNthSectionResult for analysis %d: %w", analysis.Id, err)
		logging.Errorf(c, err.Error())
	} else {
		result.NthSectionResult = nthSectionResult
	}

	// TODO (nqmtuan): add culprit actions for:
	//     * commenting on related bugs

	return result, nil
}

func getNthSectionResult(c context.Context, cfa *model.CompileFailureAnalysis) (*pb.NthSectionAnalysisResult, error) {
	nsa, err := datastoreutil.GetNthSectionAnalysis(c, cfa)
	if err != nil {
		return nil, errors.Fmt("getting nthsection analysis: %w", err)
	}
	if nsa == nil {
		return nil, nil
	}
	if nsa.BlameList == nil {
		return nil, errors.New("couldn't find blamelist")
	}
	result := &pb.NthSectionAnalysisResult{
		Status:    nsa.Status,
		StartTime: timestamppb.New(nsa.StartTime),
		BlameList: nsa.BlameList,
	}
	if nsa.HasEnded() {
		result.EndTime = timestamppb.New(nsa.EndTime)
	}

	// Get all reruns for the current analysis
	// This should contain all reruns for nth section and culprit verification
	reruns, err := datastoreutil.GetRerunsForAnalysis(c, cfa)
	if err != nil {
		return nil, err
	}

	for _, rerun := range reruns {
		rerunResult := &pb.SingleRerun{
			StartTime: timestamppb.New(rerun.StartTime),
			RerunResult: &pb.RerunResult{
				RerunStatus: rerun.Status,
			},
			Bbid:   rerun.RerunBuild.IntID(),
			Commit: &rerun.GitilesCommit,
			Type:   string(rerun.Type),
		}
		if rerun.HasEnded() {
			rerunResult.EndTime = timestamppb.New(rerun.EndTime)
		}
		index, err := findRerunIndexInBlameList(rerun, nsa.BlameList)
		if err != nil {
			// There is only one case where we cannot find the rerun in blamelist
			// It is when the rerun is part of the culprit verification and is
			// the "last pass" revision.
			// In this case, we should just log and continue, and the run will appear
			// as part of culprit verification component.
			logging.Warningf(c, errors.Fmt("couldn't find index for rerun: %w", err).Error())
			continue
		}
		rerunResult.Index = strconv.FormatInt(int64(index), 10)
		result.Reruns = append(result.Reruns, rerunResult)
	}

	// Find remaining regression range
	snapshot, err := nthsection.CreateSnapshot(c, nsa)
	if err != nil {
		return nil, errors.Fmt("couldn't create snapshot: %w", err)
	}

	ff, lp, err := snapshot.GetCurrentRegressionRange()
	// GetCurrentRegressionRange return error if the regression is invalid
	// We don't want to return the error here, but just continue
	if err != nil {
		err = errors.Fmt("getCurrentRegressionRange: %w", err)
		// Log as Debugf because it is not exactly an error, but just a state of the analysis
		logging.Debugf(c, err.Error())
	} else {
		result.RemainingNthSectionRange = &pb.RegressionRange{
			FirstFailed: getCommitFromIndex(ff, nsa.BlameList, cfa),
			LastPassed:  getCommitFromIndex(lp, nsa.BlameList, cfa),
		}
	}

	// Find suspect
	suspect, err := datastoreutil.GetSuspectForNthSectionAnalysis(c, nsa)
	if err != nil {
		return nil, err
	}
	if suspect != nil {
		pbSuspect := &pb.NthSectionSuspect{
			GitilesCommit: &suspect.GitilesCommit,
			ReviewUrl:     suspect.ReviewUrl,
			ReviewTitle:   suspect.ReviewTitle,
			Commit:        &suspect.GitilesCommit,
		}

		verificationDetails, err := constructSuspectVerificationDetails(c, suspect)
		if err != nil {
			return nil, errors.Fmt("couldn't constructSuspectVerificationDetails: %w", err)
		}
		pbSuspect.VerificationDetails = verificationDetails
		result.Suspect = pbSuspect
	}

	return result, nil
}

func findRerunIndexInBlameList(rerun *model.SingleRerun, blamelist *pb.BlameList) (int32, error) {
	for i, commit := range blamelist.Commits {
		if commit.Commit == rerun.GitilesCommit.Id {
			return int32(i), nil
		}
	}
	return -1, fmt.Errorf("couldn't find index for rerun %d", rerun.Id)
}

func getCommitFromIndex(index int, blamelist *pb.BlameList, cfa *model.CompileFailureAnalysis) *buildbucketpb.GitilesCommit {
	return &buildbucketpb.GitilesCommit{
		Id:      blamelist.Commits[index].Commit,
		Host:    cfa.InitialRegressionRange.FirstFailed.Host,
		Project: cfa.InitialRegressionRange.FirstFailed.Project,
		Ref:     cfa.InitialRegressionRange.FirstFailed.Ref,
	}
}

// constructSingleRerun constructs a pb.SingleRerun using the details from the
// rerun build and latest single rerun
func constructSingleRerun(c context.Context, rerunBBID int64) (*pb.SingleRerun, error) {
	rerunBuild := &model.CompileRerunBuild{
		Id: rerunBBID,
	}
	err := datastore.Get(c, rerunBuild)
	if err != nil {
		return nil, errors.Fmt("failed getting rerun build: %w", err)
	}

	singleRerun, err := datastoreutil.GetLastRerunForRerunBuild(c, rerunBuild)
	if err != nil {
		return nil, errors.Fmt("failed getting single rerun: %w", err)
	}

	result := &pb.SingleRerun{
		StartTime: timestamppb.New(singleRerun.StartTime),
		Bbid:      rerunBBID,
		RerunResult: &pb.RerunResult{
			RerunStatus: singleRerun.Status,
		},
		Commit: &singleRerun.GitilesCommit,
	}
	if singleRerun.HasEnded() {
		result.EndTime = timestamppb.New(singleRerun.EndTime)
	}
	return result, nil
}

// constructSuspectVerificationDetails constructs a pb.SuspectVerificationDetails for the given suspect
func constructSuspectVerificationDetails(c context.Context, suspect *model.Suspect) (*pb.SuspectVerificationDetails, error) {
	// Add the current verification status
	verificationDetails := &pb.SuspectVerificationDetails{
		Status: string(suspect.VerificationStatus),
	}

	// Add rerun details for the suspect commit
	if suspect.SuspectRerunBuild != nil {
		singleRerun, err := constructSingleRerun(c, suspect.SuspectRerunBuild.IntID())
		if err != nil {
			return nil, errors.Fmt("failed getting verification rerun for suspect commit: %w", err)
		}
		verificationDetails.SuspectRerun = singleRerun
	}

	// Add rerun details for the parent commit of suspect
	if suspect.ParentRerunBuild != nil {
		singleRerun, err := constructSingleRerun(c, suspect.ParentRerunBuild.IntID())
		if err != nil {
			return nil, errors.Fmt("failed getting verification rerun for parent commit of suspect: %w", err)
		}
		verificationDetails.ParentRerun = singleRerun
	}

	return verificationDetails, nil
}

// validateQueryAnalysisRequest checks if the request is valid.
func validateQueryAnalysisRequest(req *pb.QueryAnalysisRequest) error {
	if req.BuildFailure == nil {
		return status.Errorf(codes.InvalidArgument, "BuildFailure must not be empty")
	}
	if req.BuildFailure.GetBbid() == 0 {
		return status.Errorf(codes.InvalidArgument, "BuildFailure bbid must not be empty")
	}
	return nil
}

// ListAnalyses returns existing analyses
func (server *AnalysesServer) ListAnalyses(c context.Context, req *pb.ListAnalysesRequest) (*pb.ListAnalysesResponse, error) {
	// Validate the request
	if err := validateListAnalysesRequest(req); err != nil {
		return nil, err
	}

	// Decode cursor from page token
	cursor, err := listAnalysesPageTokenVault.Cursor(c, req.PageToken)
	switch err {
	case pagination.ErrInvalidPageToken:
		return nil, status.Errorf(codes.InvalidArgument, "Invalid page token")
	case nil:
		// Continue
	default:
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Override the page size if necessary
	pageSize := int(listAnalysesPageSizeLimiter.Adjust(req.PageSize))

	// Construct the query
	q := datastore.NewQuery("CompileFailureAnalysis").Order("-create_time").Start(cursor)

	// Query datastore for compile failure analyses
	compileFailureAnalyses := make([]*model.CompileFailureAnalysis, 0, pageSize)
	var nextCursor datastore.Cursor
	err = datastore.Run(c, q, func(compileFailureAnalysis *model.CompileFailureAnalysis, getCursor datastore.CursorCB) error {
		compileFailureAnalyses = append(compileFailureAnalyses, compileFailureAnalysis)

		// Check whether the page size limit has been reached
		if len(compileFailureAnalyses) == pageSize {
			nextCursor, err = getCursor()
			if err != nil {
				return err
			}
			return datastore.Stop
		}
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Construct the next page token
	nextPageToken, err := listAnalysesPageTokenVault.PageToken(c, nextCursor)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Get the result for each compile failure analysis
	analyses := make([]*pb.Analysis, len(compileFailureAnalyses))
	err = parallel.FanOutIn(func(workC chan<- func() error) {
		for i, compileFailureAnalysis := range compileFailureAnalyses {
			workC <- func() error {
				analysis, err := GetAnalysisResult(c, compileFailureAnalysis)
				if err != nil {
					logging.Errorf(c, "Could not get analysis data for analysis %d: %s",
						compileFailureAnalysis.Id, err)
					return err
				}
				analyses[i] = analysis
				return nil
			}
		}
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.ListAnalysesResponse{
		Analyses:      analyses,
		NextPageToken: nextPageToken,
	}, nil
}

// validateListAnalysesRequest checks if the request is valid.
func validateListAnalysesRequest(req *pb.ListAnalysesRequest) error {
	if req.PageSize < 0 {
		return status.Errorf(codes.InvalidArgument, "Page size can't be negative")
	}

	return nil
}

// validateListTestAnalysesRequest checks if the ListTestAnalysesRequest is valid.
func validateListTestAnalysesRequest(req *pb.ListTestAnalysesRequest) error {
	if req.Project == "" {
		return status.Errorf(codes.InvalidArgument, "project must not be empty")
	}
	if req.PageSize < 0 {
		return status.Errorf(codes.InvalidArgument, "page size must not be negative")
	}
	return nil
}

func validateBatchGetTestAnalysesRequest(req *pb.BatchGetTestAnalysesRequest) error {
	// MaxTestFailures is the maximum number of test failures to be queried in one request.
	const MaxTestFailures = 100
	if err := util.ValidateProject(req.Project); err != nil {
		return errors.Fmt("project: %w", err)
	}
	if len(req.TestFailures) == 0 {
		return errors.New("test_failures: unspecified")
	}
	if len(req.TestFailures) > MaxTestFailures {
		return errors.Fmt("test_failures: no more than %v may be queried at a time", MaxTestFailures)
	}
	for i, tf := range req.TestFailures {
		if tf.GetTestId() == "" {
			return errors.Fmt("test_variants[%v]: test_id: unspecified", i)
		}
		if tf.VariantHash == "" {
			return errors.Fmt("test_variants[%v]: variant_hash: unspecified", i)
		}
		if tf.RefHash == "" {
			return errors.Fmt("test_variants[%v]: ref_hash: unspecified", i)
		}
		if tf.SourcePosition < 0 {
			return errors.Fmt("test_variants[%v]: source_position: must not be negative", i)
		}
		if err := rdbpbutil.ValidateTestID(tf.TestId); err != nil {
			return errors.Fmt("test_variants[%v].test_id: %w", i, err)
		}
		if err := util.ValidateVariantHash(tf.VariantHash); err != nil {
			return errors.Fmt("test_variants[%v].variant_hash: %w", i, err)
		}
		if err := util.ValidateRefHash(tf.RefHash); err != nil {
			return errors.Fmt("test_variants[%v].ref_hash: %w", i, err)
		}
	}
	return nil
}

func defaultFieldMask() *fieldmaskpb.FieldMask {
	return &fieldmaskpb.FieldMask{
		Paths: []string{"*"},
	}
}
