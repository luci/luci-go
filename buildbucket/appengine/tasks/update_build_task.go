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
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/pubsub/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildstatus"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

type pushRequest struct {
	Message      pubsub.PubsubMessage `json:"message"`
	Subscription string               `json:"subscription"`
}

type buildTaskUpdate struct {
	*pb.BuildTaskUpdate

	// Message id of the pubsub message that sent this request.
	msgID string

	// Subscription of the pubsub message that sent this request.
	subscription string
}

func unpackUpdateBuildTaskMsg(ctx context.Context, body io.Reader) (req buildTaskUpdate, err error) {
	req = buildTaskUpdate{}

	blob, err := io.ReadAll(body)
	if err != nil {
		return req, transient.Tag.Apply(errors.
			Fmt("failed to read the request body: %w", err))
	}

	// process pubsub message
	var msg pushRequest
	if err := json.Unmarshal(blob, &msg); err != nil {
		return req, errors.Fmt("failed to unmarshal UpdateBuildTask PubSub message: %w", err)
	}
	// process UpdateBuildTask message data
	data, err := base64.StdEncoding.DecodeString(msg.Message.Data)
	if err != nil {
		return req, errors.Fmt("cannot decode UpdateBuildTask message data as base64: %w", err)
	}

	bldTskUpdte := &pb.BuildTaskUpdate{}
	if err := (proto.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, bldTskUpdte); err != nil {
		return req, errors.Fmt("failed to unmarshal the BuildTaskUpdate pubsub data: %w", err)
	}
	req.BuildTaskUpdate = bldTskUpdte
	req.subscription = msg.Subscription
	req.msgID = msg.Message.MessageId
	return req, nil
}

func validateTaskStatus(taskStatus pb.Status, allowPending bool) error {
	switch taskStatus {
	case pb.Status_ENDED_MASK,
		pb.Status_STATUS_UNSPECIFIED:
		return errors.Fmt("task.status: invalid status %s for UpdateBuildTask", taskStatus)
	}

	if !allowPending && taskStatus == pb.Status_SCHEDULED {
		return errors.Fmt("task.status: invalid status %s for UpdateBuildTask", taskStatus)
	}

	if _, ok := pb.Status_value[taskStatus.String()]; !ok {
		return errors.Fmt("task.status: invalid status %s for UpdateBuildTask", taskStatus)
	}
	return nil
}

func validateTask(task *pb.Task, allowPending bool) error {
	if task.GetId().GetId() == "" {
		return errors.New("task.id: required")
	}
	if task.GetUpdateId() == 0 {
		return errors.New("task.UpdateId: required")
	}
	if err := validateTaskStatus(task.Status, allowPending); err != nil {
		return errors.Fmt("task.Status: %w", err)
	}
	detailsInKb := float64(len(task.GetDetails().String()) / 1024)
	if detailsInKb > 10 {
		return errors.New("task.details is greater than 10 kb")
	}
	return nil
}

// validateBuildTaskUpdate ensures that the build_id, task, status, and details
// are correctly set be sender.
func validateBuildTaskUpdate(req *pb.BuildTaskUpdate) error {
	if req.BuildId == "" {
		return errors.New("build_id required")
	}
	return validateTask(req.Task, false)
}

// validateBuildTask ensures that the taskID provided in the request matches
// the taskID that is stored in the build model. If there is no task associated
// with the build, an error is returned and the update message is lost.
func validateBuildTask(req *pb.BuildTaskUpdate, infra *model.BuildInfra) error {
	switch {
	case infra.Proto.GetBackend() == nil:
		return errors.Fmt("build %s does not support task backend", req.BuildId)
	case infra.Proto.Backend.GetTask().GetId().GetId() == "":
		return errors.New("no task is associated with the build")
	case infra.Proto.Backend.Task.Id.GetTarget() != req.Task.Id.GetTarget() || (infra.Proto.Backend.Task.Id.GetId() != "" && infra.Proto.Backend.Task.Id.GetId() != req.Task.Id.GetId()):
		return errors.New("TaskID in request does not match TaskID associated with build")
	}
	if protoutil.IsEnded(infra.Proto.Backend.Task.Status) {
		return errors.New("cannot update an ended task")
	}
	return nil
}

