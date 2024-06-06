// Copyright 2023 The LUCI Authors.
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
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildstatus"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// CheckLiveness is to check if the given build has received any updates during
// the timeout period.
func CheckLiveness(ctx context.Context, buildID int64, heartbeatTimeout uint32) error {
	bld, err := common.GetBuild(ctx, buildID)
	if err != nil {
		return errors.Annotate(err, "failed to get build %d", buildID).Err()
	}

	if protoutil.IsEnded(bld.Status) {
		// No need to check for an ended build.
		return nil
	}
	// Enqueue a continuation CheckBuildLiveness task
	if !isTimeout(ctx, bld, heartbeatTimeout) {
		// lefted excution timeout.
		delay := bld.Proto.ExecutionTimeout.AsDuration() - (clock.Now(ctx).Sub(bld.Proto.StartTime.AsTime()))
		if heartbeatTimeout > 0 && uint32(delay.Seconds()) > heartbeatTimeout {
			delay = time.Duration(heartbeatTimeout) * time.Second
		}
		return transient.Tag.Apply(CheckBuildLiveness(ctx, buildID, heartbeatTimeout, delay))
	}

	// Time out. Should mark the build as INFRA_FAILURE.
	enqueueTask := false
	txnErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Fetch and check the build again before failing it. In case the build is
		// changed during the short time window from the first check to now.
		//
		// Fetch build steps as well. Ignore any ErrNoSuchEntity error.
		// `step.CancelIncomplete` will return false if no steps and steps update
		//  will be skipped.
		steps := &model.BuildSteps{Build: datastore.KeyForObj(ctx, bld)}
		if err := model.GetIgnoreMissing(ctx, bld, steps); err != nil {
			return err
		}

		if !isTimeout(ctx, bld, heartbeatTimeout) {
			if !protoutil.IsEnded(bld.Status) {
				enqueueTask = true
			}
			return nil
		}

		oldStatus := bld.Proto.Status
		now := clock.Now(ctx)
		statusUpdater := buildstatus.Updater{
			Build:       bld,
			Infra:       nil,
			BuildStatus: &buildstatus.StatusWithDetails{Status: pb.Status_INFRA_FAILURE},
			UpdateTime:  now,
			PostProcess: SendOnBuildStatusChange,
		}
		bs, err := statusUpdater.Do(ctx)
		if err != nil {
			return errors.Annotate(err, "failed to update status").Err()
		}
		toSave := []any{bld, bs}
		switch changed, err := steps.CancelIncomplete(ctx, timestamppb.New(now)); {
		case err != nil:
			return errors.Annotate(err, "failed to cancel steps").Err()
		case changed:
			toSave = append(toSave, steps)
		}
		logging.Infof(ctx, "Build %d timed out, updating status: %s -> %s", bld.ID, oldStatus, bld.Proto.Status)
		return datastore.Put(ctx, toSave)
	}, nil)
	if txnErr != nil {
		return transient.Tag.Apply(errors.Annotate(txnErr, "failed to fail the build %d", buildID).Err())
	}

	if enqueueTask {
		// Enqueue a continuation Task
		return transient.Tag.Apply(CheckBuildLiveness(ctx, buildID, heartbeatTimeout, time.Duration(heartbeatTimeout)*time.Second))
	}
	metrics.BuildCompleted(ctx, bld)
	return nil
}

// isTimeout checks if the build exceeds the scheduling timeout, execution
// timeout or heartbeat timeout after the last touch.
func isTimeout(ctx context.Context, bld *model.Build, heartbeatTimeout uint32) bool {
	now := clock.Now(ctx)
	switch bld.Proto.Status {
	case pb.Status_SCHEDULED:
		if now.Sub(bld.Proto.CreateTime.AsTime()) >= bld.Proto.SchedulingTimeout.AsDuration() {
			return true
		}
	case pb.Status_STARTED:
		if now.Sub(bld.Proto.StartTime.AsTime()) >= bld.Proto.ExecutionTimeout.AsDuration() {
			return true
		}
		if heartbeatTimeout != 0 && now.Sub(bld.Proto.UpdateTime.AsTime()) >= time.Duration(heartbeatTimeout)*time.Second {
			return true
		}
	}
	return false
}
