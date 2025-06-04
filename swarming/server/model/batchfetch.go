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

package model

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

// ErrNoSuchFetchOperation is returned when trying to get result of an unknown
// fetch operation.
var ErrNoSuchFetchOperation = errors.New("no such fetch operation")

// BatchFetcher can fetch entities by key, in batches.
//
// This allows to relatively efficiently "join" results of some datastore query
// with another set of entities.
//
// It is a map from some `K` (that is just a local key, can be anything at all
// as long as it is comparable) to an entity-to-be-fetched `*E`. The entity key
// is part of `E`.
//
// The map is populated incrementally via Fetch(key, &Entity{...}) calls. Then
// Wait is called to wait for all pending fetches to complete. Then results are
// available via Get(key) calls.
//
// The advantage of that approach over just making a single multi-get is that
// we can start fetching things earlier, while still iterating over the query
// results.
type BatchFetcher[K comparable, E any] struct {
	ctx       context.Context    // the root context canceled by Close
	cancel    context.CancelFunc // used by Close to abort everything
	eg        *errgroup.Group    // the group that does all fetches
	batchSize int                // how many entities to fetch in a batch

	m        sync.RWMutex
	stopping bool           // true if Wait was called
	stopped  bool           // true if Wait finished running
	entries  map[K]entry[E] // pending and finished fetches
	pending  batch[K, E]    // the pending batch being assembled
}

// entry is a pending, running or completed fetch operation.
type entry[E any] struct {
	entity *E    // set only if the fetch succeeded
	err    error // an error fetching this entity, e.g. ErrNoSuchEntity
}

// batch is a set of (key, entity) pairs, stored as two parallel arrays for
// convenience of passing entities to datastore.Get.
type batch[K comparable, E any] struct {
	keys []K
	ents []*E
}

func (b *batch[K, E]) add(key K, ent *E) {
	b.keys = append(b.keys, key)
	b.ents = append(b.ents, ent)
}

func (b *batch[K, E]) size() int {
	return len(b.keys)
}

// NewBatchFetcher creates a batch fetcher.
//
// It should be used only for one fetch session. It uses the given context for
// all operations.
//
// Close must be called at some point to shutdown internal goroutines. It is OK
// to call it multiple times or from a defer.
//
// Recommended batchSize is 300 to match the default datastore query page size.
// In that case batches will be fetched essentially in parallel with query
// pages.
//
// Concurrency limit puts a limit on a number of concurrently running fetches
// (to avoid consuming excessive resources).
func NewBatchFetcher[K comparable, E any](ctx context.Context, batchSize, concurrencyLimit int) *BatchFetcher[K, E] {
	ctx, cancel := context.WithCancel(ctx)
	eg, _ := errgroup.WithContext(ctx)
	eg.SetLimit(concurrencyLimit)
	return &BatchFetcher[K, E]{
		ctx:       ctx,
		cancel:    cancel,
		eg:        eg,
		batchSize: batchSize,
		entries:   map[K]entry[E]{},
	}
}

// Fetch schedules fetching of the given entity.
//
// It will be fetched as a part of a batch multi-get call when the number of
// pending entities is equal to the configured batch size. Blocks if there are
// already more than configured number of concurrent multi-get calls happening
// right now.
//
// The result can be retrieved by Get(key), but only after Wait is called.
//
// Panics if there's already a fetch operation with the given key. Panics if
// the fetcher is already stopped (i.e. Wait or Close was called).
func (b *BatchFetcher[K, E]) Fetch(key K, entity *E) {
	if batch := b.addToPending(key, entity); batch != nil {
		b.flush(batch)
	}
}

// Wait waits for all fetch operations to complete.
//
// Should be called before Get can be used. It is fine to call it multiple
// times (redundant calls will just do nothing).
//
// If there were any errors while fetching entities (including ErrNoSuchEntity
// errors or context cancellation errors), they will be returned by the
// corresponding Get() calls.
func (b *BatchFetcher[K, E]) Wait() {
	b.m.Lock()
	b.stopping = true
	remaining := b.pending
	b.pending = batch[K, E]{}
	b.m.Unlock()

	if remaining.size() != 0 {
		b.flush(&remaining)
	}

	// Errors are communicated through entry.error.
	_ = b.eg.Wait()
	// We are done. Cancel the context to shutdown internal goroutines there.
	b.cancel()

	b.m.Lock()
	b.stopped = true
	b.m.Unlock()
}

// Close cancels all pending and running fetch operations (if any).
//
// It is fine to call it multiple times.
func (b *BatchFetcher[K, E]) Close() {
	b.cancel() // makes all running and future flush ops fail with context error
	b.Wait()   // wait for them to actually finish
}

