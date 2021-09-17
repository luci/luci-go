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

package analytics

import (
	"context"
	"flag"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/module"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/analytics")

// ModuleOptions contain configuration of the analytics server module.
type ModuleOptions struct {
	// Google Analytics measurement ID to use like "G-XXXXXX".
	//
	// Default is empty, meaning Google analytics integration is disabled.
	AnalyticsID string
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.AnalyticsID,
		"analytics-id",
		o.AnalyticsID,
		`Google analytics measurement ID in "G-XXXXXX" format.`,
	)
}

// NewModuleFromFlags initializes options through command line flags.
//
// Calling this function registers flags in flag.CommandLine. They are usually
// parsed in server.Main(...).
func NewModuleFromFlags() module.Module {
	opts := &ModuleOptions{}
	opts.Register(flag.CommandLine)
	return &analyticsModule{opts: opts}
}

// analyticsModule implements module.Module.
type analyticsModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*analyticsModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*analyticsModule) Dependencies() []module.Dependency {
	return nil
}

var ctxKey = "go.chromium.org/luci/server/analytics/ctxKey"

// Initialize is part of module.Module interface.
func (m *analyticsModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	if m.opts.AnalyticsID != "" {
		if !rGA4Allowed.MatchString(m.opts.AnalyticsID) {
			return ctx, errors.Reason("given --analytics-id %q is not a measurement ID (should be like G-XXXXXX)", m.opts.AnalyticsID).Err()
		}
		return context.WithValue(ctx, &ctxKey, makeGTagSnippet(m.opts.AnalyticsID)), nil
	}
	return ctx, nil
}
