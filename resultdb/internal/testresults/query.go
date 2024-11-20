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
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"cloud.google.com/go/spanner"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// AllFields is a field mask that selects all TestResults fields.
var AllFields = mask.All(&pb.TestResult{})

// limitedFields is a field mask for TestResult to use when the caller only
// has the listLimited permission for test results.
var limitedFields = mask.MustFromReadMask(&pb.TestResult{},
	"name",
	"test_id",
	"result_id",
	"expected",
	"status",
	"start_time",
	"duration",
	"variant_hash",
	"failure_reason",
	"skip_reason",
)

// limitedReasonLength is the length to which the failure reason's primary error
// message will be truncated for a TestResult when the caller only has the
// listLimited permission for test results.
const limitedReasonLength = 140

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
	"skip_reason",
)

// ListMask returns mask.Mask converted from field_mask.FieldMask.
// It returns a default mask with all fields except summary_html if readMask is
// empty.
func ListMask(readMask *field_mask.FieldMask) (*mask.Mask, error) {
	if len(readMask.GetPaths()) == 0 {
		return defaultListMask, nil
	}
	return mask.FromFieldMask(readMask, &pb.TestResult{}, mask.AdvancedSemantics())
}

// Query specifies test results to fetch.
type Query struct {
	InvocationIDs invocations.IDSet
	Predicate     *pb.TestResultPredicate
	PageSize      int // must be positive
	PageToken     string
	Mask          *mask.Mask
}

