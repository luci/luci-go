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
	"fmt"
	"sync"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/appstatus"
	"go.chromium.org/luci/resultdb/internal/metrics"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// InvocationShards is the sharding level for the Invocations table.
// Column Invocations.ShardId is a value in range [0, InvocationShards).
const InvocationShards = 100

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
		return appstatus.Attachf(err, codes.NotFound, "%s not found", id.Name())

	case err != nil:
		return errors.Annotate(err, "failed to fetch %s", id.Name()).Err()

	default:
		return nil
	}
}

// TooManyInvocationsTag set in an error indicates that too many invocations
// matched a condition.
var TooManyInvocationsTag = errors.BoolTag{
	Key: errors.NewTagKey("too many matching invocations matched the condition"),
}

// ReadReachableInvocations returns a transitive closure of roots.
// If the returned error is non-nil, it is annotated with a gRPC code.
//
// limit must be positive. If the size of the transitive closure exceeds the
// limit, returns an error tagged with TooManyInvocationsTag.
func ReadReachableInvocations(ctx context.Context, txn Txn, limit int, roots InvocationIDSet) (InvocationIDSet, error) {
	defer metrics.Trace(ctx, "ReadReachableInvocations")()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if limit <= 0 {
		panic("limit <= 0")
	}
	if len(roots) > limit {
		panic("len(roots) > limit")
	}

	ret := make(InvocationIDSet, len(roots))
	var mu sync.Mutex
	var visit func(id InvocationID) error

	sem := semaphore.NewWeighted(64)
	eg, ctx := errgroup.WithContext(ctx)
	visit = func(id InvocationID) error {
		mu.Lock()
		defer mu.Unlock()

		// Check if we already started/finished fetching this invocation.
		if ret.Has(id) {
			return nil
		}

		// Consider fetching a new invocation.
		if len(ret) == limit {
			cancel()
			return errors.Reason("more than %d invocations match", limit).Tag(TooManyInvocationsTag).Err()
		}

		// Mark the invocation as being processed.
		ret.Add(id)

		// Concurrently fetch the inclusions without a lock.
		eg.Go(func() error {
			if err := sem.Acquire(ctx, 1); err != nil {
				return err
			}
			included, err := ReadIncludedInvocations(ctx, txn, id)
			sem.Release(1)
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
	return ret, nil
}

// ReadInvocationFull reads one invocation struct from Spanner.
// If the invocation does not exist, the returned error is annotated with
// NotFound GRPC code.
func ReadInvocationFull(ctx context.Context, txn Txn, id InvocationID) (*pb.Invocation, error) {
	invs, err := ReadInvocationsFull(ctx, txn, NewInvocationIDSet(id))
	if err != nil {
		return nil, err
	}
	return invs[id], nil
}

// ReadInvocationsFull returns multiple invocations.
// If any of them are not found, returns an error.
func ReadInvocationsFull(ctx context.Context, txn Txn, ids InvocationIDSet) (map[InvocationID]*pb.Invocation, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	st := spanner.NewStatement(`
		SELECT
		 i.InvocationId,
		 i.State,
		 i.CreateTime,
		 i.FinalizeTime,
		 i.Deadline,
		 i.Tags,
		 i.Interrupted,
		 ARRAY(SELECT IncludedInvocationId FROM IncludedInvocations incl WHERE incl.InvocationID = i.InvocationId)
		FROM Invocations i
		WHERE i.InvocationID IN UNNEST(@invIDs)
	`)
	st.Params = ToSpannerMap(map[string]interface{}{
		"invIDs": ids,
	})
	ret := make(map[InvocationID]*pb.Invocation, len(ids))
	var b Buffer
	err := Query(ctx, fmt.Sprintf("full invocations %s", ids), txn, st, func(row *spanner.Row) error {
		var id InvocationID
		included := InvocationIDSet{}
		inv := &pb.Invocation{}
		err := b.FromSpanner(row,
			&id,
			&inv.State,
			&inv.CreateTime,
			&inv.FinalizeTime,
			&inv.Deadline,
			&inv.Tags,
			&inv.Interrupted,
			&included)
		if err != nil {
			return err
		}
		inv.Name = pbutil.InvocationName(string(id))
		inv.IncludedInvocations = included.Names()
		if _, ok := ret[id]; ok {
			panic("query is incorrect; it returned duplicated invocation IDs")
		}
		ret[id] = inv
		return nil
	})
	if err != nil {
		return nil, err
	}
	for id := range ids {
		if _, ok := ret[id]; !ok {
			return nil, appstatus.Errorf(codes.NotFound, "%s not found", id.Name())
		}
	}
	return ret, nil
}

// ReadInvocationState returns the invocation's state.
func ReadInvocationState(ctx context.Context, txn Txn, id InvocationID) (pb.Invocation_State, error) {
	var state pb.Invocation_State
	err := ReadInvocation(ctx, txn, id, map[string]interface{}{"State": &state})
	return state, err
}

// ReadInvocationRealm returns the invocation's realm.
func ReadInvocationRealm(ctx context.Context, txn Txn, id InvocationID) (string, error) {
	var realm string
	err := ReadInvocation(ctx, txn, id, map[string]interface{}{"Realm": &realm})
	return realm, err
}
