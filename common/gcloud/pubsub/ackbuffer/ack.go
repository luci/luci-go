// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ackbuffer

import (
	"github.com/luci/luci-go/common/gcloud/pubsub"
	"golang.org/x/net/context"
)

// Acknowledger sends ACKs to a Pub/Sub interface.
type Acknowledger interface {
	// Ack acknowledges one or more Pub/Sub message ACK IDs.
	Ack(ctx context.Context, ackIDs ...string) error

	// AckBatchSize returns the maximum number of ACKs that can be sent at a time.
	AckBatchSize() int
}

type pubsubACK struct {
	ps    pubsub.Connection
	sub   pubsub.Subscription
	batch int
}

// NewACK creates a Acknowledger instance from a pubsub.Connection
// implementation.
//
// If batch is <= 0, the maximum ACK batch size will be used.
func NewACK(ps pubsub.Connection, s pubsub.Subscription, batch int) Acknowledger {
	if batch <= 0 {
		batch = pubsub.MaxMessageAckPerRequest
	}

	return &pubsubACK{
		ps:    ps,
		sub:   s,
		batch: batch,
	}
}

func (a *pubsubACK) Ack(c context.Context, ackIDs ...string) error {
	return a.ps.Ack(c, a.sub, ackIDs...)
}

func (a *pubsubACK) AckBatchSize() int {
	return a.batch
}
