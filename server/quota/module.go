// Copyright 2022 The LUCI Authors.
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

package quota

import (
	"context"
	"flag"

	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/quota/quotapb"
	"go.chromium.org/luci/server/redisconn"
)

// ModuleName is the globally-unique name for this module.
// Useful for registering this module as a dependency of other modules.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/quota")

// Ensure quotaModule implements server.Module at compile-time.
var _ module.Module = &quotaModule{}

// quotaModule implements module.Module.
type quotaModule struct {
	opts *ModuleOptions
}

var stateKey = "holds a *quotaModuleState"

type quotaModuleState struct {
	pool *redis.Pool
}

// Dependencies returns required and optional dependencies for this module.
// Implements module.Module.
func (*quotaModule) Dependencies() []module.Dependency {
	return []module.Dependency{
		module.RequiredDependency(redisconn.ModuleName),
	}
}

func withRedisConn(ctx context.Context, cb func(redis.Conn) error) (err error) {
	conn, err := redisconn.Get(ctx)
	if err != nil {
		err = errors.Annotate(err, "quota: unable to get redis connection").Err()
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			logging.Errorf(ctx, "quota: unable to close redis connection: %s", err)
		}
	}()
	return cb(conn)
}

// Initialize initializes this module by setting ModuleOptions in the context
// and optionally registering the admin service.
//
// Implements module.Module.
func (m *quotaModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	quotapb.RegisterAdminServer(host, &quotaAdmin{})
	return ctx, nil
}

// Name returns the module.Name for this module.
// Implements module.Module.
func (*quotaModule) Name() module.Name {
	return ModuleName
}

// ModuleOptions is a set of configuration options for the quota module.
type ModuleOptions struct {
	// TODO(iannucci): add option to select alternate database?
}

// Register adds command line flags for these module options to the given
// *flag.FlagSet. Mutates module options by initializing defaults.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
}

// NewModule returns a module.Module for the quota library initialized from the
// given *ModuleOptions.
func NewModule(opts *ModuleOptions) module.Module {
	return &quotaModule{}
}

// NewModuleFromFlags returns a module.Module for the quota library which can be
// initialized from command line flags.
func NewModuleFromFlags() module.Module {
	return NewModule(nil)
}
