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

package buildcron

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	"go.chromium.org/luci/buildbucket/protoutil"
)

const waitDuration = 5 * time.Minute

// ScanBuilderQueues scans all of the BuilderQueue entities and look for
// builds that have been ended and enqueue tasks to pop them.
// Best effort, errors are ignored.
func ScanBuilderQueues(ctx context.Context) error {
	q := datastore.NewQuery(model.BuilderQueueKind)

	eg, _ := errgroup.WithContext(ctx)
	eg.SetLimit(64)
	_ = datastore.RunBatch(ctx, 1000, q,
		func(bldrQ *model.BuilderQueue) error {
			eg.Go(func() error {
				checkBuilderQueue(ctx, bldrQ)
				return nil
			})
			return nil
		})

	_ = eg.Wait()

	return nil
}

// checkBuilderQueue checks the builds in the queues and try to enqueue
// PopPendingBuildTask for the builds that have been ended.
// Best effort, if there's any issue, just log it and wait for the next cron job.
func checkBuilderQueue(ctx context.Context, bldrQ *model.BuilderQueue) {
	if len(bldrQ.TriggeredBuilds) == 0 && len(bldrQ.PendingBuilds) == 0 {
		return
	}

	builds := make([]*model.Build, 0, len(bldrQ.TriggeredBuilds)+len(bldrQ.PendingBuilds))
	for _, bID := range bldrQ.TriggeredBuilds {
		builds = append(builds, &model.Build{
			ID: bID,
		})
	}
	for _, bID := range bldrQ.PendingBuilds {
		builds = append(builds, &model.Build{
			ID: bID,
		})
	}
	err := datastore.Get(ctx, builds)
	if err != nil {
		logging.Debugf(ctx, "error getting builds %q: %s", builds, err)
		return
	}

	now := clock.Now(ctx)
	eg, _ := errgroup.WithContext(ctx)
	eg.SetLimit(64)
	for _, b := range builds {
		eg.Go(func() error {
			if !protoutil.IsEnded(b.Status) {
				return nil
			}

			if b.Proto.EndTime.AsTime().Add(waitDuration).After(now) {
				// Wait for 5 minutes to make sure the proper build termination
				// code path has the chance to run first.
				return nil
			}

			err := tasks.CreatePopPendingBuildTask(ctx, &taskdefs.PopPendingBuildTask{
				BuildId:   b.ID,
				BuilderId: b.Proto.Builder,
			}, fmt.Sprintf("scan_builder_queues:%d", b.ID))
			if err != nil {
				logging.Debugf(ctx, "error creating PopPendingBuildTask for build %d: %s", b.ID, err)
			}
			return nil
		})
	}
	_ = eg.Wait()
}
