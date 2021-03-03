// Copyright 2021 The LUCI Authors.
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

package handler

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/impl/submit"
)

// Finalize finalizes a run.
//
// Submits CLs in order if this run is FullRun and all Tryjobs have passed.
// Otherwise, Cancels the trigger and posts the result on each CL.
func (*Impl) Finalize(ctx context.Context, rs *state.RunState) (eventbox.SideEffectFn, *state.RunState, error) {
	switch status := rs.Run.Status; {
	case run.IsEnded(status):
		logging.Warningf(ctx, "run has already finalized; skip finalization")
		return nil, rs, nil
	case status != run.Status_FINALIZING:
		return nil, nil, errors.Reason("expected run has %s status; got %s", run.Status_FINALIZING, status).Err()
	}

	cls, err := rs.LoadRunCLs(ctx)
	if err != nil {
		return nil, nil, err
	}
	passed, _, err := rs.FetchTryjobResult(ctx)
	if err != nil {
		return nil, nil, err
	}

	switch rid, mode := rs.Run.ID, rs.Run.Mode; {
	case mode == run.FullRun && passed:
		// TODO: check tree status. If closed, enqueue finalize event and recheck
		// after x minutes.
		res, err := submit.Reserve(ctx, submit.ReserveInput{
			RunID:    rs.Run.ID,
			CLs:      cls,
			Deadline: computeDeadline(ctx, len(cls)),
		})
		switch {
		case err != nil:
			return nil, nil, errors.Annotate(err, "failed to reserve submission queue").Err()
		case res.Waitlisted:
			// Return because submission queue will notify this Run to finalize
			// when it comes to its turn.
			return nil, rs, nil
		}
		ctx, cancel := context.WithDeadline(ctx, res.Deadline)
		defer cancel()
		switch l := len(cls); {
		case l == 0:
			panic(fmt.Errorf("impossible; Run %q has 0 RunCL", rid))
		case l == 1:
			// TODO: submit first CL
		default:
			reportCh, err := dispatcher.NewChannel(ctx, &dispatcher.Options{
				Buffer: buffer.Options{
					MaxLeases:    1,
					BatchSize:    1,
					FullBehavior: &buffer.DropOldestBatch{},
				},
				DropFn: dispatcher.DropFnQuiet,
			}, func(b *buffer.Batch) error {
				return submit.SaveProgress(ctx, rid, b.Data[0].(common.CLIDs))
			})
			if err != nil {
				return nil, nil, errors.Annotate(err, "failed to create submission progress report channel").Err()
			}
			for i := range res.RemainingCLs {
				// TODO: Submit Gerrit CL
				if i < len(res.RemainingCLs)-1 {
					reportCh.C <- res.RemainingCLs[i+1:]
				}
			}
			reportCh.CloseAndDrain(ctx)
			if err := submit.MarkComplete(ctx, rid); err != nil {
				return nil, nil, errors.Annotate(err, "failed to mark run submission as complete").Err()
			}
		}
		rs = rs.ShallowCopy()
		rs.Run.Status = run.Status_SUCCEEDED
		rs.Run.EndTime = clock.Now(ctx)
		return nil, rs, nil
	case mode == run.DryRun || !passed:
		// TODO: cancel Trigger.
		return nil, rs, nil
	default:
		return nil, nil, errors.Reason("unknown run mode %s; can't finalize the run", mode).Err()
	}
}

var maxDurationPerCL = 3 * time.Second

func computeDeadline(ctx context.Context, numCLs int) time.Time {
	ret := clock.Now(ctx).Add(maxDurationPerCL * time.Duration(numCLs))
	if dl, ok := ctx.Deadline(); ok && dl.Before(ret) {
		ret = dl
	}
	return ret
}
