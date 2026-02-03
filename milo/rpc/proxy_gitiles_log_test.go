// Copyright 2024 The LUCI Authors.
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

package rpc

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"

	milopb "go.chromium.org/luci/milo/proto/v1"
)

type mockGitilesClient struct {
	// Embed the interface to avoid having to implement all methods.
	gitiles.GitilesClient
	LogFunc func(ctx context.Context, in *gitiles.LogRequest, opts ...grpc.CallOption) (*gitiles.LogResponse, error)
}

func (m *mockGitilesClient) Log(ctx context.Context, in *gitiles.LogRequest, opts ...grpc.CallOption) (*gitiles.LogResponse, error) {
	return m.LogFunc(ctx, in, opts...)
}

func TestProxyGitilesLog(t *testing.T) {
	t.Parallel()

	ftt.Run("ProxyGitilesLog", t, func(t *ftt.Test) {
		ctx := context.Background()
		svcImpl := &MiloInternalService{}
		svc := WithStatusDecorator(svcImpl)

		t.Run("request validation", func(t *ftt.Test) {
			_, err := svc.ProxyGitilesLog(ctx, &milopb.ProxyGitilesLogRequest{
				Host:    "invalid.host",
				Request: &gitiles.LogRequest{},
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("host: must be a subdomain of .googlesource.com"))
		})
		t.Run("passes through errors correctly", func(t *ftt.Test) {
			svcImpl.GetGitilesClient = func(c context.Context, host string, as auth.RPCAuthorityKind) (gitiles.GitilesClient, error) {
				return &mockGitilesClient{
					LogFunc: func(ctx context.Context, in *gitiles.LogRequest, opts ...grpc.CallOption) (*gitiles.LogResponse, error) {
						return nil, status.Error(codes.NotFound, "test error message")
					},
				}, nil
			}

			_, err := svc.ProxyGitilesLog(ctx, &milopb.ProxyGitilesLogRequest{
				Host: "chromium.googlesource.com",
				Request: &gitiles.LogRequest{
					Project: "project",
				},
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("test error message"))
		})
		t.Run("end-to-end success", func(t *ftt.Test) {
			expectedRequest := &gitiles.LogRequest{
				Project:    "chromium/src",
				Committish: strings.Repeat("a", 40),
			}
			expectedResponse := &gitiles.LogResponse{
				Log: []*git.Commit{
					{Id: strings.Repeat("a", 40)},
				},
			}
			svcImpl.GetGitilesClient = func(c context.Context, host string, as auth.RPCAuthorityKind) (gitiles.GitilesClient, error) {
				return &mockGitilesClient{
					LogFunc: func(ctx context.Context, in *gitiles.LogRequest, opts ...grpc.CallOption) (*gitiles.LogResponse, error) {
						assert.Loosely(t, in, should.Match(expectedRequest))
						return proto.Clone(expectedResponse).(*gitiles.LogResponse), nil
					},
				}, nil
			}

			rsp, err := svc.ProxyGitilesLog(ctx, &milopb.ProxyGitilesLogRequest{
				Host:    "chromium.googlesource.com",
				Request: proto.Clone(expectedRequest).(*gitiles.LogRequest),
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp, should.Match(expectedResponse))
		})
	})
}
