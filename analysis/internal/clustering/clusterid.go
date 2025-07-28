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
	"fmt"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// MaxClusterIDBytes is the maximum number of bytes the algorithm-determined
// cluster ID may occupy. This is the raw number of bytes; if the ID is hex-
// encoded (e.g. for use in a BigQuery table), its length in characters may
// be double this number.
const MaxClusterIDBytes = 16

// RulesAlgorithmPrefix is the algorithm name prefix used by all versions
// of the rules-based clustering algorithm.
const RulesAlgorithmPrefix = "rules-"

// TestNameAlgorithmPrefix is the algorithm name prefix used by all versions
// of the test name clustering algorithm.
const TestNameAlgorithmPrefix = "testname-"

// FailureReasonAlgorithmPrefix is the algorithm name prefix used by all versions
// of the failure reason clustering algorithm.
const FailureReasonAlgorithmPrefix = "reason-"

// ClusterID represents the identity of a cluster. The LUCI Project is
// omitted as it is assumed to be implicit from the context.
type ClusterID struct {
	// Algorithm is the name of the clustering algorithm that identified
	// the cluster.
	Algorithm string `json:"algorithm"`
	// ID is the cluster identifier returned by the algorithm. The underlying
	// identifier is at most 16 bytes, but is represented here as a hexadecimal
	// string of up to 32 lowercase hexadecimal characters.
	ID string `json:"id"`
}

// Key returns a value that can be used to uniquely identify the Cluster.
// This is designed for cases where it is desirable for cluster IDs
// to be used as keys in a map.
func (c ClusterID) Key() string {
	return fmt.Sprintf("%s:%s", c.Algorithm, c.ID)
}

// String returns a string-representation of the cluster, for debugging.
func (c ClusterID) String() string {
	return c.Key()
}

// Validate validates the algorithm and ID parts
// of the cluster ID are valid.
func (c ClusterID) Validate() error {
	if !AlgorithmRe.MatchString(c.Algorithm) {
		return errors.New("algorithm not valid")
	}
	if err := c.ValidateIDPart(); err != nil {
		return err
	}
	return nil
}

// ValidateIDPart validates that the ID part of the cluster ID is valid.
func (c ClusterID) ValidateIDPart() error {
	valid := true
	for _, r := range c.ID {
		// ID must be always be stored in lowercase, so that string equality can
		// be used to determine if IDs are the same.
		if !(('0' <= r && r <= '9') || ('a' <= r && r <= 'f')) {
			valid = false
		}
	}
	if !valid || (len(c.ID)%2 != 0) {
		return errors.New("ID is not valid lowercase hexadecimal bytes")
	}
	bytes := len(c.ID) / 2
	if bytes > MaxClusterIDBytes {
		return fmt.Errorf("ID is too long (got %v bytes, want at most %v bytes)", bytes, MaxClusterIDBytes)
	}
	if bytes == 0 {
		return errors.New("ID is empty")
	}
	return nil
}

// IsEmpty returns whether the cluster ID is equal to its
// zero value.
func (c ClusterID) IsEmpty() bool {
	return c.Algorithm == "" && c.ID == ""
}

// IsBugCluster returns whether this cluster is backed by a failure
// association rule, and produced by a version of the failure association
// rule based clustering algorithm.
func (c ClusterID) IsBugCluster() bool {
	return strings.HasPrefix(c.Algorithm, RulesAlgorithmPrefix)
}

// IsTestNameCluster returns whether this cluster was made by a version
// of the test name clustering algorithm.
func (c ClusterID) IsTestNameCluster() bool {
	return strings.HasPrefix(c.Algorithm, TestNameAlgorithmPrefix)
}

// IsFailureReasonCluster returns whether this cluster was made by a version
// of the failure reason clustering algorithm.
func (c ClusterID) IsFailureReasonCluster() bool {
	return strings.HasPrefix(c.Algorithm, FailureReasonAlgorithmPrefix)
}

// SortClusters sorts the given clusters in ascending algorithm and then ID
// order.
func SortClusters(cs []ClusterID) {
	// There are almost always a tiny number of clusters per test result,
	// so a bubble-sort is surpringly faster than the built-in quicksort
	// which has to make memory allocations.
	for {
		done := true
		for i := 0; i < len(cs)-1; i++ {
			if isClusterLess(cs[i+1], cs[i]) {
				cs[i+1], cs[i] = cs[i], cs[i+1]
				done = false
			}
		}
		if done {
			break
		}
	}
}

// ClustersAreSortedNoDuplicates verifies that clusters are in sorted order
// and there are no duplicate clusters.
func ClustersAreSortedNoDuplicates(cs []ClusterID) bool {
	for i := 0; i < len(cs)-1; i++ {
		if !isClusterLess(cs[i], cs[i+1]) {
			return false
		}
	}
	return true
}

func isClusterLess(a ClusterID, b ClusterID) bool {
	if a.Algorithm == b.Algorithm {
		return a.ID < b.ID
	}
	return a.Algorithm < b.Algorithm
}
