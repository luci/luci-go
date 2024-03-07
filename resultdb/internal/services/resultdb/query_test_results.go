// Copyright 2019 The LUCI Authors.
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

package resultdb

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/span"
)

// queryRequest is implemented by *pb.QueryTestResultsRequest,
// *pb.QueryTestExonerationsRequest and *pb.QueryTestResultStatisticsRequest.
type queryRequest interface {
	GetInvocations() []string
}

// queryRequestWithPaging is implemented by *pb.QueryTestResultsRequest and
// *pb.QueryTestExonerationsRequest.
type queryRequestWithPaging interface {
	queryRequest
	GetPageSize() int32
}

// validateQueryRequest returns a non-nil error if req is determined to be
// invalid.
func validateQueryRequest(req queryRequest) error {
	if len(req.GetInvocations()) == 0 {
		return errors.Reason("invocations: unspecified").Err()
	}
	for _, name := range req.GetInvocations() {
		if err := pbutil.ValidateInvocationName(name); err != nil {
			return errors.Annotate(err, "invocations: %q", name).Err()
		}
	}

	if paging, ok := req.(queryRequestWithPaging); ok {
		// validate paging
		if err := pagination.ValidatePageSize(paging.GetPageSize()); err != nil {
			return errors.Annotate(err, "page_size").Err()
		}
	}
	return nil
}

// validateQueryTestResultsRequest returns a non-nil error if req is determined
// to be invalid.
func validateQueryTestResultsRequest(req *pb.QueryTestResultsRequest) error {
	if err := pbutil.ValidateTestResultPredicate(req.Predicate); err != nil {
		return errors.Annotate(err, "predicate").Err()
	}

	return validateQueryRequest(req)
}

// QueryTestResults implements pb.ResultDBServer.
func (s *resultDBServer) QueryTestResults(ctx context.Context, in *pb.QueryTestResultsRequest) (*pb.QueryTestResultsResponse, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := permissions.VerifyInvocationsByName(ctx, in.Invocations, rdbperms.PermListTestResults); err != nil {
		return nil, err
	}

	if err := validateQueryTestResultsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	readMask, err := testresults.ListMask(in.GetReadMask())
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Query is valid - increment the queryInvocationsCount metric
	queryInvocationsCount.Add(ctx, 1, "QueryTestResults", len(in.Invocations))

	// Get the transitive closure.
	invs, err := graph.Reachable(ctx, invocations.MustParseNames(in.Invocations))
	if err != nil {
		return nil, errors.Annotate(err, "failed to read the reach").Err()
	}
	invocationIDs, err := invs.IDSet()
	if err != nil {
		return nil, err
	}

	// Query test results.
	q := testresults.Query{
		Predicate:     in.Predicate,
		PageSize:      pagination.AdjustPageSize(in.PageSize),
		PageToken:     in.PageToken,
		InvocationIDs: invocationIDs,
		Mask:          readMask,
	}
	trs, token, err := q.Fetch(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read test results").Err()
	}

	return &pb.QueryTestResultsResponse{
		NextPageToken: token,
		TestResults:   trs,
	}, nil
}
