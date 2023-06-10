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
	"io"
	"strconv"
	"time"

	"google.golang.org/api/pubsub/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	fieldmaskpb "google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
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

func validateTaskStatus(taskStatus pb.Status) error {
	switch taskStatus {
	case pb.Status_SCHEDULED,
		pb.Status_ENDED_MASK,
		pb.Status_STATUS_UNSPECIFIED:
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
		if backend.Target == target && backend.Subscription == req.subscription {
			isValid = true
		}
	}

	if !isValid {
		return errors.Reason("pubsub subscription %s did not match the one configured for target %s", req.subscription, target).Err()
	}
	return nil
}

// validateBuildTaskUpdate ensures that the build_id, task, status, and details
// are correctly set be sender.
func validateBuildTaskUpdate(ctx context.Context, req *pb.BuildTaskUpdate) error {
	if req.BuildId == "" {
		return errors.Reason("build_id required").Err()
	}
	if req.GetTask().GetId().GetId() == "" {
		return errors.Reason("task.id: required").Err()
	}
	if err := validateTaskStatus(req.Task.Status); err != nil {
		return errors.Annotate(err, "task.Status").Err()
	}
	detailsInKb := float64(len(req.Task.GetDetails().String()) / 1024)
	if detailsInKb > 10 {
		return errors.Reason("task.details is greater than 10 kb").Err()
	}
	return nil
}

// validateBuildTask ensures that the taskID provided in the request matches
// the taskID that is stored in the build model. If there is no task associated
// with the build, the task is associated here.
func validateBuildTask(ctx context.Context, req *pb.BuildTaskUpdate, build *model.Build) (*pb.Build, error) {

	// create a new build mask to read infra
	mask, _ := model.NewBuildMask("", nil, &pb.BuildMask{
		Fields: &fieldmaskpb.FieldMask{
			Paths: []string{"id", "infra"},
		},
	})
	bld, err := build.ToProto(ctx, mask, nil)
	if err != nil {
		return nil, errors.Annotate(err, "could not load build details").Err()
	}
	if bld.Infra.GetBackend() == nil {
		bld.Infra.Backend = &pb.BuildInfra_Backend{}
	}
	if bld.Infra.Backend.GetTask() == nil {
		bld.Infra.Backend.Task = &pb.Task{}
	} else if bld.Infra.Backend.Task.Id.GetId() != req.Task.Id.GetId() ||
		bld.Infra.Backend.Task.Id.GetTarget() != req.Task.Id.GetTarget() {
		return nil, errors.Reason("task ID in request does not match task ID associated with build").Err()
	}
	if protoutil.IsEnded(bld.Infra.Backend.Task.Status) {
		return nil, appstatus.Errorf(codes.FailedPrecondition, "cannot update an ended task")
	}
	bld.Infra.Backend.Task = req.Task
	return bld, nil
}

func updateTaskEntity(ctx context.Context, req *pb.BuildTaskUpdate, build *pb.Build) error {
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		b, err := common.GetBuild(ctx, build.Id)
		if err != nil {
			if _, isAppStatusErr := appstatus.Get(err); isAppStatusErr {
				return err
			}
			return appstatus.Errorf(codes.Internal, "failed to get build %d: %s", build.Id, err)
		}
		bk := datastore.KeyForObj(ctx, b)
		infra := &model.BuildInfra{
			Build: bk,
			Proto: build.Infra,
		}
		toSave := []any{infra}
		return datastore.Put(ctx, toSave)
	}, nil)

	return txErr
	// TODO [randymaldonado@] send pubsub notifications and update metrics.
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

	bld, err := common.GetBuild(ctx, buildID)
	if err != nil {
		return errors.Annotate(err, "invalid build").Err()
	}

	// Pre-check if the task can be updated before updating it with a transaction.
	// Ensures that the taskID provided in the request matches the taskID that is
	// stored in the build model. If there is no task associated with the build model,
	// the task is associated here in buildInfra.Infra.Backend.Task.
	build, err := validateBuildTask(ctx, req.BuildTaskUpdate, bld)
	if err != nil {
		return errors.Annotate(err, "invalid task").Err()
	}

	err = updateTaskEntity(ctx, req.BuildTaskUpdate, build)
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
		return err
	}

	return cache.Set(ctx, req.msgID, []byte{1}, 10*time.Minute)
}
