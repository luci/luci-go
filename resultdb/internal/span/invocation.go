// Copyright 2019 The LUCI Authors.
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

package span

import (
	"context"
	"sort"
	"sync"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/metrics"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// InvocationShards is the sharding level for the Invocations table.
// Column Invocations.ShardId is a value in range [0, InvocationShards).
const InvocationShards = 100

// InvocationID can convert an invocation id to various formats.
type InvocationID string

// SortInvocationIDs sorts ids lexicographically.
func SortInvocationIDs(ids []InvocationID) {
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
}

// MustParseInvocationName converts an invocation name to an InvocationID.
// Panics if the name is invalid. Useful for situations when name was already
// validated.
func MustParseInvocationName(name string) InvocationID {
	id, err := pbutil.ParseInvocationName(name)
	if err != nil {
		panic(err)
	}
	return InvocationID(id)
}

// MustParseInvocationNames converts invocation names to InvocationIDs.
// Panics if a name is invalid. Useful for situations when names were already
// validated.
func MustParseInvocationNames(names []string) []InvocationID {
	ids := make([]InvocationID, len(names))
	for i, name := range names {
		ids[i] = MustParseInvocationName(name)
	}
	return ids
}

// InvocationIDFromRowID converts a Spanner-level row ID to an InvocationID.
func InvocationIDFromRowID(rowID string) InvocationID {
	return InvocationID(stripHashPrefix(rowID))
}

// Name returns an invocation name.
func (id InvocationID) Name() string {
	return pbutil.InvocationName(string(id))
}

// RowID returns an invocation ID used in spanner rows.
// If id is empty, returns "".
func (id InvocationID) RowID() string {
	if id == "" {
		return ""
	}
	return prefixWithHash(string(id))
}

// Key returns a invocation spanner key.
func (id InvocationID) Key(suffix ...interface{}) spanner.Key {
	ret := make(spanner.Key, 1+len(suffix))
	ret[0] = id.RowID()
	copy(ret[1:], suffix)
	return ret
}

// ReadInvocation reads one invocation from Spanner.
// If the invocation does not exist, the returned error is annotated with
// NotFound GRPC code.
// For ptrMap see ReadRow comment in util.go.
func ReadInvocation(ctx context.Context, txn Txn, id InvocationID, ptrMap map[string]interface{}) error {
	if id == "" {
		return errors.Reason("id is unspecified").Err()
	}
	err := ReadRow(ctx, txn, "Invocations", id.Key(), ptrMap)
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return errors.Reason("%q not found", id.Name()).
			InternalReason("%s", err).
			Tag(grpcutil.NotFoundTag).
			Err()

	case err != nil:
		return errors.Annotate(err, "failed to fetch %q", id.Name()).Err()

	default:
		return nil
	}
}

// ReadInvocationFull reads one invocation struct from Spanner.
// If the invocation does not exist, the returned error is annotated with
// NotFound GRPC code.
func ReadInvocationFull(ctx context.Context, txn Txn, id InvocationID) (*pb.Invocation, error) {
	name := id.Name()
	pred := &pb.InvocationPredicate{
		Names:            []string{name},
		IgnoreInclusions: true,
	}
	invs, err := QueryInvocations(ctx, txn, pred, 1)
	switch {
	case err != nil:
		return nil, err
	case len(invs) == 0:
		return nil, errors.Reason("%q not found", name).Tag(grpcutil.NotFoundTag).Err()
	}
	return invs[id], nil
}

// TooManyInvocationsTag set in an error indicates that too many invocations
// matched a condition.
var TooManyInvocationsTag = errors.BoolTag{
	Key: errors.NewTagKey("too many matching invocations matched the condition"),
}

