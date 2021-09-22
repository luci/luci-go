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

package impl

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
)

// ServiceAccessGroup defines a group whose members are allowed to use the
// service API and UI.
//
// It nominally grants read access only. Permission to modify individual
// groups is controlled by "owners" group field.
const ServiceAccessGroup = "auth-service-access"

// rpcACL maps gRPC (service, method) to a group that controls access to it.
//
// A method may be `*` to indicate "any method in this service". A group may
// be "public" to indicate the method is publicly accessible (including for
// anonymous unauthenticated access).
var rpcACL = map[serviceMethod]string{
	// Discovery API is used by the RPC Explorer to show the list of APIs. It just
	// returns the proto descriptors already available through the public source
	// code.
	{"discovery.Discovery", "*"}: "public",

	// GetSelf just checks credentials and doesn't access any data.
	{"auth.service.Accounts", "GetSelf"}: "public",

	// All methods to work with groups require authorization.
	{"auth.service.Groups", "*"}: ServiceAccessGroup,

	// All methods to work with allowlists require authorization.
	{"auth.service.Allowlists", "*"}: ServiceAccessGroup,

	// Internals are used by the UI which is accessible only to authorized users.
	{"auth.internals.Internals", "*"}: ServiceAccessGroup,
}

type serviceMethod struct {
	service string
	method  string
}

// AuthorizeRPCAccess is a gRPC unary interceptor that checks the caller is
// in the group that grants access to the auth service API.
func AuthorizeRPCAccess(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	// FullMethod has form "/<service>/<method>".
	parts := strings.Split(info.FullMethod, "/")
	if len(parts) != 3 || parts[0] != "" {
		panic(fmt.Sprintf("unexpected format of infra.FullMethod: %q", info.FullMethod))
	}
	service, method := parts[1], parts[2]

	group := rpcACL[serviceMethod{service, method}]
	if group == "" {
		group = rpcACL[serviceMethod{service, "*"}]
		if group == "" {
			// If you see this error, update rpcACL to include your new API.
			return nil, status.Errorf(codes.PermissionDenied, "calling unrecognized API %q", info.FullMethod)
		}
	}

	if group == "public" {
		return handler(ctx, req)
	}

	switch ok, err := auth.IsMember(ctx, group); {
	case err != nil:
		logging.Errorf(ctx, "Error when checking %q when accessing %q: %s", group, info.FullMethod, err)
		return nil, status.Errorf(codes.Internal, "internal error when checking the ACL")
	case ok:
		return handler(ctx, req)
	default:
		return nil, status.Errorf(codes.PermissionDenied, "%q is not a member of %q", auth.CurrentIdentity(ctx), group)
	}
}
