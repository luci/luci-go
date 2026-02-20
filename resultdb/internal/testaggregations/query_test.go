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
			t.Run("With search criteria", func(t *ftt.Test) {
				ClearAllVerdictsMatching(expected)
				t.Run("With matching filter", func(t *ftt.Test) {
					// All test results have the tag "mytag:myvalue" so this part of the filter
					// should have no effect.
					query.SearchCriteria = `test_id_structured.module_name = "m1" AND test_id_structured.module_scheme = "junit" AND test_id_structured.module_variant.key = "value"` +
						` AND tags.mytag = "myvalue"`

					// Counts for module m1.
					expected[0].MatchedVerdictCounts = &pb.TestAggregation_VerdictCounts{
						Failed:      1,
						Flaky:       1,
						Passed:      1,
						Skipped:     1,
						Exonerated:  1,
						FailedBase:  2,
						FlakyBase:   1,
						PassedBase:  1,
						SkippedBase: 1,
					}
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
				t.Run("With non-matching filter", func(t *ftt.Test) {
					query.SearchCriteria = `tags.mytag = "not-found-value"`

					assert.Loosely(t, fetchAll(query), should.Match(expected))
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
					query.PageSize = len(expected) + 1 // Enough for for all modules, plus one.
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
					query.PageSize = len(expected) + 1 // Enough for all modules, plus one.
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
			t.Run("With search criteria", func(t *ftt.Test) {
				// m5 has no test results so this confirms the filter matches across both test results
				// and modules.
				query.SearchCriteria = `test_id_structured.module_name = "m4" OR test_id_structured.module_name = "m5"`

				expected := ExpectedModuleAggregationsIDOrder()
				ClearAllModulesMatching(expected)
				ClearAllVerdictsMatching(expected)
				SetAllVerdictsMatching(expected[3:5]) // m4 is at offset 3, m5 is at offset 4
				SetAllModulesMatching(expected[3:5])

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
			t.Run("With search criteria", func(t *ftt.Test) {
				// All test results have the tag "mytag:myvalue" so this part of the filter
				// should have no effect.
				query.SearchCriteria = `test_id_structured.module_name = "m1" AND test_id_structured.module_scheme = "junit" AND test_id_structured.module_variant.key = "value"` +
					` AND tags.mytag = "myvalue"`

				expected := ExpectedCoarseAggregationsIDOrder()
				ClearAllVerdictsMatching(expected)
				SetAllVerdictsMatching(expected[0:2])
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
			t.Run("With search criteria", func(t *ftt.Test) {
				// All test results have the tag "mytag:myvalue" so this part of the filter
				// should have no effect.
				query.SearchCriteria = `test_id_structured.module_name = "m1" AND test_id_structured.module_scheme = "junit" AND test_id_structured.module_variant.key = "value"` +
					` AND tags.mytag = "myvalue"`

				expected := ExpectedFineAggregationsIDOrder()
				ClearAllVerdictsMatching(expected)
				SetAllVerdictsMatching(expected[0:4])
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
		t.Run("AIP-160 search criteria integration", func(t *ftt.Test) {
			expected := ExpectedFineAggregationsIDOrder()
			ClearAllVerdictsMatching(expected)
			query.Level = pb.AggregationLevel_FINE

			// These tests do not seek to comprehensively validate filter semantics (the
			// parser-generator library does most of that), they validate the AIP-160 filter
			// from `testresultsv2` package is correctly integrated and all required columns exist
			// (no invalid SQL is generated).
			query.SearchCriteria = `test_id_structured.module_name != "module"` +
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
				SetAllVerdictsMatching(expected)
				assert.Loosely(t, fetchAll(query), should.Match(expected))
			})
			t.Run("With limited access (some upgraded to full)", func(t *ftt.Test) {
				// Some results should match, but not all, because we filter on
				// tags and variant and these fields are only visible to us on those
				// results we have full access to.
				query.Access.Level = permissions.LimitedAccess
				query.Access.Realms = []string{"testdata:m3-s2"}
				results := fetchAll(query)

				for _, item := range expected {
					if item.Id.Id.ModuleName != "m3" {
						item.Id.Id.ModuleVariant = nil
					}
				}
				SetAllVerdictsMatching(expected[5:6])
				assert.Loosely(t, results, should.Match(expected))
			})
			t.Run("With limited access only", func(t *ftt.Test) {
				// No results should match because we filter on tags and variant and
				// these fields are only visible to us if we have full access to them.
				query.Access.Level = permissions.LimitedAccess
				query.Access.Realms = []string{}
				results := fetchAll(query)

				for _, item := range expected {
					item.Id.Id.ModuleVariant = nil
				}
				assert.Loosely(t, results, should.Match(expected))
			})
			t.Run("With implicit filter", func(t *ftt.Test) {
				// Check an aip.dev/160 implicit filter.
				query.SearchCriteria = `exonerated_test`
				SetAllVerdictsMatching(expected[1:2])
				assert.Loosely(t, fetchAll(query), should.Match(expected))
			})
		})

		t.Run("AIP-160 filter", func(t *ftt.Test) {
			t.Run("matched_verdict_counts", func(t *ftt.Test) {
				query.Level = pb.AggregationLevel_FINE
				expected := ExpectedFineAggregationsIDOrder()

				t.Run("passed", func(t *ftt.Test) {
					query.Filter = "matched_verdict_counts.passed > 0"
					assert.Loosely(t, fetchAll(query), should.Match(expected[0:1]))
				})
				t.Run("failed", func(t *ftt.Test) {
					query.Filter = "matched_verdict_counts.failed > 0"
					assert.Loosely(t, fetchAll(query), should.Match(expected[0:1]))
				})
				t.Run("exonerated", func(t *ftt.Test) {
					query.Filter = "matched_verdict_counts.exonerated > 0"
					assert.Loosely(t, fetchAll(query), should.Match(expected[1:2]))
				})
				t.Run("flaky", func(t *ftt.Test) {
					query.Filter = "matched_verdict_counts.flaky > 0"
					assert.Loosely(t, fetchAll(query), should.Match(expected[2:3]))
				})
				t.Run("skipped", func(t *ftt.Test) {
					query.Filter = "matched_verdict_counts.skipped > 0"
					assert.Loosely(t, fetchAll(query), should.Match(expected[3:4]))
				})
				t.Run("execution_errored", func(t *ftt.Test) {
					query.Filter = "matched_verdict_counts.execution_errored > 0"
					assert.Loosely(t, fetchAll(query), should.Match(expected[4:5]))
				})
				t.Run("precluded", func(t *ftt.Test) {
					query.Filter = "matched_verdict_counts.precluded > 0"
					assert.Loosely(t, fetchAll(query), should.Match(expected[5:6]))
				})
			})
			t.Run("module_status", func(t *ftt.Test) {
				query.Level = pb.AggregationLevel_MODULE
				expected := ExpectedModuleAggregationsIDOrder()

				t.Run("FAILED", func(t *ftt.Test) {
					query.Filter = "module_status = FAILED"
					assert.Loosely(t, fetchAll(query), should.Match(expected[2:3]))
				})
				t.Run("RUNNING", func(t *ftt.Test) {
					query.Filter = "module_status = RUNNING"
					assert.Loosely(t, fetchAll(query), should.Match(expected[1:2]))
				})
				t.Run("FLAKY", func(t *ftt.Test) {
					query.Filter = "module_status = FLAKY"
					assert.Loosely(t, fetchAll(query), should.Match(expected[6:7]))
				})
			})
			t.Run("module_matches", func(t *ftt.Test) {
				query.Level = pb.AggregationLevel_MODULE
				expected := ExpectedModuleAggregationsIDOrder()

				t.Run("true", func(t *ftt.Test) {
					expected = []*pb.TestAggregation{
						expected[0], // m1
						expected[4], // m5
					}
					query.SearchCriteria = `"m1" OR "m5"`
					query.Filter = "module_matches = true"
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
				t.Run("false", func(t *ftt.Test) {
					expected = []*pb.TestAggregation{
						expected[1], // m2
						expected[2], // m3
						expected[3], // m4
						expected[5], // m6
						expected[6], // m7
					}
					query.SearchCriteria = `"m1" OR "m5"`
					query.Filter = "module_matches = false"

					// As we are filtering to modules that don't match the filter criteria.
					ClearAllModulesMatching(expected)
					ClearAllVerdictsMatching(expected)
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
			})
			t.Run("All fields supported at all levels", func(t *ftt.Test) {
				// For fine, coarse and invocation levels, the module_status should be treated as always UNSPECIFIED,
				// and module_matches as always FALSE, as this is what is on the response row.
				query.Filter = "matched_verdict_counts.passed > 0 OR matched_verdict_counts.flaky > 0 OR matched_verdict_counts.failed > 0" +
					" OR matched_verdict_counts.skipped > 0 OR matched_verdict_counts.execution_errored > 0 OR matched_verdict_counts.precluded > 0" +
					" OR matched_verdict_counts.exonerated > 0 OR module_status = FAILED OR module_matches = true"

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
					assert.Loosely(t, fetchAll(query), should.Match(expected))
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
