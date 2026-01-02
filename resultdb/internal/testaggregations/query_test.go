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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestQuery(t *testing.T) {
	ftt.Run("Query", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.

		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceholderServiceConfig())
		assert.NoErr(t, err)

		rootInvID := rootinvocations.ID("root-inv")
		rootInvRow := rootinvocations.NewBuilder(rootInvID).Build()
		ms := insert.RootInvocationWithRootWorkUnit(rootInvRow)
		ms = append(ms, CreateTestData(rootInvID)...)
		testutil.MustApply(ctx, t, ms...)

		query := &SingleLevelQuery{
			RootInvocationID: rootInvID,
			Level:            pb.AggregationLevel_FINE,
			Access: permissions.RootInvocationAccess{
				Level: permissions.FullAccess,
			},
			PageSize: 100,
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
			query.Level = pb.AggregationLevel_INVOCATION

			expected := ExpectedRootInvocationAggregation()

			t.Run("Baseline", func(t *ftt.Test) {
				aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nextToken, should.Equal(""))
				assert.Loosely(t, aggs, should.Match(expected))
			})
			t.Run("With contains test result filter", func(t *ftt.Test) {
				t.Run("With matching filter", func(t *ftt.Test) {
					// All test results have the tag "mytag:myvalue" so this part of the filter
					// should have no effect.
					query.ContainsTestResultFilter = `test_id_structured.module_name = "m1" AND test_id_structured.module_scheme = "junit" AND test_id_structured.module_variant.key = "value"` +
						` AND tags.mytag = "myvalue"`

					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
				t.Run("With non-matching filter", func(t *ftt.Test) {
					query.ContainsTestResultFilter = `tags.mytag = "not-found-value"`

					// No test results in the invocation matches the filter.
					assert.Loosely(t, fetchAll(query), should.HaveLength(0))
				})
			})
			t.Run("Limited access", func(t *ftt.Test) {
				// Should make no difference as this only relies on information
				// that is visible to limited users.
				query.Access.Level = permissions.LimitedAccess
				assert.Loosely(t, fetchAll(query), should.Match(expected))
			})
		})

		t.Run("Module Level", func(t *ftt.Test) {
			query.Level = pb.AggregationLevel_MODULE

			expected := ExpectedModuleAggregationsIDOrder()
			t.Run("Baseline", func(t *ftt.Test) {
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
			t.Run("With prefix filter", func(t *ftt.Test) {
				query.TestPrefixFilter = &pb.TestIdentifierPrefix{
					Level: pb.AggregationLevel_MODULE,
					Id: &pb.TestIdentifier{
						ModuleName:        "m2",
						ModuleScheme:      "noconfig",
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
			t.Run("With contains test result filter", func(t *ftt.Test) {
				// All test results have the tag "mytag:myvalue" so this part of the filter
				// should have no effect.
				query.ContainsTestResultFilter = `test_id_structured.module_name = "m1" AND test_id_structured.module_scheme = "junit" AND test_id_structured.module_variant.key = "value"` +
					` AND tags.mytag = "myvalue"`

				expected := ExpectedModuleAggregationsIDOrder()
				expected = expected[0:1]
				assert.Loosely(t, fetchAll(query), should.Match(expected))
			})
			t.Run("With limited access", func(t *ftt.Test) {
				query.Access.Level = permissions.LimitedAccess
				query.Access.Realms = []string{"testdata:m3-s2"}
				for _, item := range expected {
					if item.Id.Id.ModuleName != "m3" {
						item.Id.Id.ModuleVariant = nil
					}
				}
				assert.Loosely(t, fetchAll(query), should.Match(expected))
			})
		})

		t.Run("Coarse Level", func(t *ftt.Test) {
			query.Level = pb.AggregationLevel_COARSE
			expected := ExpectedCoarseAggregationsIDOrder()

			t.Run("Baseline", func(t *ftt.Test) {
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
			t.Run("With prefix filter", func(t *ftt.Test) {
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
			t.Run("With contains test result filter", func(t *ftt.Test) {
				// All test results have the tag "mytag:myvalue" so this part of the filter
				// should have no effect.
				query.ContainsTestResultFilter = `test_id_structured.module_name = "m1" AND test_id_structured.module_scheme = "junit" AND test_id_structured.module_variant.key = "value"` +
					` AND tags.mytag = "myvalue"`

				expected := ExpectedCoarseAggregationsIDOrder()
				expected = expected[0:2]
				assert.Loosely(t, fetchAll(query), should.Match(expected))
			})
			t.Run("With limited access", func(t *ftt.Test) {
				query.Access.Level = permissions.LimitedAccess
				query.Access.Realms = []string{"testdata:m3-s2"}
				for _, item := range expected {
					if item.Id.Id.ModuleName != "m3" {
						item.Id.Id.ModuleVariant = nil
					}
				}
				assert.Loosely(t, fetchAll(query), should.Match(expected))
			})
		})

		t.Run("Fine level", func(t *ftt.Test) {
			query.Level = pb.AggregationLevel_FINE
			expected := ExpectedFineAggregationsIDOrder()

			t.Run("Baseline", func(t *ftt.Test) {
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
			t.Run("With prefix filter", func(t *ftt.Test) {
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
			t.Run("With contains test result filter", func(t *ftt.Test) {
				// All test results have the tag "mytag:myvalue" so this part of the filter
				// should have no effect.
				query.ContainsTestResultFilter = `test_id_structured.module_name = "m1" AND test_id_structured.module_scheme = "junit" AND test_id_structured.module_variant.key = "value"` +
					` AND tags.mytag = "myvalue"`

				expected := ExpectedFineAggregationsIDOrder()
				expected = expected[0:4]
				assert.Loosely(t, fetchAll(query), should.Match(expected))
			})
			t.Run("With limited access", func(t *ftt.Test) {
				query.Access.Level = permissions.LimitedAccess
				query.Access.Realms = []string{"testdata:m3-s2"}
				for _, item := range expected {
					if item.Id.Id.ModuleName != "m3" {
						item.Id.Id.ModuleVariant = nil
					}
				}
				assert.Loosely(t, fetchAll(query), should.Match(expected))
			})
		})
		t.Run("AIP-160 test result filter integration", func(t *ftt.Test) {
			expected := ExpectedFineAggregationsIDOrder()
			query.Level = pb.AggregationLevel_FINE

			// These tests do not seek to comprehensively validate filter semantics (the
			// parser-generator library does most of that), they validate the AIP-160 filter
			// from `testresultsv2` package is correctly integrated and all required columns exist
			// (no invalid SQL is generated).
			query.ContainsTestResultFilter = `test_id_structured.module_name != "module"` +
				` AND test_id_structured.module_scheme != "scheme"` +
				` AND test_id_structured.module_variant.key = "value"` +
				` AND test_id_structured.module_variant_hash != "varianthash"` +
				` AND test_id_structured.coarse_name != "coarse"` +
				` AND test_id_structured.fine_name != "fine"` +
				` AND test_id_structured.case_name != "case"` +
				` AND test_metadata.name != "somename"` +
				` AND tags.mytag = "myvalue"` +
				` AND test_metadata.location.repo != "repo"` +
				` AND test_metadata.location.file_name != "filename"` +
				` AND (status != PRECLUDED OR status = PRECLUDED)` +
				` AND duration < 100s`

			t.Run("With full access", func(t *ftt.Test) {
				query.Access.Level = permissions.FullAccess
				assert.Loosely(t, fetchAll(query), should.Match(expected))
			})
			t.Run("With limited access (some upgraded to full)", func(t *ftt.Test) {
				// Some results should match, but not all, because we filter on
				// tags and variant and these fields are only visible to us on those
				// results we have full access to.
				query.Access.Level = permissions.LimitedAccess
				query.Access.Realms = []string{"testdata:m3-s2"}
				results := fetchAll(query)
				expected = expected[5:6]
				assert.Loosely(t, results, should.Match(expected))
			})
			t.Run("With limited access only", func(t *ftt.Test) {
				// No results should match because we filter on tags and variant and
				// these fields are only visible to us if we have full access to them.
				query.Access.Level = permissions.LimitedAccess
				query.Access.Realms = []string{}
				results := fetchAll(query)
				assert.Loosely(t, results, should.HaveLength(0))
			})
			t.Run("With implicit filter", func(t *ftt.Test) {
				// Check an aip.dev/160 implicit filter.
				query.ContainsTestResultFilter = `exonerated_test`
				expected = expected[1:2]
				assert.Loosely(t, fetchAll(query), should.Match(expected))
			})
		})

		t.Run("Top-level filter", func(t *ftt.Test) {
			t.Run("verdict_counts", func(t *ftt.Test) {
				query.Level = pb.AggregationLevel_FINE
				expected := ExpectedFineAggregationsIDOrder()

				t.Run("passed", func(t *ftt.Test) {
					query.Filter = "verdict_counts.passed > 0"
					assert.Loosely(t, fetchAll(query), should.Match(expected[0:1]))
				})
				t.Run("failed", func(t *ftt.Test) {
					query.Filter = "verdict_counts.failed > 0"
					assert.Loosely(t, fetchAll(query), should.Match(expected[0:1]))
				})
				t.Run("exonerated", func(t *ftt.Test) {
					query.Filter = "verdict_counts.exonerated > 0"
					assert.Loosely(t, fetchAll(query), should.Match(expected[1:2]))
				})
				t.Run("flaky", func(t *ftt.Test) {
					query.Filter = "verdict_counts.flaky > 0"
					assert.Loosely(t, fetchAll(query), should.Match(expected[2:3]))
				})
				t.Run("skipped", func(t *ftt.Test) {
					query.Filter = "verdict_counts.skipped > 0"
					assert.Loosely(t, fetchAll(query), should.Match(expected[3:4]))
				})
				t.Run("execution_errored", func(t *ftt.Test) {
					query.Filter = "verdict_counts.execution_errored > 0"
					assert.Loosely(t, fetchAll(query), should.Match(expected[4:5]))
				})
				t.Run("precluded", func(t *ftt.Test) {
					query.Filter = "verdict_counts.precluded > 0"
					assert.Loosely(t, fetchAll(query), should.Match(expected[5:6]))
				})
			})
			t.Run("module_status", func(t *ftt.Test) {
				query.Level = pb.AggregationLevel_MODULE
				expected := ExpectedModuleAggregationsIDOrder()

				t.Run("ERRORED", func(t *ftt.Test) {
					query.Filter = "module_status = ERRORED"
					assert.Loosely(t, fetchAll(query), should.Match(expected[2:3]))
				})
				t.Run("RUNNING", func(t *ftt.Test) {
					query.Filter = "module_status = RUNNING"
					assert.Loosely(t, fetchAll(query), should.Match(expected[1:2]))
				})
			})
			t.Run("All fields supported at all levels", func(t *ftt.Test) {
				// For fine, coarse and invocation levels, the module_status should be treated as always UNSPECIFIED,
				// as this is what is on the response row.
				query.Filter = "verdict_counts.passed > 0 OR verdict_counts.flaky > 0 OR verdict_counts.failed > 0" +
					" OR verdict_counts.skipped > 0 OR verdict_counts.execution_errored > 0 OR verdict_counts.precluded > 0" +
					" OR verdict_counts.exonerated > 0 OR module_status = ERRORED"

				t.Run("At fine-level", func(t *ftt.Test) {
					query.Level = pb.AggregationLevel_FINE
					expected := ExpectedFineAggregationsIDOrder()
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
				t.Run("At coarse-level", func(t *ftt.Test) {
					query.Level = pb.AggregationLevel_COARSE
					expected := ExpectedCoarseAggregationsIDOrder()
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
				t.Run("At module-level", func(t *ftt.Test) {
					query.Level = pb.AggregationLevel_MODULE
					expected := ExpectedModuleAggregationsIDOrder()
					assert.Loosely(t, fetchAll(query), should.Match(expected[0:3]))
				})
				t.Run("At invocation-level", func(t *ftt.Test) {
					query.Level = pb.AggregationLevel_INVOCATION
					expected := ExpectedRootInvocationAggregation()
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
			})
		})
	})
}
