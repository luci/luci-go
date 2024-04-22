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

package gaemiddleware

import (
	"context"
	"net/http"
	"sync"

	"go.opentelemetry.io/otel"
	otelmetricnoop "go.opentelemetry.io/otel/metric/noop"
	oteltracenoop "go.opentelemetry.io/otel/trace/noop"

	"go.chromium.org/luci/gae/filter/dscache"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/filter/readonly"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/common/data/caching/cacheContext"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/settings"
	"go.chromium.org/luci/server/warmup"

	"go.chromium.org/luci/appengine/gaesecrets"
	"go.chromium.org/luci/appengine/gaesettings"
)

// errSimulatedMemcacheOutage is returned by all memcache calls if
// SimulateMemcacheOutage setting is enabled.
var errSimulatedMemcacheOutage = errors.New("simulated memcache outage")

// Environment is a middleware environment. Its parameters define how the
// middleware is applied, and which services are enlisted.
//
// This is low-level API. Use 'gaemiddeware/standard' instead.
type Environment struct {
	// MemcacheAvailable is true if the environment has working memcache.
	//
	// If false, also implies disabled datastore caching layer.
	MemcacheAvailable bool

	// DSReadOnly, if true, causes a read-only datastore layer to be imposed,
	// preventing datastore writes.
	//
	// For any given datastore instance, at most one caching layer may be used.
	// All other instances must be ReadOnly to prevent errant writes from breaking
	// the assumptions of that caching layer. For example, if a Flex VM is being
	// used in conjunction with a non-read-only Classic AppEngine instance.
	DSReadOnly bool

	// DSReadOnlyPredicate returns true for keys that must not be mutated.
	//
	// Effective only when DSReadOnly is true. If nil, all keys are considered
	// read-only.
	DSReadOnlyPredicate readonly.Predicate

	// Prepare will be called once after init() time, but before serving requests.
	//
	// The given context is very bare, use it only for logging and deadlines and
	// stuff like that. It has no other services installed.
	Prepare func(context.Context)

	// WithInitialRequest is called at the very beginning of the handler. It
	// contains a reference to the handler's HTTP request.
	//
	// This should install basic services into the Context, including:
	// - Logging
	// - luci/GAE services.
	WithInitialRequest func(context.Context, *http.Request) context.Context

	// WithConfig is called during service setup to install the "gaeconfig" layer.
	//
	// If nil, no config layer will be installed.
	WithConfig func(context.Context) context.Context

	// WithAuth is called during service setup to install the "gaeauth" layer.
	//
	// If nil, no auth layer will be installed.
	WithAuth func(context.Context) context.Context

	// ExtraMiddleware, if not nil, is additional middleware chain to append to
	// the end of the Base middleware chain to perform per-request monitoring.
	ExtraMiddleware router.MiddlewareChain

	// ExtraHandlers, if not nil, is used to install additional handlers when
	// InstallHandlers is called.
	ExtraHandlers func(r *router.Router, base router.MiddlewareChain)

	prepareOnce sync.Once

	// processCacheData holds all global LRU caches.
	processCacheData *caching.ProcessCacheData
	// globalSettings holds global app settings lazily updated from the datastore.
	globalSettings *settings.Settings
}

// ensurePrepared is called before handling requests to initialize global state.
func (e *Environment) ensurePrepared(ctx context.Context) {
	e.prepareOnce.Do(func() {
		disableOTEL()
		e.processCacheData = caching.NewProcessCacheData()
		e.globalSettings = settings.New(gaesettings.Storage{})
		if e.Prepare != nil {
			e.Prepare(ctx)
		}
	})
}

// disableOTEL is called to install noop OpenTelemetry providers.
//
// We don't use OpenTelemetry on GAEv1. But many libraries (including LUCI ones)
// register OTEL tracers and meters. If we just ignore OTEL completely, this
// results a memory leak. Instead we install noop providers that silently ignore
// everything (instead of accumulating stuff in memory, in expectation that
// a real provider will be installed at some point).
//
// See https://github.com/open-telemetry/opentelemetry-go-contrib/issues/5190.
func disableOTEL() {
	otel.SetTracerProvider(oteltracenoop.TracerProvider{})
	otel.SetMeterProvider(otelmetricnoop.MeterProvider{})
}

// InstallHandlers installs handlers for an Environment's framework routes.
//
// See InstallHandlersWithMiddleware for more information.
func (e *Environment) InstallHandlers(r *router.Router) {
	e.InstallHandlersWithMiddleware(r, e.Base())
}

