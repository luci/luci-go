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

// Package frontend implements HTTP server that handles requests to default
// module.
package frontend

import (
	"net/http"
	"strings"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/examples/appengine/helloworld_standard/proto"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
)

// templateBundle is used to render HTML templates. It provides a base args
// passed to all templates.
var templateBundle = &templates.Bundle{
	Loader:    templates.FileSystemLoader("templates"),
	DebugMode: info.IsDevAppServer,
	DefaultArgs: func(c context.Context) (templates.Args, error) {
		loginURL, err := auth.LoginURL(c, "/")
		if err != nil {
			return nil, err
		}
		logoutURL, err := auth.LogoutURL(c, "/")
		if err != nil {
			return nil, err
		}
		isAdmin, err := auth.IsMember(c, "administrators")
		if err != nil {
			return nil, err
		}
		return templates.Args{
			"AppVersion":  strings.Split(info.VersionID(c), ".")[0],
			"IsAnonymous": auth.CurrentIdentity(c) == "anonymous:anonymous",
			"IsAdmin":     isAdmin,
			"User":        auth.CurrentUser(c),
			"LoginURL":    loginURL,
			"LogoutURL":   logoutURL,
		}, nil
	},
}

// pageBase returns the middleware chain for page handlers.
func pageBase() router.MiddlewareChain {
	return gaemiddleware.BaseProd().Extend(
		templates.WithTemplates(templateBundle),
		auth.Authenticate(server.UsersAPIAuthMethod{}),
	)
}

// prpcBase returns the middleware chain for pRPC API handlers.
func prpcBase() router.MiddlewareChain {
	// OAuth 2.0 with email scope is registered as a default authenticator
	// by importing "go.chromium.org/luci/appengine/gaeauth/server".
	// No need to setup an authenticator here.
	//
	// For authorization checks, we use per-service decorators; see
	// service registration code.
	return gaemiddleware.BaseProd()
}

//// Routes.

func checkAPIAccess(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	// Implement authorization check here, for example:
	//
	// import "github.com/golang/protobuf/proto"
	// import "google.golang.org/grpc/codes"
	// import "go.chromium.org/luci/grpc/grpcutil"
	//
	// hasAccess, err := auth.IsMember(c, "my-users")
	// if err != nil {
	//   return nil, grpcutil.Errf(codes.Internal, "%s", err)
	// }
	// if !hasAccess {
	//   return nil, grpcutil.Errf(codes.PermissionDenied, "%s is not allowed to call APIs", auth.CurrentIdentity(c))
	// }

	return c, nil
}

func init() {
	r := router.New()
	gaemiddleware.InstallHandlers(r)
	r.GET("/", pageBase(), indexPage)

	var api prpc.Server
	helloworld.RegisterGreeterServer(&api, &helloworld.DecoratedGreeter{
		Service: &greeterService{},
		Prelude: checkAPIAccess,
	})
	discovery.Enable(&api)
	api.InstallHandlers(r, prpcBase())

	http.DefaultServeMux.Handle("/", r)
}

//// Handlers.

func indexPage(c *router.Context) {
	templates.MustRender(c.Context, c.Writer, "pages/index.html", nil)
}
