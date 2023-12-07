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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	clientauth "go.chromium.org/luci/auth"
	clientauthtest "go.chromium.org/luci/auth/integration/authtest"
	"go.chromium.org/luci/auth/integration/localauth"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/sdlogger"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/experiments"
	"go.chromium.org/luci/server/internal/testpb"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var fakeUser = &auth.User{
	Identity: "user:a@example.com",
	Email:    "a@example.com",
}

var fakeAuthDB = authtest.NewFakeDB(
	authtest.MockMembership("user:a@example.com", "group-1"),
	authtest.MockMembership("user:a@example.com", "group-2"),
)

var testExperiment = experiments.Register("test-experiment")

const (
	testServerAccountEmail = "fake-email@example.com"
	testCloudProjectID     = "cloud-project-id"
	testImageVersion       = "v123"
)

func TestServer(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)

		srv, err := newTestServer(ctx, nil)
		So(err, ShouldBeNil)
		defer srv.cleanup()

		Convey("VirtualHost", func() {
			srv.Routes.GET("/test", nil, func(c *router.Context) {
				c.Writer.Write([]byte("default-router"))
			})
			srv.VirtualHost("test-host.example.com").GET("/test", nil, func(c *router.Context) {
				c.Writer.Write([]byte("test-host-router"))
			})

			srv.ServeInBackground()
			defer srv.StopBackgroundServing()

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
			srv.Routes.GET("/test", nil, func(c *router.Context) {
				logging.Infof(c.Request.Context(), "Info log")
				tc.Add(time.Second)
				logging.Warningf(c.Request.Context(), "Warn log")
				c.Writer.WriteHeader(201)
				c.Writer.Write([]byte("Hello, world"))
			})

			srv.ServeInBackground()
			defer srv.StopBackgroundServing()

			resp, err := srv.GetMain("/test", map[string]string{
				"User-Agent":      "Test-user-agent",
				"X-Forwarded-For": "1.1.1.1,2.2.2.2,3.3.3.3",
			})
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, "Hello, world")

			// Stderr log captures details about the request.
			const traceID = "projects/cloud-project-id/traces/680b4e7c8b763a1b1d49d4955c848621"
			So(srv.stderr.Last(1), ShouldResemble, []sdlogger.LogEntry{
				{
					Severity:  sdlogger.WarningSeverity,
					Timestamp: sdlogger.Timestamp{Seconds: 1454472307, Nanos: 7},
					TraceID:   traceID,
					RequestInfo: &sdlogger.RequestInfo{
						Method:       "GET",
						URL:          "http://" + srv.mainAddr + "/test",
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
			So(srv.stdout.Last(2), ShouldResemble, []sdlogger.LogEntry{
				{
					Severity:  sdlogger.InfoSeverity,
					Message:   "Info log",
					Timestamp: sdlogger.Timestamp{Seconds: 1454472306, Nanos: 7},
					TraceID:   traceID,
					Operation: &sdlogger.Operation{
						ID: "6325253fec738dd7a9e28bf921119c16",
					},
				},
				{
					Severity:  sdlogger.WarningSeverity,
					Message:   "Warn log",
					Timestamp: sdlogger.Timestamp{Seconds: 1454472307, Nanos: 7},
					TraceID:   traceID,
					Operation: &sdlogger.Operation{
						ID: "6325253fec738dd7a9e28bf921119c16",
					},
				},
			})
		})

		Convey("Context features", func() {
			So(testContextFeatures(srv.Context, false), ShouldBeNil)
			srv.Routes.GET("/request", nil, func(c *router.Context) {
				if err := testContextFeatures(c.Request.Context(), true); err != nil {
					http.Error(c.Writer, err.Error(), 500)
				}
			})

			srv.ServeInBackground()
			defer srv.StopBackgroundServing()

			_, err := srv.GetMain("/request", nil)
			So(err, ShouldBeNil)
		})

		Convey("Context cancellation on client timeout", func() {
			cancelled := make(chan struct{})
			srv.Routes.GET("/request", nil, func(c *router.Context) {
				select {
				case <-c.Request.Context().Done():
					close(cancelled)
				case <-time.After(time.Minute):
				}
				http.Error(c.Writer, "This should basically be ignored", 500)
			})

			srv.ServeInBackground()
			defer srv.StopBackgroundServing()

			_, err := srv.GetMainWithTimeout("/request", nil, time.Second)
			So(err, ShouldNotBeNil)

			wasCancelled := false
			select {
			case <-cancelled:
				wasCancelled = true
			case <-time.After(time.Minute):
			}
			So(wasCancelled, ShouldBeTrue)
		})

		Convey("Warmup and cleanup callbacks", func() {
			var warmups []string
			var cleanups []string

			srv.RegisterWarmup(func(ctx context.Context) {
				if err := testContextFeatures(ctx, false); err != nil {
					panic(err)
				}
				warmups = append(warmups, "a")
			})
			srv.RegisterWarmup(func(ctx context.Context) {
				warmups = append(warmups, "b")
			})

			srv.RegisterCleanup(func(ctx context.Context) {
				if err := testContextFeatures(ctx, false); err != nil {
					panic(err)
				}
				cleanups = append(cleanups, "a")
			})
			srv.RegisterCleanup(func(ctx context.Context) {
				cleanups = append(cleanups, "b")
			})

			srv.ServeInBackground()

			So(warmups, ShouldResemble, []string{"a", "b"})
			So(cleanups, ShouldBeNil)

			srv.StopBackgroundServing()

			So(warmups, ShouldResemble, []string{"a", "b"})
			So(cleanups, ShouldResemble, []string{"b", "a"})
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
				activities <- nameErrPair{"background 1", testContextFeatures(ctx, false)}
			})

			srv.ServeInBackground()
			defer srv.StopBackgroundServing()

			// Run one more activity after starting the serving loop.
			srv.RunInBackground("background 2", func(ctx context.Context) {
				activities <- nameErrPair{"background 2", testContextFeatures(ctx, false)}
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
			srv.Routes.GET("/client-auth", nil, func(c *router.Context) {
				scopes := strings.Split(c.Request.Header.Get("Ask-Scope"), " ")
				ts, err := auth.GetTokenSource(c.Request.Context(), auth.AsSelf, auth.WithScopes(scopes...))
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
			defer srv.StopBackgroundServing()

			So(call("A B"), ShouldEqual, "fake_token_1")
			So(call("B C"), ShouldEqual, "fake_token_2")
			So(call("A B"), ShouldEqual, "fake_token_1") // reused the cached token

			// 0-th token is generated during startup in initAuth() to test creds.
			So(srv.tokens.TokenScopes("fake_token_0"), ShouldResemble, auth.CloudOAuthScopes)
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
				state := auth.GetState(rc.Request.Context())
				c.So(state.DB(), ShouldEqual, fakeAuthDB)
				c.So(state.PeerIdentity(), ShouldEqual, fakeUser.Identity)
				c.So(state.PeerIP().String(), ShouldEqual, "2.2.2.2")
				c.So(auth.CurrentUser(rc.Request.Context()), ShouldEqual, fakeUser)
				c.So(auth.CurrentIdentity(rc.Request.Context()), ShouldEqual, fakeUser.Identity)
				yes, err := auth.IsMember(rc.Request.Context(), "group-1")
				c.So(err, ShouldBeNil)
				c.So(yes, ShouldBeTrue)
			})

			srv.ServeInBackground()
			defer srv.StopBackgroundServing()

			_, err := srv.GetMain("/auth-state", map[string]string{
				"X-Forwarded-For": "1.1.1.1,2.2.2.2,3.3.3.3",
			})
			So(err, ShouldBeNil)
		})

		Convey("Egress", func(c C) {
			request := make(chan *http.Request, 1)
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				request <- r.Clone(context.Background())
			}))
			defer ts.Close()

			srv.Routes.GET("/test-egress", nil, func(rc *router.Context) {
				req, _ := http.NewRequest("GET", ts.URL, nil)
				req.Header.Add("User-Agent", "zzz")

				t, err := auth.GetRPCTransport(rc.Request.Context(), auth.NoAuth)
				c.So(err, ShouldBeNil)
				client := http.Client{Transport: t}

				resp, err := client.Do(req)
				c.So(err, ShouldBeNil)
				io.ReadAll(resp.Body)
				resp.Body.Close()
			})

			srv.ServeInBackground()
			defer srv.StopBackgroundServing()

			_, err := srv.GetMain("/test-egress", nil)
			So(err, ShouldBeNil)

			var req *http.Request
			select {
			case req = <-request:
			default:
			}
			So(req, ShouldNotBeNil)
			So(req.UserAgent(), ShouldEqual,
				fmt.Sprintf("LUCI-Server (service: service-name; job: namespace/job; ver: %s); zzz", testImageVersion))
		})

		Convey("/auth/api/v1/server/* handlers", func(c C) {
			srv.ServeInBackground()
			defer srv.StopBackgroundServing()

			resp, err := srv.GetMain("/auth/api/v1/server/info", nil)
			So(err, ShouldBeNil)

			info := signing.ServiceInfo{}
			So(json.Unmarshal([]byte(resp), &info), ShouldBeNil)
			So(info, ShouldResemble, signing.ServiceInfo{
				AppID:              testCloudProjectID,
				AppRuntime:         "go",
				AppRuntimeVersion:  runtime.Version(),
				AppVersion:         testImageVersion,
				ServiceAccountName: testServerAccountEmail,
			})

			// TODO(vadimsh): Add a test for /.../certificates once implemented.
			// TODO(vadimsh): Add a test for /.../client_id once implemented.
		})
	})
}

