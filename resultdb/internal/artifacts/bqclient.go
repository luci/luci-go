// Copyright 2024 The LUCI Authors.
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
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"text/template"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/pagination"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type BQClient interface {
	ReadArtifactGroups(ctx context.Context, opts ReadArtifactGroupsOpts) (groups []*ArtifactGroup, nextPageToken string, err error)
	ReadArtifacts(ctx context.Context, opts ReadArtifactsOpts) (rows []*MatchingArtifact, nextPageToken string, err error)
}

// NewClient creates a new client for reading text_artifacts table.
func NewClient(ctx context.Context, gcpProject string) (*Client, error) {
	client, err := bq.NewClient(ctx, gcpProject)
	if err != nil {
		return nil, err
	}
	return &Client{client: client}, nil
}

// Client to read ResultDB text_artifacts.
type Client struct {
	client *bigquery.Client
}

// Close releases any resources held by the client.
func (c *Client) Close() error {
	return c.client.Close()
}

// ArtifactGroup represents matching artifacts in each (test id, variant hash, artifact id) group.
// This can be either invocation or test result level artifacts.
type ArtifactGroup struct {
	// For invocation level artifact, this field will be empty.
	TestID string
	// For invocation level artifact, this is union of all variants of test results directly included by the invocation.
	// For result level artifact, this is the test variant.
	Variant bigquery.NullJSON
	// A hash of the variant above.
	VariantHash string
	ArtifactID  string
	// At most 3 matching artifacts are included here.
	// MatchingArtifacts are sorted in partition_time DESC, invocation_id, result_id order.
	Artifacts []*MatchingArtifact
	// Total number of matching artifacts.
	MatchingCount int32
	// Maximum partition_time of artifacts in this group.
	MaxPartitionTime time.Time
}

// MatchingArtifact is a single matching artifact.
type MatchingArtifact struct {
	InvocationID string
	// For invocation level artifact, this field will be empty.
	ResultID string
	// The creation time of the parent invocation.
	PartitionTime time.Time
	// Test status associated with this artifact.
	// For invocation level artifact, this field will be empty.
	TestStatus bigquery.NullString
	// Artifact content that matches the search.
	Match string
	// Artifact content that is immediately before the match. At most one line above.
	MatchWithContextBefore string
	// Artifact content that is immediately after the match. At most one line below.
	MatchWithContextAfter string
}

func parseReadArtifactGroupsPageToken(pageToken string) (afterMaxPartitionTimeUnix, maxInsertTimeUnix int, afterTestID, afterVariantHash, afterArtifactID string, err error) {
	tokens, err := pagination.ParseToken(pageToken)
	if err != nil {
		return 0, 0, "", "", "", err
	}

	if len(tokens) != 5 {
		return 0, 0, "", "", "", pagination.InvalidToken(errors.Reason("expected 5 components, got %d", len(tokens)).Err())
	}

	afterMaxPartitionTimeUnix, err = strconv.Atoi(tokens[0])
	if err != nil {
		return 0, 0, "", "", "", pagination.InvalidToken(errors.Reason("expect the first page_token component to be an integer").Err())
	}
	maxInsertTimeUnix, err = strconv.Atoi(tokens[1])
	if err != nil {
		return 0, 0, "", "", "", pagination.InvalidToken(errors.Reason("expect the second page_token component to be an integer").Err())
	}
	return afterMaxPartitionTimeUnix, maxInsertTimeUnix, tokens[2], tokens[3], tokens[4], nil
}

var maxMatchSize = 10 * 1024 // 10kib.

