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

package resultdb

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockClientFactory struct {
	expectedRequest proto.Message

	inv         *rdbpb.Invocation
	err         error
	updateToken string
}

// MakeClient implements RecorderClientFactory.
func (mcf *mockClientFactory) MakeClient(ctx context.Context, host string, project string) (*RecorderClient, error) {
	return &RecorderClient{
		client: &mockedRecorderClient{
			expectedRequest: mcf.expectedRequest,
			inv:             mcf.inv,
			err:             mcf.err,
			updateToken:     mcf.updateToken,
		},
		swarmingProject: "example",
	}, nil
}

// NewMockRecorderClientFactory returns RecorderClientFactory to create
// mocked recorder client for tests.
func NewMockRecorderClientFactory(expectedRequest proto.Message, inv *rdbpb.Invocation, err error, updateToken string) *mockClientFactory {
	return &mockClientFactory{
		expectedRequest: expectedRequest,
		inv:             inv,
		err:             err,
		updateToken:     updateToken,
	}
}

type mockedRecorderClient struct {
	rdbpb.RecorderClient // implement all remaning methods by nil-panicing

	expectedRequest proto.Message

	inv         *rdbpb.Invocation
	err         error
	updateToken string
}

func (c *mockedRecorderClient) CreateInvocation(ctx context.Context, req *rdbpb.CreateInvocationRequest, opts ...grpc.CallOption) (*rdbpb.Invocation, error) {
	req.RequestId = "" // RequestId is a uuid, do not compare with it.
	if c.expectedRequest != nil && !proto.Equal(c.expectedRequest, req) {
		return nil, status.Errorf(codes.InvalidArgument, "unexpected request")
	}
	if c.updateToken != "" {
		h, _ := opts[0].(grpc.HeaderCallOption)
		h.HeaderAddr.Set(rdbpb.UpdateTokenMetadataKey, c.updateToken)
	}
	return c.inv, c.err
}

func (c *mockedRecorderClient) FinalizeInvocation(ctx context.Context, req *rdbpb.FinalizeInvocationRequest, opts ...grpc.CallOption) (*rdbpb.Invocation, error) {
	if c.expectedRequest != nil && !proto.Equal(c.expectedRequest, req) {
		return nil, status.Errorf(codes.InvalidArgument, "unexpected request")
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	token := md.Get(rdbpb.UpdateTokenMetadataKey)
	if len(token) != 1 || (c.updateToken != "" && token[0] != c.updateToken) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid update token")
	}
	return c.inv, c.err
}
