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
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"
	durpb "github.com/golang/protobuf/ptypes/duration"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// ReadTestResult reads specified TestResult within the transaction.
// If the TestResult does not exist, the returned error is annotated with
// NotFound GRPC code.
func ReadTestResult(ctx context.Context, txn Txn, name string) (*pb.TestResult, error) {
	invID, testPath, resultID := pbutil.MustParseTestResultName(name)
	tr := &pb.TestResult{
		Name:     name,
		TestPath: testPath,
		ResultId: resultID,
		Expected: true,
	}

	var maybeUnexpected spanner.NullBool
	var micros int64
	err := ReadRow(ctx, txn, "TestResults", spanner.Key{invID, testPath, resultID}, map[string]interface{}{
		"ExtraVariantPairs": &tr.ExtraVariantPairs,
		"IsUnexpected":      &maybeUnexpected,
		"Status":            &tr.Status,
		"SummaryMarkdown":   &tr.SummaryMarkdown,
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

	populateExpectedField(tr, maybeUnexpected)
	populateDurationField(tr, micros)
	return tr, nil
}

// ReadTestResults reads all test results, if any, of an invocation within the transaction.
func ReadTestResults(ctx context.Context, txn Txn, name string, excludeExpected bool, cursor *internalpb.Cursor, pageSize int) ([]*pb.TestResult, *internalpb.Cursor, error) {
	// Construct query for test results.
	invID := pbutil.MustParseInvocationName(name)
	queryParams := map[string]interface{}{
		"invID": invID,
		"limit": pageSize + 1, // request one more so we know if we've exhausted the collection
	}

	positionClause := ""
	if key := cursor.GetPosition(); key != nil {
		if len(key) != 2 {
			return nil, nil, errors.Reason(
				"expected string slice of {TestPath, ResultId} for TestResults cursor position, got %q",
				key).Err()
		}

		positionClause = `AND (
			(TestPath > @testPath)
			OR (TestPath = @testPath AND ResultId >= @resultId))`
		queryParams["testPath"] = key[0]
		queryParams["resultId"] = key[1]
	}

	expectedClause := ""
	if excludeExpected {
		expectedClause = "AND IsUnexpected"
	}

	query := spanner.NewStatement(fmt.Sprintf(`
		SELECT
			TestPath,
			ResultId,
			ExtraVariantPairs,
			IsUnexpected,
			Status,
			SummaryMarkdown,
			StartTime,
			RunDurationUsec,
			Tags,
			InputArtifacts,
			OutputArtifacts
		FROM TestResults
		WHERE InvocationId=@invID
			%s
			%s
		ORDER BY CommitTimestamp, TestPath, ResultId ASC
		LIMIT @limit
		`, positionClause, expectedClause))
	query.Params = queryParams

	// Fetch the TestResults.
	it := txn.Query(ctx, query)
	defer it.Stop()

	trs := []*pb.TestResult{}
	for {
		row, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, nil, err
		}

		var maybeUnexpected spanner.NullBool
		var micros int64

		tr := &pb.TestResult{}
		err = FromSpanner(row,
			&tr.TestPath,
			&tr.ResultId,
			&tr.ExtraVariantPairs,
			&maybeUnexpected,
			&tr.Status,
			&tr.SummaryMarkdown,
			&tr.StartTime,
			&micros,
			&tr.Tags,
			&tr.InputArtifacts,
			&tr.OutputArtifacts,
		)
		if err != nil {
			return nil, nil, err
		}

		tr.Name = pbutil.TestResultName(invID, tr.TestPath, tr.ResultId)
		populateExpectedField(tr, maybeUnexpected)
		populateDurationField(tr, micros)

		trs = append(trs, tr)
	}

	// If we got fewer than pageSize+1, then we've exhausted the collection, so return everything,
	// with a nil-positioned cursor.
	if len(trs) < pageSize+1 {
		return trs, internal.NewCursor(nil), nil
	}

	// Otherwise, construct the next cursor.
	trLast := trs[pageSize]
	return trs[:pageSize], internal.NewCursor([]string{trLast.TestPath, trLast.ResultId}), nil
}

func populateDurationField(tr *pb.TestResult, micros int64) {
	if micros > 0 {
		tr.Duration = FromMicros(micros)
	}
}

func populateExpectedField(tr *pb.TestResult, maybeUnexpected spanner.NullBool) {
	if maybeUnexpected.Valid {
		tr.Expected = !maybeUnexpected.Bool
	}
}

// ToMicros converts a duration.Duration proto to microseconds.
func ToMicros(d *durpb.Duration) int64 {
	return 1e6*d.Seconds + int64(1e-3*float64(d.Nanos))
}

// FromMicros converts microseconds to a duration.Duration proto.
func FromMicros(micros int64) *durpb.Duration {
	return ptypes.DurationProto(time.Duration(1e3 * micros))
}
