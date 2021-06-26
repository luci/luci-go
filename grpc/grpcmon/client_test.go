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

package grpcmon

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
)

type echoService struct {
	err error
}

func (s *echoService) Say(c context.Context, req *SayRequest) (*SayResponse, error) {
	return &SayResponse{Msg: req.GetMsg()}, s.err
}

func testServer(ctx context.Context) *http.Server {
	r := router.NewWithRootContext(ctx)
	server := prpc.Server{Authenticator: prpc.NoAuthentication}
	server.InstallHandlers(r, nil)
	RegisterEchoServer(&server, &echoService{})
	return &http.Server{Handler: r}
}

func TestClientRPCStatsMonitor(t *testing.T) {
	method := "/grpcmon.Echo/Say"

	Convey("ClientRPCStatsMonitor", t, func() {
		// spin up a server
		srv, svc := grpc.NewServer(), &echoService{}
		RegisterEchoServer(srv, svc)
		l, err := net.Listen("tcp", "localhost:0")
		So(err, ShouldBeNil)
		go srv.Serve(l)
		defer srv.Stop()

		// construct a client
		conn, err := grpc.Dial(
			l.Addr().String(),
			grpc.WithInsecure(),
			grpc.WithBlock(),
			grpc.WithStatsHandler(&ClientRPCStatsMonitor{}),
		)
		So(err, ShouldBeNil)
		defer conn.Close()
		client := NewEchoClient(conn)
		ctx, memStore := testContext()

		run := func(err error, msg string) {
			svc.err = err
			resp, rerr := client.Say(ctx, &SayRequest{Msg: msg})
			if err == nil {
				So(resp.GetMsg(), ShouldEqual, msg)
			}
			So(rerr, ShouldResemble, err)
		}
		count := func(code string) int64 {
			val := memStore.Get(ctx, grpcClientCount, time.Time{}, []interface{}{method, code})
			So(val, ShouldNotBeNil)
			return val.(int64)
		}
		duration := func(code string) interface{} {
			return memStore.Get(ctx, grpcClientDuration, time.Time{}, []interface{}{method, code})
		}

		Convey("Captures count and duration", func() {
			// grpc uses time.Now() to assign a value to grpc.End.{BeginTime, EndTime}, and
			// we cannot stub it out.
			//
			// Therefore, this only checks the duration has been set or not.
			// i.e., nil or not.
			So(duration("OK"), ShouldBeNil)
			run(nil, "echo!")
			So(count("OK"), ShouldEqual, 1)
			So(duration("OK"), ShouldNotBeNil)

			So(duration("PERMISSION_DENIED"), ShouldBeNil)
			run(status.Error(codes.PermissionDenied, "no permission"), "echo!")
			So(count("PERMISSION_DENIED"), ShouldEqual, 1)
			So(duration("PERMISSION_DENIED"), ShouldNotBeNil)

			So(duration("UNAUTHENTICATED"), ShouldBeNil)
			run(status.Error(codes.Unauthenticated, "no auth"), "echo!")
			So(count("UNAUTHENTICATED"), ShouldEqual, 1)
			So(duration("UNAUTHENTICATED"), ShouldNotBeNil)
		})
	})
}
