// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pubsub

import (
	"google.golang.org/cloud/pubsub"
)

// Message is a Google Pub/Sub API message type. It's included here as a
// convenience so users don't have to import the other library in addition to
// this one.
type Message pubsub.Message

func localMessageToPubSub(m []*Message) (ms []*pubsub.Message) {
	if len(m) > 0 {
		ms = make([]*pubsub.Message, len(m))
		for i, msg := range m {
			ms[i] = (*pubsub.Message)(msg)
		}
	}
	return
}

func pubSubMessageToLocal(ms []*pubsub.Message) (m []*Message) {
	if len(ms) > 0 {
		m = make([]*Message, len(ms))
		for i, msg := range ms {
			m[i] = (*Message)(msg)
		}
	}
	return
}
