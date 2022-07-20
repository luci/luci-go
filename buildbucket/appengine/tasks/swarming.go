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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

const (
	// bbagentReservedGracePeriod is the time reserved by bbagent in order to have
	// time to have a couple retry rounds for UpdateBuild RPCs
	// TODO(crbug.com/1328646): may need to adjust the grace_period based on
	// UpdateBuild's new performance in Buildbucket Go.
	bbagentReservedGracePeriod = 180

	// buildTimeOut is the maximum amount of time to try to sync a build.
	buildTimeOut = 2 * 24 * time.Hour

	// cacheDir is the path, relative to the swarming run dir, to the directory that
	// contains the mounted swarming named caches. It will be prepended to paths of
	// caches defined in global or builder configs.
	cacheDir = "cache"

	// pubsubTopicTemplate is the topic template where Swarming publishes
	// notifications on the task update.
	pubsubTopicTemplate = "projects/%s/topics/swarming"

	// pubSubUserDataTemplate is the Swarming topic user data template.
	pubSubUserDataTemplate = `{
		"build_id": %d,
		"created_ts": %d,
		"swarming_hostname": %s
}`

	// swarmingCreateTaskGiveUpTimeout indicates how long to retry
	// the createSwarmingTask before giving up with INFRA_FAILURE.
	swarmingCreateTaskGiveUpTimeout = 10 * 60 * time.Second

	// swarmingTimeFormat is time format used by swarming.
	swarmingTimeFormat = "2006-01-02T15:04:05.999999999"
)

// SyncBuild synchronizes the build with Swarming.
// If the swarming task does not exist yet, creates it.
// Otherwise, updates the build state to match swarming task state.
// Enqueues a new sync push task if the build did not end.
//
// Cloud tasks handler will retry the task if any error is thrown, unless it's
// tagged with tq.Fatal.
func SyncBuild(ctx context.Context, buildID int64, generation int64) error {
	bld := &model.Build{ID: buildID}
	infra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, bld)}
	switch err := datastore.Get(ctx, bld, infra); {
	case errors.Contains(err, datastore.ErrNoSuchEntity):
		return tq.Fatal.Apply(errors.Annotate(err, "build %d or buildInfra not found", buildID).Err())
	case err != nil:
		return transient.Tag.Apply(errors.Annotate(err, "failed to fetch build %d or buildInfra", buildID).Err())
	}
	if protoutil.IsEnded(bld.Status) {
		logging.Infof(ctx, "build %d is ended", buildID)
		return nil
	}
	if clock.Now(ctx).Sub(bld.CreateTime) > buildTimeOut {
		logging.Infof(ctx, "build %d (create_time:%s) has passed the sync deadline: %s", buildID, bld.CreateTime, buildTimeOut.String())
		return nil
	}

	bld.Proto.Infra = infra.Proto
	swarm, err := clients.NewSwarmingClient(ctx, infra.Proto.Swarming.Hostname, bld.Project)
	if err != nil {
		return tq.Fatal.Apply(errors.Annotate(err, "failed to create a swarming client for build %d (%s), in %s", buildID, bld.Project, infra.Proto.Swarming.Hostname).Err())
	}
	if bld.Proto.Infra.Swarming.TaskId == "" {
		if err := createSwarmingTask(ctx, bld, swarm); err != nil {
			// Mark build as Infra_failure for fatal and non-retryable errors.
			if tq.Fatal.In(err) {
				return failBuild(ctx, bld.ID, err.Error())
			}
			return err
		}
	} else {
		if err := syncBuildWithTaskResult(ctx, bld.ID, bld.Proto.Infra.Swarming.TaskId, swarm); err != nil {
			// Tq should retry non-fatal errors.
			if !tq.Fatal.In(err) {
				return err
			}
			// For fatal errors, we just log it and continue to the part of enqueueing
			// the next generation sync task.
			logging.Errorf(ctx, "Dropping the sync task due to the fatal error: %s", err)
		}
	}

	// TODO(crbug.com/1328646): Enqueue the next generation of swarming-build-sync task.
	return nil
}