// ReadReachableInvocations fetches all invocations reachable from the roots.
// If the returned error is non-nil, it is annotated with a gRPC code.
//
// limit must be positive.
// If the number of matching invocations exceeds limit, returns an error
// tagged with TooManyInvocationsTag.
//
// Does not re-fetch roots.
func ReadReachableInvocations(ctx context.Context, txn Txn, limit int, roots map[InvocationID]*pb.Invocation) (map[InvocationID]*pb.Invocation, error) {
	defer metrics.Trace(ctx, "ReadReachableInvocations")()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if limit <= 0 {
		panic("limit <= 0")
	}
	if len(roots) > limit {
		panic("len(roots) > limit")
	}
	ret := make(map[InvocationID]*pb.Invocation, limit)
	for id, inv := range roots {
		ret[id] = inv
	}

	var mu sync.Mutex
	var visit func(id InvocationID) error

	visitIncluded := func(inv *pb.Invocation) error {
		for _, name := range inv.IncludedInvocations {
			if err := visit(MustParseInvocationName(name)); err != nil {
				return err
			}
		}
		return nil
	}

	eg, ctx := errgroup.WithContext(ctx)
	visit = func(id InvocationID) error {
		mu.Lock()
		defer mu.Unlock()

		// Check if we already started/finished fetching this invocation.
		if _, ok := ret[id]; ok {
			return nil
		}

		// Consider fetching a new invocation.
		if len(ret) == limit {
			cancel()
			return errors.Reason("more than %d invocations match", limit).Tag(TooManyInvocationsTag).Err()
		}

		// Mark the invocation as being fetched.
		ret[id] = nil

		// Concurrently fetch the invocation without a lock.
		// Then record it with a lock.
		eg.Go(func() error {
			inv, err := ReadInvocationFull(ctx, txn, id)
			if err != nil {
				return err
			}

			mu.Lock()
			ret[id] = inv
			mu.Unlock()

			return visitIncluded(inv)
		})
		return nil
	}

	// Trigger fetching by requesting all roots.
	for _, inv := range roots {
		if err := visitIncluded(inv); err != nil {
			return nil, err
		}
	}

	// Wait for the entire graph to be fetched.
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return ret, nil
}

// QueryInvocations returns invocations that satisfy the predicate.
// Assumes pred is valid.
//
// Does not support paging.
// The limit must be positive.
// If the number of matching invocations exceeds the limit, returns an error
// tagged with TooManyInvocationsTag.
func QueryInvocations(ctx context.Context, txn Txn, pred *pb.InvocationPredicate, limit int) (map[InvocationID]*pb.Invocation, error) {
	switch {
	case limit <= 0:
		panic("limit <= 0")
	case len(pred.GetNames()) == 0 && len(pred.GetTags()) == 0:
		panic("names and tags are empty")
	}

	// Fetch the root invocations.
	st := spanner.NewStatement(`
		SELECT
		 i.InvocationId,
		 i.State,
		 i.CreateTime,
		 i.FinalizeTime,
		 i.Deadline,
		 i.Tags,
		 ARRAY(SELECT IncludedInvocationId FROM IncludedInvocations incl WHERE incl.InvocationID = i.InvocationId)
		FROM Invocations i
		WHERE
			i.InvocationID IN UNNEST(@invIDs) OR
			i.InvocationID IN (SELECT t.InvocationID from InvocationsByTag t WHERE t.TagID IN UNNEST(@tagIDs)
			)
		LIMIT @limit
	`)
	st.Params = ToSpannerMap(map[string]interface{}{
		"limit":  limit + 1, // request one more to detect overflow
		"invIDs": MustParseInvocationNames(pred.GetNames()),
		"tagIDs": TagRowIDs(pred.GetTags()...),
	})
	roots := make(map[InvocationID]*pb.Invocation, limit)
	var b Buffer
	err := query(ctx, txn, st, func(row *spanner.Row) error {
		if len(roots) == limit {
			return errors.Reason("more than %d invocations match the predicate", limit).Tag(TooManyInvocationsTag).Err()
		}
		var id InvocationID
		inv := &pb.Invocation{}
		var included []InvocationID
		err := b.FromSpanner(row,
			&id,
			&inv.State,
			&inv.CreateTime,
			&inv.FinalizeTime,
			&inv.Deadline,
			&inv.Tags,
			&included)
		if err != nil {
			return err
		}
		inv.Name = pbutil.InvocationName(string(id))
		inv.IncludedInvocations = make([]string, len(included))
		for i, id := range included {
			inv.IncludedInvocations[i] = id.Name()
		}
		sort.Strings(inv.IncludedInvocations)
		if _, ok := roots[id]; ok {
			panic("query is incorect; it returned duplicated invocation IDs")
		}
		roots[id] = inv
		return nil
	})
	if err != nil {
		return nil, err
	}

	if pred.IgnoreInclusions {
		return roots, nil
	}

	return ReadReachableInvocations(ctx, txn, limit, roots)
}
