// Copyright 2019 The LUCI Authors.
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

package span

import (
	"context"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// MustParseTestExonerationName extracts invocation, test id and exoneration
// IDs from the name.
// Panics on failure.
func MustParseTestExonerationName(name string) (invID InvocationID, testID, exonerationID string) {
	invIDStr, testID, exonerationID, err := pbutil.ParseTestExonerationName(name)
	if err != nil {
		panic(err)
	}
	invID = InvocationID(invIDStr)
	return
}

// ReadTestExonerationFull reads a test exoneration from Spanner.
// If it does not exist, the returned error is annotated with NotFound GRPC
// code.
func ReadTestExonerationFull(ctx context.Context, txn Txn, name string) (*pb.TestExoneration, error) {
	invIDStr, testID, exonerationID, err := pbutil.ParseTestExonerationName(name)
	if err != nil {
		return nil, err
	}
	invID := InvocationID(invIDStr)

	ret := &pb.TestExoneration{
		Name:          name,
		TestId:        testID,
		ExonerationId: exonerationID,
	}

	// Populate fields from TestExonerations table.
	var explanationHTML Compressed
	err = ReadRow(ctx, txn, "TestExonerations", invID.Key(testID, exonerationID), map[string]interface{}{
		"Variant":         &ret.Variant,
		"ExplanationHTML": &explanationHTML,
	})
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return nil, appstatus.Attachf(err, codes.NotFound, "%s not found", ret.Name)

	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch %q", ret.Name).Err()

	default:
		ret.ExplanationHtml = string(explanationHTML)
		return ret, nil
	}
}

// TestExonerationQuery specifies test exonerations to fetch.
type TestExonerationQuery struct {
	InvocationIDs InvocationIDSet
	Predicate     *pb.TestExonerationPredicate
	PageSize      int // must be positive
	PageToken     string
}

// Fetch returns a page test of exonerations matching the query.
// Returned test exonerations are ordered by test id.
func (q *TestExonerationQuery) Fetch(ctx context.Context, txn *spanner.ReadOnlyTransaction) (tes []*pb.TestExoneration, nextPageToken string, err error) {
	if q.PageSize <= 0 {
		panic("PageSize <= 0")
	}

	st := spanner.NewStatement(`
		SELECT InvocationId, TestId, ExonerationId, Variant,ExplanationHtml
		FROM TestExonerations
		WHERE InvocationId IN UNNEST(@invIDs)
			# Skip test exonerations after the one specified in the page token.
			AND (
				(TestId > @afterTestId) OR
				(TestId = @afterTestId AND InvocationId > @afterInvocationId) OR
				(TestId = @afterTestId AND InvocationId = @afterInvocationId AND ExonerationID > @afterExonerationID)
		  )
		ORDER BY TestId, InvocationId, ExonerationId
		LIMIT @limit
	`)
	st.Params["invIDs"] = q.InvocationIDs
	st.Params["limit"] = q.PageSize
	st.Params["afterInvocationId"],
		st.Params["afterTestId"],
		st.Params["afterExonerationID"],
		err = parseTestObjectPageToken(q.PageToken)
	if err != nil {
		return
	}

	// TODO(nodir): add support for q.Predicate.TestId.
	// TODO(nodir): add support for q.Predicate.Variant.

	tes = make([]*pb.TestExoneration, 0, q.PageSize)
	var b Buffer
	var explanationHTML Compressed
	err = Query(ctx, txn, st, func(row *spanner.Row) error {
		var invID InvocationID
		ex := &pb.TestExoneration{}
		err := b.FromSpanner(row, &invID, &ex.TestId, &ex.ExonerationId, &ex.Variant, &explanationHTML)
		if err != nil {
			return err
		}
		ex.Name = pbutil.TestExonerationName(string(invID), ex.TestId, ex.ExonerationId)
		ex.ExplanationHtml = string(explanationHTML)
		tes = append(tes, ex)
		return nil
	})
	if err != nil {
		tes = nil
		return
	}

	// If we got pageSize results, then we haven't exhausted the collection and
	// need to return the next page token.
	if len(tes) == q.PageSize {
		last := tes[q.PageSize-1]
		invID, testID, exID := MustParseTestExonerationName(last.Name)
		nextPageToken = pagination.Token(string(invID), testID, exID)
	}
	return
}
