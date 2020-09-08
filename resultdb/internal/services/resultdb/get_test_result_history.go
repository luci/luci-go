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

	"go.chromium.org/luci/resultdb/pbutil"
	"google.golang.org/grpc/codes"

	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testresults"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// DefaultPageSize is used when not specified or set to 0.
	DefaultPageSize = 100
)

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

// validateGetTestResultHistoryRequest checks that the required fields are set,
// and that field values are valid.
func validateGetTestResultHistoryRequest(in *pb.GetTestResultHistoryRequest) error {
	if in.GetRealm() == "" {
		return appstatus.BadRequest(errors.Reason("realm is required").Err())
	}
	// There is no need to check for both being set because of the oneof field
	// in the request message.
	if in.GetTimeRange() == nil {
		return appstatus.BadRequest(errors.Reason("time_range must be specified").Err())
	}
	if in.GetPageSize() < 0 {
		return appstatus.BadRequest(errors.Reason("page_size, if specified, must be a positive integer").Err())
	}

	if in.GetVariantPredicate() != nil {
		// Skip this validation if variant predicate is not specified, as it would fail otherwise.
		if err := pbutil.ValidateVariantPredicate(in.GetVariantPredicate()); err != nil {
			return appstatus.BadRequest(errors.Annotate(err, "variant_predicate is invalid").Err())
		}
	}
	// TODO: Validate page token.
	return nil
}

// GetTestResultHistory implements pb.ResultDBServer.
func (s *resultDBServer) GetTestResultHistory(ctx context.Context, in *pb.GetTestResultHistoryRequest) (*pb.GetTestResultHistoryResponse, error) {
	if err := verifyGetResultHistoryPermission(ctx, in.GetRealm()); err != nil {
		return nil, err
	}
	if err := validateGetTestResultHistoryRequest(in); err != nil {
		return nil, err
	}

	if in.PageSize == 0 {
		in.PageSize = DefaultPageSize
	}

	errC := make(chan error)
	defer close(errC)
	entries := make([]*pb.GetTestResultHistoryResponse_Entry, 0, in.PageSize)
	// TODO-NOW: Start transaction
	// TODO: Should we allow for stale reads here?
	results := resultsForRequest(ctx, in, errC)
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

// resultsForRequest iterates over the invocations matching the request and
// streams the results they contain on the returned channel.
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
				results := relevantResultsForInv(ctx, rInv.Name, in, errC)
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

// resultsInvocation represents a reference to an invocation containing results
// paired with the ordinal fields of an indexed invocation, which could be
// itself or one that transitively includes it.
type resultsInvocation struct {
	Name      string
	Timestamp *timestamp.Timestamp
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
		idxInvs, err := invocations.ByTimestamp(ctx, in.GetTimeRange(), int(in.PageSize), in.Realm)
		if err != nil {
			errC <- err
			return
		}
		for _, indexedInv := range idxInvs {
			invNames := streamReachable(ctx, indexedInv.Name, errC)
			moreInvocations := true
			for moreInvocations {
				select {
				case invName, ok := <-invNames:
					if !ok {
						moreInvocations = false
						break
					}
					ret <- resultsInvocation{
						Name:      invName,
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
		invIDs, err := invocations.Reachable(ctx, invocations.MustParseNames([]string{indexedInvName}))
		if err != nil {
			errC <- errors.Annotate(err, "failed to read the reach").Err()
			return
		}
		for _, invName := range invIDs.Names() {
			select {
			case <-ctx.Done():
				return
			default:
				ret <- invName
			}
		}

	}()
	return ret
}

// relevantResultsForInv queries spanner for the results of the given invocation
// matching the request's predicates, and streams them on the returned channel.
func relevantResultsForInv(ctx context.Context, invName string, in *pb.GetTestResultHistoryRequest, errC chan<- error) <-chan *pb.TestResult {
	ret := make(chan *pb.TestResult)
	go func() {
		defer close(ret)
		query := testresults.Query{
			InvocationIDs: invocations.MustParseNames([]string{invName}),
			Predicate: &pb.TestResultPredicate{
				Variant:      in.VariantPredicate,
				TestIdRegexp: in.TestIdRegexp,
			},
			PageSize: int(in.PageSize),
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
			errC <- err
		}
	}()
	return ret
}
