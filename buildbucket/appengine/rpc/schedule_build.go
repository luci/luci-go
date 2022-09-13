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

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	bb "go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildid"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/internal/resultdb"
	"go.chromium.org/luci/buildbucket/appengine/internal/search"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}

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
	case teeErr(validateDimension(dim), &err) != nil:
		return err
	case dim.Key == "caches":
		return errors.Annotate(errors.Reason("caches may only be specified in builder configs (cr-buildbucket.cfg)").Err(), "key").Err()
	case dim.Key == "pool":
		return errors.Annotate(errors.Reason("pool may only be specified in builder configs (cr-buildbucket.cfg)").Err(), "key").Err()
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

// prohibitedProperties is used to prohibit properties from being set (see
// validateProperties). Contains slices of path components forming a prohibited
// path. For example, to prohibit a property "a.b", add an element ["a", "b"].
var prohibitedProperties = [][]string{
	{"$recipe_engine/buildbucket"},
	{"$recipe_engine/runtime", "is_experimental"},
	{"$recipe_engine/runtime", "is_luci"},
	{"branch"},
	{"buildbucket"},
	{"buildername"},
	{"repository"},
}

// structContains returns whether the struct contains a value at the given path.
// An empty slice of path components always returns true.
func structContains(s *structpb.Struct, path []string) bool {
	for _, p := range path {
		v, ok := s.GetFields()[p]
		if !ok {
			return false
		}
		s = v.GetStructValue()
	}
	return true
}

// validateProperties validates the given properties.
func validateProperties(p *structpb.Struct) error {
	for _, path := range prohibitedProperties {
		if structContains(p, path) {
			return errors.Reason("%q must not be specified", strings.Join(path, ".")).Err()
		}
	}
	return nil
}

// validateParent validates the given parent build.
func validateParent(ctx context.Context) (*model.Build, error) {
	_, pBld, err := validateBuildToken(ctx, 0, false)
	switch {
	case err != nil:
		return nil, err
	case pBld == nil:
		return nil, nil
	case protoutil.IsEnded(pBld.Proto.Status):
		return nil, errors.Reason("%d has ended, cannot add child to it", pBld.ID).Err()
	default:
		return pBld, nil
	}
}

// validateSchedule validates the given request.
func validateSchedule(req *pb.ScheduleBuildRequest, wellKnownExperiments stringset.Set, parent *model.Build) error {
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
	case req.Properties != nil && teeErr(validateProperties(req.Properties), &err) != nil:
		return errors.Annotate(err, "properties").Err()
	case parent == nil && req.CanOutliveParent != pb.Trinary_UNSET:
		return errors.Reason("can_outlive_parent is specified without parent build token").Err()
	case teeErr(validateTags(req.Tags, TagNew), &err) != nil:
		return errors.Annotate(err, "tags").Err()
	}

	for expName := range req.Experiments {
		if err := config.ValidateExperimentName(expName, wellKnownExperiments); err != nil {
			return errors.Annotate(err, "experiment %q", expName).Err()
		}
	}

	// TODO(crbug/1042991): Validate Properties.
	return nil
}

