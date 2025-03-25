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

package rulesalgorithm

import (
	"fmt"
	"time"

	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/rules/cache"
	"go.chromium.org/luci/analysis/internal/clustering/rules/lang"
)

type Algorithm struct{}

// AlgorithmVersion is the version of the clustering algorithm. The algorithm
// version should be incremented whenever existing test results may be
// clustered differently (i.e. Cluster(f) returns a different value for some
// f that may have been already ingested).
const AlgorithmVersion = 3

// AlgorithmName is the identifier for the clustering algorithm.
// LUCI Analysis requires all clustering algorithms to have a unique
// identifier. Must match the pattern ^[a-z0-9-.]{1,32}$.
//
// The AlgorithmName must encode the algorithm version, so that each version
// of an algorithm has a different name.
var AlgorithmName = fmt.Sprintf("%sv%v", clustering.RulesAlgorithmPrefix, AlgorithmVersion)

// Cluster incrementally (re-)clusters the given test failure, updating the
// matched cluster IDs. The passed existingRulesVersion and ruleIDs
// should be the ruleset.RulesVersion and cluster IDs of the previous call
// to Cluster (if any) from which incremental clustering should occur.
//
// If clustering has not been performed previously, and clustering is to be
// performed from scratch, existingRulesVersion should be rules.StartingEpoch
// and ruleIDs should be an empty list.
//
// This method is on the performance-critical path for re-clustering.
//
// To avoid unnecessary allocations, the method will modify the passed ruleIDs.
func (a *Algorithm) Cluster(ruleset *cache.Ruleset, existingRulesVersion time.Time, ruleIDs map[string]struct{}, failure *clustering.Failure) {
	for id := range ruleIDs {
		// Remove matches with rules that are no longer active.
		if !ruleset.IsRuleActive(id) {
			delete(ruleIDs, id)
		}
	}

	// For efficiency, only match new/modified rules since the
	// last call to Cluster(...).
	newRules := ruleset.ActiveRulesWithPredicateUpdatedSince(existingRulesVersion)
	for _, r := range newRules {
		f := lang.Failure{
			Test:   failure.TestID,
			Reason: failure.Reason.GetPrimaryErrorMessage(),
		}
		matches := r.Expr.Evaluate(f)
		if failure.PreviousTestID != "" {
			// Also try matching against the old test ID.
			f = lang.Failure{
				Test:   failure.PreviousTestID,
				Reason: failure.Reason.GetPrimaryErrorMessage(),
			}
			matches = matches || r.Expr.Evaluate(f)
		}
		if matches {
			ruleIDs[r.Rule.RuleID] = struct{}{}
		} else {
			// If this is a modified rule (rather than a new rule)
			// it may have matched previously. Delete any existing
			// match.
			delete(ruleIDs, r.Rule.RuleID)
		}
	}
}
