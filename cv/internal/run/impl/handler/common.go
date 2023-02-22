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
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/acls"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/rpc/versioning"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// endRun sets Run to the provided status and populates `EndTime`.
//
// Returns the side effect when Run is ended.
//
// Panics if the provided status is not ended status.
func (impl *Impl) endRun(ctx context.Context, rs *state.RunState, st run.Status) eventbox.SideEffectFn {
	if !run.IsEnded(st) {
		panic(fmt.Errorf("can't end run with non-final status %s", st))
	}

	rs.Status = st
	now := datastore.RoundTime(clock.Now(ctx).UTC())
	rs.EndTime = now
	rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
		Time: timestamppb.New(now),
		Kind: &run.LogEntry_RunEnded_{
			RunEnded: &run.LogEntry_RunEnded{},
		},
	})
	for id, op := range rs.OngoingLongOps.GetOps() {
		if !op.GetCancelRequested() {
			logging.Warningf(ctx, "Requesting best-effort cancellation of long op %q %T", id, op.GetWork())
			op.CancelRequested = true
		}
	}

	return eventbox.Chain(
		func(ctx context.Context) error {
			return impl.removeRunFromCLs(ctx, rs.ID, rs.CLs)
		},
		func(ctx context.Context) error {
			txndefer.Defer(ctx, func(postTransCtx context.Context) {
				logging.Infof(postTransCtx, "finalized Run with status %s", st)
			})
			return impl.PM.NotifyRunFinished(ctx, rs.ID)
		},
		func(ctx context.Context) error {
			switch rs.Mode {
			case run.NewPatchsetRun:
				// Do not export NPRs.
				return nil
			default:
				return impl.BQExporter.Schedule(ctx, rs.ID)
			}
		},
		func(ctx context.Context) error {
			// If this Run is successfully ended (i.e. saved successfully to
			// Datastore), the EVersion will be increased by 1 based on how
			// eventbox works. If this eventbox behavior is changed in the future,
			// this logic should be revisited.
			return impl.Publisher.RunEnded(ctx, rs.ID, rs.Status, rs.EVersion+1)
		},
		func(ctx context.Context) error {
			txndefer.Defer(ctx, func(ctx context.Context) {
				commonFields := []any{
					rs.ID.LUCIProject(),
					rs.ConfigGroupID.Name(),
					string(rs.Mode),
					versioning.RunStatusV0(rs.Status).String(), // translate to public status
				}
				successfullyStarted := !rs.StartTime.IsZero()
				startAwareFields := append(commonFields, successfullyStarted)
				metrics.Public.RunEnded.Add(ctx, 1, startAwareFields...)
				if successfullyStarted {
					// Some run might not start successfully. E.g. user doesn't have the
					// privilege to start the Run, those Runs will be created but ended
					// by CV right away. Therefore, when the duration calculation (end-
					// start) is not applicable for those Runs.
					metrics.Public.RunDuration.Add(ctx, rs.EndTime.Sub(rs.StartTime).Seconds(), commonFields...)
				}
				metrics.Public.RunTotalDuration.Add(ctx, rs.EndTime.Sub(rs.CreateTime).Seconds(), startAwareFields...)
			})
			return nil
		},
	)
}

// removeRunFromCLs atomically updates state of CL entities involved in this
// Run.
//
// For each CL:
//   - marks its Snapshot as outdated, which prevents Project Manager from
//     operating on potentially outdated CL Snapshots;
//   - schedules refresh of CL snapshot;
//   - removes Run's ID from the list of CL's IncompleteRuns.
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
	return impl.CLUpdater.ScheduleBatch(ctx, runID.LUCIProject(), cls, changelist.UpdateCLTask_RUN_REMOVAL)
}

type reviewInputMeta struct {

	// notify is whom to notify.
	notify gerrit.Whoms
	// message provides the reason and details of the review change performed.
	//
	// This is posted as a comment in the CL.
	message string
	// addToAttention is whom to add in the attention set.
	addToAttention gerrit.Whoms
	// reason explains the reason of the attention.
	reason string
}

