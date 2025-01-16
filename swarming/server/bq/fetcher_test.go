// Copyright 2024 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bq

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	bqpb "go.chromium.org/luci/swarming/proto/bq"
	"go.chromium.org/luci/swarming/server/model"
)

func TestFetcher(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		var testTime = time.Date(2024, time.March, 3, 4, 5, 6, 0, time.UTC)
		const queryBatchSize = 3
		const flushThreshold = 7

		ctx := memory.Use(context.Background())

		// Create a bunch of test entities to export.
		type entity struct {
			ID int64 `gae:"$id"`
			TS time.Time
		}
		for i := 1; i < 20; i++ {
			assert.NoErr(t, datastore.Put(ctx, &entity{
				ID: int64(i),
				TS: testTime.Add(time.Duration(i) * time.Second),
			}))
		}
		datastore.GetTestable(ctx).CatchupIndexes()

		var convertErr error
		var convertCalls []int

		fetcher := &Fetcher[entity, *wrapperspb.Int64Value]{
			entityKind:     "entity",
			timestampField: "TS",
			queryBatchSize: queryBatchSize,
			// Converts entity IDs to Int64Value protos.
			convert: func(_ context.Context, ents []*entity) ([]*wrapperspb.Int64Value, error) {
				convertCalls = append(convertCalls, len(ents))
				if convertErr != nil {
					return nil, convertErr
				}
				out := make([]*wrapperspb.Int64Value, len(ents))
				for i, ent := range ents {
					out[i] = &wrapperspb.Int64Value{Value: ent.ID}
				}
				return out, nil
			},
		}

		var flushErr func(batchSize int) error
		var flushCalls []int
		var collected []int64

		// Takes Int64Value protos and appends values in them to `collected`.
		flusher := &Flusher{
			CountThreshold: 1000, // unlimited
			ByteThreshold:  flushThreshold,
			Marshal:        proto.Marshal,
			Flush: func(rows [][]byte) error {
				total := 0
				for _, row := range rows {
					total += len(row)
				}
				flushCalls = append(flushCalls, total)
				if flushErr != nil {
					if err := flushErr(total); err != nil {
						return err
					}
				}
				for _, row := range rows {
					var wrapper wrapperspb.Int64Value
					if err := proto.Unmarshal(row, &wrapper); err != nil {
						panic(err)
					}
					collected = append(collected, wrapper.Value)
				}
				return nil
			},
		}

		t.Run("Visits correct time range", func(t *ftt.Test) {
			err := fetcher.Fetch(ctx, testTime.Add(3*time.Second), 10*time.Second, []*Flusher{flusher})
			assert.NoErr(t, err)
			assert.Loosely(t, collected, should.Resemble([]int64{3, 4, 5, 6, 7, 8, 9, 10, 11, 12}))

			// Respects queryBatchSize (== 3 entities).
			assert.Loosely(t, convertCalls, should.Resemble([]int{3, 3, 3, 1}))

			// Respects flushThreshold (== 7 bytes). Each message is 2 bytes long. It
			// takes 4 messages (== 8 bytes) to trigger the flush. The last flush
			// sends the remaining 2 messages.
			assert.Loosely(t, flushCalls, should.Resemble([]int{8, 8, 4}))
		})

		t.Run("Convert error", func(t *ftt.Test) {
			wantErr := errors.New("BOOM")
			convertErr = wantErr
			gotErr := fetcher.Fetch(ctx, testTime.Add(3*time.Second), 10*time.Second, []*Flusher{flusher})
			assert.Loosely(t, gotErr, should.Equal(wantErr))
			assert.Loosely(t, collected, should.BeEmpty)
		})

		t.Run("Flush error", func(t *ftt.Test) {
			wantErr := errors.New("BOOM")
			flushErr = func(int) error { return wantErr }
			gotErr := fetcher.Fetch(ctx, testTime.Add(3*time.Second), 10*time.Second, []*Flusher{flusher})
			assert.Loosely(t, gotErr, should.Equal(wantErr))
			assert.Loosely(t, collected, should.BeEmpty)
		})

		t.Run("Final flush error", func(t *ftt.Test) {
			wantErr := errors.New("BOOM")
			flushErr = func(size int) error {
				if size == 4 {
					return wantErr
				}
				return nil
			}
			gotErr := fetcher.Fetch(ctx, testTime.Add(3*time.Second), 10*time.Second, []*Flusher{flusher})
			assert.Loosely(t, gotErr, should.Equal(wantErr))
			assert.Loosely(t, collected, should.Resemble([]int64{3, 4, 5, 6, 7, 8, 9, 10})) // doesn't have the last 2
		})
	})
}

