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

package rpc

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	milopb "go.chromium.org/luci/milo/proto/v1"

	// Ensure those permissions are registered in memory.
	_ "go.chromium.org/luci/buildbucket/bbperms"
	_ "go.chromium.org/luci/resultdb/rdbperms"
)

// The maximum number of permissions allowed in the BatchCheckPermissions RPC.
const maxPermissions = 100

// BatchCheckPermissions implements milopb.MiloInternal service
func (s *MiloInternalService) BatchCheckPermissions(ctx context.Context, req *milopb.BatchCheckPermissionsRequest) (_ *milopb.BatchCheckPermissionsResponse, err error) {
	// Validate request.
	err = validateBatchCheckPermissionsRequest(req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Convert the permission names to permissions and check whether they exist.
	perms, err := realms.GetPermissions(req.Permissions...)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Check whether the user has each permission.
	results := make(map[string]bool, len(perms))
	for _, perm := range perms {
		allowed, err := auth.HasPermission(ctx, perm, req.GetRealm(), nil)
		if err != nil {
			return nil, err
		}
		results[perm.Name()] = allowed
	}

	return &milopb.BatchCheckPermissionsResponse{Results: results}, nil
}

func validateBatchCheckPermissionsRequest(req *milopb.BatchCheckPermissionsRequest) error {
	if req.GetRealm() == "" {
		return errors.Reason("realm: must be specified").Err()
	}
	if err := realms.ValidateRealmName(req.Realm, realms.GlobalScope); err != nil {
		return errors.Annotate(err, "realm").Err()
	}

	if len(req.GetPermissions()) > maxPermissions {
		return errors.Reason("permissions: at most %d permissions can be specified", maxPermissions).Err()
	}

	return nil
}
