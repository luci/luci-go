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
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	historyDefaultPageSize = 100
	historyPoolSize        = 10
)

// GetTestResultHistory implements pb.ResultDBServer.
//
// If the given context contains a deadline, this func will try to return a
// partial result rather than exceeding it.
//
// Note that this implementation may swallow errors such as context.Canceled or
// context.DeadlineExceeded issued by the libs it calls. E.g. spanner.
func (s *resultDBServer) GetTestResultHistory(ctx context.Context, in *pb.GetTestResultHistoryRequest) (*pb.GetTestResultHistoryResponse, error) {
	if err := verifyGetTestResultHistoryPermission(ctx, in.GetRealm()); err != nil {
		return nil, err
	}
	if err := validateGetTestResultHistoryRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	if in.PageSize == 0 {
		in.PageSize = historyDefaultPageSize
	}
	ctx, cancelTx := span.ReadOnlyTransaction(ctx)
	defer cancelTx()

	// Expire the inner context 5 seconds before the deadline so that we can
	// return a partial response rather than an error.
	var workerCtx context.Context
	var cancelWorker context.CancelFunc
	dl, ok := ctx.Deadline()
	if ok {
		workerCtx, cancelWorker = context.WithDeadline(ctx, dl.Add(-5*time.Second))

	} else {
		workerCtx, cancelWorker = context.WithCancel(ctx)
	}
	defer cancelWorker()
	// Ignore the derived context as we have a single worker.
	// The closure of the results channel is sufficient to know that the work
	// is done.
	eg, _ := errgroup.WithContext(workerCtx)
	results := make(chan *pb.GetTestResultHistoryResponse_Entry)
	eg.Go(func() error {
		defer close(results)
		err := parallel.WorkPool(historyPoolSize, func(workC chan<- func() error) {
			// Make each task keep a pointer to the mutex of the task that
			// needs to finish streaming results before it.
			var prevResults *sync.Mutex

			invocations.ByTimestamp(workerCtx, in.Realm, in.GetTimeRange(), func(inv invocations.ID, ts *timestamp.Timestamp) error {
				task := &resultStreamingTask{
					ctx:         workerCtx,
					in:          in,
					invs:        invocations.NewIDSet(inv),
					ts:          ts,
					results:     results,
					prevResults: prevResults,
				}
				// Each task's mutex should start locked s.t. subsequent tasks
				// do not stream results before it.
				task.resultsLock.Lock()
				prevResults = &task.resultsLock
				select {
				case workC <- task.Run:
				case <-workerCtx.Done():
					return workerCtx.Err()
				}
				return nil
			})
		})
		return errors.SingleError(err)
	})

	ret := &pb.GetTestResultHistoryResponse{
		Entries: make([]*pb.GetTestResultHistoryResponse_Entry, 0, in.PageSize),
	}
	// Collect results sent over the results channel until either:
	//  - The page of results is full,
	//  - The context is Done,
	//  - Or the worker is done with its work (and closes the results channel).
	//
	// TODO(crbug.com/1074407): Implement ordering of the results indexed
	// together by (TestID, VariantHash) .
	//
	// NB: The results in the channel are already ordered by indexed timestamp.
	//
	// TODO(crbug.com/1074407): Implement paging support.
ResultsLoop:
	for {
		select {
		case entry, ok := <-results:
			if !ok {
				break ResultsLoop
			}
			ret.Entries = append(ret.Entries, entry)
			if len(ret.Entries) == int(in.PageSize) {
				cancelWorker()
				break ResultsLoop
			}
		case <-ctx.Done():
			break ResultsLoop
		}
	}

	err := eg.Wait()

	if err != nil &&
		err == context.Canceled ||
		err == context.DeadlineExceeded ||
		status.Code(errors.Unwrap(err)) == codes.Canceled ||
		status.Code(errors.Unwrap(err)) == codes.DeadlineExceeded {
		logging.Warningf(ctx, "Timed out (%s), returning partial response", err)
		return ret, nil
	}
	return ret, err
}

// verifyGetTestResultHistoryPermission checks that the caller has permission to
// get test results from the specified realm.
func verifyGetTestResultHistoryPermission(ctx context.Context, realm string) error {
	if realm == "" {
		return appstatus.BadRequest(errors.Reason("realm is required").Err())
	}
	switch allowed, err := auth.HasPermission(ctx, permListTestResults, realm); {
	case err != nil:
		return err
	case !allowed:
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s in realm %q`, permListTestResults, realm)
	}
	return nil
}

// validateGetTestResultHistoryRequest checks that the required fields are set,
// and that field values are valid.
func validateGetTestResultHistoryRequest(in *pb.GetTestResultHistoryRequest) error {
	if in.GetRealm() == "" {
		return errors.Reason("realm is required").Err()
	}
	// TODO(crbug.com/1107680): Add support for commit position ranges.
	tr := in.GetTimeRange()
	if tr == nil {
		return errors.Reason("time_range must be specified").Err()
	}
	if in.GetPageSize() < 0 {
		return errors.Reason("page_size, if specified, must be a positive integer").Err()
	}
	if in.GetVariantPredicate() != nil {
		if err := pbutil.ValidateVariantPredicate(in.GetVariantPredicate()); err != nil {
			return errors.Annotate(err, "variant_predicate").Err()
		}
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
	ctx         context.Context
	in          *pb.GetTestResultHistoryRequest
	invs        invocations.IDSet
	ts          *timestamp.Timestamp
	results     chan<- *pb.GetTestResultHistoryResponse_Entry
	resultsLock sync.Mutex
	prevResults *sync.Mutex
}

// Run gets the matching results reachable from the given invocation.
// Then, it streams them over the results channel, but only after the previous
// worker is done streaming its results.
func (rst *resultStreamingTask) Run() error {
	defer rst.resultsLock.Unlock()

	reachableInvs, err := invocations.Reachable(rst.ctx, rst.invs)
	if err != nil {
		return err
	}

	// This flag will tell batch workers when to start streaming results.
	readyToStream := false
	rwm := &sync.RWMutex{}
	// Batch workers will wait on this cond for `readyToStream` to be set.
	resultsStreamCond := sync.NewCond(rwm.RLocker())

	go func() {
		// Unless we are the first worker, wait for the previous one to be done
		// streaming results.
		if rst.prevResults != nil {
			rst.prevResults.Lock()
		}
		// Signal batch workers they can start streaming resutls.
		rwm.Lock()
		readyToStream = true
		resultsStreamCond.Broadcast()
		rwm.Unlock()
	}()

	eg, ctx := errgroup.WithContext(rst.ctx)
	defer eg.Wait()
	for _, batch := range reachableInvs.Batches() {
		batch := batch
		eg.Go(func() error {
			// TODO(crbug.com/1107678): Implement support for FieldMask to return
			// only a subset of each result.
			query := testresults.Query{
				InvocationIDs: batch,
				Predicate: &pb.TestResultPredicate{
					Variant:      rst.in.VariantPredicate,
					TestIdRegexp: rst.in.TestIdRegexp,
				},
			}
			return query.Run(ctx, func(r *pb.TestResult) error {
				resultsStreamCond.L.Lock()
				for !readyToStream {
					resultsStreamCond.Wait()
				}
				resultsStreamCond.L.Unlock()

				select {
				case <-ctx.Done():
					return ctx.Err()
				case rst.results <- &pb.GetTestResultHistoryResponse_Entry{Result: r, InvocationTimestamp: rst.ts}:
					return nil
				}
			})
		})
	}
	return eg.Wait()
}
