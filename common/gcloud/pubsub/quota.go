// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pubsub

import (
	"time"
)

// Cloud PubSub quota is documented here:
// https://cloud.google.com/pubsub/quotas
const (
	// MaxPublishSize is the maximum size, in bytes, of the published message
	// (10 MB).
	//
	// See: https://cloud.google.com/pubsub/publisher
	MaxPublishSize = 10 * 1024 * 1024

	// MaxPublishBatchSize is the maximum PubSub batch size.
	MaxPublishBatchSize = 1000

	// MaxProjectMessagesPerSecond is the maximum number of requests per second,
	// across the entire project.
	MaxProjectMessagesPerSecond = 10000

	// MaxSubscriptionPullSize is the maximum number of subscription records that
	// can be pulled at a time.
	MaxSubscriptionPullSize = 100

	// MaxMessageAckPerRequest is the maximum number of messages one can
	// acknowledge in a single "acknowledge" call.
	//
	// NOTE: This is not verified, and is inspired by "MaxSubscriptionPullSize".
	MaxMessageAckPerRequest = 100

	// DefaultMaxAckDelay is the default maximum ACK delay.
	DefaultMaxAckDelay = (60 * time.Second)
)
