// Copyright 2016 The LUCI Authors.
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

// Binary frontend implements HTTP server that handles requests to 'default'
// module.
package main

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl"
)

func main() {
	impl.Main(func(srv *server.Server, services *impl.Services) error {
		// Exposed RPC services.
		admin.RegisterCertificateAuthoritiesServer(srv, services.Certs)
		admin.RegisterAdminServer(srv, services.Admin)
		minter.RegisterTokenMinterServer(srv, services.Minter)

		// Authorization check for admin services.
		srv.RegisterUnaryServerInterceptors(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			if strings.HasPrefix(info.FullMethod, "/tokenserver.admin.") {
				logging.Warningf(ctx, "%q is calling %q", auth.CurrentIdentity(ctx), info.FullMethod)
				switch admin, err := auth.IsMember(ctx, "administrators"); {
				case err != nil:
					return nil, status.Errorf(codes.Internal, "can't check ACL - %s", err)
				case !admin:
					return nil, status.Errorf(codes.PermissionDenied, "not an admin")
				}
			}
			return handler(ctx, req)
		})

		// The service has no UI, so just redirect to the RPC Explorer.
		srv.Routes.GET("/", nil, func(c *router.Context) {
			http.Redirect(c.Writer, c.Request, "/rpcexplorer/", http.StatusFound)
		})

		return nil
	})
}
