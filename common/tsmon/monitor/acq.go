// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package monitor

import (
	"math"
	"time"

	"github.com/luci/luci-go/common/tsmon/distribution"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/types"

	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto"
)

const (
	metricNamePrefix = "/chrome/infra/"
)

// SerializeCells creates a MetricsCollection message from a slice of cells.
func SerializeCells(cells []types.Cell) *pb.MetricsCollection {
	collection := pb.MetricsCollection{
		Data: make([]*pb.MetricsData, len(cells)),
	}

	for i, cell := range cells {
		collection.Data[i] = SerializeCell(cell)
	}

	return &collection
}

// SerializeCell creates one MetricsData message from a cell.
func SerializeCell(c types.Cell) *pb.MetricsData {
	d := pb.MetricsData{}
	d.Name = c.Name
	d.Description = c.Description
	d.MetricNamePrefix = metricNamePrefix
	d.Fields = field.Serialize(c.Fields, c.FieldVals)
	d.StartTimestampUs = uint64(c.ResetTime.UnixNano() / int64(time.Microsecond))
	c.Target.PopulateProto(&d)

	SerializeValue(c.ValueType, c.Value, &d)
	return &d
}

// SerializeValue writes one metric's value into the MetricsData message.
func SerializeValue(typ types.ValueType, value interface{}, d *pb.MetricsData) {
	switch typ {
	case types.NonCumulativeIntType:
		d.Gauge = value.(int64)
	case types.CumulativeIntType:
		d.Counter = value.(int64)
	case types.NonCumulativeFloatType:
		d.NoncumulativeDoubleValue = value.(float64)
	case types.CumulativeFloatType:
		d.CumulativeDoubleValue = value.(float64)
	case types.StringType:
		d.StringValue = value.(string)
	case types.BoolType:
		d.BooleanValue = value.(bool)
	case types.CumulativeDistributionType:
		d.Distribution = serializeDistribution(value.(*distribution.Distribution))
		d.Distribution.IsCumulative = true
	case types.NonCumulativeDistributionType:
		d.Distribution = serializeDistribution(value.(*distribution.Distribution))
		d.Distribution.IsCumulative = false
	}
}

func runningZeroes(values []int64) []int64 {
	ret := []int64{}

	var count int64
	for _, v := range values {
		if v == 0 {
			count++
		} else {
			if count != 0 {
				ret = append(ret, -count)
				count = 0
			}
			ret = append(ret, v)
		}
	}
	return ret
}

func serializeDistribution(d *distribution.Distribution) *pb.PrecomputedDistribution {
	ret := pb.PrecomputedDistribution{}

	// Copy the bucketer params.
	if d.Bucketer().Width() == 0 {
		switch d.Bucketer().GrowthFactor() {
		case 2:
			ret.SpecType = pb.PrecomputedDistribution_CANONICAL_POWERS_OF_2
		case math.Pow(10, 0.2):
			ret.SpecType = pb.PrecomputedDistribution_CANONICAL_POWERS_OF_10_P_0_2
		case 10:
			ret.SpecType = pb.PrecomputedDistribution_CANONICAL_POWERS_OF_10
		}
	}

	if ret.SpecType == pb.PrecomputedDistribution_UNKNOWN {
		ret.SpecType = pb.PrecomputedDistribution_CUSTOM_PARAMETERIZED
		ret.Width = d.Bucketer().Width()
		ret.GrowthFactor = d.Bucketer().GrowthFactor()
		ret.NumBuckets = int32(d.Bucketer().NumFiniteBuckets())
	}

	// Copy the distribution bucket values.  Exclude the overflow buckets on each
	// end.
	if len(d.Buckets()) >= 1 {
		if len(d.Buckets()) == d.Bucketer().NumBuckets() {
			ret.Bucket = runningZeroes(d.Buckets()[1 : len(d.Buckets())-1])
		} else {
			ret.Bucket = runningZeroes(d.Buckets()[1:])
		}
	}

	// Add overflow buckets if present.
	if len(d.Buckets()) >= 1 {
		ret.Underflow = d.Buckets()[0]
	}
	if len(d.Buckets()) == d.Bucketer().NumBuckets() {
		ret.Overflow = d.Buckets()[d.Bucketer().NumBuckets()-1]
	}

	if d.Count() > 0 {
		ret.Mean = d.Sum() / float64(d.Count())
	}

	return &ret
}
