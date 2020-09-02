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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/resultdb/internal/invocations"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)
// TODO-NOW: Build.
// TODO-NOW: Tests for this:
//      Unit:
//	  Permissions
//	  Validation
//	  TODO: Unit-tests for iterators.
//	End-to-End:
//	  No Permission
//	  Invalid Request
//	  SinglePage at each call
//		More than page_size
//		less than page_size
//		no results
//	  MultiplePages at each call:
//		eventually more results than page_size
//		eventually less results than page_size
//		no results

// verifyGetResultHistoryPermission checks that the caller has permission to
// get test results from the specified realm.
func verifyGetResultHistoryPermission(ctx context.Context, realm string) error {
	if realm == "" {
		return appstatus.BadRequest(errors.Reason("realm is required").Err())
	}
	switch allowed, err := auth.HasPermission(ctx, permGetTestResult, realm); {
	case err != nil:
		return err
	case !allowed:
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %s in realm %q`, permGetTestResult, realm)
	}
	return nil
}
// TODO-NOW: validate request //   E.g. realm, test id, variant, range specified, page size is a non-negative number, token if given is valid,
// validateGetTestResultHistoryRequest checks that the required fields are set,
// and that field values are valid.
func validateGetTestResultHistoryRequest(ctx context.Context, in *pb.GetTestResultHistoryRequest) error {
	if in.GetRealm() == "" {
		return appstatus.BadRequest(errors.Reason("realm is required").Err())
	}
	// There is no need to check for both being set because of the oneof field
	// in the request message.
	if in.GetGetCpRange() == nil && in.GetTimeRange() == nil {
		return appstatus.BadRequest(errors.Reason("exactly one of (cp_range, time_range) must be specified").Err())
	}
	if in.GetPageSize() < 0 {
		return appstatus.BadRequest(errors.Reason("page_size, if specified, must be a positive integer").Err())
	}
	// TODO: Validate page token.
	return nil
}

// GetTestResultHistory implements pb.ResultDBServer.
func (s *resultDBServer) GetTestResultHistory(ctx context.Context, in *pb.GetTestResultHistoryRequest) (*pb.GetTestResultHistoryResponse, error) {
	err := verifyGetResultHistoryPermission(ctx, in.GetRealm())
	if err != nil {
		return nil, err
	}

	errC := make(chan error)
	defer close(errC)
	// TODO-NOW: check this datatype is the right one.
	entries := make([]*pb.GetTestResultHistoryResponse_Entry, 0, in.PageSize)
	// TODO-NOW: Start read-only transaction, stale OK
	results := ResultsForRequest(ctx, in, errC)
	moreResults := true
	for moreResults {
		select {
		case entry, ok := <-results:
			if !ok {
				moreResults = false
				break
			}
			entries = append(entries, entry)
			if len(entries) == in.PageSize {
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


// ResultsForRequest iterates over the invocations matching the request and
// streams the results they contain on the returned channel.
func ResultsForRequest(ctx context.Context, in *pb.GetTestResultHistoryRequest, errC chan<- error) <-chan pb.GetTestResultHistoryResponse_Entry {
	ret := make(chan pb.TestResult)
	go func(){
		defer close(ret)
		rInvs := resultInvocationsForRequest(ctx, in, errC)
		for {
			select {
			case <-ctx.Done():
				return
			case rInv, ok <- rInvs:
				if !ok {
					// no more invocations for this request
					return
				}
				results := relevantResultsForInv(ctx, in, rInv.Name, errC)
				moreResults := true
				for moreResults {
					select {
					case result, ok <- results:
						if !ok {
							// Use flag to break out of both the select and the for.
							moreResults = false
							break
						}
						ret <- pb.GetTestResultHistoryResponse_Entry {
							Result: result,
							InvocationTimestamp: rInv.Timestamp,
						}
					case <-ctx.Done():
						return
					}
				}
			}
		}

	}()
	return ret, nil
}

// resultsInvocation represents a reference to an invocation containing results
// paired with the ordinal fields of an indexed invocation, which could be
// itself or one that transitively includes it.
type resultsInvocation struct {
	Name string,
	Timestamp timestamp.Timestamp,
	// TODO: Support ordinals.
}

// resultInvocationsForRequest gets the invocations in the requested range via
// the history index, and streams their transitive closure over the returned
// channel.
func resultInvocationsForRequest(ctx context.Context, in *pb.GetTestResultHistoryRequest, errC chan<- error) <-chan resultsInvocation {
	ret := make(chan resultsInvocation)
	go func() {
		defer close(ret)
		// TODO-NOW: Implement
		for _, indexedInv := range Invocations.ByTimestamp(ctx, in.TimeRange, in.PageSize, in.Realm) {
			invNames := streamReachable(ctx, indexedInv.Name)
			moreInvocations := true
			for moreInvocations {
				select {
				case invName, ok <- invNames:
					if !ok {
						moreInvocations = false
						break
					}
					ret <- resultsInvocation{
						Name: inv,
						Timestamp: indexedInv.CreateTime,
					}
				case <-ctx.Done():
					return
				}

			}

		}
	}()
	return ret
}

// streamReachable gets reachable invocations and streams them to the returned channel.
func streamReachable(ctx context.Context, indexedInvName string, errC chan<- error) <-chan string {
	ret := make(chan string)
	go func() {
		defer close(ret)
		invIDs, err := invocations.Reachable(ctx, []string{indexedInvName})
		if err != nil {
			errC <- errors.Annotate(err, "failed to read the reach").Err()
			return
		}
		for _, invName := range invs.Names() {
			select {
			case <-ctx.Done():
				return
			default:
				ret <-invName
			}
		}

	}()
	return ret
}

// relevantResultsForInv queries spanner for the results of the given invocation
// matching the request's predicates, and streams them on the returned channel.
func relevantResultsForInv(ctx context.Context, invName string, in *pb.GetTestResultHistoryRequest, errC chan<- error) <-chan pb.TestResult {
	ret := make(chan pb.TestResult)
	go func(){
		defer close(ret)
		query := Query{
			InvocationIDs: invocations.MustParseNames([]string{invName}) ,
			// TODO-NOW: Fill predicate from in
			Predicate: ,
			PageSize: in.PageSize,
		}
		stopQuery := error("stop querying test results")
		err := query.run(ctx, func(r *pb.TestResult) error {
			select {
			case <-ctx.Done():
				return stopQuery
			default:
				ret <-r
			}
			return nil
		})
		if err != nil && err != stopQuery {
			errC <-err
		}
	}()
	return ret
}

