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
	"net"

	"google.golang.org/grpc"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

// Serverless is an enumeration of recognized serverless runtimes.
//
// It exists to allow modules to recognize they run in specific serverless
// environments in case modules explicitly depend on features provided by these
// environment.
//
// For example, on GAE (and only there!) headers `X-Appengine-*` can be trusted
// and some modules take advantage of that.
type Serverless string

const (
	// Unknown means the server didn't detect any serverless environment.
	Unknown Serverless = ""
	// GAE means the server is running on Google Cloud AppEngine.
	GAE Serverless = "GAE"
	// CloudRun means the server is running on Google Serverless Cloud Run.
	CloudRun Serverless = "Cloud Run"
)

// IsGCP is true for a serverless Google platform.
func (s Serverless) IsGCP() bool {
	return s == GAE || s == CloudRun
}

// Module represents some optional part of a server.
//
// It is generally a stateful object constructed from some configuration. It can
// be hooked to the server in server.New(...) or server.Main(...).
type Module interface {
	// Name is a name of this module for logs and dependency relations.
	//
	// Usually a package that implements the module registers module's name as
	// a public global variable (so other packages can refer to it) and this
	// method just returns it.
	Name() Name

	// Dependencies returns dependencies (required and optional) of this module.
	Dependencies() []Dependency

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
// tweak their behavior, if necessary.
type HostOptions struct {
	Prod         bool       // set when running in production (not on a dev workstation)
	Serverless   Serverless // set when running in a serverless environment, implies Prod
	CloudProject string     // name of hosting Google Cloud Project if running in GCP
	CloudRegion  string     // name of a hosting region (e.g. 'us-central1') if known
}

// Host is part of server.Server API that modules are allowed to use during
// their initialization to inject useful functionality into the server.
//
// TODO(vadimsh):
//   - Allow adding a middleware to the default middleware chain.
type Host interface {
	// ServiceRegistrar is a registrar that can be used to register gRPC services.
	//
	// The services registered here will be exposed through both gRPC and pRPC
	// protocols on corresponding ports.
	grpc.ServiceRegistrar

	// HTTPAddr is the address the main HTTP port is bound to.
	//
	// May be nil if the server doesn't expose the main port.
	HTTPAddr() net.Addr

	// GRPCAddr is the address the gRPC port is bound to.
	//
	// May be nil if the server doesn't expose the gRPC port.
	GRPCAddr() net.Addr

	// UserAgent can be put into "User-Agent" header when calling other servers.
	//
	// It includes some public details about the server (like its name and
	// version). Placing them into "User-Agent" header is useful since it is
	// exposed in the logs of the server being called. It allows to identify at
	// a glance what software is making calls. This is similar to how e.g. "curl"
	// places its version into "User-Agent" header.
	//
	// Note that HTTP clients obtained through go.chromium.org/luci/server/auth
	// are already setup to report this header. Thus this method is primarily
	// useful for manually constructed http.Client and for gRPC clients.
	UserAgent() string

	// Routes returns a router that servers HTTP requests hitting the main port.
	//
	// The module can use it to register additional request handlers.
	//
	// Note that users of server.Server can register more routers through
	// server.AddPort(...) and server.VirtualHost(...). Such application-specific
	// routers are not accessible to modules.
	Routes() *router.Router

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

	// RegisterWarmup registers a callback that is run in server's Serve right
	// before the serving loop.
	//
	// It receives the global server context (including all customizations made
	// by the user code in server.Main). Intended for best-effort warmups: there's
	// no way to gracefully abort the server startup from a warmup callback.
	//
	// All module's essential initialization should happen in its Initialize
	// method instead. The downside is that Initialize runs before the user code
	// in server.Main and it can't depend on modifications done there.
	RegisterWarmup(cb func(context.Context))

	// RegisterCleanup registers a callback that is run in server's Serve after
	// the server exits the serving loop.
	//
	// It receives the global server context.
	RegisterCleanup(cb func(context.Context))

	// RegisterUnaryServerInterceptors registers grpc.UnaryServerInterceptor's
	// applied to all unary RPCs that hit the server.
	//
	// Interceptors are chained in order they are registered, which matches
	// the order of modules in the list of modules. The first registered
	// interceptor becomes the outermost.
	RegisterUnaryServerInterceptors(intr ...grpc.UnaryServerInterceptor)

	// RegisterStreamServerInterceptors registers grpc.StreamServerInterceptor's
	// applied to all streaming RPCs that hit the server.
	//
	// Interceptors are chained in order they are registered, which matches
	// the order of modules in the list of modules. The first registered
	// interceptor becomes the outermost.
	RegisterStreamServerInterceptors(intr ...grpc.StreamServerInterceptor)

	// RegisterCookieAuth registers an implementation of a cookie-based
	// authentication scheme.
	//
	// If there are multiple such schemes registered, the server will refuse to
	// start.
	RegisterCookieAuth(method auth.Method)
}
