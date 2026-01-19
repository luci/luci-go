// Copyright 2026 The LUCI Authors.
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

package testresultsv2

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestQuery(t *testing.T) {
	ftt.Run("Query", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		// Create test data.
		rootInvID := rootinvocations.ID("root-inv")
		rootInvRow := rootinvocations.NewBuilder(rootInvID).Build()
		ms := insert.RootInvocationWithRootWorkUnit(rootInvRow)
		// We create 10 results with varying IDs to test ordering and filtering.

		// IDs will be:
		// m1, s1, v1, c1, f1, t0, w1, r1
		// m1, s1, v1, c1, f1, t1, w1, r1
		// ...
		// m1, s1, v1, c1, f1, t9, w1, r1
		var expected []*TestResultRow
		for i := 0; i < 10; i++ {
			row := NewBuilder().
				WithRootInvocationShardID(rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: i % rootinvocations.RootInvocationShardCount}).
				WithModuleName("m1").
				WithModuleScheme("s1").
				WithModuleVariant(pbutil.Variant("k", "v1")).
				WithCoarseName("c1").
				WithFineName("f1").
				WithCaseName(fmt.Sprintf("t%d", i)).
				WithWorkUnitID("w1").
				WithResultID("r1").
				Build()

			ms = append(ms, InsertForTesting(row))
			expected = append(expected, row)
		}
		testutil.MustApply(ctx, t, ms...)

		q := &Query{
			RootInvocation: rootInvID,
		}
		opts := spanutil.BufferingOptions{
			FirstPageSize:  10,
			SecondPageSize: 10,
			GrowthFactor:   1.0,
		}

		// Helper to fetch all results using the iterator.
		fetchAll := func(ctx context.Context, q *Query, opts spanutil.BufferingOptions) ([]*TestResultRow, error) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			it := q.List(ctx, ID{}, opts)
			var results []*TestResultRow
			for {
				row, err := it.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					return nil, err
				}
				results = append(results, row)
			}
			return results, nil
		}

		t.Run("Baseline", func(t *ftt.Test) {
			results, err := fetchAll(ctx, q, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.Match(expected))
		})

		t.Run("Page tokens work correctly", func(t *ftt.Test) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			opts.FirstPageSize = 3
			opts.SecondPageSize = 1
			it := q.List(ctx, ID{}, opts)

			// Fetch first page (3 items).
			var page1 []*TestResultRow
			for i := 0; i < 3; i++ {
				row, err := it.Next()
				assert.Loosely(t, err, should.BeNil)
				page1 = append(page1, row)
			}
			assert.Loosely(t, page1, should.Match(expected[:3]))

			// Get token.
			token := it.PageToken()
			assert.Loosely(t, token, should.NotBeZero)

			// Start new query from token.
			it2 := q.List(ctx, token, opts)
			var remaining []*TestResultRow
			for {
				row, err := it2.Next()
				if err == iterator.Done {
					break
				}
				assert.Loosely(t, err, should.BeNil)
				remaining = append(remaining, row)
			}
			assert.Loosely(t, remaining, should.Match(expected[3:]))
		})

		t.Run("With prefix filter", func(t *ftt.Test) {
			// Insert a result that shouldn't match.
			otherRow := NewBuilder().
				WithRootInvocationShardID(rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: 0}).
				WithModuleName("m2"). // Different module
				Build()
			testutil.MustApply(ctx, t, InsertForTesting(otherRow))

			q.TestPrefixFilter = &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_MODULE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m1",
					ModuleScheme:      "s1",
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k", "v1")),
				},
			}

			results, err := fetchAll(ctx, q, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.Match(expected))
		})

		t.Run("With nominated IDs", func(t *ftt.Test) {
			verdictID := func(i int) VerdictID {
				return VerdictID{
					RootInvocationShardID: rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: i % rootinvocations.RootInvocationShardCount},
					ModuleName:            "m1",
					ModuleScheme:          "s1",
					ModuleVariantHash:     pbutil.VariantHash(pbutil.Variant("k", "v1")),
					CoarseName:            "c1",
					FineName:              "f1",
					CaseName:              fmt.Sprintf("t%d", i),
				}
			}
			nominated := []VerdictID{
				verdictID(3),
				verdictID(1),
				verdictID(5),
			}
			// Note: rows are still returned in primary key order, which may not match the order in the list.
			expected := []*TestResultRow{
				expected[1],
				expected[3],
				expected[5],
			}

			q.VerdictIDs = nominated
			results, err := fetchAll(ctx, q, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.Match(expected))
		})

		t.Run("With empty invocation", func(t *ftt.Test) {
			emptyInvID := rootinvocations.ID("empty-inv")
			emptyInvRow := rootinvocations.NewBuilder(emptyInvID).Build()
			ms := insert.RootInvocationWithRootWorkUnit(emptyInvRow)
			testutil.MustApply(ctx, t, ms...)

			q.RootInvocation = emptyInvID
			results, err := fetchAll(ctx, q, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, results, should.BeEmpty)
		})
	})
}
