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
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/exonerations"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

// validateQueryTestExonerationsRequest returns a non-nil error if req is determined
// to be invalid.
func validateQueryTestExonerationsRequest(req *pb.QueryTestExonerationsRequest) error {
	if len(req.Invocations) > 1 {
		return errors.New("invocations: only one invocation is allowed")
	}
	if err := pbutil.ValidateTestExonerationPredicate(req.Predicate); err != nil {
		return errors.Fmt("predicate: %w", err)
	}

	return validateQueryRequest(req)
}

// QueryTestExonerations implements pb.ResultDBServer.
func (s *resultDBServer) QueryTestExonerations(ctx context.Context, in *pb.QueryTestExonerationsRequest) (*pb.QueryTestExonerationsResponse, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := permissions.VerifyInvocationsByName(ctx, in.Invocations, rdbperms.PermListTestExonerations); err != nil {
		return nil, err
	}

	if err := validateQueryTestExonerationsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Query is valid - increment the queryInvocationsCount metric
	queryInvocationsCount.Add(ctx, 1, "QueryTestExonerations", len(in.Invocations))

	// Get the transitive closure.
	invs, err := graph.Reachable(ctx, invocations.MustParseNames(in.Invocations))
	if err != nil {
		return nil, err
	}

	exonerationInvs, err := invs.WithExonerationsIDSet()
	if err != nil {
		return nil, err
	}

	// Query test exonerations.
	q := exonerations.Query{
		Predicate:     in.Predicate,
		PageSize:      pagination.AdjustPageSize(in.PageSize),
		PageToken:     in.PageToken,
		InvocationIDs: exonerationInvs,
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
