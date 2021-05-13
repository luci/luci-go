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

// Package state defines the model for a Run state.
package state

import (
	"context"
	"fmt"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/run"
)

// RunState represents the current state of a Run.
//
// It consists of the Run entity and its child entities (could be partial
// depending on the event received).
type RunState struct {
	Run run.Run
	// TODO(yiwzhang): add RunOwner, []RunCL, []RunTryjob.

	cachedConfigGroup *config.ConfigGroup
}

// ShallowCopy returns a shallow copy of run state
func (rs *RunState) ShallowCopy() *RunState {
	if rs == nil {
		return nil
	}
	ret := &RunState{
		Run: rs.Run,

		cachedConfigGroup: rs.cachedConfigGroup,
	}
	return ret
}

// EndRun sets Run to the provided status and populates `EndTime`.
//
// Panics if the provided status is not ended status.
func (rs *RunState) EndRun(ctx context.Context, status run.Status) {
	if !run.IsEnded(status) {
		panic(fmt.Errorf("can't end run with non-final status %s", status))
	}
	rs.Run.Status = status
	rs.Run.EndTime = clock.Now(ctx).UTC()
}

// RemoveRunFromCLs removes the Run from the IncompleteRuns list of all
// CL entities associated with this Run.
func (rs *RunState) RemoveRunFromCLs(ctx context.Context) error {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("must be called in a transaction")
	}
	cls, err := changelist.LoadCLs(ctx, rs.Run.CLs)
	if err != nil {
		return err
	}
	for _, cl := range cls {
		cl.Mutate(ctx, func(cl *changelist.CL) bool {
			return cl.IncompleteRuns.DelSorted(rs.Run.ID)
		})
	}
	if err := datastore.Put(ctx, cls); err != nil {
		return errors.Annotate(err, "failed to put CLs").Tag(transient.Tag).Err()
	}
	return nil
}

// LoadConfigGroup loads the ConfigGroup used by this Run.
//
// Result is cached inside the state.
func (rs *RunState) LoadConfigGroup(ctx context.Context) (*config.ConfigGroup, error) {
	cgID := rs.Run.ConfigGroupID
	if rs.cachedConfigGroup != nil && cgID == rs.cachedConfigGroup.ID {
		return rs.cachedConfigGroup, nil
	}
	var err error
	rs.cachedConfigGroup, err = config.GetConfigGroup(ctx, rs.Run.ID.LUCIProject(), cgID)
	if err != nil {
		return nil, err
	}
	return rs.cachedConfigGroup, nil
}
