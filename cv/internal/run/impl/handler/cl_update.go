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
	"sort"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// OnCLsUpdated implements Handler interface.
func (impl *Impl) OnCLsUpdated(ctx context.Context, rs *state.RunState, clids common.CLIDs) (*Result, error) {
	switch status := rs.Status; {
	case status == run.Status_STATUS_UNSPECIFIED:
		err := errors.Reason("CRITICAL: Received CLUpdated events but Run is in unspecified status").Err()
		common.LogError(ctx, err)
		panic(err)
	case status == run.Status_SUBMITTING:
		return &Result{State: rs, PreserveEvents: true}, nil
	case isCurrentlyResettingTriggers(rs):
		// It's likely CL is updated when resetting the triggers, defer the process
		// of CLsUpdated event till resetting the trigger is done.
		return &Result{State: rs, PreserveEvents: true}, nil
	case run.IsEnded(status):
		logging.Debugf(ctx, "skipping OnCLUpdated because Run is %s", status)
		return &Result{State: rs}, nil
	}
	clids.Dedupe()

	cg, runCLs, cls, err := loadCLsAndConfig(ctx, rs, clids)
	if err != nil {
		return nil, err
	}

	hasNilSnapshot := false
	var earliestReconsiderAt time.Time
	clsCausingCancellation := make(common.CLIDsSet, len(clids))
	cancellationReasons := make([]string, 0, len(clids))
	for i, clid := range clids {
		cl, runCL := cls[i], runCLs[i]
		if cl.Snapshot == nil {
			// This doesn't necessarily assume that shouldCancel() would
			// or would not return a cancelReason for nil Snapshot; hence,
			// let it decide. hasNilSnapshot is to decide whether
			// runs.CheckRunCreate() should be checked or not.
			hasNilSnapshot = true
		}
		switch reconsiderAt, cancellationReason := shouldCancel(ctx, cl, runCL, cg); {
		case !reconsiderAt.IsZero():
			if earliestReconsiderAt.IsZero() || earliestReconsiderAt.After(reconsiderAt) {
				earliestReconsiderAt = reconsiderAt
			}
		case cancellationReason != "":
			clsCausingCancellation.Add(clid)
			cancellationReasons = append(cancellationReasons, cancellationReason)
		}
	}

	sort.Strings(cancellationReasons)
	switch numCLsCausingCancellation := len(clsCausingCancellation); {
	// If all CLs in this Run have been updated at the same time and cause
	// Run cancellation, directly cancel the Run. Otherwise, if part of the
	// CLs cause Run cancellation, LUCI CV would need to reset the trigger on
	// the rest of the CLs in this Run so that the existing trigger won't
	// end up with creating new Run that developers are not intended. For example
	// imagine a stack of CL A, B, C where C depends on B depends on A and a
	// Dry Run is currently running involving all CLs. If B gains a new patchset,
	// CV needs to reset the trigger on A and C and then patiently wait for
	// developers creating a new Run with all 3 CLs.
	case numCLsCausingCancellation == len(rs.CLs):
		return impl.Cancel(ctx, rs, cancellationReasons)
	case rs.HasRootCL() && clsCausingCancellation.Has(rs.RootCL):
		// if the root cl has caused the cancellation, there's no need to reset
		// the trigger for all other CLs because they don't have trigger at all.
		// Directly cancelling the run would be enough.
		return impl.Cancel(ctx, rs, cancellationReasons)
	case numCLsCausingCancellation > 0:
		rs = rs.ShallowCopy()
		meta := reviewInputMeta{
			// Just pick the very first reason as the reason to reset the trigger.
			// In typical cases, there will only be one cancellation reason anyway.
			message: fmt.Sprintf("Reset the trigger of this CL because %s", cancellationReasons[0]),
			notify:  rs.Mode.GerritNotifyTargets(),
		}
		metas := make(map[common.CLID]reviewInputMeta, len(rs.CLs)-numCLsCausingCancellation)
		if rs.HasRootCL() {
			metas[rs.RootCL] = meta
		} else {
			for _, clid := range rs.CLs {
				if !clsCausingCancellation.Has(clid) {
					metas[clid] = meta
				}
			}
		}
		scheduleTriggersReset(ctx, rs, metas, run.Status_CANCELLED)
		// TODO(yiwzhang): It is unfortunate that CV has to set the cancellation
		// reason before triggers are reset and status is turned to CANCELLED. Find
		// a way to pass the cancellation reasons to `onCompletedResetTriggers`.
		rs.CancellationReasons = append(rs.CancellationReasons, cancellationReasons...)
		sort.Strings(rs.CancellationReasons)
		return &Result{State: rs}, nil
	case !earliestReconsiderAt.IsZero():
		logging.Debugf(ctx, "Will reconsider OnCLUpdated event(s) after %s", earliestReconsiderAt.Sub(clock.Now(ctx)))
		if err := impl.RM.Invoke(ctx, rs.ID, earliestReconsiderAt); err != nil {
			return nil, err
		}
		return &Result{State: rs, PreserveEvents: true}, nil
	}

	// If any of the CLs has a nil snapshot, skip acls.CheckRunCreate().
	// It needs snapshots for the entire CL set.
	if hasNilSnapshot {
		return &Result{State: rs}, nil
	}

	// Check the Run creation, in case the Run is no longer valid
	// with the newly updated CL info.
	rs = rs.ShallowCopy()
	allRunCLs, allCLs := runCLs, cls
	remainingCLIDs := rs.CLs.Set()
	remainingCLIDs.DelAll(clids)
	if len(remainingCLIDs) > 0 {
		remainingRunCLs, remainingCLs, err := loadRunCLsAndCLs(ctx, rs.ID, remainingCLIDs.ToCLIDs())
		if err != nil {
			return nil, err
		}
		allRunCLs = append(allRunCLs, remainingRunCLs...)
		allCLs = append(allCLs, remainingCLs...)
	}
	if _, err := checkRunCreate(ctx, impl.GFactory, rs, cg, allRunCLs, allCLs); err != nil {
		return nil, err
	}
	return &Result{State: rs}, nil
}

