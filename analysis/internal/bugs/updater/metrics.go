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

package updater

import (
	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/bugs"
)

// ExtractResidualMetrics extracts the values of metrics
// calculated based only on residual failures.
// For suggested clusters, residual failures are the
// failures left after failures that are already associated
// with a bug are removed.
func ExtractResidualMetrics(c *analysis.Cluster) *bugs.ClusterMetrics {
	residualImpact := bugs.ClusterMetrics{}
	for id, counts := range c.MetricValues {
		residualImpact[id] = extractMetricValues(counts)
	}
	return &residualImpact
}

func extractMetricValues(counts metrics.TimewiseCounts) bugs.MetricValues {
	return bugs.MetricValues{
		OneDay:   counts.OneDay.Residual,
		ThreeDay: counts.ThreeDay.Residual,
		SevenDay: counts.SevenDay.Residual,
	}
}

// SetResidualMetrics sets the value of metrics calculated
// on residual failures.
// This method exists for testing purposes only.
func SetResidualMetrics(cs *analysis.Cluster, impact *bugs.ClusterMetrics) {
	for k, v := range *impact {
		cs.MetricValues[k] = replaceResidualImpact(cs.MetricValues[k], v)
	}
}

func replaceResidualImpact(counts metrics.TimewiseCounts, impact bugs.MetricValues) metrics.TimewiseCounts {
	counts.OneDay.Residual = impact.OneDay
	counts.ThreeDay.Residual = impact.ThreeDay
	counts.SevenDay.Residual = impact.SevenDay
	return counts
}
