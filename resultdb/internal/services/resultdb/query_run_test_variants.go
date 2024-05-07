// Copyright 2024 The LUCI Authors.
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
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/runtestvariants"
	"go.chromium.org/luci/resultdb/internal/testvariants"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

const (
	// RunTestVariantsResponseLimitBytes is the default soft limit on the number of bytes
	// that should be returned by a run test variants query.
	RunTestVariantsResponseLimitBytes = 20 * 1000 * 1000 // 20 MB
)

func validateQueryRunTestVariantsRequest(req *pb.QueryRunTestVariantsRequest) error {
	_, err := pbutil.ParseInvocationName(req.GetInvocation())
	if err != nil {
		return errors.Annotate(err, "invocation").Err()
	}

	// Validate page size is non-negative.
	if err := pagination.ValidatePageSize(req.PageSize); err != nil {
		return errors.Annotate(err, "page_size").Err()
	}

	// Validate result limit is non-negative.
	if err := testvariants.ValidateResultLimit(req.ResultLimit); err != nil {
		return errors.Annotate(err, "result_limit").Err()
	}

	return nil
}

// QueryRunTestVariants returns test variants for a test run. A test run
// comprises only the test results directly inside an invocation,
// excluding test results from included invocations.
//
// Exonerations and sources are not returned.
//
// Designed to be used for incremental ingestion of test results
// from an export root in conjuction with the `invocation-ready-for-export`
// pub/sub.
func (s *resultDBServer) QueryRunTestVariants(ctx context.Context, req *pb.QueryRunTestVariantsRequest) (*pb.QueryRunTestVariantsResponse, error) {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := permissions.VerifyInvocationByName(ctx, req.GetInvocation(), rdbperms.PermListTestResults); err != nil {
		return nil, err
	}

	err := validateQueryRunTestVariantsRequest(req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	token, err := runtestvariants.ParsePageToken(req.PageToken)
	if err != nil {
		return nil, err
	}

	query := runtestvariants.Query{
		InvocationID:       invocations.MustParseName(req.Invocation),
		PageSize:           pagination.AdjustPageSize(req.PageSize),
		PageToken:          token,
		ResultLimit:        testvariants.AdjustResultLimit(req.ResultLimit),
		ResponseLimitBytes: RunTestVariantsResponseLimitBytes,
	}
	result, err := query.Run(ctx)
	if err != nil {
		return nil, err
	}

	return &pb.QueryRunTestVariantsResponse{
		TestVariants:  result.TestVariants,
		NextPageToken: result.NextPageToken.Serialize(),
	}, nil
}
