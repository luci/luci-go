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
	"fmt"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"
	durpb "github.com/golang/protobuf/ptypes/duration"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/appstatus"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// MustParseTestResultName retrieves the invocation ID, unescaped test id, and
// result ID.
//
// Panics if the name is invalid. Should be used only with trusted data.
//
// MustParseTestResultName is faster than pbutil.ParseTestResultName.
func MustParseTestResultName(name string) (invID InvocationID, testID, resultID string) {
	parts := strings.Split(name, "/")
	if len(parts) != 6 || parts[0] != "invocations" || parts[2] != "tests" || parts[4] != "results" {
		panic(errors.Reason("malformed test result name: %q", name).Err())
	}

	invID = InvocationID(parts[1])
	testID = parts[3]
	resultID = parts[5]

	unescaped, err := url.PathUnescape(testID)
	if err != nil {
		panic(errors.Annotate(err, "malformed test id %q", testID).Err())
	}
	testID = unescaped

	return
}

// ReadTestResult reads specified TestResult within the transaction.
// If the TestResult does not exist, the returned error is annotated with
// NotFound GRPC code.
func ReadTestResult(ctx context.Context, txn Txn, name string) (*pb.TestResult, error) {
	invID, testID, resultID := MustParseTestResultName(name)
	tr := &pb.TestResult{
		Name:     name,
		TestId:   testID,
		ResultId: resultID,
		Expected: true,
	}

	var maybeUnexpected spanner.NullBool
	var micros int64
	var summaryHTML Compressed
	err := ReadRow(ctx, txn, "TestResults", invID.Key(testID, resultID), map[string]interface{}{
		"Variant":         &tr.Variant,
		"IsUnexpected":    &maybeUnexpected,
		"Status":          &tr.Status,
		"SummaryHTML":     &summaryHTML,
		"StartTime":       &tr.StartTime,
		"RunDurationUsec": &micros,
		"Tags":            &tr.Tags,
		"InputArtifacts":  &tr.InputArtifacts,
		"OutputArtifacts": &tr.OutputArtifacts,
	})
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return nil, appstatus.Attachf(err, codes.NotFound, "%s not found", name)

	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch %q", name).Err()
	}

	tr.SummaryHtml = string(summaryHTML)
	populateExpectedField(tr, maybeUnexpected)
	populateDurationField(tr, micros)
	return tr, nil
}

// TestResultQuery specifies test results to fetch.
type TestResultQuery struct {
	InvocationIDs     InvocationIDSet
	Predicate         *pb.TestResultPredicate // Predicate.Invocation must be nil.
	PageSize          int                     // must be positive
	PageToken         string
	SelectVariantHash bool
}

func queryTestResults(ctx context.Context, txn *spanner.ReadOnlyTransaction, q TestResultQuery, f func(tr *pb.TestResult, variantHash string) error) error {
	if q.PageSize < 0 {
		panic("PageSize < 0")
	}

	extraSelect := ""
	if q.SelectVariantHash {
		extraSelect = "tr.VariantHash,"
	}
	from := "TestResults tr"
	if q.Predicate.GetExpectancy() == pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS {
		// We must return only test results of test variants that have unexpected results.
		//
		// The following query ensures that we first select test variants with
		// unexpected results, and then for each variant do a lookup in TestResults
		// table.
		from = `
			VariantsWithUnexpectedResults vur
			JOIN@{FORCE_JOIN_ORDER=TRUE} TestResults tr
				ON vur.TestId = tr.TestId AND vur.VariantHash = tr.VariantHash
		`
	}

	limit := ""
	if q.PageSize > 0 {
		limit = `LIMIT @limit`
	}

	st := spanner.NewStatement(fmt.Sprintf(`
		WITH VariantsWithUnexpectedResults AS (
			# Note: this query is not executed if it ends up not used in the top-level
			# query.
			SELECT DISTINCT TestId, VariantHash
			FROM TestResults@{FORCE_INDEX=UnexpectedTestResults}
			WHERE IsUnexpected AND InvocationId IN UNNEST(@invIDs)
		)
		SELECT
			tr.InvocationId,
			tr.TestId,
			tr.ResultId,
			tr.Variant,
			tr.IsUnexpected,
			tr.Status,
			tr.SummaryHtml,
			tr.StartTime,
			tr.RunDurationUsec,
			tr.Tags,
			tr.InputArtifacts,
			tr.OutputArtifacts,
			%s
		FROM %s
		WHERE InvocationId IN UNNEST(@invIDs)
			# Skip test results after the one specified in the page token.
			AND (
				(tr.InvocationId > @afterInvocationId) OR
				(tr.InvocationId = @afterInvocationId AND tr.TestId > @afterTestId) OR
				(tr.InvocationId = @afterInvocationId AND tr.TestId = @afterTestId AND tr.ResultId > @afterResultId)
			)
			AND REGEXP_CONTAINS(tr.TestId, @TestIdRegexp)
			AND (@variantHashEquals IS NULL OR tr.VariantHash = @variantHashEquals)
			AND (@variantContains IS NULL OR (
				SELECT LOGICAL_AND(kv IN UNNEST(tr.Variant))
				FROM UNNEST(@variantContains) kv
			))
		ORDER BY tr.InvocationId, tr.TestId, tr.ResultId
		%s
	`, extraSelect, from, limit))
	st.Params["invIDs"] = q.InvocationIDs
	st.Params["limit"] = q.PageSize

	// Filter by test id.
	testIDRegexp := q.Predicate.GetTestIdRegexp()
	if testIDRegexp == "" {
		testIDRegexp = ".*"
	}
	st.Params["TestIdRegexp"] = fmt.Sprintf("^%s$", testIDRegexp)

	// Filter by variant.
	st.Params["variantHashEquals"] = spanner.NullString{}
	st.Params["variantContains"] = []string(nil)
	switch p := q.Predicate.GetVariant().GetPredicate().(type) {
	case *pb.VariantPredicate_Equals:
		st.Params["variantHashEquals"] = pbutil.VariantHash(p.Equals)
	case *pb.VariantPredicate_Contains:
		st.Params["variantContains"] = pbutil.VariantToStrings(p.Contains)
	case nil:
		// No filter.
	default:
		panic(errors.Reason("unexpected variant predicate %q", q.Predicate.GetVariant()).Err())
	}

	// Apply page token.
	var err error
	st.Params["afterInvocationId"],
		st.Params["afterTestId"],
		st.Params["afterResultId"],
		err = parseTestObjectPageToken(q.PageToken)
	if err != nil {
		return err
	}

	// Read the results.
	var summaryHTML Compressed
	var b Buffer
	return Query(ctx, txn, st, func(row *spanner.Row) error {
		var invID InvocationID
		var maybeUnexpected spanner.NullBool
		var micros int64
		var variantHash string
		tr := &pb.TestResult{}

		ptrs := []interface{}{
			&invID,
			&tr.TestId,
			&tr.ResultId,
			&tr.Variant,
			&maybeUnexpected,
			&tr.Status,
			&summaryHTML,
			&tr.StartTime,
			&micros,
			&tr.Tags,
			&tr.InputArtifacts,
			&tr.OutputArtifacts,
		}
		if q.SelectVariantHash {
			ptrs = append(ptrs, &variantHash)
		}

		err = b.FromSpanner(row, ptrs...)
		if err != nil {
			return err
		}

		tr.Name = pbutil.TestResultName(string(invID), tr.TestId, tr.ResultId)
		tr.SummaryHtml = string(summaryHTML)
		populateExpectedField(tr, maybeUnexpected)
		populateDurationField(tr, micros)

		return f(tr, variantHash)
	})
}

