// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package monitor

import (
	"net/http"

	"github.com/golang/protobuf/proto"
	gcps "github.com/luci/luci-go/common/gcloud/pubsub"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"
	"google.golang.org/cloud"
	"google.golang.org/cloud/pubsub"
)

type pubSubMonitor struct {
	topic *pubsub.TopicHandle
}

// NewPubsubMonitor returns a Monitor that sends metrics to the Cloud Pub/Sub
// API.
//
// The provided client should do implement sufficient authentication to send
// Cloud Pub/Sub requests.
func NewPubsubMonitor(ctx context.Context, client *http.Client, topic gcps.Topic) (Monitor, error) {
	project, name := topic.Split()

	psClient, err := pubsub.NewClient(ctx, project, cloud.WithBaseHTTP(client))
	if err != nil {
		return nil, err
	}

	return &pubSubMonitor{
		topic: psClient.Topic(name),
	}, nil
}

func (m *pubSubMonitor) ChunkSize() int {
	return gcps.MaxPublishBatchSize
}

func (m *pubSubMonitor) Send(ctx context.Context, cells []types.Cell) error {
	collection := SerializeCells(cells)

	data, err := proto.Marshal(collection)
	if err != nil {
		return err
	}

	_, err = m.topic.Publish(ctx, &pubsub.Message{Data: data})
	return err
}
