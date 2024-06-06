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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	spanutil "go.chromium.org/luci/analysis/internal/span"
)

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

// ClaimInvocationMutation creates a mutation to claim an invocation for the
// given root invocation.
func ClaimInvocationMutation(project string, invocationID string, rootInvocationID string) *spanner.Mutation {
	if project == "" {
		panic("project must not be empty")
	}
	if invocationID == "" {
		panic("invocationID must not be empty")
	}
	if rootInvocationID == "" {
		panic("rootInvocationID must not be empty")
	}
	values := map[string]any{
		"Project":              project,
		"InvocationID":         invocationID,
		"IngestedInvocationID": rootInvocationID,
		"CreationTime":         spanner.CommitTimestamp,
	}
	return spanutil.InsertMap("Invocations", values)
}
