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
	"sort"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/gomodule/redigo/redis"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tracing"
	pb "go.chromium.org/luci/resultdb/proto/v1"
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
var TooManyTag = errtag.Make("too many matching invocations matched the condition", true)

// Reachable returns all invocations reachable from roots along the inclusion
// edges. May return an appstatus-annotated error.
func Reachable(ctx context.Context, roots invocations.IDSet) (ReachableInvocations, error) {
	// TODO (nqmtuan): useRootCache should be set to true.
	invs, err := reachable(ctx, roots, true)
	if err != nil {
		return ReachableInvocations{}, err
	}
	return invs, nil
}

// ReachableSkipRootCache is similar to Reachable, but it ignores cache
// for the roots.
//
// Useful to keep cache-hit stats high in cases where the roots are known not to
// have cache.
func ReachableSkipRootCache(ctx context.Context, roots invocations.IDSet) (ReachableInvocations, error) {
	invs, err := reachable(ctx, roots, false)
	if err != nil {
		return ReachableInvocations{}, err
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
		return ReachableInvocations{}, err
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
		} else if state == pb.Invocation_FINALIZED {
			// Only populate the cache if the invocation exists and is
			// finalized.
			reachCache(root).TryWrite(ctx, uncachedReachable)
		}
	}

	logging.Debugf(ctx, "%d invocations are reachable from %s", len(reachable.Invocations), roots.Names())
	return reachable, nil
}

// reachableUncached queries the Spanner database for the reachability graph if the data is not in the reach cache.
func reachableUncached(ctx context.Context, roots invocations.IDSet) (ri ReachableInvocations, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.graph.reachable")
	defer func() { tracing.End(ts, err) }()

	reachableInvocations := invocations.NewIDSet()
	reachableInvocations.Union(roots)

	// Stores a mapping from reachable invocations to the invocation
	// they were included by. Roots are not captured.
	// If the same invocation is included by two or more invocations,
	// only one of them is recorded as the parent. The exact
	// parent invocation selected (if multiple are possible) is not
	// defined, but it is guaranteed that following parents will
	// eventually lead to a root (i.e. there are no cycles in the
	// parent graph).
	reachableInvocationToParent := make(map[invocations.ID]invocations.ID)

	// Map back from parent to child invocations.
	includedMap := map[invocations.ID][]invocations.ID{}

	// Find all reachable invocations traversing the graph one level at a time.
	nextLevel := invocations.NewIDSet()
	nextLevel.Union(roots)
	for len(nextLevel) > 0 {
		includedInvs, err := queryIncludedInvocations(ctx, nextLevel, includedMap)
		if err != nil {
			return ReachableInvocations{}, err
		}

		nextLevel = invocations.NewIDSet()
		for inv, invParent := range includedInvs {
			// Avoid duplicate lookups and cycles.
			if _, ok := reachableInvocations[inv]; ok {
				continue
			}

			nextLevel.Add(inv)
			reachableInvocations.Add(inv)
			reachableInvocationToParent[inv] = invParent
		}
	}

	var withTestResults invocations.IDSet
	var withExonerations invocations.IDSet
	var invDetails map[invocations.ID]invocationDetails

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
		invDetails, err = queryInvocationDetails(ctx, reachableInvocations)
		if err != nil {
			return errors.Annotate(err, "querying realms of reachable invocations").Err()
		}
		return nil
	})
	if err := eg.Wait(); err != nil {
		return ReachableInvocations{}, err
	}

	sortIncludedMap(includedMap)

	// Limit the returned reachable invocations to those that exist in the
	// Invocations table; they will have a realm.
	invs := make(map[invocations.ID]ReachableInvocation, len(reachableInvocations))
	distinctSources := make(map[SourceHash]*pb.Sources)
	for id, details := range invDetails {
		includedIds := []invocations.ID{}
		if _, ok := includedMap[id]; ok {
			includedIds = includedMap[id]
		}
		inv := ReachableInvocation{
			HasTestResults:        withTestResults.Has(id),
			HasTestExonerations:   withExonerations.Has(id),
			Realm:                 details.Realm,
			Instructions:          details.Instructions,
			IncludedInvocationIDs: includedIds,
		}

		sources := resolveSources(id, reachableInvocationToParent, invDetails)
		if sources != nil {
			sourceHash := HashSources(sources)
			distinctSources[sourceHash] = sources
			inv.SourceHash = sourceHash
		}
		invs[id] = inv
	}

	r := ReachableInvocations{
		Invocations: invs,
		Sources:     distinctSources,
	}

	return r, nil
}

func sortIncludedMap(includedMap map[invocations.ID][]invocations.ID) {
	for invID, childrenInvIDs := range includedMap {
		sort.Slice(includedMap[invID], func(i, j int) bool {
			return childrenInvIDs[i] < childrenInvIDs[j]
		})
	}
}

// resolveSources resolves the sources tested by the given invocation.
func resolveSources(id invocations.ID, invToParent map[invocations.ID]invocations.ID, invToDetails map[invocations.ID]invocationDetails) *pb.Sources {
	// If the invocation specifies that it inherits sources,
	// walk the invocation graph back towards the root to
	// resolve the sources.
	invID := id
	details := invToDetails[invID]
	for details.InheritSources {
		var ok bool
		invID, ok = invToParent[invID]
		if !ok {
			// We have walked all the way back to the root,
			// and even the root indicates it is inheriting sources.
			// The actual sources cannot be resolved.
			return nil
		}
		details = invToDetails[invID]
	}
	if details.Sources != nil {
		// Sources found.
		return details.Sources
	}
	// The invocation we inheriting sources from
	// has no sources.
	return nil
}

