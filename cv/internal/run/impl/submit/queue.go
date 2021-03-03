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

package submit

import (
	"context"
	"time"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

type queue struct {
	_kind string `gae:"$kind,SubmitQueue"`
	// ID is ID for this submission queue. e.g. "SubmitQueue/chromium".
	ID string `gae:"$id"`
	// InProgress is the run that is currently being submitted.
	InProgress *submission
	// Deadline is the time when the in-progress submission should be stopped.
	Deadline time.Time
	// Waitlist is the all runs that are currently waiting for their turns.
	Waitlist []*submission
}

type submission struct {
	// RunID is ID of the Run.
	RunID common.RunID
	// RemainingCLs records the progress of this Run submission.
	RemainingCLs common.CLIDs
}

// ReserveInput contains all input arguments of `Reserve` function.
type ReserveInput struct {
	// RunID is ID of the Run for submission.
	RunID common.RunID
	// CLs are all CLs that need to be submitted.
	CLs []*run.RunCL
	// Deadline is the time when the in-progress submission should be stopped.
	//
	// Note that, the result MAY contain a shorter deadline. See the scenario
	// in the `Deadline` field of `ReserveResult`.
	Deadline time.Time
}

// ReserveResult is the result of `Reserve` function.
type ReserveResult struct {
	// Waitlisted is true if the requested Run is currently in waitlist or
	// there is a nother Run currently being submitted and the requested Run
	// has been successfully put into waitlist (this run will be notified once
	// it comes to its turn).
	//
	// If Waitlisted is false, it means the requested submission is current and
	// the callsite can proceed with the actual submission.
	Waitlisted bool
	// RemainingCLs are IDs of not-yet-submitted CLs for this submission.
	//
	// For brand-new submission, it is the IDs of `CLs` in the request ordered
	// in submission order.
	// For submission that is attempted before but get waitlisted again (e.g.
	// error occurs in the middle of the submission), it is `CLs` reported in
	// last `SaveProgress()` at the submission site. Note that this information
	// could be stale as it is possible that the submission succeeds and
	// progress saving fails.
	RemainingCLs common.CLIDs
	// Deadline is the time when the in-progress submission should be stopped.
	//
	// In most of the times, it is the same as the `Deadline` in the request.
	// However, when an in-progress submission fails in the middle and the retry
	// resumes this submission by reserving the submitting queue again, if
	// this submission has not met the deadline, the return Deadline will be
	// be Deadline in the first successful reservation instead of the requested
	// Deadline.
	Deadline time.Duration
}

// Reserve reserves the exclusive privilege of submitting queue for given
// MaxDuration.
func Reserve(ctx context.Context, input ReserveInput) (ReserveResult, error) {
	panic("not implemented")
}

// SaveProgress records the remaining not-yet-submitted CLs for this Run.
func SaveProgress(ctx context.Context, runID common.RunID, remainingCLs common.CLIDs) error {
	panic("not implemented")
}

// MarkComplete mark the submission completes for the given Run.
//
// It notifies the next Run in queue if any.
func MarkComplete(ctx context.Context, runID common.RunID) error {
	panic("not implemented")
}
