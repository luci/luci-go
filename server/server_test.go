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

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gkelogger"

	"go.chromium.org/luci/lucictx"

	clientauth "go.chromium.org/luci/auth"
	clientauthtest "go.chromium.org/luci/auth/integration/authtest"
	"go.chromium.org/luci/auth/integration/localauth"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/settings"

	. "github.com/smartystreets/goconvey/convey"
)

var fakeUser = &auth.User{
	Identity: "user:a@example.com",
	Email:    "a@example.com",
}

var fakeAuthDB = authtest.FakeDB{
	"user:a@example.com": {"group 1", "group 2"},
}

func TestServer(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)

		srv, err := newTestServer(ctx, nil)
		So(err, ShouldBeNil)
		defer srv.cleanup()

		Reset(func() { So(srv.StopBackgroundServing(), ShouldBeNil) })

		Convey("VirtualHost", func() {
			srv.Routes.GET("/test", router.MiddlewareChain{}, func(c *router.Context) {
				c.Writer.Write([]byte("default-router"))
			})
			srv.VirtualHost("test-host.example.com").GET("/test", router.MiddlewareChain{}, func(c *router.Context) {
				c.Writer.Write([]byte("test-host-router"))
			})

			srv.ServeInBackground()

			// Requests with unknown Host header go to the default router.
			resp, err := srv.GetMain("/test", map[string]string{
				"Host": "unknown.example.com",
			})
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, "default-router")

			// Requests with NO Host header go to the default router as well.
			resp, err = srv.GetMain("/test", map[string]string{})
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, "default-router")

			// Requests that match a registered virtual host go to its router.
			resp, err = srv.GetMain("/test", map[string]string{
				"Host": "test-host.example.com",
			})
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, "test-host-router")
		})

		Convey("Logging", func() {
			srv.Routes.GET("/test", router.MiddlewareChain{}, func(c *router.Context) {
				logging.Infof(c.Context, "Info log")
				tc.Add(time.Second)
				logging.Warningf(c.Context, "Warn log")
				c.Writer.WriteHeader(201)
				c.Writer.Write([]byte("Hello, world"))
			})

			srv.ServeInBackground()
			resp, err := srv.GetMain("/test", map[string]string{
				"User-Agent":      "Test-user-agent",
				"X-Forwarded-For": "1.1.1.1,2.2.2.2,3.3.3.3",
			})
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, "Hello, world")

			// Stderr log captures details about the request.
			So(srv.stderr.Last(1), ShouldResemble, []gkelogger.LogEntry{
				{
					Severity: "warning",
					Time:     "1454472307.7",
					RequestInfo: &gkelogger.RequestInfo{
						Method:       "GET",
						URL:          srv.mainAddr + "/test",
						Status:       201,
						RequestSize:  "0",
						ResponseSize: "12", // len("Hello, world")
						UserAgent:    "Test-user-agent",
						RemoteIP:     "2.2.2.2",
						Latency:      "1.000000s",
					},
				},
			})
			// Stdout log captures individual log lines.
			So(srv.stdout.Last(2), ShouldResemble, []gkelogger.LogEntry{
				{
					Severity: "info",
					Message:  "Info log",
					Time:     "1454472306.7",
					Operation: &gkelogger.Operation{
						ID: "6694d2c422acd208a0072939487f6999",
					},
				},
				{
					Severity: "warning",
					Message:  "Warn log",
					Time:     "1454472307.7",
					Operation: &gkelogger.Operation{
						ID: "6694d2c422acd208a0072939487f6999",
					},
				},
			})
		})

		Convey("Context features", func() {
			So(testContextFeatures(srv.Context), ShouldBeNil)
			srv.Routes.GET("/request", router.MiddlewareChain{}, func(c *router.Context) {
				if err := testContextFeatures(c.Context); err != nil {
					http.Error(c.Writer, err.Error(), 500)
				}
			})
			srv.ServeInBackground()
			_, err := srv.GetMain("/request", nil)
			So(err, ShouldBeNil)
		})

		Convey("RunInBackground", func() {
			// Queue one activity before starting the serving loop to verify this code
			// path works.
			type nameErrPair struct {
				name string
				err  error
			}
			activities := make(chan nameErrPair, 2)
			srv.RunInBackground("background 1", func(ctx context.Context) {
				activities <- nameErrPair{"background 1", testContextFeatures(ctx)}
			})

			srv.ServeInBackground()

			// Run one more activity after starting the serving loop.
			srv.RunInBackground("background 2", func(ctx context.Context) {
				activities <- nameErrPair{"background 2", testContextFeatures(ctx)}
			})

			wait := func() {
				select {
				case pair := <-activities:
					if pair.err != nil {
						t.Errorf("Activity %q:\n%s", pair.name, strings.Join(errors.RenderStack(pair.err), "\n"))
					}
				case <-time.After(10 * time.Second):
					panic("timeout")
				}
			}

			// Verify both activities have successfully ran.
			wait()
			wait()
		})

		Convey("Client auth", func() {
			srv.Routes.GET("/client-auth", router.MiddlewareChain{}, func(c *router.Context) {
				scopes := strings.Split(c.Request.Header.Get("Ask-Scope"), " ")
				ts, err := auth.GetTokenSource(c.Context, auth.AsSelf, auth.WithScopes(scopes...))
				if err != nil {
					http.Error(c.Writer, err.Error(), 500)
					return
				}
				tok, err := ts.Token()
				if err != nil {
					http.Error(c.Writer, err.Error(), 500)
				} else {
					c.Writer.Write([]byte(tok.AccessToken))
				}
			})

			call := func(scope string) string {
				resp, err := srv.GetMain("/client-auth", map[string]string{"Ask-Scope": scope})
				So(err, ShouldBeNil)
				// If something is really-really broken, the test can theoretically
				// pick up *real* LUCI_CONTEXT auth and somehow see real tokens. This
				// is unlikely (if anything, scopes like "A" are not valid). But if
				// this happens, make sure not to log such tokens.
				if !strings.HasPrefix(resp, "fake_token_") {
					t.Fatalf("Not a fake token! Refusing to log it and exiting.")
				}
				return resp
			}

			srv.ServeInBackground()

			So(call("A B"), ShouldEqual, "fake_token_1")
			So(call("B C"), ShouldEqual, "fake_token_2")
			So(call("A B"), ShouldEqual, "fake_token_1") // reused the cached token

			// 0-th token is generated during startup in initAuth() to test creds.
			So(srv.tokens.TokenScopes("fake_token_0"), ShouldResemble, DefaultOAuthScopes)
			// Tokens generated via calls above.
			So(srv.tokens.TokenScopes("fake_token_1"), ShouldResemble, []string{"A", "B"})
			So(srv.tokens.TokenScopes("fake_token_2"), ShouldResemble, []string{"B", "C"})
		})

		Convey("Auth state", func(c C) {
			authn := auth.Authenticator{
				Methods: []auth.Method{
					authtest.FakeAuth{User: fakeUser},
				},
			}
			mw := router.NewMiddlewareChain(authn.GetMiddleware())
			srv.Routes.GET("/auth-state", mw, func(rc *router.Context) {
				state := auth.GetState(rc.Context)
				c.So(state.DB(), ShouldEqual, fakeAuthDB)
				c.So(state.PeerIdentity(), ShouldEqual, fakeUser.Identity)
				c.So(state.PeerIP().String(), ShouldEqual, "2.2.2.2")
				c.So(auth.CurrentUser(rc.Context), ShouldEqual, fakeUser)
				c.So(auth.CurrentIdentity(rc.Context), ShouldEqual, fakeUser.Identity)
				yes, err := auth.IsMember(rc.Context, "group 1")
				c.So(err, ShouldBeNil)
				c.So(yes, ShouldBeTrue)
			})
			srv.ServeInBackground()
			_, err := srv.GetMain("/auth-state", map[string]string{
				"X-Forwarded-For": "1.1.1.1,2.2.2.2,3.3.3.3",
			})
			So(err, ShouldBeNil)
		})
	})
}

