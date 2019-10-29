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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const (
	pageSizeMax     = 1000
	pageSizeDefault = 100
)

func validateListTestResultsRequest(req *pb.ListTestResultsRequest) error {
	if req.GetInvocation() == "" {
		return errors.Reason("invocation missing").Err()
	}

	if err := pbutil.ValidateInvocationName(req.Invocation); err != nil {
		return errors.Annotate(err, "invocation").Err()
	}

	if req.GetPageSize() < 0 {
		return errors.Reason("negative page size").Err()
	}

	return nil
}

func (s *resultDBServer) ListTestResults(ctx context.Context, in *pb.ListTestResultsRequest) (*pb.ListTestResultsResponse, error) {
	if err := validateListTestResultsRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	// Get cursor.
	cursor, err := internal.Cursor(in.PageToken)
	if err != nil {
		return nil, errors.Annotate(err, "bad page_token").
			Tag(grpcutil.InvalidArgumentTag).Err()
	}

	// Query for test results.
	txn, err := span.Client(ctx).BatchReadOnlyTransaction(ctx, spanner.StrongRead())
	if err != nil {
		return nil, err
	}
	defer txn.Close()

	trs, cursor, err := span.ReadTestResults(
		ctx, txn, in.Invocation, false, cursor, adjustPageSize(in.PageSize))
	if err != nil {
		return nil, err
	}

	// Get token for next page.
	token, err := internal.Token(cursor)
	if err != nil {
		return nil, errors.Annotate(err, "next_page_token").Err()
	}

	return &pb.ListTestResultsResponse{TestResults: trs, NextPageToken: token}, nil
}

// adjustPageSize takes the given requested pageSize and adjusts as necessary for querying Spanner.
func adjustPageSize(pageSize int32) int {
	switch {
	case pageSize > 0 && pageSize < pageSizeMax:
		return int(pageSize)
	case pageSize > 0:
		return pageSizeMax
	default:
		return pageSizeDefault
	}
}