// syncBuildWithTaskResult syncs Build entity in the datastore with a result of the swarming task.
func syncBuildWithTaskResult(ctx context.Context, buildID int64, taskID string, swarm clients.SwarmingClient) error {
	taskResult, err := swarm.GetTaskResult(ctx, taskID)
	if err != nil {
		logging.Errorf(ctx, "failed to fetch swarming task %s for build %d: %s", taskID, buildID, err)
		if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code >= 400 && apiErr.Code < 500 {
			return failBuild(ctx, buildID, fmt.Sprintf("invalid swarming task %s", taskID))
		}
		return transient.Tag.Apply(err)
	}
	if taskResult == nil {
		return failBuild(ctx, buildID, fmt.Sprintf("Swarming task %s unexpectedly disappeared", taskID))
	}

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		bld := &model.Build{
			ID: buildID,
		}
		infra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, bld)}
		if err := datastore.Get(ctx, bld, infra); err != nil {
			return transient.Tag.Apply(errors.Annotate(err, "failed to fetch build or buildInfra: %d", bld.ID).Err())
		}

		statusChanged, err := updateBuildFromTaskResult(ctx, bld, infra, taskResult)
		if err != nil {
			return tq.Fatal.Apply(err)
		}
		if !statusChanged {
			return nil
		}

		toPut := []interface{}{bld, infra}
		switch {
		case bld.Proto.Status == pb.Status_STARTED:
			if err := NotifyPubSub(ctx, bld); err != nil {
				return transient.Tag.Apply(err)
			}
			metrics.BuildStarted(ctx, bld)
		case protoutil.IsEnded(bld.Proto.Status):
			steps := &model.BuildSteps{Build: datastore.KeyForObj(ctx, bld)}
			if changed, _ := steps.CancelIncomplete(ctx, bld.Proto.EndTime); changed {
				toPut = append(toPut, steps)
			}
			if err := sendOnBuildCompletion(ctx, bld); err != nil {
				return transient.Tag.Apply(err)
			}
			metrics.BuildCompleted(ctx, bld)
		}
		return transient.Tag.Apply(datastore.Put(ctx, toPut...))
	}, nil)
	if err != nil {
		return err
	}
	return nil
}

