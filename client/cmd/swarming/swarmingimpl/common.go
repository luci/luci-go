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
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

// TODO(vadimsh): Convert into flag.Value.
func stateMap(value string) (state swarmingv2.StateQuery, err error) {
	value = strings.ToLower(value)
	switch value {
	case "pending":
		state = swarmingv2.StateQuery_QUERY_PENDING
	case "running":
		state = swarmingv2.StateQuery_QUERY_RUNNING
	case "pending_running":
		state = swarmingv2.StateQuery_QUERY_PENDING_RUNNING
	case "completed":
		state = swarmingv2.StateQuery_QUERY_COMPLETED
	case "completed_success":
		state = swarmingv2.StateQuery_QUERY_COMPLETED_SUCCESS
	case "completed_failure":
		state = swarmingv2.StateQuery_QUERY_COMPLETED_FAILURE
	case "expired":
		state = swarmingv2.StateQuery_QUERY_EXPIRED
	case "timed_out":
		state = swarmingv2.StateQuery_QUERY_TIMED_OUT
	case "bot_died":
		state = swarmingv2.StateQuery_QUERY_BOT_DIED
	case "canceled":
		state = swarmingv2.StateQuery_QUERY_CANCELED
	case "":
	case "all":
		state = swarmingv2.StateQuery_QUERY_ALL
	case "deduped":
		state = swarmingv2.StateQuery_QUERY_DEDUPED
	case "killed":
		state = swarmingv2.StateQuery_QUERY_KILLED
	case "no_resource":
		state = swarmingv2.StateQuery_QUERY_NO_RESOURCE
	case "client_error":
		state = swarmingv2.StateQuery_QUERY_CLIENT_ERROR
	default:
		err = errors.Reason("Invalid state %s", value).Err()
	}
	return state, err
}

// TaskIsAlive is true if the task is pending or running.
func TaskIsAlive(t swarmingv2.TaskState) bool {
	return t == swarmingv2.TaskState_PENDING || t == swarmingv2.TaskState_RUNNING
}

// TaskIsCompleted is true if the task has completed (successfully or not).
func TaskIsCompleted(t swarmingv2.TaskState) bool {
	return t == swarmingv2.TaskState_COMPLETED
}
