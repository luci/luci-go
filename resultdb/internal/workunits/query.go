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

package workunits

import (
	"context"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// MaxAncestorTraversalHeight is the maximum number of ancestors to traverse
// to prevent infinite loops in case of data corruption (cycles).
const MaxAncestorTraversalHeight = 50

// Query specifies work units to fetch.
type Query struct {
	RootInvocationID rootinvocations.ID
	Predicate        *pb.WorkUnitPredicate
	Mask             ReadMask
}

// Query returns work units matching the query.
func (q *Query) Query(ctx context.Context) (wus []*WorkUnitRow, err error) {
	if q.Predicate.GetAncestorsOf() != "" {
		return q.fetchAncestors(ctx)
	}
	return nil, errors.New("predicate not implemented")
}

func (q *Query) fetchAncestors(ctx context.Context) ([]*WorkUnitRow, error) {
	startID, err := ParseName(q.Predicate.AncestorsOf)
	if err != nil {
		// This should have been caught in validation, but check just in case.
		return nil, appstatus.BadRequest(errors.Fmt("predicate: ancestors_of: %w", err))
	}

	// Note: The check ensuring startID.RootInvocationID matches q.RootInvocationID
	// has been moved to the service-level request validation.

	var ancestors []*WorkUnitRow
	currID := startID

	// Loop to traverse upwards.
	// i=0 reads the start node (to find its parent).
	// i>0 reads the actual ancestors.
	// Limit to MaxAncestorTraversalHeight ancestors + 1 start node.
	for i := 0; i < MaxAncestorTraversalHeight+1; i++ {
		// Optimization: We don't return the start node (i==0), so don't
		// fetch its extended properties even if requested for the output.
		mask := q.Mask
		if i == 0 {
			mask = ExcludeExtendedProperties
		}

		row, err := Read(ctx, currID, mask)
		if err != nil {
			if appstatus.Code(err) == codes.NotFound {
				// If the start node itself is missing, return the NotFound error
				// so the client knows their request was invalid.
				if i == 0 {
					return nil, err
				}
				// If an intermediate ancestor is missing (broken chain), just return
				// what we found so far. This is an unlikely scenario.
				return ancestors, nil
			}
			return nil, err
		}

		// If this is an ancestor (not the start node), add to results.
		if i > 0 {
			ancestors = append(ancestors, row)
		}

		// Check if we reached the root (no parent).
		if !row.ParentWorkUnitID.Valid {
			break
		}

		// Prepare for next iteration.
		currID = ID{
			RootInvocationID: row.ID.RootInvocationID,
			WorkUnitID:       row.ParentWorkUnitID.StringVal,
		}
	}

	return ancestors, nil
}
