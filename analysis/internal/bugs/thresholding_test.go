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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

func TestThresholding(t *testing.T) {
	t.Parallel()

	Convey("With Cluster", t, func() {
		cl := &ClusterImpact{
			metrics.CriticalFailuresExonerated.ID: MetricImpact{
				OneDay:   60,
				ThreeDay: 180,
				SevenDay: 420,
			},
			metrics.Failures.ID: MetricImpact{
				OneDay:   100,
				ThreeDay: 300,
				SevenDay: 700,
			},
		}
		Convey("MeetsThreshold", func() {
			Convey("No cluster meets empty threshold", func() {
				t := []*configpb.ImpactMetricThreshold{}
				So(cl.MeetsThreshold(t), ShouldBeFalse)
			})
			Convey("Critical failures exonerated thresholding", func() {
				t := setThresholdByID(metrics.CriticalFailuresExonerated.ID, &configpb.MetricThreshold{OneDay: proto.Int64(60)})
				So(cl.MeetsThreshold(t), ShouldBeTrue)

				t = setThresholdByID(metrics.CriticalFailuresExonerated.ID, &configpb.MetricThreshold{OneDay: proto.Int64(61)})
				So(cl.MeetsThreshold(t), ShouldBeFalse)
			})
			Convey("Test results failed thresholding", func() {
				t := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{OneDay: proto.Int64(100)})
				So(cl.MeetsThreshold(t), ShouldBeTrue)

				t = setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{OneDay: proto.Int64(101)})
				So(cl.MeetsThreshold(t), ShouldBeFalse)
			})
			Convey("One day threshold", func() {
				t := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{OneDay: proto.Int64(100)})
				So(cl.MeetsThreshold(t), ShouldBeTrue)

				t = setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{OneDay: proto.Int64(101)})
				So(cl.MeetsThreshold(t), ShouldBeFalse)
			})
			Convey("Three day threshold", func() {
				t := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{ThreeDay: proto.Int64(300)})
				So(cl.MeetsThreshold(t), ShouldBeTrue)

				t = setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{ThreeDay: proto.Int64(301)})
				So(cl.MeetsThreshold(t), ShouldBeFalse)
			})
			Convey("Seven day threshold", func() {
				t := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{SevenDay: proto.Int64(700)})
				So(cl.MeetsThreshold(t), ShouldBeTrue)

				t = setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{SevenDay: proto.Int64(701)})
				So(cl.MeetsThreshold(t), ShouldBeFalse)
			})
		})
		Convey("InflateThreshold", func() {
			Convey("Empty threshold", func() {
				t := []*configpb.ImpactMetricThreshold{}
				result := InflateThreshold(t, 15)
				So(result, ShouldResembleProto, []*configpb.ImpactMetricThreshold{})
			})
			Convey("One day threshold", func() {
				t := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{OneDay: proto.Int64(100)})
				result := InflateThreshold(t, 15)
				So(result, ShouldResembleProto, []*configpb.ImpactMetricThreshold{
					{MetricId: string(metrics.Failures.ID), Threshold: &configpb.MetricThreshold{OneDay: proto.Int64(115)}},
				})
			})
			Convey("Three day threshold", func() {
				t := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{ThreeDay: proto.Int64(100)})
				result := InflateThreshold(t, 15)
				So(result, ShouldResembleProto, []*configpb.ImpactMetricThreshold{
					{MetricId: string(metrics.Failures.ID), Threshold: &configpb.MetricThreshold{ThreeDay: proto.Int64(115)}},
				})
			})
			Convey("Seven day threshold", func() {
				t := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{SevenDay: proto.Int64(100)})
				result := InflateThreshold(t, 15)
				So(result, ShouldResembleProto, []*configpb.ImpactMetricThreshold{
					{MetricId: string(metrics.Failures.ID), Threshold: &configpb.MetricThreshold{SevenDay: proto.Int64(115)}},
				})
			})
		})
		Convey("ExplainThresholdMet", func() {
			t := &configpb.ImpactMetricThreshold{
				MetricId: string(metrics.Failures.ID),
				Threshold: &configpb.MetricThreshold{
					OneDay:   proto.Int64(101), // Not met.
					ThreeDay: proto.Int64(299), // Met.
					SevenDay: proto.Int64(699), // Met.
				},
			}
			explanation := cl.ExplainThresholdMet([]*configpb.ImpactMetricThreshold{t})
			So(explanation, ShouldResemble, ThresholdExplanation{
				Metric:        metrics.Failures.HumanReadableName,
				TimescaleDays: 3,
				Threshold:     299,
			})
		})
		Convey("ExplainThresholdNotMet", func() {
			t := []*configpb.ImpactMetricThreshold{
				{MetricId: metrics.CriticalFailuresExonerated.ID.String(), Threshold: &configpb.MetricThreshold{
					OneDay: proto.Int64(61), // Not met.
				}},
				{MetricId: metrics.HumanClsFailedPresubmit.ID.String(), Threshold: &configpb.MetricThreshold{
					SevenDay: proto.Int64(701), // Not met.
				}},
				{MetricId: metrics.TestRunsFailed.ID.String(), Threshold: &configpb.MetricThreshold{
					ThreeDay: proto.Int64(301), // Not met.
				}},
				{MetricId: metrics.Failures.ID.String(), Threshold: &configpb.MetricThreshold{
					OneDay: proto.Int64(101), // Not met.
				}},
			}
			explanation := ExplainThresholdNotMet(t)
			So(explanation, ShouldResemble, []ThresholdExplanation{
				{
					Metric:        metrics.CriticalFailuresExonerated.HumanReadableName,
					TimescaleDays: 1,
					Threshold:     61,
				},
				{
					Metric:        metrics.HumanClsFailedPresubmit.HumanReadableName,
					TimescaleDays: 7,
					Threshold:     701,
				},
				{
					Metric:        metrics.TestRunsFailed.HumanReadableName,
					TimescaleDays: 3,
					Threshold:     301,
				},
				{
					Metric:        metrics.Failures.HumanReadableName,
					TimescaleDays: 1,
					Threshold:     101,
				},
			})
		})
		Convey("MergeThresholdMetExplanations", func() {
			input := []ThresholdExplanation{
				{
					Metric:        metrics.HumanClsFailedPresubmit.HumanReadableName,
					TimescaleDays: 7,
					Threshold:     20,
				},
				{
					Metric:        metrics.TestRunsFailed.HumanReadableName,
					TimescaleDays: 3,
					Threshold:     100,
				},
				{
					Metric:        metrics.HumanClsFailedPresubmit.HumanReadableName,
					TimescaleDays: 7,
					Threshold:     10,
				},
				{
					Metric:        metrics.TestRunsFailed.HumanReadableName,
					TimescaleDays: 3,
					Threshold:     200,
				},
				{
					Metric:        metrics.TestRunsFailed.HumanReadableName,
					TimescaleDays: 7,
					Threshold:     700,
				},
			}
			result := MergeThresholdMetExplanations(input)
			So(result, ShouldResemble, []ThresholdExplanation{
				{
					Metric:        metrics.HumanClsFailedPresubmit.HumanReadableName,
					TimescaleDays: 7,
					Threshold:     20,
				},
				{
					Metric:        metrics.TestRunsFailed.HumanReadableName,
					TimescaleDays: 3,
					Threshold:     200,
				},
				{
					Metric:        metrics.TestRunsFailed.HumanReadableName,
					TimescaleDays: 7,
					Threshold:     700,
				},
			})
		})
	})
	Convey("Zero value not inflated", t, func() {
		input := int64(0)
		output := inflateSingleThreshold(&input, 200)
		So(output, ShouldNotBeNil)
		So(*output, ShouldEqual, 0)
	})
	Convey("Non-zero value should not be inflated to zero", t, func() {
		input := int64(1)
		output := inflateSingleThreshold(&input, -200)
		So(output, ShouldNotBeNil)
		So(*output, ShouldNotEqual, 0)
	})
}

func setThresholdByID(metricID metrics.ID, t *configpb.MetricThreshold) []*configpb.ImpactMetricThreshold {
	return []*configpb.ImpactMetricThreshold{
		{MetricId: metricID.String(), Threshold: t},
	}
}
