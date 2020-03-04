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
		req.Header.Add(AuthTokenKey, authTokenValue(authToken))
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
		Convey("succeeds", func() {
			srv := NewServer(ServerConfig{Address: ":42", AuthToken: "hello"})
			So(srv, ShouldNotBeNil)
		})
		Convey("uses the default address, if not given", func() {
			srv := NewServer(ServerConfig{})
			So(srv, ShouldNotBeNil)
			So(srv.cfg.Address, ShouldNotEqual, "")
		})
	})
}

func TestServer(t *testing.T) {
	t.Parallel()

	Convey("Server", t, func() {
		ctx := context.Background()
		srv := NewServer(ServerConfig{AuthToken: "secret"})
		So(srv, ShouldNotBeNil)

		addr, cleanup := installTestListener(srv)
		defer cleanup()

		Convey("Creates a random auth token, if not given", func() {
			srv.cfg.AuthToken = ""
			So(srv.Start(ctx), ShouldBeNil)
			So(srv.cfg.AuthToken, ShouldNotEqual, "")
		})

		Convey("Start", func() {
			So(srv.Start(ctx), ShouldBeNil)

			Convey("fails if called twice", func() {
				So(srv.Start(ctx), ShouldErrLike, "cannot call Start twice")
			})

			Convey("fails after being closed", func() {
				// close the server
				err := srv.Close()
				So(err, ShouldBeNil)
				So(<-srv.ErrC(), ShouldEqual, http.ErrServerClosed)

				// start after close should fail
				So(srv.Start(ctx), ShouldErrLike, "cannot call Start twice")
			})
		})

		Convey("Close closes the HTTP server", func() {
			So(srv.Start(ctx), ShouldBeNil)

			// check that the server is up.
			_, err := httpGet(addr, "secret")
			So(err, ShouldBeNil)

			// close the server
			err = srv.Close()
			So(err, ShouldBeNil)
			So(<-srv.ErrC(), ShouldEqual, http.ErrServerClosed)
		})

		Convey("Close fails before Start being called", func() {
			So(srv.Close(), ShouldErrLike, ErrCloseBeforeStart)
		})

		Convey("Closing the context closes the HTTP server", func() {
			ctx, cancel := context.WithCancel(ctx)
			So(srv.Start(ctx), ShouldBeNil)

			// check that the server is up.
			_, err := httpGet(addr, "secret")
			So(err, ShouldBeNil)

			// close the context, and check the server is down.
			cancel()
			So(<-srv.ErrC(), ShouldEqual, http.ErrServerClosed)
		})

		Convey("Run", func() {
			handlerErr := make(chan error)
			runErr := make(chan error)
			expected := errors.New("an error-1")

			Convey("succeeds", func() {
				// launch a go routine with Run
				go func() {
					runErr <- srv.Run(ctx, func(ctx context.Context) error {
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
					runErr <- srv.Run(ctx, func(ctx context.Context) error {
						<-ctx.Done()
						return ctx.Err()
					})
				}()

				// check that the server is running
				_, err := httpGet(fmt.Sprint(addr, "/hello"), "secret")
				So(err, ShouldBeNil)

				// close the server to emit a server error.
				srv.httpSrv.Close()
				So(<-runErr, ShouldEqual, http.ErrServerClosed)
			})
		})
	})
}

func TestServerExport(t *testing.T) {
	t.Parallel()

	Convey("Export returns the configured address and auth_token", t, func() {
		ctx := context.Background()
		srv := NewServer(ServerConfig{Address: ":42", AuthToken: "hello"})
		ctx = srv.Export(ctx)
		sink := lucictx.GetResultSink(ctx)
		So(sink, ShouldNotBeNil)
		So(sink, ShouldNotBeNil)
		So(sink.Address, ShouldEqual, ":42")
		So(sink.AuthToken, ShouldEqual, "hello")
	})
}
