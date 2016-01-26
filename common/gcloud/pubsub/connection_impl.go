// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pubsub

import (
	"net/http"

	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"
	"google.golang.org/cloud"
	"google.golang.org/cloud/pubsub"
)

// connectionImpl is an implementation of Connection that communicates directly to
// the Google Cloud Pub/Sub system.
//
// Currently, all errors are regarded as transient.
type connectionImpl struct {
	client *http.Client
}

// NewConnection instantiates a new Connection instance configured to use the
// Google Cloud Pub/Sub system.
//
// The supplied Client must be properly authenticated to interface with the
// named Pub/Sub system.
func NewConnection(c *http.Client) Connection {
	return &connectionImpl{
		client: c,
	}
}

func (p *connectionImpl) TopicExists(c context.Context, t Topic) (bool, error) {
	proj, err := t.ProjectErr()
	if err != nil {
		return false, err
	}

	exists, err := pubsub.TopicExists(p.with(c, proj), string(t))
	return exists, (err)
}

func (p *connectionImpl) SubExists(c context.Context, s Subscription) (bool, error) {
	proj, err := s.ProjectErr()
	if err != nil {
		return false, err
	}

	exists, err := pubsub.SubExists(p.with(c, proj), string(s))
	return exists, errors.WrapTransient(err)
}

func (p *connectionImpl) Publish(c context.Context, t Topic, msgs ...*Message) ([]string, error) {
	proj, err := t.ProjectErr()
	if err != nil {
		return nil, err
	}

	ids, err := pubsub.Publish(p.with(c, proj), string(t), localMessageToPubSub(msgs)...)
	return ids, errors.WrapTransient(err)
}

func (p *connectionImpl) Pull(c context.Context, s Subscription, n int) ([]*Message, error) {
	proj, err := s.ProjectErr()
	if err != nil {
		return nil, err
	}

	msgs, err := pubsub.Pull(p.with(c, proj), string(s), n)
	return pubSubMessageToLocal(msgs), errors.WrapTransient(err)
}

func (p *connectionImpl) Ack(c context.Context, s Subscription, ackIDs ...string) error {
	proj, err := s.ProjectErr()
	if err != nil {
		return err
	}

	return errors.WrapTransient(pubsub.Ack(p.with(c, proj), string(s), ackIDs...))
}

func (p *connectionImpl) with(c context.Context, project string) context.Context {
	return cloud.WithContext(c, project, p.client)
}
