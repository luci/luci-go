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
	"path/filepath"
	"regexp"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/data/stringset"
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
	maxExpirationSecs  = 7 * 24 * 60 * 60
	maxGracePeriodSecs = 60 * 60
	// maxTimeoutSecs is the maximum allowed timeout for I/O and
	// hard timeouts.
	// The overall timeout including the grace period and all overheads must fit
	// under 7 days (per RBE limits). So this value is slightly less than 7 days.
	maxTimeoutSecs = 7*24*60*60 - maxGracePeriodSecs - 60

	// maxPubsubLength is the limit for both PubsubTopic and PubsubUserdata.
	maxPubsubLength     = 1024
	maxCmdArgs          = 128
	maxEnvKeyCount      = 64
	maxEnvKeyLength     = 64
	maxEnvValueLength   = 1024
	maxOutputCount      = 4096
	maxOutputPathLength = 512
)

var (
	envKeyRe      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	casInstanceRe = regexp.MustCompile(`^projects/[a-z0-9-]+/instances/[a-z0-9-_]+$`)
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
	case teeErr(validateLength(req.GetPubsubUserdata(), maxPubsubLength), &err) != nil:
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

func validateLength(val string, limit int) error {
	if len(val) > limit {
		return errors.Reason("too long %q: %d > %d", val, len(val), limit).Err()
	}
	return nil
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
	var err error
	switch {
	case props == nil:
		return errors.New("required")
	case teeErr(validateTimeoutSecs(props.GracePeriodSecs, maxGracePeriodSecs), &err) != nil:
		return errors.Annotate(err, "grace_period_secs").Err()
	case teeErr(validateTimeoutSecs(props.ExecutionTimeoutSecs, maxTimeoutSecs), &err) != nil:
		return errors.Annotate(err, "execution_timeout_secs").Err()
	case teeErr(validateTimeoutSecs(props.IoTimeoutSecs, maxTimeoutSecs), &err) != nil:
		return errors.Annotate(err, "io_timeout_secs").Err()
	case teeErr(validateCommand(props.Command), &err) != nil:
		return errors.Annotate(err, "command").Err()
	case teeErr(validateEnv(props.Env), &err) != nil:
		return errors.Annotate(err, "env").Err()
	case teeErr(validateEnvPrefixes(props.EnvPrefixes), &err) != nil:
		return errors.Annotate(err, "env_prefixes").Err()
	case teeErr(validateCasInputRoot(props.CasInputRoot), &err) != nil:
		return errors.Annotate(err, "cas_input_root").Err()
	case teeErr(validateOutputs(props.Outputs), &err) != nil:
		return errors.Annotate(err, "outputs").Err()
	}
	return nil
}

func validateCommand(cmd []string) error {
	if len(cmd) == 0 {
		return errors.New("required")
	}
	if len(cmd) > maxCmdArgs {
		return errors.Reason("can have up to %d arguments", maxCmdArgs).Err()
	}
	return nil
}

func validateEnvKey(key string) error {
	var err error
	switch {
	case key == "":
		return errors.New("key is required")
	case teeErr(validateLength(key, maxEnvKeyLength), &err) != nil:
		return err
	case !envKeyRe.MatchString(key):
		return errors.Reason("%q should match %s", key, envKeyRe).Err()
	default:
		return nil
	}
}

func validateEnv(env []*apipb.StringPair) error {
	if len(env) > maxEnvKeyCount {
		return errors.Reason("can have up to %d keys", maxEnvKeyCount).Err()
	}
	envKeys := stringset.New(len(env))
	for i, env := range env {
		if !envKeys.Add(env.Key) {
			return errors.New("same key cannot be specified twice")
		}
		if err := validateEnvKey(env.Key); err != nil {
			return errors.Annotate(err, "key %d", i).Err()
		}
		if err := validateLength(env.Value, maxEnvValueLength); err != nil {
			return errors.Annotate(err, "value %d", i).Err()
		}
	}
	return nil
}

func validatePath(path string, maxLen int) error {
	var err error
	switch {
	case path == "":
		return errors.New("cannot be empty")
	case teeErr(validateLength(path, maxLen), &err) != nil:
		return err
	case strings.Contains(path, "\\"):
		return errors.New(`cannot contain "\\". On Windows forward-slashes will be replaced with back-slashes.`)
	case strings.HasPrefix(path, "/"):
		return errors.New(`cannot start with "/"`)
	case path != filepath.Clean(path):
		return errors.New("is not normalized")
	default:
		return nil
	}
}

func validateEnvPrefixes(envPrefixes []*apipb.StringListPair) error {
	if len(envPrefixes) > maxEnvKeyCount {
		return errors.Reason("can have up to %d keys", maxEnvKeyCount).Err()
	}
	epKeys := stringset.New(len(envPrefixes))
	for i, ep := range envPrefixes {
		if !epKeys.Add(ep.Key) {
			return errors.New("same key cannot be specified twice")
		}
		if err := validateEnvKey(ep.Key); err != nil {
			return errors.Annotate(err, "key %d", i).Err()
		}
		if len(ep.Value) == 0 {
			return errors.New("value is required")
		}
		for j, path := range ep.Value {
			if err := validatePath(path, maxEnvValueLength); err != nil {
				return errors.Annotate(err, "value %d-%d", i, j).Err()
			}
		}
	}
	return nil
}

func validateCasInputRoot(casInputRoot *apipb.CASReference) error {
	var err error
	switch {
	case casInputRoot == nil:
		return nil
	case casInputRoot.CasInstance == "":
		return errors.New("cas_instance is required")
	case !casInstanceRe.MatchString(casInputRoot.CasInstance):
		return errors.Reason("cas_instance %q should match %s", casInputRoot.CasInstance, casInstanceRe).Err()
	case teeErr(validateDigest(casInputRoot.Digest), &err) != nil:
		return errors.Annotate(err, "digest").Err()
	default:
		return nil
	}
}

func validateDigest(digest *apipb.Digest) error {
	switch {
	case digest == nil:
		return errors.New("required")
	case digest.Hash == "":
		return errors.New("hash is required")
	case digest.SizeBytes == 0:
		return errors.New("size_bytes is required")
	default:
		return nil
	}
}

func validateOutputs(outputs []string) error {
	if len(outputs) > maxOutputCount {
		return errors.Reason("can have up to %d outputs", maxOutputPathLength).Err()
	}
	for i, output := range outputs {
		if err := validatePath(output, maxOutputPathLength); err != nil {
			return errors.Annotate(err, "output %d", i).Err()
		}
	}
	return nil
}
