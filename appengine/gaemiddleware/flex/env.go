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

// Package flex exposes gaemiddleware Environments for AppEngine's Flex
// enviornment.
package flex

import (
	"net/http"
	"sync"

	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/mutexpool"
	"go.chromium.org/luci/luci_config/appengine/gaeconfig"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/router"

	authClient "go.chromium.org/luci/appengine/gaeauth/client"
	gaeauth "go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaeauth/server/gaesigner"
	"go.chromium.org/luci/appengine/gaemiddleware"

	"go.chromium.org/gae/impl/cloud"

	"cloud.google.com/go/compute/metadata"

	"golang.org/x/net/context"
)

var (
	// ProcessCache is a process-global LRU cache. It may be shared between
	// multiple subsystems.
	ProcessCache = lru.New(65535)

	// globalInitOnce initializes our global Flex configuration once per
	// process.
	//
	// This initialization requires being run in a Flex enviornment.
	globalInitOnce sync.Once

	// globalFlex is the global luci/gae cloud Flex services definition.
	globalFlex *cloud.Flex

	// globalFlexConfig is a process-wide Flex enviornment configuration.
	globalFlexConfig *cloud.Config

	// globalRequestCounter is a per-instance atomic counter used to differentiate
	// requests from each other.
	globalRequestCounter uint32

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
		AccessTokenProvider: authClient.GetAccessToken,
		AnonymousTransport:  func(context.Context) http.RoundTripper { return http.DefaultTransport },
		Cache:               &auth.MemoryCache{LRU: ProcessCache},
		Locks:               &mutexpool.P{},
		IsDevMode:           !metadata.OnGCE(),
	}
)

func initGlobalFlex() {
	globalInitOnce.Do(func() {
		// Context to use for initialization.
		c := context.Background()

		globalFlex = &cloud.Flex{
			Cache: ProcessCache,
		}

		var err error
		if globalFlexConfig, err = globalFlex.Configure(c); err != nil {
			panic(errors.Annotate(err, "could not create Flex config").Err())
		}
	})
}

// ReadOnlyFlex is an Environment designed for cooperative Flex support
// environments.
var ReadOnlyFlex = gaemiddleware.Environment{
	DSDisableCache: true,
	DSReadOnly:     true,
	WithInitialRequest: func(c context.Context, req *http.Request) context.Context {
		// Install the HTTP inbound request into the Context.
		c = withHTTPRequest(c, req)
		c = caching.WithProcessCache(c, ProcessCache)

		// Install our Cloud services.
		initGlobalFlex()
		flexReq := globalFlex.Request(req)
		c = globalFlexConfig.Use(c, flexReq)

		logging.Infof(c, "Handling request for trace context: %s", flexReq.TraceID)
		return c
	},
	WithConfig: gaeconfig.UseFlex,
	WithAuth: func(c context.Context) context.Context {
		return auth.SetConfig(c, &globalAuthConfig)
	},
	MonitoringMiddleware: nil, // TODO: Add monitoring middleware.
	ExtraHandlers: func(r *router.Router, base router.MiddlewareChain) {
		// Install a handler for basic health checking. We respond with HTTP 200 to
		// indicate that we're always healthy.
		r.GET("/_ah/health", router.MiddlewareChain{},
			func(c *router.Context) { c.Writer.WriteHeader(http.StatusOK) })
	},
}

// WithGlobal returns a Context that is not attached to a specific request.
func WithGlobal(c context.Context) context.Context {
	return ReadOnlyFlex.With(c, &http.Request{})
}
