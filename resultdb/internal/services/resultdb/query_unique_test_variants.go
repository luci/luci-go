// Copyright 2022 The LUCI Authors.
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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/uniquetestvariants"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"
)

// QueryUniqueTestVariants implements pb.ResultDBServer.
func (s *resultDBServer) QueryUniqueTestVariants(
	ctx context.Context,
	in *pb.QueryUniqueTestVariantsRequest,
) (res *pb.QueryUniqueTestVariantsResponse, err error) {
	switch allowed, err := auth.HasPermission(ctx, rdbperms.PermListTestResults, in.Realm, nil); {
	case err != nil:
		return nil, err
	case !allowed:
		return nil, appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s in realm %q`, rdbperms.PermListTestResults, in.Realm)
	}

	in.PageSize = int32(pagination.AdjustPageSize(in.PageSize))
	return uniquetestvariants.Query(span.Single(ctx), in)
}

// validateQueryUniqueTestVariantsRequest checks that the required fields are set,
// and that field values are valid. It also assgins the default page size if
// it's not set.
func validateQueryUniqueTestVariantsRequest(in *pb.QueryUniqueTestVariantsRequest) error {
	if in.GetRealm() == "" {
		return errors.Reason("realm is required").Err()
	}
	if err := realms.ValidateRealmName(in.Realm, realms.GlobalScope); err != nil {
		return errors.Annotate(err, "realm").Err()
	}

	if in.GetTestId() == "" {
		return errors.Reason("test_id is required").Err()
	}

	if in.GetPageSize() < 0 {
		return errors.Reason("page_size, if specified, must be a positive integer").Err()
	}

	if err := pagination.ValidatePageSize(in.GetPageSize()); err != nil {
		return errors.Annotate(err, "page_size").Err()
	}

	return nil
}
