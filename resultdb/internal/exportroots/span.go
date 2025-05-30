// Copyright 2024 The LUCI Authors.
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

// Package exportroots contains methods for reading and writing
// export root records in Spanner. Export roots are stored for each
// invocation and are used to facilitate low-latency exports of invocations
// inside each root.
package exportroots

import (
	"context"
	"sort"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type ExportRoot struct {
	// The identity of the invocation.
	Invocation invocations.ID
	// The identity of the invocation that is the export root.
	RootInvocation invocations.ID
	// Whether inherited sources for this invocation have been resolved.
	// The value is then stored in InheritedSources.
	IsInheritedSourcesSet bool
	// The sources this invocation is eligible to inherit for its inclusion
	// (directly or indirectly) from RootInvocationId.
	// This may to a concrete luci.resultdb.v1.Sources value (if concrete
	// sources are eligible to be inherited) or to a nil value (if empty
	// sources are eligible to be inherited).
	// To be able to distinguish inheriting empty/nil sources and the inherited
	// sources not being resolved yet, see HasInheritedSourcesResolved.
	InheritedSources *pb.Sources
	// Whether an `invocation-ready-for-export` pub/sub notification was
	// sent for this row.
	IsNotified bool
	// The timestamp a `invocation-ready-for-export` pub/sub notification was
	// sent for this row. Used for debugging and to avoid triggering
	// pub/sub notifications that were already sent.
	NotifiedTime time.Time
}

type RootRestriction struct {
	// Whether to restrict the root invocations returned.
	UseRestriction bool
	// The root invocations to return. Used if UseRestriction is set.
	InvocationIDs invocations.IDSet
}

// ReadForInvocation reads export roots for the given invocation.
// rootRestriction restricts which export roots are returned.
func ReadForInvocation(ctx context.Context, invocationID invocations.ID, rootRestriction RootRestriction) ([]ExportRoot, error) {
	params := map[string]any{
		"InvocationId": invocationID,
	}
	whereClause := "InvocationId = @InvocationId"
	if rootRestriction.UseRestriction {
		whereClause += " AND RootInvocationId IN UNNEST(@RootRestrictionIds)"
		params["RootRestrictionIds"] = rootRestriction.InvocationIDs
	}

	results, err := readWhere(ctx, whereClause, params)
	if err != nil {
		return nil, errors.Fmt("read export roots by invocation: %w", err)
	}

	// Sort to ensure determinism.
	sortResults(results)
	return results, nil
}

// ReadForInvocations reads given export roots for the given invocations,
// limited to the export roots in the given root invocation IDs.
// Results are grouped by invocation ID and then root invocation ID.
func ReadForInvocations(ctx context.Context, invocationIDs invocations.IDSet, rootInvocationIDs invocations.IDSet) (map[invocations.ID]map[invocations.ID]ExportRoot, error) {
	params := map[string]any{
		"InvocationIds":     invocationIDs,
		"RootInvocationIds": rootInvocationIDs,
	}
	whereClause := "InvocationId IN UNNEST(@InvocationIds) AND RootInvocationId IN UNNEST(@RootInvocationIds)"

	results, err := readWhere(ctx, whereClause, params)
	if err != nil {
		return nil, errors.Fmt("read export roots for invocations: %w", err)
	}

	// Group results by invocation ID and root invocation ID.
	result := make(map[invocations.ID]map[invocations.ID]ExportRoot)
	for _, r := range results {
		if _, ok := result[r.Invocation]; !ok {
			result[r.Invocation] = make(map[invocations.ID]ExportRoot)
		}
		result[r.Invocation][r.RootInvocation] = r
	}

	return result, nil
}

// ReadAllForTesting reads all export roots. Useful for unit testing.
// Do not use in production, will not scale.
func ReadAllForTesting(ctx context.Context) ([]ExportRoot, error) {
	results, err := readWhere(ctx, "TRUE", nil)
	if err != nil {
		return nil, errors.Fmt("read all export roots: %w", err)
	}
	// Sort to ensure determinism.
	sortResults(results)
	return results, nil
}

// sortResults performs an in-place sort of export root records
// by InvocationID, then by RootInvocationID.
func sortResults(results []ExportRoot) {
	sort.Slice(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.Invocation != b.Invocation {
			return a.Invocation < b.Invocation
		}
		return a.RootInvocation < b.RootInvocation
	})
}

