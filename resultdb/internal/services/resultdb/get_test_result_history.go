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
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
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

	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	workerErr := make(chan error, 1)
	results := make(chan *pb.GetTestResultHistoryResponse_Entry)
	go func() {
		defer close(results)
		workerErr <- parallel.WorkPool(historyPoolSize, func(workC chan<- func() error) {
			// Make each task keep a pointer to the done channel of task that
			// needs to finish streaming results before it.
			prev := make(chan struct{})
			close(prev)

			err := invocations.ByTimestamp(workerCtx, in.Realm, in.GetTimeRange(), func(inv invocations.ID, ts *timestamp.Timestamp) error {
				task := &resultStreamingTask{
					in:      in,
					invs:    invocations.NewIDSet(inv),
					ts:      ts,
					results: results,
					done:    make(chan struct{}),
					prev:    prev,
				}
				prev = task.done
				select {
				case workC <- func() error { return task.Run(workerCtx) }:
				case <-workerCtx.Done():
					return workerCtx.Err()
				}
				return nil
			})
			if err != nil {
				workC <- func() error { return err }
			}
		})
	}()

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

	// If we cancelled the context on purpose because we have enough results,
	// we can safely ignore the error.
	if ctx.Err() == nil && workerCtx.Err() != nil && int64(len(ret.Entries)) == in.PageSize {
		return ret, nil
	}
	return ret, <-workerErr
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
	ctx     context.Context
	in      *pb.GetTestResultHistoryRequest
	invs    invocations.IDSet
	ts      *timestamp.Timestamp
	results chan<- *pb.GetTestResultHistoryResponse_Entry
	done    chan struct{}
	prev    <-chan struct{}
}

// Run's returned function gets the matching results reachable from the given
// invocation.
// Then, it streams them over the results channel, but only after the previous
// worker is done streaming its results.
func (rst *resultStreamingTask) Run(ctx context.Context) error {
	defer close(rst.done)

	reachableInvs, err := invocations.Reachable(ctx, rst.invs)
	if err != nil {
		return err
	}

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
					Variant:      rst.in.VariantPredicate,
					TestIdRegexp: rst.in.TestIdRegexp,
				},
			}
			return query.Run(ctx, func(r *pb.TestResult) error {
				select {
				case <-rst.prev:
				case <-ctx.Done():
					return ctx.Err()
				}

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
