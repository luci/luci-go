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

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
	"go.chromium.org/luci/logdog/server/config"
	"go.chromium.org/luci/server/auth"
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

// CheckProjectReader tests whether the current user belongs to one of the
// project's declared reader groups.
//
// Logs the outcome inside. The error is non-nil only if the check itself fails.
func CheckProjectReader(ctx context.Context, pcfg *svcconfig.ProjectConfig) (bool, error) {
	return checkMember(ctx, "READ", pcfg.ReaderAuthGroups...)
}

// CheckProjectWriter tests whether the current user belongs to one of the
// project's declared writer groups.
//
// Logs the outcome inside. The error is non-nil only if the check itself fails.
func CheckProjectWriter(ctx context.Context, pcfg *svcconfig.ProjectConfig) (bool, error) {
	return checkMember(ctx, "WRITE", pcfg.WriterAuthGroups...)
}

func checkMember(ctx context.Context, action string, groups ...string) (bool, error) {
	// On dev-appserver, the superuser has implicit group membership to
	// everything.
	if info.IsDevAppServer(ctx) {
		if u := auth.CurrentUser(ctx); u.Superuser {
			logging.Fields{
				"identity": u.Identity,
				"groups":   groups,
			}.Infof(ctx, "Granting superuser implicit group membership for %s on development server.", action)
			return true, nil
		}
	}
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
