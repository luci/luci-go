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

package rpcacl

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestInterceptor(t *testing.T) {
	t.Parallel()

	interceptor := Interceptor(Map{
		"/all/*":               All,
		"/authenticated/*":     Authenticated,
		"/authorized/*":        "some-group",
		"/mixed/all":           All,
		"/mixed/authenticated": Authenticated,
		"/mixed/authorized":    "some-group",
	}).Unary()

	check := func(ctx context.Context, service, method string) codes.Code {
		info := &grpc.UnaryServerInfo{
			FullMethod: fmt.Sprintf("/%s/%s", service, method),
		}
		_, err := interceptor(ctx, nil, info, func(context.Context, any) (any, error) {
			return nil, nil
		})
		return status.Code(err)
	}

	ftt.Run("Anonymous", t, func(t *ftt.Test) {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{})

		assert.Loosely(t, check(ctx, "all", "method"), should.Equal(codes.OK))
		assert.Loosely(t, check(ctx, "authenticated", "method"), should.Equal(codes.Unauthenticated))
		assert.Loosely(t, check(ctx, "authorized", "method"), should.Equal(codes.PermissionDenied))
		assert.Loosely(t, check(ctx, "unknown", "method"), should.Equal(codes.PermissionDenied))

		assert.Loosely(t, check(ctx, "mixed", "all"), should.Equal(codes.OK))
		assert.Loosely(t, check(ctx, "mixed", "authenticated"), should.Equal(codes.Unauthenticated))
		assert.Loosely(t, check(ctx, "mixed", "authorized"), should.Equal(codes.PermissionDenied))
		assert.Loosely(t, check(ctx, "mixed", "unknown"), should.Equal(codes.PermissionDenied))
	})

	ftt.Run("Authenticated, but not authorized", t, func(t *ftt.Test) {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"some-random-group"},
		})

		assert.Loosely(t, check(ctx, "all", "method"), should.Equal(codes.OK))
		assert.Loosely(t, check(ctx, "authenticated", "method"), should.Equal(codes.OK))
		assert.Loosely(t, check(ctx, "authorized", "method"), should.Equal(codes.PermissionDenied))
		assert.Loosely(t, check(ctx, "unknown", "method"), should.Equal(codes.PermissionDenied))

		assert.Loosely(t, check(ctx, "mixed", "all"), should.Equal(codes.OK))
		assert.Loosely(t, check(ctx, "mixed", "authenticated"), should.Equal(codes.OK))
		assert.Loosely(t, check(ctx, "mixed", "authorized"), should.Equal(codes.PermissionDenied))
		assert.Loosely(t, check(ctx, "mixed", "unknown"), should.Equal(codes.PermissionDenied))
	})

	ftt.Run("Authorized", t, func(t *ftt.Test) {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"some-group"},
		})

		assert.Loosely(t, check(ctx, "all", "method"), should.Equal(codes.OK))
		assert.Loosely(t, check(ctx, "authenticated", "method"), should.Equal(codes.OK))
		assert.Loosely(t, check(ctx, "authorized", "method"), should.Equal(codes.OK))
		assert.Loosely(t, check(ctx, "unknown", "method"), should.Equal(codes.PermissionDenied))

		assert.Loosely(t, check(ctx, "mixed", "all"), should.Equal(codes.OK))
		assert.Loosely(t, check(ctx, "mixed", "authenticated"), should.Equal(codes.OK))
		assert.Loosely(t, check(ctx, "mixed", "authorized"), should.Equal(codes.OK))
		assert.Loosely(t, check(ctx, "mixed", "unknown"), should.Equal(codes.PermissionDenied))
	})
}
