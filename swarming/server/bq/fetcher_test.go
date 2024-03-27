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
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFetcher(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
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
			So(datastore.Put(ctx, &entity{
				ID: int64(i),
				TS: testTime.Add(time.Duration(i) * time.Second),
			}), ShouldBeNil)
		}
		datastore.GetTestable(ctx).CatchupIndexes()

		var convertErr error
		var convertCalls []int

		fetcher := &Fetcher[entity, *wrapperspb.Int64Value]{
			entityKind:     "entity",
			timestampField: "TS",
			queryBatchSize: queryBatchSize,
			flushThreshold: flushThreshold,
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
		flushCB := func(rows [][]byte) error {
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
		}

		Convey("Visits correct time range", func() {
			err := fetcher.Fetch(ctx, testTime.Add(3*time.Second), 10*time.Second, flushCB)
			So(err, ShouldBeNil)
			So(collected, ShouldResemble, []int64{3, 4, 5, 6, 7, 8, 9, 10, 11, 12})

			// Respects queryBatchSize (== 3 entities).
			So(convertCalls, ShouldResemble, []int{3, 3, 3, 1})

			// Respects flushThreshold (== 7 bytes). Each message is 2 bytes long. It
			// takes 4 messages (== 8 bytes) to trigger the flush. The last flush
			// sends the remaining 2 messages.
			So(flushCalls, ShouldResemble, []int{8, 8, 4})
		})

		Convey("Convert error", func() {
			wantErr := errors.New("BOOM")
			convertErr = wantErr
			gotErr := fetcher.Fetch(ctx, testTime.Add(3*time.Second), 10*time.Second, flushCB)
			So(gotErr, ShouldEqual, wantErr)
			So(collected, ShouldBeEmpty)
		})

		Convey("Flush error", func() {
			wantErr := errors.New("BOOM")
			flushErr = func(int) error { return wantErr }
			gotErr := fetcher.Fetch(ctx, testTime.Add(3*time.Second), 10*time.Second, flushCB)
			So(gotErr, ShouldEqual, wantErr)
			So(collected, ShouldBeEmpty)
		})

		Convey("Final flush error", func() {
			wantErr := errors.New("BOOM")
			flushErr = func(size int) error {
				if size == 4 {
					return wantErr
				}
				return nil
			}
			gotErr := fetcher.Fetch(ctx, testTime.Add(3*time.Second), 10*time.Second, flushCB)
			So(gotErr, ShouldEqual, wantErr)
			So(collected, ShouldResemble, []int64{3, 4, 5, 6, 7, 8, 9, 10}) // doesn't have the last 2
		})
	})
}

func TestBQRowDescriptors(t *testing.T) {
	t.Parallel()

	// Verify we can get descriptors of all necessary BQ rows without panics.
	_ = TaskRequestFetcher().Descriptor()

	// TODO(vadimsh): These panic currently due to use of recursive
	// structpb.Struct.
	// _ = BotEventsFetcher().Descriptor()
	// _ = TaskRunResultsFetcher().Descriptor()
	// _ = TaskResultSummariesFetcher().Descriptor()
}
