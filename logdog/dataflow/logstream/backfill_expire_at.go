// Copyright 2024 The LUCI Authors.
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

package logstream

import (
	"context"
	"fmt"
	"reflect"
	"time"

	cloudds "cloud.google.com/go/datastore"
	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/core/graph/window"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/core/graph/window/trigger"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/log"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/register"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/impl/cloud"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"

	"go.chromium.org/luci/logdog/dataflow/dsutils"
)

func init() {
	beam.RegisterType(reflect.TypeOf((*backfillExpireAtFromCreatedFn)(nil)).Elem())
	register.DoFn4x1(&backfillExpireAtFromCreatedFn{})
	register.Emitter2[string, dsutils.NamespaceKey]()
}

type BackfillOptions struct {
	// DryRun controls whether the datastore updates should be applied.
	DryRun bool
	// Workers controls the number of go worker in each beam worker.
	Workers int
	// BatchSize controls the number of elements each go worker attempt to process
	// at a time.
	BatchSize int
	// SkipCreatedAfter entities created after this timestamp will be skipped.
	// Older entities are always finalized. This allow us to do updates without
	// using a transaction since this is the only process that will update those
	// entities.
	SkipCreatedAfter time.Time
	// Expiry is the duration added to entities creation time to obtain the expiry
	// date.
	Expiry time.Duration
}

// BackfillExpireAtFromCreated takes a PCollection<NamespaceKey> and updates all
// the entities' ExpireAt timestamp from its Created timestamp according to
// expiry.
func BackfillExpireAtFromCreated(s beam.Scope, project string, nk beam.PCollection, opts BackfillOptions) beam.PCollection {
	s = s.Scope(fmt.Sprintf("logdog.backfill-expire-at.%s", project))

	// Use a trigger to divide the window into smaller collections so hopefully
	// the pipeline can don't have to retry the entire shard when an element
	// fails to be processed.
	nk = beam.WindowInto(
		s,
		window.NewGlobalWindows(),
		nk,
		beam.Trigger(
			trigger.Repeat(
				trigger.AfterProcessingTime().PlusDelay(10*time.Minute),
			),
		),
		beam.PanesDiscard(),
	)

	// Group by PCollection key (`${namespace}-${shard}`) so they can be batched
	// together.
	groupedNk := beam.GroupByKey(s, nk)
	failedNk := beam.ParDo(s, &backfillExpireAtFromCreatedFn{
		Project:          project,
		Expiry:           opts.Expiry,
		SkipCreatedAfter: opts.SkipCreatedAfter,
		DryRun:           opts.DryRun,
		MaxBatchSize:     opts.BatchSize,
		Workers:          opts.Workers,
	}, groupedNk)

	// Try failed batches again but with less workers.
	// It's ok if they fail yet again. We will just leave a bit of extra entities
	// in the datastore. If we want, we can sweep all entities without expiry in
	// the future.
	groupedFailedNk := beam.GroupByKey(s, failedNk)
	return beam.ParDo(s, &backfillExpireAtFromCreatedFn{
		Project:          project,
		Expiry:           opts.Expiry,
		SkipCreatedAfter: opts.SkipCreatedAfter,
		DryRun:           opts.DryRun,
		MaxBatchSize:     opts.BatchSize,
		Workers:          1,
	}, groupedFailedNk)
}

// backfillExpireAtFromCreatedFn is a DoFn that backfills the ExpireAt timestamp
// according to the created timestamp on the entity and the provided Expiry.
type backfillExpireAtFromCreatedFn struct {
	Project          string
	Expiry           time.Duration
	SkipCreatedAfter time.Time
	DryRun           bool
	MaxBatchSize     int
	Workers          int

	withEnv        func(context.Context) context.Context
	skippedCounter beam.Counter
	updatedCounter beam.Counter
	failedCounter  beam.Counter
}

// Setup implements beam DoFn protocol.
func (fn *backfillExpireAtFromCreatedFn) Setup(ctx context.Context) error {
	if fn.withEnv == nil {
		client, err := cloudds.NewClient(ctx, fn.Project)
		if err != nil {
			return errors.Annotate(err, "failed to construct cloud datastore client").Err()
		}
		fn.withEnv = func(ctx context.Context) context.Context {
			return (&cloud.ConfigLite{
				ProjectID: fn.Project,
				DS:        client,
			}).Use(ctx)
		}
	}

	namespace := fmt.Sprintf("logdog.backfill-expire-at.%s", fn.Project)
	fn.skippedCounter = beam.NewCounter(namespace, "skipped")
	fn.updatedCounter = beam.NewCounter(namespace, "updated")
	fn.failedCounter = beam.NewCounter(namespace, "failed")

	return nil
}

