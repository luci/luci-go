// Copyright 2021 The LUCI Authors.
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

package bqlog

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"

	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	batchSizeMaxBytes = 50
	defaultBatchAgeMax = time.Hour // ~infinity, cut by size
	defaultMaxLiveSizeBytes = 200
}

func TestBundler(t *testing.T) {
	t.Parallel()

	Convey("With bundler", t, func() {
		ctx := context.Background()
		ctx, _, _ = tsmon.WithFakes(ctx)
		tsmon.GetState(ctx).SetStore(store.NewInMemory(&target.Task{}))

		counter := func(m types.Metric, fieldVals ...any) int64 {
			val, _ := tsmon.GetState(ctx).Store().Get(ctx, m, time.Time{}, fieldVals).(int64)
			return val
		}

		var m sync.Mutex
		wrote := map[string]int{}

		writer := &FakeBigQueryWriter{
			Send: func(r *storagepb.AppendRowsRequest) error {
				m.Lock()
				defer m.Unlock()
				wrote[r.WriteStream] += len(r.GetProtoRows().Rows.SerializedRows)
				return nil
			},
		}

		b := Bundler{
			CloudProject: "project",
			Dataset:      "dataset",
		}
		b.RegisterSink(Sink{
			Prototype: &durationpb.Duration{},
			Table:     "durations",
		})
		b.RegisterSink(Sink{
			Prototype: &timestamppb.Timestamp{},
			Table:     "timestamps",
		})

		Convey("Start+drain empty", func() {
			b.Start(ctx, writer)
			b.Shutdown(ctx)
			So(wrote, ShouldHaveLength, 0)
		})

		Convey("Start+send+drain OK", func() {
			b.Start(ctx, writer)
			b.Log(ctx, &durationpb.Duration{Seconds: 1})
			b.Log(ctx, &durationpb.Duration{Seconds: 2})
			b.Log(ctx, &timestamppb.Timestamp{Seconds: 1})
			b.Log(ctx, &timestamppb.Timestamp{Seconds: 2})
			b.Log(ctx, &timestamppb.Timestamp{Seconds: 3})
			b.Shutdown(ctx)

			So(wrote, ShouldResemble, map[string]int{
				"projects/project/datasets/dataset/tables/durations/_default":  2,
				"projects/project/datasets/dataset/tables/timestamps/_default": 3,
			})

			So(counter(metricSentCounter, "project.dataset.durations"), ShouldEqual, 2)
			So(counter(metricSentCounter, "project.dataset.timestamps"), ShouldEqual, 3)
		})

		Convey("Drops rows on fatal errors", func() {
			writer.Recv = func() (*storagepb.AppendRowsResponse, error) {
				return nil, status.Errorf(codes.InvalidArgument, "Bad")
			}

			b.Start(ctx, writer)
			b.Log(ctx, &durationpb.Duration{Seconds: 1})
			b.Log(ctx, &timestamppb.Timestamp{Seconds: 1})
			b.Shutdown(ctx)

			So(counter(metricSentCounter, "project.dataset.durations"), ShouldEqual, 0)
			So(counter(metricSentCounter, "project.dataset.timestamps"), ShouldEqual, 0)
			So(counter(metricDroppedCounter, "project.dataset.durations", "DISPATCHER"), ShouldEqual, 1)
			So(counter(metricDroppedCounter, "project.dataset.timestamps", "DISPATCHER"), ShouldEqual, 1)
			So(counter(metricErrorsCounter, "project.dataset.durations", "INVALID_ARGUMENT"), ShouldEqual, 1)
			So(counter(metricErrorsCounter, "project.dataset.timestamps", "INVALID_ARGUMENT"), ShouldEqual, 1)
		})

		Convey("Batching and dropping excesses", func() {
			countdown := int64(2)
			batchLen := make(chan int)

			writer.Send = func(r *storagepb.AppendRowsRequest) error {
				if atomic.AddInt64(&countdown, -1) >= 0 {
					batchLen <- len(r.GetProtoRows().Rows.SerializedRows)
					return nil
				}
				return status.Errorf(codes.Internal, "Closed")
			}

			b.Start(ctx, writer)
			for i := 0; i < 1000; i++ {
				b.Log(ctx, &durationpb.Duration{Seconds: int64(i)})
			}

			// Make sure we get small batches.
			So(<-batchLen, ShouldEqual, 25)
			So(<-batchLen, ShouldEqual, 16)

			// Quickly drop the rest by shutting down without waiting.
			ctx, cancel := context.WithCancel(ctx)
			cancel()
			b.Shutdown(ctx)

			So(counter(metricSentCounter, "project.dataset.durations"), ShouldEqual, 25+16)
			So(counter(metricDroppedCounter, "project.dataset.durations", "DISPATCHER"), ShouldBeGreaterThan, 0)
		})
	})
}