// templateBuildMask enumerates properties to read from template builds. See
// scheduleRequestFromTemplate.
var templateBuildMask = model.HardcodedBuildMask(
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
	if err := perm.HasInBuilder(ctx, bbperms.BuildsGet, bld.Proto.Builder); err != nil {
		return nil, err
	}

	b := bld.ToSimpleBuildProto(ctx)
	if err := model.LoadBuildDetails(ctx, templateBuildMask, nil, b); err != nil {
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
// requests in a map of Bucket ID -> Builder name -> *pb.BuilderConfig.
//
// A single returned error means a global error which applies to every request.
// Otherwise, it would be a MultiError where len(MultiError) equals to len(reqs).
func fetchBuilderConfigs(ctx context.Context, reqs []*pb.ScheduleBuildRequest) (map[string]map[string]*pb.BuilderConfig, error) {
	merr := make(errors.MultiError, len(reqs))
	var bcks []*model.Bucket

	// bckCfgs and bldrCfgs use a double-pointer because GetIgnoreMissing will
	// indirectly overwrite the pointer in the model struct when loading from the
	// datastore (so, populating Proto and Config fields and using those values
	// won't help).
	bckCfgs := map[string]**pb.Bucket{} // Bucket ID -> **pb.Bucket
	var bldrs []*model.Builder
	bldrCfgs := map[string]map[string]**pb.BuilderConfig{} // Bucket ID -> Builder name -> **pb.BuilderConfig

	idxMap := map[string]map[string][]int{} // Bucket ID -> Builder name -> a list of index
	for i, req := range reqs {
		bucket := protoutil.FormatBucketID(req.Builder.Project, req.Builder.Bucket)
		if _, ok := bldrCfgs[bucket]; !ok {
			bldrCfgs[bucket] = make(map[string]**pb.BuilderConfig)
			idxMap[bucket] = map[string][]int{}
		}
		if _, ok := bldrCfgs[bucket][req.Builder.Builder]; ok {
			idxMap[bucket][req.Builder.Builder] = append(idxMap[bucket][req.Builder.Builder], i)
			continue
		}
		if _, ok := bckCfgs[bucket]; !ok {
			b := &model.Bucket{
				Parent: model.ProjectKey(ctx, req.Builder.Project),
				ID:     req.Builder.Bucket,
			}
			bckCfgs[bucket] = &b.Proto
			bcks = append(bcks, b)
		}
		b := &model.Builder{
			Parent: model.BucketKey(ctx, req.Builder.Project, req.Builder.Bucket),
			ID:     req.Builder.Builder,
		}
		bldrCfgs[bucket][req.Builder.Builder] = &b.Config
		bldrs = append(bldrs, b)
		idxMap[bucket][req.Builder.Builder] = append(idxMap[bucket][req.Builder.Builder], i)
	}

	// Note; this will fill in bckCfgs and bldrCfgs.
	if err := model.GetIgnoreMissing(ctx, bcks, bldrs); err != nil {
		return nil, errors.Annotate(err, "failed to fetch entities").Err()
	}

	// Check buckets to see if they support dynamically scheduling builds for builders which are not pre-defined.
	for _, b := range bcks {
		if b.Proto.GetName() == "" {
			bucket := protoutil.FormatBucketID(b.Parent.StringID(), b.ID)
			for _, bldrIdx := range idxMap[bucket] {
				for idx := range bldrIdx {
					merr[idx] = appstatus.Errorf(codes.NotFound, "bucket not found: %q", b.ID)
				}
			}
		}
	}
	for _, b := range bldrs {
		// Since b.Config isn't a pointer type it will always be non-nil. However, since name is validated
		// as required, it can be used as a proxy for determining whether the builder config was found or
		// not. If it's unspecified, the builder wasn't found. Builds for builders which aren't pre-configured
		// can only be scheduled in buckets which support dynamic builders.
		if b.Config.GetName() == "" {
			bucket := protoutil.FormatBucketID(b.Parent.Parent().StringID(), b.Parent.StringID())
			// TODO(crbug/1042991): Check if bucket is explicitly configured for dynamic builders.
			// Currently buckets do not require pre-defined builders iff they have no Swarming config.
			if (*bckCfgs[bucket]).GetSwarming() == nil {
				delete(bldrCfgs[bucket], b.ID)
				continue
			}
			for _, idx := range idxMap[bucket][b.ID] {
				merr[idx] = appstatus.Errorf(codes.NotFound, "builder not found: %q", b.ID)
			}
		}
	}

	// deref all the pointers.
	ret := make(map[string]map[string]*pb.BuilderConfig, len(bldrCfgs))
	for bucket, builders := range bldrCfgs {
		m := make(map[string]*pb.BuilderConfig, len(builders))
		for builderName, builder := range builders {
			m[builderName] = *builder
		}
		ret[bucket] = m
	}

	// doesn't contain any errors.
	if merr.First() == nil {
		return ret, nil
	}
	return ret, merr.AsError()
}

// generateBuildNumbers mutates the given builds, setting build numbers and
// build address tags.
//
// It would return a MultiError (if any) where len(MultiError) equals to len(reqs).
func generateBuildNumbers(ctx context.Context, builds []*model.Build) error {
	merr := make(errors.MultiError, len(builds))
	seq := make(map[string][]*model.Build)
	idxMap := make(map[string][]int) // BuilderID -> a list of index
	for i, b := range builds {
		name := protoutil.FormatBuilderID(b.Proto.Builder)
		seq[name] = append(seq[name], b)
		idxMap[name] = append(idxMap[name], i)
	}
	_ = parallel.WorkPool(min(64, len(builds)), func(work chan<- func() error) {
		for name, blds := range seq {
			name := name
			blds := blds
			work <- func() error {
				n, err := model.GenerateSequenceNumbers(ctx, name, len(blds))
				if err != nil {
					for _, idx := range idxMap[name] {
						merr[idx] = err
					}
					return nil
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

	if merr.First() == nil {
		return nil
	}
	return merr.AsError()
}

// builderMatches returns whether or not the given builder matches the given
// predicate. A match occurs if any regex matches and none of the exclusions
// rule the builder out. If there are no regexes, a match always occurs unless
// an exclusion rules the builder out. The predicate must be validated.
func builderMatches(builder string, pred *pb.BuilderPredicate) bool {
	// TODO(crbug/1042991): Cache compiled regexes (possibly in internal/config package).
	for _, r := range pred.GetRegexExclude() {
		if m, err := regexp.MatchString(fmt.Sprintf("^%s$", r), builder); err == nil && m {
			return false
		}
	}

	if len(pred.GetRegex()) == 0 {
		return true
	}
	for _, r := range pred.Regex {
		if m, err := regexp.MatchString(fmt.Sprintf("^%s$", r), builder); err == nil && m {
			return true
		}
	}
	return false
}

// experimentsMatch returns whether or not the given experimentSet matches the
// given includeOnExperiment or omitOnExperiment.
func experimentsMatch(experimentSet stringset.Set, includeOnExperiment, omitOnExperiment []string) bool {
	for _, e := range omitOnExperiment {
		if experimentSet.Has(e) {
			return false
		}
	}

	if len(includeOnExperiment) > 0 {
		include := false

		for _, e := range includeOnExperiment {
			if experimentSet.Has(e) {
				include = true
				break
			}
		}

		if !include {
			return false
		}

	}

	return true
}

// setDimensions computes the dimensions from the given request and builder
// config, setting them in the proto. Mutates the given *pb.Build.
// build.Infra.Swarming must be set (see setInfra).
func setDimensions(req *pb.ScheduleBuildRequest, cfg *pb.BuilderConfig, build *pb.Build) {
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
	build.Infra.Swarming.TaskDimensions = taskDims
}

// setExecutable computes the executable from the given request and builder
// config, setting it in the proto. Mutates the given *pb.Build.
func setExecutable(req *pb.ScheduleBuildRequest, cfg *pb.BuilderConfig, build *pb.Build) {
	build.Exe = cfg.GetExe()
	if build.Exe == nil {
		build.Exe = &pb.Executable{}
	}

	if cfg.GetRecipe() != nil {
		build.Exe.CipdPackage = cfg.Recipe.CipdPackage
		build.Exe.CipdVersion = cfg.Recipe.CipdVersion
		if build.Exe.CipdVersion == "" {
			build.Exe.CipdVersion = "refs/heads/master"
		}
	}

	// The request has highest precedence, but may only override CIPD version.
	if req.GetExe().GetCipdVersion() != "" {
		build.Exe.CipdVersion = req.Exe.CipdVersion
	}
}

// activeGlobalExpsForBuilder filters the global experiments, returning the
// experiments that apply to this builder, as well as experiments which are
// ignored.
//
// If experiments are known, but don't apply to the builder, then they're
// returned in a form where their DefaultValue and MinimumValue are 0.
//
// Ignored experiments are global experiments which no longer do anything,
// and should be removed from the build (even if specified via
// ScheduleBuildRequest).
func activeGlobalExpsForBuilder(build *pb.Build, globalCfg *pb.SettingsCfg) (active []*pb.ExperimentSettings_Experiment, ignored stringset.Set) {
	exps := globalCfg.GetExperiment().GetExperiments()
	if len(exps) == 0 {
		return nil, nil
	}

	active = make([]*pb.ExperimentSettings_Experiment, 0, len(exps))
	ignored = stringset.New(0)

	bid := protoutil.FormatBuilderID(build.Builder)
	for _, exp := range exps {
		if exp.Inactive {
			ignored.Add(exp.Name)
			continue
		}
		if !builderMatches(bid, exp.Builders) {
			exp = proto.Clone(exp).(*pb.ExperimentSettings_Experiment)
			exp.DefaultValue = 0
			exp.MinimumValue = 0
		}
		active = append(active, exp)
	}

	return
}

// setExperiments computes the experiments from the given request, builder and
// global config, setting them in the proto. Mutates the given *pb.Build.
// build.Infra.Swarming, build.Input and build.Exe must not be nil (see
// setInfra, setInput and setExecutable respectively). The request must not set
// legacy experiment values (see normalizeSchedule).
func setExperiments(ctx context.Context, req *pb.ScheduleBuildRequest, cfg *pb.BuilderConfig, globalCfg *pb.SettingsCfg, build *pb.Build) {
	globalExps, ignoredExps := activeGlobalExpsForBuilder(build, globalCfg)

	// Set up the dice-rolling apparatus
	exps := make(map[string]int32, len(cfg.GetExperiments())+len(globalExps))
	er := make(map[string]pb.BuildInfra_Buildbucket_ExperimentReason, len(exps))

	// 1. Populate with defaults
	for _, exp := range globalExps {
		exps[exp.Name] = exp.DefaultValue
		er[exp.Name] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_GLOBAL_DEFAULT
	}
	// 2. Overwrite with builder config
	for name, value := range cfg.GetExperiments() {
		er[name] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_BUILDER_CONFIG
		exps[name] = value
	}
	// 3. Overwrite with minimum global experiment values
	for _, exp := range globalExps {
		if exp.MinimumValue > exps[exp.Name] {
			er[exp.Name] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_GLOBAL_MINIMUM
			exps[exp.Name] = exp.MinimumValue
		}
	}
	// 4. Explicit requests have highest precedence
	for name, enabled := range req.GetExperiments() {
		er[name] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED
		if enabled {
			exps[name] = 100
		} else {
			exps[name] = 0
		}
	}
	// 5. Remove all inactive global expirements
	ignoredExps.Iter(func(expName string) bool {
		if _, ok := exps[expName]; ok {
			er[expName] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_GLOBAL_INACTIVE
			delete(exps, expName)
		}
		return true
	})

	selections := make(map[string]bool, len(exps))

	// Finally, roll the dice. We order `exps` here for test determinisim.
	expNames := make([]string, 0, len(exps))
	for exp := range exps {
		expNames = append(expNames, exp)
	}
	sort.Strings(expNames)
	for _, exp := range expNames {
		pct := exps[exp]
		switch {
		case pct >= 100:
			selections[exp] = true
		case pct <= 0:
			selections[exp] = false
		default:
			selections[exp] = mathrand.Int31n(ctx, 100) < pct
		}
	}

	// For now, continue to set legacy field values from the experiments.
	build.Canary = selections[bb.ExperimentBBCanarySoftware]
	build.Input.Experimental = selections[bb.ExperimentNonProduction]

	// Set experimental values.
	if len(build.Exe.Cmd) > 0 {
		// If the user explicitly set Exe, that counts as a builder
		// configuration.
		er[bb.ExperimentBBAgent] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_BUILDER_CONFIG

		// If they explicitly picked recipes, this experiment is false.
		// If they explicitly picked luciexe, this experiment is true
		selections[bb.ExperimentBBAgent] = build.Exe.Cmd[0] != "recipes"
	} else if selections[bb.ExperimentBBAgent] {
		// User didn't explicitly set Exe, bbagent was selected
		build.Exe.Cmd = []string{"luciexe"}
	} else {
		// User didn't explicitly set Exe, bbagent was not selected
		build.Exe.Cmd = []string{"recipes"}
	}

	// Request > experimental > proto precedence.
	if selections[bb.ExperimentNonProduction] && req.GetPriority() == 0 {
		build.Infra.Swarming.Priority = 255
	}

	for exp, en := range selections {
		if !en {
			continue
		}
		build.Input.Experiments = append(build.Input.Experiments, exp)
	}
	sort.Strings(build.Input.Experiments)

	if len(er) > 0 {
		build.Infra.Buildbucket.ExperimentReasons = er
	}

	return
}

// defBuilderCacheTimeout is the default value for WaitForWarmCache in the
// pb.BuildInfra_Swarming_CacheEntry whose Name is "builder" (see setInfra).
var defBuilderCacheTimeout = durationpb.New(4 * time.Minute)

// configuredCacheToTaskCache returns the equivalent
// *pb.BuildInfra_Swarming_CacheEntry for the given *pb.BuilderConfig_CacheEntry.
func configuredCacheToTaskCache(builderCache *pb.BuilderConfig_CacheEntry) *pb.BuildInfra_Swarming_CacheEntry {
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
// setting them in the proto. Mutates the given *pb.Build. build.Builder must be
// set. Does not set build.Infra.Buildbucket.Hostname or
// build.Infra.Logdog.Prefix, which can only be determined at creation time.
func setInfra(req *pb.ScheduleBuildRequest, cfg *pb.BuilderConfig, build *pb.Build, globalCfg *pb.SettingsCfg) {
	build.Infra = &pb.BuildInfra{
		Bbagent: &pb.BuildInfra_BBAgent{
			CacheDir:    "cache",
			PayloadPath: "kitchen-checkout",
		},
		Buildbucket: &pb.BuildInfra_Buildbucket{
			RequestedDimensions:    req.GetDimensions(),
			RequestedProperties:    req.GetProperties(),
			KnownPublicGerritHosts: globalCfg.GetKnownPublicGerritHosts(),
		},
		Logdog: &pb.BuildInfra_LogDog{
			Hostname: globalCfg.GetLogdog().GetHostname(),
			Project:  build.Builder.GetProject(),
		},
		Resultdb: &pb.BuildInfra_ResultDB{
			Hostname: globalCfg.GetResultdb().GetHostname(),
		},
		Swarming: &pb.BuildInfra_Swarming{
			Hostname:           cfg.GetSwarmingHost(),
			ParentRunId:        req.GetSwarming().GetParentRunId(),
			Priority:           int32(cfg.GetPriority()),
			TaskServiceAccount: cfg.GetServiceAccount(),
		},
	}
	if build.Infra.Swarming.Priority == 0 {
		build.Infra.Swarming.Priority = 30
	}

	if cfg.GetRecipe() != nil {
		build.Infra.Recipe = &pb.BuildInfra_Recipe{
			CipdPackage: cfg.Recipe.CipdPackage,
			Name:        cfg.Recipe.Name,
		}
	}

	globalCaches := globalCfg.GetSwarming().GetGlobalCaches()
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
			Name:             fmt.Sprintf("builder_%x_v2", sha256.Sum256([]byte(protoutil.FormatBuilderID(build.Builder)))),
			Path:             "builder",
			WaitForWarmCache: defBuilderCacheTimeout,
		})
	}

	sort.Slice(taskCaches, func(i, j int) bool {
		return taskCaches[i].Path < taskCaches[j].Path
	})
	build.Infra.Swarming.Caches = taskCaches

	if req.GetPriority() > 0 {
		build.Infra.Swarming.Priority = req.Priority
	}
	setDimensions(req, cfg, build)
}

// setInput computes the input values from the given request and builder config,
// setting them in the proto. Mutates the given *pb.Build. May panic if the
// builder config is invalid.
func setInput(ctx context.Context, req *pb.ScheduleBuildRequest, cfg *pb.BuilderConfig, build *pb.Build) {
	build.Input = &pb.Build_Input{
		Properties: &structpb.Struct{},
	}

	if cfg.GetRecipe() != nil {
		// TODO(crbug/1042991): Deduplicate property parsing logic with config validation for properties.
		build.Input.Properties.Fields = make(map[string]*structpb.Value, len(cfg.Recipe.Properties)+len(cfg.Recipe.PropertiesJ)+1)
		for _, prop := range cfg.Recipe.Properties {
			k, v := strpair.Parse(prop)
			build.Input.Properties.Fields[k] = &structpb.Value{
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
			if err := protojson.Unmarshal([]byte(v), s); err != nil {
				// Builder config should have been validated already.
				panic(errors.Annotate(err, "error parsing %q", v).Err())
			}
			build.Input.Properties.Fields[k] = s.Fields[k]
		}
		build.Input.Properties.Fields["recipe"] = &structpb.Value{
			Kind: &structpb.Value_StringValue{
				StringValue: cfg.Recipe.Name,
			},
		}
	} else if cfg.GetProperties() != "" {
		if err := protojson.Unmarshal([]byte(cfg.Properties), build.Input.Properties); err != nil {
			// Builder config should have been validated already.
			panic(errors.Annotate(err, "error unmarshaling builder properties for %q", cfg.Name).Err())
		}
	}

	if build.Input.Properties.Fields == nil {
		build.Input.Properties.Fields = make(map[string]*structpb.Value, len(req.GetProperties().GetFields()))
	}

	allowedOverrides := stringset.NewFromSlice(cfg.GetAllowedPropertyOverrides()...)
	anyOverride := allowedOverrides.Has("*")
	for k, v := range req.GetProperties().GetFields() {
		if build.Input.Properties.Fields[k] != nil && !anyOverride && !allowedOverrides.Has(k) {
			logging.Warningf(ctx, "ScheduleBuild: Unpermitted Override for property %q for builder %q (ignored)", k, protoutil.FormatBuilderID(build.Builder))
		}
		build.Input.Properties.Fields[k] = v
	}

	build.Input.GitilesCommit = req.GetGitilesCommit()
	build.Input.GerritChanges = req.GetGerritChanges()
}

// setTags computes the tags from the given request, setting them in the proto.
// Mutates the given *pb.Build.
func setTags(req *pb.ScheduleBuildRequest, build *pb.Build) {
	tags := protoutil.StringPairMap(req.GetTags())
	if req.GetBuilder() != nil {
		tags.Add("builder", req.Builder.Builder)
	}
	if gc := req.GetGitilesCommit(); gc != nil {
		if buildset := protoutil.GitilesBuildSet(gc); buildset != "" {
			tags.Add("buildset", buildset)
		}
		tags.Add("gitiles_ref", gc.Ref)
	}
	for _, ch := range req.GetGerritChanges() {
		tags.Add("buildset", protoutil.GerritBuildSet(ch))
	}
	build.Tags = protoutil.StringPairs(tags)
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
// setting them in the proto. Mutates the given *pb.Build.
func setTimeouts(req *pb.ScheduleBuildRequest, cfg *pb.BuilderConfig, build *pb.Build) {
	// Timeouts in the request have highest precedence, followed by
	// values in the builder config, followed by default values.
	switch {
	case req.GetExecutionTimeout() != nil:
		build.ExecutionTimeout = req.ExecutionTimeout
	case cfg.GetExecutionTimeoutSecs() > 0:
		build.ExecutionTimeout = &durationpb.Duration{
			Seconds: int64(cfg.ExecutionTimeoutSecs),
		}
	default:
		build.ExecutionTimeout = defExecutionTimeout
	}

	switch {
	case req.GetGracePeriod() != nil:
		build.GracePeriod = req.GracePeriod
	case cfg.GetGracePeriod() != nil:
		build.GracePeriod = cfg.GracePeriod
	default:
		build.GracePeriod = defGracePeriod
	}

	switch {
	case req.GetSchedulingTimeout() != nil:
		build.SchedulingTimeout = req.SchedulingTimeout
	case cfg.GetExpirationSecs() > 0:
		build.SchedulingTimeout = &durationpb.Duration{
			Seconds: int64(cfg.ExpirationSecs),
		}
	default:
		build.SchedulingTimeout = defSchedulingTimeout
	}
}

// buildFromScheduleRequest returns a build proto created from the given
// request and builder config. Sets fields except those which can only be
// determined at creation time.
func buildFromScheduleRequest(ctx context.Context, req *pb.ScheduleBuildRequest, ancestors []int64, cfg *pb.BuilderConfig, globalCfg *pb.SettingsCfg) (b *pb.Build) {
	b = &pb.Build{
		Builder:         req.Builder,
		Critical:        cfg.GetCritical(),
		WaitForCapacity: cfg.GetWaitForCapacity() == pb.Trinary_YES,
	}

	if req.Critical != pb.Trinary_UNSET {
		b.Critical = req.Critical
	}

	if len(ancestors) > 0 {
		b.AncestorIds = ancestors
		// Temporarily accept req.CanOutliveParent to be unset, and treat it
		// the same as pb.Trinary_YES.
		// This is to prevent breakage due to unmatched timelines of deployments
		// (for example recipes rolls and bb CLI rolls).
		// TODO(crbug.com/1031205): after the parent tracking feature is stabled,
		// we should require req.CanOutliveParent to be set.
		b.CanOutliveParent = req.GetCanOutliveParent() != pb.Trinary_NO
	}

	setExecutable(req, cfg, b)
	setInfra(req, cfg, b, globalCfg) // Requires setExecutable.
	setInput(ctx, req, cfg, b)
	setTags(req, b)
	setTimeouts(req, cfg, b)
	setExperiments(ctx, req, cfg, globalCfg, b)         // Requires setExecutable, setInfra, setInput.
	if err := setInfraAgent(b, globalCfg); err != nil { // Requires setExecutable, setInfra, setExperiments.
		// TODO(crbug.com/1266060) bubble up the error after TaskBackend workflow is ready.
		// The current ScheduleBuild doesn't need this info. Swallow it to not interrupt the normal workflow.
		logging.Warningf(ctx, "Failed to set build.Infra.Buildbucket.Agent for build %d: %s", b.Id, err)
	}
	return
}

// setInfraAgent populate the agent info from the given settings.
// Mutates the given *pb.Build.
// The build.Builder, build.Canary, build.Exe, and build.Infra.Buildbucket must be set.
func setInfraAgent(build *pb.Build, globalCfg *pb.SettingsCfg) error {
	build.Infra.Buildbucket.Agent = &pb.BuildInfra_Buildbucket_Agent{}
	setInfraAgentInputData(build, globalCfg)
	if err := setInfraAgentSource(build, globalCfg); err != nil {
		return err
	}
	// TODO(crbug.com/1345722) In the future, bbagent will entirely manage the
	// user executable payload, which means Buildbucket should not specify the
	// payload path.
	// We should change the purpose field and use symbolic paths in the input
	// like "$exe" and "$agentUtils".
	// Reference: https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/3792330/comments/734e18f7_b7f4726d
	build.Infra.Buildbucket.Agent.Purposes = map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
		"kitchen-checkout": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
	}
	return nil
}

// setInfraAgentInputData populate input cipd info from the given settings.
// In the future, they can be also from per-builder-level or per-request-level.
// Mutates the given *pb.Build.
// The build.Builder, build.Canary, build.Exe, and build.Infra.Buildbucket.Agent must be set
func setInfraAgentInputData(build *pb.Build, globalCfg *pb.SettingsCfg) {
	inputData := make(map[string]*pb.InputDataRef)
	build.Infra.Buildbucket.Agent.Input = &pb.BuildInfra_Buildbucket_Agent_Input{
		Data: inputData,
	}
	userPackages := globalCfg.GetSwarming().GetUserPackages()
	if userPackages == nil {
		return
	}
	cipdServer := globalCfg.GetCipd().GetServer()
	id := protoutil.FormatBuilderID(build.Builder)

	experiments := stringset.NewFromSlice(build.GetInput().GetExperiments()...)

	for _, p := range userPackages {
		if !builderMatches(id, p.Builders) {
			continue
		}

		if !experimentsMatch(experiments, p.GetIncludeOnExperiment(), p.GetOmitOnExperiment()) {
			continue
		}

		path := UserPackageDir
		if p.Subdir != "" {
			path = fmt.Sprintf("%s/%s", path, p.Subdir)
		}
		if _, ok := inputData[path]; !ok {
			inputData[path] = &pb.InputDataRef{
				DataType: &pb.InputDataRef_Cipd{
					Cipd: &pb.InputDataRef_CIPD{
						Server: cipdServer,
					},
				},
				OnPath: []string{path, fmt.Sprintf("%s/%s", path, "bin")},
			}
		}

		inputData[path].GetCipd().Specs = append(inputData[path].GetCipd().Specs, &pb.InputDataRef_CIPD_PkgSpec{
			Package: p.PackageName,
			Version: extractCipdVersion(p, build),
		})
	}

	if build.Exe.GetCipdPackage() != "" || build.Exe.GetCipdVersion() != "" {
		inputData["kitchen-checkout"] = &pb.InputDataRef{
			DataType: &pb.InputDataRef_Cipd{
				Cipd: &pb.InputDataRef_CIPD{
					Server: cipdServer,
					Specs: []*pb.InputDataRef_CIPD_PkgSpec{
						{
							Package: build.Exe.GetCipdPackage(),
							Version: build.Exe.GetCipdVersion(),
						},
					},
				},
			},
		}
	}
}

// setInfraAgentSource extracts bbagent source info from the given settings.
// In the future, they can be also from per-builder-level or per-request-level.
// Mutates the given *pb.Build.
// The build.Canary, build.Infra.Buildbucket.Agent must be set
func setInfraAgentSource(build *pb.Build, globalCfg *pb.SettingsCfg) error {
	bbagent := globalCfg.GetSwarming().GetBbagentPackage()
	if bbagent == nil {
		return nil
	}

	if !strings.HasSuffix(bbagent.PackageName, "/${platform}") {
		return errors.New("bad settings: bbagent package name must end with '/${platform}'")
	}
	cipdHost := globalCfg.GetCipd().GetServer()
	build.Infra.Buildbucket.Agent.Source = &pb.BuildInfra_Buildbucket_Agent_Source{
		DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
			Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
				Package: bbagent.PackageName,
				Version: extractCipdVersion(bbagent, build),
				Server:  cipdHost,
			},
		},
	}
	return nil
}

// setExperimentsFromProto sets experiments in the model (see model/build.go).
// build.Proto.Input.Experiments and
// build.Proto.Infra.Buildbucket.ExperimentReasons must be set (see setExperiments).
func setExperimentsFromProto(build *model.Build) {
	setExps := stringset.NewFromSlice(build.Proto.Input.Experiments...)
	for exp := range build.Proto.Infra.Buildbucket.ExperimentReasons {
		if !setExps.Has(exp) {
			build.Experiments = append(build.Experiments, fmt.Sprintf("-%s", exp))
		}
	}
	for _, exp := range build.Proto.Input.Experiments {
		build.Experiments = append(build.Experiments, fmt.Sprintf("+%s", exp))
	}
	sort.Strings(build.Experiments)

	build.Canary = build.Proto.Canary
	build.Experimental = build.Proto.Input.Experimental
}

// scheduleBuilds handles requests to schedule builds. Requests must be validated and authorized.
// The length of returned builds always equal to len(reqs).
// A single returned error means a global error which applies to every request.
// Otherwise, it would be a MultiError where len(MultiError) equals to len(reqs).
func scheduleBuilds(ctx context.Context, globalCfg *pb.SettingsCfg, reqs ...*pb.ScheduleBuildRequest) ([]*model.Build, error) {
	if len(reqs) == 0 {
		return []*model.Build{}, nil
	}

	dryRun := reqs[0].DryRun
	for _, req := range reqs {
		if req.DryRun != dryRun {
			return nil, appstatus.BadRequest(errors.Reason("all requests must have the same dry_run value").Err())
		}
	}

	now := clock.Now(ctx).UTC()
	user := auth.CurrentIdentity(ctx)
	appID := info.AppID(ctx) // e.g. cr-buildbucket

	merr := make(errors.MultiError, len(reqs))
	// Bucket -> Builder -> *pb.BuilderConfig.
	cfgs, err := fetchBuilderConfigs(ctx, reqs)
	if me, ok := err.(errors.MultiError); ok {
		merr = mergeErrs(merr, me, "error fetching builders", func(i int) int { return i })
	} else if err != nil {
		return nil, err
	}

	validReq, idxMapBlds := getValidReqs(reqs, merr)
	var idxMapNums []int
	blds := make([]*model.Build, len(validReq))
	nums := make([]*model.Build, 0, len(validReq))
	var ids []int64
	if dryRun {
		ids = make([]int64, len(validReq))
	} else {
		ids = buildid.NewBuildIDs(ctx, now, len(validReq))
	}

	_, pBld, err := validateBuildToken(ctx, 0, false)
	if err != nil {
		return nil, err
	}

	var ancestors []int64
	switch {
	case pBld == nil:
		ancestors = make([]int64, 0)
	case len(pBld.AncestorIds) > 0:
		ancestors = append(pBld.AncestorIds, pBld.ID)
	default:
		ancestors = append(ancestors, pBld.ID)
	}

	for i := range blds {
		origI := idxMapBlds[i]
		bucket := fmt.Sprintf("%s/%s", validReq[i].Builder.Project, validReq[i].Builder.Bucket)
		cfg := cfgs[bucket][validReq[i].Builder.Builder]

		// TODO(crbug.com/1042991): Parallelize build creation from requests if necessary.
		build := buildFromScheduleRequest(ctx, reqs[i], ancestors, cfg, globalCfg)

		blds[i] = &model.Build{
			ID:         ids[i],
			CreatedBy:  user,
			CreateTime: now,
			Proto:      build,
		}

		if !dryRun {
			// Set proto field values which can only be determined at creation-time.
			blds[i].Proto.CreatedBy = string(user)
			blds[i].Proto.CreateTime = timestamppb.New(now)
			blds[i].Proto.Id = ids[i]
			blds[i].Proto.Infra.Buildbucket.Hostname = fmt.Sprintf("%s.appspot.com", appID)
			blds[i].Proto.Infra.Logdog.Prefix = fmt.Sprintf("buildbucket/%s/%d", appID, blds[i].Proto.Id)
			protoutil.SetStatus(now, blds[i].Proto, pb.Status_SCHEDULED)
		}

		setExperimentsFromProto(blds[i])
		blds[i].IsLuci = cfg != nil
		blds[i].PubSubCallback.Topic = validReq[i].GetNotify().GetPubsubTopic()
		blds[i].PubSubCallback.UserData = validReq[i].GetNotify().GetUserData()
		// Tags are stored in the outer struct (see model/build.go).
		blds[i].Tags = protoutil.StringPairMap(blds[i].Proto.Tags).Format()

		exp := make(map[int64]struct{})
		for _, d := range blds[i].Proto.Infra.GetSwarming().GetTaskDimensions() {
			exp[d.Expiration.GetSeconds()] = struct{}{}
		}
		if len(exp) > 6 {
			merr[origI] = appstatus.BadRequest(errors.Reason("build %d contains more than 6 unique expirations", i).Err())
			continue
		}

		if cfg.GetBuildNumbers() == pb.Toggle_YES {
			idxMapNums = append(idxMapNums, origI)
			nums = append(nums, blds[i])
		}
	}
	if dryRun {
		return blds, nil
	}
	if err := generateBuildNumbers(ctx, nums); err != nil {
		me := err.(errors.MultiError)
		merr = mergeErrs(merr, me, "error generating build numbers", func(idx int) int { return idxMapNums[idx] })
	}

	validBlds, idxMapValidBlds := getValidBlds(blds, merr, idxMapBlds)
	err = parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() error { return model.UpdateBuilderStat(ctx, validBlds, now) }
		if rdbHost := globalCfg.GetResultdb().GetHostname(); rdbHost != "" {
			work <- func() error { return resultdb.CreateInvocations(ctx, validBlds, cfgs, rdbHost) }
		}
		work <- func() error { return search.UpdateTagIndex(ctx, validBlds) }
	})
	if err != nil {
		errs := err.(errors.MultiError)
		for _, e := range errs {
			if me, ok := e.(errors.MultiError); ok {
				merr = mergeErrs(merr, me, "", func(idx int) int { return idxMapValidBlds[idx] })
			} else {
				return nil, e // top-level error
			}
		}
	}

	// This parallel work isn't combined with the above parallel work to ensure build entities and Swarming
	// task creation tasks are only created if everything else has succeeded (since everything can't be done
	// in one transaction).
	_ = parallel.WorkPool(min(64, len(validBlds)), func(work chan<- func() error) {
		for i, b := range validBlds {
			origI := idxMapValidBlds[i]
			if merr[origI] != nil {
				validBlds[i] = nil
				continue
			}

			b := b
			reqID := reqs[origI].RequestId
			bucket := fmt.Sprintf("%s/%s", reqs[origI].Builder.Project, reqs[origI].Builder.Bucket)
			cfg := cfgs[bucket][reqs[origI].Builder.Builder]
			work <- func() error {
				toPut := []interface{}{
					b,
					&model.BuildInfra{
						Build: datastore.KeyForObj(ctx, b),
						Proto: b.Proto.Infra,
					},
					&model.BuildInputProperties{
						Build: datastore.KeyForObj(ctx, b),
						Proto: b.Proto.Input.Properties,
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

					if cfg == nil {
						return nil
					}

					if stringset.NewFromSlice(b.Proto.Input.Experiments...).Has(bb.ExperimentBackendGo) {
						if err := tasks.CreateSwarmingBuildTask(ctx, &taskdefs.CreateSwarmingBuildTask{
							BuildId: b.ID,
						}); err != nil {
							return errors.Annotate(err, "failed to enqueue CreateSwarmingBuildTask: %d", b.ID).Err()
						}
					} else {
						if err := tasks.CreateSwarmingTask(ctx, &taskdefs.CreateSwarmingTask{
							BuildId: b.ID,
						}); err != nil {
							return errors.Annotate(err, "failed to enqueue CreateSwarmingTask: %d", b.ID).Err()
						}
					}
					return nil
				}, nil)

				// Record any error happened in the above transaction.
				if err != nil {
					validBlds[i] = nil
					merr[origI] = err
					return nil
				}
				metrics.BuildCreated(ctx, b)
				return nil
			}
		}
	})

	if merr.First() == nil {
		return validBlds, nil
	}
	// Map back to final results to make sure len(resBlds) always equal to len(reqs).
	resBlds := make([]*model.Build, len(reqs))
	for i, bld := range validBlds {
		if merr[idxMapValidBlds[i]] == nil {
			resBlds[idxMapValidBlds[i]] = bld
		}
	}
	return resBlds, merr
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

// validateScheduleBuild validates and authorizes the given request, returning
// a normalized version of the request and field mask.
func validateScheduleBuild(ctx context.Context, wellKnownExperiments stringset.Set, req *pb.ScheduleBuildRequest, parent *model.Build) (*pb.ScheduleBuildRequest, *model.BuildMask, error) {
	var err error
	if err = validateSchedule(req, wellKnownExperiments, parent); err != nil {
		return nil, nil, appstatus.BadRequest(err)
	}
	normalizeSchedule(req)

	m, err := model.NewBuildMask("", req.Fields, req.Mask)
	if err != nil {
		return nil, nil, appstatus.BadRequest(errors.Annotate(err, "invalid mask").Err())
	}

	if req, err = scheduleRequestFromTemplate(ctx, req); err != nil {
		return nil, nil, err
	}
	if err = perm.HasInBucket(ctx, bbperms.BuildsAdd, req.Builder.Project, req.Builder.Bucket); err != nil {
		return nil, nil, err
	}
	return req, m, nil
}

// ScheduleBuild handles a request to schedule a build. Implements pb.BuildsServer.
func (*Builds) ScheduleBuild(ctx context.Context, req *pb.ScheduleBuildRequest) (*pb.Build, error) {
	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "error fetching service config").Err()
	}
	wellKnownExperiments := protoutil.WellKnownExperiments(globalCfg)

	pBld, err := validateParent(ctx)
	if err != nil {
		return nil, err
	}

	req, m, err := validateScheduleBuild(ctx, wellKnownExperiments, req, pBld)
	if err != nil {
		return nil, err
	}

	blds, err := scheduleBuilds(ctx, globalCfg, req)
	if err != nil {
		if merr, ok := err.(errors.MultiError); ok {
			return nil, merr.First()
		}
		return nil, err
	}
	if req.DryRun {
		// Dry run build is not saved in datastore, return the proto right away.
		return blds[0].Proto, nil
	}

	// No need to redact the response here, because we're effectively just sending
	// the caller's inputs back to them.
	return blds[0].ToProto(ctx, m, nil)
}

// scheduleBuilds handles requests to schedule builds.
// The length of returned builds and errors (if any) always equal to the len(reqs).
// The returned error type is always MultiError.
func (*Builds) scheduleBuilds(ctx context.Context, globalCfg *pb.SettingsCfg, reqs []*pb.ScheduleBuildRequest) ([]*pb.Build, errors.MultiError) {
	// The ith error is the error associated with the ith request.
	merr := make(errors.MultiError, len(reqs))
	// The ith mask is the field mask derived from the ith request.
	masks := make([]*model.BuildMask, len(reqs))
	wellKnownExperiments := protoutil.WellKnownExperiments(globalCfg)

	errorInBatch := func(err error) errors.MultiError {
		for i, e := range merr {
			if e == nil {
				merr[i] = appstatus.BadRequest(errors.Annotate(err, "error in schedule batch").Err())
			}
		}
		return merr
	}

	// Validate parent.
	pBld, err := validateParent(ctx)
	if err != nil {
		return nil, errorInBatch(err)
	}

	// Validate requests.
	_ = parallel.WorkPool(min(64, len(reqs)), func(work chan<- func() error) {
		for i, req := range reqs {
			i := i
			req := req
			work <- func() error {
				reqs[i], masks[i], merr[i] = validateScheduleBuild(ctx, wellKnownExperiments, req, pBld)
				return nil
			}
		}
	})

	validReqs, idxMapValidReqs := getValidReqs(reqs, merr)
	// Non-MultiError error should apply to every item and fail all requests.
	blds, err := scheduleBuilds(ctx, globalCfg, validReqs...)
	if err != nil {
		if me, ok := err.(errors.MultiError); ok {
			merr = mergeErrs(merr, me, "", func(i int) int { return idxMapValidReqs[i] })
		} else {
			return nil, errorInBatch(err)
		}
	}

	ret := make([]*pb.Build, len(blds))
	_ = parallel.WorkPool(min(64, len(blds)), func(work chan<- func() error) {
		for i, bld := range blds {
			if bld == nil {
				continue
			}
			origI := idxMapValidReqs[i]
			i := i
			bld := bld
			work <- func() error {
				// Note: We don't redact the Build response here because we expect any user with
				// BuildsAdd permission should also have BuildsGet.
				// TODO(crbug/1042991): Don't re-read freshly written entities (see ToProto).
				ret[i], merr[origI] = bld.ToProto(ctx, masks[origI], nil)
				return nil
			}
		}
	})

	if merr.First() == nil {
		return ret, nil
	}
	origRet := make([]*pb.Build, len(reqs))
	for i, origI := range idxMapValidReqs {
		if merr[origI] == nil {
			origRet[origI] = ret[i]
		}
	}
	return origRet, merr
}

// mergeErrs merges errs into origErrs according to the idxMapper.
func mergeErrs(origErrs, errs errors.MultiError, reason string, idxMapper func(int) int) errors.MultiError {
	for i, err := range errs {
		if err != nil {
			origErrs[idxMapper(i)] = errors.Annotate(err, reason).Err()
		}
	}
	return origErrs
}

// getValidReqs returns a list of valid ScheduleBuildRequest where its corresponding error is nil,
// as well as an index map where idxMap[returnedIndex] == originalIndex.
func getValidReqs(reqs []*pb.ScheduleBuildRequest, errs errors.MultiError) ([]*pb.ScheduleBuildRequest, []int) {
	if len(reqs) != len(errs) {
		panic("The length of reqs and the length of errs must be the same.")
	}
	var validReqs []*pb.ScheduleBuildRequest
	var idxMap []int
	for i, req := range reqs {
		if errs[i] == nil {
			idxMap = append(idxMap, i)
			validReqs = append(validReqs, req)
		}
	}
	return validReqs, idxMap
}

// getValidBlds returns a list of valid builds where its corresponding error is nil.
// as well as an index map where idxMap[returnedIndex] == originalIndex.
func getValidBlds(blds []*model.Build, origErrs errors.MultiError, idxMapBlds []int) ([]*model.Build, []int) {
	if len(blds) != len(idxMapBlds) {
		panic("The length of blds and the length of idxMapBlds must be the same.")
	}
	var validBlds []*model.Build
	var idxMap []int
	for i, bld := range blds {
		origI := idxMapBlds[i]
		if origErrs[origI] == nil {
			idxMap = append(idxMap, origI)
			validBlds = append(validBlds, bld)
		}
	}
	return validBlds, idxMap
}

func extractCipdVersion(p *pb.SwarmingSettings_Package, b *pb.Build) string {
	if b.Canary && p.VersionCanary != "" {
		return p.VersionCanary
	}
	return p.Version
}
