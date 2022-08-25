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

package clustering

import (
	"time"
)

// ClusterResults represents the results of clustering a list of
// test failures.
type ClusterResults struct {
	// AlgorithmsVersion is the version of clustering algorithms used to
	// cluster test results in this chunk. (This is a version over the
	// set of algorithms, distinct from the version of a single algorithm,
	// e.g.: v1 -> {reason-v1}, v2 -> {reason-v1, testname-v1},
	// v3 -> {reason-v2, testname-v1}.)
	AlgorithmsVersion int64
	// ConfigVersion is the version of Weetbix project configuration
	// used to cluster the test results. Clustering algorithms can rely
	// on the configuration to alter their behaviour, so changes to
	// the configuration should trigger re-clustering of test results.
	ConfigVersion time.Time
	// RulesVersion is the version of failure association rules used
	// to cluster test results.  This is most recent PredicateLastUpdated
	// time in the snapshot of failure association rules used to cluster
	// the test results.
	RulesVersion time.Time
	// Algorithms is the set of algorithms that were used to cluster
	// the test results. Each entry is an algorithm name.
	// When stored alongside the clustered test results, this allows only
	// the new algorithms to be run when re-clustering (for efficiency).
	Algorithms map[string]struct{}
	// Clusters records the clusters each test result is in;
	// one slice of ClusterIDs for each test result. For each test result,
	// clusters must be in sorted order, with no duplicates.
	Clusters [][]ClusterID
}

// AlgorithmsAndClustersEqual returns whether the algorithms and clusters of
// two cluster results are equivalent.
func AlgorithmsAndClustersEqual(a *ClusterResults, b *ClusterResults) bool {
	if !setsEqual(a.Algorithms, b.Algorithms) {
		return false
	}
	if len(a.Clusters) != len(b.Clusters) {
		return false
	}
	for i, aClusters := range a.Clusters {
		bClusters := b.Clusters[i]
		if !ClustersEqual(aClusters, bClusters) {
			return false
		}
	}
	return true
}

// ClustersEqual returns whether the clusters in `as` are element-wise
// equal to those in `bs`.
// To test set-wise cluster equality, this method is called with
// clusters in sorted order, and no duplicates.
func ClustersEqual(as []ClusterID, bs []ClusterID) bool {
	if len(as) != len(bs) {
		return false
	}
	for i, a := range as {
		b := bs[i]
		if a.Algorithm != b.Algorithm {
			return false
		}
		if a.ID != b.ID {
			return false
		}
	}
	return true
}

// setsEqual returns whether two sets are equal.
func setsEqual(a map[string]struct{}, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for key := range a {
		if _, ok := b[key]; !ok {
			return false
		}
	}
	return true
}
