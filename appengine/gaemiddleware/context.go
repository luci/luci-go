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

package gaemiddleware

import (
	"net/http"

	"golang.org/x/net/context"
	"google.golang.org/appengine"

	"github.com/luci/gae/filter/dscache"
	"github.com/luci/gae/impl/prod"
	"github.com/luci/gae/service/urlfetch"

	"github.com/luci/luci-go/common/data/caching/cacheContext"
	"github.com/luci/luci-go/common/data/caching/proccache"
	"github.com/luci/luci-go/common/logging"

	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authdb"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/settings"

	"github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaeauth/server/gaesigner"
	"github.com/luci/luci-go/appengine/gaesecrets"
	"github.com/luci/luci-go/appengine/gaesettings"
	"github.com/luci/luci-go/appengine/tsmon"
	"github.com/luci/luci-go/luci_config/appengine/gaeconfig"
)

var (
	// globalProcessCache holds state cached between requests.
	globalProcessCache = &proccache.Cache{}

	// globalSettings holds global app settings lazily updated from the datastore.
	globalSettings = settings.New(gaesettings.Storage{})

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

// WithProd adds various production GAE LUCI services to the context.
//
// Basically, it installs GAE-specific backends and caches for various
// subsystems to make them work in GAE environment.
//
// One example is a backend for Logging: github.com/luci/luci-go/common/logging.
// Logs emitted through a WithProd() context go to GAE logs.
//
// 'Production' here means the services will use real GAE APIs (not mocks or
// stubs), so WithProd should never be used from unit tests.
func WithProd(c context.Context, req *http.Request) context.Context {
	// These are needed to use fetchCachedSettings.
	c = logging.SetLevel(c, logging.Debug)
	c = prod.Use(c, req)
	c = settings.Use(c, globalSettings)

	// Fetch and apply configuration stored in the datastore.
	cachedSettings := fetchCachedSettings(c)
	c = logging.SetLevel(c, cachedSettings.LoggingLevel)
	if !cachedSettings.DisableDSCache {
		c = dscache.AlwaysFilterRDS(c)
	}

	// The rest of the service may use applied configuration.
	c = proccache.Use(c, globalProcessCache)
	c = gaeconfig.Use(c)
	c = gaesecrets.Use(c, nil)
	c = globalAuthCache.UseRequestCache(c)
	c = auth.ModifyConfig(c, func(auth.Config) auth.Config { return globalAuthConfig })

	// Wrap this in a cache context so that lookups for any of the aforementioned
	// items are fast.
	return cacheContext.Wrap(c)
}

// BaseProd returns a middleware chain to use for all GAE requests.
//
// This middleware chain installs prod GAE services into the request context
// (via WithProd), and wraps the request with a panic catcher and monitoring
// hooks.
func BaseProd() router.MiddlewareChain {
	if appengine.IsDevAppServer() {
		return router.NewMiddlewareChain(prodServices, globalTsMonState.Middleware)
	}
	return router.NewMiddlewareChain(
		prodServices, middleware.WithPanicCatcher, globalTsMonState.Middleware,
	)
}

// prodServices is a middleware that installs the set of standard production
// AppEngine services by calling WithProd.
func prodServices(c *router.Context, next router.Handler) {
	// Create a cancelable Context that cancels when this request finishes.
	//
	// We do this because Contexts will leak resources and/or goroutines if they
	// have timers or can be canceled, but aren't. Predictably canceling the
	// parent will ensure that any such resource leaks are cleaned up.
	var cancelFunc context.CancelFunc
	c.Context, cancelFunc = context.WithCancel(c.Context)
	defer cancelFunc()

	// Apply production settings.
	c.Context = WithProd(c.Context, c.Request)

	next(c)
}
