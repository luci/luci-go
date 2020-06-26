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
	"sync"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/trace"

	"go.chromium.org/luci/resultdb/internal/span"
)

// MaxNodes is the maximum number of invocation nodes that ResultDB
// can operate on at a time.
const MaxNodes = 10000

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

// Reachable returns all invocations reachable from roots along the inclusion
// edges.
// May return an appstatus-annotated error.
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

	sem := semaphore.NewWeighted(64)
	visit = func(id ID) error {
		mu.Lock()
		defer mu.Unlock()

		// Check if we already started/finished fetching this invocation.
		if reachable.Has(id) {
			return nil
		}

		// Consider fetching a new invocation.
		if len(reachable) == MaxNodes {
			cancel()
			return errors.Reason("more than %d invocations match", MaxNodes).Tag(TooManyTag).Err()
		}

		// Mark the invocation as being processed.
		reachable.Add(id)

		// Concurrently fetch the inclusions without a lock.
		eg.Go(func() error {
			if err := sem.Acquire(ctx, 1); err != nil {
				return err
			}
			included, err := ReadIncluded(ctx, txn, id)
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
	logging.Debugf(ctx, "%d invocations are reachable from %s", len(reachable), roots.Names())
	return reachable, nil
}
