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

package buganizer

import (
	"context"
	"flag"

	"go.chromium.org/luci/server/module"
)

// The modes for the buganizer integration client.
// Each mode defines how LUCI Analysis will behave
// with Buganizer.
// `ModeDisable` is the default mode, and ignores Buganizer bugs.
// `ModeProvided` uses the provided Buganizer integration client.
const (
	ModeDisable  = "disable"
	ModeProvided = "provided"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/analysis/internal/bugs/buganizer")

// ModuleOptions contain configuartion for the Buganizer module.
type ModuleOptions struct {
	// Option for the buganizer management, defaults to `disable`
	// Acceptable values are:
	// `disable` - Indicates that Buganizer integration should not be used.
	// `provided` - Indicates that Buganizer integration should use provided client.
	// `fake` - Indicates that Buganizer integration should use the fake Buganizer implementation.
	BuganizerClientMode string

	// Option that indicates the base subdomain for the Buganizer endpoint.
	// For example: placeholder-issuetracker-c2p, that value will
	// be used by the clients to initiate connections with Buganizer.
	BuganizerEndpointBase string

	// Option for which OAuth scope to use for PerRPC credentials
	// for the Buganizer client to use for authenticating requests.
	// Example: "https://www.googleapis.com/auth/placeholder".
	BuganizerEndpointOAuthScope string

	// The component ID used for creating test issues in Buganizer.
	BuganizerTestComponentId int64

	// The email used by LUCI Analysis to file bugs.
	BuganizerSelfEmail string

	// Whether Buganizer is being used in a test environment.
	BuganizerTestMode bool
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.BuganizerClientMode,
		"buganizer-mode",
		o.BuganizerClientMode,
		"The Buganizer integration client mode to use.",
	)
	f.StringVar(
		&o.BuganizerEndpointBase,
		"buganizer-endpoint-base",
		o.BuganizerEndpointBase,
		"The base subdomain for Buganizer endpoint.",
	)

	f.StringVar(
		&o.BuganizerEndpointOAuthScope,
		"buganizer-endpoint-oauth-scope",
		o.BuganizerEndpointOAuthScope,
		"The Buganizer oauth scope to use for authenticating requests.",
	)

	f.Int64Var(
		&o.BuganizerTestComponentId,
		"buganizer-test-component-id",
		o.BuganizerTestComponentId,
		"The Buganizer component to be used for creating test issues.",
	)

	f.StringVar(
		&o.BuganizerSelfEmail,
		"buganizer-self-email",
		o.BuganizerSelfEmail,
		"The email that LUCI Analysis uses to file bugs. Used to distinguish actions taken by LUCI Analysis from user actions.",
	)

	f.BoolVar(
		&o.BuganizerTestMode,
		"buganizer-test-mode",
		o.BuganizerTestMode,
		"Indicates whether Buganizer is used in test mode.",
	)
}

// NewModule returns a server module that sets a context value for Buganizer
// integration mode.
// That value can be used to determine how the server will interact
// with Buganizer.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &buganizerModule{opts: opts}
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

// buganizerModule implements module.Module.
type buganizerModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*buganizerModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*buganizerModule) Dependencies() []module.Dependency {
	return nil
}

// BuganizerClientModeKey the key to get the value for Buganizer integration
// mode from the Context.
var BuganizerClientModeKey = "go.chromium.org/luci/analysis/internal/bugs/buganizer:buganizerMode"

// BuganizerEndpointBaseKey the key to get the value for Buganizer's endpoint
// base subdomain from the Context.
var BuganizerEndpointBaseKey = "go.chromium.org/luci/analysis/internal/bugs/buganizer:buganizerEndpointBase"

// BuganizerEndpointOAuthScopeKey the key to get the value for Buganier OAuth scope
// from the context.
var BuganizerEndpointOAuthScopeKey = "go.chromium.org/luci/analysis/internal/bugs/buganizer:buganizerEndpointOAuthScopeKey"

// BuganizerTestComponentIdKey the context key to get Buganizer test component id.
var BuganizerTestComponentIdKey = "go.chromium.org/luci/analysis/internal/bugs/buganizer:buganizerTestComponentIdKey"

// BuganizerSelfEmailKey the context key to get email that
// LUCI Analysis uses to file bugs in Buganizer.
var BuganizerSelfEmailKey = "go.chromium.org/luci/analysis/internal/bugs/buganizer:luciAnalysisBuganizerEmailKey"

// BuganizerTestModeKey the context key to get whether Buganizer is in test mode.
var BuganizerTestModeKey = "go.chromium.org/luci/analysis/internal/bugs/buganizer:buganizerTestModeKey"

// Initialize is part of module.Module interface.
func (m *buganizerModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	ctx = context.WithValue(ctx, &BuganizerClientModeKey, m.opts.BuganizerClientMode)
	ctx = context.WithValue(ctx, &BuganizerEndpointBaseKey, m.opts.BuganizerEndpointBase)
	ctx = context.WithValue(ctx, &BuganizerEndpointOAuthScopeKey, m.opts.BuganizerEndpointOAuthScope)
	ctx = context.WithValue(ctx, &BuganizerTestComponentIdKey, m.opts.BuganizerTestComponentId)
	ctx = context.WithValue(ctx, &BuganizerSelfEmailKey, m.opts.BuganizerSelfEmail)
	ctx = context.WithValue(ctx, &BuganizerTestModeKey, m.opts.BuganizerTestMode)
	return ctx, nil
}
