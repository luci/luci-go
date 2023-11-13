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
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/tq"
	apipb "go.chromium.org/luci/swarming/proto/api_v2"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildstatus"
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
	// cacheDir is the path, relative to the swarming run dir, to the directory that
	// contains the mounted swarming named caches. It will be prepended to paths of
	// caches defined in global or builder configs.
	cacheDir = "cache"

	// pubsubTopicTemplate is the topic template where Swarming publishes
	// notifications on the task update.
	pubsubTopicTemplate = "projects/%s/topics/swarming-go"

	// swarmingCreateTaskGiveUpTimeout indicates how long to retry
	// the createSwarmingTask before giving up with INFRA_FAILURE.
	swarmingCreateTaskGiveUpTimeout = 10 * 60 * time.Second

	// swarmingTimeFormat is time format used by swarming.
	swarmingTimeFormat = "2006-01-02T15:04:05.999999999"
)

// userdata will be sent back and forth between Swarming and Buildbucket.
type userdata struct {
	BuildID          int64  `json:"build_id"`
	CreatedTS        int64  `json:"created_ts"`
	SwarmingHostname string `json:"swarming_hostname"`
}

// notification captures all fields that Buildbucket needs from the message of Swarming notification subscription.
type notification struct {
	messageID string
	taskID    string
	*userdata
}

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
	if clock.Now(ctx).Sub(bld.CreateTime) > model.BuildMaxCompletionTime {
		logging.Infof(ctx, "build %d (create_time:%s) has passed the sync deadline: %s", buildID, bld.CreateTime, model.BuildMaxCompletionTime)
		return nil
	}

	bld.Proto.Infra = infra.Proto
	swarm, err := clients.NewSwarmingClient(ctx, infra.Proto.Swarming.Hostname, bld.Project)
	if err != nil {
		logging.Errorf(ctx, "failed to create a swarming client for build %d (%s), in %s: %s", buildID, bld.Project, infra.Proto.Swarming.Hostname, err)
		return failBuild(ctx, buildID, fmt.Sprintf("failed to create a swarming client:%s", err))
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

	// Enqueue a continuation sync task in 5m.
	if clock.Now(ctx).Sub(bld.CreateTime) < model.BuildMaxCompletionTime {
		if err := SyncSwarmingBuildTask(ctx, &taskdefs.SyncSwarmingBuildTask{BuildId: buildID, Generation: generation + 1}, 5*time.Minute); err != nil {
			return transient.Tag.Apply(errors.Annotate(err, "failed to enqueue the continuation sync task for build %d", buildID).Err())
		}
	}
	return nil
}

// SubNotify handles swarming-go PubSub push messages produced by Swarming.
// For a retryable error, it will be tagged with transient.Tag.
func SubNotify(ctx context.Context, body io.Reader) error {
	nt, err := unpackMsg(ctx, body)
	if err != nil {
		return err
	}
	// TODO(crbug/1328646): delete the log once the new Go flow becomes stable.
	logging.Infof(ctx, "Received message - messageID:%s, taskID:%s, userdata:%+v", nt.messageID, nt.taskID, nt.userdata)

	// Try not to process same message more than once.
	cache := caching.GlobalCache(ctx, "swarming-pubsub-msg-id")
	if cache == nil {
		return errors.Reason("global cache is not found").Tag(transient.Tag).Err()
	}
	msgCached, err := cache.Get(ctx, nt.messageID)
	switch {
	case err == caching.ErrCacheMiss: // no-op, continue
	case err != nil:
		return errors.Annotate(err, "failed to read %s from the global cache", nt.messageID).Tag(transient.Tag).Err()
	case msgCached != nil:
		logging.Infof(ctx, "seen this message %s before, ignoring", nt.messageID)
		return nil
	}

	taskURL := func(hostname, taskID string) string {
		return fmt.Sprintf("https://%s/task?id=%s", hostname, taskID)
	}
	// load build and build infra.
	logging.Infof(ctx, "received swarming notification for build %d", nt.BuildID)
	bld := &model.Build{ID: nt.BuildID}
	infra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, bld)}
	switch err := datastore.Get(ctx, bld, infra); {
	case errors.Contains(err, datastore.ErrNoSuchEntity):
		if clock.Now(ctx).Sub(time.Unix(0, nt.CreatedTS*int64(time.Microsecond)).UTC()) < time.Minute {
			return errors.Annotate(err, "Build %d or BuildInfra for task %s not found yet", nt.BuildID, taskURL(nt.SwarmingHostname, nt.taskID)).Tag(transient.Tag).Err()
		}
		return errors.Annotate(err, "Build %d or BuildInfra for task %s not found", nt.BuildID, taskURL(nt.SwarmingHostname, nt.taskID)).Err()
	case err != nil:
		return errors.Annotate(err, "failed to fetch build %d or buildInfra", nt.BuildID).Tag(transient.Tag).Err()
	}
	if protoutil.IsEnded(bld.Status) {
		logging.Infof(ctx, "build(%d) is completed and immutable.", nt.BuildID)
		return nil
	}

	// ensure the loaded build is associated with the task.
	bld.Proto.Infra = infra.Proto
	sw := bld.Proto.GetInfra().GetSwarming()
	if nt.SwarmingHostname != sw.GetHostname() {
		return errors.Reason("swarming_hostname %s of build %d does not match %s", sw.Hostname, nt.BuildID, nt.SwarmingHostname).Err()
	}
	if strings.TrimSpace(sw.GetTaskId()) == "" {
		return errors.Reason("build %d is not associated with a task", nt.BuildID).Tag(transient.Tag).Err()
	}
	if nt.taskID != sw.GetTaskId() {
		return errors.Reason("swarming_task_id %s of build %d does not match %s", sw.TaskId, nt.BuildID, nt.taskID).Err()
	}

	// update build.
	swarm, err := clients.NewSwarmingClient(ctx, sw.Hostname, bld.Project)
	if err != nil {
		return errors.Annotate(err, "failed to create a swarming client for build %d (%s), in %s", nt.BuildID, bld.Project, sw.Hostname).Err()
	}
	if err := syncBuildWithTaskResult(ctx, nt.BuildID, sw.TaskId, swarm); err != nil {
		return err
	}

	return cache.Set(ctx, nt.messageID, []byte{1}, 10*time.Minute)
}