// TODO (beining@): This query only searches from the first 10 MB of each artifacts.
// Big artifacts which have being splitted across multiple rows (as we have a limit of 10MB for a BQ row), only the first row will be included in the search.
// We need to consider three scenarios if we want search across all shards
// - The match content may split to multiple shards
// - The match content is fully in a shard, but a context being split in another shard
// - Multiple match content in different shards
var readArtifactGroupTmpl = template.Must(template.New("").Parse(`
	WITH dedup AS (
		SELECT
			invocation_id,
			test_id,
			result_id,
			artifact_id,
			-- variant is used as grouping key, and pagination token in the following query.
			-- For invocation level artifact, the invocation variant union is used.
			-- For result level artifact, the test variant is used.
			{{if .invocationLevel}}
				ANY_VALUE(invocation_variant_union) as variant,
				ANY_VALUE(invocation_variant_union_hash) as variant_hash,
			{{else}}
				ANY_VALUE(test_variant) as variant,
				ANY_VALUE(test_variant_hash) as variant_hash,
			{{end}}
			ANY_VALUE(content) as content,
			ANY_VALUE(test_status) as test_status,
			ANY_VALUE(partition_time) as partition_time,
  	FROM text_artifacts
		WHERE project = @project
			-- Only include the first shard for each artifact.
			AND shard_id = 0
			AND REGEXP_CONTAINS(content, @searchRegex)
			{{if .prefixMatchTestID}}
				AND STARTS_WITH(test_id, @testIDMatcherString)
			{{else if .exactMatchTestID}}
				AND test_id = @testIDMatcherString
			{{end}}
			{{if .prefixMatchArtifactID}}
				AND STARTS_WITH(artifact_id, @artifactIDMatcherString)
			{{else if .exactMatchArtifactID}}
				AND artifact_id = @artifactIDMatcherString
			{{end}}
			-- Start time is exclusive.
			AND partition_time > @startTime
			-- End time is inclusive.
			AND partition_time <= @endTime
			{{if .invocationLevel}}
				-- Invocation level artifact has empty test id.
				AND test_id = ""
			{{else}}
				-- Exclude invocation level artifact.
				AND test_id != ""
			{{end}}
			-- Only return artifacts which caller has permission to access.
			AND realm in UNNEST(@subRealms)
			-- Exclude rows that are added after the max insert time.
			AND UNIX_SECONDS(insert_time) < @maxInsertTimeUnix
		GROUP BY project, test_id, artifact_id, invocation_id, result_id, shard_id
	)
	SELECT test_id as TestID,
			ANY_VALUE(variant) as Variant,
			variant_hash as VariantHash,
			artifact_id as ArtifactID,
			COUNT(*) as MatchingCount,
			max(partition_time) as MaxPartitionTime,
			ARRAY_AGG(STRUCT(
					invocation_id as InvocationID,
					result_id as ResultID,
					partition_time as PartitionTime,
					test_status as TestStatus,
					SUBSTR(REGEXP_EXTRACT(content, @searchRegex), 0, @maxMatchSize) AS Match,
					-- Truncate from the front for context before the match. This is done by reversing the returned string, truncate from the back and reverse the string back.
					REVERSE(SUBSTR(REVERSE(REGEXP_EXTRACT(content, @searchContextBeforeRegex)), 0, @maxMatchSize)) AS MatchWithContextBefore,
					SUBSTR(REGEXP_EXTRACT(content, @searchContextAfterRegex), 0, @maxMatchSize) AS MatchWithContextAfter)
				ORDER BY partition_time DESC, invocation_id, result_id LIMIT 3) as Artifacts,
		FROM dedup
		GROUP BY test_id, variant_hash, artifact_id
		{{if .pagination}}
			HAVING
				UNIX_SECONDS(MaxPartitionTime) < @afterMaxPartitionTimeUnix
				OR (UNIX_SECONDS(MaxPartitionTime) = @afterMaxPartitionTimeUnix AND TestID > @afterTestID)
				OR (UNIX_SECONDS(MaxPartitionTime) = @afterMaxPartitionTimeUnix AND TestID = @afterTestID AND VariantHash > @afterVariantHash)
				OR (UNIX_SECONDS(MaxPartitionTime) = @afterMaxPartitionTimeUnix AND TestID = @afterTestID AND VariantHash = @afterVariantHash AND  ArtifactID > @afterArtifactID)
		{{end}}
		ORDER BY MaxPartitionTime DESC , TestID, VariantHash, ArtifactID
		LIMIT @limit
	`))

