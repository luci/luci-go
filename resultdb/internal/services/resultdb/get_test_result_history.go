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
	"sort"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
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
	if err := validateGetTestResultHistoryRequest(in); err != nil {
		return nil, err
	}

	if in.PageSize == 0 {
		in.PageSize = defaultPageSize
	}

	// This channel will be passed to the generator goroutines to surface any
	// errors they encounter to this main goroutine.
	errC := make(chan error)
	defer close(errC)

	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Start streaming results.
	results := resultsForRequest(ctx, in, errC)

	// Accumulate them here.
	entries := make([]*pb.GetTestResultHistoryResponse_Entry, 0, in.PageSize)

	moreResults := true
	for moreResults {
		select {
		case entry, ok := <-results:
			if !ok {
				moreResults = false
				break
			}
			entries = append(entries, entry)
			if len(entries) == int(in.PageSize) {
				moreResults = false
				break
			}
		case <-ctx.Done():
			moreResults = false
			break
		case err := <-errC:
			return nil, err
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
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s in realm %q`, permGetTestResult, realm)
	}
	return nil
}

// validateGetTestResultHistoryRequest checks that the required fields are set,
// and that field values are valid.
func validateGetTestResultHistoryRequest(in *pb.GetTestResultHistoryRequest) error {
	if in.GetRealm() == "" {
		return appstatus.BadRequest(errors.Reason("realm is required").Err())
	}
	if in.GetTimeRange() == nil {
		return appstatus.BadRequest(errors.Reason("time_range must be specified").Err())
	}
	if in.GetPageSize() < 0 {
		return appstatus.BadRequest(errors.Reason("page_size, if specified, must be a positive integer").Err())
	}

	if in.GetVariantPredicate() != nil {
		if err := pbutil.ValidateVariantPredicate(in.GetVariantPredicate()); err != nil {
			return appstatus.BadRequest(errors.Annotate(err, "variant_predicate").Err())
		}
	}
	return nil
}

// resultsForRequest gets the requested test results.
//
// Iterates over the invocations matching the request and streams the results
// they contain (that match the predicate) on the returned channel.
func resultsForRequest(ctx context.Context, in *pb.GetTestResultHistoryRequest, errC chan<- error) <-chan *pb.GetTestResultHistoryResponse_Entry {
	ret := make(chan *pb.GetTestResultHistoryResponse_Entry)
	go func() {
		defer close(ret)
		rInvs := resultInvocationsForRequest(ctx, in, errC)
		for {
			select {
			case <-ctx.Done():
				return
			case rInv, ok := <-rInvs:
				if !ok {
					// no more invocations for this request
					return
				}
				results := matchingResultsInInv(ctx, rInv.ID, in, errC)
				moreResults := true
				for moreResults {
					select {
					case result, ok := <-results:
						if !ok {
							// Use flag to break out of both the select and the for.
							moreResults = false
							break
						}
						ret <- &pb.GetTestResultHistoryResponse_Entry{
							Result:              result,
							InvocationTimestamp: rInv.Timestamp,
						}
					case <-ctx.Done():
						return
					}
				}
			}
		}

	}()
	return ret
}

// resultInvocationsForRequest finds all invocations that potentially contain
// the requested results.
//
// Gets the invocations in the requested range via the history index,
// and streams their transitive closure over the returned channel.
func resultInvocationsForRequest(ctx context.Context, in *pb.GetTestResultHistoryRequest, errC chan<- error) <-chan invocations.Historical {
	ret := make(chan invocations.Historical)
	go func() {
		defer close(ret)
		idxInvs, err := invocations.ByTimestamp(ctx, in.GetTimeRange(), in.Realm)
		if err != nil {
			select {
			case errC <- err:
			default:
				logging.Warningf(ctx, "Got %s, but error channel is closed", err)
				return
			}
		}
		for _, indexedInv := range idxInvs {
			reachableInvs, err := invocations.Reachable(ctx, invocations.NewIDSet(indexedInv.ID))
			if err != nil {
				err = errors.Annotate(err, "failed to read the reachables").Err()
				select {
				case errC <- err:
				default:
					logging.Warningf(ctx, "Got %s, but error channel is closed", err)
					return
				}
			}
			reachableIDs := make([]string, 0, len(reachableInvs))
			for invID := range reachableInvs {
				reachableIDs = append(reachableIDs, string(invID))
			}
			sort.Strings(reachableIDs)
			for _, invID := range reachableIDs {
				ret <- invocations.Historical{
					ID:        invocations.ID(invID),
					Timestamp: indexedInv.Timestamp,
				}
			}
		}
	}()
	return ret
}

// matchingResultsInInv queries spanner for the results of the given invocation
// matching the request's predicate, and streams them on the returned channel.
func matchingResultsInInv(ctx context.Context, invID invocations.ID, in *pb.GetTestResultHistoryRequest, errC chan<- error) <-chan *pb.TestResult {
	ret := make(chan *pb.TestResult)
	go func() {
		defer close(ret)
		query := testresults.Query{
			InvocationIDs: invocations.NewIDSet(invID),
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
				ret <- r
			}
			return nil
		})
		if err != nil && err != stopQuery {
			select {
			case errC <- err:
			default:
				logging.Warningf(ctx, "Got %s, but error channel is closed", err)
				return
			}

		}
	}()
	return ret
}
