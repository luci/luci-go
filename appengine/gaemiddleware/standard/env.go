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

// Package standard exposes a gaemiddleware Environment for Classic AppEngine.
package standard

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/appengine"

	"go.chromium.org/luci/appengine/gaeauth/client"
	gaeauth "go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaeauth/server/gaesigner"
	"go.chromium.org/luci/appengine/gaemiddleware"
	gaetsmon "go.chromium.org/luci/appengine/tsmon"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/config/appengine/gaeconfig"
	"go.chromium.org/luci/gae/impl/prod"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/portal"
	"go.chromium.org/luci/server/pprof"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tsmon"
)

var (
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
		DBProvider:          authdb.NewDBCache(gaeauth.GetAuthDB),
		Signer:              gaesigner.Signer{},
		AccessTokenProvider: client.GetAccessToken,
		AnonymousTransport: func(ctx context.Context) http.RoundTripper {
			return &contextAwareURLFetch{ctx}
		},
		FrontendClientID: gaeauth.FetchFrontendClientID,
		IsDevMode:        appengine.IsDevAppServer(),
	}

	// globalTsMonState holds configuration and state related to time series
	// monitoring.
	globalTsMonState = &tsmon.State{
		CustomMonitor: tsmonDebugMonitor(),
		Target: func(ctx context.Context) target.Task {
			return target.Task{
				DataCenter:  "appengine",
				ServiceName: info.AppID(ctx),
				JobName:     info.ModuleName(ctx),
				HostName:    strings.SplitN(info.VersionID(ctx), ".", 2)[0],
			}
		},
		InstanceID:        info.InstanceID,
		TaskNumAllocator:  gaetsmon.DatastoreTaskNumAllocator{},
		FlushInMiddleware: true,
	}
)

// tsmonDebugMonitor returns debug monitor when running on dev server.
func tsmonDebugMonitor() monitor.Monitor {
	if appengine.IsDevAppServer() {
		return monitor.NewDebugMonitor("")
	}
	return nil
}

// classicEnv is an AppEngine Classic GAE environment configuration. This is the
// default AppEngine environment for simple (all-classic) layouts.
var classicEnv = gaemiddleware.Environment{
	MemcacheAvailable:  true,
	WithInitialRequest: prod.Use,
	WithConfig:         gaeconfig.Use,
	WithAuth: func(ctx context.Context) context.Context {
		return auth.Initialize(ctx, &globalAuthConfig)
	},

	ExtraMiddleware: func() router.MiddlewareChain {
		mw := make([]router.Middleware, 0, 2)
		if !appengine.IsDevAppServer() {
			mw = append(mw, middleware.WithPanicCatcher)
		}
		mw = append(mw, globalTsMonState.Middleware)
		return router.NewMiddlewareChain(mw...)
	}(),

	ExtraHandlers: func(r *router.Router, base router.MiddlewareChain) {
		gaeauth.InstallHandlers(r, base)
		gaetsmon.InstallHandlers(r, base)
		portal.InstallHandlers(r, base, &gaeauth.UsersAPIAuthMethod{})
		pprof.InstallHandlers(r, base)
	},
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
func With(ctx context.Context, req *http.Request) context.Context {
	return classicEnv.With(ctx, req)
}

// Base returns a middleware chain to use for all GAE requests.
//
// This middleware chain installs prod GAE services into the request context
// (via With), and wraps the request with a panic catcher and monitoring
// hooks.
func Base() router.MiddlewareChain { return classicEnv.Base() }

// InstallHandlers installs handlers for framework routes using classic
// production services' default middleware.
//
// See InstallHandlersWithMiddleware for more information.
func InstallHandlers(r *router.Router) { classicEnv.InstallHandlers(r) }

// InstallHandlersWithMiddleware installs handlers for framework routes using
// classic production services.
//
// These routes are needed for various services provided in Base context to
// work:
//   - Authentication related routes (gaeauth)
//   - Settings pages (gaesettings)
//   - Various housekeeping crons (tsmon, gaeconfig)
//   - Warmup (warmup)
//
// They must be installed into a default module, but it is also safe to
// install them into a non-default module. This may be handy if you want to
// move cron handlers into a non-default module.
//
// 'base' is expected to be an Environment's Base() or its derivative. It must
// NOT do any interception of requests (e.g. checking and rejecting
// unauthenticated requests).
func InstallHandlersWithMiddleware(r *router.Router, base router.MiddlewareChain) {
	classicEnv.InstallHandlersWithMiddleware(r, base)
}
