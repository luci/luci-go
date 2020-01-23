// Copyright 2020 The LUCI Authors.
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

// Modules is a framework for extending server.Server with optional reusable
// bundles of functionality (called "modules" naturally).
package modules

import (
	"context"
	"flag"
)

// Module represents some optional reusable part of a server.
//
// It is generally a stateful object than can be hooked up into the server
// life cycle via server.Options.Modules or via server.RegisterModule(...).
type Module interface {
	// RegisterFlags registers configuration as CLI flags (if any).
	RegisterFlags(*flag.FlagSet)

	// Initialize is called during the server startup after the core functionality
	// is initialized, but before the server enters the serving loop.
	//
	// The method receives the global server context and can return a derived
	// context which will become the new global server context. That way the
	// module may inject additional state into the context inherited by all
	// requests.
	//
	// The module may use the given Host interface to register itself in various
	// server systems. It must not retain 'host': the object it points to becomes
	// invalid after Initialize finishes and using it causes panics. This is
	// intentional.
	//
	// Modules are initializes sequentially in order the are registered. A module
	// observes effects of initialization of all prior modules, e.g. via
	// context.Context.
	//
	// An error causes the process to crash with the corresponding message.
	Initialize(ctx context.Context, host Host, opts HostOptions) (context.Context, error)
}

// Base implements Module by doing nothing.
//
// Can be embedded into structs to minimally implement Module.
type Base struct{}

// RegisterFlags is part of Module interface.
func (Base) RegisterFlags(*flag.FlagSet) {}

// Initialize is part of Module interface.
func (Base) Initialize(ctx context.Context, host Host, opts HostOptions) (context.Context, error) {
	return ctx, nil
}

// HostOptions are options the server was started with.
//
// It is a subset of server.Options that modules are allowed to see. Note that
// server.Options aren't usable directly anyhow due to go module import cycles.
type HostOptions struct {
	Prod         bool   // set when running in production (not on a dev workstation)
	GAE          bool   // set when running on GAE, implies Prod
	Hostname     string // used for logging and metric fields, default is os.Hostname
	CloudProject string // name of hosting Google Cloud Project if running in GCP
}

// Host is part of server.Server API that modules are allowed to use during
// their initialization to inject useful functionality into the server.
//
// TODO(vadimsh):
//  * Allow adding a middleware to the default middleware chain.
//  * Allow adding an interceptor to the pRPC server interceptor chain.
//  * Allow registering HTTP routes.
//  * Allow registering pRPC servers.
type Host interface {
	// RegisterCleanup registers a callback that is run in server's ListenAndServe
	// after the server exits the serving loop.
	RegisterCleanup(cb func())

	// RunInBackground launches the given callback in a separate goroutine right
	// before starting the serving loop.
	//
	// Should be used for background asynchronous activities like reloading
	// configs.
	//
	// All logs lines emitted by the callback are annotated with "activity" field
	// which can be arbitrary, but by convention has format "<namespace>.<name>",
	// where "luci" namespace is reserved for internal activities.
	//
	// The context passed to the callback is canceled when the server is shutting
	// down. It is expected the goroutine will exit soon after the context is
	// canceled.
	RunInBackground(activity string, f func(context.Context))
}
