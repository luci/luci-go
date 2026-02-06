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
	"strings"

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
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/permissions.VerifyInvocations")
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
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/permissions.VerifyRootInvocation")
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

// VerifyWorkUnitAccessOptions defines the permissions required to reach a certain access level
// on the work unit.
type VerifyWorkUnitAccessOptions struct {
	// The permission(s) required on the *root invocation*'s realm to reach
	// FullAccess access level.
	Full []realms.Permission
	// The permission(s) required on the *root invocation*'s realm to reach
	// LimitedAccess access level.
	Limited []realms.Permission
	// The permission(s) required on the *work unit*'s realm to upgrade limited
	// access to full access.
	UpgradeLimitedToFull []realms.Permission
}

// ListWorkUnitsAccessModel defines the permissions used to authorize access
// when listing work units (e.g. QueryWorkUnits).
var ListWorkUnitsAccessModel = VerifyWorkUnitAccessOptions{
	Full:                 []realms.Permission{rdbperms.PermListWorkUnits},        // At root invocation level
	Limited:              []realms.Permission{rdbperms.PermListLimitedWorkUnits}, // At root invocation level
	UpgradeLimitedToFull: []realms.Permission{rdbperms.PermGetWorkUnit},          // At work unit level
}

// GetWorkUnitsAccessModel defines the permissions used to authorize access
// when getting work units (e.g. Get or BatchGetWorkUnits).
var GetWorkUnitsAccessModel = VerifyWorkUnitAccessOptions{
	Full:                 []realms.Permission{rdbperms.PermGetWorkUnit},          // At root invocation level
	Limited:              []realms.Permission{rdbperms.PermListLimitedWorkUnits}, // At root invocation level
	UpgradeLimitedToFull: []realms.Permission{rdbperms.PermGetWorkUnit},          // At work unit level
}

// GetArtifactsAccessModel defines the permissions used to authorize access
// when getting artifacts (e.g. GetArtifact).
var GetArtifactsAccessModel = VerifyWorkUnitAccessOptions{
	Full:                 []realms.Permission{rdbperms.PermGetArtifact},          // At root invocation level
	Limited:              []realms.Permission{rdbperms.PermListLimitedArtifacts}, // At root invocation level
	UpgradeLimitedToFull: []realms.Permission{rdbperms.PermGetArtifact},          // At work unit level
}

// ListArtifactsAccessModel defines the permissions used to authorize access
// when listing artifacts (e.g. ListArtifacts).
var ListArtifactsAccessModel = VerifyWorkUnitAccessOptions{
	Full:                 []realms.Permission{rdbperms.PermListArtifacts},        // At root invocation level
	Limited:              []realms.Permission{rdbperms.PermListLimitedArtifacts}, // At root invocation level
	UpgradeLimitedToFull: []realms.Permission{rdbperms.PermGetArtifact},          // At work unit level
}

// ListAggregatesAccessModel defines the permissions used to authorize access
// when listing test aggregations (e.g. QueryTestAggregations).
var ListAggregatesAccessModel = VerifyWorkUnitAccessOptions{
	// Need ListWorkUnits because test aggregations include module aggregations,
	// which rely on the work unit's module and status fields.
	Full: []realms.Permission{ // At root invocation level
		rdbperms.PermListWorkUnits,
		rdbperms.PermListTestResults,
		rdbperms.PermListTestExonerations,
	},
	Limited: []realms.Permission{ // At root invocation level
		rdbperms.PermListLimitedWorkUnits,
		rdbperms.PermListLimitedTestResults,
		rdbperms.PermListLimitedTestExonerations,
	},
	UpgradeLimitedToFull: []realms.Permission{ // At work unit level
		rdbperms.PermGetWorkUnit,
		rdbperms.PermGetTestResult,
		rdbperms.PermGetTestExoneration,
	},
}

// ListVerdictsAccessModel defines the permissions used to authorize access
// when listing test verdicts (e.g. QueryTestVerdicts).
var ListVerdictsAccessModel = VerifyWorkUnitAccessOptions{
	Full: []realms.Permission{ // At root invocation level
		rdbperms.PermListTestResults,
		rdbperms.PermListTestExonerations,
	},
	Limited: []realms.Permission{ // At root invocation level
		rdbperms.PermListLimitedTestResults,
		rdbperms.PermListLimitedTestExonerations,
	},
	UpgradeLimitedToFull: []realms.Permission{ // At work unit level
		rdbperms.PermGetTestResult,
		rdbperms.PermGetTestExoneration,
	},
}