// Get returns the fetched entity for the given operation key.
//
// Must be called only after Wait(). Panics otherwise.
//
// Returns an error if this entity could not be fetched. In particular
// the error is ErrNoSuchEntity if there's no such entity. Returns a context
// error if the batcher was closed (or its underlying context has expired)
// before the entity could be fetched.
//
// Returns ErrNoSuchFetchOperation if this operation key is unknown.
func (b *BatchFetcher[K, E]) Get(key K) (*E, error) {
	b.m.RLock()
	defer b.m.RUnlock()
	if !b.stopped {
		panic("not stopped yet")
	}
	if entry, ok := b.entries[key]; ok {
		return entry.entity, entry.err
	}
	return nil, ErrNoSuchFetchOperation
}

// addToPending adds K to the pending batch, returning the batch if it is full
// after adding K.
func (b *BatchFetcher[K, E]) addToPending(key K, entity *E) *batch[K, E] {
	b.m.Lock()
	defer b.m.Unlock()
	if b.stopping {
		panic("already stopping or stopped")
	}

	if _, ok := b.entries[key]; ok {
		panic(fmt.Sprintf("already have operation with key %v", key))
	}
	b.entries[key] = entry[E]{nil, nil}

	b.pending.add(key, entity)
	if b.pending.size() == b.batchSize {
		full := b.pending
		b.pending = batch[K, E]{}
		return &full
	}
	return nil
}

// flush launches the operation to fetch entities in the batch.
//
// May block if already reached the concurrency limit.
func (b *BatchFetcher[K, E]) flush(batch *batch[K, E]) {
	b.eg.Go(func() error {
		err := datastore.Get(b.ctx, batch.ents)

		var errCB func(i int) error
		var merr errors.MultiError
		if ok := errors.As(err, &merr); ok {
			errCB = func(i int) error { return merr[i] }
		} else {
			errCB = func(_ int) error { return err }
		}

		b.m.Lock()
		defer b.m.Unlock()

		for idx, key := range batch.keys {
			if err := errCB(idx); err != nil {
				b.entries[key] = entry[E]{nil, err}
			} else {
				b.entries[key] = entry[E]{batch.ents[idx], nil}
			}
		}

		return nil
	})
}

// PerformanceStatsHolder provides the PerformanceStats entity key.
//
// It is implemented by TaskRunResult and TaskResultSummary.
type PerformanceStatsHolder interface {
	PerformanceStatsKey(ctx context.Context) *datastore.Key
}

// PerformanceStatsFetcher can fetch PerformanceStats entities in parallel.
//
// It is a wrapper over generic BatchFetcher.
type PerformanceStatsFetcher struct {
	fetcher *BatchFetcher[*apipb.TaskResultResponse, PerformanceStats]
}

// NewPerformanceStatsFetcher constructs a new fetcher.
//
// It should be closed with Close to avoid leaking goroutines.
func NewPerformanceStatsFetcher(ctx context.Context) *PerformanceStatsFetcher {
	return &PerformanceStatsFetcher{
		fetcher: NewBatchFetcher[*apipb.TaskResultResponse, PerformanceStats](ctx, 300, 5),
	}
}

// Close makes sure all internal goroutines are canceled and stopped.
func (f *PerformanceStatsFetcher) Close() {
	f.fetcher.Close()
}

// Fetch starts fetching PerformanceStats of the task if it has them.
func (f *PerformanceStatsFetcher) Fetch(ctx context.Context, task *apipb.TaskResultResponse, key PerformanceStatsHolder) {
	if dskey := key.PerformanceStatsKey(ctx); dskey != nil {
		f.fetcher.Fetch(task, &PerformanceStats{Key: dskey})
	}
}

// Finish waits for all fetches to finish and populates PerformanceStats fields.
//
// Updates protos in `tasks` in-place.
func (f *PerformanceStatsFetcher) Finish(tasks []*apipb.TaskResultResponse) error {
	f.fetcher.Wait()
	for _, task := range tasks {
		switch stat, err := f.fetcher.Get(task); {
		case errors.Is(err, ErrNoSuchFetchOperation):
			// Didn't even try to fetch it, e.g. the task is pending, this is fine.
		case errors.Is(err, datastore.ErrNoSuchEntity):
			// The task has no stats attached, this is fine.
		case err != nil:
			return errors.Fmt("fetching stats of %q: %w", task.TaskId, err)
		default:
			if task.PerformanceStats, err = stat.ToProto(); err != nil {
				return errors.Fmt("processing stats of %q: %w", task.TaskId, err)
			}
		}
	}
	return nil
}
