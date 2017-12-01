// Copyright 2017 The LUCI Authors.
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

package model

import (
	"golang.org/x/net/context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
)

// TODO(smut): Move RPC-related functions into their own package.

// isAuthorized returns whether the current user is authorized to use the RPC services.
func isAuthorized(c context.Context) (bool, error) {
	// TODO(smut): Create other groups for this.
	is, err := auth.IsMember(c, "administrators")
	if err != nil {
		return false, errors.Annotate(err, "failed to check group membership").Err()
	}
	return is, err
}

// AuthInterceptor intercepts RPC requests that require authentication, ensuring the requester is authorized.
func AuthInterceptor(c context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	switch authorized, err := isAuthorized(c); {
	case err != nil:
		return nil, InternalRPCError(c, err)
	case !authorized:
		return nil, grpc.Errorf(codes.PermissionDenied, "Unauthorized user")
	}
	return handler(c, req)
}

// Logs and returns an internal gRPC error.
func InternalRPCError(c context.Context, err error) error {
	errors.Log(c, err)
	return grpc.Errorf(codes.Internal, "Internal server error")
}
