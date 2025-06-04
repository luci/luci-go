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
	"time"

	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	bqpb "go.chromium.org/luci/swarming/proto/bq"
	"go.chromium.org/luci/swarming/server/model"
)

// How many entities to fetch by default before staring converting them.
//
// 300 is the internal page size as used by Cloud Datastore queries.
const queryBatchSize = 300

// Fetcher knows how to fetch Datastore data that should be exported to BQ.
//
// `E` is the entity struct to fetch (e.g. model.TaskResultSummary) and `R` is
// the proto message representing the row (e.g. *bqpb.TaskResult).
type Fetcher[E any, R proto.Message] struct {
	entityKind     string // e.g. "TaskRequest"
	timestampField string // e.g. "created_ts"
	queryBatchSize int    // how many entities to fetch before staring converting them

	// convert is a callback that converts a batch of fetched entities to BQ rows.
	//
	// It can fetch any additional data it needs.
	convert func(context.Context, []*E) ([]R, error)
}

// AbstractFetcher is implemented by all Fetcher[E, R].
type AbstractFetcher interface {
	// Descriptor is the proto descriptor of the BQ row protobuf message.
	Descriptor() *descriptorpb.DescriptorProto
	// Fetch fetches entities, converts them to BQ rows and flushes the result.
	Fetch(ctx context.Context, start time.Time, duration time.Duration, flushers []*Flusher) error
}

// Flusher knows how to serialize rows and flush a bunch of them.
type Flusher struct {
	// CountThreshold is how many rows to buffer before flushing them.
	CountThreshold int
	// ByteThreshold is how many bytes to buffer before flushing them.
	ByteThreshold int
	// Marshal converts a row proto to a raw byte message.
	Marshal func(proto.Message) ([]byte, error)
	// Flush flushes a batch of converted messages.
	Flush func([][]byte) error

	pendingRows     [][]byte
	pendingRowsSize int
}

// Descriptor is the proto descriptor of the row protobuf message.
func (f *Fetcher[E, R]) Descriptor() *descriptorpb.DescriptorProto {
	var msg R
	desc, err := adapt.NormalizeDescriptor(msg.ProtoReflect().Descriptor())
	if err != nil {
		panic(err) // we have a unit test for this
	}
	return desc
}

// Fetch fetches entities, converts them to BQ rows and flushes the result.
//
// Visits entities in range `[start, start+duration)`. Calls `flush` callback
// when it wants to send a batch of rows to BQ.
func (f *Fetcher[E, R]) Fetch(ctx context.Context, start time.Time, duration time.Duration, flushers []*Flusher) error {
	var pendingEntities []*E

	flushPending := func(force bool) error {
		batch := pendingEntities
		pendingEntities = nil

		// Convert to row proto messages.
		var protos []R
		if len(batch) != 0 {
			var err error
			if protos, err = f.convert(ctx, batch); err != nil {
				return err
			}
		}

		// Feed to flushers.
		for _, flusher := range flushers {
			// Split into batches of ~= flusher.Threshold each, flush them.
			for _, msg := range protos {
				pb, err := flusher.Marshal(msg)
				if err != nil {
					return errors.Fmt("failed to marshal BQ row %v: %w", msg, err)
				}
				flusher.pendingRows = append(flusher.pendingRows, pb)
				flusher.pendingRowsSize += len(pb)
				if len(flusher.pendingRows) >= flusher.CountThreshold || flusher.pendingRowsSize >= flusher.ByteThreshold {
					flushing := flusher.pendingRows
					flusher.pendingRows = nil
					flusher.pendingRowsSize = 0
					if err := flusher.Flush(flushing); err != nil {
						return err
					}
				}
			}
			// Flush the final batch if asked to.
			if force && len(flusher.pendingRows) != 0 {
				if err := flusher.Flush(flusher.pendingRows); err != nil {
					return err
				}
			}
		}

		return nil
	}

	q := datastore.NewQuery(f.entityKind).
		Gte(f.timestampField, start).
		Lt(f.timestampField, start.Add(duration))

	var innerErr error
	err := datastore.Run(ctx, q, func(ent *E) error {
		innerErr = nil
		pendingEntities = append(pendingEntities, ent)
		if len(pendingEntities) >= f.queryBatchSize {
			innerErr = flushPending(false)
		}
		return innerErr
	})
	if flushErr := flushPending(true); flushErr != nil {
		return flushErr
	}

	switch {
	case err == nil:
		return nil
	case err == innerErr:
		// The callback failed (e.g. flushPending returned an error). Propagate
		// the error as is. It comes from the `flush` callback.
		return innerErr
	default:
		// If datastore.Run returned an error not from the callback, it must be
		// the error from the datastore library itself (e.g. a query timeout). Mark
		// it as a transient error.
		return transient.Tag.Apply(errors.Fmt("datastore query error: %w", err))
	}
}

// TaskRequestFetcher makes a fetcher that can produce TaskRequest BQ rows.
func TaskRequestFetcher() AbstractFetcher {
	return &Fetcher[model.TaskRequest, *bqpb.TaskRequest]{
		entityKind:     "TaskRequest",
		timestampField: "created_ts",
		queryBatchSize: queryBatchSize,
		convert: func(_ context.Context, entities []*model.TaskRequest) ([]*bqpb.TaskRequest, error) {
			out := make([]*bqpb.TaskRequest, len(entities))
			for i, ent := range entities {
				out[i] = taskRequest(ent)
			}
			return out, nil
		},
	}
}

