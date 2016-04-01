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
	// Pull retrieves up to the specified number of of Pub/Sub messages to
	// process.
	Pull(context.Context, int) ([]*pubsub.Message, error)
}

// PubSubSource is a Source implementation built on top of a pubsub.PubSub.
type pubSubSource struct {
	ps  pubsub.Connection
	sub pubsub.Subscription
}

// NewSource generates a new Source by wrapping a pubsub.Connection
// implementation. This Source is bound to a single subscription.
func NewSource(ps pubsub.Connection, s pubsub.Subscription) Source {
	return &pubSubSource{
		ps:  ps,
		sub: s,
	}
}

func (s *pubSubSource) Pull(c context.Context, batchSize int) (msgs []*pubsub.Message, err error) {
	return s.ps.Pull(c, s.sub, batchSize)
}
