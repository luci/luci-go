// Copyright 2025 The LUCI Authors.
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

package testrealms

import (
	"bytes"
	"context"
	"text/template"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/pagination"
	spanutil "go.chromium.org/luci/analysis/internal/span"
)

// QueryTestsOptions specifies options for QueryTests().
type QueryTestsOptions struct {
	// Set, to perform case sensitive matching.
	CaseSensitive bool
	// The fully qualified realms to query.
	Realms []string
	// The maximum page size to return.
	PageSize int
	// The starting page token, if any.
	PageToken string
}

// QueryTests finds all the test IDs with the specified testIDSubstring.
func (c *Client) QueryTests(ctx context.Context, project, testIDSubstring string, opts QueryTestsOptions) (testIDs []string, nextPageToken string, err error) {
	paginationTestID := ""
	if opts.PageToken != "" {
		paginationTestID, err = parseQueryTestsPageToken(opts.PageToken)
		if err != nil {
			return nil, "", err
		}
	}

	input := map[string]any{
		"hasLimit":      opts.PageSize > 0,
		"caseSensitive": opts.CaseSensitive,
	}
	stmt, err := generateStatement(QueryTestsQueryTmpl, QueryTestsQueryTmpl.Name(), input)
	if err != nil {
		return nil, "", err
	}
	q := c.client.Query(stmt)
	q.DefaultDatasetID = bqutil.InternalDatasetID

	q.Parameters = []bigquery.QueryParameter{
		{Name: "project", Value: project},
		{Name: "searchPattern", Value: "%" + spanutil.QuoteLike(testIDSubstring) + "%"},
		{Name: "realms", Value: opts.Realms},
		{Name: "limit", Value: opts.PageSize},
		{Name: "paginationTestId", Value: paginationTestID},
	}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, "", errors.Fmt("querying tests: %w", err)
	}

	type resultRow struct {
		TestID bigquery.NullString
	}
	for {
		row := &resultRow{}
		err := it.Next(row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, "", errors.Fmt("obtain next test ID row: %w", err)
		}
		testIDs = append(testIDs, row.TestID.StringVal)
	}

	if opts.PageSize != 0 && len(testIDs) == opts.PageSize {
		lastTestID := testIDs[len(testIDs)-1]
		nextPageToken = pagination.Token(lastTestID)
	}
	return testIDs, nextPageToken, nil
}

// parseQueryTestsPageToken parses the positions from the page token.
func parseQueryTestsPageToken(pageToken string) (afterTestId string, err error) {
	tokens, err := pagination.ParseToken(pageToken)
	if err != nil {
		return "", err
	}

	if len(tokens) != 1 {
		return "", pagination.InvalidToken(errors.Fmt("expected 1 components, got %d", len(tokens)))
	}

	return tokens[0], nil
}

// generateStatement generates a BigQuery statement from a text template.
func generateStatement(tmpl *template.Template, name string, input any) (string, error) {
	sql := &bytes.Buffer{}
	err := tmpl.ExecuteTemplate(sql, name, input)
	if err != nil {
		return "", errors.Fmt("failed to generate statement: %s: %w", name, err)
	}
	return sql.String(), nil
}

var QueryTestsQueryTmpl = template.Must(template.New("QueryTestsQuery").Parse(`
	SELECT test_id as TestID
	FROM test_realms
	WHERE
		project = @project
			AND test_id > @paginationTestId
			AND realm IN UNNEST(@realms)
			{{if .caseSensitive}}
				AND (
					test_id LIKE @searchPattern
					OR test_name LIKE @searchPattern
				)
			{{else}}
				AND (
					test_id_lower LIKE LOWER(@searchPattern)
					OR test_name_lower LIKE LOWER(@searchPattern)
				)
			{{end}}
	GROUP BY test_id
	ORDER BY test_id ASC
	{{if .hasLimit}}
		LIMIT @limit
	{{end}}
`))
