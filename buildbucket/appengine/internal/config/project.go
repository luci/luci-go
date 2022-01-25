// Copyright 2021 The LUCI Authors.
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

package config

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// maximumExpiration is the maximum allowed expiration_secs for a builder
// dimensions field.
const maximumExpiration = 21*(24*time.Hour)

var (
	authGroupNameRegex = regexp.MustCompile(`^([a-z\-]+/)?[0-9a-z_\-\.@]{1,100}$`)

	bucketRegex  = regexp.MustCompile(`^[a-z0-9\-_.]{1,100}$`)
	builderRegex = regexp.MustCompile(`^[a-zA-Z0-9\-_.\(\) ]{1,128}$`)

	serviceAccountRegex = regexp.MustCompile(`^[0-9a-zA-Z_\-\.\+\%]+@[0-9a-zA-Z_\-\.]+$`)

	dimensionKeyRegex = regexp.MustCompile(`^[a-zA-Z\_\-]+$`)

	// cacheNameRegex is copied from
	// https://chromium.googlesource.com/infra/luci/luci-py/+/f60b298f9057f19ddd7ffe26ec4c81cf8a9fa594/appengine/swarming/server/task_request.py#129
	// Keep it synchronized.
	cacheNameRegex = regexp.MustCompile(`^[a-z0-9_]+$`)
	// cacheNameMaxLength cannot be added into the above cacheNameRegex as Golang
	// regex expression can only specify a maximum repetition count below 1000.
	cacheNameMaxLength = 4096

	experimentNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)*$`)
)

// validateProjectCfg implements validation.Func and validates the content of
// Buildbucket project config file.
//
// Validation errors are returned via validation.Context. Non-validation errors
// are directly returned.
func validateProjectCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	cfg := pb.BuildbucketCfg{}
	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Errorf("invalid BuildbucketCfg proto message: %s", err)
		return nil
	}

	globalCfg, err := GetSettingsCfg(ctx.Context)
	if err != nil {
		// This error is unrelated to the data being validated. So directly return
		// to instruct config service to retry.
		return errors.Annotate(err, "error fetching service config").Err()
	}
	wellKnownExperiments := protoutil.WellKnownExperiments(globalCfg)

	if len(cfg.AclSets) > 0 {
		ctx.Errorf("acl_sets: deprecated (use go/lucicfg)")
	}

	if len(cfg.BuilderMixins) != 0 {
		ctx.Errorf("builder_mixins: deprecated (use go/lucicfg)")
	}

	// The format of configSet here is "projects/.*"
	project := strings.Split(configSet, "/")[1]
	bucketNames := stringset.New(len(cfg.Buckets))
	for i, bucket := range cfg.Buckets {
		ctx.Enter("buckets #%d - %s", i, bucket.Name)
		switch err := validateBucketName(bucket.Name, project); {
		case err != nil:
			ctx.Errorf("invalid name %q: %s", bucket.Name, err)
		case bucketNames.Has(bucket.Name):
			ctx.Errorf("duplicate bucket name %q", bucket.Name)
		case i > 0 && strings.Compare(bucket.Name, cfg.Buckets[i-1].Name) < 0:
			ctx.Warningf("bucket %q out of order", bucket.Name)
		}
		bucketNames.Add(bucket.Name)
		validateAcls(ctx, bucket.Acls)
		if len(bucket.AclSets) > 0 {
			ctx.Errorf("acl_sets: deprecated (use go/lucicfg)")
		}
		if s := bucket.Swarming; s != nil {
			validateProjectSwarming(ctx, s, wellKnownExperiments)
		}
		ctx.Exit()
	}
	return nil
}

// validateProjectSwarming validates project_config.Swarming.
func validateProjectSwarming(ctx *validation.Context, s *pb.Swarming, wellKnownExperiments stringset.Set) {
	ctx.Enter("swarming")
	defer ctx.Exit()

	if s.GetTaskTemplateCanaryPercentage().GetValue() > 100 {
		ctx.Errorf("task_template_canary_percentage.value must must be in [0, 100]")
	}
	if builderDefaults := s.BuilderDefaults; builderDefaults != nil {
		ctx.Errorf("builder_defaults: deprecated (use go/lucicfg)")
	}

	builderNames := stringset.New(len(s.Builders))
	for i, b := range s.Builders {
		ctx.Enter("builders #%d - %s", i, b.Name)
		validateBuilderCfg(ctx, b, wellKnownExperiments)
		if builderNames.Has(b.Name) {
			ctx.Errorf("name: duplicate")
		} else {
			builderNames.Add(b.Name)
		}
		ctx.Exit()
	}
}

func validateBucketName(bucket, project string) error {
	switch {
	case bucket == "":
		return errors.New("bucket name is not specified")
	case strings.HasPrefix(bucket, "luci.") && !strings.HasPrefix(bucket, fmt.Sprintf("luci.%s.", project)):
		return errors.Reason("must start with 'luci.%s.' because it starts with 'luci.' and is defined in the %q project", project, project).Err()
	case !bucketRegex.MatchString(bucket):
		return errors.Reason("%q does not match %q", bucket, bucketRegex).Err()
	}
	return nil
}

func validateAcls(ctx *validation.Context, acls []*pb.Acl) {
	for i, acl := range acls {
		ctx.Enter("acls #%d", i)
		switch {
		case acl.Group != "" && acl.Identity != "":
			ctx.Errorf("either group or identity must be set, not both")
		case acl.Group == "" && acl.Identity == "":
			ctx.Errorf("group or identity must be set")
		case acl.Group != "" && !authGroupNameRegex.MatchString(acl.Group):
			ctx.Errorf("invalid group: %s", acl.Group)
		case acl.Identity != "":
			identityStr := acl.Identity
			if !strings.Contains(acl.Identity, ":") {
				identityStr = fmt.Sprintf("user:%s", acl.Identity)
			}
			if err := identity.Identity(identityStr).Validate(); err != nil {
				ctx.Errorf("%q invalid: %s", acl.Identity, err)
			}
		}
		ctx.Exit()
	}
}

// validateBuilderCfg validate a Builder config message.
func validateBuilderCfg(ctx *validation.Context, b *pb.Builder, wellKnownExperiments stringset.Set) {
	// name
	if !builderRegex.MatchString(b.Name) {
		ctx.Errorf("name must match %s", builderRegex)
	}

	validateHostname(ctx, "swarming_host", b.SwarmingHost)

	// swarming_tags
	for i, swarmingTag := range b.SwarmingTags {
		ctx.Enter("swarming_tags #%d", i)
		if swarmingTag != "vpython:native-python-wrapper" {
			ctx.Errorf("Deprecated. Used only to enable \"vpython:native-python-wrapper\"")
		}
		ctx.Exit()
	}

	validateDimensions(ctx, b.Dimensions)

	// resultdb
	if b.Resultdb.GetHistoryOptions().GetCommit() != nil {
		ctx.Errorf("resultdb.history_options.commit must be unset")
	}

	validateCaches(ctx, b.Caches)

	// exe and recipe
	switch {
	case (b.Exe == nil && b.Recipe == nil) || (b.Exe != nil && b.Recipe != nil):
		ctx.Errorf("exactly one of exe or recipe must be specified")
	case b.Exe != nil && b.Exe.CipdPackage == "":
		ctx.Errorf("exe.cipd_package: unspecified")
	case b.Recipe != nil:
		if b.Properties != "" {
			ctx.Errorf("recipe and properties cannot be set together")
		}
		ctx.Enter("recipe")
		validateBuilderRecipe(ctx, b.Recipe)
		ctx.Exit()
	}

	// priority
	if (b.Priority != 0) && (b.Priority < 20 || b.Priority > 255) {
		ctx.Errorf("priority: must be in [20, 255] range; got %d", b.Priority)
	}

	// properties
	if b.Properties != "" {
		if !strings.HasPrefix(b.Properties, "{") || !json.Valid([]byte(b.Properties)) {
			ctx.Errorf("properties is not a JSON object")
		}
	}

	// service_account
	if b.ServiceAccount != "" && b.ServiceAccount != "bot" && !serviceAccountRegex.MatchString(b.ServiceAccount) {
		ctx.Errorf("service_account %q doesn't match %q", b.ServiceAccount, serviceAccountRegex)
	}

	// experiments
	for expName, percent := range b.Experiments {
		ctx.Enter("experiments %q", expName)
		if err := ValidateExperimentName(expName, wellKnownExperiments); err != nil {
			ctx.Error(err)
		}
		if percent < 0 || percent > 100 {
			ctx.Errorf("value must be in [0, 100]")
		}
		ctx.Exit()
	}

	// mixins
	if len(b.Mixins) > 0 {
		ctx.Errorf("mixins: deprecated (use go/lucicfg)")
	}
}

func validateBuilderRecipe(ctx *validation.Context, recipe *pb.Builder_Recipe) {
	if recipe.Name == "" {
		ctx.Errorf("name: unspecified")
	}
	if recipe.CipdPackage == ""{
		ctx.Errorf("cipd_package: unspecified")
	}

	seenKeys := make(stringset.Set)
	validateProps := func(field string, props []string, isJson bool) {
		for i, prop := range props {
			ctx.Enter("%s #%d - %s", field, i, prop)
			if !strings.Contains(prop,":") {
				ctx.Errorf("doesn't have a colon")
				ctx.Exit()
				continue
			}
			switch k, v := strpair.Parse(prop); {
			case k == "":
				ctx.Errorf("key not specified")
			case seenKeys.Has(k):
				ctx.Errorf("duplicate property")
			case k == "buildbucket":
				ctx.Errorf("reserved property")
			case k == "$recipe_engine/runtime":
				jsonMap := make(map[string]interface{})
				if err := json.Unmarshal([]byte(v), &jsonMap); err != nil {
					ctx.Errorf("not a JSON object: %s", err)
				}
				for key := range jsonMap {
					if key == "is_luci" || key == "is_experimental" {
						ctx.Errorf("key %q: reserved key", key)
					}
				}
			case isJson:
				if !json.Valid([]byte(v)) {
					ctx.Errorf("not a JSON object")
				}
			default:
				seenKeys.Add(k)
			}
			ctx.Exit()
		}
	}

	validateProps("properties", recipe.Properties, false)
	validateProps("properties_j", recipe.PropertiesJ, true)
}

// validateDimensions validates dimensions in project configs.
// A dimension supports 2 forms -
// "<key>:<value>" and "<expiration_secs>:<key>:<value>" .
func validateDimensions(ctx *validation.Context, dimensions []string) {
	expirations := make(map[int64]bool)

	for i, dim := range dimensions {
		ctx.Enter("dimensions #%d - %q", i, dim)
		if !strings.Contains(dim, ":") {
			ctx.Errorf("%q does not have ':'", dim)
			ctx.Exit()
			continue
		}
		key, value := strpair.Parse(dim)

		// For <expiration_secs>:<key>:<value>
		if expiration, err := strconv.ParseInt(key, 10, 64); err == nil {
			key, value = strpair.Parse(value)
			switch {
			case expiration < 0 || time.Duration(expiration) > maximumExpiration / time.Second:
				ctx.Errorf("expiration_secs is outside valid range; up to %s", maximumExpiration)
			case expiration%60 != 0:
				ctx.Errorf("expiration_secs must be a multiple of 60 seconds")
			default:
				expirations[expiration] = true
			}
		}

		switch {
		case key == "":
			ctx.Errorf("missing key")
		case key == "caches":
			ctx.Errorf("dimension key must not be 'caches'; caches must be declared via caches field")
		case !dimensionKeyRegex.MatchString(key):
			ctx.Errorf("key %q does not match pattern %q", key, dimensionKeyRegex)
		case value == "":
			ctx.Errorf("missing value")
		}

		if len(expirations) >= 6 {
			ctx.Errorf("at most 6 different expiration_secs values can be used")
		}
		ctx.Exit()
	}
}

func validateCaches(ctx *validation.Context, caches []*pb.Builder_CacheEntry) {
	cacheNames := stringset.New(len(caches))
	cachePaths := stringset.New(len(caches))
	fallbackSecs := make(map[int32]bool)
	for i, cache := range caches {
		ctx.Enter("caches #%d", i)
		validateCacheEntry(ctx, cache)
		if cache.Name != "" && cacheNames.Has(cache.Name) {
			ctx.Errorf("duplicate name")
		}
		if cache.Path != "" && cachePaths.Has(cache.Path) {
			ctx.Errorf("duplicate path")
		}
		cacheNames.Add(cache.Name)
		cachePaths.Add(cache.Path)
		switch secs := cache.WaitForWarmCacheSecs; {
		case secs == 0:
		case secs < 60:
			ctx.Errorf("wait_for_warm_cache_secs must be at least 60 seconds")
		case secs%60 != 0:
			ctx.Errorf("wait_for_warm_cache_secs must be rounded on 60 seconds")
		default:
			fallbackSecs[secs] = true
		}
		ctx.Exit()
	}

	if len(fallbackSecs) > 7 {
		// There can only be 8 task_slices.
		ctx.Errorf("'too many different (%d) wait_for_warm_cache_secs values; max 7", len(fallbackSecs))
	}
}

func validateCacheEntry(ctx *validation.Context, entry *pb.Builder_CacheEntry) {
	switch name := entry.Name; {
	case name == "":
		ctx.Errorf("name: required")
	case !cacheNameRegex.MatchString(name):
		ctx.Errorf("name: %q does not match %q", name, cacheNameRegex)
	case len(name) > cacheNameMaxLength:
		ctx.Errorf("name: length should be less than %d", cacheNameMaxLength)
	}

	ctx.Enter("path")
	switch path := entry.Path; {
	case path == "":
		ctx.Errorf("required")
	case strings.Contains(path, "\\"):
		ctx.Errorf("cannot contain \\. On Windows forward-slashes will be replaced with back-slashes.")
	case strings.Contains(path, ".."):
		ctx.Errorf("cannot contain '..'")
	case strings.HasPrefix(path, "/"):
		ctx.Errorf("cannot start with '/'")
	}
	ctx.Exit()
}

func ValidateExperimentName(expName string, wellKnownExperiments stringset.Set) error {
	switch {
	case !experimentNameRE.MatchString(expName):
		return errors.Reason("does not match %q", experimentNameRE).Err()
	case strings.HasPrefix(expName, "luci.") && !wellKnownExperiments.Has(expName):
		return errors.New(`unknown experiment has reserved prefix "luci."`)
	}
	return nil
}
