// Copyright 2024 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package notifications contains the logic about send Swarming notifications.
package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"cloud.google.com/go/pubsub"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/swarming/server/notifications/taskspb"
)

type PubSubNotifier struct {
	// A shared pubsub client
	client *pubsub.Client

	// A lock for the topics cache.
	lock   sync.RWMutex
	topics map[string]*pubsub.Topic

	// True if "Stop()" was called.
	stopped bool
}

// PubSubNotification defines the message schema for Swarming to send the task
// completion PubSub message.
// The message attributes may contain "auth_token" if users provided it.
type PubSubNotification struct {
	TaskID   string `json:"task_id"`
	Userdata string `json:"userdata"`
}

// NewPubSubNotifier initializes a PubSubNotifier.
func NewPubSubNotifier(ctx context.Context, cloudProj string) (*PubSubNotifier, error) {
	creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get AsSelf credentails").Err()
	}
	psClient, err := pubsub.NewClient(
		ctx, cloudProj,
		option.WithGRPCDialOption(grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{})),
		option.WithGRPCDialOption(grpc.WithStatsHandler(otelgrpc.NewClientHandler())),
		option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)),
	)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create a pubsub client for %s", cloudProj).Err()
	}
	return &PubSubNotifier{client: psClient}, nil
}

// Stop properly stops the PubSubNotifier.
func (ps *PubSubNotifier) Stop() {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if ps.stopped {
		return
	}
	for _, topic := range ps.topics {
		topic.Stop()
	}
	ps.topics = nil
	_ = ps.client.Close()
	ps.stopped = true
}

// RegisterTQTasks registers task queue handlers.
//
// Tasks are actually submitted from the Python side.
func (ps *PubSubNotifier) RegisterTQTasks(disp *tq.Dispatcher) {
	disp.RegisterTaskClass(tq.TaskClass{
		ID:        "pubsub-go",
		Kind:      tq.NonTransactional,
		Prototype: (*taskspb.PubSubNofityTask)(nil),
		Queue:     "pubsub-go", // to replace "pubsub" taskqueue in Py.
		Handler: func(ctx context.Context, payload proto.Message) error {
			return ps.handlePubSubNotifyTask(ctx, payload.(*taskspb.PubSubNofityTask))
		},
	})
}

// handlePubSubNotifyTask sends the notification about a task completion.
// For fatal errors, a tq.Fatal tag will be applied.
// For retryable errors, a transient.Tag will be applied.
func (ps *PubSubNotifier) handlePubSubNotifyTask(ctx context.Context, t *taskspb.PubSubNofityTask) error {
	cloudProj, topicID, err := parsePubSubTopicName(t.GetTopic())
	if err != nil {
		return tq.Fatal.Apply(err)
	}
	topic, err := ps.getTopic(cloudProj, topicID)
	if err != nil {
		return transient.Tag.Apply(err)
	}

	data, err := json.MarshalIndent(&PubSubNotification{
		TaskID:   t.TaskId,
		Userdata: t.Userdata,
	}, "", "  ")
	if err != nil {
		return errors.Annotate(err, "cannot compose the pubsub msg").Tag(tq.Fatal).Err()
	}
	psMsg := &pubsub.Message{
		Data: data,
	}
	if t.AuthToken != "" {
		psMsg.Attributes = map[string]string{"auth_token": t.AuthToken}
	}
	result := topic.Publish(ctx, psMsg)
	_, err = result.Get(ctx)
	// TODO(b/325342884): Add a ts_mon_metric to record the pubsub latency.
	return errors.Annotate(err, "failed to publish the msg to %s", t.Topic).Tag(transient.Tag).Err()
}

// getTopic returns the reference for a topic. If the topic is not in ps.topics
// it will create a new one and store into it
func (ps *PubSubNotifier) getTopic(cloudProj, topicID string) (*pubsub.Topic, error) {
	topicName := fmt.Sprintf("projects/%s/topics/%s", cloudProj, topicID)

	ps.lock.RLock()
	topic := ps.topics[topicName]
	ps.lock.RUnlock()

	if topic != nil {
		return topic, nil
	}

	ps.lock.Lock()
	defer ps.lock.Unlock()

	if topic, ok := ps.topics[topicName]; ok {
		return topic, nil
	}
	if ps.stopped {
		return nil, errors.New("cannot create a topic; the PubSubNotifier has already stopped")
	}
	if ps.topics == nil {
		ps.topics = make(map[string]*pubsub.Topic)
	}
	ps.topics[topicName] = ps.client.TopicInProject(topicID, cloudProj)

	return ps.topics[topicName], nil
}