// QueryTestResults reads test results matching the predicate.
// Returned test results from the same invocation are contiguous.
func QueryTestResults(ctx context.Context, txn *spanner.ReadOnlyTransaction, q TestResultQuery) (trs []*pb.TestResult, nextPageToken string, err error) {
	if q.PageSize <= 0 {
		panic("PageSize <= 0")
	}

	trs = make([]*pb.TestResult, 0, q.PageSize)
	err = queryTestResults(ctx, txn, q, func(tr *pb.TestResult, variantHash string) error {
		trs = append(trs, tr)
		return nil
	})
	if err != nil {
		trs = nil
		return
	}

	// If we got pageSize results, then we haven't exhausted the collection and
	// need to return the next page token.
	if len(trs) == q.PageSize {
		last := trs[q.PageSize-1]
		invID, testID, resultID := MustParseTestResultName(last.Name)
		nextPageToken = pagination.Token(string(invID), testID, resultID)
	}
	return
}

// QueryTestResultsStreaming is like QueryTestResults, but returns calls fn
// instead of returning a slice.
func QueryTestResultsStreaming(ctx context.Context, txn *spanner.ReadOnlyTransaction, q TestResultQuery, f func(tr *pb.TestResult, variantHash string) error) error {
	if q.PageSize > 0 {
		panic("PageSize is specified when QueryTestResultsStreaming")
	}
	return queryTestResults(ctx, txn, q, f)
}

func populateDurationField(tr *pb.TestResult, micros int64) {
	tr.Duration = FromMicros(micros)
}

func populateExpectedField(tr *pb.TestResult, maybeUnexpected spanner.NullBool) {
	tr.Expected = !maybeUnexpected.Valid || !maybeUnexpected.Bool
}

// ToMicros converts a duration.Duration proto to microseconds.
func ToMicros(d *durpb.Duration) int64 {
	if d == nil {
		return 0
	}
	return 1e6*d.Seconds + int64(1e-3*float64(d.Nanos))
}

// FromMicros converts microseconds to a duration.Duration proto.
func FromMicros(micros int64) *durpb.Duration {
	return ptypes.DurationProto(time.Duration(1e3 * micros))
}

// parseTestObjectPageToken parses the page token into invocation ID, test id
// and a test object id.
func parseTestObjectPageToken(pageToken string) (inv InvocationID, testID, objID string, err error) {
	switch pos, tokErr := pagination.ParseToken(pageToken); {
	case tokErr != nil:
		err = pageTokenError(tokErr)

	case pos == nil:

	case len(pos) != 3:
		err = pageTokenError(errors.Reason("expected 3 position strings, got %q", pos).Err())

	default:
		inv = InvocationID(pos[0])
		testID = pos[1]
		objID = pos[2]
	}

	return
}

// pageTokenError returns a generic error message that a page token
// is invalid and records err as an internal error.
// The returned error is anontated with INVALID_ARUGMENT code.
func pageTokenError(err error) error {
	return appstatus.Attachf(err, codes.InvalidArgument, "invalid page_token")
}
