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

	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/history"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	historyWorkers = 10
)

// GetTestResultHistory implements pb.ResultDBServer.
func (s *resultDBServer) GetTestResultHistory(ctx context.Context, in *pb.GetTestResultHistoryRequest) (*pb.GetTestResultHistoryResponse, error) {
	if err := verifyGetTestResultHistoryPermission(ctx, in.GetRealm()); err != nil {
		return nil, err
	}
	if err := validateGetTestResultHistoryRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	resultsOffset := history.InitPaging(in)

	ctx, cancelTx := span.ReadOnlyTransaction(ctx)
	defer cancelTx()

	outOfTime := history.ExpirationFlag(ctx)
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()

	workerErr := make(chan error, 1)
	results := make(chan history.PageItem)
	go func() {
		defer close(results)
		pool := history.NewStreamerPool(historyWorkers, in, results, workerErr)
		defer func() { <-pool.Purge(workerCtx) }()
		err := invocations.ByTimestamp(workerCtx, in.Realm, in.GetTimeRange(), func(inv invocations.ID, ts *timestamp.Timestamp) error {
			pool.AddInv(workerCtx, inv, ts)
			return nil
		})
		if err != nil {
			select {
			case workerErr <- err:
			case <-ctx.Done():
				// It is okay to dismiss ctx.Err() here because there's
				// already an error in workerErr.
			}
			return
		}
	}()

	ret := &pb.GetTestResultHistoryResponse{
		Entries: make([]*pb.GetTestResultHistoryResponse_Entry, 0, in.PageSize),
	}

	firstUnsortedEntry := 0

	for item := range results {
		if item.Entry != nil {
			ret.Entries = append(ret.Entries, item.Entry)
		}
		if item.PageBreak != nil {
			if resultsOffset > len(ret.Entries) {
				return ret, appstatus.BadRequest(errors.Reason("bad page token: more results to skip than exist at index point").Err())
			}
			sortEntries(ret.Entries[firstUnsortedEntry:])
			ret.Entries = ret.Entries[resultsOffset:]
			resultsOffset = 0

			if len(ret.Entries) >= int(in.PageSize) {
				cancelWorker()
				ret.NextPageToken = history.MakePageToken(item.PageBreak, int(in.PageSize)-firstUnsortedEntry)
				ret.Entries = ret.Entries[:in.PageSize]
				return ret, nil
			}
			if *outOfTime && firstUnsortedEntry > 0 {
				cancelWorker()
				ret.NextPageToken = history.MakePageToken(item.PageBreak, 0)
				ret.Entries = ret.Entries[:firstUnsortedEntry]
				return ret, nil
			}

			firstUnsortedEntry = len(ret.Entries)
		}
	}

	select {
	case err := <-workerErr:
		return ret, err
	default:
		return ret, nil
	}
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
		if err := history.ValidatePageToken(in.GetPageToken()); err != nil {
			return err
		}
	}
	return nil
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
