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
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testresults"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// nWorkers is the number of concurrent workers fetching results to serve
// a single request.
const nWorkers = 10

// workItem represents a task to get test results reachable from the given
// invocation.
type workItem struct {
	inv       invocations.ID
	ip        indexPoint
	resultsC  chan *pb.GetTestResultHistoryResponse_Entry
	predicate *pb.TestResultPredicate
}

func (wi *workItem) fetchOneInv(ctx context.Context) error {
	defer close(wi.resultsC)
	reachableInvs, err := invocations.Reachable(ctx, invocations.NewIDSet(wi.inv))
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	for _, batch := range reachableInvs.Batches() {
		batch := batch
		eg.Go(func() error {
			// TODO(crbug.com/1107678): Implement support for FieldMask to
			// return only a subset of each result.
			query := &testresults.Query{
				InvocationIDs: batch,
				Predicate:     wi.predicate,
			}

			return query.Run(ctx, func(r *pb.TestResult) error {
				entry := &pb.GetTestResultHistoryResponse_Entry{Result: r}
				wi.ip.initEntry(entry)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case wi.resultsC <- entry:
					return nil
				}
			})
		})
	}
	return eg.Wait()
}

// Query represents the details of the results history to be retrieved.
type Query struct {
	Request        *pb.GetTestResultHistoryRequest
	workC          chan *workItem
	allResultsC    chan *workItem
	results        []*pb.GetTestResultHistoryResponse_Entry
	resultOffset   int
	nextPageToken  string
	ignoreDeadline bool
}

// Execute runs the query and returns the results in a slice, along with a token
// to get the next page of results (if any) and an error.
func (q *Query) Execute(ctx context.Context) ([]*pb.GetTestResultHistoryResponse_Entry, string, error) {
	q.resultOffset = initPaging(q.Request)
	q.results = make([]*pb.GetTestResultHistoryResponse_Entry, 0, q.Request.PageSize)
	q.workC = make(chan *workItem, nWorkers)
	q.allResultsC = make(chan *workItem, nWorkers)

	ctx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		defer close(q.allResultsC)
		defer close(q.workC)
		return errors.Annotate(q.dispatchWorkItems(ctx), "history.Query.dispatchWorkItems").Err()
	})

	eg.Go(func() error {
		return errors.Annotate(q.fetchAll(ctx), "history.Query.fetchAll").Err()
	})

	eg.Go(func() error {
		return errors.Annotate(q.collect(ctx), "history.Query.collect").Err()
	})

	err := eg.Wait()
	if pageBreak.In(err) {
		err = nil
	}
	return q.results, q.nextPageToken, err
}

// dispatchWorkItems creates a task for each indexed invocation in the
// requested range as well as a channel to collect each task's result.
func (q *Query) dispatchWorkItems(ctx context.Context) error {
	predicate := &pb.TestResultPredicate{
		Variant:      q.Request.VariantPredicate,
		TestIdRegexp: q.Request.TestIdRegexp,
	}

	invQ := &invocations.HistoryQuery{
		Realm:     q.Request.Realm,
		TimeRange: q.Request.GetTimeRange(),
		Predicate: predicate,
	}
	return invQ.ByTimestamp(ctx, func(inv invocations.ID, ts *timestamppb.Timestamp) error {
		wi := &workItem{
			inv:       inv,
			ip:        (*tsIndexPoint)(ts),
			resultsC:  make(chan *pb.GetTestResultHistoryResponse_Entry),
			predicate: predicate,
		}
		select {
		case q.workC <- wi:
		case <-ctx.Done():
			return ctx.Err()
		}

		select {
		case q.allResultsC <- wi:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	})
}

// collect gets results from individual tasks, saves it to the query's `results`
// slice.
// For each item, we compute its offset within the index point s.t.
// we can generate a page token for resuming the results at the next item.
// Also, note that this assumes that each indexed invocation will have a unique
// timestamp, and thus, at least for now, a single indexed invocation counts as
// an index point.
func (q *Query) collect(ctx context.Context) error {
	entries := make([]*pb.GetTestResultHistoryResponse_Entry, 0)
	for wi := range q.allResultsC {
		entries = entries[:0]
	entriesLoop:
		for {
			select {
			case entry, ok := <-wi.resultsC:
				if !ok {
					break entriesLoop
				}
				entries = append(entries, entry)
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		sortEntries(entries)
		for i, entry := range entries {
			if i < q.resultOffset {
				continue
			}
			q.resultOffset = 0

			q.results = append(q.results, entry)
			if len(q.results) == int(q.Request.PageSize) {
				q.nextPageToken = pageToken(wi.ip, i+1)
				return errors.Reason("page full").Tag(pageBreak).Err()
			}
		}
		// If we are about to run out of time, this is a good breaking point.
		if q.outOfTime(ctx) && len(q.results) > 0 {
			q.nextPageToken = pageToken(wi.ip, len(entries))
			return errors.Reason("out of time").Tag(pageBreak).Err()
		}
	}
	return nil
}

// fetchAll starts workers that will fetch results for each invocation in
// q.workC and stream them over the task's channel.
func (q *Query) fetchAll(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	for i := 0; i < nWorkers; i++ {
		eg.Go(func() error {
			for wi := range q.workC {
				if err := wi.fetchOneInv(ctx); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return eg.Wait()
}

// outOfTime returns true if the context will expire in less than 5 seconds.
func (q *Query) outOfTime(ctx context.Context) bool {
	if q.ignoreDeadline {
		return false
	}
	dl, ok := ctx.Deadline()
	return ok && clock.Until(ctx, dl) < 5*time.Second
}
