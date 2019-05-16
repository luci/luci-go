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
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/caching/cacheContext"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gkelogger"
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

	testCtx       context.Context          // base context for tests
	testSeed      int64                    // used to seed rng in tests
	testStdout    gkelogger.LogEntryWriter // mocks stdout in tests
	testStderr    gkelogger.LogEntryWriter // mocks stderr in tests
	testListeners map[string]net.Listener  // addr => net.Listener, for tests
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

	ctx  context.Context // the root server context, holds all global state
	opts Options         // options passed to New

	stdout gkelogger.LogEntryWriter // for logging to stdout, nil in dev mode
	stderr gkelogger.LogEntryWriter // for logging to stderr, nil in dev mode

	m       sync.Mutex     // protects fields below
	httpSrv []*http.Server // all registered HTTP servers
	started bool           // true inside and after ListenAndServe
	stopped bool           // true inside and after Shutdown
	done    chan struct{}  // closed after Shutdown returns

	rndM sync.Mutex // protects rnd
	rnd  *rand.Rand // used to generate trace and operation IDs
}

// New constructs a new server instance.
//
// It hosts one or more HTTP servers and starts and stops them in unison. It is
// also responsible for preparing contexts for incoming requests.
func New(opts Options) *Server {
	seed := opts.testSeed
	if seed == 0 {
		if err := binary.Read(cryptorand.Reader, binary.BigEndian, &seed); err != nil {
			panic(err)
		}
	}

	srv := &Server{
		ctx:  context.Background(),
		opts: opts,
		done: make(chan struct{}),
		rnd:  rand.New(rand.NewSource(seed)),
	}

	if opts.testCtx != nil {
		srv.ctx = opts.testCtx
	}

	// When running in production use the ugly looking JSON format that is hard to
	// read by humans but which is parsed by google-fluentd.
	//
	// To support per-request log grouping in Stackdriver Logs UI emit two log
	// streams:
	//  * stderr: top-level HTTP requests (conceptually "200 GET /path").
	//  * stdout: all application logs, correlated with HTTP logs in a particular
	//    way (see the link below).
	//
	// This technique is primarily intended for GAE Flex, but it works anywhere:
	// https://cloud.google.com/appengine/articles/logging#linking_app_logs_and_requests
	//
	// Stderr stream is also used to log all global activities that happens
	// outside of any request handler (stuff like initialization, shutdown,
	// background goroutines, etc).
	//
	// In non-production mode use the human-friendly format and a single log
	// stream.
	if opts.Prod {
		if opts.testStdout != nil {
			srv.stdout = opts.testStdout
		} else {
			srv.stdout = &gkelogger.Sink{Out: os.Stdout}
		}
		if opts.testStderr != nil {
			srv.stderr = opts.testStderr
		} else {
			srv.stderr = &gkelogger.Sink{Out: os.Stderr}
		}
		srv.ctx = logging.SetFactory(srv.ctx,
			gkelogger.Factory(srv.stderr, gkelogger.LogEntry{
				Operation: &gkelogger.Operation{
					ID: srv.genUniqueID(32), // correlate all global server logs together
				},
			}),
		)
	} else {
		srv.ctx = gologger.StdConfig.Use(srv.ctx)
	}

	srv.ctx = caching.WithProcessCacheData(srv.ctx, caching.NewProcessCacheData())

	// TODO(vadimsh): Install settings via settings.Use(...).
	// TODO(vadimsh): Install secrets via secrets.Set(...).
	// TODO(vadimsh): Install auth via auth.Initialize(...).

	srv.Routes = srv.RegisterHTTP(opts.HTTPAddr)

	// Health check/readiness probe endpoint.
	srv.Routes.GET("/health", router.MiddlewareChain{}, func(c *router.Context) {
		c.Writer.Write([]byte("OK"))
	})

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

// genUniqueID returns pseudo-random hex string of given even length.
func (s *Server) genUniqueID(l int) string {
	b := make([]byte, l/2)
	s.rndM.Lock()
	s.rnd.Read(b)
	s.rndM.Unlock()
	return hex.EncodeToString(b)
}

// rootMiddleware prepares the per-request context.
func (s *Server) rootMiddleware(c *router.Context, next router.Handler) {
	// Either grab Trace ID from X-Cloud-Trace-Context header or generate a new
	// random one. By using X-Cloud-Trace-Context we tell Stackdriver to associate
	// logs with existing traces. This header may be set by Google HTTPs load
	// balancer itself, see https://cloud.google.com/load-balancing/docs/https.
	//
	// TraceID is also used to group log lines together in Stackdriver UI, even if
	// such trace doesn't actually exist.
	traceID := getCloudTraceID(c.Request)
	if traceID == "" {
		traceID = s.genUniqueID(32)
	}

	// Track how many response bytes are sent and what status is set.
	rw := iotools.NewResponseWriter(c.Writer)
	c.Writer = rw

	// Observe maximum emitted severity to use it as an overall severity for the
	// request log entry.
	severityTracker := gkelogger.SeverityTracker{Out: s.stdout}

	// Log the overall request information when the request finishes. Use TraceID
	// to correlate this log entry with entries emitted by the request handler
	// below.
	started := clock.Now(s.ctx)
	defer func() {
		now := clock.Now(s.ctx)
		entry := gkelogger.LogEntry{
			Severity: severityTracker.MaxSeverity(),
			Time:     gkelogger.FormatTime(now),
			TraceID:  traceID,
			RequestInfo: &gkelogger.RequestInfo{
				Method:       c.Request.Method,
				URL:          getRequestURL(c.Request),
				Status:       rw.Status(),
				RequestSize:  fmt.Sprintf("%d", c.Request.ContentLength),
				ResponseSize: fmt.Sprintf("%d", rw.ResponseSize()),
				UserAgent:    c.Request.UserAgent(),
				RemoteIP:     getRemoteIP(c.Request),
				Latency:      fmt.Sprintf("%fs", now.Sub(started).Seconds()),
			},
		}
		if s.opts.Prod {
			s.stderr.Write(&entry)
		} else {
			logging.Infof(s.ctx, "%d %s %q (%s)",
				entry.RequestInfo.Status,
				entry.RequestInfo.Method,
				entry.RequestInfo.URL,
				entry.RequestInfo.Latency,
			)
		}
	}()

	ctx, cancel := context.WithTimeout(s.ctx, time.Minute)
	defer cancel()

	// Make the request logger emit log entries with same TraceID.
	if s.opts.Prod {
		ctx = logging.SetFactory(ctx, gkelogger.Factory(&severityTracker, gkelogger.LogEntry{
			TraceID:   traceID,
			Operation: &gkelogger.Operation{ID: s.genUniqueID(32)},
		}))
	}

	ctx = caching.WithRequestCache(ctx)

	c.Context = cacheContext.Wrap(ctx)
	next(c)
}

// getCloudTraceID extract Trace ID from X-Cloud-Trace-Context header.
func getCloudTraceID(r *http.Request) string {
	// hdr has form "<trace-id>/<span-id>;<trace-options>".
	hdr := r.Header.Get("X-Cloud-Trace-Context")
	idx := strings.IndexByte(hdr, '/')
	if idx == -1 {
		return ""
	}
	return hdr[:idx]
}

// getRemoteIP extracts end-user IP address from X-Forwarded-For header.
func getRemoteIP(r *http.Request) string {
	// X-Forwarded-For header is set by Cloud Load Balancer and has format:
	//   [<untrusted part>,]<IP that connected to LB>,<unimportant>[,<more>].
	//
	// <untrusted part> is present if the original request from the Internet comes
	// with X-Forwarded-For header. We can't trust IPs specified there. We assume
	// Cloud Load Balancer sanitizes the format of this field though.
	//
	// <IP that connected to LB> is what we are after.
	//
	// <unimportant> is "global forwarding rule external IP" which we don't care
	// about.
	//
	// <more> is present only if we proxy the request through more layers of
	// load balancers *while it is already inside GKE cluster*. We assume we don't
	// do that (if we ever do, Options{...} should be extended with a setting that
	// specifies how many layers of load balancers to skip to get to the original
	// IP).
	//
	// See https://cloud.google.com/load-balancing/docs/https for more info.
	forwardedFor := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	if len(forwardedFor) >= 2 {
		return forwardedFor[len(forwardedFor)-2]
	}

	// Fallback to the peer IP if X-Forwarded-For is not set. Happens when
	// connecting to the server's port directly from within the cluster.
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "0.0.0.0"
	}
	return ip
}

// getRequestURL reconstructs original request URL to log it (best effort).
func getRequestURL(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto != "https" {
		proto = "http"
	}
	host := r.Host
	if r.Host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s://%s%s", proto, host, r.RequestURI)
}
