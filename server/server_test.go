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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gkelogger"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"

	. "github.com/smartystreets/goconvey/convey"
)

func TestServer(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)

		srv, err := newTestServer(ctx)
		So(err, ShouldBeNil)
		defer srv.cleanup()
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
						ID: "9566c74d10037c4d7bbb0407d1e2c649",
					},
				},
				{
					Severity: "warning",
					Message:  "Warn log",
					Time:     "1454472307.7",
					TraceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Operation: &gkelogger.Operation{
						ID: "9566c74d10037c4d7bbb0407d1e2c649",
					},
				},
			})
		})

		Convey("Secrets", func() {
			srv.Routes.GET("/secret", router.MiddlewareChain{}, func(c *router.Context) {
				s, err := secrets.GetSecret(c.Context, "secret_name")
				if err != nil {
					c.Writer.WriteHeader(500)
				} else {
					c.Writer.Write([]byte(s.Current))
				}
			})
			resp, err := srv.Get("/secret", nil)
			So(err, ShouldBeNil)
			So(resp, ShouldNotBeEmpty)
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
		logging.Infof(c.Context, "Hello, world")
		secrets.GetSecret(c.Context, "key-name") // e.g. checking XSRF token
		c.Writer.Write([]byte("Hello, world"))
	})

	// Don't actually store logs from all many-many iterations of the loop below.
	srv.stdout.discard = true
	srv.stderr.discard = true

	// Launch the server and wait for it to start serving to make sure all guts
	// are initialized.
	srv.ServeInBackground()
	defer srv.StopBackgroundServing()
	if _, err = srv.Get("/health", nil); err != nil {
		b.Fatal(err)
	}

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

	mainAddr string
	cleanup  func()
	serveErr errorEvent
}

func newTestServer(ctx context.Context) (*testServer, error) {
	srv := &testServer{
		serveErr: errorEvent{signal: make(chan struct{})},
	}

	tmpSecret, err := tempSecret()
	if err != nil {
		return nil, err
	}
	srv.cleanup = func() { os.Remove(tmpSecret.Name()) }

	srv.Server = New(Options{
		Prod:           true,
		HTTPAddr:       "main_addr",
		AdminAddr:      "admin_addr",
		RootSecretPath: tmpSecret.Name(),

		testCtx:    ctx,
		testSeed:   1,
		testStdout: &srv.stdout,
		testStderr: &srv.stderr,

		// Bind to auto-assigned ports.
		testListeners: map[string]net.Listener{
			"main_addr":  setupListener(),
			"admin_addr": setupListener(),
		},
	})

	mainPort := srv.opts.testListeners["main_addr"].Addr().(*net.TCPAddr).Port
	srv.mainAddr = fmt.Sprintf("http://127.0.0.1:%d", mainPort)

	return srv, nil
}

func (s *testServer) ServeInBackground() {
	go func() { s.serveErr.Set(s.ListenAndServe()) }()
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

func tempSecret() (out *os.File, err error) {
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
	secret := secrets.Secret{Current: []byte("test secret")}
	if err := json.NewEncoder(f).Encode(&secret); err != nil {
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