func HandleCancelSwarmingTask(ctx context.Context, hostname string, taskID string, realm string) error {
	// Validate
	switch err := realms.ValidateRealmName(realm, realms.GlobalScope); {
	case err != nil:
		return tq.Fatal.Apply(err)
	case hostname == "":
		return errors.Reason("hostname is empty").Tag(tq.Fatal).Err()
	case taskID == "":
		return errors.Reason("taskID is empty").Tag(tq.Fatal).Err()
	}

	// Send the cancellation request.
	project, _ := realms.Split(realm)
	swarm, err := clients.NewSwarmingClient(ctx, hostname, project)
	if err != nil {
		return errors.Annotate(err, "failed to create a swarming client for task %s in %s", taskID, hostname).Tag(tq.Fatal).Err()
	}
	res, err := swarm.CancelTask(ctx, &apipb.TaskCancelRequest{KillRunning: true, TaskId: taskID})
	if err != nil {
		if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code >= 500 {
			return errors.Annotate(err, "transient error in cancelling the task %s", taskID).Tag(transient.Tag).Err()
		}
		return errors.Annotate(err, "fatal error in cancelling the task %s", taskID).Tag(tq.Fatal).Err()
	}

	// Non-Canceled in the body indicates the task may have already ended. Hence, just logging it.
	if !res.Canceled {
		logging.Warningf(ctx, "Swarming response for cancelling task %s: %+v", taskID, res)
	}
	return nil
}

