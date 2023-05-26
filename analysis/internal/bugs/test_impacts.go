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

package bugs

import (
	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

func TestBugFilingThresholds() []*configpb.ImpactMetricThreshold {
	return []*configpb.ImpactMetricThreshold{
		// Should be equally or more onerous than the lowest
		// priority threshold.
		{
			MetricId:  metrics.Failures.ID.String(),
			Threshold: &configpb.MetricThreshold{OneDay: proto.Int64(75)},
		},
	}
}

// P0Impact returns cluster impact that is consistent with a P0 bug.
func P0Impact() *ClusterImpact {
	return &ClusterImpact{metrics.Failures.ID: MetricImpact{OneDay: 1500}}
}

// P1Impact returns cluster impact that is consistent with a P1 bug.
func P1Impact() *ClusterImpact {
	return &ClusterImpact{metrics.Failures.ID: MetricImpact{OneDay: 750}}
}

// LowP1Impact returns cluster impact that is consistent with a P1
// bug, but if hysteresis is applied, could also be compatible with P2.
func LowP1Impact() *ClusterImpact {
	// (500 * (1.0 + PriorityHysteresisPercent / 100.0)) - 1
	return &ClusterImpact{metrics.Failures.ID: MetricImpact{OneDay: 549}}
}

// P2Impact returns cluster impact that is consistent with a P2 bug.
func P2Impact() *ClusterImpact {
	return &ClusterImpact{metrics.Failures.ID: MetricImpact{OneDay: 300}}
}

// HighP3Impact returns cluster impact that is consistent with a P3
// bug, but if hysteresis is applied, could also be compatible with P2.
func HighP3Impact() *ClusterImpact {
	// (100 / (1.0 + PriorityHysteresisPercent / 100.0)) + 1
	return &ClusterImpact{metrics.Failures.ID: MetricImpact{OneDay: 91}}
}

// P3Impact returns cluster impact that is consistent with a P3 bug.
func P3Impact() *ClusterImpact {
	return &ClusterImpact{metrics.Failures.ID: MetricImpact{OneDay: 75}}
}

// HighestNotFiledImpact returns the highest cluster impact
// that can be consistent with a bug not being filed.
func HighestNotFiledImpact() *ClusterImpact {
	return &ClusterImpact{metrics.Failures.ID: MetricImpact{OneDay: 74}} // 75 - 1
}

// P3LowestBeforeClosureImpact returns cluster impact that
// is the lowest impact that can be compatible with a P3 bug,
// after including hysteresis.
func P3LowestBeforeClosureImpact() *ClusterImpact {
	// (50 / (1.0 + PriorityHysteresisPercent / 100.0)) + 1
	return &ClusterImpact{metrics.Failures.ID: MetricImpact{OneDay: 46}}
}

// ClosureImpact returns cluster impact that is consistent with a
// closed (verified) bug.
func ClosureImpact() *ClusterImpact {
	return &ClusterImpact{}
}
