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
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
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

var errRowTooBig = errors.Reason("row too big").Tag(tq.Fatal).Err()

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

	if p.Infra.Swarming == nil {
		// Backfill Infra.Swarming for builds running on Swarming implemented backends.
		// TODO(crbug.com/1508416) Stop backfill after Buildbucket taskbackend
		// migration completes and all BQ queries are migrated away from Infra.Swarming.
		err = tryBackfillSwarming(p)
		if err != nil {
			// Since the backfill is best effort, only log the error but not fail
			// this bq export task.
			logging.Warningf(ctx, "failed to backfill swarming data for build %d: %s", buildID, err)
		}
	} else if p.Infra.Backend == nil {
		// Backfill Infra.Backend for builds running on Swarming directly.
		// TODO(crbug.com/1508416) Stop backfill after Buildbucket taskbackend
		// migration completes - by then there will be no more builds running on
		// Swarming directly, so this flow can be removed.
		err = tryBackfillBackend(p)
		if err != nil {
			// Since the backfill is best effort, only log the error but not fail
			// this bq export task.
			logging.Warningf(ctx, "failed to backfill backend data for build %d: %s", buildID, err)
		}
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

	// Try upload the build before striping any info.
	// TODO(b/364947089): Add a metric to monitor the builds are being stripped or
	// given up.
	if err = tryUpload(ctx, buildID, p); err != errRowTooBig {
		return err
	}

	// Reduce Build size and try upload again.
	if buildIsSmallEnough := tryReduceBuildSize(ctx, buildID, p); !buildIsSmallEnough {
		return errRowTooBig
	}

	return tryUpload(ctx, buildID, p)
}

func tryUpload(ctx context.Context, buildID int64, p *pb.Build) error {
	// Set timeout to avoid a hanging call.
	// Cloud task default timeout is 10 min, worst case we need to call Insert
	// twice, so set a shorter timeout for each call.
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()

	row := &lucibq.Row{
		InsertID: strconv.FormatInt(p.Id, 10),
		Message:  p,
	}

	err := clients.GetBqClient(ctx).Insert(ctx, "raw", "completed_builds", row)
	if err == nil {
		return nil
	}

	if pme, _ := err.(bigquery.PutMultiError); len(pme) != 0 {
		return tq.Fatal.Apply(errors.Fmt("bad row for build %d: %w", buildID, err))
	}

	gerr, _ := err.(*googleapi.Error)
	if gerr != nil && gerr.Code == http.StatusRequestEntityTooLarge {
		return errRowTooBig
	}
	return transient.Tag.Apply(errors.Fmt("transient error when inserting BQ for build %d: %w", buildID, err))
}

func buildIsSmallEnough(p proto.Message) bool {
	pj, _ := protojson.Marshal(p)
	return len(pj) <= maxBuildSizeInBQ
}

// tryReduceBuildSize strip out info from p to make it fit in maxBuildSizeInBQ.
//
// Return a bool for whether the build is successfully reduced to under maxBuildSizeInBQ.
//
// Update p in place.
func tryReduceBuildSize(ctx context.Context, buildID int64, p *pb.Build) bool {
	// Strip out the outputProperties first.
	if p.Output.GetProperties() != nil {
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
	if buildIsSmallEnough(p) {
		return true
	}

	logging.Warningf(ctx, "striping out step logs for build %d in BQ exporting", buildID)
	for _, step := range p.GetSteps() {
		step.Logs = nil
	}

	return buildIsSmallEnough(p)
}

// tryBackfillSwarming does a best effort backfill on Infra.Swarming for builds
// running on Swarming implemented backends.
// TODO(crbug.com/1508416) Stop backfill after Buildbucket taskbackend
// migration completes and all BQ queries are migrated away from Infra.Swarming.
func tryBackfillSwarming(b *pb.Build) (err error) {
	backend := b.Infra.Backend
	if backend.GetTask().GetId().GetId() == "" {
		// No backend task associated with the build, bail out.
		return
	}
	if !strings.HasPrefix(backend.Task.Id.Target, "swarming://") {
		// The build doesn't run on a Swarming implemented backend, bail out.
		return
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
	sw.BotDimensions, err = protoutil.BotDimensionsFromBackend(b)
	return
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

// tryBackfillBackend does a best effort backfill on Infra.Backend for builds
// running on Swarming directly.
// TODO(crbug.com/1508416) Stop backfill after Buildbucket taskbackend
// migration completes.
func tryBackfillBackend(b *pb.Build) (err error) {
	sw := b.Infra.Swarming
	if sw.GetTaskId() == "" {
		// No swarming task associated with the build, bail out.
		return
	}

	backend := &pb.BuildInfra_Backend{
		Hostname: sw.Hostname,
		Task: &pb.Task{
			Id: &pb.TaskID{
				Target: computeBackendTarget(sw.Hostname),
				Id:     sw.TaskId,
			},
			// Status, Link, StatusDetails, SummaryMarkdown and UpdateId are not
			// populated in this backfill.
		},
		Caches:         swarmingCacheToCommonCache(sw.Caches),
		TaskDimensions: sw.TaskDimensions,
		Config: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"priority":        structpb.NewNumberValue(float64(sw.Priority)),
				"service_account": structpb.NewStringValue(sw.TaskServiceAccount),
			},
		},
	}
	b.Infra.Backend = backend

	// set backend.Task.Details
	backend.Task.Details, err = protoutil.AddBotDimensionsToTaskDetails(sw.BotDimensions, nil)
	return err
}

// computeBackendTarget returns the backend target based on the swarming hostname.
// It's essentially a hack. The accurate way is to find the backend in global
// config by matching the hostname. But it's a bit heavy for a temporary solution
// like this.
func computeBackendTarget(swHost string) string {
	swHostRe := regexp.MustCompile(`(.*).appspot.com`)
	var swInstance string
	if m := swHostRe.FindStringSubmatch(swHost); m != nil {
		swInstance = m[1]
	}
	return fmt.Sprintf("swarming://%s", swInstance)
}

// swarmingCacheToCommonCache returns the equivalent []*pb.CacheEntry
// for the given []*pb.BuildInfra_Swarming_CacheEntry.
func swarmingCacheToCommonCache(swCache []*pb.BuildInfra_Swarming_CacheEntry) []*pb.CacheEntry {
	var cache []*pb.CacheEntry
	for _, c := range swCache {
		cacheEntry := &pb.CacheEntry{
			EnvVar:           c.GetEnvVar(),
			Name:             c.GetName(),
			Path:             c.GetPath(),
			WaitForWarmCache: c.GetWaitForWarmCache(),
		}
		cache = append(cache, cacheEntry)
	}
	return cache
}
