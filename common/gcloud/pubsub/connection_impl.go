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
	proj, name, err := t.SplitErr()
	if err != nil {
		return false, err
	}

	var exists bool
	err = contextAwareCall(c, func(c context.Context) (err error) {
		exists, err = pubsub.TopicExists(p.with(c, proj), name)
		return
	})
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (p *connectionImpl) SubExists(c context.Context, s Subscription) (bool, error) {
	proj, name, err := s.SplitErr()
	if err != nil {
		return false, err
	}

	var exists bool
	err = contextAwareCall(c, func(c context.Context) (err error) {
		exists, err = pubsub.SubExists(p.with(c, proj), name)
		return
	})
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (p *connectionImpl) Publish(c context.Context, t Topic, msgs ...*Message) ([]string, error) {
	proj, name, err := t.SplitErr()
	if err != nil {
		return nil, err
	}

	// Check if our Context has finished. Currently, the pubsub library does not
	// interrupt calls on Context cancellation.
	if err := c.Err(); err != nil {
		return nil, err
	}

	var ids []string
	err = contextAwareCall(c, func(c context.Context) (err error) {
		ids, err = pubsub.Publish(p.with(c, proj), name, localMessageToPubSub(msgs)...)
		return
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (p *connectionImpl) Pull(c context.Context, s Subscription, n int) ([]*Message, error) {
	proj, name, err := s.SplitErr()
	if err != nil {
		return nil, err
	}

	var msgs []*pubsub.Message
	err = contextAwareCall(c, func(c context.Context) (err error) {
		msgs, err = pubsub.Pull(p.with(c, proj), name, n)
		return
	})
	if err != nil {
		return nil, err
	}
	return pubSubMessageToLocal(msgs), nil
}

func (p *connectionImpl) Ack(c context.Context, s Subscription, ackIDs ...string) error {
	proj, name, err := s.SplitErr()
	if err != nil {
		return err
	}

	return contextAwareCall(c, func(c context.Context) error {
		return pubsub.Ack(p.with(c, proj), name, ackIDs...)
	})
}

func (p *connectionImpl) with(c context.Context, project string) context.Context {
	return cloud.WithContext(c, project, p.client)
}

// contextAwareCall invokes the supplied function, f, and returns with either
// f's error code or the Context's finished error code, whichever happens
// first.
//
// Note that if f has side effects, they may still happen after this function
// has returned due to Context completion, since nothing can abort f's execution
// once executed. It is important to ensure that if this method returns an
// error value, it is checked immediately, and that any data that f touches is
// only consumed if this method returns nil.
func contextAwareCall(c context.Context, f func(context.Context) error) error {
	errC := make(chan error, 1)

	go func() {
		defer close(errC)
		errC <- f(c)
	}()

	select {
	case <-c.Done():
		// Return immediately. Our "f" will finish and have its error status
		// ignored.
		return c.Err()

	case err := <-errC:
		// We currently treat all Pub/Sub errors as transient.
		return errors.WrapTransient(err)
	}
}
