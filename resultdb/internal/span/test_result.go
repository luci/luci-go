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
	"time"

	"cloud.google.com/go/spanner"
	"github.com/Masterminds/squirrel"
	"github.com/golang/protobuf/ptypes"
	durpb "github.com/golang/protobuf/ptypes/duration"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/metrics"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// MustParseTestResultName retrieves the invocation ID, unescaped test path, and
// result ID.
// Panics if the name is invalid. Useful for situations when name was already
// validated.
func MustParseTestResultName(name string) (invID InvocationID, testPath, resultID string) {
	invIDStr, testPath, resultID, err := pbutil.ParseTestResultName(name)
	if err != nil {
		panic(err)
	}
	invID = InvocationID(invIDStr)
	return
}

// ReadTestResult reads specified TestResult within the transaction.
// If the TestResult does not exist, the returned error is annotated with
// NotFound GRPC code.
func ReadTestResult(ctx context.Context, txn Txn, name string) (*pb.TestResult, error) {
	invID, testPath, resultID := MustParseTestResultName(name)
	tr := &pb.TestResult{
		Name:     name,
		TestPath: testPath,
		ResultId: resultID,
		Expected: true,
	}

	var maybeUnexpected spanner.NullBool
	var micros int64
	var summaryMarkdown Snappy
	err := ReadRow(ctx, txn, "TestResults", invID.Key(testPath, resultID), map[string]interface{}{
		"ExtraVariantPairs": &tr.ExtraVariantPairs,
		"IsUnexpected":      &maybeUnexpected,
		"Status":            &tr.Status,
		"SummaryMarkdown":   &summaryMarkdown,
		"StartTime":         &tr.StartTime,
		"RunDurationUsec":   &micros,
		"Tags":              &tr.Tags,
		"InputArtifacts":    &tr.InputArtifacts,
		"OutputArtifacts":   &tr.OutputArtifacts,
	})
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return nil, errors.Reason("%q not found", name).
			InternalReason("%s", err).
			Tag(grpcutil.NotFoundTag).
			Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch %q", name).Err()
	}

	tr.SummaryMarkdown = string(summaryMarkdown)
	populateExpectedField(tr, maybeUnexpected)
	populateDurationField(tr, micros)
	return tr, nil
}

// TestResultQuery specifies test results to fetch.
type TestResultQuery struct {
	InvocationIDs []InvocationID
	Predicate     *pb.TestResultPredicate // Predicate.Invocation must be nil.
	PageSize      int                     // must be positive
	CursorToken   string
}

