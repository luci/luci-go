// Copyright 2020 The LUCI Authors.
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

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// Cancel implements Handler interface.
func (impl *Impl) Cancel(ctx context.Context, rs *state.RunState, reasons []string) (*Result, error) {
	switch status := rs.Status; {
	case status == run.Status_STATUS_UNSPECIFIED:
		err := errors.New("CRITICAL: can't cancel a Run with unspecified status")
		common.LogError(ctx, err)
		panic(err)
	case status == run.Status_SUBMITTING:
		// Can't cancel while submitting.
		return &Result{State: rs, PreserveEvents: true}, nil
	case run.IsEnded(status):
		logging.Debugf(ctx, "skipping cancellation because Run is %s", status)
		return &Result{State: rs}, nil
	}
	cg, err := prjcfg.GetConfigGroup(ctx, rs.ID.LUCIProject(), rs.ConfigGroupID)
	if err != nil {
		return nil, errors.Fmt("prjcfg.GetConfigGroup: %w", err)
	}
	rs = rs.ShallowCopy()
	// make sure reasons are unique and doesn't contain empty reasons.
	uniqueReasons := stringset.NewFromSlice(reasons...)
	uniqueReasons.Del("")
	rs.CancellationReasons = uniqueReasons.ToSortedSlice()
	childRuns, err := run.LoadChildRuns(ctx, rs.ID)
	if err != nil {
		return nil, errors.Fmt("failed to load child runs: %w", err)
	}
	se := impl.endRun(ctx, rs, run.Status_CANCELLED, cg, childRuns)

	return &Result{
		State:        rs,
		SideEffectFn: se,
	}, nil
}
