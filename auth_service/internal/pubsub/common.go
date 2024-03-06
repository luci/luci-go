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

// Package pubsub contains PubSub-related functionality, including:
// - authorization of accounts to subscribe; and
// - pushing of PubSub notifications when the AuthDB changes.
package pubsub

import (
	"context"

	gcps "go.chromium.org/luci/common/gcloud/pubsub"
	"go.chromium.org/luci/gae/service/info"
)

const (
	// The topic name for AuthDB changes.
	AuthDBChangeTopicName = "auth-db-changed"
)

// mockedPubsubClientKey is the context key to indicate using a mocked
// Pubsub client in tests.
var mockedPubsubClientKey = "mock Pubsub client key for testing only"

// getProject returns the app ID from the current context.
func getProject(ctx context.Context) string {
	return info.AppID(ctx)
}

// newClient creates a new Pubsub client.
func newClient(ctx context.Context) (PubsubClient, error) {
	if mockClient, ok := ctx.Value(&mockedPubsubClientKey).(*MockPubsubClient); ok {
		// Return a mock Pubsub client for tests.
		return mockClient, nil
	}

	client, err := newProdClient(ctx)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// GetAuthDBChangeTopic returns the full topic path for changes to the
// AuthDB.
func GetAuthDBChangeTopic(ctx context.Context) string {
	return string(gcps.NewTopic(getProject(ctx), AuthDBChangeTopicName))
}
