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
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

const CurrentBucketSchemaVersion = 13

// maximumExpiration is the maximum allowed expiration_secs for a builder
// dimensions field.
const maximumExpiration = 21 * (24 * time.Hour)

// maxEntityCount is the Datastore maximum number of entities per transaction.
const maxEntityCount = 500

// maxBatchSize is the maximum allowed size in a batch Datastore operations. The
// Datastore actual maximum API request size is 10MB. We set it to 9MB to give
// some buffer room.
var maxBatchSize = 9 * 1000 * 1000

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

// changeLog is a temporary struct to track all changes in UpdateProjectCfg.
type changeLog struct {
	item   string
	action string
}

// UpdateProjectCfg fetches all projects' Buildbucket configs from luci-config
// and update into Datastore.
func UpdateProjectCfg(ctx context.Context) error {
	client := cfgclient.Client(ctx)
	// Cannot fetch all projects configs at once because luci-config still uses
	// GAEv1 in Python2 framework, which has a response size limit on ~35MB.
	// Have to first fetch all projects config metadata and then fetch the actual
	// config in parallel.
	cfgMetas, err := client.GetProjectConfigs(ctx, "${appid}.cfg", true)
	if err != nil {
		return errors.Annotate(err, "while fetching project configs' metadata").Err()
	}
	cfgs := make([]*config.Config, len(cfgMetas))
	err = parallel.WorkPool(min(64, len(cfgMetas)), func(work chan<- func() error) {
		for i, meta := range cfgMetas {
			i := i
			cfgSet := meta.ConfigSet
			work <- func() error {
				cfg, err := client.GetConfig(ctx, cfgSet, "${appid}.cfg", false)
				if err != nil {
					return errors.Annotate(err, "failed to fetch the project config for %s", string(cfgSet)).Err()
				}
				cfgs[i] = cfg
				return nil
			}
		}
	})
	if err != nil {
		// Just log the error, and continue to update configs for those which don't
		// have errors.
		logging.Errorf(ctx, err.Error())
	}

	var bucketKeys []*datastore.Key
	if err := datastore.GetAll(ctx, datastore.NewQuery(model.BucketKind), &bucketKeys); err != nil {
		return errors.Annotate(err, "failed to fetch all bucket keys").Err()
	}

	var changes []*changeLog
	bucketsToDelete := make(map[string]map[string]*datastore.Key) // project -> bucket -> bucket keys
	for _, bk := range bucketKeys {
		project := bk.Parent().StringID()
		if _, ok := bucketsToDelete[project]; !ok {
			bucketsToDelete[project] = make(map[string]*datastore.Key)
		}
		bucketsToDelete[project][bk.StringID()] = bk
	}

	for i, meta := range cfgMetas {
		project := meta.ConfigSet.Project()
		if cfgs[i] == nil || cfgs[i].Content == "" {
			delete(bucketsToDelete, project)
			continue
		}
		pCfg := &pb.BuildbucketCfg{}
		if err := prototext.Unmarshal([]byte(cfgs[i].Content), pCfg); err != nil {
			logging.Errorf(ctx, "config of project %s is broken: %s", project, err)
			// If a project config is broken, we don't delete the already stored buckets.
			delete(bucketsToDelete, project)
			continue
		}

		revision := meta.Revision
		// revision is empty in file-system mode. Use SHA1 of the config as revision.
		if revision == "" {
			cntHash := sha1.Sum([]byte(cfgs[i].Content))
			revision = "sha1:" + hex.EncodeToString(cntHash[:])
		}

		for _, cfgBucket := range pCfg.Buckets {
			cfgBktName := shortBucketName(cfgBucket.Name)
			storedBucket := &model.Bucket{
				ID:     cfgBktName,
				Parent: model.ProjectKey(ctx, project),
			}
			delete(bucketsToDelete[project], cfgBktName)
			if err := model.GetIgnoreMissing(ctx, storedBucket); err != nil {
				return err
			}

			if storedBucket.Schema == CurrentBucketSchemaVersion && storedBucket.Revision == revision {
				continue
			}

			var builders []*model.Builder
			bktKey := model.BucketKey(ctx, project, cfgBktName)
			if err := datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(bktKey), &builders); err != nil {
				return errors.Annotate(err, "failed to fetch builders for %s.%s", project, cfgBktName).Err()
			}

			// Builders that in the current Datastore.
			// The map item will be dynamically removed when iterating on builder
			// configs in order to know which builders no longer exist in the latest configs.
			bldrMap := make(map[string]*model.Builder) // full builder name -> *model.Builder
			for _, bldr := range builders {
				bldrMap[bldr.FullBuilderName()] = bldr
			}
			// buildersToPut[i] contains a list of builders to update which is under
			// the maxEntityCount and maxBatchsize limit.
			buildersToPut := [][]*model.Builder{[]*model.Builder{}}
			currentBatchSize := 0
			for _, cfgBuilder := range cfgBucket.GetSwarming().GetBuilders() {
				if !checkPoolDimExists(cfgBuilder) {
					cfgBuilder.Dimensions = append(cfgBuilder.Dimensions, fmt.Sprintf("pool:luci.%s.%s", project, cfgBktName))
				}

				cfgBldrName := fmt.Sprintf("%s.%s.%s", project, cfgBktName, cfgBuilder.Name)
				cfgBuilderHash, bldrSize, err := computeBuilderHash(cfgBuilder)
				if err != nil {
					return errors.Annotate(err, "while computing hash for builder:%s", cfgBldrName).Err()
				}
				if bldr, ok := bldrMap[cfgBldrName]; ok {
					delete(bldrMap, cfgBldrName)
					if bldr.ConfigHash == cfgBuilderHash {
						continue
					}
				}

				if bldrSize > maxBatchSize {
					return errors.Reason("builder %s size exceeds %d bytes", cfgBldrName, maxBatchSize).Err()
				}
				bldrToPut := &model.Builder{
					ID:         cfgBuilder.Name,
					Parent:     bktKey,
					Config:     cfgBuilder,
					ConfigHash: cfgBuilderHash,
				}
				currentBatchIdx := len(buildersToPut) - 1
				if currentBatchSize+bldrSize <= maxBatchSize && len(buildersToPut[currentBatchIdx])+1 <= maxEntityCount {
					currentBatchSize += bldrSize
					buildersToPut[currentBatchIdx] = append(buildersToPut[currentBatchIdx], bldrToPut)
				} else {
					buildersToPut = append(buildersToPut, []*model.Builder{bldrToPut})
					currentBatchSize = bldrSize
				}
			}

			// Update the bucket in this for loop iteration, its builders and delete non-existent builders.
			if len(cfgBucket.Swarming.GetBuilders()) != 0 {
				// Trim builders. They're stored in separate Builder entities.
				cfgBucket.Swarming.Builders = []*pb.BuilderConfig{}
			}
			bucketToUpdate := &model.Bucket{
				ID:       cfgBktName,
				Parent:   model.ProjectKey(ctx, project),
				Schema:   CurrentBucketSchemaVersion,
				Revision: revision,
				Proto:    cfgBucket,
			}
			var bldrsToDel []*model.Builder
			for _, bldr := range bldrMap {
				bldrsToDel = append(bldrsToDel, &model.Builder{
					ID:     bldr.ID,
					Parent: bldr.Parent,
				})
			}
			var err error
			// check if they can update transactionally.
			if len(buildersToPut) == 1 && len(buildersToPut[0])+len(bldrsToDel) < maxEntityCount {
				err = transacUpdate(ctx, bucketToUpdate, buildersToPut[0], bldrsToDel)
			} else {
				err = nonTransacUpdate(ctx, bucketToUpdate, buildersToPut, bldrsToDel)
			}
			if err != nil {
				return errors.Annotate(err, "for bucket %s.%s", project, cfgBktName).Err()
			}

			// TODO(crbug.com/1362157) Delete after the correctness is proved in Prod.
			changes = append(changes, &changeLog{item: project + "." + cfgBktName, action: "put"})
			for _, bldrs := range buildersToPut {
				for _, bldr := range bldrs {
					changes = append(changes, &changeLog{item: bldr.FullBuilderName(), action: "put"})
				}
			}
			for _, bldr := range bldrsToDel {
				changes = append(changes, &changeLog{item: bldr.FullBuilderName(), action: "delete"})
			}
		}
	}

	// Delete non-existent buckets (and all associated builders).
	var toDelete []*datastore.Key
	for _, bktMap := range bucketsToDelete {
		for _, bktKey := range bktMap {
			bldrKeys := []*datastore.Key{}
			if err := datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(bktKey), &bldrKeys); err != nil {
				return err
			}
			toDelete = append(toDelete, bldrKeys...)
			toDelete = append(toDelete, bktKey)
		}
	}

	// TODO(crbug.com/1362157) Delete after the correctness is proved in Prod.
	for _, bkt := range toDelete {
		changes = append(changes, &changeLog{item: bkt.Parent().StringID() + "." + bkt.StringID(), action: "delete"})
	}
	if len(changes) == 0 {
		logging.Debugf(ctx, "No changes this time.")
	} else {
		logging.Debugf(ctx, "Made %d changes:", len(changes))
		for _, change := range changes {
			logging.Debugf(ctx, "%s, Action:%s", change.item, change.action)
		}
	}
	return datastore.Delete(ctx, toDelete)
}

