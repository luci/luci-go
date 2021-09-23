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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// Cancel implements Handler interface.
func (impl *Impl) Cancel(ctx context.Context, rs *state.RunState) (*Result, error) {
	// TODO(crbug/1215612): record the cause of cancellation inside Run.
	switch status := rs.Status; {
	case status == run.Status_STATUS_UNSPECIFIED:
		err := errors.Reason("CRITICAL: can't cancel a Run with unspecified status").Err()
		common.LogError(ctx, err)
		panic(err)
	case status == run.Status_SUBMITTING:
		// Can't cancel while submitting.
		return &Result{State: rs, PreserveEvents: true}, nil
	case run.IsEnded(status):
		logging.Debugf(ctx, "skipping cancellation because Run is %s", status)
		return &Result{State: rs}, nil
	}

	rs = rs.ShallowCopy()
	se := impl.endRun(ctx, rs, run.Status_CANCELLED)
	if rs.StartTime.IsZero() {
		// This run has never started but already gets a cancelled event.
		rs.StartTime = rs.EndTime
	}

	return &Result{
		State:        rs,
		SideEffectFn: se,
	}, nil
}
