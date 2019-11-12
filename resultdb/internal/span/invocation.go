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

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

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

// InvocationIDFromRowID converts a Spanner-level row ID to an InvocationID.
func InvocationIDFromRowID(rowID string) InvocationID {
	return InvocationID(stripHashPrefix(rowID))
}

// Name returns an invocation name.
func (id InvocationID) Name() string {
	return pbutil.InvocationName(string(id))
}

// RowID returns an invocation ID used in spanner rows.
func (id InvocationID) RowID() string {
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
	inv := &pb.Invocation{Name: id.Name()}

	eg, ctx := errgroup.WithContext(ctx)

	// Populate fields from Invocation table.
	eg.Go(func() error {
		return ReadInvocation(ctx, txn, id, map[string]interface{}{
			"State":           &inv.State,
			"CreateTime":      &inv.CreateTime,
			"FinalizeTime":    &inv.FinalizeTime,
			"Deadline":        &inv.Deadline,
			"BaseTestVariant": &inv.BaseTestVariant,
			"Tags":            &inv.Tags,
		})
	})

	// Populate included_invocations.
	eg.Go(func() error {
		included, err := ReadIncludedInvocations(ctx, txn, id)
		if err != nil {
			return err
		}
		inv.IncludedInvocations = make([]string, len(included))
		for i, id := range included {
			inv.IncludedInvocations[i] = id.Name()
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return inv, nil
}

// ReadReachableInvocations fetches invocations reachable from the roots.
// If the returned error is non-nil, it is annotated with a gRPC code.
func ReadReachableInvocations(ctx context.Context, txn *spanner.ReadOnlyTransaction, roots ...InvocationID) (map[InvocationID]*pb.Invocation, error) {
	ret := map[InvocationID]*pb.Invocation{}
	var mu sync.Mutex

	var visit func(id InvocationID)

	// read reads an invocation and calls visit for all invocations it includes.
	read := func(id InvocationID) (*pb.Invocation, error) {
		inv, err := ReadInvocationFull(ctx, txn, id)
		if err != nil {
			return nil, err
		}

		for _, name := range inv.IncludedInvocations {
			visit(MustParseInvocationName(name))
		}
		return inv, nil
	}

	eg, ctx := errgroup.WithContext(ctx)
	visit = func(id InvocationID) {
		mu.Lock()
		// Check if we already started/finished fetching this invocation.
		if _, ok := ret[id]; ok {
			mu.Unlock()
			return
		}

		// Mark the invocation as being fetched.
		ret[id] = nil
		mu.Unlock()

		// Concurrently fetch the invocation without a lock.
		// Then record it with a lock.
		eg.Go(func() error {
			inv, err := read(id)
			if err != nil {
				return err
			}

			mu.Lock()
			ret[id] = inv
			mu.Unlock()
			return nil
		})
	}

	// Trigger fetching by requesting all roots.
	for _, id := range roots {
		visit(id)
	}

	// Wait for the entire graph to be fetched.
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return ret, nil
}
