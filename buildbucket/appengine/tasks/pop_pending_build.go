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
	"fmt"
	"math"
	"slices"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// PopPendingBuildTask is responsible for popping and triggering
// builds from the BuilderQueue pending_builds queue.
// PopPendingBuildTask runs at build termination for builder with Config.MaxConcurrentBuilds > 0,
// or at builder config ingestion for Config.MaxConcurrentBuilds increases and resets.
// A popped build is sent to task backend and moved to the BuilderQueue triggered_builds set.
func PopPendingBuildTask(ctx context.Context, bID int64, bldrID *pb.BuilderID) error {
	bldrQID := protoutil.FormatBuilderID(bldrID)
	bldr := &model.Builder{
		ID:     bldrID.Builder,
		Parent: model.BucketKey(ctx, bldrID.Project, bldrID.Bucket),
	}
	if err := datastore.Get(ctx, bldr); err != nil {
		return errors.Fmt("failed to get builder: %s: %w", bldrQID, err)
	}
	mcb := bldr.Config.GetMaxConcurrentBuilds()
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		bldrQ := &model.BuilderQueue{ID: bldrQID}
		switch err := datastore.Get(ctx, bldrQ); {
		case err == datastore.ErrNoSuchEntity:
			logging.Infof(ctx, "build: %d was not tracked for builder: %s with max_concurrent_builds: %d; BuilderQueue does not exist", bID, bldrQID, mcb)
			return nil
		case err != nil:
			return err
		}

		// buildID is specified, try to remove it from builder queues.
		if bID != 0 {
			removeBuildFromBuilderQueues(ctx, bldrQ, bldrQID, bID)
		}
		if err := popAndTriggerBuilds(ctx, bldrQ, mcb); err != nil {
			return err
		}
		if mcb == 0 {
			if err := datastore.Delete(ctx, bldrQ); err != nil {
				return errors.Fmt("failed to delete the BuilderQueue: %s: %w", bldrQID, err)
			}
		} else {
			if err := datastore.Put(ctx, bldrQ); err != nil {
				return errors.Fmt("failed to update the BuilderQueue: %s: %w", bldrQID, err)
			}
		}

		return nil
	}, nil)
	if err != nil {
		return errors.Fmt("error updating BuilderQueue for builder: %s and build: %d: %w", bldrQID, bID, err)
	}
	return nil
}

// removeBuildFromBuilderQueues removes the buildID from the queues.
func removeBuildFromBuilderQueues(ctx context.Context, bldrQ *model.BuilderQueue, bldrQID string, bID int64) {
	found := false
	// First try to remove the build from the triggered_builds queue.
	for i, b := range bldrQ.TriggeredBuilds {
		if b == bID {
			found = true
			bldrQ.TriggeredBuilds = slices.Delete(bldrQ.TriggeredBuilds, i, i+1)
			break
		}
	}
	if found {
		return
	}

	// The build could be a cancelled pending build, check pending_builds.
	for i, b := range bldrQ.PendingBuilds {
		if b == bID {
			found = true
			bldrQ.PendingBuilds = slices.Delete(bldrQ.PendingBuilds, i, i+1)
			break
		}
	}
	if !found {
		logging.Infof(
			ctx, "build %d was not tracked for builder %s; "+
				"build not found in triggered_builds %q or pending_builds %q",
			bID, bldrQID, bldrQ.TriggeredBuilds, bldrQ.PendingBuilds)
	}
}

// popAndTriggerBuilds pops pending builds from the pending_builds queue and sends them to task backend.
func popAndTriggerBuilds(ctx context.Context, bldrQ *model.BuilderQueue, mcb uint32) error {
	// Handle the case where max_concurrent_builds is disabled.
	// Purge *all* the pending builds.
	if mcb == 0 {
		mcb = math.MaxUint32
	}

	if len(bldrQ.PendingBuilds) == 0 || len(bldrQ.TriggeredBuilds) >= int(mcb) {
		return nil
	}

	// Pop as many pending builds as allowed by max_concurrent_builds.
	toPop := min(int(mcb)-len(bldrQ.TriggeredBuilds), len(bldrQ.PendingBuilds))
	if toPop == 0 {
		return nil
	}

	now := clock.Now(ctx)
	nowpb := timestamppb.New(now)

	if toPop == 1 {
		err := CreateBackendBuildTask(
			ctx, &taskdefs.CreateBackendBuildTask{
				BuildId:     bldrQ.PendingBuilds[0],
				RequestId:   uuid.New().String(),
				DequeueTime: nowpb,
			})
		if err != nil {
			return err
		}
		bldrQ.TriggeredBuilds = append(bldrQ.TriggeredBuilds, bldrQ.PendingBuilds[0])
		bldrQ.PendingBuilds = bldrQ.PendingBuilds[1:]
		return nil
	}

	batchCreateTask := &taskdefs.BatchCreateBackendBuildTasks{
		DequeueTime:            nowpb,
		DeduplicationKeyPrefix: fmt.Sprintf("%d_%d", now.UnixNano(), mathrand.Int63(ctx)),
	}
	for _, pb := range bldrQ.PendingBuilds[:toPop] {
		batchCreateTask.Requests = append(batchCreateTask.Requests,
			&taskdefs.BatchCreateBackendBuildTasks_Request{
				BuildId:   pb,
				RequestId: uuid.New().String(),
			})
		bldrQ.TriggeredBuilds = append(bldrQ.TriggeredBuilds, pb)
	}
	bldrQ.PendingBuilds = bldrQ.PendingBuilds[toPop:]

	return createBatchCreateBackendBuildTasks(ctx, batchCreateTask, "")
}
