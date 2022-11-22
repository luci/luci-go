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

// Package scopedauth defines a LUCI Server module used to configure how
// LUCI Analysis authenticates to buildbucket, resultdb and gerrit.
// It is defined because dev instances of LUCI Analysis may read from
// production buildbucket/ResultDB and there are special authentication
// requirements when this occurs.
package scopedauth

import (
	"context"
	"errors"
	"flag"
	"net/http"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/module"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/analysis/internal/scopedauth")

// ModuleOptions contain configuration of the Cloud Spanner server module.
type ModuleOptions struct {
	DisableProjectScopedAuth bool // disable project-scoped auth for buildbucket, resultdb, gerrit.
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.BoolVar(
		&o.DisableProjectScopedAuth,
		"disable-project-scoped-auth",
		false,
		"If project-scoped authentication should be disabled, and LUCI Analysis should "+
			"authenticate using its GAE service account instead. Useful in cases where "+
			"dev needs to read data from prod. Defaults to false.",
	)
}

// NewModule returns a server module that adds authentication settings
// to the context.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &authModule{opts: opts}
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

// authModule implements module.Module.
type authModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*authModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*authModule) Dependencies() []module.Dependency {
	return nil
}

// Initialize is part of module.Module interface.
func (m *authModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	ctx = UseProjectScopedAuthSetting(ctx, !m.opts.DisableProjectScopedAuth)
	return ctx, nil
}

var (
	clientContextKey = "go.chromium.org/luci/analysis/internal/scopedauth:setting"
)

// UseProjectScopedAuthSetting installs the setting to use (or not use)
// project-scoped authentication into the context.
func UseProjectScopedAuthSetting(ctx context.Context, useProjectScopedAuth bool) context.Context {
	return context.WithValue(ctx, &clientContextKey, useProjectScopedAuth)
}

// GetRPCTransport returns returns http.RoundTripper to use
// for outbound HTTP RPC requests to buildbucket, resultdb and
// gerrit. Project is name of the LUCI Project which the request
// should be performed as (if project-scoped authentication is
// enabled).
func GetRPCTransport(ctx context.Context, project string, opts ...auth.RPCOption) (http.RoundTripper, error) {
	enabled, ok := ctx.Value(&clientContextKey).(bool)
	if !ok {
		return nil, errors.New("project-scoped authentication setting not configured in context")
	}

	var t http.RoundTripper
	var err error
	if enabled {
		options := append([]auth.RPCOption{auth.WithProject(project)}, opts...)
		t, err = auth.GetRPCTransport(ctx, auth.AsProject, options...)
	} else {
		t, err = auth.GetRPCTransport(ctx, auth.AsSelf, opts...)
	}

	if err != nil {
		return nil, err
	}
	return t, nil
}
