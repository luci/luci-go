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
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/rdbperms"
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
		return appstatus.Error(codes.PermissionDenied, desc)
	}

	return nil
}

// VerifyRootInvocation verifies the caller has the given permissions in
// the realm of the given root invocation. If the root invocation is not
// found, a NotFound appstatus error is returned (thus disclosing the
// non-existence of the root invocation to all callers).
func VerifyRootInvocation(ctx context.Context, id rootinvocations.ID, permissions ...realms.Permission) (err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.permissions.VerifyRootInvocation")
	defer func() { tracing.End(ts, err) }()

	realm, err := rootinvocations.ReadRealm(ctx, id)
	if err != nil {
		// If the root invocation is not found, returns NotFound appstatus error.
		return err
	}

	// Note: HasPermission does not make RPCs.
	for _, permission := range permissions {
		switch allowed, err := auth.HasPermission(ctx, permission, realm, nil); {
		case err != nil:
			return err
		case !allowed:
			return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s in realm of root invocation %q`, permission, id.Name())
		}
	}
	return nil
}

type AccessLevel int

const (
	// The user has no access to the resource.
	NoAccess AccessLevel = iota
	// The user has limited access to the resource (e.g. only masked access).
	LimitedAccess
	// The user has full access to the resource.
	FullAccess
)

// QueryWorkUnitAccessOptions defines the permissions required to reach a certain access level
// on the work unit.
type QueryWorkUnitAccessOptions struct {
	// The permission required on the *root invocation*'s realm to reach
	// FullAccess access level.
	Full realms.Permission
	// The permission required on the *root invocation*'s realm to reach
	// LimitedAccess access level.
	Limited realms.Permission
	// The permission required on the *work unit*'s realm to upgrade limited
	// access to full access.
	UpgradeLimitedToFull realms.Permission
}

// GetWorkUnitsAccessModel defines the permissions used to authorize access
// when getting work units (e.g. Get or BatchGetWorkUnits).
var GetWorkUnitsAccessModel = QueryWorkUnitAccessOptions{
	Full:                 rdbperms.PermGetWorkUnit,          // At root invocation level
	Limited:              rdbperms.PermListLimitedWorkUnits, // At root invocation level
	UpgradeLimitedToFull: rdbperms.PermGetWorkUnit,          // At work unit level
}

var ListArtifactsAccessModel = QueryWorkUnitAccessOptions{
	Full:                 rdbperms.PermListArtifacts,        // At root invocation level
	Limited:              rdbperms.PermListLimitedArtifacts, // At root invocation level
	UpgradeLimitedToFull: rdbperms.PermGetArtifact,          // At work unit level
}

// QueryWorkUnitAccess determines the access the user has to the given work unit.
//
// A NotFound appstatus error is returned if the root invocation is not found (thereby
// disclosing its existence, even to unauthenticated callers). A NotFound appstatus error
// may also be returned if the work unit is not found, if it was necessary to check the
// work unit realm.
func QueryWorkUnitAccess(ctx context.Context, id workunits.ID, opts QueryWorkUnitAccessOptions) (accessLevel AccessLevel, err error) {
	result, err := QueryWorkUnitsAccess(ctx, []workunits.ID{id}, opts)
	if err != nil {
		return NoAccess, err
	}
	return result[0], nil
}

// QueryWorkUnitsAccess determines the access the user has to the given work units.
// All work units must belong to the same root invocation.
//
// A NotFound appstatus error is returned if the root invocation is not found (thereby
// disclosing its existence, even to unauthenticated callers). A NotFound appstatus error
// may also be returned if any work unit is not found, if it was necessary to check the
// work unit realm.
func QueryWorkUnitsAccess(ctx context.Context, ids []workunits.ID, opts QueryWorkUnitAccessOptions) (accessLevels []AccessLevel, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.permissions.QueryWorkUnitsAccess")
	defer func() { tracing.End(ts, err) }()

	if len(ids) == 0 {
		return nil, nil
	}
	rootInvID := ids[0].RootInvocationID

	// Check all IDs have the same root invocation, to ensure integrity of the
	// permission checks that follow.
	for _, id := range ids {
		if id.RootInvocationID != rootInvID {
			return nil, fmt.Errorf("all work units must belong to the same root invocation")
		}
	}

	// Avoid hotspotting the root invocation record by reading the realm from the same
	// shard as the work unit.
	rootInvRealm, err := rootinvocations.ReadRealmFromShard(ctx, ids[0].RootInvocationShardID())
	if err != nil {
		// If the root invocation is not found, returns NotFound appstatus error.
		return nil, err
	}

	// Note: HasPermission does not make RPCs.
	allowed, err := auth.HasPermission(ctx, opts.Full, rootInvRealm, nil)
	if err != nil {
		// Some sort of internal error doing the permission check.
		return nil, err
	}
	if allowed {
		// Break out early, we have full access to all work units.
		return repeatAccessLevel(FullAccess, len(ids)), nil
	}
	allowed, err = auth.HasPermission(ctx, opts.Limited, rootInvRealm, nil)
	if err != nil {
		// Some sort of internal error doing the permission check.
		return nil, err
	}
	if allowed {
		// We have limited access. Try to see if we can upgrade it to full access.
		workUnitRealms, err := workunits.ReadRealms(ctx, ids)
		if err != nil {
			// If any work unit is not found, returns NotFound appstatus error.
			return nil, err
		}

		accessLevels = make([]AccessLevel, len(ids))
		for i, id := range ids {
			allowed, err := auth.HasPermission(ctx, opts.UpgradeLimitedToFull, workUnitRealms[id], nil)
			if err != nil {
				// Some sort of internal error doing the permission check.
				return nil, err
			}
			if allowed {
				accessLevels[i] = FullAccess
			} else {
				accessLevels[i] = LimitedAccess
			}
		}
		return accessLevels, nil
	}
	return repeatAccessLevel(NoAccess, len(ids)), nil
}

func repeatAccessLevel(accessLevel AccessLevel, n int) []AccessLevel {
	ret := make([]AccessLevel, n)
	for i := range ret {
		ret[i] = accessLevel
	}
	return ret
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
