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

// Package classic exposes a gaemiddleware Environment for Classic AppEngine.
package standard

import (
	"net/http"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/appengine/gaeauth/client"
	"go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaeauth/server/gaesigner"
	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/appengine/tsmon"
	"go.chromium.org/luci/luci_config/appengine/gaeconfig"

	"go.chromium.org/gae/impl/prod"
	"go.chromium.org/gae/service/urlfetch"

	"google.golang.org/appengine"

	"golang.org/x/net/context"
)

var (
	// globalAuthCache is used to cache various auth token.
	globalAuthCache = &server.Memcache{Namespace: "__luciauth__"}

	// globalAuthConfig is configuration of the server/auth library.
	//
	// It specifies concrete GAE-based implementations for various interfaces
	// used by the library.
	//
	// It is indirectly stateful (since NewDBCache returns a stateful object that
	// keeps AuthDB cache in local memory), and thus it's defined as a long living
	// global variable.
	//
	// Used in prod contexts only.
	globalAuthConfig = auth.Config{
		DBProvider:          authdb.NewDBCache(server.GetAuthDB),
		Signer:              gaesigner.Signer{},
		AccessTokenProvider: client.GetAccessToken,
		AnonymousTransport:  urlfetch.Get,
		Cache:               globalAuthCache,
		IsDevMode:           appengine.IsDevAppServer(),
	}

	// globalTsMonState holds state related to time series monitoring.
	globalTsMonState = &tsmon.State{}
)

// classicEnv is an AppEngine Classic GAE environment configuration. This is the
// default AppEngine environment for simple (all-classic) layouts.
var classicEnv = gaemiddleware.Environment{
	PassthroughPanics: appengine.IsDevAppServer(),
	WithInitialRequest: func(c context.Context, req *http.Request) context.Context {
		// Install our production services.
		return prod.Use(c, req)
	},
	WithConfig: gaeconfig.Use,
	WithAuth: func(c context.Context) context.Context {
		c = auth.ModifyConfig(c, func(auth.Config) auth.Config { return globalAuthConfig })
		return c
	},
	MonitoringMiddleware: globalTsMonState.Middleware,
}

// With adds various production GAE LUCI services to the context.
//
// Basically, it installs GAE-specific backends and caches for various
// subsystems to make them work in GAE environment.
//
// One example is a backend for Logging: go.chromium.org/luci/common/logging.
// Logs emitted through a With() context go to GAE logs.
//
// 'Production' here means the services will use real GAE APIs (not mocks or
// stubs), so With should never be used from unit tests.
func With(c context.Context, req *http.Request) context.Context {
	return classicEnv.With(c, req)
}

// Base returns a middleware chain to use for all GAE requests.
//
// This middleware chain installs prod GAE services into the request context
// (via With), and wraps the request with a panic catcher and monitoring
// hooks.
func Base() router.MiddlewareChain { return classicEnv.Base() }

// InstallHandlers installs handlers for framework routes using classic
// production services.
//
// See gaemiddleware.InstallHandlersWithMiddleware for details.
func InstallHandlers(r *router.Router) {
	gaemiddleware.InstallHandlersWithMiddleware(r, classicEnv.Base())
}
