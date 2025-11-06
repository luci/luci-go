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

// Package servertest contains helpers for running server integration tests.
//
// This package needs a lot more work. It is basically a placeholder now with
// a single test that makes sure OpenTelemetry stuff can start (since it broke
// in production before). The API is not final.
package servertest

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth/integration/gcemeta"
	"go.chromium.org/luci/auth/integration/localauth"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/server"
	srvauth "go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/module"
)

// FakeClientIdentity is how clients authenticate as by default when using fake
// RPC client auth.
//
// See DisableFakeClientRPCAuth in Settings for details.
const FakeClientIdentity identity.Identity = "user:client@servertest.example.com"

// Settings are parameters for launching a test server.
type Settings struct {
	// Options are base server options.
	//
	// In production they are usually parsed from flags, but here they should
	// be supplied explicitly (unless defaults are not good).
	//
	// Some of the fields will be forcefully overridden to make sure the server
	// behaves well in the test environment. In particular `Prod` will always be
	// false.
	//
	// If nil, some safe defaults will be used.
	Options *server.Options

	// Modules is a list of configured modules to use in the server.
	//
	// Unlike the production server, no flag parsing is done. Modules need to be
	// created using concrete options suitable for the test (i.e. do not use
	// NewModuleFromFlags, since there are no flags).
	Modules []module.Module

	// Init is called during the server initialization before starting it.
	//
	// This is a place where various routes can be registered and extra background
	// activities started.
	Init func(srv *server.Server) error

	// DisableFakeGCEMetadata can be used to **disable** fake GCE metadata server.
	//
	// The GCE metadata server is used to grab authentication tokens when the
	// server makes calls to other services.
	//
	// By default (when DisableFakeGCEMetadata is false), the test server is
	// launched in an environment with a fake GCE metadata server that returns
	// fake tokens. It means it won't be able to make any real calls to external
	// services (and also accidentally do something bad).
	//
	// Additionally this fake GCE metadata makes the server believe it runs in a
	// GCP environment even when running on a local workstation. This will make it
	// hit more production code paths, potentially uncovering bugs there.
	//
	// If DisableFakeGCEMetadata is true the test server will use whatever is
	// available in the execution environment or configured via Options. This
	// generally means it will use real authentication tokens.
	DisableFakeGCEMetadata bool

	// DisableFakeClientRPCAuth can be used to **disable** fake RPC auth tokens.
	//
	// By default (when this option is false), the server is configured to accept
	// fake per-RPC authentication tokens and reject real ones. Clients that make
	// authenticated calls will need to be configured to send such fake tokens.
	// This can be done by injecting lucictx.LocalAuth into the context, see
	// FakeClientRPCAuth().
	//
	// RPC calls made from clients that use FakeClientRPCAuth() will be
	// authenticated as coming from FakeClientIdentity.
	//
	// If DisableFakeClientRPCAuth is true the test server will accept only real
	// OAuth and/or OpenID tokens (depending on Options). It means the test
	// clients will also need to generate real tokens, which may require
	// configuring tests to run under some real service account.
	DisableFakeClientRPCAuth bool
}

// TestServer represents a running server-under-test.
type TestServer struct {
	srv       *server.Server
	tm        *testingModule
	localAuth *lucictx.LocalAuth
	cleanup   []func()
	cancel    context.CancelFunc
}

