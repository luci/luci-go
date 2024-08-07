// Copyright 2024 The LUCI Authors.
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

// Package hosts defines a LUCI Server module used to configure the
// hostnames of the services LUCI Analysis communicates with in a deplpoyment.
package hosts

import (
	"context"
	"errors"
	"flag"

	"go.chromium.org/luci/server/module"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/bisection/hosts")

// ModuleOptions contain configuration of the Hosts server module.
type ModuleOptions struct {
	// The hostname to use for pRPC requests to LUCI Bisection (e.g. from builders).
	APIHost string
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.APIHost,
		"luci-bisection-host",
		"",
		"The hostname of the LUCI Bisection pRPC service builders should connect to (e.g. bisection.api.luci.app).",
	)
}

// NewModule returns a server module that adds authentication settings
// to the context.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &hostModule{opts: opts}
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

// hostModule implements module.Module.
type hostModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*hostModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*hostModule) Dependencies() []module.Dependency {
	return nil
}

// Initialize is part of module.Module interface.
func (m *hostModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	if m.opts.APIHost == "" {
		return nil, errors.New("-luci-bisection-host must be set")
	}
	ctx = UseHosts(ctx, *m.opts)
	return ctx, nil
}

var (
	clientContextKey = "go.chromium.org/luci/bisection/hosts:setting"
)

// UseHosts installs the configures hosts into the context.
func UseHosts(ctx context.Context, config ModuleOptions) context.Context {
	return context.WithValue(ctx, &clientContextKey, config)
}

// APIHost returns the hostname of the LUCI Bisection pRPC client.
// E.g. "bisection.api.luci.app".
func APIHost(ctx context.Context) (string, error) {
	opts, ok := ctx.Value(&clientContextKey).(ModuleOptions)
	if !ok {
		return "", errors.New("hosts not configured in context")
	}
	return opts.APIHost, nil
}
