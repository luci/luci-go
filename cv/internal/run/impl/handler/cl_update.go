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

	for i := range clids {
		switch cl, runCL := cls[i], runCLs[i]; {
		case cl.Snapshot.GetPatchset() > runCL.Detail.GetPatchset():
			// New PS discovered.
			return impl.Cancel(ctx, rs)
		case cl.Snapshot.GetGerrit().GetInfo().GetRef() != runCL.Detail.GetGerrit().GetInfo().GetRef():
			// Ref has changed (e.g. master -> main migration).
			return impl.Cancel(ctx, rs)
		case hasTriggerChanged(runCL, trigger.Find(cl.Snapshot.GetGerrit().GetInfo(), cg.Content)):
			return impl.Cancel(ctx, rs)
		}
	}
	return &Result{State: rs}, nil
}

func hasTriggerChanged(runCL *run.RunCL, curTrigger *run.Trigger) bool {
	if runCL.Trigger == nil {
		panic(fmt.Errorf("runCL must have a trigger"))
	}
	oldTrigger := runCL.Trigger
	switch {
	case curTrigger == nil:
		return true // trigger removal
	case oldTrigger.GetMode() != curTrigger.GetMode():
		return true // mode has changed
	case !oldTrigger.GetTime().AsTime().Equal(curTrigger.GetTime().AsTime()):
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
