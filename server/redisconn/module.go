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

package redisconn

import (
	"context"
	"flag"
	"fmt"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"

	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn/adminpb"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/redisconn")

// ModuleOptions contain configuration of the Redis server module.
type ModuleOptions struct {
	RedisAddr string // Redis server to connect to as "host:port"
	RedisDB   int    // index of a logical Redis DB to use by default
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.RedisAddr,
		"redis-addr",
		o.RedisAddr,
		`Redis server to connect to as "host:port" (optional, Redis calls won't work if not given)`,
	)
	f.IntVar(
		&o.RedisDB,
		"redis-db",
		o.RedisDB,
		fmt.Sprintf("Index of a logical Redis DB to use by default (default is %d)", o.RedisDB),
	)
}

// NewModule returns a server module that adds a Redis connection pool to the
// global server context and installs Redis as the default caching.BlobCache
// implementation.
//
// The Redis connection pool can be used through redisconn.Get(ctx).
//
// Does nothing if RedisAddr options is unset. In this case redisconn.Get(ctx)
// returns ErrNotConfigured.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &redisModule{opts: opts}
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

// redisModule implements module.Module.
type redisModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*redisModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*redisModule) Dependencies() []module.Dependency {
	return nil
}

// Initialize is part of module.Module interface.
func (m *redisModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	if m.opts.RedisAddr == "" {
		return ctx, nil
	}

	pool := NewPool(m.opts.RedisAddr, m.opts.RedisDB)
	ctx = UsePool(ctx, pool)

	// Use Redis as caching.BlobCache provider.
	ctx = caching.WithGlobalCache(ctx, func(namespace string) caching.BlobCache {
		return &redisBlobCache{Prefix: fmt.Sprintf("luci.blobcache.%s:", namespace)}
	})

	// Close all connections when exiting gracefully.
	host.RegisterCleanup(func(ctx context.Context) {
		if err := pool.Close(); err != nil {
			logging.Warningf(ctx, "Failed to close Redis pool - %s", err)
		}
	})

	// Populate pool metrics on tsmon flush.
	tsmon.RegisterCallbackIn(ctx, func(ctx context.Context) {
		ReportStats(ctx, pool, "default")
	})

	// Expose an admin API that can be used to e.g. flush Redis DB. This is
	// especially useful on GAE where reaching Redis otherwise requires launching
	// a VM to get into the private network.
	adminpb.RegisterAdminServer(host, &adminServer{pool: pool})

	return ctx, nil
}
