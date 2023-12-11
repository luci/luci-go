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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	configpb "go.chromium.org/luci/analysis/proto/config"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestThresholding(t *testing.T) {
	t.Parallel()

	Convey("With Cluster", t, func() {
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
		Convey("MeetsAnyOfThresholds", func() {
			Convey("No cluster meets empty threshold", func() {
				t := []*configpb.ImpactMetricThreshold{}
				So(cl.MeetsAnyOfThresholds(t), ShouldBeFalse)
			})
			Convey("Critical failures exonerated thresholding", func() {
				t := setThresholdByID(metrics.CriticalFailuresExonerated.ID, &configpb.MetricThreshold{OneDay: proto.Int64(60)})
				So(cl.MeetsAnyOfThresholds(t), ShouldBeTrue)

				t = setThresholdByID(metrics.CriticalFailuresExonerated.ID, &configpb.MetricThreshold{OneDay: proto.Int64(61)})
				So(cl.MeetsAnyOfThresholds(t), ShouldBeFalse)
			})
			Convey("Test results failed thresholding", func() {
				t := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{OneDay: proto.Int64(100)})
				So(cl.MeetsAnyOfThresholds(t), ShouldBeTrue)

				t = setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{OneDay: proto.Int64(101)})
				So(cl.MeetsAnyOfThresholds(t), ShouldBeFalse)
			})
			Convey("One day threshold", func() {
				t := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{OneDay: proto.Int64(100)})
				So(cl.MeetsAnyOfThresholds(t), ShouldBeTrue)

				t = setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{OneDay: proto.Int64(101)})
				So(cl.MeetsAnyOfThresholds(t), ShouldBeFalse)
			})
			Convey("Three day threshold", func() {
				t := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{ThreeDay: proto.Int64(300)})
				So(cl.MeetsAnyOfThresholds(t), ShouldBeTrue)

				t = setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{ThreeDay: proto.Int64(301)})
				So(cl.MeetsAnyOfThresholds(t), ShouldBeFalse)
			})
			Convey("Seven day threshold", func() {
				t := setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{SevenDay: proto.Int64(700)})
				So(cl.MeetsAnyOfThresholds(t), ShouldBeTrue)

				t = setThresholdByID(metrics.Failures.ID, &configpb.MetricThreshold{SevenDay: proto.Int64(701)})
				So(cl.MeetsAnyOfThresholds(t), ShouldBeFalse)
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