// updateBuildFromTaskResult mutate the build and infra entities according to
// the given task result. Return true if the build status has been changed.
// Note, it will not write the entities into Datastore.
func updateBuildFromTaskResult(ctx context.Context, bld *model.Build, infra *model.BuildInfra, taskResult *swarming.SwarmingRpcsTaskResult) (bool, error) {
	if protoutil.IsEnded(bld.Status) {
		// Completed builds are immutable.
		return false, nil
	}

	oldStatus := bld.Status
	sw := infra.Proto.Swarming
	// Update BotDimensions
	sw.BotDimensions = []*pb.StringPair{}
	for _, dim := range taskResult.BotDimensions {
		for _, v := range dim.Value {
			sw.BotDimensions = append(sw.BotDimensions, &pb.StringPair{Key: dim.Key, Value: v})
		}
	}
	sort.Slice(sw.BotDimensions, func(i, j int) bool {
		if sw.BotDimensions[i].Key == sw.BotDimensions[j].Key {
			return sw.BotDimensions[i].Value < sw.BotDimensions[j].Value
		} else {
			return sw.BotDimensions[i].Key < sw.BotDimensions[j].Key
		}
	})

	now := clock.Now(ctx)
	// A helper function to correctly set Build ended status from taskResult. It
	// corrects the build start_time only if start_time is empty and taskResult
	// has start_ts populated.
	setEndStatus := func(st pb.Status) {
		if !protoutil.IsEnded(st) {
			return
		}
		if bld.Proto.StartTime == nil {
			if startTime, err := time.Parse(swarmingTimeFormat, taskResult.StartedTs); err == nil {
				protoutil.SetStatus(startTime, bld.Proto, pb.Status_STARTED)
			}
		}
		endTime := now
		if t, err := time.Parse(swarmingTimeFormat, taskResult.CompletedTs); err == nil {
			endTime = t
		} else if t, err := time.Parse(swarmingTimeFormat, taskResult.AbandonedTs); err == nil {
			endTime = t
		}
		// It is possible that swarming task was marked as NO_RESOURCE the moment
		// it was created. Swarming VM time is not synchronized with buildbucket VM
		// time, so adjust end_time if needed.
		if endTime.Before(bld.Proto.CreateTime.AsTime()) {
			endTime = bld.Proto.CreateTime.AsTime()
		}

		protoutil.SetStatus(endTime, bld.Proto, st)
	}

	// Update build status
	switch taskResult.State {
	case "PENDING":
		if bld.Status == pb.Status_STARTED {
			// Most probably, race between PubSub push handler and Cron job.
			// With swarming, a build cannot go from STARTED back to PENDING,
			// so ignore this.
			return false, nil
		}
		protoutil.SetStatus(now, bld.Proto, pb.Status_SCHEDULED)
	case "RUNNING":
		if startTime, err := time.Parse(swarmingTimeFormat, taskResult.StartedTs); err == nil {
			protoutil.SetStatus(startTime, bld.Proto, pb.Status_STARTED)
		} else {
			protoutil.SetStatus(now, bld.Proto, pb.Status_STARTED)
		}
	case "CANCELED", "KILLED":
		setEndStatus(pb.Status_CANCELED)
	case "NO_RESOURCE":
		setEndStatus(pb.Status_INFRA_FAILURE)
		bld.Proto.StatusDetails = &pb.StatusDetails{
			ResourceExhaustion: &pb.StatusDetails_ResourceExhaustion{},
		}
	case "EXPIRED":
		setEndStatus(pb.Status_INFRA_FAILURE)
		bld.Proto.StatusDetails = &pb.StatusDetails{
			ResourceExhaustion: &pb.StatusDetails_ResourceExhaustion{},
			Timeout:            &pb.StatusDetails_Timeout{},
		}
	case "TIMED_OUT":
		setEndStatus(pb.Status_INFRA_FAILURE)
		bld.Proto.StatusDetails = &pb.StatusDetails{
			Timeout: &pb.StatusDetails_Timeout{},
		}
	case "BOT_DIED", "CLIENT_ERROR":
		// BB only supplies bbagent CIPD packages in a task, no other user packages.
		// So the CLIENT_ERROR task state should be treated as build INFRA_FAILURE.
		setEndStatus(pb.Status_INFRA_FAILURE)
	case "COMPLETED":
		if taskResult.Failure {
			// If this truly was a non-infra failure, bbagent would catch that and
			// mark the build as FAILURE.
			// That did not happen, so this is an infra failure.
			setEndStatus(pb.Status_INFRA_FAILURE)
		} else {
			setEndStatus(pb.Status_SUCCESS)
		}
	default:
		return false, errors.Reason("Unexpected task state: %s", taskResult.State).Err()
	}
	if bld.Proto.Status == oldStatus {
		return false, nil
	}
	logging.Infof(ctx, "Build %s status: %s -> %s", bld.ID, oldStatus, bld.Proto.Status)
	return true, nil
}

