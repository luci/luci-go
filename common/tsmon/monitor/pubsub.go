// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package monitor

import (
	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/auth"
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
func NewPubsubMonitor(credentialPath string, project string, topic string) (Monitor, error) {
	authOpts := auth.Options{
		Scopes:                 pubsub.PublisherScopes,
		ServiceAccountJSONPath: credentialPath,
	}

	httpClient, err := auth.NewAuthenticator(auth.SilentLogin, authOpts).Client()
	if err != nil {
		return nil, err
	}

	return &pubSubMonitor{
		Context: context.Background(),
		ps:      pubsub.NewConnection(httpClient, project),
		topic:   pubsub.Topic(topic),
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
