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

// Package ui holds the module that contains information about
// UI handling.
package ui

import (
	"context"
	"flag"

	"go.chromium.org/luci/server/module"
)

var ModuleName = module.RegisterName("go.chromium.org/luci/analysis/internal/ui")

type ModuleOptions struct {
	UIBaseURL string
}

func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.UIBaseURL,
		"ui-base-url",
		"luci-milo-dev.appspot.com/ui/tests",
		"The LUCI UI base URL to send UI requests to.",
	)
}

// NewModule returns a server module that sets a context value for UI URL.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &uiModule{opts: opts}
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

// uiModule implements module.Module.
type uiModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*uiModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*uiModule) Dependencies() []module.Dependency {
	return nil
}

// UIBaseURLKey the key to get the value for UI Base URL from the Context.
var UIBaseURLKey = "go.chromium.org/luci/analysis/internal/ui:uiBaseURL"

func (m *uiModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	ctx = context.WithValue(ctx, &UIBaseURLKey, m.opts.UIBaseURL)
	return ctx, nil
}
