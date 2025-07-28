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
	"time"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/clustering/shards"
)

// ReclusteringTarget captures the rules and algorithms a re-clustering run
// is re-clustering to.
type ReclusteringTarget struct {
	// RulesVersion is the rules version the re-clustering run is attempting
	// to achieve.
	RulesVersion time.Time `json:"rulesVersion"`
	// ConfigVersion is the config version the re-clustering run is attempting
	// to achieve.
	ConfigVersion time.Time `json:"configVersion"`
	// AlgorithmsVersion is the algorithms version the re-clustering run is
	// attempting to achieve.
	AlgorithmsVersion int64 `json:"algorithmsVersion"`
}

// ReclusteringProgress captures the progress re-clustering a
// given LUCI project's test results using specific rules
// versions or algorithms versions.
type ReclusteringProgress struct {
	// ProgressPerMille is the progress of the current re-clustering run,
	// measured in thousandths (per mille).
	ProgressPerMille int `json:"progressPerMille"`
	// Next is the goal of the current re-clustering run. (For which
	// ProgressPerMille is specified.)
	Next ReclusteringTarget `json:"next"`
	// Last is the goal of the last completed re-clustering run.
	Last ReclusteringTarget `json:"last"`
}

// ReadReclusteringProgress reads the re-clustering progress for
// the given LUCI project.
func ReadReclusteringProgress(ctx context.Context, project string) (*ReclusteringProgress, error) {
	return ReadReclusteringProgressUpTo(ctx, project, MaxAttemptTimestamp)
}

// ReadReclusteringProgressUpTo reads the re-clustering progress for
// the given LUCI project up to the given reclustering attempt
// timestamp.
//
// For the latest re-clustering progess, pass MaxAttemptTimestamp.
// For re-clustering as it was at a given attempt minute in the
// past, pass the timestamp for that minute. Reading past reclustering
// progress is useful for understanding the versions of rules and
// algorithms reflected in outputs of BigQuery jobs which ran
// using data some minutes old.
//
// When reading historic progress, data for reclustering progress
// as it was part way through a minute is not available; this is
// also why the passed timestamp is called 'upToAttemptTimestamp'
// not an asAtTime.
func ReadReclusteringProgressUpTo(ctx context.Context, project string, upToAttemptTimestamp time.Time) (*ReclusteringProgress, error) {
	// Reading reclustering progress as at a time in the past can also
	// be achieved with Spanner stale reads, but this does not have
	// good testability. Implement reading past reclustering state
	// using the history we are already storing in the table.
	txn, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	lastCompleted, err := ReadLastCompleteUpTo(txn, project, upToAttemptTimestamp)
	if err != nil {
		return nil, err
	}

	last, err := ReadLastUpTo(txn, project, upToAttemptTimestamp)
	if err != nil {
		return nil, err
	}

	runProgress := 0

	// For the most recent run, try to read live progress
	// from the reclustering shards table.
	liveProgress, err := shards.ReadProgress(txn, project, last.AttemptTimestamp)
	if err != nil {
		return nil, err
	}
	// Use live progress if it is available.
	if liveProgress.ShardCount > 0 && liveProgress.ShardCount == liveProgress.ShardsReported {
		// Scale run progress to being from 0 to 1000.
		runProgress = int(liveProgress.Progress / liveProgress.ShardCount)
		if runProgress == shards.MaxProgress {
			// If the run is complete.
			lastCompleted = last
		}
	} else {
		// Otherwise, try to use the progress that was on the last
		// run with progress.
		lastWithProgress, err := ReadLastWithProgressUpTo(txn, project, upToAttemptTimestamp)
		if err != nil {
			return nil, err
		}

		// If the last reclustering run row with progress has the same
		// reclustering objective as the current run, use its progress.
		if last.RulesVersion.Equal(lastWithProgress.RulesVersion) &&
			last.AlgorithmsVersion == lastWithProgress.AlgorithmsVersion &&
			last.ConfigVersion.Equal(lastWithProgress.ConfigVersion) {
			// Scale run progress to being from 0 to 1000.
			runProgress = int(lastWithProgress.Progress / lastWithProgress.ShardCount)
		}
	}

	return &ReclusteringProgress{
		ProgressPerMille: runProgress,
		Next: ReclusteringTarget{
			RulesVersion:      last.RulesVersion,
			ConfigVersion:     last.ConfigVersion,
			AlgorithmsVersion: last.AlgorithmsVersion,
		},
		Last: ReclusteringTarget{
			RulesVersion:      lastCompleted.RulesVersion,
			ConfigVersion:     lastCompleted.ConfigVersion,
			AlgorithmsVersion: lastCompleted.AlgorithmsVersion,
		},
	}, nil
}

// IsReclusteringToNewAlgorithms returns whether LUCI Analysis's
// clustering output is being updated to use a newer standard of
// algorithms and is not yet stable. The algorithms version LUCI Analysis
// is re-clustering to is accessible via LatestAlgorithmsVersion.
func (p *ReclusteringProgress) IsReclusteringToNewAlgorithms() bool {
	return p.Last.AlgorithmsVersion < p.Next.AlgorithmsVersion
}

// IsReclusteringToNewConfig returns whether LUCI Analysis's
// clustering output is in the process of being updated to a later
// configuration standard and is not yet stable.
// The configuration version LUCI Analysis is re-clustering to is
// accessible via LatestConfigVersion.
// Clients using re-clustering output should verify they are using
// the configuration version defined by LatestConfigVersion when
// interpreting the output.
func (p *ReclusteringProgress) IsReclusteringToNewConfig() bool {
	return p.Last.ConfigVersion.Before(p.Next.ConfigVersion)
}

// IncorporatesRulesVersion returns returns whether LUCI Analysis
// clustering output incorporates all rule changes up to
// the given predicate last updated time. Later changes
// may also be included, in full or in part.
func (p *ReclusteringProgress) IncorporatesRulesVersion(rulesVersion time.Time) bool {
	return !rulesVersion.After(p.Last.RulesVersion)
}
