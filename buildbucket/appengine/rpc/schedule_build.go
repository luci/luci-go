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

package rpc

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	bb "go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildid"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/internal/resultdb"
	"go.chromium.org/luci/buildbucket/appengine/internal/search"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// validateExpirationDuration validates the given expiration duration.
func validateExpirationDuration(d *durationpb.Duration) error {
	switch {
	case d.GetNanos() != 0:
		return errors.Reason("nanos must not be specified").Err()
	case d.GetSeconds() < 0:
		return errors.Reason("seconds must not be negative").Err()
	case d.GetSeconds()%60 != 0:
		return errors.Reason("seconds must be a multiple of 60").Err()
	default:
		return nil
	}
}

// validateRequestedDimension validates the requested dimension.
func validateRequestedDimension(dim *pb.RequestedDimension) error {
	var err error
	switch {
	case teeErr(validateExpirationDuration(dim.GetExpiration()), &err) != nil:
		return errors.Annotate(err, "expiration").Err()
	case dim.GetKey() == "":
		return errors.Reason("key must be specified").Err()
	case dim.Key == "caches":
		return errors.Annotate(errors.Reason("caches may only be specified in builder configs (cr-buildbucket.cfg)").Err(), "key").Err()
	case dim.Key == "pool":
		return errors.Annotate(errors.Reason("pool may only be specified in builder configs (cr-buildbucket.cfg)").Err(), "key").Err()
	case dim.Value == "":
		return errors.Reason("value must be specified").Err()
	default:
		return nil
	}
}

// validateRequestedDimensions validates the requested dimensions.
func validateRequestedDimensions(dims []*pb.RequestedDimension) error {
	for i, dim := range dims {
		if err := validateRequestedDimension(dim); err != nil {
			return errors.Annotate(err, "[%d]", i).Err()
		}
	}
	return nil
}

// validateExecutable validates the given executable.
func validateExecutable(exe *pb.Executable) error {
	var err error
	switch {
	case exe.GetCipdPackage() != "":
		return errors.Reason("cipd_package must not be specified").Err()
	case exe.GetCipdVersion() != "" && teeErr(common.ValidateInstanceVersion(exe.CipdVersion), &err) != nil:
		return errors.Annotate(err, "cipd_version").Err()
	default:
		return nil
	}
}

// validateGerritChange validates a given gerrit change.
func validateGerritChange(ch *pb.GerritChange) error {
	switch {
	case ch.GetChange() == 0:
		return errors.Reason("change must be specified").Err()
	case ch.Host == "":
		return errors.Reason("host must be specified").Err()
	case ch.Patchset == 0:
		return errors.Reason("patchset must be specified").Err()
	case ch.Project == "":
		return errors.Reason("project must be specified").Err()
	default:
		return nil
	}
}

// validateGerritChanges validates the given gerrit changes.
func validateGerritChanges(changes []*pb.GerritChange) error {
	for i, ch := range changes {
		if err := validateGerritChange(ch); err != nil {
			return errors.Annotate(err, "[%d]", i).Err()
		}
	}
	return nil
}

// validateNotificationConfig validates the given notification config.
func validateNotificationConfig(n *pb.NotificationConfig) error {
	switch {
	case n.GetPubsubTopic() == "":
		return errors.Reason("pubsub_topic must be specified").Err()
	case len(n.UserData) > 4096:
		return errors.Reason("user_data cannot exceed 4096 bytes").Err()
	default:
		return nil
	}
}

// validateSchedule validates the given request.
func validateSchedule(req *pb.ScheduleBuildRequest) error {
	var err error
	switch {
	case strings.Contains(req.GetRequestId(), "/"):
		return errors.Reason("request_id cannot contain '/'").Err()
	case req.GetBuilder() == nil && req.GetTemplateBuildId() == 0:
		return errors.Reason("builder or template_build_id is required").Err()
	case req.Builder != nil && teeErr(protoutil.ValidateRequiredBuilderID(req.Builder), &err) != nil:
		return errors.Annotate(err, "builder").Err()
	case teeErr(validateRequestedDimensions(req.Dimensions), &err) != nil:
		return errors.Annotate(err, "dimensions").Err()
	case teeErr(validateExecutable(req.Exe), &err) != nil:
		return errors.Annotate(err, "exe").Err()
	case teeErr(validateGerritChanges(req.GerritChanges), &err) != nil:
		return errors.Annotate(err, "gerrit_changes").Err()
	case req.GitilesCommit != nil && teeErr(validateCommitWithRef(req.GitilesCommit), &err) != nil:
		return errors.Annotate(err, "gitiles_commit").Err()
	case req.Notify != nil && teeErr(validateNotificationConfig(req.Notify), &err) != nil:
		return errors.Annotate(err, "notify").Err()
	case req.Priority < 0 || req.Priority > 255:
		return errors.Reason("priority must be in [0, 255]").Err()
	case teeErr(validateTags(req.Tags, TagNew), &err) != nil:
		return errors.Annotate(err, "tags").Err()
	}

	for expName := range req.Experiments {
		if err := validateExperimentName(expName); err != nil {
			return errors.Annotate(err, "experiment %q", expName).Err()
		}
	}

	// TODO(crbug/1042991): Validate Properties.
	return nil
}

var experimentNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)*$`)

func validateExperimentName(expName string) error {
	switch {
	case !experimentNameRE.MatchString(expName):
		return errors.Reason("does not match %q", experimentNameRE).Err()
	case strings.HasPrefix(expName, "luci.") && !bb.WellKnownExperiments.Has(expName):
		return errors.New(`unknown experiment has reserved prefix "luci."`)
	}
	return nil
}

// templateBuildMask enumerates properties to read from template builds. See
// scheduleRequestFromTemplate.
var templateBuildMask = mask.MustFromReadMask(
	&pb.Build{},
	"builder",
	"critical",
	"exe",
	"input.experimental",
	"input.gerrit_changes",
	"input.gitiles_commit",
	"input.properties",
	"tags",
)

// scheduleRequestFromTemplate returns a request with fields populated by the
// given template_build_id if there is one. Fields set in the request override
// fields populated from the template. Does not modify the incoming request.
func scheduleRequestFromTemplate(ctx context.Context, req *pb.ScheduleBuildRequest) (*pb.ScheduleBuildRequest, error) {
	if req.GetTemplateBuildId() == 0 {
		return req, nil
	}

	bld, err := getBuild(ctx, req.TemplateBuildId)
	if err != nil {
		return nil, err
	}
	if err := perm.HasInBuilder(ctx, perm.BuildsGet, bld.Proto.Builder); err != nil {
		return nil, err
	}

	b := bld.ToSimpleBuildProto(ctx)
	if err := model.LoadBuildDetails(ctx, templateBuildMask, b); err != nil {
		return nil, err
	}

	ret := &pb.ScheduleBuildRequest{
		Builder:       b.Builder,
		Critical:      b.Critical,
		Exe:           b.Exe,
		GerritChanges: b.Input.GerritChanges,
		GitilesCommit: b.Input.GitilesCommit,
		Properties:    b.Input.Properties,
		Tags:          b.Tags,
	}

	ret.Experiments = make(map[string]bool, len(bld.Experiments))
	bld.IterExperiments(func(enabled bool, exp string) bool {
		ret.Experiments[exp] = enabled
		return true
	})

	// proto.Merge concatenates repeated fields. Here the desired behavior is replacement,
	// so clear slices from the return value before merging, if specified in the request.
	if req.Exe != nil {
		ret.Exe = nil
	}
	if len(req.GerritChanges) > 0 {
		ret.GerritChanges = nil
	}
	if req.Properties != nil {
		ret.Properties = nil
	}
	if len(req.Tags) > 0 {
		ret.Tags = nil
	}
	proto.Merge(ret, req)
	ret.TemplateBuildId = 0

	return ret, nil
}

// fetchBuilderConfigs returns the Builder configs referenced by the given
// requests in a map of Bucket ID -> Builder name -> *pb.Builder.
func fetchBuilderConfigs(ctx context.Context, reqs []*pb.ScheduleBuildRequest) (map[string]map[string]*pb.Builder, error) {
	cfgs := map[string]map[string]*pb.Builder{}
	var bldrs []*model.Builder
	for _, req := range reqs {
		bucket := fmt.Sprintf("%s/%s", req.Builder.Project, req.Builder.Bucket)
		if _, ok := cfgs[bucket]; !ok {
			cfgs[bucket] = make(map[string]*pb.Builder)
		}
		if _, ok := cfgs[bucket][req.Builder.Builder]; ok {
			continue
		}
		b := &model.Builder{
			Parent: model.BucketKey(ctx, req.Builder.Project, req.Builder.Bucket),
			ID:     req.Builder.Builder,
		}
		cfgs[bucket][req.Builder.Builder] = &b.Config
		bldrs = append(bldrs, b)
	}
	if err := datastore.Get(ctx, bldrs); err != nil {
		// TODO(crbug/1042991): Return InvalidArgument if the error is "not found".
		return nil, err
	}
	return cfgs, nil
}

// generateBuildNumbers mutates the given builds, setting build numbers and
// build address tags.
func generateBuildNumbers(ctx context.Context, builds []*model.Build) error {
	seq := make(map[string][]*model.Build)
	for _, b := range builds {
		name := protoutil.FormatBuilderID(b.Proto.Builder)
		seq[name] = append(seq[name], b)
	}
	return parallel.WorkPool(64, func(work chan<- func() error) {
		for name, blds := range seq {
			name := name
			blds := blds
			work <- func() error {
				n, err := model.GenerateSequenceNumbers(ctx, name, len(blds))
				if err != nil {
					return err
				}
				for i, b := range blds {
					b.Proto.Number = n + int32(i)
					addr := fmt.Sprintf("build_address:luci.%s.%s/%s/%d", b.Proto.Builder.Project, b.Proto.Builder.Bucket, b.Proto.Builder.Builder, b.Proto.Number)
					b.Tags = append(b.Tags, addr)
					sort.Strings(b.Tags)
				}
				return nil
			}
		}
	})
}

// setDimensions computes the dimensions from the given request and builder
// config, setting them in the model. Mutates the given *model.Build.
// build.Proto.Infra.Swarming must be set (see setInfra).
func setDimensions(req *pb.ScheduleBuildRequest, cfg *pb.Builder, build *model.Build) {
	// Requested dimensions override dimensions specified in the builder config by wiping out all
	// same-key dimensions (regardless of expiration time) in the builder config.
	//
	// For example if:
	// Request contains: ("key", "value 1", 60), ("key", "value 2", 120)
	// Config contains: ("key", "value 3", 180), ("key", "value 2", 240)
	//
	// Then the result is:
	// ("key", "value 1", 60), ("key", "value 2", 120)
	// Even though the expiration times didn't conflict and theoretically could have been merged.

	// If the config contains any reference to the builder dimension, ignore its auto builder dimension setting.
	seenBuilder := false

	// key -> slice of dimensions (key, value, expiration) with matching keys.
	dims := make(map[string][]*pb.RequestedDimension)

	// cfg.Dimensions is a slice of strings. Each string has already been validated to match either
	// <key>:<value> or <exp>:<key>:<value>, where <exp> is an int64 expiration time, <key> is a
	// non-empty string which can't be parsed as int64, and <value> is a string which may be empty.
	// <key>:<value> is shorthand for 0:<key>:<value>. An empty <value> means the dimension should be excluded.
	// TODO(crbug/1042991): Deduplicate dimension parsing logic with config validation for dimensions.
	for _, d := range cfg.GetDimensions() {
		// Split at the first colon and check if it's an int64 or not.
		// If k is an int64, v is of the form <key>:<value>. Otherwise k is the <key> and v is the <value>.
		k, v := strpair.Parse(d)
		exp, err := strconv.ParseInt(k, 10, 64)
		if err == nil {
			// k was an int64, so v is in <key>:<value> form.
			k, v = strpair.Parse(v)
		} else {
			exp = 0
			// k was the <key> and v was the <value>.
		}
		if k == "builder" {
			seenBuilder = true
		}
		if v == "" {
			// Omit empty <value>.
			continue
		}
		dim := &pb.RequestedDimension{
			Key:   k,
			Value: v,
		}
		if exp > 0 {
			dim.Expiration = &durationpb.Duration{
				Seconds: exp,
			}
		}
		dims[k] = append(dims[k], dim)
	}

	if cfg.GetAutoBuilderDimension() == pb.Toggle_YES && !seenBuilder {
		dims["builder"] = []*pb.RequestedDimension{
			{
				Key:   "builder",
				Value: cfg.Name,
			},
		}
	}

	// key -> slice of dimensions (key, value, expiration) with matching keys.
	reqDims := make(map[string][]*pb.RequestedDimension, len(cfg.GetDimensions()))
	for _, d := range req.GetDimensions() {
		reqDims[d.Key] = append(reqDims[d.Key], d)
	}
	for k, d := range reqDims {
		dims[k] = d
	}

	taskDims := make([]*pb.RequestedDimension, 0, len(reqDims))
	for _, d := range dims {
		taskDims = append(taskDims, d...)
	}
	sort.Slice(taskDims, func(i, j int) bool {
		if taskDims[i].Key == taskDims[j].Key {
			return taskDims[i].Expiration.GetSeconds() < taskDims[j].Expiration.GetSeconds()
		}
		return taskDims[i].Key < taskDims[j].Key
	})
	build.Proto.Infra.Swarming.TaskDimensions = taskDims
}

// setExecutable computes the executable from the given request and builder
// config, setting it in the model. Mutates the given *model.Build.
// build.Experiments must be set (see setExperiments).
func setExecutable(req *pb.ScheduleBuildRequest, cfg *pb.Builder, build *model.Build) {
	build.Proto.Exe = cfg.GetExe()
	if build.Proto.Exe == nil {
		build.Proto.Exe = &pb.Executable{}
	}

	if cfg.GetRecipe() != nil {
		build.Proto.Exe.CipdPackage = cfg.Recipe.CipdPackage
		build.Proto.Exe.CipdVersion = cfg.Recipe.CipdVersion
		if build.Proto.Exe.CipdVersion == "" {
			build.Proto.Exe.CipdVersion = "refs/heads/master"
		}
	}

	if len(build.Proto.Exe.Cmd) == 0 {
		build.Proto.Exe.Cmd = []string{"recipes"}
		if build.ExperimentStatus(bb.ExperimentBBAgent) == pb.Trinary_YES {
			build.Proto.Exe.Cmd = []string{"luciexe"}
		}
	}

	// The request has highest precedence, but may only override CIPD version.
	if req.GetExe().GetCipdVersion() != "" {
		build.Proto.Exe.CipdVersion = req.Exe.CipdVersion
	}
}

// setExperiments computes the experiments from the given request and builder
// config, setting them in the model and proto. Mutates the given *model.Build.
// build.Proto.Input must not be nil.
func setExperiments(ctx context.Context, req *pb.ScheduleBuildRequest, cfg *pb.Builder, build *model.Build) {
	// Experiment -> enabled.
	exps := make(map[string]bool, len(bb.WellKnownExperiments)+len(req.GetExperiments()))

	// Experiment values in the experiments field of the request have highest precedence,
	// followed by legacy fields in the request, followed by values in the builder config.

	// All well-known experiments need to show up in the model.Build, so
	// initialize them all to false.
	for wke := range bb.WellKnownExperiments {
		exps[wke] = false
	}

	// Override from builder config.
	for exp, pct := range cfg.GetExperiments() {
		exps[exp] = mathrand.Int31n(ctx, 100) < pct
	}

	// note that legacy Canary/Experimental req fields were incorporated via
	// normalizeSchedule already.

	// Finally override with explicitly requested experiments.
	for exp, en := range req.GetExperiments() {
		exps[exp] = en
	}

	// The model stores all experiment values, while the proto only contains enabled experiments.
	for exp, en := range exps {
		if en {
			build.Experiments = append(build.Experiments, fmt.Sprintf("+%s", exp))
			build.Proto.Input.Experiments = append(build.Proto.Input.Experiments, exp)
		} else {
			// The "-luci.non_production" case is specifically exempt from the
			// datastore index. See `model.Build.Experiments`.
			if exp == bb.ExperimentNonProduction {
				continue
			}
			build.Experiments = append(build.Experiments, fmt.Sprintf("-%s", exp))
		}
	}

	// For now, continue to set legacy field values from the experiments.
	if en := exps[bb.ExperimentBBCanarySoftware]; en {
		build.Canary = true
		build.Proto.Canary = true
	}
	if en := exps[bb.ExperimentNonProduction]; en {
		build.Experimental = true
		build.Proto.Input.Experimental = true
	}
	sort.Strings(build.Proto.Input.Experiments)
	sort.Strings(build.Experiments)
}

// defBuilderCacheTimeout is the default value for WaitForWarmCache in the
// pb.BuildInfra_Swarming_CacheEntry whose Name is "builder" (see setInfra).
var defBuilderCacheTimeout = durationpb.New(4 * time.Minute)

// configuredCacheToTaskCache returns the equivalent
// *pb.BuildInfra_Swarming_CacheEntry for the given *pb.Builder_CacheEntry.
func configuredCacheToTaskCache(builderCache *pb.Builder_CacheEntry) *pb.BuildInfra_Swarming_CacheEntry {
	taskCache := &pb.BuildInfra_Swarming_CacheEntry{
		EnvVar: builderCache.EnvVar,
		Name:   builderCache.Name,
		Path:   builderCache.Path,
	}
	if taskCache.Name == "" {
		taskCache.Name = taskCache.Path
	}
	if builderCache.WaitForWarmCacheSecs > 0 {
		taskCache.WaitForWarmCache = &durationpb.Duration{
			Seconds: int64(builderCache.WaitForWarmCacheSecs),
		}
	}
	return taskCache
}

// setInfra computes the infra values from the given request and builder config,
// setting them in the model. Mutates the given *model.Build. build.Experiments
// and build.Proto.Builder must be set (see setExperiments and scheduleBuilds).
func setInfra(appID, logdogHost, rdbHost string, req *pb.ScheduleBuildRequest, cfg *pb.Builder, build *model.Build, globalCaches []*pb.Builder_CacheEntry) {
	build.Proto.Infra = &pb.BuildInfra{
		Buildbucket: &pb.BuildInfra_Buildbucket{
			RequestedDimensions: req.GetDimensions(),
			RequestedProperties: req.GetProperties(),
		},
		Logdog: &pb.BuildInfra_LogDog{
			Hostname: logdogHost,
			Prefix:   fmt.Sprintf("buildbucket/%s/%d", appID, build.Proto.Id),
			Project:  build.Proto.Builder.GetProject(),
		},
		Resultdb: &pb.BuildInfra_ResultDB{
			Hostname: rdbHost,
		},
		Swarming: &pb.BuildInfra_Swarming{
			Hostname:           cfg.GetSwarmingHost(),
			ParentRunId:        req.GetSwarming().GetParentRunId(),
			Priority:           int32(cfg.GetPriority()),
			TaskServiceAccount: cfg.GetServiceAccount(),
		},
	}
	if build.Proto.Infra.Swarming.Priority == 0 {
		build.Proto.Infra.Swarming.Priority = 30
	}

	if cfg.GetRecipe() != nil {
		build.Proto.Infra.Recipe = &pb.BuildInfra_Recipe{
			CipdPackage: cfg.Recipe.CipdPackage,
			Name:        cfg.Recipe.Name,
		}
	}

	taskCaches := make([]*pb.BuildInfra_Swarming_CacheEntry, len(cfg.GetCaches()), len(cfg.GetCaches())+len(globalCaches))
	names := stringset.New(len(cfg.GetCaches()))
	paths := stringset.New(len(cfg.GetCaches()))
	for i, c := range cfg.GetCaches() {
		taskCaches[i] = configuredCacheToTaskCache(c)
		names.Add(taskCaches[i].Name)
		paths.Add(taskCaches[i].Path)
	}

	// Requested caches have precedence over global caches.
	// Apply global caches whose names and paths weren't overriden.
	for _, c := range globalCaches {
		if !names.Has(c.Name) && !paths.Has(c.Path) {
			taskCaches = append(taskCaches, configuredCacheToTaskCache(c))
		}
	}

	if !paths.Has("builder") {
		taskCaches = append(taskCaches, &pb.BuildInfra_Swarming_CacheEntry{
			Name:             fmt.Sprintf("builder_%x_v2", sha256.Sum256([]byte(protoutil.FormatBuilderID(build.Proto.Builder)))),
			Path:             "builder",
			WaitForWarmCache: defBuilderCacheTimeout,
		})
	}

	sort.Slice(taskCaches, func(i, j int) bool {
		return taskCaches[i].Path < taskCaches[j].Path
	})
	build.Proto.Infra.Swarming.Caches = taskCaches

	switch {
	case req.GetPriority() > 0:
		build.Proto.Infra.Swarming.Priority = req.Priority
	case build.ExperimentStatus(bb.ExperimentBBAgent) == pb.Trinary_YES:
		build.Proto.Infra.Swarming.Priority = 255
	}
}

// setInput computes the input values from the given request and builder config,
// setting them in the model. Mutates the given *model.Build. May panic if the
// builder config is invalid.
func setInput(req *pb.ScheduleBuildRequest, cfg *pb.Builder, build *model.Build) {
	build.Proto.Input = &pb.Build_Input{
		Properties: &structpb.Struct{},
	}

	if cfg.GetRecipe() != nil {
		// TODO(crbug/1042991): Deduplicate property parsing logic with config validation for properties.
		build.Proto.Input.Properties.Fields = make(map[string]*structpb.Value, len(cfg.Recipe.Properties)+len(cfg.Recipe.PropertiesJ)+1)
		for _, prop := range cfg.Recipe.Properties {
			k, v := strpair.Parse(prop)
			build.Proto.Input.Properties.Fields[k] = &structpb.Value{
				Kind: &structpb.Value_StringValue{
					StringValue: v,
				},
			}
		}

		// Values are JSON-encoded strings which need to be unmarshalled to structpb.Struct.
		// jsonpb unmarshals dicts to structpb.Struct, but cannot unmarshal directly to
		// structpb.Value, so create a dummy dict in order to get the structpb.Value.
		// TODO(crbug/1042991): Deduplicate legacy property parsing with buildbucket/cli.
		for _, prop := range cfg.Recipe.PropertiesJ {
			k, v := strpair.Parse(prop)
			s := &structpb.Struct{}
			v = fmt.Sprintf("{\"%s\": %s}", k, v)
			if err := jsonpb.UnmarshalString(v, s); err != nil {
				// Builder config should have been validated already.
				panic(errors.Annotate(err, "error parsing %q", v).Err())
			}
			build.Proto.Input.Properties.Fields[k] = s.Fields[k]
		}
		build.Proto.Input.Properties.Fields["recipe"] = &structpb.Value{
			Kind: &structpb.Value_StringValue{
				StringValue: cfg.Recipe.Name,
			},
		}
	} else if cfg.GetProperties() != "" {
		if err := jsonpb.UnmarshalString(cfg.Properties, build.Proto.Input.Properties); err != nil {
			// Builder config should have been validated already.
			panic(errors.Annotate(err, "error unmarshaling builder properties for %q", cfg.Name).Err())
		}
	}

	if build.Proto.Input.Properties.Fields == nil {
		build.Proto.Input.Properties.Fields = make(map[string]*structpb.Value, len(req.GetProperties().GetFields()))
	}
	for k, v := range req.GetProperties().GetFields() {
		build.Proto.Input.Properties.Fields[k] = v
	}

	build.Proto.Input.GitilesCommit = req.GetGitilesCommit()
	build.Proto.Input.GerritChanges = req.GetGerritChanges()
}

// setTags computes the tags from the given request, setting them in the model.
// Mutates the given *model.Build.
func setTags(req *pb.ScheduleBuildRequest, build *model.Build) {
	tags := protoutil.StringPairMap(req.GetTags())
	if req.GetBuilder() != nil {
		tags.Add("builder", req.Builder.Builder)
	}
	if req.GetGitilesCommit() != nil {
		tags.Add("buildset", protoutil.GitilesBuildSet(req.GitilesCommit))
		tags.Add("gitiles_ref", req.GitilesCommit.Ref)
	}
	for _, ch := range req.GetGerritChanges() {
		tags.Add("buildset", protoutil.GerritBuildSet(ch))
	}
	build.Tags = tags.Format()
}

var (
	// defExecutionTimeout is the default value for pb.Build.ExecutionTimeout.
	// See setTimeouts.
	defExecutionTimeout = durationpb.New(3 * time.Hour)

	// defExecutionTimeout is the default value for pb.Build.GracePeriod.
	// See setTimeouts.
	defGracePeriod = durationpb.New(30 * time.Second)

	// defExecutionTimeout is the default value for pb.Build.SchedulingTimeout.
	// See setTimeouts.
	defSchedulingTimeout = durationpb.New(6 * time.Hour)
)

// setTimeouts computes the timeouts from the given request and builder config,
// setting them in the model. Mutates the given *model.Build.
func setTimeouts(req *pb.ScheduleBuildRequest, cfg *pb.Builder, build *model.Build) {
	// Timeouts in the request have highest precedence, followed by
	// values in the builder config, followed by default values.
	switch {
	case req.GetExecutionTimeout() != nil:
		build.Proto.ExecutionTimeout = req.ExecutionTimeout
	case cfg.GetExecutionTimeoutSecs() > 0:
		build.Proto.ExecutionTimeout = &durationpb.Duration{
			Seconds: int64(cfg.ExecutionTimeoutSecs),
		}
	default:
		build.Proto.ExecutionTimeout = defExecutionTimeout
	}

	switch {
	case req.GetGracePeriod() != nil:
		build.Proto.GracePeriod = req.GracePeriod
	case cfg.GetGracePeriod() != nil:
		build.Proto.GracePeriod = cfg.GracePeriod
	default:
		build.Proto.GracePeriod = defGracePeriod
	}

	switch {
	case req.GetSchedulingTimeout() != nil:
		build.Proto.SchedulingTimeout = req.SchedulingTimeout
	case cfg.GetExpirationSecs() > 0:
		build.Proto.SchedulingTimeout = &durationpb.Duration{
			Seconds: int64(cfg.ExpirationSecs),
		}
	default:
		build.Proto.SchedulingTimeout = defSchedulingTimeout
	}
}

// scheduleBuilds handles requests to schedule builds. Requests must be
// validated and authorized.
func scheduleBuilds(ctx context.Context, reqs ...*pb.ScheduleBuildRequest) ([]*model.Build, error) {
	now := clock.Now(ctx).UTC()
	user := auth.CurrentIdentity(ctx)
	cfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "error fetching service config").Err()
	}
	caches := cfg.GetSwarming().GetGlobalCaches()
	logdogHost := cfg.GetLogdog().GetHostname()
	rdbHost := cfg.GetResultdb().GetHostname()

	// Bucket -> Builder -> *pb.Builder.
	cfgs, err := fetchBuilderConfigs(ctx, reqs)
	if err != nil {
		return nil, errors.Annotate(err, "error fetching builders").Err()
	}

	blds := make([]*model.Build, len(reqs))
	nums := make([]*model.Build, 0, len(reqs))
	ids := buildid.NewBuildIDs(ctx, now, len(reqs))
	for i := range blds {
		bucket := fmt.Sprintf("%s/%s", reqs[i].Builder.Project, reqs[i].Builder.Bucket)
		cfg := cfgs[bucket][reqs[i].Builder.Builder]

		// TODO(crbug/1042991): Parallelize build creation from requests if necessary.
		blds[i] = &model.Build{
			ID:         ids[i],
			CreatedBy:  user,
			CreateTime: now,
			Proto: pb.Build{
				Builder:         reqs[i].Builder,
				CreatedBy:       string(user),
				CreateTime:      timestamppb.New(now),
				Id:              ids[i],
				Status:          pb.Status_SCHEDULED,
				WaitForCapacity: cfg.GetWaitForCapacity() == pb.Trinary_YES,
			},
		}

		blds[i].Proto.Critical = cfg.GetCritical()
		if reqs[i].Critical != pb.Trinary_UNSET {
			blds[i].Proto.Critical = reqs[i].Critical
		}

		setInput(reqs[i], cfg, blds[i])
		setExperiments(ctx, reqs[i], cfg, blds[i])                                    // Requires setInput.
		setExecutable(reqs[i], cfg, blds[i])                                          // Requires setExperiments.
		setInfra(info.AppID(ctx), logdogHost, rdbHost, reqs[i], cfg, blds[i], caches) // Requires setExperiments.
		setDimensions(reqs[i], cfg, blds[i])                                          // Requires setInfra.
		setTags(reqs[i], blds[i])
		setTimeouts(reqs[i], cfg, blds[i])

		exp := make(map[int64]struct{})
		for _, d := range blds[i].Proto.Infra.GetSwarming().GetTaskDimensions() {
			exp[d.Expiration.Seconds] = struct{}{}
		}
		if len(exp) > 6 {
			return nil, appstatus.BadRequest(errors.Reason("build %d contains more than 6 unique expirations", i).Err())
		}

		if cfg.GetBuildNumbers() == pb.Toggle_YES {
			nums = append(nums, blds[i])
		}
	}
	if err := generateBuildNumbers(ctx, nums); err != nil {
		return nil, errors.Annotate(err, "error generating build numbers").Err()
	}

	err = parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() error { return model.UpdateBuilderStat(ctx, blds, now) }
		if rdbHost != "" {
			work <- func() error { return resultdb.CreateInvocations(ctx, blds, cfgs, rdbHost) }
		}
		work <- func() error { return search.UpdateTagIndex(ctx, blds) }
	})
	if err != nil {
		return nil, err
	}

	// This parallel work isn't combined with the above parallel work to ensure build entities and Swarming
	// task creation tasks are only created if everything else has succeeded (since everything can't be done
	// in one transaction).
	err = parallel.WorkPool(64, func(work chan<- func() error) {
		for i, b := range blds {
			b := b
			// blds and reqs slices map 1:1.
			reqID := reqs[i].RequestId
			work <- func() error {
				toPut := []interface{}{
					b,
					&model.BuildInfra{
						Build: datastore.KeyForObj(ctx, b),
						Proto: model.DSBuildInfra{
							BuildInfra: *b.Proto.Infra,
						},
					},
					&model.BuildInputProperties{
						Build: datastore.KeyForObj(ctx, b),
						Proto: model.DSStruct{
							Struct: *b.Proto.Input.Properties,
						},
					},
				}
				r := model.NewRequestID(ctx, b.ID, now, reqID)

				// Write the entities and trigger a task queue task to create the Swarming task.
				err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					// Deduplicate by request ID.
					if reqID != "" {
						switch err := datastore.Get(ctx, r); {
						case err == datastore.ErrNoSuchEntity:
							toPut = append(toPut, r)
						case err != nil:
							return errors.Annotate(err, "failed to deduplicate request ID: %d", b.ID).Err()
						default:
							b.ID = r.BuildID
							if err := datastore.Get(ctx, b); err != nil {
								return errors.Annotate(err, "failed to fetch deduplicated build: %d", b.ID).Err()
							}
							return nil
						}
					}

					// Request was not a duplicate.
					switch err := datastore.Get(ctx, &model.Build{ID: b.ID}); {
					case err == nil:
						return appstatus.Errorf(codes.AlreadyExists, "build already exists: %d", b.ID)
					case err != datastore.ErrNoSuchEntity:
						return errors.Annotate(err, "failed to fetch build: %d", b.ID).Err()
					}

					if err := datastore.Put(ctx, toPut...); err != nil {
						return errors.Annotate(err, "failed to store build: %d", b.ID).Err()
					}

					if err := tasks.CreateSwarmingTask(ctx, &taskdefs.CreateSwarmingTask{
						BuildId: b.ID,
					}); err != nil {
						return errors.Annotate(err, "failed to enqueue swarming task creation task: %d", b.ID).Err()
					}
					return nil
				}, nil)
				if err != nil {
					return err
				}

				// TODO(crbug/1042991): Update build creation metric.
				return nil
			}
		}
	})
	if err != nil {
		return nil, err
	}

	return blds, nil
}

// normalizeSchedule converts deprecated fields to non-deprecated ones.
//
// In particular, this currently converts the Canary and Experimental fields to
// the non-deprecated Experiments field.
func normalizeSchedule(req *pb.ScheduleBuildRequest) {
	if req.Experiments == nil {
		req.Experiments = map[string]bool{}
	}

	if _, has := req.Experiments[bb.ExperimentBBCanarySoftware]; !has {
		if req.Canary == pb.Trinary_YES {
			req.Experiments[bb.ExperimentBBCanarySoftware] = true
		} else if req.Canary == pb.Trinary_NO {
			req.Experiments[bb.ExperimentBBCanarySoftware] = false
		}
		req.Canary = pb.Trinary_UNSET
	}

	if _, has := req.Experiments[bb.ExperimentNonProduction]; !has {
		if req.Experimental == pb.Trinary_YES {
			req.Experiments[bb.ExperimentNonProduction] = true
		} else if req.Experimental == pb.Trinary_NO {
			req.Experiments[bb.ExperimentNonProduction] = false
		}
		req.Experimental = pb.Trinary_UNSET
	}
}

// ScheduleBuild handles a request to schedule a build. Implements pb.BuildsServer.
func (*Builds) ScheduleBuild(ctx context.Context, req *pb.ScheduleBuildRequest) (*pb.Build, error) {
	var err error
	if err = validateSchedule(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	normalizeSchedule(req)

	m, err := getFieldMask(req.Fields)
	if err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "fields").Err())
	}

	if req, err = scheduleRequestFromTemplate(ctx, req); err != nil {
		return nil, err
	}
	if err = perm.HasInBucket(ctx, perm.BuildsAdd, req.Builder.Project, req.Builder.Bucket); err != nil {
		return nil, err
	}

	blds, err := scheduleBuilds(ctx, req)
	if err != nil {
		return nil, err
	}
	return blds[0].ToProto(ctx, m)
}
