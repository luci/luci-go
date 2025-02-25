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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"
)

func init() {
	batchSizeMaxBytes = 50
	defaultBatchAgeMax = time.Hour // ~infinity, cut by size
	defaultMaxLiveSizeBytes = 200
}

func TestBundler(t *testing.T) {
	t.Parallel()

	ftt.Run("With bundler", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx, _, _ = tsmon.WithFakes(ctx)
		tsmon.GetState(ctx).SetStore(store.NewInMemory(&target.Task{}))

		counter := func(m types.Metric, fieldVals ...any) int64 {
			val, _ := tsmon.GetState(ctx).Store().Get(ctx, m, fieldVals).(int64)
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

		t.Run("Start+drain empty", func(t *ftt.Test) {
			b.Start(ctx, writer)
			b.Shutdown(ctx)
			assert.Loosely(t, wrote, should.HaveLength(0))
		})

		t.Run("Start+send+drain OK", func(t *ftt.Test) {
			b.Start(ctx, writer)
			b.Log(ctx, &durationpb.Duration{Seconds: 1})
			b.Log(ctx, &durationpb.Duration{Seconds: 2})
			b.Log(ctx, &timestamppb.Timestamp{Seconds: 1})
			b.Log(ctx, &timestamppb.Timestamp{Seconds: 2})
			b.Log(ctx, &timestamppb.Timestamp{Seconds: 3})
			b.Shutdown(ctx)

			assert.Loosely(t, wrote, should.Match(map[string]int{
				"projects/project/datasets/dataset/tables/durations/_default":  2,
				"projects/project/datasets/dataset/tables/timestamps/_default": 3,
			}))

			assert.Loosely(t, counter(metricSentCounter, "project.dataset.durations"), should.Equal(2))
			assert.Loosely(t, counter(metricSentCounter, "project.dataset.timestamps"), should.Equal(3))
		})

		t.Run("Drops rows on fatal errors", func(t *ftt.Test) {
			writer.Recv = func() (*storagepb.AppendRowsResponse, error) {
				return nil, status.Errorf(codes.InvalidArgument, "Bad")
			}

			b.Start(ctx, writer)
			b.Log(ctx, &durationpb.Duration{Seconds: 1})
			b.Log(ctx, &timestamppb.Timestamp{Seconds: 1})
			b.Shutdown(ctx)

			assert.Loosely(t, counter(metricSentCounter, "project.dataset.durations"), should.BeZero)
			assert.Loosely(t, counter(metricSentCounter, "project.dataset.timestamps"), should.BeZero)
			assert.Loosely(t, counter(metricDroppedCounter, "project.dataset.durations", "DISPATCHER"), should.Equal(1))
			assert.Loosely(t, counter(metricDroppedCounter, "project.dataset.timestamps", "DISPATCHER"), should.Equal(1))
			assert.Loosely(t, counter(metricErrorsCounter, "project.dataset.durations", "INVALID_ARGUMENT"), should.Equal(1))
			assert.Loosely(t, counter(metricErrorsCounter, "project.dataset.timestamps", "INVALID_ARGUMENT"), should.Equal(1))
		})

		t.Run("Batching and dropping excesses", func(t *ftt.Test) {
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
			assert.Loosely(t, <-batchLen, should.Equal(25))
			assert.Loosely(t, <-batchLen, should.Equal(16))

			// Quickly drop the rest by shutting down without waiting.
			ctx, cancel := context.WithCancel(ctx)
			cancel()
			b.Shutdown(ctx)

			assert.Loosely(t, counter(metricSentCounter, "project.dataset.durations"), should.Equal(25+16))
			assert.Loosely(t, counter(metricDroppedCounter, "project.dataset.durations", "DISPATCHER"), should.BeGreaterThan(0))
		})
	})
}
