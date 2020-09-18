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

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	historyDefaultPageSize = 100

	// ConcurrentInvocationTrees is how many indexed invocations to get
	// reachable results for, concurrently.
	ConcurrentInvocationTrees = 10
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
	// Ignore the derived context as we have a single worker.
	// The closure of the resultChans channel is sufficient to know that the
	// work is done.
	eg, _ := errgroup.WithContext(workerCtx)
	idxInvChans := make(chan chan *pb.GetTestResultHistoryResponse_Entry, ConcurrentInvocationTrees)
	eg.Go(func() error {
		defer close(idxInvChans)
		innerEg, innerWorkerCtx := errgroup.WithContext(workerCtx)
		var b spanutil.Buffer
		err := invocations.ByTimestamp(workerCtx, in.Realm, in.GetTimeRange(), func(r *spanner.Row) error {
			inv := &invocations.Historical{}
			if err := b.FromSpanner(r, &inv.ID, &inv.IndexTimestamp); err != nil {
				return err
			}
			results := make(chan *pb.GetTestResultHistoryResponse_Entry)
			select {
			case idxInvChans <- results:
				innerEg.Go(func() error {
					defer close(results)
					return matchingResultsInInvTree(innerWorkerCtx, in, inv, results)
				})
			case <-workerCtx.Done():
				return workerCtx.Err()
			}
			return nil
		})
		if err != nil {
			innerEg.Wait()
			return err
		}
		return innerEg.Wait()
	})

	ret := &pb.GetTestResultHistoryResponse{
		Entries: make([]*pb.GetTestResultHistoryResponse_Entry, 0, in.PageSize),
	}
	// Collect entries for each indexed invocation.
	// these are received from a separate 'results' channel each,
	// and these channels are themselves received from the 'idxInvChans' channel.
	// Continue until either:
	//  - The page of results is full,
	//  - The context is Done,
	//  - There are no more indexed invocations to get results for.
	//
	// NB: The indexed invocations are already ordered by indexed timestamp.
	//
	// TODO(crbug.com/1074407): Implement paging support.
ResultsLoop:
	for {
		select {
		case results, ok := <-idxInvChans:
			if !ok {
				// No more indexed invocations.
				break ResultsLoop
			}
		NextIndexedInvocation:
			for {
				select {
				case entry, ok := <-results:
					if !ok {
						// TODO(crbug.com/1074407): Implement ordering of the
						// results indexed together by (TestID, VariantHash).
						break NextIndexedInvocation
					}
					ret.Entries = append(ret.Entries, entry)
					if len(ret.Entries) == int(in.PageSize) {
						cancelWorker()
						// Ignore the cancellation returned by the worker.
						if err := eg.Wait(); err != nil && err != context.Canceled {
							return ret, err
						}
						return ret, nil
					}
				case <-ctx.Done():
					break ResultsLoop
				}
			}
		case <-ctx.Done():
			break ResultsLoop
		}
	}
	return ret, eg.Wait()
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

// matchingResultsInInvTree gets the matching results reachable from a given
// invocation, and streams them over the given channel.
func matchingResultsInInvTree(ctx context.Context, in *pb.GetTestResultHistoryRequest, idxInv *invocations.Historical, ret chan<- *pb.GetTestResultHistoryResponse_Entry) error {
	reachableInvs, err := invocations.Reachable(ctx, invocations.NewIDSet(idxInv.ID))
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	for _, batch := range reachableInvs.Batches() {
		batch := batch
		eg.Go(func() error {
			// TODO(crbug.com/1107678): Implement support for FieldMask to return
			// only a subset of each result.
			query := testresults.Query{
				InvocationIDs: batch,
				Predicate: &pb.TestResultPredicate{
					Variant:      in.VariantPredicate,
					TestIdRegexp: in.TestIdRegexp,
				},
			}
			return query.Run(ctx, func(r *pb.TestResult) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case ret <- &pb.GetTestResultHistoryResponse_Entry{Result: r, InvocationTimestamp: idxInv.IndexTimestamp}:
					return nil
				}
			})
		})
	}
	return eg.Wait()
}
