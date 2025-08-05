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
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

func TestThresholding(t *testing.T) {
	t.Parallel()

	ftt.Run("MeetsThreshold", t, func(t *ftt.Test) {
		oneDayThreshold := new(int64)
		*oneDayThreshold = 10
		threeDayThreshold := new(int64)
		*threeDayThreshold = 30
		sevenDayThreshold := new(int64)
		*sevenDayThreshold = 70
		testCases := []struct {
			name                  string
			clusterMetrics        *ClusterMetrics
			metricID              metrics.ID
			metricThreshold       *configpb.MetricThreshold
			expectedThresholdsMet ThresholdsMetPerTimeInterval
		}{
			{
				name: "debugging test",
				clusterMetrics: &ClusterMetrics{
					metrics.CriticalFailuresExonerated.ID: MetricValues{
						OneDay:   60,
						ThreeDay: 180,
						SevenDay: 420,
					},
				},
				metricID: metrics.CriticalFailuresExonerated.ID,
				metricThreshold: &configpb.MetricThreshold{
					OneDay:   oneDayThreshold,
					ThreeDay: threeDayThreshold,
					SevenDay: sevenDayThreshold,
				},
				expectedThresholdsMet: ThresholdsMetPerTimeInterval{
					OneDay:   true,
					ThreeDay: true,
					SevenDay: true,
				},
			},
			{
				// oneday and threeday metrics should be compared to the sevenday threshold
				name: "metrics for shorter time intervals should be compared to thresholds for longer time intervals",
				clusterMetrics: &ClusterMetrics{
					metrics.CriticalFailuresExonerated.ID: MetricValues{
						OneDay:   100,
						ThreeDay: 180,
						SevenDay: 420,
					},
				},
				metricID: metrics.CriticalFailuresExonerated.ID,
				metricThreshold: &configpb.MetricThreshold{
					SevenDay: sevenDayThreshold,
				},
				expectedThresholdsMet: ThresholdsMetPerTimeInterval{
					OneDay:   true,
					ThreeDay: true,
					SevenDay: true,
				},
			},
			{
				name: "metrics for longer time intervals should not be compared to thresholds for shorter time intervals",
				clusterMetrics: &ClusterMetrics{
					metrics.CriticalFailuresExonerated.ID: MetricValues{
						OneDay:   100,
						ThreeDay: 180,
						SevenDay: 420,
					},
				},
				metricID: metrics.CriticalFailuresExonerated.ID,
				metricThreshold: &configpb.MetricThreshold{
					OneDay: oneDayThreshold,
				},
				expectedThresholdsMet: ThresholdsMetPerTimeInterval{
					OneDay:   true,
					ThreeDay: false,
					SevenDay: false,
				},
			},
		}

		for _, tc := range testCases {
			t.Parallel(tc.name, func(t *ftt.Test) {
				thresholdsMet := tc.clusterMetrics.MeetsThreshold(tc.metricID, tc.metricThreshold)
				if diff := cmp.Diff(tc.expectedThresholdsMet, thresholdsMet); diff != "" {
					t.Errorf("Resolution diff (-want +got):\n%s", diff)
				}
			})
		}
	})

	ftt.Run("With Cluster", t, func(t *ftt.Test) {
		cl := &ClusterMetrics{
			metrics.CriticalFailuresExonerated.ID: MetricValues{
				OneDay:   60,
				ThreeDay: 180,
				SevenDay: 420,
			},
			metrics.Failures.ID: MetricValues{
				OneDay:   100,
				ThreeDay: 300,
				SevenDay: 700,
			},
		}
		t.Run("MeetsAnyOfThresholds", func(t *ftt.Test) {
			t.Run("No cluster meets empty threshold", func(t *ftt.Test) {
				thresh := []*configpb.ImpactMetricThreshold{}
				assert.Loosely(t, cl.MeetsAnyOfThresholds(thresh), should.BeFalse)
			})
			t.Run("Critical failures exonerated thresholding", func(t *ftt.Test) {
				thresh := setThresholdByID(metrics.CriticalFailuresExonerated.ID, &configpb.MetricThreshold{OneDay: proto.Int64(60)})
				assert.Loosely(t, cl.MeetsAnyOfThresholds(thresh), should.BeTrue)

				thresh = setThresholdByID(metrics.CriticalFailuresExonerated.ID, &configpb.MetricThreshold{OneDay: proto.Int64(61)})
				assert.Loosely(t, cl.MeetsAnyOfThresholds(thresh), should.BeFalse)
			})
			t.Run("Test results failed thresholding", func(t *ftt.Test) {
				thresh := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{OneDay: proto.Int64(100)})
				assert.Loosely(t, cl.MeetsAnyOfThresholds(thresh), should.BeTrue)

				thresh = setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{OneDay: proto.Int64(101)})
				assert.Loosely(t, cl.MeetsAnyOfThresholds(thresh), should.BeFalse)
			})
			t.Run("One day threshold", func(t *ftt.Test) {
				thresh := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{OneDay: proto.Int64(100)})
				assert.Loosely(t, cl.MeetsAnyOfThresholds(thresh), should.BeTrue)

				thresh = setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{OneDay: proto.Int64(101)})
				assert.Loosely(t, cl.MeetsAnyOfThresholds(thresh), should.BeFalse)
			})
			t.Run("Three day threshold", func(t *ftt.Test) {
				thresh := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{ThreeDay: proto.Int64(300)})
				assert.Loosely(t, cl.MeetsAnyOfThresholds(thresh), should.BeTrue)

				thresh = setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{ThreeDay: proto.Int64(301)})
				assert.Loosely(t, cl.MeetsAnyOfThresholds(thresh), should.BeFalse)
			})
			t.Run("Seven day threshold", func(t *ftt.Test) {
				thresh := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{SevenDay: proto.Int64(700)})
				assert.Loosely(t, cl.MeetsAnyOfThresholds(thresh), should.BeTrue)

				thresh = setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{SevenDay: proto.Int64(701)})
				assert.Loosely(t, cl.MeetsAnyOfThresholds(thresh), should.BeFalse)
			})
		})
		t.Run("InflateThreshold", func(t *ftt.Test) {
			t.Run("Empty threshold", func(t *ftt.Test) {
				thresh := []*configpb.ImpactMetricThreshold{}
				result := InflateThreshold(thresh, 15)
				assert.Loosely(t, result, should.Match([]*configpb.ImpactMetricThreshold{}))
			})
			t.Run("One day threshold", func(t *ftt.Test) {
				thresh := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{OneDay: proto.Int64(100)})
				result := InflateThreshold(thresh, 15)
				assert.Loosely(t, result, should.Match([]*configpb.ImpactMetricThreshold{
					{MetricId: string(metrics.Failures.ID), Threshold: &configpb.MetricThreshold{OneDay: proto.Int64(115)}},
				}))
			})
			t.Run("Three day threshold", func(t *ftt.Test) {
				thresh := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{ThreeDay: proto.Int64(100)})
				result := InflateThreshold(thresh, 15)
				assert.Loosely(t, result, should.Match([]*configpb.ImpactMetricThreshold{
					{MetricId: string(metrics.Failures.ID), Threshold: &configpb.MetricThreshold{ThreeDay: proto.Int64(115)}},
				}))
			})
			t.Run("Seven day threshold", func(t *ftt.Test) {
				thresh := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{SevenDay: proto.Int64(100)})
				result := InflateThreshold(thresh, 15)
				assert.Loosely(t, result, should.Match([]*configpb.ImpactMetricThreshold{
					{MetricId: string(metrics.Failures.ID), Threshold: &configpb.MetricThreshold{SevenDay: proto.Int64(115)}},
				}))
			})
		})
	})
	ftt.Run("Zero value not inflated", t, func(t *ftt.Test) {
		input := int64(0)
		output := inflateSingleThreshold(&input, 200)
		assert.Loosely(t, output, should.NotBeNil)
		assert.Loosely(t, *output, should.BeZero)
	})
	ftt.Run("Non-zero value should not be inflated to zero", t, func(t *ftt.Test) {
		input := int64(1)
		output := inflateSingleThreshold(&input, -200)
		assert.Loosely(t, output, should.NotBeNil)
		assert.Loosely(t, *output, should.NotEqual(0))
	})
}

func setThresholdByID(metricID metrics.ID, t *configpb.MetricThreshold) []*configpb.ImpactMetricThreshold {
	return []*configpb.ImpactMetricThreshold{
		{MetricId: metricID.String(), Threshold: t},
	}
}
