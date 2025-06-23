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

package resultdb

import (
	"context"

	"github.com/golang/mock/gomock"

	"go.chromium.org/luci/common/proto"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

// MockedClient is a mocked ResultDB client for testing.
// It wraps a rdbpb.MockResultDBClient and a context with the mocked client.
type MockedClient struct {
	Client *rdbpb.MockResultDBClient
	Ctx    context.Context
}

// NewMockedClient creates a MockedClient for testing.
func NewMockedClient(ctx context.Context, ctl *gomock.Controller) *MockedClient {
	mockClient := rdbpb.NewMockResultDBClient(ctl)
	return &MockedClient{
		Client: mockClient,
		Ctx:    context.WithValue(ctx, &mockResultDBClientKey, mockClient),
	}
}

// QueryTestVariants mocks the QueryTestVariants RPC.
func (mc *MockedClient) QueryTestVariants(req *rdbpb.QueryTestVariantsRequest, res *rdbpb.QueryTestVariantsResponse) {
	mc.Client.EXPECT().QueryTestVariants(gomock.Any(), proto.MatcherEqual(req),
		gomock.Any()).Return(res, nil)
}

// QueryArtifacts mocks the QueryArtifacts RPC.
func (mc *MockedClient) QueryArtifacts(req *rdbpb.QueryArtifactsRequest, res *rdbpb.QueryArtifactsResponse) {
	mc.Client.EXPECT().QueryArtifacts(gomock.Any(), proto.MatcherEqual(req),
		gomock.Any()).Return(res, nil)
}

// QueryRunTestVerdicts mocks the QueryRunTestVerdicts RPC.
func (mc *MockedClient) QueryRunTestVerdicts(req *rdbpb.QueryRunTestVerdictsRequest, res *rdbpb.QueryRunTestVerdictsResponse) {
	mc.Client.EXPECT().QueryRunTestVerdicts(gomock.Any(), proto.MatcherEqual(req),
		gomock.Any()).Return(res, nil)
}

// GetInvocation mocks the GetInvocation RPC.
func (mc *MockedClient) GetInvocation(req *rdbpb.GetInvocationRequest, res *rdbpb.Invocation) {
	mc.Client.EXPECT().GetInvocation(gomock.Any(), proto.MatcherEqual(req),
		gomock.Any()).Return(res, nil)
}

// GetRealm is a shortcut of GetInvocation to get realm of the invocation.
func (mc *MockedClient) GetRealm(inv, realm string) {
	req := &rdbpb.GetInvocationRequest{
		Name: inv,
	}
	mc.GetInvocation(req, &rdbpb.Invocation{
		Name:  inv,
		Realm: realm,
	})
}

// BatchGetTestVariants mocks the BatchGetTestVariants RPC.
func (mc *MockedClient) BatchGetTestVariants(req *rdbpb.BatchGetTestVariantsRequest, res *rdbpb.BatchGetTestVariantsResponse) {
	mc.Client.EXPECT().BatchGetTestVariants(gomock.Any(), proto.MatcherEqual(req),
		gomock.Any()).Return(res, nil)
}
