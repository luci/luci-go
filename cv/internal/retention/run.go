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

package retention

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/runquery"
)

// runsPerTask controls how many runs to wipeout per TQ task.
const runsPerTask = 200

// scheduleWipeoutRuns schedules tasks to wipe out old runs that are out of the
// retention period.
//
// The tasks will be uniformly distributed over the next 16 hours.
// TODO(yiwzhang): change it to 1 hour after the first execution that needs
// to delete ~1 million runs.
func scheduleWipeoutRuns(ctx context.Context, tqd *tq.Dispatcher) error {
	// data retention should work for disabled projects as well
	projects, err := prjcfg.GetAllProjectIDs(ctx, false)
	if err != nil {
		return err
	}

	cutoff := clock.Now(ctx).Add(-retentionPeriod).UTC()
	dc, err := dispatcher.NewChannel(ctx, &dispatcher.Options{
		DropFn: dispatcher.DropFnQuiet,
		Buffer: buffer.Options{
			MaxLeases:     10,
			BatchItemsMax: runsPerTask,
			FullBehavior:  &buffer.InfiniteGrowth{},
			Retry:         retry.Default,
		},
	}, func(b *buffer.Batch) error {
		runIDStrs := make(sort.StringSlice, len(b.Data))
		for i, item := range b.Data {
			runIDStrs[i] = string(item.Item.(common.RunID))
		}
		sort.Sort(runIDStrs)
		task := &tq.Task{
			Payload: &WipeoutRunsTask{
				Ids: runIDStrs,
			},
			Delay: common.DistributeOffset(16*time.Hour, runIDStrs...),
		}
		return tqd.AddTask(ctx, task)
	})
	if err != nil {
		panic(errors.Annotate(err, "failed to create dispatcher to schedule wipeout tasks"))
	}

	var wg sync.WaitGroup
	wg.Add(len(projects))
	poolErr := parallel.WorkPool(min(8, len(projects)), func(workCh chan<- func() error) {
		for _, proj := range projects {
			proj := proj
			workCh <- func() error {
				defer wg.Done()
				runs, err := findRunsToWipeoutForProject(ctx, proj, cutoff)
				switch {
				case err != nil:
					return errors.Annotate(err, "failed to find runs to wipe out for project %q", proj).Tag(transient.Tag).Err()
				case len(runs) == 0:
					return nil
				}
				logging.Infof(ctx, "found %d runs to wipeout for project %q", len(runs), proj)
				for _, r := range runs {
					dc.C <- r
				}
				return nil
			}
		}
	})
	wg.Wait()
	dc.CloseAndDrain(ctx)
	return poolErr
}

func findRunsToWipeoutForProject(ctx context.Context, proj string, cutoff time.Time) (common.RunIDs, error) {
	// cutoffRunID is a non-existing run ID used for range query purpose
	// only. All the runs in the query result should be created strictly
	// before the cutoff time.
	cutoffRunID := common.MakeRunID(proj, cutoff, math.MaxInt, []byte("whatever"))
	qb := runquery.ProjectQueryBuilder{
		Project: proj,
	}.Before(cutoffRunID)
	keys, err := qb.GetAllRunKeys(ctx)
	if err != nil {
		return nil, err
	}
	ret := make(common.RunIDs, len(keys))
	for i, key := range keys {
		ret[i] = common.RunID(key.StringID())
	}
	return ret, nil
}

func registerWipeoutRunsTask(tqd *tq.Dispatcher) {
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:           "wipeout-runs",
		Queue:        "data-retention",
		Prototype:    &WipeoutRunsTask{},
		Kind:         tq.NonTransactional,
		Quiet:        true,
		QuietOnError: true,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*WipeoutRunsTask)
			err := wipeoutRuns(ctx, common.MakeRunIDs(task.GetIds()...))
			return common.TQifyError(ctx, err)
		},
	})
}

// wipeoutRuns wipes out runs for the provided run IDs.
//
// skip runs that do not exist or are still in retention period.
func wipeoutRuns(ctx context.Context, runIDs common.RunIDs) error {
	runs, err := run.LoadRunsFromIDs(runIDs...).DoIgnoreNotFound(ctx)
	switch {
	case err != nil:
		return errors.Annotate(err, "failed to load runs").Tag(transient.Tag).Err()
	case len(runs) == 0:
		return nil
	}

	return parallel.WorkPool(min(10, len(runIDs)), func(workC chan<- func() error) {
		for _, r := range runs {
			r := r
			workC <- func() error {
				return wipeoutRun(ctx, r)
			}
		}
	})
}

// wipeoutRun wipes out the given run if run is no longer in retention period.
//
// No-op if the run is still in the retention period.
func wipeoutRun(ctx context.Context, r *run.Run) error {
	ctx = logging.SetField(ctx, "run", string(r.ID))
	if !r.CreateTime.Before(clock.Now(ctx).Add(-retentionPeriod)) {
		// skip if it is still in the retention period.
		logging.Warningf(ctx, "WipeoutRun: too young to wipe out: %s < %s",
			clock.Now(ctx).Sub(r.CreateTime), retentionPeriod)
		return nil
	}

	// Find out all the child entities of Run entities. As of Jan. 2024, this
	// includes:
	//  - RunLog
	//  - RunCL
	//  - TryjobExecutionState
	//  - TryjobExecutionLog
	runKey := datastore.KeyForObj(ctx, r)
	var toDelete []*datastore.Key
	q := datastore.NewQuery("").Ancestor(runKey).KeysOnly(true)
	if err := datastore.GetAll(ctx, q, &toDelete); err != nil {
		return errors.Annotate(err, "failed to query all child entities of run").Tag(transient.Tag).Err()
	}
	toDelete = append(toDelete, runKey)

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		return datastore.Delete(ctx, toDelete)
	}, nil)

	if err != nil {
		return errors.Annotate(err, "failed to delete run entities and it's child entities in a transaction").Tag(transient.Tag).Err()
	}
	logging.Infof(ctx, "successfully wiped out run %s", r.ID)
	return nil
}
