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
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon/distribution"
)

type echoService struct {
	err error
}

func (s *echoService) Say(ctx context.Context, req *SayRequest) (*SayResponse, error) {
	return &SayResponse{Msg: req.GetMsg()}, s.err
}

func TestClientRPCStatsMonitor(t *testing.T) {
	method := "/grpcmon.Echo/Say"
	fields := func(fs ...any) (ret []any) {
		return append([]any{method}, fs...)
	}

	ftt.Run("ClientRPCStatsMonitor", t, func(t *ftt.Test) {
		// spin up a server
		srv, svc := grpc.NewServer(), &echoService{}
		RegisterEchoServer(srv, svc)
		l, err := net.Listen("tcp", "localhost:0")
		assert.Loosely(t, err, should.BeNil)
		go func() { _ = srv.Serve(l) }()
		defer srv.Stop()

		// construct a client
		conn, err := grpc.NewClient(
			l.Addr().String(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithStatsHandler(&ClientRPCStatsMonitor{}),
		)
		assert.Loosely(t, err, should.BeNil)
		defer func() { assert.Loosely(t, conn.Close(), should.BeNil) }()
		client := NewEchoClient(conn)
		ctx, memStore := testContext()

		run := func(err error, msg string) {
			svc.err = err
			resp, rerr := client.Say(ctx, &SayRequest{Msg: msg})
			if err == nil {
				assert.Loosely(t, rerr, should.BeNil)
				assert.Loosely(t, resp.GetMsg(), should.Equal(msg))
			} else {
				assert.Loosely(t, rerr.Error(), should.Equal(err.Error()))
			}
		}
		t.Run("Captures count and duration", func(t *ftt.Test) {
			count := func(code string) int64 {
				val := memStore.Get(ctx, grpcClientCount, fields(code))
				assert.Loosely(t, val, should.NotBeZero)
				return val.(int64)
			}
			duration := func(code string) any {
				return memStore.Get(ctx, grpcClientDuration, fields(code))
			}

			// grpc uses time.Now() to assign a value to
			// grpc.End.{BeginTime, EndTime}, and we cannot stub it out.
			//
			// Therefore, this only checks the duration has been set or not.
			// i.e., nil or not.
			assert.Loosely(t, duration("OK"), should.BeNil)
			run(nil, "echo!")
			assert.Loosely(t, count("OK"), should.Equal(1))
			assert.Loosely(t, duration("OK"), should.NotBeNil)

			assert.Loosely(t, duration("PERMISSION_DENIED"), should.BeNil)
			run(status.Error(codes.PermissionDenied, "no permission"), "echo!")
			assert.Loosely(t, count("PERMISSION_DENIED"), should.Equal(1))
			assert.Loosely(t, duration("PERMISSION_DENIED"), should.NotBeNil)

			assert.Loosely(t, duration("UNAUTHENTICATED"), should.BeNil)
			run(status.Error(codes.Unauthenticated, "no auth"), "echo!")
			assert.Loosely(t, count("UNAUTHENTICATED"), should.Equal(1))
			assert.Loosely(t, duration("UNAUTHENTICATED"), should.NotBeNil)
		})

		t.Run("Captures sent/received messages", func(t *ftt.Test) {
			count := func(code string) (float64, float64) {
				sent := memStore.Get(ctx, grpcClientSentMsg, fields())
				assert.Loosely(t, sent, should.NotBeNil)
				recv := memStore.Get(ctx, grpcClientRecvMsg, fields())
				assert.Loosely(t, recv, should.NotBeNil)
				return sent.(*distribution.Distribution).Sum(), recv.(*distribution.Distribution).Sum()
			}
			bytes := func(code string) (float64, float64) {
				sent := memStore.Get(ctx, grpcClientSentByte, fields())
				assert.Loosely(t, sent, should.NotBeNil)
				recv := memStore.Get(ctx, grpcClientRecvByte, fields())
				assert.Loosely(t, recv, should.NotBeNil)
				return sent.(*distribution.Distribution).Sum(), recv.(*distribution.Distribution).Sum()
			}

			run(nil, "echo!")
			sentCount, recvCount := count("OK")
			assert.Loosely(t, sentCount, should.Equal(1.0))
			assert.Loosely(t, recvCount, should.Equal(1.0))

			sentBytes, recvBytes := bytes("OK")
			assert.Loosely(t, sentBytes, should.BeGreaterThan(0.0))
			assert.Loosely(t, recvBytes, should.BeGreaterThan(0.0))
		})
	})
}
