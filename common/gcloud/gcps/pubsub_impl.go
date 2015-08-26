// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gcps

import (
	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"
	"google.golang.org/cloud/pubsub"
)

// pubSubImpl is an implementation of PubSub that communicates directly to the
// Google Cloud Pub/Sub system.
//
// Currently, all errors are regarded as transient.
type pubSubImpl struct {
	ctx context.Context
}

// New instantiates a new PubSub instance configured to use the Google Cloud
// Pub/Sub system.
//
// The supplied context must be properly authenticated to interface with the
// named Pub/Sub system.
func New(ctx context.Context) PubSub {
	return &pubSubImpl{
		ctx: ctx,
	}
}

func (p *pubSubImpl) TopicExists(t Topic) (bool, error) {
	exists, err := pubsub.TopicExists(p.ctx, string(t))
	return exists, (err)
}

func (p *pubSubImpl) SubExists(s Subscription) (bool, error) {
	exists, err := pubsub.SubExists(p.ctx, string(s))
	return exists, errors.WrapTransient(err)
}

func (p *pubSubImpl) Publish(t Topic, msgs ...*pubsub.Message) ([]string, error) {
	ids, err := pubsub.Publish(p.ctx, string(t), msgs...)
	return ids, errors.WrapTransient(err)
}

func (p *pubSubImpl) Pull(s Subscription, n int) ([]*pubsub.Message, error) {
	msgs, err := pubsub.Pull(p.ctx, string(s), n)
	return msgs, errors.WrapTransient(err)
}

func (p *pubSubImpl) Ack(s Subscription, ackIDs ...string) error {
	return errors.WrapTransient(pubsub.Ack(p.ctx, string(s), ackIDs...))
}
