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

package tq

import (
	"context"
	"flag"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/module"
)

// ModuleOptions contain configuration of the TQ server module.
//
// It will be used to initialize Default dispatcher.
type ModuleOptions struct {
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	// TODO(vadimsh): Add some. Probably the external hostname of the server
	// (for non-GAE Cloud Tasks) and something related to ttq sweeping.
}

// NewModule returns a server module that sets up a TQ dispatcher.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &tqModule{opts: opts}
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

// tqModule implements module.Module.
type tqModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*tqModule) Name() string {
	return "go.chromium.org/luci/server/tq"
}

// Initialize is part of module.Module interface.
func (m *tqModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	if opts.CloudProject == "" {
		return nil, errors.Reason("cloud project name is required").Err()
	}
	if opts.CloudRegion == "" {
		return nil, errors.Reason("cloud region name is required").Err()
	}
	Default.CloudProject = opts.CloudProject
	Default.CloudRegion = opts.CloudRegion
	Default.InstallRoutes(host.Routes(), "/internal/tasks/")
	return ctx, nil
}