// InstallHandlersWithMiddleware installs handlers for an Environment's
// framework routes.
//
// In addition to Environment-specific handlers, InstallHandlersWithMiddleware
// installs:
//   - Warmup Handler (warmup)
func (e *Environment) InstallHandlersWithMiddleware(r *router.Router, base router.MiddlewareChain) {
	warmup.InstallHandlersDeprecated(r, base)

	if e.ExtraHandlers != nil {
		e.ExtraHandlers(r, base)
	}
}

// With adds various production GAE LUCI services to the context.
//
// Basically, it installs GAE-specific backends and caches for various
// subsystems to make them work in GAE environment.
//
// One example is a backend for Logging: go.chromium.org/luci/common/logging.
// Logs emitted through a WithProd() context go to GAE logs.
//
// 'Production' here means the services will use real GAE APIs (not mocks or
// stubs), so With should never be used from unit tests.
func (e *Environment) With(ctx context.Context, req *http.Request) context.Context {
	// Set an initial logging level. We'll configure this to be more specific
	// later once we can load settings.
	ctx = logging.SetLevel(ctx, logging.Debug)

	// Ensure one-time initialization happened.
	e.ensurePrepared(ctx)

	// Install process and request LRU caches.
	ctx = caching.WithProcessCacheData(ctx, e.processCacheData)
	ctx = caching.WithRequestCache(ctx)

	ctx = e.WithInitialRequest(ctx, req)

	// A previous layer must have installed a "luci/gae" datastore.
	if datastore.Raw(ctx) == nil {
		panic("no luci/gae datastore is installed")
	}

	// The global cache depends on luci/gae's memcache installed.
	if e.MemcacheAvailable {
		ctx = caching.WithGlobalCache(ctx, blobCacheProvider)
	}

	// These are needed to use fetchCachedSettings.
	ctx = settings.Use(ctx, e.globalSettings)

	// Fetch and apply configuration stored in the datastore.
	cachedSettings := fetchCachedSettings(ctx)
	ctx = logging.SetLevel(ctx, cachedSettings.LoggingLevel)
	if e.MemcacheAvailable {
		if bool(cachedSettings.SimulateMemcacheOutage) {
			logging.Warningf(ctx, "Memcache outage simulation is enabled")
			var fb featureBreaker.FeatureBreaker
			ctx, fb = featureBreaker.FilterMC(ctx, errSimulatedMemcacheOutage)
			fb.BreakFeatures(nil,
				"GetMulti", "AddMulti", "SetMulti", "DeleteMulti",
				"CompareAndSwapMulti", "Flush", "Stats")
		}
		if !bool(cachedSettings.DisableDSCache) {
			ctx = dscache.FilterRDS(ctx, nil)
		}
	}
	if e.DSReadOnly {
		ctx = readonly.FilterRDS(ctx, e.DSReadOnlyPredicate)
	}
	ctx = txndefer.FilterRDS(ctx)

	// The rest of the service may use applied configuration.
	if e.WithConfig != nil {
		ctx = e.WithConfig(ctx)
	}
	ctx = secrets.Use(ctx, gaesecrets.New(nil))
	if e.WithAuth != nil {
		ctx = e.WithAuth(ctx)
	}

	// Wrap this in a cache context so that lookups for any of the aforementioned
	// items are fast.
	return cacheContext.Wrap(ctx)
}

// Base returns a middleware chain to use for all GAE environment
// requests.
//
// Base DOES NOT install "luci/gae" services. To install appropriate services,
// use methods from a sub-package. This is done so that a given AppEngine
// environment doesn't need to include the superset of packages across all
// supported environments.
//
// This middleware chain installs prod GAE services into the request context
// (via With), and wraps the request with a panic catcher and monitoring
// hooks.
func (e *Environment) Base() router.MiddlewareChain {
	// addServices is a middleware that installs the set of standard production
	// AppEngine services by calling With.
	addServices := func(c *router.Context, next router.Handler) {
		// Create a cancelable Context that cancels when this request finishes.
		//
		// We do this because Contexts will leak resources and/or goroutines if they
		// have timers or can be canceled, but aren't. Predictably canceling the
		// parent will ensure that any such resource leaks are cleaned up.
		ctx, cancelFunc := context.WithCancel(c.Request.Context())
		defer cancelFunc()

		// Apply production settings.
		c.Request = c.Request.WithContext(e.With(ctx, c.Request))

		next(c)
	}

	mw := router.NewMiddlewareChain(addServices)
	mw = mw.Extend(e.ExtraMiddleware...)
	return mw
}
