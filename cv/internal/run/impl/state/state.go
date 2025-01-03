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
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	tspb "go.chromium.org/luci/tree_status/proto/v1"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/tree"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run"
)

// RunState represents the current state of a Run.
type RunState struct {
	run.Run
	LogEntries []*run.LogEntry

	// Helper fields used during state mutations.

	// NewLongOpIDs which should be scheduled transactionally with the state
	// transition.
	NewLongOpIDs []string
	// SubmissionScheduled is true if a submission will be attempted after state
	// transition completes.
	SubmissionScheduled bool
}

// ShallowCopy returns a shallow copy of this RunState.
func (rs *RunState) ShallowCopy() *RunState {
	if rs == nil {
		return nil
	}
	ret := *rs
	// Intentionally use nil check instead of checking len(slice), because
	// otherwise, the copy will always have nil slice even if the `rs` has
	// zero-length slice which will fail the equality check in test.
	if rs.CLs != nil {
		ret.CLs = append(common.CLIDs(nil), rs.CLs...)
	}
	if rs.CancellationReasons != nil {
		rs.CancellationReasons = append([]string(nil), rs.CancellationReasons...)
	}
	if rs.LogEntries != nil {
		ret.LogEntries = append([]*run.LogEntry(nil), rs.LogEntries...)
	}
	if rs.NewLongOpIDs != nil {
		ret.NewLongOpIDs = append([]string(nil), rs.NewLongOpIDs...)
	}
	if rs.DepRuns != nil {
		ret.DepRuns = append(common.RunIDs(nil), rs.DepRuns...)
	}
	return &ret
}

// DeepCopy returns a deep copy of this RunState.
//
// This is an expensive operation. It should only be called in tests.
func (rs *RunState) DeepCopy() *RunState {
	if rs == nil {
		return nil
	}
	// Explicitly copy by hand instead of creating a shallow copy first like
	// `ShallowCopy` to ensure all newly added fields will be *deep* copied.
	// TODO(yiwzhang): Make a generic recursive deep copy (similar to
	// cvtesting.SafeShouldResemble) which recognizes proto and uses `Clone`
	// to DeepCopy instead.
	ret := &RunState{
		Run: run.Run{
			ID:                                rs.ID,
			CreationOperationID:               rs.CreationOperationID,
			Mode:                              rs.Mode,
			Status:                            rs.Status,
			EVersion:                          rs.EVersion,
			CreateTime:                        rs.CreateTime,
			StartTime:                         rs.StartTime,
			UpdateTime:                        rs.UpdateTime,
			EndTime:                           rs.EndTime,
			Owner:                             rs.Owner,
			CreatedBy:                         rs.CreatedBy,
			BilledTo:                          rs.BilledTo,
			ConfigGroupID:                     rs.ConfigGroupID,
			RootCL:                            rs.RootCL,
			Options:                           proto.Clone(rs.Options).(*run.Options),
			Submission:                        proto.Clone(rs.Submission).(*run.Submission),
			Tryjobs:                           proto.Clone(rs.Tryjobs).(*run.Tryjobs),
			OngoingLongOps:                    proto.Clone(rs.OngoingLongOps).(*run.OngoingLongOps),
			LatestCLsRefresh:                  rs.LatestCLsRefresh,
			LatestTryjobsRefresh:              rs.LatestTryjobsRefresh,
			QuotaExhaustionMsgLongOpRequested: rs.QuotaExhaustionMsgLongOpRequested,
		},
		SubmissionScheduled: rs.SubmissionScheduled,
	}
	// Intentionally use nil check instead of checking len(slice), because
	// otherwise, the copy will always have nil slice even if the `rs` has
	// zero-length slice which will fail the equality check in test.
	if rs.CLs != nil {
		ret.CLs = append(common.CLIDs(nil), rs.CLs...)
	}
	if rs.CancellationReasons != nil {
		rs.CancellationReasons = append([]string(nil), rs.CancellationReasons...)
	}
	if rs.LogEntries != nil {
		ret.LogEntries = make([]*run.LogEntry, len(rs.LogEntries))
		for i, entry := range rs.LogEntries {
			ret.LogEntries[i] = proto.Clone(entry).(*run.LogEntry)
		}
	}
	if rs.NewLongOpIDs != nil {
		ret.NewLongOpIDs = append([]string(nil), rs.NewLongOpIDs...)
	}
	if rs.DepRuns != nil {
		ret.DepRuns = append(common.RunIDs(nil), rs.DepRuns...)
	}
	return ret
}

// CloneSubmission clones the `Submission` property.
//
// Initializes the property if this is nil.
func (rs *RunState) CloneSubmission() {
	if rs.Submission == nil {
		rs.Submission = &run.Submission{}
	} else {
		rs.Submission = proto.Clone(rs.Submission).(*run.Submission)
	}
}