// unpackMsg unpacks swarming-go pubsub message and extracts message id,
// swarming hostname, creation time, task id and build id.
func unpackMsg(ctx context.Context, body io.Reader) (*notification, error) {
	blob, err := io.ReadAll(body)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read the request body").Tag(transient.Tag).Err()
	}

	// process pubsub message
	// See https://cloud.google.com/pubsub/docs/push#receive_push
	var msg struct {
		Message struct {
			Attributes map[string]string `json:"attributes,omitempty"`
			Data       string            `json:"data,omitempty"`
			MessageID  string            `json:"messageId,omitempty"`
		} `json:"message"`
	}
	if err := json.Unmarshal(blob, &msg); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal swarming PubSub message").Err()
	}

	// process swarming message data
	swarmData, err := base64.StdEncoding.DecodeString(msg.Message.Data)
	if err != nil {
		return nil, errors.Annotate(err, "cannot decode message data as base64").Err()
	}
	var data struct {
		TaskID   string `json:"task_id"`
		Userdata string `json:"userdata"`
	}
	if err := json.Unmarshal(swarmData, &data); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal the swarming pubsub data").Err()
	}
	ud := &userdata{}
	if err := json.Unmarshal([]byte(data.Userdata), ud); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal userdata").Err()
	}

	// validate swarming message data
	switch {
	case strings.TrimSpace(data.TaskID) == "":
		return nil, errors.Reason("task_id not found in message data").Err()
	case ud.BuildID <= 0:
		return nil, errors.Reason("invalid build_id %d", ud.BuildID).Err()
	case ud.CreatedTS <= 0:
		return nil, errors.Reason("invalid created_ts %d", ud.CreatedTS).Err()
	case strings.TrimSpace(ud.SwarmingHostname) == "":
		return nil, errors.Reason("swarming hostname not found in userdata").Err()
	case strings.Contains(ud.SwarmingHostname, "://"):
		return nil, errors.Reason("swarming hostname %s must not contain '://'", ud.SwarmingHostname).Err()
	}

	return &notification{
		messageID: msg.Message.MessageID,
		taskID:    data.TaskID,
		userdata:  ud,
	}, nil
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

	var statusChanged bool
	bld := &model.Build{
		ID: buildID,
	}
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		infra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, bld)}
		if err := datastore.Get(ctx, bld, infra); err != nil {
			return transient.Tag.Apply(errors.Annotate(err, "failed to fetch build or buildInfra: %d", bld.ID).Err())
		}

		if protoutil.IsEnded(bld.Status) {
			return nil
		}
		if bld.Status == pb.Status_STARTED && taskResult.State == apipb.TaskState_PENDING {
			// Most probably, race between PubSub push handler and Cron job.
			// With swarming, a build cannot go from STARTED back to PENDING,
			// so ignore this.
			return nil
		}

		botDimsChanged := updateBotDimensions(infra, taskResult)

		bs, steps, err := updateBuildStatusFromTaskResult(ctx, bld, taskResult)
		if err != nil {
			return tq.Fatal.Apply(err)
		}

		shouldUpdate := false
		if bs != nil {
			shouldUpdate = true
			statusChanged = true
		}
		if bs == nil && botDimsChanged && bld.Proto.Status == pb.Status_STARTED {
			shouldUpdate = true
		}
		if !shouldUpdate {
			return nil
		}

		toPut := []any{bld, infra}
		if bs != nil {
			toPut = append(toPut, bs)
		}
		if steps != nil {
			toPut = append(toPut, steps)
		}
		return transient.Tag.Apply(datastore.Put(ctx, toPut...))
	}, nil)

	switch {
	case err != nil:
	case !statusChanged:
	case bld.Status == pb.Status_STARTED:
		metrics.BuildStarted(ctx, bld)
	case protoutil.IsEnded(bld.Status):
		metrics.BuildCompleted(ctx, bld)
	}
	return err
}

// updateBotDimensions mutates the infra entity to update the bot dimensions
// according to the given task result.
// Note, it will not write the entities into Datastore.
func updateBotDimensions(infra *model.BuildInfra, taskResult *apipb.TaskResultResponse) bool {
	sw := infra.Proto.Swarming
	botDimsChanged := false

	// Update BotDimensions
	oldBotDimsMap := protoutil.StringPairMap(sw.BotDimensions)
	newBotDims := []*pb.StringPair{}
	for _, dim := range taskResult.BotDimensions {
		for _, v := range dim.Value {
			if !botDimsChanged && !oldBotDimsMap.Contains(dim.Key, v) {
				botDimsChanged = true
			}
			newBotDims = append(newBotDims, &pb.StringPair{Key: dim.Key, Value: v})
		}
	}
	if len(newBotDims) != len(sw.BotDimensions) {
		botDimsChanged = true
	}
	sw.BotDimensions = newBotDims

	sort.Slice(sw.BotDimensions, func(i, j int) bool {
		if sw.BotDimensions[i].Key == sw.BotDimensions[j].Key {
			return sw.BotDimensions[i].Value < sw.BotDimensions[j].Value
		}
		return sw.BotDimensions[i].Key < sw.BotDimensions[j].Key
	})
	return botDimsChanged
}

