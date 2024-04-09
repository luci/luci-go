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
	"math/rand"
	"reflect"
	"time"

	cloudds "cloud.google.com/go/datastore"
	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/log"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/register"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/impl/cloud"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/logdog/dataflow/dsutils"
)

func init() {
	beam.RegisterType(reflect.TypeOf((*backfillExpireAtFromCreatedFn)(nil)).Elem())
	register.DoFn3x1(&backfillExpireAtFromCreatedFn{})
	register.Emitter2[string, dsutils.KeyBatch]()
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
	// RetryCount is the number of times a failed batch will be retried.
	RetryCount int
}

// BackfillExpireAtFromCreated takes a PCollection<KeyBatch> and updates all the
// entities' ExpireAt timestamp from its Created timestamp according to expiry.
func BackfillExpireAtFromCreated(s beam.Scope, cloudProject string, logStreamKeys beam.PCollection, opts BackfillOptions) beam.PCollection {
	s = s.Scope(fmt.Sprintf("logdog.backfill-expire-at.%s", cloudProject))

	failedKeys := beam.ParDo(s, &backfillExpireAtFromCreatedFn{
		CloudProject:     cloudProject,
		Expiry:           opts.Expiry,
		SkipCreatedAfter: opts.SkipCreatedAfter,
		DryRun:           opts.DryRun,
		UpdateBatchSize:  opts.BatchSize,
	}, logStreamKeys)

	// Retry failed batches a few times but with less go workers (to reduce
	// the chance of datastore contention).
	// It's ok if they fail yet again. We will just leave a bit of extra entities
	// in the datastore. If we want, we can sweep all entities without expiry in
	// the future.
	for i := 0; i < opts.RetryCount; i++ {
		failedKeys = beam.Reshuffle(s, failedKeys)
		failedKeys = beam.ParDo(s, &backfillExpireAtFromCreatedFn{
			CloudProject:     cloudProject,
			Expiry:           opts.Expiry,
			SkipCreatedAfter: opts.SkipCreatedAfter,
			DryRun:           opts.DryRun,
			UpdateBatchSize:  opts.BatchSize,
			Workers:          1,
		}, failedKeys)
	}

	// Log the failed batches.
	return beam.ParDo(s, func(ctx context.Context, keyBatch dsutils.KeyBatch, emit func(dsutils.KeyBatch)) {
		if len(keyBatch.Keys) == 0 {
			return
		}

		firstKey := keyBatch.Keys[0].String()
		lastKey := keyBatch.Keys[len(keyBatch.Keys)-1].String()
		log.Errorf(ctx, "batch still failed after retries Namespace: `%s` Start: `%s` End: `%s`", keyBatch.Namespace, firstKey, lastKey)
		emit(keyBatch)
	}, failedKeys)
}

// backfillExpireAtFromCreatedFn is a DoFn that backfills the ExpireAt timestamp
// according to the created timestamp on the entity and the provided Expiry.
type backfillExpireAtFromCreatedFn struct {
	CloudProject     string
	Expiry           time.Duration
	SkipCreatedAfter time.Time
	DryRun           bool
	UpdateBatchSize  int
	Workers          int

	withDatastoreEnv func(context.Context) context.Context
	skippedCounter   beam.Counter
	updatedCounter   beam.Counter
	deletedCounter   beam.Counter
	notFoundCounter  beam.Counter
	failedCounter    beam.Counter
}

// Setup implements beam DoFn protocol.
func (fn *backfillExpireAtFromCreatedFn) Setup(ctx context.Context) error {
	if fn.withDatastoreEnv == nil {
		client, err := cloudds.NewClient(ctx, fn.CloudProject, option.WithEndpoint("batch-datastore.googleapis.com:443"))
		if err != nil {
			return errors.Annotate(err, "failed to construct cloud datastore client").Err()
		}
		fn.withDatastoreEnv = func(ctx context.Context) context.Context {
			return (&cloud.ConfigLite{
				ProjectID: fn.CloudProject,
				DS:        client,
			}).Use(ctx)
		}
	}

	namespace := fmt.Sprintf("logdog.backfill-expire-at.%s", fn.CloudProject)
	fn.skippedCounter = beam.NewCounter(namespace, "skipped")
	fn.updatedCounter = beam.NewCounter(namespace, "updated")
	fn.deletedCounter = beam.NewCounter(namespace, "deleted")
	fn.notFoundCounter = beam.NewCounter(namespace, "not-found")
	fn.failedCounter = beam.NewCounter(namespace, "failed")

	return nil
}

// RandomizedExponentialBackoff is similar to ExponentialBackoff but the actual
// delay is a random duration between
// [thisDelay, thisDelay * (1+MaxIncreaseRatio)).
type RandomizedExponentialBackoff struct {
	retry.ExponentialBackoff
	MaxIncreaseRatio float64
}

// Next implements Iterator.
func (b *RandomizedExponentialBackoff) Next(ctx context.Context, err error) time.Duration {
	// Get ExponentialBackoff base delay.
	delay := b.ExponentialBackoff.Next(ctx, err)
	if delay == retry.Stop {
		return retry.Stop
	}
	delay += time.Duration(rand.Float64() * b.MaxIncreaseRatio * float64(delay))
	return delay
}

func retryParams() retry.Iterator {
	return &RandomizedExponentialBackoff{
		ExponentialBackoff: retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:   time.Second,
				Retries: 6,
			},
			Multiplier: 2,
		},
		MaxIncreaseRatio: 0.5,
	}
}

