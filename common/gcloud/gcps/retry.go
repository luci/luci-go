// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gcps

import (
	"github.com/luci/luci-go/common/retry"
	"golang.org/x/net/context"
	"google.golang.org/cloud/pubsub"
)

// Retry wraps a Connection and retries on transient errors.
type Retry struct {
	// Connection is the base Connection to wrap and retry.
	Connection

	// Factory is the retry.Factory to use. If nil, retry.Default will be used.
	Factory retry.Factory

	// Callback, if not nil, will be called when an error is encountered.
	Callback retry.Callback
}

// TopicExists implements the Connection interface.
func (r *Retry) TopicExists(c context.Context, t Topic) (exists bool, err error) {
	err = retry.Retry(c, r.retryFactory(), func() (err error) {
		exists, err = r.Connection.TopicExists(c, t)
		return
	}, r.Callback)
	return
}

// SubExists implements the Connection interface.
func (r *Retry) SubExists(c context.Context, s Subscription) (exists bool, err error) {
	err = retry.Retry(c, r.retryFactory(), func() (err error) {
		exists, err = r.Connection.SubExists(c, s)
		return
	}, r.Callback)
	return
}

// Publish implements the Connection interface.
func (r *Retry) Publish(c context.Context, t Topic, msgs ...*pubsub.Message) (ids []string, err error) {
	err = retry.Retry(c, r.retryFactory(), func() (err error) {
		ids, err = r.Connection.Publish(c, t, msgs...)
		return
	}, r.Callback)
	return
}

// Pull implements the Connection interface.
func (r *Retry) Pull(c context.Context, s Subscription, batch int) (msgs []*pubsub.Message, err error) {
	err = retry.Retry(c, r.retryFactory(), func() (err error) {
		msgs, err = r.Connection.Pull(c, s, batch)
		return
	}, r.Callback)
	return
}

// Ack implements the Connection interface.
func (r *Retry) Ack(c context.Context, s Subscription, ackIDs ...string) (err error) {
	return retry.Retry(c, r.retryFactory(), func() error {
		return r.Connection.Ack(c, s, ackIDs...)
	}, r.Callback)
}

func (r *Retry) retryFactory() retry.Factory {
	var f retry.Factory
	if r.Factory != nil {
		f = r.Factory
	} else {
		f = retry.Default
	}
	return retry.TransientOnly(f)
}
