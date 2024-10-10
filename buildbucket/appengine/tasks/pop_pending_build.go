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
	"math"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
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
		return errors.Annotate(err, "failed to get builder: %s", bldrQID).Err()
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

		// buildID is specified, try to remove it from triggered_builds.
		if bID != 0 {
			if !removeBuildFromTriggered(bldrQ, bID) {
				logging.Infof(ctx, "build: %d was not tracked for this builder: %s; build not found in triggered_builds", bID, bldrQID)
			}
		}
		if err := popAndTriggerBuilds(ctx, bldrQ, mcb); err != nil {
			return err
		}
		if mcb == 0 {
			if err := datastore.Delete(ctx, bldrQ); err != nil {
				return errors.Annotate(err, "failed to delete the BuilderQueue: %s", bldrQID).Err()
			}
		} else {
			if err := datastore.Put(ctx, bldrQ); err != nil {
				return errors.Annotate(err, "failed to update the BuilderQueue: %s", bldrQID).Err()
			}
		}

		return nil
	}, nil)
	if err != nil {
		return errors.Annotate(err, "error updating BuilderQueue for builder: %s and build: %d", bldrQID, bID).Err()
	}
	return nil
}

// removeBuildFromTriggered removes the buildID from the triggered_builds set.
// Return false if the buildID was not found.
func removeBuildFromTriggered(bldrQ *model.BuilderQueue, bID int64) bool {
	found := false
	for i, b := range bldrQ.TriggeredBuilds {
		if b == bID {
			found = true
			// Remove the buildID without preserving order.
			bldrQ.TriggeredBuilds[i] = bldrQ.TriggeredBuilds[len(bldrQ.TriggeredBuilds)-1]
			bldrQ.TriggeredBuilds = bldrQ.TriggeredBuilds[:len(bldrQ.TriggeredBuilds)-1]
			break
		}
	}
	return found
}

// popAndTriggerBuilds pops pending builds from the pending_builds queue and sends them to task backend.
func popAndTriggerBuilds(ctx context.Context, bldrQ *model.BuilderQueue, mcb uint32) error {
	// Handle the case where max_concurrent_builds is disabled.
	// Purge *all* the pending builds.
	if mcb == 0 {
		mcb = math.MaxUint32
	}

	// Pop as many pending builds as allowed by max_concurrent_builds.
	if len(bldrQ.PendingBuilds) > 0 && len(bldrQ.TriggeredBuilds) < int(mcb) {
		toPop := int(math.Min(float64(int(mcb)-len(bldrQ.TriggeredBuilds)), float64(len(bldrQ.PendingBuilds))))
		for _, pb := range bldrQ.PendingBuilds[:toPop] {
			err := CreateBackendBuildTask(ctx, &taskdefs.CreateBackendBuildTask{
				BuildId:     pb,
				RequestId:   uuid.New().String(),
				DequeueTime: timestamppb.New(clock.Now(ctx)),
			})
			if err != nil {
				return err
			}
			bldrQ.TriggeredBuilds = append(bldrQ.TriggeredBuilds, pb)
		}
		bldrQ.PendingBuilds = bldrQ.PendingBuilds[toPop:]
	}

	return nil
}