// ProcessElement implements beam DoFn protocol.
func (fn *backfillExpireAtFromCreatedFn) ProcessElement(
	ctx context.Context,
	keyBatch dsutils.KeyBatch,
	emit func(dsutils.KeyBatch),
) error {
	// Short circuit if there's nothing in the batch.
	if len(keyBatch.Keys) == 0 {
		return nil
	}

	// Setup query context.
	ctx = fn.withDatastoreEnv(ctx)
	ctx, err := info.Namespace(ctx, keyBatch.Namespace)
	if err != nil {
		return errors.Annotate(err, "failed to apply namespace: `%s`", keyBatch.Namespace).Err()
	}

	return parallel.WorkPool(fn.Workers, func(c chan<- func() error) {
		for i := 0; i*fn.UpdateBatchSize < len(keyBatch.Keys); i++ {
			keys := keyBatch.Keys[i*fn.UpdateBatchSize : min((i+1)*fn.UpdateBatchSize, len(keyBatch.Keys))]
			startKey := keys[0]
			endKey := keys[len(keys)-1]
			c <- func() error {
				err := retry.Retry(ctx, retryParams, func() error {
					return fn.updateEntities(ctx, keys)
				}, func(err error, d time.Duration) {
					log.Errorf(ctx, "retry batch (Namespace: `%s`, Start: `%s`, End: `%s`) in %s; it failed due to: %v",
						keyBatch.Namespace, startKey.String(), endKey.String(), d.String(), err)
				})
				// In case of a datastore error, log the error and send the batch to the next
				// stage instead of returning an error so the worker can keep progressing.
				// Some keys might've been processed already. But it doesn't hurt to
				// retry them.
				if err != nil {
					fn.failedCounter.Inc(ctx, int64(len(keys)*2))
					log.Errorf(ctx, "batch (Namespace: `%s`, Start: `%s`, End: `%s`) failed due to %v",
						keyBatch.Namespace, startKey.String(), endKey.String(), err)
					emit(dsutils.KeyBatch{Namespace: keyBatch.Namespace, Keys: keys})
					return nil
				}

				return nil
			}
		}
	})
}

type entityWithCreatedAndExpireAt struct {
	Key      *datastore.Key        `gae:"$key"`
	Created  time.Time             `gae:",noindex"`
	ExpireAt time.Time             `gae:",noindex"`
	Extra    datastore.PropertyMap `gae:",extra"`
}

func (fn *backfillExpireAtFromCreatedFn) updateEntities(
	ctx context.Context,
	logStreamKeys []*datastore.Key,
) error {
	// Prepare the entity list to read.
	entities := make([]*entityWithCreatedAndExpireAt, 0, len(logStreamKeys)*2)
	for _, streamKey := range logStreamKeys {
		entities = append(entities, &entityWithCreatedAndExpireAt{
			Key: streamKey,
		})

		// Also update the corresponding LogStreamState.
		stateKey := streamKey.KeyContext().NewKey("LogStreamState", "", 1, streamKey)
		entities = append(entities, &entityWithCreatedAndExpireAt{
			Key: stateKey,
		})
	}

	// Read the entities.
	err := datastore.Get(ctx, entities)
	if err != nil {
		me := err.(errors.MultiError)
		lme := errors.NewLazyMultiError(len(me))
		for i, ierr := range me {
			if errors.Is(ierr, datastore.ErrNoSuchEntity) {
				ierr = nil        // ignore ErrNoSuchEntity
				entities[i] = nil // nil out the entity, so we can skip it later.
			}
			lme.Assign(i, ierr)
		}

		// Return an error only if we encounter an error other than ErrNoSuchEntity.
		if err := lme.Get(); err != nil {
			return errors.Annotate(err, "failed to read entities").Err()
		}
	}

	now := clock.Now(ctx)

	// Get the list of entities to update.
	notFound := 0
	entitiesToPut := make([]*entityWithCreatedAndExpireAt, 0)
	entitiesToDelete := make([]*entityWithCreatedAndExpireAt, 0, len(entities))
	for _, entity := range entities {
		if entity == nil {
			notFound++
			continue
		}

		createdAt := entity.Created

		// If somehow the created timestamp is not defined, make it expire in
		// `fn.Expiry` from now.
		if createdAt.IsZero() {
			createdAt = now
		}

		// Skip entities created after certain timestamp. This allow us to perform
		// updates without using a transaction since those entities should've been
		// finalized. (i.e. no other process is updating the same entity).
		//
		// Newer entities do not need to be backfilled as the ExpireAt should
		// already be populated.
		if createdAt.After(fn.SkipCreatedAfter) {
			continue
		}

		expectedExpireAt := createdAt.Add(fn.Expiry).UTC()

		if expectedExpireAt.Equal(entity.ExpireAt) {
			continue
		}

		if expectedExpireAt.Before(now) {
			entitiesToDelete = append(entitiesToDelete, entity)
			continue
		}

		entity.ExpireAt = expectedExpireAt
		entitiesToPut = append(entitiesToPut, entity)
	}

	// Update the entities.
	if !fn.DryRun {
		err := datastore.Put(ctx, entitiesToPut)
		if err != nil {
			return errors.Annotate(err, "failed to Put entities").Err()
		}
	}
	fn.updatedCounter.Inc(ctx, int64(len(entitiesToPut)))

	if !fn.DryRun {
		err = datastore.Delete(ctx, entitiesToDelete)
		if err != nil {
			return errors.Annotate(err, "failed to Delete entities").Err()
		}
	}
	fn.deletedCounter.Inc(ctx, int64(len(entitiesToDelete)))

	fn.skippedCounter.Inc(ctx, int64(len(entities)-len(entitiesToPut)-len(entitiesToDelete)-notFound))
	fn.notFoundCounter.Inc(ctx, int64(notFound))
	return nil
}
