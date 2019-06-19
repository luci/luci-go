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
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/caching/cacheContext"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gkelogger"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/signals"

	clientauth "go.chromium.org/luci/auth"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/portal"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/settings"
)

// Options are exposed as command line flags.
type Options struct {
	Prod           bool               // must be set when running in production
	HTTPAddr       string             // address to bind the main listening socket to
	AdminAddr      string             // address to bind the admin socket to
	RootSecretPath string             // path to a JSON file with the root secret key
	SettingsPath   string             // path to a JSON file with app settings
	ClientAuth     clientauth.Options // base settings for client auth options
	TokenCacheDir  string             // where to cache auth tokens (optional)

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
	f.StringVar(&o.RootSecretPath, "root-secret-path", o.RootSecretPath, "Path to a JSON file with the root secret key")
	f.StringVar(&o.SettingsPath, "settings-path", o.SettingsPath, "Path to a JSON file with app settings")
	f.StringVar(
		&o.ClientAuth.ServiceAccountJSONPath,
		"service-account-json",
		o.ClientAuth.ServiceAccountJSONPath,
		"Path to a JSON file with service account private key",
	)
	f.StringVar(
		&o.ClientAuth.ActAsServiceAccount,
		"act-as",
		o.ClientAuth.ActAsServiceAccount,
		"Act as this service account",
	)
	f.StringVar(
		&o.TokenCacheDir,
		"token-cache-dir",
		o.TokenCacheDir,
		"Where to cache auth tokens (optional)",
	)
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
	ready   chan struct{}  // closed right before starting the serving loop
	done    chan struct{}  // closed after Shutdown returns

	rndM sync.Mutex // protects rnd
	rnd  *rand.Rand // used to generate trace and operation IDs

	bgrCtx    context.Context    // root context for background work, canceled in Shutdown
	bgrCancel context.CancelFunc // cancels bgrCtx
	bgrWg     sync.WaitGroup     // waits for runInBackground goroutines to stop

	secrets  *secrets.DerivedStore     // indirectly used to derive XSRF tokens and such
	settings *settings.ExternalStorage // backing store for settings.Get(...) API

	authM        sync.RWMutex
	authPerScope map[string]scopedAuth // " ".join(scopes) => ...
}

