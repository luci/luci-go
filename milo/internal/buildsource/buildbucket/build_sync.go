// Copyright 2017 The LUCI Authors.
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

package buildbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gomodule/redigo/redis"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/milo/internal/model"
	"go.chromium.org/luci/milo/internal/model/milostatus"
	"go.chromium.org/luci/milo/internal/utils"
)

// BuildSummaryStorageDuration is the maximum lifetime of a BuildSummary.
//
// Lifetime is the time elapsed since the Build creation time.
// Cron runs periodically to scan and remove all the Builds of which lifetime
// exceeded this duration.
//
// BuildSummaries are kept alive longer than builds in buildbuckets. So we can
// compute blamelist for builds that are at the end of their lifetime.
//
// TODO(weiweilin): expose BuildStorageDuration from buildbucket and compute
// BuildSummaryStorageDuration base on that (e.g. add two months). So we can
// ensure BuildSummaries are kept alive longer than builds.
const BuildSummaryStorageDuration = time.Hour * 24 * 30 * 20 // ~20 months

// BuildSummarySyncThreshold is the maximum duration before Milo attempts to
// sync non-terminal builds with buildbucket.
//
// Cron runs periodically to scan and sync all the non-terminal Builds of
// that was updated more than `BuildSummarySyncThreshold` ago.
const BuildSummarySyncThreshold = time.Hour * 24 * 2 // 2 days

var (
	deletedBuildsCounter = metric.NewCounter(
		"luci/milo/cron/delete-builds/delete-count",
		"The number of BuildSummaries deleted by Milo delete-builds cron job",
		nil,
	)

	updatedBuildsCounter = metric.NewCounter(
		"luci/milo/cron/sync-builds/updated-count",
		"The number of BuildSummaries that are out of sync and updated by Milo sync-builds cron job",
		nil,
		field.String("project"),
		field.String("bucket"),
		field.String("builder"),
		field.Int("number"),
		field.Int("ID"),
	)
)

// DeleteOldBuilds is a cron job that deletes BuildSummaries that are older than
// BuildSummaryStorageDuration.
func DeleteOldBuilds(c context.Context) error {
	const (
		batchSize = 1024
		nWorkers  = 8
	)

	buildPurgeCutoffTime := clock.Now(c).Add(-BuildSummaryStorageDuration)
	q := datastore.NewQuery("BuildSummary").
		Lt("Created", buildPurgeCutoffTime).
		Order("Created").
		// Apply a limit so the call won't timeout.
		Limit(batchSize * nWorkers * 32).
		KeysOnly(true)

	return parallel.FanOutIn(func(taskC chan<- func() error) {
		buildsC := make(chan []*datastore.Key, nWorkers)
		statsC := make(chan int, nWorkers)

		// Collect stats.
		taskC <- func() error {
			for deletedCount := range statsC {
				deletedBuildsCounter.Add(c, int64(deletedCount))
			}

			return nil
		}

		// Find builds to delete.
		taskC <- func() error {
			defer close(buildsC)

			bsKeys := make([]*datastore.Key, 0, batchSize)
			err := datastore.RunBatch(c, batchSize, q, func(key *datastore.Key) error {
				bsKeys = append(bsKeys, key)
				if len(bsKeys) == batchSize {
					buildsC <- bsKeys
					bsKeys = make([]*datastore.Key, 0, batchSize)
				}
				return nil
			})
			if err != nil {
				return err
			}

			if len(bsKeys) > 0 {
				buildsC <- bsKeys
			}
			return nil
		}

		// Spawn workers to delete builds.
		taskC <- func() error {
			defer close(statsC)

			return parallel.WorkPool(nWorkers, func(workC chan<- func() error) {
				for bks := range buildsC {
					// Bind to a local variable so each worker can have their own copy.
					bks := bks
					workC <- func() error {
						// Flatten first w/o filtering to calculate how many builds were
						// actually removed.
						err := errors.Flatten(datastore.Delete(c, bks))

						allErrs := 0
						if err != nil {
							allErrs = len(err.(errors.MultiError))
						}

						statsC <- len(bks) - allErrs

						return err
					}
				}
			})
		}
	})
}

var summaryBuildMask = &field_mask.FieldMask{
	Paths: []string{
		"id",
		"builder",
		"number",
		"create_time",
		"start_time",
		"end_time",
		"update_time",
		"status",
		"summary_markdown",
		"tags",
		"infra.buildbucket.hostname",
		"infra.swarming",
		"input.experimental",
		"input.gitiles_commit",
		"output.properties",
		"critical",
	},
}

