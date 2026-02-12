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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/pagination"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/analysis/rpc"
)

// Retrieves a summary of test verdicts for a given test ID in a given project
// and in a given range of times.
func (s *testHistoryServer) QueryStats(ctx context.Context, req *pb.QueryTestHistoryStatsRequest) (*pb.QueryTestHistoryStatsResponse, error) {
	if err := validateQueryTestHistoryStatsRequest(req); err != nil {
		return nil, rpc.InvalidArgumentError(err)
	}

	subRealms, err := perms.QuerySubRealmsNonEmpty(ctx, req.Project, req.Predicate.SubRealm, nil, perms.ListTestResultsAndExonerations...)
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
	opts := testresults.ReadTestHistoryOptions{
		Project:                 req.Project,
		TestID:                  req.TestId,
		PreviousTestID:          previousTestID,
		SubRealms:               subRealms,
		VariantPredicate:        req.Predicate.VariantPredicate,
		SubmittedFilter:         req.Predicate.SubmittedFilter,
		TimeRange:               req.Predicate.PartitionTimeRange,
		ExcludeBisectionResults: !req.Predicate.IncludeBisectionResults,
		PageSize:                pageSize,
		PageToken:               req.PageToken,
	}

	now := clock.Now(ctx)
	readTxn, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	groups, nextPageToken, err := testresults.ReadTestHistoryStats(readTxn, opts, now)
	if err != nil {
		return nil, err
	}

	return &pb.QueryTestHistoryStatsResponse{
		Groups:        groups,
		NextPageToken: nextPageToken,
	}, nil
}

func validateQueryTestHistoryStatsRequest(req *pb.QueryTestHistoryStatsRequest) error {
	if err := pbutil.ValidateProject(req.GetProject()); err != nil {
		return errors.Fmt("project: %w", err)
	}
	if err := rdbpbutil.ValidateTestID(req.TestId); err != nil {
		return errors.Fmt("test_id: %w", err)
	}

	if err := pbutil.ValidateTestVerdictPredicate(req.GetPredicate()); err != nil {
		return errors.Fmt("predicate: %w", err)
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Fmt("page_size: %w", err)
	}

	return nil
}
