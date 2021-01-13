// Copyright 2021 The LUCI Authors.
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

package warmup

import (
	"context"
	"flag"

	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/warmup")

// ModuleOptions contain configuration of the warmup server module.
type ModuleOptions struct {
	// empty for now
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	// no flags for now
}

// NewModule returns a server module that runs warmup handlers registered via
// Register prior to starting the serving loop.
//
// When running on GAE also registered noop /_ah/warmup handler, as recommended
// by GAE docs.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &warmupModule{opts: opts}
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

// warmupModule implements module.Module.
type warmupModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*warmupModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*warmupModule) Dependencies() []module.Dependency {
	return nil
}

// Initialize is part of module.Module interface.
func (m *warmupModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	host.RegisterWarmup(func(ctx context.Context) { Warmup(ctx) })

	if opts.GAE {
		// See https://cloud.google.com/appengine/docs/standard/go/configuring-warmup-requests.
		// All warmups should happen *before* the serving loop and /_ah/warmup
		// should just always return OK.
		host.Routes().GET("/_ah/warmup", router.MiddlewareChain{}, func(*router.Context) {})
	}

	return ctx, nil
}
