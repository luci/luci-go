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
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
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

	// ConcurrentInvocationTrees is how many indexed invocations to get
	// reachable results for, concurrently.
	ConcurrentInvocationTrees = 5
)

// GetTestResultHistory implements pb.ResultDBServer.
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

	workerDone, cancelWorker, results := streamResults(ctx, in)
	defer cancelWorker()

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
				// Ignore the cancellation returned by the worker.
				if err := workerDone(); err != nil && err != context.Canceled {
					return ret, err
				}
				return ret, nil
			}
		case <-ctx.Done():
			break ResultsLoop
		}
	}
	return ret, workerDone()
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

// workerArgs contains common args for result-streaming workers.
type workerArgs struct {
	in       *pb.GetTestResultHistoryRequest
	reachSem *semaphore.Weighted
	results  chan *pb.GetTestResultHistoryResponse_Entry
}

// streamResultsReachableFrom gets the matching results reachable from a given
// invocation, and streams them over the given channel.
func (hs workerArgs) streamResultsReachableFrom(ctx context.Context, inv invocations.ID, ts *timestamp.Timestamp, previous <-chan bool, next chan<- bool) error {

	// Get reachable invocations, throttle concurrency with semaphore.
	err := hs.reachSem.Acquire(ctx, 1)
	if err != nil {
		return err
	}
	reachableInvs, err := invocations.Reachable(ctx, invocations.NewIDSet(inv))
	hs.reachSem.Release(1)
	if err != nil {
		return err
	}

	// Before proceeding, ensure the previous worker is done streaming results.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-previous:
	}
	defer func() {
		next <- true
	}()

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()
	for _, batch := range reachableInvs.Batches() {
		batch := batch
		eg.Go(func() error {
			// TODO(crbug.com/1107678): Implement support for FieldMask to return
			// only a subset of each result.
			query := testresults.Query{
				InvocationIDs: batch,
				Predicate: &pb.TestResultPredicate{
					Variant:      hs.in.VariantPredicate,
					TestIdRegexp: hs.in.TestIdRegexp,
				},
			}
			return query.Run(ctx, func(r *pb.TestResult) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case hs.results <- &pb.GetTestResultHistoryResponse_Entry{Result: r, InvocationTimestamp: ts}:
					return nil
				}
			})
		})
	}
	return eg.Wait()
}

// streamResults starts workers serving the request's results and returns:
// - A function that blocks completion of all workers.
// - A function that cancels the workers' context.
// - A channel where the result entries will be streamed on.
func streamResults(ctx context.Context, in *pb.GetTestResultHistoryRequest) (func() error, context.CancelFunc, chan *pb.GetTestResultHistoryResponse_Entry) {
	workerCtx, cancelWorker := context.WithCancel(ctx)

	// Ignore the derived context as we have a single worker per request in
	// this group.
	// The closure of the results channel is sufficient to know that the
	// work is done.
	// NB: We don't wait for `eg` in this function, instead we return its
	// `.Wait` function to the caller for them to wait on it when appropriate,
	// E.g. after setting up the receiving end of the resutls channel.
	eg, _ := errgroup.WithContext(workerCtx)

	wa := workerArgs{
		in: in,
		// Use this semaphore to limit the number of workers calling
		// invocations.Reachable concurrently.
		// If we make this too large, we may stall progress as the workers that
		// need to stream their results first may be starved off spanner access.
		reachSem: semaphore.NewWeighted(int64(ConcurrentInvocationTrees)),
		results:  make(chan *pb.GetTestResultHistoryResponse_Entry),
	}

	eg.Go(func() error {
		defer close(wa.results)
		// This group will have a worker per each indexed invocation.
		// Each will in turn employ its own group of workers getting results
		// for the batches of its reachable invocations.
		innerEg, innerWorkerCtx := errgroup.WithContext(workerCtx)
		defer innerEg.Wait()
		// Make a slice of channels that the workers will use to coordinate
		// the streaming of their results to the collector.
		cs := make([]chan bool, 0)
		// Buffer the channels to avoid deadlock when threads are limited.
		cs = append(cs, make(chan bool, 1))
		// Unblock the first worker.
		cs[0] <- true

		invocations.ByTimestamp(workerCtx, in.Realm, in.GetTimeRange(), func(inv invocations.ID, ts *timestamp.Timestamp) error {
			cs = append(cs, make(chan bool, 1))
			i := len(cs)
			innerEg.Go(func() error {
				return wa.streamResultsReachableFrom(innerWorkerCtx, inv, ts, cs[i-2], cs[i-1])
			})
			return nil
		})
		return innerEg.Wait()
	})
	return eg.Wait, cancelWorker, wa.results
}
