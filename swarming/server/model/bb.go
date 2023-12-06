// Copyright 2023 The LUCI Authors.
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

package model

import (
	"context"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/service/datastore"
)

// BuildTask stores the Buildbucket related fields.
//
// Present only if the corresponding TaskRequest's HasBuildTask is true.
type BuildTask struct {
	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Key identifies the task.
	//
	// See BuildTaskKey.
	Key *datastore.Key `gae:"$key"`

	// BuildID is the Buildbucket build ID associated with the Swarming task.
	BuildID string `gae:"build_id,noindex"`

	// BuildbucketHost is the Buildbucket host that has the build.
	BuildbucketHost string `gae:"buildbucket_host,noindex"`

	// UpdateID is a monotonically increasing integer that is used to compare when
	// updates (state changes) have occurred. A timestamp measured in ms is used.
	UpdateID int64 `gae:"update_id,noindex"`

	// LatestTaskStatus is a the latest build status sent to Buildbucket.
	//
	// It is converted from TaskRunResult.State.
	LatestTaskStatus bbpb.Status `gae:"latest_task_status,noindex"`

	// PubSubTopic is the pubsub topic name that will be used to send
	// UpdateBuildTask messages to Buildbucket.
	PubSubTopic string `gae:"pubsub_topic,noindex"`

	// BotDimensions are bot dimensions at the moment the bot claimed the task.
	//
	// The same as in TaskRunResult.BotDimensions. Stored here to avoid extra
	// datastore fetch when sending updates to Buildbucket.
	BotDimensions BotDimensions `gae:"bot_dimensions"`

	// LegacyTaskStatus is no longer used.
	LegacyTaskStatus LegacyProperty `gae:"task_status"`
}

// BuildTaskKey construct a BuildTask key given a task request key.
func BuildTaskKey(ctx context.Context, taskReq *datastore.Key) *datastore.Key {
	return datastore.NewKey(ctx, "BuildTask", "", 1, taskReq)
}
