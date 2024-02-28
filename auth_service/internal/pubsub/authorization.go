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

// Package pubsub contains Pubsub-related functionality, including:
// - authorization of accounts to subscribe; and
// - pushing of Pubsub notifications when the AuthDB changes.
package pubsub

import (
	"context"
	"strings"

	"go.chromium.org/luci/common/errors"
	gcps "go.chromium.org/luci/common/gcloud/pubsub"
	"go.chromium.org/luci/gae/service/info"
)

const (
	// The topic name for AuthDB changes.
	AuthDBChangeTopicName = "auth-db-changed"

	// The IAM role required for PubSub subscribing.
	subscriberRole = "roles/pubsub.subscriber"
)

// mockedPubsubClientKey is the context key to indicate using a mocked
// Pubsub client in tests.
var mockedPubsubClientKey = "mock Pubsub client key for testing only"

// IsAuthorizedSubscriber returns whether the account is authorized to
// subscribe to Pubsub notifications of AuthDB changes.
func IsAuthorizedSubscriber(ctx context.Context, email string) (authorized bool, retErr error) {
	client, err := newClient(ctx)
	if err != nil {
		return false, errors.Annotate(err, "error creating Pubsub client").Err()
	}
	defer func() {
		err := client.Close()
		if retErr == nil {
			retErr = err
		}
	}()

	policy, err := client.GetIAMPolicy(ctx)
	if err != nil {
		return false, err
	}

	return policy.HasRole(emailToIAMIdentity(email), subscriberRole), nil
}

// AuthorizeSubscriber authorizes the account to subscribe to Pubsub
// notifications of AuthDB changes.
//
// Note this does not actually create the subscription, but rather,
// makes the account eligible to subscribe.
func AuthorizeSubscriber(ctx context.Context, email string) (retErr error) {
	client, err := newClient(ctx)
	if err != nil {
		return errors.Annotate(err, "error creating Pubsub client").Err()
	}
	defer func() {
		err := client.Close()
		if retErr == nil {
			retErr = err
		}
	}()

	policy, err := client.GetIAMPolicy(ctx)
	if err != nil {
		return errors.Annotate(err, "error getting IAM policy for PubSub topic").Err()
	}

	identity := emailToIAMIdentity(email)
	if policy.HasRole(identity, subscriberRole) {
		// Already authorized to subscribe.
		return nil
	}

	// Grant authorization to subscribe.
	policy.Add(identity, subscriberRole)
	if err := client.SetIAMPolicy(ctx, policy); err != nil {
		return errors.Annotate(err, "failed to authorize %s", identity).Err()
	}

	return nil
}

// DeauthorizeSubscriber revokes the subscribing authorization for the
// account to Pubsub notifications of AuthDB changes (i.e. making it
// ineligible to subscribe).
func DeauthorizeSubscriber(ctx context.Context, email string) (retErr error) {
	client, err := newClient(ctx)
	if err != nil {
		return errors.Annotate(err, "error creating Pubsub client").Err()
	}
	defer func() {
		err := client.Close()
		if retErr == nil {
			retErr = err
		}
	}()

	policy, err := client.GetIAMPolicy(ctx)
	if err != nil {
		return errors.Annotate(err, "error getting IAM policy for PubSub topic").Err()
	}

	identity := emailToIAMIdentity(email)
	if !policy.HasRole(identity, subscriberRole) {
		// Already unauthorized to subscribe.
		return nil
	}

	// Revoke authorization to subscribe.
	policy.Remove(identity, subscriberRole)
	if err := client.SetIAMPolicy(ctx, policy); err != nil {
		return errors.Annotate(err, "failed to deauthorize %s", identity).Err()
	}

	return nil
}

// GetProject returns the app ID from the current context.
func GetProject(ctx context.Context) string {
	return info.AppID(ctx)
}

// GetAuthDBChangeTopic returns the full topic path for changes to the
// AuthDB.
func GetAuthDBChangeTopic(ctx context.Context) string {
	return string(gcps.NewTopic(GetProject(ctx), AuthDBChangeTopicName))
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

func emailToIAMIdentity(email string) string {
	if strings.HasSuffix(email, ".gserviceaccount.com") {
		return "serviceAccount:" + email
	}
	return "user:" + email
}
