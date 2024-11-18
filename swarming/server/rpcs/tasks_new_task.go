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
	"path"
	"regexp"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/cipd/common"
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

	maxPubsubUserDataLength = 1024
	maxSliceCount           = 8
	maxTagCount             = 256
	maxCmdArgs              = 128
	maxEnvKeyCount          = 64
	maxEnvKeyLength         = 64
	maxEnvValueLength       = 1024
	maxOutputCount          = 4096
	maxOutputPathLength     = 512
	maxCacheCount           = 32
	maxCacheNameLength      = 128
	maxCachePathLength      = 256
	maxCIPDPackageCount     = 64
	maxCIPDServerLength     = 1024
	maxPackagePathLength    = 256
	maxDimensionKeyCount    = 32
	maxDimensionValueCount  = 16
	orDimSep                = "|"
	maxOrDimCount           = 8
)

var (
	envKeyRe      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	casInstanceRe = regexp.MustCompile(`^projects/[a-z0-9-]+/instances/[a-z0-9-_]+$`)
	cacheNameRe   = regexp.MustCompile(`^[a-z0-9_]+$`)
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
	case len(req.GetTaskSlices()) > maxSliceCount:
		return errors.Reason("can have up to %d slices", maxSliceCount).Err()
	case teeErr(validateParentTaskID(ctx, req.GetParentTaskId()), &err) != nil:
		return errors.Annotate(err, "parent_task_id").Err()
	case teeErr(validate.Priority(req.GetPriority()), &err) != nil:
		return errors.Annotate(err, "priority").Err()
	case teeErr(validateServiceAccount(req.GetServiceAccount()), &err) != nil:
		return errors.Annotate(err, "service_account").Err()
	case req.GetPubsubTopic() == "" && req.GetPubsubAuthToken() != "":
		return errors.New("pubsub_auth_token requires pubsub_topic")
	case req.GetPubsubTopic() == "" && req.GetPubsubUserdata() != "":
		return errors.New("pubsub_userdata requires pubsub_topic")
	case teeErr(validateLength(req.GetPubsubUserdata(), maxPubsubUserDataLength), &err) != nil:
		return errors.Annotate(err, "pubsub_userdata").Err()
	case teeErr(validateBotPingToleranceSecs(req.GetBotPingToleranceSecs()), &err) != nil:
		return errors.Annotate(err, "bot_ping_tolerance").Err()
	case teeErr(validateRealm(req.GetRealm()), &err) != nil:
		return err
	case len(req.GetTags()) > maxTagCount:
		return errors.Reason("up to %d tags", maxTagCount).Err()
	}

	_, _, err = validate.PubSubTopicName(req.GetPubsubTopic())
	if err != nil {
		return errors.Annotate(err, "pubsub_topic").Err()
	}

	for i, tag := range req.GetTags() {
		if err := validate.Tag(tag); err != nil {
			return errors.Annotate(err, "tag %d", i).Err()
		}
	}

	var sb []byte
	var pool string
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

		if err := validateTimeoutSecs(s.ExpirationSecs, maxExpirationSecs); err != nil {
			return errors.Annotate(err, "invalid expiration_secs of slice %d", i).Err()
		}

		curPool, err := validateProperties(s.Properties)
		if err != nil {
			return errors.Annotate(err, "invalid properties of slice %d", i).Err()
		}

		if pool == "" {
			pool = curPool
		} else if pool != curPool {
			return errors.Reason("each task slice must use the same pool dimensions; %q != %q", pool, curPool).Err()
		}
	}

	if len(req.TaskSlices) == 1 {
		return nil
	}

	propsSet := stringset.New(len(req.TaskSlices))
	for i, s := range req.TaskSlices {
		pb, err := proto.Marshal(s.Properties)
		if err != nil {
			return errors.Reason("failed to marshal properties for slice %d", i).Err()
		}
		if !propsSet.Add(string(pb)) {
			return errors.New("cannot request duplicate task slice")
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

func validateProperties(props *apipb.TaskProperties) (string, error) {
	var err error
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
		teeErr(validatePath(props.RelativeCwd, maxPackagePathLength), &err) != nil:
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
	cachePathSet, err := validateCaches(props.GetCaches())
	if err != nil {
		return "", errors.Annotate(err, "caches").Err()
	}

	if err = validateCipdInput(props.GetCipdInput(), props.GetIdempotent(), cachePathSet); err != nil {
		return "", errors.Annotate(err, "cipd_input").Err()
	}

	pool, err := validateDimensions(props.Dimensions)
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

func validatePath(p string, maxLen int) error {
	var err error
	switch {
	case p == "":
		return errors.New("cannot be empty")
	case teeErr(validateLength(p, maxLen), &err) != nil:
		return err
	case strings.Contains(p, "\\"):
		return errors.New(`cannot contain "\\". On Windows forward-slashes will be replaced with back-slashes.`)
	case strings.HasPrefix(p, "/"):
		return errors.New(`cannot start with "/"`)
	case p != path.Clean(p):
		return errors.Reason("%q is not normalized. Normalized is %q", p, path.Clean(p)).Err()
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
		for j, p := range ep.Value {
			if err := validatePath(p, maxEnvValueLength); err != nil {
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

func validateCaches(caches []*apipb.CacheEntry) (stringset.Set, error) {
	if len(caches) > maxCacheCount {
		return nil, errors.Reason("can have up to %d caches", maxCacheCount).Err()
	}

	nameSet := stringset.New(len(caches))
	pathSet := stringset.New(len(caches))
	for i, c := range caches {
		if err := validateCacheName(c.GetName()); err != nil {
			return nil, errors.Annotate(err, "cache name %d", i).Err()
		}
		if !nameSet.Add(c.GetName()) {
			return nil, errors.New("same cache name cannot be specified twice")
		}
		if err := validatePath(c.GetPath(), maxCachePathLength); err != nil {
			return nil, errors.Annotate(err, "cache path %d", i).Err()
		}
		if !pathSet.Add(c.GetPath()) {
			return nil, errors.New("same cache path cannot be specified twice")
		}
	}
	return pathSet, nil
}

func validateCacheName(name string) error {
	if name == "" {
		return errors.New("required")
	}
	if err := validateLength(name, maxCacheNameLength); err != nil {
		return err
	}
	if !cacheNameRe.MatchString(name) {
		return errors.Reason("%q should match %s", name, cacheNameRe).Err()
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
	case len(cipdInput.Packages) == 0:
		return errors.New("cannot have an empty package list")
	case len(cipdInput.Packages) > maxCIPDPackageCount:
		return errors.Reason("up to %d CIPD packages can be listed for a task", maxCIPDPackageCount).Err()
	}

	type pathName struct {
		p    string
		name string
	}
	pkgPathNames := make(map[pathName]struct{}, len(cipdInput.Packages))
	for i, pkg := range cipdInput.Packages {
		if err = validateCIPDPackage(pkg, idempotent, cachePathSet); err != nil {
			return errors.Annotate(err, "package %d", i).Err()
		}

		pn := pathName{pkg.Path, pkg.PackageName}
		if _, ok := pkgPathNames[pn]; ok {
			return errors.Reason("package %q is specified more than once in path %s", pkg.PackageName, pkg.Path).Err()
		} else {
			pkgPathNames[pn] = struct{}{}
		}
	}

	return nil
}

func validateCIPDServer(server string) error {
	if server == "" {
		return errors.New("required")
	}
	if err := validateLength(server, maxCIPDServerLength); err != nil {
		return err
	}
	return validate.SecureURL(server)
}

func validateCIPDPackageCommon(pkg *apipb.CipdPackage) error {
	var err error
	switch {
	case pkg == nil:
		return errors.New("required")
	case pkg.PackageName == "":
		return errors.New("package_name is required")
	case teeErr(validate.CipdPackageName(pkg.PackageName), &err) != nil:
		return errors.Annotate(err, "package_name").Err()
	case pkg.Version == "":
		return errors.New("version is required")
	case teeErr(validate.CipdPackageVersion(pkg.Version), &err) != nil:
		return errors.Annotate(err, "version").Err()
	default:
		return nil
	}
}

func validateCIPDClientPackage(pkg *apipb.CipdPackage) error {
	if pkg.GetPath() != "" {
		return errors.New("path must be unset")
	}
	return validateCIPDPackageCommon(pkg)
}

func validateCIPDPackage(pkg *apipb.CipdPackage, idempotent bool, cachePathSet stringset.Set) error {
	var err error
	switch {
	case teeErr(validateCIPDPackageCommon(pkg), &err) != nil:
		return err
	case teeErr(validatePath(pkg.Path, maxPackagePathLength), &err) != nil:
		return errors.Annotate(err, "path").Err()
	case cachePathSet.Has(pkg.Path):
		return errors.Reason(
			"path %q is mapped to a named cache and cannot be a target of CIPD installation",
			pkg.Path).Err()
	case idempotent && teeErr(validatePinnedInstanceVersion(pkg.Version), &err) != nil:
		return errors.New(
			"an idempotent task cannot have unpinned packages; use tags or instance IDs as package versions")
	default:
		return nil
	}
}

func validatePinnedInstanceVersion(v string) error {
	if common.ValidateInstanceID(v, common.AnyHash) == nil ||
		common.ValidateInstanceTag(v) == nil {
		return nil
	}
	return errors.Reason("%q is not a pinned instance version", v).Err()
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
