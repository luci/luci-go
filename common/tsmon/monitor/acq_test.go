// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package monitor

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/tsmon/distribution"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"

	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRunningZeroes(t *testing.T) {
	data := []struct {
		values []int64
		want   []int64
	}{
		{[]int64{1, 0, 1}, []int64{1, -1, 1}},
		{[]int64{1, 0, 0, 1}, []int64{1, -2, 1}},
		{[]int64{1, 0, 0, 0, 1}, []int64{1, -3, 1}},
		{[]int64{1, 0, 1, 0, 2}, []int64{1, -1, 1, -1, 2}},
		{[]int64{1, 0, 1, 0, 0, 2}, []int64{1, -1, 1, -2, 2}},
		{[]int64{1, 0, 0, 1, 0, 0, 2}, []int64{1, -2, 1, -2, 2}},

		// Leading zeroes
		{[]int64{0, 1}, []int64{-1, 1}},
		{[]int64{0, 0, 1}, []int64{-2, 1}},
		{[]int64{0, 0, 0, 1}, []int64{-3, 1}},

		// Trailing zeroes
		{[]int64{1}, []int64{1}},
		{[]int64{1, 0}, []int64{1}},
		{[]int64{1, 0, 0}, []int64{1}},
		{[]int64{}, []int64{}},
		{[]int64{0}, []int64{}},
		{[]int64{0, 0}, []int64{}},
	}

	for i, d := range data {
		Convey(fmt.Sprintf("%d. runningZeroes(%v)", i, d.values), t, func() {
			got := runningZeroes(d.values)
			So(got, ShouldResemble, d.want)
		})
	}
}

func TestSerializeDistribution(t *testing.T) {
	Convey("Fixed width params", t, func() {
		d := distribution.New(distribution.FixedWidthBucketer(10, 20))
		dpb := serializeDistribution(d)

		So(*dpb, ShouldResemble, pb.PrecomputedDistribution{
			SpecType:     pb.PrecomputedDistribution_CUSTOM_PARAMETERIZED.Enum(),
			Width:        proto.Float64(10),
			GrowthFactor: proto.Float64(0),
			NumBuckets:   proto.Int32(20),
		})
	})

	Convey("Growth factor 2 params", t, func() {
		d := distribution.New(distribution.GeometricBucketer(2, 20))
		dpb := serializeDistribution(d)

		So(*dpb, ShouldResemble, pb.PrecomputedDistribution{
			SpecType: pb.PrecomputedDistribution_CANONICAL_POWERS_OF_2.Enum(),
		})
	})

	Convey("Growth factor 10^0.2 params", t, func() {
		d := distribution.New(distribution.GeometricBucketer(math.Pow(10, 0.2), 20))
		dpb := serializeDistribution(d)

		So(*dpb, ShouldResemble, pb.PrecomputedDistribution{
			SpecType: pb.PrecomputedDistribution_CANONICAL_POWERS_OF_10_P_0_2.Enum(),
		})
	})

	Convey("Growth factor 10 params", t, func() {
		d := distribution.New(distribution.GeometricBucketer(10, 20))
		dpb := serializeDistribution(d)

		So(*dpb, ShouldResemble, pb.PrecomputedDistribution{
			SpecType: pb.PrecomputedDistribution_CANONICAL_POWERS_OF_10.Enum(),
		})
	})

	Convey("Custom geometric params", t, func() {
		d := distribution.New(distribution.GeometricBucketer(4, 20))
		dpb := serializeDistribution(d)

		So(*dpb, ShouldResemble, pb.PrecomputedDistribution{
			SpecType:     pb.PrecomputedDistribution_CUSTOM_PARAMETERIZED.Enum(),
			Width:        proto.Float64(0),
			GrowthFactor: proto.Float64(4),
			NumBuckets:   proto.Int32(20),
		})
	})

	Convey("Populates buckets", t, func() {
		d := distribution.New(distribution.FixedWidthBucketer(10, 2))
		d.Add(0)
		d.Add(1)
		d.Add(2)
		d.Add(20)

		dpb := serializeDistribution(d)
		So(*dpb, ShouldResemble, pb.PrecomputedDistribution{
			SpecType:     pb.PrecomputedDistribution_CUSTOM_PARAMETERIZED.Enum(),
			Width:        proto.Float64(10),
			GrowthFactor: proto.Float64(0),
			NumBuckets:   proto.Int32(2),

			Bucket:    []int64{3},
			Underflow: proto.Int64(0),
			Overflow:  proto.Int64(1),
			Mean:      proto.Float64(5.75),
		})
	})
}

