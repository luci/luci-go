// Copyright 2021 The LUCI Authors.
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
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"

	. "github.com/smartystreets/goconvey/convey"
)

type testStatsHandler struct {
	handleRPC func(context.Context, stats.RPCStats)
}

func (tsh *testStatsHandler) HandleRPC(ctx context.Context, s stats.RPCStats) {
	tsh.handleRPC(ctx, s)
}

func (tsh *testStatsHandler) TagRPC(ctx context.Context, i *stats.RPCTagInfo) context.Context {
	return ctx
}

func (tsh *testStatsHandler) TagConn(ctx context.Context, t *stats.ConnTagInfo) context.Context {
	return ctx
}

// HandleConn processes the Conn stats.
func (tsh *testStatsHandler) HandleConn(context.Context, stats.ConnStats) {
	// do nothing
}

func TestWithMultiStatsHandler(t *testing.T) {
	Convey("Test WithMultiStatsHandler", t, func() {
		ctx := context.Background()
		h1 := &testStatsHandler{}
		h2 := &testStatsHandler{}

		// spin up a server
		srv, svc := grpc.NewServer(), &echoService{}
		RegisterEchoServer(srv, svc)
		l, err := net.Listen("tcp", "localhost:0")
		So(err, ShouldBeNil)
		go func() { _ = srv.Serve(l) }()
		defer srv.Stop()

		connect := func(opt grpc.DialOption) *grpc.ClientConn {
			conn, err := grpc.Dial(
				l.Addr().String(),
				grpc.WithInsecure(),
				grpc.WithBlock(),
				opt,
			)
			So(err, ShouldBeNil)
			return conn
		}

		Convey("w/o params", func() {
			conn := connect(WithMultiStatsHandler())
			defer func() { So(conn.Close(), ShouldBeNil) }()
			NewEchoClient(conn).Say(ctx, &SayRequest{Msg: "echo!"})
		})

		Convey("w/ testStatsHandler", func() {
			called := false
			h1.handleRPC = func(context.Context, stats.RPCStats) {
				called = true
			}

			Convey("alone", func() {
				conn := connect(WithMultiStatsHandler(h1))
				defer func() { So(conn.Close(), ShouldBeNil) }()
				NewEchoClient(conn).Say(ctx, &SayRequest{Msg: "echo!"})
				So(called, ShouldBeTrue)
			})

			Convey("w/ nil", func() {
				conn := connect(WithMultiStatsHandler(nil, h1, nil))
				defer func() { So(conn.Close(), ShouldBeNil) }()
				NewEchoClient(conn).Say(ctx, &SayRequest{Msg: "echo!"})
				So(called, ShouldBeTrue)
			})
		})

		Convey("runs the handlers in order", func() {
			values := []int{}
			c1, c2 := 0, 0

			h1.handleRPC = func(context.Context, stats.RPCStats) {
				if c1 == 0 { // handleRPC is called multiple times per RPC call.
					values = append(values, 2)
				}
				c1 += 1
			}
			h2.handleRPC = func(context.Context, stats.RPCStats) {
				if c2 == 0 {
					values = append(values, 1)
				}
				c2 += 1
			}
			conn := connect(WithMultiStatsHandler(h1, h2))
			defer func() { So(conn.Close(), ShouldBeNil) }()
			NewEchoClient(conn).Say(ctx, &SayRequest{Msg: "echo!"})
			So(values, ShouldResemble, []int{2, 1})
		})

	})
}
