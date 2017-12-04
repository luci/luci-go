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
	"strings"

	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/luci_config/appengine/gaeconfig"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/pprof"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tsmon"

	authClient "go.chromium.org/luci/appengine/gaeauth/client"
	gaeauth "go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/gaeauth/server/gaesigner"
	"go.chromium.org/luci/appengine/gaemiddleware"
	gaetsmon "go.chromium.org/luci/appengine/tsmon"

	"go.chromium.org/gae/impl/cloud"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"

	"cloud.google.com/go/compute/metadata"

	"golang.org/x/net/context"
)

var (
	// ListeningAddr is a address to bind the listening socket to.
	ListeningAddr string

	// globalFlex is the global luci/gae cloud Flex services definition.
	globalFlex *cloud.Flex

	// globalFlexConfig is a process-wide Flex enviornment configuration.
	globalFlexConfig *cloud.Config

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
		IsDevMode:           !metadata.OnGCE(),
	}

	// globalTsMonState holds configuration and state related to time series
	// monitoring.
	//
	// TODO(vadimsh): We can flush asynchronously on Flex. Unfortunately, the
	// request context is canceled as soon as the request ends, killing all
	// outliving goroutines. We either need a way to detach it, or run the flush
	// using the global context (the logging is moot in this case, since we'll
	// have no trace ID for logs).
	globalTsMonState = &tsmon.State{
		IsDevMode: !metadata.OnGCE(),
		Target: func(c context.Context) target.Task {
			return target.Task{
				DataCenter:  "appengine",
				ServiceName: info.AppID(c),
				JobName:     info.ModuleName(c),
				HostName:    strings.SplitN(info.VersionID(c), ".", 2)[0],
			}
		},
		InstanceID:       info.InstanceID,
		TaskNumAllocator: gaetsmon.DatastoreTaskNumAllocator{},
	}
)

func init() {
	if metadata.OnGCE() {
		ListeningAddr = ":8080" // assume real Flex
	} else {
		ListeningAddr = "127.0.0.1:8080" // dev environment
	}
}

// ReadOnlyFlex is an Environment designed for cooperative Flex support
// environments.
var ReadOnlyFlex = gaemiddleware.Environment{
	MemcacheAvailable: false,
	DSReadOnly:        true,
	DSReadOnlyPredicate: func(k *datastore.Key) (isRO bool) {
		// HACK(vadimsh): This is needed to allow tsmon middleware to bypass
		// read-only filter on the datastore. It needs writable access to the
		// datastore for DatastoreTaskNumAllocator to work. It doesn't rely on
		// dscache, and thus it is safe to mutate datastore from Flex side.
		return k.Namespace() != gaetsmon.DatastoreNamespace
	},
	Prepare: func(c context.Context) {
		globalFlex = &cloud.Flex{
			Cache: lru.New(65535),
		}
		var err error
		if globalFlexConfig, err = globalFlex.Configure(c); err != nil {
			panic(errors.Annotate(err, "could not create Flex config").Err())
		}
	},
	WithInitialRequest: func(c context.Context, req *http.Request) context.Context {
		// Install the HTTP inbound request into the Context.
		c = withHTTPRequest(c, req)
		// Install our Cloud services.
		return globalFlexConfig.Use(c, globalFlex.Request(req))
	},
	WithConfig: gaeconfig.UseFlex,
	WithAuth: func(c context.Context) context.Context {
		return auth.SetConfig(c, &globalAuthConfig)
	},
	MonitoringMiddleware: globalTsMonState.Middleware,
	ExtraHandlers: func(r *router.Router, base router.MiddlewareChain) {
		pprof.InstallHandlers(r, base)
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
