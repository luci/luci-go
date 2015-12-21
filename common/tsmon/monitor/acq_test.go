// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package monitor

import (
	"fmt"
	"math"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/tsmon/distribution"

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