func backfillCanceledTask(ctx context.Context, build *model.Build, infra *model.BuildInfra, task *pb.Task) []any {
	switch {
	case task.UpdateId <= infra.Proto.Backend.Task.UpdateId:
		// Returning nil since there is no work to do here.
		// The task in the request is outdated.
		return nil
	case protoutil.IsEnded(infra.Proto.Backend.Task.Status):
		// Only need to backfill if the backend task in datastore is still
		// in running state.
		return nil
	case !protoutil.IsEnded(task.Status):
		// Only need to backfill the backend task to set it's end state.
		// We could bypass other intermidiate updates between the backend server
		// schedules the task cancelation and the task actually being canceled.
		return nil
	}
	logging.Infof(ctx, "Backfill build %d's backend task, setting it's state to %q", build.ID, task.Status)
	proto.Merge(infra.Proto.Backend.Task, task)
	toSave := []any{infra}
	return toSave
}

func prepareUpdate(ctx context.Context, build *model.Build, infra *model.BuildInfra, task *pb.Task, useTaskSuccess bool) ([]any, error) {
	if task.UpdateId <= infra.Proto.Backend.Task.UpdateId {
		// Returning nil since there is no work to do here.
		// The task in the request is outdated.
		return nil, nil
	}
	// Required fields to change
	now := clock.Now(ctx)
	build.Proto.UpdateTime = timestamppb.New(now)
	proto.Merge(infra.Proto.Backend.Task, task)

	toSave := []any{build, infra}

	bs, steps, err := updateBuildStatusOnTaskStatusChange(ctx, build, infra, nil, &buildstatus.StatusWithDetails{Status: task.Status, Details: task.StatusDetails}, now, useTaskSuccess)
	if err != nil {
		return nil, err
	}
	if bs != nil {
		toSave = append(toSave, bs)
	}
	if steps != nil {
		toSave = append(toSave, steps)
	}
	return toSave, nil
}

// updateTaskEntity updates a bunch of entities associated with a build.
//
// May return an appstatus NotFound error if some of them are missing. They
// should all exist.
func updateTaskEntity(ctx context.Context, req *pb.BuildTaskUpdate, buildID int64, useTaskSuccess bool) error {
	var build *model.Build
	setBuildToEnd := false

	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Reset in case of the transaction retry.
		build = nil
		setBuildToEnd = false

		entities, err := common.GetBuildEntities(ctx, buildID, model.BuildKind, model.BuildInfraKind)
		if err != nil {
			return errors.Fmt("invalid Build or BuildInfra: %w", err)
		}

		build = entities[0].(*model.Build)
		infra := entities[1].(*model.BuildInfra)
		var toSave []any
		switch {
		case build.Status == pb.Status_CANCELED:
			// It's possible that this build got forcefully cancelled by the
			// "cancel-build" internal task after the grace period.
			// The task sets the build to be CANCELED and enqueues a
			// CancelBackendTask task which will make a CancelTasks RPC to the
			// backend asynchronously - while still leaving the backend task to
			// running state.
			// So we should backfill the backend task here when the task is
			// finally canceled by the backend, even though the build already
			// ends.
			toSave = backfillCanceledTask(ctx, build, infra, req.Task)
		case protoutil.IsEnded(build.Status):
			// Otherwise cannot update an ended build.
			logging.Infof(ctx, "Build %d is ended", build.ID)
			return nil
		default:
			toSave, err = prepareUpdate(ctx, build, infra, req.Task, useTaskSuccess)
			if err != nil {
				return err
			}
			setBuildToEnd = protoutil.IsEnded(req.Task.Status)
		}

		if len(toSave) == 0 {
			return nil
		}
		return datastore.Put(ctx, toSave)
	}, nil)

	if txErr != nil {
		return txErr
	}

	if setBuildToEnd {
		metrics.BuildCompleted(ctx, build)
	}
	return nil
}

