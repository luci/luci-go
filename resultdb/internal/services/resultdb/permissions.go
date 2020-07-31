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
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/pbutil"
)

var (
	permGetInvocation      = realms.RegisterPermission("resultdb.invocations.get")
	permGetTestExoneration = realms.RegisterPermission("resultdb.testExonerations.get")
	permGetTestResult      = realms.RegisterPermission("resultdb.testResults.get")
	permGetArtifact        = realms.RegisterPermission("resultdb.artifacts.get")

	permListTestExonerations = realms.RegisterPermission("resultdb.testExonerations.list")
	permListTestResults      = realms.RegisterPermission("resultdb.testResults.list")
	permListArtifacts        = realms.RegisterPermission("resultdb.artifacts.list")
)

// verifyPermission checks if the caller has the specified permission on the
// realm that the invocation with the specified id belongs to.
func verifyPermission(ctx context.Context, permission realms.Permission, id invocations.ID) error {
	realm, err := invocations.ReadRealm(span.Single(ctx), id)
	if err != nil {
		return err
	}
	switch allowed, err := auth.HasPermission(ctx, permission, realm); {
	case err != nil:
		return err
	case !allowed:
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s in realm of invocation %s`, permission, id)
	}
	return nil
}

// verifyPermissionInvNames does the same as verifyPermission but accepts
// invocation names (variadic)  instead of a single  invocations.ID.
func verifyPermissionInvNames(ctx context.Context, permission realms.Permission, invNames ...string) error {
	for _, n := range invNames {
		invIDStr, inputErr := pbutil.ParseInvocationName(n)
		if inputErr != nil {
			return appstatus.BadRequest(inputErr)
		}
		if err := verifyPermission(ctx, permission, invocations.ID(invIDStr)); err != nil {
			return err
		}
	}
	return nil
}
