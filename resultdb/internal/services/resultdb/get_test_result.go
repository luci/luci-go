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

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func verifyGetTestResultPermission(ctx context.Context, resultName string) error {
	invID, _, _, err := pbutil.ParseTestResultName(resultName)
	if err != nil {
		return appstatus.BadRequest(errors.Fmt("name: %w", err))
	}

	return permissions.VerifyInvocation(ctx, invocations.ID(invID), rdbperms.PermGetTestResult)
}

func validateGetTestResultRequest(req *pb.GetTestResultRequest) error {
	if err := pbutil.ValidateTestResultName(req.Name); err != nil {
		return errors.Fmt("name: %w", err)
	}

	return nil
}

func (s *resultDBServer) GetTestResult(ctx context.Context, in *pb.GetTestResultRequest) (*pb.TestResult, error) {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := verifyGetTestResultPermission(ctx, in.GetName()); err != nil {
		return nil, err
	}
	if err := validateGetTestResultRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	tr, err := testresults.Read(ctx, in.Name)
	if err != nil {
		return nil, err
	}

	return tr, nil
}
