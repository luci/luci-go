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
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.BuganizerClientMode,
		"buganizer-mode",
		o.BuganizerClientMode,
		"The Buganizer integration client mode to use.",
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

// Initialize is part of module.Module interface.
func (m *buganizerModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	ctx = context.WithValue(ctx, &BuganizerClientModeKey, m.opts.BuganizerClientMode)
	return ctx, nil
}
