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

func validateGetTestResultRequest(req *pb.GetTestResultRequest) error {
	if req.GetName() == "" {
		return errors.Reason("name missing").Err()
	}

	if err := pbutil.ValidateTestResultName(req.Name); err != nil {
		return errors.Annotate(err, "req.Name").Err()
	}

	return nil
}

func (s *resultDBServer) GetTestResult(ctx context.Context, in *pb.GetTestResultRequest) (*pb.TestResult, error) {
	if err := validateGetTestResultRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	txn, err := span.Client(ctx).BatchReadOnlyTransaction(ctx, spanner.StrongRead())
	if err != nil {
		return nil, err
	}
	defer txn.Close()

	invID, testPath, resultID := pbutil.MustParseTestResultName(in.Name)
	return span.ReadTestResult(ctx, txn, invID, testPath, resultID)
}
