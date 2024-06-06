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
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/ingestion/resultdb"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// tryClaimInvocations tries to claim use of the given invocations'
// data for exclusive ingestion under the given root invocation,
// in the scope of the given LUCI Project.
//
// This method returns a map which will contain 'true' for each
// invocation it successfully claimed.

// Changepoint analysis only supports ingestion of an invocation's data
// once, as ingesting it multiple times violates statistical independence
// assumptions made by the changepoint model. To prevent duplicate
// ingestion of the same invocation, a system of 'claiming' invocations
// exists. An invocation can be claimed by only one root invocation.
//
// Note: This function must NOT be called with a transactional context.
func tryClaimInvocations(ctx context.Context, project string, rootInvocationID string, invIDs []string) (map[string]bool, error) {
	var claimMap map[string]bool

	f := func(ctx context.Context) error {
		invMap, err := readInvocations(ctx, project, invIDs)
		if err != nil {
			return errors.Annotate(err, "read invocations").Err()
		}

		claimMap = make(map[string]bool)
		for _, invID := range invIDs {
			ingestedInvID, ok := invMap[invID]
			if !ok {
				// Invocation is currently unclaimed. Claim it.
				claimMap[invID] = true
				span.BufferWrite(ctx, ClaimInvocationMutation(project, invID, rootInvocationID))
			} else if ingestedInvID == rootInvocationID {
				// Invocation previously claimed by us.
				claimMap[invID] = true
			}
		}
		return nil
	}

	_, err := span.ReadWriteTransaction(ctx, f)
	if err != nil {
		return nil, err
	}
	return claimMap, nil
}

// tryClaimInvocation tries to claim use of the given invocation's
// data for exclusive ingestion under the given root invocation,
// in the scope of the given LUCI Project.
// If claimed, this method returns true. Otherwise it returns false,
// indicating an attempt to ingest duplicate data.
//
// Changepoint analysis only supports ingestion of an invocation's data
// once, as ingesting it multiple times violates statistical independence
// assumptions made by the changepoint model. To prevent duplicate
// ingestion of the same invocation, a system of 'claiming' invocations
// exists. An invocation can be claimed by only one root invocation.
//
// Note: This function must NOT be called with a transactional context.
func tryClaimInvocation(ctx context.Context, rootProject string, invocationID string, rootInvocationID string) (bool, error) {
	result, err := tryClaimInvocations(ctx, rootProject, rootInvocationID, []string{invocationID})
	if err != nil {
		return false, err
	}
	return result[invocationID], nil
}

// shouldClaimInLowLatencyPipeline returns whether an invocation with given
// sources should be claimed in the low-latency ingestion pipeline.
func shouldClaimInLowLatencyPipeline(sources *pb.Sources) bool {
	return sources != nil && len(sources.Changelists) == 0
}

// shouldClaimInHighLatencyPipeline returns whether an invocation with given
// sources should be claimed in the high-latency ingestion pipeline.
func shouldClaimInHighLatencyPipeline(sources *pb.Sources) bool {
	// Invocations without changelists are ingestible by low-latency
	// pipeline as we do not need to know the CV run result to determine
	// if they can be ingested.
	// Other invocations with sources should be ingested in the
	// high-latency pipeline.
	return sources != nil && len(sources.Changelists) > 0
}

// invocationIDsToClaimHighLatency gets all runs' invocation IDs for given test
// variants, where it makes sense to claim the invocation in the high-latency
// ingestion pipeline.
//
// This is used for a batch query to spanner to check for duplicate runs.
func invocationIDsToClaimHighLatency(tvs []*rdbpb.TestVariant, sourcesMap map[string]*pb.Sources) ([]string, error) {
	m := map[string]bool{}
	for _, tv := range tvs {
		srcs := sourcesMap[tv.SourcesId]
		if !shouldClaimInHighLatencyPipeline(srcs) {
			// Do not try to claim here.
			continue
		}

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