func TestH2C(t *testing.T) {
	t.Parallel()

	Convey("With server", t, func() {
		ctx := context.Background()

		srv, err := newTestServer(ctx, &Options{AllowH2C: true})
		So(err, ShouldBeNil)
		defer srv.cleanup()

		srv.Routes.GET("/test", nil, func(c *router.Context) {
			if err := testContextFeatures(c.Request.Context(), true); err != nil {
				http.Error(c.Writer, err.Error(), 500)
			} else {
				c.Writer.WriteHeader(200)
				c.Writer.Write([]byte("Hello, world"))
			}
		})

		srv.ServeInBackground()
		defer srv.StopBackgroundServing()

		Convey("HTTP/1", func() {
			srv.client = http.DefaultClient
			resp, err := srv.GetMain("/test", nil)
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, "Hello, world")
		})

		Convey("HTTP/2 Cleartext", func() {
			// See https://medium.com/@thrawn01/http-2-cleartext-h2c-client-example-in-go-8167c7a4181e
			srv.client = &http.Client{
				Transport: &http2.Transport{
					// So http2.Transport doesn't complain the URL scheme isn't 'https'
					AllowHTTP: true,
					// Pretend we are dialing a TLS endpoint.
					// Note, we ignore the passed tls.Config
					DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
						return net.Dial(network, addr)
					},
				},
			}
			resp, err := srv.GetMain("/test", nil)
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, "Hello, world")
		})
	})
}