// updateBuildStatusFromTaskResult mutates the build entity to update the top
// level status, and also the update time of the build.
// Note, it will not write the entities into Datastore.
func updateBuildStatusFromTaskResult(ctx context.Context, bld *model.Build, taskResult *apipb.TaskResultResponse) (bs *model.BuildStatus, steps *model.BuildSteps, err error) {
	now := clock.Now(ctx)
	oldStatus := bld.Status
	// A helper function to correctly set Build ended status from taskResult. It
	// corrects the build start_time only if start_time is empty and taskResult
	// has start_ts populated.
	setEndStatus := func(st pb.Status, details *pb.StatusDetails) {
		if !protoutil.IsEnded(st) {
			return
		}
		if bld.Proto.StartTime == nil && taskResult.StartedTs.AsTime().Unix() != 0 {
			startTime := taskResult.StartedTs.AsTime()
			// Backfill build start time.
			protoutil.SetStatus(startTime, bld.Proto, pb.Status_STARTED)
		}

		endTime := now
		if t := taskResult.CompletedTs; t != nil {
			endTime = t.AsTime()
		} else if t := taskResult.AbandonedTs; t != nil {
			endTime = t.AsTime()
		}
		// It is possible that swarming task was marked as NO_RESOURCE the moment
		// it was created. Swarming VM time is not synchronized with buildbucket VM
		// time, so adjust end_time if needed.
		if endTime.Before(bld.Proto.CreateTime.AsTime()) {
			endTime = bld.Proto.CreateTime.AsTime()
		}

		stWithDetails := &buildstatus.StatusWithDetails{Status: st, Details: details}
		bs, steps, err = updateBuildStatusOnTaskStatusChange(ctx, bld, stWithDetails, stWithDetails, endTime)
	}

	// Update build status
	switch taskResult.State {
	case apipb.TaskState_PENDING:
		if bld.Status == pb.Status_STATUS_UNSPECIFIED {
			// Scheduled Build should have SCHEDULED status already, so in theory this
			// should not happen.
			// Adding a log to confirm this.
			logging.Debugf(ctx, "build %d has unspecified status, setting it to pending", bld.ID)
			protoutil.SetStatus(now, bld.Proto, pb.Status_SCHEDULED)
		} else {
			// Most probably, race between PubSub push handler and Cron job.
			// With swarming, a build cannot go from STARTED/ended back to PENDING,
			// so ignore this.
			return
		}
	case apipb.TaskState_RUNNING:
		updateTime := now
		if t := taskResult.StartedTs; t != nil {
			updateTime = t.AsTime()
		}
		stWithDetails := &buildstatus.StatusWithDetails{Status: pb.Status_STARTED}
		bs, steps, err = updateBuildStatusOnTaskStatusChange(ctx, bld, stWithDetails, stWithDetails, updateTime)
	case apipb.TaskState_CANCELED, apipb.TaskState_KILLED:
		setEndStatus(pb.Status_CANCELED, nil)
	case apipb.TaskState_NO_RESOURCE:
		setEndStatus(pb.Status_INFRA_FAILURE, &pb.StatusDetails{
			ResourceExhaustion: &pb.StatusDetails_ResourceExhaustion{},
		})
	case apipb.TaskState_EXPIRED:
		setEndStatus(pb.Status_INFRA_FAILURE, &pb.StatusDetails{
			ResourceExhaustion: &pb.StatusDetails_ResourceExhaustion{},
			Timeout:            &pb.StatusDetails_Timeout{},
		})
	case apipb.TaskState_TIMED_OUT:
		setEndStatus(pb.Status_INFRA_FAILURE, &pb.StatusDetails{
			Timeout: &pb.StatusDetails_Timeout{},
		})
	case apipb.TaskState_BOT_DIED, apipb.TaskState_CLIENT_ERROR:
		// BB only supplies bbagent CIPD packages in a task, no other user packages.
		// So the CLIENT_ERROR task state should be treated as build INFRA_FAILURE.
		setEndStatus(pb.Status_INFRA_FAILURE, nil)
	case apipb.TaskState_COMPLETED:
		if taskResult.Failure {
			switch bld.Proto.Output.GetStatus() {
			case pb.Status_FAILURE:
				setEndStatus(pb.Status_FAILURE, nil)
			case pb.Status_CANCELED:
				setEndStatus(pb.Status_CANCELED, nil)
			default:
				//If this truly was a non-infra failure, bbagent would catch that and
				//mark the build as FAILURE.
				//That did not happen, so this is an infra failure.
				setEndStatus(pb.Status_INFRA_FAILURE, nil)
			}
		} else {
			finalStatus := pb.Status_SUCCESS
			if protoutil.IsEnded(bld.Proto.Output.GetStatus()) {
				// Swarming task ends with COMPLETED(SUCCESS), use the build status
				// as final status.
				finalStatus = bld.Proto.Output.GetStatus()
			}
			setEndStatus(finalStatus, nil)
		}
	default:
		err = errors.Reason("Unexpected task state: %s", taskResult.State).Err()
		return
	}

	if bld.Proto.Status != oldStatus {
		logging.Infof(ctx, "Build %d status: %s -> %s", bld.ID, oldStatus, bld.Proto.Status)
		return
	}
	return
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
	token, err := buildtoken.GenerateToken(ctx, build.ID, pb.TokenBody_BUILD)
	if err != nil {
		return tq.Fatal.Apply(err)
	}
	secrets := &pb.BuildSecrets{
		StartBuildToken:               token,
		BuildToken:                    token,
		ResultdbInvocationUpdateToken: build.ResultDBUpdateToken,
	}
	secretsBytes, err := proto.Marshal(secrets)
	if err != nil {
		return tq.Fatal.Apply(err)
	}
	for _, t := range taskReq.TaskSlices {
		t.Properties.SecretBytes = secretsBytes
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
			t.Properties.SecretBytes = nil
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
		// now that swarm.CreateTask is idempotent, we should reuse the task,
		// instead of cancelling it.
		logging.Errorf(ctx, "created a task %s, but failed to update datastore with the error:%s", res.TaskId, err)
		return transient.Tag.Apply(errors.Annotate(err, "failed to update build %d", build.ID).Err())
	}
	return nil
}

