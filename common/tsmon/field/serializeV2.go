// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package field

import (
	"github.com/golang/protobuf/proto"

	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto_v2"
)

// SerializeDescriptor returns a slice of field descriptors, representing just
// the names and types of fields.
func SerializeDescriptor(fields []Field) []*pb.MetricsDataSet_MetricFieldDescriptor {
	ret := make([]*pb.MetricsDataSet_MetricFieldDescriptor, len(fields))

	for i, f := range fields {
		d := &pb.MetricsDataSet_MetricFieldDescriptor{
			Name: proto.String(f.Name),
		}

		switch f.Type {
		case StringType:
			d.FieldType = pb.MetricsDataSet_MetricFieldDescriptor_STRING.Enum()
		case BoolType:
			d.FieldType = pb.MetricsDataSet_MetricFieldDescriptor_BOOL.Enum()
		case IntType:
			d.FieldType = pb.MetricsDataSet_MetricFieldDescriptor_INT64.Enum()
		}

		ret[i] = d
	}

	return ret
}

// Serialize returns a slice of ts_mon_proto.MetricsData.MetricsField messages
// representing the field names and values.
func Serialize(fields []Field, values []interface{}) []*pb.MetricsData_MetricField {
	ret := make([]*pb.MetricsData_MetricField, len(fields))

	for i, f := range fields {
		d := &pb.MetricsData_MetricField{
			Name: proto.String(f.Name),
		}

		switch f.Type {
		case StringType:
			d.Value = &pb.MetricsData_MetricField_StringValue{values[i].(string)}
		case BoolType:
			d.Value = &pb.MetricsData_MetricField_BoolValue{values[i].(bool)}
		case IntType:
			d.Value = &pb.MetricsData_MetricField_Int64Value{values[i].(int64)}
		}

		ret[i] = d
	}

	return ret
}