func TestSerializeCell(t *testing.T) {
	defaultTarget := &target.Task{}

	Convey("Int", t, func() {
		ret := serializeCell(types.Cell{
			types.MetricInfo{
				MetricName: "foo",
				Fields:     []field.Field{},
				ValueType:  types.NonCumulativeIntType,
			},
			types.CellData{
				FieldVals: []interface{}{},
				Target:    nil,
				ResetTime: time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC),
				Value:     int64(42),
			},
		}, defaultTarget)
		So(ret, ShouldResemble, &pb.MetricsData{
			Name:             proto.String("foo"),
			MetricNamePrefix: proto.String("/chrome/infra/"),
			Fields:           []*pb.MetricsField{},
			StartTimestampUs: proto.Uint64(946782245000000),
			Task:             &pb.Task{},
			Gauge:            proto.Int64(42),
		})
	})

	Convey("Counter", t, func() {
		ret := serializeCell(types.Cell{
			types.MetricInfo{
				MetricName: "foo",
				Fields:     []field.Field{},
				ValueType:  types.CumulativeIntType,
			},
			types.CellData{
				FieldVals: []interface{}{},
				Target:    nil,
				ResetTime: time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC),
				Value:     int64(42),
			},
		}, defaultTarget)
		So(ret, ShouldResemble, &pb.MetricsData{
			Name:             proto.String("foo"),
			MetricNamePrefix: proto.String("/chrome/infra/"),
			Fields:           []*pb.MetricsField{},
			StartTimestampUs: proto.Uint64(946782245000000),
			Task:             &pb.Task{},
			Counter:          proto.Int64(42),
		})
	})

	Convey("Float", t, func() {
		ret := serializeCell(types.Cell{
			types.MetricInfo{
				MetricName: "foo",
				Fields:     []field.Field{},
				ValueType:  types.NonCumulativeFloatType,
			},
			types.CellData{
				FieldVals: []interface{}{},
				Target:    nil,
				ResetTime: time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC),
				Value:     float64(42),
			},
		}, defaultTarget)
		So(ret, ShouldResemble, &pb.MetricsData{
			Name:             proto.String("foo"),
			MetricNamePrefix: proto.String("/chrome/infra/"),
			Fields:           []*pb.MetricsField{},
			StartTimestampUs: proto.Uint64(946782245000000),
			Task:             &pb.Task{},
			NoncumulativeDoubleValue: proto.Float64(42),
		})
	})

	Convey("FloatCounter", t, func() {
		ret := serializeCell(types.Cell{
			types.MetricInfo{
				MetricName: "foo",
				Fields:     []field.Field{},
				ValueType:  types.CumulativeFloatType,
			},
			types.CellData{
				FieldVals: []interface{}{},
				Target:    nil,
				ResetTime: time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC),
				Value:     float64(42),
			},
		}, defaultTarget)
		So(ret, ShouldResemble, &pb.MetricsData{
			Name:             proto.String("foo"),
			MetricNamePrefix: proto.String("/chrome/infra/"),
			Fields:           []*pb.MetricsField{},
			StartTimestampUs: proto.Uint64(946782245000000),
			Task:             &pb.Task{},
			CumulativeDoubleValue: proto.Float64(42),
		})
	})

	Convey("String", t, func() {
		ret := serializeCell(types.Cell{
			types.MetricInfo{
				MetricName: "foo",
				Fields:     []field.Field{},
				ValueType:  types.StringType,
			},
			types.CellData{
				FieldVals: []interface{}{},
				Target:    nil,
				ResetTime: time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC),
				Value:     "hello",
			},
		}, defaultTarget)
		So(ret, ShouldResemble, &pb.MetricsData{
			Name:             proto.String("foo"),
			MetricNamePrefix: proto.String("/chrome/infra/"),
			Fields:           []*pb.MetricsField{},
			StartTimestampUs: proto.Uint64(946782245000000),
			Task:             &pb.Task{},
			StringValue:      proto.String("hello"),
		})
	})

	Convey("Boolean", t, func() {
		ret := serializeCell(types.Cell{
			types.MetricInfo{
				MetricName: "foo",
				Fields:     []field.Field{},
				ValueType:  types.BoolType,
			},
			types.CellData{
				FieldVals: []interface{}{},
				Target:    nil,
				ResetTime: time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC),
				Value:     true,
			},
		}, defaultTarget)
		So(ret, ShouldResemble, &pb.MetricsData{
			Name:             proto.String("foo"),
			MetricNamePrefix: proto.String("/chrome/infra/"),
			Fields:           []*pb.MetricsField{},
			StartTimestampUs: proto.Uint64(946782245000000),
			Task:             &pb.Task{},
			BooleanValue:     proto.Bool(true),
		})
	})

	Convey("NonDefaultTarget", t, func() {
		target := target.Task{
			ServiceName: proto.String("hello"),
			JobName:     proto.String("world"),
		}

		ret := serializeCell(types.Cell{
			types.MetricInfo{
				MetricName: "foo",
				Fields:     []field.Field{},
				ValueType:  types.NonCumulativeIntType,
			},
			types.CellData{
				FieldVals: []interface{}{},
				Target:    &target,
				ResetTime: time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC),
				Value:     int64(42),
			},
		}, defaultTarget)
		So(ret, ShouldResemble, &pb.MetricsData{
			Name:             proto.String("foo"),
			MetricNamePrefix: proto.String("/chrome/infra/"),
			Fields:           []*pb.MetricsField{},
			StartTimestampUs: proto.Uint64(946782245000000),
			Task: &pb.Task{
				ServiceName: proto.String("hello"),
				JobName:     proto.String("world"),
			},
			Gauge: proto.Int64(42),
		})
	})
}