// testContextFeatures check that the context has all subsystems enabled.
func testContextFeatures(ctx context.Context) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = errors.Reason("Panic: %s", p).Err()
		}
	}()

	// Secrets work.
	switch s, err := secrets.GetSecret(ctx, "secret_name"); {
	case err != nil:
		return errors.Annotate(err, "secrets").Err()
	case len(s.Current) == 0:
		return errors.Reason("unexpectedly got empty secret").Err()
	}

	// Settings work.
	var val struct{ Val string }
	switch err := settings.Get(ctx, "settings-key", &val); {
	case err != nil:
		return errors.Annotate(err, "settings").Err()
	case val.Val != "settings-value":
		return errors.Reason("unexpectedly got wrong value %q", val.Val).Err()
	}

	// Client auth works (a test for advanced features is in TestServer).
	ts, err := auth.GetTokenSource(ctx, auth.AsSelf, auth.WithScopes("A", "B"))
	if err != nil {
		return errors.Annotate(err, "token source").Err()
	}
	switch tok, err := ts.Token(); {
	case err != nil:
		return errors.Annotate(err, "token").Err()
	case tok.AccessToken != "fake_token_1":
		// Refuse to log tokens that appear like a real ones (in case the test is
		// totally failing and picking up real credentials).
		if strings.HasPrefix(tok.AccessToken, "fake_token_") {
			return errors.Reason("unexpected token %q", tok.AccessToken).Err()
		}
		return errors.Reason("unexpected token that looks like a real one").Err()
	}

	// AuthDB is available (a test for advanced features is in TestServer).
	switch state := auth.GetState(ctx); {
	case state == nil:
		return errors.Reason("auth.State unexpectedly nil").Err()
	case !reflect.DeepEqual(state.DB(), fakeAuthDB): // these are maps
		return errors.Reason("unexpected auth.DB %v", state.DB()).Err()
	}

	// Redis pool is available.
	if p := redisconn.GetPool(ctx); p == nil {
		return errors.Reason("redis is not available").Err()
	}

	// Datastore is available.
	type testEntity struct {
		ID   int64 `gae:"$id"`
		Body string
	}
	if err := datastore.Put(ctx, &testEntity{ID: 123, Body: "Hi"}); err != nil {
		return errors.Annotate(err, "datastore").Err()
	}

	return nil
}

