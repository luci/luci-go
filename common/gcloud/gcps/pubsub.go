// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gcps

import (
	"google.golang.org/cloud/pubsub"
)

var (
	// PublisherScopes is the set of OAuth2 scopes needed for a publisher to
	// publish messages.
	PublisherScopes = []string{
		pubsub.ScopePubSub,
	}

	// SubscriberScopes is the set of OAuth2 scopes needed for a subscriber to
	// pull and acknowledge messages.
	SubscriberScopes = []string{
		pubsub.ScopePubSub,
	}
)

// PubSub is an interface around Pub/Sub functionality.
//
// Any method may return an errors.TransientError to indicate that the
// encountered error was transient.
type PubSub interface {
	// TopicExists tests if a given Topic exists.
	TopicExists(Topic) (bool, error)

	// SubscriptionExists tests if a given Subscription exists.
	SubExists(Subscription) (bool, error)

	// Publish publishes a batch of Pub/Sub messages.
	Publish(Topic, ...*pubsub.Message) ([]string, error)

	// Pull pulls messages from the subscription. It returns up the requested
	// number of messages.
	Pull(Subscription, int) ([]*pubsub.Message, error)

	// Ack acknowledges one or more Pub/Sub message ACK IDs.
	Ack(Subscription, ...string) error
}
