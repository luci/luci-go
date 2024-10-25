// Copyright 2022 The LUCI Authors.
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

package rpcutil

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	tsmonstore "go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestIdentityKindCountingInterceptor(t *testing.T) {
	ctx, _ := tsmon.WithDummyInMemory(context.Background())
	store := tsmon.Store(ctx)
	interceptor := IdentityKindCountingInterceptor()
	fakeServerInfo := grpc.UnaryServerInfo{FullMethod: "/service/method"}
	fakeHandler := func(_ context.Context, _ any) (any, error) {
		return struct{}{}, status.Error(codes.Internal, "dummy error")
	}

	ftt.Run(`Increments counter`, t, func(t *ftt.Test) {
		t.Cleanup(func() {
			tsmon.GetState(ctx).SetStore(tsmonstore.NewInMemory(&target.Task{}))
			store = tsmon.GetState(ctx).Store()
		})
		t.Run(`user`, func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:user@example.com",
			})
			_, _ = interceptor(ctx, nil, &fakeServerInfo, fakeHandler)
			assert.Loosely(t, store.Get(ctx, IdentityKindCounter, []any{"service", "method", "user"}), should.Equal(1))
		})

		t.Run(`anonymous`, func(t *ftt.Test) {
			_, _ = interceptor(ctx, nil, &fakeServerInfo, fakeHandler)
			assert.Loosely(t, store.Get(ctx, IdentityKindCounter, []any{"service", "method", "user"}), should.BeNil)
			assert.Loosely(t, store.Get(ctx, IdentityKindCounter, []any{"service", "method", "anonymous"}), should.Equal(1))
		})
	})
}