func computeSwarmingNewTaskReq(ctx context.Context, build *model.Build) (*apipb.NewTaskRequest, error) {
	sw := build.Proto.GetInfra().GetSwarming()
	if sw == nil {
		return nil, errors.New("build.Proto.Infra.Swarming isn't set")
	}
	taskReq := &apipb.NewTaskRequest{
		// to prevent accidental multiple task creation
		RequestUuid:      uuid.NewSHA1(uuid.Nil, []byte(strconv.FormatInt(build.ID, 10))).String(),
		Name:             fmt.Sprintf("bb-%d-%s", build.ID, build.BuilderID),
		Realm:            build.Realm(),
		Tags:             computeTags(ctx, build),
		Priority:         int32(sw.Priority),
		PoolTaskTemplate: apipb.NewTaskRequest_SKIP,
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
		!strings.Contains(build.ExperimentsString(), buildbucket.ExperimentParentTracking)) {
		taskReq.ParentTaskId = sw.ParentRunId
	}

	if sw.TaskServiceAccount != "" {
		taskReq.ServiceAccount = sw.TaskServiceAccount
	}

	taskReq.PubsubTopic = fmt.Sprintf(pubsubTopicTemplate, info.AppID(ctx))
	ud := &userdata{
		BuildID:          build.ID,
		CreatedTS:        clock.Now(ctx).UnixNano() / int64(time.Microsecond),
		SwarmingHostname: sw.Hostname,
	}
	udBytes, err := json.Marshal(ud)
	if err != nil {
		return nil, errors.Annotate(err, "failed to marshal pubsub userdata").Err()
	}
	taskReq.PubsubUserdata = string(udBytes)
	return taskReq, err
}

