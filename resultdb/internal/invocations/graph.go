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
	"bytes"
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
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/spanutil"
)

// MaxNodes is the maximum number of invocation nodes that ResultDB
// can operate on at a time.
const MaxNodes = 20000

// reachCacheExpiration is expiration duration of ReachCache.
// It is more important to have *some* expiration; the value itself matters less
// because Redis evicts LRU keys only with *some* expiration set,
// see volatile-lru policy: https://redis.io/topics/lru-cache
const reachCacheExpiration = 30 * 24 * time.Hour // 30 days

// InclusionKey returns a spanner key for an Inclusion row.
func InclusionKey(including, included ID) spanner.Key {
	return spanner.Key{including.RowID(), included.RowID()}
}

// ReadIncluded reads ids of included invocations.
func ReadIncluded(ctx context.Context, id ID) (IDSet, error) {
	var ret IDSet
	var b spanutil.Buffer
	err := span.Read(ctx, "IncludedInvocations", id.Key().AsPrefix(), []string{"IncludedInvocationId"}).Do(func(row *spanner.Row) error {
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

// Reachable returns all invocations reachable from roots along the inclusion
// edges.
// May return an appstatus-annotated error.
func Reachable(ctx context.Context, roots IDSet) (IDSet, error) {
	return reachable(ctx, roots, true)
}

// ReachableSkipRootCache is similar to Reachable, but it ignores cache
// for the roots.
//
// Useful to keep cache-hit stats high in cases where the roots are known not to
// have cache.
func ReachableSkipRootCache(ctx context.Context, roots IDSet) (IDSet, error) {
	return reachable(ctx, roots, false)
}

func reachable(ctx context.Context, roots IDSet, useRootCache bool) (reachable IDSet, err error) {
	ctx, ts := trace.StartSpan(ctx, "resultdb.readReachableInvocations")
	defer func() { ts.End(err) }()

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	tooMany := func() error {
		return errors.Reason("more than %d invocations match", MaxNodes).Tag(TooManyTag).Err()
	}

	reachable = make(IDSet, len(roots))
	var mu sync.Mutex

	spanSem := semaphore.NewWeighted(64) // limits Spanner RPC concurrency.

	var visit func(id ID, useCache bool) error
	visit = func(id ID, useCache bool) error {
		mu.Lock()
		defer mu.Unlock()

		// Check if we already started/finished fetching this invocation.
		if reachable.Has(id) {
			return nil
		}

		// Consider fetching a new invocation.
		if len(reachable) == MaxNodes {
			return tooMany()
		}

		// Mark the invocation as being processed.
		reachable.Add(id)

		// Concurrently fetch the inclusions without a lock.
		eg.Go(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}

			if useCache {
				// First check the cache.
				switch ids, err := ReachCache(id).Read(ctx); {
				case err == redisconn.ErrNotConfigured || err == ErrUnknownReach:
					// Ignore this error.
				case err != nil:
					logging.Warningf(ctx, "ReachCache: failed to read %s: %s", id, err)
				default:
					// Cache hit. Copy the results to `reachable` and exit without
					// recursion.
					mu.Lock()
					defer mu.Unlock()
					reachable.Union(ids)
					if len(reachable) > MaxNodes {
						return tooMany()
					}
					return nil
				}
				// Cache miss. => Read from Spanner.
			}

			if err := spanSem.Acquire(ctx, 1); err != nil {
				return err
			}
			included, err := ReadIncluded(ctx, id)
			spanSem.Release(1)
			if err != nil {
				return errors.Annotate(err, "failed to read inclusions of %s", id.Name()).Err()
			}

			for id := range included {
				// Always use cache for children.
				if err := visit(id, true); err != nil {
					return err
				}
			}
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
type ReachCache ID

// key returns the Redis key.
func (c ReachCache) key() string {
	return fmt.Sprintf("reach:%s", c)
}

// Write writes the new value.
// The value does not have to include c, this is implied.
func (c ReachCache) Write(ctx context.Context, value IDSet) (err error) {
	ctx, ts := trace.StartSpan(ctx, "resultdb.reachCache.write")
	ts.Attribute("id", string(c))
	defer func() { ts.End(err) }()

	conn, err := redisconn.Get(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	key := c.key()

	marshaled := &bytes.Buffer{}
	for id := range value {
		if id != ID(c) {
			fmt.Fprintln(marshaled, id)
		}
	}
	if marshaled.Len() == 0 {
		// Redis does not support empty values. Write just "\n".
		fmt.Fprintln(marshaled)
	}
	ts.Attribute("size", marshaled.Len())

	conn.Send("SET", key, marshaled.Bytes())
	conn.Send("EXPIRE", key, int(reachCacheExpiration.Seconds()))
	_, err = conn.Do("")
	return err
}

// TryWrite tries to write the new value. On failure, logs it.
func (c ReachCache) TryWrite(ctx context.Context, value IDSet) {
	switch err := c.Write(ctx, value); {
	case err == redisconn.ErrNotConfigured:

	case err != nil:
		logging.Warningf(ctx, "ReachCache: failed to write %s: %s", c, err)
	}
}

// ErrUnknownReach is returned by ReachCache.Read if the cached value is absent.
var ErrUnknownReach = fmt.Errorf("the reachable set is unknown")

var memberSep = []byte("\n")

// Read reads the current value.
// Returns ErrUnknownReach if the value is absent.
//
// If err is nil, ids includes c, even if it was not passed in Write().
func (c ReachCache) Read(ctx context.Context) (ids IDSet, err error) {
	ctx, ts := trace.StartSpan(ctx, "resultdb.reachCache.read")
	ts.Attribute("id", string(c))
	defer func() { ts.End(err) }()

	conn, err := redisconn.Get(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	members, err := redis.Bytes(conn.Do("GET", c.key()))
	switch {
	case err == redis.ErrNil:
		return nil, ErrUnknownReach
	case err != nil:
		return nil, err
	}
	ts.Attribute("size", len(members))

	split := bytes.Split(members, memberSep)
	ids = make(IDSet, len(split)+1)
	ids.Add(ID(c))
	for _, id := range split {
		if len(id) > 0 {
			ids.Add(ID(id))
		}
	}
	return ids, nil
}
