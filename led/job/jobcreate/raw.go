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

package jobcreate

import (
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	"go.chromium.org/luci/led/job"
)

func jobDefinitionFromSwarming(sw *job.Swarming, r *swarmingpb.NewTaskRequest) {
	// we ignore r.Properties; TaskSlices are the only thing generated from modern
	// swarming tasks.
	// TODO(b/296244642): Remove redundant job.Swarming.Task <-> swarming.NewTaskRequest conversion
	sw.Task = &swarmingpb.NewTaskRequest{}
	t := sw.Task

	t.TaskSlices = make([]*swarmingpb.TaskSlice, len(r.TaskSlices))
	for i, inslice := range r.TaskSlices {
		outslice := &swarmingpb.TaskSlice{
			ExpirationSecs:  inslice.ExpirationSecs,
			Properties:      inslice.Properties,
			WaitForCapacity: inslice.WaitForCapacity,
		}
		t.TaskSlices[i] = outslice
	}

	t.Priority = int32(r.Priority) + 10
	t.ServiceAccount = r.ServiceAccount
	t.Realm = r.Realm
	// CreateTime is unused for new task requests
	// Name is overwritten
	// Tags should not be explicitly provided by the user for the new task
	t.User = r.User
	// TaskId is unpopulated for new task requests
	// ParentTaskId is unpopulated for new task requests
	// ParentRunId is unpopulated for new task requests
	// PubsubNotification is intentionally not propagated.
	t.BotPingToleranceSecs = r.BotPingToleranceSecs
}
