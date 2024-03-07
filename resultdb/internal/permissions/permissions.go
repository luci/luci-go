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
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
)

// VerifyInvocation checks if the caller has the specified permissions on the
// realm that the invocation with the specified id belongs to.
// There must must be a valid Spanner transaction in the given context, which
// may be a span.Single().
func VerifyInvocation(ctx context.Context, id invocations.ID, permissions ...realms.Permission) error {
	return VerifyInvocations(ctx, invocations.NewIDSet(id), permissions...)
}

// VerifyInvocations checks multiple invocations' realms for the specified
// permissions.
// There must must be a valid Spanner transaction in the given context, which
// may be a span.Single().
func VerifyInvocations(ctx context.Context, ids invocations.IDSet, permissions ...realms.Permission) (err error) {
	if len(ids) == 0 {
		return nil
	}
	ctx, ts := tracing.Start(ctx, "resultdb.permissions.VerifyInvocations")
	defer func() { tracing.End(ts, err) }()

	realms, err := invocations.ReadRealms(ctx, ids)
	if err != nil {
		return err
	}

	// Note: HasPermissionsInRealms does not make RPCs.
	verified, desc, err := HasPermissionsInRealms(ctx, realms, permissions...)
	if err != nil {
		return err
	}
	if !verified {
		return appstatus.Errorf(codes.PermissionDenied, desc)
	}

	return nil
}

// VerifyInvocationsByName does the same as VerifyInvocations but accepts
// invocation names instead of an invocations.IDSet.
// There must must be a valid Spanner transaction in the given context, which
// may be a span.Single().
func VerifyInvocationsByName(ctx context.Context, invNames []string, permissions ...realms.Permission) error {
	ids, err := invocations.ParseNames(invNames)
	if err != nil {
		return appstatus.BadRequest(err)
	}
	return VerifyInvocations(ctx, ids, permissions...)
}

// VerifyInvocationByName does the same as VerifyInvocation but accepts
// an invocation name instead of an invocations.ID.
// There must must be a valid Spanner transaction in the given context, which
// may be a span.Single().
func VerifyInvocationByName(ctx context.Context, invName string, permissions ...realms.Permission) error {
	return VerifyInvocationsByName(ctx, []string{invName}, permissions...)
}

// HasPermissionsInRealms checks multiple invocations' realms for the specified
// permissions. Returns:
//   - whether the caller has all permissions in all invocations' realms
//   - description of the first identified missing permission for an invocation
//     (if applicable)
//   - an error if one occurred
func HasPermissionsInRealms(ctx context.Context, realms map[invocations.ID]string, permissions ...realms.Permission) (bool, string, error) {
	checked := stringset.New(1)
	for id, realm := range realms {
		if !checked.Add(realm) {
			continue
		}
		// Note: HasPermission does not make RPCs.
		for _, permission := range permissions {
			switch allowed, err := auth.HasPermission(ctx, permission, realm, nil); {
			case err != nil:
				return false, "", err
			case !allowed:
				return false, fmt.Sprintf(`caller does not have permission %s in realm of invocation %s`, permission, id), nil
			}
		}
	}
	return true, "", nil
}

// QuerySubRealmsNonEmpty returns subRealms that the user has the given permission in the given project.
// It returns an appstatus annotated error if there is no realm in which the user has the permission.
func QuerySubRealmsNonEmpty(ctx context.Context, project string, attrs realms.Attrs, permission realms.Permission) ([]string, error) {
	if project == "" {
		return nil, errors.New("project must be specified")
	}
	allowedRealms, err := auth.QueryRealms(ctx, permission, project, attrs)
	if err != nil {
		return nil, err
	}
	if len(allowedRealms) == 0 {
		return nil, appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %v in any realm in project %q`, permission, project)
	}
	subRealms := make([]string, 0, len(allowedRealms))
	for _, r := range allowedRealms {
		_, subRealm := realms.Split(r)
		subRealms = append(subRealms, subRealm)
	}
	return subRealms, nil
}
