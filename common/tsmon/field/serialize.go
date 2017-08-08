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

package field

import (
	"github.com/golang/protobuf/proto"

	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"
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
