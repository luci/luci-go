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
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"
	durpb "github.com/golang/protobuf/ptypes/duration"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal"
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

// ReadTestResults reads the specified test results, if any, of an invocation within the transaction.
func ReadTestResults(ctx context.Context, txn Txn, invName string, includeExpected bool, cursorTok string, pageSize int) (trs []*pb.TestResult, nextCursorTok string, err error) {
	invID := pbutil.MustParseInvocationName(invName)

	var (
		table       = "TestResults"
		conditions  = []string{"(InvocationId = @invID)"}
		queryParams = map[string]interface{}{"invID": invID, "limit": pageSize}
	)

	// Set start position if requested.
	switch pos, tokErr := internal.ParsePageToken(cursorTok); {
	case tokErr != nil:
		err = errors.Reason("invalid page_token").
			InternalReason("%s", tokErr).
			Tag(grpcutil.InvalidArgumentTag).Err()
		return

	case pos == nil:
		break

	case len(pos) == 2:
		conditions = append(conditions, `(
			(TestPath > @testPath)
			OR (TestPath = @testPath AND ResultId > @resultID)
		)`)
		queryParams["testPath"] = pos[0]
		queryParams["resultID"] = pos[1]

	default:
		err = errors.Reason("invalid page_token").
			InternalReason("unexpected string slice %q for TestResults cursor position", pos).
			Tag(grpcutil.InvalidArgumentTag).Err()
		return
	}

	// Set conditions for filtering expected results if requested.
	if !includeExpected {
		table += "@{FORCE_INDEX=UnexpectedTestResults}"
		conditions = append(conditions, "IsUnexpected")
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
		FROM %s
		WHERE %s
		ORDER BY TestPath, ResultId
		LIMIT @limit
	`, table, strings.Join(conditions, " AND ")))
	query.Params = queryParams

	it := txn.Query(ctx, query)
	defer it.Stop()

	trs = make([]*pb.TestResult, 0, pageSize)
	for {
		var row *spanner.Row
		row, err = it.Next()
		if err == iterator.Done {
			err = nil
			break
		}
		if err != nil {
			trs = nil
			return
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
			trs = nil
			return
		}

		tr.Name = pbutil.TestResultName(invID, tr.TestPath, tr.ResultId)
		populateExpectedField(tr, maybeUnexpected)
		populateDurationField(tr, micros)

		trs = append(trs, tr)
	}

	// If we got fewer than pageSize, then we've exhausted the collection, so return everything,
	// with a nil-positioned cursor.
	if len(trs) < pageSize {
		return
	}

	// Otherwise, construct the next cursor.
	trLast := trs[pageSize-1]
	nextCursorTok = internal.PageToken(trLast.TestPath, trLast.ResultId)
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