// readWhere reads export roots matching the given where clause,
// substituting params for any SQL parameters used in that clause.
func readWhere(ctx context.Context, whereClause string, params map[string]any) ([]ExportRoot, error) {
	stmt := spanner.NewStatement(`
		SELECT
			InvocationId,
			RootInvocationId,
			IsInheritedSourcesSet,
			InheritedSources,
			NotifiedTime
		FROM InvocationExportRoots r
		WHERE (` + whereClause + `)
	`)
	stmt.Params = spanutil.ToSpannerMap(params)

	it := span.Query(ctx, stmt)

	var rows []ExportRoot
	var b spanutil.Buffer
	err := it.Do(func(r *spanner.Row) error {
		var row ExportRoot
		var inheritedSourcesCmp spanutil.Compressed
		var notifiedTime spanner.NullTime

		err := b.FromSpanner(r, &row.Invocation, &row.RootInvocation, &row.IsInheritedSourcesSet, &inheritedSourcesCmp, &notifiedTime)
		if err != nil {
			return errors.Fmt("read export root row: %w", err)
		}

		if len(inheritedSourcesCmp) > 0 {
			row.InheritedSources = &pb.Sources{}
			if err := proto.Unmarshal(inheritedSourcesCmp, row.InheritedSources); err != nil {
				return errors.Fmt("unmarshal inherited sources: %w", err)
			}
		}
		if notifiedTime.Valid {
			row.NotifiedTime = notifiedTime.Time
			row.IsNotified = true
		}

		rows = append(rows, row)
		return nil
	})
	if err != nil {
		return nil, errors.Fmt("read export roots: %w", err)
	}
	return rows, nil
}

// Create returns a mutation to create the given ExportRoot row. The
// specified NotifiedTime is ignored and set to the commit timestamp if
// IsNotified is set.
func Create(root ExportRoot) *spanner.Mutation {
	// Add an extra layer of protection against invalid data getting into the database.
	if err := pbutil.ValidateInvocationID(string(root.Invocation)); err != nil {
		panic(errors.Fmt("invalid invocation ID: %w", err))
	}
	if err := pbutil.ValidateInvocationID(string(root.RootInvocation)); err != nil {
		panic(errors.Fmt("invalid root invocation ID: %w", err))
	}
	if root.InheritedSources != nil && !root.IsInheritedSourcesSet {
		// Note that the alternative combination InheritedSources == nil with
		// IsInheritedSourcesSet = true is valid, and means the invocation is
		// inheriting no sources.
		panic("IsInheritedSourcesSet must be set if InheritedSources is not nil")
	}
	return spanutil.InsertMap("InvocationExportRoots", map[string]any{
		"InvocationId":          root.Invocation,
		"RootInvocationId":      root.RootInvocation,
		"IsInheritedSourcesSet": root.IsInheritedSourcesSet,
		"InheritedSources":      spanutil.Compressed(pbutil.MustMarshal(root.InheritedSources)),
		"NotifiedTime":          spanner.NullTime{Valid: root.IsNotified, Time: spanner.CommitTimestamp},
	})
}

// SetInheritedSources creates a mutation to set the given export root's
// inherited sources.
//
// Before calling this method, the caller *must* confirm that the export root's
// inherited sources are not already set, in the same Read/Write transaction as
// this update.
func SetInheritedSources(root ExportRoot) *spanner.Mutation {
	if !root.IsInheritedSourcesSet {
		panic("IsInheritedSourcesSet should be set to reflect the caller's intent to set inherited sources")
	}
	return spanutil.UpdateMap("InvocationExportRoots", map[string]any{
		"InvocationId":          root.Invocation,
		"RootInvocationId":      root.RootInvocation,
		"IsInheritedSourcesSet": root.IsInheritedSourcesSet,
		"InheritedSources":      spanutil.Compressed(pbutil.MustMarshal(root.InheritedSources)),
	})
}

// SetNotified creates a mutation to set the given export root as notified.
//
// Before calling this method, the caller *must* confirm that the export root
// was NOT already notified, in the same Read/Write transaction as this update.
func SetNotified(root ExportRoot) *spanner.Mutation {
	if !root.IsNotified {
		panic("IsNotified should be set to reflect the caller's intent to set the export root as notified")
	}
	return spanutil.UpdateMap("InvocationExportRoots", map[string]any{
		"InvocationId":     root.Invocation,
		"RootInvocationId": root.RootInvocation,
		"NotifiedTime":     spanner.CommitTimestamp,
	})
}