// ProcessElement implements beam DoFn protocol.
func (fn *backfillExpireAtFromCreatedFn) ProcessElement(
	ctx context.Context,
	eleKey string,
	loadKey func(*dsutils.NamespaceKey) bool,
	emit func(string, dsutils.NamespaceKey),
) error {
	ctx = fn.withEnv(ctx)

	var nk dsutils.NamespaceKey
	if !loadKey(&nk) {
		return nil
	}
	namespace := nk.Namespace
	ctx, err := info.Namespace(ctx, namespace)
	if err != nil {
		return errors.Annotate(err, "failed to apply namespace: `%s`", namespace).Err()
	}

	reportError := func(err error, batch []*datastore.Key) {
		// Log the error and send the batch to the next stage instead of triggering
		// an error so the worker can keep progressing.
		log.Errorf(ctx, "%v", err)
		for _, failedNk := range batch {
			emit(eleKey, dsutils.NamespaceKey{Namespace: namespace, Key: failedNk})
		}
	}

	return parallel.WorkPool(fn.Workers, func(c chan<- func() error) {
		nextBatch := make([]*datastore.Key, 0, fn.MaxBatchSize)

		// Keep acquiring and processing batches.
		for {
			nextBatch = append(nextBatch, nk.Key)

			if len(nextBatch) == fn.MaxBatchSize {
				currentBatch := nextBatch
				c <- func() error {
					err := fn.flushBatch(ctx, currentBatch)
					if err != nil {
						reportError(err, currentBatch)
					}
					return nil
				}
				nextBatch = make([]*datastore.Key, 0, fn.MaxBatchSize)
			}

			if !loadKey(&nk) {
				break
			}
		}

		// Process the remaining items in the batch.
		c <- func() error {
			err := fn.flushBatch(ctx, nextBatch)
			if err != nil {
				reportError(err, nextBatch)
			}
			return nil
		}
	})
}

type entityWithCreatedAndExpireAt struct {
	Key      *datastore.Key        `gae:"$key"`
	Created  time.Time             `gae:",noindex"`
	ExpireAt time.Time             `gae:",noindex"`
	Extra    datastore.PropertyMap `gae:",extra"`
}

func (fn *backfillExpireAtFromCreatedFn) flushBatch(
	ctx context.Context,
	keys []*datastore.Key,
) error {
	if len(keys) == 0 {
		return nil
	}

	entities := make([]*entityWithCreatedAndExpireAt, 0, len(keys))
	for _, k := range keys {
		entities = append(entities, &entityWithCreatedAndExpireAt{
			Key: k,
		})
	}
	err := datastore.Get(ctx, entities)
	if err != nil {
		return err
	}

	updatedEntities := make([]*entityWithCreatedAndExpireAt, 0, len(entities))
	for _, entity := range entities {
		createdAt := entity.Created

		// If somehow the created timestamp is not defined, make it expire in
		// `fn.Expiry` from now.
		if createdAt.IsZero() {
			createdAt = clock.Now(ctx)
		}

		// Skip entities created after certain timestamp. This allow us to perform
		// updates without using a transaction since those entities should've been
		// finalized. (i.e. no other process is updating the same entity).
		if createdAt.After(fn.SkipCreatedAfter) {
			continue
		}

		expectedExpireAt := createdAt.Add(fn.Expiry).UTC()

		if expectedExpireAt.Equal(entity.ExpireAt) {
			continue
		}

		entity.ExpireAt = expectedExpireAt
		updatedEntities = append(updatedEntities, entity)
	}

	if !fn.DryRun {
		err := datastore.Put(ctx, updatedEntities)
		if err != nil {
			fn.failedCounter.Inc(ctx, int64(len(keys)))
			return errors.Annotate(err, "failed to Put entities").Err()
		}
	}

	fn.updatedCounter.Inc(ctx, int64(len(updatedEntities)))
	fn.skippedCounter.Inc(ctx, int64(len(keys)-len(updatedEntities)))
	return nil
}
