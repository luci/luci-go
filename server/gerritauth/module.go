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

package gerritauth

import (
	"context"
	"flag"

	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/warmup"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/gerritauth")

// ModuleOptions contain configuration of the gerritauth server module.
type ModuleOptions struct {
	// Method is an instance of AuthMethod to configure.
	//
	// If nil, will configure the global Method instance.
	Method *AuthMethod

	// Header is a name of the request header to check for JWTs.
	//
	// Default is "X-Gerrit-Auth".
	Header string

	// SignerAccounts are emails of services account that sign Gerrit JWTs.
	//
	// If empty, authentication based on Gerrit JWTs will be disabled.
	SignerAccounts []string

	// Audience is an expected "aud" field of JWTs.
	//
	// Required if SignerAccount is not empty.
	Audience string
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	if o.Header == "" {
		o.Header = "X-Gerrit-Auth"
	}
	f.StringVar(
		&o.Header,
		"gerrit-auth-header",
		o.Header,
		`Name of the request header to check for JWTs.`,
	)
	f.Var(luciflag.StringSlice(&o.SignerAccounts),
		"gerrit-auth-signer-account",
		"Email of a Gerrit service account trusted to sign Gerrit JWT tokens. "+
			"May be specified multiple times. If empty, authentication based on Gerrit JWTs will be disabled.",
	)
	f.StringVar(
		&o.Audience,
		"gerrit-auth-audience",
		o.Audience,
		`Expected "aud" field of the JWTs. Required if -gerrit-auth-signer-account is used.`,
	)
}

// NewModule returns a server module that configures Gerrit auth method.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &serverModule{opts: opts}
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

// serverModule implements module.Module.
type serverModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*serverModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*serverModule) Dependencies() []module.Dependency {
	return nil
}

// Initialize is part of module.Module interface.
func (m *serverModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	if len(m.opts.SignerAccounts) != 0 {
		if m.opts.Audience == "" {
			return nil, errors.Reason("-gerrit-auth-audience is required when -gerrit-auth-signer-account is used").Err()
		}
	} else if opts.Prod {
		logging.Warningf(ctx, "Disabling Gerrit JWT auth: -gerrit-auth-signer-account is unset")
	}

	method := m.opts.Method
	if method == nil {
		method = &Method
	}
	method.Header = m.opts.Header
	method.SignerAccounts = m.opts.SignerAccounts
	method.Audience = m.opts.Audience

	if method.isConfigured() {
		warmup.Register("server/gerritauth", method.Warmup)
	}
	return ctx, nil
}
