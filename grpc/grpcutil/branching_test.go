// Copyright 2025 The LUCI Authors.
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

package grpcutil

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestUnaryBranchingInterceptor(t *testing.T) {
	t.Parallel()

	const ctxTagKey = "context tag key"

	tagInCtx := func(ctx context.Context) string {
		val, _ := ctx.Value(ctxTagKey).(string)
		return val
	}

	fakeInterceptor := func(tag string) grpc.UnaryServerInterceptor {
		return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			if cur := tagInCtx(ctx); cur != "" {
				t.Errorf("Got unexpected context already tagged with %q", cur)
			}
			return handler(context.WithValue(ctx, ctxTagKey, tag), req)
		}
	}

	iceptor := UnaryBranchingInterceptor([]UnaryBranch{
		{
			Match:       MatchServices("some.Service"),
			Interceptor: fakeInterceptor("a"),
		},
		{
			Match:       MatchServices("some.Service"),
			Interceptor: fakeInterceptor("ignored"),
		},
		{
			Match:       MatchServices("another.Service", "third.Service"),
			Interceptor: fakeInterceptor("b"),
		},
	})

	call := func(fullMethod string) string {
		called := false
		gotTag := ""
		incoming := &emptypb.Empty{}
		iceptor(
			context.Background(),
			incoming,
			&grpc.UnaryServerInfo{FullMethod: fullMethod},
			func(ctx context.Context, req any) (any, error) {
				assert.That(t, called, should.BeFalse)
				assert.That(t, req, should.Equal(any(incoming)))
				called = true
				gotTag = tagInCtx(ctx)
				return nil, nil
			},
		)
		assert.That(t, called, should.BeTrue)
		return gotTag
	}

	assert.That(t, call("/some.Service/RPC"), should.Equal("a"))
	assert.That(t, call("/another.Service/RPC"), should.Equal("b"))
	assert.That(t, call("/third.Service/RPC"), should.Equal("b"))
	assert.That(t, call("/unknown.Service/RPC"), should.Equal(""))
	assert.That(t, call("/some.Servicezzzz/RPC"), should.Equal(""))
	assert.That(t, call("/Service/RPC"), should.Equal(""))
}
