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
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	historyDefaultPageSize = 100
	historyPoolSize        = 10
)

// historyItem is used by the result workers to communicate to the collector
// either an item, or a signal that the results under the current invocations
// all have been sent.
type historyItem struct {
	entry *pb.GetTestResultHistoryResponse_Entry
	// When this field is non-zero, the next item streamed on the channel will
	// belong to a different invocation tree.
	treeCompleted invocations.ID
	// If this is given, discard the first this many results, after sorting the
	// results streamed so far.
	// Used after the first batch of results (under a tree) when responding to
	// a request with a NextPageToken.
	skipResults int
}

// GetTestResultHistory implements pb.ResultDBServer.
//
// If the given context contains a deadline, this func will try to return a
// partial result rather than exceeding it.
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

	workerErr := make(chan error, 1)
	results := make(chan historyItem)
	go func() {
		defer close(results)
		defer close(workerErr)
		workerErr <- parallel.WorkPool(historyPoolSize, func(workC chan<- func() error) {
			// Make each task keep a pointer to the done channel of task that
			// needs to finish streaming results before it.
			prev := make(chan struct{})
			close(prev)

			// When getting a page of results, start with the results under
			// `startInv` skipping `startRes` results.
			var startInv string
			var startRes int
			if in.PageToken != "" {
				parts, _ := pagination.ParseToken(in.PageToken)
				startInv = parts[1]
				startRes, _ = strconv.Atoi(parts[2])
			}

			invocations.ByTimestamp(workerCtx, in.Realm, in.GetTimeRange(), func(inv invocations.ID, ts *timestamp.Timestamp) error {
				skipResults := 0
				// Skip all invocations until afterInv is matched.
				if startInv != "" {
					if startInv == string(inv) {
						startInv = ""
						skipResults = startRes
					} else {
						return nil
					}
				}
				task := &resultStreamingTask{
					in:          in,
					inv:         inv,
					ts:          ts,
					results:     results,
					done:        make(chan struct{}),
					prev:        prev,
					skipResults: skipResults,
				}
				if skipResults != 0 {
					skipResults = 0
				}
				prev = task.done
				select {
				case workC <- task.Run(workerCtx):
				case <-workerCtx.Done():
					return workerCtx.Err()
				}
				return nil
			})
		})
	}()

	ret := &pb.GetTestResultHistoryResponse{
		Entries: make([]*pb.GetTestResultHistoryResponse_Entry, 0, in.PageSize),
	}

	firstUnsortedEntry := 0

	// Collect results sent over the results channel until either:
	//  - The page of results is full,
	//  - The context is Done,
	//  - Or the worker is done with its work (and closes the results channel).
ResultsLoop:
	for {
		select {
		case item, ok := <-results:
			if !ok {
				break ResultsLoop
			}
			if item.entry != nil {
				ret.Entries = append(ret.Entries, item.entry)
			}
			if item.treeCompleted != invocations.ID("") {
				sortEntries(ret.Entries[firstUnsortedEntry:])
				ret.Entries = ret.Entries[item.skipResults:]
				if len(ret.Entries) >= int(in.PageSize) {
					cancelWorker()
					ret.NextPageToken = makeHistoryPageToken(in.Realm, item.treeCompleted, int(in.PageSize)-firstUnsortedEntry)
					ret.Entries = ret.Entries[:in.PageSize]
					break ResultsLoop
				}
				firstUnsortedEntry = len(ret.Entries)
			}
		case <-ctx.Done():
			// Drop the unsorted results at the end.
			ret.Entries = ret.Entries[:firstUnsortedEntry]
			break ResultsLoop
		}
	}

	var retErr error
	wErr := <-workerErr
	errors.WalkLeaves(wErr, func(err error) bool {
		errCode := status.Code(err)
		if err != context.Canceled && err != context.DeadlineExceeded &&
			errCode != codes.Canceled && errCode != codes.DeadlineExceeded {
			// Found an error unrelated to context cancellation return it to
			// the RPC caller.
			retErr = wErr
			return false
		}
		return true
	})
	return ret, retErr
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
	if in.GetPageToken() != "" {
		parts, err := pagination.ParseToken(in.GetPageToken())
		if err != nil {
			return err
		}
		if len(parts) != 3 {
			return errors.Reason("invalid page token - wrong number of fields").Err()
		}
		if parts[0] != in.GetRealm() {
			return errors.Reason("invalid page token - realm mismatch").Err()
		}
		if pbutil.ValidateInvocationID(parts[1]) != nil {
			return errors.Reason("invalid page token - bad invocation field").Err()
		}
		if _, err := strconv.Atoi(parts[2]); err != nil {
			return errors.Reason("invalid page token - bad offset field").Err()
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
	in          *pb.GetTestResultHistoryRequest
	inv         invocations.ID
	ts          *timestamp.Timestamp
	results     chan<- historyItem
	done        chan struct{}
	prev        <-chan struct{}
	skipResults int
}

// Run's returned function gets the matching results reachable from the given
// invocation.
// Then, it streams them over the results channel, but only after the previous
// worker is done streaming its results.
func (rst *resultStreamingTask) Run(ctx context.Context) func() error {
	return func() error {
		defer close(rst.done)

		reachableInvs, err := invocations.Reachable(ctx, invocations.NewIDSet(rst.inv))
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
					}

					select {
					case <-ctx.Done():
						return ctx.Err()
					case rst.results <- historyItem{
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

		err = eg.Wait()
		if err == nil {
			// Signal the receiver that the results for this invocation tree are
			// complete, s.t. it can sort the results streamed so far.
			rst.results <- historyItem{
				treeCompleted: rst.inv,
				skipResults:   rst.skipResults,
			}
		}
		return err
	}
}

// sortEntries sorts results in the slice by (TestId, VariantHash, ResultId).
// It's assumed that all the results in the slice are indexed under the same
// timestamp/ordinal.
func sortEntries(s []*pb.GetTestResultHistoryResponse_Entry) {
	l := func(i, j int) bool {
		if s[i].Result.TestId == s[j].Result.TestId {
			if s[i].Result.VariantHash == s[j].Result.VariantHash {
				return s[i].Result.ResultId < s[j].Result.ResultId
			}
			return s[i].Result.VariantHash < s[j].Result.VariantHash
		}
		return s[i].Result.TestId < s[j].Result.TestId
	}
	sort.Slice(s, l)
}

func makeHistoryPageToken(realm string, inv invocations.ID, offset int) string {
	return pagination.Token(realm, string(inv), fmt.Sprint(offset))
}
