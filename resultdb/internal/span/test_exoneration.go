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
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// MustParseTestExonerationName extracts invocation, test path and exoneration
// IDs from the name.
// Panics on failure.
func MustParseTestExonerationName(name string) (invID InvocationID, testPath, exonerationID string) {
	invIDStr, testPath, exonerationID, err := pbutil.ParseTestExonerationName(name)
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
	invIDStr, testPath, exonerationID, err := pbutil.ParseTestExonerationName(name)
	if err != nil {
		return nil, err
	}
	invID := InvocationID(invIDStr)

	ret := &pb.TestExoneration{
		Name:          name,
		TestPath:      testPath,
		ExonerationId: exonerationID,
	}

	// Populate fields from TestExonerations table.
	var explanationMarkdown Snappy
	err = ReadRow(ctx, txn, "TestExonerations", invID.Key(testPath, exonerationID), map[string]interface{}{
		"Variant":             &ret.Variant,
		"ExplanationMarkdown": &explanationMarkdown,
	})
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return nil, errors.Reason("%q not found", ret.Name).
			InternalReason("%s", err).
			Tag(grpcutil.NotFoundTag).
			Err()

	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch %q", ret.Name).Err()

	default:
		ret.ExplanationMarkdown = string(explanationMarkdown)
		return ret, nil
	}
}

// TestExonerationQuery specifies test exonerations to fetch.
type TestExonerationQuery struct {
	InvocationIDs InvocationIDSet
	Predicate     *pb.TestExonerationPredicate // Predicate.Invocation must be nil.
	PageSize      int                          // must be positive
	PageToken     string
}

// QueryTestExonerations reads test exonerations matching the predicate.
// Returned test exonerations from the same invocation are contiguous.
func QueryTestExonerations(ctx context.Context, txn *spanner.ReadOnlyTransaction, q TestExonerationQuery) (tes []*pb.TestExoneration, nextPageToken string, err error) {
	switch {
	case q.PageSize <= 0:
		panic("PageSize <= 0")
	}

	st := spanner.NewStatement(`
		SELECT InvocationId, TestPath, ExonerationId, Variant, ExplanationMarkdown
		FROM TestExonerations
		WHERE InvocationId IN UNNEST(@invIDs)
			# Skip test exonerations after the one specified in the page token.
			AND (
				(InvocationId > @afterInvocationID) OR
				(InvocationId = @afterInvocationID AND TestPath > @afterTestPath) OR
				(InvocationId = @afterInvocationID AND TestPath = @afterTestPath AND ExonerationID > @afterExonerationID)
		  )
		ORDER BY InvocationId, TestPath, ExonerationId
		LIMIT @limit
	`)
	st.Params["invIDs"] = q.InvocationIDs
	st.Params["limit"] = q.PageSize
	st.Params["afterInvocationId"],
		st.Params["afterTestPath"],
		st.Params["afterExonerationID"],
		err = parseTestObjectPageToken(q.PageToken)
	if err != nil {
		return
	}

	// TODO(nodir): add support for q.Predicate.TestPath.
	// TODO(nodir): add support for q.Predicate.Variant.

	tes = make([]*pb.TestExoneration, 0, q.PageSize)
	var b Buffer
	var explanationMarkdown Snappy
	err = query(ctx, txn, st, func(row *spanner.Row) error {
		var invID InvocationID
		ex := &pb.TestExoneration{}
		err := b.FromSpanner(row, &invID, &ex.TestPath, &ex.ExonerationId, &ex.Variant, &explanationMarkdown)
		if err != nil {
			return err
		}
		ex.Name = pbutil.TestExonerationName(string(invID), ex.TestPath, ex.ExonerationId)
		ex.ExplanationMarkdown = string(explanationMarkdown)
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
		invID, testPath, exID := MustParseTestExonerationName(last.Name)
		nextPageToken = pagination.Token(string(invID), testPath, exID)
	}
	return
}