// computeTags computes the Swarming task request tags to use.
// Note it doesn't compute kitchen related tags.
func computeTags(ctx context.Context, build *model.Build) []string {
	tags := []string{
		"buildbucket_bucket:" + build.BucketID,
		fmt.Sprintf("buildbucket_build_id:%d", build.ID),
		fmt.Sprintf("buildbucket_hostname:%s", build.Proto.GetInfra().GetBuildbucket().GetHostname()),
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
func computeTaskSlice(build *model.Build) ([]*apipb.TaskSlice, error) {
	// expiration_secs -> []*StringPair
	dims := map[int64][]*apipb.StringPair{}
	for _, cache := range build.Proto.GetInfra().GetSwarming().GetCaches() {
		expSecs := cache.WaitForWarmCache.GetSeconds()
		if expSecs <= 0 {
			continue
		}
		if _, ok := dims[expSecs]; !ok {
			dims[expSecs] = []*apipb.StringPair{}
		}
		dims[expSecs] = append(dims[expSecs], &apipb.StringPair{
			Key:   "caches",
			Value: cache.Name,
		})
	}
	for _, dim := range build.Proto.GetInfra().GetSwarming().GetTaskDimensions() {
		expSecs := dim.Expiration.GetSeconds()
		if _, ok := dims[expSecs]; !ok {
			dims[expSecs] = []*apipb.StringPair{}
		}
		dims[expSecs] = append(dims[expSecs], &apipb.StringPair{
			Key:   dim.Key,
			Value: dim.Value,
		})
	}

	// extract base dim and delete it from the map.
	baseDim, ok := dims[0]
	if !ok {
		baseDim = []*apipb.StringPair{}
	}
	delete(dims, 0)
	if len(dims) > 6 {
		return nil, errors.New("At most 6 different expiration_secs to be allowed in swarming")
	}

	baseSlice := &apipb.TaskSlice{
		ExpirationSecs:  int32(build.Proto.GetSchedulingTimeout().GetSeconds()),
		WaitForCapacity: build.Proto.GetWaitForCapacity(),
		Properties: &apipb.TaskProperties{
			CipdInput:            computeCipdInput(build),
			ExecutionTimeoutSecs: int32(build.Proto.GetExecutionTimeout().GetSeconds()),
			GracePeriodSecs:      int32(build.Proto.GetGracePeriod().GetSeconds() + bbagentReservedGracePeriod),
			Caches:               computeTaskSliceCaches(build),
			Dimensions:           baseDim,
			EnvPrefixes:          computeEnvPrefixes(build),
			Env: []*apipb.StringPair{
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

	// TODO(vadimsh): Remove this when no longer needed, ETA Oct 2022. This is
	// used to load test Swarming's slice expiration mechanism.
	sliceWaitForCapacity := build.Proto.GetWaitForCapacity() &&
		strings.Contains(build.ExperimentsString(), buildbucket.ExperimentWaitForCapacity)

	// Create extra task slices by copying the base task slice. Adding the
	// corresponding expiration and desired dimensions
	lastExp := 0
	taskSlices := make([]*apipb.TaskSlice, len(expSecs)+1)
	for i, sec := range expSecs {
		prop := &apipb.TaskProperties{}
		if err := deepCopy(baseSlice.Properties, prop); err != nil {
			return nil, err
		}
		taskSlices[i] = &apipb.TaskSlice{
			WaitForCapacity: sliceWaitForCapacity,
			ExpirationSecs:  int32(sec - lastExp),
			Properties:      prop,
		}
		// dims[i] should be added into all previous non-expired task slices.
		for j := 0; j <= i; j++ {
			taskSlices[j].Properties.Dimensions = append(taskSlices[j].Properties.Dimensions, dims[int64(sec)]...)
		}
		lastExp = sec
	}

	// Tweak expiration on the baseSlice, which is the last slice.
	exp := max(int(baseSlice.ExpirationSecs)-lastExp, 60)
	baseSlice.ExpirationSecs = int32(exp)
	taskSlices[len(taskSlices)-1] = baseSlice

	sortDim := func(strPairs []*apipb.StringPair) {
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
func computeTaskSliceCaches(build *model.Build) []*apipb.CacheEntry {
	caches := make([]*apipb.CacheEntry, len(build.Proto.Infra.Swarming.GetCaches()))
	for i, c := range build.Proto.Infra.Swarming.GetCaches() {
		caches[i] = &apipb.CacheEntry{
			Name: c.Name,
			Path: filepath.Join(cacheDir, c.Path),
		}
	}
	return caches
}

// computeCipdInput returns swarming task CIPD input.
// Note: this function only considers v2 bbagent builds.
// The build.Proto.Infra.Buildbucket.Agent.Source must be set
func computeCipdInput(build *model.Build) *apipb.CipdInput {
	return &apipb.CipdInput{
		Packages: []*apipb.CipdPackage{{
			PackageName: build.Proto.GetInfra().GetBuildbucket().GetAgent().GetSource().GetCipd().GetPackage(),
			Version:     build.Proto.GetInfra().GetBuildbucket().GetAgent().GetSource().GetCipd().GetVersion(),
			Path:        ".",
		}},
	}
}

// computeEnvPrefixes returns env_prefixes key in swarming properties.
// Note: this function only considers v2 bbagent builds.
func computeEnvPrefixes(build *model.Build) []*apipb.StringListPair {
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
	prefixes := make([]*apipb.StringListPair, len(keys))
	for i, key := range keys {
		prefixes[i] = &apipb.StringListPair{
			Key:   key,
			Value: prefixesMap[key],
		}
	}
	return prefixes
}

// computeCommand computes the command for bbagent.
func computeCommand(build *model.Build) []string {
	if strings.Contains(build.ExperimentsString(), buildbucket.ExperimentBBAgentGetBuild) {
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
func deepCopy(src, dst any) error {
	srcBytes, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(srcBytes, dst)
}
