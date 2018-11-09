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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/retry"

	. "github.com/smartystreets/goconvey/convey"
)

func sayHello(c C) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.So(r.Method, ShouldEqual, "POST")
		c.So(r.URL.Path, ShouldEqual, "/prpc/prpc.Greeter/SayHello")
		c.So(r.Header.Get("Accept"), ShouldEqual, "application/prpc; encoding=binary")
		c.So(r.Header.Get("Content-Type"), ShouldEqual, "application/prpc; encoding=binary")
		c.So(r.Header.Get("User-Agent"), ShouldEqual, "prpc-test")

		if timeout := r.Header.Get(HeaderTimeout); timeout != "" {
			c.So(timeout, ShouldEqual, "10000000u")
		}

		reqBody, err := ioutil.ReadAll(r.Body)
		c.So(err, ShouldBeNil)

		var req HelloRequest
		err = proto.Unmarshal(reqBody, &req)
		c.So(err, ShouldBeNil)

		if req.Name == "TOO BIG" {
			w.Header().Set("Content-Length", "999999999999")
		}
		w.Header().Set("X-Lower-Case-Header", "CamelCaseValueStays")

		res := HelloReply{Message: "Hello " + req.Name}
		buf, err := proto.Marshal(&res)
		c.So(err, ShouldBeNil)

		code := codes.OK
		status := http.StatusOK
		if req.Name == "NOT FOUND" {
			code = codes.NotFound
			status = http.StatusNotFound
		}

		w.Header().Set(HeaderGRPCCode, strconv.Itoa(int(code)))
		w.Header().Set("Content-Type", ContentTypePRPC)
		w.WriteHeader(status)

		_, err = w.Write(buf)
		c.So(err, ShouldBeNil)
	}
}

func doPanicHandler(w http.ResponseWriter, r *http.Request) {
	panic("test panic")
}

