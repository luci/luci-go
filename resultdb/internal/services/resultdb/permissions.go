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

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
)

var (
	permReadInvocation = realms.RegisterPermission("resultdb.invocations.read")
	permReadTestResult = realms.RegisterPermission("resultdb.testResults.read")
	permReadArtifact   = realms.RegisterPermission("resultdb.artifacts.read")
)

func verifyArtifactReadPermission(ctx context.Context, name string) error {
	invIDStr, _, _, _, err := pbutil.ParseArtifactName(name)
	if err != nil {
		return err
	}
	return verifyPermission(ctx, permReadArtifact, invocations.ID(invIDStr))
}

// verifyPermission checks if the caller has the specified permission on the
// realm that the specified invocation belongs to.
//
// Both invocation name (as string) and invocation id (as invocations.ID)
// are acceptable values for the inv argument.
func verifyPermission(ctx context.Context, permission realms.Permission, inv interface{}) error {
	// Accept invocation name, invocation id
	var id invocations.ID
	switch inv.(type) {
	case string:
		invIDStr, err := pbutil.ParseInvocationName(inv.(string))
		if err != nil {
			return err
		}
		id = invocations.ID(invIDStr)
	case invocations.ID:
		id = inv.(invocations.ID)
	default:
		return errors.Reason("expected inv to be either an invocation name or an invocation id").Err()
	}
	realm, err := invocations.ReadRealm(ctx, span.Client(ctx).ReadOnlyTransaction(), id)
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
