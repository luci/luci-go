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

package testresults

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"

	pb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/resultdb/pbutil"
	"go.chromium.org/luci/server/span"
)

// ReadPassingRootInvocationsBySourceOptions specifies options for ReadPassingRootInvocationsBySource.
type ReadPassingRootInvocationsBySourceOptions struct {
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
	// The maximum number of root invocation IDs to return.
	Limit int
}

// ReadPassingResultsBySource reads recent test result names for a given
// test variant, ordered by proximity to a given source position.
// This function queries the TestResultsBySourcePosition table.
// Must be called in a Spanner transactional context.
func ReadPassingResultsBySource(ctx context.Context, opts ReadPassingRootInvocationsBySourceOptions) ([]string, error) {
	stmt := spanner.NewStatement(`
		SELECT t.InvocationId, t.ResultId
		FROM TestResultsBySourcePosition AS t
		WHERE t.Project = @project
		  AND t.SubRealm IN UNNEST(@subRealms)
		  AND t.TestId = @testId
		  AND t.VariantHash = @variantHash
		  AND t.SourceRefHash = @sourceRefHash
		  AND t.SourcePosition <= @maxSourcePosition
		  AND t.Status = @passStatus
		GROUP BY t.InvocationId, t.ResultId
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
		"passStatus":        int64(pb.TestResultStatus_PASS),
		"limit":             opts.Limit,
	}

	var results []string
	err := span.Query(ctx, stmt).Do(func(r *spanner.Row) error {
		var invocationID, resultID string
		if err := r.Columns(&invocationID, &resultID); err != nil {
			return fmt.Errorf("read root invocation id column: %w", err)
		}

		results = append(results, pbutil.LegacyTestResultName(invocationID, opts.TestID, resultID))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("querying recent passes by source: %w", err)
	}
	return results, nil
}
