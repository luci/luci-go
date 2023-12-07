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

// Package rpcacl implements a gRPC interceptor that checks per-RPC ACLs.
//
// It makes decisions purely based on the name of the called RPC method. It
// doesn't check request messages at all.
//
// This interceptor is useful as the very first coarse ACL check that just
// verifies the caller is known to the service. For simple services, that may be
// the only check. But most services will most likely need to make additional
// checks in the request handler (or another service-specific interceptor) that
// use data from the request message to make service-specific decisions.
package rpcacl

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/server/auth"
)

// Map maps RPC methods to callers that are allowed to call them.
//
// Each key is either "/<service>/<method>" to indicate a single method, or
// "/<service>/*" to indicate all methods in a service.
//
// Values are LUCI group names with authorized callers or following special
// string:
//   - rpcacl.Authenticated: any authenticated caller is authorized.
//   - rpcacl.All: any caller at all is authorized (any authenticated caller and
//     anonymous unauthenticated callers).
//
// Both rpcacl.All and rpcacl.Authenticated imply the method is publicly
// accessible (since it is not hard at all to get an authentication token
// representing *some* account).
//
// Note that group names will be publicly exposed in the PermissionDenied
// response messages. Do not use secret code names in group names.
type Map map[string]string

const (
	// Authenticated represents all authenticated (non-anonymous) callers.
	Authenticated = "!AUTHENTICATED"
	// All represents any caller at all, including anonymous.
	All = "!ALL"
)

// Interceptor returns a server interceptor that checks per-RPC ACLs.
//
// The mapping maps an RPC method to a set of callers that is authorized to
// call it. It should cover all services and methods exposed by the gRPC server.
// Access to undeclared services or methods will be denied with PermissionDenied
// error.
//
// This interceptor implements "static" authorization rules that do not change
// during lifetime of a server process. If you need to adjust ACLs dynamically,
// implement your own interceptor.
//
// Panics if mapping entries are malformed.
func Interceptor(mapping Map) grpcutil.UnifiedServerInterceptor {
	type serviceMethod struct {
		service string // full gRPC service name
		method  string // method name or `*`
	}

	// Copy the map to make sure it doesn't change under us. This configuration is
	// static. Verify magic "!" prefix is used only for known magic entries.
	rpcACL := make(map[serviceMethod]string, len(mapping))
	for key, val := range mapping {
		// key must be "/service/method".
		parts := strings.Split(key, "/")
		if len(parts) != 3 || parts[0] != "" || parts[1] == "" || parts[2] == "" {
			panic(fmt.Sprintf("unexpected key format: %q", key))
		}
		service, method := parts[1], parts[2]

		switch {
		case val == "":
			panic(fmt.Sprintf("for key %q: empty value", key))
		case strings.HasPrefix(val, "!") && val != Authenticated && val != All:
			panic(fmt.Sprintf("for key %q: invalid value %q", key, val))
		}

		rpcACL[serviceMethod{service, method}] = val
	}

	return func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) (err error) {
		// fullMethod has form "/<service>/<method>".
		parts := strings.Split(fullMethod, "/")
		if len(parts) != 3 || parts[0] != "" {
			panic(fmt.Sprintf("unexpected format of full method name: %q", fullMethod))
		}
		service, method := parts[1], parts[2]

		group := rpcACL[serviceMethod{service, method}]
		if group == "" {
			group = rpcACL[serviceMethod{service, "*"}]
			if group == "" {
				// If you see this error update rpcacl.Map to include your new API.
				return status.Errorf(codes.PermissionDenied, "rpcacl: calling unrecognized method %q", fullMethod)
			}
		}

		if group == All {
			return handler(ctx)
		}

		if group == Authenticated {
			if auth.CurrentIdentity(ctx).Kind() == identity.Anonymous {
				return status.Errorf(codes.Unauthenticated, "rpcacl: unauthenticated calls are not allowed")
			}
			return handler(ctx)
		}

		switch ok, err := auth.IsMember(ctx, group); {
		case err != nil:
			logging.Errorf(ctx, "Error when checking %q when accessing %q: %s", group, fullMethod, err)
			return status.Errorf(codes.Internal, "rpcacl: internal error when checking the ACL")
		case ok:
			return handler(ctx)
		default:
			return status.Errorf(codes.PermissionDenied, "rpcacl: %q is not a member of %q", auth.CurrentIdentity(ctx), group)
		}
	}
}
