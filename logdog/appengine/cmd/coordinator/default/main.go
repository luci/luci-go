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

// Package module is a simple AppEngine LUCI service. It supplies basic LUCI
// service frontend and backend functionality.
//
// No RPC requests should target this service; instead, they are redirected to
// the appropriate service via "dispatch.yaml".
package module

import (
	"net/http"

	adminPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/admin/v1"
	logsPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	registrationPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/registration/v1"
	servicesPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints/admin"

	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"
)

// Run installs and executes this site.
func init() {
	ps := endpoints.ProdService{}

	r := router.New()

	// Standard HTTP endpoints.
	standard.InstallHandlers(r)

	// Register all of the handlers that we want to show up in RPC explorer (via
	// pRPC discovery).
	//
	// Note that most of these services have dedicated service handlers, and any
	// RPCs sent to this module will automatically be routed to them via
	// "dispatch.yaml".
	svr := &prpc.Server{
		UnaryServerInterceptor: grpcmon.NewUnaryServerInterceptor(nil),
	}
	logsPb.RegisterLogsServer(svr, dummyLogsService)
	registrationPb.RegisterRegistrationServer(svr, dummyRegistrationService)
	servicesPb.RegisterServicesServer(svr, dummyServicesService)
	adminPb.RegisterAdminServer(svr, admin.New())
	discovery.Enable(svr)

	base := standard.Base().Extend(ps.Base)
	svr.InstallHandlers(r, base)

	// Redirect "/" to "/app/".
	r.GET("/", router.MiddlewareChain{}, func(c *router.Context) {
		http.Redirect(c.Writer, c.Request, "/app/", http.StatusFound)
	})
	// Redirect "/v/?s=..." to "/logs/..."
	r.GET("/v/", router.MiddlewareChain{}, func(c *router.Context) {
		path := "/logs/" + c.Request.URL.Query().Get("s")
		http.Redirect(c.Writer, c.Request, path, http.StatusFound)
	})

	http.Handle("/", r)
}
