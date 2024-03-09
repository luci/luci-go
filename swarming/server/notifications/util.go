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
	"regexp"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

var (
	// topicNameRE is the full topic name regex.
	topicNameRE = regexp.MustCompile(`^projects/(.*)/topics/(.*)$`)
)

// parsePubSubTopicName parses the full topic name.
func parsePubSubTopicName(topic string) (string, string, error) {
	matches := topicNameRE.FindAllStringSubmatch(topic, -1)
	if matches == nil || len(matches[0]) != 3 {
		return "", "", errors.Reason("topic %q does not match %q", topic, topicNameRE).Err()
	}

	return matches[0][1], matches[0][2], nil
}

// setBBStatus converts a swarming task result's state to a buildbucket status,
// updates StatusDetails and SummaryMarkdown accordingly. It Modifies the given
// *bbpb.Task in place.
//
// Note: this function could be move to a better place after Buildbucket
// TaskBackend Implementation logic is migrated to Go.
func setBBStatus(state apipb.TaskState, failure bool, bbTask *bbpb.Task) {
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
