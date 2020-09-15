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
	defaultPageSize = 100
)

// GetTestResultHistory implements pb.ResultDBServer.
func (s *resultDBServer) GetTestResultHistory(ctx context.Context, in *pb.GetTestResultHistoryRequest) (*pb.GetTestResultHistoryResponse, error) {
	if err := verifyGetTestResultHistoryPermission(ctx, in.GetRealm()); err != nil {
		return nil, err
	}
	if err := validateGetTestResultHistoryRequest(ctx, in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	if in.PageSize == 0 {
		in.PageSize = defaultPageSize
	}

	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Start streaming results.
	results := make(chan entryOrError)
	go resultsForRequest(ctx, in, results)

	// Accumulate them here.
	entries := make([]*pb.GetTestResultHistoryResponse_Entry, 0, in.PageSize)

	moreResults := true
	for moreResults {
		select {
		case msg, ok := <-results:
			if !ok {
				moreResults = false
				break
			}
			if msg.err != nil {
				return nil, msg.err
			}
			entry := msg.entry
			entries = append(entries, entry)
			if len(entries) == int(in.PageSize) {
				moreResults = false
				break
			}
		case <-ctx.Done():
			moreResults = false
			break
		}
	}
	return &pb.GetTestResultHistoryResponse{
		Entries: entries,
	}, nil

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
func validateGetTestResultHistoryRequest(ctx context.Context, in *pb.GetTestResultHistoryRequest) error {
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

type entryOrError struct {
	entry *pb.GetTestResultHistoryResponse_Entry
	err   error
}

// resultsForRequest gets the requested test results.
//
// Iterates over the invocations matching the request and streams the results
// they contain (that match the predicate) on the given channel.
func resultsForRequest(ctx context.Context, in *pb.GetTestResultHistoryRequest, ret chan<- entryOrError) {
	defer close(ret)
	// The invocations will be streamed over this channel.
	// It will be closed by the goroutine when there are no more results or
	// when the context is done.
	rInvs := make(chan historicalOrError)
	go resultInvocationsForRequest(ctx, in, rInvs)

	for msg := range rInvs {
		if msg.err != nil {
			ret <- entryOrError{
				err: msg.err,
			}
		}
		matchingResultsInInv(ctx, newInvocationCohort(msg.inv), in, ret)
	}
}

type historicalOrError struct {
	inv *invocations.Historical
	err error
}

// resultInvocationsForRequest finds all invocations that potentially contain
// the requested results.
//
// Gets the invocations in the requested range via the history index,
// and streams their transitive closure over the given channel.
func resultInvocationsForRequest(ctx context.Context, in *pb.GetTestResultHistoryRequest, ret chan<- historicalOrError) {
	defer close(ret)
	idxInvs, err := invocations.ByTimestamp(ctx, in.GetTimeRange(), in.Realm)
	if err != nil {
		ret <- historicalOrError{err: err}
		return
	}
	for _, indexedInv := range idxInvs {
		reachableInvs, err := invocations.Reachable(ctx, invocations.NewIDSet(indexedInv.ID))
		if err != nil {
			ret <- historicalOrError{err: err}
			return
		}

		for invID := range reachableInvs {
			select {
			case <-ctx.Done():
				return
			default:
				ret <- historicalOrError{
					inv: &invocations.Historical{
						ID:        invID,
						Timestamp: indexedInv.Timestamp,
					},
				}
			}
		}
	}
}

// invocationCohort represents a set of invocations that occur at the same time
// for purposes of test results history.
// E.g. on the same invocation tree (with a common indexed ancestor), or at the
// same commit position.
type invocationCohort struct {
	IDSet    invocations.IDSet
	Ordinals invocations.Historical
}

func newInvocationCohort(invs ...*invocations.Historical) *invocationCohort {
	if len(invs) == 0 {
		panic("At least one invocation is required to create a cohort")
	}
	ret := &invocationCohort{
		IDSet:    invocations.NewIDSet(invs[0].ID),
		Ordinals: *invs[0],
	}
	ret.Ordinals.ID = ""
	for i := 1; i < len(invs); i++ {
		if !ret.Ordinals.Concurrent(invs[i]) {
			panic("All invocations in the cohort must be concurrent")
		}
		ret.IDSet.Add(invs[i].ID)
	}

	return ret
}

// matchingResultsInInv queries spanner for the results of the given cohort of
// invocations matching the request's predicate, and streams them as history
// entries on the given channel.
func matchingResultsInInv(ctx context.Context, cohort *invocationCohort, in *pb.GetTestResultHistoryRequest, ret chan<- entryOrError) {
	// TODO(crbug.com/1074407): Implement ordering of the results by
	// (TestID, VariantHash).
	//
	// TODO(crbug.com/1107678): Implement support for FieldMask to return
	// only a subset of each result.
	query := testresults.Query{
		InvocationIDs: cohort.IDSet,
		Predicate: &pb.TestResultPredicate{
			Variant:      in.VariantPredicate,
			TestIdRegexp: in.TestIdRegexp,
		},
	}
	stopQuery := errors.New("stop querying test results")
	err := query.Run(ctx, func(r *pb.TestResult) error {
		select {
		case <-ctx.Done():
			return stopQuery
		default:
			ret <- entryOrError{
				entry: &pb.GetTestResultHistoryResponse_Entry{
					Result:              r,
					InvocationTimestamp: cohort.Ordinals.Timestamp,
				},
			}
		}
		return nil
	})
	if err != nil && err != stopQuery {
		ret <- entryOrError{err: err}
	}
}
