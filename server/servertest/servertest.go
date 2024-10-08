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
	"net"
	"net/http"
	"os"
	"time"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/integration/authtest"
	"go.chromium.org/luci/auth/integration/gcemeta"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/module"
)

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
	// The GCE metadata server is used to grab authentication tokens when making
	// calls to other services.
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
}

// TestServer represents a running server-under-test.
type TestServer struct {
	srv     *server.Server
	tm      *testingModule
	cleanup []func()
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
		return nil, errors.Annotate(err, "populating options").Err()
	}

	// This populates even more defaults as a side effect. This is basically just
	// like launching the real server with an empty command line.
	var fs flag.FlagSet
	opts.Register(&fs)
	if err := fs.Parse(nil); err != nil {
		return nil, errors.Annotate(err, "initializing flags").Err()
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

	ts := &TestServer{}

	success := false
	defer func() {
		if !success {
			ts.Shutdown()
		}
	}()

	if !settings.DisableFakeGCEMetadata {
		metaSrv := &gcemeta.Server{
			Generator: &authtest.FakeTokenGenerator{Email: "test@example.com"},
			Email:     "test@example.com",
			Scopes: []string{
				"https://www.googleapis.com/auth/cloud-platform",
				"https://www.googleapis.com/auth/userinfo.email",
			},
			MinTokenLifetime: 15 * time.Minute,
		}
		metaHost, err := metaSrv.Start(ctx)
		if err != nil {
			return nil, errors.Annotate(err, "launching fake GCE metadata server").Err()
		}
		ts.cleanup = append(ts.cleanup, func() { _ = metaSrv.Stop(ctx) })
		// Unfortunately we need to modify the global state, since this is where
		// methods like `metadata.OnGCE` read the metadata server address from.
		for _, env := range []string{"GCE_METADATA_ROOT", "GCE_METADATA_IP", "GCE_METADATA_HOST"} {
			if err := os.Setenv(env, metaHost); err != nil {
				return nil, errors.Annotate(err, "setting env var %q", env).Err()
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
		return nil, errors.Annotate(err, "initializing server").Err()
	}
	if !ts.tm.initialized {
		panic("should not be possible: a successful server init initializes all modules")
	}

	// Let the caller code register routes and other stuff. This is the same thing
	// that server.Main is doing.
	if settings.Init != nil {
		if err := settings.Init(ts.srv); err != nil {
			return nil, errors.Annotate(err, "user-supplied init callback").Err()
		}
	}
	srvCtx := ts.Context()

	// Launch the server and wait until it responds to the health check. Note that
	// the port is already open: our request will just queue up on the listening
	// socket in case the goroutine is slow to start. No extra synchronization is
	// necessary.
	serveDone := make(chan error, 1)
	go func() { serveDone <- ts.srv.Serve() }()

	healthCtx, healthCancel := context.WithTimeout(ctx, time.Minute)
	defer healthCancel()

	logging.Infof(srvCtx, "Waiting for a health check to pass...")
	healthDone := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(
			healthCtx,
			"GET",
			fmt.Sprintf("http://%s/healthz", ts.HTTPAddr()),
			nil,
		)
		if err != nil {
			panic(err) // should be impossible
		}
		resp, err := http.DefaultClient.Do(req)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if err == nil && resp.StatusCode != 200 {
			err = errors.Reason("health check replied with status code %d", resp.StatusCode).Err()
		}
		healthDone <- err
	}()

	serveLoopErr := func(err error) error {
		if err == nil {
			logging.Errorf(srvCtx, "The server was interrupted unexpectedly when starting")
			return errors.Reason("server was interrupted unexpectedly when starting").Err()
		}
		logging.Errorf(srvCtx, "The server failed to start: %s", err)
		return errors.Annotate(err, "server failed to start").Err()
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
				return nil, errors.Annotate(healthErr, "health check error and the server is stuck starting").Err()
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

////////////////////////////////////////////////////////////////////////////////

var testingModuleName = module.RegisterName("go.chromium.org/luci/server/servertest")

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