func generateReadArtifactGroupsQuery(opts ReadArtifactGroupsOpts) (string, error) {
	var b bytes.Buffer
	data := map[string]any{
		"pagination":            opts.PageToken != "",
		"prefixMatchArtifactID": isPrefixMatch(opts.ArtifactIDMatcher),
		"exactMatchArtifactID":  isExactMatch(opts.ArtifactIDMatcher),
		"invocationLevel":       opts.IsInvocationLevel,
	}
	if !opts.IsInvocationLevel {
		data["prefixMatchTestID"] = isPrefixMatch(opts.TestIDMatcher)
		data["exactMatchTestID"] = isExactMatch(opts.TestIDMatcher)
	}
	err := readArtifactGroupTmpl.ExecuteTemplate(&b, "", data)
	if err != nil {
		return "", errors.Annotate(err, "execute template").Err()
	}
	return b.String(), nil
}

type ReadArtifactGroupsOpts struct {
	Project      string
	SearchString *pb.ArtifactContentMatcher
	// This field is ignore, if IsInvocationLevel is True.
	TestIDMatcher     *pb.IDMatcher
	ArtifactIDMatcher *pb.IDMatcher
	// If true, query invocation level artifacts. Otherwise, query test result level artifacts.
	IsInvocationLevel bool
	StartTime         time.Time
	EndTime           time.Time
	SubRealms         []string
	Limit             int
	PageToken         string
}

// ReadArtifactGroups reads either test result level artifacts or invocation level artifacts depending on opts.IsInvocationLevel.
func (c *Client) ReadArtifactGroups(ctx context.Context, opts ReadArtifactGroupsOpts) (groups []*ArtifactGroup, nextPageToken string, err error) {
	var afterMaxPartitionTimeUnix int
	// We exclude rows that are added after the max insert time from this query, and queries for subsequent pages.
	// Because newly added rows might cause some test variant artifact group disappear during pagination (adding a new row with partition_time greater than afterMaxPartitionTimeUnix).
	// Insert_time column in text_artifacts table is the server time of row insertions.
	// Because there might be some latency between insert_time and the time when the row becomes visible to queries.
	// We substract 10 seconds here as it is relatively safe to assume that rows with insert_time t-10s should all be visible at time t.
	maxInsertTimeUnix := int(time.Now().Add(-10 * time.Second).Unix())
	var afterTestID, afterVariantHash, afterArtifactID string
	if opts.PageToken != "" {
		afterMaxPartitionTimeUnix, maxInsertTimeUnix, afterTestID, afterVariantHash, afterArtifactID, err = parseReadArtifactGroupsPageToken(opts.PageToken)
		if err != nil {
			return nil, "", err
		}
	}
	query, err := generateReadArtifactGroupsQuery(opts)
	if err != nil {
		return nil, "", err
	}

	q := c.client.Query(query)
	q.DefaultDatasetID = "internal"
	q.Parameters = []bigquery.QueryParameter{
		{Name: "project", Value: opts.Project},
		{Name: "testIDMatcherString", Value: idMatchString(opts.TestIDMatcher)},
		{Name: "artifactIDMatcherString", Value: idMatchString(opts.ArtifactIDMatcher)},
		{Name: "searchRegex", Value: newMatchWithContextRegexBuilder(opts.SearchString).withCaptureMatch(true).build()},
		{Name: "searchContextBeforeRegex", Value: newMatchWithContextRegexBuilder(opts.SearchString).withCaptureContextBefore(true).build()},
		{Name: "searchContextAfterRegex", Value: newMatchWithContextRegexBuilder(opts.SearchString).withCaptureContextAfter(true).build()},
		{Name: "maxMatchSize", Value: maxMatchSize},
		{Name: "startTime", Value: opts.StartTime},
		{Name: "endTime", Value: opts.EndTime},
		{Name: "subRealms", Value: opts.SubRealms},
		{Name: "limit", Value: opts.Limit},
		{Name: "afterMaxPartitionTimeUnix", Value: afterMaxPartitionTimeUnix},
		{Name: "afterTestID", Value: afterTestID},
		{Name: "afterVariantHash", Value: afterVariantHash},
		{Name: "afterArtifactID", Value: afterArtifactID},
		{Name: "maxInsertTimeUnix", Value: maxInsertTimeUnix},
	}

	it, err := q.Read(ctx)
	if err != nil {
		return nil, "", errors.Annotate(err, "running query").Err()
	}
	results := []*ArtifactGroup{}
	for {
		row := &ArtifactGroup{}
		err := it.Next(row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, "", errors.Annotate(err, "obtain next artifact groups row").Err()
		}
		results = append(results, row)
	}
	if opts.Limit != 0 && len(results) == int(opts.Limit) {
		lastGroup := results[len(results)-1]
		// Page token are constructed in the same way for both invocation and test result level artifact.
		// For invocation artifact, test id is already empty string, the rest of the token element are enough to ensure absolute ordering.
		nextPageToken = pagination.Token(fmt.Sprint(lastGroup.MaxPartitionTime.Unix()), fmt.Sprint(maxInsertTimeUnix), lastGroup.TestID, lastGroup.VariantHash, lastGroup.ArtifactID)
	}
	return results, nextPageToken, nil
}

