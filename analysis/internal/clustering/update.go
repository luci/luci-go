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
	cpb "go.chromium.org/luci/analysis/internal/clustering/proto"
)

// Update describes changes made to the clustering of a chunk.
type Update struct {
	// Project is the LUCI Project containing the chunk which is being
	// (re-)clustered.
	Project string
	// ChunkID is the identity of the chunk which is being (re-)clustered.
	ChunkID string
	// Updates describes how each failure in the cluster was (re)clustered.
	// It contains one entry for each failure in the cluster that has
	// had its clusters changed.
	Updates []*FailureUpdate
}

// FailureUpdate describes the changes made to the clustering
// of a specific test failure.
type FailureUpdate struct {
	// TestResult is the failure that was re-clustered.
	TestResult *cpb.Failure
	// PreviousClusters are the clusters the failure was previously in.
	PreviousClusters []ClusterID
	// PreviousClusters are the clusters the failure is now in.
	NewClusters []ClusterID
}
