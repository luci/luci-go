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
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/target"
	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"
	"go.chromium.org/luci/common/tsmon/types"
)

type dataSetKey struct {
	targetHash uint64
	metricName string
}

// SerializeCells creates a MetricsCollection message from a slice of cells.
func SerializeCells(cells []types.Cell, now time.Time) []*pb.MetricsCollection {
	collections := map[uint64]*pb.MetricsCollection{}
	dataSets := map[dataSetKey]*pb.MetricsDataSet{}

	// TODO(1026140): the hash and proto of a Target object should be created
	// at the time of the object creation to avoid unnecessary invocation of
	// Target.Hash() and Target.PopulateProto()
	for _, c := range cells {
		// Find the collection, add it if it doesn't exist.
		targetHash := c.Target.Hash()
		collection, ok := collections[targetHash]
		if !ok {
			collection = &pb.MetricsCollection{}
			collections[targetHash] = collection
			c.Target.PopulateProto(collection)

			// add is_tsmon to indicate that this target is a tsmon schema.
			collection.RootLabels = append(collection.RootLabels, target.RootLabel("is_tsmon", true))
		}

		// Find the data set, add it if it doesn't exist.
		key := dataSetKey{targetHash, c.Name}
		dataSet, ok := dataSets[key]
		if !ok {
			dataSet = SerializeDataSet(c)
			dataSets[key] = dataSet
			collection.MetricsDataSet = append(collection.MetricsDataSet, dataSet)
		}

		// Add the data to the data set.
		dataSet.Data = append(dataSet.Data, SerializeValue(c, now))
	}

	// Turn the hash into a list and return it.
	ret := make([]*pb.MetricsCollection, 0, len(collections))
	for _, collection := range collections {
		ret = append(ret, collection)
	}
	return ret
}

func buildMetricName(name string) string {
	if strings.HasPrefix(name, "/") {
		return name
	}
	return fmt.Sprintf("%s%s", MetricNamePrefix, name)
}

// SerializeDataSet creates a new MetricsDataSet without any data, but just with
// the metric metadata fields populated.
func SerializeDataSet(c types.Cell) *pb.MetricsDataSet {
	d := pb.MetricsDataSet{}
	d.MetricName = proto.String(buildMetricName(c.Name))
	d.FieldDescriptor = field.SerializeDescriptor(c.Fields)
	d.Description = proto.String(c.Description)

	if c.ValueType.IsCumulative() {
		d.StreamKind = pb.StreamKind_CUMULATIVE.Enum()
	} else {
		d.StreamKind = pb.StreamKind_GAUGE.Enum()
	}

	switch c.ValueType {
	case types.NonCumulativeIntType, types.CumulativeIntType:
		d.ValueType = pb.ValueType_INT64.Enum()
	case types.NonCumulativeFloatType, types.CumulativeFloatType:
		d.ValueType = pb.ValueType_DOUBLE.Enum()
	case types.NonCumulativeDistributionType, types.CumulativeDistributionType:
		d.ValueType = pb.ValueType_DISTRIBUTION.Enum()
	case types.StringType:
		d.ValueType = pb.ValueType_STRING.Enum()
	case types.BoolType:
		d.ValueType = pb.ValueType_BOOL.Enum()
	}

	if c.Units.IsSpecified() {
		d.Annotations = &pb.Annotations{
			Unit: proto.String(string(c.Units)),
			// Annotation.Timestamp can be true only if ValueType == Int or
			// Float types. Distribution is a collection of Float values, and is
			// often used to track the durations of certain events, such as
			// RPC duration.
			//
			// However, distribution itself is not a time value, and, therefore,
			// Annotation.Timestamp must not be True in any metrics with
			// ValueType_DISTRIBUTION.
			//
			// This sets isTimeUnit with False for distribution metrics, so that
			// the monitoring UI will display the time unit string beside
			// the Y-Axis for distribution metrics. It won't be able to adjust
			// the time scale, though.
			Timestamp: proto.Bool(
				*d.ValueType != pb.ValueType_DISTRIBUTION && c.Units.IsTime(),
			),
		}
	}
	return &d
}

// SerializeValue creates a new MetricsData representing this cell's value.
func SerializeValue(c types.Cell, now time.Time) *pb.MetricsData {
	d := pb.MetricsData{}
	d.Field = field.Serialize(c.Fields, c.FieldVals)

	if c.ValueType.IsCumulative() {
		d.StartTimestamp = timestamppb.New(c.ResetTime)
	} else {
		d.StartTimestamp = timestamppb.New(now)
	}
	d.EndTimestamp = timestamppb.New(now)

	switch c.ValueType {
	case types.NonCumulativeIntType, types.CumulativeIntType:
		d.Value = &pb.MetricsData_Int64Value{c.Value.(int64)}
	case types.NonCumulativeFloatType, types.CumulativeFloatType:
		d.Value = &pb.MetricsData_DoubleValue{c.Value.(float64)}
	case types.CumulativeDistributionType, types.NonCumulativeDistributionType:
		d.Value = &pb.MetricsData_DistributionValue{serializeDistribution(c.Value.(*distribution.Distribution))}
	case types.StringType:
		d.Value = &pb.MetricsData_StringValue{c.Value.(string)}
	case types.BoolType:
		d.Value = &pb.MetricsData_BoolValue{c.Value.(bool)}
	}
	return &d
}

func serializeDistribution(d *distribution.Distribution) *pb.MetricsData_Distribution {
	ret := pb.MetricsData_Distribution{
		Count: proto.Int64(d.Count()),
	}

	if d.Count() > 0 {
		ret.Mean = proto.Float64(d.Sum() / float64(d.Count()))
	}

	// Copy the bucketer params.
	if d.Bucketer().Width() == 0 {
		ret.BucketOptions = &pb.MetricsData_Distribution_ExponentialBuckets{
			&pb.MetricsData_Distribution_ExponentialOptions{
				NumFiniteBuckets: proto.Int32(int32(d.Bucketer().NumFiniteBuckets())),
				GrowthFactor:     proto.Float64(d.Bucketer().GrowthFactor()),
				Scale:            proto.Float64(d.Bucketer().Scale()),
			},
		}
	} else {
		ret.BucketOptions = &pb.MetricsData_Distribution_LinearBuckets{
			&pb.MetricsData_Distribution_LinearOptions{
				NumFiniteBuckets: proto.Int32(int32(d.Bucketer().NumFiniteBuckets())),
				Width:            proto.Float64(d.Bucketer().Width()),
				Offset:           proto.Float64(0.0),
			},
		}
	}

	// Copy the distribution bucket values.  Include the overflow buckets on
	// either end.
	ret.BucketCount = d.Buckets()

	return &ret
}