// V2PubSubHandler is a webhook that stores the builds coming in from pubsub.
func V2PubSubHandler(ctx *router.Context) {
	err := v2PubSubHandlerImpl(ctx.Request.Context(), ctx.Request)
	if err != nil {
		logging.Errorf(ctx.Request.Context(), "error while handling pubsub event")
		errors.Log(ctx.Request.Context(), err)
	}
	if transient.Tag.In(err) {
		// Transient errors are 4xx so that PubSub retries them.
		// TODO(crbug.com/1099036): Address High traffic builders causing errors.
		ctx.Writer.WriteHeader(http.StatusTooEarly)
		return
	}
	// No errors or non-transient errors are 200s so that PubSub does not retry
	// them.
	ctx.Writer.WriteHeader(http.StatusOK)
}

// v2PubSubHandlerImpl takes the http.Request, expects to find
// a common.PubSubSubscription JSON object in the Body, containing a
// BuildsV2PubSub, and handles the contents with generateSummary.
func v2PubSubHandlerImpl(c context.Context, r *http.Request) error {
	msg := utils.PubSubSubscription{}
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		// This might be a transient error, e.g. when the json format changes
		// and Milo isn't updated yet.
		return errors.Annotate(err, "could not decode message").Tag(transient.Tag).Err()
	}
	if v, ok := msg.Message.Attributes["version"].(string); ok && v != "v1" {
		// TODO(nodir): switch to v2, crbug.com/826006
		logging.Debugf(c, "unsupported pubsub message version %q. Ignoring", v)
		return nil
	}
	bData, err := msg.GetData()
	if err != nil {
		return errors.Annotate(err, "could not parse pubsub message string").Err()
	}
	buildsV2Msg := &buildbucketpb.BuildsV2PubSub{}
	opts := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	if err := opts.Unmarshal(bData, buildsV2Msg); err != nil {
		return err
	}

	return processBuild(c, buildsV2Msg.Build)
}

// PubSubHandler is a webhook that stores the builds coming in from pubsub.
func PubSubHandler(ctx *router.Context) {
	err := pubSubHandlerImpl(ctx.Request.Context(), ctx.Request)
	if err != nil {
		logging.Errorf(ctx.Request.Context(), "error while handling pubsub event")
		errors.Log(ctx.Request.Context(), err)
	}
	if transient.Tag.In(err) {
		// Transient errors are 4xx so that PubSub retries them.
		// TODO(crbug.com/1099036): Address High traffic builders causing errors.
		ctx.Writer.WriteHeader(http.StatusTooEarly)
		return
	}
	// No errors or non-transient errors are 200s so that PubSub does not retry
	// them.
	ctx.Writer.WriteHeader(http.StatusOK)
}

// pubSubHandlerImpl takes the http.Request, expects to find
// a common.PubSubSubscription JSON object in the Body, containing a bbPSEvent,
// and handles the contents with generateSummary.
func pubSubHandlerImpl(c context.Context, r *http.Request) error {
	msg := utils.PubSubSubscription{}
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		// This might be a transient error, e.g. when the json format changes
		// and Milo isn't updated yet.
		return errors.Annotate(err, "could not decode message").Tag(transient.Tag).Err()
	}
	if v, ok := msg.Message.Attributes["version"].(string); ok && v != "v1" {
		// TODO(nodir): switch to v2, crbug.com/826006
		logging.Debugf(c, "unsupported pubsub message version %q. Ignoring", v)
		return nil
	}
	bData, err := msg.GetData()
	if err != nil {
		return errors.Annotate(err, "could not parse pubsub message string").Err()
	}

	event := struct {
		Build    bbv1.LegacyApiCommonBuildMessage `json:"build"`
		Hostname string                           `json:"hostname"`
	}{}
	if err := json.Unmarshal(bData, &event); err != nil {
		return errors.Annotate(err, "could not parse pubsub message data").Err()
	}

	client, err := BuildsClient(c, event.Hostname, auth.AsSelf)
	if err != nil {
		return err
	}
	build, err := client.GetBuild(c, &buildbucketpb.GetBuildRequest{
		Id:     event.Build.Id,
		Fields: summaryBuildMask,
	})
	if err != nil {
		return err
	}
	return processBuild(c, build)
}

