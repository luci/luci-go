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

// Package frontend implements HTTP server that handles requests to 'default'
// module.
package frontend

import (
	"net/http"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/api/minter/v1"

	"go.chromium.org/luci/tokenserver/appengine/impl/services/admin/adminsrv"
	"go.chromium.org/luci/tokenserver/appengine/impl/services/admin/certauthorities"
	"go.chromium.org/luci/tokenserver/appengine/impl/services/minter/tokenminter"
)

// adminPrelude returns a prelude that authorizes only administrators.
func adminPrelude(serviceName string) func(context.Context, string, proto.Message) (context.Context, error) {
	return func(c context.Context, method string, _ proto.Message) (context.Context, error) {
		logging.Infof(c, "%s: %q is calling %q", serviceName, auth.CurrentIdentity(c), method)
		switch admin, err := auth.IsMember(c, "administrators"); {
		case err != nil:
			return nil, grpc.Errorf(codes.Internal, "can't check ACL - %s", err)
		case !admin:
			return nil, grpc.Errorf(codes.PermissionDenied, "not an admin")
		}
		return c, nil
	}
}

func init() {
	r := router.New()

	// Install auth, config and tsmon handlers.
	gaemiddleware.InstallHandlers(r)

	// The service has no UI, so just redirect to stock RPC explorer.
	r.GET("/", router.MiddlewareChain{}, func(c *router.Context) {
		http.Redirect(c.Writer, c.Request, "/rpcexplorer/", http.StatusFound)
	})

	// Install all RPC servers. Catch panics, report metrics to tsmon (including
	// panics themselves, as Internal errors).
	api := prpc.Server{
		UnaryServerInterceptor: grpcmon.NewUnaryServerInterceptor(grpcutil.NewUnaryServerPanicCatcher(nil)),
	}
	admin.RegisterCertificateAuthoritiesServer(&api, &admin.DecoratedCertificateAuthorities{
		Service: certauthorities.NewServer(),
		Prelude: adminPrelude("admin.CertificateAuthorities"),
	})
	admin.RegisterAdminServer(&api, &admin.DecoratedAdmin{
		Service: adminsrv.NewServer(),
		Prelude: adminPrelude("admin.Admin"),
	})
	minter.RegisterTokenMinterServer(&api, tokenminter.NewServer()) // auth inside
	discovery.Enable(&api)
	api.InstallHandlers(r, gaemiddleware.BaseProd())

	// Expose all this stuff.
	http.DefaultServeMux.Handle("/", r)
}