// createSwarmingTask creates a swarming task for the build.
// Requires build.proto.infra to be populated.
// If the returned error is fatal and non-retryable, the tq.Fatal tag will be added.
func createSwarmingTask(ctx context.Context, build *model.Build, swarm clients.SwarmingClient) error {
	taskReq, err := computeSwarmingNewTaskReq(ctx, build)
	if err != nil {
		return tq.Fatal.Apply(err)
	}

	// Insert secret bytes.
	token, err := buildtoken.GenerateToken(build.ID)
	if err != nil {
		return tq.Fatal.Apply(err)
	}
	secrets := &pb.BuildSecrets{
		BuildToken:                    token,
		ResultdbInvocationUpdateToken: build.ResultDBUpdateToken,
	}
	secretsBytes, err := proto.Marshal(secrets)
	if err != nil {
		return tq.Fatal.Apply(err)
	}
	for _, t := range taskReq.TaskSlices {
		t.Properties.SecretBytes = base64.RawURLEncoding.EncodeToString(secretsBytes)
	}

	// Create a swarming task
	res, err := swarm.CreateTask(ctx, taskReq)
	if err != nil {
		// Give up if HTTP 500s are happening continuously. Otherwise re-throw the
		// error so Cloud Tasks retries the task.
		if apiErr, _ := err.(*googleapi.Error); apiErr == nil || apiErr.Code >= 500 {
			if clock.Now(ctx).Sub(build.CreateTime) < swarmingCreateTaskGiveUpTimeout {
				return transient.Tag.Apply(errors.Annotate(err, "failed to create a swarming task").Err())
			}
			logging.Errorf(ctx, "Give up Swarming task creation retry after %s", swarmingCreateTaskGiveUpTimeout.String())
		}
		// Strip out secret bytes and dump the task definition to the log.
		for _, t := range taskReq.TaskSlices {
			t.Properties.SecretBytes = ""
		}
		logging.Errorf(ctx, "Swarming task creation failure:%s. CreateTask request: %+v\nResponse: %+v", err, taskReq, res)
		return tq.Fatal.Apply(errors.Annotate(err, "failed to create a swarming task").Err())
	}

	// Update the build with the build token and new task id.
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		bld := &model.Build{
			ID: build.ID,
		}
		infra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, bld)}
		if err := datastore.Get(ctx, bld, infra); err != nil {
			return errors.Annotate(err, "failed to fetch build or buildInfra: %d", bld.ID).Err()
		}

		if infra.Proto.Swarming.TaskId != "" {
			return errors.Reason("build already has a task %s", infra.Proto.Swarming.TaskId).Err()
		}
		infra.Proto.Swarming.TaskId = res.TaskId
		bld.UpdateToken = token

		return datastore.Put(ctx, bld, infra)
	}, nil)
	if err != nil {
		logging.Errorf(ctx, "created a task, but failed to update datastore with the error:%s \n"+
			"cancelling task %s, best effort", err, res.TaskId)
		if err := CancelSwarmingTask(ctx, &taskdefs.CancelSwarmingTask{
			Hostname: build.Proto.Infra.Swarming.Hostname,
			TaskId:   res.TaskId,
			Realm:    build.Realm(),
		}); err != nil {
			return transient.Tag.Apply(errors.Annotate(err, "failed to enqueue swarming task cancellation task for build %d", build.ID).Err())
		}
		return transient.Tag.Apply(errors.Annotate(err, "failed to update build %d", build.ID).Err())
	}
	return nil
}

// failBuild fails the given build with INFRA_FAILURE status.
func failBuild(ctx context.Context, buildID int64, msg string) error {
	bld := &model.Build{
		ID: buildID,
	}

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		switch err := datastore.Get(ctx, bld); {
		case err == datastore.ErrNoSuchEntity:
			logging.Warningf(ctx, "build %d not found: %s", buildID, err)
			return nil
		case err != nil:
			return transient.Tag.Apply(errors.Annotate(err, "failed to fetch build: %d", bld.ID).Err())
		}
		protoutil.SetStatus(clock.Now(ctx), bld.Proto, pb.Status_INFRA_FAILURE)
		bld.Proto.SummaryMarkdown = msg

		if err := sendOnBuildCompletion(ctx, bld); err != nil {
			return transient.Tag.Apply(err)
		}

		return datastore.Put(ctx, bld)
	}, nil)
	if err != nil {
		return transient.Tag.Apply(errors.Annotate(err, "failed to terminate build: %d", buildID).Err())
	}
	metrics.BuildCompleted(ctx, bld)
	return nil
}

