// Copyright 2016 The LUCI Authors.
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

package monitor

import (
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"
)

func TestSerializeDistribution(t *testing.T) {
	ftt.Run("Fixed width params", t, func(t *ftt.Test) {
		d := distribution.New(distribution.FixedWidthBucketer(10, 20))
		dpb := serializeDistribution(d)

		assert.Loosely(t, dpb, should.Resemble(&pb.MetricsData_Distribution{
			Count: proto.Int64(0),
			BucketOptions: &pb.MetricsData_Distribution_LinearBuckets{
				LinearBuckets: &pb.MetricsData_Distribution_LinearOptions{
					NumFiniteBuckets: proto.Int32(20),
					Width:            proto.Float64(10),
					Offset:           proto.Float64(0),
				},
			},
		}))
	})

	ftt.Run("Exponential buckets", t, func(t *ftt.Test) {
		d := distribution.New(distribution.GeometricBucketer(2, 20))
		d.Add(0)
		d.Add(4)
		d.Add(1024)
		d.Add(1536)
		d.Add(1048576)

		dpb := serializeDistribution(d)
		assert.Loosely(t, dpb, should.Resemble(&pb.MetricsData_Distribution{
			Count: proto.Int64(5),
			Mean:  proto.Float64(210228),
			BucketOptions: &pb.MetricsData_Distribution_ExponentialBuckets{
				ExponentialBuckets: &pb.MetricsData_Distribution_ExponentialOptions{
					NumFiniteBuckets: proto.Int32(20),
					GrowthFactor:     proto.Float64(2),
					Scale:            proto.Float64(1),
				},
			},
			BucketCount: []int64{1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 1},
		}))
	})

	ftt.Run("Linear buckets", t, func(t *ftt.Test) {
		d := distribution.New(distribution.FixedWidthBucketer(10, 2))
		d.Add(0)
		d.Add(1)
		d.Add(2)
		d.Add(20)

		dpb := serializeDistribution(d)
		assert.Loosely(t, dpb, should.Resemble(&pb.MetricsData_Distribution{
			Count: proto.Int64(4),
			Mean:  proto.Float64(5.75),
			BucketOptions: &pb.MetricsData_Distribution_LinearBuckets{
				LinearBuckets: &pb.MetricsData_Distribution_LinearOptions{
					NumFiniteBuckets: proto.Int32(2),
					Width:            proto.Float64(10),
					Offset:           proto.Float64(0),
				},
			},
			BucketCount: []int64{0, 3, 0, 1},
		}))
	})
}