// updateBuildTask allows the Backend to preemptively update the
// status of the task (e.g. if it knows that the task has crashed, etc.).
func updateBuildTask(ctx context.Context, req buildTaskUpdate) error {
	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return transient.Tag.Apply(errors.Fmt("error fetching service config: %w", err))
	}

	target := req.GetTask().GetId().GetTarget()
	if target == "" {
		return errors.New("task.id.target not provided.")
	}

	var subscription string
	useTaskSuccess := false
	for _, backend := range globalCfg.Backends {
		if backend.Target == target {
			switch backend.Mode.(type) {
			case *pb.BackendSetting_LiteMode_:
				return errors.Fmt("backend target %s is in lite mode. The task update isn't supported", target)
			case *pb.BackendSetting_FullMode_:
				subscription = fmt.Sprintf("projects/%s/subscriptions/%s", info.AppID(ctx), backend.GetFullMode().GetPubsubId())
				useTaskSuccess = backend.GetFullMode().GetSucceedBuildIfTaskIsSucceeded()
			}
			break
		}
	}

	buildID, err := strconv.ParseInt(req.GetBuildId(), 10, 64)
	if err != nil {
		return errors.Fmt("bad build id: %w", err)
	}
	if subscription != req.subscription {
		return errors.Fmt("pubsub subscription: %s did not match the one configured for target %s", req.subscription, target)
	}
	if err := validateBuildTaskUpdate(req.BuildTaskUpdate); err != nil {
		return errors.Fmt("invalid BuildTaskUpdate: %w", err)
	}
	logging.Infof(ctx, "Received an BuildTaskUpdate message for build %q", req.BuildId)

	// TODO(b/288158829): remove it once the root cause for the Skia failure is found.
	if strings.Contains(req.subscription, "skia") {
		logging.Debugf(ctx, "BuildTaskUpdate.Task: %v", req.BuildTaskUpdate.Task)
	}

	entities, err := common.GetBuildEntities(ctx, buildID, model.BuildInfraKind)
	if err != nil {
		if status, _ := appstatus.Get(err); status.Code() == codes.NotFound {
			return errors.New("missing BuildInfra entity")
		}
		return transient.Tag.Apply(errors.Fmt("failed to fetch BuildInfra: %w", err))
	}
	infra := entities[0].(*model.BuildInfra)

	// Pre-check if the task can be updated before updating it with a transaction.
	// Ensures that the taskID provided in the request matches the taskID that is
	// stored in the build model. If there is no task associated with the build model,
	// an error is returned and the update message is lost.
	err = validateBuildTask(req.BuildTaskUpdate, infra)
	if err != nil {
		return errors.Fmt("invalid task: %w", err)
	}

	// Skip already applied update. This is also double checked in the transaction
	// later in prepareUpdate.
	if req.Task.UpdateId <= infra.Proto.Backend.Task.UpdateId {
		logging.Infof(ctx, "Already seen update ID %d (last seen is %d)",
			req.Task.UpdateId, infra.Proto.Backend.Task.UpdateId)
		return nil
	}

	err = updateTaskEntity(ctx, req.BuildTaskUpdate, buildID, useTaskSuccess)
	if err != nil {
		if status, _ := appstatus.Get(err); status.Code() == codes.NotFound {
			return errors.Fmt("missing some build entities: %w", err)
		}
		return transient.Tag.Apply(errors.Fmt("failed to update the build entities: %w", err))
	}

	return nil
}

// UpdateBuildTask handles task backend PubSub push messages produced by various
// task backends.
// For a retryable error, it will be tagged with transient.Tag.
func UpdateBuildTask(ctx context.Context, body io.Reader) error {
	req, err := unpackUpdateBuildTaskMsg(ctx, body)
	if err != nil {
		return err
	}

	// Try not to process same message more than once.
	cache := caching.GlobalCache(ctx, "update-build-task-pubsub-msg-id")
	if cache == nil {
		return transient.Tag.Apply(errors.New("global cache is not found"))
	}
	msgCached, err := cache.Get(ctx, req.msgID)
	switch {
	case err == caching.ErrCacheMiss: // no-op, continue
	case err != nil:
		return transient.Tag.Apply(errors.Fmt("failed to read %s from the global cache: %w", req.msgID, err))
	case msgCached != nil:
		logging.Infof(ctx, "seen this message %s before, ignoring", req.msgID)
		return nil
	}
	err = updateBuildTask(ctx, req)
	if err != nil {
		return errors.Fmt("failed to update the build from message %s: %w", req.msgID, err)
	}

	// Best effort at setting the cache. No big deal if this fails.
	err = cache.Set(ctx, req.msgID, []byte{1}, 10*time.Minute)
	if err != nil {
		logging.Warningf(ctx, "Failed to update cache: %s", err)
	}
	return nil
}
