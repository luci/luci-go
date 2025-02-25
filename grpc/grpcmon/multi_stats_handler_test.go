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
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/stats"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type testStatsHandler struct {
	tagRPC func()
}

func (tsh *testStatsHandler) HandleRPC(ctx context.Context, s stats.RPCStats) {
}

func (tsh *testStatsHandler) TagRPC(ctx context.Context, i *stats.RPCTagInfo) context.Context {
	tsh.tagRPC()
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
	ftt.Run("Test WithMultiStatsHandler", t, func(t *ftt.Test) {
		ctx := context.Background()
		h1 := &testStatsHandler{}
		h2 := &testStatsHandler{}

		// spin up a server
		srv, svc := grpc.NewServer(), &echoService{}
		RegisterEchoServer(srv, svc)
		l, err := net.Listen("tcp", "localhost:0")
		assert.Loosely(t, err, should.BeNil)
		go func() { _ = srv.Serve(l) }()
		defer srv.Stop()

		connect := func(opt grpc.DialOption) *grpc.ClientConn {
			conn, err := grpc.NewClient(
				l.Addr().String(),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				opt,
			)
			assert.Loosely(t, err, should.BeNil)
			return conn
		}

		t.Run("w/o params", func(t *ftt.Test) {
			conn := connect(WithMultiStatsHandler())
			defer func() { assert.Loosely(t, conn.Close(), should.BeNil) }()
			NewEchoClient(conn).Say(ctx, &SayRequest{Msg: "echo!"})
		})

		t.Run("w/ testStatsHandler", func(t *ftt.Test) {
			ch := make(chan int, 1)
			h1.tagRPC = func() {
				ch <- 1
			}

			check := func(t testing.TB) {
				t.Helper()

				close(ch)
				assert.Loosely(t, <-ch, should.Equal(1), truth.LineContext())
			}

			t.Run("alone", func(t *ftt.Test) {
				conn := connect(WithMultiStatsHandler(h1))
				defer func() { assert.Loosely(t, conn.Close(), should.BeNil) }()
				NewEchoClient(conn).Say(ctx, &SayRequest{Msg: "echo!"})
				check(t)
			})

			t.Run("w/ nil", func(t *ftt.Test) {
				conn := connect(WithMultiStatsHandler(nil, h1, nil))
				defer func() { assert.Loosely(t, conn.Close(), should.BeNil) }()
				NewEchoClient(conn).Say(ctx, &SayRequest{Msg: "echo!"})
				check(t)
			})
		})

		t.Run("runs the handlers in order", func(t *ftt.Test) {
			ch := make(chan int, 3)
			h1.tagRPC = func() {
				ch <- 2
			}
			h2.tagRPC = func() {
				ch <- 1
			}
			conn := connect(WithMultiStatsHandler(h1, h2, h1))
			defer func() { assert.Loosely(t, conn.Close(), should.BeNil) }()
			NewEchoClient(conn).Say(ctx, &SayRequest{Msg: "echo!"})

			close(ch)
			values := []int{}
			for val := range ch {
				values = append(values, val)
			}
			assert.Loosely(t, values, should.Match([]int{2, 1, 2}))
		})

	})
}