// TODO (beining@): This query only search the first 10MB of each artifact.
// See the comment of readArtifactGroupTmpl for more detail.
var readArtifactsTmpl = template.Must(template.New("").Parse(`
	SELECT
		invocation_id as InvocationID,
		result_id as ResultID,
		ANY_VALUE(partition_time) as PartitionTime,
		ANY_VALUE(test_status) as TestStatus,
		SUBSTR(REGEXP_EXTRACT(ANY_VALUE(content), @searchRegex), 0, @maxMatchSize) AS Match,
		-- Truncate from the front for context before the match. This is done by reversing the returned string, truncate from the back and reverse the string back.
		REVERSE(SUBSTR(REVERSE(REGEXP_EXTRACT(ANY_VALUE(content), @searchContextBeforeRegex)), 0, @maxMatchSize)) AS MatchWithContextBefore,
		SUBSTR(REGEXP_EXTRACT(ANY_VALUE(content), @searchContextAfterRegex), 0, @maxMatchSize) AS MatchWithContextAfter
	FROM text_artifacts
	WHERE project = @project
		{{if .invocationLevel }}
				AND test_id = ""
				AND invocation_variant_union_hash = @variantHash
		{{else}}
				AND test_id = @testID
				AND test_variant_hash = @variantHash
		{{end}}
		AND artifact_id = @artifactID
		-- Only include the first shard for each artifact.
		AND shard_id = 0
		AND REGEXP_CONTAINS(content, @searchRegex)
		-- Start time is exclusive.
		AND partition_time > @startTime
		-- End time is inclusive.
		AND partition_time <= @endTime
		-- Only return artifacts which caller has permission to access.
		AND realm in UNNEST(@subRealms)
	{{if .pagination}}
		AND (
			UNIX_SECONDS(partition_time) < @afterPartitionTimeUnix
			OR (UNIX_SECONDS(partition_time) = @afterPartitionTimeUnix AND invocation_id > @afterInvocationID)
			OR (UNIX_SECONDS(partition_time) = @afterPartitionTimeUnix AND invocation_id = @afterInvocationID AND result_id > @afterResultID)
		)
	{{end}}
	-- deduplicate rows.
	GROUP BY project, test_id, artifact_id, invocation_id, result_id, shard_id
	ORDER BY PartitionTime DESC, InvocationID, ResultID
	LIMIT @limit
`))

func generateReadArtifactsQuery(opts ReadArtifactsOpts) (string, error) {
	var b bytes.Buffer
	err := readArtifactsTmpl.ExecuteTemplate(&b, "", map[string]any{
		"pagination":      opts.PageToken != "",
		"InvocationLevel": opts.IsInvocationLevel,
	})
	if err != nil {
		return "", errors.Annotate(err, "execute template").Err()
	}
	return b.String(), nil
}