// QueryTestResults reads test results matching the predicate.
// Returned test results from the same invocation are contiguous.
func QueryTestResults(ctx context.Context, txn *spanner.ReadOnlyTransaction, q TestResultQuery) (trs []*pb.TestResult, nextCursorTok string, err error) {
	defer metrics.Trace(ctx, "QueryTestRsults(ctx, txn, %#v)", q)()

	switch {
	case q.PageSize <= 0:
		panic("PageSize <= 0")
	case q.Predicate.GetInvocation() != nil:
		panic("q.Predicate.Invocation != nil")
	}

	sql := squirrel.
		Select(
			"InvocationId",
			"TestPath",
			"ResultId",
			"ExtraVariantPairs",
			"IsUnexpected",
			"Status",
			"SummaryMarkdown",
			"StartTime",
			"RunDurationUsec",
			"Tags",
			"InputArtifacts",
			"OutputArtifacts",
		).
		From("TestResults").
		Where("InvocationId IN UNNEST(@invIDs)").
		OrderBy("InvocationId", "TestPath", "ResultId").
		Limit(uint64(q.PageSize))

	queryParams := map[string]interface{}{
		"invIDs": q.InvocationIDs,
	}

	// Set start position if requested.
	switch pos, tokErr := pagination.ParseToken(q.CursorToken); {
	case tokErr != nil:
		err = errors.Reason("invalid page_token").
			InternalReason("%s", tokErr).
			Tag(grpcutil.InvalidArgumentTag).Err()
		return

	case pos == nil:
		break

	case len(pos) == 3:
		sql = sql.Where(`(
			(InvocationId > @cursorInvocationID)
			OR (InvocationId = @cursorInvocationID AND TestPath > @cursorTestPath)
			OR (InvocationId = @cursorInvocationID AND TestPath = @cursorTestPath AND ResultId > @cursorResultID)
			)`)
		queryParams["cursorInvocationID"] = InvocationID(pos[0])
		queryParams["cursorTestPath"] = pos[1]
		queryParams["cursorResultID"] = pos[2]

	default:
		err = errors.Reason("invalid page_token").
			InternalReason("unexpected string slice %q for TestResults cursor position", pos).
			Tag(grpcutil.InvalidArgumentTag).Err()
		return
	}

	if q.Predicate.GetTestPath() != nil {
		// TODO(nodir): add support for q.Predicate.TestPath.
		return nil, "", grpcutil.Unimplemented
	}
	if q.Predicate.GetVariant() != nil {
		// TODO(nodir): add support for q.Predicate.Variant.
		return nil, "", grpcutil.Unimplemented
	}
	if q.Predicate.GetExpectancy() != pb.TestResultPredicate_ALL {
		// TODO(nodir): add support for q.Predicate.Expectancy.
		return nil, "", grpcutil.Unimplemented
	}

	sqlStr, _, err := sql.ToSql()
	if err != nil {
		return
	}
	st := spanner.NewStatement(sqlStr)
	st.Params = ToSpannerMap(queryParams)
	logging.Infof(ctx, "querying test results: %s", st.SQL)
	logging.Infof(ctx, "query parameters: %#v", st.Params)

	trs = make([]*pb.TestResult, 0, q.PageSize)
	var summaryMarkdown Snappy
	err = txn.Query(ctx, st).Do(func(row *spanner.Row) error {
		var invID InvocationID
		var maybeUnexpected spanner.NullBool
		var micros int64
		tr := &pb.TestResult{}
		err = FromSpanner(row,
			&invID,
			&tr.TestPath,
			&tr.ResultId,
			&tr.ExtraVariantPairs,
			&maybeUnexpected,
			&tr.Status,
			&summaryMarkdown,
			&tr.StartTime,
			&micros,
			&tr.Tags,
			&tr.InputArtifacts,
			&tr.OutputArtifacts,
		)
		if err != nil {
			return err
		}

		tr.Name = pbutil.TestResultName(string(invID), tr.TestPath, tr.ResultId)
		tr.SummaryMarkdown = string(summaryMarkdown)
		populateExpectedField(tr, maybeUnexpected)
		populateDurationField(tr, micros)

		trs = append(trs, tr)
		return nil
	})
	if err != nil {
		trs = nil
		return
	}

	// If we got pageSize results, then we haven't exhausted the collection and
	// need to return a cursor.
	if len(trs) == q.PageSize {
		last := trs[q.PageSize-1]
		invID, testPath, resultID := MustParseTestResultName(last.Name)
		nextCursorTok = pagination.Token(string(invID), testPath, resultID)
	}
	return
}

func populateDurationField(tr *pb.TestResult, micros int64) {
	tr.Duration = FromMicros(micros)
}

func populateExpectedField(tr *pb.TestResult, maybeUnexpected spanner.NullBool) {
	tr.Expected = !maybeUnexpected.Valid || !maybeUnexpected.Bool
}

// ToMicros converts a duration.Duration proto to microseconds.
func ToMicros(d *durpb.Duration) int64 {
	return 1e6*d.Seconds + int64(1e-3*float64(d.Nanos))
}

// FromMicros converts microseconds to a duration.Duration proto.
func FromMicros(micros int64) *durpb.Duration {
	return ptypes.DurationProto(time.Duration(1e3 * micros))
}
