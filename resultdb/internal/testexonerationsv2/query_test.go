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

package testexonerationsv2

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"testing"

	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
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
		// We create 10 exonerations with varying IDs to test ordering and filtering.

		// IDs will be:
		// m1, s1, v1, c1, f1, t1, w1, e0
		// m1, s1, v1, c1, f1, t1, w1, e1
		// ...
		// m1, s1, v1, c1, f1, t1, w1, e9
		var expected []*TestExonerationRow
		for i := 0; i < 10; i++ {
			row := NewBuilder().
				WithRootInvocationShardID(rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: i % rootinvocations.RootInvocationShardCount}).
				WithModuleName("m1").
				WithModuleScheme("s1").
				WithModuleVariant(pbutil.Variant("k", "v1")).
				WithCoarseName("c1").
				WithFineName("f1").
				WithCaseName(fmt.Sprintf("t%d", 10-i)).
				WithWorkUnitID("w1").
				WithExonerationID("e1").
				WithRealm(fmt.Sprintf("testproject:realm-%d", i%2)). // Alternating realms: realm-0, realm-1
				Build()

			ms = append(ms, InsertForTesting(row))
			expected = append(expected, row)
		}
		testutil.MustApply(ctx, t, ms...)

		q := &Query{
			RootInvocation: rootInvID,
			Access:         permissions.RootInvocationAccess{Level: permissions.FullAccess},
		}
		opts := spanutil.BufferingOptions{
			FirstPageSize:  10,
			SecondPageSize: 10,
			GrowthFactor:   1.0,
			MaxPageSize:    10_000,
		}

		// Helper to fetch all results using the iterator.
		fetchAll := func(ctx context.Context, q *Query, opts spanutil.BufferingOptions) ([]*TestExonerationRow, error) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			it := q.List(ctx, PageToken{}, opts)
			var results []*TestExonerationRow
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
			it := q.List(ctx, PageToken{}, opts)

			// Fetch first page (3 items).
			var page1 []*TestExonerationRow
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
			var remaining []*TestExonerationRow
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
			verdictID := func(i int) testresultsv2.VerdictID {
				return testresultsv2.VerdictID{
					RootInvocationShardID: rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: i % rootinvocations.RootInvocationShardCount},
					ModuleName:            "m1",
					ModuleScheme:          "s1",
					ModuleVariantHash:     pbutil.VariantHash(pbutil.Variant("k", "v1")),
					CoarseName:            "c1",
					FineName:              "f1",
					CaseName:              fmt.Sprintf("t%d", 10-i),
				}
			}
			nominated := []testresultsv2.VerdictID{
				verdictID(3),
				verdictID(1),
				verdictID(1),
				verdictID(5),
				verdictID(100), // Does not exist.
				verdictID(1),
			}

			// Note: verdicts must be return in the order they are requested, including duplicates.
			expected := []*TestExonerationRow{
				withRequestOrdinal(expected[3], 1),
				withRequestOrdinal(expected[1], 2),
				withRequestOrdinal(expected[1], 3),
				withRequestOrdinal(expected[5], 4),
				withRequestOrdinal(expected[1], 6),
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

		t.Run("With limited access", func(t *ftt.Test) {
			q.Access.Level = permissions.LimitedAccess

			t.Run("Baseline", func(t *ftt.Test) {
				// No realms allowed.
				expectedMasked := maskedExonerations(expected, nil)
				results, err := fetchAll(ctx, q, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, results, should.Match(expectedMasked))
			})

			t.Run("With upgraded realms", func(t *ftt.Test) {
				// Allow access to realm-0.
				q.Access.Realms = []string{"testproject:realm-0"}

				expectedMasked := maskedExonerations(expected, q.Access.Realms)
				results, err := fetchAll(ctx, q, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, results, should.Match(expectedMasked))

				// Verify that we actually have a mix of masked and unmasked.
				var maskedCount, unmaskedCount int
				for _, r := range results {
					if r.IsMasked {
						maskedCount++
					} else {
						unmaskedCount++
					}
				}
				assert.Loosely(t, maskedCount, should.BeGreaterThan(0))
				assert.Loosely(t, unmaskedCount, should.BeGreaterThan(0))
			})
		})

		t.Run("Ordering by test ID", func(t *ftt.Test) {
			q.Order = testresultsv2.OrderingByTestID

			// Re-sort expectations.
			sort.Slice(expected, func(i, j int) bool {
				// For the example data, test IDs only differ in case name.
				return expected[i].ID.CaseName < expected[j].ID.CaseName
			})

			t.Run("Baseline", func(t *ftt.Test) {
				results, err := fetchAll(ctx, q, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, results, should.Match(expected))
			})

			t.Run("Pagination", func(t *ftt.Test) {
				opts.FirstPageSize = 1
				opts.SecondPageSize = 1
				opts.MaxPageSize = 1

				results, err := fetchAll(ctx, q, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, results, should.Match(expected))
			})

			t.Run("With prefix filter", func(t *ftt.Test) {
				q.TestPrefixFilter = &pb.TestIdentifierPrefix{
					Level: pb.AggregationLevel_CASE,
					Id:    expected[5].ToProto().TestIdStructured,
				}
				results, err := fetchAll(ctx, q, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, results, should.Match(expected[5:6]))
			})
		})
	})
}

// maskedExonerations returns masked copies of the given test results,
// with results from realms in the given list *not* masked.
func maskedExonerations(exonerations []*TestExonerationRow, allowedRealms []string) []*TestExonerationRow {
	var masked []*TestExonerationRow
	for _, r := range exonerations {
		// Clone the row.
		m := *r
		if !slices.Contains(allowedRealms, r.Realm) {
			m.IsMasked = true
			m.ModuleVariant = nil
		}
		masked = append(masked, &m)
	}
	return masked
}

func withRequestOrdinal(row *TestExonerationRow, ordinal int) *TestExonerationRow {
	result := *row
	result.RequestOrdinal = ordinal
	return &result
}
