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

package artifacts

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var followAllEdges = &pb.ArtifactPredicate_EdgeTypeSet{
	IncludedInvocations: true,
	TestResults:         true,
}

// Query specifies artifacts to fetch.
type Query struct {
	InvocationIDs       invocations.IDSet
	ParentIDRegexp      string
	FollowEdges         *pb.ArtifactPredicate_EdgeTypeSet
	TestResultPredicate *pb.TestResultPredicate
	ContentTypeRegexp   string
	ArtifactIDRegexp    string
	PageSize            int // must be positive
	PageToken           string
	WithRBECASHash      bool
	WithGcsURI          bool
	WithRbeURI          bool
	Mask                *mask.Mask
}

// Artifact contains pb.Artifact and its RBECAS hash.
type Artifact struct {
	*pb.Artifact
	RBECASHash string
}

// tmplQueryArtifacts is a template for the SQL expression that queries
// artifacts. See also ArtifactQuery.
var tmplQueryArtifacts = template.Must(template.New("artifactQuery").Parse(`
@{USE_ADDITIONAL_PARALLELISM=TRUE}
WITH VariantsWithUnexpectedResults AS (
	SELECT DISTINCT TestId, VariantHash
	FROM TestResults@{FORCE_INDEX=UnexpectedTestResults, spanner_emulator.disable_query_null_filtered_index_check=true}
	WHERE IsUnexpected AND InvocationId IN UNNEST(@invIDs)
),
VariantsWithUnexpectedResultsOnly AS (
	SELECT TestId, VariantHash
	FROM VariantsWithUnexpectedResults vur
		JOIN@{FORCE_JOIN_ORDER=TRUE, JOIN_METHOD=HASH_JOIN} TestResults tr
			USING (TestId, VariantHash)
	WHERE InvocationId IN UNNEST(@invIDs)
	GROUP BY TestId, VariantHash
	HAVING LOGICAL_AND(IFNULL(IsUnexpected, false))
),
FilteredTestResults AS (
	SELECT InvocationId, FORMAT("tr/%s/%s", TestId, ResultId) as ParentId
	FROM
	{{ if .InterestingTestResults }}
		VariantsWithUnexpectedResults vur
		JOIN@{FORCE_JOIN_ORDER=TRUE, JOIN_METHOD=HASH_JOIN} TestResults tr
			USING (TestId, VariantHash)
	{{ else if .OnlyUnexpectedTestResults }}
		VariantsWithUnexpectedResultsOnly vuro
		JOIN@{FORCE_JOIN_ORDER=TRUE, JOIN_METHOD=HASH_JOIN} TestResults tr
			USING (TestId, VariantHash)
	{{ else }}
		TestResults tr
	{{ end }}
	WHERE InvocationId IN UNNEST(@invIDs)
{{ if .Params.variantHashEquals }}
		AND tr.VariantHash = @variantHashEquals
{{ end }}
{{ if .Params.variantContains }}
		AND (SELECT LOGICAL_AND(kv IN UNNEST(Variant)) FROM UNNEST(@variantContains) kv)
{{ end }}
)
SELECT InvocationId, ParentId, ArtifactId, ContentType, Size,
{{ if .Q.WithRBECASHash }}
	RBECASHash,
{{ end }}
{{ if .Q.WithGcsURI }}
	GcsURI,
{{ end }}
{{ if .Q.WithRbeURI }}
	RBEURI
{{ end }}
FROM Artifacts art
{{ if .JoinWithTestResults }}
LEFT JOIN FilteredTestResults tr USING (InvocationId, ParentId)
{{ end }}
WHERE art.InvocationId IN UNNEST(@invIDs)
{{ if .Params.afterInvocationId }}
	# Skip artifacts after the one specified in the page token.
	AND (
		(art.InvocationId > @afterInvocationId) OR
		(art.InvocationId = @afterInvocationId AND art.ParentId > @afterParentId) OR
		(art.InvocationId = @afterInvocationId AND art.ParentId = @afterParentId AND art.ArtifactId > @afterArtifactId)
	)
{{ end }}
{{ if .Params.ParentIdRegexp }}
	AND REGEXP_CONTAINS(art.ParentId, @ParentIdRegexp)
{{end}}
{{ if .JoinWithTestResults }} AND (art.ParentId = "" OR tr.ParentId IS NOT NULL) {{ end }}
{{ if .Params.contentTypeRegexp }}
		AND REGEXP_CONTAINS(IFNULL(art.ContentType, ""), @contentTypeRegexp)
{{ end }}
{{ if .Params.artifactIdRegexp }}
		AND REGEXP_CONTAINS(IFNULL(art.ArtifactID, ""), @artifactIdRegexp)
{{ end }}
ORDER BY InvocationId, ParentId, ArtifactId
{{ if gt .Q.PageSize 0 }} LIMIT @limit {{ end }}
`))

