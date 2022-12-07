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

	"go.chromium.org/luci/buildbucket/appengine/internal/compression"
	"google.golang.org/protobuf/proto"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
)

func notifyPubSub(ctx context.Context, task *taskdefs.NotifyPubSub) error {
	if task.GetBuildId() == 0 {
		return errors.Reason("build_id is required").Err()
	}
	return tq.AddTask(ctx, &tq.Task{
		Payload: task,
	})
}

// NotifyPubSub enqueues tasks to notify Pub/Sub about the given build.
// TODO(crbug/1091604): Move next to Pub/Sub notification task handler.
// Currently the task handler is implemented in Python.
func NotifyPubSub(ctx context.Context, b *model.Build) error {
	if err := notifyPubSub(ctx, &taskdefs.NotifyPubSub{
		BuildId: b.ID,
	}); err != nil {
		return errors.Annotate(err, "failed to enqueue global pubsub notification task: %d", b.ID).Err()
	}
	if b.PubSubCallback.Topic == "" {
		return nil
	}

	logging.Warningf(ctx, "Build %d is using the legacy PubSubCallback field", b.ID)
	if err := notifyPubSub(ctx, &taskdefs.NotifyPubSub{
		BuildId:  b.ID,
		Callback: true,
	}); err != nil {
		return errors.Annotate(err, "failed to enqueue callback pubsub notification task: %d", b.ID).Err()
	}
	return nil
}

// PublishBuildsV2Notification is the handler of notify-pubsub-go where it
// fetches all build fields, converts and publishes to builds_v2 pubsub.
func PublishBuildsV2Notification(ctx context.Context, buildID int64) error {
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
	p.Input.Properties = nil
	p.Output.Properties = nil

	data, err := proto.Marshal(buildLarge)
	if err != nil {
		return errors.Annotate(err, "failed to marshal buildLarge").Err()
	}
	compressed, err := compression.ZlibCompress(data)
	if err != nil {
		return errors.Annotate(err, "failed to compress large fields for %d", buildID).Err()
	}
	return tq.AddTask(ctx, &tq.Task{
		Payload: &taskdefs.BuildsV2PubSub{
			Build:            p,
			BuildLargeFields: compressed,
		},
	})
}