// VerifyWorkUnitAccess determines the access the user has to a work unit or work
// unit-scoped resource, such as test results, test artifacts or test exonerations.
// It further verifies that the user has at least the `minimumAccessLevel` specified.
//
// A NotFound appstatus error is returned if the root invocation is not found (thereby
// disclosing its existence, even to unauthenticated callers). A NotFound appstatus error
// may also be returned if the work unit is not found, if it was necessary to check the
// work unit realm.
//
// If the required permission level is not attained for each work unit, a PermissionDenied
// appstatus error is returned.
func VerifyWorkUnitAccess(ctx context.Context, id workunits.ID, opts VerifyWorkUnitAccessOptions, minimumAccessLevel AccessLevel) (accessLevel AccessLevel, err error) {
	result, err := VerifyWorkUnitsAccess(ctx, []workunits.ID{id}, opts, minimumAccessLevel)
	if err != nil {
		return NoAccess, err
	}
	return result[0], nil
}

// VerifyWorkUnitsAccess determines the access the user has to work units or work
// unit-scoped resources, such as test results, test artifacts or test exonerations.
// It further verifies that for each work unit, this access is at least the
// `minimumAccessLevel` specified.
//
// All work units IDs specified must belong to the same root invocation. As the user
// needs access to the root invocation to access its work units, this method also checks
// access to the root invocation.
//
// To operate correctly, this method requires the correct permission set to be specified
// via `opts`, according to the resource for which access is ultimately being verified
// (e.g. test results, test artifacts, test exonerations, work units...).
//
// A NotFound appstatus error is returned if the root invocation is not found (thereby
// disclosing its existence, even to unauthenticated callers). A NotFound appstatus error
// may also be returned if any work unit is not found, if it was necessary to check the
// work unit realm.
//
// If the required permission level is not attained for each work unit, a PermissionDenied
// appstatus error is returned.
func VerifyWorkUnitsAccess(ctx context.Context, ids []workunits.ID, opts VerifyWorkUnitAccessOptions, minimumAccessLevel AccessLevel) (accessLevels []AccessLevel, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/permissions.VerifyWorkUnitsAccess")
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
		return nil, fmt.Errorf("read realm from shard: %w", err)
	}

	// Note: HasPermission does not make RPCs.
	allowed, err := hasAllPermissions(ctx, rootInvRealm, opts.Full...)
	if err != nil {
		// Some sort of internal error doing the permission check.
		return nil, err
	}
	if allowed {
		// Break out early, we have full access to all work units.
		return repeatAccessLevel(FullAccess, len(ids)), nil
	}

	allowed, err = hasAllPermissions(ctx, rootInvRealm, opts.Limited...)
	if err != nil {
		// Some sort of internal error doing the permission check.
		return nil, err
	}
	if !allowed {
		if minimumAccessLevel != NoAccess {
			return nil, NoRootInvocationAccessError(ids[0].RootInvocationID, opts)
		}
		return repeatAccessLevel(NoAccess, len(ids)), nil
	}
	// We have limited access. Try to see if we can upgrade it to full access.
	workUnitRealms, err := workunits.ReadRealms(ctx, ids)
	if err != nil {
		// If any work unit is not found, returns NotFound appstatus error.
		return nil, fmt.Errorf("read realms: %w", err)
	}

	accessLevels = make([]AccessLevel, len(ids))
	for i, id := range ids {
		allowed, err := hasAllPermissions(ctx, workUnitRealms[id], opts.UpgradeLimitedToFull...)
		if err != nil {
			// Some sort of internal error doing the permission check.
			return nil, err
		}
		if allowed {
			accessLevels[i] = FullAccess
		} else {
			if minimumAccessLevel == FullAccess {
				return nil, noUpgradeAccessError(id, workUnitRealms[id], opts)
			}
			accessLevels[i] = LimitedAccess
		}
	}
	return accessLevels, nil
}

// RootInvocationAccess represents the access level of the user has to
// the requested work unit-scoped resources (test results, artifacts,
// exonerations, etc.) in a root invocation.
//
// This can be passed to test aggregation and test verdict queries
// to inform the result masking that needs to occur.
type RootInvocationAccess struct {
	// The level of access to the root invocation.
	//
	// If this is Full, the user has full access to all (test
	// results / artifacts / exonerations) in the root invocation.
	//
	// If this is Limited, the user has limited access to all (test
	// results / artifacts / exonerations), upgraded to full access only
	// for resources in a work unit that has a realm listed under `Realms`.
	Level AccessLevel
	// The work unit realms the user has full access to.
	// IMPORTANT: This is only set if `Level` is Limited.
	Realms []string
}

