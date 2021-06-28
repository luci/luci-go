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

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// OnCLUpdated implements Handler interface.
func (impl *Impl) OnCLUpdated(ctx context.Context, rs *state.RunState, clids common.CLIDs) (*Result, error) {
	switch status := rs.Run.Status; {
	case status == run.Status_STATUS_UNSPECIFIED:
		err := errors.Reason("CRITICAL: Received CLUpdated events but Run is in unspecified status").Err()
		common.LogError(ctx, err)
		panic(err)
	case status == run.Status_SUBMITTING:
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
		runCLs, err = run.LoadRunCLs(ectx, rs.Run.ID, clids)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	cg, err := rs.LoadConfigGroup(ctx)
	if err != nil {
		return nil, err
	}

	preserveEvents := false
	for i := range clids {
		switch shouldCancel(ctx, cls[i], runCLs[i], cg) {
		case yes:
			return impl.Cancel(ctx, rs)
		case preserveEvent:
			preserveEvents = true
		}
	}
	return &Result{State: rs, PreserveEvents: preserveEvents}, nil
}

type shouldCancelResult int

const (
	yes shouldCancelResult = iota
	no
	preserveEvent
)

func shouldCancel(ctx context.Context, cl *changelist.CL, rcl *run.RunCL, cg *prjcfg.ConfigGroup) shouldCancelResult {
	clString := fmt.Sprintf("CL %d %s", cl.ID, cl.ExternalID)
	switch kind, reason := cl.AccessKindWithReason(ctx, cg.ProjectString()); kind {
	case changelist.AccessDenied:
		logging.Warningf(ctx, "No longer have access to %s: %s", clString, reason)
		return yes
	case changelist.AccessDeniedProbably:
		logging.Warningf(ctx, "Probably no longer have access to %s (%s), not canceling yet", clString, reason)
		// Keep the run Running for now. The access should become either
		// AccessGranted or AccessDenied, eventually.
		return preserveEvent
	case changelist.AccessUnknown:
		logging.Errorf(ctx, "Unknown access to %s (%s), not canceling yet", clString, reason)
		// Keep the run Running for now, it should become clear eventually.
		return no
	case changelist.AccessGranted:
		// The expected and most likely case.
	default:
		panic(fmt.Errorf("unknown AccessKind %d in %s", kind, clString))
	}

	if o, c := rcl.Detail.GetPatchset(), cl.Snapshot.GetPatchset(); o != c {
		logging.Infof(ctx, "%s has new patchset %d => %d", clString, o, c)
		return yes
	}
	if o, c := rcl.Detail.GetGerrit().GetInfo().GetRef(), cl.Snapshot.GetGerrit().GetInfo().GetRef(); o != c {
		logging.Warningf(ctx, "%s has new ref %q => %q", clString, o, c)
		return yes
	}
	if o, c := rcl.Trigger, trigger.Find(cl.Snapshot.GetGerrit().GetInfo(), cg.Content); hasTriggerChanged(o, c) {
		logging.Infof(ctx, "%s has new trigger\nOLD: %s\nNEW: %s", clString, o, c)
		return yes
	}
	return no
}

func hasTriggerChanged(old, cur *run.Trigger) bool {
	switch {
	case cur == nil:
		return true // trigger removal
	case old.GetMode() != cur.GetMode():
		return true // mode has changed
	case !old.GetTime().AsTime().Equal(cur.GetTime().AsTime()):
		return true // triggering time has changed
	default:
		// Theoretically, if the triggering user changes, it should also be counted
		// as changed trigger. However, checking whether triggering time has changed
		// is ~enough because it is extremely rare that votes from different users
		// have the exact same timestamp. And even if it really happens, CV won't
		// be able to handle this case because CV will generate the same Run ID as
		// user is not taken into account during ID generation.
		return false
	}
}
