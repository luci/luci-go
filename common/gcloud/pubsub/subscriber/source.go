// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package subscriber

import (
	"github.com/luci/luci-go/common/gcloud/pubsub"
	"golang.org/x/net/context"
)

// Source is used to pull Pub/Sub messages in batches.
type Source interface {
	// Pull retrieves a new batch of Pub/Sub messages to process.
	Pull(context.Context) ([]*pubsub.Message, error)
}

// PubSubSource is a Source implementation built on top of a pubsub.PubSub.
type pubSubSource struct {
	ps    pubsub.Connection
	sub   pubsub.Subscription
	batch int
}

// NewSource generates a new Source by wrapping a pubsub.Connection
// implementation. This Source is bound to a single subscription.
//
// If the supplied batch size is <= 0, the maximum allowed Pub/Sub batch size
// will be used.
func NewSource(ps pubsub.Connection, s pubsub.Subscription, batch int) Source {
	if batch <= 0 {
		batch = pubsub.MaxSubscriptionPullSize
	}
	return &pubSubSource{
		ps:    ps,
		sub:   s,
		batch: batch,
	}
}

func (s *pubSubSource) Pull(c context.Context) (msgs []*pubsub.Message, err error) {
	return s.ps.Pull(c, s.sub, s.batch)
}