// sendOnBuildCompletion sends a bunch of related events when build is reaching
// to an end status, e.g. finalizing the resultdb invocation, exporting to Bq,
// and notify pubsub topics.
func sendOnBuildCompletion(ctx context.Context, bld *model.Build) error {
	if err := FinalizeResultDB(ctx, &taskdefs.FinalizeResultDB{
		BuildId: bld.ID,
	}); err != nil {
		return errors.Annotate(err, "failed to enqueue resultDB finalization task: %d", bld.ID).Err()
	}
	if err := ExportBigQuery(ctx, &taskdefs.ExportBigQuery{
		BuildId: bld.ID,
	}); err != nil {
		return errors.Annotate(err, "failed to enqueue bigquery export task: %d", bld.ID).Err()
	}
	if err := NotifyPubSub(ctx, bld); err != nil {
		return errors.Annotate(err, "failed to enqueue pubsub notification task: %d", bld.ID).Err()
	}
	return nil
}

func computeSwarmingNewTaskReq(ctx context.Context, build *model.Build) (*swarming.SwarmingRpcsNewTaskRequest, error) {
	sw := build.Proto.GetInfra().GetSwarming()
	if sw == nil {
		return nil, errors.New("build.Proto.Infra.Swarming isn't set")
	}
	taskReq := &swarming.SwarmingRpcsNewTaskRequest{
		// to prevent accidental multiple task creation
		RequestUuid: uuid.NewSHA1(uuid.Nil, []byte(strconv.FormatInt(build.ID, 10))).String(),
		Name:        fmt.Sprintf("bb-%d-%s", build.ID, build.BuilderID),
		Realm:       build.Realm(),
		Tags:        computeTags(ctx, build),
		Priority:    int64(sw.Priority),
	}

	if build.Proto.Number > 0 {
		taskReq.Name = fmt.Sprintf("%s-%d", taskReq.Name, build.Proto.Number)
	}

	taskSlices, err := computeTaskSlice(build)
	if err != nil {
		errors.Annotate(err, "failed to computing task slices").Err()
	}
	taskReq.TaskSlices = taskSlices

	// Only makes swarming to track the build's parent if Buildbucket doesn't
	// track.
	// Buildbucket should track the parent/child build relationships for all
	// Buildbucket Builds.
	// Except for children of led builds, whose parents are still tracked by
	// swarming using sw.parent_run_id.
	// TODO(crbug.com/1031205): remove the check on
	// luci.buildbucket.parent_tracking after this experiment is on globally and
	// we're ready to remove it.
	if sw.ParentRunId != "" && (len(build.Proto.AncestorIds) == 0 ||
		strings.Contains(build.ExperimentsString(), buildbucket.ExperimentParentTracking)) {
		taskReq.ParentTaskId = sw.ParentRunId
	}

	if sw.TaskServiceAccount != "" {
		taskReq.ServiceAccount = sw.TaskServiceAccount
	}

	taskReq.PubsubTopic = fmt.Sprintf(pubsubTopicTemplate, info.AppID(ctx))
	taskReq.PubsubUserdata = fmt.Sprintf(pubSubUserDataTemplate, build.ID, clock.Now(ctx).UnixNano()/1000, sw.Hostname)

	return taskReq, err
}

// computeTags computes the Swarming task request tags to use.
// Note it doesn't compute kitchen related tags.
func computeTags(ctx context.Context, build *model.Build) []string {
	tags := []string{
		"buildbucket_bucket:" + build.BucketID,
		fmt.Sprintf("buildbucket_build_id:%d", build.ID),
		fmt.Sprintf("buildbucket_hostname:%s.appspot.com", info.AppID(ctx)),
		"luci_project:" + build.Project,
	}
	if build.Canary {
		tags = append(tags, "buildbucket_template_canary:1")
	} else {
		tags = append(tags, "buildbucket_template_canary:0")
	}

	tags = append(tags, build.Tags...)
	sort.Strings(tags)
	return tags
}

