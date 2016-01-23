// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ackbuffer

import (
	"github.com/luci/luci-go/common/gcloud/gcps"
	"golang.org/x/net/context"
)

// Acknowledger sends ACKs to a Pub/Sub interface.
//
// gcps.Connection naturally implements this interface.
type Acknowledger interface {
	// Ack acknowledges one or more Pub/Sub message ACK IDs.
	Ack(ctx context.Context, ackIDs ...string) error

	// AckBatchSize returns the maximum number of ACKs that can be sent at a time.
	AckBatchSize() int
}

type gcpsACK struct {
	ps    gcps.Connection
	sub   gcps.Subscription
	batch int
}

// NewACK creates a Acknowledger instance from a gcps.Connection implementation.
//
// If batch is <= 0, the maximum ACK batch size will be used.
func NewACK(ps gcps.Connection, s gcps.Subscription, batch int) Acknowledger {
	if batch <= 0 {
		batch = gcps.MaxMessageAckPerRequest
	}

	return &gcpsACK{
		ps:    ps,
		sub:   s,
		batch: batch,
	}
}

func (a *gcpsACK) Ack(c context.Context, ackIDs ...string) error {
	return a.ps.Ack(c, a.sub, ackIDs...)
}

func (a *gcpsACK) AckBatchSize() int {
	return a.batch
}
