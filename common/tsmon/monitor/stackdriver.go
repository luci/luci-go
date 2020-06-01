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
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	monitoring "cloud.google.com/go/monitoring/apiv3"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/types"

	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"
	gdistpb "google.golang.org/genproto/googleapis/api/distribution"
	metpb "google.golang.org/genproto/googleapis/api/metric"
	mpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

const (
	// chunks represent the number of chunks that can be handled in one go.
	// As per https://github.com/googleapis/go-genproto/blob/master/googleapis/monitoring/v3/metric_service.pb.go#L841
	// The max number of timeseries per call is 200.
	chunks       = 200
	customMetric = "custom.googleapis.com" + metricNamePrefix
	genTask      = "generic_task"
	nameSpace    = "chrome_infra"
)

type StackDriverMonitor struct {
	clt *monitoring.MetricClient

	project string
	region  string
}

func NewStackDriverMonitor(ctx context.Context, project_id, region string) (*StackDriverMonitor, error) {
	clt, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return nil, err
	}
	return &StackDriverMonitor{
		clt:     clt,
		project: project_id,
	}, nil
}

func (s *StackDriverMonitor) Send(ctx context.Context, cells []types.Cell) error {
	tss, err := s.cellToTimeSeries(ctx, clock.Now(ctx), cells)
	if err != nil {
		return err
	}
	return s.clt.CreateTimeSeries(ctx, &mpb.CreateTimeSeriesRequest{
		Name:       "projects/" + s.project,
		TimeSeries: tss,
	})
}

func (s *StackDriverMonitor) cellToTimeSeries(ctx context.Context, start time.Time, cells []types.Cell) ([]*mpb.TimeSeries, error) {
	var tss []*mpb.TimeSeries
	collections := map[uint64]*pb.MetricsCollection{}
	for _, c := range cells {
		// To not change things around too much , might as well populate the metric collection target
		// and take the information from there.
		targetHash := c.Target.Hash()
		collection, ok := collections[targetHash]
		if !ok {
			var collection pb.MetricsCollection
			collections[targetHash] = &collection
			c.Target.PopulateProto(&collection)
		}
		task := collection.GetTask()
		// tsc := collection.GetTargetSchema()
		// fmt.Println(tsc)
		point, pType, err := valueToPoint(c, start)
		if err != nil {
			return nil, err
		}
		tss = append(tss, &mpb.TimeSeries{
			Metric: &metpb.Metric{
				Type:   customMetric + c.Name,
				Labels: fieldsToLabels(c.Fields, c.FieldVals),
			},
			Resource: &monitoredres.MonitoredResource{
				Type: genTask,
				Labels: map[string]string{
					"project_id": s.project,
					"task_id":    strconv.Itoa(int(task.GetTaskNum())),
					"location":   s.region,
					"job":        task.GetJobName(),
					"namespace":  nameSpace,
				},
			},
			MetricKind: pType,
			Points: []*mpb.Point{
				point,
			},
		})
	}
	return tss, nil
}

func toSDTimestamp(t time.Time) *timestamp.Timestamp {
	return &timestamp.Timestamp{
		Seconds: t.Unix(),
	}
}

// ValueToPoint takes a cell and returns a Metric type and a point.
func valueToPoint(c types.Cell, now time.Time) (*mpb.Point, metric.MetricDescriptor_MetricKind, error) {
	var res mpb.Point
	mType := metric.MetricDescriptor_GAUGE
	// Default metric type is GAUGE which have start == end.
	start, end := now, now
	switch c.ValueType {
	case types.CumulativeIntType:
		start = c.ResetTime
		mType = metric.MetricDescriptor_CUMULATIVE
		fallthrough
	case types.NonCumulativeIntType:
		res.Value = &mpb.TypedValue{
			Value: &mpb.TypedValue_Int64Value{
				Int64Value: c.Value.(int64),
			},
		}
	case types.CumulativeFloatType:
		start = c.ResetTime
		mType = metric.MetricDescriptor_CUMULATIVE
		fallthrough
	case types.NonCumulativeFloatType:
		res.Value = &mpb.TypedValue{
			Value: &mpb.TypedValue_DoubleValue{
				DoubleValue: c.Value.(float64),
			},
		}
	case types.StringType:
		res.Value = &mpb.TypedValue{
			Value: &mpb.TypedValue_StringValue{
				StringValue: c.Value.(string),
			},
		}
	case types.BoolType:
		res.Value = &mpb.TypedValue{
			Value: &mpb.TypedValue_BoolValue{
				BoolValue: c.Value.(bool),
			},
		}
	case types.CumulativeDistributionType:
		start = c.ResetTime
		mType = metric.MetricDescriptor_CUMULATIVE
		fallthrough
	case types.NonCumulativeDistributionType:
		res.Value = &mpb.TypedValue{
			Value: &mpb.TypedValue_DistributionValue{
				DistributionValue: distToGoDist(c.Value.(*distribution.Distribution)),
			},
		}
	default:
		return nil, 0, status.Errorf(codes.InvalidArgument, "value: %v unknown", c.ValueType)
	}

	res.Interval = &mpb.TimeInterval{
		StartTime: toSDTimestamp(start),
		EndTime:   toSDTimestamp(end),
	}
	return &res, mType, nil
}

// distToGoDist converts between ts-mon Distribution snd Stackdriver distribution.
func distToGoDist(d *distribution.Distribution) *gdistpb.Distribution {
	ret := gdistpb.Distribution{
		BucketCounts: []int64(d.Buckets()),
	}
	if d.Count() > 0 {
		ret.Mean = d.Sum() / float64(d.Count())
	}

	if d.Bucketer().Width() == 0 {
		ret.BucketOptions = &gdistpb.Distribution_BucketOptions{
			Options: &gdistpb.Distribution_BucketOptions_ExponentialBuckets{
				ExponentialBuckets: &gdistpb.Distribution_BucketOptions_Exponential{
					NumFiniteBuckets: int32(d.Bucketer().NumFiniteBuckets()),
					GrowthFactor:     d.Bucketer().GrowthFactor(),
					Scale:            1.0,
				},
			},
		}
		return &ret
	}

	ret.BucketOptions = &gdistpb.Distribution_BucketOptions{
		Options: &gdistpb.Distribution_BucketOptions_LinearBuckets{
			LinearBuckets: &gdistpb.Distribution_BucketOptions_Linear{
				NumFiniteBuckets: int32(d.Bucketer().NumFiniteBuckets()),
				Width:            d.Bucketer().Width(),
				Offset:           0.0,
			},
		},
	}

	return &ret
}

func fieldsToLabels(fields []field.Field, fvs []interface{}) map[string]string {
	res := make(map[string]string)
	for i, f := range fields {
		// Not sure if letting fmt, do the reflecting is great.
		res[f.Name] = fmt.Sprintf("%v", fvs[i])
	}
	return res
}

func (s *StackDriverMonitor) ChunkSize() int {
	return chunks
}

func (s *StackDriverMonitor) Close() error {
	return s.Close()
}
