// Copyright 2022 The LUCI Authors.
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

package tasks

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	lucibq "go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// maxBuildSizeInBQ is (10MB-5KB) as the maximum allowed request size in either
// streaming API or storage write API is 10MB. And we want to leave 5KB buffer
// room during message conversion.
// Note: make it to var so that it can be tested in unit tests without taking up
// too much memory.
var maxBuildSizeInBQ = 10*1000*1000 - 5*1000

// ExportBuild saves the build into BiqQuery.
// The returned error has transient.Tag or tq.Fatal in order to tell tq to drop
// or retry.
func ExportBuild(ctx context.Context, buildID int64) error {
	b := &model.Build{ID: buildID}
	switch err := datastore.Get(ctx, b); {
	case err == datastore.ErrNoSuchEntity:
		return errors.Annotate(err, "build %d not found when exporting into BQ", buildID).Tag(tq.Fatal).Err()
	case err != nil:
		return errors.Annotate(err, "error fetching builds").Tag(transient.Tag).Err()
	}
	p, err := b.ToProto(ctx, model.NoopBuildMask, nil)
	if err != nil {
		return errors.Annotate(err, "failed to convert build to proto").Err()
	}

	// Backfill Infra.Swarming for builds running on Swarming implemented backends.
	// TODO(crbug.com/1508416) Stop backfill after Buildbucket taskbackend
	// migration completes and all BQ queries are migrated away from Infra.Swarming.
	err = tryBackfillSwarming(p)
	if err != nil {
		// Since the backfill is best effort, only log the error but not fail
		// this bq export task.
		logging.Warningf(ctx, "failed to backfill swarming data for build %d: %s", buildID, err)
	}

	// Clear fields that we don't want in BigQuery.
	p.Infra.Buildbucket.Hostname = ""
	if p.Infra.Backend.GetTask() != nil {
		p.Infra.Backend.Task.UpdateId = 0
	}
	for _, step := range p.GetSteps() {
		step.SummaryMarkdown = ""
		step.MergeBuild = nil
		for _, log := range step.Logs {
			name := log.Name
			log.Reset()
			log.Name = name
		}
	}

	// Check if the cleaned Build too large.
	pj, err := protojson.Marshal(p)
	if err != nil {
		logging.Errorf(ctx, "failed to calculate Build size for %d: %s, continue to try to insert the build...", buildID, err)
	}
	// We only strip out the outputProperties here.
	// Usually,large build size is caused by large outputProperties size since we
	// only expanded the 1MB Datastore limit per field for BuildOutputProperties.
	// If there are failures for any other fields, we'd like this job to continue
	// to try so that we can be alerted.
	if len(pj) > maxBuildSizeInBQ && p.Output.GetProperties() != nil {
		logging.Warningf(ctx, "striping out outputProperties for build %d in BQ exporting", buildID)
		p.Output.Properties = &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"strip_reason": {
					Kind: &structpb.Value_StringValue{
						StringValue: "output properties is stripped because it's too large which makes the whole build larger than BQ limit(10MB)",
					},
				},
			},
		}
	}
	// Set timeout to avoid a hanging call.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	row := &lucibq.Row{
		InsertID: strconv.FormatInt(p.Id, 10),
		Message:  p,
	}
	if err := clients.GetBqClient(ctx).Insert(ctx, "raw", "completed_builds", row); err != nil {
		if pme, _ := err.(bigquery.PutMultiError); len(pme) != 0 {
			return errors.Annotate(err, "bad row for build %d", buildID).Tag(tq.Fatal).Err()
		}
		return errors.Annotate(err, "transient error when inserting BQ for build %d", buildID).Tag(transient.Tag).Err()
	}
	return nil
}

// tryBackfillSwarming does a best effort backfill on Infra.Swarming for builds
// running on Swarming implemented backends.
// TODO(crbug.com/1508416) Stop backfill after Buildbucket taskbackend
// migration completes and all BQ queries are migrated away from Infra.Swarming.
func tryBackfillSwarming(b *pb.Build) error {
	if b.Infra.Swarming != nil {
		return nil
	}

	backend := b.Infra.Backend
	if backend.GetTask().GetId().GetId() == "" {
		// No backend task associated with the build, bail out.
		return nil
	}
	if !strings.HasPrefix(backend.Task.Id.Target, "swarming://") {
		// The build doesn't run on a Swarming implemented backend, bail out.
		return nil
	}

	sw := &pb.BuildInfra_Swarming{
		Hostname:       backend.Hostname,
		TaskId:         backend.Task.Id.Id,
		Caches:         commonCacheToSwarmingCache(backend.Caches),
		TaskDimensions: backend.TaskDimensions,
		// ParentRunId is not set because Buildbucket (instead of backend) manages
		// builds' parent/child relationships.
	}
	b.Infra.Swarming = sw

	if backend.Config != nil {
		for k, v := range backend.Config.AsMap() {
			if k == "priority" {
				if p, ok := v.(float64); ok {
					sw.Priority = int32(p)
				}
			}
			if k == "service_account" {
				if s, ok := v.(string); ok {
					sw.TaskServiceAccount = s
				}
			}
		}
	}

	var botDimensions map[string][]string
	sw.BotDimensions = make([]*pb.StringPair, 0)
	if backend.Task.Details != nil {
		details := backend.Task.Details.GetFields()
		if bds, ok := details["bot_dimensions"]; ok {
			bdsJSON, err := bds.MarshalJSON()
			if err != nil {
				return errors.Annotate(err, "failed to marshal task details to JSON for build %d", b.Id).Err()
			}
			err = json.Unmarshal(bdsJSON, &botDimensions)
			if err != nil {
				return errors.Annotate(err, "failed to unmarshal task details JSON for build %d", b.Id).Err()
			}

			for k, vs := range botDimensions {
				for _, v := range vs {
					sw.BotDimensions = append(sw.BotDimensions, &pb.StringPair{Key: k, Value: v})
				}
			}
			protoutil.SortStringPairs(sw.BotDimensions)
		}
	}

	return nil
}

// commonCacheToSwarmingCache returns the equivalent
// []*pb.BuildInfra_Swarming_CacheEntry for the given []*pb.CacheEntry.
func commonCacheToSwarmingCache(cache []*pb.CacheEntry) []*pb.BuildInfra_Swarming_CacheEntry {
	var swarmingCache []*pb.BuildInfra_Swarming_CacheEntry
	for _, c := range cache {
		cacheEntry := &pb.BuildInfra_Swarming_CacheEntry{
			EnvVar:           c.GetEnvVar(),
			Name:             c.GetName(),
			Path:             c.GetPath(),
			WaitForWarmCache: c.GetWaitForWarmCache(),
		}
		swarmingCache = append(swarmingCache, cacheEntry)
	}
	return swarmingCache
}
