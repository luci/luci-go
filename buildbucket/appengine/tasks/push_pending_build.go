// Copyright 2024 The LUCI Authors.
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

	"github.com/google/uuid"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// PushPendingBuildTask is responsible for updating the BuilderQueue entity.
// It is triggered at the build creation for builder with Config.MaxConcurrentBuilds > 0.
// A new build can either be pushed to triggered_builds and sent to task backend, or
// pushed to pending_builds where it will wait to be popped when the builder gets some capacity.
func PushPendingBuildTask(ctx context.Context, bID int64, bldrID *pb.BuilderID) error {
	bldrQID := protoutil.FormatBuilderID(bldrID)
	bldr := &model.Builder{
		ID:     bldrID.Builder,
		Parent: model.BucketKey(ctx, bldrID.Project, bldrID.Bucket),
	}
	if err := datastore.Get(ctx, bldr); err != nil {
		return errors.Fmt("failed to get builder: %s: %w", bldrQID, err)
	}

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		mcb := bldr.Config.GetMaxConcurrentBuilds()
		// max_concurrent_builds has been reset (set to 0) after the task was created.
		// Send the build to task backend without updating the BuilderQueue.
		if mcb == 0 {
			return CreateBackendBuildTask(ctx, &taskdefs.CreateBackendBuildTask{
				BuildId:   bID,
				RequestId: uuid.New().String(),
			})
		}

		bldrQ := &model.BuilderQueue{ID: bldrQID}
		if err := model.GetIgnoreMissing(ctx, bldrQ); err != nil {
			return err
		}

		if len(bldrQ.PendingBuilds) == 0 && len(bldrQ.TriggeredBuilds) < int(mcb) {
			err := CreateBackendBuildTask(ctx, &taskdefs.CreateBackendBuildTask{
				BuildId:   bID,
				RequestId: uuid.New().String(),
			})
			if err != nil {
				return err
			}
			bldrQ.TriggeredBuilds = append(bldrQ.TriggeredBuilds, bID)
		} else {
			bldrQ.PendingBuilds = append(bldrQ.PendingBuilds, bID)
		}
		if err := datastore.Put(ctx, bldrQ); err != nil {
			return errors.Fmt("failed to update the BuilderQueue: %s: %w", bldrQID, err)
		}

		return nil
	}, nil)
	if err != nil {
		return errors.Fmt("error updating BuilderQueue for builder: %s and build: %d: %w", bldrQID, bID, err)
	}
	return nil
}
