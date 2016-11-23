// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.
package field

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"

	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto_v2"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSerializeDescriptor(t *testing.T) {
	data := []struct {
		fields []Field
		want   []*pb.MetricsDataSet_MetricFieldDescriptor
	}{
		{
			fields: []Field{String("foo")},
			want: []*pb.MetricsDataSet_MetricFieldDescriptor{
				{
					Name:      proto.String("foo"),
					FieldType: pb.MetricsDataSet_MetricFieldDescriptor_STRING.Enum(),
				},
			},
		},
		{
			fields: []Field{Int("foo")},
			want: []*pb.MetricsDataSet_MetricFieldDescriptor{
				{
					Name:      proto.String("foo"),
					FieldType: pb.MetricsDataSet_MetricFieldDescriptor_INT64.Enum(),
				},
			},
		},
		{
			fields: []Field{Bool("foo")},
			want: []*pb.MetricsDataSet_MetricFieldDescriptor{
				{
					Name:      proto.String("foo"),
					FieldType: pb.MetricsDataSet_MetricFieldDescriptor_BOOL.Enum(),
				},
			},
		},
	}

	for i, d := range data {
		Convey(fmt.Sprintf("%d. SerializeDescriptor(%v)", i, d.fields), t, func() {
			got := SerializeDescriptor(d.fields)
			So(got, ShouldResemble, d.want)
		})
	}
}

func TestSerialize(t *testing.T) {
	data := []struct {
		fields []Field
		values []interface{}
		want   []*pb.MetricsData_MetricField
	}{
		{
			fields: []Field{String("foo")},
			values: makeInterfaceSlice("v"),
			want: []*pb.MetricsData_MetricField{
				{
					Name:  proto.String("foo"),
					Value: &pb.MetricsData_MetricField_StringValue{"v"},
				},
			},
		},
		{
			fields: []Field{Int("foo")},
			values: makeInterfaceSlice(int64(123)),
			want: []*pb.MetricsData_MetricField{
				{
					Name:  proto.String("foo"),
					Value: &pb.MetricsData_MetricField_Int64Value{123},
				},
			},
		},
		{
			fields: []Field{Bool("foo")},
			values: makeInterfaceSlice(true),
			want: []*pb.MetricsData_MetricField{
				{
					Name:  proto.String("foo"),
					Value: &pb.MetricsData_MetricField_BoolValue{true},
				},
			},
		},
	}

	for i, d := range data {
		Convey(fmt.Sprintf("%d. Serialize(%v, %v)", i, d.fields, d.values), t, func() {
			got := Serialize(d.fields, d.values)
			So(got, ShouldResemble, d.want)
		})
	}
}
