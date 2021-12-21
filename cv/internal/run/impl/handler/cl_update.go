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

	"golang.org/x/sync/errgroup"

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
	case isTriggersCancellationOngoing(rs):
		// It's likely CL is updated due to trigger cancellation, defer the process
		// of CLsUpdated event till triggers cancellation is done.
		return &Result{State: rs, PreserveEvents: true}, nil
	case run.IsEnded(status):
		logging.Debugf(ctx, "skipping OnCLUpdated because Run is %s", status)
		return &Result{State: rs}, nil
	}
	clids.Dedupe()

	var cls []*changelist.CL
	var runCLs []*run.RunCL
	eg, ectx := errgroup.WithContext(ctx)
	eg.Go(func() (err error) {
		cls, err = changelist.LoadCLs(ectx, clids)
		return err
	})
	eg.Go(func() (err error) {
		runCLs, err = run.LoadRunCLs(ectx, rs.ID, clids)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	cg, err := prjcfg.GetConfigGroup(ctx, rs.ID.LUCIProject(), rs.ConfigGroupID)
	if err != nil {
		return nil, err
	}

	preserveEvents := false
	var earliestReconsiderAt time.Time
	for i := range clids {
		switch reconsiderAt, cancellationReason := shouldCancel(ctx, cls[i], runCLs[i], cg); {
		case !reconsiderAt.IsZero():
			preserveEvents = true
			if earliestReconsiderAt.IsZero() || earliestReconsiderAt.After(reconsiderAt) {
				earliestReconsiderAt = reconsiderAt
			}
		case cancellationReason != "":
			return impl.Cancel(ctx, rs, []string{cancellationReason})
		}
	}
	if preserveEvents {
		logging.Debugf(ctx, "Will reconsider OnCLUpdated event(s) after %s", earliestReconsiderAt.Sub(clock.Now(ctx)))
		if err := impl.RM.Invoke(ctx, rs.ID, earliestReconsiderAt); err != nil {
			return nil, err
		}
	}
	return &Result{State: rs, PreserveEvents: preserveEvents}, nil
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
	o, c := rcl.Trigger, trigger.Find(cl.Snapshot.GetGerrit().GetInfo(), cg.Content)
	if whatChanged := hasTriggerChanged(o, c, cl.ExternalID.MustURL()); whatChanged != "" {
		logging.Infof(ctx, "%s has new trigger\nOLD: %s\nNEW: %s", clString, o, c)
		return time.Time{}, whatChanged
	}
	return time.Time{}, ""
}

func hasTriggerChanged(old, cur *run.Trigger, clURL string) string {
	switch {
	case cur == nil:
		return fmt.Sprintf("the trigger on %s has been removed", clURL)
	case old.GetMode() != cur.GetMode():
		return fmt.Sprintf("the triggering vote on %s has requested a different run mode: %s", clURL, cur.GetMode())
	case !old.GetTime().AsTime().Equal(cur.GetTime().AsTime()):
		return fmt.Sprintf(
			"the timestamp of the triggering vote on %s has changed from %s to %s",
			clURL, old.GetTime().AsTime(), cur.GetTime().AsTime())
	default:
		// Theoretically, if the triggering user changes, it should also be counted
		// as changed trigger. However, checking whether triggering time has changed
		// is ~enough because it is extremely rare that votes from different users
		// have the exact same timestamp. And even if it really happens, CV won't
		// be able to handle this case because CV will generate the same Run ID as
		// user is not taken into account during ID generation.
		return ""
	}
}

func isTriggersCancellationOngoing(rs *state.RunState) bool {
	for _, op := range rs.OngoingLongOps.GetOps() {
		if op.GetCancelTriggers() != nil {
			return true
		}
	}
	return false
}
