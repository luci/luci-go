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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gkelogger"

	"go.chromium.org/luci/lucictx"

	clientauth "go.chromium.org/luci/auth"
	clientauthtest "go.chromium.org/luci/auth/integration/authtest"
	"go.chromium.org/luci/auth/integration/localauth"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
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

		srv, err := newTestServer(ctx)
		So(err, ShouldBeNil)
		defer srv.cleanup()

		// Run one activity before starting the serving loop to verify this code
		// path works. See also "RunInBackground" convey below.
		activities := make(chan string, 2)
		srv.RunInBackground("background 1", func(context.Context) {
			activities <- "background 1"
		})

		srv.ServeInBackground()
		Reset(func() { So(srv.StopBackgroundServing(), ShouldBeNil) })

		Convey("Logging", func() {
			srv.Routes.GET("/test", router.MiddlewareChain{}, func(c *router.Context) {
				logging.Infof(c.Context, "Info log")
				tc.Add(time.Second)
				logging.Warningf(c.Context, "Warn log")
				c.Writer.WriteHeader(201)
				c.Writer.Write([]byte("Hello, world"))
			})

			resp, err := srv.Get("/test", map[string]string{
				"User-Agent":            "Test-user-agent",
				"X-Cloud-Trace-Context": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/00001;trace=TRUE",
				"X-Forwarded-For":       "1.1.1.1,2.2.2.2,3.3.3.3",
			})
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, "Hello, world")

			// Stderr log captures details about the request.
			So(srv.stderr.Last(1), ShouldResemble, []gkelogger.LogEntry{
				{
					Severity: "warning",
					Time:     "1454472307.7",
					TraceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
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
					TraceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Operation: &gkelogger.Operation{
						ID: "6694d2c422acd208a0072939487f6999",
					},
				},
				{
					Severity: "warning",
					Message:  "Warn log",
					Time:     "1454472307.7",
					TraceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Operation: &gkelogger.Operation{
						ID: "6694d2c422acd208a0072939487f6999",
					},
				},
			})
		})

		Convey("Secrets", func() {
			srv.Routes.GET("/secret", router.MiddlewareChain{}, func(c *router.Context) {
				s, err := secrets.GetSecret(c.Context, "secret_name")
				if err != nil {
					http.Error(c.Writer, err.Error(), 500)
				} else {
					c.Writer.Write([]byte(s.Current))
				}
			})
			resp, err := srv.Get("/secret", nil)
			So(err, ShouldBeNil)
			So(resp, ShouldNotBeEmpty)
		})

		Convey("Settings", func() {
			srv.Routes.GET("/settings", router.MiddlewareChain{}, func(c *router.Context) {
				var val struct{ Val string }
				if err := settings.Get(c.Context, "settings-key", &val); err != nil {
					http.Error(c.Writer, err.Error(), 500)
				} else {
					c.Writer.Write([]byte(val.Val))
				}
			})
			resp, err := srv.Get("/settings", nil)
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, "settings-value")
		})

		Convey("RunInBackground", func() {
			// Run one more activity after starting the serving loop.
			srv.RunInBackground("background 2", func(context.Context) {
				activities <- "background 2"
			})

			s := stringset.New(2)
			wait := func() {
				select {
				case name := <-activities:
					s.Add(name)
				case <-time.After(10 * time.Second):
					panic("timeout")
				}
			}

			// Verify both activities have started (this hangs otherwise).
			wait()
			wait()
			So(s.Len(), ShouldEqual, 2)
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
				resp, err := srv.Get("/client-auth", map[string]string{"Ask-Scope": scope})
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

			So(call("A B"), ShouldEqual, "fake_token_1")
			So(call("B C"), ShouldEqual, "fake_token_2")
			So(call("A B"), ShouldEqual, "fake_token_1") // reused the cached token

			// 0-th token is generated during startup in initAuth() to test creds.
			So(srv.tokens.TokenScopes("fake_token_0"), ShouldResemble, []string{clientauth.OAuthScopeEmail})
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
			_, err := srv.Get("/auth-state", map[string]string{
				"X-Forwarded-For": "1.1.1.1,2.2.2.2,3.3.3.3",
			})
			So(err, ShouldBeNil)
		})
	})
}

func BenchmarkServer(b *testing.B) {
	srv, err := newTestServer(context.Background())
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
		req.Header.Set("X-Cloud-Trace-Context", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/00001;trace=TRUE")
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

	mainAddr string
	cleanup  func()
	serveErr errorEvent
}

func newTestServer(ctx context.Context) (srv *testServer, err error) {
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

	srv.Server = New(Options{
		Prod:           true,
		HTTPAddr:       "main_addr",
		AdminAddr:      "admin_addr",
		RootSecretPath: tmpSecret.Name(),
		SettingsPath:   tmpSettings.Name(),
		ClientAuth:     clientauth.Options{Method: clientauth.LUCIContextMethod},

		testCtx:    ctx,
		testSeed:   1,
		testStdout: &srv.stdout,
		testStderr: &srv.stderr,

		// Bind to auto-assigned ports.
		testListeners: map[string]net.Listener{
			"main_addr":  setupListener(),
			"admin_addr": setupListener(),
		},

		testAuthDB: fakeAuthDB,
	})

	mainPort := srv.opts.testListeners["main_addr"].Addr().(*net.TCPAddr).Port
	srv.mainAddr = fmt.Sprintf("http://127.0.0.1:%d", mainPort)

	return srv, nil
}

func (s *testServer) ServeInBackground() {
	go func() { s.serveErr.Set(s.ListenAndServe()) }()
	if _, err := s.Get("/health", nil); err != nil {
		panic(err)
	}
}

func (s *testServer) StopBackgroundServing() error {
	s.Shutdown()
	return s.serveErr.Get()
}

// Get makes a blocking request, aborting it if the server dies.
func (s *testServer) Get(uri string, headers map[string]string) (resp string, err error) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var req *http.Request
		if req, err = http.NewRequest("GET", s.mainAddr+uri, nil); err != nil {
			return
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
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
