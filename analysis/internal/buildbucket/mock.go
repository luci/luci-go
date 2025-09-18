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

package buildbucket

import (
	"context"

	"github.com/golang/mock/gomock"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbgrpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/common/proto"
)

// MockedClient is a mocked Buildbucket client for testing.
// It wraps a bbpb.MockBuildsClient and a context with the mocked client.
type MockedClient struct {
	Client *bbgrpcpb.MockBuildsClient
	Ctx    context.Context
}

// NewMockedClient creates a MockedClient for testing.
func NewMockedClient(ctx context.Context, ctl *gomock.Controller) *MockedClient {
	mockClient := bbgrpcpb.NewMockBuildsClient(ctl)
	return &MockedClient{
		Client: mockClient,
		Ctx:    useBuildsClientForTesting(ctx, mockClient),
	}
}

// GetBuild Mocks the GetBuild RPC.
func (mc *MockedClient) GetBuild(req *bbpb.GetBuildRequest, res *bbpb.Build) {
	mc.Client.EXPECT().GetBuild(gomock.Any(), proto.MatcherEqual(req), gomock.Any()).Return(res, nil)
}
