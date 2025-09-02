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
	"slices"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/proto"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

type FakeClient struct {
	// Embed the interface so we do not need to re-implement every method. This will be set to
	// nil so any unimplemented methods will lead to a nil reference panic.
	rdbpb.ResultDBClient

	// The TestMetadata accessible via the client.
	TestMetadata []*rdbpb.TestMetadataDetail
}

func (f *FakeClient) QueryTestMetadata(ctx context.Context, in *rdbpb.QueryTestMetadataRequest, opts ...grpc.CallOption) (*rdbpb.QueryTestMetadataResponse, error) {
	if in.PageToken != "" || in.PageSize != 0 {
		panic("pagination not implemented in fake")
	}
	var results []*rdbpb.TestMetadataDetail
	for _, item := range f.TestMetadata {
		if item.Project == in.Project && slices.Contains(in.Predicate.TestIds, item.TestId) {
			results = append(results, item)
		}
	}
	return &rdbpb.QueryTestMetadataResponse{
		TestMetadata: results,
	}, nil
}

// UseClientForTesting replaces the ResultDB client used by NewClient calls
// with the given client. Use for testing only.
func UseClientForTesting(ctx context.Context, client rdbpb.ResultDBClient) context.Context {
	return context.WithValue(ctx, &mockResultDBClientKey, client)
}

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
		Ctx:    UseClientForTesting(ctx, mockClient),
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
