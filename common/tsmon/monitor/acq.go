// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package monitor

import (
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"

	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto"
)

const (
	metricNamePrefix = "/chrome/infra"
)

func serializeCells(cells []types.Cell, t target.Target) *pb.MetricsCollection {
	collection := pb.MetricsCollection{
		Data: make([]*pb.MetricsData, len(cells)),
	}

	for i, cell := range cells {
		collection.Data[i] = serializeCell(cell, t)
	}

	return &collection
}

func serializeCell(c types.Cell, t target.Target) *pb.MetricsData {
	d := pb.MetricsData{}
	d.Name = proto.String(c.MetricName)
	d.MetricNamePrefix = proto.String(metricNamePrefix)
	d.Fields = field.Serialize(c.Fields, c.FieldVals)
	d.StartTimestampUs = proto.Uint64(uint64(c.ResetTime.UnixNano() / int64(time.Microsecond)))
	t.PopulateProto(&d)

	switch c.ValueType {
	case types.NonCumulativeIntType:
		d.Gauge = proto.Int64(c.Value.(int64))
	case types.CumulativeIntType:
		d.Counter = proto.Int64(c.Value.(int64))
	case types.NonCumulativeFloatType:
		d.NoncumulativeDoubleValue = proto.Float64(c.Value.(float64))
	case types.CumulativeFloatType:
		d.CumulativeDoubleValue = proto.Float64(c.Value.(float64))
	case types.StringType:
		d.StringValue = proto.String(c.Value.(string))
	case types.BoolType:
		d.BooleanValue = proto.Bool(c.Value.(bool))
	}
	return &d
}
