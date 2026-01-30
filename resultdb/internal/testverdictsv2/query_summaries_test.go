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

package testverdictsv2

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestQuerySummaries(t *testing.T) {
	ftt.Run("QuerySummaries", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("root-inv")
		rootInvRow := rootinvocations.NewBuilder(rootInvID).Build()
		ms := insert.RootInvocationWithRootWorkUnit(rootInvRow)
		ms = append(ms, CreateTestData(rootInvID)...)
		testutil.MustApply(ctx, t, ms...)

		q := &QuerySummaries{
			RootInvocationID: rootInvID,
			Order:            Ordering{},
			Access: permissions.RootInvocationAccess{
				Level: permissions.FullAccess,
			},
		}
		opts := FetchOptions{
			PageSize:           100,
			ResponseLimitBytes: 0, // No limit.
			VerdictResultLimit: StandardVerdictResultLimit,
			VerdictSizeLimit:   StandardVerdictSizeLimit,
			TotalResultLimit:   10_000,
		}

		fetchOne := func(q *QuerySummaries, token PageToken, opts FetchOptions) (verdicts []*pb.TestVerdict, nextToken PageToken, err error) {
			txn, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			verdicts, nextToken, err = q.Fetch(txn, token, opts)
			return
		}

		fetchAll := func(q *QuerySummaries, opts FetchOptions) []*pb.TestVerdict {
			var results []*pb.TestVerdict
			var token PageToken
			for {
				verdicts, nextToken, err := fetchOne(q, token, opts)
				assert.Loosely(t, err, should.BeNil, truth.LineContext(1))
				results = append(results, verdicts...)
				if nextToken == (PageToken{}) {
					assert.Loosely(t, len(verdicts), should.BeLessThanOrEqual(opts.PageSize))
					break
				} else {
					if opts.ResponseLimitBytes == 0 && opts.TotalResultLimit == 0 {
						// If there were no secondary constraints on the page size,
						// verify the page is exactly opts.PageSize, not simply less than or equal.
						assert.Loosely(t, len(verdicts), should.Equal(opts.PageSize))
					} else {
						// If there was a secondary constraint, there should be between 1
						// and opts.PageSize results.
						assert.Loosely(t, len(verdicts), should.BeGreaterThan(0))
						assert.Loosely(t, len(verdicts), should.BeLessThanOrEqual(opts.PageSize))
					}
				}
				token = nextToken
			}
			return results
		}

		expected := ToBasicView(ExpectedVerdicts(rootInvID))

		t.Run("Baseline", func(t *ftt.Test) {
			verdicts, token, err := fetchOne(q, PageToken{}, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, token, should.Equal(PageToken{}))
			assert.Loosely(t, verdicts, should.Match(expected))
		})
		t.Run("With empty invocation", func(t *ftt.Test) {
			emptyInvID := rootinvocations.ID("empty-inv")
			emptyInvRow := rootinvocations.NewBuilder(emptyInvID).Build()
			ms := insert.RootInvocationWithRootWorkUnit(emptyInvRow)
			testutil.MustApply(ctx, t, ms...)
			q.RootInvocationID = emptyInvID

			verdicts, token, err := fetchOne(q, PageToken{}, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, token, should.Equal(PageToken{}))
			assert.Loosely(t, verdicts, should.HaveLength(0))
		})
		t.Run("Ordering", func(t *ftt.Test) {
			// Map expected items to map for easy retrieval
			m := make(map[string]*pb.TestVerdict)
			for _, v := range expected {
				m[v.TestId] = v
			}

			t.Run("By UI priority", func(t *ftt.Test) {
				q.Order = Ordering{ByUIPriority: true}
				expectedUIOrder := []*pb.TestVerdict{
					m[flatTestID("m1", "c1", "f1", "t2")], // Failed, Priority 0
					m[flatTestID("m1", "c2", "f1", "t6")], // Precluded, Priority 30
					m[flatTestID("m2", "c1", "f1", "t7")], // Execution Errored, Priority 30
					m[flatTestID("m1", "c1", "f1", "t3")], // Flaky, Priority 70
					m[flatTestID("m1", "c1", "f2", "t5")], // Exonerated, Priority 90
					m[flatTestID("m1", "c1", "f1", "t4")], // Skipped, Priority 100
					m[flatTestID("m1", "c1", "f1", "t1")], // Passed, Priority 100. Sorted after t4 because of primary key order.
				}

				t.Run("Without pagination", func(t *ftt.Test) {
					verdicts, token, err := fetchOne(q, PageToken{}, opts)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, token, should.Equal(PageToken{}))
					assert.Loosely(t, verdicts, should.Match(expectedUIOrder))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					opts.PageSize = 1
					results := fetchAll(q, opts)
					assert.Loosely(t, results, should.Match(expectedUIOrder))
				})
			})
			t.Run("By UI priority, then Test ID", func(t *ftt.Test) {
				q.Order = Ordering{ByUIPriority: true, ByStructuredTestID: true}
				expectedUIOrder := []*pb.TestVerdict{
					m[flatTestID("m1", "c1", "f1", "t2")], // Failed, Priority 0
					m[flatTestID("m1", "c2", "f1", "t6")], // Precluded, Priority 30
					m[flatTestID("m2", "c1", "f1", "t7")], // Execution Errored, Priority 30
					m[flatTestID("m1", "c1", "f1", "t3")], // Flaky, Priority 70
					m[flatTestID("m1", "c1", "f2", "t5")], // Exonerated, Priority 90
					m[flatTestID("m1", "c1", "f1", "t1")], // Passed, Priority 100.
					m[flatTestID("m1", "c1", "f1", "t4")], // Skipped, Priority 100
				}

				t.Run("Without pagination", func(t *ftt.Test) {
					verdicts, token, err := fetchOne(q, PageToken{}, opts)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, token, should.Equal(PageToken{}))
					assert.Loosely(t, verdicts, should.Match(expectedUIOrder))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					opts.PageSize = 1
					results := fetchAll(q, opts)
					assert.Loosely(t, results, should.Match(expectedUIOrder))
				})
			})
			t.Run("By Test ID", func(t *ftt.Test) {
				q.Order = Ordering{ByStructuredTestID: true}

				expectedUIOrder := []*pb.TestVerdict{
					m[flatTestID("m1", "c1", "f1", "t1")],
					m[flatTestID("m1", "c1", "f1", "t2")],
					m[flatTestID("m1", "c1", "f1", "t3")],
					m[flatTestID("m1", "c1", "f1", "t4")],
					m[flatTestID("m1", "c1", "f2", "t5")],
					m[flatTestID("m1", "c2", "f1", "t6")],
					m[flatTestID("m2", "c1", "f1", "t7")],
				}

				t.Run("Without pagination", func(t *ftt.Test) {
					verdicts, token, err := fetchOne(q, PageToken{}, opts)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, token, should.Equal(PageToken{}))
					assert.Loosely(t, verdicts, should.Match(expectedUIOrder))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					opts.PageSize = 1
					results := fetchAll(q, opts)
					assert.Loosely(t, results, should.Match(expectedUIOrder))
				})
			})
		})
		t.Run("With page size", func(t *ftt.Test) {
			opts.PageSize = 1
			results := fetchAll(q, opts)
			assert.Loosely(t, results, should.Match(expected))
		})
		t.Run("With response limit bytes", func(t *ftt.Test) {
			t.Run("Makes progress", func(t *ftt.Test) {
				// While results may be split over multiple pages, they should all be
				// returned.
				// Set limit to allow at least one item.
				maxItemSize := 0
				for _, item := range expected {
					maxItemSize = max(maxItemSize, proto.Size(item))
				}
				opts.ResponseLimitBytes = maxItemSize + 1000
				results := fetchAll(q, opts)
				assert.Loosely(t, results, should.Match(expected))
			})
			t.Run("Limit too small", func(t *ftt.Test) {
				// Expect error because the limit is too low to return even one row.
				opts.ResponseLimitBytes = 1
				_, _, err := fetchOne(q, PageToken{}, opts)
				assert.Loosely(t, err, should.ErrLike("a single verdict ("))
				assert.Loosely(t, err, should.ErrLike("bytes) was larger than the total response limit (1 bytes)"))
			})
			t.Run("Limit is applied correctly", func(t *ftt.Test) {
				// Should return two rows, as the limit is hard.
				opts.ResponseLimitBytes = (proto.Size(expected[0]) + 1000) + (proto.Size(expected[1]) + 1000) + 1

				verdicts, token, err := fetchOne(q, PageToken{}, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, token, should.NotEqual(PageToken{}))
				assert.Loosely(t, verdicts, should.Match(expected[:2]))
			})
		})
		t.Run("With contains test result filter", func(t *ftt.Test) {
			// These tests do not seek to comprehensively validate filter semantics (the
			// parser-generator library does most of that), they validate the AIP-160 filter
			// from `testresultsv2` package is correctly integrated and all required columns exist
			// (no invalid SQL is generated).
			q.ContainsTestResultFilter = `test_id_structured.module_name != "module"` +
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
				q.Access.Level = permissions.FullAccess
				assert.Loosely(t, fetchAll(q, opts), should.Match(expected))
			})
			t.Run("With limited access (some upgraded to full)", func(t *ftt.Test) {
				// Only some results should match, because we filter on
				// tags and variant and these fields are only visible to us on those
				// results we have full access to.
				q.Access.Level = permissions.LimitedAccess
				q.Access.Realms = []string{"testproject:t4-r1"}
				expectedLimited := ToBasicView(ExpectedMaskedVerdicts(ExpectedVerdicts(rootInvID), q.Access.Realms))
				expectedLimited = []*pb.TestVerdict{VerdictByCaseName(expectedLimited, "t4")}
				assert.Loosely(t, fetchAll(q, opts), should.Match(expectedLimited))
			})
			t.Run("With implicit filter", func(t *ftt.Test) {
				// Check an aip.dev/160 implicit filter.
				q.ContainsTestResultFilter = `t3`
				expected = []*pb.TestVerdict{VerdictByCaseName(expected, "t3")}
				assert.Loosely(t, fetchAll(q, opts), should.Match(expected))
			})
		})
		t.Run("With prefix filter", func(t *ftt.Test) {
			q.TestPrefixFilter = &pb.TestIdentifierPrefix{
				Level: pb.AggregationLevel_MODULE,
				Id: &pb.TestIdentifier{
					ModuleName:        "m1",
					ModuleScheme:      "junit",
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
				},
			}

			t.Run("module-level filter", func(t *ftt.Test) {
				expected = FilterVerdicts(expected, func(v *pb.TestVerdict) bool {
					return v.TestIdStructured.ModuleName == "m1"
				})
				assert.Loosely(t, expected, should.HaveLength(6))
				assert.Loosely(t, fetchAll(q, opts), should.Match(expected))
			})
			t.Run("coarse name-level filter", func(t *ftt.Test) {
				q.TestPrefixFilter.Level = pb.AggregationLevel_COARSE
				q.TestPrefixFilter.Id.CoarseName = "c1"
				expected = FilterVerdicts(expected, func(v *pb.TestVerdict) bool {
					return v.TestIdStructured.ModuleName == "m1" && v.TestIdStructured.CoarseName == "c1"
				})
				assert.Loosely(t, expected, should.HaveLength(5))
				assert.Loosely(t, fetchAll(q, opts), should.Match(expected))
			})
			t.Run("fine name-level filter", func(t *ftt.Test) {
				q.TestPrefixFilter.Level = pb.AggregationLevel_FINE
				q.TestPrefixFilter.Id.CoarseName = "c1"
				q.TestPrefixFilter.Id.FineName = "f1"
				expected = FilterVerdicts(expected, func(v *pb.TestVerdict) bool {
					return v.TestIdStructured.ModuleName == "m1" &&
						v.TestIdStructured.CoarseName == "c1" &&
						v.TestIdStructured.FineName == "f1"
				})
				assert.Loosely(t, expected, should.HaveLength(4))
				assert.Loosely(t, fetchAll(q, opts), should.Match(expected))
			})
			t.Run("case name-level filter", func(t *ftt.Test) {
				q.TestPrefixFilter.Level = pb.AggregationLevel_CASE
				q.TestPrefixFilter.Id.CoarseName = "c1"
				q.TestPrefixFilter.Id.FineName = "f1"
				q.TestPrefixFilter.Id.CaseName = "t2"
				expected = FilterVerdicts(expected, func(v *pb.TestVerdict) bool {
					return v.TestIdStructured.ModuleName == "m1" &&
						v.TestIdStructured.CoarseName == "c1" &&
						v.TestIdStructured.FineName == "f1" &&
						v.TestIdStructured.CaseName == "t2"
				})
				assert.Loosely(t, expected, should.HaveLength(1))
				assert.Loosely(t, fetchAll(q, opts), should.Match(expected))
			})
		})
		t.Run("With limited access", func(t *ftt.Test) {
			q.Access.Level = permissions.LimitedAccess

			// ExpectedMaskedVerdicts needs the full verdicts to determine
			// what masking to apply. We can then convert to basic view
			// before comparing.
			expected = ExpectedVerdicts(rootInvID)
			t.Run("Baseline", func(t *ftt.Test) {
				expectedLimited := ToBasicView(ExpectedMaskedVerdicts(expected, nil))
				assert.Loosely(t, fetchAll(q, opts), should.Match(expectedLimited))
			})

			t.Run("With upgraded realms", func(t *ftt.Test) {
				q.Access.Realms = []string{"testproject:t3-r1", "testproject:t4-r1"}
				expectedLimited := ToBasicView(ExpectedMaskedVerdicts(expected, q.Access.Realms))
				assert.Loosely(t, fetchAll(q, opts), should.Match(expectedLimited))
			})
		})
		t.Run("With verdict filter", func(t *ftt.Test) {
			q.EffectiveStatusFilter = []pb.TestVerdictPredicate_VerdictEffectiveStatus{
				pb.TestVerdictPredicate_PRECLUDED,
				pb.TestVerdictPredicate_EXONERATED,
				pb.TestVerdictPredicate_PASSED,
			}
			expected = FilterVerdicts(expected, func(v *pb.TestVerdict) bool {
				return v.TestIdStructured.CaseName == "t1" || // Passed
					v.TestIdStructured.CaseName == "t5" || // Exonerated
					v.TestIdStructured.CaseName == "t6" // Precluded
			})
			assert.Loosely(t, fetchAll(q, opts), should.Match(expected))
		})
	})
}
