// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package field

import (
	"github.com/golang/protobuf/proto"

	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto_v1"
)

// SerializeV1 returns a slice of ts_mon_proto.MetricsField messages
// representing the field names, types and values.
func SerializeV1(fields []Field, values []interface{}) []*pb.MetricsField {
	ret := make([]*pb.MetricsField, len(fields))

	for i, f := range fields {
		d := &pb.MetricsField{
			Name: proto.String(f.Name),
		}

		switch f.Type {
		case StringType:
			d.Type = pb.MetricsField_STRING.Enum()
			d.StringValue = proto.String(values[i].(string))
		case BoolType:
			d.Type = pb.MetricsField_BOOL.Enum()
			d.BoolValue = proto.Bool(values[i].(bool))
		case IntType:
			d.Type = pb.MetricsField_INT.Enum()
			d.IntValue = proto.Int64(values[i].(int64))
		}

		ret[i] = d
	}

	return ret
}