// RunServer launches a test server and runs it in background.
//
// It initializes the server based on the settings, launches its serving loop in
// a background goroutine and waits for the health check to successfully pass
// before returning.
//
// The caller can then make HTTP or gRPC requests to the server via the loopback
// address in TestServer.
//
// The caller is responsible to shut the server down when done with it via a
// call to Shutdown.
//
// The given context is used as the root context for the server (eventually
// inherited by everything under it). It can be a background context if nothing
// extra is needed.
//
// Note that some server's dependencies (in particular OpenTelemetry) use global
// variables and thus running multiple services in the same process concurrently
// may result in flakiness.
func RunServer(ctx context.Context, settings *Settings) (*TestServer, error) {
	if settings == nil {
		settings = &Settings{}
	}

	var opts server.Options
	if settings.Options != nil {
		opts = *settings.Options
	}

	// This populates some defaults. Unless the test runs on GAE or Cloud Run
	// (which is extremely unlikely) it should not pick up any extra stuff.
	if _, err := server.OptionsFromEnv(&opts); err != nil {
		return nil, errors.Fmt("populating options: %w", err)
	}

	// This populates even more defaults as a side effect. This is basically just
	// like launching the real server with an empty command line.
	var fs flag.FlagSet
	opts.Register(&fs)
	if err := fs.Parse(nil); err != nil {
		return nil, errors.Fmt("initializing flags: %w", err)
	}

	// Disable prod code paths to avoid hitting real servers as much as possible.
	// Bind to any available ports dynamically to avoid clashing with other tests.
	opts.Prod = false
	opts.CloudProject = "fake-test-project"
	opts.HTTPAddr = "127.0.0.1:0"
	opts.GRPCAddr = "127.0.0.1:0"
	opts.AdminAddr = "-"

	// If running with a fake metadata server, make sure it is being picked up for
	// auth.
	if !settings.DisableFakeGCEMetadata {
		opts.ClientAuth = auth.Options{Method: auth.GCEMetadataMethod}
	}

	ctx, cancel := context.WithCancel(ctx)
	ts := &TestServer{cancel: cancel}

	success := false
	defer func() {
		if !success {
			ts.Shutdown()
		}
	}()

	if !settings.DisableFakeGCEMetadata {
		metaSrv := &gcemeta.Server{
			Generator: &FakeRPCTokenGenerator{
				Email: "server@servertest.example.com",
			},
			Email:            "server@servertest.example.com",
			Scopes:           scopes.CloudScopeSet(),
			MinTokenLifetime: 15 * time.Minute,
		}
		metaHost, err := metaSrv.Start(ctx)
		if err != nil {
			return nil, errors.Fmt("launching fake GCE metadata server: %w", err)
		}
		ts.cleanup = append(ts.cleanup, func() { _ = metaSrv.Stop(ctx) })
		// Unfortunately we need to modify the global state, since this is where
		// methods like `metadata.OnGCE` read the metadata server address from.
		for _, env := range []string{"GCE_METADATA_ROOT", "GCE_METADATA_IP", "GCE_METADATA_HOST"} {
			if err := os.Setenv(env, metaHost); err != nil {
				return nil, errors.Fmt("setting env var %q: %w", env, err)
			}
		}
	}

	// Add a special module that will examine the starting server to get ports.
	ts.tm = &testingModule{}
	modules := make([]module.Module, 0, len(settings.Modules)+1)
	modules = append(modules, settings.Modules...)
	modules = append(modules, ts.tm)

	// This binds the ports and initializes all modules, but doesn't start the
	// server loop yet.
	var err error
	if ts.srv, err = server.New(ctx, opts, modules); err != nil {
		return nil, errors.Fmt("initializing server: %w", err)
	}
	if !ts.tm.initialized {
		panic("should not be possible: a successful server init initializes all modules")
	}

	// Override RPC authentication to support fake tokens.
	if !settings.DisableFakeClientRPCAuth {
		// Generate a random seed that would be part of generated tokens and the
		// fake RPC auth will check it is there. This is a way to prevent unrelated
		// tests from talking to one another.
		seed := rand.Uint64()

		// A local auth server that clients will use to generate fake tokens.
		localAuthSrv := &localauth.Server{
			TokenGenerators: map[string]localauth.TokenGenerator{
				"default": &FakeRPCTokenGenerator{
					Seed:  seed,
					Email: FakeClientIdentity.Email(),
				},
			},
			DefaultAccountID: "default",
		}
		if ts.localAuth, err = localAuthSrv.Start(ctx); err != nil {
			return nil, errors.Fmt("failed to start local auth server: %w", err)
		}
		ts.cleanup = append(ts.cleanup, func() { _ = localAuthSrv.Stop(ctx) })

		// An RPC authentication that the server will use to check fake tokens.
		var audiences stringset.Set
		if opts.OpenIDRPCAuthEnable {
			audiences = stringset.NewFromSlice(opts.OpenIDRPCAuthAudience...)
			for _, host := range []string{ts.HTTPAddr(), ts.GRPCAddr()} {
				audiences.Add(host)
				audiences.Add("http://" + host)
				audiences.Add("https://" + host)
			}
		}
		ts.srv.SetRPCAuthMethods([]srvauth.Method{&FakeRPCAuth{
			Seed:             seed,
			IDTokensAudience: audiences,
		}})
	}

	// Let the caller code register routes and other stuff. This is the same thing
	// that server.Main is doing.
	if settings.Init != nil {
		if err := settings.Init(ts.srv); err != nil {
			return nil, errors.Fmt("user-supplied init callback: %w", err)
		}
	}

	// Launch the server and wait until it responds to the health check. Note that
	// the port is already open: our request will just queue up on the listening
	// socket in case the goroutine is slow to start. No extra synchronization is
	// necessary.
	serveDone := make(chan error, 1)
	go func() { serveDone <- ts.srv.Serve() }()

	healthCtx, healthCancel := context.WithTimeout(ctx, time.Minute)
	defer healthCancel()

	srvCtx := ts.Context()
	logging.Infof(srvCtx, "Waiting for a health check to pass...")
	healthDone := make(chan error, 1)
	go func() {
		// There's potentially a race condition between HTTP and gRPC health
		// servers. Make sure to wait for both checks to pass (in case TestServer
		// users care about a particular flavor of RPC client).
		eg, ectx := errgroup.WithContext(healthCtx)
		eg.Go(func() error { return waitHTTPHealthy(ectx, ts) })
		eg.Go(func() error { return waitGRPCHealthy(ectx, ts) })
		healthDone <- eg.Wait()
	}()

	serveLoopErr := func(err error) error {
		if err == nil {
			logging.Errorf(srvCtx, "The server was interrupted unexpectedly when starting")
			return errors.New("server was interrupted unexpectedly when starting")
		}
		logging.Errorf(srvCtx, "The server failed to start: %s", err)
		return errors.Fmt("server failed to start: %w", err)
	}

	// Wait either until the server stops (presumably due to an error) or the
	// health check fails or passes.
	select {
	case serveErr := <-serveDone:
		return nil, serveLoopErr(serveErr)

	case healthErr := <-healthDone:
		if healthErr != nil {
			logging.Errorf(srvCtx, "Health check failed: %s", healthErr)
			logging.Infof(srvCtx, "Waiting for the server loop to exit abnormally...")
			select {
			case serveErr := <-serveDone:
				return nil, serveLoopErr(serveErr)
			case <-time.After(5 * time.Second):
				logging.Errorf(srvCtx, "Giving up waiting for the server loop to exit")
				return nil, errors.Fmt("health check error and the server is stuck starting: %w", healthErr)
			}
		}
	}

	success = true // disarm the defer
	return ts, nil
}

