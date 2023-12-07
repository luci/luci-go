// Copyright 2023 The LUCI Authors.
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

package gtm

import (
	"context"
	"flag"
	"html/template"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/module"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/gtm")

// ModuleOptions contain configuration of the analytics server module.
type ModuleOptions struct {
	// ContainerID is the Google Tag Manager container ID to use like
	// "GTM-XXXXXX".
	//
	// Default is empty, meaning Google Tag Manager integration is disabled.
	ContainerID string
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.ContainerID,
		"gtm-container-id",
		o.ContainerID,
		`Google Tag Manager container ID in "GTM-XXXXXX" format.`,
	)
}

// NewModuleFromFlags initializes options through command line flags.
//
// Calling this function registers flags in flag.CommandLine. They are usually
// parsed in server.Main(...).
func NewModuleFromFlags() module.Module {
	opts := &ModuleOptions{}
	opts.Register(flag.CommandLine)
	return &gtmModule{opts: opts}
}

// gtmModule implements module.Module.
type gtmModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*gtmModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*gtmModule) Dependencies() []module.Dependency {
	return nil
}

type snippets struct {
	jsSnippet       template.HTML
	noScriptSnippet template.HTML
}

var ctxKey = "go.chromium.org/luci/server/gtm/ctxKey"

// Initialize is part of module.Module interface.
func (m *gtmModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	switch {
	case m.opts.ContainerID == "":
		return ctx, nil
	case rGTMAllowed.MatchString(m.opts.ContainerID):
		ctx = context.WithValue(ctx, &ctxKey, snippets{
			jsSnippet:       makeGTMJSSnippet(m.opts.ContainerID),
			noScriptSnippet: makeGTMNoScriptSnippet(m.opts.ContainerID),
		})
		return ctx, nil
	default:
		return ctx, errors.Reason("given -gtm-container-id %q is not a container ID (like GTM-XXXXXX)", m.opts.ContainerID).Err()
	}
}

// JSSnippet returns the GTM snippet to be inserted into the top of the page's
// HEAD as is.
//
// If the ContainerID isn't configured, this will return an empty template
// so that it's safe to insert its return value into a page's source regardless.
func JSSnippet(ctx context.Context) template.HTML {
	v, _ := ctx.Value(&ctxKey).(snippets)
	return v.jsSnippet
}

// NoScriptSnippet returns the GTM snippet that works in a no-script environment
// to be inserted into the top of the page's BODY as is.
//
// If the ContainerID isn't configured, this will return an empty template
// so that it's safe to insert its return value into a page's source regardless.
func NoScriptSnippet(ctx context.Context) template.HTML {
	v, _ := ctx.Value(&ctxKey).(snippets)
	return v.noScriptSnippet
}
