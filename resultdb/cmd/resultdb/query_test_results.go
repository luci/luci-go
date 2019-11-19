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

package main

import (
	"context"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const maxInvocationGraphSize = 1000

// validateQueryTestResultsRequest returns a non-nil error if req is determined
// to be invalid.
func validateQueryTestResultsRequest(req *pb.QueryTestResultsRequest) error {
	if err := pbutil.ValidateTestResultPredicate(req.Predicate, true); err != nil {
		return errors.Annotate(err, "predicate").Err()
	}

	if err := pagination.ValidatePageSize(req.PageSize); err != nil {
		return errors.Annotate(err, "page_size").Err()
	}

	if req.MaxStaleness != nil {
		if err := pbutil.ValidateMaxStaleness(req.MaxStaleness); err != nil {
			return errors.Annotate(err, "max_staleness").Err()
		}
	}

	return nil
}

// QueryTestResults implements pb.ResultDBServer.
func (s *resultDBServer) QueryTestResults(ctx context.Context, in *pb.QueryTestResultsRequest) (*pb.QueryTestResultsResponse, error) {
	if err := validateQueryTestResultsRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	// Open a transaction.
	txn := span.Client(ctx).ReadOnlyTransaction()
	defer txn.Close()
	if in.MaxStaleness != nil {
		st, _ := ptypes.Duration(in.MaxStaleness)
		txn.WithTimestampBound(spanner.MaxStaleness(st))
	}

	// Query invocations.
	invs, err := span.QueryInvocations(ctx, txn, in.Predicate.Invocation, maxInvocationGraphSize)
	if err != nil {
		return nil, err
	}
	invIDs := make([]span.InvocationID, 0, len(invs))
	for id := range invs {
		invIDs = append(invIDs, id)
	}

	// Query test results.
	in.Predicate.Invocation = nil
	trs, token, err := span.QueryTestResults(ctx, txn, span.TestResultQuery{
		Predicate:     in.Predicate,
		PageSize:      pagination.AdjustPageSize(in.PageSize),
		CursorToken:   in.PageToken,
		InvocationIDs: invIDs,
	})
	if err != nil {
		return nil, err
	}

	return &pb.QueryTestResultsResponse{
		NextPageToken: token,
		TestResults:   trs,
	}, nil
}
