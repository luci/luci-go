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
package sink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"testing"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/server/router"

	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func installTestListener(srv *Server) (string, func() error) {
	l, err := net.Listen("tcp", "localhost:0")
	So(err, ShouldBeNil)
	srv.testListener = l

	// return the serving address
	return fmt.Sprint("http://localhost:", l.Addr().(*net.TCPAddr).Port), l.Close
}

func startWithTestListener(srv *Server) (string, func() error) {
	addr, cleanup := installTestListener(srv)
	So(srv.Start(), ShouldBeNil)
	return addr, cleanup
}

func httpGet(url string, authToken string) (int, string, error) {
	c := http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, "", err
	}
	if authToken != "" {
		req.Header.Add(AuthTokenKey, AuthTokenValue(authToken))
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	// read the body and close the IO so that the callers don't have to close the body
	// handler.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, "", err
	}
	return resp.StatusCode, string(body), nil
}

func prpcPost(url string, authToken string, in proto.Message) (int, error) {
	// encode the payload
	byteData, err := json.Marshal(in)
	if err != nil {
		return 0, err
	}
	So(err, ShouldBeNil)
	buf := bytes.NewBuffer(byteData)

	// create request
	req, err := http.NewRequest("POST", url, buf)
	if err != nil {
		return 0, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	if authToken != "" {
		req.Header.Add(AuthTokenKey, AuthTokenValue(authToken))
	}

	// send
	c := http.Client{}
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, err
}

func TestNewServer(t *testing.T) {
	t.Parallel()

	Convey("NewServer", t, func() {
		ctx := context.Background()
		Convey("succeeds", func() {
			srv, err := NewServer(ctx, ServerConfig{Address: ":42", AuthToken: "hello"})
			So(err, ShouldBeNil)
			So(srv, ShouldNotBeNil)
		})
		Convey("creates a random auth token, if not given", func() {
			srv, err := NewServer(ctx, ServerConfig{Address: ":42"})
			So(err, ShouldBeNil)
			So(srv, ShouldNotBeNil)
			So(srv.cfg.AuthToken, ShouldNotEqual, "")
		})
		Convey("uses the default address, if not given", func() {
			srv, err := NewServer(ctx, ServerConfig{})
			So(err, ShouldBeNil)
			So(srv, ShouldNotBeNil)
			So(srv.cfg.Address, ShouldNotEqual, "")
		})
	})
}

func TestServer(t *testing.T) {
	t.Parallel()

	Convey("Server", t, func() {
		ctx := context.Background()
		srv, err := NewServer(ctx, ServerConfig{AuthToken: "secret"})
		So(err, ShouldBeNil)
		So(srv, ShouldNotBeNil)

		// install a test handler
		srv.routes.GET("/hello", router.MiddlewareChain{}, func(c *router.Context) {
			fmt.Fprintf(c.Writer, "Hello")
		})
		// install a handler that panics
		srv.routes.GET("/panic", router.MiddlewareChain{}, func(c *router.Context) {
			panic("hello!")
		})

		Convey("Start", func() {
			Convey("succeeds", func() {
				_, cleanup := startWithTestListener(srv)
				defer cleanup()
			})

			Convey("fails if called twice", func() {
				_, cleanup := startWithTestListener(srv)
				defer cleanup()
				So(srv.Start(), ShouldErrLike, "cannot call Start twice")
			})

			Convey("fails after being closed", func() {
				_, cleanup := startWithTestListener(srv)
				defer cleanup()

				// close the server
				err = srv.Close()
				So(err, ShouldBeNil)
				So(<-srv.ErrC(), ShouldEqual, http.ErrServerClosed)

				// start after close should fail
				So(srv.Start(), ShouldErrLike, "cannot call Start twice")
			})
		})

		Convey("Close closes the HTTP server", func() {
			addr, cleanup := startWithTestListener(srv)
			defer cleanup()

			// check that the server is up.
			_, err := http.Get(addr)
			So(err, ShouldBeNil)

			// close the server
			err = srv.Close()
			So(err, ShouldBeNil)
			So(<-srv.ErrC(), ShouldEqual, http.ErrServerClosed)

			// verify that the port is no longer available.
			resp, err := http.Get(addr)
			So(err, ShouldErrLike, "refused")
			So(resp, ShouldBeNil)
		})

		Convey("Run", func() {
			handlerErr := make(chan error)
			runErr := make(chan error)
			expected := errors.New("an error-1")

			Convey("succeeds", func() {
				addr, cleanup := installTestListener(srv)
				defer cleanup()

				// launch a go routine with Run
				go func() {
					runErr <- srv.Run(func(ctx context.Context) error {
						return <-handlerErr
					})
				}()

				// check that the server is running
				code, _, err := httpGet(fmt.Sprint(addr, "/hello"), "secret")
				So(err, ShouldBeNil)
				So(code, ShouldEqual, http.StatusOK)

				// finish the callback and verify that the server stopped running
				handlerErr <- expected
				So(<-srv.ErrC(), ShouldEqual, http.ErrServerClosed)
				So(<-runErr, ShouldEqual, expected)
			})

			Convey("aborts after server error", func() {
				addr, cleanup := installTestListener(srv)
				defer cleanup()

				// launch a go routine with Run
				go func() {
					runErr <- srv.Run(func(ctx context.Context) error {
						select {
						case <-ctx.Done():
						}
						return nil
					})
				}()

				// check that the server is running
				code, _, err := httpGet(fmt.Sprint(addr, "/hello"), "secret")
				So(err, ShouldBeNil)
				So(code, ShouldEqual, http.StatusOK)

				// emit a server error. Run should return the server error.
				srv.errC <- expected
				So(<-runErr, ShouldEqual, expected)
				So(<-srv.ErrC(), ShouldEqual, http.ErrServerClosed)
			})
		})

		Convey("serves HTTP requests", func() {
			Convey("with 200 OK", func() {
				addr, cleanup := startWithTestListener(srv)
				defer cleanup()

				code, resp, err := httpGet(fmt.Sprint(addr, "/hello"), "secret")
				So(err, ShouldBeNil)
				So(code, ShouldEqual, http.StatusOK)
				So(resp, ShouldEqual, "Hello")
			})

			Convey("with 404 Not Found", func() {
				addr, cleanup := startWithTestListener(srv)
				defer cleanup()

				code, _, err := httpGet(fmt.Sprint(addr, "/non-existing"), "secret")
				So(err, ShouldBeNil)
				So(code, ShouldEqual, http.StatusNotFound)
			})

			Convey("with 403 Forbidden", func() {
				Convey("if auth_token missing", func() {
					addr, cleanup := startWithTestListener(srv)
					defer cleanup()

					code, body, err := httpGet(fmt.Sprint(addr, "/hello"), "")
					So(err, ShouldBeNil)
					So(code, ShouldEqual, http.StatusForbidden)
					So(body, ShouldEqual, "auth_token is missing\n")
				})

				Convey("if auth_token mismatches", func() {
					addr, cleanup := startWithTestListener(srv)
					defer cleanup()

					code, body, err := httpGet(fmt.Sprint(addr, "/hello"), "hi")
					So(err, ShouldBeNil)
					So(code, ShouldEqual, http.StatusForbidden)
					So(body, ShouldEqual, "no valid auth_token found\n")
				})
			})
		})

		Convey("serves pRPC requests", func() {
			addr, cleanup := startWithTestListener(srv)
			defer cleanup()
			req := &sinkpb.ReportTestResultsRequest{}
			url := fmt.Sprintf("%s/prpc/luci.resultdb.sink.v1.Sink/ReportTestResults", addr)

			Convey("with 200 OK", func() {
				code, err := prpcPost(url, "secret", req)
				So(err, ShouldBeNil)
				So(code, ShouldEqual, http.StatusOK)
			})

			Convey("with 403 Forbidden", func() {
				code, err := prpcPost(url, "not-secret", req)
				So(err, ShouldBeNil)
				So(code, ShouldEqual, http.StatusForbidden)
			})
		})

		Convey("handles panic", func() {
			addr, cleanup := startWithTestListener(srv)
			defer cleanup()

			code, _, err := httpGet(fmt.Sprint(addr, "/panic"), "secret")
			So(err, ShouldBeNil)
			So(code, ShouldEqual, http.StatusInternalServerError)
		})
	})
}

func TestServerExport(t *testing.T) {
	t.Parallel()

	Convey("Export returns the configured address and auth_token", t, func() {
		ctx := context.Background()
		srv, err := NewServer(ctx, ServerConfig{Address: ":42", AuthToken: "hello"})
		So(err, ShouldBeNil)
		ctx = srv.Export(ctx)
		sink := lucictx.GetResultSink(ctx)
		So(sink, ShouldNotBeNil)
		So(sink, ShouldNotBeNil)
		So(sink.Address, ShouldEqual, ":42")
		So(sink.AuthToken, ShouldEqual, "hello")
	})
}
