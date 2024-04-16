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
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/swarming/server/metrics"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/notifications/taskspb"
	"go.chromium.org/luci/swarming/server/util/taskbackendutil"
)

type PubSubNotifier struct {
	// A shared pubsub client
	client *pubsub.Client

	// A lock for the topics cache.
	lock   sync.RWMutex
	topics map[string]*pubsub.Topic

	// True if "Stop()" was called.
	stopped bool

	cloudProject string
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
	return &PubSubNotifier{
		client:       psClient,
		cloudProject: cloudProj,
	}, nil
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
		Prototype: (*taskspb.PubSubNotifyTask)(nil),
		Queue:     "pubsub-go", // to replace "pubsub" taskqueue in Py.
		Handler: func(ctx context.Context, payload proto.Message) error {
			return ps.handlePubSubNotifyTask(ctx, payload.(*taskspb.PubSubNotifyTask))
		},
	})
	disp.RegisterTaskClass(tq.TaskClass{
		ID:        "buildbucket-notify-go",
		Kind:      tq.NonTransactional,
		Prototype: (*taskspb.BuildbucketNotifyTask)(nil),
		Queue:     "buildbucket-notify-go", // to replace "buildbucket-notify" taskqueue in Py.
		Handler: func(ctx context.Context, payload proto.Message) error {
			return ps.handleBBNotifyTask(ctx, payload.(*taskspb.BuildbucketNotifyTask))
		},
	})
}

// handlePubSubNotifyTask sends the notification about a task completion.
// For fatal errors, a tq.Fatal tag will be applied.
// For retryable errors, a transient.Tag will be applied.
func (ps *PubSubNotifier) handlePubSubNotifyTask(ctx context.Context, t *taskspb.PubSubNotifyTask) error {
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

	// Publish to TsMon pubsub latency metric.
	now := clock.Now(ctx).UTC().UnixMilli()
	startTimeMilli := t.StartTime.AsTime().UnixMilli()
	latency := now - startTimeMilli
	if latency < 0 {
		logging.Warningf(ctx, "ts_mon_metric pubsub latency %dms (%d - %d) is negative. Setting latency to 0", latency, now, startTimeMilli)
		latency = 0
	}
	pool := strpair.ParseMap(t.Tags).Get("pool")
	httpCode := grpcutil.CodeStatus(status.Code(err))
	status := t.State.String()
	logging.Debugf(ctx, "Updating TsMon pubsub metric with latency: %dms, httpCode: %d, status: %s, pool: %s", latency, httpCode, status, pool)
	metrics.TaskStatusChangePubsubLatency.Add(ctx, float64(latency), pool, status, httpCode)

	return errors.Annotate(err, "failed to publish the msg to %s", t.Topic).Tag(transient.Tag).Err()
}

// handleBBNotifyTask sends a pubsub update to Buildbucket.
func (ps *PubSubNotifier) handleBBNotifyTask(ctx context.Context, t *taskspb.BuildbucketNotifyTask) error {
	taskReq, err := model.TaskIDToRequestKey(ctx, t.GetTaskId())
	if err != nil {
		return tq.Fatal.Apply(err)
	}

	buildTask := &model.BuildTask{Key: model.BuildTaskKey(ctx, taskReq)}
	switch err := datastore.Get(ctx, buildTask); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return errors.Annotate(err, "cannot find BuildTask").Tag(tq.Fatal).Err()
	case err != nil:
		return errors.Annotate(err, "failed to fetch BuildTask").Tag(transient.Tag).Err()
	}

	// Shouldn't make the update.
	if buildTask.UpdateID >= t.UpdateId || buildTask.LatestTaskStatus == t.State {
		return nil
	}

	resultSummary := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, taskReq)}
	if err := datastore.Get(ctx, resultSummary); err != nil {
		return errors.Annotate(err, "failed to fetch TaskResultSummary").Tag(transient.Tag).Err()
	}

	// construct bbpb.Task
	botDims := buildTask.BotDimensions
	if len(botDims) == 0 {
		botDims = resultSummary.BotDimensions
	}
	bbTask := &bbpb.Task{
		Id: &bbpb.TaskID{
			Id:     model.RequestKeyToTaskID(taskReq, model.AsRequest),
			Target: fmt.Sprintf("swarming://%s", ps.cloudProject),
		},
		UpdateId: t.UpdateId,
		Details: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"bot_dimensions": {
					Kind: &structpb.Value_StructValue{
						StructValue: botDims.ToStructPB(),
					},
				},
			},
		},
	}
	taskbackendutil.SetBBStatus(t.State, resultSummary.Failure, bbTask)

	// send the update msg via PubSub
	cloudProj, topicID, err := parsePubSubTopicName(buildTask.PubSubTopic)
	if err != nil {
		return tq.Fatal.Apply(err)
	}
	topic, err := ps.getTopic(cloudProj, topicID)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	bytes, err := proto.Marshal(&bbpb.BuildTaskUpdate{
		BuildId: buildTask.BuildID,
		Task:    bbTask,
	})
	if err != nil {
		return errors.Annotate(err, "failed to marshal BuildTaskUpdate").Tag(tq.Fatal).Err()
	}
	result := topic.Publish(ctx, &pubsub.Message{
		Data: bytes,
	})
	if _, err = result.Get(ctx); err != nil {
		return errors.Annotate(err, "failed to send buildbucket task update").Tag(transient.Tag).Err()
	}

	// update the BuildTask entity in Datastore
	buildTask.LatestTaskStatus = t.State
	buildTask.UpdateID = t.UpdateId
	if len(buildTask.BotDimensions) == 0 {
		buildTask.BotDimensions = botDims
	}
	return errors.Annotate(datastore.Put(ctx, buildTask), "failed to update BuildTask").Tag(transient.Tag).Err()
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
