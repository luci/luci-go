// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package monitor

import (
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/tsmon/distribution"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"

	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto_v2"
	. "github.com/smartystreets/goconvey/convey"
)

func TestSerializeDistribution(t *testing.T) {
	Convey("Fixed width params", t, func() {
		d := distribution.New(distribution.FixedWidthBucketer(10, 20))
		dpb := serializeDistribution(d)

		So(*dpb, ShouldResemble, pb.MetricsData_Distribution{
			Count: proto.Int64(0),
			BucketOptions: &pb.MetricsData_Distribution_LinearBuckets{
				LinearBuckets: &pb.MetricsData_Distribution_LinearOptions{
					NumFiniteBuckets: proto.Int32(20),
					Width:            proto.Float64(10),
					Offset:           proto.Float64(0),
				},
			},
			BucketCount: []int64{},
		})
	})

	Convey("Geometric params", t, func() {
		d := distribution.New(distribution.GeometricBucketer(2, 20))
		dpb := serializeDistribution(d)

		So(*dpb, ShouldResemble, pb.MetricsData_Distribution{
			Count: proto.Int64(0),
			BucketOptions: &pb.MetricsData_Distribution_ExponentialBuckets{
				ExponentialBuckets: &pb.MetricsData_Distribution_ExponentialOptions{
					NumFiniteBuckets: proto.Int32(20),
					GrowthFactor:     proto.Float64(2),
					Scale:            proto.Float64(1),
				},
			},
			BucketCount: []int64{},
		})
	})

	Convey("Populates buckets", t, func() {
		d := distribution.New(distribution.FixedWidthBucketer(10, 2))
		d.Add(0)
		d.Add(1)
		d.Add(2)
		d.Add(20)

		dpb := serializeDistribution(d)
		So(*dpb, ShouldResemble, pb.MetricsData_Distribution{
			Count: proto.Int64(4),
			Mean:  proto.Float64(5.75),
			BucketOptions: &pb.MetricsData_Distribution_LinearBuckets{
				LinearBuckets: &pb.MetricsData_Distribution_LinearOptions{
					NumFiniteBuckets: proto.Int32(2),
					Width:            proto.Float64(10),
					Offset:           proto.Float64(0),
				},
			},
			BucketCount: []int64{-1, 3, -1, 1},
		})
	})
}

