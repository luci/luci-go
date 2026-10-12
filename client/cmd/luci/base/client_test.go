// Copyright 2026 The LUCI Authors.
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

package base

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestNewAnalysisClients(t *testing.T) {
	ftt.Run(`NewAnalysisClients`, t, func(t *ftt.Test) {
		ctx := context.Background()
		af := &AuthFlags{DefaultOpts: auth.Options{SecretsDir: t.TempDir()}}

		// Without Parse(), NewAnalysisClients returns an error.
		_, _, _, err := af.NewAnalysisClients(ctx, chromeinfra.AnalysisHost)
		assert.Loosely(t, err, should.NotBeNil)

		// After Parse(), NewAnalysisClients returns all three pRPC clients.
		assert.Loosely(t, af.Parse(), should.BeNil)
		rulesClient, clustersClient, testHistoryClient, err := af.NewAnalysisClients(ctx, chromeinfra.AnalysisHost)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, rulesClient, should.NotBeNil)
		assert.Loosely(t, clustersClient, should.NotBeNil)
		assert.Loosely(t, testHistoryClient, should.NotBeNil)
	})
}

type mockResultDBClient struct {
	pb.ResultDBClient
	getRootInvocation func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error)
	getInvocation     func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error)
}

func (m *mockResultDBClient) GetRootInvocation(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
	if m.getRootInvocation != nil {
		return m.getRootInvocation(ctx, in)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func (m *mockResultDBClient) GetInvocation(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error) {
	if m.getInvocation != nil {
		return m.getInvocation(ctx, in)
	}
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

func TestIsLegacyInvocation(t *testing.T) {
	ftt.Run(`IsLegacyInvocation`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`nil client or empty ID returns false`, func(t *ftt.Test) {
			assert.Loosely(t, IsLegacyInvocation(ctx, nil, "build-123"), should.BeFalse)
			mock := &mockResultDBClient{}
			assert.Loosely(t, IsLegacyInvocation(ctx, mock, ""), should.BeFalse)
		})

		t.Run(`root invocation found -> false`, func(t *ftt.Test) {
			mock := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					assert.Loosely(t, in.Name, should.Equal("rootInvocations/build-123"))
					return &pb.RootInvocation{Name: in.Name}, nil
				},
			}
			assert.Loosely(t, IsLegacyInvocation(ctx, mock, "build-123"), should.BeFalse)
		})

		t.Run(`root invocation NotFound + legacy GetInvocation OK -> true`, func(t *ftt.Test) {
			mock := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return nil, status.Error(codes.NotFound, "not found")
				},
				getInvocation: func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error) {
					assert.Loosely(t, in.Name, should.Equal("invocations/build-123"))
					return &pb.Invocation{Name: in.Name}, nil
				},
			}
			assert.Loosely(t, IsLegacyInvocation(ctx, mock, "123"), should.BeTrue)
		})

		t.Run(`root invocation NotFound + legacy GetInvocation error -> false`, func(t *ftt.Test) {
			mock := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return nil, status.Error(codes.NotFound, "not found")
				},
				getInvocation: func(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error) {
					return nil, status.Error(codes.NotFound, "not found")
				},
			}
			assert.Loosely(t, IsLegacyInvocation(ctx, mock, "build-123"), should.BeFalse)
		})

		t.Run(`root invocation returns internal error -> false`, func(t *ftt.Test) {
			mock := &mockResultDBClient{
				getRootInvocation: func(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
					return nil, status.Error(codes.Internal, "internal error")
				},
			}
			assert.Loosely(t, IsLegacyInvocation(ctx, mock, "build-123"), should.BeFalse)
		})
	})
}
