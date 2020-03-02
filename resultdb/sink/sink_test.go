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
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/server/router"

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

func httpGet(url, authToken string) (int, error) {
	// create request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	if authToken != "" {
		req.Header.Add(AuthTokenKey, AuthTokenValue(authToken))
	}

	// send
	c := http.Client{}
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
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

		// install a handler that panics
		routes, _ := srv.httpSrv.Handler.(*router.Router)
		routes.GET("/panic", router.MiddlewareChain{}, func(c *router.Context) {
			panic("hello!")
		})

		addr, cleanup := installTestListener(srv)
		defer cleanup()

		Convey("Start", func() {
			So(srv.Start(), ShouldBeNil)

			Convey("fails if called twice", func() {
				So(srv.Start(), ShouldErrLike, "cannot call Start twice")
			})

			Convey("fails after being closed", func() {
				// close the server
				err = srv.Close()
				So(err, ShouldBeNil)
				So(<-srv.ErrC(), ShouldEqual, http.ErrServerClosed)

				// start after close should fail
				So(srv.Start(), ShouldErrLike, "cannot call Start twice")
			})
		})

		Convey("Close closes the HTTP server", func() {
			So(srv.Start(), ShouldBeNil)

			// check that the server is up.
			_, err := httpGet(addr, "secret")
			So(err, ShouldBeNil)

			// close the server
			err = srv.Close()
			So(err, ShouldBeNil)
			So(<-srv.ErrC(), ShouldEqual, http.ErrServerClosed)
		})

		Convey("Run", func() {
			handlerErr := make(chan error)
			runErr := make(chan error)
			expected := errors.New("an error-1")

			Convey("succeeds", func() {
				// launch a go routine with Run
				go func() {
					runErr <- srv.Run(func(ctx context.Context) error {
						return <-handlerErr
					})
				}()

				// check that the server is running
				_, err := httpGet(fmt.Sprint(addr, "/hello"), "secret")
				So(err, ShouldBeNil)

				// finish the callback and verify that the server stopped running
				handlerErr <- expected
				So(<-srv.ErrC(), ShouldEqual, http.ErrServerClosed)
				So(<-runErr, ShouldEqual, expected)
			})

			Convey("aborts after server error", func() {
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
				_, err := httpGet(fmt.Sprint(addr, "/hello"), "secret")
				So(err, ShouldBeNil)

				// emit a server error. Run should return the server error.
				srv.errC <- expected
				So(<-runErr, ShouldEqual, expected)
				So(<-srv.ErrC(), ShouldEqual, http.ErrServerClosed)
			})
		})

		Convey("handles panic", func() {
			So(srv.Start(), ShouldBeNil)
			code, err := httpGet(fmt.Sprint(addr, "/panic"), "secret")
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
