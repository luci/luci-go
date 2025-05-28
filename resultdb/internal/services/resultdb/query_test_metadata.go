// Copyright 2023 The LUCI Authors.
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

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/testmetadata"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func validateQueryTestMetadataRequest(req *pb.QueryTestMetadataRequest) error {
	if err := pbutil.ValidateProject(req.GetProject()); err != nil {
		return errors.Fmt("project: %w", err)
	}
	if err := pbutil.ValidateTestMetadataPredicate(req.GetPredicate()); err != nil {
		return errors.Fmt("predicate: %w", err)
	}
	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Fmt("page_size: %w", err)
	}
	return nil
}

// QueryTestMetadata implements pb.ResultDBServer.
// It returns a collection of test metadata that matches the request.
func (s *resultDBServer) QueryTestMetadata(ctx context.Context, in *pb.QueryTestMetadataRequest) (*pb.QueryTestMetadataResponse, error) {
	// Validate project before using it to check permission.
	if err := pbutil.ValidateProject(in.Project); err != nil {
		return nil, appstatus.BadRequest(errors.Fmt("project: %w", err))
	}
	subRealms, err := permissions.QuerySubRealmsNonEmpty(ctx, in.Project, nil, rdbperms.PermListTestMetadata)
	if err != nil {
		return nil, err
	}
	if err := validateQueryTestMetadataRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	q := testmetadata.Query{
		Project:   in.Project,
		SubRealms: subRealms,
		Predicate: in.Predicate,
		PageSize:  pagination.AdjustPageSize(in.PageSize),
		PageToken: in.PageToken,
	}
	testMetadataDetails, nextPageToken, err := q.Fetch(ctx)
	if err != nil {
		return nil, errors.Fmt("fetch: %w", err)
	}
	return &pb.QueryTestMetadataResponse{
		TestMetadata:  testMetadataDetails,
		NextPageToken: nextPageToken,
	}, nil
}