// genStmt generates a spanner statement and returns it without executing it.
func (q *Query) genStmt(ctx context.Context) (spanner.Statement, error) {
	if q.PageSize < 0 {
		panic("PageSize < 0")
	}

	// Prepare query params.
	params := map[string]any{}
	params["invIDs"] = q.InvocationIDs
	params["limit"] = q.PageSize
	addREParamMaybe(params, "contentTypeRegexp", q.ContentTypeRegexp)
	addREParamMaybe(params, "artifactIdRegexp", q.ArtifactIDRegexp)
	addREParamMaybe(params, "ParentIdRegexp", q.parentIDRegexp())

	if err := invocations.TokenToMap(q.PageToken, params, "afterInvocationId", "afterParentId", "afterArtifactId"); err != nil {
		return spanner.Statement{}, err
	}

	testresults.PopulateVariantParams(params, q.TestResultPredicate.GetVariant())

	// Prepeare statement generation input.
	var input struct {
		JoinWithTestResults       bool
		InterestingTestResults    bool
		OnlyUnexpectedTestResults bool
		Q                         *Query
		Params                    map[string]any
	}
	input.Params = params
	// If we need to filter artifacts by attributes of test results, then
	// join with test results table.
	if q.FollowEdges == nil || q.FollowEdges.TestResults {
		input.JoinWithTestResults = q.TestResultPredicate.GetVariant() != nil
		switch q.TestResultPredicate.GetExpectancy() {
		case pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS:
			input.JoinWithTestResults = true
			input.InterestingTestResults = true
		case pb.TestResultPredicate_VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS:
			input.JoinWithTestResults = true
			input.OnlyUnexpectedTestResults = true
		}
	}
	input.Q = q

	st, err := spanutil.GenerateStatement(tmplQueryArtifacts, input)
	st.Params = params
	return st, err
}

func (q *Query) run(ctx context.Context, f func(*Artifact) error) (err error) {
	st, err := q.genStmt(ctx)
	if err != nil {
		return err
	}
	var b spanutil.Buffer
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		a := &Artifact{
			Artifact: &pb.Artifact{},
		}
		var invID invocations.ID
		var parentID string
		var contentType spanner.NullString
		var size spanner.NullInt64
		var rbecasHash spanner.NullString
		var gcsURI spanner.NullString
		var rbeURI spanner.NullString

		ptrs := []any{
			&invID, &parentID, &a.ArtifactId, &contentType, &size,
		}
		if q.WithRBECASHash {
			ptrs = append(ptrs, &rbecasHash)
		}
		if q.WithGcsURI {
			ptrs = append(ptrs, &gcsURI)
		}
		if q.WithRbeURI {
			ptrs = append(ptrs, &rbeURI)
		}
		if err := b.FromSpanner(row, ptrs...); err != nil {
			return err
		}

		// Initialize artifact name.
		switch testID, resultID, err := ParseParentID(parentID); {
		case err != nil:
			return err
		case testID == "":
			a.Name = pbutil.LegacyInvocationArtifactName(string(invID), a.ArtifactId)
		default:
			a.Name = pbutil.LegacyTestResultArtifactName(string(invID), testID, resultID, a.ArtifactId)
		}

		a.ContentType = contentType.StringVal
		a.SizeBytes = size.Int64
		a.RBECASHash = rbecasHash.StringVal
		a.GcsUri = gcsURI.StringVal
		a.RbeUri = rbeURI.StringVal
		a.HasLines = IsLogSupportedArtifact(a.ArtifactId, a.ContentType)

		return f(a)
	})
}

