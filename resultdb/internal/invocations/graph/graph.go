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
	"time"

	"cloud.google.com/go/spanner"
	"github.com/gomodule/redigo/redis"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
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
	reachable = NewReachableInvocations()
	uncachedRoots := invocations.NewIDSet()
	if useRootCache {
		for id := range roots {
			// First check the cache.
			switch reachables, err := reachCache(id).Read(ctx); {
			case err == redisconn.ErrNotConfigured || err == ErrUnknownReach:
				// Ignore this error.
				uncachedRoots.Add(id)
			case err != nil:
				logging.Warningf(ctx, "ReachCache: failed to read %s: %s", id, err)
				uncachedRoots.Add(id)
			default:
				// Cache hit. Copy the results to `reachable`.
				reachable.Union(reachables)
			}
		}
	} else {
		uncachedRoots.Union(roots)
	}

	if len(uncachedRoots) == 0 {
		return reachable, nil
	}

	uncachedReachable, err := reachableUncached(ctx, uncachedRoots)
	if err != nil {
		return nil, err
	}
	reachable.Union(uncachedReachable)

	// If we queried for one root and we had a cache miss, try to insert the
	// reachable invocations, so that the cache will hopefully be populated
	// next time.
	if len(uncachedRoots) == 1 {
		var root invocations.ID
		for id := range uncachedRoots {
			root = id
		}
		state, err := invocations.ReadState(ctx, root)
		if err != nil {
			logging.Warningf(ctx, "reachable: failed to read root invocation %s: %s", root, err)
		} else if state == resultpb.Invocation_FINALIZED {
			// Only populate the cache if the invocation exists and is
			// finalized.
			reachCache(root).TryWrite(ctx, uncachedReachable)
		}
	}

	logging.Debugf(ctx, "%d invocations are reachable from %s", len(reachable), roots.Names())
	return reachable, nil
}

// reachableUncached queries the Spanner database for the reachability graph if the data is not in the reach cache.
func reachableUncached(ctx context.Context, roots invocations.IDSet) (reachable ReachableInvocations, err error) {
	ctx, ts := trace.StartSpan(ctx, "resultdb.graph.reachable")
	defer func() { ts.End(err) }()

	reachableInvocations := invocations.NewIDSet()
	reachableInvocations.Union(roots)

	// Find all reachable invocations traversing the graph one level at a time.
	nextLevel := invocations.NewIDSet()
	nextLevel.Union(roots)
	for len(nextLevel) > 0 {
		nextLevel, err = queryInvocations(ctx, `
		SELECT
			ii.IncludedInvocationID,
		FROM
			UNNEST(@invocations) inv
			JOIN IncludedInvocations ii ON inv = ii.InvocationID`, nextLevel)
		if err != nil {
			return nil, err
		}

		// Avoid duplicate lookups and cycles.
		nextLevel.RemoveAll(reachableInvocations)
		reachableInvocations.Union(nextLevel)
	}

	var withTestResults invocations.IDSet
	var withExonerations invocations.IDSet
	var realms map[invocations.ID]string

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		var err error
		withTestResults, err = queryInvocations(ctx, `SELECT DISTINCT tr.InvocationID FROM UNNEST(@invocations) inv JOIN TestResults tr on tr.InvocationId = inv`, reachableInvocations)
		if err != nil {
			return errors.Annotate(err, "querying invocations with test results").Err()
		}
		return nil
	})
	eg.Go(func() error {
		var err error
		withExonerations, err = queryInvocations(ctx, `SELECT DISTINCT te.InvocationID FROM UNNEST(@invocations) inv JOIN TestExonerations te on te.InvocationId = inv`, reachableInvocations)
		if err != nil {
			return errors.Annotate(err, "querying invocations with test exonerations").Err()
		}
		return nil
	})
	eg.Go(func() error {
		var err error
		realms, err = invocations.QueryRealms(ctx, reachableInvocations)
		if err != nil {
			return errors.Annotate(err, "querying realms of reachable invocations").Err()
		}
		return nil
	})
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	// Limit the returned reachable invocations to those that exist in the
	// Invocations table; they will have a realm.
	reachable = make(ReachableInvocations, len(realms))
	for inv, realm := range realms {
		reachable[inv] = ReachableInvocation{
			HasTestResults:      withTestResults.Has(inv),
			HasTestExonerations: withExonerations.Has(inv),
			Realm:               realm,
		}
	}
	return reachable, nil
}

func queryInvocations(ctx context.Context, query string, invocationsParam invocations.IDSet) (invocations.IDSet, error) {
	invs := invocations.NewIDSet()
	st := spanner.NewStatement(query)
	st.Params = spanutil.ToSpannerMap(spanutil.ToSpannerMap(map[string]any{
		"invocations": invocationsParam,
	}))
	b := &spanutil.Buffer{}
	err := span.Query(ctx, st).Do(func(r *spanner.Row) error {
		var invocationID invocations.ID
		if err := b.FromSpanner(r, &invocationID); err != nil {
			return err
		}
		invs.Add(invocationID)
		return nil
	})
	return invs, err
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
	return fmt.Sprintf("reach3:%s", c)
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