func parseReadArtifactsPageToken(pageToken string) (afterPartitionTimeUnix int, afterInvocationID, afterResultID string, err error) {
	tokens, err := pagination.ParseToken(pageToken)
	if err != nil {
		return 0, "", "", err
	}

	if len(tokens) != 3 {
		return 0, "", "", pagination.InvalidToken(errors.Reason("expected 3 components, got %d", len(tokens)).Err())
	}

	afterPartitionTimeUnix, err = strconv.Atoi(tokens[0])
	if err != nil {
		return 0, "", "", pagination.InvalidToken(errors.Reason("expect the first page_token component to be an integer").Err())
	}
	return afterPartitionTimeUnix, tokens[1], tokens[2], nil
}

type ReadArtifactsOpts struct {
	Project      string
	SearchString *pb.ArtifactContentMatcher
	// If true, query invocation level artifact. Otherwise, query test result level artifacts.
	IsInvocationLevel bool
	// If IsInvocationLevel is true, this field is ignored.
	TestID string
	// If IsInvocationLevel is true, this field is the hash of invocaiton variant union (union of all variants of test results directly included by the invocation).
	// Otherwise, this is the test variant hash.
	VariantHash string
	ArtifactID  string
	StartTime   time.Time
	EndTime     time.Time
	SubRealms   []string
	Limit       int
	PageToken   string
}

// ReadArtifacts reads either test result artifacts or invocation level artifacts,
// depending on opts.IsInvocationLevel.
func (c *Client) ReadArtifacts(ctx context.Context, opts ReadArtifactsOpts) (rows []*MatchingArtifact, nextPageToken string, err error) {
	var afterPartitionTimeUnix int
	var afterInvocationID, afterResultID string
	if opts.PageToken != "" {
		afterPartitionTimeUnix, afterInvocationID, afterResultID, err = parseReadArtifactsPageToken(opts.PageToken)
		if err != nil {
			return nil, "", err
		}
	}
	query, err := generateReadArtifactsQuery(opts)
	if err != nil {
		return nil, "", err
	}
	q := c.client.Query(query)
	q.DefaultDatasetID = "internal"
	q.Parameters = []bigquery.QueryParameter{
		{Name: "project", Value: opts.Project},
		{Name: "testID", Value: opts.TestID},
		{Name: "artifactID", Value: opts.ArtifactID},
		{Name: "variantHash", Value: opts.VariantHash},
		{Name: "searchRegex", Value: newMatchWithContextRegexBuilder(opts.SearchString).withCaptureMatch(true).build()},
		{Name: "searchContextBeforeRegex", Value: newMatchWithContextRegexBuilder(opts.SearchString).withCaptureContextBefore(true).build()},
		{Name: "searchContextAfterRegex", Value: newMatchWithContextRegexBuilder(opts.SearchString).withCaptureContextAfter(true).build()},
		{Name: "maxMatchSize", Value: maxMatchSize},
		{Name: "startTime", Value: opts.StartTime},
		{Name: "endTime", Value: opts.EndTime},
		{Name: "subRealms", Value: opts.SubRealms},
		{Name: "limit", Value: opts.Limit},
		{Name: "afterPartitionTimeUnix", Value: afterPartitionTimeUnix},
		{Name: "afterInvocationID", Value: afterInvocationID},
		{Name: "afterResultID", Value: afterResultID},
	}

	it, err := q.Read(ctx)
	if err != nil {
		return nil, "", errors.Annotate(err, "running query").Err()
	}
	results := []*MatchingArtifact{}
	for {
		row := &MatchingArtifact{}
		err := it.Next(row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, "", errors.Annotate(err, "obtain next matching artifact row").Err()
		}
		results = append(results, row)
	}
	if opts.Limit != 0 && len(results) == int(opts.Limit) {
		last := results[len(results)-1]
		// Page token are constructed in the same way for both invocation and test result level artifact.
		// For invocation artifact, result id is already empty string, the rest of the token element are enough to ensure absolute ordering.
		nextPageToken = pagination.Token(fmt.Sprint(last.PartitionTime.Unix()), last.InvocationID, last.ResultID)
	}
	return results, nextPageToken, nil
}

