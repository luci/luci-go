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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tree"
)

// RunState represents the current state of a Run.
type RunState struct {
	Run run.Run

	// Helper fields used during state mutations.

	// SubmissionScheduled is true if a submission will be attempted after state
	// transition completes.
	SubmissionScheduled bool
	// cg is the cached config group used by this Run.
	cg *config.ConfigGroup
}

// ShallowCopy returns a shallow copy of run state
func (rs *RunState) ShallowCopy() *RunState {
	if rs == nil {
		return nil
	}
	ret := &RunState{
		Run:                 rs.Run,
		SubmissionScheduled: rs.SubmissionScheduled,

		cg: rs.cg,
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
	if rs.cg != nil && cgID == rs.cg.ID {
		return rs.cg, nil
	}
	var err error
	rs.cg, err = config.GetConfigGroup(ctx, rs.Run.ID.LUCIProject(), cgID)
	if err != nil {
		return nil, err
	}
	return rs.cg, nil
}

// CheckTree returns whether Tree is open for this Run.
//
// Returns true if no Tree is defined for this Run. Updates the latest
// result to `run.Submission`.
func (rs *RunState) CheckTree(ctx context.Context, tc tree.Client) (bool, error) {
	cg, err := rs.LoadConfigGroup(ctx)
	if err != nil {
		return false, err
	}
	treeOpen := true
	if treeURL := cg.Content.GetVerifiers().GetTreeStatus().GetUrl(); treeURL != "" {
		status, err := tc.FetchLatest(ctx, treeURL)
		if err != nil {
			return false, err
		}
		treeOpen = status.State == tree.Open || status.State == tree.Throttled
	}
	rs.Run.Submission.TreeOpen = treeOpen
	rs.Run.Submission.LastTreeCheckTime = timestamppb.New(clock.Now(ctx).UTC())
	return treeOpen, nil
}
