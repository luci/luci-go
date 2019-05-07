// Copyright 2019 The LUCI Authors.
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

// Package server implements an environment for running LUCI servers.
package server

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"go.chromium.org/luci/common/data/caching/cacheContext"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/signals"

	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/router"
)

// Options are exposed as command line flags.
type Options struct {
	Prod      bool   // must be set when running in production
	HTTPAddr  string // address to bind the main listening socket to
	AdminAddr string // address to bind the admin socket to

	testListeners map[string]net.Listener // addr => net.Listener, for tests
}

// Register registers the command line flags.
func (o *Options) Register(f *flag.FlagSet) {
	if o.HTTPAddr == "" {
		o.HTTPAddr = "127.0.0.1:8800"
	}
	if o.AdminAddr == "" {
		o.AdminAddr = "127.0.0.1:8900"
	}
	f.BoolVar(&o.Prod, "prod", o.Prod, "Switch the server into production mode")
	f.StringVar(&o.HTTPAddr, "http-addr", o.HTTPAddr, "Address to bind the main listening socket to")
	f.StringVar(&o.AdminAddr, "admin-addr", o.AdminAddr, "Address to bind the admin socket to")
}

// Server is responsible for initializing and launching the serving environment.
//
// Doesn't do TLS. Should be sitting behind a load balancer that terminates
// TLS.
type Server struct {
	Routes *router.Router // HTTP routes exposed via opts.HTTPAddr

	ctx     context.Context // the root server context, holds all global state
	opts    Options         // options passed to New
	m       sync.Mutex      // protects fields below
	httpSrv []*http.Server  // all registered HTTP servers
	started bool            // true inside and after ListenAndServe
	stopped bool            // true inside and after Shutdown
	done    chan struct{}   // closed after Shutdown returns
}

// New constructs a new server instance.
//
// It hosts one or more HTTP servers and starts and stops them in unison. It is
// also responsible for preparing contexts for incoming requests.
func New(opts Options) *Server {
	// TODO(vadimsh): Use JSON format when opts.Prod is true so that fluentd can
	// understand it.
	ctx := gologger.StdConfig.Use(context.Background())
	ctx = caching.WithProcessCacheData(ctx, caching.NewProcessCacheData())

	// TODO(vadimsh): Install settings via settings.Use(...).
	// TODO(vadimsh): Install secrets via secrets.Set(...).
	// TODO(vadimsh): Install auth via auth.Initialize(...).

	srv := &Server{
		ctx:  ctx,
		opts: opts,
		done: make(chan struct{}),
	}
	srv.Routes = srv.RegisterHTTP(opts.HTTPAddr)

	// TODO(vadimsh): Populate admin routes (admin portal, pprof).
	srv.RegisterHTTP(opts.AdminAddr)

	return srv
}

// RegisterHTTP prepares an additional HTTP server.
//
// Can be used to open more listening HTTP ports (in addition to opts.HTTPAddr
// and opts.AdminAddr). Returns a router that should be populated with routes
// exposed through the added server.
//
// Should be called before ListenAndServe (panics otherwise).
func (s *Server) RegisterHTTP(addr string) *router.Router {
	s.m.Lock()
	defer s.m.Unlock()
	if s.started {
		panic("the server has already been started")
	}

	// Setup middleware chain used by ALL requests.
	r := router.New()
	r.Use(router.NewMiddlewareChain(
		s.rootMiddleware,            // prepares the per-request context
		middleware.WithPanicCatcher, // transforms panics into HTTP 500
	))

	s.httpSrv = append(s.httpSrv, &http.Server{
		Addr:     addr,
		Handler:  r,
		ErrorLog: nil, // TODO(vadimsh): Log via 'logging' package.
	})
	return r
}

// ListenAndServe launches the serving loop.
//
// Blocks forever or until the server is stopped via Shutdown (from another
// goroutine or from a SIGTERM handler). Returns nil if the server was shutdown
// correctly or an error if it failed to start or unexpectedly died. The error
// is logged inside.
//
// Should be called only once. Panics otherwise.
func (s *Server) ListenAndServe() error {
	s.m.Lock()
	wasRunning := s.started
	httpSrv := append(make([]*http.Server, 0, len(s.httpSrv)), s.httpSrv...)
	s.started = true
	s.m.Unlock()
	if wasRunning {
		panic("the server has already been started")
	}

	// Catch SIGTERM while inside this function.
	stop := signals.HandleInterrupt(s.Shutdown)
	defer stop()

	// Run serving loops in parallel.
	errs := make(errors.MultiError, len(httpSrv))
	wg := sync.WaitGroup{}
	wg.Add(len(httpSrv))
	for i, srv := range httpSrv {
		logging.Infof(s.ctx, "Serving http://%s", srv.Addr)
		i := i
		srv := srv
		go func() {
			defer wg.Done()
			if err := s.serveLoop(srv); err != http.ErrServerClosed {
				logging.WithError(err).Errorf(s.ctx, "Server at %s failed", srv.Addr)
				errs[i] = err
				s.Shutdown() // close all other servers
			}
		}()
	}
	wg.Wait()

	// Per http.Server docs, we end up here *immediately* after Shutdown call was
	// initiated. Some requests can still be in-flight. We block until they are
	// done (as indicated by Shutdown call itself exiting).
	logging.Infof(s.ctx, "Waiting for the server to stop...")
	<-s.done
	logging.Infof(s.ctx, "The server has stopped")

	if errs.First() != nil {
		return errs
	}
	return nil
}

// Shutdown gracefully stops the server if it was running.
//
// Blocks until the server is stopped. Can be called multiple times.
func (s *Server) Shutdown() {
	s.m.Lock()
	defer s.m.Unlock()
	if s.stopped {
		return
	}

	logging.Infof(s.ctx, "Shutting down the server...")

	// Stop them all in parallel. Each Shutdown call blocks until the
	// corresponding server is stopped.
	wg := sync.WaitGroup{}
	wg.Add(len(s.httpSrv))
	for _, srv := range s.httpSrv {
		srv := srv
		go func() {
			defer wg.Done()
			srv.Shutdown(s.ctx)
		}()
	}
	wg.Wait()

	// Notify ListenAndServe that it can exit now.
	s.stopped = true
	close(s.done)
}

// serveLoop binds the socket and launches the serving loop.
//
// Basically srv.ListenAndServe with some testing helpers.
func (s *Server) serveLoop(srv *http.Server) error {
	// If not running tests, let http.Server bind the socket as usual.
	if s.opts.testListeners == nil {
		return srv.ListenAndServe()
	}
	// In test mode the listener MUST be prepared already.
	if l, _ := s.opts.testListeners[srv.Addr]; l != nil {
		return srv.Serve(l)
	}
	return fmt.Errorf("test listener is not set")
}

// rootMiddleware prepares the per-request context.
func (s *Server) rootMiddleware(c *router.Context, next router.Handler) {
	ctx, cancel := context.WithTimeout(s.ctx, time.Minute)
	defer cancel()

	// TODO(vadimsh): Configure pre-request logger.
	ctx = caching.WithRequestCache(ctx)

	c.Context = cacheContext.Wrap(ctx)
	next(c)
}
