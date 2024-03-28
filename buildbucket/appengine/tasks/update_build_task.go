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
		return req, errors.Annotate(err, "failed to read the request body").Tag(transient.Tag).Err()
	}

	// process pubsub message
	var msg pushRequest
	if err := json.Unmarshal(blob, &msg); err != nil {
		return req, errors.Annotate(err, "failed to unmarshal UpdateBuildTask PubSub message").Err()
	}
	// process UpdateBuildTask message data
	data, err := base64.StdEncoding.DecodeString(msg.Message.Data)
	if err != nil {
		return req, errors.Annotate(err, "cannot decode UpdateBuildTask message data as base64").Err()
	}

	bldTskUpdte := &pb.BuildTaskUpdate{}
	if err := (proto.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, bldTskUpdte); err != nil {
		return req, errors.Annotate(err, "failed to unmarshal the BuildTaskUpdate pubsub data").Err()
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
		return errors.Reason("task.status: invalid status %s for UpdateBuildTask", taskStatus).Err()
	}

	if !allowPending && taskStatus == pb.Status_SCHEDULED {
		return errors.Reason("task.status: invalid status %s for UpdateBuildTask", taskStatus).Err()
	}

	if _, ok := pb.Status_value[taskStatus.String()]; !ok {
		return errors.Reason("task.status: invalid status %s for UpdateBuildTask", taskStatus).Err()
	}
	return nil
}

func validatePubsubSubscription(ctx context.Context, req buildTaskUpdate) error {
	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return errors.Annotate(err, "error fetching service config").Err()
	}

	target := req.GetTask().GetId().GetTarget()
	if target == "" {
		return errors.Reason("could not validate message. task.id.target not provided.").Err()
	}

	isValid := false
	for _, backend := range globalCfg.Backends {
		if backend.Target == target {
			var subscription string
			switch backend.Mode.(type) {
			case *pb.BackendSetting_LiteMode_:
				return errors.Reason("backend target %s is in lite mode. The task update isn't supported", target).Err()
			case *pb.BackendSetting_FullMode_:
				subscription = fmt.Sprintf("projects/%s/subscriptions/%s", info.AppID(ctx), backend.GetFullMode().GetPubsubId())
			}
			if subscription == req.subscription {
				isValid = true
			}
			break
		}
	}

	if !isValid {
		return errors.Reason("pubsub subscription %s did not match the one configured for target %s", req.subscription, target).Err()
	}
	return nil
}

func validateTask(task *pb.Task, allowPending bool) error {
	if task.GetId().GetId() == "" {
		return errors.Reason("task.id: required").Err()
	}
	if task.GetUpdateId() == 0 {
		return errors.Reason("task.UpdateId: required").Err()
	}
	if err := validateTaskStatus(task.Status, allowPending); err != nil {
		return errors.Annotate(err, "task.Status").Err()
	}
	detailsInKb := float64(len(task.GetDetails().String()) / 1024)
	if detailsInKb > 10 {
		return errors.Reason("task.details is greater than 10 kb").Err()
	}
	return nil
}

// validateBuildTaskUpdate ensures that the build_id, task, status, and details
// are correctly set be sender.
func validateBuildTaskUpdate(ctx context.Context, req *pb.BuildTaskUpdate) error {
	if req.BuildId == "" {
		return errors.Reason("build_id required").Err()
	}
	return validateTask(req.Task, false)
}

// validateBuildTask ensures that the taskID provided in the request matches
// the taskID that is stored in the build model. If there is no task associated
// with the build, an error is returned and the update message is lost.
func validateBuildTask(ctx context.Context, req *pb.BuildTaskUpdate, infra *model.BuildInfra) error {
	switch {
	case infra.Proto.GetBackend() == nil:
		return appstatus.Errorf(codes.NotFound, "Build %s does not support task backend", req.BuildId)
	case infra.Proto.Backend.GetTask().GetId().GetId() == "":
		return appstatus.Errorf(codes.NotFound, "No task is associated with the build. Cannot update.")
	case infra.Proto.Backend.Task.Id.GetTarget() != req.Task.Id.GetTarget() || (infra.Proto.Backend.Task.Id.GetId() != "" && infra.Proto.Backend.Task.Id.GetId() != req.Task.Id.GetId()):
		return errors.Reason("TaskID in request does not match TaskID associated with build").Err()
	}
	if protoutil.IsEnded(infra.Proto.Backend.Task.Status) {
		return appstatus.Errorf(codes.FailedPrecondition, "cannot update an ended task")
	}
	return nil
}

