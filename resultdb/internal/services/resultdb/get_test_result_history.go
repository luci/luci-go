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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/history"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

// GetTestResultHistory implements pb.ResultDBServer.
func (s *resultDBServer) GetTestResultHistory(ctx context.Context, in *pb.GetTestResultHistoryRequest) (*pb.GetTestResultHistoryResponse, error) {
	if err := verifyGetTestResultHistoryPermission(ctx, in.GetRealm()); err != nil {
		return nil, err
	}
	if err := validateGetTestResultHistoryRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	ctx, cancelTx := span.ReadOnlyTransaction(ctx)
	defer cancelTx()

	q := &history.Query{Request: in}
	entries, token, err := q.Execute(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.GetTestResultHistoryResponse{
		Entries:       entries,
		NextPageToken: token,
	}, nil
}

// verifyGetTestResultHistoryPermission checks that the caller has permission to
// get test results from the specified realm.
func verifyGetTestResultHistoryPermission(ctx context.Context, realm string) error {
	if realm == "" {
		return appstatus.BadRequest(errors.Reason("realm is required").Err())
	}
	switch allowed, err := auth.HasPermission(ctx, rdbperms.PermListTestResults, realm, nil); {
	case err != nil:
		return err
	case !allowed:
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s in realm %q`, rdbperms.PermListTestResults, realm)
	}
	return nil
}

// validateGetTestResultHistoryRequest checks that the required fields are set,
// and that field values are valid.
func validateGetTestResultHistoryRequest(in *pb.GetTestResultHistoryRequest) error {
	if in.GetRealm() == "" {
		return errors.Reason("realm is required").Err()
	}
	if err := realms.ValidateRealmName(in.Realm, realms.GlobalScope); err != nil {
		return errors.Annotate(err, "realm").Err()
	}
	// TODO(crbug.com/1107680): Add support for commit position ranges.
	tr := in.GetTimeRange()
	if tr == nil {
		return errors.Reason("time_range must be specified").Err()
	}
	if in.GetPageSize() < 0 {
		return errors.Reason("page_size, if specified, must be a positive integer").Err()
	}
	if in.GetVariantPredicate() != nil {
		if err := pbutil.ValidateVariantPredicate(in.GetVariantPredicate()); err != nil {
			return errors.Annotate(err, "variant_predicate").Err()
		}
	}
	if in.GetPageToken() != "" {
		if err := history.ValidatePageToken(in.GetPageToken()); err != nil {
			return err
		}
	}
	return nil
}
