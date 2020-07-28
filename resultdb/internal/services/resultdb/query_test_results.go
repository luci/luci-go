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

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"
	durpb "github.com/golang/protobuf/ptypes/duration"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// queryRequest is implemented by *pb.QueryTestResultsRequest,
// *pb.QueryTestExonerationsRequest and *pb.QueryTestResultStatisticsRequest.
type queryRequest interface {
	GetInvocations() []string
	GetMaxStaleness() *durpb.Duration
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

	if req.GetMaxStaleness() != nil {
		if err := pbutil.ValidateMaxStaleness(req.GetMaxStaleness()); err != nil {
			return errors.Annotate(err, "max_staleness").Err()
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
	if err := verifyPermissionInvNames(ctx, permListTestResults, in.Invocations...); err != nil {
		return nil, err
	}

	if err := validateQueryTestResultsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Open a transaction.
	txn := spanutil.Client(ctx).ReadOnlyTransaction()
	defer txn.Close()
	if in.MaxStaleness != nil {
		st, _ := ptypes.Duration(in.MaxStaleness)
		txn.WithTimestampBound(spanner.MaxStaleness(st))
	}

	// Get the transitive closure.
	invs, err := invocations.Reachable(ctx, txn, invocations.MustParseNames(in.Invocations))
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	readMask, err := testresults.ListMask(in.GetReadMask())
	if err != nil {
		return nil, err
	}

	// Query test results.
	q := testresults.Query{
		Predicate:     in.Predicate,
		PageSize:      pagination.AdjustPageSize(in.PageSize),
		PageToken:     in.PageToken,
		InvocationIDs: invs,
		Mask:          readMask,
	}
	trs, token, err := q.Fetch(ctx, txn)
	if err != nil {
		return nil, err
	}

	return &pb.QueryTestResultsResponse{
		NextPageToken: token,
		TestResults:   trs,
	}, nil
}
