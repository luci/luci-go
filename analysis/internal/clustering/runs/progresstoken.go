// Copyright 2022 The LUCI Authors.
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

package runs

import (
	"context"
	"errors"
	"time"

	"go.chromium.org/luci/server/span"
)

// ProgressToken is used to report the progress reported for a shard.
// To avoid double-counting progress, the progress state must be
// saved and restored between tasks that report for the shard.
type ProgressToken struct {
	project              string
	attemptTimestamp     time.Time
	reportedOnce         bool
	lastReportedProgress int
	invalid              bool
}

// ProgressState encapsulates the reporting state of a progress token.
// The reporting state avoids a shard double-reporting progress.
type ProgressState struct {
	// ReportedOnce is whether any progress has been reported for
	// the shard.
	ReportedOnce bool
	// LastReportedProgress is the last reported progress for the
	// shard.
	LastReportedProgress int
}

// NewProgressToken initialises a new progress token with the given LUCI
// project ID, attempt key and state.
func NewProgressToken(project string, attemptTimestamp time.Time, state *ProgressState) *ProgressToken {
	return &ProgressToken{
		project:              project,
		attemptTimestamp:     attemptTimestamp,
		reportedOnce:         state.ReportedOnce,
		lastReportedProgress: state.LastReportedProgress,
	}
}

// ExportState exports the state of the progress token. After the export,
// the token is invalidated and cannot be used to report progress anymore.
func (p *ProgressToken) ExportState() (*ProgressState, error) {
	if p.invalid {
		return nil, errors.New("state cannot be exported; token is invalid")
	}
	p.invalid = true
	return &ProgressState{
		ReportedOnce:         p.reportedOnce,
		LastReportedProgress: p.lastReportedProgress,
	}, nil
}

// ReportProgress reports the progress for the current shard. Progress ranges
// from 0 to 1000, with 1000 indicating the all work assigned to the shard
// is complete.
func (p *ProgressToken) ReportProgress(ctx context.Context, value int) error {
	if p.invalid {
		return errors.New("no more progress can be reported; token is invalid")
	}
	// Bound progress values to the allowed range.
	if value < 0 || value > 1000 {
		return errors.New("progress value must be between 0 and 1000")
	}
	if p.reportedOnce && p.lastReportedProgress == value {
		// Progress did not change, nothing to do.
		return nil
	}
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		deltaProgress := value - p.lastReportedProgress
		err := reportProgress(ctx, p.project, p.attemptTimestamp, !p.reportedOnce, deltaProgress)
		return err
	})
	if err != nil {
		// If we get an error back, we are not sure if the transaction
		// failed to commit, or if it did commit but our connection to
		// Spanner dropped. We should treat the token as invalid and
		// not report any more progress for this shard.
		p.invalid = true
		return err
	}
	p.reportedOnce = true
	p.lastReportedProgress = value
	return nil
}
