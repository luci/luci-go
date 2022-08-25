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

package algorithms

import (
	"encoding/hex"
	"errors"

	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/failurereason"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/rulesalgorithm"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/testname"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/rules/cache"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
)

// Algorithm represents the interface that each clustering algorithm
// generating suggested clusters must implement.
type Algorithm interface {
	// Name returns the identifier of the clustering algorithm.
	Name() string
	// Cluster clusters the given test failure and returns its cluster ID (if
	// it can be clustered) or nil otherwise. THe returned cluster ID must be
	// at most 16 bytes.
	Cluster(config *compiledcfg.ProjectConfig, failure *clustering.Failure) []byte
	// FailureAssociationRule returns a failure association rule that
	// captures the definition of the cluster containing the given example.
	FailureAssociationRule(config *compiledcfg.ProjectConfig, example *clustering.Failure) string
	// ClusterKey returns the unhashed clustering key which is common
	// across all test results in a cluster. This will be displayed
	// on the cluster page or cluster listing.
	ClusterKey(config *compiledcfg.ProjectConfig, example *clustering.Failure) string
	// ClusterDescription returns a description of the cluster, for use when
	// filing bugs, with the help of the given example failure.
	ClusterDescription(config *compiledcfg.ProjectConfig, summary *clustering.ClusterSummary) (*clustering.ClusterDescription, error)
}

// AlgorithmsVersion is the version of the set of algorithms used.
// Changing the set of algorithms below (including add, update or
// deletion of an algorithm) should result in this version being
// incremented.
//
// In case of algorithm deletion, make sure to update this constant
// appropriately to ensure the AlgorithmsVersion still increases
// (I.E. DO NOT simply delete "+ <myalgorithm>.AlgorithmVersion"
// when deleting an algorithm without rolling its value (plus one)
// into the constant.)
const AlgorithmsVersion = 1 + failurereason.AlgorithmVersion +
	testname.AlgorithmVersion + rulesalgorithm.AlgorithmVersion

// suggestingAlgorithms is the set of clustering algorithms used by
// Weetbix to generate suggested clusters.
// When an algorithm is added or removed from the set,
// or when an algorithm is updated, ensure the AlgorithmsVersion
// above increments.
var suggestingAlgorithms = []Algorithm{
	&failurereason.Algorithm{},
	&testname.Algorithm{},
}

// rulesAlgorithm is the rules-based clustering algorithm used by
// Weetbix. When this algorithm is changed, ensure the AlgorithmsVersion
// above increments.
var rulesAlgorithm = rulesalgorithm.Algorithm{}

// The set of all algorithms known by Weetbix.
var algorithmNames map[string]struct{}

// The set of all suggested algorithms known by Weetbix.
var suggestedAlgorithmNames map[string]struct{}

func init() {
	algorithmNames = make(map[string]struct{})
	suggestedAlgorithmNames = make(map[string]struct{})
	algorithmNames[rulesalgorithm.AlgorithmName] = struct{}{}
	for _, a := range suggestingAlgorithms {
		algorithmNames[a.Name()] = struct{}{}
		suggestedAlgorithmNames[a.Name()] = struct{}{}
	}
}

