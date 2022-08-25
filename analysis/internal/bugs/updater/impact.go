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
	"go.chromium.org/luci/analysis/internal/bugs"
)

// ExtractResidualImpact extracts the residual impact from a
// cluster. For suggested clusters, residual impact
// is the impact of the cluster after failures that are already
// part of a bug cluster are removed.
func ExtractResidualImpact(c *analysis.Cluster) *bugs.ClusterImpact {
	return &bugs.ClusterImpact{
		CriticalFailuresExonerated: bugs.MetricImpact{
			OneDay:   c.CriticalFailuresExonerated1d.Residual,
			ThreeDay: c.CriticalFailuresExonerated3d.Residual,
			SevenDay: c.CriticalFailuresExonerated7d.Residual,
		},
		TestResultsFailed: bugs.MetricImpact{
			OneDay:   c.Failures1d.Residual,
			ThreeDay: c.Failures3d.Residual,
			SevenDay: c.Failures7d.Residual,
		},
		TestRunsFailed: bugs.MetricImpact{
			OneDay:   c.TestRunFails1d.Residual,
			ThreeDay: c.TestRunFails3d.Residual,
			SevenDay: c.TestRunFails7d.Residual,
		},
		PresubmitRunsFailed: bugs.MetricImpact{
			OneDay:   c.PresubmitRejects1d.Residual,
			ThreeDay: c.PresubmitRejects3d.Residual,
			SevenDay: c.PresubmitRejects7d.Residual,
		},
	}
}

// SetResidualImpact sets the residual impact on a cluster summary.
func SetResidualImpact(cs *analysis.Cluster, impact *bugs.ClusterImpact) {
	cs.CriticalFailuresExonerated1d.Residual = impact.CriticalFailuresExonerated.OneDay
	cs.CriticalFailuresExonerated3d.Residual = impact.CriticalFailuresExonerated.ThreeDay
	cs.CriticalFailuresExonerated7d.Residual = impact.CriticalFailuresExonerated.SevenDay

	cs.Failures1d.Residual = impact.TestResultsFailed.OneDay
	cs.Failures3d.Residual = impact.TestResultsFailed.ThreeDay
	cs.Failures7d.Residual = impact.TestResultsFailed.SevenDay

	cs.TestRunFails1d.Residual = impact.TestRunsFailed.OneDay
	cs.TestRunFails3d.Residual = impact.TestRunsFailed.ThreeDay
	cs.TestRunFails7d.Residual = impact.TestRunsFailed.SevenDay

	cs.PresubmitRejects1d.Residual = impact.PresubmitRunsFailed.OneDay
	cs.PresubmitRejects3d.Residual = impact.PresubmitRunsFailed.ThreeDay
	cs.PresubmitRejects7d.Residual = impact.PresubmitRunsFailed.SevenDay
}
