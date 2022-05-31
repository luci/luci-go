// Copyright 2020 The LUCI Authors.
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
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/testvariants"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/span"
)

// QueryTestVariants implements pb.ResultDBServer.
func (s *resultDBServer) QueryTestVariants(ctx context.Context, in *pb.QueryTestVariantsRequest) (*pb.QueryTestVariantsResponse, error) {
	if err := permissions.VerifyInvocationsByName(ctx, in.Invocations, rdbperms.PermListTestResults, rdbperms.PermListTestExonerations); err != nil {
		return nil, err
	}

	if err := validateQueryTestVariantsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	readMask, err := testvariants.QueryMask(in.GetReadMask())
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Open a transaction.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Get the transitive closure.
	invs, err := invocations.Reachable(ctx, invocations.MustParseNames(in.Invocations))
	if err != nil {
		return nil, errors.Annotate(err, "failed to read the reach").Err()
	}

	// Query test variants.
	q := testvariants.Query{
		InvocationIDs: invs,
		Predicate:     in.Predicate,
		ResultLimit:   testvariants.AdjustResultLimit(in.ResultLimit),
		PageSize:      pagination.AdjustPageSize(in.PageSize),
		PageToken:     in.PageToken,
		Mask:          readMask,
	}

	var tvs []*pb.TestVariant
	var token string
	for len(tvs) == 0 {
		if tvs, token, err = q.Fetch(ctx); err != nil {
			return nil, errors.Annotate(err, "failed to read test variants").Err()
		}

		if token == "" || outOfTime(ctx) {
			break
		}
		q.PageToken = token
	}

	return &pb.QueryTestVariantsResponse{
		TestVariants:  tvs,
		NextPageToken: token,
	}, nil
}

// outOfTime returns true if the context will expire in less than 500ms.
func outOfTime(ctx context.Context) bool {
	dl, ok := ctx.Deadline()
	return ok && clock.Until(ctx, dl) < 500*time.Millisecond
}

// validateQueryTestVariantsRequest returns a non-nil error if req is determined
// to be invalid.
func validateQueryTestVariantsRequest(in *pb.QueryTestVariantsRequest) error {
	if err := validateQueryRequest(in); err != nil {
		return err
	}

	if err := testvariants.ValidateResultLimit(in.ResultLimit); err != nil {
		return errors.Annotate(err, "result_limit").Err()
	}

	return nil
}
