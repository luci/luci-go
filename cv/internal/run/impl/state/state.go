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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/cv/internal/common/tree"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run"
)

// RunState represents the current state of a Run.
type RunState struct {
	Run        run.Run
	LogEntries []*run.LogEntry

	// Helper fields used during state mutations.

	// SubmissionScheduled is true if a submission will be attempted after state
	// transition completes.
	SubmissionScheduled bool
}

// ShallowCopy returns a shallow copy of run state
func (rs *RunState) ShallowCopy() *RunState {
	if rs == nil {
		return nil
	}
	ret := &RunState{
		Run:                 rs.Run,
		LogEntries:          append(make([]*run.LogEntry, 0, len(rs.LogEntries)), rs.LogEntries...),
		SubmissionScheduled: rs.SubmissionScheduled,
	}
	return ret
}

// CheckTree returns whether Tree is open for this Run.
//
// Returns true if no Tree or Options.SkipTreeChecks is configured for this Run.
// Updates the latest result to `run.Submission`.
func (rs *RunState) CheckTree(ctx context.Context, tc tree.Client) (bool, error) {
	treeOpen := true
	if !rs.Run.Options.GetSkipTreeChecks() {
		cg, err := prjcfg.GetConfigGroup(ctx, rs.Run.ID.LUCIProject(), rs.Run.ConfigGroupID)
		if err != nil {
			return false, err
		}
		if treeURL := cg.Content.GetVerifiers().GetTreeStatus().GetUrl(); treeURL != "" {
			status, err := tc.FetchLatest(ctx, treeURL)
			if err != nil {
				return false, err
			}
			treeOpen = status.State == tree.Open || status.State == tree.Throttled
			rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
				Time: timestamppb.New(clock.Now(ctx)),
				Kind: &run.LogEntry_TreeChecked_{
					TreeChecked: &run.LogEntry_TreeChecked{
						Open: treeOpen,
					},
				},
			})
		}
	}
	rs.Run.Submission.TreeOpen = treeOpen
	rs.Run.Submission.LastTreeCheckTime = timestamppb.New(clock.Now(ctx).UTC())
	return treeOpen, nil
}