type invocationDetails struct {
	Realm          string
	InheritSources bool
	Sources        *pb.Sources
	// Instructions will have its content set to empty string.
	Instructions *pb.Instructions
}

// queryInvocationDetails reads realm and source information
// for the given list of invocations.
func queryInvocationDetails(ctx context.Context, ids invocations.IDSet) (map[invocations.ID]invocationDetails, error) {
	st := spanner.NewStatement(`
		SELECT
			i.InvocationId,
			i.Realm,
			i.InheritSources,
			i.Sources,
			i.Instructions,
		FROM UNNEST(@invIDs) inv
		JOIN Invocations i
		ON i.InvocationId = inv`)
	st.Params = spanutil.ToSpannerMap(map[string]any{
		"invIDs": ids,
	})
	b := &spanutil.Buffer{}
	results := make(map[invocations.ID]invocationDetails)
	err := spanutil.Query(ctx, st, func(r *spanner.Row) error {
		var invocationID invocations.ID
		var realm spanner.NullString
		var inheritSources spanner.NullBool
		var sources spanutil.Compressed
		var instructions spanutil.Compressed
		if err := b.FromSpanner(r, &invocationID, &realm, &inheritSources, &sources, &instructions); err != nil {
			return err
		}
		var sourcesProto *pb.Sources
		if len(sources) > 0 {
			sourcesProto = &pb.Sources{}
			err := proto.Unmarshal(sources, sourcesProto)
			if err != nil {
				return err
			}
		}
		var instructionsProto *pb.Instructions
		if len(instructions) > 0 {
			instructionsProto = &pb.Instructions{}
			err := proto.Unmarshal(instructions, instructionsProto)
			if err != nil {
				return err
			}
			// We do not care about step instructions in this case.
			// Filter them out to reduce the memory consumption.
			instructionsProto = instructionutil.FilterInstructionType(instructionsProto, pb.InstructionType_TEST_RESULT_INSTRUCTION)
			instructionsProto = instructionutil.RemoveInstructionsContent(instructionsProto)
		}
		results[invocationID] = invocationDetails{
			Realm:          realm.StringVal,
			InheritSources: inheritSources.Valid && inheritSources.Bool,
			Sources:        sourcesProto,
			Instructions:   instructionsProto,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// queryIncludedInvocations returns the set of invocations
// included from invocations `ids`, as well as the invocation
// they were included from.
//
// The returned map has a key for each included invocation.
// The value corresponding to the key is the parent invocation.
//
// The same invocation can be included from multiple invocations,
// i.e. there are multiple parents, then the parent in the map
// is selected arbitrarily.
func queryIncludedInvocations(ctx context.Context, ids invocations.IDSet, includedMap map[invocations.ID][]invocations.ID) (map[invocations.ID]invocations.ID, error) {
	st := spanner.NewStatement(`
	SELECT
		ii.InvocationID,
		ii.IncludedInvocationID,
	FROM
		UNNEST(@invocations) inv
		JOIN IncludedInvocations ii ON inv = ii.InvocationID`)
	st.Params = spanutil.ToSpannerMap(spanutil.ToSpannerMap(map[string]any{
		"invocations": ids,
	}))
	results := make(map[invocations.ID]invocations.ID)

	b := &spanutil.Buffer{}
	err := span.Query(ctx, st).Do(func(r *spanner.Row) error {
		var invocationID invocations.ID
		var includedInvocationID invocations.ID
		if err := b.FromSpanner(r, &invocationID, &includedInvocationID); err != nil {
			return err
		}

		includedMap[invocationID] = append(includedMap[invocationID], includedInvocationID)

		if includingInvocationID, ok := results[includedInvocationID]; ok {
			// If this invocation was included via multiple paths,
			// keep just the one with the lexicographically first
			// invocation ID.
			if invocationID < includingInvocationID {
				results[includedInvocationID] = invocationID
			}
		} else {
			results[includedInvocationID] = invocationID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
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
	return fmt.Sprintf("reach4:%s", c)
}

// Write writes the new value.
// The value does not have to include c, this is implied.
func (c reachCache) Write(ctx context.Context, value ReachableInvocations) (err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.reachCache.write",
		attribute.String("id", string(c)),
	)
	defer func() { tracing.End(ts, err) }()

	// Expect the set of reachable invocations to include the invocation
	// for which the cache entry is.
	if _, ok := value.Invocations[invocations.ID(c)]; !ok {
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
	ts.SetAttributes(attribute.Int("size", len(marshaled)))

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
	ctx, ts := tracing.Start(ctx, "resultdb.reachCache.read",
		attribute.String("id", string(c)),
	)
	defer func() { tracing.End(ts, err) }()

	conn, err := redisconn.Get(ctx)
	if err != nil {
		return ReachableInvocations{}, err
	}
	defer conn.Close()

	b, err := redis.Bytes(conn.Do("GET", c.key()))
	switch {
	case err == redis.ErrNil:
		return ReachableInvocations{}, ErrUnknownReach
	case err != nil:
		return ReachableInvocations{}, err
	}
	ts.SetAttributes(attribute.Int("size", len(b)))

	return unmarshalReachableInvocations(b)
}
