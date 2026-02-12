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

package lowlatency

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/pbutil"
	"go.chromium.org/luci/server/span"

	spanutil "go.chromium.org/luci/analysis/internal/span"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// ReadPassingTestResultsBySourceOptions specifies options for ReadPassingTestResultsBySource.
type ReadPassingTestResultsBySourceOptions struct {
	// The LUCI Project.
	Project string
	// The sub-realms the user is authorized to query.
	SubRealms []string
	// The test ID.
	TestID string
	// The variant hash.
	VariantHash string
	// The hash of the source ref (e.g. git branch or Android build branch).
	SourceRefHash []byte
	// The maximum source position to search for passes at or before.
	MaxSourcePosition int64
	// The maximum number of test results to return.
	Limit int
}

// ReadPassingResultsBySource reads recent test result names for a given
// test variant, ordered by proximity to a given source position.
// This function queries the TestResultsBySourcePosition table.
// Must be called in a Spanner transactional context.
func ReadPassingResultsBySource(ctx context.Context, opts ReadPassingTestResultsBySourceOptions) ([]string, error) {
	stmt := spanner.NewStatement(`
		SELECT ANY_VALUE(t.RootInvocationId) as RootInvocationId, t.InvocationId, t.ResultId
		FROM TestResultsBySourcePosition AS t
		WHERE t.Project = @project
		  AND t.SubRealm IN UNNEST(@subRealms)
		  AND t.TestId = @testId
		  AND t.VariantHash = @variantHash
		  AND t.SourceRefHash = @sourceRefHash
		  AND t.SourcePosition <= @maxSourcePosition
			AND (
				t.StatusV2 = @passedStatusV2
				OR (t.StatusV2 IS NULL AND t.Status = @passStatus)
			)
		GROUP BY IF(STARTS_WITH(t.RootInvocationId, "root:"), t.RootInvocationId, ""), t.InvocationId, t.ResultId
		ORDER BY MAX(t.SourcePosition) DESC
		LIMIT @limit
	`)
	stmt.Params = map[string]interface{}{
		"project":           opts.Project,
		"subRealms":         opts.SubRealms,
		"testId":            opts.TestID,
		"variantHash":       opts.VariantHash,
		"sourceRefHash":     opts.SourceRefHash,
		"maxSourcePosition": opts.MaxSourcePosition,
		"passedStatusV2":    int64(pb.TestResult_PASSED),
		"passStatus":        int64(pb.TestResultStatus_PASS),
		"limit":             opts.Limit,
	}

	var results []string
	var b spanutil.Buffer
	err := span.Query(ctx, stmt).Do(func(r *spanner.Row) error {
		var rootInvocationRowID string
		var workUnitID WorkUnitID
		var resultID string
		if err := b.FromSpanner(r, &rootInvocationRowID, &workUnitID, &resultID); err != nil {
			return fmt.Errorf("read passing result: %w", err)
		}
		if workUnitID.IsLegacy {
			// The work unit is actually a legacy invocation.
			results = append(results, pbutil.LegacyTestResultName(workUnitID.Value, opts.TestID, resultID))
		} else {
			rootInvocationID := RootInvocationIDFromRowID(rootInvocationRowID)
			results = append(results, pbutil.TestResultName(rootInvocationID.Value, workUnitID.Value, opts.TestID, resultID))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("querying recent passes by source: %w", err)
	}
	return results, nil
}
