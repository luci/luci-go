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
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/tsmon"
	tsmonstore "go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIdentityKindCountingInterceptor(t *testing.T) {
	ctx, _ := tsmon.WithDummyInMemory(context.Background())
	store := tsmon.Store(ctx)
	interceptor := IdentityKindCountingInterceptor()
	fakeServerInfo := grpc.UnaryServerInfo{FullMethod: "/service/method"}
	fakeHandler := func(_ context.Context, _ any) (any, error) {
		return struct{}{}, status.Error(codes.Internal, "dummy error")
	}

	Convey(`Increments counter`, t, func() {
		Reset(func() {
			tsmon.GetState(ctx).SetStore(tsmonstore.NewInMemory(&target.Task{}))
			store = tsmon.GetState(ctx).Store()
		})
		Convey(`user`, func() {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:user@example.com",
			})
			_, _ = interceptor(ctx, nil, &fakeServerInfo, fakeHandler)
			So(store.Get(ctx, IdentityKindCounter, time.Time{}, []any{"service", "method", "user"}), ShouldEqual, 1)
		})

		Convey(`anonymous`, func() {
			_, _ = interceptor(ctx, nil, &fakeServerInfo, fakeHandler)
			So(store.Get(ctx, IdentityKindCounter, time.Time{}, []any{"service", "method", "user"}), ShouldBeNil)
			So(store.Get(ctx, IdentityKindCounter, time.Time{}, []any{"service", "method", "anonymous"}), ShouldEqual, 1)
		})
	})
}
