// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.
package field

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"

	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto_v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSerializeV1(t *testing.T) {
	data := []struct {
		fields []Field
		values []interface{}
		want   []*pb.MetricsField
	}{
		{
			fields: []Field{String("foo")},
			values: makeInterfaceSlice("v"),
			want: []*pb.MetricsField{
				{
					Name:        proto.String("foo"),
					Type:        pb.MetricsField_STRING.Enum(),
					StringValue: proto.String("v"),
				},
			},
		},
		{
			fields: []Field{Int("foo")},
			values: makeInterfaceSlice(int64(123)),
			want: []*pb.MetricsField{
				{
					Name:     proto.String("foo"),
					Type:     pb.MetricsField_INT.Enum(),
					IntValue: proto.Int64(123),
				},
			},
		},
		{
			fields: []Field{Bool("foo")},
			values: makeInterfaceSlice(true),
			want: []*pb.MetricsField{
				{
					Name:      proto.String("foo"),
					Type:      pb.MetricsField_BOOL.Enum(),
					BoolValue: proto.Bool(true),
				},
			},
		},
	}

	for i, d := range data {
		Convey(fmt.Sprintf("%d. SerializeV1(%v, %v)", i, d.fields, d.values), t, func() {
			got := SerializeV1(d.fields, d.values)
			So(got, ShouldResemble, d.want)
		})
	}
}