// transacUpdate updates the given bucket, its builders or delete its builders transactionally.
func transacUpdate(ctx context.Context, bucketToUpdate *model.Bucket, buildersToPut, bldrsToDel []*model.Builder) error {
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		if err := datastore.Put(ctx, bucketToUpdate); err != nil {
			return errors.Annotate(err, "failed to put bucket").Err()
		}
		if err := datastore.Put(ctx, buildersToPut); err != nil {
			return errors.Annotate(err, "failed to put %d builders", len(buildersToPut)).Err()
		}
		if err := datastore.Delete(ctx, bldrsToDel); err != nil {
			return errors.Annotate(err, "failed to delete %d builders", len(bldrsToDel)).Err()
		}
		return nil
	}, nil)
	return err
}

// nonTransacUpdate updates the given bucket, its builders or delete its builders non-transactionally.
func nonTransacUpdate(ctx context.Context, bucketToUpdate *model.Bucket, buildersToPut [][]*model.Builder, bldrsToDel []*model.Builder) error {
	// delete builders in bldrsToDel
	for i := 0; i < (len(bldrsToDel)+maxBatchSize-1)/maxBatchSize; i++ {
		startIdx := i * maxBatchSize
		endIdx := startIdx + maxBatchSize
		if endIdx > len(bldrsToDel) {
			endIdx = len(bldrsToDel)
		}
		if err := datastore.Delete(ctx, bldrsToDel[startIdx:endIdx]); err != nil {
			return errors.Annotate(err, "failed to delete builders[%d:%d]", startIdx, endIdx).Err()
		}
	}

	// put builders in buildersToPut
	for _, bldrs := range buildersToPut {
		if err := datastore.Put(ctx, bldrs); err != nil {
			return errors.Annotate(err, "failed to put builders starting from %s", bldrs[0].ID).Err()
		}
	}

	// put the bucket
	return datastore.Put(ctx, bucketToUpdate)
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}

