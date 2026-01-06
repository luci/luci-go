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
	"fmt"

	"cloud.google.com/go/spanner"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tracing"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ResponseLimitReachedErr is returned by the callback to indicate that the
// response size limit has been reached. The work unit passed to the callback
// returning this error should be included in the current page, and the next
// page will start after it.
var ResponseLimitReachedErr = errors.New("response limit reached")

// MaxAncestorTraversalHeight is the maximum number of ancestors to traverse
// to prevent infinite loops in case of data corruption (cycles).
const MaxAncestorTraversalHeight = 50

// Query specifies work units to fetch.
type Query struct {
	RootInvocationID rootinvocations.ID
	// Pagination is not supported if predicate.AncestorsOf is specified.
	Predicate *pb.WorkUnitPredicate
	Mask      ReadMask
	PageSize  int
	PageToken string

	// Internal state used to compute the pagination token.
	lastWorkUnitID string
}

// Query returns work units matching the query.
func (q *Query) Query(ctx context.Context, f func(*WorkUnitRow) error) (pageToken string, err error) {
	if q.Predicate.GetAncestorsOf() != "" {
		// Pagination is not supported for ancestors_of queries.
		return "", q.queryAncestors(ctx, f)
	}
	return q.queryAll(ctx, f)
}

func (q *Query) queryAncestors(ctx context.Context, f func(*WorkUnitRow) error) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/workunits.queryAncestors",
		attribute.String("root_invocation", string(q.RootInvocationID)),
	)
	defer func() { tracing.End(ts, err) }()

	if q.PageToken != "" {
		return pagination.InvalidToken(errors.New("page_token is not supported for ancestors_of queries"))
	}
	startID, err := ParseName(q.Predicate.AncestorsOf)
	if err != nil {
		// This should have been caught in validation, but check just in case.
		return appstatus.BadRequest(errors.Fmt("predicate: ancestors_of: %w", err))
	}

	// Note: The check ensuring startID.RootInvocationID matches q.RootInvocationID
	// has been moved to the service-level request validation.

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
					return err
				}
				// If an intermediate ancestor is missing (broken chain), just return
				// what we found so far. This is an unlikely scenario.
				return nil
			}
			return err
		}

		// If this is an ancestor (not the start node), add to results.
		if i > 0 {
			if err := f(row); err != nil {
				if errors.Is(err, ResponseLimitReachedErr) {
					return appstatus.Errorf(codes.Unimplemented, "pagination is not supported for ancestors_of queries")
				}
				return err
			}
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

	return nil
}

func (q *Query) queryAll(ctx context.Context, f func(*WorkUnitRow) error) (pageToken string, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/workunits.queryAll",
		attribute.String("root_invocation", string(q.RootInvocationID)),
		attribute.Int("page_size", q.PageSize),
	)
	defer func() { tracing.End(ts, err) }()

	if q.PageSize < 0 {
		return "", errors.New("PageSize < 0")
	}

	st := spanner.NewStatement(fmt.Sprintf(`
		SELECT %s
		FROM WorkUnits wu
		WHERE wu.RootInvocationShardId IN UNNEST(@ids)
			AND (
				(wu.RootInvocationShardId > @afterRootInvocationShardId) OR
				(wu.RootInvocationShardId = @afterRootInvocationShardId AND wu.WorkUnitId > @afterWorkUnitId)
			)
		ORDER BY wu.RootInvocationShardId, wu.WorkUnitId
		LIMIT @limit
	`, columnsToRead("wu", q.Mask)))

	st.Params = map[string]any{
		"ids":                        q.RootInvocationID.AllShardIDs(),
		"limit":                      q.PageSize,
		"afterRootInvocationShardId": "",
		"afterWorkUnitId":            "",
	}

	if q.PageToken != "" {
		tokens, err := pagination.ParseToken(q.PageToken)
		if err != nil {
			return "", err
		}
		if len(tokens) != 1 {
			return "", pagination.InvalidToken(errors.Fmt("expected 1 components, got %d", len(tokens)))
		}
		st.Params["afterWorkUnitId"] = tokens[0]
		id := ID{RootInvocationID: q.RootInvocationID, WorkUnitID: tokens[0]}
		st.Params["afterRootInvocationShardId"] = id.RootInvocationShardID().RowID()
	}

	var b spanutil.Buffer
	rowsProccessed := 0
	err = spanutil.Query(ctx, st, func(row *spanner.Row) error {
		wu, err := readRow(row, q.Mask, b)
		if err != nil {
			return err
		}
		// lastWorkUnitID captures the last processed work unit id.
		q.lastWorkUnitID = wu.ID.WorkUnitID
		if err := f(wu); err != nil {
			return err
		}
		rowsProccessed++
		return nil
	})
	if err != nil {
		if errors.Is(err, ResponseLimitReachedErr) {
			return pagination.Token(q.lastWorkUnitID), nil
		}
		return "", err
	}

	if rowsProccessed == q.PageSize {
		return pagination.Token(q.lastWorkUnitID), nil
	}
	return "", nil
}