// Run calls f for artifacts matching the query.
//
// Refer to Fetch() for the ordering of returned artifacts.
func (q *Query) Run(ctx context.Context, f func(*Artifact) error) error {
	if q.PageSize != 0 {
		panic("PageSize is specified when Query.Run")
	}
	return q.run(ctx, f)
}

// FetchProtos returns a page of artifact protos matching q.
//
// Returned artifacts are ordered by level (invocation or test result).
// Test result artifacts are sorted by parent invocation ID, test ID and
// artifact ID.
func (q *Query) FetchProtos(ctx context.Context) (arts []*pb.Artifact, nextPageToken string, err error) {
	if q.PageSize <= 0 {
		panic("PageSize <= 0")
	}

	err = q.run(ctx, func(a *Artifact) error {
		// Always keep the name field because it is used to generate the pagination token.
		name := a.Name
		if err := q.Mask.Trim(a.Artifact); err != nil {
			return errors.Fmt("trimming fields for artifact: %s: %w", name, err)
		}
		a.Name = name
		arts = append(arts, a.Artifact)
		return nil
	})
	if err != nil {
		arts = nil
		return
	}

	// If we got pageSize results, then we haven't exhausted the collection and
	// need to return the next page token.
	if len(arts) == q.PageSize {
		last := arts[q.PageSize-1]
		invID, testID, resultID, artifactID := MustParseLegacyName(last.Name)
		parentID := ParentID(testID, resultID)
		nextPageToken = pagination.Token(string(invID), parentID, artifactID)
	}
	return
}

// parentIDRegexp returns a regular expression for ParentId column.
// Uses q.FollowEdges and q.TestResultPredicate.TestIdRegexp to compute it.
// The returned regexp is not necessarily surrounded with ^ or $.
func (q *Query) parentIDRegexp() string {
	// If it is explicitly specified, use it.
	if q.ParentIDRegexp != "" {
		if q.TestResultPredicate != nil || q.FollowEdges != nil {
			// Do not ignore our bugs.
			panic("explicit ParentIDRegexp is mutually exclusive with TestResultPredicate and FollowEdges")
		}
		return q.ParentIDRegexp
	}

	testIDRE := q.TestResultPredicate.GetTestIdRegexp()
	hasTestIDRE := testIDRE != "" && testIDRE != ".*"

	edges := q.FollowEdges
	if edges == nil {
		edges = followAllEdges
	}

	if edges.IncludedInvocations && edges.TestResults && !hasTestIDRE {
		// Fast path.
		return ".*"
	}

	// Collect alternatives and then combine them with "|".
	var alts []string

	if edges.IncludedInvocations {
		// Invocation-level artifacts have empty parent ID.
		alts = append(alts, "")
	}

	if edges.TestResults {
		// TestResult-level artifacts have parent ID formatted as
		// "tr/{testID}/{resultID}"
		if hasTestIDRE {
			alts = append(alts, fmt.Sprintf("tr/%s/[^/]+", testIDRE))
		} else {
			alts = append(alts, "tr/.+")
		}
	}

	// Note: the surrounding parens are important. Without them any expression
	// matches.
	return fmt.Sprintf("(%s)", strings.Join(alts, "|"))
}

// addREParamMaybe adds a regexp parameter surrounded with ^ and $,
// unless re matches everything.
func addREParamMaybe(params map[string]any, name, re string) {
	if re != "" && re != ".*" {
		params[name] = fmt.Sprintf("^%s$", re)
	}
}
