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
)

// MockedClient is a mocked Buildbucket client for testing.
// It wraps a bbpb.MockBuildsClient and a context with the mocked client.
type MockedClient struct {
	Client *bbpb.MockBuildsClient
	Ctx    context.Context
}

// NewMockedClient creates a MockedClient for testing.
func NewMockedClient(ctx context.Context, ctl *gomock.Controller) *MockedClient {
	mockClient := bbpb.NewMockBuildsClient(ctl)
	return &MockedClient{
		Client: mockClient,
		Ctx:    context.WithValue(ctx, &mockedBBClientKey, mockClient),
	}
}
