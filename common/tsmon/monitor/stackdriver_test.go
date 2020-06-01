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
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"

	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/genproto/googleapis/api/monitoredres"

	gdistpb "google.golang.org/genproto/googleapis/api/distribution"
	metpb "google.golang.org/genproto/googleapis/api/metric"
	mpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

func TestCellToTimeSeries(t *testing.T) {
	now := time.Date(2001, 1, 2, 3, 4, 5, 6, time.UTC)
	reset := time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC)

	popDist := distribution.New(distribution.FixedWidthBucketer(10, 2))
	popDist.Add(0)
	popDist.Add(1)
	popDist.Add(2)
	popDist.Add(20)

	tests := []struct {
		name      string
		projectID string
		start     time.Time
		fail      bool
		in        []types.Cell
		want      []*mpb.TimeSeries
	}{
		{
			name:      "Empty",
			projectID: "EmptyProject",
			start:     now,
		}, {
			name:      "Single cell",
			projectID: "SingleCell",
			start:     now,
			in: []types.Cell{
				{
					types.MetricInfo{
						Name:        "foo",
						Description: "bar",
						Fields:      []field.Field{},
						ValueType:   types.CumulativeIntType,
					},
					types.MetricMetadata{},
					types.CellData{
						FieldVals: []interface{}{},
						Target:    &target.Task{},
						ResetTime: reset,
						Value:     int64(42),
					},
				},
			},
			want: []*mpb.TimeSeries{
				{
					Metric: &metpb.Metric{
						Type:   customMetric + "foo",
						Labels: make(map[string]string),
					},
					MetricKind: metric.MetricDescriptor_CUMULATIVE,
					Resource: &monitoredres.MonitoredResource{
						Type: genTask,
						Labels: map[string]string{
							"project_id": "SingleCell",
							"task_id":    "0",
							"location":   "",
							"job":        "",
							"namespace":  "chrome_infra",
						},
					},
					Points: []*mpb.Point{
						{
							Interval: &mpb.TimeInterval{
								StartTime: toSDTimestamp(reset),
								EndTime:   toSDTimestamp(now),
							},
							Value: &mpb.TypedValue{
								Value: &mpb.TypedValue_Int64Value{
									Int64Value: 42,
								},
							},
						},
					},
				},
			},
		}, {
			name:      "Cumalative Float",
			projectID: "SingleCell",
			start:     now,
			in: []types.Cell{
				{
					types.MetricInfo{
						Name:        "foo",
						Description: "bar",
						Fields:      []field.Field{},
						ValueType:   types.CumulativeFloatType,
					},
					types.MetricMetadata{},
					types.CellData{
						FieldVals: []interface{}{},
						Target:    &target.Task{},
						ResetTime: reset,
						Value:     float64(42),
					},
				},
			},
			want: []*mpb.TimeSeries{
				{
					Metric: &metpb.Metric{
						Type:   customMetric + "foo",
						Labels: make(map[string]string),
					},
					MetricKind: metric.MetricDescriptor_CUMULATIVE,
					Resource: &monitoredres.MonitoredResource{
						Type: genTask,
						Labels: map[string]string{
							"project_id": "SingleCell",
							"task_id":    "0",
							"location":   "",
							"job":        "",
							"namespace":  "chrome_infra",
						},
					},
					Points: []*mpb.Point{
						{
							Interval: &mpb.TimeInterval{
								StartTime: toSDTimestamp(reset),
								EndTime:   toSDTimestamp(now),
							},
							Value: &mpb.TypedValue{
								Value: &mpb.TypedValue_DoubleValue{
									DoubleValue: 42,
								},
							},
						},
					},
				},
			},
		}, {
			name:      "String",
			projectID: "SingleCell",
			start:     now,
			in: []types.Cell{
				{
					types.MetricInfo{
						Name:        "foo",
						Description: "bar",
						Fields:      []field.Field{},
						ValueType:   types.StringType,
					},
					types.MetricMetadata{},
					types.CellData{
						FieldVals: []interface{}{},
						Target:    &target.Task{},
						ResetTime: reset,
						Value:     "hello",
					},
				},
			},
			want: []*mpb.TimeSeries{
				{
					Metric: &metpb.Metric{
						Type:   customMetric + "foo",
						Labels: make(map[string]string),
					},
					MetricKind: metric.MetricDescriptor_GAUGE,
					Resource: &monitoredres.MonitoredResource{
						Type: genTask,
						Labels: map[string]string{
							"project_id": "SingleCell",
							"task_id":    "0",
							"location":   "",
							"job":        "",
							"namespace":  "chrome_infra",
						},
					},
					Points: []*mpb.Point{
						{
							Interval: &mpb.TimeInterval{
								StartTime: toSDTimestamp(now),
								EndTime:   toSDTimestamp(now),
							},
							Value: &mpb.TypedValue{
								Value: &mpb.TypedValue_StringValue{
									StringValue: "hello",
								},
							},
						},
					},
				},
			},
		}, {
			name:      "Bool",
			projectID: "SingleCell",
			start:     now,
			in: []types.Cell{
				{
					types.MetricInfo{
						Name:        "foo",
						Description: "bar",
						Fields:      []field.Field{},
						ValueType:   types.BoolType,
					},
					types.MetricMetadata{},
					types.CellData{
						FieldVals: []interface{}{},
						Target:    &target.Task{},
						ResetTime: reset,
						Value:     true,
					},
				},
			},
			want: []*mpb.TimeSeries{
				{
					Metric: &metpb.Metric{
						Type:   customMetric + "foo",
						Labels: make(map[string]string),
					},
					MetricKind: metric.MetricDescriptor_GAUGE,
					Resource: &monitoredres.MonitoredResource{
						Type: genTask,
						Labels: map[string]string{
							"project_id": "SingleCell",
							"task_id":    "0",
							"location":   "",
							"job":        "",
							"namespace":  "chrome_infra",
						},
					},
					Points: []*mpb.Point{
						{
							Interval: &mpb.TimeInterval{
								StartTime: toSDTimestamp(now),
								EndTime:   toSDTimestamp(now),
							},
							Value: &mpb.TypedValue{
								Value: &mpb.TypedValue_BoolValue{
									BoolValue: true,
								},
							},
						},
					},
				},
			},
		}, {
			name:      "Distribution Linear",
			projectID: "SingleCell",
			start:     now,
			in: []types.Cell{
				{
					types.MetricInfo{
						Name:        "foo",
						Description: "bar",
						Fields:      []field.Field{},
						ValueType:   types.NonCumulativeDistributionType,
					},
					types.MetricMetadata{},
					types.CellData{
						FieldVals: []interface{}{},
						Target:    &target.Task{},
						ResetTime: reset,
						Value:     distribution.New(distribution.FixedWidthBucketer(10, 20)),
					},
				},
			},
			want: []*mpb.TimeSeries{
				{
					Metric: &metpb.Metric{
						Type:   customMetric + "foo",
						Labels: make(map[string]string),
					},
					MetricKind: metric.MetricDescriptor_GAUGE,
					Resource: &monitoredres.MonitoredResource{
						Type: genTask,
						Labels: map[string]string{
							"project_id": "SingleCell",
							"task_id":    "0",
							"location":   "",
							"job":        "",
							"namespace":  "chrome_infra",
						},
					},
					Points: []*mpb.Point{
						{
							Interval: &mpb.TimeInterval{
								StartTime: toSDTimestamp(now),
								EndTime:   toSDTimestamp(now),
							},
							Value: &mpb.TypedValue{
								Value: &mpb.TypedValue_DistributionValue{
									DistributionValue: &gdistpb.Distribution{
										BucketOptions: &gdistpb.Distribution_BucketOptions{
											Options: &gdistpb.Distribution_BucketOptions_LinearBuckets{
												LinearBuckets: &gdistpb.Distribution_BucketOptions_Linear{
													NumFiniteBuckets: 20,
													Width:            10,
													Offset:           0.0,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, {
			name:      "Distribution Exponential Culumative",
			projectID: "SingleCell",
			start:     now,
			in: []types.Cell{
				{
					types.MetricInfo{
						Name:        "foo",
						Description: "bar",
						Fields:      []field.Field{},
						ValueType:   types.CumulativeDistributionType,
					},
					types.MetricMetadata{},
					types.CellData{
						FieldVals: []interface{}{},
						Target:    &target.Task{},
						ResetTime: reset,
						Value:     distribution.New(distribution.GeometricBucketer(2, 20)),
					},
				},
			},
			want: []*mpb.TimeSeries{
				{
					Metric: &metpb.Metric{
						Type:   customMetric + "foo",
						Labels: make(map[string]string),
					},
					MetricKind: metric.MetricDescriptor_CUMULATIVE,
					Resource: &monitoredres.MonitoredResource{
						Type: genTask,
						Labels: map[string]string{
							"project_id": "SingleCell",
							"task_id":    "0",
							"location":   "",
							"job":        "",
							"namespace":  "chrome_infra",
						},
					},
					Points: []*mpb.Point{
						{
							Interval: &mpb.TimeInterval{
								StartTime: toSDTimestamp(reset),
								EndTime:   toSDTimestamp(now),
							},
							Value: &mpb.TypedValue{
								Value: &mpb.TypedValue_DistributionValue{
									DistributionValue: &gdistpb.Distribution{
										BucketOptions: &gdistpb.Distribution_BucketOptions{
											Options: &gdistpb.Distribution_BucketOptions_ExponentialBuckets{
												ExponentialBuckets: &gdistpb.Distribution_BucketOptions_Exponential{
													NumFiniteBuckets: 20,
													GrowthFactor:     2,
													Scale:            1.0,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, {
			name:      "Distribution Linear Populated",
			projectID: "SingleCell",
			start:     now,
			in: []types.Cell{
				{
					types.MetricInfo{
						Name:        "foo",
						Description: "bar",
						Fields:      []field.Field{},
						ValueType:   types.NonCumulativeDistributionType,
					},
					types.MetricMetadata{},
					types.CellData{
						FieldVals: []interface{}{},
						Target:    &target.Task{},
						ResetTime: reset,
						Value:     popDist,
					},
				},
			},
			want: []*mpb.TimeSeries{
				{
					Metric: &metpb.Metric{
						Type:   customMetric + "foo",
						Labels: make(map[string]string),
					},
					MetricKind: metric.MetricDescriptor_GAUGE,
					Resource: &monitoredres.MonitoredResource{
						Type: genTask,
						Labels: map[string]string{
							"project_id": "SingleCell",
							"task_id":    "0",
							"location":   "",
							"job":        "",
							"namespace":  "chrome_infra",
						},
					},
					Points: []*mpb.Point{
						{
							Interval: &mpb.TimeInterval{
								StartTime: toSDTimestamp(now),
								EndTime:   toSDTimestamp(now),
							},
							Value: &mpb.TypedValue{
								Value: &mpb.TypedValue_DistributionValue{
									DistributionValue: &gdistpb.Distribution{
										BucketCounts: []int64{0, 3, 0, 1},
										Mean:         5.75,
										BucketOptions: &gdistpb.Distribution_BucketOptions{
											Options: &gdistpb.Distribution_BucketOptions_LinearBuckets{
												LinearBuckets: &gdistpb.Distribution_BucketOptions_Linear{
													NumFiniteBuckets: 2,
													Width:            10,
													Offset:           0.0,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	var sdMon StackDriverMonitor
	ctx := context.Background()

	for _, tst := range tests {
		sdMon.project = tst.projectID
		res, err := sdMon.cellToTimeSeries(ctx, tst.start, tst.in)
		if got, want := (err != nil), tst.fail; got != want {
			t.Errorf("%s: cellToTimeSeries(ctx, %v , _) = %t want: %t, err: %v", tst.name, tst.start, got, want, err)
			continue
		}
		if diff := pretty.Compare(tst.want, res); diff != "" {
			t.Errorf("%s: cellToTimeSeries(ctx, %v, _) differ -want +got\n%s", tst.name, tst.start, diff)
		}
	}
}
