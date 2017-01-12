// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gaeconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/luci/luci-go/common/config/impl/filesystem"
	"github.com/luci/luci-go/luci_config/appengine/backend/datastore"
	"github.com/luci/luci-go/luci_config/appengine/backend/memcache"
	gaeformat "github.com/luci/luci-go/luci_config/appengine/format"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/caching"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/client"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/erroring"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/format"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/testconfig"
	"github.com/luci/luci-go/server/router"

	"github.com/luci/gae/service/info"

	"golang.org/x/net/context"
)

// ErrNotConfigured is returned by cfgclient methods if the config service URL
// is not set. Usually happens for new apps.
var ErrNotConfigured = errors.New("config service URL is not set in settings")

// devCfgDir is a name of the directory with config files when running in
// local dev appserver model. See New for details.
const devCfgDir = "devcfg"

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
		c.Context = installConfigBackend(c.Context, mustFetchCachedSettings(c.Context), be, true)
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
	return installConfigBackend(c, mustFetchCachedSettings(c), be, false)
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
// - A per-process in-memory cache (proccache).
//
// Lookups move from the bottom of the list up through the top, then to the
// primary backend service.
func installConfigBackend(c context.Context, s *Settings, be backend.B, dsCron bool) context.Context {
	if be == nil {
		// Non-testing, build a Backend.
		be = getPrimaryBackend(c, s)
	}

	// Install a FormatRegistry. Register common config service protobufs with it.
	c = gaeformat.WithRegistry(c, gaeformat.Default())

	be = &format.Backend{
		B:           be,
		GetRegistry: gaeformat.GetRegistry,
	}

	// Apply caching configuration.
	exp := time.Duration(s.CacheExpirationSec) * time.Second
	if exp > 0 {
		// Back the raw service with memcache.
		be = memcache.Backend(be, exp)

		// If our datastore cache is enabled, install a handler for refresh. This
		// will be loaded by dsCache's "HandlerFunc".
		if s.DatastoreCacheMode != dsCacheDisabled {
			dsc := datastore.Config{
				RefreshInterval: exp,
				FailOpen:        s.DatastoreCacheMode == dsCacheEnabled,
			}

			if !dsCron {
				// For non-cron, install the datastore cache Backend.
				be = dsc.Backend(be)
			} else {
				// For cron, do not install the datastore cache Backend. Instead,
				// install a lasting Handler into the Context to be used for cache
				// resolution. This is necessary since resolution calls will not be
				// the result of an actual resolution command (e.g., cache.Get).
				c = dsc.WithHandler(c, datastore.CronLoader(be), datastore.RPCDeadline)
			}
		}

		// Install in-memory cache (proccache).
		be = caching.ProcCache(be, exp)
	}

	c = backend.WithBackend(c, be)
	return c
}

func getPrimaryBackend(c context.Context, settings *Settings) backend.B {
	// Identify our config service backend (in testing, it will be supplied).
	if settings.ConfigServiceURL == "" {
		if info.IsDevAppServer(c) {
			return devServerBackend()
		}
		return erroring.New(ErrNotConfigured)
	}
	return &client.Backend{&client.RemoteProvider{
		BaseURL: settings.ConfigServiceURL,
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
