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

package rpcs

import (
	"bytes"
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth/realms"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/validate"
)

const (
	// maxSecretBytesLength is the max length of secret bytes.
	maxSecretBytesLength = 20 * 1024

	// minTimeOutSecs is the lower bound for various timeouts.
	minTimeOutSecs = 30
	// maxExpirationSecs is the maximum allowed expiration for a pending task.
	// Seven days in seconds.
	maxExpirationSecs = 7 * 24 * 60 * 60

	// maxPubsubLength is the limit for both PubsubTopic and PubsubUserdata.
	maxPubsubLength = 1024
)

// NewTask implements the corresponding RPC method.
func (srv *TasksServer) NewTask(ctx context.Context, req *apipb.NewTaskRequest) (*apipb.TaskRequestMetadataResponse, error) {
	if err := validateNewTask(ctx, req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad request: %s", err)
	}
	return nil, status.Errorf(codes.Unimplemented, "not implemented yet")
}

// teeErr saves `err` in `keep` and then returns `err`
func teeErr(err error, keep *error) error {
	*keep = err
	return err
}

func validateNewTask(ctx context.Context, req *apipb.NewTaskRequest) error {
	var err error
	switch {
	case req.GetName() == "":
		return errors.New("name is required")
	case req.GetProperties() != nil:
		return errors.New("properties is deprecated, use task_slices instead.")
	case req.GetExpirationSecs() != 0:
		return errors.New("expiration_secs is deprecated, set it in task_slices instead.")
	case len(req.GetTaskSlices()) == 0:
		return errors.New("task_slices is required")
	case teeErr(validateParentTaskID(ctx, req.GetParentTaskId()), &err) != nil:
		return errors.Annotate(err, "parent_task_id").Err()
	case teeErr(validate.Priority(req.GetPriority()), &err) != nil:
		return errors.Annotate(err, "priority").Err()
	case teeErr(validateServiceAccount(req.GetServiceAccount()), &err) != nil:
		return errors.Annotate(err, "service_account").Err()
	case teeErr(validatePubsubTopic(req.GetPubsubTopic()), &err) != nil:
		return errors.Annotate(err, "pubsub_topic").Err()
	case teeErr(validatePubsubUserdata(req.GetPubsubUserdata()), &err) != nil:
		return errors.Annotate(err, "pubsub_userdata").Err()
	case teeErr(validateBotPingToleranceSecs(req.GetBotPingToleranceSecs()), &err) != nil:
		return errors.Annotate(err, "bot_ping_tolerance").Err()
	case teeErr(validateRealm(req.GetRealm()), &err) != nil:
		return err
	}

	for i, tag := range req.GetTags() {
		if err := validate.Tag(tag); err != nil {
			return errors.Annotate(err, "tag %d", i).Err()
		}
	}

	var sb []byte
	for i, s := range req.TaskSlices {
		curSecret := s.Properties.GetSecretBytes()
		curLen := len(curSecret)
		if curLen > maxSecretBytesLength {
			return errors.Reason("secret_bytes of slice %d has size %d, exceeding limit %d", i, curLen, maxSecretBytesLength).Err()
		}
		if len(sb) == 0 {
			sb = curSecret
		} else {
			if curLen > 0 && !bytes.Equal(sb, curSecret) {
				return errors.New("when using secret_bytes multiple times, all values must match")
			}
		}

		if err := validateTimeoutSecs(s.GetExpirationSecs(), maxExpirationSecs); err != nil {
			return errors.Annotate(err, "invalid expiration_secs of slice %d", i).Err()
		}

		if err := validateProperties(s.Properties); err != nil {
			return errors.Annotate(err, "invalid properties of slice %d", i).Err()
		}
	}
	return nil
}

func validateParentTaskID(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	_, err := model.TaskIDToRequestKey(ctx, id)
	return err
}

func validateServiceAccount(sa string) error {
	if sa == "" || sa == "none" || sa == "bot" {
		return nil
	}
	return validate.ServiceAccount(sa)
}

func validatePubsubTopic(topic string) error {
	switch {
	case topic == "":
		return nil
	case len(topic) > maxPubsubLength:
		return errors.Reason("too long %s: %d > %d", topic, len(topic), maxPubsubLength).Err()
	case !strings.Contains(topic, "/"):
		return errors.Reason("%q must be a well formatted pubsub topic", topic).Err()
	default:
		return nil
	}
}

func validatePubsubUserdata(data string) error {
	if len(data) > maxPubsubLength {
		return errors.Reason("too long %s: %d > %d", data, len(data), maxPubsubLength).Err()
	}
	return nil
}

func validateRealm(realm string) error {
	if realm == "" {
		return nil
	}
	return realms.ValidateRealmName(realm, realms.GlobalScope)
}

func validateBotPingToleranceSecs(bpt int32) error {
	if bpt == 0 {
		return nil
	}
	return validate.BotPingTolerance(int64(bpt))
}

func validateTimeoutSecs(timeoutSecs int32, max int32) error {
	if timeoutSecs > max || timeoutSecs < minTimeOutSecs {
		return errors.Reason(
			"%d must be between %ds and %ds", timeoutSecs, minTimeOutSecs, max).Err()
	}
	return nil
}

func validateProperties(props *apipb.TaskProperties) error {
	return nil
}
