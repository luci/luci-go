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

package testresults

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/trace"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// AllFields is a field mask that selects all TestResults fields.
var AllFields = mask.All(&pb.TestResult{})

// defaultListMask is the default field mask to use for QueryTestResults and
// ListTestResults requests.
var defaultListMask = mask.MustFromReadMask(&pb.TestResult{},
	"name",
	"test_id",
	"result_id",
	"variant",
	"variant_hash",
	"expected",
	"status",
	"start_time",
	"duration",
	"test_location",
)

// ListMask returns mask.Mask converted from field_mask.FieldMask.
// It returns a default mask with all fields except summary_html if readMask is
// empty.
func ListMask(readMask *field_mask.FieldMask) (mask.Mask, error) {
	if len(readMask.GetPaths()) == 0 {
		return defaultListMask, nil
	}
	return mask.FromFieldMask(readMask, &pb.TestResult{}, false, false)
}

// Query specifies test results to fetch.
type Query struct {
	InvocationIDs invocations.IDSet
	Predicate     *pb.TestResultPredicate
	PageSize      int // must be positive
	PageToken     string
	Mask          mask.Mask
}

func (q *Query) run(ctx context.Context, f func(*pb.TestResult) error) (err error) {
	ctx, ts := trace.StartSpan(ctx, "testresults.Query.run")
	defer func() { ts.End(err) }()

	if q.PageSize < 0 {
		panic("PageSize < 0")
	}

	limit := ""
	if q.PageSize > 0 {
		limit = `LIMIT @limit`
	}

	selectSQL, parser := q.selectClause()

	st := spanner.NewStatement(fmt.Sprintf(`
		@{USE_ADDITIONAL_PARALLELISM=TRUE}
		%s
		FROM TestResults tr
		WHERE InvocationId IN UNNEST(@invIDs)
			# Skip test results after the one specified in the page token.
			AND (
				(tr.InvocationId > @afterInvocationId) OR
				(tr.InvocationId = @afterInvocationId AND tr.TestId > @afterTestId) OR
				(tr.InvocationId = @afterInvocationId AND tr.TestId = @afterTestId AND tr.ResultId > @afterResultId)
			)
			AND (@testVariants IS NULL OR STRUCT(tr.TestId, tr.VariantHash) IN UNNEST(@testVariants))
			AND REGEXP_CONTAINS(tr.TestId, @TestIdRegexp)
			AND (@variantHashEquals IS NULL OR tr.VariantHash = @variantHashEquals)
			AND (@variantContains IS NULL
				OR ARRAY_LENGTH(@variantContains) = 0
				OR (SELECT LOGICAL_AND(kv IN UNNEST(tr.Variant)) FROM UNNEST(@variantContains) kv)
			)
		ORDER BY InvocationId, TestId, ResultId
		%s
	`, selectSQL, limit))
	st.Params["invIDs"] = q.InvocationIDs
	st.Params["limit"] = q.PageSize
	st.Params["testVariants"] = []*testVariant(nil)

	// Filter by expectency.
	if q.Predicate.GetExpectancy() == pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS {
		switch testVariants, err := q.fetchVariantsWithUnexpectedResults(ctx); {
		case err != nil:
			return errors.Annotate(err, "failed to fetch variants with unexpected results").Err()
		case len(testVariants) == 0:
			return nil
		default:
			st.Params["testVariants"] = testVariants
		}
	}

	// Filter by test id.
	testIDRegexp := q.Predicate.GetTestIdRegexp()
	if testIDRegexp == "" {
		testIDRegexp = ".*"
	}
	st.Params["TestIdRegexp"] = fmt.Sprintf("^%s$", testIDRegexp)

	// Filter by variant.
	PopulateVariantParams(&st, q.Predicate.GetVariant())

	// Apply page token.
	err = invocations.TokenToMap(q.PageToken, st.Params, "afterInvocationId", "afterTestId", "afterResultId")
	if err != nil {
		return err
	}

	// Read the results.
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		tr, err := parser(row)
		if err != nil {
			return err
		}
		return f(tr)
	})
}

