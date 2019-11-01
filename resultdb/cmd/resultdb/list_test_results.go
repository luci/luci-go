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

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const (
	pageSizeMax     = 1000
	pageSizeDefault = 100
)

func validateListTestResultsRequest(req *pb.ListTestResultsRequest) error {
	if err := pbutil.ValidateInvocationName(req.GetInvocation()); err != nil {
		return errors.Annotate(err, "invocation").Err()
	}

	if err := pbutil.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Annotate(err, "page_size").Err()
	}

	return nil
}

// ListTestResults implements pb.ResultDBServer.
func (s *resultDBServer) ListTestResults(ctx context.Context, in *pb.ListTestResultsRequest) (*pb.ListTestResultsResponse, error) {
	if err := validateListTestResultsRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	txn, err := span.Client(ctx).BatchReadOnlyTransaction(ctx, spanner.StrongRead())
	if err != nil {
		return nil, err
	}
	defer txn.Close()

	trs, tok, err := span.ReadTestResults(
		ctx, txn, in.Invocation, true, in.GetPageToken(), adjustPageSize(in.GetPageSize()))
	if err != nil {
		return nil, err
	}

	return &pb.ListTestResultsResponse{
		TestResults:   trs,
		NextPageToken: tok,
	}, nil
}

// adjustPageSize takes the given requested pageSize and adjusts as necessary for querying Spanner.
func adjustPageSize(pageSize int32) int {
	switch {
	case pageSize >= pageSizeMax:
		return pageSizeMax
	case pageSize > 0:
		return int(pageSize)
	default:
		return pageSizeDefault
	}
}
