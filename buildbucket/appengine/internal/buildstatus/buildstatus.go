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
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

type Updater struct {
	Build *model.Build

	BuildStatus  pb.Status
	OutputStatus pb.Status
	TaskStatus   pb.Status

	UpdateTime time.Time

	PostProcess func(c context.Context, bld *model.Build) error
}

func (u *Updater) calculateBuildStatus() pb.Status {
	switch {
	case u.BuildStatus != pb.Status_STATUS_UNSPECIFIED:
		// If top level status is provided, use that directly.
		// TODO(crbug.com/1450399): remove this case after the callsites are updated
		// to not set top level status directly.
		return u.BuildStatus
	case u.OutputStatus == pb.Status_STATUS_UNSPECIFIED && u.TaskStatus == pb.Status_STATUS_UNSPECIFIED:
		return pb.Status_STATUS_UNSPECIFIED
	case u.OutputStatus == pb.Status_STARTED:
		return pb.Status_STARTED
	case protoutil.IsEnded(u.TaskStatus):
		return u.TaskStatus
	default:
		// no change.
		return u.Build.Proto.Status
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
		return nil, errors.Reason("must update build status in a transaction").Err()
	}

	if protoutil.IsEnded(u.Build.Proto.Status) {
		return nil, errors.Reason("cannot update status for an ended build").Err()
	}

	// Check the provided statuses.
	if u.OutputStatus != pb.Status_STATUS_UNSPECIFIED && u.TaskStatus != pb.Status_STATUS_UNSPECIFIED {
		return nil, errors.Reason("impossible: update build output status and task status at the same time").Err()
	}

	newBuildStatus := u.BuildStatus
	if newBuildStatus == pb.Status_STATUS_UNSPECIFIED {
		newBuildStatus = u.calculateBuildStatus()
	}
	if newBuildStatus == pb.Status_STATUS_UNSPECIFIED {
		// Nothing provided to update.
		return nil, errors.Reason("cannot set a build status to UNSPECIFIED").Err()
	}

	if newBuildStatus == u.Build.Proto.Status {
		// Nothing to update.
		return nil, nil
	}

	protoutil.SetStatus(u.UpdateTime, u.Build.Proto, newBuildStatus)

	// Update BuildStatus.
	entities, err := common.GetBuildEntities(ctx, u.Build.ID, model.BuildStatusKind)
	if err != nil {
		return nil, err
	}
	bs := entities[0].(*model.BuildStatus)
	bs.Status = newBuildStatus

	// post process after build status change.
	if err := u.PostProcess(ctx, u.Build); err != nil {
		return nil, err
	}

	return bs, nil
}
