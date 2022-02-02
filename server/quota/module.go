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
	"go.chromium.org/luci/server/redisconn"

	"go.chromium.org/luci/server/quota/quotaconfig"
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

// Initialize initializes this module by installing cron routes if configured (see ModuleOptions).
// Implements module.Module
func (m *quotaModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	// TODO(crbug/1280055): Ensure m.opts.ConfigInterface is not nil.
	if m.opts.ConfigRefreshCronHandlerID != "" {
		cron.RegisterHandler(m.opts.ConfigRefreshCronHandlerID, func(ctx context.Context) error {
			return m.opts.ConfigInterface.Refresh(ctx)
		})
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
	// ConfigInterface is the quotaconfig.Interface this module should use to
	// fetch policy configs. Refresh should be called periodically to ensure
	// policy configs are up to date (see ConfigRefreshHandler below).
	ConfigInterface quotaconfig.Interface

	// ConfigRefreshCronHandlerID is the ID for this module's config refresh
	// handler. The handler will be installed at <serving-prefix>/<ID>. The serving
	// prefix is controlled by the server/cron module. If unspecified, module users
	// should refresh policy configs periodically by manually calling
	// ConfigInterface.Refresh (see ConfigInterface above).
	ConfigRefreshCronHandlerID string
}

// Register adds command line flags for these module options to the given
// *flag.FlagSet. Mutates module options by initializing defaults.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
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
