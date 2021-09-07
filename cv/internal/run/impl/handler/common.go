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
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/filter/txndefer"

	commonpb "go.chromium.org/luci/cv/api/common/v1"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit/cancel"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// endRun sets Run to the provided status and populates `EndTime`.
//
// Returns the side effect when Run is ended.
//
// Panics if the provided status is not ended status.
func (impl *Impl) endRun(ctx context.Context, rs *state.RunState, st commonpb.Run_Status) eventbox.SideEffectFn {
	if !run.IsEnded(st) {
		panic(fmt.Errorf("can't end run with non-final status %s", st))
	}

	rs.Run.Status = st
	now := clock.Now(ctx)
	rs.Run.EndTime = now.UTC()
	rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
		Time: timestamppb.New(now),
		Kind: &run.LogEntry_RunEnded_{
			RunEnded: &run.LogEntry_RunEnded{},
		},
	})
	rid := rs.Run.ID

	return eventbox.Chain(
		func(ctx context.Context) error {
			return impl.removeRunFromCLs(ctx, rid, rs.Run.CLs)
		},
		func(ctx context.Context) error {
			txndefer.Defer(ctx, func(postTransCtx context.Context) {
				logging.Infof(postTransCtx, "finalized Run with status %s", st)
			})
			return impl.PM.NotifyRunFinished(ctx, rid)
		},
		func(ctx context.Context) error {
			return impl.BQExporter.Schedule(ctx, rid)
		},
		func(ctx context.Context) error {
			return impl.Publisher.RunEnded(ctx, &rs.Run)
		},
	)
}

// removeRunFromCLs atomically updates state of CL entities involved in this Run.
//
// For each CL:
//   * marks its Snapshot as outdated, which prevents Project Manager from
//     operating on potentially outdated CL Snapshots;
//   * schedules refresh of CL snapshot;
//   * removes Run's ID from the list of CL's IncompleteRuns.
func (impl *Impl) removeRunFromCLs(ctx context.Context, runID common.RunID, clids common.CLIDs) error {
	muts, err := impl.CLMutator.BeginBatch(ctx, runID.LUCIProject(), clids)
	if err != nil {
		return err
	}
	for _, mut := range muts {
		mut.CL.IncompleteRuns.DelSorted(runID)
		if mut.CL.Snapshot != nil {
			mut.CL.Snapshot.Outdated = &changelist.Snapshot_Outdated{}
		}
	}
	cls, err := impl.CLMutator.FinalizeBatch(ctx, muts)
	if err != nil {
		return err
	}
	return impl.CLUpdater.ScheduleBatch(ctx, runID.LUCIProject(), cls)
}

func (impl *Impl) cancelCLTriggers(ctx context.Context, runID common.RunID, toCancel []*run.RunCL, runCLExternalIDs []changelist.ExternalID, message string, cg *prjcfg.ConfigGroup) error {
	clids := make(common.CLIDs, len(toCancel))
	for i, runCL := range toCancel {
		clids[i] = runCL.ID
	}
	cls, err := changelist.LoadCLs(ctx, clids)
	if err != nil {
		return err
	}

	luciProject := runID.LUCIProject()
	var forceRefresh []*changelist.CL
	var lock sync.Mutex
	err = parallel.WorkPool(min(len(toCancel), 10), func(work chan<- func() error) {
		for i := range toCancel {
			i := i
			work <- func() error {
				err := cancel.Cancel(ctx, impl.GFactory, cancel.Input{
					CL:               cls[i],
					Trigger:          toCancel[i].Trigger,
					LUCIProject:      luciProject,
					Message:          message,
					Requester:        "Run Manager",
					Notify:           cancel.OWNER | cancel.VOTERS,
					LeaseDuration:    time.Minute,
					ConfigGroups:     []*prjcfg.ConfigGroup{cg},
					RunCLExternalIDs: runCLExternalIDs,
				})
				switch {
				case err == nil:
					return nil
				case cancel.ErrPreconditionFailedTag.In(err) || cancel.ErrPermanentTag.In(err):
					lock.Lock()
					forceRefresh = append(forceRefresh, cls[i])
					lock.Unlock()
					fallthrough
				default:
					return errors.Annotate(err, "failed to cancel triggers for cl %d", cls[i].ID).Err()
				}
			}
		}
	})
	if len(forceRefresh) != 0 {
		if ferr := impl.CLUpdater.ScheduleBatch(ctx, runID.LUCIProject(), forceRefresh); ferr != nil {
			logging.Warningf(ctx, "Failed to schedule best-effort force refresh of %d CLs: %s", len(forceRefresh), ferr)
		}
	}
	if err != nil {
		switch merr, ok := err.(errors.MultiError); {
		case !ok:
			return err
		default:
			return common.MostSevereError(merr)
		}
	}
	return nil
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
