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
	"go.chromium.org/luci/gae/service/datastore"

	bqpb "go.chromium.org/luci/swarming/proto/bq"
	"go.chromium.org/luci/swarming/server/model"
)

const (
	// How many entities to fetch by default before staring converting them.
	//
	// 300 is the internal page size as used by Cloud Datastore queries.
	queryBatchSize = 300

	// How many raw BigQuery serialized bytes to buffer before flushing them.
	//
	// BigQuery's hard limit is 10MB.
	flushThreshold = 5 * 1024 * 1024
)

// Fetcher knows how to fetch Datastore data that should be exported to BQ.
//
// `E` is the entity struct to fetch (e.g. model.TaskResultSummary) and `R` is
// the proto message representing the row (e.g. *bqpb.TaskResult).
type Fetcher[E any, R proto.Message] struct {
	entityKind     string // e.g. "TaskRequest"
	timestampField string // e.g. "created_ts"
	queryBatchSize int    // how many entities to fetch before staring converting them
	flushThreshold int    // how many bytes to buffer before flushing them

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
	Fetch(ctx context.Context, start time.Time, duration time.Duration, flush func(rows [][]byte) error) error
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
func (f *Fetcher[E, R]) Fetch(ctx context.Context, start time.Time, duration time.Duration, flush func(rows [][]byte) error) error {
	var pendingEntities []*E
	var pendingRows [][]byte
	var pendingRowsSize int

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

		// Split into batches of ~= flushThreshold each, flush them.
		for _, msg := range protos {
			pb, err := proto.Marshal(msg)
			if err != nil {
				return errors.Annotate(err, "failed to marshal BQ row %v", msg).Err()
			}
			pendingRows = append(pendingRows, pb)
			pendingRowsSize += len(pb)
			if pendingRowsSize >= f.flushThreshold {
				flushing := pendingRows
				pendingRows = nil
				pendingRowsSize = 0
				if err := flush(flushing); err != nil {
					return err
				}
			}
		}

		// Flush the final batch if asked to.
		if force && len(pendingRows) != 0 {
			return flush(pendingRows)
		}
		return nil
	}

	q := datastore.NewQuery(f.entityKind).
		Gte(f.timestampField, start).
		Lt(f.timestampField, start.Add(duration))

	err := datastore.Run(ctx, q, func(ent *E) error {
		pendingEntities = append(pendingEntities, ent)
		if len(pendingEntities) >= f.queryBatchSize {
			return flushPending(false)
		}
		return nil
	})
	if err := flushPending(true); err != nil {
		return err
	}

	return err
}

// TaskRequestFetcher makes a fetcher that can produce TaskRequest BQ rows.
func TaskRequestFetcher() AbstractFetcher {
	return &Fetcher[model.TaskRequest, *bqpb.TaskRequest]{
		entityKind:     "TaskRequest",
		timestampField: "created_ts",
		queryBatchSize: queryBatchSize,
		flushThreshold: flushThreshold,
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
		flushThreshold: flushThreshold,
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
		flushThreshold: flushThreshold,
		convert: func(context.Context, []*model.TaskRunResult) ([]*bqpb.TaskResult, error) {
			panic("not implemented")
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
		flushThreshold: flushThreshold,
		convert: func(context.Context, []*model.TaskResultSummary) ([]*bqpb.TaskResult, error) {
			panic("not implemented")
		},
	}
}
