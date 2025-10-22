// Copyright 2023 The LUCI Authors.
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

// Package testmetadata implements methods to query from TestMetadata spanner table.
package testmetadata

import (
	"context"
	"encoding/hex"
	"text/template"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Query a set of TestMetadataRow for a list of tests
// which have the same same subRealm and sourceRef in a LUCI project.
type Query struct {
	Project   string
	SubRealms []string
	Predicate *pb.TestMetadataPredicate

	PageSize  int
	PageToken string
}

// Fetch distinct matching TestMetadataDetails from on the TestMetadata table.
// The returned TestMetadataDetails are ordered by testID, refHash ascendingly.
// If the test exists in multiples subRealms, lexicographically minimum subRealm is returned.
func (q *Query) Fetch(ctx context.Context) (tmr []*pb.TestMetadataDetail, nextPageToken string, err error) {
	if q.PageSize <= 0 {
		panic("can't use fetch with non-positive page size")
	}

	err = q.run(ctx, func(row *pb.TestMetadataDetail) error {
		tmr = append(tmr, row)
		return nil
	})
	if err != nil {
		return tmr, nextPageToken, err
	}
	if len(tmr) == q.PageSize {
		lastRow := tmr[q.PageSize-1]
		nextPageToken = pagination.Token(lastRow.TestId, lastRow.RefHash)
	}
	return tmr, nextPageToken, err
}

func parseQueryPageToken(pageToken string) (afterTestID, afterRefHash string, err error) {
	tokens, err := pagination.ParseToken(pageToken)
	if err != nil {
		return "", "", err
	}
	if len(tokens) != 2 {
		return "", "", pagination.InvalidToken(errors.Fmt("expected 2 components, got %d", len(tokens)))
	}
	return tokens[0], tokens[1], nil
}

// Run the query and call f for each test metadata detail returned from the query.
func (q *Query) run(ctx context.Context, f func(tmd *pb.TestMetadataDetail) error) error {
	st, err := spanutil.GenerateStatement(queryTmpl, map[string]any{
		"pagination":           len(q.PageToken) > 0,
		"hasLimit":             q.PageSize > 0,
		"filterTestID":         len(q.Predicate.GetTestIds()) != 0,
		"filterPreviousTestID": len(q.Predicate.GetPreviousTestIds()) != 0,
	})
	if err != nil {
		return err
	}
	params := map[string]any{
		"project":         q.Project,
		"testIDs":         q.Predicate.GetTestIds(),
		"previousTestIDs": q.Predicate.GetPreviousTestIds(),
		"subRealms":       q.SubRealms,
		"limit":           q.PageSize,
	}

	if q.PageToken != "" {
		afterTestID, afterRefHash, err := parseQueryPageToken(q.PageToken)
		if err != nil {
			return err
		}
		params["afterTestID"] = afterTestID
		params["afterRefHash"], err = hex.DecodeString(afterRefHash)
		if err != nil {
			return pagination.InvalidToken(err)
		}
	}
	st.Params = spanutil.ToSpannerMap(params)

	var b spanutil.Buffer
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		tmd := &pb.TestMetadataDetail{}
		var compressedTestMetadata spanutil.Compressed
		var compressedSourceRef spanutil.Compressed
		var refHash []byte
		if err := b.FromSpanner(row,
			&tmd.Project,
			&tmd.TestId,
			&refHash,
			&compressedSourceRef,
			&compressedTestMetadata); err != nil {
			return err
		}
		tmd.Name = pbutil.TestMetadataName(tmd.Project, tmd.TestId, refHash)
		tmd.RefHash = hex.EncodeToString(refHash)
		tmd.TestMetadata = &pb.TestMetadata{}
		if err := proto.Unmarshal(compressedTestMetadata, tmd.TestMetadata); err != nil {
			return err
		}
		tmd.SourceRef = &pb.SourceRef{}
		if err := proto.Unmarshal(compressedSourceRef, tmd.SourceRef); err != nil {
			return err
		}
		return f(tmd)
	})
}

var queryTmpl = template.Must(template.New("").Parse(`
		SELECT
			tm.Project,
			tm.TestId,
			tm.RefHash,
			ANY_VALUE(tm.SourceRef) AS SourceRef,
			ANY_VALUE(tm.TestMetadata HAVING MIN tm.SubRealm) AS TestMetadata
		FROM TestMetadata
			{{if .filterPreviousTestID}}
			@{FORCE_INDEX=TestMetadataByPreviousTestId, spanner_emulator.disable_query_null_filtered_index_check=true}
			{{end}}
			tm
		WHERE tm.Project = @project
			{{if .filterTestID}}
				AND tm.TestId IN UNNEST(@testIDs)
			{{end}}
			{{if .filterPreviousTestID}}
				-- This is an important hint for the query planner to
				-- know it can answer the query with the null-filtered index.
				AND tm.PreviousTestId IS NOT NULL
				AND tm.PreviousTestId IN UNNEST(@previousTestIDs)
			{{end}}
			AND tm.SubRealm IN UNNEST(@subRealms)
			{{if .pagination}}
				AND ((tm.TestId > @afterTestID) OR
					(tm.TestId = @afterTestID AND tm.RefHash > @afterRefHash))
			{{end}}
		GROUP BY tm.Project, tm.TestId, tm.RefHash
		ORDER BY tm.TestId, tm.RefHash
		{{if .hasLimit}}LIMIT @limit{{end}}
`))
