// Copyright 2016 The LUCI Authors.
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

package gaeconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.chromium.org/luci/appengine/datastorecache"
	"go.chromium.org/luci/common/config/impl/filesystem"
	"go.chromium.org/luci/luci_config/appengine/backend/datastore"
	"go.chromium.org/luci/luci_config/appengine/backend/memcache"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/caching"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/client"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/erroring"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/format"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/testconfig"
	serverCaching "go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/gae/service/info"

	"golang.org/x/net/context"

	// Import to register the corresponding formatter.
	_ "go.chromium.org/luci/luci_config/server/cfgclient/textproto"
)

// ErrNotConfigured is returned by cfgclient methods if the config service URL
// is not set. Usually happens for new apps.
var ErrNotConfigured = errors.New("config service URL is not set in settings")

const (
	// devCfgDir is a name of the directory with config files when running in
	// local dev appserver model. See New for details.
	devCfgDir = "devcfg"
)

// cacheConfig defines how a LUCI Config backend handles caching.
type cacheConfig struct {
	// GlobalCache, if not nil, installs a cache layer immediately after the
	// remote configuration service and caches its responses.
	GlobalCache func(c context.Context, b backend.B, s *Settings) backend.B

	// With, if not nil, is called immediately after service caching has been
	// configured, with the service backend. It can be used to augment the
	// Context.
	With func(c context.Context, be backend.B, s *Settings) context.Context

	// DataCache, if not nil, installs a cache layer immediately after a
	// formatting cache layer has been installed. This can be used to buffer
	// accesses to the underlying remote service.
	DataCache func(c context.Context, b backend.B, s *Settings) backend.B
}

// InstallCacheCronHandler installs the configuration service datastore caching
// cron handler. This must be installed, and an associated cron must be set up,
// if datastore caching is enabled.
//
// The cron should be configured to hit the handler at:
// /admin/config/cache/manager
func InstallCacheCronHandler(r *router.Router, base router.MiddlewareChain) {
	installCacheCronHandlerImpl(r, base, nil)
}

// Install our cache cron handler into the supplied Router.
//
// bf is a generator function used to get the primary (service) Backend to build
// on top of. If nil, the "production" (one used by Use) Backend will be used.
func installCacheCronHandlerImpl(r *router.Router, base router.MiddlewareChain, be backend.B) {
	base = base.Extend(func(c *router.Context, next router.Handler) {
		// Install our Backend into our Context.
		c.Context = installConfigBackend(c.Context, mustFetchCachedSettings(c.Context), be, cacheConfig{
			// Use memcache to buffer raw service requests.
			GlobalCache: func(c context.Context, be backend.B, s *Settings) backend.B {
				return memcache.Backend(be, s.CacheExpiration())
			},

			// For cron, do not install the datastore cache Backend. Instead,
			// install a lasting Handler into the Context to be used for cache
			// resolution. This is necessary since resolution calls will not be
			// the result of an actual resolution command (e.g., cache.Get).
			With: func(c context.Context, be backend.B, s *Settings) context.Context {
				dsc := datastoreCacheConfig(s)
				if dsc != nil {
					c = dsc.WithHandler(c, datastore.CronLoader(be), datastore.RPCDeadline)
				}
				return c
			},
		})
		next(c)
	})

	datastore.Cache.InstallCronRoute("/admin/config/cache/manager", r, base)
}

// Use installs the default luci-config client.
//
// The client is configured to use luci-config URL specified in the settings,
// using GAE app service account for authentication.
//
// If running in prod, and the settings don't specify luci-config URL, produces
// an implementation of backend.B that returns ErrNotConfigured from all
// methods.
//
// If running on devserver, and the settings don't specify luci-config URL,
// returns a filesystem-based implementation that reads configs from a directory
// (or a symlink) named 'devcfg' located in the GAE module directory (where
// app.yaml is) or its immediate parent directory.
//
// If such directory can not be located, produces an implementation of
// cfgclient that returns errors from all methods.
//
// Panics if it can't load the settings (should not happen since they are in
// the local memory cache usually).
func Use(c context.Context) context.Context { return useImpl(c, nil) }

