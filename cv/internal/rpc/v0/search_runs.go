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

package rpc

import (
	"context"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/common/sync/parallel"
	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/acls"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/rpc/pagination"
	"go.chromium.org/luci/cv/internal/run"
)

// Default and max numbers of result paging.
// If you update either of these values, please update the service proto.

const defaultPageSize = 32
const maxPageSize = 128

// SearchRuns implements RunsServer; it fetches multiple Runs given search criteria.
func (s *RunsServer) SearchRuns(ctx context.Context, req *apiv0pb.SearchRunsRequest) (resp *apiv0pb.SearchRunsResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	if err = checkCanUseAPI(ctx, "SearchRuns"); err != nil {
		return
	}
	limit, err := pagination.ValidatePageSize(req, defaultPageSize, maxPageSize)
	if err != nil {
		return nil, err
	}
	var pt *run.PageToken
	if s := req.GetPageToken(); s != "" {
		pt = &run.PageToken{}
		if err := pagination.DecryptPageToken(ctx, s, pt); err != nil {
			return nil, err
		}
	}

	if req.GetPredicate() == nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "Predicate is required")
	}
	pred := req.GetPredicate()
	if pred.GetProject() == "" {
		return nil, appstatus.Errorf(codes.InvalidArgument, "Project is required")
	}

	// TODO(qyearsley): Implement query by Gerrit changes.
	// This will involve getting RunCLs from external IDs (Lookup) and either
	// adding a new function to run/query.go or looking up by one CL then
	// filtering.
	if len(pred.GetGerritChanges()) != 0 {
		return nil, appstatus.Errorf(codes.InvalidArgument, "Query by GerritChanges not yet implemented")
	}

	qb := run.ProjectQueryBuilder{
		Project: req.Predicate.GetProject(),
		Limit:   limit,
	}.PageToken(pt)

	runs, nextPageToken, err := qb.LoadRuns(ctx, acls.NewRunReadChecker())
	if err != nil {
		return nil, err
	}

	// Convert run.Runs to apiv0pb.Runs.
	respRuns, err := populateRuns(ctx, runs)
	if err != nil {
		return nil, err
	}

	encryptedNextPageToken, err := pagination.EncryptPageToken(ctx, nextPageToken)
	if err != nil {
		return nil, err
	}
	return &apiv0pb.SearchRunsResponse{
		Runs:          respRuns,
		NextPageToken: encryptedNextPageToken,
	}, nil
}

// populateRuns converts run.Runs to apiv0pb.Runs for the response.
//
// This includes fetching and populating extra information, including RunCLs
// and Tryjobs, to fill in details in each apiv0pb.Run.
//
// TODO(qyearsley): Consider optimizing this by using datastore.Get batch
// calls, e.g. for fetching all Tryjob entities with one call.
func populateRuns(ctx context.Context, runs []*run.Run) ([]*apiv0pb.Run, error) {
	respRuns := make([]*apiv0pb.Run, len(runs))
	errs := parallel.WorkPool(min(len(runs), 16), func(work chan<- func() error) {
		for i, r := range runs {
			i, r := i, r
			work <- func() (err error) {
				respRuns[i], err = populateRunResponse(ctx, r)
				return
			}
		}
	})
	return respRuns, common.MostSevereError(errs)
}
