// Copyright 2015 The LUCI Authors.
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

package coordinator

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/logdog/server/config"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
)

var (
	// PermLogsCreate is a permission required for RegisterPrefix RPC.
	PermLogsCreate = realms.RegisterPermission("logdog.logs.create")
	// PermLogsGet is a permission required for reading individual streams.
	PermLogsGet = realms.RegisterPermission("logdog.logs.get")
	// PermLogsList is a permission required for listing streams in a prefix.
	PermLogsList = realms.RegisterPermission("logdog.logs.list")
)

// PermissionDeniedErr is a generic "doesn't exist or don't have access" error.
//
// If the request is anonymous, it is an Unauthenticated error instead.
func PermissionDeniedErr(ctx context.Context) error {
	if id := auth.CurrentIdentity(ctx); id.Kind() == identity.Anonymous {
		return grpcutil.Unauthenticated
	}
	return grpcutil.Errf(codes.PermissionDenied,
		"The resource doesn't exist or you do not have permission to access it.")
}

// CheckPermission checks the caller has the requested permission.
//
// `realm` can be an empty string when accessing older LogPrefix entities not
// associated with any realms or when RegisterPrefix is called without a realm.
//
// If the project has `enforce_realms_in` setting ON in the "@root" realm, will
// use realms ACLs exclusively. Otherwise the overall ACL is a union of the
// realms ACLs and legacy ACLs. Fallbacks to legacy ACLs are logged.
//
// Logs the outcome inside (`prefix` is used only in this logging). Returns
// gRPC errors that can be returned directly to the caller.
func CheckPermission(ctx context.Context, perm realms.Permission, prefix types.StreamName, realm string) error {
	// Check no cross-project mix up is happening as a precaution.
	project := Project(ctx)
	if realm != "" {
		if projInRealm, _ := realms.Split(realm); projInRealm != project {
			logging.Errorf(ctx, "Unexpectedly checking realm %q in a context of project %q", realm, project)
			return grpcutil.Internal
		}
	}

	// The project as a whole is either in "allow realms, but fallback to old
	// ACLs" or in "enforce realms" modes, depending on configuration stored in
	// the @root realm. We can't just check `realm` here because it can be an
	// empty string.
	enforce, err := auth.ShouldEnforceRealmACL(ctx, realms.Join(project, realms.RootRealm))
	if err != nil {
		logging.WithError(err).Errorf(ctx, "failed to check realms ACL enforcement")
		return grpcutil.Internal
	}

	// When registering a new prefix a realm is required if in the "enforce" mode.
	// Otherwise the missing realm is substituted with "@legacy". That way old
	// LogPrefix entities without "realm" field get some read ACLs, and legacy
	// RegisterPrefix callers that omit "realm" field still hit some realm ACLs
	// before hitting the legacy ones.
	if realm == "" {
		if perm == PermLogsCreate && enforce {
			logging.Fields{
				"identity": auth.CurrentIdentity(ctx),
				"prefix":   prefix,
				"project":  project,
			}.Errorf(ctx, "Missing `realm` in the request")
			return grpcutil.Errf(codes.InvalidArgument, "a realm is required when registering a prefix in project %q", project)
		}
		realm = realms.Join(project, realms.LegacyRealm)
	}

	// Log all the details to help debug permission issues.
	ctx = logging.SetFields(ctx, logging.Fields{
		"identity": auth.CurrentIdentity(ctx),
		"perm":     perm,
		"prefix":   prefix,
		"realm":    realm,
	})

	// Do the realms ACL check.
	granted, err := auth.HasPermission(ctx, perm, realm, nil)
	if err != nil {
		logging.WithError(err).Errorf(ctx, "failed to check realms ACL")
		return grpcutil.Internal
	}

	// If in the enforcement mode, this is all we do. Don't use legacy ACLs.
	// Also if got a positive result, no need to check legacy ACLs at all.
	if enforce || granted {
		if granted {
			logging.Debugf(ctx, "Permission granted")
			return nil
		}
		logging.Warningf(ctx, "Permission denied")
		return PermissionDeniedErr(ctx)
	}

	// On a negative outcome fallback to legacy ACLs, so nothing breaks.
	switch perm {
	case PermLogsCreate:
		granted, err = checkLegacyProjectWriter(ctx)
	case PermLogsGet, PermLogsList:
		granted, err = checkLegacyProjectReader(ctx)
	default:
		panic(fmt.Sprintf("CheckPermission got unexpected permissions %q", perm))
	}
	if err != nil {
		return grpcutil.Internal
	}

	// Log if the permission is granted through the legacy ACL to be able to
	// eventually verify legacy ACLs can be removed. Note that the outcome of
	// the legacy ACL check was already logged by the legacy ACL implementation.
	if granted {
		logging.Warningf(ctx, "crbug.com/1172492: triggered fallback to legacy ACLs")
		return nil
	}

	return PermissionDeniedErr(ctx)
}

// CheckAdminUser tests whether the current user belongs to the administrative
// users group.
//
// Logs the outcome inside. The error is non-nil only if the check itself fails.
func CheckAdminUser(ctx context.Context) (bool, error) {
	cfg, err := config.Config(ctx)
	if err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to load service config")
		return false, err
	}
	return checkMember(ctx, "ADMIN", cfg.Coordinator.AdminAuthGroup)
}

// CheckServiceUser tests whether the current user belongs to the backend
// services users group.
//
// Logs the outcome inside. The error is non-nil only if the check itself fails.
func CheckServiceUser(ctx context.Context) (bool, error) {
	cfg, err := config.Config(ctx)
	if err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to load service config")
		return false, err
	}
	return checkMember(ctx, "SERVICE", cfg.Coordinator.ServiceAuthGroup)
}

// checkLegacyProjectReader tests whether the current user belongs to one of the
// current project's declared reader groups.
//
// Usable only when inside some project namespace, see WithProjectNamespace.
// Panics otherwise.
//
// Logs the outcome inside. The error is non-nil only if the check itself fails.
func checkLegacyProjectReader(ctx context.Context) (bool, error) {
	pcfg, err := ProjectConfig(ctx)
	if err != nil {
		panic("checkLegacyProjectReader is called outside of a project namespace")
	}
	return checkMember(ctx, "READ", pcfg.ReaderAuthGroups...)
}

// checkLegacyProjectWriter tests whether the current user belongs to one of the
// current project's declared writer groups.
//
// Usable only when inside some project namespace, see WithProjectNamespace.
// Panics otherwise.
//
// Logs the outcome inside. The error is non-nil only if the check itself fails.
func checkLegacyProjectWriter(ctx context.Context) (bool, error) {
	pcfg, err := ProjectConfig(ctx)
	if err != nil {
		panic("checkLegacyProjectWriter is called outside of a project namespace")
	}
	return checkMember(ctx, "WRITE", pcfg.WriterAuthGroups...)
}

func checkMember(ctx context.Context, action string, groups ...string) (bool, error) {
	switch yes, err := auth.IsMember(ctx, groups...); {
	case err != nil:
		logging.Fields{
			"identity":       auth.CurrentIdentity(ctx),
			"groups":         groups,
			logging.ErrorKey: err,
		}.Errorf(ctx, "Membership check failed")
		return false, err
	case yes:
		logging.Fields{
			"identity": auth.CurrentIdentity(ctx),
			"groups":   groups,
		}.Debugf(ctx, "User %s access granted.", action)
		return true, nil
	default:
		logging.Fields{
			"identity": auth.CurrentIdentity(ctx),
			"groups":   groups,
		}.Warningf(ctx, "User %s access denied.", action)
		return false, nil
	}
}
