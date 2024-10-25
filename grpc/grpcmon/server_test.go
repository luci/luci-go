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
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
)

func TestUnaryServerInterceptor(t *testing.T) {
	ftt.Run("Captures count and duration", t, func(t *ftt.Test) {
		c, memStore := testContext()

		// Handler that runs for 500 ms.
		handler := func(ctx context.Context, req any) (any, error) {
			clock.Get(ctx).(testclock.TestClock).Add(500 * time.Millisecond)
			return struct{}{}, status.Error(codes.Internal, "errored internally")
		}

		// Run the handler with the interceptor.
		UnaryServerInterceptor(c, nil, &grpc.UnaryServerInfo{
			FullMethod: "/service/method",
		}, handler)

		count := memStore.Get(c, grpcServerCount, []any{"/service/method", 13, "INTERNAL"})
		assert.Loosely(t, count, should.Equal(1))

		duration := memStore.Get(c, grpcServerDuration, []any{"/service/method", 13, "INTERNAL"})
		assert.Loosely(t, duration.(*distribution.Distribution).Sum(), should.Equal(500.0))
	})
}

func TestStreamServerInterceptor(t *testing.T) {
	ftt.Run("Captures count and duration", t, func(t *ftt.Test) {
		c, memStore := testContext()

		// Handler that runs for 500 ms.
		handler := func(srv any, ss grpc.ServerStream) error {
			clock.Get(ss.Context()).(testclock.TestClock).Add(500 * time.Millisecond)
			return status.Error(codes.Internal, "errored internally")
		}

		// Run the handler with the interceptor.
		StreamServerInterceptor(nil, &serverStream{nil, c}, &grpc.StreamServerInfo{
			FullMethod: "/service/method",
		}, handler)

		count := memStore.Get(c, grpcServerCount, []any{"/service/method", 13, "INTERNAL"})
		assert.Loosely(t, count, should.Equal(1))

		duration := memStore.Get(c, grpcServerDuration, []any{"/service/method", 13, "INTERNAL"})
		assert.Loosely(t, duration.(*distribution.Distribution).Sum(), should.Equal(500.0))
	})
}

type serverStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (ss *serverStream) Context() context.Context {
	return ss.ctx
}

func testContext() (context.Context, store.Store) {
	c := context.Background()
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
	c = gologger.StdConfig.Use(c)
	c, _, _ = tsmon.WithFakes(c)
	memStore := store.NewInMemory(&target.Task{})
	tsmon.GetState(c).SetStore(memStore)
	return c, memStore
}