func shouldCancel(ctx context.Context, cl *changelist.CL, rcl *run.RunCL, cg *prjcfg.ConfigGroup) (time.Time, string) {
	project := cg.ProjectString()
	clString := fmt.Sprintf("CL %d %s", cl.ID, cl.ExternalID)
	switch kind, reason := cl.AccessKindWithReason(ctx, project); kind {
	case changelist.AccessDenied:
		logging.Warningf(ctx, "No longer have access to %s: %s", clString, reason)
		return time.Time{}, fmt.Sprintf("no longer have access to %s: %s", cl.ExternalID.MustURL(), reason)
	case changelist.AccessDeniedProbably:
		logging.Warningf(ctx, "Probably no longer have access to %s (%s), not canceling yet", clString, reason)
		// Keep the run Running for now. The access should become either
		// AccessGranted or AccessDenied, eventually.
		return cl.Access.GetByProject()[project].GetNoAccessTime().AsTime(), ""
	case changelist.AccessUnknown:
		logging.Errorf(ctx, "Unknown access to %s (%s), not canceling yet", clString, reason)
		// Keep the run Running for now, it should become clear eventually.
		return time.Time{}, ""
	case changelist.AccessGranted:
		// The expected and most likely case.
	default:
		panic(fmt.Errorf("unknown AccessKind %d in %s", kind, clString))
	}

	if o, c := rcl.Detail.GetPatchset(), cl.Snapshot.GetPatchset(); o != c {
		logging.Infof(ctx, "%s has new patchset %d => %d", clString, o, c)
		return time.Time{}, fmt.Sprintf("the patchset of %s has changed from %d to %d", cl.ExternalID.MustURL(), o, c)
	}
	if o, c := rcl.Detail.GetGerrit().GetInfo().GetRef(), cl.Snapshot.GetGerrit().GetInfo().GetRef(); o != c {
		logging.Warningf(ctx, "%s has new ref %q => %q", clString, o, c)
		return time.Time{}, fmt.Sprintf("the ref of %s has moved from %s to %s", cl.ExternalID.MustURL(), o, c)
	}
	if rcl.Trigger != nil {
		o, c := rcl.Trigger, trigger.Find(&trigger.FindInput{
			ChangeInfo:                   cl.Snapshot.GetGerrit().GetInfo(),
			ConfigGroup:                  cg.Content,
			TriggerNewPatchsetRunAfterPS: cl.TriggerNewPatchsetRunAfterPS,
		})
		if whatChanged := run.HasTriggerChanged(o, c, cl.ExternalID.MustURL()); whatChanged != "" {
			logging.Infof(ctx, "%s has new trigger\nOLD: %s\nNEW: %s", clString, o, c)
			return time.Time{}, whatChanged
		}
	}
	return time.Time{}, ""
}
