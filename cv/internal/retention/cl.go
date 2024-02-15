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
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run/runquery"
)

// clsPerTask controls how many cls to wipeout per TQ task.
const clsPerTask = 200

// scheduleWipeoutCLTasks schedules tasks to wipe out old CLs that are out of
// the retention period.
//
// The tasks will be uniformly distributed over the next hour.
func scheduleWipeoutCLTasks(ctx context.Context, tqd *tq.Dispatcher) error {
	cls, err := changelist.QueryCLIDsUpdatedBefore(ctx, clock.Now(ctx).Add(-retentionPeriod))
	switch {
	case err != nil:
		return err
	case len(cls) == 0:
		logging.Infof(ctx, "no CLs to wipe out")
		return nil
	}

	logging.Infof(ctx, "schedule tasks to wipeout %d CLs", len(cls))
	return parallel.WorkPool(min(10, len(cls)/clsPerTask), func(workCh chan<- func() error) {
		for _, chunk := range chunk(cls, clsPerTask) {
			clidStrs := make([]string, len(chunk))
			for i, clid := range chunk {
				clidStrs[i] = strconv.FormatInt(int64(clid), 10)
			}
			task := &tq.Task{
				Payload: &WipeoutCLsTask{
					Ids: common.CLIDsAsInt64s(chunk),
				},
				Delay: common.DistributeOffset(wipeoutTasksDistInterval, clidStrs...),
			}
			workCh <- func() error {
				return retry.Retry(ctx, retry.Default, func() error {
					return tqd.AddTask(ctx, task)
				}, nil)
			}
		}
	})
}

func registerWipeoutCLsTask(tqd *tq.Dispatcher) {
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:           "wipeout-cls",
		Queue:        "data-retention",
		Prototype:    &WipeoutCLsTask{},
		Kind:         tq.NonTransactional,
		Quiet:        true,
		QuietOnError: true,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*WipeoutCLsTask)
			err := wipeoutCLs(ctx, common.MakeCLIDs(task.GetIds()...))
			return common.TQifyError(ctx, err)
		},
	})
}

// wipeoutCLs wipes out the provided CLs.
func wipeoutCLs(ctx context.Context, clids common.CLIDs) error {
	return parallel.WorkPool(min(10, len(clids)), func(workCh chan<- func() error) {
		for _, clid := range clids {
			clid := clid
			workCh <- func() error {
				return wipeoutCL(ctx, clid)
			}
		}
	})
}

// wipeoutCL wipes out the provided CL if CL is no longer referenced by any
// runs.
func wipeoutCL(ctx context.Context, clid common.CLID) error {
	ctx = logging.SetField(ctx, "CL", clid)
	runs, err := runquery.CLQueryBuilder{
		CLID:  clid,
		Limit: 1, // As long as there is 1 run referencing this CL, CV can't delete
	}.GetAllRunKeys(ctx)
	switch {
	case err != nil:
		return errors.Annotate(err, "failed to query runs involving CL %d", clid).Tag(transient.Tag).Err()
	case len(runs) > 0:
		logging.Warningf(ctx, "WipeoutCL: skip wipeout because CL is still referenced by run %s", runs[0].StringID())
		return nil
	}

	if err := changelist.Delete(ctx, clid); err != nil {
		return errors.Annotate(err, "failed to delete CL %d", clid).Tag(transient.Tag).Err()
	}
	logging.Infof(ctx, "successfully wiped out CL %d", clid)
	return nil
}
