// Copyright 2023 The LUCI Authors.
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

// Package pbutil contains methods for manipulating LUCI Analysis protos.
package pbutil

import (
	configpb "go.chromium.org/luci/analysis/proto/config"
)

// MetricThresholdByID returns the metricThreshold with the given id from a list of thresholds.
// It returns nil if the metric id doesn't exist in the thresholds list.
func MetricThresholdByID(id string, thresholds []*configpb.ImpactMetricThreshold) *configpb.MetricThreshold {
	for _, t := range thresholds {
		if t.MetricId == id {
			return t.Threshold
		}
	}
	return nil
}
