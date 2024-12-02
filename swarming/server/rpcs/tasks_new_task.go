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
	"fmt"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/realms"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
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

	maxPubsubUserDataLength = 1024
	maxSliceCount           = 8
	maxTagCount             = 256
	maxCmdArgs              = 128
	maxOutputCount          = 4096
	maxOutputPathLength     = 512
	maxCIPDServerLength     = 1024
	maxDimensionKeyCount    = 32
	maxDimensionValueCount  = 16
	orDimSep                = "|"
	maxOrDimCount           = 8

	defaultBotPingToleranceSecs = 1200
)

var (
	casInstanceRe = regexp.MustCompile(`^projects/[a-z0-9-]+/instances/[a-z0-9-_]+$`)
	cacheNameRe   = regexp.MustCompile(`^[a-z0-9_]+$`)
)

// NewTask implements the corresponding RPC method.
func (srv *TasksServer) NewTask(ctx context.Context, req *apipb.NewTaskRequest) (*apipb.TaskRequestMetadataResponse, error) {
	logNewRequest(ctx, req)

	pool, err := validateNewTask(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad request: %s", err)
	}

	// Check permissions
	state := State(ctx)

	// pool level check
	checkResult := state.ACL.CheckPoolPerm(ctx, pool, acls.PermPoolsCreateTask)
	if !checkResult.Permitted || checkResult.InternalError {
		return nil, checkResult.ToGrpcErr()
	}

	// task realm level checks
	if err = setTaskRealm(ctx, state, req, pool); err != nil {
		return nil, err
	}
	saToCheck := req.ServiceAccount
	if nonEmailServiceAccount(req.ServiceAccount) {
		saToCheck = ""
	}
	checkResult = State(ctx).ACL.CheckNewTaskAllowed(ctx, req.Realm, saToCheck)
	if !checkResult.Permitted || checkResult.InternalError {
		return nil, checkResult.ToGrpcErr()
	}

	_, err = toTaskRequestEntities(ctx, req, pool)
	if err != nil {
		return nil, err
	}
	return nil, status.Errorf(codes.Unimplemented, "not implemented yet")
}

// teeErr saves `err` in `keep` and then returns `err`
func teeErr(err error, keep *error) error {
	*keep = err
	return err
}