// Shutdown closes the server waiting for it to fully terminate.
func (ts *TestServer) Shutdown() {
	if ts.srv != nil { // possible when exiting early in RunServer
		ts.srv.Shutdown()
	}
	for _, cb := range ts.cleanup {
		cb()
	}
	ts.cancel()
}

// Context is the server's root context with all modules initialized.
func (ts *TestServer) Context() context.Context {
	return ts.srv.Context
}

// HTTPAddr is "127.0.0.1:<port>" of the server's HTTP port.
func (ts *TestServer) HTTPAddr() string {
	return ts.tm.httpAddr.String()
}

// GRPCAddr is "127.0.0.1:<port>" of the server's gRPC port.
func (ts *TestServer) GRPCAddr() string {
	return ts.tm.grpcAddr.String()
}

// GRPCConn returns a gRPC client connection to the server's gRPC port.
func (ts *TestServer) GRPCConn(opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	finalOpts := make([]grpc.DialOption, 0, len(opts)+1)
	finalOpts = append(finalOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	finalOpts = append(finalOpts, opts...)
	return grpc.NewClient("passthrough:///"+ts.GRPCAddr(), finalOpts...)
}

// FakeClientRPCAuth can be used to inject a fake local auth token provider into
// the LUCI context.
//
// It can be inserted into the client context using regular lucictx API:
//
//	ctx = lucictx.SetLocalAuth(ctx, srv.FakeClientRPCAuth())
//
// Return nil if DisableFakeClientRPCAuth is true.
func (ts *TestServer) FakeClientRPCAuth() *lucictx.LocalAuth {
	return ts.localAuth
}

////////////////////////////////////////////////////////////////////////////////

var testingModuleName = module.RegisterName("go.chromium.org/luci/server/servertest")

// This is known to isHealthCheckerUA in server.go
const healthCheckUA = "LUCI-ServerTest-Health"

// testingModule is a module.Module injected into test server to get access to
// some of the server's API.
type testingModule struct {
	initialized bool
	httpAddr    net.Addr
	grpcAddr    net.Addr
}

func (tm *testingModule) Name() module.Name                 { return testingModuleName }
func (tm *testingModule) Dependencies() []module.Dependency { return nil }

func (tm *testingModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	tm.initialized = true
	tm.httpAddr = host.HTTPAddr()
	tm.grpcAddr = host.GRPCAddr()
	return ctx, nil
}

func waitHTTPHealthy(ctx context.Context, ts *TestServer) error {
	req, err := http.NewRequestWithContext(
		ctx,
		"GET",
		fmt.Sprintf("http://%s/healthz", ts.HTTPAddr()),
		nil,
	)
	if err != nil {
		panic(err) // should be impossible
	}
	req.Header.Set("User-Agent", healthCheckUA)
	resp, err := http.DefaultClient.Do(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err == nil && resp.StatusCode != 200 {
		err = errors.Fmt("health check replied with status code %d", resp.StatusCode)
	}
	return err
}

func waitGRPCHealthy(ctx context.Context, ts *TestServer) error {
	conn, err := ts.GRPCConn(grpc.WithUserAgent(healthCheckUA))
	if err != nil {
		return errors.Fmt("constructing gRPC health check client: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := grpc_health_v1.NewHealthClient(conn).Watch(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		return errors.Fmt("error calling gRPC health check Watch: %w", err)
	}

	for {
		status, err := stream.Recv()
		if err != nil {
			return errors.Fmt("gRPC health check error: %w", err)
		}
		if status.Status == grpc_health_v1.HealthCheckResponse_SERVING {
			return nil
		}
	}
}
