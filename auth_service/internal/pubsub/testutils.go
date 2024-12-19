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

package pubsub

import (
	"context"

	"cloud.google.com/go/iam"
	"github.com/golang/mock/gomock"
)

// MockedPubsubClient is a mocked Pubsub client for testing.
// It wraps a MockPubsubClient and a context with the mocked client.
type MockedPubsubClient struct {
	Client *MockPubsubClient
	Ctx    context.Context
}

// NewMockedClient returns a new MockedPubsubClient for testing.
func NewMockedClient(ctx context.Context, ctl *gomock.Controller) *MockedPubsubClient {
	mockClient := NewMockPubsubClient(ctl)
	return &MockedPubsubClient{
		Client: mockClient,
		Ctx:    context.WithValue(ctx, &mockedPubsubClientKey, mockClient),
	}
}

// StubPolicy returns a stub IAM policy, where only the given IAM identities
// have been granted the subscriber role. The stub policy can be used as
// the return value of mocks in tests.
func StubPolicy(iamIdentities ...string) *iam.Policy {
	p := &iam.Policy{}
	for _, id := range iamIdentities {
		p.Add(id, subscriberRole)
	}

	return p
}
