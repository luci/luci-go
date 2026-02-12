// Copyright 2026 The LUCI Authors.
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

package testhistory

import (
	"context"

	"go.chromium.org/luci/common/errors"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/pagination"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/analysis/rpc"
)

// Retrieves variants for a given test ID in a given project that were recorded
// in the past 90 days.
func (*testHistoryServer) QueryVariants(ctx context.Context, req *pb.QueryVariantsRequest) (*pb.QueryVariantsResponse, error) {
	if err := validateQueryVariantsRequest(req); err != nil {
		return nil, rpc.InvalidArgumentError(err)
	}

	subRealms, err := perms.QuerySubRealmsNonEmpty(ctx, req.Project, req.SubRealm, nil, rdbperms.PermListTestResults)
	if err != nil {
		return nil, err
	}
	var previousTestID string
	if req.FollowTestIdRenaming {
		previousTestID, err = queryPreviousTestIDFromResultDB(ctx, req.Project, req.TestId)
		if err != nil {
			return nil, err
		}
	}

	pageSize := int(rpc.PageSizeLimiter.Adjust(req.PageSize))
	opts := testresults.ReadVariantsOptions{
		Project:          req.GetProject(),
		TestID:           req.GetTestId(),
		PreviousTestID:   previousTestID,
		SubRealms:        subRealms,
		VariantPredicate: req.VariantPredicate,
		PageSize:         pageSize,
		PageToken:        req.PageToken,
	}

	variants, nextPageToken, err := testresults.ReadVariants(span.Single(ctx), opts)
	if err != nil {
		return nil, err
	}

	return &pb.QueryVariantsResponse{
		Variants:      variants,
		NextPageToken: nextPageToken,
	}, nil
}

func validateQueryVariantsRequest(req *pb.QueryVariantsRequest) error {
	if err := pbutil.ValidateProject(req.GetProject()); err != nil {
		return errors.Fmt("project: %w", err)
	}
	if err := rdbpbutil.ValidateTestID(req.TestId); err != nil {
		return errors.Fmt("test_id: %w", err)
	}
	if req.SubRealm != "" {
		if err := realms.ValidateRealmName(req.SubRealm, realms.ProjectScope); err != nil {
			return errors.Fmt("sub_realm: %w", err)
		}
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Fmt("page_size: %w", err)
	}

	if req.GetVariantPredicate() != nil {
		if err := pbutil.ValidateVariantPredicate(req.GetVariantPredicate()); err != nil {
			return errors.Fmt("predicate: %w", err)
		}
	}

	return nil
}