func useImpl(c context.Context, be backend.B) context.Context {
	return installConfigBackend(c, mustFetchCachedSettings(c), be, cacheConfig{
		// Use memcache to buffer raw service requests.
		GlobalCache: func(c context.Context, be backend.B, s *Settings) backend.B {
			return memcache.Backend(be, s.CacheExpiration())
		},

		// Install datastore caching layer.
		DataCache: func(c context.Context, be backend.B, s *Settings) backend.B {
			if dsc := datastoreCacheConfig(s); dsc != nil {
				be = dsc.Backend(be)
			}

			// Cache configurations in memory for local consistency. This will cache
			// individual configuration perspectives for individual users.
			be = caching.LRUBackend(be, serverCaching.ProcessCache(c), s.CacheExpiration())

			return be
		},
	})
}

// UseFlex installs the default luci-config client for an AppEngine Flex
// environment.
//
// UseFlex has the same effect as Use, except that the backing cache is
// a process-local cache instead of the AppEngine memcache service.
//
// UseFlex may optionally supply an LRUcache to use for process-wide
// configuration caching. If nil, no process-wide caching will be performed.
func UseFlex(c context.Context) context.Context {
	// TODO: Install a path to load configurations from datastore cache. ATM,
	// it can't be done because using datastore cache requires the ability to
	// write to datastore.
	ccfg := cacheConfig{
		GlobalCache: func(c context.Context, be backend.B, s *Settings) backend.B {
			return caching.LRUBackend(be, serverCaching.ProcessCache(c), s.CacheExpiration())
		},
	}
	return installConfigBackend(c, mustFetchCachedSettings(c), nil, ccfg)
}

// installConfigBackend chooses a primary backend, then reenforces it with
// caches based on the configuration.
//
// The primary backend is a live LUCI config service if configured, a permanent
// error service if not configured, or a debug service if running on a
// development server.
//
// If caching is enabled, this is re-enforced with the following caches:
// - A memcache-backed cache.
// - A datastore-backed cache (if enabled).
// - A per-process in-memory cache (LRU).
//
// Lookups move from the bottom of the list up through the top, then to the
// primary backend service.
func installConfigBackend(c context.Context, s *Settings, be backend.B, ccfg cacheConfig) context.Context {
	if be == nil {
		// Non-testing, build a Backend.
		be = getPrimaryBackend(c, s)
	}

	// Apply caching configuration.
	if exp := s.CacheExpiration(); exp > 0 {
		// Back the raw service with a global cache.
		if ccfg.GlobalCache != nil {
			be = ccfg.GlobalCache(c, be, s)
		}

		// Add a formatting Backend. All Backends after this will use the formatted
		// version of the entry.
		be = &format.Backend{B: be}

		// After raw service, install data-level caching.
		if ccfg.With != nil {
			c = ccfg.With(c, be, s)
		}

		if ccfg.DataCache != nil {
			be = ccfg.DataCache(c, be, s)
		}
	}

	c = backend.WithBackend(c, be)
	return c
}

func datastoreCacheConfig(s *Settings) *datastore.Config {
	if s.DatastoreCacheMode == DSCacheDisabled {
		return nil
	}
	return &datastore.Config{
		RefreshInterval: s.CacheExpiration(),
		FailOpen:        true,
		LockerFunc:      datastorecache.MemLocker,
	}
}

func getPrimaryBackend(c context.Context, settings *Settings) backend.B {
	// Identify our config service backend (in testing, it will be supplied).
	if settings.ConfigServiceHost == "" {
		if info.IsDevAppServer(c) {
			return devServerBackend()
		}
		return erroring.New(ErrNotConfigured)
	}
	return &client.Backend{&client.RemoteProvider{
		Host:     settings.ConfigServiceHost,
		Insecure: false,
	}}
}

// devServerConfig returns backend.B to use on a devserver.
//
// See New for details.
func devServerBackend() backend.B {
	pwd := os.Getenv("PWD") // os.Getwd works funny with symlinks, use PWD
	candidates := []string{
		filepath.Join(pwd, devCfgDir),
		filepath.Join(filepath.Dir(pwd), devCfgDir),
	}
	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			fs, err := filesystem.New(dir)
			if err != nil {
				return erroring.New(err)
			}
			return &client.Backend{&testconfig.Provider{
				Base: fs,
			}}
		}
	}
	return erroring.New(fmt.Errorf("luci-config: could not find local configs in any of %s", candidates))
}