func TestOptions(t *testing.T) {
	t.Parallel()

	Convey("With temp dir", t, func() {
		tmpDir, err := ioutil.TempDir("", "luci-server-test")
		So(err, ShouldBeNil)
		Reset(func() { os.RemoveAll(tmpDir) })

		Convey("AuthDBPath works", func(c C) {
			body := `groups {
				name: "group"
				members: "user:a@example.com"
			}`

			opts := Options{AuthDBPath: filepath.Join(tmpDir, "authdb.textpb")}
			So(ioutil.WriteFile(opts.AuthDBPath, []byte(body), 0600), ShouldBeNil)

			testRequestHandler(&opts, func(rc *router.Context) {
				db := auth.GetState(rc.Context).DB()
				yes, err := db.IsMember(rc.Context, "user:a@example.com", []string{"group"})
				c.So(err, ShouldBeNil)
				c.So(yes, ShouldBeTrue)
			})
		})
	})
}

func BenchmarkServer(b *testing.B) {
	srv, err := newTestServer(context.Background(), nil)
	if err != nil {
		b.Fatal(err)
	}
	defer srv.cleanup()

	// The route we are going to hit from the benchmark.
	srv.Routes.GET("/test", router.MiddlewareChain{}, func(c *router.Context) {
		var val struct{ Val string }
		logging.Infof(c.Context, "Hello, world")
		secrets.GetSecret(c.Context, "key-name") // e.g. checking XSRF token
		settings.Get(c.Context, "settings-key", &val)
		for i := 0; i < 10; i++ {
			// E.g. calling bunch of Cloud APIs.
			ts, _ := auth.GetTokenSource(c.Context, auth.AsSelf, auth.WithScopes("A", "B", "C"))
			ts.Token()
		}
		c.Writer.Write([]byte("Hello, world"))
	})

	// Don't actually store logs and tokens from all many-many iterations of
	// the loop below.
	srv.stdout.discard = true
	srv.stderr.discard = true
	srv.tokens.KeepRecord = false

	// Launch the server and wait for it to start serving to make sure all guts
	// are initialized.
	srv.ServeInBackground()
	defer srv.StopBackgroundServing()

	// Actual benchmark loop. Note that we bypass network layer here completely
	// (by not using http.DefaultClient).
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		req, err := http.NewRequest("GET", "/test", nil)
		if err != nil {
			b.Fatal(err)
		}
		req.Header.Set("X-Forwarded-For", "1.1.1.1,2.2.2.2,3.3.3.3")
		rr := httptest.NewRecorder()
		srv.Routes.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			b.Fatalf("unexpected status %d", rr.Code)
		}
	}
}

