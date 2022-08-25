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

package rpc

import (
	"strings"

	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/rulesalgorithm"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func createClusterIdPB(b clustering.ClusterID) *pb.ClusterId {
	return &pb.ClusterId{
		Algorithm: aliasAlgorithm(b.Algorithm),
		Id:        b.ID,
	}
}

func aliasAlgorithm(algorithm string) string {
	// Drop the version number from the rules algorithm,
	// e.g. "rules-v2" -> "rules".
	// Clients may want to identify if the rules algorithm
	// was used to cluster, to identify clusters for which they
	// can lookup the corresponding rule.
	// Hiding the version information avoids clients
	// accidentally depending on it in their code, which would
	// make changing the version of the rules-based clustering
	// algorithm breaking for clients.
	// It is anticipated that future updates to the rules-based
	// clustering algorithm will mostly be about tweaking the
	// failure-matching semantics, and will retain the property
	// that the Cluster ID corresponds to the Rule ID.
	if strings.HasPrefix(algorithm, clustering.RulesAlgorithmPrefix) {
		return "rules"
	}
	return algorithm
}

func resolveAlgorithm(algorithm string) string {
	// Resolve an alias to the rules algorithm to the concrete
	// implementation, e.g. "rules" -> "rules-v2".
	if algorithm == "rules" {
		return rulesalgorithm.AlgorithmName
	}
	return algorithm
}