// select returns the SELECT clause of the SQL statement to fetch test results,
// as well as a function to convert a Spanner row to a TestResult message.
// The returned SELECT clause assumes that the TestResults has alias "tr".
// The returned parser is stateful and must not be called concurrently.
func (q *Query) selectClause() (sql string, parser func(*spanner.Row) (*pb.TestResult, error)) {
	sql = `SELECT
		tr.InvocationId,
		tr.TestId,
		tr.ResultId,
		tr.Variant,
		tr.VariantHash,
		tr.IsUnexpected,
		tr.Status,
		tr.StartTime,
		tr.RunDurationUsec,
		tr.TestLocationFileName,
		tr.TestLocationLine,
	`

	// Select extra columns depending on the mask.
	var extraSelect []string
	readMask := q.Mask
	if readMask.IsEmpty() {
		readMask = defaultListMask
	}
	selectIfIncluded := func(column, field string) {
		switch inc, err := readMask.Includes(field); {
		case err != nil:
			panic(err)
		case inc != mask.Exclude:
			extraSelect = append(extraSelect, column)
		}
	}
	selectIfIncluded("tr.SummaryHtml", "summary_html")
	selectIfIncluded("tr.Tags", "tags")
	sql += strings.Join(extraSelect, ",")

	// Build a parser function.
	var b spanutil.Buffer
	var summaryHTML spanutil.Compressed
	parser = func(row *spanner.Row) (*pb.TestResult, error) {
		var invID invocations.ID
		var maybeUnexpected spanner.NullBool
		var micros spanner.NullInt64
		var testLocationFileName spanner.NullString
		var testLocationLine spanner.NullInt64
		tr := &pb.TestResult{}

		ptrs := []interface{}{
			&invID,
			&tr.TestId,
			&tr.ResultId,
			&tr.Variant,
			&tr.VariantHash,
			&maybeUnexpected,
			&tr.Status,
			&tr.StartTime,
			&micros,
			&testLocationFileName,
			&testLocationLine,
		}

		for _, v := range extraSelect {
			switch v {
			case "tr.SummaryHtml":
				ptrs = append(ptrs, &summaryHTML)
			case "tr.Tags":
				ptrs = append(ptrs, &tr.Tags)
			default:
				panic("impossible")
			}
		}

		err := b.FromSpanner(row, ptrs...)
		if err != nil {
			return nil, err
		}

		// Generate test result name now in case tr.TestId and tr.ResultId become
		// empty after q.Mask.Trim(tr).
		trName := pbutil.TestResultName(string(invID), tr.TestId, tr.ResultId)
		tr.SummaryHtml = string(summaryHTML)
		populateExpectedField(tr, maybeUnexpected)
		populateDurationField(tr, micros)
		populateTestLocation(tr, testLocationFileName, testLocationLine)
		if err := q.Mask.Trim(tr); err != nil {
			return nil, errors.Annotate(err, "error trimming fields for %s", tr.Name).Err()
		}
		// Always include name in tr because name is needed to calculate
		// page token.
		tr.Name = trName
		return tr, nil
	}
	return
}

type testVariant struct {
	TestID      string
	VariantHash string
}

func (q *Query) fetchVariantsWithUnexpectedResults(ctx context.Context) (testVariants []testVariant, err error) {
	ctx, ts := trace.StartSpan(ctx, "testresults.Query.fetchVariantsWithUnexpectedResults")
	defer func() { ts.End(err) }()

	set := map[testVariant]struct{}{}
	var mu sync.Mutex

	st := spanner.NewStatement(`
		@{USE_ADDITIONAL_PARALLELISM=TRUE}
		SELECT DISTINCT TestId, VariantHash
		FROM TestResults@{FORCE_INDEX=UnexpectedTestResults}
		WHERE IsUnexpected AND InvocationId IN UNNEST(@invIDs)
	`)
	st.Params["invIDs"] = q.InvocationIDs
	subStmts := invocations.ShardStatement(st, "invIDs")

	eg, ctx := errgroup.WithContext(ctx)
	for _, st := range subStmts {
		st := st
		eg.Go(func() error {
			return spanutil.Query(ctx, st, func(row *spanner.Row) error {
				var tv testVariant
				if err := row.Columns(&tv.TestID, &tv.VariantHash); err != nil {
					return err
				}

				mu.Lock()
				set[tv] = struct{}{}
				mu.Unlock()
				return nil
			})
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	testVariants = make([]testVariant, 0, len(set))
	for tv := range set {
		testVariants = append(testVariants, tv)
	}

	return testVariants, nil
}

// Fetch returns a page of test results matching q.
// Returned test results are ordered by parent invocation ID, test ID and result
// ID.
func (q *Query) Fetch(ctx context.Context) (trs []*pb.TestResult, nextPageToken string, err error) {
	if q.PageSize <= 0 {
		panic("PageSize <= 0")
	}

	trs = make([]*pb.TestResult, 0, q.PageSize)
	err = q.run(ctx, func(tr *pb.TestResult) error {
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
		invID, testID, resultID := MustParseName(last.Name)
		nextPageToken = pagination.Token(string(invID), testID, resultID)
	}
	return
}

// Run calls f for test results matching the query.
// The test results are ordered by parent invocation ID, test ID and result ID.
func (q *Query) Run(ctx context.Context, f func(*pb.TestResult) error) error {
	if q.PageSize > 0 {
		panic("PageSize is specified when Query.Run")
	}
	return q.run(ctx, f)
}

// PopulateVariantParams populates variantHashEquals and variantContains
// parameters based on the predicate.
func PopulateVariantParams(st *spanner.Statement, variantPredicate *pb.VariantPredicate) {
	st.Params["variantHashEquals"] = spanner.NullString{}
	st.Params["variantContains"] = []string(nil)
	switch p := variantPredicate.GetPredicate().(type) {
	case *pb.VariantPredicate_Equals:
		st.Params["variantHashEquals"] = pbutil.VariantHash(p.Equals)
	case *pb.VariantPredicate_Contains:
		st.Params["variantContains"] = pbutil.VariantToStrings(p.Contains)
	case nil:
		// No filter.
	default:
		panic(errors.Reason("unexpected variant predicate %q", variantPredicate).Err())
	}
}