// computeTaskSlice computes swarming task slices.
// build.Proto.Infra must be set.
func computeTaskSlice(build *model.Build) ([]*swarming.SwarmingRpcsTaskSlice, error) {
	// expiration_secs -> []*SwarmingRpcsStringPair
	dims := map[int64][]*swarming.SwarmingRpcsStringPair{}
	for _, cache := range build.Proto.GetInfra().GetSwarming().GetCaches() {
		expSecs := cache.WaitForWarmCache.GetSeconds()
		if _, ok := dims[expSecs]; !ok {
			dims[expSecs] = []*swarming.SwarmingRpcsStringPair{}
		}
		dims[expSecs] = append(dims[expSecs], &swarming.SwarmingRpcsStringPair{
			Key:   "caches",
			Value: cache.Name,
		})
	}
	for _, dim := range build.Proto.GetInfra().GetSwarming().GetTaskDimensions() {
		expSecs := dim.Expiration.GetSeconds()
		if _, ok := dims[expSecs]; !ok {
			dims[expSecs] = []*swarming.SwarmingRpcsStringPair{}
		}
		dims[expSecs] = append(dims[expSecs], &swarming.SwarmingRpcsStringPair{
			Key:   dim.Key,
			Value: dim.Value,
		})
	}

	// extract base dim and delete it from the map.
	baseDim, ok := dims[0]
	if !ok {
		baseDim = []*swarming.SwarmingRpcsStringPair{}
	}
	delete(dims, 0)
	if len(dims) > 6 {
		return nil, errors.New("At most 6 different expiration_secs to be allowed in swarming")
	}

	baseSlice := &swarming.SwarmingRpcsTaskSlice{
		ExpirationSecs:  build.Proto.GetSchedulingTimeout().GetSeconds(),
		WaitForCapacity: build.Proto.GetWaitForCapacity(),
		Properties: &swarming.SwarmingRpcsTaskProperties{
			CipdInput:            computeCipdInput(build),
			ExecutionTimeoutSecs: build.Proto.GetExecutionTimeout().GetSeconds(),
			GracePeriodSecs:      build.Proto.GetGracePeriod().GetSeconds() + bbagentReservedGracePeriod,
			Caches:               computeTaskSliceCaches(build),
			Dimensions:           baseDim,
			EnvPrefixes:          computeEnvPrefixes(build),
			Env: []*swarming.SwarmingRpcsStringPair{
				{Key: "BUILDBUCKET_EXPERIMENTAL", Value: strings.ToUpper(strconv.FormatBool(build.Experimental))},
			},
			Command: computeCommand(build),
		},
	}

	// sort dims map by expiration_sec.
	var expSecs []int
	for expSec := range dims {
		expSecs = append(expSecs, int(expSec))
	}
	sort.Ints(expSecs)

	// Create extra task slices by copying the base task slice. Adding the
	// corresponding expiration and desired dimensions
	lastExp := 0
	taskSlices := make([]*swarming.SwarmingRpcsTaskSlice, len(expSecs)+1)
	for i, sec := range expSecs {
		prop := &swarming.SwarmingRpcsTaskProperties{}
		if err := deepCopy(baseSlice.Properties, prop); err != nil {
			return nil, err
		}
		taskSlices[i] = &swarming.SwarmingRpcsTaskSlice{
			ExpirationSecs: int64(sec - lastExp),
			Properties:     prop,
		}
		// dims[i] should be added into all previous non-expired task slices.
		for j := 0; j <= i; j++ {
			taskSlices[j].Properties.Dimensions = append(taskSlices[j].Properties.Dimensions, dims[int64(sec)]...)
		}
		lastExp = sec
	}

	// Tweak expiration on the baseSlice, which is the last slice.
	exp := max(int(baseSlice.ExpirationSecs)-lastExp, 60)
	baseSlice.ExpirationSecs = int64(exp)
	taskSlices[len(taskSlices)-1] = baseSlice

	sortDim := func(strPairs []*swarming.SwarmingRpcsStringPair) {
		sort.Slice(strPairs, func(i, j int) bool {
			if strPairs[i].Key == strPairs[j].Key {
				return strPairs[i].Value < strPairs[j].Value
			}
			return strPairs[i].Key < strPairs[j].Key
		})
	}
	// sort dimensions in each task slice.
	for _, t := range taskSlices {
		sortDim(t.Properties.Dimensions)
	}
	return taskSlices, nil
}