func processBuild(c context.Context, build *buildbucketpb.Build) error {
	// TODO(iannucci,nodir): get the bot context too
	// TODO(iannucci,nodir): support manifests/got_revision
	bs, err := model.BuildSummaryFromBuild(c, build.Infra.Buildbucket.Hostname, build)
	if err != nil {
		return err
	}
	if err := bs.AddManifestKeysFromBuildSets(c); err != nil {
		return err
	}

	logging.Debugf(c, "Received from %s: build %s (%s)\n%v",
		build.Infra.Buildbucket.Hostname, bs.ProjectID, bs.BuildID, bs.Summary.Status, bs)

	updateBuilderSummary, err := shouldUpdateBuilderSummary(c, bs)
	if err != nil {
		updateBuilderSummary = true
		logging.WithError(err).Warningf(c, "failed to determine whether the builder summary from %s should be updated. Fallback to always update.", bs.BuilderID)
	}
	err = transient.Tag.Apply(datastore.RunInTransaction(c, func(c context.Context) error {
		curBS := &model.BuildSummary{BuildKey: bs.BuildKey}
		if err := datastore.Get(c, curBS); err != nil && err != datastore.ErrNoSuchEntity {
			return errors.Annotate(err, "reading current BuildSummary").Err()
		}

		if bs.Version <= curBS.Version {
			logging.Warningf(c, "current BuildSummary is newer: %d <= %d",
				bs.Version, curBS.Version)
			return nil
		}

		if err := datastore.Put(c, bs); err != nil {
			return err
		}

		if !updateBuilderSummary {
			return nil
		}

		return model.UpdateBuilderForBuild(c, bs)
	}, nil))

	return err
}

var (
	// Sustained datastore updates should not be higher than once per second per
	// entity.
	// See https://cloud.google.com/datastore/docs/concepts/limits.
	entityUpdateIntervalInS int64 = 2

	// A Redis LUA script that update the key with the new integer value if and
	// only if the provided value is greater than the recorded value (0 if none
	// were recorded). It returns 1 if the value is updated and 0 otherwise.
	updateIfLargerScript = redis.NewScript(1, `
local newValueStr = ARGV[1]
local newValue = tonumber(newValueStr)
local existingValueStr = redis.call('GET', KEYS[1])
local existingValue = tonumber(existingValueStr) or 0
if newValue < existingValue then
	return 0
elseif newValue == existingValue then
	-- large u64/i64 (>2^53) may lose precision after being converted to f64.
	-- Compare the last 5 digits if the f64 presentation of the integers are the
	-- same.
	local newValue = tonumber(string.sub(newValueStr, -5)) or 0
	local existingValue = tonumber(string.sub(existingValueStr, -5)) or 0
	if newValue <= existingValue then
		return 0
	end
end

redis.call('SET', KEYS[1], newValueStr, 'EX', ARGV[2])
return 1
`)
)

// shouldUpdateBuilderSummary determines whether the builder summary should be
// updated with the provided build summary.
//
// If the function is called with builds from the same builder multiple times
// within a time bucket, it will block the function until the start of the next
// time bucket and only return true for the call with the lastest created build
// with a terminal status, and return false for other calls occurred within the
// same time bucket.
func shouldUpdateBuilderSummary(c context.Context, buildSummary *model.BuildSummary) (bool, error) {
	if !buildSummary.Summary.Status.Terminal() {
		return false, nil
	}

	conn, err := redisconn.Get(c)
	if err != nil {
		return true, err
	}
	defer conn.Close()

	createdAt := buildSummary.Created.UnixNano()

	now := clock.Now(c).Unix()
	thisTimeBucket := time.Unix(now-now%entityUpdateIntervalInS, 0)
	thisTimeBucketKey := fmt.Sprintf("%s:%v", buildSummary.BuilderID, thisTimeBucket.Unix())

	// Check if there's a builder summary update occurred in this time bucket.
	_, err = redis.String(conn.Do("SET", thisTimeBucketKey, createdAt, "NX", "EX", entityUpdateIntervalInS+1))
	switch err {
	case redis.ErrNil:
		// continue
	case nil:
		// There's no builder summary update occurred in this time bucket yet, we
		// should run the update.
		return true, nil
	default:
		return true, err
	}

	// There's already a builder summary update occurred in this time bucket. Try
	// scheduling the update for the next time bucket instead.
	nextTimeBucket := thisTimeBucket.Add(time.Duration(entityUpdateIntervalInS * int64(time.Second)))
	nextTimeBucketKey := fmt.Sprintf("%s:%v", buildSummary.BuilderID, nextTimeBucket.Unix())
	replaced, err := redis.Int(updateIfLargerScript.Do(conn, nextTimeBucketKey, createdAt, entityUpdateIntervalInS+1))
	if err != nil {
		return true, err
	}

	if replaced == 0 {
		// There's already an update with a newer build scheduled for the next time
		// bucket, skip the update.
		return false, nil
	}

	// Wait until the start of the next time bucket.
	clock.Sleep(c, clock.Until(c, nextTimeBucket))
	newCreatedAt, err := redis.Int64(conn.Do("GET", nextTimeBucketKey))
	if err != nil {
		return true, err
	}

	// If the update event was not replaced by event by a newer build, we should
	// run the update with this build.
	if newCreatedAt == createdAt {
		return true, nil
	}

	// Otherwise, skip the update.
	logging.Infof(c, "skipping BuilderSummary update from builder %s because a newer build was found", buildSummary.BuilderID)
	return false, nil
}