func validateNewTask(ctx context.Context, req *apipb.NewTaskRequest) (pool string, err error) {
	switch {
	case req.GetName() == "":
		return "", errors.New("name is required")
	case req.GetProperties() != nil:
		return "", errors.New("properties is deprecated, use task_slices instead.")
	case req.GetExpirationSecs() != 0:
		return "", errors.New("expiration_secs is deprecated, set it in task_slices instead.")
	case len(req.GetTaskSlices()) == 0:
		return "", errors.New("task_slices is required")
	case len(req.GetTaskSlices()) > maxSliceCount:
		return "", errors.Reason("can have up to %d slices", maxSliceCount).Err()
	case teeErr(validateParentTaskID(ctx, req.GetParentTaskId()), &err) != nil:
		return "", errors.Annotate(err, "parent_task_id").Err()
	case teeErr(validate.Priority(req.GetPriority()), &err) != nil:
		return "", errors.Annotate(err, "priority").Err()
	case teeErr(validateServiceAccount(req.GetServiceAccount()), &err) != nil:
		return "", errors.Annotate(err, "service_account").Err()
	case req.GetPubsubTopic() == "" && req.GetPubsubAuthToken() != "":
		return "", errors.New("pubsub_auth_token requires pubsub_topic")
	case req.GetPubsubTopic() == "" && req.GetPubsubUserdata() != "":
		return "", errors.New("pubsub_userdata requires pubsub_topic")
	case teeErr(validate.Length(req.GetPubsubUserdata(), maxPubsubUserDataLength), &err) != nil:
		return "", errors.Annotate(err, "pubsub_userdata").Err()
	case teeErr(validateBotPingToleranceSecs(req.GetBotPingToleranceSecs()), &err) != nil:
		return "", errors.Annotate(err, "bot_ping_tolerance").Err()
	case teeErr(validateRealm(req.GetRealm()), &err) != nil:
		return "", err
	case len(req.GetTags()) > maxTagCount:
		return "", errors.Reason("up to %d tags", maxTagCount).Err()
	}

	_, _, err = validate.PubSubTopicName(req.GetPubsubTopic())
	if err != nil {
		return "", errors.Annotate(err, "pubsub_topic").Err()
	}

	for i, tag := range req.GetTags() {
		if err := validate.Tag(tag); err != nil {
			return "", errors.Annotate(err, "tag %d", i).Err()
		}
	}

	var sb []byte
	for i, s := range req.TaskSlices {
		curSecret := s.Properties.GetSecretBytes()
		curLen := len(curSecret)
		if curLen > maxSecretBytesLength {
			return "", errors.Reason("secret_bytes of slice %d has size %d, exceeding limit %d", i, curLen, maxSecretBytesLength).Err()
		}
		if len(sb) == 0 {
			sb = curSecret
		} else {
			if curLen > 0 && !bytes.Equal(sb, curSecret) {
				return "", errors.New("when using secret_bytes multiple times, all values must match")
			}
		}

		if err := validateTimeoutSecs(s.ExpirationSecs, maxExpirationSecs); err != nil {
			return "", errors.Annotate(err, "invalid expiration_secs of slice %d", i).Err()
		}

		curPool, err := validateProperties(s.Properties)
		if err != nil {
			return "", errors.Annotate(err, "invalid properties of slice %d", i).Err()
		}

		if pool == "" {
			pool = curPool
		} else if pool != curPool {
			return "", errors.Reason("each task slice must use the same pool dimensions; %q != %q", pool, curPool).Err()
		}
	}

	if len(req.TaskSlices) == 1 {
		return pool, nil
	}

	propsSet := stringset.New(len(req.TaskSlices))
	for i, s := range req.TaskSlices {
		pb, err := proto.Marshal(s.Properties)
		if err != nil {
			return "", errors.Reason("failed to marshal properties for slice %d", i).Err()
		}
		if !propsSet.Add(string(pb)) {
			return "", errors.New("cannot request duplicate task slice")
		}
	}
	return pool, nil
}

func validateParentTaskID(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	_, err := model.TaskIDToRequestKey(ctx, id)
	return err
}

func nonEmailServiceAccount(sa string) bool {
	return sa == "" || sa == "none" || sa == "bot"
}

