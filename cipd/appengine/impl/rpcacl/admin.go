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

// Package rpcacl contains helpers for checking ACLs of individual RPCs.
package rpcacl

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
)

// AdminGroup is a name of a group with accounts that can use Admin API.
const AdminGroup = "administrators"

// CheckAdmin returns nil if the caller is an administrator.
//
// Returns PermissionDenied gRPC error otherwise. It also logs the admin access.
func CheckAdmin(ctx context.Context) error {
	switch yep, err := auth.IsMember(ctx, AdminGroup); {
	case err != nil:
		logging.WithError(err).Errorf(ctx, "IsMember(%q) failed", AdminGroup)
		return status.Errorf(codes.Internal, "failed to check ACL")
	case !yep:
		logging.Warningf(ctx, "Denying access for %q, not in %q group", auth.CurrentIdentity(ctx), AdminGroup)
		return status.Errorf(codes.PermissionDenied, "not allowed")
	default:
		logging.Infof(ctx, "RPC is called by admin %q", auth.CurrentIdentity(ctx))
		return nil
	}
}
