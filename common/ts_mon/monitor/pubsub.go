// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package monitor

import (
	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/gcloud/gcps"
	"github.com/luci/luci-go/common/ts_mon/target"
	"github.com/luci/luci-go/common/ts_mon/types"
	"google.golang.org/cloud"
	"google.golang.org/cloud/pubsub"
)

type pubSubMonitor struct {
	ps    gcps.PubSub
	topic gcps.Topic
}

// NewPubsubMonitor returns a Monitor that sends metrics to the Cloud Pub/Sub
// API.
func NewPubsubMonitor(credentialPath string, project string, topic string) (Monitor, error) {
	authOpts := auth.Options{
		Scopes:                 gcps.PublisherScopes,
		ServiceAccountJSONPath: credentialPath,
	}

	httpClient, err := auth.NewAuthenticator(auth.SilentLogin, authOpts).Client()
	if err != nil {
		return nil, err
	}

	return &pubSubMonitor{
		ps:    gcps.New(cloud.NewContext(project, httpClient)),
		topic: gcps.Topic(topic),
	}, nil
}

func (m *pubSubMonitor) ChunkSize() int {
	return 1000
}

func (m *pubSubMonitor) Send(cells []types.Cell, t target.Target) error {
	collection := serializeCells(cells, t)

	data, err := proto.Marshal(collection)
	if err != nil {
		return err
	}

	_, err = m.ps.Publish(m.topic, &pubsub.Message{Data: data})
	return err
}
