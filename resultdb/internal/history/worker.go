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

package history

import (
	"context"

	"github.com/golang/protobuf/ptypes/timestamp"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testresults"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// NWorkers is the number of concurrent workers fetching results to serve
	// a single request.
	NWorkers = 10
)

var rpcSemaphore = semaphore.NewWeighted(NWorkers)

// WorkItem represents a task to get test results reachable under a given
// invocation.
type WorkItem struct {
	inv      invocations.ID
	ts       *timestamp.Timestamp
	resultsC chan PageItem
	req      *pb.GetTestResultHistoryRequest
}

// DispatchWorkItems creates a task for each indexed invocation in the
// requested range as well as a channel to collect each task's result.
func DispatchWorkItems(ctx context.Context, req *pb.GetTestResultHistoryRequest, wiC chan<- WorkItem, piCC chan<- chan PageItem) error {
	if err := rpcSemaphore.Acquire(ctx, 1); err != nil {
		return err
	}
	defer rpcSemaphore.Release(1)
	return invocations.ByTimestamp(ctx, req.Realm, req.GetTimeRange(), func(inv invocations.ID, ts *timestamp.Timestamp) error {
		piC := make(chan PageItem)
		wiC <- WorkItem{inv, ts, piC, req}
		piCC <- piC
		return nil
	})
}

// Fetch starts workers that will fetch results for each invocation in `wiC`
// and stream them over the task's channel.
func Fetch(ctx context.Context, wiC <-chan WorkItem) error {
	eg, workerCtx := errgroup.WithContext(ctx)
	for i := 0; i < NWorkers; i++ {
		eg.Go(func() error {
			for {
				wi, ok := <-wiC
				if !ok {
					return nil
				}
				if err := fetchResults(workerCtx, wi); err != nil {
					return err
				}
			}
		})
	}
	return eg.Wait()
}

// Collect gets results from individual tasks, and streams them over the
// `globalResults` channel.
func Collect(ctx context.Context, piCC <-chan chan PageItem, globalResults chan<- PageItem) error {
	for piC := range piCC {
		items := make([]PageItem, 0)
		for pi := range piC {
			items = append(items, pi)
		}
		sortEntries(items)
		for i, r := range items {
			// If we stopped here, the next page would start at i + 1.
			// I.e. skip i+1 results from the current index point.
			r.Offset = i + 1
			select {
			case globalResults <- r:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return nil
}

func fetchResults(ctx context.Context, wi WorkItem) error {
	defer close(wi.resultsC)
	if err := rpcSemaphore.Acquire(ctx, 1); err != nil {
		return err
	}

	reachableInvs, err := invocations.Reachable(ctx, invocations.NewIDSet(wi.inv))
	rpcSemaphore.Release(1)
	if err != nil {
		return err
	}

	eg, workerCtx := errgroup.WithContext(ctx)
	defer eg.Wait()
	for _, batch := range reachableInvs.Batches() {
		batch := batch
		eg.Go(func() error {
			// TODO(crbug.com/1107678): Implement support for FieldMask to
			// return only a subset of each result.
			query := testresults.Query{
				InvocationIDs: batch,
				Predicate: &pb.TestResultPredicate{
					Variant:      wi.req.VariantPredicate,
					TestIdRegexp: wi.req.TestIdRegexp,
				},
			}

			if err := rpcSemaphore.Acquire(ctx, 1); err != nil {
				return err
			}
			defer rpcSemaphore.Release(1)

			return query.Run(workerCtx, func(r *pb.TestResult) error {
				select {
				case <-workerCtx.Done():
					return workerCtx.Err()
				case wi.resultsC <- PageItem{
					Entry: &pb.GetTestResultHistoryResponse_Entry{
						Result:              r,
						InvocationTimestamp: wi.ts,
					},
					ip: (*tsIndexPoint)(wi.ts),
				}:
					return nil
				}
			})
		})
	}
	return eg.Wait()
}