func (impl *Impl) resetCLTriggers(ctx context.Context, runID common.RunID, toReset []*run.RunCL, cg *prjcfg.ConfigGroup, meta reviewInputMeta) error {
	clids := make(common.CLIDs, len(toReset))
	for i, runCL := range toReset {
		clids[i] = runCL.ID
	}

	cls, err := changelist.LoadCLsByIDs(ctx, clids)
	if err != nil {
		return err
	}

	luciProject := runID.LUCIProject()
	var forceRefresh []*changelist.CL
	var lock sync.Mutex
	err = parallel.WorkPool(min(len(toReset), 10), func(work chan<- func() error) {
		for i := range cls {
			i := i
			work <- func() error {
				err := trigger.Reset(ctx, trigger.ResetInput{
					CL:                cls[i],
					Triggers:          (&run.Triggers{}).WithTrigger(toReset[i].Trigger),
					LUCIProject:       luciProject,
					Message:           meta.message,
					Requester:         "Run Manager",
					Notify:            meta.notify,
					LeaseDuration:     time.Minute,
					ConfigGroups:      []*prjcfg.ConfigGroup{cg},
					AddToAttentionSet: meta.addToAttention,
					AttentionReason:   meta.reason,
					GFactory:          impl.GFactory,
					CLMutator:         impl.CLMutator,
				})
				switch {
				case err == nil:
					return nil
				case trigger.ErrResetPreconditionFailedTag.In(err) || trigger.ErrResetPermanentTag.In(err):
					lock.Lock()
					forceRefresh = append(forceRefresh, cls[i])
					lock.Unlock()
					fallthrough
				default:
					return errors.Annotate(err, "failed to reset triggers for cl %d", cls[i].ID).Err()
				}
			}
		}
	})
	if len(forceRefresh) != 0 {
		if ferr := impl.CLUpdater.ScheduleBatch(ctx, runID.LUCIProject(), forceRefresh, changelist.UpdateCLTask_RESET_CL_TRIGGER); ferr != nil {
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

// scheduleTriggersReset enqueues a ResetTriggers long op for a given Run.
//
// No-op if trigger reset is already ongoing.
func scheduleTriggersReset(ctx context.Context, rs *state.RunState, metas map[common.CLID]reviewInputMeta, statusIfSucceeded run.Status) {
	switch {
	case !run.IsEnded(statusIfSucceeded):
		panic(fmt.Errorf("expected a terminal status; got %s", statusIfSucceeded))
	case isCurrentlyResettingTriggers(rs):
		return
	}
	reqs := make([]*run.OngoingLongOps_Op_TriggersCancellation_Request, 0, len(rs.CLs))
	for clid, meta := range metas {
		reqs = append(reqs, &run.OngoingLongOps_Op_TriggersCancellation_Request{
			Clid:                 int64(clid),
			Notify:               fromGerritWhoms(meta.notify),
			Message:              meta.message,
			AddToAttention:       fromGerritWhoms(meta.addToAttention),
			AddToAttentionReason: meta.reason,
		})
	}
	rs.EnqueueLongOp(&run.OngoingLongOps_Op{
		Deadline: timestamppb.New(clock.Now(ctx).Add(maxTriggersCancellationDuration)),
		Work: &run.OngoingLongOps_Op_CancelTriggers{
			CancelTriggers: &run.OngoingLongOps_Op_TriggersCancellation{
				Requests:             reqs,
				RunStatusIfSucceeded: statusIfSucceeded,
			},
		},
	})
}

func isCurrentlyResettingTriggers(rs *state.RunState) bool {
	for _, op := range rs.OngoingLongOps.GetOps() {
		if op.GetCancelTriggers() != nil {
			return true
		}
	}
	return false
}

func fromGerritWhoms(whoms gerrit.Whoms) []run.OngoingLongOps_Op_TriggersCancellation_Whom {
	if len(whoms) == 0 {
		return nil
	}
	ret := make([]run.OngoingLongOps_Op_TriggersCancellation_Whom, len(whoms))
	for i, whom := range whoms {
		switch whom {
		case gerrit.Owner:
			ret[i] = run.OngoingLongOps_Op_TriggersCancellation_OWNER
		case gerrit.Reviewers:
			ret[i] = run.OngoingLongOps_Op_TriggersCancellation_REVIEWERS
		case gerrit.CQVoters:
			ret[i] = run.OngoingLongOps_Op_TriggersCancellation_CQ_VOTERS
		case gerrit.PSUploader:
			ret[i] = run.OngoingLongOps_Op_TriggersCancellation_PS_UPLOADER
		default:
			panic(fmt.Errorf("unknown gerrit.Whom; got %s", whom))
		}
	}
	return ret
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}

func loadCLsAndConfig(ctx context.Context, rs *state.RunState, clids common.CLIDs) (*prjcfg.ConfigGroup, []*run.RunCL, []*changelist.CL, error) {
	var cg *prjcfg.ConfigGroup
	var runCLs []*run.RunCL
	var cls []*changelist.CL
	eg, ectx := errgroup.WithContext(ctx)
	eg.Go(func() (err error) {
		cg, err = prjcfg.GetConfigGroup(ectx, rs.ID.LUCIProject(), rs.ConfigGroupID)
		return err
	})
	eg.Go(func() (err error) {
		cls, err = changelist.LoadCLsByIDs(ectx, clids)
		return err
	})
	eg.Go(func() (err error) {
		runCLs, err = run.LoadRunCLs(ectx, rs.ID, clids)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, nil, nil, err
	}
	return cg, runCLs, cls, nil
}

func loadRunCLsAndCLs(ctx context.Context, rid common.RunID, clids common.CLIDs) ([]*run.RunCL, []*changelist.CL, error) {
	var runCLs []*run.RunCL
	var cls []*changelist.CL
	eg, ectx := errgroup.WithContext(ctx)
	eg.Go(func() (err error) {
		cls, err = changelist.LoadCLsByIDs(ectx, clids)
		return err
	})
	eg.Go(func() (err error) {
		runCLs, err = run.LoadRunCLs(ectx, rid, clids)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, nil, err
	}
	return runCLs, cls, nil
}

func checkRunCreate(ctx context.Context, rs *state.RunState, cg *prjcfg.ConfigGroup, runCLs []*run.RunCL, cls []*changelist.CL) (ok bool, err error) {
	if len(runCLs) == 0 {
		return true, nil
	}
	trs := make([]*run.Trigger, len(runCLs))
	for i, r := range runCLs {
		trs[i] = r.Trigger
	}
	switch aclResult, err := acls.CheckRunCreate(ctx, cg, trs, cls); {
	case err != nil:
		return false, errors.Annotate(err, "acls.CheckRunCreate").Err()
	case !aclResult.OK():
		var b strings.Builder
		b.WriteString("the Run does not pass eligibility checks. See reasons at:")
		for cl := range aclResult {
			fmt.Fprintf(&b, "\n  * %s", cl.ExternalID.MustURL())
		}
		rs.LogInfof(ctx, "Run failed", b.String())
		metas := make(map[common.CLID]reviewInputMeta, len(cls))
		for _, cl := range cls {
			switch rs.Mode {
			case run.NewPatchsetRun:
				// silently
				metas[cl.ID] = reviewInputMeta{}
			case run.DryRun, run.QuickDryRun, run.FullRun:
				metas[cl.ID] = reviewInputMeta{
					message:        aclResult.Failure(cl),
					notify:         gerrit.Whoms{gerrit.Owner, gerrit.CQVoters},
					addToAttention: gerrit.Whoms{gerrit.Owner, gerrit.CQVoters},
					reason:         "CQ/CV Run failed",
				}
			default:
				panic(errors.Reason("unknown run mode %s", rs.Mode).Err())
			}
		}
		scheduleTriggersReset(ctx, rs, metas, run.Status_FAILED)
		return false, nil
	}
	return true, nil
}