func TestRPCServers(t *testing.T) {
	t.Parallel()

	// Helpers for testing context manipulations.
	type ctxKey string
	getFromCtx := func(ctx context.Context) string {
		if s, ok := ctx.Value(ctxKey("xxx")).(string); ok {
			return s
		}
		return "root"
	}
	addToCtx := func(ctx context.Context, val string) context.Context {
		return context.WithValue(ctx, ctxKey("xxx"), getFromCtx(ctx)+":"+val)
	}
	addingIntr := func(val string) grpcutil.UnifiedServerInterceptor {
		return func(ctx context.Context, _ string, handler func(context.Context) error) error {
			return handler(addToCtx(ctx, val))
		}
	}

	Convey("With server", t, func() {
		ctx := context.Background()

		srv, err := newTestServer(ctx, nil)
		So(err, ShouldBeNil)
		defer srv.cleanup()

		rpcSvc := &testRPCServer{}
		testpb.RegisterTestServer(srv, rpcSvc)

		conn := srv.GrpcClientConn()
		defer func() { _ = conn.Close() }()

		clients := []struct {
			protocol string
			impl     testpb.TestClient
		}{
			{
				"prpc", testpb.NewTestClient(&prpc.Client{
					Host:    srv.mainAddr,
					Options: &prpc.Options{Insecure: true},
				}),
			},
			{
				"grpc", testpb.NewTestClient(conn),
			},
		}

		for _, cl := range clients {
			protocol := cl.protocol
			rpcClient := cl.impl

			Convey(protocol+" client", func() {
				Convey("Context features", func() {
					rpcSvc.unary = func(ctx context.Context, _ *testpb.Request) (*testpb.Response, error) {
						if err := testContextFeatures(ctx, true); err != nil {
							return nil, err
						}
						return &testpb.Response{}, nil
					}

					srv.ServeInBackground()
					defer srv.StopBackgroundServing()

					_, err := rpcClient.Unary(context.Background(), &testpb.Request{})
					So(err, ShouldBeNil)
				})

				Convey("Non-nil OK errors", func() {
					// See https://github.com/grpc/grpc-go/pull/6374.
					rpcSvc.unary = func(ctx context.Context, _ *testpb.Request) (*testpb.Response, error) {
						return nil, malformedGrpcError{}
					}

					srv.ServeInBackground()
					defer srv.StopBackgroundServing()

					_, err := rpcClient.Unary(context.Background(), &testpb.Request{})
					So(err, ShouldHaveGRPCStatus, codes.Unknown)
				})

				Convey("Panic catcher is installed", func() {
					rpcSvc.unary = func(ctx context.Context, _ *testpb.Request) (*testpb.Response, error) {
						panic("BOOM")
					}

					srv.ServeInBackground()
					defer srv.StopBackgroundServing()

					_, err := rpcClient.Unary(context.Background(), &testpb.Request{})
					So(err, ShouldHaveGRPCStatus, codes.Internal)

					// Logged the panic.
					So(srv.stdout.Last(2)[0].Fields["panic.error"], ShouldEqual, "BOOM")
				})

				Convey("Unary interceptors", func() {
					srv.RegisterStreamServerInterceptors(
						addingIntr("ignore").Stream(),
					)
					srv.RegisterUnaryServerInterceptors(
						addingIntr("1").Unary(),
						addingIntr("2").Unary(),
					)
					srv.RegisterUnifiedServerInterceptors(
						addingIntr("3"),
						addingIntr("4"),
					)

					rpcSvc.unary = func(ctx context.Context, _ *testpb.Request) (*testpb.Response, error) {
						return &testpb.Response{Text: getFromCtx(ctx)}, nil
					}

					srv.ServeInBackground()
					defer srv.StopBackgroundServing()

					resp, err := rpcClient.Unary(context.Background(), &testpb.Request{})
					So(err, ShouldBeNil)
					So(resp.Text, ShouldEqual, "root:1:2:3:4")
				})

				if protocol == "prpc" {
					return // streaming is not support by prpc
				}

				Convey("Context features in stream RPCs", func() {
					rpcSvc.clientServerStream = func(ss testpb.Test_ClientServerStreamServer) error {
						if err := testContextFeatures(ss.Context(), true); err != nil {
							return err
						}
						for {
							req, err := ss.Recv()
							if err == io.EOF {
								return nil
							}
							if err != nil {
								return err
							}
							if err := ss.Send(&testpb.Response{Text: req.Text + ":pong"}); err != nil {
								return err
							}
						}
					}

					srv.ServeInBackground()
					defer srv.StopBackgroundServing()

					cs, err := rpcClient.ClientServerStream(context.Background())
					So(err, ShouldBeNil)

					for i := 0; i < 5; i++ {
						ping := fmt.Sprintf("ping-%d", i)
						So(cs.Send(&testpb.Request{Text: ping}), ShouldBeNil)
						res, err := cs.Recv()
						So(err, ShouldBeNil)
						So(res.Text, ShouldEqual, ping+":pong")
					}
					So(cs.CloseSend(), ShouldBeNil)

					_, err = cs.Recv()
					So(err, ShouldEqual, io.EOF)
				})

				Convey("Panic catcher in stream RPCs", func() {
					rpcSvc.serverStream = func(req *testpb.Request, ss testpb.Test_ServerStreamServer) error {
						_ = ss.Send(&testpb.Response{Text: req.Text + ":pong"})
						panic("BOOM")
					}

					srv.ServeInBackground()
					defer srv.StopBackgroundServing()

					ss, err := rpcClient.ServerStream(context.Background(), &testpb.Request{Text: "ping"})
					So(err, ShouldBeNil)

					resp, err := ss.Recv()
					So(err, ShouldBeNil)
					So(resp.Text, ShouldEqual, "ping:pong")

					_, err = ss.Recv()
					So(err, ShouldHaveGRPCStatus, codes.Internal)

					// Logged the panic.
					So(srv.stdout.Last(2)[0].Fields["panic.error"], ShouldEqual, "BOOM")
				})

				Convey("Stream interceptors", func() {
					srv.RegisterUnaryServerInterceptors(
						addingIntr("ignore").Unary(),
					)
					srv.RegisterStreamServerInterceptors(
						addingIntr("1").Stream(),
						addingIntr("2").Stream(),
					)
					srv.RegisterUnifiedServerInterceptors(
						addingIntr("3"),
						addingIntr("4"),
					)

					rpcSvc.clientStream = func(ss testpb.Test_ClientStreamServer) error {
						var all []string
						for {
							switch req, err := ss.Recv(); {
							case err == io.EOF:
								all = append(all, getFromCtx(ss.Context()))
								return ss.SendAndClose(&testpb.Response{Text: strings.Join(all, ":")})
							case err != nil:
								return nil
							default:
								all = append(all, req.Text)
							}
						}
					}

					srv.ServeInBackground()
					defer srv.StopBackgroundServing()

					cs, err := rpcClient.ClientStream(context.Background())
					So(err, ShouldBeNil)

					So(cs.Send(&testpb.Request{Text: "a"}), ShouldBeNil)
					So(cs.Send(&testpb.Request{Text: "b"}), ShouldBeNil)

					resp, err := cs.CloseAndRecv()
					So(err, ShouldBeNil)
					So(resp.Text, ShouldEqual, "a:b:root:1:2:3:4")
				})
			})
		}
	})
}

