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
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/common/buildcel"
	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	modeldefs "go.chromium.org/luci/buildbucket/appengine/model/defs"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

const CurrentBucketSchemaVersion = 14

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

	// DefExecutionTimeout is the default value for pb.Build.ExecutionTimeout.
	// See setTimeouts.
	DefExecutionTimeout = 3 * time.Hour

	// DefSchedulingTimeout is the default value for pb.Build.SchedulingTimeout.
	// See setTimeouts.
	DefSchedulingTimeout = 6 * time.Hour
)

// changeLog is a temporary struct to track all changes in UpdateProjectCfg.
type changeLog struct {
	item   string
	action string
}

// UpdateProjectCfg fetches all projects' Buildbucket configs from luci-config
// and update into Datastore.
// TODO(b/356205234): Currently the function is very convoluted and would
// benefit from a refactoring that splits it into multiple functions.
func UpdateProjectCfg(ctx context.Context) error {
	client := cfgclient.Client(ctx)
	// Cannot fetch all projects configs at once because luci-config still uses
	// GAEv1 in Python2 framework, which has a response size limit on ~35MB.
	// Have to first fetch all projects config metadata and then fetch the actual
	// config in parallel.
	cfgMetas, err := client.GetProjectConfigs(ctx, "${appid}.cfg", true)
	if err != nil {
		return errors.Fmt("while fetching project configs' metadata: %w", err)
	}
	cfgs := make([]*config.Config, len(cfgMetas))
	err = parallel.WorkPool(min(64, len(cfgMetas)), func(work chan<- func() error) {
		for i, meta := range cfgMetas {
			cfgSet := meta.ConfigSet
			work <- func() error {
				cfg, err := client.GetConfig(ctx, cfgSet, "${appid}.cfg", false)
				if err != nil {
					return errors.Fmt("failed to fetch the project config for %s: %w", string(cfgSet), err)
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
		return errors.Fmt("failed to fetch all bucket keys: %w", err)
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

	var projKeys []*datastore.Key
	if err := datastore.GetAll(ctx, datastore.NewQuery(model.ProjectKind), &projKeys); err != nil {
		return errors.Fmt("failed to fetch all project keys: %w", err)
	}
	projsToDelete := make(map[string]*datastore.Key) // project -> project keys
	for _, projKey := range projKeys {
		projsToDelete[projKey.StringID()] = projKey
	}
	var projsToPut []*model.Project

	bldrMetrics, err := mapBuilderMetricsToBuilders(ctx)
	if err != nil {
		return err
	}
	bldrsMCB := stringset.New(0)
	for i, meta := range cfgMetas {
		project := meta.ConfigSet.Project()
		if cfgs[i] == nil || cfgs[i].Content == "" {
			delete(bucketsToDelete, project)
			delete(projsToDelete, project)
			continue
		}
		pCfg := &pb.BuildbucketCfg{}
		if err := prototext.Unmarshal([]byte(cfgs[i].Content), pCfg); err != nil {
			logging.Errorf(ctx, "config of project %s is broken: %s", project, err)
			// If a project config is broken, we don't delete the already stored
			// buckets and projects.
			delete(bucketsToDelete, project)
			delete(projsToDelete, project)
			continue
		}
		delete(projsToDelete, project)

		revision := meta.Revision
		// revision is empty in file-system mode. Use SHA1 of the config as revision.
		if revision == "" {
			cntHash := sha1.Sum([]byte(cfgs[i].Content))
			revision = "sha1:" + hex.EncodeToString(cntHash[:])
		}

		// Check if the shared project-level config has changed.
		prjChanged, err := isCommonConfigChanged(ctx, pCfg.CommonConfig, project)
		if err != nil {
			return err
		}
		if prjChanged {
			projsToPut = append(projsToPut, &model.Project{
				ID:           project,
				CommonConfig: pCfg.CommonConfig,
			})
		}

		// shadow bucket id -> bucket ids it shadows
		shadows := make(map[string][]string)
		// bucket id -> bucket entity
		buckets := make(map[string]*model.Bucket)
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

			// Collect the custom builder metric -> builders mapping.
			// We need to get the full mapping regardless if this project's config
			// is changed or not. That's why we do it here instead of later when
			// iterating through builders.
			for _, cfgBuilder := range cfgBucket.GetSwarming().GetBuilders() {
				for _, cm := range cfgBuilder.CustomMetricDefinitions {
					if _, ok := bldrMetrics[cm.Name]; ok {
						bldrMetrics[cm.Name].Add(fmt.Sprintf("%s/%s/%s", project, cfgBktName, cfgBuilder.Name))
					}
				}
			}

			if storedBucket.Schema == CurrentBucketSchemaVersion && storedBucket.Revision == revision {
				// Keep the stored buckets for now, so that in the case that a newly
				// added bucket reuses an existing bucket as its shadow bucket, the shadow
				// bucket gets updated later with the list of buckets it shadows.
				buckets[cfgBktName] = storedBucket
				if storedBucket.Proto.GetShadow() != "" {
					// This bucket is shadowed.
					shadow := storedBucket.Proto.Shadow
					shadows[shadow] = append(shadows[shadow], cfgBktName)
				}
				continue
			}

			var builders []*model.Builder
			bktKey := model.BucketKey(ctx, project, cfgBktName)
			if err := datastore.GetAll(ctx, datastore.NewQuery(model.BuilderKind).Ancestor(bktKey), &builders); err != nil {
				return errors.Fmt("failed to fetch builders for %s.%s: %w", project, cfgBktName, err)
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
			buildersToPut := [][]*model.Builder{{}}
			currentBatchSize := 0
			for _, cfgBuilder := range cfgBucket.GetSwarming().GetBuilders() {
				if !checkPoolDimExists(cfgBuilder) {
					cfgBuilder.Dimensions = append(cfgBuilder.Dimensions, fmt.Sprintf("pool:luci.%s.%s", project, cfgBktName))
				}

				cfgBldrName := protoutil.ToBuilderIDString(project, cfgBktName, cfgBuilder.Name)
				cfgBuilderHash, bldrSize, err := computeBuilderHash(cfgBuilder)
				if err != nil {
					return errors.Fmt("while computing hash for builder:%s: %w", cfgBldrName, err)
				}

				if bldr, ok := bldrMap[cfgBldrName]; ok {
					delete(bldrMap, cfgBldrName)
					// Trigger a PopPendingBuildTask if max_concurrent_builds
					// is increased or reset.
					old := bldr.Config.GetMaxConcurrentBuilds()
					new := cfgBuilder.GetMaxConcurrentBuilds()
					if old != 0 && (new > old || new == 0) {
						bldrsMCB.Add(cfgBldrName)
					}
					if bldr.ConfigHash == cfgBuilderHash {
						continue
					}
				}

				if bldrSize > maxBatchSize {
					return errors.Fmt("builder %s size exceeds %d bytes", cfgBldrName, maxBatchSize)
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
			buckets[cfgBktName] = bucketToUpdate

			if cfgBucket.GetShadow() != "" {
				// This bucket is shadowed.
				shadow := cfgBucket.Shadow
				shadows[shadow] = append(shadows[shadow], cfgBktName)
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
				return errors.Fmt("for bucket %s.%s: %w", project, cfgBktName, err)
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

		// Update shadow buckets.
		var shadowBucketsToUpdate []*model.Bucket
		for shadow, shadowed := range shadows {
			shadowed = stringset.NewFromSlice(shadowed...).ToSortedSlice() // Deduplicate
			toUpdate, ok := buckets[shadow]
			if !ok {
				logging.Infof(ctx, "cannot find config for shadow bucket %s in project %s", shadow, project)
				continue
			}
			if !slices.Equal(toUpdate.Shadows, shadowed) {
				toUpdate.Shadows = shadowed
				shadowBucketsToUpdate = append(shadowBucketsToUpdate, toUpdate)
			}
		}
		if len(shadowBucketsToUpdate) > 0 {
			if err := datastore.Put(ctx, shadowBucketsToUpdate); err != nil {
				return errors.Fmt("for shadow buckets in project %s: %w", project, err)
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
	// Delete non-existent projects. Their associated buckets/builders has been
	// put into toDelete in the above code.
	for _, projKey := range projsToDelete {
		toDelete = append(toDelete, projKey)
	}

	// TODO(crbug.com/1362157) Delete after the correctness is proved in Prod.
	for _, keys := range toDelete {
		changes = append(changes, &changeLog{item: keys.StringID(), action: "delete"})
	}
	for _, prj := range projsToPut {
		changes = append(changes, &changeLog{item: prj.ID, action: "put"})
	}

	if len(changes) == 0 {
		logging.Debugf(ctx, "No changes this time.")
	} else {
		logging.Debugf(ctx, "Made %d changes:", len(changes))
		for _, change := range changes {
			logging.Debugf(ctx, "%s, Action:%s", change.item, change.action)
		}
	}
	if err := datastore.Put(ctx, projsToPut); err != nil {
		return err
	}

	bldrMetricEnt, err := prepareBuilderMetricsToPut(ctx, bldrMetrics)
	if err != nil {
		return err
	}
	if bldrMetricEnt != nil {
		logging.Debugf(ctx, "Updating custom builder metrics: %+v", bldrMetricEnt.Metrics)
		if err := datastore.Put(ctx, bldrMetricEnt); err != nil {
			return err
		}
	}

	// Trigger all the popPendingBuild tasks.
	for bldrID := range bldrsMCB {
		bldrID, err := protoutil.ParseBuilderID(bldrID)
		if err != nil {
			return err
		}
		// Import CreatePopPendingBuildTask will cause a circular dependency error.
		err = tq.AddTask(ctx, &tq.Task{
			Title: "pop-pending-build-0",
			Payload: &taskdefs.PopPendingBuildTask{
				BuilderId: bldrID,
			},
		})
		if err != nil {
			return err
		}
	}
	return datastore.Delete(ctx, toDelete)
}

func isCommonConfigChanged(ctx context.Context, newCfg *pb.BuildbucketCfg_CommonConfig, project string) (bool, error) {
	storedPrj := &model.Project{ID: project}
	switch err := datastore.Get(ctx, storedPrj); {
	case err == datastore.ErrNoSuchEntity:
		if len(newCfg.GetBuildsNotificationTopics()) != 0 {
			return true, nil
		} else {
			return false, nil
		}
	case err != nil:
		return false, errors.Fmt("error fetching project entity - %s: %w", project, err)
	}

	storedTopics := storedPrj.CommonConfig.GetBuildsNotificationTopics()
	if len(newCfg.GetBuildsNotificationTopics()) != len(storedTopics) {
		return true, nil
	}

	storedTopicsSet := stringset.New(len(storedTopics))
	for _, t := range storedTopics {
		storedTopicsSet.Add(t.Name + "|" + t.Compression.String())
	}
	for _, t := range newCfg.GetBuildsNotificationTopics() {
		if !storedTopicsSet.Has(t.Name + "|" + t.Compression.String()) {
			return true, nil
		}
	}
	return false, nil
}

// transacUpdate updates the given bucket, its builders or delete its builders transactionally.
func transacUpdate(ctx context.Context, bucketToUpdate *model.Bucket, buildersToPut, bldrsToDel []*model.Builder) error {
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		if err := datastore.Put(ctx, bucketToUpdate); err != nil {
			return errors.Fmt("failed to put bucket: %w", err)
		}
		if err := datastore.Put(ctx, buildersToPut); err != nil {
			return errors.Fmt("failed to put %d builders: %w", len(buildersToPut), err)
		}
		if err := datastore.Delete(ctx, bldrsToDel); err != nil {
			return errors.Fmt("failed to delete %d builders: %w", len(bldrsToDel), err)
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
			return errors.Fmt("failed to delete builders[%d:%d]: %w", startIdx, endIdx, err)
		}
	}

	// put builders in buildersToPut
	for _, bldrs := range buildersToPut {
		if err := datastore.Put(ctx, bldrs); err != nil {
			return errors.Fmt("failed to put builders starting from %s: %w", bldrs[0].ID, err)
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
		return errors.Fmt("error fetching service config: %w", err)
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
		case bucket.GetSwarming() != nil && bucket.GetDynamicBuilderTemplate() != nil:
			ctx.Errorf("mutually exclusive fields swarming and dynamic_builder_template both exist in bucket %q", bucket.Name)
		case bucket.GetDynamicBuilderTemplate() != nil && bucket.GetShadow() != "":
			ctx.Errorf("dynamic bucket %q cannot have a shadow bucket %q", bucket.Name, bucket.Shadow)
		}
		bucketNames.Add(bucket.Name)
		// TODO(crbug/1399576): Change this once bucket proto replaces Swarming message name
		if s := bucket.Swarming; s != nil {
			validateProjectSwarming(ctx, s, wellKnownExperiments, project, globalCfg)
		}
		if builderTemp := bucket.DynamicBuilderTemplate.GetTemplate(); builderTemp != nil {
			validateBuilderCfg(ctx, builderTemp, wellKnownExperiments, project, globalCfg, true)
		}
		ctx.Exit()
	}

	if cfg.CommonConfig != nil {
		validateBuildNotifyTopics(ctx, cfg.CommonConfig.BuildsNotificationTopics, project)
	}
	return nil
}

// validateBuildNotifyTopics validate `builds_notification_topics` field.
func validateBuildNotifyTopics(ctx *validation.Context, topics []*pb.BuildbucketCfg_Topic, project string) {
	if len(topics) == 0 {
		return
	}

	ctx.Enter("builds_notification_topics")
	defer ctx.Exit()

	errs := make(errors.MultiError, len(topics))
	_ = parallel.WorkPool(min(6, len(topics)), func(work chan<- func() error) {
		for i, topic := range topics {
			cloudProj, topicID, err := clients.ValidatePubSubTopicName(topic.Name)
			if err != nil {
				errs[i] = err
				continue
			}
			work <- func() error {
				client, err := clients.NewPubsubClient(ctx.Context, cloudProj, project)
				if err != nil {
					errs[i] = errors.Fmt("failed to create a pubsub client for %q: %w", cloudProj, err)
					return nil
				}
				cTopic := client.Topic(topicID)
				switch perms, err := cTopic.IAM().TestPermissions(ctx.Context, []string{"pubsub.topics.publish"}); {
				case err != nil:
					errs[i] = errors.Fmt("failed to check luci project account's permission for %s: %w", topic.Name, err)
				case len(perms) < 1:
					errs[i] = errors.Fmt("luci project account (%s-scoped@luci-project-accounts.iam.gserviceaccount.com) doesn't have the publish permission for %s", project, topic.Name)
				}
				return nil
			}
		}
	})

	for _, err := range errs {
		if err != nil {
			ctx.Errorf("builds_notification_topics: %s", err)
		}
	}
}

// validateProjectSwarming validates project_config.Swarming.
func validateProjectSwarming(ctx *validation.Context, s *pb.Swarming, wellKnownExperiments stringset.Set, project string, globalCfg *pb.SettingsCfg) {
	ctx.Enter("swarming")
	defer ctx.Exit()

	if s.GetTaskTemplateCanaryPercentage().GetValue() > 100 {
		ctx.Errorf("task_template_canary_percentage.value must must be in [0, 100]")
	}

	builderNames := stringset.New(len(s.Builders))
	for i, b := range s.Builders {
		ctx.Enter("builders #%d - %s", i, b.Name)
		validateBuilderCfg(ctx, b, wellKnownExperiments, project, globalCfg, false)
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
		return errors.Fmt("must start with 'luci.%s.' because it starts with 'luci.' and is defined in the %q project", project, project)
	case !bucketRegex.MatchString(bucket):
		return errors.Fmt("%q does not match %q", bucket, bucketRegex)
	}
	return nil
}

// ValidateTaskBackendTarget validates the target value for a
// buildbucket task backend.
func ValidateTaskBackendTarget(globalCfg *pb.SettingsCfg, target string) error {
	for _, backendSetting := range globalCfg.Backends {
		if backendSetting.Target == target {
			return nil
		}
	}
	return errors.New("provided backend target was not in global config")
}

// validateTaskBackendConfigJson makes an api call to the task backend server's
// ValidateConfigs RPC. If there are errors with the config it propagates them
// into the validation context.
func validateTaskBackendConfigJson(ctx *validation.Context, backend *pb.BuilderConfig_Backend, project string) error {
	globalCfg, err := GetSettingsCfg(ctx.Context)
	if err != nil {
		return err
	}
	backendClient, err := clients.NewBackendClient(ctx.Context, project, backend.Target, globalCfg)
	if err != nil {
		if errors.Is(err, clients.ErrTaskBackendLite) {
			return errors.New("no config_json allowed for TaskBackendLite")
		}
		return err
	}
	configJsonPb := &structpb.Struct{}
	err = configJsonPb.UnmarshalJSON([]byte(backend.ConfigJson))
	if err != nil {
		return err
	}
	req := &pb.ValidateConfigsRequest{
		Configs: []*pb.ValidateConfigsRequest_ConfigContext{
			{
				Target:     backend.Target,
				ConfigJson: configJsonPb,
			},
		},
	}
	resp, err := backendClient.ValidateConfigs(ctx.Context, req)
	if err != nil {
		return err
	}
	if len(resp.ConfigErrors) > 0 {
		for _, configErr := range resp.ConfigErrors {
			ctx.Errorf("error validating task backend ConfigJson at index %d: %s", configErr.Index, configErr.Error)
		}
	}
	return nil
}

func validateTaskBackend(ctx *validation.Context, backend *pb.BuilderConfig_Backend, project string) {
	globalCfg, err := GetSettingsCfg(ctx.Context)
	if err != nil {
		ctx.Errorf("could not get global settings config")
	}

	// validating backend.Target
	err = ValidateTaskBackendTarget(globalCfg, backend.GetTarget())
	if err != nil {
		ctx.Errorf("error validating task backend target: %s", err)
	}
	if backend.ConfigJson != "" {
		err = validateTaskBackendConfigJson(ctx, backend, project)
		if err != nil {
			ctx.Errorf("error validating task backend ConfigJson: %s", err)
		}
	}
}

// validateBuilderCfg validate a Builder config message.
func validateBuilderCfg(ctx *validation.Context, b *pb.BuilderConfig, wellKnownExperiments stringset.Set, project string, globalCfg *pb.SettingsCfg, isDynamic bool) {
	// TODO(iannucci): also validate builder allowed_property_overrides field. See
	// //lucicfg/starlark/stdlib/internal/luci/rules/builder.star

	// name
	if isDynamic && b.Name != "" {
		ctx.Errorf("builder name should not be set in a dynamic bucket")
	} else if !isDynamic && !builderRegex.MatchString(b.Name) {
		ctx.Errorf("name must match %s", builderRegex)
	}

	// auto_builder_dimension
	if isDynamic && b.GetAutoBuilderDimension() == pb.Toggle_YES {
		ctx.Errorf("should not toggle on auto_builder_dimension in a dynamic bucket")
	}

	// Need to do separate checks here since backend and backend_alt can both be set.
	// Either backend or swarming must be set, but not both.
	switch {
	case b.GetSwarmingHost() != "" && b.GetBackend() != nil:
		ctx.Errorf("only one of swarming host or task backend is allowed")
	case b.GetBackend() != nil:
		validateTaskBackend(ctx, b.Backend, project)
	case b.GetSwarmingHost() != "":
		validateHostname(ctx, "swarming_host", b.SwarmingHost)
	default:
		ctx.Errorf("either swarming host or task backend must be set")
	}

	if b.GetBackendAlt() != nil {
		// validate backend_alt
		validateTaskBackend(ctx, b.BackendAlt, project)
	}

	// validate swarming_tags
	for i, swarmingTag := range b.SwarmingTags {
		ctx.Enter("swarming_tags #%d", i)
		if swarmingTag != "vpython:native-python-wrapper" {
			ctx.Errorf("Deprecated. Used only to enable \"vpython:native-python-wrapper\"")
		}
		ctx.Exit()
	}

	validateDimensions(ctx, b.Dimensions, false)

	// timeouts
	if b.ExecutionTimeoutSecs != 0 || b.ExpirationSecs != 0 {
		exeTimeout := time.Duration(b.ExecutionTimeoutSecs) * time.Second
		schedulingTimeout := time.Duration(b.ExpirationSecs) * time.Second
		if exeTimeout == 0 {
			exeTimeout = DefExecutionTimeout
		}
		if schedulingTimeout == 0 {
			schedulingTimeout = DefSchedulingTimeout
		}
		if exeTimeout+schedulingTimeout > model.BuildMaxCompletionTime {
			exeTimeoutSec := int(exeTimeout.Seconds())
			schedulingTimeoutSec := int(schedulingTimeout.Seconds())
			limitTimeoutSec := int(model.BuildMaxCompletionTime.Seconds())
			switch {
			case b.ExecutionTimeoutSecs == 0:
				ctx.Errorf("(default) execution_timeout_secs %d + expiration_secs %d exceeds max build completion time %d", exeTimeoutSec, schedulingTimeoutSec, limitTimeoutSec)
			case b.ExpirationSecs == 0:
				ctx.Errorf("execution_timeout_secs %d + (default) expiration_secs %d exceeds max build completion time %d", exeTimeoutSec, schedulingTimeoutSec, limitTimeoutSec)
			default:
				ctx.Errorf("execution_timeout_secs %d + expiration_secs %d exceeds max build completion time %d", exeTimeoutSec, schedulingTimeoutSec, limitTimeoutSec)
			}
		}
	}
	if b.HeartbeatTimeoutSecs != 0 &&
		!isLiteBackend(b.GetBackend().GetTarget(), globalCfg) &&
		!isLiteBackend(b.GetBackendAlt().GetTarget(), globalCfg) {
		ctx.Errorf("heartbeat_timeout_secs should only be set for builders using a TaskBackendLite backend")
	}

	// resultdb
	if b.Resultdb.GetHistoryOptions().GetCommit() != nil {
		ctx.Errorf("resultdb.history_options.commit must be unset")
	}

	for i, bqExport := range b.Resultdb.GetBqExports() {
		if err := rdbpbutil.ValidateBigQueryExport(bqExport); err != nil {
			ctx.Errorf("error validating resultdb.bq_exports[%d]: %s", i, err)
		}
	}

	validateCaches(ctx, b.Caches)

	// exe and recipe
	switch {
	case (b.Exe == nil && b.Recipe == nil && !isDynamic) || (b.Exe != nil && b.Recipe != nil):
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

	// shadow_builder_adjustments
	if b.ShadowBuilderAdjustments != nil {
		ctx.Enter("shadow_builder_adjustments")
		if isDynamic {
			ctx.Errorf("cannot set shadow_builder_adjustments in a dynamic builder template")
		} else {
			if b.ShadowBuilderAdjustments.GetProperties() != "" {
				if !strings.HasPrefix(b.ShadowBuilderAdjustments.Properties, "{") || !json.Valid([]byte(b.ShadowBuilderAdjustments.Properties)) {
					ctx.Errorf("properties is not a JSON object")
				}
			}

			// Ensure pool and dimensions are consistent.
			// In builder config:
			// * setting shadow_pool would add the corresponding "pool:<shadow_pool>" dimension
			//   to shadow_dimensions;
			// * setting shadow_dimensions with "pool:<shadow_pool>" would also set shadow_pool.
			dims := b.ShadowBuilderAdjustments.GetDimensions()
			if b.ShadowBuilderAdjustments.GetPool() != "" && len(dims) == 0 {
				ctx.Errorf("dimensions.pool must be consistent with pool")
			}
			if len(dims) != 0 {
				validateDimensions(ctx, dims, true)

				empty := stringset.New(len(dims))
				nonEmpty := stringset.New(len(dims))
				for _, dim := range b.ShadowBuilderAdjustments.Dimensions {
					_, key, value := ParseDimension(dim)
					if value == "" {
						if nonEmpty.Has(key) {
							ctx.Errorf("dimensions contain both empty and non-empty value for the same key - %q", key)
						}
						empty.Add(key)
					} else {
						if empty.Has(key) {
							ctx.Errorf("dimensions contain both empty and non-empty value for the same key - %q", key)
						}
						nonEmpty.Add(key)
					}
					if key == "pool" && value != b.ShadowBuilderAdjustments.Pool {
						ctx.Errorf("dimensions.pool must be consistent with pool")
					}
				}
			}
		}
		ctx.Exit()
	}

	// custome_build_metrics
	if len(b.CustomMetricDefinitions) > 0 {
		validateCustomMetricDefinitions(ctx, b.CustomMetricDefinitions, globalCfg)
	}
}

func validateCustomMetricDefinitions(ctx *validation.Context, cms []*pb.CustomMetricDefinition, globalCfg *pb.SettingsCfg) {
	ctx.Enter("custome_build_metrics")
	defer ctx.Exit()

	registeredMetrics := make(map[string]*pb.CustomMetric, len(globalCfg.GetCustomMetrics()))
	for _, gm := range globalCfg.GetCustomMetrics() {
		registeredMetrics[gm.Name] = gm
	}

	for _, cm := range cms {
		validateCustomMetricDefinition(ctx, cm, registeredMetrics)
	}
}

func validateCustomMetricDefinition(ctx *validation.Context, cm *pb.CustomMetricDefinition, registeredMetrics map[string]*pb.CustomMetric) {
	ctx.Enter("custome_build_metrics %s", cm.Name)
	defer ctx.Exit()
	// Name
	if cm.Name == "" {
		ctx.Errorf("name is required")
		return
	}

	cmRgst, ok := registeredMetrics[cm.Name]
	if !ok {
		ctx.Errorf("not registered in Buildbucket service config")
		return
	}

	// Predicates
	if _, err := buildcel.NewBool(cm.Predicates); err != nil {
		ctx.Errorf("predicates: %s", err)
	}

	// Fields
	var missedFs []string
	for _, f := range cmRgst.ExtraFields {
		if _, ok := cm.GetExtraFields()[f]; !ok {
			missedFs = append(missedFs, f)
		}
	}
	if len(missedFs) > 0 {
		ctx.Errorf("field(s) %q must be included", missedFs)
	}

	if len(cm.GetExtraFields()) > 0 {
		if metrics.IsBuilderMetric(cmRgst.GetMetricBase()) {
			ctx.Errorf("custom builder metric cannot have extra_fields")
		}
		if _, err := buildcel.NewStringMap(cm.GetExtraFields()); err != nil {
			ctx.Errorf("extra_fields: %s", err)
		}
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
				jsonMap := make(map[string]any)
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

// ParseDimension parses a dimension string.
// A dimension supports 2 forms -
// "<key>:<value>" and "<expiration_secs>:<key>:<value>" .
func ParseDimension(d string) (exp int64, k string, v string) {
	k, v = strpair.Parse(d)
	var err error
	exp, err = strconv.ParseInt(k, 10, 64)
	if err == nil {
		// k was an int64, so v is in <key>:<value> form.
		k, v = strpair.Parse(v)
	} else {
		exp = 0
		// k was the <key> and v was the <value>.
	}
	return
}

// validateDimensions validates dimensions in project configs.
// A dimension supports 2 forms -
// "<key>:<value>" and "<expiration_secs>:<key>:<value>" .
func validateDimensions(ctx *validation.Context, dimensions []string, allowMissingValue bool) {
	expirations := make(map[int64]bool)

	for i, dim := range dimensions {
		ctx.Enter("dimensions #%d - %q", i, dim)
		if !strings.Contains(dim, ":") {
			ctx.Errorf("%q does not have ':'", dim)
			ctx.Exit()
			continue
		}
		expiration, key, value := ParseDimension(dim)
		switch {
		case expiration < 0 || time.Duration(expiration) > maximumExpiration/time.Second:
			ctx.Errorf("expiration_secs is outside valid range; up to %s", maximumExpiration)
		case expiration%60 != 0:
			ctx.Errorf("expiration_secs must be a multiple of 60 seconds")
		case key == "":
			ctx.Errorf("missing key")
		case key == "caches":
			ctx.Errorf("dimension key must not be 'caches'; caches must be declared via caches field")
		case !dimensionKeyRegex.MatchString(key):
			ctx.Errorf("key %q does not match pattern %q", key, dimensionKeyRegex)
		case value == "" && !allowMissingValue:
			ctx.Errorf("missing value")
		default:
			expirations[expiration] = true
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
	case !protoutil.ExperimentNameRE.MatchString(expName):
		return errors.Fmt("does not match %q", protoutil.ExperimentNameRE)
	case strings.HasPrefix(expName, "luci.") && !wellKnownExperiments.Has(expName):
		return errors.New(`unknown experiment has reserved prefix "luci."`)
	}
	return nil
}

func isLiteBackend(target string, globalCfg *pb.SettingsCfg) bool {
	if target == "" {
		return false
	}
	for _, backend := range globalCfg.Backends {
		if backend.Target == target {
			return backend.GetLiteMode() != nil
		}
	}
	return false
}

// mapBuilderMetricsToBuilders calculates a map where the key is a custom builder
// metric names, and the value are a list of builder names (in the format of
// "<project>.<bucket>.<builder>") that report to the metric.
func mapBuilderMetricsToBuilders(ctx context.Context) (map[string]stringset.Set, error) {
	globalCfg, err := GetSettingsCfg(ctx)
	if err != nil {
		return nil, errors.Fmt("error fetching service config: %w", err)
	}
	res := make(map[string]stringset.Set)
	for _, gm := range globalCfg.GetCustomMetrics() {
		if metrics.IsBuilderMetric(gm.GetMetricBase()) {
			res[gm.Name] = stringset.New(0)
		}
	}
	return res, nil
}

// prepareBuilderMetricsToPut compares the new custom_builder_metric -> builders
// mapping with the one saved in datastore, and return a new entity if the
// mapping needs to update.
func prepareBuilderMetricsToPut(ctx context.Context, bldrMetrics map[string]stringset.Set) (*model.CustomBuilderMetrics, error) {
	ent := &model.CustomBuilderMetrics{Key: model.CustomBuilderMetricsKey(ctx)}
	err := datastore.Get(ctx, ent)
	if err != nil && err != datastore.ErrNoSuchEntity {
		return nil, errors.Fmt("fetching CustomBuilderMetrics: %w", err)
	}
	metricNames := make([]string, 0, len(bldrMetrics))
	for m := range bldrMetrics {
		metricNames = append(metricNames, m)
	}
	sort.Strings(metricNames)
	newMetrics := &modeldefs.CustomBuilderMetrics{}
	for _, m := range metricNames {
		bldrs := bldrMetrics[m].ToSortedSlice()
		if len(bldrs) == 0 {
			continue
		}
		bldrIDs := make([]*pb.BuilderID, len(bldrs))
		for i, bldr := range bldrs {
			bldrIDs[i], err = protoutil.ParseBuilderID(bldr)
			if err != nil {
				return nil, err
			}
		}
		newMetrics.Metrics = append(newMetrics.Metrics, &modeldefs.CustomBuilderMetric{
			Name:     m,
			Builders: bldrIDs,
		})
	}

	if proto.Equal(ent.Metrics, newMetrics) {
		// Nothing to update
		return nil, nil
	}
	ent.Metrics = newMetrics
	ent.LastUpdate = clock.Now(ctx).UTC()
	return ent, nil
}
