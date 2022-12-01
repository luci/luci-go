// Copyright 2022 The LUCI Authors.
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

package graph

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/gomodule/redigo/redis"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/resultdb/internal/invocations"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/span"
)

// MaxNodes is the maximum number of invocation nodes that ResultDB
// can operate on at a time.
const MaxNodes = 20000

// reachCacheExpiration is expiration duration of ReachCache.
// It is more important to have *some* expiration; the value itself matters less
// because Redis evicts LRU keys only with *some* expiration set,
// see volatile-lru policy: https://redis.io/topics/lru-cache
const reachCacheExpiration = 30 * 24 * time.Hour // 30 days

// readReachableInvocation reads summary information about a reachable
// invocation.
func readReachableInvocation(ctx context.Context, id invocations.ID) (ReachableInvocation, error) {
	// Both of these requests should be very cheap, as they check only for the existance of one matching row.
	var hasTestResults bool
	opts := &spanner.ReadOptions{Limit: 1}
	err := span.ReadWithOptions(ctx, "TestResults", id.Key().AsPrefix(), []string{"InvocationId"}, opts).Do(func(r *spanner.Row) error {
		hasTestResults = true
		return nil
	})
	if err != nil {
		return ReachableInvocation{}, err
	}

	var hasTestExonerations bool
	opts = &spanner.ReadOptions{Limit: 1}
	err = span.ReadWithOptions(ctx, "TestExonerations", id.Key().AsPrefix(), []string{"InvocationId"}, opts).Do(func(r *spanner.Row) error {
		hasTestExonerations = true
		return nil
	})
	if err != nil {
		return ReachableInvocation{}, err
	}
	return ReachableInvocation{
		HasTestResults:      hasTestResults,
		HasTestExonerations: hasTestExonerations,
	}, nil
}

// TooManyTag set in an error indicates that too many invocations
// matched a condition.
var TooManyTag = errors.BoolTag{
	Key: errors.NewTagKey("too many matching invocations matched the condition"),
}

// Reachable returns all invocations reachable from roots along the inclusion
// edges.  Will return an error if there are more than MaxNodes invocations.
// May return an appstatus-annotated error.
func Reachable(ctx context.Context, roots invocations.IDSet) (ReachableInvocations, error) {
	allIDs := ReachableInvocations{}
	processor := func(ctx context.Context, invs ReachableInvocations) error {
		allIDs.Union(invs)
		// Yes, this is an artificial limit.  With 20,000 invocations you are already likely
		// to run into problems if you try to process all of these in one go (e.g. in a
		// Spanner query).  If you want more, use the batched call and handle a batch at a time.
		if len(allIDs) > MaxNodes {
			return errors.Reason("more than %d invocations match", MaxNodes).Tag(TooManyTag).Err()
		}
		return nil
	}
	err := BatchedReachable(ctx, roots, processor)
	return allIDs, err
}

// BatchedReachable calls the processor for batches of all invocations reachable
// from roots along the inclusion edges.  Using this function avoids the MaxNodes
// limitation.
// If the processor returns an error, no more batches will be processed and the
// error will be returned immediately.
func BatchedReachable(ctx context.Context, roots invocations.IDSet, processor func(context.Context, ReachableInvocations) error) error {
	invs, err := reachable(ctx, roots, true)
	if err != nil {
		return err
	}
	for _, batch := range invs.Batches() {
		if err := processor(ctx, batch); err != nil {
			return err
		}
	}
	return nil
}

// ReachableSkipRootCache is similar to BatchedReachable, but it ignores cache
// for the roots.
//
// Useful to keep cache-hit stats high in cases where the roots are known not to
// have cache.
func ReachableSkipRootCache(ctx context.Context, roots invocations.IDSet) (ReachableInvocations, error) {
	invs, err := reachable(ctx, roots, false)
	if err != nil {
		return nil, err
	}
	return invs, nil
}