func TestSerializeCell(t *testing.T) {
	now := time.Date(2001, 1, 2, 3, 4, 5, 6, time.UTC)
	reset := time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC)

	nowTS := &pb.Timestamp{Seconds: proto.Int64(978404645), Nanos: proto.Int32(6)}
	resetTS := &pb.Timestamp{Seconds: proto.Int64(946782245), Nanos: proto.Int32(6)}

	emptyTask := &pb.MetricsCollection_Task{&pb.Task{
		ServiceName: proto.String(""),
		JobName:     proto.String(""),
		DataCenter:  proto.String(""),
		HostName:    proto.String(""),
		TaskNum:     proto.Int(0),
	}}

	Convey("Int", t, func() {
		ret := SerializeCells([]types.Cell{{
			types.MetricInfo{
				Name:        "foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.NonCumulativeIntType,
			},
			types.MetricMetadata{},
			types.CellData{
				FieldVals: []interface{}{},
				Target:    &target.Task{},
				ResetTime: reset,
				Value:     int64(42),
			},
		}}, now)
		So(ret, ShouldResemble, []*pb.MetricsCollection{{
			TargetSchema: emptyTask,
			MetricsDataSet: []*pb.MetricsDataSet{{
				MetricName:      proto.String("/chrome/infra/foo"),
				FieldDescriptor: []*pb.MetricsDataSet_MetricFieldDescriptor{},
				StreamKind:      pb.StreamKind_GAUGE.Enum(),
				ValueType:       pb.ValueType_INT64.Enum(),
				Description:     proto.String("bar"),
				Data: []*pb.MetricsData{{
					Value:          &pb.MetricsData_Int64Value{42},
					Field:          []*pb.MetricsData_MetricField{},
					StartTimestamp: nowTS,
					EndTimestamp:   nowTS,
				}},
			}},
		}})
	})

	Convey("Counter", t, func() {
		ret := SerializeCells([]types.Cell{{
			types.MetricInfo{
				Name:        "foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.CumulativeIntType,
			},
			types.MetricMetadata{},
			types.CellData{
				FieldVals: []interface{}{},
				Target:    &target.Task{},
				ResetTime: reset,
				Value:     int64(42),
			},
		}}, now)
		So(ret, ShouldResemble, []*pb.MetricsCollection{{
			TargetSchema: emptyTask,
			MetricsDataSet: []*pb.MetricsDataSet{{
				MetricName:      proto.String("/chrome/infra/foo"),
				FieldDescriptor: []*pb.MetricsDataSet_MetricFieldDescriptor{},
				StreamKind:      pb.StreamKind_CUMULATIVE.Enum(),
				ValueType:       pb.ValueType_INT64.Enum(),
				Description:     proto.String("bar"),
				Data: []*pb.MetricsData{{
					Value:          &pb.MetricsData_Int64Value{42},
					Field:          []*pb.MetricsData_MetricField{},
					StartTimestamp: resetTS,
					EndTimestamp:   nowTS,
				}},
			}},
		}})
	})

	Convey("Float", t, func() {
		ret := SerializeCells([]types.Cell{{
			types.MetricInfo{
				Name:        "foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.NonCumulativeFloatType,
			},
			types.MetricMetadata{},
			types.CellData{
				FieldVals: []interface{}{},
				Target:    &target.Task{},
				ResetTime: reset,
				Value:     float64(42),
			},
		}}, now)
		So(ret, ShouldResemble, []*pb.MetricsCollection{{
			TargetSchema: emptyTask,
			MetricsDataSet: []*pb.MetricsDataSet{{
				MetricName:      proto.String("/chrome/infra/foo"),
				FieldDescriptor: []*pb.MetricsDataSet_MetricFieldDescriptor{},
				StreamKind:      pb.StreamKind_GAUGE.Enum(),
				ValueType:       pb.ValueType_DOUBLE.Enum(),
				Description:     proto.String("bar"),
				Data: []*pb.MetricsData{{
					Value:          &pb.MetricsData_DoubleValue{42},
					Field:          []*pb.MetricsData_MetricField{},
					StartTimestamp: nowTS,
					EndTimestamp:   nowTS,
				}},
			}},
		}})
	})

	Convey("FloatCounter", t, func() {
		ret := SerializeCells([]types.Cell{{
			types.MetricInfo{
				Name:        "foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.CumulativeFloatType,
			},
			types.MetricMetadata{},
			types.CellData{
				FieldVals: []interface{}{},
				Target:    &target.Task{},
				ResetTime: reset,
				Value:     float64(42),
			},
		}}, now)
		So(ret, ShouldResemble, []*pb.MetricsCollection{{
			TargetSchema: emptyTask,
			MetricsDataSet: []*pb.MetricsDataSet{{
				MetricName:      proto.String("/chrome/infra/foo"),
				FieldDescriptor: []*pb.MetricsDataSet_MetricFieldDescriptor{},
				StreamKind:      pb.StreamKind_CUMULATIVE.Enum(),
				ValueType:       pb.ValueType_DOUBLE.Enum(),
				Description:     proto.String("bar"),
				Data: []*pb.MetricsData{{
					Value:          &pb.MetricsData_DoubleValue{42},
					Field:          []*pb.MetricsData_MetricField{},
					StartTimestamp: resetTS,
					EndTimestamp:   nowTS,
				}},
			}},
		}})
	})

	Convey("String", t, func() {
		ret := SerializeCells([]types.Cell{{
			types.MetricInfo{
				Name:        "foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.StringType,
			},
			types.MetricMetadata{},
			types.CellData{
				FieldVals: []interface{}{},
				Target:    &target.Task{},
				ResetTime: reset,
				Value:     "hello",
			},
		}}, now)
		So(ret, ShouldResemble, []*pb.MetricsCollection{{
			TargetSchema: emptyTask,
			MetricsDataSet: []*pb.MetricsDataSet{{
				MetricName:      proto.String("/chrome/infra/foo"),
				FieldDescriptor: []*pb.MetricsDataSet_MetricFieldDescriptor{},
				StreamKind:      pb.StreamKind_GAUGE.Enum(),
				ValueType:       pb.ValueType_STRING.Enum(),
				Description:     proto.String("bar"),
				Data: []*pb.MetricsData{{
					Value:          &pb.MetricsData_StringValue{"hello"},
					Field:          []*pb.MetricsData_MetricField{},
					StartTimestamp: nowTS,
					EndTimestamp:   nowTS,
				}},
			}},
		}})
	})

	Convey("Boolean", t, func() {
		ret := SerializeCells([]types.Cell{{
			types.MetricInfo{
				Name:        "foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.BoolType,
			},
			types.MetricMetadata{},
			types.CellData{
				FieldVals: []interface{}{},
				Target:    &target.Task{},
				ResetTime: reset,
				Value:     true,
			},
		}}, now)
		So(ret, ShouldResemble, []*pb.MetricsCollection{{
			TargetSchema: emptyTask,
			MetricsDataSet: []*pb.MetricsDataSet{{
				MetricName:      proto.String("/chrome/infra/foo"),
				FieldDescriptor: []*pb.MetricsDataSet_MetricFieldDescriptor{},
				StreamKind:      pb.StreamKind_GAUGE.Enum(),
				ValueType:       pb.ValueType_BOOL.Enum(),
				Description:     proto.String("bar"),
				Data: []*pb.MetricsData{{
					Value:          &pb.MetricsData_BoolValue{true},
					Field:          []*pb.MetricsData_MetricField{},
					StartTimestamp: nowTS,
					EndTimestamp:   nowTS,
				}},
			}},
		}})
	})

	Convey("NonDefaultTarget", t, func() {
		target := target.Task{
			ServiceName: "hello",
			JobName:     "world",
		}

		ret := SerializeCells([]types.Cell{{
			types.MetricInfo{
				Name:        "foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.NonCumulativeIntType,
			},
			types.MetricMetadata{},
			types.CellData{
				FieldVals: []interface{}{},
				Target:    &target,
				ResetTime: reset,
				Value:     int64(42),
			},
		}}, now)
		So(ret, ShouldResemble, []*pb.MetricsCollection{{
			TargetSchema: &pb.MetricsCollection_Task{&pb.Task{
				ServiceName: proto.String("hello"),
				JobName:     proto.String("world"),
				DataCenter:  proto.String(""),
				HostName:    proto.String(""),
				TaskNum:     proto.Int(0),
			}},
			MetricsDataSet: []*pb.MetricsDataSet{{
				MetricName:      proto.String("/chrome/infra/foo"),
				FieldDescriptor: []*pb.MetricsDataSet_MetricFieldDescriptor{},
				StreamKind:      pb.StreamKind_GAUGE.Enum(),
				ValueType:       pb.ValueType_INT64.Enum(),
				Description:     proto.String("bar"),
				Data: []*pb.MetricsData{{
					Value:          &pb.MetricsData_Int64Value{42},
					Field:          []*pb.MetricsData_MetricField{},
					StartTimestamp: nowTS,
					EndTimestamp:   nowTS,
				}},
			}},
		}})
	})
}