// scopedAuth holds TokenSource and Authenticator that produced it.
type scopedAuth struct {
	source oauth2.TokenSource
	authen *clientauth.Authenticator
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

	// Configure base server subsystems by injecting them into the root context
	// inherited later by all requests.
	srv := &Server{
		ctx:          context.Background(),
		opts:         opts,
		ready:        make(chan struct{}),
		done:         make(chan struct{}),
		rnd:          rand.New(rand.NewSource(seed)),
		secrets:      secrets.NewDerivedStore(secrets.Secret{}),
		settings:     &settings.ExternalStorage{},
		authPerScope: map[string]scopedAuth{},
	}
	if opts.testCtx != nil {
		srv.ctx = opts.testCtx
	}
	srv.initLogging()
	srv.ctx = caching.WithProcessCacheData(srv.ctx, caching.NewProcessCacheData())
	srv.ctx = secrets.Set(srv.ctx, srv.secrets)
	if !opts.Prod && opts.SettingsPath == "" {
		// In dev mode use settings backed by memory.
		srv.ctx = settings.Use(srv.ctx, settings.New(&settings.MemoryStorage{}))
	} else {
		// In prod mode use setting backed by a file, see also initSettings().
		srv.ctx = settings.Use(srv.ctx, settings.New(srv.settings))
	}

	// Configure auth subsystem. The rest of the initialization happens in
	// initAuth() later, when launching the server.
	srv.ctx = auth.Initialize(srv.ctx, &auth.Config{
		DBProvider:          nil, // TODO(vadimsh): Implement.
		Signer:              nil, // TODO(vadimsh): Implement.
		AccessTokenProvider: srv.getAccessToken,
		AnonymousTransport:  func(context.Context) http.RoundTripper { return http.DefaultTransport },
		EndUserIP:           getRemoteIP,
		IsDevMode:           !srv.opts.Prod,
	})

	srv.Routes = srv.RegisterHTTP(opts.HTTPAddr)

	// Health check/readiness probe endpoint.
	srv.Routes.GET("/health", router.MiddlewareChain{}, func(c *router.Context) {
		c.Writer.Write([]byte("OK"))
	})

	// Install endpoints accessible through admin port.
	admin := srv.RegisterHTTP(opts.AdminAddr)
	admin.GET("/", router.MiddlewareChain{}, func(c *router.Context) {
		http.Redirect(c.Writer, c.Request, "/admin/portal", http.StatusFound)
	})
	portal.InstallHandlers(admin, router.MiddlewareChain{}, portal.AssumeTrustedPort)

	// Prepare the context used for background work. It is canceled as soon as we
	// enter the shutdown sequence.
	srv.bgrCtx, srv.bgrCancel = context.WithCancel(srv.ctx)

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
		s.Fatal(errors.Reason("the server has already been started").Err())
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

// RunInBackground launches the given callback in a separate goroutine right
// before starting the serving loop.
//
// If the server is already running, launches it right away. If the server
// fails to start, the goroutines will never be launched.
//
// Should be used for background asynchronous activities like reloading configs.
//
// All logs lines emitted by the callback are annotated with "activity" field
// which can be arbitrary, but by convention has format "<namespace>.<name>",
// where "luci" namespace is reserved for internal activities.
//
// The callback receives a context with following services available:
//   * go.chromium.org/luci/common/logging: Logging.
//   * go.chromium.org/luci/server/caching: Process cache.
//   * go.chromium.org/luci/server/secrets: Secrets.
//   * go.chromium.org/luci/server/settings: Access to app settings.
//   * go.chromium.org/luci/server/auth: Making authenticated calls.
//
// The context passed to the callback is canceled when the server is shutting
// down. It is expected the goroutine will exit soon after the context is
// canceled.
func (s *Server) RunInBackground(activity string, f func(context.Context)) {
	s.runInBackground(activity, func(c context.Context) {
		// Block until the server is ready or shutdown.
		select {
		case <-s.ready:
			f(c)
		case <-s.bgrCtx.Done():
			// the server is closed, no need to run f() anymore
		}
	})
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
		s.Fatal(errors.Reason("the server has already been started").Err())
	}

	if err := s.initSecrets(); err != nil {
		logging.WithError(err).Errorf(s.ctx, "Failed to initialize secrets store")
		return errors.Annotate(err, "failed to initialize secrets store").Err()
	}
	if err := s.initSettings(); err != nil {
		logging.WithError(err).Errorf(s.ctx, "Failed to initialize settings")
		return errors.Annotate(err, "failed to initialize settings").Err()
	}
	if err := s.initAuth(); err != nil {
		logging.WithError(err).Errorf(s.ctx, "Failed to initialize auth")
		return errors.Annotate(err, "failed to initialize auth").Err()
	}

	// Catch SIGTERM while inside this function.
	stop := signals.HandleInterrupt(s.Shutdown)
	defer stop()

	// Unblock all pending RunInBackground goroutines, so they can start.
	close(s.ready)

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

	// Tell all runInBackground goroutines to stop.
	s.bgrCancel()

	// Stop all http.Servers in parallel. Each Shutdown call blocks until the
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

	// Wait for all background goroutines to stop.
	s.bgrWg.Wait()

	// Notify ListenAndServe that it can exit now.
	s.stopped = true
	close(s.done)
}

// Fatal logs the error and immediately shuts down the process with exit code 3.
//
// No cleanup is performed. Deferred statements are not run. Not recoverable.
func (s *Server) Fatal(err error) {
	errors.Log(s.ctx, err)
	os.Exit(3)
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
	return errors.Reason("test listener is not set").Err()
}

