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

	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/module"
	pb "go.chromium.org/luci/server/quotabeta/proto"
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

// Dependencies returns required and optional dependencies for this module.
// Implements module.Module.
func (*quotaModule) Dependencies() []module.Dependency {
	return []module.Dependency{
		module.OptionalDependency(cron.ModuleName),
		module.RequiredDependency(redisconn.ModuleName),
	}
}

// Initialize initializes this module by ensuring a quotaconfig.Interface
// implementation is available in the serving context, and installing cron
// routes if configured (see ModuleOptions).
// Implements module.Module.
func (m *quotaModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	// server.Main calls Initialize before calling the user-provided callback where
	// users of the quota library are likely to provide a quotaconfig.Interface.
	// Use a warmup callback, which is called after the user-provided callback has
	// been executed, to run initialization logic requiring quotaconfig.Interface.
	host.RegisterWarmup(func(ctx context.Context) {
		// getInterface panics if a quotaconfig.Interface isn't available (see Use).
		// Intentionally force a panic if one isn't available.
		cfg := getInterface(ctx)

		// Register the cron handler if specified.
		if m.opts.ConfigRefreshCronHandlerID != "" {
			cron.RegisterHandler(m.opts.ConfigRefreshCronHandlerID, func(ctx context.Context) error {
				// Fetch the quotaconfig.Interface from the context each time
				// in case the implementation being used changes later on.
				return getInterface(ctx).Refresh(ctx)
			})
		}

		// Attempt to call Refresh so policy configs are available before serving.
		// This is best-effort, users should ensure they're calling Refresh regularly
		// (either manually, or by setting m.opts.ConfigRefreshCronHandlerID).
		_ = cfg.Refresh(ctx)
	})

	if m.opts.AdminServiceReaders != "" && m.opts.AdminServiceWriters != "" {
		pb.RegisterQuotaAdminServer(host, NewQuotaAdminServer(m.opts.AdminServiceReaders, m.opts.AdminServiceWriters))
	}
	return ctx, nil
}

// Name returns the module.Name for this module.
// Implements module.Module.
func (*quotaModule) Name() module.Name {
	return ModuleName
}

// ModuleOptions is a set of configuration options for the quota module.
type ModuleOptions struct {
	// AdminServerReaders is a Chrome Infra Auth group authorized to use read-only
	// methods of the quota admin pRPC service. If unspecified, the service will
	// not be exposed.
	AdminServiceReaders string

	// AdminServerWriters is a Chrome Infra Auth group authorized to use all
	// methods of the quota admin pRPC service. If unspecified, the service will
	// not be exposed.
	AdminServiceWriters string

	// ConfigRefreshCronHandlerID is the ID for this module's config refresh
	// handler. If specified, the module ensures Refresh is called on the
	// quotaconfig.Interface in the server context. The handler will be installed
	// at <serving-prefix>/<ID>. The serving prefix is controlled by server/cron.
	// If unspecified, module users should refresh policy configs periodically by
	// manually calling Refresh.
	ConfigRefreshCronHandlerID string
}

// Register adds command line flags for these module options to the given
// *flag.FlagSet. Mutates module options by initializing defaults.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(&o.AdminServiceReaders, "quota-admin-service-readers", "", "Chrome Infra Auth group authorized to use read-only admin methods.")
	f.StringVar(&o.AdminServiceWriters, "quota-admin-service-writers", "", "Chrome Infra Auth group authorized to use all admin methods.")
	f.StringVar(&o.ConfigRefreshCronHandlerID, "quota-cron-handler-id", "", "Config refresh handler ID.")
}

// NewModule returns a module.Module for the quota library initialized from the
// given *ModuleOptions.
func NewModule(opts *ModuleOptions) module.Module {
	return &quotaModule{
		opts: opts,
	}
}

// NewModuleFromFlags returns a module.Module for the quota library which can be
// initialized from command line flags.
func NewModuleFromFlags() module.Module {
	opts := &ModuleOptions{}
	opts.Register(flag.CommandLine)
	return NewModule(opts)
}
