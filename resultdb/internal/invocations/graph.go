// Copyright 2020 The LUCI Authors.
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

package invocations

import (
	"context"
	"fmt"
	"sync"

	"github.com/gomodule/redigo/redis"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/server/redisconn"

	"go.chromium.org/luci/resultdb/internal/span"
)

// GraphSizeLimit is the maximum graph size (in terms of nodes) that ResultDB
// can operate on.
const GraphSizeLimit = 10000

// InclusionKey returns a spanner key for an Inclusion row.
func InclusionKey(including, included ID) spanner.Key {
	return spanner.Key{including.RowID(), included.RowID()}
}

// ReadIncluded reads ids of included invocations.
func ReadIncluded(ctx context.Context, txn span.Txn, id ID) (IDSet, error) {
	var ret IDSet
	var b span.Buffer
	err := txn.Read(ctx, "IncludedInvocations", id.Key().AsPrefix(), []string{"IncludedInvocationId"}).Do(func(row *spanner.Row) error {
		var included ID
		if err := b.FromSpanner(row, &included); err != nil {
			return err
		}
		if ret == nil {
			ret = make(IDSet)
		}
		ret.Add(included)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// TooManyTag set in an error indicates that too many invocations
// matched a condition.
var TooManyTag = errors.BoolTag{
	Key: errors.NewTagKey("too many matching invocations matched the condition"),
}

// Reachable returns a transitive closure of roots.
// If the returned error is non-nil, it is annotated with a gRPC code.
func Reachable(ctx context.Context, txn span.Txn, roots IDSet) (reachable IDSet, err error) {
	ctx, ts := trace.StartSpan(ctx, "resultdb.readReachableInvocations")
	defer func() { ts.End(err) }()

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	reachable = make(IDSet, len(roots))
	var mu sync.Mutex
	var visit func(id ID) error

	spanSem := semaphore.NewWeighted(64) // limits Spanner RPCs concurrency.

	visit = func(id ID) error {
		mu.Lock()
		defer mu.Unlock()

		// Check if we already started/finished fetching this invocation.
		if reachable.Has(id) {
			return nil
		}

		// Consider fetching a new invocation.
		if len(reachable) == GraphSizeLimit {
			cancel()
			return errors.Reason("more than %d invocations match", GraphSizeLimit).Tag(TooManyTag).Err()
		}

		// Mark the invocation as being processed.
		reachable.Add(id)

		// Concurrently fetch the inclusions without a lock.
		eg.Go(func() error {
			// First check the cache.
			switch ids, err := ReachCache(id).Read(ctx); {
			case err == redisconn.ErrNotConfigured || err == ErrUnknownReach:
				// Ignore this error.
			case err != nil:
				logging.Warningf(ctx, "ReachCache: failed to read: %s", err)
			default:
				// Cache hit. Copy the results to `reachable` and exit without
				// recursion.
				mu.Lock()
				defer mu.Unlock()
				reachable.Union(ids)
				return nil
			}

			if err := spanSem.Acquire(ctx, 1); err != nil {
				return err
			}
			included, err := ReadIncluded(ctx, txn, id)
			spanSem.Release(1)
			if err != nil {
				return err
			}

			for id := range included {
				if err := visit(id); err != nil {
					return err
				}
			}
			return nil
		})
		return nil
	}

	// Trigger fetching by requesting all roots.
	for id := range roots {
		if err := visit(id); err != nil {
			return nil, err
		}
	}

	// Wait for the entire graph to be fetched.
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	logging.Debugf(ctx, "%d invocations are reachable from %s", len(reachable), roots.Names())
	return reachable, nil
}

// ReachCache is cache of invocations reachable from the given
// invocation, stored in Redis. The cached set is either correct or absent.
//
// The cache must be written only after the set of reachable invocations
// becomes immutable, i.e. when the including invocation is finalized.
// This is important to be able to tolerate transient Redis failures
// and avoid a situation where we failed to update the currently stored set,
// ignored the failure and then, after Redis came back online, read the
// incorrect set.
type ReachCache ID

// key returns the Redis key.
func (c ReachCache) key() string {
	return fmt.Sprintf("reach:%s", c)
}

// ErrUnknownReach is returned by ReachCache.Read if the
// set is not available.
var ErrUnknownReach = fmt.Errorf("the reachable set is unknown")

// Read reads the current value into dest.
// Returns ErrUnknownReach if the value is absent.
func (c ReachCache) Read(ctx context.Context) (ids IDSet, err error) {
	ctx, ts := trace.StartSpan(ctx, "resultdb.reachCache.read")
	ts.Attribute("id", string(c))
	defer func() { ts.End(err) }()

	conn, err := redisconn.Get(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	key := c.key()
	conn.Send("MULTI")
	conn.Send("EXISTS", key)
	conn.Send("SMEMBERS", key)
	vs, err := redis.Values(conn.Do("EXEC"))
	if err != nil {
		return nil, err
	}
	if exists := vs[0].(int64); exists == 0 {
		return nil, ErrUnknownReach
	}

	members := vs[1].([]interface{})
	ids = make(IDSet, len(ids))
	for _, id := range members {
		ids.Add(ID(id.([]byte)))
	}
	ts.Attribute("size", len(ids))
	return ids, nil
}

// Write writes the new value.
func (c ReachCache) Write(ctx context.Context, value IDSet) (err error) {
	ctx, ts := trace.StartSpan(ctx, "resultdb.reachCache.write")
	ts.Attribute("id", string(c))
	ts.Attribute("size", len(value))
	defer func() { ts.End(err) }()

	conn, err := redisconn.Get(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	key := c.key()
	sAddArgs := make(redis.Args, 0, 1+len(value)).
		Add(key).
		AddFlat(value.Strings())

	conn.Send("MULTI")
	conn.Send("DEL", key)
	if len(value) > 0 {
		conn.Send("SADD", sAddArgs...)
	}
	_, err = conn.Do("EXEC")
	return err
}

// TryWrite tries to write the new value. On failure, logs the error.
func (c ReachCache) TryWrite(ctx context.Context, value IDSet) {
	switch err := c.Write(ctx, value); {
	case err == redisconn.ErrNotConfigured:

	case err != nil:
		logging.Warningf(ctx, "ReachCache: failed to write: %s", err)
	}
}
