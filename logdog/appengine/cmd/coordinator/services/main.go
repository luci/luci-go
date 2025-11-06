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

package main

import (
	"net/http"

	"google.golang.org/appengine"

	gaeserver "go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	registrationPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/registration/v1"
	servicesPb "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints/registration"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints/services"
	"go.chromium.org/luci/logdog/server/config"
)

// Run installs and executes this site.
func main() {
	// Disable cookie-based authentication for this service.
	// It should not be used as all RPCs served by this instance are
	// made by Logdog backend. Moreover, the default implementation retains
	// user email addresses in datastore indefinitely which is not acceptable
	// for privacy reasons.
	gaeserver.DisableCookieAuth()

	r := router.New()

	// Setup Cloud Endpoints.
	svr := prpc.Server{
		UnaryServerInterceptor: grpcutil.ChainUnaryServerInterceptors(
			grpcmon.UnaryServerInterceptor,
			auth.AuthenticatingInterceptor([]auth.Method{
				&gaeserver.OAuth2Method{Scopes: []string{scopes.Email}},
			}).Unary(),
		),
	}
	servicesPb.RegisterServicesServer(&svr, services.New(services.ServerSettings{
		NumQueues: 10,
	}))
	registrationPb.RegisterRegistrationServer(&svr, registration.New())

	// Standard HTTP endpoints.
	base := standard.Base().Extend(config.Middleware(&config.Store{}))
	svr.InstallHandlers(r, base)
	standard.InstallHandlers(r)

	http.Handle("/", r)
	appengine.Main()
}
