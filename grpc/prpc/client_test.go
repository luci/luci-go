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

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/klauspost/compress/gzip"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/retry"

	"go.chromium.org/luci/grpc/prpc/prpcpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func sayHello(c C) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.So(r.Method, ShouldEqual, "POST")
		c.So(r.URL.Path == "/prpc/prpc.Greeter/SayHello" || r.URL.Path == "/python/prpc/prpc.Greeter/SayHello", ShouldBeTrue)
		c.So(r.Header.Get("Content-Type"), ShouldEqual, "application/prpc; encoding=binary")
		c.So(r.Header.Get("User-Agent"), ShouldEqual, "prpc-test")

		if timeout := r.Header.Get(HeaderTimeout); timeout != "" {
			c.So(timeout, ShouldEqual, "10000000u")
		}

		reqBody, err := io.ReadAll(r.Body)
		c.So(err, ShouldBeNil)

		var req HelloRequest
		err = proto.Unmarshal(reqBody, &req)
		c.So(err, ShouldBeNil)

		if req.Name == "TOO BIG" {
			w.Header().Set("Content-Length", "999999999999")
		}
		w.Header().Set("X-Lower-Case-Header", "CamelCaseValueStays")

		res := HelloReply{Message: "Hello " + req.Name}
		if r.URL.Path == "/python/prpc/prpc.Greeter/SayHello" {
			res.Message = res.Message + " from python service"
		}
		var buf []byte

		if req.Name == "ACCEPT JSONPB" {
			c.So(r.Header.Get("Accept"), ShouldEqual, "application/json")
			sbuf, err := (&jsonpb.Marshaler{}).MarshalToString(&res)
			c.So(err, ShouldBeNil)
			buf = []byte(sbuf)
		} else {
			c.So(r.Header.Get("Accept"), ShouldEqual, "application/prpc; encoding=binary")
			buf, err = proto.Marshal(&res)
			c.So(err, ShouldBeNil)
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
		c.So(err, ShouldBeNil)
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

func shouldHaveMessagesLike(actual any, expected ...any) string {
	log := actual.(*memlogger.MemLogger)
	msgs := log.Messages()

	So(msgs, ShouldHaveLength, len(expected))
	for i, actual := range msgs {
		expected := expected[i].(memlogger.LogEntry)
		So(actual.Level, ShouldEqual, expected.Level)
		So(actual.Msg, ShouldContainSubstring, expected.Msg)
	}
	return ""
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

	Convey("Client", t, func() {
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

		req := &HelloRequest{Name: "John"}
		res := &HelloReply{}

		Convey("Call", func() {
			Convey("Works", func(c C) {
				client, server := setUp(sayHello(c))
				defer server.Close()

				var hd metadata.MD
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res, grpc.Header(&hd))
				So(err, ShouldBeNil)
				So(res.Message, ShouldEqual, "Hello John")
				So(hd["x-lower-case-header"], ShouldResemble, []string{"CamelCaseValueStays"})

				So(log, shouldHaveMessagesLike, expectedCallLogEntry(client))
			})

			Convey("Works with PathPrefix", func(c C) {
				client, server := setUp(sayHello(c))
				defer server.Close()

				client.PathPrefix = "/python/prpc"
				var hd metadata.MD
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res, grpc.Header(&hd))
				So(err, ShouldBeNil)
				So(res.Message, ShouldEqual, "Hello John from python service")
			})

			Convey("Works with response in JSONPB", func(c C) {
				req.Name = "ACCEPT JSONPB"
				client, server := setUp(sayHello(c))
				client.Options.AcceptContentSubtype = "json"
				defer server.Close()

				var hd metadata.MD
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res, grpc.Header(&hd))
				So(err, ShouldBeNil)
				So(res.Message, ShouldEqual, "Hello ACCEPT JSONPB")
				So(hd["x-lower-case-header"], ShouldResemble, []string{"CamelCaseValueStays"})

				So(log, shouldHaveMessagesLike, expectedCallLogEntry(client))
			})

			Convey("With outgoing metadata", func(c C) {
				var receivedHeader http.Header
				greeter := sayHello(c)
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
				So(err, ShouldBeNil)

				So(receivedHeader["Key"], ShouldResemble, []string{"value 1", "value 2"})
				So(receivedHeader["Data-Bin"], ShouldResemble, []string{"AAECAw=="})
			})

			Convey("Works with compression", func(c C) {
				req := &HelloRequest{Name: strings.Repeat("A", 1024)}

				client, server := setUp(func(w http.ResponseWriter, r *http.Request) {

					// Parse request.
					c.So(r.Header.Get("Accept-Encoding"), ShouldEqual, "gzip")
					c.So(r.Header.Get("Content-Encoding"), ShouldEqual, "gzip")
					gz, err := gzip.NewReader(r.Body)
					c.So(err, ShouldBeNil)
					defer gz.Close()
					reqBody, err := io.ReadAll(gz)
					c.So(err, ShouldBeNil)

					var actualReq HelloRequest
					err = proto.Unmarshal(reqBody, &actualReq)
					c.So(err, ShouldBeNil)
					c.So(&actualReq, ShouldResembleProto, req)

					// Write response.
					resBytes, err := proto.Marshal(&HelloReply{Message: "compressed response"})
					c.So(err, ShouldBeNil)
					resBody, err := compressBlob(resBytes)
					c.So(err, ShouldBeNil)

					w.Header().Set("Content-Type", mtPRPCBinary)
					w.Header().Set("Content-Encoding", "gzip")
					w.Header().Set(HeaderGRPCCode, "0")
					w.WriteHeader(http.StatusOK)
					_, err = w.Write(resBody)
					c.So(err, ShouldBeNil)
				})

				defer server.Close()

				client.EnableRequestCompression = true
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(err, ShouldBeNil)
				So(res.Message, ShouldEqual, "compressed response")
			})

			Convey("With a deadline <= now, does not execute.", func(c C) {
				client, server := setUp(doPanicHandler)
				defer server.Close()

				ctx, cancelFunc := clock.WithDeadline(ctx, clock.Now(ctx))
				defer cancelFunc()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(status.Code(err), ShouldEqual, codes.DeadlineExceeded)
				So(err, ShouldErrLike, "overall deadline exceeded")
			})

			Convey("With a deadline in the future, sets the deadline header.", func(c C) {
				client, server := setUp(sayHello(c))
				defer server.Close()

				ctx, cancelFunc := clock.WithDeadline(ctx, clock.Now(ctx).Add(10*time.Second))
				defer cancelFunc()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(err, ShouldBeNil)
				So(res.Message, ShouldEqual, "Hello John")

				So(log, shouldHaveMessagesLike, expectedCallLogEntry(client))
			})

			Convey("With a deadline in the future and a per-RPC deadline, applies the per-RPC deadline", func(c C) {
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
				So(status.Code(err), ShouldEqual, codes.DeadlineExceeded)
				So(err, ShouldErrLike, "overall deadline exceeded")

				So(calls, ShouldEqual, 2)
			})

			Convey(`With a maximum response size smaller than the response, returns "ErrResponseTooBig".`, func(c C) {
				client, server := setUp(sayHello(c))
				defer server.Close()

				client.MaxResponseSize = 8
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(err, ShouldHaveGRPCStatus, codes.Unavailable)
				So(err, ShouldErrLike, "exceeds the client limit")
				So(ProtocolErrorDetails(err), ShouldResembleProto, &prpcpb.ErrorDetails{
					Error: &prpcpb.ErrorDetails_ResponseTooBig{
						ResponseTooBig: &prpcpb.ResponseTooBig{
							ResponseSize:  12,
							ResponseLimit: 8,
						},
					},
				})
			})

			Convey(`When the response returns a huge Content-Length, returns "ErrResponseTooBig".`, func(c C) {
				client, server := setUp(sayHello(c))
				defer server.Close()

				req.Name = "TOO BIG"
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(err, ShouldHaveGRPCStatus, codes.Unavailable)
				So(err, ShouldErrLike, "exceeds the client limit")
				So(ProtocolErrorDetails(err), ShouldResembleProto, &prpcpb.ErrorDetails{
					Error: &prpcpb.ErrorDetails_ResponseTooBig{
						ResponseTooBig: &prpcpb.ResponseTooBig{
							ResponseSize:  999999999999,
							ResponseLimit: DefaultMaxResponseSize,
						},
					},
				})
			})

			Convey("Doesn't log expected codes", func(c C) {
				client, server := setUp(sayHello(c))
				defer server.Close()

				req.Name = "NOT FOUND"

				// Have it logged by default
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(log, shouldHaveMessagesLike,
					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed permanently"})

				log.Reset()

				// And don't have it if using ExpectedCode.
				err = client.Call(ctx, "prpc.Greeter", "SayHello", req, res, ExpectedCode(codes.NotFound))
				So(status.Code(err), ShouldEqual, codes.NotFound)
				So(log, shouldHaveMessagesLike, expectedCallLogEntry(client))
			})

			Convey("HTTP 500 x2", func(c C) {
				client, server := setUp(transientErrors(2, true, http.StatusInternalServerError, sayHello(c)))
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(err, ShouldBeNil)
				So(res.Message, ShouldEqual, "Hello John")

				So(log, shouldHaveMessagesLike,
					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently"},

					expectedCallLogEntry(client),
				)
			})

			Convey("HTTP 500 many", func(c C) {
				client, server := setUp(transientErrors(10, true, http.StatusInternalServerError, sayHello(c)))
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(status.Code(err), ShouldEqual, codes.Internal)
				So(status.Convert(err).Message(), ShouldEqual, "Server misbehaved")

				So(log, shouldHaveMessagesLike,
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

			Convey("HTTP 500 without gRPC header", func(c C) {
				client, server := setUp(transientErrors(10, false, http.StatusInternalServerError, sayHello(c)))
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(status.Code(err), ShouldEqual, codes.Internal)

				So(log, shouldHaveMessagesLike,
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

			Convey("HTTP 503 without gRPC header", func(c C) {
				client, server := setUp(transientErrors(10, false, http.StatusServiceUnavailable, sayHello(c)))
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(status.Code(err), ShouldEqual, codes.Unavailable)
			})

			Convey("Forbidden", func(c C) {
				client, server := setUp(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set(HeaderGRPCCode, strconv.Itoa(int(codes.PermissionDenied)))
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprintln(w, "Access denied")
				})
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(status.Code(err), ShouldEqual, codes.PermissionDenied)
				So(status.Convert(err).Message(), ShouldEqual, "Access denied")

				So(log, shouldHaveMessagesLike,
					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed permanently"},
				)
			})

			Convey(HeaderGRPCCode, func(c C) {
				client, server := setUp(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set(HeaderGRPCCode, strconv.Itoa(int(codes.Canceled)))
					w.WriteHeader(http.StatusBadRequest)
				})
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(status.Code(err), ShouldEqual, codes.Canceled)
			})

			Convey("Concurrency limit", func(c C) {
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
					sayHello(c)(w, r)
				})
				defer server.Close()

				client.MaxConcurrentRequests = maxConcurrentRequests

				// Execute a bunch of requests concurrently.
				wg := sync.WaitGroup{}
				for i := 0; i < totalRequests; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						err := client.Call(ctx, "prpc.Greeter", "SayHello", &HelloRequest{Name: "John"}, &HelloReply{})
						c.So(err, ShouldBeNil)
					}()
				}
				wg.Wait()

				// Make sure concurrency limit wasn't violated.
				for i := 0; i < totalRequests; i++ {
					select {
					case concur := <-reports:
						So(concur, ShouldBeLessThanOrEqualTo, maxConcurrentRequests)
					default:
						t.Fatal("Some requests didn't execute")
					}
				}
			})
		})
	})
}
