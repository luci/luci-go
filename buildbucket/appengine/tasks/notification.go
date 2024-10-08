// Copyright 2020 The LUCI Authors.
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

package tasks

import (
	"context"
	"strconv"

	"cloud.google.com/go/pubsub"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/compression"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// notifyPubSub enqueues tasks to Python side.
func notifyPubSub(ctx context.Context, task *taskdefs.NotifyPubSub) error {
	if task.GetBuildId() == 0 {
		return errors.Reason("build_id is required").Err()
	}
	return tq.AddTask(ctx, &tq.Task{
		Payload: task,
	})
}

// NotifyPubSub enqueues tasks to notify Pub/Sub about the given build.
func NotifyPubSub(ctx context.Context, b *model.Build) error {
	// TODO(crbug.com/1406393#c5): Stop pushing into Python side `builds` topic
	// once all subscribers moved away.
	if err := notifyPubSub(ctx, &taskdefs.NotifyPubSub{
		BuildId: b.ID,
	}); err != nil {
		return errors.Annotate(err, "failed to enqueue global pubsub notification task: %d", b.ID).Err()
	}

	if err := tq.AddTask(ctx, &tq.Task{
		Payload: &taskdefs.NotifyPubSubGoProxy{
			BuildId: b.ID,
			Project: b.Proto.GetBuilder().GetProject(),
		},
	}); err != nil {
		return errors.Annotate(err, "failed to enqueue NotifyPubSubGoProxy task: %d", b.ID).Err()
	}

	if b.PubSubCallback.Topic == "" {
		return nil
	}

	if err := tq.AddTask(ctx, &tq.Task{
		Payload: &taskdefs.NotifyPubSubGo{
			BuildId:  b.ID,
			Topic:    &pb.BuildbucketCfg_Topic{Name: b.PubSubCallback.Topic},
			Callback: true,
		},
	}); err != nil {
		return errors.Annotate(err, "failed to enqueue Go callback pubsub notification task: %d", b.ID).Err()
	}
	return nil
}

// EnqueueNotifyPubSubGo dispatches NotifyPubSubGo tasks to send builds_v2
// notifications.
func EnqueueNotifyPubSubGo(ctx context.Context, buildID int64, project string) error {
	// Enqueue a task for publishing to the internal global "builds_v2" topic.
	err := tq.AddTask(ctx, &tq.Task{
		Payload: &taskdefs.NotifyPubSubGo{
			BuildId: buildID,
		},
	})
	if err != nil {
		return errors.Annotate(err, "failed to enqueue a notification task to builds_v2 topic for build %d", buildID).Err()
	}

	proj := &model.Project{
		ID: project,
	}
	if err := errors.Filter(datastore.Get(ctx, proj), datastore.ErrNoSuchEntity); err != nil {
		return errors.Annotate(err, "failed to fetch project %s for %d", project, buildID).Err()
	}
	for _, t := range proj.CommonConfig.GetBuildsNotificationTopics() {
		if t.Name == "" {
			continue
		}
		if err := tq.AddTask(ctx, &tq.Task{
			Payload: &taskdefs.NotifyPubSubGo{
				BuildId: buildID,
				Topic:   t,
			},
		}); err != nil {
			return errors.Annotate(err, "failed to enqueue notification task: %d for external topic %s ", buildID, t.Name).Err()
		}
	}
	return nil
}

// PublishBuildsV2Notification is the handler of notify-pubsub-go where it
// actually sends build notifications to the internal or external topic.
func PublishBuildsV2Notification(ctx context.Context, buildID int64, topic *pb.BuildbucketCfg_Topic, callback bool) error {
	b := &model.Build{ID: buildID}
	switch err := datastore.Get(ctx, b); {
	case err == datastore.ErrNoSuchEntity:
		logging.Warningf(ctx, "cannot find build %d", buildID)
		return nil
	case err != nil:
		return errors.Annotate(err, "error fetching build %d", buildID).Tag(transient.Tag).Err()
	}

	p, err := b.ToProto(ctx, model.NoopBuildMask, nil)
	if err != nil {
		return errors.Annotate(err, "failed to convert build to proto when in publishing builds_v2 flow").Err()
	}

	// Drop input/output properties and steps, and move them into build_large_fields.
	buildLarge := &pb.Build{
		Input: &pb.Build_Input{
			Properties: p.Input.GetProperties(),
		},
		Output: &pb.Build_Output{
			Properties: p.Output.GetProperties(),
		},
		Steps: p.Steps,
	}
	p.Steps = nil
	if p.Input != nil {
		p.Input.Properties = nil
	}
	if p.Output != nil {
		p.Output.Properties = nil
	}

	buildLargeBytes, err := proto.Marshal(buildLarge)
	if err != nil {
		return errors.Annotate(err, "failed to marshal buildLarge").Err()
	}
	var compressed []byte
	// If topic is nil or empty, it gets Compression_ZLIB.
	switch topic.GetCompression() {
	case pb.Compression_ZLIB:
		compressed, err = compression.ZlibCompress(buildLargeBytes)
	case pb.Compression_ZSTD:
		compressed = make([]byte, 0, len(buildLargeBytes)/2) // hope for at least 2x compression
		compressed = compression.ZstdCompress(buildLargeBytes, compressed)
	default:
		return tq.Fatal.Apply(errors.Reason("unsupported compression method %s", topic.GetCompression().String()).Err())
	}
	if err != nil {
		return errors.Annotate(err, "failed to compress large fields for %d", buildID).Err()
	}

	bldV2 := &pb.BuildsV2PubSub{
		Build:            p,
		BuildLargeFields: compressed,
		Compression:      topic.GetCompression(),
	}

	prj := b.Project // represent the project to make the pubsub call.
	var msg proto.Message
	msg = bldV2
	if callback {
		msg = &pb.PubSubCallBack{
			BuildPubsub: bldV2,
			UserData:    b.PubSubCallback.UserData,
		}
		prj = "" // represent the service to make the pubsub call.
	}

	switch {
	case topic.GetName() != "":
		return publishToExternalTopic(ctx, msg, generateBuildsV2Attributes(p), topic.Name, prj)
	default:
		//  publish to the internal `builds_v2` topic.
		return tq.AddTask(ctx, &tq.Task{
			Payload: bldV2,
		})
	}
}

// publishToExternalTopic publishes the given pubsub msg to the given topic
// with the identity of the luciProject account or current service account.
func publishToExternalTopic(ctx context.Context, msg proto.Message, attrs map[string]string, topicName, luciProject string) error {
	cloudProj, topicID, err := clients.ValidatePubSubTopicName(topicName)
	if err != nil {
		return tq.Fatal.Apply(err)
	}

	blob, err := (protojson.MarshalOptions{Indent: "\t"}).Marshal(msg)
	if err != nil {
		return errors.Annotate(err, "failed to marshal pubsub message").Tag(tq.Fatal).Err()
	}

	psClient, err := clients.NewPubsubClient(ctx, cloudProj, luciProject)
	defer psClient.Close()
	if err != nil {
		return transient.Tag.Apply(err)
	}

	topic := psClient.Topic(topicID)
	defer topic.Stop()
	result := topic.Publish(ctx, &pubsub.Message{
		Data:       blob,
		Attributes: attrs,
	})
	_, err = result.Get(ctx)
	return transient.Tag.Apply(err)
}

func generateBuildsV2Attributes(b *pb.Build) map[string]string {
	if b == nil {
		return map[string]string{}
	}
	return map[string]string{
		"project":      b.Builder.GetProject(),
		"bucket":       b.Builder.GetBucket(),
		"builder":      b.Builder.GetBuilder(),
		"is_completed": strconv.FormatBool(protoutil.IsEnded(b.Status)),
		"version":      "v2",
	}
}
