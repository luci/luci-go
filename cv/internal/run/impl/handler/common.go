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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// endRun sets Run to the provided status and populates `EndTime`.
//
// Returns the side effect when Run is ended.
//
// Panics if the provided status is not ended status.
func endRun(ctx context.Context, rs *state.RunState, st run.Status, pm PM) eventbox.SideEffectFn {
	if !run.IsEnded(st) {
		panic(fmt.Errorf("can't end run with non-final status %s", st))
	}
	rs.Run.Status = st
	rs.Run.EndTime = clock.Now(ctx).UTC()
	rid := rs.Run.ID
	return eventbox.Chain(
		func(ctx context.Context) error {
			return removeRunFromCLs(ctx, rid, rs.Run.CLs)
		},
		func(ctx context.Context) error {
			return pm.NotifyRunFinished(ctx, rid)
		},
		// TODO(qyearsley): Submit a task to do BQ export.
	)
}

// removeRunFromCLs removes the Run from the IncompleteRuns list of all
// CLs involved in this Run.
func removeRunFromCLs(ctx context.Context, runID common.RunID, clids common.CLIDs) error {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("must be called in a transaction")
	}
	cls, err := changelist.LoadCLs(ctx, clids)
	if err != nil {
		return err
	}
	for _, cl := range cls {
		cl.Mutate(ctx, func(cl *changelist.CL) bool {
			return cl.IncompleteRuns.DelSorted(runID)
		})
	}
	if err := datastore.Put(ctx, cls); err != nil {
		return errors.Annotate(err, "failed to put CLs").Tag(transient.Tag).Err()
	}
	return nil
}
