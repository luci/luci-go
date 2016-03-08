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

// ClientFactory is a function that creates an HTTP client.
type ClientFactory func(ctx context.Context) (*http.Client, error)

type pubSubMonitor struct {
	clientFactory ClientFactory
	topic         pubsub.Topic
}

// NewPubsubMonitor returns a Monitor that sends metrics to the Cloud Pub/Sub
// API.
//
// The provided client should do implement sufficient authentication to send
// Cloud Pub/Sub requests.
func NewPubsubMonitor(c ClientFactory, project string, topic string) (Monitor, error) {
	return &pubSubMonitor{
		clientFactory: c,
		topic:         pubsub.NewTopic(project, topic),
	}, nil
}

func (m *pubSubMonitor) ChunkSize() int {
	return 1000
}

func (m *pubSubMonitor) Send(ctx context.Context, cells []types.Cell) error {
	collection := SerializeCells(cells)

	data, err := proto.Marshal(collection)
	if err != nil {
		return err
	}

	client, err := m.clientFactory(ctx)
	if err != nil {
		return err
	}

	ps := pubsub.NewConnection(client)
	_, err = ps.Publish(ctx, m.topic, &pubsub.Message{Data: data})
	return err
}
