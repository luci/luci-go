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

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
)

const luciAnalysisAccessGroup = "luci-analysis-access"

// auditUsersAccessGroup is the group which contains people authorised
// to see the details of the user who last created/modified entities.
const auditUsersAccessGroup = "googlers"

const googlerOnlyGroup = "googlers"

const luciAnalysisAdminGroup = "service-luci-analysis-admins"

// Checks if this call is allowed, returns an error if it is.
func checkAllowedPrelude(ctx context.Context, methodName string, req proto.Message) (context.Context, error) {
	if err := checkAllowed(ctx, luciAnalysisAccessGroup); err != nil {
		return ctx, err
	}
	return ctx, nil
}

func checkAllowedAdminPrelude(ctx context.Context, methodName string, req proto.Message) (context.Context, error) {
	if err := checkAllowed(ctx, luciAnalysisAdminGroup); err != nil {
		return ctx, err
	}
	return ctx, nil
}

// Logs and converts the errors to GRPC type errors.
func gRPCifyAndLogPostlude(ctx context.Context, methodName string, rsp proto.Message, err error) error {
	return appstatus.GRPCifyAndLog(ctx, err)
}

func checkAllowed(ctx context.Context, allowedGroup string) error {
	switch yes, err := auth.IsMember(ctx, allowedGroup); {
	case err != nil:
		return errors.Fmt("failed to check ACL: %w", err)
	case !yes:
		return appstatus.Errorf(codes.PermissionDenied, "not a member of %s", allowedGroup)
	default:
		return nil
	}
}