// checkPoolDimExists check if the pool dimension exists in the builder config.
func checkPoolDimExists(cfgBuilder *pb.BuilderConfig) bool {
	for _, dim := range cfgBuilder.GetDimensions() {
		if strings.HasPrefix(dim, "pool:") {
			return true
		}
	}
	return false
}

// shortBucketName returns bucket name without "luci.<project_id>." prefix.
func shortBucketName(name string) string {
	parts := strings.SplitN(name, ".", 3)
	if len(parts) == 3 && parts[0] == "luci" {
		return parts[2]
	}
	return name
}

// computeBuilderHash computes BuilderConfig hash.
// It returns the computed hash, BuilderConfig size or the error.
func computeBuilderHash(cfg *pb.BuilderConfig) (string, int, error) {
	bCfg, err := proto.MarshalOptions{Deterministic: true}.Marshal(cfg)
	if err != nil {
		return "", 0, err
	}
	sha256Bldr := sha256.Sum256(bCfg)
	return hex.EncodeToString(sha256Bldr[:]), len(bCfg), nil
}

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

// validateBuilderCfg validate a Builder config message.
func validateBuilderCfg(ctx *validation.Context, b *pb.BuilderConfig, wellKnownExperiments stringset.Set) {
	// TODO(iannucci): also validate builder allowed_property_overrides field. See
	// //lucicfg/starlark/stdlib/internal/luci/rules/builder.star

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
}

func validateBuilderRecipe(ctx *validation.Context, recipe *pb.BuilderConfig_Recipe) {
	if recipe.Name == "" {
		ctx.Errorf("name: unspecified")
	}
	if recipe.CipdPackage == "" {
		ctx.Errorf("cipd_package: unspecified")
	}

	seenKeys := make(stringset.Set)
	validateProps := func(field string, props []string, isJson bool) {
		for i, prop := range props {
			ctx.Enter("%s #%d - %s", field, i, prop)
			if !strings.Contains(prop, ":") {
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
			case expiration < 0 || time.Duration(expiration) > maximumExpiration/time.Second:
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

func validateCaches(ctx *validation.Context, caches []*pb.BuilderConfig_CacheEntry) {
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

func validateCacheEntry(ctx *validation.Context, entry *pb.BuilderConfig_CacheEntry) {
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