// testContextFeatures check that the context has all subsystems enabled.
func testContextFeatures(ctx context.Context, hasTraceID bool) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = errors.Reason("Panic: %s", p).Err()
		}
	}()

	// A TraceID is populated (or not) as expected.
	traceID := trace.SpanContextFromContext(ctx).TraceID().String()
	if hasTraceID && traceID == "00000000000000000000000000000000" {
		return errors.New("unexpectedly empty trace ID")
	}
	if !hasTraceID && traceID != "00000000000000000000000000000000" {
		return errors.New("unexpectedly populated trace ID")
	}

	// Experiments work.
	if !testExperiment.Enabled(ctx) {
		return errors.New("the experiment is unexpectedly off")
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
	case state.DB() != fakeAuthDB:
		return errors.Reason("unexpected auth.DB %v", state.DB()).Err()
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
			So(os.WriteFile(opts.AuthDBPath, []byte(body), 0600), ShouldBeNil)

			testRequestHandler(&opts, func(rc *router.Context) {
				db := auth.GetState(rc.Request.Context()).DB()
				yes, err := db.IsMember(rc.Request.Context(), "user:a@example.com", []string{"group"})
				c.So(err, ShouldBeNil)
				c.So(yes, ShouldBeTrue)
			})
		})
	})
}

