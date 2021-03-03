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

	cfgpb "go.chromium.org/luci/cv/api/config/v2"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

type queue struct {
	_kind string `gae:"$kind,SubmitQueue"`
	// ID is the ID of this submit queue. e.g. "SubmitQueue/chromium".
	ID string `gae:"$id"`
	// InProgress is the run that is currently in submission.
	InProgress *item
	// Deadline is the time when the in-progress submission should be stopped.
	Deadline time.Time `gae:",noindex"`
	// Waitlist contains all runs that are currently waiting for their turns.
	Waitlist []*item
	// Opts controls the rate of submission. nil means no rate limiting.
	Opts *cfgpb.SubmitOptions
	// History records the timestamps of all submissions happend within
	// `Opts.BurstDelay` if supplied.
	History []time.Time
}

type item struct {
	// RunID is the ID of the Run to be submitted.
	RunID common.RunID `gae:",noindex"`
	// RemainingCLs records the progress of this Run submission.
	RemainingCLs common.CLIDs `gae:",noindex"`
}

// ReserveInput contains input arguments of `Reserve` function.
type ReserveInput struct {
	// RunID is the ID of the Run for submission.
	RunID common.RunID
	// CLs are all CLs that need to be submitted.
	CLs []*run.RunCL
	// Deadline is the time when this submission should stop.
	//
	// Note that, the result MAY contain a shorter deadline. See the scenario
	// in the `Deadline` field of `ReserveResult`.
	Deadline time.Time
}

// ReserveResult is the result of `Reserve` function.
type ReserveResult struct {
	// Waitlisted is true if the requested Run is currently in waitlist or
	// there is another Run currently in submission and the requested Run
	// has been successfully put into the waitlist.
	//
	// Run in waitlist will be notified once it comes to its turn.
	//
	// If Waitlisted is false, it means the requested submission is current and
	// the callsite can proceed with the actual submission.
	Waitlisted bool
	// RemainingCLs are IDs of not-yet-submitted CLs in this submission.
	//
	// The submission progress is reported by `SaveProgress()`. Note that this
	// information may be stale as it is possible that the submission succeeds
	// and progress saving fails.
	RemainingCLs common.CLIDs
	// Deadline is the time when the the requested submission should stop.
	//
	// In most of the times, it is the same as the `Deadline` in the request.
	// However, when an in-progress submission fails in the middle and then the
	// retry resumes this submission by reserving the submit queue again, if
	// this submission has not met the existing deadline, the existing deadeline
	// will be returned instead of the `Deadline` in the input request.
	Deadline time.Time
}

// Reserve reserves the submit queue to submit CLs for the given Run.
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