func transientErrors(count int, grpcHeader bool, then http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if count > 0 {
			count--
			if grpcHeader {
				w.Header().Set(HeaderGRPCCode, strconv.Itoa(int(codes.Internal)))
			}
			w.WriteHeader(http.StatusInternalServerError)
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

func shouldHaveMessagesLike(actual interface{}, expected ...interface{}) string {
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
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res, Header(&hd))
				So(err, ShouldBeNil)
				So(res.Message, ShouldEqual, "Hello John")
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

				ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("key", "value"))

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(err, ShouldBeNil)

				So(receivedHeader.Get("key"), ShouldEqual, "value")
			})

			Convey("With a deadline <= now, does not execute.", func(c C) {
				client, server := setUp(doPanicHandler)
				defer server.Close()

				ctx, cancelFunc := clock.WithDeadline(ctx, clock.Now(ctx))
				defer cancelFunc()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(err.Error(), ShouldEqual, context.DeadlineExceeded.Error())
			})

			Convey("With a deadline in the future, sets the deadline header.", func(c C) {
				client, server := setUp(sayHello(c))
				defer server.Close()

				ctx, cancelFunc := clock.WithDeadline(ctx, clock.Now(ctx).Add(10*time.Second))
				defer cancelFunc()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(err, ShouldBeNil)
				So(res.Message, ShouldEqual, "Hello John")

				So(log, shouldHaveMessagesLike,
					expectedCallLogEntry(client),
					memlogger.LogEntry{
						Level: logging.Debug,
						Msg:   fmt.Sprintf("RPC: Using context deadline %q", clock.Now(ctx).Add(10*time.Second)),
					})
			})

			Convey("With a deadline in the future and a per-RPC deadline, applies the per-RPC deadline", func(c C) {
				retries := 5

				// Set an overall deadline.
				overallDeadline := time.Duration(10*time.Second)*time.Duration(retries) - 1
				ctx, cancelFunc := clock.WithTimeout(ctx, overallDeadline)
				defer cancelFunc()

				client, server := setUp(advanceClockAndErr(tc, 10*time.Second))
				defer server.Close()

				// All of our HTTP requests should terminate >= timeout. Synchronize
				// around this to ensure that our Context is always the functional
				// client error.
				client.testPostHTTP = func(c context.Context, err error) error {
					<-c.Done()
					return c.Err()
				}

				client.Options.PerRPCTimeout = 10 * time.Second
				client.Options.Retry = func() retry.Iterator {
					return &countingRetryIterator{
						retries: &retries,
					}
				}

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(err.Error(), ShouldEqual, context.DeadlineExceeded.Error())
				So(retries, ShouldEqual, 0)
			})

			Convey(`With a maximum content length smaller than the response, returns "ErrResponseTooBig".`, func(c C) {
				client, server := setUp(sayHello(c))
				defer server.Close()

				client.MaxContentLength = 8
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(err, ShouldEqual, ErrResponseTooBig)
			})

			Convey(`When the response returns a huge Content Length, returns "ErrResponseTooBig".`, func(c C) {
				client, server := setUp(sayHello(c))
				defer server.Close()

				req.Name = "TOO BIG"
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(err, ShouldEqual, ErrResponseTooBig)
			})

			Convey("Doesn't log expected codes", func(c C) {
				client, server := setUp(sayHello(c))
				defer server.Close()

				req.Name = "NOT FOUND"

				// Have it logged by default
				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(grpc.Code(err), ShouldEqual, codes.NotFound)
				So(log, shouldHaveMessagesLike,
					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed permanently"})

				log.Reset()

				// And don't have it if using ExpectedCode.
				err = client.Call(ctx, "prpc.Greeter", "SayHello", req, res, ExpectedCode(codes.NotFound))
				So(grpc.Code(err), ShouldEqual, codes.NotFound)
				So(log, shouldHaveMessagesLike, expectedCallLogEntry(client))
			})

			Convey("HTTP 500 x2", func(c C) {
				client, server := setUp(transientErrors(2, true, sayHello(c)))
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(err, ShouldBeNil)
				So(res.Message, ShouldEqual, "Hello John")

				So(log, shouldHaveMessagesLike,
					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently. Will retry in 0"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently. Will retry in 0"},

					expectedCallLogEntry(client),
				)
			})

			Convey("HTTP 500 many", func(c C) {
				client, server := setUp(transientErrors(10, true, sayHello(c)))
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(grpc.Code(err), ShouldEqual, codes.Internal)
				So(grpc.ErrorDesc(err), ShouldEqual, "Server misbehaved")

				So(log, shouldHaveMessagesLike,
					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently. Will retry in 0"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently. Will retry in 0"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently. Will retry in 0"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed permanently"},
				)
			})

			Convey("HTTP 500 without gRPC header", func(c C) {
				client, server := setUp(transientErrors(10, false, sayHello(c)))
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(grpc.Code(err), ShouldEqual, codes.Unknown)

				So(log, shouldHaveMessagesLike,
					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently. Will retry in 0"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently. Will retry in 0"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed transiently. Will retry in 0"},

					expectedCallLogEntry(client),
					memlogger.LogEntry{Level: logging.Warning, Msg: "RPC failed permanently"},
				)
			})

			Convey("Forbidden", func(c C) {
				client, server := setUp(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set(HeaderGRPCCode, strconv.Itoa(int(codes.PermissionDenied)))
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprintln(w, "Access denied")
				})
				defer server.Close()

				err := client.Call(ctx, "prpc.Greeter", "SayHello", req, res)
				So(grpc.Code(err), ShouldEqual, codes.PermissionDenied)
				So(grpc.ErrorDesc(err), ShouldEqual, "Access denied")

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
				So(grpc.Code(err), ShouldEqual, codes.Canceled)
			})
		})
	})
}

type countingRetryIterator struct {
	retries *int
}

func (it *countingRetryIterator) Next(c context.Context, err error) time.Duration {
	*(it.retries)--
	if *it.retries <= 0 {
		return retry.Stop
	}
	return 0
}
