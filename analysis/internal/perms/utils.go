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

package perms

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
)

var (
	ErrInvalidRealm     = errors.New("realm must be in the format <project>:<realm>")
	ErrMultipleProjects = errors.New("all realms must be from the same projects")
)

// SplitRealm splits the realm into the LUCI project name and the (sub)realm.
// Returns ErrInvalidRealm if the provided realm doesn't have a valid format.
func SplitRealm(realm string) (proj string, subRealm string, err error) {
	parts := strings.SplitN(realm, ":", 2)
	if len(parts) != 2 {
		return "", "", ErrInvalidRealm
	}
	return parts[0], parts[1], nil
}

// SplitRealms splits the realms into the LUCI project name and the (sub)realms.
// All realms must belong to the same project.
//
// Returns ErrInvalidRealm if any of the realm doesn't have a valid format.
// Returns ErrMultipleProjects if not all realms are from the same project.
func SplitRealms(realms []string) (proj string, subRealms []string, err error) {
	if len(realms) == 0 {
		return "", nil, nil
	}

	subRealms = make([]string, 0, len(realms))
	proj, subRealm, err := SplitRealm(realms[0])
	if err != nil {
		return "", nil, ErrInvalidRealm
	}
	subRealms = append(subRealms, subRealm)
	for _, realm := range realms[1:] {
		currentProj, subRealm, err := SplitRealm(realm)
		if err != nil {
			return "", nil, ErrInvalidRealm
		}
		if currentProj != proj {
			return "", nil, ErrMultipleProjects
		}
		subRealms = append(subRealms, subRealm)

	}
	return proj, subRealms, nil
}

// VerifyPermissions is a wrapper around luci/server/auth.HasPermission that checks
// whether the user has all the listed permissions and return an appstatus
// annotated error if users have no permission.
func VerifyPermissions(ctx context.Context, realm string, attrs realms.Attrs, permissions ...realms.Permission) error {
	for _, perm := range permissions {
		allowed, err := auth.HasPermission(ctx, perm, realm, attrs)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s in realm %q`, perm, realm)
		}
	}
	return nil
}

// VerifyProjectPermissions verifies the caller has the given permissions in the
// @project realm of the given project. If the caller does not have permission,
// an appropriate appstatus error is returned, which should be returned
// immediately to the RPC caller.
func VerifyProjectPermissions(ctx context.Context, project string, permissions ...realms.Permission) error {
	realm := realms.Join(project, realms.ProjectRealm)
	for _, p := range permissions {
		allowed, err := HasProjectPermission(ctx, project, p)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s in realm %q`, p, realm)
		}
	}
	return nil
}

// HasProjectPermission returns if the caller has the given permission in
// the @project realm of the given project. This method only returns an error
// if there is some AuthDB issue.
func HasProjectPermission(ctx context.Context, project string, permission realms.Permission) (bool, error) {
	realm := realms.Join(project, realms.ProjectRealm)
	switch allowed, err := auth.HasPermission(ctx, permission, realm, nil); {
	case err != nil:
		return false, err
	case !allowed:
		return false, nil
	}
	return true, nil
}

// QueryRealms is a wrapper around luci/server/auth.QueryRealms that returns a
// list of realms where the current caller has all the listed permissions.
//
// A project is required.
//
// The permissions should be flagged in the process with UsedInQueryRealms
// flag, which lets the runtime know it must prepare indexes for the
// corresponding QueryRealms call.
func QueryRealms(ctx context.Context, project string, attrs realms.Attrs, permissions ...realms.Permission) ([]string, error) {
	if project == "" {
		return nil, errors.New("project must be specified")
	}
	if len(permissions) == 0 {
		return nil, errors.New("at least one permission must be specified")
	}

	allowedRealms, err := auth.QueryRealms(ctx, permissions[0], project, attrs)
	if err != nil {
		return nil, err
	}
	allowedRealmSet := stringset.NewFromSlice(allowedRealms...)

	for _, perm := range permissions[1:] {
		allowedRealms, err := auth.QueryRealms(ctx, perm, project, attrs)
		if err != nil {
			return nil, err
		}
		allowedRealmSet = allowedRealmSet.Intersect(stringset.NewFromSlice(allowedRealms...))
	}

	return allowedRealmSet.ToSortedSlice(), nil
}

// QueryRealmsNonEmpty is similar to QueryRealms but it returns an
// appstatus annotated error if there are no realms
// that the user has all of the given permissions in.
func QueryRealmsNonEmpty(ctx context.Context, project string, attrs realms.Attrs, permissions ...realms.Permission) ([]string, error) {
	realms, err := QueryRealms(ctx, project, attrs, permissions...)
	if err != nil {
		return nil, err
	}
	if len(realms) == 0 {
		return nil, appstatus.Errorf(codes.PermissionDenied, `caller does not have permissions %v in any realm in project %q`, permissions, project)
	}
	return realms, nil
}

// QuerySubRealmsNonEmpty is similar to QueryRealmsNonEmpty with the following differences:
//  1. an optional subRealm argument allows results to be limited to a
//     specific realm (matching `<project>:<subRealm>`).
//  2. a list of subRealms is returned instead of a list of realms
//     (e.g. ["realm1", "realm2"] instead of ["project:realm1", "project:realm2"])
func QuerySubRealmsNonEmpty(ctx context.Context, project, subRealm string, attrs realms.Attrs, permissions ...realms.Permission) ([]string, error) {
	if project == "" {
		return nil, errors.New("project must be specified")
	}
	if len(permissions) == 0 {
		return nil, errors.New("at least one permission must be specified")
	}

	if subRealm != "" {
		realm := project + ":" + subRealm
		if err := VerifyPermissions(ctx, realm, attrs, permissions...); err != nil {
			return nil, err
		}
		return []string{subRealm}, nil
	}

	realms, err := QueryRealmsNonEmpty(ctx, project, attrs, permissions...)
	if err != nil {
		return nil, err
	}
	_, subRealms, err := SplitRealms(realms)
	if err != nil {
		// Realms returned by `QueryRealms` should always be valid.
		// This should never happen.
		panic(err)
	}
	return subRealms, nil
}