// CheckTree returns whether Tree is open for this Run.
//
// Returns true if no Tree or Options.SkipTreeChecks is configured for this Run.
// Updates the latest result to `rs.Submission`.
// In case fetching the status results in error for the first time, it records
// the time in the appropriate field in Submission.
// Records a new LogEntry.
func (rs *RunState) CheckTree(ctx context.Context, treeFactory tree.ClientFactory) (treeOpen bool, err error) {
	defer func() {
		rs.Submission.TreeOpen = treeOpen
		rs.Submission.LastTreeCheckTime = timestamppb.New(clock.Now(ctx).UTC())
	}()
	if rs.Options.GetSkipTreeChecks() {
		return true, nil
	}

	luciProject := rs.ID.LUCIProject()
	cg, err := prjcfg.GetConfigGroup(ctx, luciProject, rs.ConfigGroupID)
	if err != nil {
		return false, err
	}

	treeName := tree.TreeName(cg.Content.GetVerifiers().GetTreeStatus())
	if treeName == "" {
		return true, nil // no tree defined
	}
	tc, err := treeFactory.MakeClient(ctx, luciProject)
	if err != nil {
		return false, fmt.Errorf("failed to create tree client for project %q: %w", luciProject, err)
	}
	resp, err := tc.GetStatus(ctx, &tspb.GetStatusRequest{
		Name: fmt.Sprintf("trees/%s/status/latest", treeName),
	})
	if err != nil {
		if rs.Submission.TreeErrorSince == nil {
			rs.Submission.TreeErrorSince = timestamppb.New(clock.Now(ctx))
		}
		logging.Errorf(ctx, "failed to fetch tree status for %s: %s", treeName, err)
		return false, err
	}

	rs.Submission.TreeErrorSince = nil
	switch resp.GetGeneralState() {
	case tspb.GeneralState_OPEN, tspb.GeneralState_THROTTLED:
		treeOpen = true
	}
	rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
		Time: timestamppb.New(clock.Now(ctx)),
		Kind: &run.LogEntry_TreeChecked_{
			TreeChecked: &run.LogEntry_TreeChecked{
				Open: treeOpen,
			},
		},
	})
	return treeOpen, nil
}

// EnqueueLongOp adds a new long op to the Run state and returns its ID.
//
// The actual long operation will be scheduled transactioncally with the Run
// mutation.
func (rs *RunState) EnqueueLongOp(op *run.OngoingLongOps_Op) string {
	if err := validateLongOp(op); err != nil {
		panic(errors.Annotate(err, "validateLongOp").Err())
	}
	// Find an ID which wasn't used yet.
	// Use future EVersion as a prefix to ensure resulting ID is unique over Run's
	// lifetime.
	id := ""
	prefix := rs.EVersion + 1
	suffix := len(rs.NewLongOpIDs) + 1
	for {
		id = fmt.Sprintf("%d-%d", prefix, suffix)
		if _, dup := rs.OngoingLongOps.GetOps()[id]; !dup {
			break
		}
		suffix++
	}

	if rs.OngoingLongOps == nil {
		rs.OngoingLongOps = &run.OngoingLongOps{}
	} else {
		rs.OngoingLongOps = proto.Clone(rs.OngoingLongOps).(*run.OngoingLongOps)
	}
	if rs.OngoingLongOps.Ops == nil {
		rs.OngoingLongOps.Ops = make(map[string]*run.OngoingLongOps_Op, 1)
	}
	rs.OngoingLongOps.Ops[id] = op
	rs.NewLongOpIDs = append(rs.NewLongOpIDs, id)
	return id
}

func validateLongOp(op *run.OngoingLongOps_Op) error {
	switch {
	case op.GetDeadline() == nil:
		return errors.New("deadline is required")
	case op.GetWork() == nil:
		return errors.New("work is required")
	case op.GetResetTriggers() != nil:
		if st := op.GetResetTriggers().GetRunStatusIfSucceeded(); !run.IsEnded(st) {
			return errors.Reason("expect terminal run status; got %s", st).Err()
		}
	}
	return nil
}

// RequestLongOpCancellation records soft request to cancel a long running op.
//
// This request is asynchronous but it's stored in the Run state.
func (rs *RunState) RequestLongOpCancellation(opID string) {
	if _, exists := rs.OngoingLongOps.GetOps()[opID]; !exists {
		panic(fmt.Errorf("long Operation %q doesn't exist", opID))
	}
	rs.OngoingLongOps = proto.Clone(rs.OngoingLongOps).(*run.OngoingLongOps)
	rs.OngoingLongOps.GetOps()[opID].CancelRequested = true
}

// RemoveCompletedLongOp removes long op from the ongoing ones.
func (rs *RunState) RemoveCompletedLongOp(opID string) {
	if _, exists := rs.OngoingLongOps.GetOps()[opID]; !exists {
		panic(fmt.Errorf("long Operation %q doesn't exist", opID))
	}
	if len(rs.OngoingLongOps.GetOps()) == 1 {
		rs.OngoingLongOps = nil
		return
	}
	// At least 1 other long op will remain.
	rs.OngoingLongOps = proto.Clone(rs.OngoingLongOps).(*run.OngoingLongOps)
	delete(rs.OngoingLongOps.Ops, opID)
}

// LogInfo adds a generic LogEntry visible in debug UI as is.
//
// Don't use for adding a complicated info formatted into the message string,
// use a specialized LogEntry type instead.
func (rs *RunState) LogInfo(ctx context.Context, label, message string) {
	rs.LogInfoAt(clock.Now(ctx), label, message)
}

// LogInfof is like LogInfo but formats the message according to format
// specifier.
func (rs *RunState) LogInfof(ctx context.Context, label, format string, args ...any) {
	rs.LogInfo(ctx, label, fmt.Sprintf(format, args...))
}

// LogInfoAt is LogInfo with a custom timestamp.
func (rs *RunState) LogInfoAt(at time.Time, label, message string) {
	rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
		Time: timestamppb.New(at),
		Kind: &run.LogEntry_Info_{
			Info: &run.LogEntry_Info{
				Label:   label,
				Message: message,
			},
		},
	})
}

// LogInfofAt is like LogInfoAt but formats the message according to format
// specifier.
func (rs *RunState) LogInfofAt(at time.Time, label, format string, args ...any) {
	rs.LogInfoAt(at, label, fmt.Sprintf(format, args...))
}