func reachable(ctx context.Context, roots invocations.IDSet, useRootCache bool) (reachable ReachableInvocations, err error) {
	ctx, ts := trace.StartSpan(ctx, "resultdb.graph.reachable")
	defer func() { ts.End(err) }()

	originalCtx := ctx
	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	visited := make(invocations.IDSet, len(roots))
	reachable = make(ReachableInvocations, len(roots))
	hasCacheMiss := false
	var mu sync.Mutex

	spanSem := semaphore.NewWeighted(64) // limits Spanner RPC concurrency.

	var visit func(id invocations.ID, useCache bool) error
	visit = func(id invocations.ID, useCache bool) error {
		mu.Lock()
		defer mu.Unlock()

		// Check if we already started/finished fetching this invocation.
		if visited.Has(id) {
			return nil
		}

		// Mark the invocation as being processed.
		visited.Add(id)

		// Concurrently fetch the inclusions without a lock.
		eg.Go(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}

			if useCache {
				// First check the cache.
				switch reachables, err := reachCache(id).Read(ctx); {
				case err == redisconn.ErrNotConfigured || err == ErrUnknownReach:
					// Ignore this error.
				case err != nil:
					logging.Warningf(ctx, "ReachCache: failed to read %s: %s", id, err)
				default:
					// Cache hit. Copy the results to `reachable` and exit without
					// recursion.
					mu.Lock()
					defer mu.Unlock()
					reachable.Union(reachables)
					return nil
				}
				// Cache miss. => Read from Spanner.
			}

			// Find and visit children.
			if err := spanSem.Acquire(ctx, 1); err != nil {
				return err
			}
			included, err := invocations.ReadIncluded(ctx, id)
			spanSem.Release(1)
			if err != nil {
				return errors.Annotate(err, "reading invocations included in %s", id.Name()).Err()
			}

			for id := range included {
				// Always use cache for children.
				if err := visit(id, true); err != nil {
					return err
				}
			}

			// Read summary information about the current invocation.
			if err := spanSem.Acquire(ctx, 1); err != nil {
				return err
			}
			inv, err := readReachableInvocation(ctx, id)
			spanSem.Release(1)
			if err != nil {
				return errors.Annotate(err, "reading reachable invocation %s", id.Name()).Err()
			}
			mu.Lock()
			defer mu.Unlock()
			reachable[id] = inv
			hasCacheMiss = true
			return nil
		})
		return nil
	}

	// Trigger fetching by requesting all roots.
	for id := range roots {
		if err := visit(id, useRootCache); err != nil {
			return nil, err
		}
	}

	// Wait for the entire graph to be fetched.
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	// Restore the original context as the waitgroup context is cancelled.
	ctx = originalCtx

	// If we queried for one root and we had a cache miss, try to insert the
	// reachable invocations, so that the cache will hopefully be populated
	// next time.
	if len(roots) == 1 && hasCacheMiss {
		var root invocations.ID
		for id := range roots {
			root = id
		}
		state, err := invocations.ReadState(ctx, root)
		if err != nil {
			logging.Warningf(ctx, "reachable: failed to read root invocation %s: %s", root, err)
		} else if state == resultpb.Invocation_FINALIZED {
			// Only populate the cache if the invocation exists and is
			// finalized.
			reachCache(root).TryWrite(ctx, reachable)
		}
	}

	logging.Debugf(ctx, "%d invocations are reachable from %s", len(reachable), roots.Names())
	return reachable, nil
}

// ReachCache is a cache of all invocations reachable from the given
// invocation, stored in Redis. The cached set is either correct or absent.
//
// The cache must be written only after the set of reachable invocations
// becomes immutable, i.e. when the including invocation is finalized.
// This is important to be able to tolerate transient Redis failures
// and avoid a situation where we failed to update the currently stored set,
// ignored the failure and then, after Redis came back online, read the
// stale set.
type reachCache invocations.ID

// key returns the Redis key.
func (c reachCache) key() string {
	return fmt.Sprintf("reach2:%s", c)
}

// Write writes the new value.
// The value does not have to include c, this is implied.
func (c reachCache) Write(ctx context.Context, value ReachableInvocations) (err error) {
	ctx, ts := trace.StartSpan(ctx, "resultdb.reachCache.write")
	ts.Attribute("id", string(c))
	defer func() { ts.End(err) }()

	// Expect the set of reachable invocations to include the invocation
	// for which the cache entry is.
	if _, ok := value[invocations.ID(c)]; !ok {
		return errors.New("value is invalid, does not contain the root invocation itself")
	}

	conn, err := redisconn.Get(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	key := c.key()

	marshaled, err := value.marshal()
	if err != nil {
		return errors.Annotate(err, "marshal").Err()
	}
	ts.Attribute("size", len(marshaled))

	if err := conn.Send("SET", key, marshaled); err != nil {
		return err
	}
	if err := conn.Send("EXPIRE", key, int(reachCacheExpiration.Seconds())); err != nil {
		return err
	}
	_, err = conn.Do("")
	return err
}

// TryWrite tries to write the new value. On failure, logs it.
func (c reachCache) TryWrite(ctx context.Context, value ReachableInvocations) {
	switch err := c.Write(ctx, value); {
	case err == redisconn.ErrNotConfigured:

	case err != nil:
		logging.Warningf(ctx, "ReachCache: failed to write %s: %s", c, err)
	}
}

// ErrUnknownReach is returned by ReachCache.Read if the cached value is absent.
var ErrUnknownReach = fmt.Errorf("the reachable set is unknown")

// Read reads the current value.
// Returns ErrUnknownReach if the value is absent.
//
// If err is nil, ids includes c, even if it was not passed in Write().
func (c reachCache) Read(ctx context.Context) (invs ReachableInvocations, err error) {
	ctx, ts := trace.StartSpan(ctx, "resultdb.reachCache.read")
	ts.Attribute("id", string(c))
	defer func() { ts.End(err) }()

	conn, err := redisconn.Get(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	b, err := redis.Bytes(conn.Do("GET", c.key()))
	switch {
	case err == redis.ErrNil:
		return nil, ErrUnknownReach
	case err != nil:
		return nil, err
	}
	ts.Attribute("size", len(b))

	return unmarshalReachableInvocations(b)
}
