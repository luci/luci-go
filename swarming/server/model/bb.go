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

	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
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

	// LatestTaskStatus is a the latest status sent to Buildbucket.
	//
	// It is a TaskRunResult.State, but will be converted to Buildbucket status
	// when sending out the update.
	LatestTaskStatus apipb.TaskState `gae:"latest_task_status,noindex"`

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

// ToProto returns the corresponding bbpb.Task proto.
//
// The TaskResultSummary is used to populate status details.
func (bt *BuildTask) ToProto(result *TaskResultSummary, target string) *bbpb.Task {
	botDims := bt.BotDimensions
	if len(botDims) == 0 {
		botDims = result.BotDimensions
	}

	bbTask := &bbpb.Task{
		Id: &bbpb.TaskID{
			Id:     RequestKeyToTaskID(result.TaskRequestKey(), AsRequest),
			Target: target,
		},
		UpdateId: bt.UpdateID,
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

	SetBBTaskStatus(result.State, result.Failure, bbTask)
	return bbTask
}

// SetBBTaskStatus converts a swarming task result's state to a buildbucket
// status, updates StatusDetails and SummaryMarkdown accordingly.
//
// It Modifies the given *bbpb.Task in place.
func SetBBTaskStatus(state apipb.TaskState, failure bool, bbTask *bbpb.Task) {
	bbTask.Status = bbpb.Status_STATUS_UNSPECIFIED
	bbTask.SummaryMarkdown = ""

	switch state {
	case apipb.TaskState_PENDING:
		bbTask.Status = bbpb.Status_SCHEDULED
	case apipb.TaskState_RUNNING:
		bbTask.Status = bbpb.Status_STARTED
	case apipb.TaskState_EXPIRED:
		bbTask.Status = bbpb.Status_INFRA_FAILURE
		bbTask.SummaryMarkdown = "Task expired."
		bbTask.StatusDetails = &bbpb.StatusDetails{
			ResourceExhaustion: &bbpb.StatusDetails_ResourceExhaustion{},
			Timeout:            &bbpb.StatusDetails_Timeout{},
		}
	case apipb.TaskState_TIMED_OUT:
		bbTask.Status = bbpb.Status_INFRA_FAILURE
		bbTask.SummaryMarkdown = "Task timed out."
		bbTask.StatusDetails = &bbpb.StatusDetails{
			Timeout: &bbpb.StatusDetails_Timeout{},
		}
	case apipb.TaskState_CLIENT_ERROR:
		bbTask.Status = bbpb.Status_FAILURE
		bbTask.SummaryMarkdown = "Task client error."
	case apipb.TaskState_BOT_DIED:
		bbTask.Status = bbpb.Status_INFRA_FAILURE
		bbTask.SummaryMarkdown = "Task bot died."
	case apipb.TaskState_CANCELED, apipb.TaskState_KILLED:
		bbTask.Status = bbpb.Status_CANCELED
	case apipb.TaskState_NO_RESOURCE:
		bbTask.Status = bbpb.Status_INFRA_FAILURE
		bbTask.SummaryMarkdown = "Task did not start, no resource"
		bbTask.StatusDetails = &bbpb.StatusDetails{
			ResourceExhaustion: &bbpb.StatusDetails_ResourceExhaustion{},
		}
	case apipb.TaskState_COMPLETED:
		if failure {
			bbTask.Status = bbpb.Status_FAILURE
			bbTask.SummaryMarkdown = "Task completed with failure."
		} else {
			bbTask.Status = bbpb.Status_SUCCESS
		}
	}
}