////////////////////////////////////////////////////////////////////////////////

type testServer struct {
	*Server

	stdout logsRecorder
	stderr logsRecorder

	tokens clientauthtest.FakeTokenGenerator

	mainAddr  string
	adminAddr string

	cleanup  func()
	serveErr errorEvent
	serving  int32
}

func newTestServer(ctx context.Context, o *Options) (srv *testServer, err error) {
	srv = &testServer{
		serveErr: errorEvent{signal: make(chan struct{})},
		tokens: clientauthtest.FakeTokenGenerator{
			KeepRecord: true,
		},
	}

	// Run the server in the fake LUCI_CONTEXT auth context, so almost all auth
	// code paths are exercised, but we still use fake tokens.
	authSrv := localauth.Server{
		TokenGenerators: map[string]localauth.TokenGenerator{
			"authtest": &srv.tokens,
		},
		DefaultAccountID: "authtest",
	}
	la, err := authSrv.Start(ctx)
	if err != nil {
		return nil, err
	}
	ctx = lucictx.SetLocalAuth(ctx, la)

	// Contortions to cleanup on all possible failures.
	var cleanup []*os.File
	doCleanup := func() {
		authSrv.Stop(ctx)
		for _, f := range cleanup {
			os.Remove(f.Name())
		}
	}
	defer func() {
		if err != nil {
			doCleanup()
		} else {
			srv.cleanup = doCleanup
		}
	}()

	tmpSecret, err := tempJSONFile(&secrets.Secret{Current: []byte("test secret")})
	if err != nil {
		return nil, err
	}
	cleanup = append(cleanup, tmpSecret)

	tmpSettings, err := tempJSONFile(map[string]interface{}{
		"settings-key": map[string]string{
			"Val": "settings-value",
		},
	})
	if err != nil {
		return nil, err
	}
	cleanup = append(cleanup, tmpSettings)

	var opts Options
	if o != nil {
		opts = *o
	}

	opts.Prod = true
	opts.HTTPAddr = "main_addr"
	opts.AdminAddr = "admin_addr"
	opts.RootSecretPath = tmpSecret.Name()
	opts.SettingsPath = tmpSettings.Name()
	opts.ClientAuth = clientauth.Options{Method: clientauth.LUCIContextMethod}
	opts.RedisAddr = "localhost:99999999" // doesn't matter, we won't actually dial it
	opts.LimiterMaxConcurrentRPCs = 100000

	opts.testCtx = ctx
	opts.testSeed = 1
	opts.testStdout = &srv.stdout
	opts.testStderr = &srv.stderr

	if opts.AuthDBPath == "" {
		opts.testAuthDB = fakeAuthDB
	}

	// Bind to auto-assigned ports.
	opts.testListeners = map[string]net.Listener{
		"main_addr":  setupListener(),
		"admin_addr": setupListener(),
	}

	if srv.Server, err = New(opts); err != nil {
		return nil, err
	}

	// TODO(vadimsh): This really should be memory.UseDS (which doesn't exist),
	// since only Datastore is implemented outside of GAE. It doesn't matter
	// for this particular test though. Note that memory.Use overrides our mocked
	// logger, but we need it. Bring it back.
	srv.Context = logging.SetFactory(memory.Use(srv.Context), logging.GetFactory(srv.Context))

	mainPort := srv.Options.testListeners["main_addr"].Addr().(*net.TCPAddr).Port
	srv.mainAddr = fmt.Sprintf("http://127.0.0.1:%d", mainPort)

	adminPort := srv.Options.testListeners["admin_addr"].Addr().(*net.TCPAddr).Port
	srv.adminAddr = fmt.Sprintf("http://127.0.0.1:%d", adminPort)

	return srv, nil
}

