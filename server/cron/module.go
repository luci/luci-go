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

package cron

import (
	"context"
	"flag"
	"strings"

	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/module"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/cron")

// ModuleOptions contain configuration of the cron server module.
//
// It will be used to initialize Default dispatcher.
type ModuleOptions struct {
	// Dispatcher is a dispatcher to use.
	//
	// Default is the global Default instance.
	Dispatcher *Dispatcher

	// ServingPrefix is a URL path prefix to serve registered cron handlers from.
	//
	// GETs to a URL under this prefix (regardless which one) will be treated
	// as Cloud Scheduler calls.
	//
	// Must start with "/internal/". Default is "/internal/cron".
	ServingPrefix string

	// AuthorizedCallers is a list of service accounts Cloud Scheduler may use to
	// call cron HTTP endpoints.
	//
	// See https://cloud.google.com/scheduler/docs/http-target-auth for details.
	//
	// Can be empty on Appengine, since there calls are authenticated using
	// "X-Appengine-Cron" header.
	//
	// Default is an empty list.
	AuthorizedCallers []string
}

// Register registers the command line flags.
//
// Mutates `o` by populating defaults.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	if o.ServingPrefix == "" {
		o.ServingPrefix = "/internal/cron"
	}
	f.StringVar(&o.ServingPrefix, "cron-serving-prefix", o.ServingPrefix,
		`URL prefix to serve registered cron handlers from, must start with '/internal/'.`)
	f.Var(luciflag.StringSlice(&o.AuthorizedCallers), "cron-authorized-caller",
		`Service account email to accept calls from. May be repeated.`)
}

// NewModule returns a server module that sets up a cron dispatcher.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &cronModule{opts: opts}
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

// cronModule implements module.Module.
type cronModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*cronModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*cronModule) Dependencies() []module.Dependency {
	return nil
}

// Initialize is part of module.Module interface.
func (m *cronModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	logging.Infof(ctx, "Cron is serving handlers from %q", m.opts.ServingPrefix)
	if !strings.HasPrefix(m.opts.ServingPrefix, "/internal/") {
		return nil, errors.Reason(`-cron-serving-prefix must start with "/internal/", got %q`, m.opts.ServingPrefix).Err()
	}

	if m.opts.Dispatcher == nil {
		m.opts.Dispatcher = &Default
	}

	m.opts.Dispatcher.GAE = opts.Serverless == module.GAE
	m.opts.Dispatcher.DisableAuth = !opts.Prod
	m.opts.Dispatcher.AuthorizedCallers = m.opts.AuthorizedCallers
	m.opts.Dispatcher.InstallCronRoutes(host.Routes(), m.opts.ServingPrefix)

	return ctx, nil
}
