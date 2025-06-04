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

package pubsub

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
var ModuleName = module.RegisterName("go.chromium.org/luci/server/pubsub")

// ModuleOptions contain configuration of the pubsub server module.
//
// It will be used to initialize Default dispatcher.
type ModuleOptions struct {
	// Dispatcher is a dispatcher to use.
	//
	// Default is the global Default instance.
	Dispatcher *Dispatcher

	// ServingPrefix is a URL path prefix to serve registered pubsub handlers from.
	//
	// POSTs to a URL under this prefix (regardless which one) will be treated
	// as Cloud Pub/Sub message pushes.
	//
	// Must start with "/internal/". Default is "/internal/pubsub".
	ServingPrefix string

	// AuthorizedCallers is a list of service accounts Cloud Pub/Sub may use to
	// call pub/sub HTTP endpoints.
	//
	// See https://cloud.google.com/pubsub/docs/authenticate-push-subscriptions for details.
	//
	// This must be specified for Cloud Pub/Sub messages to be accepted.
	//
	// Default is an empty list.
	AuthorizedCallers []string
}

// Register registers the command line flags.
//
// Mutates `o` by populating defaults.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	if o.ServingPrefix == "" {
		o.ServingPrefix = "/internal/pubsub"
	}
	f.StringVar(&o.ServingPrefix, "pubsub-serving-prefix", o.ServingPrefix,
		`URL prefix to serve registered pubsub handlers from, must start with '/internal/'.`)
	f.Var(luciflag.StringSlice(&o.AuthorizedCallers), "pubsub-authorized-caller",
		`Service account email to accept calls from. May be repeated.`)
}

// NewModule returns a server module that sets up a pubsub dispatcher.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &pubsubModule{opts: opts}
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

// pubsubModule implements module.Module.
type pubsubModule struct {
	opts *ModuleOptions
}

// Name is part of module.Module interface.
func (*pubsubModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*pubsubModule) Dependencies() []module.Dependency {
	return nil
}

// Initialize is part of module.Module interface.
func (m *pubsubModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	logging.Infof(ctx, "Pubsub is serving handlers from %q", m.opts.ServingPrefix)
	if !strings.HasPrefix(m.opts.ServingPrefix, "/internal/") {
		return nil, errors.Fmt(`-pubsub-serving-prefix must start with "/internal/", got %q`, m.opts.ServingPrefix)
	}

	if m.opts.Dispatcher == nil {
		m.opts.Dispatcher = &Default
	}

	m.opts.Dispatcher.DisableAuth = !opts.Prod
	m.opts.Dispatcher.AuthorizedCallers = m.opts.AuthorizedCallers
	m.opts.Dispatcher.InstallPubSubRoutes(host.Routes(), m.opts.ServingPrefix)

	return ctx, nil
}