// BotEventsFetcher makes a fetcher that can produce BotEvent BQ rows.
func BotEventsFetcher() AbstractFetcher {
	return &Fetcher[model.BotEvent, *bqpb.BotEvent]{
		entityKind:     "BotEvent",
		timestampField: "ts",
		queryBatchSize: queryBatchSize,
		convert: func(_ context.Context, entities []*model.BotEvent) ([]*bqpb.BotEvent, error) {
			out := make([]*bqpb.BotEvent, len(entities))
			for i, ent := range entities {
				out[i] = botEvent(ent)
			}
			return out, nil
		},
	}
}

// TaskRunResultsFetcher makes a fetcher that can produce TaskResult BQ rows.
func TaskRunResultsFetcher() AbstractFetcher {
	return &Fetcher[model.TaskRunResult, *bqpb.TaskResult]{
		entityKind:     "TaskRunResult",
		timestampField: "completed_ts",
		queryBatchSize: queryBatchSize,
		convert: func(ctx context.Context, entities []*model.TaskRunResult) ([]*bqpb.TaskResult, error) {
			return convertTaskResults(ctx, entities, taskRunResult)
		},
	}
}

// TaskResultSummariesFetcher makes a fetcher that can produce TaskResult BQ
// rows.
func TaskResultSummariesFetcher() AbstractFetcher {
	return &Fetcher[model.TaskResultSummary, *bqpb.TaskResult]{
		entityKind:     "TaskResultSummary",
		timestampField: "completed_ts",
		queryBatchSize: queryBatchSize,
		convert: func(ctx context.Context, entities []*model.TaskResultSummary) ([]*bqpb.TaskResult, error) {
			return convertTaskResults(ctx, entities, taskResultSummary)
		},
	}
}

// TaskRunResult and TaskResultSummary fetchers share implementation. The only
// difference is in how they are converted to BQ rows in the end by the
// `converter` callback.

type taskResultEntity[T any] interface {
	*T
	TaskRequestKey() *datastore.Key
	PerformanceStatsKey(ctx context.Context) *datastore.Key
}

func convertTaskResults[T any, TP taskResultEntity[T]](
	ctx context.Context,
	entities []TP,
	converter func(TP, *model.TaskRequest, *model.PerformanceStats) *bqpb.TaskResult,
) ([]*bqpb.TaskResult, error) {
	// Things to fetch.
	reqs := make([]*model.TaskRequest, 0, len(entities))
	stat := make([]*model.PerformanceStats, 0, len(entities))

	// Index of the original entity => index of its PerformanceStats in `stat`.
	statMap := make(map[int]int, len(entities))

	for idx, ent := range entities {
		reqs = append(reqs, &model.TaskRequest{Key: ent.TaskRequestKey()})
		if statsKey := ent.PerformanceStatsKey(ctx); statsKey != nil {
			stat = append(stat, &model.PerformanceStats{Key: statsKey})
			statMap[idx] = len(stat) - 1
		}
	}

	var reqsErr error
	var statErr error

	// Fetch both lists in parallel.
	if err := datastore.Get(ctx, reqs, stat); err != nil {
		var merr errors.MultiError
		if errors.As(err, &merr) {
			// Errors specific to entities being fetched.
			reqsErr, statErr = merr[0], merr[1]
		} else {
			// Some generic RPC error, applies to both lists.
			reqsErr, statErr = err, err
		}
	}

	// Join the results and convert to BQ format.
	out := make([]*bqpb.TaskResult, 0, len(entities))
	for idx, ent := range entities {
		taskID := model.RequestKeyToTaskID(ent.TaskRequestKey(), model.AsRequest)

		// TaskRequest **must** be there. Skip broken tasks that don't have it.
		req, err := entityOrErr(reqs, reqsErr, idx)
		switch {
		case errors.Is(err, datastore.ErrNoSuchEntity):
			logging.Errorf(ctx, "Missing TaskRequest for %q, skipping", taskID)
			continue
		case err != nil:
			return nil, transient.Tag.Apply(errors.Fmt("fetching TaskRequest %q: %w", taskID, err))
		}

		// PerformanceStats may be missing if the task didn't run, this is OK.
		var stats *model.PerformanceStats
		if statsIdx, ok := statMap[idx]; ok {
			stats, err = entityOrErr(stat, statErr, statsIdx)
			if err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
				return nil, transient.Tag.Apply(errors.Fmt("fetching PerformanceStats for %q: %w", taskID, err))
			}
		}

		out = append(out, converter(ent, req, stats))
	}

	return out, nil
}

func entityOrErr[T any](res []T, err error, idx int) (T, error) {
	if err == nil {
		return res[idx], nil
	}

	var merr errors.MultiError
	if errors.As(err, &merr) {
		if err := merr[idx]; err != nil {
			var zero T
			return zero, err
		}
		return res[idx], nil
	}

	var zero T
	return zero, err
}