// computeTaskSliceCaches computes the task slice caches.
func computeTaskSliceCaches(build *model.Build) []*swarming.SwarmingRpcsCacheEntry {
	caches := make([]*swarming.SwarmingRpcsCacheEntry, len(build.Proto.Infra.Swarming.GetCaches()))
	for i, c := range build.Proto.Infra.Swarming.GetCaches() {
		caches[i] = &swarming.SwarmingRpcsCacheEntry{
			Name: c.Name,
			Path: filepath.Join(cacheDir, c.Path),
		}
	}
	return caches
}

// computeCipdInput returns swarming task CIPD input.
// Note: this function only considers v2 bbagent builds.
// The build.Proto.Infra.Buildbucket.Agent.Source must be set
func computeCipdInput(build *model.Build) *swarming.SwarmingRpcsCipdInput {
	return &swarming.SwarmingRpcsCipdInput{
		Packages: []*swarming.SwarmingRpcsCipdPackage{{
			PackageName: build.Proto.GetInfra().GetBuildbucket().GetAgent().GetSource().GetCipd().GetPackage(),
			Version:     build.Proto.GetInfra().GetBuildbucket().GetAgent().GetSource().GetCipd().GetVersion(),
			Path:        ".",
		}},
	}
}

// computeEnvPrefixes returns env_prefixes key in swarming properties.
// Note: this function only considers v2 bbagent builds.
func computeEnvPrefixes(build *model.Build) []*swarming.SwarmingRpcsStringListPair {
	prefixesMap := map[string][]string{}
	for _, c := range build.Proto.GetInfra().GetSwarming().GetCaches() {
		if c.EnvVar != "" {
			if _, ok := prefixesMap[c.EnvVar]; !ok {
				prefixesMap[c.EnvVar] = []string{}
			}
			prefixesMap[c.EnvVar] = append(prefixesMap[c.EnvVar], filepath.Join(cacheDir, c.Path))
		}
	}
	var keys []string
	for key := range prefixesMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	prefixes := make([]*swarming.SwarmingRpcsStringListPair, len(keys))
	for i, key := range keys {
		prefixes[i] = &swarming.SwarmingRpcsStringListPair{
			Key:   key,
			Value: prefixesMap[key],
		}
	}
	return prefixes
}

// computeCommand computes the command for bbagent.
func computeCommand(build *model.Build) []string {
	bbagentGetBuildEnabled := false
	for _, exp := range build.Experiments {
		if exp == buildbucket.ExperimentBBAgentGetBuild {
			bbagentGetBuildEnabled = true
			break
		}
	}

	if bbagentGetBuildEnabled {
		return []string{
			"bbagent${EXECUTABLE_SUFFIX}",
			"-host",
			build.Proto.GetInfra().GetBuildbucket().GetHostname(),
			"-build-id",
			strconv.FormatInt(build.ID, 10),
		}
	}

	return []string{
		"bbagent${EXECUTABLE_SUFFIX}",
		bbinput.Encode(&pb.BBAgentArgs{
			Build:                  build.Proto,
			CacheDir:               build.Proto.GetInfra().GetBbagent().GetCacheDir(),
			KnownPublicGerritHosts: build.Proto.GetInfra().GetBuildbucket().GetKnownPublicGerritHosts(),
			PayloadPath:            build.Proto.GetInfra().GetBbagent().GetPayloadPath(),
		}),
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// deepCopy deep copies src to dst using json marshaling for non-proto messages.
func deepCopy(src, dst interface{}) error {
	srcBytes, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(srcBytes, dst)
}
