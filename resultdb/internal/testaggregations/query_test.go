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

package testaggregations

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestQuery(t *testing.T) {
	ftt.Run("Query", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("root-inv")
		testutil.MustApply(ctx, t, CreateTestData(rootInvID)...)

		query := &SingleLevelQuery{
			RootInvocationID: rootInvID,
			Level:            pb.AggregationLevel_FINE,
			PageSize:         100,
		}

		fetchAll := func(query *SingleLevelQuery) []*pb.TestAggregation {
			expectedPageSize := query.PageSize
			var results []*pb.TestAggregation
			var token string
			for {
				aggs, nextToken, err := query.Fetch(span.Single(ctx), token)
				assert.Loosely(t, err, should.BeNil, truth.LineContext(1))
				results = append(results, aggs...)
				if nextToken == "" {
					assert.Loosely(t, len(aggs), should.BeLessThanOrEqual(expectedPageSize))
					break
				} else {
					// While AIPs do not require it, the current implementation
					// should always return full pages unless the next page token
					// is empty.
					assert.Loosely(t, len(aggs), should.Equal(expectedPageSize))
				}
				token = nextToken
			}
			return results
		}

		t.Run("Root Invocation Level", func(t *ftt.Test) {
			expected := ExpectedRootInvocationAggregation()

			query.Level = pb.AggregationLevel_INVOCATION
			aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextToken, should.Equal(""))
			assert.Loosely(t, aggs, should.HaveLength(1))
			assert.Loosely(t, aggs[0], should.Match(expected))
		})

		t.Run("Module Level", func(t *ftt.Test) {
			query.Level = pb.AggregationLevel_MODULE

			t.Run("Default sorting", func(t *ftt.Test) {
				expected := ExpectedModuleAggregationsIDOrder()
				t.Run("Without pagination", func(t *ftt.Test) {
					query.PageSize = 7 // More than large enough for all modules.
					aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nextToken, should.Equal(""))
					assert.Loosely(t, aggs, should.Match(expected))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					query.PageSize = 1
					all := fetchAll(query)
					assert.Loosely(t, all, should.Match(expected))
				})
			})
			t.Run("UI sorting", func(t *ftt.Test) {
				query.Order = Ordering{ByUIPriority: true}
				expected := ExpectedModuleAggregationsUIOrder()
				t.Run("Without pagination", func(t *ftt.Test) {
					query.PageSize = 7 // Enough for all modules, plus one.
					query.Level = pb.AggregationLevel_MODULE
					aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nextToken, should.Equal(""))
					assert.Loosely(t, aggs, should.Match(expected))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					query.PageSize = 1
					all := fetchAll(query)
					assert.Loosely(t, all, should.Match(expected))
				})
			})
			t.Run("With module-level filter", func(t *ftt.Test) {
				query.TestPrefixFilter = &pb.TestIdentifierPrefix{
					Level: pb.AggregationLevel_MODULE,
					Id: &pb.TestIdentifier{
						ModuleName:        "m2",
						ModuleScheme:      "junit",
						ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					},
				}
				expected := ExpectedModuleAggregationsIDOrder()
				expected = expected[1:2]
				t.Run("With module variant", func(t *ftt.Test) {
					query.TestPrefixFilter.Id.ModuleVariant = pbutil.Variant("key", "value")
					query.TestPrefixFilter.Id.ModuleVariantHash = ""
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
				t.Run("With module variant hash", func(t *ftt.Test) {
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
			})
		})

		t.Run("Coarse Level", func(t *ftt.Test) {
			query.Level = pb.AggregationLevel_COARSE

			t.Run("Default sorting", func(t *ftt.Test) {
				expected := ExpectedCoarseAggregationsIDOrder()
				t.Run("Without pagination", func(t *ftt.Test) {
					query.PageSize = 5 // Enough for all items, plus one.
					aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nextToken, should.Equal(""))
					assert.Loosely(t, aggs, should.Match(expected))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					query.PageSize = 1
					all := fetchAll(query)
					assert.Loosely(t, all, should.Match(expected))
				})
			})
			t.Run("UI sorting", func(t *ftt.Test) {
				query.Order = Ordering{ByUIPriority: true}
				expected := ExpectedCoarseAggregationsUIOrder()
				t.Run("Without pagination", func(t *ftt.Test) {
					query.PageSize = 5 // More than large enough for all items.
					aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nextToken, should.Equal(""))
					assert.Loosely(t, aggs, should.Match(expected))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					query.PageSize = 1
					all := fetchAll(query)
					assert.Loosely(t, all, should.Match(expected))
				})
			})
			t.Run("With filters", func(t *ftt.Test) {
				query.TestPrefixFilter = &pb.TestIdentifierPrefix{
					Level: pb.AggregationLevel_MODULE,
					Id: &pb.TestIdentifier{
						ModuleName:    "m1",
						ModuleScheme:  "junit",
						ModuleVariant: pbutil.Variant("key", "value"),
					},
				}

				expected := ExpectedCoarseAggregationsIDOrder()
				t.Run("module-level filter", func(t *ftt.Test) {
					expected = expected[0:2]
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
				t.Run("coarse name-level filter", func(t *ftt.Test) {
					query.TestPrefixFilter.Level = pb.AggregationLevel_COARSE
					query.TestPrefixFilter.Id.CoarseName = "c2"
					expected = expected[1:2]
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
			})
		})

		t.Run("Fine level", func(t *ftt.Test) {
			query.Level = pb.AggregationLevel_FINE

			t.Run("Default sorting", func(t *ftt.Test) {
				expected := ExpectedFineAggregationsIDOrder()
				t.Run("Without pagination", func(t *ftt.Test) {
					query.PageSize = 7 // More than large enough for all items.
					aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nextToken, should.Equal(""))
					assert.Loosely(t, aggs, should.Match(expected))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					query.PageSize = 1
					all := fetchAll(query)
					assert.Loosely(t, all, should.Match(expected))
				})
			})
			t.Run("UI sorting", func(t *ftt.Test) {
				query.Order = Ordering{ByUIPriority: true}
				expected := ExpectedFineAggregationsUIOrder()
				t.Run("Without pagination", func(t *ftt.Test) {
					query.PageSize = 7 // More than large enough for all items.
					aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nextToken, should.Equal(""))
					assert.Loosely(t, aggs, should.Match(expected))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					query.PageSize = 1
					all := fetchAll(query)
					assert.Loosely(t, all, should.Match(expected))
				})
			})
			t.Run("With filters", func(t *ftt.Test) {
				query.TestPrefixFilter = &pb.TestIdentifierPrefix{
					Level: pb.AggregationLevel_MODULE,
					Id: &pb.TestIdentifier{
						ModuleName:        "m1",
						ModuleScheme:      "junit",
						ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					},
				}

				expected := ExpectedFineAggregationsIDOrder()
				t.Run("module-level filter", func(t *ftt.Test) {
					expected = expected[0:4]
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
				t.Run("coarse name-level filter", func(t *ftt.Test) {
					query.TestPrefixFilter.Level = pb.AggregationLevel_COARSE
					query.TestPrefixFilter.Id.CoarseName = "c1"
					expected = expected[0:3]
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
				t.Run("fine name-level filter", func(t *ftt.Test) {
					query.TestPrefixFilter.Level = pb.AggregationLevel_FINE
					query.TestPrefixFilter.Id.CoarseName = "c1"
					query.TestPrefixFilter.Id.FineName = "f2"
					expected = expected[1:2]
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
			})
		})
	})
}
