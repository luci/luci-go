// Copyright 2016 The LUCI Authors.
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

package prpc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/klauspost/compress/gzip"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/grpc/prpc/internal/testpb"
	"go.chromium.org/luci/grpc/prpc/prpcpb"
)

func sayHello(t testing.TB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		assert.Loosely(t, r.Method, should.Equal("POST"))
		assert.Loosely(t, r.URL.Path == "/prpc/prpc.Greeter/SayHello" || r.URL.Path == "/python/prpc/prpc.Greeter/SayHello", should.BeTrue)
		assert.Loosely(t, r.Header.Get("Content-Type"), should.Equal("application/prpc; encoding=binary"))
		assert.Loosely(t, r.Header.Get("User-Agent"), should.Equal("prpc-test"))

		if timeout := r.Header.Get(HeaderTimeout); timeout != "" {
			assert.Loosely(t, timeout, should.Equal("10000000u"))
		}

		reqBody, err := io.ReadAll(r.Body)
		assert.Loosely(t, err, should.BeNil)

		var req testpb.HelloRequest
		err = proto.Unmarshal(reqBody, &req)
		assert.Loosely(t, err, should.BeNil)

		if req.Name == "TOO BIG" {
			w.Header().Set("Content-Length", "999999999999")
		}
		w.Header().Set("X-Lower-Case-Header", "CamelCaseValueStays")

		res := testpb.HelloReply{Message: "Hello " + req.Name}
		if r.URL.Path == "/python/prpc/prpc.Greeter/SayHello" {
			res.Message = res.Message + " from python service"
		}
		var buf []byte

		if req.Name == "ACCEPT JSONPB" {
			assert.Loosely(t, r.Header.Get("Accept"), should.Equal("application/json"))
			sbuf, err := codecJSONV1.Encode(nil, &res)
			assert.Loosely(t, err, should.BeNil)
			buf = []byte(sbuf)
		} else {
			assert.Loosely(t, r.Header.Get("Accept"), should.Equal("application/prpc; encoding=binary"))
			buf, err = proto.Marshal(&res)
			assert.Loosely(t, err, should.BeNil)
		}

		code := codes.OK
		status := http.StatusOK
		if req.Name == "NOT FOUND" {
			code = codes.NotFound
			status = http.StatusNotFound
		}

		w.Header().Set("Content-Type", r.Header.Get("Accept"))
		w.Header().Set(HeaderGRPCCode, strconv.Itoa(int(code)))
		w.WriteHeader(status)

		_, err = w.Write(buf)
		assert.Loosely(t, err, should.BeNil)
	}
}

func doPanicHandler(w http.ResponseWriter, r *http.Request) {
	panic("test panic")
}

func transientErrors(count int, grpcHeader bool, httpStatus int, then http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if count > 0 {
			count--
			if grpcHeader {
				w.Header().Set(HeaderGRPCCode, strconv.Itoa(int(codes.Internal)))
			}
			w.WriteHeader(httpStatus)
			fmt.Fprintln(w, "Server misbehaved")
			return
		}
		then.ServeHTTP(w, r)
	}
}

