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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/logdog/server/config"
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
		return status.Error(codes.Unauthenticated, "Authentication required.")
	}
	return status.Errorf(codes.PermissionDenied,
		"The resource doesn't exist or you do not have permission to access it.")
}

// CheckPermission checks the caller has the requested permission.
//
// Logs the outcome inside (`prefix` is used only in this logging). Returns
// gRPC errors that can be returned directly to the caller.
func CheckPermission(ctx context.Context, perm realms.Permission, prefix types.StreamName, realm string) error {
	// Log all the details to help debug permission issues.
	ctx = logging.SetFields(ctx, logging.Fields{
		"identity": auth.CurrentIdentity(ctx),
		"perm":     perm,
		"prefix":   prefix,
		"realm":    realm,
	})

	// Check no cross-project mix up is happening as a precaution.
	project := Project(ctx)
	if projInRealm, _ := realms.Split(realm); projInRealm != project {
		logging.Errorf(ctx, "Unexpectedly checking realm %q in a context of project %q", realm, project)
		return status.Error(codes.Internal, "internal server error")
	}

	// Do the realms ACL check.
	switch granted, err := auth.HasPermission(ctx, perm, realm, nil); {
	case err != nil:
		logging.WithError(err).Errorf(ctx, "failed to check realms ACL")
		return status.Error(codes.Internal, "internal server error")
	case granted:
		logging.Debugf(ctx, "Permission granted")
		return nil
	default:
		logging.Warningf(ctx, "Permission denied")
		return PermissionDeniedErr(ctx)
	}
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
