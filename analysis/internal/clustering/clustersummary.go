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

// ClusterSummary captures information about a cluster.
// This is a subset of the information captured by LUCI Analysis for failures.
type ClusterSummary struct {
	// Example is an example failure contained within the cluster.
	Example Failure

	// TopTests is a list of up to 5 most commonly occurring tests
	// included in the cluster.
	TopTests []string
}
