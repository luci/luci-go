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

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	spanutil "go.chromium.org/luci/analysis/internal/span"
)

// hasCheckPoint returns true if a checkpoint exists in the
// TestVariantBranchCheckpoint table.
// This function need to be call in the context of a transaction.
func hasCheckPoint(ctx context.Context, cp CheckPoint) (bool, error) {
	_, err := span.ReadRow(ctx, "TestVariantBranchCheckpoint", spanner.Key{cp.InvocationID, cp.StartingTestID, cp.StartingVariantHash}, []string{"InvocationId"})
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			return false, nil
		}
		return false, errors.Annotate(err, "read TestVariantBranchCheckpoint").Err()
	}
	// No error, row exists.
	return true, nil
}

// ToMutation return a spanner Mutation to insert a CheckPoint into
func (cp CheckPoint) ToMutation() *spanner.Mutation {
	values := map[string]any{
		"InvocationId":        cp.InvocationID,
		"StartingTestId":      cp.StartingTestID,
		"StartingVariantHash": cp.StartingVariantHash,
		"InsertionTime":       spanner.CommitTimestamp,
	}
	return spanutil.InsertMap("TestVariantBranchCheckpoint", values)
}

// readInvocations reads the Invocations spanner table for invocation IDs.
// It returns a mapping of (InvocationID, IngestedInvocationID) for the found
// invocations.
// This function assumes that it is running inside a transaction.
func readInvocations(ctx context.Context, project string, invocationIDs []string) (map[string]string, error) {
	result := map[string]string{}
	// Create the keyset.
	keys := make([]spanner.Key, len(invocationIDs))
	for i := 0; i < len(keys); i++ {
		keys[i] = spanner.Key{project, invocationIDs[i]}
	}
	keyset := spanner.KeySetFromKeys(keys...)
	cols := []string{"InvocationID", "IngestedInvocationID"}

	err := span.Read(ctx, "Invocations", keyset, cols).Do(
		func(row *spanner.Row) error {
			var b spanutil.Buffer
			var invID string
			var ingestedInvID string
			if err := b.FromSpanner(row, &invID, &ingestedInvID); err != nil {
				return errors.Annotate(err, "read values from spanner").Err()
			}
			result[invID] = ingestedInvID
			return nil
		},
	)

	if err != nil {
		return nil, err
	}
	return result, nil
}

// invocationsToMutations returns the Invocations mutations for invocationIDs.
func invocationsToMutations(ctx context.Context, project string, invocationIDs []string, ingestedInvocationID string) []*spanner.Mutation {
	mutations := make([]*spanner.Mutation, len(invocationIDs))
	for i, invocationID := range invocationIDs {
		values := map[string]any{
			"Project":              project,
			"InvocationID":         invocationID,
			"IngestedInvocationID": ingestedInvocationID,
			"CreationTime":         spanner.CommitTimestamp,
		}
		mutations[i] = spanutil.InsertMap("Invocations", values)
	}
	return mutations
}