// VerifyAllWorkUnitsAccess determines the access the user has a work unit-scoped
// resource (e.g. test results, test artifacts, test exonerations)
// in a root invocation. It further verifies that this access is at least the
// `minimumAccessLevel` specified.
//
// As the user needs access to the root invocation to access its work units, this
// method also checks access to the root invocation.
//
// To operate correctly, this method requires the correct permission set to be specified
// via `opts`, according to the resource for which access is ultimately being verified
// (e.g. test results, test artifacts, work units...).
//
// A NotFound appstatus error is returned if the root invocation is not found (thereby
// disclosing its existence, even to unauthenticated callers).
//
// If the required permission level is not attained for each work unit, a PermissionDenied
// appstatus error is returned.
func VerifyAllWorkUnitsAccess(ctx context.Context, id rootinvocations.ID, opts VerifyWorkUnitAccessOptions, minimumAccessLevel AccessLevel) (result RootInvocationAccess, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/permissions.VerifyAllWorkUnitsAccess")
	defer func() { tracing.End(ts, err) }()

	rootInvRealm, err := rootinvocations.ReadRealm(ctx, id)
	if err != nil {
		// If the root invocation is not found, returns NotFound appstatus error.
		return RootInvocationAccess{}, err
	}

	// Note: HasPermission does not make RPCs.
	allowed, err := hasAllPermissions(ctx, rootInvRealm, opts.Full...)
	if err != nil {
		// Some sort of internal error doing the permission check.
		return RootInvocationAccess{}, err
	}
	if allowed {
		// Break out early, we have full access to the root invocation
		// and its work units.
		return RootInvocationAccess{Level: FullAccess}, nil
	}

	if minimumAccessLevel == FullAccess {
		// We required full access and we don't have it. Break out early.
		//
		// The approach we take in VerifyWorkUnitsAccess where we try
		// to see if we have limited access to the root invocation and can
		// upgrade limited access to full access for given work units
		// is not viable here, as it is not data-race safe (new work units
		// may be added after we queried all work units realms above).
		return RootInvocationAccess{}, noFullRootInvocationAccessError(id, opts)
	}

	// At this point the user does not have full access, could they have
	// limited access?
	allowed, err = hasAllPermissions(ctx, rootInvRealm, opts.Limited...)
	if err != nil {
		// Some sort of internal error doing the permission check.
		return RootInvocationAccess{}, err
	}
	if !allowed {
		// Break out early, we have no access to the root invocation or
		// its work units.
		if minimumAccessLevel != NoAccess {
			return RootInvocationAccess{}, NoRootInvocationAccessError(id, opts)
		}
		return RootInvocationAccess{Level: NoAccess}, nil
	}

	// The user has limited access to the root invocation.
	uniqueRealms, err := workunits.ReadAllRealms(ctx, id)
	if err != nil {
		return RootInvocationAccess{}, err
	}
	allowedRealms := make([]string, 0, len(uniqueRealms))
	// Check if the user has full access to any of the work units.
	for _, realm := range uniqueRealms {
		allowed, err := hasAllPermissions(ctx, realm, opts.UpgradeLimitedToFull...)
		if err != nil {
			// Some sort of internal error doing the permission check.
			return RootInvocationAccess{}, err
		}
		if allowed {
			allowedRealms = append(allowedRealms, realm)
		}
	}
	return RootInvocationAccess{Level: LimitedAccess, Realms: allowedRealms}, nil
}

// NoRootInvocationAccessError returns a PermissionDenied appstatus error
// indicating that the user has no access to a root invocation.
func NoRootInvocationAccessError(id rootinvocations.ID, opts VerifyWorkUnitAccessOptions) error {
	// There are two access paths: limited access and full access. Neither were satisfied.
	if len(opts.Full) == 1 && len(opts.Limited) == 1 {
		return appstatus.Errorf(codes.PermissionDenied, "caller does not have permission %s (or %s) in realm of root invocation %q", opts.Full[0], opts.Limited[0], id.Name())
	}
	return appstatus.Errorf(codes.PermissionDenied, "caller does not have permissions %s (or %s) in realm of root invocation %q",
		permissionsList(opts.Limited...), permissionsList(opts.Full...), id.Name())
}