func prepareUpdate(ctx context.Context, build *model.Build, infra *model.BuildInfra, task *pb.Task) ([]any, error) {
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

	bs, steps, err := updateBuildStatusOnTaskStatusChange(ctx, build, nil, &buildstatus.StatusWithDetails{Status: task.Status}, now)
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

func updateTaskEntity(ctx context.Context, req *pb.BuildTaskUpdate, buildID int64) error {
	var build *model.Build
	setBuildToEnd := false
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		entities, err := common.GetBuildEntities(ctx, buildID, model.BuildKind, model.BuildInfraKind)
		if err != nil {
			return errors.Annotate(err, "invalid Build or BuildInfra").Err()
		}

		build = entities[0].(*model.Build)
		infra := entities[1].(*model.BuildInfra)

		if protoutil.IsEnded(build.Status) {
			// Cannot update an ended build.
			logging.Infof(ctx, "build %d is ended", build.ID)
			return nil
		}

		toSave, err := prepareUpdate(ctx, build, infra, req.Task)
		if err != nil {
			return err
		}

		setBuildToEnd = protoutil.IsEnded(req.Task.Status)
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
	buildID, err := strconv.ParseInt(req.GetBuildId(), 10, 64)
	if err != nil {
		return errors.Annotate(err, "bad build id").Err()
	}
	if err := validatePubsubSubscription(ctx, req); err != nil {
		return errors.Annotate(err, "pubsub subscription").Err()
	}
	if err := validateBuildTaskUpdate(ctx, req.BuildTaskUpdate); err != nil {
		return errors.Annotate(err, "invalid BuildTaskUpdate").Err()
	}
	logging.Infof(ctx, "Received an BuildTaskUpdate message for build %q", req.BuildId)

	// TODO(b/288158829): remove it once the root cause for the Skia failure is found.
	if strings.Contains(req.subscription, "skia") {
		logging.Debugf(ctx, "BuildTaskUpdate.Task: %v", req.BuildTaskUpdate.Task)
	}

	entities, err := common.GetBuildEntities(ctx, buildID, model.BuildInfraKind)
	if err != nil {
		return errors.Annotate(err, "invalid buildInfra").Err()
	}
	infra := entities[0].(*model.BuildInfra)

	// Pre-check if the task can be updated before updating it with a transaction.
	// Ensures that the taskID provided in the request matches the taskID that is
	// stored in the build model. If there is no task associated with the build model,
	// an error is returned and the update message is lost.
	err = validateBuildTask(ctx, req.BuildTaskUpdate, infra)
	if err != nil {
		return errors.Annotate(err, "invalid task").Err()
	}

	err = updateTaskEntity(ctx, req.BuildTaskUpdate, buildID)
	if err != nil {
		if _, isAppStatusErr := appstatus.Get(err); isAppStatusErr {
			return err
		} else {
			return appstatus.Errorf(codes.Internal, "failed to update the build entity: %s", err)
		}
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
		return errors.Reason("global cache is not found").Tag(transient.Tag).Err()
	}
	msgCached, err := cache.Get(ctx, req.msgID)
	switch {
	case err == caching.ErrCacheMiss: // no-op, continue
	case err != nil:
		return errors.Annotate(err, "failed to read %s from the global cache", req.msgID).Tag(transient.Tag).Err()
	case msgCached != nil:
		logging.Infof(ctx, "seen this message %s before, ignoring", req.msgID)
		return nil
	}
	err = updateBuildTask(ctx, req)
	if err != nil {
		return errors.Annotate(err, "failed to update the build from message %s", req.msgID).Tag(transient.Tag).Err()
	}

	return cache.Set(ctx, req.msgID, []byte{1}, 10*time.Minute)
}
