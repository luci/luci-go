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

package permissions

import (
	"context"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
)

// VerifyInvocation checks if the caller has the specified permission on the
// realm that the invocation with the specified id belongs to.
// There must not already be a transaction in the given context.
func VerifyInvocation(ctx context.Context, permission realms.Permission, ids ...invocations.ID) error {
	return VerifyBatch(ctx, permission, invocations.NewIDSet(ids...))
}

// VerifyBatch is checks multiple invocations' realms for the specified
// permission.
// There must not already be a transaction in the given context.
func VerifyBatch(ctx context.Context, permission realms.Permission, ids invocations.IDSet) (err error) {
	if len(ids) == 0 {
		return nil
	}
	ctx, ts := trace.StartSpan(ctx, "resultdb.permissions.VerifyBatch")
	defer func() { ts.End(err) }()

	realms, err := invocations.ReadRealms(span.Single(ctx), ids)
	if err != nil {
		return err
	}

	checked := stringset.New(1)
	for id, realm := range realms {
		if !checked.Add(realm) {
			continue
		}
		// Note: HasPermission does not make RPCs.
		switch allowed, err := auth.HasPermission(ctx, permission, realm, nil); {
		case err != nil:
			return err
		case !allowed:
			return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s in realm of invocation %s`, permission, id)
		}
	}
	return nil
}

// VerifyInvNames does the same as VerifyInvocation but accepts
// invocation names (variadic)  instead of a single invocations.ID.
// There must not already be a transaction in the given context.
func VerifyInvNames(ctx context.Context, permission realms.Permission, invNames ...string) error {
	ids, err := invocations.ParseNames(invNames)
	if err != nil {
		return appstatus.BadRequest(err)
	}
	return VerifyBatch(ctx, permission, ids)
}