func TestUniqueServerlessHostname(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		So(uniqueServerlessHostname("a", "b", "cccc"), ShouldEqual, "a-b-b6fbd675f98e2abd")
	})
}

var (
	// These vars are used in TestResolveDependencies().
	//
	// If go test runs with -count > 1, module.RegisterName() panics with
	// "already registered", because the registry is a global variable.
	// Hence, these are declared out of TestResolveDepenencies().
	a = module.RegisterName("a")
	b = module.RegisterName("b")
	c = module.RegisterName("c")
	d = module.RegisterName("d")
)

func TestResolveDependencies(t *testing.T) {
	mod := func(n module.Name, deps ...module.Dependency) module.Module {
		return &testModule{name: n, deps: deps}
	}

	resolve := func(mods ...module.Module) ([]string, error) {
		resolved, err := resolveDependencies(mods)
		if err != nil {
			return nil, err
		}
		names := make([]string, len(resolved))
		for i, m := range resolved {
			names[i] = m.Name().String()
		}
		return names, nil
	}

	Convey("Works at all", t, func() {
		names, err := resolve(
			mod(a, module.RequiredDependency(c), module.RequiredDependency(b)),
			mod(b, module.RequiredDependency(c)),
			mod(c),
		)
		So(err, ShouldBeNil)
		So(names, ShouldResemble, []string{"c", "b", "a"})
	})

	Convey("Preserves original order if no deps", t, func() {
		names, err := resolve(mod(a), mod(b), mod(c))
		So(err, ShouldBeNil)
		So(names, ShouldResemble, []string{"a", "b", "c"})
	})

	Convey("Two disjoint trees", t, func() {
		names, err := resolve(
			mod(a, module.RequiredDependency(b)), mod(b),
			mod(c, module.RequiredDependency(d)), mod(d),
		)
		So(err, ShouldBeNil)
		So(names, ShouldResemble, []string{"b", "a", "d", "c"})
	})

	Convey("Dup dependency is fine", t, func() {
		names, err := resolve(
			mod(a, module.RequiredDependency(c), module.RequiredDependency(c)),
			mod(b, module.RequiredDependency(c), module.RequiredDependency(c)),
			mod(c),
		)
		So(err, ShouldBeNil)
		So(names, ShouldResemble, []string{"c", "a", "b"})
	})

	Convey("Cycle", t, func() {
		names, err := resolve(
			mod(a, module.RequiredDependency(b)),
			mod(b, module.RequiredDependency(c)),
			mod(c, module.RequiredDependency(a)),
		)
		So(err, ShouldBeNil)
		So(names, ShouldResemble, []string{"c", "b", "a"})
	})

	Convey("Skips optional missing deps", t, func() {
		names, err := resolve(
			mod(a, module.OptionalDependency(c), module.RequiredDependency(b)),
			mod(b, module.OptionalDependency(c)),
		)
		So(err, ShouldBeNil)
		So(names, ShouldResemble, []string{"b", "a"})
	})

	Convey("Detects dups", t, func() {
		_, err := resolve(mod(a), mod(b), mod(a))
		So(err, ShouldErrLike, "duplicate module")
	})

	Convey("Checks required deps", t, func() {
		_, err := resolve(
			mod(a, module.RequiredDependency(b), module.RequiredDependency(c)),
			mod(b, module.RequiredDependency(c)),
		)
		So(err, ShouldErrLike, `module "a" requires module "c"`)
	})
}