func advanceClockAndErr(tc testclock.TestClock, d time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tc.Add(d)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func shouldHaveMessagesLike(t testing.TB, log *memlogger.MemLogger, expected ...memlogger.LogEntry) {
	t.Helper()
	msgs := log.Messages()

	assert.Loosely(t, msgs, should.HaveLength(len(expected)), truth.LineContext())
	for i, actual := range msgs {
		assert.Loosely(t, actual.Level, should.Equal(expected[i].Level), truth.LineContext())
		assert.Loosely(t, actual.Msg, should.ContainSubstring(expected[i].Msg), truth.LineContext())
	}
}

func TestClient(t *testing.T) {
	t.Parallel()

	setUp := func(h http.HandlerFunc) (*Client, *httptest.Server) {
		server := httptest.NewServer(h)
		client := &Client{
			Host: strings.TrimPrefix(server.URL, "http://"),
			Options: &Options{
				Retry: func() retry.Iterator {
					return &retry.Limited{
						Retries: 3,
						Delay:   0,
					}
				},
				Insecure:  true,
				UserAgent: "prpc-test",
			},
		}
		return client, server
	}

	ftt.Run("Client", t, func(t *ftt.Test) {
		// These unit tests use real HTTP connections to localhost. Since go 1.7
		// 'net/http' library uses the context deadline to derive the connection
		// timeout: it grabs the deadline (as time.Time) from the context and
		// compares it to the current time. So we can't put arbitrary mocked time
		// into the testclock (as it ends up in the context deadline passed to
		// 'net/http'). We either have to use real clock in the unit tests, or
		// "freeze" the time at the real "now" value.
		ctx, tc := testclock.UseTime(context.Background(), time.Now().Local())
		ctx = memlogger.Use(ctx)
		log := logging.Get(ctx).(*memlogger.MemLogger)
		expectedCallLogEntry := func(c *Client) memlogger.LogEntry {
			return memlogger.LogEntry{
				Level: logging.Debug,
				Msg:   fmt.Sprintf("RPC %s/prpc.Greeter.SayHello", c.Host),
			}
		}

		req := &testpb.HelloRequest{Name: "John"}
		res := &testpb.HelloReply{}

		t.Run("Call", func(t *ftt.Test) {
			t.Run("Works", func(c *ftt.Test) {
				client, server := setUp(sayHello(c))
				defer server.Close()

				var hd metadata.MD
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res, grpc.Header(&hd))
				assert.Loosely(c, err, should.BeNil)
				assert.Loosely(c, res.Message, should.Equal("Hello John"))
				assert.Loosely(c, hd["x-lower-case-header"], should.Resemble([]string{"CamelCaseValueStays"}))

				shouldHaveMessagesLike(c, log, expectedCallLogEntry(client))
			})

			t.Run("Works with PathPrefix", func(t *ftt.Test) {
				client, server := setUp(sayHello(t))
				defer server.Close()

				client.PathPrefix = "/python/prpc"
				var hd metadata.MD
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res, grpc.Header(&hd))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.Message, should.Equal("Hello John from python service"))
			})

			t.Run("Works with response in JSONPB", func(t *ftt.Test) {
				req.Name = "ACCEPT JSONPB"
				client, server := setUp(sayHello(t))
				client.Options.AcceptContentSubtype = "json"
				defer server.Close()

				var hd metadata.MD
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res, grpc.Header(&hd))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.Message, should.Equal("Hello ACCEPT JSONPB"))
				assert.Loosely(t, hd["x-lower-case-header"], should.Resemble([]string{"CamelCaseValueStays"}))

				shouldHaveMessagesLike(t, log, expectedCallLogEntry(client))
			})

			t.Run("With outgoing metadata", func(t *ftt.Test) {
				var receivedHeader http.Header
				greeter := sayHello(t)
				client, server := setUp(func(w http.ResponseWriter, r *http.Request) {
					receivedHeader = r.Header
					greeter(w, r)
				})
				defer server.Close()

				ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
					"key", "value 1",
					"key", "value 2",
					"data-bin", string([]byte{0, 1, 2, 3}),
				))

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, receivedHeader["Key"], should.Resemble([]string{"value 1", "value 2"}))
				assert.Loosely(t, receivedHeader["Data-Bin"], should.Resemble([]string{"AAECAw=="}))
			})

			t.Run("Works with compression", func(t *ftt.Test) {
				req := &testpb.HelloRequest{Name: strings.Repeat("A", 1024)}

				client, server := setUp(func(w http.ResponseWriter, r *http.Request) {

					// Parse request.
					assert.Loosely(t, r.Header.Get("Accept-Encoding"), should.Equal("gzip"))
					assert.Loosely(t, r.Header.Get("Content-Encoding"), should.Equal("gzip"))
					gz, err := gzip.NewReader(r.Body)
					assert.Loosely(t, err, should.BeNil)
					defer gz.Close()
					reqBody, err := io.ReadAll(gz)
					assert.Loosely(t, err, should.BeNil)

					var actualReq testpb.HelloRequest
					err = proto.Unmarshal(reqBody, &actualReq)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, &actualReq, should.Resemble(req))

					// Write response.
					resBytes, err := proto.Marshal(&testpb.HelloReply{Message: "compressed response"})
					assert.Loosely(t, err, should.BeNil)
					resBody, err := compressBlob(resBytes)
					assert.Loosely(t, err, should.BeNil)

					w.Header().Set("Content-Type", mtPRPCBinary)
					w.Header().Set("Content-Encoding", "gzip")
					w.Header().Set(HeaderGRPCCode, "0")
					w.WriteHeader(http.StatusOK)
					_, err = w.Write(resBody)
					assert.Loosely(t, err, should.BeNil)
				})

				defer server.Close()

				client.EnableRequestCompression = true
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.Message, should.Equal("compressed response"))
			})

			t.Run("With a deadline <= now, does not execute.", func(t *ftt.Test) {
				client, server := setUp(doPanicHandler)
				defer server.Close()

				ctx, cancelFunc := clock.WithDeadline(ctx, clock.Now(ctx))
				defer cancelFunc()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				assert.Loosely(t, status.Code(err), should.Equal(codes.DeadlineExceeded))
				assert.Loosely(t, err, should.ErrLike("overall deadline exceeded"))
			})

			t.Run("With a deadline in the future, sets the deadline header.", func(t *ftt.Test) {
				client, server := setUp(sayHello(t))
				defer server.Close()

				ctx, cancelFunc := clock.WithDeadline(ctx, clock.Now(ctx).Add(10*time.Second))
				defer cancelFunc()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.Message, should.Equal("Hello John"))

				shouldHaveMessagesLike(t, log, expectedCallLogEntry(client))
			})

			t.Run("With a deadline in the future and a per-RPC deadline, applies the per-RPC deadline", func(t *ftt.Test) {
				// Set an overall deadline.
				overallDeadline := time.Second + 500*time.Millisecond
				ctx, cancel := clock.WithTimeout(ctx, overallDeadline)
				defer cancel()

				client, server := setUp(advanceClockAndErr(tc, time.Second))
				defer server.Close()

				calls := 0
				// All of our HTTP requests should terminate >= timeout. Synchronize
				// around this to ensure that our Context is always the functional
				// client error.
				client.testPostHTTP = func(ctx context.Context, err error) error {
					calls++
					<-ctx.Done()
					return ctx.Err()
				}

				client.Options.PerRPCTimeout = time.Second

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				assert.Loosely(t, status.Code(err), should.Equal(codes.DeadlineExceeded))
				assert.Loosely(t, err, should.ErrLike("overall deadline exceeded"))

				assert.Loosely(t, calls, should.Equal(2))
			})

			t.Run(`With a maximum response size smaller than the response, returns "ErrResponseTooBig".`, func(t *ftt.Test) {
				client, server := setUp(sayHello(t))
				defer server.Close()

				client.MaxResponseSize = 8
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Unavailable))
				assert.Loosely(t, err, should.ErrLike("exceeds the client limit"))
				assert.Loosely(t, ProtocolErrorDetails(err), should.Resemble(&prpcpb.ErrorDetails{
					Error: &prpcpb.ErrorDetails_ResponseTooBig{
						ResponseTooBig: &prpcpb.ResponseTooBig{
							ResponseSize:  12,
							ResponseLimit: 8,
						},
					},
				}))
			})

			t.Run(`When the response returns a huge Content-Length, returns "ErrResponseTooBig".`, func(t *ftt.Test) {
				client, server := setUp(sayHello(t))
				defer server.Close()

				req.Name = "TOO BIG"
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Unavailable))
				assert.Loosely(t, err, should.ErrLike("exceeds the client limit"))
				assert.Loosely(t, ProtocolErrorDetails(err), should.Resemble(&prpcpb.ErrorDetails{
					Error: &prpcpb.ErrorDetails_ResponseTooBig{
						ResponseTooBig: &prpcpb.ResponseTooBig{
							ResponseSize:  999999999999,
							ResponseLimit: DefaultMaxResponseSize,
						},
					},
				}))
			})

			t.Run("Doesn't log expected codes", func(t *ftt.Test) {
				client, server := setUp(sayHello(t))
				defer server.Close()

				req.Name = "NOT FOUND"

				// Have it logged by default
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				shouldHaveMessagesLike(t, log,
					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed permanently"})

				log.Reset()

				// And don't have it if using ExpectedCode.
				err = client.Call(ctx, "prpc.Greeter", "SayHello", req, res, ExpectedCode(codes.NotFound))
				assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
				shouldHaveMessagesLike(t, log, expectedCallLogEntry(client))
			})

			t.Run("HTTP 500 x2", func(t *ftt.Test) {
				client, server := setUp(transientErrors(2, true, http.StatusInternalServerError, sayHello(t)))
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.Message, should.Equal("Hello John"))

				shouldHaveMessagesLike(t, log,
					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently"},

					expectedCallLogEntry(client),
				)
			})

			t.Run("HTTP 500 many", func(t *ftt.Test) {
				client, server := setUp(transientErrors(10, true, http.StatusInternalServerError, sayHello(t)))
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				assert.Loosely(t, status.Code(err), should.Equal(codes.Internal))
				assert.Loosely(t, status.Convert(err).Message(), should.Equal("Server misbehaved"))

				shouldHaveMessagesLike(t, log,
					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed permanently"},
				)
			})

			t.Run("HTTP 500 without gRPC header", func(t *ftt.Test) {
				client, server := setUp(transientErrors(10, false, http.StatusInternalServerError, sayHello(t)))
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				assert.Loosely(t, status.Code(err), should.Equal(codes.Internal))

				shouldHaveMessagesLike(t, log,
					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed permanently"},
				)
			})

			t.Run("HTTP 503 without gRPC header", func(t *ftt.Test) {
				client, server := setUp(transientErrors(10, false, http.StatusServiceUnavailable, sayHello(t)))
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				assert.Loosely(t, status.Code(err), should.Equal(codes.Unavailable))
			})

			t.Run("Forbidden", func(t *ftt.Test) {
				client, server := setUp(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set(HeaderGRPCCode, strconv.Itoa(int(codes.PermissionDenied)))
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprintln(w, "Access denied")
				})
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, status.Convert(err).Message(), should.Equal("Access denied"))

				shouldHaveMessagesLike(t, log,
					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed permanently"},
				)
			})

			t.Run(HeaderGRPCCode, func(t *ftt.Test) {
				client, server := setUp(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set(HeaderGRPCCode, strconv.Itoa(int(codes.Canceled)))
					w.WriteHeader(http.StatusBadRequest)
				})
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				assert.Loosely(t, status.Code(err), should.Equal(codes.Canceled))
			})

			t.Run("Concurrency limit", func(t *ftt.Test) {
				const (
					maxConcurrentRequests = 3
					totalRequests         = 10
				)

				cur := int64(0)
				reports := make(chan int64, totalRequests)

				// For each request record how many parallel requests were running at
				// the same time.
				client, server := setUp(func(w http.ResponseWriter, r *http.Request) {
					reports <- atomic.AddInt64(&cur, 1)
					defer atomic.AddInt64(&cur, -1)
					// Note: dependence on the real clock is racy, but in the worse case
					// (if client.Call guts are extremely slow) we'll get a false positive
					// result. In other words, if the code under test is correct (and it
					// is right now), the test will always succeed no matter what. If the
					// code under test is not correct (i.e. regresses), we'll start seeing
					// test errors most of the time, with occasional false successes.
					time.Sleep(200 * time.Millisecond)
					sayHello(t)(w, r)
				})
				defer server.Close()

				client.MaxConcurrentRequests = maxConcurrentRequests

				// Execute a bunch of requests concurrently.
				wg := sync.WaitGroup{}
				for i := 0; i < totalRequests; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						err := client.Call(ctx, "prpc.Greeter", "SayHello", &testpb.HelloRequest{Name: "John"}, &testpb.HelloReply{})
						assert.Loosely(t, err, should.BeNil)
					}()
				}
				wg.Wait()

				// Make sure concurrency limit wasn't violated.
				for i := 0; i < totalRequests; i++ {
					select {
					case concur := <-reports:
						assert.Loosely(t, concur, should.BeLessThanOrEqual(maxConcurrentRequests))
					default:
						t.Fatal("Some requests didn't execute")
					}
				}
			})
		})
	})
}
