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

// Package buildstatus provides the build status computation related functions.
package buildstatus

import (
	"context"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

type StatusWithDetails struct {
	Status  pb.Status
	Details *pb.StatusDetails
}

func (sd *StatusWithDetails) isSet() bool {
	return sd != nil && sd.Status != pb.Status_STATUS_UNSPECIFIED
}

type Updater struct {
	Build *model.Build
	Infra *model.BuildInfra

	BuildStatus  *StatusWithDetails
	OutputStatus *StatusWithDetails
	TaskStatus   *StatusWithDetails

	SucceedBuildIfTaskSucceeded bool

	UpdateTime time.Time

	PostProcess func(c context.Context, bld *model.Build, inf *model.BuildInfra) error
}

// buildEndStatus calculates the final status of a build based on its output
// status and backend task status.
func (u *Updater) buildEndStatus(outStatus, taskStatus *StatusWithDetails) *StatusWithDetails {
	switch {
	case !outStatus.isSet() || !protoutil.IsEnded(outStatus.Status):
		if taskStatus.Status == pb.Status_SUCCESS && !u.SucceedBuildIfTaskSucceeded {
			// outStatus should have been an ended status since taskStatus is.
			// Something must be wrong.
			return &StatusWithDetails{Status: pb.Status_INFRA_FAILURE}
		} else {
			// This could happen if the task crashes when running the build, or
			// SucceedBuildIfTaskSucceeded is true, use the task status.
			return taskStatus
		}
	case outStatus.Status == pb.Status_SUCCESS:
		// Either taskStatus.Status is also SUCCESS or a failure, can use taskStatus
		// as the final one.
		return taskStatus
	case outStatus.Status == pb.Status_CANCELED:
		if u.Build.Proto.CancelTime != nil {
			// The build is canceled intentially through the CancelBuild RPC.
			// So it means the build is no longer needed.
			return outStatus
		} else {
			// The build is canceled by backend, likely because of an issue.
			if taskStatus.Status == pb.Status_SUCCESS {
				// Should not happen.
				return &StatusWithDetails{Status: pb.Status_INFRA_FAILURE}
			}
			return taskStatus
		}
	default:
		// outStatus already contains failure, use outStatus as the final one.
		return outStatus
	}
}
func (u *Updater) calculateBuildStatus() *StatusWithDetails {
	switch {
	case u.BuildStatus != nil && u.BuildStatus.Status != pb.Status_STATUS_UNSPECIFIED:
		// If top level status is provided, use that directly.
		// TODO(crbug.com/1450399): remove this case after the callsites are updated
		// to not set top level status directly.
		return u.BuildStatus
	case !u.OutputStatus.isSet() && !u.TaskStatus.isSet():
		return &StatusWithDetails{Status: pb.Status_STATUS_UNSPECIFIED}
	case u.OutputStatus.isSet() && u.OutputStatus.Status == pb.Status_STARTED:
		return &StatusWithDetails{Status: pb.Status_STARTED}
	case u.TaskStatus.isSet() && protoutil.IsEnded(u.TaskStatus.Status):
		return u.buildEndStatus(&StatusWithDetails{
			Status:  u.Build.Proto.Output.GetStatus(),
			Details: u.Build.Proto.Output.GetStatusDetails()},
			u.TaskStatus)
	default:
		// no change.
		return &StatusWithDetails{Status: u.Build.Proto.Status, Details: u.Build.Proto.StatusDetails}
	}
}

// Do updates the top level build status, and performs actions
// when build status changes:
// * it updates the corresponding BuildStatus entity for the build;
// * it triggers PubSub notify task;
// * if the build is ended, it triggers BQ export task.
//
// Note that pubsub notification on build start will not retry on failure.
//
// Must be run inside a transaction.
//
// The post-processes that should happen after the status update is committed
// is not included, and the callsite needs to handle them separately. These
// include:
// * update build event metrics
// * cancel descendent builds when this build is ended.
func (u *Updater) Do(ctx context.Context) (*model.BuildStatus, error) {
	if datastore.Raw(ctx) == nil || datastore.CurrentTransaction(ctx) == nil {
		return nil, errors.New("must update build status in a transaction")
	}

	if protoutil.IsEnded(u.Build.Proto.Status) {
		return nil, errors.New("cannot update status for an ended build")
	}

	// Check the provided statuses.
	if u.OutputStatus.isSet() && u.TaskStatus.isSet() {
		return nil, errors.New("impossible: update build output status and task status at the same time")
	}

	newBuildStatus := u.BuildStatus
	if !newBuildStatus.isSet() {
		newBuildStatus = u.calculateBuildStatus()
	}
	if !newBuildStatus.isSet() {
		// Nothing provided to update.
		return nil, errors.New("cannot set a build status to UNSPECIFIED")
	}

	if newBuildStatus.Status == u.Build.Proto.Status {
		// Nothing to update.
		return nil, nil
	}

	protoutil.SetStatus(u.UpdateTime, u.Build.Proto, newBuildStatus.Status)
	u.Build.Proto.StatusDetails = newBuildStatus.Details

	// Update custom builder metrics.
	// Currently only the consecutive failure count metrics need to be evaluated
	// at build completion, so skip this for the successful ones.
	if protoutil.IsEnded(newBuildStatus.Status) && newBuildStatus.Status != pb.Status_SUCCESS {
		err := model.EvaluateBuildForCustomBuilderMetrics(ctx, u.Build, true)
		if err != nil {
			logging.Errorf(ctx, "failed to evaluate build for custom builder metrics: %s", err)
		}
	}

	// Update BuildStatus.
	entities, err := common.GetBuildEntities(ctx, u.Build.ID, model.BuildStatusKind)
	if err != nil {
		return nil, err
	}
	bs := entities[0].(*model.BuildStatus)
	bs.Status = newBuildStatus.Status

	// post process after build status change.
	if err := u.PostProcess(ctx, u.Build, u.Infra); err != nil {
		return nil, errors.Fmt("failed to run post process when updating build %d to %s: %w", u.Build.ID, newBuildStatus.Status, err)
	}

	return bs, nil
}
