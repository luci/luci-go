// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package monitor

import (
	"net/http"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/gcloud/pubsub"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"
)

type pubSubMonitor struct {
	context.Context
	ps    pubsub.Connection
	topic pubsub.Topic
}

// NewPubsubMonitor returns a Monitor that sends metrics to the Cloud Pub/Sub
// API.
//
// The provided client should do implement sufficient authentication to send
// Cloud Pub/Sub requests.
func NewPubsubMonitor(ctx context.Context, c *http.Client, project string, topic string) (Monitor, error) {
	return &pubSubMonitor{
		Context: ctx,
		ps:      pubsub.NewConnection(c),
		topic:   pubsub.NewTopic(project, topic),
	}, nil
}

func (m *pubSubMonitor) ChunkSize() int {
	return 1000
}

func (m *pubSubMonitor) Send(cells []types.Cell, defaultTarget types.Target) error {
	collection := serializeCells(cells, defaultTarget)

	data, err := proto.Marshal(collection)
	if err != nil {
		return err
	}

	_, err = m.ps.Publish(m, m.topic, &pubsub.Message{Data: data})
	return err
}
