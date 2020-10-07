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

package resultdb

import (
	"context"

	"github.com/golang/protobuf/ptypes/timestamp"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testresults"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// historyStreamerPool represents a pool of workers that groups
// "simultaneous" invocations to get results for and launches a
// `resultStreamingTask` for each group with up to `size` concurrent workers.
type historyStreamerPool struct {
	req         *pb.GetTestResultHistoryRequest
	sem         *semaphore.Weighted
	pendingTask *resultStreamingTask
	prevC       chan struct{}
	resultsC    chan<- historyPageItem
	errC        chan<- error
	size        int64
}

// newHistoryStreamerPool inits a new historyStreamerPool.
func newHistoryStreamerPool(size int64, in *pb.GetTestResultHistoryRequest, results chan<- historyPageItem, errors chan<- error) historyStreamerPool {
	ret := historyStreamerPool{
		req:      in,
		resultsC: results,
		errC:     errors,
		prevC:    make(chan struct{}),
		sem:      semaphore.NewWeighted(size),
		size:     size,
	}
	close(ret.prevC)
	return ret
}

// purge launches the currently pending task.
func (p *historyStreamerPool) purge(ctx context.Context) {
	t := p.pendingTask
	err := p.sem.Acquire(ctx, 1)
	if err == nil {
		go func() {
			defer p.sem.Release(1)
			t.Run(ctx)
		}()
	}
}

// wait waits for all running workers in the poool to finish.
func (p *historyStreamerPool) wait(ctx context.Context) error {
	return p.sem.Acquire(ctx, p.size)
}

// addInv adds the given invocation to the currently pending task if it is
// indexed at the same timestamp.
// If it is not, it launches the previously pending task and creates a new one
// with the given invocation.
func (p *historyStreamerPool) addInv(ctx context.Context, inv invocations.ID, ts *timestamp.Timestamp) error {
	var prev chan struct{}
	if p.pendingTask == nil {
		prev = make(chan struct{})
		close(prev)
	} else {
		if ts.AsTime().Equal(p.pendingTask.ts.AsTime()) {
			p.pendingTask.invs.Add(inv)
			return nil
		}
		p.purge(ctx)
		prev = p.pendingTask.done
	}
	p.pendingTask = &resultStreamingTask{
		pool: p,
		invs: invocations.NewIDSet(inv),
		ts:   ts,
		done: make(chan struct{}),
		prev: prev,
	}
	return nil
}

// resultStreamingTask represents a task with two parts:
//   - A traversal of the inclusion graph to find the set of reachable
//     invocations indexed under the same timestamp/commit position,
//     which can be done in parallel for several invocations in the index.
//     (Though note that such traversal may itself involve multiple parallel
//     requests to the database if the results are not yet cached.)
//   - A query for the test results contained by the invocations in the set
//     above.
//     This query can be done in parallel with queries for other sets
//     of invocations, but note that their results need to be streamed in order.
type resultStreamingTask struct {
	invs invocations.IDSet
	ts   *timestamp.Timestamp
	done chan struct{}
	prev <-chan struct{}
	pool *historyStreamerPool
}

// Run gets the matching results reachable from the given invocation set.
// Then, it streams them over the results channel, but only after the previous
// worker is done streaming its results.
func (rst *resultStreamingTask) Run(ctx context.Context) {
	defer close(rst.done)

	reachableInvs, err := invocations.Reachable(ctx, rst.invs)
	if err != nil {
		select {
		case rst.pool.errC <- err:
		default:
			return
		}
	}

	eg, workerCtx := errgroup.WithContext(ctx)
	defer eg.Wait()
	for _, batch := range reachableInvs.Batches() {
		batch := batch
		eg.Go(func() error {
			// TODO(crbug.com/1107678): Implement support for FieldMask to return
			// only a subset of each result.
			query := testresults.Query{
				InvocationIDs: batch,
				Predicate: &pb.TestResultPredicate{
					Variant:      rst.pool.req.VariantPredicate,
					TestIdRegexp: rst.pool.req.TestIdRegexp,
				},
			}

			return query.Run(workerCtx, func(r *pb.TestResult) error {
				<-rst.prev

				select {
				case <-workerCtx.Done():
					return workerCtx.Err()
				case rst.pool.resultsC <- historyPageItem{
					entry: &pb.GetTestResultHistoryResponse_Entry{
						Result:              r,
						InvocationTimestamp: rst.ts,
					},
				}:
					return nil
				}
			})
		})
	}

	if err := eg.Wait(); err != nil {
		select {
		case rst.pool.errC <- err:
		default:
		}
		return
	}

	select {
	case rst.pool.resultsC <- historyPageItem{
		pageBreak: (*tsIndexPoint)(rst.ts),
	}:
	case <-ctx.Done():
	}

}
