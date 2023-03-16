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

package changepoints

import (
	"context"

	"go.chromium.org/luci/analysis/internal/ingestion/control"
	"go.chromium.org/luci/analysis/internal/ingestion/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/common/errors"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

// duplicateMap contructs an duplicate map for test variants.
// It returns a map of (invocationID, bool). If an invocation ID is found in
// the map keys, then it is from a duplicate run. If not, then the run is not
// duplicate.
// Note: This function should be called with a transactional context.
func duplicateMap(ctx context.Context, tvs []*rdbpb.TestVariant, payload *taskspb.IngestTestResults) (map[string]bool, error) {
	invIDs, err := invocationIDsFromTestVariants(tvs)
	if err != nil {
		return nil, errors.Annotate(err, "invocation ids from test variant").Err()
	}
	invMap, err := readInvocations(ctx, payload.Build.Project, invIDs)
	if err != nil {
		return nil, errors.Annotate(err, "read invocations").Err()
	}
	result := map[string]bool{}
	buildInvID := control.BuildInvocationName(payload.Build.Id)
	for invID, ingestedInvID := range invMap {
		// If the ingested invocation ID stored in Spanner is different from the
		// current invocation ID, it means this is a duplicate run.
		if ingestedInvID != buildInvID {
			result[invID] = true
		}
	}
	return result, nil
}

// invocationIDsFromTestVariants gets all runs' invocation IDs for given test
// variants.
// This is used for a batch query to spanner to check for duplicate runs.
func invocationIDsFromTestVariants(tvs []*rdbpb.TestVariant) ([]string, error) {
	m := map[string]bool{}
	for _, tv := range tvs {
		for _, tr := range tv.Results {
			invocationName, err := resultdb.InvocationFromTestResultName(tr.Result.Name)
			if err != nil {
				return nil, err
			}
			m[invocationName] = true
		}
	}

	// Return the keys from m.
	result := make([]string, len(m))
	i := 0
	for invocationName := range m {
		result[i] = invocationName
		i++
	}
	return result, nil
}