type matchWithContextRegexBuilder struct {
	matcher              *pb.ArtifactContentMatcher
	captureMatch         bool
	captureContextBefore bool
	captureContextAfter  bool
}

// NewMatchWithContextRegexBuilder creates a new matchWithContextRegexBuilder.
func newMatchWithContextRegexBuilder(matcher *pb.ArtifactContentMatcher) *matchWithContextRegexBuilder {
	return &matchWithContextRegexBuilder{
		matcher: matcher,
	}
}

// withCaptureMatch sets the captureMatch flag.
func (b *matchWithContextRegexBuilder) withCaptureMatch(captureMatch bool) *matchWithContextRegexBuilder {
	b.captureMatch = captureMatch
	return b
}

// withCaptureContextBefore sets the captureContextBefore flag.
func (b *matchWithContextRegexBuilder) withCaptureContextBefore(captureContextBefore bool) *matchWithContextRegexBuilder {
	b.captureContextBefore = captureContextBefore
	return b
}

// withCaptureContextAfter sets the captureContextAfter flag.
func (b *matchWithContextRegexBuilder) withCaptureContextAfter(captureContextAfter bool) *matchWithContextRegexBuilder {
	b.captureContextAfter = captureContextAfter
	return b
}

func (b *matchWithContextRegexBuilder) build() string {
	optionalLineBreakRegex := "(?:\\r\\n|\\r|\\n)?"
	searchRegex := regexPattern(b.matcher)

	// matchWithOptionalLineBreakGreedy and matchWithOptionalLineBreakLazy match
	// a sequence of text that may or may not include a line break.
	// The main difference between these two regex patterns lies in the use of
	// greedy (*) versus non-greedy (*?) quantifiers on the non-line-break characters.
	matchWithOptionalLineBreakGreedy := fmt.Sprintf("[^\\r\\n]*%s[^\\r\\n]*", optionalLineBreakRegex)
	matchWithOptionalLineBreakLazy := fmt.Sprintf("[^\\r\\n]*?%s[^\\r\\n]*?", optionalLineBreakRegex)

	matchBefore := matchWithOptionalLineBreakLazy
	matchAfter := matchWithOptionalLineBreakGreedy
	if b.captureContextBefore {
		matchBefore = fmt.Sprintf("(%s)", matchWithOptionalLineBreakLazy)
	}
	if b.captureContextAfter {
		matchAfter = fmt.Sprintf("(%s)", matchWithOptionalLineBreakGreedy)
	}
	if b.captureMatch {
		searchRegex = fmt.Sprintf("(%s)", searchRegex)
	}
	return optionalLineBreakRegex + matchBefore + searchRegex + matchAfter + optionalLineBreakRegex
}

func regexPattern(matcher *pb.ArtifactContentMatcher) string {
	caseInsensitiveFlag := "(?i)"
	if matcher == nil {
		// This should never happen.
		panic("match should not be nil")
	}
	switch m := matcher.Matcher.(type) {
	case *pb.ArtifactContentMatcher_ExactContain:
		return caseInsensitiveFlag + regexp.QuoteMeta(m.ExactContain)
	case *pb.ArtifactContentMatcher_RegexContain:
		return m.RegexContain
	default:
		panic("No matching operations")
	}
}

func isPrefixMatch(m *pb.IDMatcher) bool {
	if m == nil {
		return false
	}
	switch m.Matcher.(type) {
	case *pb.IDMatcher_HasPrefix:
		return true
	default:
		return false
	}
}

func isExactMatch(m *pb.IDMatcher) bool {
	if m == nil {
		return false
	}
	switch m.Matcher.(type) {
	case *pb.IDMatcher_ExactEqual:
		return true
	default:
		return false
	}
}

func idMatchString(m *pb.IDMatcher) string {
	if m == nil {
		return ""
	}
	switch m.Matcher.(type) {
	case *pb.IDMatcher_HasPrefix:
		return m.GetHasPrefix()
	case *pb.IDMatcher_ExactEqual:
		return m.GetExactEqual()
	default:
		return ""
	}
}
