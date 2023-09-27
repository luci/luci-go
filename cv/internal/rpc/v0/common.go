// Copyright 2021 The LUCI Authors.
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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/cv/internal/acls"
)

// checkCanUseAPI ensures that calling user is granted permission to use
// unstable v0 API.
func checkCanUseAPI(ctx context.Context, name string) error {
	switch yes, err := auth.IsMember(ctx, acls.V0APIAllowGroup); {
	case err != nil:
		return appstatus.Errorf(codes.Internal, "failed to check ACL")
	case !yes:
		return appstatus.Errorf(codes.PermissionDenied, "not a member of %s", acls.V0APIAllowGroup)
	default:
		logging.Debugf(ctx, "%s is calling %s", auth.CurrentIdentity(ctx), name)
		return nil
	}
}