// Cluster performs (incremental re-)clustering of the given test
// failures using all registered clustering algorithms and the
// specified set of failure association rules and config.
//
// If the test results have not been previously clustered, pass
// an existing ClusterResults of NewEmptyClusterResults(...)
// to cluster test results from scratch.
//
// If the test results have been previously clustered, pass the
// ClusterResults returned by the last call to Cluster.
//
// Cluster(...) will always return a set of ClusterResults which
// are as- or more-recent than the existing ClusterResults.
// This is defined as the following postcondition:
//  returned.AlgorithmsVersion > existing.AlgorithmsVersion ||
//  (returned.AlgorithmsVersion == existing.AlgorithmsVersion &&
//    returned.ConfigVersion >= existing.ConfigVersion &&
//    returned.RulesVersion >= existing.RulesVersion)
func Cluster(config *compiledcfg.ProjectConfig, ruleset *cache.Ruleset, existing clustering.ClusterResults, failures []*clustering.Failure) clustering.ClusterResults {
	if existing.AlgorithmsVersion > AlgorithmsVersion {
		// We are running out-of-date clustering algorithms. Do not
		// try to improve on the existing clustering. This can
		// happen if we are rolling out a new version of Weetbix.
		return existing
	}

	newSuggestedAlgorithms := false
	for _, alg := range suggestingAlgorithms {
		if _, ok := existing.Algorithms[alg.Name()]; !ok {
			newSuggestedAlgorithms = true
		}
	}
	// We should recycle the previous suggested clusters for performance if:
	// (1) the algorithms to be run are the same (or a subset)
	//     of what was previously run, and
	// (2) the config available to us is not later than
	//     what was available when clustering occurred.
	//
	// Implied is that we may only update to suggested clusters based
	// on an earlier version of config if there are new algorithms.
	reuseSuggestedAlgorithmResults := !newSuggestedAlgorithms &&
		!config.LastUpdated.After(existing.ConfigVersion)

	// For rule-based clustering.
	_, reuseRuleAlgorithmResults := existing.Algorithms[rulesalgorithm.AlgorithmName]
	existingRulesVersion := existing.RulesVersion
	if !reuseRuleAlgorithmResults {
		// Although we may have previously run rule-based clustering, we did
		// not run the current version of that algorithm. Invalidate all
		// previous analysis; match against all rules again.
		existingRulesVersion = rules.StartingEpoch
	}

	result := make([][]clustering.ClusterID, len(failures))
	for i, f := range failures {
		newIDs := make([]clustering.ClusterID, 0, len(suggestingAlgorithms)+2)
		ruleIDs := make(map[string]struct{})

		existingIDs := existing.Clusters[i]
		for _, id := range existingIDs {
			if reuseSuggestedAlgorithmResults {
				if _, ok := suggestedAlgorithmNames[id.Algorithm]; ok {
					// The algorithm was run previously and its results are still valid.
					// Retain its results.
					newIDs = append(newIDs, id)
				}
			}
			if reuseRuleAlgorithmResults && id.Algorithm == rulesalgorithm.AlgorithmName {
				// The rules algorithm was previously run. Record the past results,
				// but separately. Some previously matched rules may have been
				// updated or made inactive since, so we need to treat these
				// separately (and pass them to the rules algorithm to filter
				// through).
				ruleIDs[id.ID] = struct{}{}
			}
		}

		if !reuseSuggestedAlgorithmResults {
			// Run the suggested clustering algorithms.
			for _, a := range suggestingAlgorithms {
				id := a.Cluster(config, f)
				if id == nil {
					continue
				}
				newIDs = append(newIDs, clustering.ClusterID{
					Algorithm: a.Name(),
					ID:        hex.EncodeToString(id),
				})
			}
		}

		if ruleset.Version.Predicates.After(existingRulesVersion) {
			// Match against the (incremental) set of rules.
			rulesAlgorithm.Cluster(ruleset, existingRulesVersion, ruleIDs, f)
		}
		// Otherwise test results were already clustered with an equal or later
		// version of rules. This can happen if our cached ruleset is out of date.
		// Re-use the existing analysis in this case; don't try to improve on it.

		for rID := range ruleIDs {
			id := clustering.ClusterID{
				Algorithm: rulesalgorithm.AlgorithmName,
				ID:        rID,
			}
			newIDs = append(newIDs, id)
		}

		// Keep the output deterministic by sorting the clusters in the
		// output.
		clustering.SortClusters(newIDs)
		result[i] = newIDs
	}

	// Base re-clustering on rule predicate changes,
	// as only the rule predicate matters for clustering.
	newRulesVersion := ruleset.Version.Predicates
	if existingRulesVersion.After(newRulesVersion) {
		// If the existing rule-matching is more current than our current
		// ruleset allows, we will have kept its results, and should keep
		// its RulesVersion.
		// This can happen sometimes if our cached ruleset is out of date.
		// This is normal.
		newRulesVersion = existingRulesVersion
	}
	newConfigVersion := existing.ConfigVersion
	if !reuseSuggestedAlgorithmResults {
		// If the we recomputed the suggested clusters, record the version
		// of config we used.
		newConfigVersion = config.LastUpdated
	}

	return clustering.ClusterResults{
		AlgorithmsVersion: AlgorithmsVersion,
		ConfigVersion:     newConfigVersion,
		RulesVersion:      newRulesVersion,
		Algorithms:        algorithmNames,
		Clusters:          result,
	}
}

// ErrAlgorithmNotExist is returned if an algorithm with the given
// name does not exist. This may indicate the algorithm
// is newer or older than the current version.
var ErrAlgorithmNotExist = errors.New("algorithm does not exist")

// SuggestingAlgorithm returns the algorithm for generating
// suggested clusters with the given name. If the algorithm does
// not exist, ErrAlgorithmNotExist is returned.
func SuggestingAlgorithm(algorithm string) (Algorithm, error) {
	for _, a := range suggestingAlgorithms {
		if a.Name() == algorithm {
			return a, nil
		}
	}
	// We may be running old code, or the caller may be asking
	// for an old (version of an) algorithm.
	return nil, ErrAlgorithmNotExist
}

// NewEmptyClusterResults returns a new ClusterResults for a list of
// test results of length count. The ClusterResults will indicate the
// test results have not been clustered.
func NewEmptyClusterResults(count int) clustering.ClusterResults {
	return clustering.ClusterResults{
		// Algorithms version 0 is the empty set of clustering algorithms.
		AlgorithmsVersion: 0,
		ConfigVersion:     config.StartingEpoch,
		// The RulesVersion StartingEpoch refers to the empty set of rules.
		RulesVersion: rules.StartingEpoch,
		Algorithms:   make(map[string]struct{}),
		Clusters:     make([][]clustering.ClusterID, count),
	}
}
