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

	configpb "go.chromium.org/luci/analysis/proto/config"
)

func TestThresholding(t *testing.T) {
	t.Parallel()

	Convey("With Cluster", t, func() {
		cl := &ClusterImpact{
			CriticalFailuresExonerated: MetricImpact{
				OneDay:   60,
				ThreeDay: 180,
				SevenDay: 420,
			},
			TestResultsFailed: MetricImpact{
				OneDay:   100,
				ThreeDay: 300,
				SevenDay: 700,
			},
			TestRunsFailed: MetricImpact{
				OneDay:   30,
				ThreeDay: 90,
				SevenDay: 210,
			},
			PresubmitRunsFailed: MetricImpact{
				OneDay:   3,
				ThreeDay: 9,
				SevenDay: 21,
			},
		}
		Convey("MeetsThreshold", func() {
			t := &configpb.ImpactThreshold{}
			Convey("No cluster meets empty threshold", func() {
				So(cl.MeetsThreshold(t), ShouldBeFalse)
			})
			Convey("Critical failures exonerated thresholding", func() {
				t.CriticalFailuresExonerated = &configpb.MetricThreshold{OneDay: proto.Int64(60)}
				So(cl.MeetsThreshold(t), ShouldBeTrue)

				t.CriticalFailuresExonerated = &configpb.MetricThreshold{OneDay: proto.Int64(61)}
				So(cl.MeetsThreshold(t), ShouldBeFalse)
			})
			Convey("Test results failed thresholding", func() {
				t.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(100)}
				So(cl.MeetsThreshold(t), ShouldBeTrue)

				t.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(101)}
				So(cl.MeetsThreshold(t), ShouldBeFalse)
			})
			Convey("Test runs failed thresholding", func() {
				t.TestRunsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(30)}
				So(cl.MeetsThreshold(t), ShouldBeTrue)

				t.TestRunsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(31)}
				So(cl.MeetsThreshold(t), ShouldBeFalse)
			})
			Convey("Presubmit runs failed thresholding", func() {
				t.PresubmitRunsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(3)}
				So(cl.MeetsThreshold(t), ShouldBeTrue)

				t.PresubmitRunsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(4)}
				So(cl.MeetsThreshold(t), ShouldBeFalse)
			})
			Convey("One day threshold", func() {
				t.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(100)}
				So(cl.MeetsThreshold(t), ShouldBeTrue)

				t.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(101)}
				So(cl.MeetsThreshold(t), ShouldBeFalse)
			})
			Convey("Three day threshold", func() {
				t.TestResultsFailed = &configpb.MetricThreshold{ThreeDay: proto.Int64(300)}
				So(cl.MeetsThreshold(t), ShouldBeTrue)

				t.TestResultsFailed = &configpb.MetricThreshold{ThreeDay: proto.Int64(301)}
				So(cl.MeetsThreshold(t), ShouldBeFalse)
			})
			Convey("Seven day threshold", func() {
				t.TestResultsFailed = &configpb.MetricThreshold{SevenDay: proto.Int64(700)}
				So(cl.MeetsThreshold(t), ShouldBeTrue)

				t.TestResultsFailed = &configpb.MetricThreshold{SevenDay: proto.Int64(701)}
				So(cl.MeetsThreshold(t), ShouldBeFalse)
			})
		})
		Convey("InflateThreshold", func() {
			t := &configpb.ImpactThreshold{}
			Convey("Empty threshold", func() {
				result := InflateThreshold(t, 15)
				So(result, ShouldResembleProto, &configpb.ImpactThreshold{})
			})
			Convey("Critical test failures exonerated", func() {
				t.CriticalFailuresExonerated = &configpb.MetricThreshold{OneDay: proto.Int64(100)}
				result := InflateThreshold(t, 15)
				So(result, ShouldResembleProto, &configpb.ImpactThreshold{
					CriticalFailuresExonerated: &configpb.MetricThreshold{OneDay: proto.Int64(115)},
				})
			})
			Convey("Test results failed", func() {
				t.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(100)}
				result := InflateThreshold(t, 15)
				So(result, ShouldResembleProto, &configpb.ImpactThreshold{
					TestResultsFailed: &configpb.MetricThreshold{OneDay: proto.Int64(115)},
				})
			})
			Convey("Test runs failed", func() {
				t.TestRunsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(100)}
				result := InflateThreshold(t, 15)
				So(result, ShouldResembleProto, &configpb.ImpactThreshold{
					TestRunsFailed: &configpb.MetricThreshold{OneDay: proto.Int64(115)},
				})
			})
			Convey("Presubmit runs failed", func() {
				t.PresubmitRunsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(100)}
				result := InflateThreshold(t, 15)
				So(result, ShouldResembleProto, &configpb.ImpactThreshold{
					PresubmitRunsFailed: &configpb.MetricThreshold{OneDay: proto.Int64(115)},
				})
			})
			Convey("One day threshold", func() {
				t.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(100)}
				result := InflateThreshold(t, 15)
				So(result, ShouldResembleProto, &configpb.ImpactThreshold{
					TestResultsFailed: &configpb.MetricThreshold{OneDay: proto.Int64(115)},
				})
			})
			Convey("Three day threshold", func() {
				t.TestResultsFailed = &configpb.MetricThreshold{ThreeDay: proto.Int64(100)}
				result := InflateThreshold(t, 15)
				So(result, ShouldResembleProto, &configpb.ImpactThreshold{
					TestResultsFailed: &configpb.MetricThreshold{ThreeDay: proto.Int64(115)},
				})
			})
			Convey("Seven day threshold", func() {
				t.TestResultsFailed = &configpb.MetricThreshold{SevenDay: proto.Int64(100)}
				result := InflateThreshold(t, 15)
				So(result, ShouldResembleProto, &configpb.ImpactThreshold{
					TestResultsFailed: &configpb.MetricThreshold{SevenDay: proto.Int64(115)},
				})
			})
		})
		Convey("ExplainThresholdMet", func() {
			t := &configpb.ImpactThreshold{
				TestResultsFailed: &configpb.MetricThreshold{
					OneDay:   proto.Int64(101), // Not met.
					ThreeDay: proto.Int64(299), // Met.
					SevenDay: proto.Int64(699), // Met.
				},
			}
			explanation := cl.ExplainThresholdMet(t)
			So(explanation, ShouldResemble, ThresholdExplanation{
				Metric:        "Test Results Failed",
				TimescaleDays: 3,
				Threshold:     299,
			})
		})
		Convey("ExplainThresholdNotMet", func() {
			t := &configpb.ImpactThreshold{
				CriticalFailuresExonerated: &configpb.MetricThreshold{
					OneDay: proto.Int64(61), // Not met.
				},
				TestResultsFailed: &configpb.MetricThreshold{
					OneDay: proto.Int64(101), // Not met.
				},
				TestRunsFailed: &configpb.MetricThreshold{
					ThreeDay: proto.Int64(301), // Not met.
				},
				PresubmitRunsFailed: &configpb.MetricThreshold{
					SevenDay: proto.Int64(701), // Not met.
				},
			}
			explanation := ExplainThresholdNotMet(t)
			So(explanation, ShouldResemble, []ThresholdExplanation{
				{
					Metric:        "Presubmit-Blocking Failures Exonerated",
					TimescaleDays: 1,
					Threshold:     61,
				},
				{
					Metric:        "Presubmit Runs Failed",
					TimescaleDays: 7,
					Threshold:     701,
				},
				{
					Metric:        "Test Runs Failed",
					TimescaleDays: 3,
					Threshold:     301,
				},
				{
					Metric:        "Test Results Failed",
					TimescaleDays: 1,
					Threshold:     101,
				},
			})
		})
		Convey("MergeThresholdMetExplanations", func() {
			input := []ThresholdExplanation{
				{
					Metric:        "Presubmit Runs Failed",
					TimescaleDays: 7,
					Threshold:     20,
				},
				{
					Metric:        "Test Runs Failed",
					TimescaleDays: 3,
					Threshold:     100,
				},
				{
					Metric:        "Presubmit Runs Failed",
					TimescaleDays: 7,
					Threshold:     10,
				},
				{
					Metric:        "Test Runs Failed",
					TimescaleDays: 3,
					Threshold:     200,
				},
				{
					Metric:        "Test Runs Failed",
					TimescaleDays: 7,
					Threshold:     700,
				},
			}
			result := MergeThresholdMetExplanations(input)
			So(result, ShouldResemble, []ThresholdExplanation{
				{
					Metric:        "Presubmit Runs Failed",
					TimescaleDays: 7,
					Threshold:     20,
				},
				{
					Metric:        "Test Runs Failed",
					TimescaleDays: 3,
					Threshold:     200,
				},
				{
					Metric:        "Test Runs Failed",
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
