// Copyright 2020 The LUCI Authors.
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

// Package gaeemulation provides a server module that adds implementation of
// some https://godoc.org/go.chromium.org/luci/gae APIs to the global server context.
//
// The implementation is based on regular Cloud APIs and works from anywhere
// (not necessarily from Appengine).
//
// Usage:
//
//	func main() {
//	  modules := []module.Module{
//	    gaeemulation.NewModuleFromFlags(),
//	  }
//	  server.Main(nil, modules, func(srv *server.Server) error {
//	    srv.Routes.GET("/", ..., func(c *router.Context) {
//	      ent := Entity{ID: "..."}
//	      err := datastore.Get(c.Context, &ent)
//	      ...
//	    })
//	    return nil
//	  })
//	}
//
// TODO(vadimsh): Currently provides datastore API only.
package gaeemulation

import (
	"context"
	"flag"
	"os"

	"cloud.google.com/go/datastore"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	"go.chromium.org/luci/appengine/gaesecrets"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/filter/dscache"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/cloud"
	"go.chromium.org/luci/grpc/grpcmon"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/secrets"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/gaeemulation")

// ModuleOptions contain configuration of the GAE Emulation server module
type ModuleOptions struct {
	DSCache                  string // currently either "disable" (default) or "redis"
	RandomSecretsInDatastore bool   // true to replace the random secrets store with the GAEv1-one
	DSConnectionPoolSize     int    // passed to WithGRPCConnectionPool, if > 0.
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.DSCache,
		"ds-cache",
		o.DSCache,
		`What datastore caching layer to use ("disable" or "redis").`,
	)
	f.BoolVar(
		&o.RandomSecretsInDatastore,
		"random-secrets-in-datastore",
		o.RandomSecretsInDatastore,
		`If set, use datastore to store random secrets instead of deriving them from a -root-secret. `+
			`Can be used for compatibility with older GAE services. Do not use in new services.`,
	)
	f.IntVar(
		&o.DSConnectionPoolSize,
		"ds-connection-pool-size",
		o.DSConnectionPoolSize,
		"If set, DS client is constructed with WithGRPCConnectionPool() and this value. ",
	)
}

// NewModule returns a server module that adds implementation of
// some https://godoc.org/go.chromium.org/luci/gae APIs to the global server
// context.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &gaeModule{opts: opts}
}

// NewModuleFromFlags is a variant of NewModule that initializes options through
// command line flags.
//
// Calling this function registers flags in flag.CommandLine. They are usually
// parsed in server.Main(...).
func NewModuleFromFlags() module.Module {
	opts := &ModuleOptions{}
	opts.Register(flag.CommandLine)
	return NewModule(opts)
}

// gaeModule implements module.Module.
type gaeModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*gaeModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*gaeModule) Dependencies() []module.Dependency {
	return []module.Dependency{
		module.OptionalDependency(redisconn.ModuleName), // for dscache, if enabled
		module.OptionalDependency(secrets.ModuleName),   // to install DS random secrets backend
	}
}

// Initialize is part of module.Module interface.
func (m *gaeModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	// Use zstd in luci/server's dscache. It will be default at some point.
	dscache.UseZstd = true

	var cacheImpl dscache.Cache
	switch m.opts.DSCache {
	case "", "disable":
		// don't use caching
	case "redis":
		pool := redisconn.GetPool(ctx)
		if pool == nil {
			return nil, errors.New("can't use `-ds-cache redis`: redisconn module is not configured")
		}
		cacheImpl = redisCache{pool: pool}
	default:
		return nil, errors.Fmt("unsupported -ds-cache %q", m.opts.DSCache)
	}

	if m.opts.RandomSecretsInDatastore {
		store, _ := secrets.CurrentStore(ctx).(*secrets.SecretManagerStore)
		if store == nil {
			return nil, errors.Fmt("-random-secrets-in-datastore requires module %q", secrets.ModuleName)
		}
		store.SetRandomSecretsStore(gaesecrets.New(nil))
	}

	if s := m.opts.DSConnectionPoolSize; s < 0 {
		return nil, errors.Fmt("-ds-connection-pool-size: must be >= 0, but %d", s)
	}

	var client *datastore.Client
	if opts.CloudProject != "" {
		var err error
		if client, err = m.initDSClient(ctx, host, opts.CloudProject, m.opts.DSConnectionPoolSize); err != nil {
			return nil, err
		}
	}
	cfg := &cloud.ConfigLite{
		IsDev:     !opts.Prod,
		ProjectID: opts.CloudProject,
		DS:        client, // if nil, datastore calls will fail gracefully(-ish)
	}

	ctx = cfg.Use(ctx)
	if cacheImpl != nil {
		ctx = dscache.FilterRDS(ctx, cacheImpl)
	}
	return txndefer.FilterRDS(ctx), nil
}

// initDSClient sets up Cloud Datastore client that uses AsSelf server token
// source.
func (m *gaeModule) initDSClient(ctx context.Context, host module.Host, cloudProject string, poolSize int) (*datastore.Client, error) {
	logging.Infof(ctx, "Setting up datastore client for project %q", cloudProject)

	// Enable auth only when using the real datastore.
	var clientOpts []option.ClientOption
	if addr := os.Getenv("DATASTORE_EMULATOR_HOST"); addr == "" {
		ts, err := auth.GetTokenSource(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
		if err != nil {
			return nil, errors.Fmt("failed to initialize the token source: %w", err)
		}
		clientOpts = []option.ClientOption{
			option.WithTokenSource(ts),
			option.WithGRPCDialOption(grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{})),
		}
	}

	if poolSize > 0 {
		clientOpts = append(clientOpts, option.WithGRPCConnectionPool(poolSize))
	}
	client, err := datastore.NewClient(ctx, cloudProject, clientOpts...)
	if err != nil {
		return nil, errors.Fmt("failed to instantiate the datastore client: %w", err)
	}

	host.RegisterCleanup(func(ctx context.Context) {
		if err := client.Close(); err != nil {
			logging.Warningf(ctx, "Failed to close the datastore client - %s", err)
		}
	})

	// TODO(vadimsh): "Ping" the datastore to verify the credentials are correct?

	return client, nil
}
