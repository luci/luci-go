// Copyright 2022 The LUCI Authors.
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

// package server implements the server to handle pRPC requests.
package server

import (
	"context"

	"go.chromium.org/luci/bisection/compilefailureanalysis/heuristic"
	gfim "go.chromium.org/luci/bisection/model"
	gfipb "go.chromium.org/luci/bisection/proto"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/pagination"
	"go.chromium.org/luci/common/pagination/dscursor"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var listAnalysesPageTokenVault = dscursor.NewVault([]byte("LUCIBisection.v1.ListAnalyses"))

// Max and default page sizes for ListAnalyses - the proto should be updated
// to reflect any changes to these values
var listAnalysesPageSizeLimiter = PageSizeLimiter{
	Max:     200,
	Default: 50,
}

// GoFinditServer implements the proto service GoFinditService.
type GoFinditServer struct{}

// GetAnalysis returns the analysis given the analysis id
func (server *GoFinditServer) GetAnalysis(c context.Context, req *gfipb.GetAnalysisRequest) (*gfipb.Analysis, error) {
	analysis := &gfim.CompileFailureAnalysis{
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
func (server *GoFinditServer) QueryAnalysis(c context.Context, req *gfipb.QueryAnalysisRequest) (*gfipb.QueryAnalysisResponse, error) {
	if err := validateQueryAnalysisRequest(req); err != nil {
		return nil, err
	}
	if req.BuildFailure.FailedStepName != "compile" {
		return nil, status.Errorf(codes.Unimplemented, "only compile failures are supported")
	}
	bbid := req.BuildFailure.GetBbid()
	logging.Infof(c, "QueryAnalysis for build %d", bbid)

	analysis, err := GetAnalysisForBuild(c, bbid)
	if err != nil {
		logging.Errorf(c, "Could not query analysis for build %d: %s", bbid, err)
		return nil, status.Errorf(codes.Internal, "failed to get analysis for build %d: %s", bbid, err)
	}
	if analysis == nil {
		logging.Infof(c, "No analysis for build %d", bbid)
		return nil, status.Errorf(codes.NotFound, "analysis not found for build %d", bbid)
	}
	analysispb, err := GetAnalysisResult(c, analysis)
	if err != nil {
		logging.Errorf(c, "Could not get analysis data for build %d: %s", bbid, err)
		return nil, status.Errorf(codes.Internal, "failed to get analysis data %s", err)
	}

	res := &gfipb.QueryAnalysisResponse{
		Analyses: []*gfipb.Analysis{analysispb},
	}
	return res, nil
}

// TriggerAnalysis triggers an analysis for a failure
func (server *GoFinditServer) TriggerAnalysis(c context.Context, req *gfipb.TriggerAnalysisRequest) (*gfipb.TriggerAnalysisResponse, error) {
	// TODO(nqmtuan): Implement this
	return nil, nil
}

// UpdateAnalysis updates the information of an analysis.
// At the mean time, it is only used for update the bugs associated with an
// analysis.
func (server *GoFinditServer) UpdateAnalysis(c context.Context, req *gfipb.UpdateAnalysisRequest) (*gfipb.Analysis, error) {
	// TODO(nqmtuan): Implement this
	return nil, nil
}

// GetAnalysisResult returns an analysis for pRPC from CompileFailureAnalysis
func GetAnalysisResult(c context.Context, analysis *gfim.CompileFailureAnalysis) (*gfipb.Analysis, error) {
	result := &gfipb.Analysis{
		AnalysisId:      analysis.Id,
		Status:          analysis.Status,
		CreatedTime:     timestamppb.New(analysis.CreateTime),
		EndTime:         timestamppb.New(analysis.EndTime),
		FirstFailedBbid: analysis.FirstFailedBuildId,
		LastPassedBbid:  analysis.LastPassedBuildId,
	}

	// Populate Builder and BuildFailureType data
	if analysis.CompileFailure != nil && analysis.CompileFailure.Parent() != nil {
		// Add details from associated compile failure
		failedBuild, err := GetBuild(c, analysis.CompileFailure.Parent().IntID())
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

	heuristicAnalysis, err := GetHeuristicAnalysis(c, analysis)
	if err != nil {
		return nil, err
	}
	if heuristicAnalysis == nil {
		// No heuristic analysis associated with the compile failure analysis
		return result, nil
	}

	suspects, err := GetSuspects(c, heuristicAnalysis)
	if err != nil {
		return nil, err
	}

	pbSuspects := make([]*gfipb.HeuristicSuspect, len(suspects))
	for i, suspect := range suspects {
		pbSuspects[i] = &gfipb.HeuristicSuspect{
			GitilesCommit:   &suspect.GitilesCommit,
			ReviewUrl:       suspect.ReviewUrl,
			Score:           int32(suspect.Score),
			Justification:   suspect.Justification,
			ConfidenceLevel: heuristic.GetConfidenceLevel(suspect.Score),
		}

		// TODO: check access permissions before including the review title.
		//       For now, we will include it by default as LUCI Bisection access
		//       should already be restricted to internal users only.
		pbSuspects[i].ReviewTitle = suspect.ReviewTitle
	}
	heuristicResult := &gfipb.HeuristicAnalysisResult{
		Status:    heuristicAnalysis.Status,
		StartTime: timestamppb.New(heuristicAnalysis.StartTime),
		EndTime:   timestamppb.New(heuristicAnalysis.EndTime),
		Suspects:  pbSuspects,
	}

	result.HeuristicResult = heuristicResult

	// Get culprits
	culprits := make([]*gfipb.Culprit, len(analysis.VerifiedCulprits))
	for i, culprit := range analysis.VerifiedCulprits {
		suspect := &gfim.Suspect{
			Id:             culprit.IntID(),
			ParentAnalysis: culprit.Parent(),
		}
		err = datastore.Get(c, suspect)
		if err != nil {
			return nil, err
		}
		culprits[i] = &gfipb.Culprit{
			Commit:      &suspect.GitilesCommit,
			ReviewUrl:   suspect.ReviewUrl,
			ReviewTitle: suspect.ReviewTitle,
		}
	}
	result.Culprits = culprits

	// TODO (nqmtuan): query for nth-section result

	// TODO (aredulla): get culprit actions, such as the revert CL for the culprit
	//                  and any related bugs

	return result, nil
}

// validateQueryAnalysisRequest checks if the request is valid.
func validateQueryAnalysisRequest(req *gfipb.QueryAnalysisRequest) error {
	if req.BuildFailure == nil {
		return status.Errorf(codes.InvalidArgument, "BuildFailure must not be empty")
	}
	if req.BuildFailure.GetBbid() == 0 {
		return status.Errorf(codes.InvalidArgument, "BuildFailure bbid must not be empty")
	}
	return nil
}

// ListAnalyses returns existing analyses
func (server *GoFinditServer) ListAnalyses(c context.Context, req *gfipb.ListAnalysesRequest) (*gfipb.ListAnalysesResponse, error) {
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
		return nil, err
	}

	// Override the page size if necessary
	pageSize := int(listAnalysesPageSizeLimiter.Adjust(req.PageSize))

	// Construct the query
	q := datastore.NewQuery("CompileFailureAnalysis").Order("-create_time").Start(cursor)

	// Query datastore for compile failure analyses
	compileFailureAnalyses := make([]*gfim.CompileFailureAnalysis, 0, pageSize)
	var nextCursor datastore.Cursor
	err = datastore.Run(c, q, func(compileFailureAnalysis *gfim.CompileFailureAnalysis, getCursor datastore.CursorCB) error {
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
		return nil, err
	}

	// Construct the next page token
	nextPageToken, err := listAnalysesPageTokenVault.PageToken(c, nextCursor)
	if err != nil {
		return nil, err
	}

	// Get the result for each compile failure analysis
	analyses := make([]*gfipb.Analysis, len(compileFailureAnalyses))
	err = parallel.FanOutIn(func(workC chan<- func() error) {
		for i, compileFailureAnalysis := range compileFailureAnalyses {
			i := i
			compileFailureAnalysis := compileFailureAnalysis
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
		return nil, err
	}

	return &gfipb.ListAnalysesResponse{
		Analyses:      analyses,
		NextPageToken: nextPageToken,
	}, nil
}

func validateListAnalysesRequest(req *gfipb.ListAnalysesRequest) error {
	if req.PageSize < 0 {
		return status.Errorf(codes.InvalidArgument, "Page size can't be negative")
	}

	return nil
}
