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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
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