// noFullRootInvocationAccessError returns a PermissionDenied appstatus error
// indicating that the user does not have full access to a root invocation.
func noFullRootInvocationAccessError(id rootinvocations.ID, opts VerifyWorkUnitAccessOptions) error {
	// There are two access paths: limited access and full access. Neither were satisfied.
	if len(opts.Full) == 1 {
		return appstatus.Errorf(codes.PermissionDenied, "caller does not have permission %s in realm of root invocation %q", opts.Full[0], id.Name())
	}
	return appstatus.Errorf(codes.PermissionDenied, "caller does not have permissions %s in realm of root invocation %q",
		permissionsList(opts.Full...), id.Name())
}

// noUpgradeAccessError returns a PermissionDenied appstatus error
// indicating the user does not have permission to upgrade limited access to
// full access.
func noUpgradeAccessError(id workunits.ID, realm string, opts VerifyWorkUnitAccessOptions) error {
	// Unlike root invocation errors, where we do not disclose the realm, this permission
	// error is only returned if the user already has limited access to the root invocation.
	// This is sufficient to allow us to disclose the realm of the work unit.
	if len(opts.UpgradeLimitedToFull) == 1 {
		return appstatus.Errorf(codes.PermissionDenied, "caller does not have permission %s in realm %q of work unit %q (trying to upgrade limited access to full access)", opts.UpgradeLimitedToFull[0], realm, id.Name())
	}
	return appstatus.Errorf(codes.PermissionDenied, "caller does not have permissions %s in realm %q of work unit %q (trying to upgrade limited access to full access)", permissionsList(opts.UpgradeLimitedToFull...), realm, id.Name())
}

// permissionsList returns a string representation of the given list of permissions.
func permissionsList(permissions ...realms.Permission) string {
	var permissionList strings.Builder
	for i, permission := range permissions {
		if i > 0 {
			permissionList.WriteString(", ")
		}
		permissionList.WriteString(permission.String())
	}
	return fmt.Sprintf("[%s]", permissionList.String())
}

// hasAllPermissions checks if the user has all the given permissions in the given realm.
func hasAllPermissions(ctx context.Context, realm string, permissions ...realms.Permission) (bool, error) {
	for _, permission := range permissions {
		allowed, err := auth.HasPermission(ctx, permission, realm, nil)
		if err != nil {
			// Some sort of internal error doing the permission check.
			return false, err
		}
		if !allowed {
			return false, nil
		}
	}
	return true, nil
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
//
// Deprecated: this method does not produce good quality error messages as
// the request field name corresponding to `invNames` is not known. Callers
// should parse the invocation names themselves and call VerifyInvocations
// directly.
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
//
// Deprecated: this method does not produce good quality error messages as
// the request field name corresponding to `invName` is not known. Callers
// should parse the invocation names themselves and call VerifyInvocation
// directly.
func VerifyInvocationByName(ctx context.Context, invName string, permissions ...realms.Permission) error {
	return VerifyInvocationsByName(ctx, []string{invName}, permissions...)
}

// Represents a resource, such as root invocation or work unit.
type NamedResource interface {
	// Name returns the resource name of the resource.
	// See also: google.aip.dev/122.
	Name() string
}

// HasPermissionsInRealms checks if the caller has a given set of permissions
// for a collection of invocations, rootInvocations or workunits.
//
// The realms map is from a resource identifier (like invocations.ID) to the
// resource's realm (e.g. "project:realm").
// Returns:
//   - whether the caller has all permissions in all realms
//   - description of the first identified missing permission for an invocation/rootInvocation/workunit.
//     (if applicable)
//   - an error if one occurred
func HasPermissionsInRealms[T interface {
	NamedResource
	comparable
}](ctx context.Context, realms map[T]string, permissions ...realms.Permission) (bool, string, error) {
	checked := stringset.New(1)
	for res, realm := range realms {
		if !checked.Add(realm) {
			continue
		}
		// Note: HasPermission does not make RPCs.
		for _, permission := range permissions {
			switch allowed, err := auth.HasPermission(ctx, permission, realm, nil); {
			case err != nil:
				return false, "", err
			case !allowed:
				return false, fmt.Sprintf(`caller does not have permission %s in realm of %q`, permission, res.Name()), nil
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