func (s *testServer) ServeInBackground() {
	go func() { s.serveErr.Set(s.ListenAndServe()) }()

	// Wait until both HTTP endpoints are serving before returning.
	if _, err := s.GetMain(healthEndpoint, nil); err != nil {
		panic(err)
	}
	if _, err := s.GetAdmin(healthEndpoint, nil); err != nil {
		panic(err)
	}

	atomic.StoreInt32(&s.serving, 1)
}

func (s *testServer) StopBackgroundServing() error {
	if atomic.LoadInt32(&s.serving) == 1 {
		s.Shutdown()
		return s.serveErr.Get()
	}
	return nil
}

// GetMain makes a blocking request to the main serving port, aborting it if
// the server dies.
func (s *testServer) GetMain(uri string, headers map[string]string) (string, error) {
	return s.get(s.mainAddr+uri, headers)
}

// GetAdmin makes a blocking request to the admin port, aborting it if
// the server dies.
func (s *testServer) GetAdmin(uri string, headers map[string]string) (string, error) {
	return s.get(s.adminAddr+uri, headers)
}

// get makes a blocking request, aborting it if the server dies.
func (s *testServer) get(uri string, headers map[string]string) (resp string, err error) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var req *http.Request
		if req, err = http.NewRequest("GET", uri, nil); err != nil {
			return
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		req.Host = headers["Host"] // req.Host (even when empty) overrides req.Header["Host"]
		var res *http.Response
		if res, err = http.DefaultClient.Do(req); err != nil {
			return
		}
		defer res.Body.Close()
		var blob []byte
		if blob, err = ioutil.ReadAll(res.Body); err != nil {
			return
		}
		if res.StatusCode >= 400 {
			err = fmt.Errorf("unexpected status %d", res.StatusCode)
		}
		resp = string(blob)
	}()

	select {
	case <-s.serveErr.signal:
		err = s.serveErr.Get()
	case <-done:
	}
	return
}

////////////////////////////////////////////////////////////////////////////////

// testRequestHandler launches a new server, calls the given callback as a
// request handler, kills the server.
//
// Useful for testing how server options influence request handler environment.
func testRequestHandler(o *Options, handler func(rc *router.Context)) {
	ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)

	srv, err := newTestServer(ctx, o)
	So(err, ShouldBeNil)
	defer srv.cleanup()

	srv.ServeInBackground()
	defer srv.StopBackgroundServing()

	srv.Routes.GET("/test", router.MiddlewareChain{}, handler)
	_, err = srv.GetMain("/test", nil)
	So(err, ShouldBeNil)
}

////////////////////////////////////////////////////////////////////////////////

func tempJSONFile(body interface{}) (out *os.File, err error) {
	var f *os.File
	defer func() {
		if f != nil && err != nil {
			os.Remove(f.Name())
		}
	}()
	f, err = ioutil.TempFile("", "luci-server-test")
	if err != nil {
		return nil, err
	}
	if err := json.NewEncoder(f).Encode(body); err != nil {
		return nil, err
	}
	return f, f.Close()
}

func setupListener() net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	return l
}

////////////////////////////////////////////////////////////////////////////////

type errorEvent struct {
	err    atomic.Value
	signal chan struct{} // closed after 'err' is populated
}

func (e *errorEvent) Set(err error) {
	if err != nil {
		e.err.Store(err)
	}
	close(e.signal)
}

func (e *errorEvent) Get() error {
	<-e.signal
	err, _ := e.err.Load().(error)
	return err
}

////////////////////////////////////////////////////////////////////////////////

type logsRecorder struct {
	discard bool
	m       sync.Mutex
	logs    []gkelogger.LogEntry
}

func (r *logsRecorder) Write(e *gkelogger.LogEntry) {
	if r.discard {
		return
	}

	// opencensus.io/trace generates random trace and span IDs. Scrub them.
	e.TraceID = ""
	e.SpanID = ""

	r.m.Lock()
	r.logs = append(r.logs, *e)
	r.m.Unlock()
}

func (r *logsRecorder) Last(n int) []gkelogger.LogEntry {
	entries := make([]gkelogger.LogEntry, n)
	r.m.Lock()
	copy(entries, r.logs[len(r.logs)-n:])
	r.m.Unlock()
	return entries
}
