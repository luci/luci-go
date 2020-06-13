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
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/lucictx"

	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestNewServer(t *testing.T) {
	t.Parallel()
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	Convey("NewServer", t, func() {
		ctx := context.Background()

		Convey("succeeds", func() {
			srv, err := NewServer(ctx, testServerConfig(ctl, ":42", "my_token"))
			So(err, ShouldBeNil)
			So(srv, ShouldNotBeNil)
		})
		Convey("uses the default address, if missing", func() {
			srv, err := NewServer(ctx, testServerConfig(ctl, "", "my_token"))
			So(err, ShouldBeNil)
			So(srv, ShouldNotBeNil)
			So(srv.cfg.Address, ShouldNotEqual, "")
		})
		Convey("generates a random auth token, if missing", func() {
			srv, err := NewServer(ctx, testServerConfig(ctl, ":42", ""))
			So(err, ShouldBeNil)
			So(srv, ShouldNotBeNil)
			So(srv.cfg.AuthToken, ShouldNotEqual, "")
		})
	})
}

func TestServer(t *testing.T) {
	t.Parallel()
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	Convey("Server", t, func() {
		req := &sinkpb.ReportTestResultsRequest{}
		ctx := context.Background()

		// a test server with a test listener
		srvCfg := testServerConfig(ctl, "", "secret")
		addr, cleanup := installTestListener(&srvCfg)
		defer cleanup()
		srv, err := NewServer(ctx, srvCfg)
		So(err, ShouldBeNil)

		Convey("Start fails", func() {
			So(srv.Start(ctx), ShouldBeNil)

			Convey("if called twice", func() {
				So(srv.Start(ctx), ShouldErrLike, "cannot call Start twice")
			})

			Convey("after being closed", func() {
				So(srv.Close(), ShouldBeNil)
				So(srv.Start(ctx), ShouldErrLike, "cannot call Start twice")
			})
		})

		Convey("Close closes the HTTP server", func() {
			So(srv.Start(ctx), ShouldBeNil)
			So(srv.Close(), ShouldBeNil)

			_, err := reportTestResults(ctx, addr, "secret", req)
			// OSes may return different messages for connection errors, but seem to return
			// with "connection" always.
			// : e.g., "No connection could be made", "connection refused"
			So(err, ShouldErrLike, "connection")
		})

		Convey("Close fails before Start being called", func() {
			So(srv.Close(), ShouldErrLike, ErrCloseBeforeStart)
		})

		Convey("Shutdown closes Done", func() {
			isClosed := func() bool {
				select {
				case <-srv.Done():
					return true
				default:
					return false
				}
			}
			So(srv.Start(ctx), ShouldBeNil)

			// check that the server is up.
			_, err := reportTestResults(ctx, addr, "secret", req)
			So(err, ShouldBeNil)

			So(isClosed(), ShouldBeFalse)
			So(srv.Shutdown(ctx), ShouldBeNil)
			So(isClosed(), ShouldBeTrue)
		})

		Convey("Run", func() {
			handlerErr := make(chan error)
			runErr := make(chan error)
			expected := errors.New("an error-1")

			Convey("succeeds", func() {
				// launch a go routine with Run
				go func() {
					runErr <- Run(ctx, srvCfg, func(ctx context.Context, cfg ServerConfig) error {
						return <-handlerErr
					})
				}()

				// check that the server is running
				_, err := reportTestResults(ctx, addr, "secret", req)
				So(err, ShouldBeNil)

				// finish the callback and verify that srv.Run returned what the callback
				// returned.
				handlerErr <- expected
				So(<-runErr, ShouldEqual, expected)
			})

			Convey("aborts after server error", func() {
				// launch a go routine with Run
				go func() {
					runErr <- Run(ctx, srvCfg, func(ctx context.Context, cfg ServerConfig) error {
						<-ctx.Done()
						return ctx.Err()
					})
				}()

				// check that the server is running
				_, err := reportTestResults(ctx, addr, "secret", req)
				So(err, ShouldBeNil)

				// close the test listener so that the server terminates.
				cleanup()
				So(<-runErr, ShouldEqual, context.Canceled)
			})
		})

		Convey("serves requests", func() {
			So(srv.Start(ctx), ShouldBeNil)

			Convey("with 200 OK", func() {
				res, err := reportTestResults(ctx, addr, "secret", req)
				So(err, ShouldBeNil)
				So(res, ShouldNotBeNil)
			})

			Convey("with 401 Unauthorized if the auth_token missing", func() {
				_, err := reportTestResults(ctx, addr, "", req)
				So(status.Code(err), ShouldEqual, codes.Unauthenticated)
			})

			Convey("with 403 Forbidden if auth_token mismatched", func() {
				_, err := reportTestResults(ctx, addr, "not-a-secret", req)
				So(status.Code(err), ShouldEqual, codes.PermissionDenied)
			})
		})
	})
}

func TestServerExport(t *testing.T) {
	t.Parallel()
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	Convey("Export returns the configured address and auth_token", t, func() {
		ctx := context.Background()
		srv, err := NewServer(ctx, testServerConfig(ctl, ":42", "hello"))
		So(err, ShouldBeNil)

		ctx = srv.Export(ctx)
		sink := lucictx.GetResultSink(ctx)
		So(sink, ShouldNotBeNil)
		So(sink, ShouldNotBeNil)
		So(sink.Address, ShouldEqual, ":42")
		So(sink.AuthToken, ShouldEqual, "hello")
	})
}
