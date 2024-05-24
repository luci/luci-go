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

	"go.chromium.org/luci/common/errors"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/ingestion/resultdb"
)

// readDuplicateInvocations contructs an duplicate map for test variants.
// It returns a map of (invocationID, bool). If an invocation ID is found in
// the map keys, then it is from a duplicate run. If not, then the run is not
// duplicate.
// It also returns a slice of invocation IDs that are not in Invocations
// table in Spanner. This is used to insert new Invocations row to Spanner.
// Note: This function should be called with a transactional context.
func readDuplicateInvocations(ctx context.Context, tvs []*rdbpb.TestVariant, project, invocationID string) (map[string]bool, []string, error) {
	invIDs, err := invocationIDsFromTestVariants(tvs)
	if err != nil {
		return nil, nil, errors.Annotate(err, "invocation ids from test variant").Err()
	}
	invMap, err := readInvocations(ctx, project, invIDs)
	if err != nil {
		return nil, nil, errors.Annotate(err, "read invocations").Err()
	}
	dupMap := map[string]bool{}
	for invID, ingestedInvID := range invMap {
		// If the ingested invocation ID stored in Spanner is different from the
		// current invocation ID, it means this is a duplicate run.
		if ingestedInvID != invocationID {
			dupMap[invID] = true
		}
	}

	newInvIDs := []string{}
	for _, invID := range invIDs {
		if _, ok := invMap[invID]; !ok {
			newInvIDs = append(newInvIDs, invID)
		}
	}
	return dupMap, newInvIDs, nil
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
