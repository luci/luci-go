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
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/analysis/internal/pagination"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testrealms"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/analysis/rpc"
)

// QueryTests finds all test IDs that contain the given substring in a given
// project that were recorded in the past 90 days.
func (s *testHistoryServer) QueryTests(ctx context.Context, req *pb.QueryTestsRequest) (*pb.QueryTestsResponse, error) {
	if err := validateQueryTestsRequest(req); err != nil {
		return nil, rpc.InvalidArgumentError(err)
	}

	subRealms, err := perms.QuerySubRealmsNonEmpty(ctx, req.Project, req.SubRealm, nil, rdbperms.PermListTestResults)
	if err != nil {
		return nil, err
	}
	authorizedRealms := make([]string, 0, len(subRealms))
	for _, subRealm := range subRealms {
		authorizedRealms = append(authorizedRealms, realms.Join(req.Project, subRealm))
	}

	pageSize := int(rpc.PageSizeLimiter.Adjust(req.PageSize))
	opts := testrealms.QueryTestsOptions{
		CaseSensitive: !req.CaseInsensitive,
		Realms:        authorizedRealms,
		PageSize:      pageSize,
		PageToken:     req.GetPageToken(),
	}

	testIDs, nextPageToken, err := s.searchClient.QueryTests(ctx, req.Project, req.TestIdSubstring, opts)
	if err != nil {
		return nil, err
	}

	return &pb.QueryTestsResponse{
		TestIds:       testIDs,
		NextPageToken: nextPageToken,
	}, nil
}

func validateQueryTestsRequest(req *pb.QueryTestsRequest) error {
	if err := pbutil.ValidateProject(req.GetProject()); err != nil {
		return errors.Fmt("project: %w", err)
	}
	if err := rpc.ValidateTestIDPart(req.TestIdSubstring); err != nil {
		return errors.Fmt("test_id_substring: %w", err)
	}
	if req.SubRealm != "" {
		if err := realms.ValidateRealmName(req.SubRealm, realms.ProjectScope); err != nil {
			return errors.Fmt("sub_realm: %w", err)
		}
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Fmt("page_size: %w", err)
	}

	return nil
}