// SyncBuilds is a cron job that sync BuildSummaries that are not in terminal
// state and haven't been updated recently.
func SyncBuilds(c context.Context) error {
	c = WithBuildsClientFactory(c, ProdBuildsClientFactory)
	return syncBuildsImpl(c)
}

func syncBuildsImpl(c context.Context) error {
	const (
		batchSize = 16
		nWorkers  = 64
	)

	buildSyncCutoffTime := clock.Now(c).Add(-BuildSummarySyncThreshold)

	return parallel.FanOutIn(func(taskC chan<- func() error) {
		buildsC := make(chan []*model.BuildSummary, nWorkers)
		updatedBuildC := make(chan *buildbucketpb.Build, nWorkers)

		// Collect stats.
		taskC <- func() error {
			for b := range updatedBuildC {
				updatedBuildsCounter.Add(c, 1, b.Builder.Project, b.Builder.Bucket, b.Builder.Builder, b.Number, b.Id)
			}
			return nil
		}

		// Find builds to update.
		taskC <- func() error {
			defer close(buildsC)

			batch := make([]*model.BuildSummary, 0, batchSize)
			findBuilds := func(q *datastore.Query) error {
				return datastore.RunBatch(c, batchSize, q, func(bs *model.BuildSummary) error {
					batch = append(batch, bs)
					if len(batch) == batchSize {
						buildsC <- batch
						batch = make([]*model.BuildSummary, 0, batchSize)
					}
					return nil
				})
			}

			pendingBuildsQuery := datastore.NewQuery("BuildSummary").
				Eq("Summary.Status", milostatus.NotRun).
				Lt("Version", buildSyncCutoffTime.UnixNano()).
				// Apply a limit so the call won't timeout.
				Limit(batchSize * nWorkers * 16)
			err := findBuilds(pendingBuildsQuery)
			if err != nil {
				return err
			}

			runningBuildsQuery := datastore.NewQuery("BuildSummary").
				Eq("Summary.Status", milostatus.Running).
				Lt("Version", buildSyncCutoffTime.UnixNano()).
				// Apply a limit so the call won't timeout.
				Limit(batchSize * nWorkers * 16)
			err = findBuilds(runningBuildsQuery)
			if err != nil {
				return err
			}

			if len(batch) > 0 {
				buildsC <- batch
			}
			return nil
		}

		// Spawn workers to update builds.
		taskC <- func() error {
			defer close(updatedBuildC)

			return parallel.WorkPool(nWorkers, func(workC chan<- func() error) {
				for builds := range buildsC {
					// Bind to a local variable so each worker can have their own copy.
					builds := builds
					workC <- func() error {
						for _, bs := range builds {
							host, err := bs.GetHost()
							if err != nil {
								logging.WithError(err).Errorf(c, "failed to get host for build summary %v, skipping", bs.BuildKey)
								continue
							}

							client, err := BuildsClient(c, host, auth.AsSelf)
							if err != nil {
								return err
							}

							var buildID int64 = 0
							builder, buildNum, err := utils.ParseLegacyBuildbucketBuildID(bs.BuildID)
							if err != nil {
								// If the BuildID is not the legacy build ID, trying parsing it as
								// the new build ID.
								buildID, err = utils.ParseBuildbucketBuildID(bs.BuildID)
								if err != nil {
									return err
								}
							}

							req := &buildbucketpb.GetBuildRequest{
								Id:          buildID,
								BuildNumber: buildNum,
								Builder:     builder,
								Fields:      summaryBuildMask,
							}
							newBuild, err := client.GetBuild(c, req)
							if status.Code(err) == codes.NotFound {
								logging.Warningf(c, "build %v not found on buildbucket. deleting it.\nrequest: %v", bs.BuildKey, req)
								err := datastore.Delete(c, bs)
								if err != nil {
									err = errors.Annotate(err, "failed to delete build %v", bs.BuildKey).Err()
									return err
								}
								continue
							} else if err != nil {
								err = errors.Annotate(err, "could not fetch build %v from buildbucket.\nrequest: %v", bs.BuildKey, req).Err()
								return err
							}

							newBS, err := model.BuildSummaryFromBuild(c, host, newBuild)
							if err != nil {
								return err
							}
							if err := newBS.AddManifestKeysFromBuildSets(c); err != nil {
								return err
							}

							// Ensure the new BuildSummary has the same key as the old
							// BuildSummary, so we won't ends up having two copies of the same
							// BuildSummary in the datastore.
							newBS.BuildKey = bs.BuildKey
							newBS.BuildID = bs.BuildID

							if !newBS.Summary.Status.Terminal() {
								continue
							}

							if err := datastore.Put(c, newBS); err != nil {
								return err
							}

							updatedBuildC <- newBuild
						}

						return nil
					}
				}
			})
		}
	})
}
