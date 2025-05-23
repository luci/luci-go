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
	"strconv"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// tryjobsPerTask controls how many tryjobs to wipeout per TQ task.
const tryjobsPerTask = 800

// scheduleWipeoutTryjobsTasks schedules tasks to wipe out old tryjobs that are
// out of the retention period.
//
// The tasks will be uniformly distributed over the next 1 hours.
func scheduleWipeoutTryjobsTasks(ctx context.Context, tqd *tq.Dispatcher) error {
	tryjobs, err := tryjob.QueryTryjobIDsUpdatedBefore(ctx, clock.Now(ctx).Add(-retentionPeriod))
	switch {
	case err != nil:
		return err
	case len(tryjobs) == 0:
		logging.Infof(ctx, "no tryjobs to wipe out")
		return nil
	}

	logging.Infof(ctx, "schedule tasks to wipeout %d tryjobs", len(tryjobs))
	return parallel.WorkPool(min(10, len(tryjobs)/tryjobsPerTask), func(workCh chan<- func() error) {
		for _, chunk := range chunk(tryjobs, tryjobsPerTask) {
			tryjobIDStrs := make([]string, len(chunk))
			for i, tjID := range chunk {
				tryjobIDStrs[i] = strconv.FormatInt(int64(tjID), 10)
			}
			task := &tq.Task{
				Payload: &WipeoutTryjobsTask{
					Ids: common.TryjobIDs(chunk).ToInt64(),
				},
				Delay: common.DistributeOffset(wipeoutTasksDistInterval, tryjobIDStrs...),
			}
			workCh <- func() error {
				return retry.Retry(ctx, retry.Default, func() error {
					return tqd.AddTask(ctx, task)
				}, nil)
			}
		}
	})
}

func registerWipeoutTryjobsTask(tqd *tq.Dispatcher) {
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:           "wipeout-tryjobs",
		Queue:        "data-retention",
		Prototype:    &WipeoutTryjobsTask{},
		Kind:         tq.NonTransactional,
		Quiet:        true,
		QuietOnError: true,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*WipeoutTryjobsTask)
			err := wipeoutTryjobs(ctx, common.MakeTryjobIDs(task.GetIds()...))
			return common.TQifyError(ctx, err)
		},
	})
}

// wipeoutTryjobs wipes out the provided tryjobs.
func wipeoutTryjobs(ctx context.Context, ids common.TryjobIDs) error {
	tryjobs, err := loadTryjobsIgnoreMissing(ctx, ids)
	if err != nil {
		return err
	}
	return parallel.WorkPool(min(10, len(tryjobs)), func(workCh chan<- func() error) {
		for _, tj := range tryjobs {
			workCh <- func() error {
				return wipeoutTryjob(ctx, tj)
			}
		}
	})
}

func loadTryjobsIgnoreMissing(ctx context.Context, ids common.TryjobIDs) ([]*tryjob.Tryjob, error) {
	tryjobs := make([]*tryjob.Tryjob, len(ids))
	for i, tjID := range ids {
		tryjobs[i] = &tryjob.Tryjob{ID: tjID}
	}
	err := datastore.Get(ctx, tryjobs)
	var merrs errors.MultiError
	switch {
	case err == nil:
		return tryjobs, nil
	case errors.As(err, &merrs):
		ret := tryjobs[:0] // reuse the same slice
		for i, err := range merrs {
			switch {
			case err == nil:
				ret = append(ret, tryjobs[i])
			case !errors.Is(err, datastore.ErrNoSuchEntity):
				count, err := merrs.Summary()
				return nil, errors.Annotate(err, "failed to load %d out of %d tryjobs", count, len(ids)).Tag(transient.Tag).Err()
			}
		}
		return ret, nil
	default:
		return nil, errors.Annotate(err, "failed to load tryjobs").Tag(transient.Tag).Err()
	}
}

// wipeoutTryjob wipes out the provided tryjob if all runs that use this tryjob
// no longer exists.
func wipeoutTryjob(ctx context.Context, tj *tryjob.Tryjob) error {
	ctx = logging.SetField(ctx, "tryjob", tj.ID)

	var runs []any
	for _, rid := range tj.AllWatchingRuns() {
		runs = append(runs, &run.Run{ID: rid})
	}
	if len(runs) > 0 {
		switch res, err := datastore.Exists(ctx, runs...); {
		case err != nil:
			return errors.Annotate(err, "failed to check the existence of runs for Tryjob %d", tj.ID).Tag(transient.Tag).Err()
		case res.Any():
			logging.Warningf(ctx, "WipeoutTryjob: skip wipeout because some run(s) using this tryjob still exists")
			return nil
		}
	}

	if err := tryjob.CondDelete(ctx, tj.ID, tj.EVersion); err != nil {
		return err
	}
	logging.Infof(ctx, "successfully wiped out tryjob %d", tj.ID)
	return nil
}
