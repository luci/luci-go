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
	txn, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	lastCompleted, err := ReadLastComplete(txn, project)
	if err != nil {
		return nil, err
	}

	lastWithProgress, err := ReadLastWithProgress(txn, project)
	if err != nil {
		return nil, err
	}

	last, err := ReadLast(txn, project)
	if err != nil {
		return nil, err
	}

	runProgress := 0
	next := ReclusteringTarget{
		RulesVersion:      last.RulesVersion,
		ConfigVersion:     last.ConfigVersion,
		AlgorithmsVersion: last.AlgorithmsVersion,
	}

	if last.RulesVersion.Equal(lastWithProgress.RulesVersion) &&
		last.AlgorithmsVersion == lastWithProgress.AlgorithmsVersion &&
		last.ConfigVersion.Equal(lastWithProgress.ConfigVersion) {
		// Scale run progress to being from 0 to 1000.
		runProgress = int(lastWithProgress.Progress / lastWithProgress.ShardCount)
	}

	return &ReclusteringProgress{
		ProgressPerMille: runProgress,
		Next:             next,
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
