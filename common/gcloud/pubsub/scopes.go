// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pubsub

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