func (q *Query) run(ctx context.Context, f func(*pb.TestResult) error) (err error) {
	ctx, ts := tracing.Start(ctx, "testresults.Query.run",
		attribute.Int("cr.dev.invocations", len(q.InvocationIDs)),
	)
	defer func() { tracing.End(ts, err) }()

	switch {
	case q.PageSize < 0:
		panic("PageSize < 0")
	case q.Predicate.GetExcludeExonerated() && q.Predicate.GetExpectancy() == pb.TestResultPredicate_ALL:
		panic("ExcludeExonerated and Expectancy=ALL are mutually exclusive")
	}

	columns, parser := q.selectClause()
	params, err := q.baseParams()
	if err != nil {
		return err
	}

	err = invocations.TokenToMap(q.PageToken, params, "afterInvocationId", "afterTestId", "afterResultId")
	if err != nil {
		return err
	}

	_, filterByTestIdOrVariant := params["literalPrefix"]
	if !filterByTestIdOrVariant {
		_, filterByTestIdOrVariant = params["variant"]
	}

	// Execute the query.
	st := q.genStatement("testResults", map[string]any{
		"params":                  params,
		"columns":                 strings.Join(columns, ", "),
		"onlyUnexpected":          q.Predicate.GetExpectancy() == pb.TestResultPredicate_VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS,
		"withUnexpected":          q.Predicate.GetExpectancy() == pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS || q.Predicate.GetExpectancy() == pb.TestResultPredicate_VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS,
		"excludeExonerated":       q.Predicate.GetExcludeExonerated(),
		"filterByTestIdOrVariant": filterByTestIdOrVariant,
	})
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
func (q *Query) selectClause() (columns []string, parser func(*spanner.Row) (*pb.TestResult, error)) {
	columns = []string{
		"InvocationId",
		"TestId",
		"ResultId",
		"IsUnexpected",
		"Status",
		"StartTime",
		"RunDurationUsec",
	}

	// Select extra columns depending on the mask.
	var extraColumns []string
	readMask := q.Mask
	if readMask.IsEmpty() {
		readMask = defaultListMask
	}
	selectIfIncluded := func(column, field string) {
		switch inc, err := readMask.Includes(field); {
		case err != nil:
			panic(err)
		case inc != mask.Exclude:
			extraColumns = append(extraColumns, column)
			columns = append(columns, column)
		}
	}
	selectIfIncluded("SummaryHtml", "summary_html")
	selectIfIncluded("Tags", "tags")
	selectIfIncluded("TestMetadata", "test_metadata")
	selectIfIncluded("Variant", "variant")
	selectIfIncluded("VariantHash", "variant_hash")
	selectIfIncluded("FailureReason", "failure_reason")
	selectIfIncluded("Properties", "properties")
	selectIfIncluded("SkipReason", "skip_reason")

	// Build a parser function.
	var b spanutil.Buffer
	var summaryHTML spanutil.Compressed
	var tmd spanutil.Compressed
	var fr spanutil.Compressed
	var properties spanutil.Compressed
	parser = func(row *spanner.Row) (*pb.TestResult, error) {
		var invID invocations.ID
		var maybeUnexpected spanner.NullBool
		var micros spanner.NullInt64
		var skipReason spanner.NullInt64
		tr := &pb.TestResult{}

		ptrs := []any{
			&invID,
			&tr.TestId,
			&tr.ResultId,
			&maybeUnexpected,
			&tr.Status,
			&tr.StartTime,
			&micros,
		}

		for _, v := range extraColumns {
			switch v {
			case "SummaryHtml":
				ptrs = append(ptrs, &summaryHTML)
			case "Tags":
				ptrs = append(ptrs, &tr.Tags)
			case "TestMetadata":
				ptrs = append(ptrs, &tmd)
			case "Variant":
				ptrs = append(ptrs, &tr.Variant)
			case "VariantHash":
				ptrs = append(ptrs, &tr.VariantHash)
			case "FailureReason":
				ptrs = append(ptrs, &fr)
			case "Properties":
				ptrs = append(ptrs, &properties)
			case "SkipReason":
				ptrs = append(ptrs, &skipReason)
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
		PopulateExpectedField(tr, maybeUnexpected)
		PopulateDurationField(tr, micros)
		PopulateSkipReasonField(tr, skipReason)
		if err := PopulateTestMetadata(tr, tmd); err != nil {
			return nil, errors.Annotate(err, "error unmarshalling test_metadata for %s", trName).Err()
		}
		if err := PopulateFailureReason(tr, fr); err != nil {
			return nil, errors.Annotate(err, "error unmarshalling failure_reason for %s", trName).Err()
		}
		if err := PopulateProperties(tr, properties); err != nil {
			return nil, errors.Annotate(err, "failed to unmarshal properties").Err()
		}
		if err := q.Mask.Trim(tr); err != nil {
			return nil, errors.Annotate(err, "error trimming fields for %s", trName).Err()
		}
		// Always include name in tr because name is needed to calculate
		// page token.
		tr.Name = trName
		return tr, nil
	}
	return
}

// Fetch returns a page of test results matching q.
// Returned test results are ordered by parent invocation ID, test ID and result
// ID.
func (q *Query) Fetch(ctx context.Context) (trs []*pb.TestResult, nextPageToken string, err error) {
	if q.PageSize <= 0 {
		panic("PageSize <= 0")
	}

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

func (q *Query) baseParams() (map[string]any, error) {
	params := map[string]any{
		"invIDs": q.InvocationIDs,
		"limit":  q.PageSize,
	}

	if re := q.Predicate.GetTestIdRegexp(); re != "" && re != ".*" {
		params["testIdRegexp"] = fmt.Sprintf("^%s$", re)
		r, err := regexp.Compile(re)
		if err != nil {
			return params, err
		}
		// We're trying to match the invocation's CommonTestIDPrefix with re,
		// so re should have a literal prefix, otherwise the match likely
		// fails.
		// For example if an invocation's CommonTestIDPrefix is "ninja://" and
		// re is ".*browser_tests.*", we wouldn't know if that invocation contains
		// any test results with matching test ids or not.
		params["literalPrefix"], _ = r.LiteralPrefix()
	}

	PopulateVariantParams(params, q.Predicate.GetVariant())

	return params, nil
}

// PopulateVariantParams populates variantHashEquals and variantContains
// parameters based on the predicate.
func PopulateVariantParams(params map[string]any, variantPredicate *pb.VariantPredicate) {
	switch p := variantPredicate.GetPredicate().(type) {
	case *pb.VariantPredicate_Equals:
		params["variantHashEquals"] = pbutil.VariantHash(p.Equals)
		params["variant"] = pbutil.VariantToStrings(p.Equals)
	case *pb.VariantPredicate_Contains:
		if len(p.Contains.Def) > 0 {
			params["variantContains"] = pbutil.VariantToStrings(p.Contains)
			params["variant"] = params["variantContains"]
		}
	case nil:
		// No filter.
	default:
		panic(errors.Reason("unexpected variant predicate %q", variantPredicate).Err())
	}
}

// queryTmpl is a set of templates that generate the SQL statements used
// by Query type.
var queryTmpl = template.Must(template.New("").Parse(`
	{{define "testResults"}}
		@{USE_ADDITIONAL_PARALLELISM=TRUE}
		WITH
		{{if .filterByTestIdOrVariant}}
			invs AS (
				SELECT
					i.InvocationId,
				FROM Invocations i
				WHERE i.InvocationId IN UNNEST(@invIDs)
				{{if .params.literalPrefix}}
					AND (
						( i.CommonTestIDPrefix IS NOT NULL
							AND (
								-- For the cases where literalPrefix is long and specific, for example:
								-- literalPrefix = "ninja://chrome/test:browser_tests/AutomationApiTest"
								-- i.CommonTestIDPrefix = "ninja://chrome/test:browser_tests/"
								STARTS_WITH(@literalPrefix, i.CommonTestIDPrefix) OR
								-- For the cases where literalPrefix is short, likely because predicate.TestIdRegexp
								-- contains non-literal regexp, for example:
								-- predicate.TestIdRegexp = "ninja://.*browser_tests/" which makes literalPrefix
								-- to be "ninja://".
								-- This condition is not very useful to improve the performance of test
								-- result history API, but without it will make the results incomplete.
								STARTS_WITH(i.CommonTestIDPrefix, @literalPrefix)
							)
						)
						-- To make sure the query can still work for the old invocations
						-- that were created without CommonTestIDPrefix.
						OR (i.CommonTestIDPrefix IS NULL AND i.CreateTime < "2021-4-13")
					)
				{{end}}
				{{if .params.variant}}
					AND (
						(SELECT LOGICAL_AND(kv IN UNNEST(i.TestResultVariantUnion)) FROM UNNEST(@variant) kv)
						-- To make sure the query can still work for the old invocations
						-- that were created without TestResultVariantUnion.
						OR (i.TestResultVariantUnion IS NULL AND i.CreateTime < "2021-4-13")
					)
				{{end}}
			),
		{{else}}
			invs AS (
				SELECT *
				FROM UNNEST(@invIDs)
				AS InvocationId
			),
		{{end}}

		{{if .excludeExonerated}}
			testVariants AS (
				{{template "variantsWithUnexpectedResults" .}}
			),
			exonerated AS (
				SELECT DISTINCT TestId, VariantHash
				FROM TestExonerations
				WHERE InvocationId IN UNNEST(@invIDs)
				{{template "testIDAndVariantFilter" .}}
			),
			variantsWithUnexpectedResults AS (
				SELECT tv.*
				FROM testVariants tv
				LEFT JOIN exonerated USING(TestId, VariantHash)
				WHERE exonerated.TestId IS NULL
			),
		{{else}}
			variantsWithUnexpectedResults AS (
				{{template "variantsWithUnexpectedResults" .}}
			),
		{{end}}

		withUnexpected AS (
			SELECT {{.columns}}
			FROM invs
			JOIN (
				variantsWithUnexpectedResults vur
				JOIN@{FORCE_JOIN_ORDER=TRUE, JOIN_METHOD=HASH_JOIN} TestResults tr USING (TestId, VariantHash)
			) USING(InvocationId)
			{{/*
				Don't have to use testIDAndVariantFilter because
				variantsWithUnexpectedResults is already filtered
			*/}}
		)

		{{if .onlyUnexpected}}
			, withOnlyUnexpected AS (
				SELECT ARRAY_AGG(tr) trs
				FROM withUnexpected tr
				GROUP BY TestId, VariantHash
				{{/*
					All results of the TestID and VariantHash are unexpected.
					IFNULL() is significant because LOGICAL_AND() skips nulls.
				*/}}
				HAVING LOGICAL_AND(IFNULL(IsUnexpected, false))
			)
			SELECT tr.*
			FROM withOnlyUnexpected owu, owu.trs tr
			WHERE true {{/* For optional conditions below */}}
		{{else if .withUnexpected}}
			SELECT * FROM withUnexpected
			WHERE true {{/* For optional conditions below */}}
		{{else}}
			SELECT {{.columns}}
			FROM invs
			JOIN  TestResults tr USING(InvocationId)
			WHERE TRUE -- placeholder of the fist where clause, so that testIDAndVariantFilter can always start with "AND".
				{{template "testIDAndVariantFilter" .}}
		{{end}}

		{{/* Apply the page token */}}
		{{if .params.afterInvocationId}}
			AND (
				(InvocationId > @afterInvocationId) OR
				(InvocationId = @afterInvocationId AND TestId > @afterTestId) OR
				(InvocationId = @afterInvocationId AND TestId = @afterTestId AND ResultId > @afterResultId)
			)
		{{end}}
		ORDER BY InvocationId, TestId, ResultId
		{{if .params.limit}}LIMIT @limit{{end}}
	{{end}}

	{{define "variantsWithUnexpectedResults"}}
		SELECT DISTINCT TestId, VariantHash
		FROM invs
		JOIN TestResults@{FORCE_INDEX=UnexpectedTestResults, spanner_emulator.disable_query_null_filtered_index_check=true} USING(InvocationId)
		WHERE IsUnexpected
		{{template "testIDAndVariantFilter" .}}
	{{end}}

	{{define "testIDAndVariantFilter"}}
		{{/* Filter by Test ID */}}
		{{if .params.testIdRegexp}}
			AND REGEXP_CONTAINS(TestId, @testIdRegexp)
		{{end}}

		{{/* Filter by Variant */}}
		{{if .params.variantHashEquals}}
			AND VariantHash = @variantHashEquals
		{{end}}
		{{if .params.variantContains }}
			AND (SELECT LOGICAL_AND(kv IN UNNEST(Variant)) FROM UNNEST(@variantContains) kv)
		{{end}}
	{{end}}
`))

func (*Query) genStatement(templateName string, input map[string]any) spanner.Statement {
	var sql bytes.Buffer
	err := queryTmpl.ExecuteTemplate(&sql, templateName, input)
	if err != nil {
		panic(fmt.Sprintf("failed to generate a SQL statement: %s", err))
	}
	return spanner.Statement{SQL: sql.String(), Params: input["params"].(map[string]any)}
}

// ToLimitedData limits the given TestResult to the fields allowed when
// the caller only has the listLimited permission for test results.
func ToLimitedData(ctx context.Context, tr *pb.TestResult) error {
	if err := limitedFields.Trim(tr); err != nil {
		return err
	}

	if tr.FailureReason != nil {
		tr.FailureReason.PrimaryErrorMessage = truncateErrorMessage(
			tr.FailureReason.PrimaryErrorMessage, limitedReasonLength)

		for i := range tr.FailureReason.Errors {
			tr.FailureReason.Errors[i].Message = truncateErrorMessage(
				tr.FailureReason.Errors[i].Message, limitedReasonLength)
		}
	}

	tr.IsMasked = true
	return nil
}

// truncateErrorMessage truncates the error message if its length exceeds the
// limit.
func truncateErrorMessage(errorMessage string, maxLength int) string {
	if len(errorMessage) <= maxLength {
		return errorMessage
	}

	runes := []rune(errorMessage)
	return string(runes[:maxLength]) + "..."
}
