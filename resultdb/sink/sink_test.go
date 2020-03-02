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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/lucictx"

	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func installTestListener(srv *Server) (string, func() error) {
	l, err := net.Listen("tcp", "localhost:0")
	So(err, ShouldBeNil)
	srv.testListener = l

	// return the serving address
	return fmt.Sprint("localhost:", l.Addr().(*net.TCPAddr).Port), l.Close
}

func reportTestResults(ctx context.Context, host, authToken string, in *sinkpb.ReportTestResultsRequest) (*sinkpb.ReportTestResultsResponse, error) {
	client := &prpc.Client{
		Host:    host,
		Options: &prpc.Options{Insecure: true},
	}
	// install the auth token into the context, if present
	if authToken != "" {
		ctx = metadata.NewOutgoingContext(
			ctx, metadata.Pairs(AuthTokenKey, AuthTokenValue(authToken)),
		)
	}
	out := &sinkpb.ReportTestResultsResponse{}
	err := client.Call(ctx, "luci.resultdb.sink.v1.Sink", "ReportTestResults", in, out)
	if err != nil {
		return nil, err
	}
	return out, nil
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
		req := &sinkpb.ReportTestResultsRequest{}
		ctx := context.Background()
		srv, err := NewServer(ctx, ServerConfig{AuthToken: "secret"})
		So(err, ShouldBeNil)
		So(srv, ShouldNotBeNil)

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
			_, err := reportTestResults(ctx, addr, "secret", req)
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
				_, err = reportTestResults(ctx, addr, "secret", req)
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
				_, err = reportTestResults(ctx, addr, "secret", req)
				So(err, ShouldBeNil)

				// emit a server error. Run should return the server error.
				srv.errC <- expected
				So(<-runErr, ShouldEqual, expected)
				So(<-srv.ErrC(), ShouldEqual, http.ErrServerClosed)
			})
		})

		Convey("serves requests", func() {
			So(srv.Start(), ShouldBeNil)

			Convey("with 200 OK", func() {
				res, err := reportTestResults(ctx, addr, "secret", req)
				So(err, ShouldBeNil)
				So(res, ShouldNotBeNil)
			})

			Convey("with 401 Unauthorized if the auth_token missing", func() {
				_, err := reportTestResults(ctx, addr, "", req)
				So(err, ShouldErrLike, codes.Unauthenticated.String())
			})

			Convey("with 403 Forbidden if auth_token mismatched", func() {
				_, err := reportTestResults(ctx, addr, "not-a-secret", req)
				So(err, ShouldErrLike, codes.PermissionDenied.String())
			})
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