func BenchmarkServer(b *testing.B) {
	srv, err := newTestServer(context.Background(), nil)
	if err != nil {
		b.Fatal(err)
	}
	defer srv.cleanup()

	// The route we are going to hit from the benchmark.
	srv.Routes.GET("/test", nil, func(c *router.Context) {
		logging.Infof(c.Request.Context(), "Hello, world")
		for i := 0; i < 10; i++ {
			// E.g. calling bunch of Cloud APIs.
			ts, _ := auth.GetTokenSource(c.Request.Context(), auth.AsSelf, auth.WithScopes("A", "B", "C"))
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
	grpcAddr  string
	adminAddr string

	cleanup  func()
	serveErr errorEvent
	serving  int32

	client *http.Client
}

func newTestServer(ctx context.Context, o *Options) (srv *testServer, err error) {
	srv = &testServer{
		serveErr: errorEvent{signal: make(chan struct{})},
		tokens: clientauthtest.FakeTokenGenerator{
			Email:      testServerAccountEmail,
			KeepRecord: true,
		},
		client: http.DefaultClient,
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
	srv.cleanup = func() { authSrv.Stop(ctx) }

	var opts Options
	if o != nil {
		opts = *o
	}

	opts.Prod = true
	opts.HTTPAddr = "main_addr"
	opts.GRPCAddr = "grpc_addr"
	opts.AdminAddr = "admin_addr"
	opts.ClientAuth = clientauth.Options{Method: clientauth.LUCIContextMethod}
	opts.CloudProject = testCloudProjectID
	opts.TsMonServiceName = "service-name"
	opts.TsMonJobName = "namespace/job"
	opts.TsMonFlushInterval = 234 * time.Second
	opts.TsMonFlushTimeout = 123 * time.Second
	opts.ContainerImageID = "registry/image:" + testImageVersion
	opts.EnableExperiments = []string{testExperiment.String()}

	opts.testSeed = 1
	opts.testStdout = &srv.stdout
	opts.testStderr = &srv.stderr
	if opts.AuthDBPath == "" {
		opts.AuthDBProvider = func(context.Context) (authdb.DB, error) {
			return fakeAuthDB, nil
		}
	}
	opts.testDisableTracing = true

	// Bind to auto-assigned ports.
	opts.testListeners = map[string]net.Listener{
		"main_addr":  setupListener(),
		"grpc_addr":  setupListener(),
		"admin_addr": setupListener(),
	}

	if srv.Server, err = New(ctx, opts, nil); err != nil {
		srv.cleanup()
		return nil, err
	}

	// TODO(vadimsh): This really should be memory.UseDS (which doesn't exist),
	// since only Datastore is implemented outside of GAE. It doesn't matter
	// for this particular test though. Note that memory.Use overrides our mocked
	// logger, but we need it. Bring it back.
	srv.Context = logging.SetFactory(memory.Use(srv.Context), logging.GetFactory(srv.Context))

	mainPort := srv.Options.testListeners["main_addr"].Addr().(*net.TCPAddr).Port
	srv.mainAddr = fmt.Sprintf("127.0.0.1:%d", mainPort)

	grpcPort := srv.Options.testListeners["grpc_addr"].Addr().(*net.TCPAddr).Port
	srv.grpcAddr = fmt.Sprintf("127.0.0.1:%d", grpcPort)

	adminPort := srv.Options.testListeners["admin_addr"].Addr().(*net.TCPAddr).Port
	srv.adminAddr = fmt.Sprintf("127.0.0.1:%d", adminPort)

	return srv, nil
}

func (s *testServer) ServeInBackground() {
	if atomic.LoadInt32(&s.serving) == 1 {
		panic("already serving")
	}

	go func() { s.serveErr.Set(s.Serve()) }()

	// Wait until both HTTP endpoints are serving before returning. Note that
	// these calls actually block until the server is up (not just fail) because
	// the listening sockets are open already, and connections just queue there
	// waiting for servers to start processing them.
	if _, err := s.GetMain(healthEndpoint, nil); err != nil {
		panic(err)
	}
	if _, err := s.GetAdmin(healthEndpoint, nil); err != nil {
		panic(err)
	}

	// Wait until the gRPC server is healthy.
	conn := s.GrpcClientConn()
	defer func() { _ = conn.Close() }()
	health := grpc_health_v1.NewHealthClient(conn)
	if _, err := health.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{}); err != nil {
		panic(err)
	}

	atomic.StoreInt32(&s.serving, 1)
}

func (s *testServer) StopBackgroundServing() {
	if atomic.LoadInt32(&s.serving) == 1 {
		s.Shutdown()
		if err := s.serveErr.Get(); err != nil {
			panic(err)
		}
	}
}

// GetMain makes a blocking request to the main serving port, aborting it if
// the server dies.
func (s *testServer) GetMain(uri string, headers map[string]string) (string, error) {
	return s.get("http://"+s.mainAddr+uri, headers, 0)
}

// GetMain makes a blocking request with timeout to the main serving port,
// aborting it if the server dies.
func (s *testServer) GetMainWithTimeout(uri string, headers map[string]string, timeout time.Duration) (string, error) {
	return s.get("http://"+s.mainAddr+uri, headers, timeout)
}

// GetAdmin makes a blocking request to the admin port, aborting it if
// the server dies.
func (s *testServer) GetAdmin(uri string, headers map[string]string) (string, error) {
	return s.get("http://"+s.adminAddr+uri, headers, 0)
}

// GrpcClientConn returns the gRPC client connection.
func (s *testServer) GrpcClientConn() *grpc.ClientConn {
	conn, err := grpc.Dial(s.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	return conn
}

// get makes a blocking request, aborting it if the server dies.
func (s *testServer) get(uri string, headers map[string]string, timeout time.Duration) (resp string, err error) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var req *http.Request
		if req, err = http.NewRequest("GET", uri, nil); err != nil {
			return
		}
		if timeout != 0 {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			req = req.WithContext(ctx)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		req.Host = headers["Host"] // req.Host (even when empty) overrides req.Header["Host"]
		var res *http.Response
		if res, err = s.client.Do(req); err != nil {
			return
		}
		defer res.Body.Close()
		var blob []byte
		if blob, err = io.ReadAll(res.Body); err != nil {
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

type testModule struct {
	name module.Name
	deps []module.Dependency
}

func (m *testModule) Name() module.Name { return m.name }

func (m *testModule) Dependencies() []module.Dependency { return m.deps }

func (m *testModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	return ctx, nil
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

	srv.Routes.GET("/test", nil, handler)
	_, err = srv.GetMain("/test", nil)
	So(err, ShouldBeNil)
}

////////////////////////////////////////////////////////////////////////////////

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
	logs    []sdlogger.LogEntry
}

func (r *logsRecorder) Write(e *sdlogger.LogEntry) {
	if r.discard {
		return
	}
	r.m.Lock()
	r.logs = append(r.logs, *e)
	r.m.Unlock()
}

func (r *logsRecorder) Last(n int) []sdlogger.LogEntry {
	entries := make([]sdlogger.LogEntry, n)
	r.m.Lock()
	copy(entries, r.logs[len(r.logs)-n:])
	r.m.Unlock()
	return entries
}

////////////////////////////////////////////////////////////////////////////////

type malformedGrpcError struct{}

func (malformedGrpcError) Error() string              { return "boom" }
func (malformedGrpcError) GRPCStatus() *status.Status { return nil }

type testRPCServer struct {
	testpb.UnimplementedTestServer

	unary              func(context.Context, *testpb.Request) (*testpb.Response, error)
	serverStream       func(*testpb.Request, testpb.Test_ServerStreamServer) error
	clientStream       func(testpb.Test_ClientStreamServer) error
	clientServerStream func(testpb.Test_ClientServerStreamServer) error
}

func (t *testRPCServer) Unary(ctx context.Context, r *testpb.Request) (*testpb.Response, error) {
	if t.unary != nil {
		return t.unary(ctx, r)
	}
	return nil, status.Errorf(codes.Unimplemented, "method Unary not implemented")
}

func (t *testRPCServer) ServerStream(r *testpb.Request, s testpb.Test_ServerStreamServer) error {
	if t.serverStream != nil {
		return t.serverStream(r, s)
	}
	return status.Errorf(codes.Unimplemented, "method ServerStream not implemented")
}

func (t *testRPCServer) ClientStream(s testpb.Test_ClientStreamServer) error {
	if t.clientStream != nil {
		return t.clientStream(s)
	}
	return status.Errorf(codes.Unimplemented, "method ClientStream not implemented")
}

func (t *testRPCServer) ClientServerStream(s testpb.Test_ClientServerStreamServer) error {
	if t.clientServerStream != nil {
		return t.clientServerStream(s)
	}
	return status.Errorf(codes.Unimplemented, "method ClientServerStream not implemented")
}
