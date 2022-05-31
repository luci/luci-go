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
	"go.chromium.org/luci/resultdb/internal/exonerations"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/span"
)

// validateQueryTestExonerationsRequest returns a non-nil error if req is determined
// to be invalid.
func validateQueryTestExonerationsRequest(req *pb.QueryTestExonerationsRequest) error {
	if err := pbutil.ValidateTestExonerationPredicate(req.Predicate); err != nil {
		return errors.Annotate(err, "predicate").Err()
	}

	return validateQueryRequest(req)
}

// QueryTestExonerations implements pb.ResultDBServer.
func (s *resultDBServer) QueryTestExonerations(ctx context.Context, in *pb.QueryTestExonerationsRequest) (*pb.QueryTestExonerationsResponse, error) {
	if err := permissions.VerifyInvocationsByName(ctx, in.Invocations, rdbperms.PermListTestExonerations); err != nil {
		return nil, err
	}

	if err := validateQueryTestExonerationsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Open a transaction.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Get the transitive closure.
	invs, err := invocations.Reachable(ctx, invocations.MustParseNames(in.Invocations))
	if err != nil {
		return nil, err
	}

	// Query test exonerations.
	q := exonerations.Query{
		Predicate:     in.Predicate,
		PageSize:      pagination.AdjustPageSize(in.PageSize),
		PageToken:     in.PageToken,
		InvocationIDs: invs,
	}
	tes, token, err := q.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.QueryTestExonerationsResponse{
		TestExonerations: tes,
		NextPageToken:    token,
	}, nil
}
