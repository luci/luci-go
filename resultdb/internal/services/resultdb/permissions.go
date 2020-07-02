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

	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
)

var (
	permReadInvocation      = realms.RegisterPermission("resultdb.invocations.read")
	permReadTestExoneration = realms.RegisterPermission("resultdb.testExonerations.read")
	permReadTestResult      = realms.RegisterPermission("resultdb.testResults.read")
	permReadArtifact        = realms.RegisterPermission("resultdb.artifacts.read")

	permListTestExonerations = realms.RegisterPermission("resultdb.testExonerations.list")
	permListTestResults      = realms.RegisterPermission("resultdb.testResults.list")
	permListArtifacts        = realms.RegisterPermission("resultdb.artifacts.list")
)

// verifyPermission checks if the caller has the specified permission on the
// realm that the invocation with the specified id belongs to.
func verifyPermission(ctx context.Context, permission realms.Permission, id invocations.ID) error {
	realm, err := invocations.ReadRealm(ctx, span.Client(ctx).Single(), id)
	if err != nil {
		return err
	}
	// Legacy realm assigned to invocations that predate its use.
	if realm == "chromium:public" {
		return nil
	}
	switch allowed, err := auth.HasPermission(ctx, permission, []string{realm}); {
	case err != nil:
		return err
	case !allowed:
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission to read the requested resource`)
	}
	return nil
}

// verifyPermissionInvName does the same as verifyPermission but accepts an
// invocation name instead of an invocations.ID, and returns two separate errors:
// One for issues with the input, and one for denied permission.
func verifyPermissionInvName(ctx context.Context, permission realms.Permission, name string) (inputErr, permErr error) {
	invIDStr, inputErr := pbutil.ParseInvocationName(name)
	if inputErr != nil {
		return inputErr, nil
	}

	return nil, verifyPermission(ctx, permission, invocations.ID(invIDStr))
}