func validateServiceAccount(sa string) error {
	if nonEmailServiceAccount(sa) {
		return nil
	}
	return validate.ServiceAccount(sa)
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

func validateProperties(props *apipb.TaskProperties) (pool string, err error) {
	switch {
	case props == nil:
		return "", errors.New("required")
	case teeErr(validateTimeoutSecs(props.GracePeriodSecs, maxGracePeriodSecs), &err) != nil:
		return "", errors.Annotate(err, "grace_period_secs").Err()
	case teeErr(validateTimeoutSecs(props.ExecutionTimeoutSecs, maxTimeoutSecs), &err) != nil:
		return "", errors.Annotate(err, "execution_timeout_secs").Err()
	case teeErr(validateTimeoutSecs(props.IoTimeoutSecs, maxTimeoutSecs), &err) != nil:
		return "", errors.Annotate(err, "io_timeout_secs").Err()
	case teeErr(validateCommand(props.Command), &err) != nil:
		return "", errors.Annotate(err, "command").Err()
	case props.RelativeCwd != "" &&
		teeErr(validate.Path(props.RelativeCwd, validate.MaxPackagePathLength), &err) != nil:
		return "", errors.Annotate(err, "relative_cwd").Err()
	case teeErr(validateEnv(props.Env), &err) != nil:
		return "", errors.Annotate(err, "env").Err()
	case teeErr(validateEnvPrefixes(props.EnvPrefixes), &err) != nil:
		return "", errors.Annotate(err, "env_prefixes").Err()
	case teeErr(validateCasInputRoot(props.CasInputRoot), &err) != nil:
		return "", errors.Annotate(err, "cas_input_root").Err()
	case teeErr(validateOutputs(props.Outputs), &err) != nil:
		return "", errors.Annotate(err, "outputs").Err()
	}
	cachePathSet, merr := validate.Caches(props.GetCaches())
	if merr.AsError() != nil {
		return "", errors.Annotate(merr.AsError(), "caches").Err()
	}

	if err = validateCipdInput(props.GetCipdInput(), props.GetIdempotent(), cachePathSet); err != nil {
		return "", errors.Annotate(err, "cipd_input").Err()
	}

	pool, err = validateDimensions(props.Dimensions)
	if err != nil {
		return "", errors.Annotate(err, "dimensions").Err()
	}
	return pool, nil
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

func validateEnv(env []*apipb.StringPair) error {
	if len(env) > validate.MaxEnvVarCount {
		return errors.Reason("can have up to %d keys", validate.MaxEnvVarCount).Err()
	}
	envKeys := stringset.New(len(env))
	for i, env := range env {
		if !envKeys.Add(env.Key) {
			return errors.New("same key cannot be specified twice")
		}
		if err := validate.EnvVar(env.Key); err != nil {
			return errors.Annotate(err, "key %d", i).Err()
		}
		if err := validate.Length(env.Value, validate.MaxEnvValueLength); err != nil {
			return errors.Annotate(err, "value %d", i).Err()
		}
	}
	return nil
}

func validateEnvPrefixes(envPrefixes []*apipb.StringListPair) error {
	if len(envPrefixes) > validate.MaxEnvVarCount {
		return errors.Reason("can have up to %d keys", validate.MaxEnvVarCount).Err()
	}
	epKeys := stringset.New(len(envPrefixes))
	for i, ep := range envPrefixes {
		if !epKeys.Add(ep.Key) {
			return errors.New("same key cannot be specified twice")
		}
		if err := validate.EnvVar(ep.Key); err != nil {
			return errors.Annotate(err, "key %d", i).Err()
		}
		if len(ep.Value) == 0 {
			return errors.New("value is required")
		}
		for j, p := range ep.Value {
			if err := validate.Path(p, validate.MaxEnvValueLength); err != nil {
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
		if err := validate.Path(output, maxOutputPathLength); err != nil {
			return errors.Annotate(err, "output %d", i).Err()
		}
	}
	return nil
}

func validateCipdInput(cipdInput *apipb.CipdInput, idempotent bool, cachePathSet stringset.Set) error {
	var err error
	switch {
	case cipdInput == nil:
		return nil
	case teeErr(validateCIPDServer(cipdInput.Server), &err) != nil:
		return errors.Annotate(err, "server").Err()
	case teeErr(validateCIPDClientPackage(cipdInput.ClientPackage), &err) != nil:
		return errors.Annotate(err, "client_package").Err()
	}
	merr := validate.CIPDPackages(cipdInput.Packages, idempotent, cachePathSet)
	if merr.AsError() == nil {
		return nil
	}
	return errors.Annotate(merr.AsError(), "packages").Err()
}

func validateCIPDServer(server string) error {
	if server == "" {
		return errors.New("required")
	}
	if err := validate.Length(server, maxCIPDServerLength); err != nil {
		return err
	}
	return validate.SecureURL(server)
}

func validateCIPDClientPackage(pkg *apipb.CipdPackage) error {
	var err error
	switch {
	case pkg == nil:
		return errors.New("required")
	case pkg.GetPath() != "":
		return errors.New("path must be unset")
	case teeErr(validate.CipdPackageName(pkg.PackageName), &err) != nil:
		return errors.Annotate(err, "package_name").Err()
	case teeErr(validate.CipdPackageVersion(pkg.Version), &err) != nil:
		return errors.Annotate(err, "version").Err()
	default:
		return nil
	}
}

func validateDimensions(dims []*apipb.StringPair) (string, error) {
	dimMap := model.StringPairsToMap(dims)
	if len(dimMap) > maxDimensionKeyCount {
		return "", errors.Reason("can have up to %d keys", maxDimensionKeyCount).Err()
	}

	singleValue := func(values []string) bool {
		return len(values) == 1 && !strings.Contains(values[0], orDimSep)
	}

	var pool string
	orDimNum := 1
	for key, values := range dimMap {
		if err := validate.DimensionKey(key); err != nil {
			return "", errors.Annotate(err, "key %q", key).Err()
		}

		if key == "pool" {
			if !singleValue(values) {
				return "", errors.New("pool cannot be specified more than once")
			}
			pool = values[0]
		}

		if key == "id" {
			if !singleValue(values) {
				return "", errors.New("id cannot be specified more than once")
			}
		}

		if len(values) > maxDimensionValueCount {
			return "", errors.Reason("can have up to %d values for key %q", maxDimensionValueCount, key).Err()
		}

		valueSet := stringset.NewFromSlice(values...)
		if len(valueSet) != len(values) {
			return "", errors.Reason("key %q has repeated values", key).Err()
		}

		for _, value := range values {
			if err := validate.DimensionValue(value); err != nil {
				return "", errors.Annotate(err, "value %q for key %q", value, key).Err()
			}

			orValues := strings.Split(value, orDimSep)
			orDimNum *= len(orValues)
			if orDimNum > maxOrDimCount {
				return "", errors.Reason(
					"%q possible dimension subset for or dimensions should not be more than %d",
					value, maxOrDimCount).Err()
			}
			for _, ov := range orValues {
				if err := validate.DimensionValue(ov); err != nil {
					return "", errors.Annotate(err, "value %q for key %q", value, key).Err()
				}
			}
		}
	}

	if pool == "" {
		return "", errors.New("pool is required")
	}

	return pool, nil
}

// setTaskRealm updates req in place if it doesn't have Realm using pool's default task realm.
//
// Assumes it is called after pool perm check, so pool config should exist.
//
// If there's an issue, returns a grpc error.
func setTaskRealm(ctx context.Context, state *RequestState, req *apipb.NewTaskRequest, pool string) error {
	if req.Realm != "" {
		logging.Infof(ctx, "Using task realm %s", req.Realm)
		return nil
	}

	poolCfg := state.Config.Pool(pool)
	if poolCfg == nil {
		panic(fmt.Sprintf("pool %q not found after pool perm check", pool))
	}

	if poolCfg.DefaultTaskRealm == "" {
		return status.Error(codes.InvalidArgument, "task realm is required")
	}

	logging.Infof(ctx, "Using default_task_realm %s", poolCfg.DefaultTaskRealm)
	req.Realm = poolCfg.DefaultTaskRealm
	return nil
}

func logNewRequest(ctx context.Context, req *apipb.NewTaskRequest) {
	secrets := make([][]byte, len(req.TaskSlices))
	for i, s := range req.TaskSlices {
		secrets[i] = s.GetProperties().GetSecretBytes()
		if len(s.GetProperties().GetSecretBytes()) > 0 {
			s.Properties.SecretBytes = []byte("<REDACTED>")
		}
	}
	logging.Infof(ctx, "NewTaskRequest %+v", req)
	for i, s := range req.TaskSlices {
		s.Properties.SecretBytes = secrets[i]
	}
}

type trEntities struct {
	request     *model.TaskRequest
	secretBytes *model.SecretBytes
}

// toTaskRequestEntities converts NewTaskRequest to trEntities.
//
// Returns a grpc error if there's an issue.
func toTaskRequestEntities(ctx context.Context, req *apipb.NewTaskRequest, pool string) (*trEntities, error) {
	chk := State(ctx).ACL
	now := clock.Now(ctx).UTC()

	tr := &model.TaskRequest{
		Created: now,
		Name:    req.Name,
		// TODO(chanli): check parent task then update this request using parent
		// data.
		ParentTaskID:         datastore.NewIndexedNullable(req.ParentTaskId),
		Authenticated:        chk.Caller(),
		User:                 req.User,
		ManualTags:           req.Tags,
		ServiceAccount:       req.ServiceAccount,
		Realm:                req.Realm,
		RealmsEnabled:        true,
		Priority:             int64(req.Priority),
		PubSubTopic:          req.PubsubTopic,
		PubSubAuthToken:      req.PubsubAuthToken,
		PubSubUserData:       req.PubsubUserdata,
		ResultDB:             model.ResultDBConfig{Enable: req.Resultdb.GetEnable()},
		BotPingToleranceSecs: int64(req.BotPingToleranceSecs),
	}
	res := &trEntities{
		request: tr,
	}

	// Priority
	if tr.Priority < 20 {
		res := chk.CheckPoolPerm(ctx, pool, acls.PermPoolsCreateHighPriorityTask)
		if res.InternalError {
			return nil, res.ToGrpcErr()
		}
		if !res.Permitted {
			// Silently drop the priority for normal users.
			tr.Priority = 20
		}
	}

	// ServiceAccount
	if tr.ServiceAccount == "" {
		tr.ServiceAccount = "none"
	}

	// BotPingToleranceSecs
	if tr.BotPingToleranceSecs == 0 {
		tr.BotPingToleranceSecs = defaultBotPingToleranceSecs
	}

	// TaskSlices
	var totalExpirationSecs int32
	for _, s := range req.GetTaskSlices() {
		totalExpirationSecs += s.ExpirationSecs
		props := toTaskProperties(s.Properties)
		ts := model.TaskSlice{
			Properties:     props,
			ExpirationSecs: int64(s.ExpirationSecs),
		}
		tr.TaskSlices = append(tr.TaskSlices, ts)

		if res.secretBytes == nil && props.HasSecretBytes {
			res.secretBytes = &model.SecretBytes{
				SecretBytes: s.Properties.SecretBytes,
			}
		}
	}
	tr.Expiration = now.Add(time.Duration(totalExpirationSecs) * time.Second)

	// TODO(chanli): apply template.
	// TODO(chanli): add auto generated tags.
	// TODO(chanli): apply server defaults.
	// TODO(chanli): apply pool config.

	return res, nil
}

func toTaskProperties(p *apipb.TaskProperties) model.TaskProperties {
	props := model.TaskProperties{
		Idempotent:           p.Idempotent,
		ExecutionTimeoutSecs: int64(p.ExecutionTimeoutSecs),
		GracePeriodSecs:      int64(p.GracePeriodSecs),
		IOTimeoutSecs:        int64(p.IoTimeoutSecs),
		Command:              p.Command,
		RelativeCwd:          p.RelativeCwd,
		Outputs:              p.Outputs,
		HasSecretBytes:       len(p.SecretBytes) > 0,
		CASInputRoot: model.CASReference{
			CASInstance: p.GetCasInputRoot().GetCasInstance(),
			Digest: model.CASDigest{
				Hash:      p.GetCasInputRoot().GetDigest().GetHash(),
				SizeBytes: p.GetCasInputRoot().GetDigest().GetSizeBytes(),
			},
		},
		CIPDInput: model.CIPDInput{
			Server: p.GetCipdInput().GetServer(),
			ClientPackage: model.CIPDPackage{
				PackageName: p.GetCipdInput().GetClientPackage().GetPackageName(),
				Version:     p.GetCipdInput().GetClientPackage().GetVersion(),
			},
		},
	}

	// Dimensions
	if len(p.Dimensions) > 0 {
		props.Dimensions = make(model.TaskDimensions, len(p.Dimensions))
		for _, d := range p.Dimensions {
			props.Dimensions[d.Key] = append(props.Dimensions[d.Key], d.Value)
		}
	}

	// Env
	if len(p.Env) > 0 {
		props.Env = make(model.Env, len(p.Env))
		for _, e := range p.Env {
			props.Env[e.Key] = e.Value
		}
	}

	// EnvPrefixes
	if len(p.EnvPrefixes) > 0 {
		props.EnvPrefixes = make(model.EnvPrefixes, len(p.EnvPrefixes))
		for _, ep := range p.EnvPrefixes {
			props.EnvPrefixes[ep.Key] = ep.Value
		}
	}

	// Caches
	if len(p.Caches) > 0 {
		props.Caches = make([]model.CacheEntry, len(p.Caches))
		for i, c := range p.Caches {
			props.Caches[i] = model.CacheEntry{
				Name: c.Name,
				Path: c.Path,
			}
		}
	}

	// CIPDInput
	if len(p.GetCipdInput().GetPackages()) > 0 {
		props.CIPDInput.Packages = make([]model.CIPDPackage, len(p.CipdInput.Packages))
		for i, pkg := range p.CipdInput.Packages {
			props.CIPDInput.Packages[i] = model.CIPDPackage{
				PackageName: pkg.PackageName,
				Version:     pkg.Version,
				Path:        pkg.Path,
			}
		}
	}
	return props
}
