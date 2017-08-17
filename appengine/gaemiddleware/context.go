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

	"go.chromium.org/gae/filter/dscache"
	"go.chromium.org/gae/impl/prod"
	"go.chromium.org/gae/service/urlfetch"

	"go.chromium.org/luci/common/data/caching/cacheContext"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/data/caching/proccache"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/settings"

	"go.chromium.org/luci/appengine/gaeauth/client"
	"go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaeauth/server/gaesigner"
	"go.chromium.org/luci/appengine/gaesecrets"
	"go.chromium.org/luci/appengine/gaesettings"
	"go.chromium.org/luci/appengine/tsmon"
	"go.chromium.org/luci/luci_config/appengine/gaeconfig"
)

var (
	// globalProcessCache holds state cached between requests.
	globalProcessCache = &proccache.Cache{}

	// globalLRUCache holds state cached between requests.
	//
	// TODO: We should choose a maximum LRU size here to stop this from growing
	//   indefinitely. However, it's replacing an indefinitely-growing cache, so
	//   for now this is acceptable.
	// TODO: This should replace uses of globalProcessCache.
	globalLRUCache = lru.New(0)

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
// One example is a backend for Logging: go.chromium.org/luci/common/logging.
// Logs emitted through a WithProd() context go to GAE logs.
//
// 'Production' here means the services will use real GAE APIs (not mocks or
// stubs), so WithProd should never be used from unit tests.
func WithProd(c context.Context, req *http.Request) context.Context {
	// These are needed to use fetchCachedSettings.
	c = logging.SetLevel(c, logging.Debug)
	c = caching.WithProcessCache(c, globalLRUCache)
	c = caching.WithRequestCache(c)
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
