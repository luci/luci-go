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

// Package module defines a framework for extending server.Server with optional
// reusable bundles of functionality (called "modules", naturally).
package module

import (
	"context"

	"google.golang.org/grpc"
)

// Module represents some optional part of a server.
//
// It is generally a stateful object constructed from some configuration. It can
// be hooked to the server in server.New(...) or server.Main(...).
type Module interface {
	// Name is a name of this module for logs and internal maps.
	//
	// Usually it is a full name of the go package that implements the module, but
	// it can be arbitrary as long as it is unique within a server. Attempting to
	// register two identically named modules results in a fatal error during
	// the server startup.
	Name() string

	// Initialize is called during the server startup after the core functionality
	// is initialized, but before the server enters the serving loop.
	//
	// The method receives the global server context and can return a derived
	// context which will become the new global server context. That way the
	// module may inject additional state into the context inherited by all
	// requests. If it returns nil, the server context won't be changed.
	//
	// The module may use the context (with all values available there) and the
	// given Host interface to register itself in various server systems. It must
	// not retain 'host': the object it points to becomes invalid after Initialize
	// finishes and using it causes panics. This is intentional.
	//
	// Modules are initialized sequentially in order they are registered. A module
	// observes effects of initialization of all prior modules, e.g. via
	// context.Context.
	//
	// An error causes the process to crash with the corresponding message.
	Initialize(ctx context.Context, host Host, opts HostOptions) (context.Context, error)
}

// HostOptions are options the server was started with.
//
// It is a subset of global server.Options that modules are allowed to use to
// tweak their behavior.
type HostOptions struct {
	Prod         bool   // set when running in production (not on a dev workstation)
	CloudProject string // name of hosting Google Cloud Project if running in GCP
}

// Host is part of server.Server API that modules are allowed to use during
// their initialization to inject useful functionality into the server.
//
// TODO(vadimsh):
//  * Allow adding a middleware to the default middleware chain.
//  * Allow registering HTTP routes.
//  * Allow registering pRPC servers.
type Host interface {
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
	// The context passed to the callback is derived from the global server
	// context and it is canceled when the server is shutting down. It is expected
	// the goroutine will exit soon after the context is canceled.
	RunInBackground(activity string, f func(context.Context))

	// RegisterCleanup registers a callback that is run in server's ListenAndServe
	// after the server exits the serving loop.
	//
	// It receives the global server context.
	RegisterCleanup(cb func(context.Context))

	// RegisterUnaryServerInterceptor registers an grpc.UnaryServerInterceptor
	// applied to all unary RPCs that hit the server.
	//
	// Interceptors are chained in order they are registered, which matches
	// the order of modules in the list of modules. The first registered
	// interceptor becomes the outermost.
	RegisterUnaryServerInterceptor(intr grpc.UnaryServerInterceptor)
}
