// Copyright 2015 The LUCI Authors.
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

package swarmingimpl

import (
	"strings"

	"go.chromium.org/luci/common/errors"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

// TODO(vadimsh): Convert into flag.Value.
func stateMap(value string) (state swarmingpb.StateQuery, err error) {
	value = strings.ToLower(value)
	switch value {
	case "pending":
		state = swarmingpb.StateQuery_QUERY_PENDING
	case "running":
		state = swarmingpb.StateQuery_QUERY_RUNNING
	case "pending_running":
		state = swarmingpb.StateQuery_QUERY_PENDING_RUNNING
	case "completed":
		state = swarmingpb.StateQuery_QUERY_COMPLETED
	case "completed_success":
		state = swarmingpb.StateQuery_QUERY_COMPLETED_SUCCESS
	case "completed_failure":
		state = swarmingpb.StateQuery_QUERY_COMPLETED_FAILURE
	case "expired":
		state = swarmingpb.StateQuery_QUERY_EXPIRED
	case "timed_out":
		state = swarmingpb.StateQuery_QUERY_TIMED_OUT
	case "bot_died":
		state = swarmingpb.StateQuery_QUERY_BOT_DIED
	case "canceled":
		state = swarmingpb.StateQuery_QUERY_CANCELED
	case "":
	case "all":
		state = swarmingpb.StateQuery_QUERY_ALL
	case "deduped":
		state = swarmingpb.StateQuery_QUERY_DEDUPED
	case "killed":
		state = swarmingpb.StateQuery_QUERY_KILLED
	case "no_resource":
		state = swarmingpb.StateQuery_QUERY_NO_RESOURCE
	case "client_error":
		state = swarmingpb.StateQuery_QUERY_CLIENT_ERROR
	default:
		err = errors.Fmt("Invalid state %s", value)
	}
	return state, err
}

// TaskIsAlive is true if the task is pending or running.
func TaskIsAlive(t swarmingpb.TaskState) bool {
	return t == swarmingpb.TaskState_PENDING || t == swarmingpb.TaskState_RUNNING
}

// TaskIsCompleted is true if the task has completed (successfully or not).
func TaskIsCompleted(t swarmingpb.TaskState) bool {
	return t == swarmingpb.TaskState_COMPLETED
}