func TestConvertTaskResults(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		// TaskRunResult that has both TaskRequest and PerformanceStats.
		req1, _ := model.TaskIDToRequestKey(ctx, "692b33d4feb00010")
		assert.NoErr(t, datastore.Put(ctx, &model.TaskRequest{
			Key:  req1,
			Name: "req-1",
		}))
		assert.NoErr(t, datastore.Put(ctx, &model.PerformanceStats{
			Key:             model.PerformanceStatsKey(ctx, req1),
			BotOverheadSecs: 123,
		}))
		res1 := &model.TaskRunResult{
			Key: model.TaskRunResultKey(ctx, req1),
			TaskResultCommon: model.TaskResultCommon{
				State: apipb.TaskState_COMPLETED,
			},
		}

		// TaskRunResult that doesn't have TaskRequest (will be skipped).
		req2, _ := model.TaskIDToRequestKey(ctx, "692b33d4feb00020")
		res2 := &model.TaskRunResult{
			Key: model.TaskRunResultKey(ctx, req2),
			TaskResultCommon: model.TaskResultCommon{
				State: apipb.TaskState_COMPLETED,
			},
		}

		// TaskRunResult that has TaskRequest, but not PerformanceStats.
		req3, _ := model.TaskIDToRequestKey(ctx, "692b33d4feb00030")
		assert.NoErr(t, datastore.Put(ctx, &model.TaskRequest{
			Key:  req3,
			Name: "req-3",
		}))
		res3 := &model.TaskRunResult{
			Key: model.TaskRunResultKey(ctx, req3),
			TaskResultCommon: model.TaskResultCommon{
				State: apipb.TaskState_BOT_DIED,
			},
		}

		// TaskRunResult with PerformanceStats entity missing.
		req4, _ := model.TaskIDToRequestKey(ctx, "692b33d4feb00040")
		assert.NoErr(t, datastore.Put(ctx, &model.TaskRequest{
			Key:  req4,
			Name: "req-4",
		}))
		res4 := &model.TaskRunResult{
			Key: model.TaskRunResultKey(ctx, req4),
			TaskResultCommon: model.TaskResultCommon{
				State: apipb.TaskState_COMPLETED,
			},
		}

		t.Run("Works", func(t *ftt.Test) {
			out, err := convertTaskResults(ctx,
				[]*model.TaskRunResult{res1, res2, res3, res4},
				func(res *model.TaskRunResult, req *model.TaskRequest, perf *model.PerformanceStats) *bqpb.TaskResult {
					perfInfo := "none"
					if perf != nil {
						perfInfo = fmt.Sprintf("%d", int(perf.BotOverheadSecs))
					}
					return &bqpb.TaskResult{
						TaskId: model.RequestKeyToTaskID(res.TaskRequestKey(), model.AsRequest),
						ServerVersions: []string{
							req.Name,
							perfInfo,
						},
					}
				},
			)
			assert.NoErr(t, err)

			assert.Loosely(t, out, should.Resemble([]*bqpb.TaskResult{
				{TaskId: "692b33d4feb00010", ServerVersions: []string{"req-1", "123"}},
				{TaskId: "692b33d4feb00030", ServerVersions: []string{"req-3", "none"}},
				{TaskId: "692b33d4feb00040", ServerVersions: []string{"req-4", "none"}},
			}))
		})

		t.Run("Transient error", func(t *ftt.Test) {
			var fb featureBreaker.FeatureBreaker
			ctx, fb = featureBreaker.FilterRDS(ctx, nil)
			fb.BreakFeatures(errors.New("BOOM"), "GetMulti")

			_, err := convertTaskResults(ctx,
				[]*model.TaskRunResult{res1, res2, res3, res4},
				func(*model.TaskRunResult, *model.TaskRequest, *model.PerformanceStats) *bqpb.TaskResult {
					panic("should not be called")
				},
			)

			assert.Loosely(t, err, should.ErrLike("BOOM"))
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
		})
	})
}

func TestBQRowDescriptors(t *testing.T) {
	t.Parallel()

	// Verify we can get descriptors of all necessary BQ rows without panics.
	_ = TaskRequestFetcher().Descriptor()
	_ = BotEventsFetcher().Descriptor()
	_ = TaskRunResultsFetcher().Descriptor()
	_ = TaskResultSummariesFetcher().Descriptor()
}