func TestSerializeCell(t *testing.T) {
	now := time.Date(2001, 1, 2, 3, 4, 5, 6, time.UTC)
	reset := time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC)

	nowTS := timestamppb.New(now)
	resetTS := timestamppb.New(reset)

	emptyTaskRootLabels := []*pb.MetricsCollection_RootLabels{
		target.RootLabel("proxy_environment", "pa"),
		target.RootLabel("acquisition_name", "mon-chrome-infra"),
		target.RootLabel("proxy_zone", "atl"),
		target.RootLabel("service_name", ""),
		target.RootLabel("job_name", ""),
		target.RootLabel("data_center", ""),
		target.RootLabel("host_name", ""),
		target.RootLabel("task_num", int64(0)),
		target.RootLabel("is_tsmon", true),
	}

	ftt.Run("Int", t, func(t *ftt.Test) {
		ret := SerializeCells([]types.Cell{{
			types.MetricInfo{
				Name:        "foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.NonCumulativeIntType,
			},
			types.MetricMetadata{Units: types.Seconds},
			types.CellData{
				FieldVals: []any{},
				Target:    &target.Task{},
				ResetTime: reset,
				Value:     int64(42),
			},
		}}, now)
		assert.Loosely(t, ret, should.Resemble([]*pb.MetricsCollection{{
			RootLabels: emptyTaskRootLabels,
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
				Annotations: &pb.Annotations{
					Unit:      proto.String(string(types.Seconds)),
					Timestamp: proto.Bool(true),
				},
			}},
		}}))
	})

	ftt.Run("Counter", t, func(t *ftt.Test) {
		ret := SerializeCells([]types.Cell{{
			types.MetricInfo{
				// Metric name with "/" prefix will be used as it is.
				Name:        "/foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.CumulativeIntType,
			},
			types.MetricMetadata{Units: types.Bytes},
			types.CellData{
				FieldVals: []any{},
				Target:    &target.Task{},
				ResetTime: reset,
				Value:     int64(42),
			},
		}}, now)
		assert.Loosely(t, ret, should.Resemble([]*pb.MetricsCollection{{
			RootLabels: emptyTaskRootLabels,
			MetricsDataSet: []*pb.MetricsDataSet{{
				MetricName:      proto.String("/foo"),
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
				Annotations: &pb.Annotations{
					Unit:      proto.String(string(types.Bytes)),
					Timestamp: proto.Bool(false),
				},
			}},
		}}))
	})

	ftt.Run("Float", t, func(t *ftt.Test) {
		ret := SerializeCells([]types.Cell{{
			types.MetricInfo{
				Name:        "foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.NonCumulativeFloatType,
			},
			types.MetricMetadata{},
			types.CellData{
				FieldVals: []any{},
				Target:    &target.Task{},
				ResetTime: reset,
				Value:     float64(42),
			},
		}}, now)
		assert.Loosely(t, ret, should.Resemble([]*pb.MetricsCollection{{
			RootLabels: emptyTaskRootLabels,
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
		}}))
	})

	ftt.Run("FloatCounter", t, func(t *ftt.Test) {
		ret := SerializeCells([]types.Cell{{
			types.MetricInfo{
				Name:        "foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.CumulativeFloatType,
			},
			types.MetricMetadata{},
			types.CellData{
				FieldVals: []any{},
				Target:    &target.Task{},
				ResetTime: reset,
				Value:     float64(42),
			},
		}}, now)
		assert.Loosely(t, ret, should.Resemble([]*pb.MetricsCollection{{
			RootLabels: emptyTaskRootLabels,
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
		}}))
	})

	ftt.Run("String", t, func(t *ftt.Test) {
		ret := SerializeCells([]types.Cell{{
			types.MetricInfo{
				Name:        "foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.StringType,
			},
			types.MetricMetadata{},
			types.CellData{
				FieldVals: []any{},
				Target:    &target.Task{},
				ResetTime: reset,
				Value:     "hello",
			},
		}}, now)
		assert.Loosely(t, ret, should.Resemble([]*pb.MetricsCollection{{
			RootLabels: emptyTaskRootLabels,
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
		}}))
	})

	ftt.Run("Boolean", t, func(t *ftt.Test) {
		ret := SerializeCells([]types.Cell{{
			types.MetricInfo{
				Name:        "foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.BoolType,
			},
			types.MetricMetadata{},
			types.CellData{
				FieldVals: []any{},
				Target:    &target.Task{},
				ResetTime: reset,
				Value:     true,
			},
		}}, now)
		assert.Loosely(t, ret, should.Resemble([]*pb.MetricsCollection{{
			RootLabels: emptyTaskRootLabels,
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
		}}))
	})

	ftt.Run("NonDefaultTarget", t, func(t *ftt.Test) {
		taskTarget := target.Task{
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
				FieldVals: []any{},
				Target:    &taskTarget,
				ResetTime: reset,
				Value:     int64(42),
			},
		}}, now)
		assert.Loosely(t, ret, should.Resemble([]*pb.MetricsCollection{{
			RootLabels: []*pb.MetricsCollection_RootLabels{
				target.RootLabel("proxy_environment", "pa"),
				target.RootLabel("acquisition_name", "mon-chrome-infra"),
				target.RootLabel("proxy_zone", "atl"),
				target.RootLabel("service_name", "hello"),
				target.RootLabel("job_name", "world"),
				target.RootLabel("data_center", ""),
				target.RootLabel("host_name", ""),
				target.RootLabel("task_num", int64(0)),
				target.RootLabel("is_tsmon", true),
			},
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
		}}))
	})

	ftt.Run("CumulativeDistribution", t, func(t *ftt.Test) {
		d := distribution.New(distribution.FixedWidthBucketer(10, 20))
		d.Add(1024)
		ret := SerializeCells([]types.Cell{{
			MetricInfo: types.MetricInfo{
				Name:        "foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.CumulativeDistributionType,
			},
			MetricMetadata: types.MetricMetadata{
				Units: types.Seconds,
			},
			CellData: types.CellData{
				FieldVals: []any{},
				Target:    &target.Task{},
				ResetTime: reset,
				Value:     d,
			},
		}}, now)
		assert.Loosely(t, ret, should.Resemble([]*pb.MetricsCollection{{
			RootLabels: emptyTaskRootLabels,
			MetricsDataSet: []*pb.MetricsDataSet{{
				MetricName:      proto.String("/chrome/infra/foo"),
				FieldDescriptor: []*pb.MetricsDataSet_MetricFieldDescriptor{},
				StreamKind:      pb.StreamKind_CUMULATIVE.Enum(),
				ValueType:       pb.ValueType_DISTRIBUTION.Enum(),
				Description:     proto.String("bar"),
				Annotations: &pb.Annotations{
					Unit: proto.String(string(types.Seconds)),
					// serializeCells() should have set timestamp with true,
					// although it is types.Seconds.
					Timestamp: proto.Bool(false),
				},
				Data: []*pb.MetricsData{{
					Value: &pb.MetricsData_DistributionValue{
						DistributionValue: &pb.MetricsData_Distribution{
							Count: proto.Int64(1),
							Mean:  proto.Float64(1024),
							BucketOptions: &pb.MetricsData_Distribution_LinearBuckets{
								LinearBuckets: &pb.MetricsData_Distribution_LinearOptions{
									NumFiniteBuckets: proto.Int32(20),
									Width:            proto.Float64(10),
									Offset:           proto.Float64(0),
								},
							},
							BucketCount: []int64{
								0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
								0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
								0, 1,
							},
						},
					},
					Field:          []*pb.MetricsData_MetricField{},
					StartTimestamp: resetTS,
					EndTimestamp:   nowTS,
				}},
			}},
		}}))
	})

	ftt.Run("NonCumulativeDistribution", t, func(t *ftt.Test) {
		d := distribution.New(distribution.FixedWidthBucketer(10, 20))
		d.Add(1024)
		ret := SerializeCells([]types.Cell{{
			MetricInfo: types.MetricInfo{
				Name:        "foo",
				Description: "bar",
				Fields:      []field.Field{},
				ValueType:   types.NonCumulativeDistributionType,
			},
			MetricMetadata: types.MetricMetadata{
				Units: types.Seconds,
			},
			CellData: types.CellData{
				FieldVals: []any{},
				Target:    &target.Task{},
				ResetTime: reset,
				Value:     d,
			},
		}}, now)
		assert.Loosely(t, ret, should.Resemble([]*pb.MetricsCollection{{
			RootLabels: emptyTaskRootLabels,
			MetricsDataSet: []*pb.MetricsDataSet{{
				MetricName:      proto.String("/chrome/infra/foo"),
				FieldDescriptor: []*pb.MetricsDataSet_MetricFieldDescriptor{},
				StreamKind:      pb.StreamKind_GAUGE.Enum(),
				ValueType:       pb.ValueType_DISTRIBUTION.Enum(),
				Description:     proto.String("bar"),
				Annotations: &pb.Annotations{
					Unit: proto.String(string(types.Seconds)),
					// serializeCells() should have set timestamp with true,
					// although it is types.Seconds.
					Timestamp: proto.Bool(false),
				},
				Data: []*pb.MetricsData{{
					Value: &pb.MetricsData_DistributionValue{
						DistributionValue: &pb.MetricsData_Distribution{
							Count: proto.Int64(1),
							Mean:  proto.Float64(1024),
							BucketOptions: &pb.MetricsData_Distribution_LinearBuckets{
								LinearBuckets: &pb.MetricsData_Distribution_LinearOptions{
									NumFiniteBuckets: proto.Int32(20),
									Width:            proto.Float64(10),
									Offset:           proto.Float64(0),
								},
							},
							BucketCount: []int64{
								0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
								0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
								0, 1,
							},
						},
					},
					Field:          []*pb.MetricsData_MetricField{},
					StartTimestamp: nowTS,
					EndTimestamp:   nowTS,
				}},
			}},
		}}))
	})
}