// runInBackground starts a goroutine that does some background work.
//
// It is expected to exit soon after its context is canceled.
func (s *Server) runInBackground(activity string, f func(context.Context)) {
	ctx := logging.SetField(s.bgrCtx, "activity", activity)
	s.bgrWg.Add(1)
	go func() {
		defer s.bgrWg.Done()
		f(ctx)
	}()
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

// initLogging initializes the server logging.
//
// Called very early during server startup process. Many server fields may not
// be initialized yet, be careful.
//
// When running in production uses the ugly looking JSON format that is hard to
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
// In non-production mode use the human-friendly format and a single log stream.
func (s *Server) initLogging() {
	if !s.opts.Prod {
		s.ctx = gologger.StdConfig.Use(s.ctx)
		return
	}

	if s.opts.testStdout != nil {
		s.stdout = s.opts.testStdout
	} else {
		s.stdout = &gkelogger.Sink{Out: os.Stdout}
	}

	if s.opts.testStderr != nil {
		s.stderr = s.opts.testStderr
	} else {
		s.stderr = &gkelogger.Sink{Out: os.Stderr}
	}

	s.ctx = logging.SetFactory(s.ctx,
		gkelogger.Factory(s.stderr, gkelogger.LogEntry{
			Operation: &gkelogger.Operation{
				ID: s.genUniqueID(32), // correlate all global server logs together
			},
		}),
	)
}

// initSecrets reads the initial root secret and launches a job to periodically
// reread it.
//
// Absence of the secret when the server starts is a fatal error. But if the
// server managed to stars successfully but can't re-read the secret later (e.g.
// the file disappeared), it logs the error and keeps using the cached secret.
func (s *Server) initSecrets() error {
	secret, err := s.readRootSecret()
	if err != nil {
		return err
	}
	s.secrets.SetRoot(secret)
	s.runInBackground("luci.secrets", func(c context.Context) {
		for {
			if r := <-clock.After(c, time.Minute); r.Err != nil {
				return // the context is canceled
			}
			secret, err := s.readRootSecret()
			if err != nil {
				logging.WithError(err).Errorf(c, "Failed to re-read the root secret, using the cached one")
			} else {
				s.secrets.SetRoot(secret)
			}
		}
	})
	return nil
}

// readRootSecret reads the secret from a path specified via -root-secret-path.
func (s *Server) readRootSecret() (secrets.Secret, error) {
	if s.opts.RootSecretPath == "" {
		if !s.opts.Prod {
			return secrets.Secret{Current: []byte("dev-non-secret")}, nil
		}
		return secrets.Secret{}, errors.Reason("-root-secret-path is required when running in prod mode").Err()
	}

	f, err := os.Open(s.opts.RootSecretPath)
	if err != nil {
		return secrets.Secret{}, err
	}
	defer f.Close()

	secret := secrets.Secret{}
	if err = json.NewDecoder(f).Decode(&secret); err != nil {
		return secrets.Secret{}, errors.Annotate(err, "not a valid JSON").Err()
	}
	if len(secret.Current) == 0 {
		return secrets.Secret{}, errors.Reason("`current` field in the root secret is empty, this is not allowed").Err()
	}
	return secret, nil
}

// initSettings reads the initial settings and launches a job to periodically
// reread them.
//
// Does nothing is if -settings-path is not set: settings are optional. If
// -settings-path is set, it must point to a structurally valid JSON file or
// the server will fail to start.
func (s *Server) initSettings() error {
	if s.opts.SettingsPath == "" {
		if s.opts.Prod {
			logging.Infof(s.ctx, "Not using settings, -settings-path is not set")
		}
		return nil
	}
	if err := s.loadSettings(s.ctx); err != nil {
		return err
	}
	s.runInBackground("luci.settings", func(c context.Context) {
		for {
			if r := <-clock.After(c, 30*time.Second); r.Err != nil {
				return // the context is canceled
			}
			if err := s.loadSettings(c); err != nil {
				logging.WithError(err).Errorf(c, "Failed to reload settings, using the cached ones")
			}
		}
	})
	return nil
}

// loadSettings loads settings from a path specified via -settings-path.
func (s *Server) loadSettings(c context.Context) error {
	f, err := os.Open(s.opts.SettingsPath)
	if err != nil {
		return errors.Annotate(err, "failed to open settings file").Err()
	}
	defer f.Close()
	return s.settings.Load(c, f)
}

// initAuth finishes auth system initialization by pre-warming caches and
// verifying auth tokens can actually be minted (i.e. supplied credentials are
// valid).
//
// If this fails, the server refuses to start.
//
// TODO(vadimsh): Do the initial fetch of AuthDB and start a goroutine that
// refreshes it.
func (s *Server) initAuth() error {
	// The default value for ClientAuth.SecretsDir is usually hardcoded to point
	// to where the token cache is located on developer machines (~/.config/...).
	// This location often doesn't exist when running from inside a container.
	// The token cache is also not really needed for production services that use
	// service accounts (they don't need cached refresh tokens). So in production
	// mode totally ignore default ClientAuth.SecretsDir and use whatever was
	// passed as -token-cache-dir. If it is empty (default), then no on-disk token
	// cache is used at all.
	//
	// If -token-cache-dir was explicitly set, always use it (even in dev mode).
	// This is useful when running containers locally: developer's credentials
	// on the host machine can be mounted inside the container.
	if s.opts.Prod || s.opts.TokenCacheDir != "" {
		s.opts.ClientAuth.SecretsDir = s.opts.TokenCacheDir
	}

	// Note: we initialize a token source for one arbitrary set of scopes here. In
	// many practical cases this is sufficient to verify that credentials are
	// valid. For example, when we use service account JSON key, if we can
	// generate a token with *some* scope (meaning Cloud accepted our signature),
	// we can generate tokens with *any* scope, since there's no restrictions on
	// what scopes are accessible to a service account, as long as the private key
	// is valid (which we just verified by generating some token).
	au, err := s.initTokenSource([]string{clientauth.OAuthScopeEmail})
	if err != nil {
		return errors.Annotate(err, "failed to initialize the token source").Err()
	}
	if _, err := au.source.Token(); err != nil {
		return errors.Annotate(err, "failed to mint an initial token").Err()
	}

	// Report who we are running as. Useful when debugging access issues.
	switch email, err := au.authen.GetEmail(); {
	case err == nil:
		logging.Infof(s.ctx, "Running as %s", email)
	case err == clientauth.ErrNoEmail:
		logging.Warningf(s.ctx, "Running as <unknown>, cautiously proceeding...")
	case err != nil:
		return errors.Annotate(err, "failed to check service account email").Err()
	}
	return nil
}

// getAccessToken generates OAuth access tokens to use for requests made by
// the server itself.
//
// It should implement caching internally. This function may be called very
// often, concurrently, from multiple goroutines.
func (s *Server) getAccessToken(c context.Context, scopes []string) (*oauth2.Token, error) {
	key := strings.Join(scopes, " ")

	s.authM.RLock()
	au, ok := s.authPerScope[key]
	s.authM.RUnlock()

	if !ok {
		var err error
		au, err = s.initTokenSource(scopes)
		if err != nil {
			return nil, err
		}
	}

	return au.source.Token()
}

// initTokenSource initializes a token source for a given list of scopes.
//
// If such token source was already initialized, just returns it and its
// parent authenticator.
func (s *Server) initTokenSource(scopes []string) (au scopedAuth, err error) {
	key := strings.Join(scopes, " ")

	s.authM.Lock()
	defer s.authM.Unlock()

	if au, ok := s.authPerScope[key]; ok {
		return au, nil
	}

	// Use ClientAuth as a base template for options, so users of Server{...} have
	// ability to customize various aspects of token generation.
	opts := s.opts.ClientAuth
	opts.Scopes = scopes

	// Note: we are using the root context here (not request-scoped context passed
	// to getAccessToken) because the authenticator *outlives* the request (by
	// being cached), thus it needs a long-living context.
	ctx := logging.SetField(s.ctx, "activity", "auth")
	au.authen = clientauth.NewAuthenticator(ctx, clientauth.SilentLogin, opts)
	au.source, err = au.authen.TokenSource()

	if err != nil {
		// ErrLoginRequired may happen only when running the server locally using
		// developer's credentials. Let them know how the problem can be fixed.
		if !s.opts.Prod && err == clientauth.ErrLoginRequired {
			if opts.ActAsServiceAccount != "" {
				// Per clientauth.Options doc, IAM is the scope required to use
				// ActAsServiceAccount feature.
				scopes = []string{clientauth.OAuthScopeIAM}
			}
			logging.Errorf(s.ctx, "Looks like you run the server locally and it doesn't have credentials for some OAuth scopes")
			logging.Errorf(s.ctx, "Run the following command to set them up: ")
			logging.Errorf(s.ctx, "  $ luci-auth login -scopes %q", strings.Join(scopes, " "))
		}
		return scopedAuth{}, err
	}

	s.authPerScope[key] = au
	return au, nil
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
