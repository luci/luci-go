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

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

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
		rootInvID := rootinvocations.ID("root-inv")
		rootInvRow := rootinvocations.NewBuilder(rootInvID).Build()
		ms := insert.RootInvocationWithRootWorkUnit(rootInvRow)
		ms = append(ms, CreateTestData(rootInvID)...)
		testutil.MustApply(ctx, t, ms...)

		q := &Query{
			RootInvocationID: rootInvID,
			PageSize:         100,
			ResultLimit:      10,
			Order:            OrderingByID,
			Access: permissions.RootInvocationAccess{
				Level: permissions.FullAccess,
			},
		}

		fetchAll := func(q *Query) []*pb.TestVerdict {
			var results []*pb.TestVerdict
			var token string
			for {
				verdicts, nextToken, err := q.Fetch(span.Single(ctx), token)
				assert.Loosely(t, err, should.BeNil, truth.LineContext())
				results = append(results, verdicts...)
				if nextToken == "" {
					assert.Loosely(t, len(verdicts), should.BeLessThanOrEqual(q.PageSize))
					break
				} else {
					if q.ResponseLimitBytes == 0 {
						// If there were no secondary constraints on the page size,
						// verify the page is exactly q.PageSize, not simply less than or equal.
						assert.Loosely(t, len(verdicts), should.Equal(q.PageSize))
					} else {
						// If there was a secondary constraint, there should be between 1
						// and q.PageSize results.
						assert.Loosely(t, len(verdicts), should.BeGreaterThan(0))
						assert.Loosely(t, len(verdicts), should.BeLessThanOrEqual(q.PageSize))
					}
				}
				token = nextToken
			}
			return results
		}

		expected := ExpectedVerdicts(rootInvID)

		t.Run("Baseline", func(t *ftt.Test) {
			t.Run("Without pagination", func(t *ftt.Test) {
				verdicts, token, err := q.Fetch(span.Single(ctx), "")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, token, should.Equal(""))
				assert.Loosely(t, verdicts, should.Match(expected))
			})
			t.Run("With pagination", func(t *ftt.Test) {
				q.PageSize = 1
				results := fetchAll(q)
				assert.Loosely(t, results, should.Match(expected))
			})
		})
		t.Run("With empty invocation", func(t *ftt.Test) {
			emptyInvID := rootinvocations.ID("empty-inv")
			emptyInvRow := rootinvocations.NewBuilder(emptyInvID).Build()
			ms := insert.RootInvocationWithRootWorkUnit(emptyInvRow)
			testutil.MustApply(ctx, t, ms...)
			q.RootInvocationID = emptyInvID

			verdicts, token, err := q.Fetch(span.Single(ctx), "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, token, should.Equal(""))
			assert.Loosely(t, verdicts, should.HaveLength(0))
		})
		t.Run("With invalid page token", func(t *ftt.Test) {
			_, _, err := q.Fetch(span.Single(ctx), "invalid-page-token")
			st, ok := appstatus.Get(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, st.Err(), should.ErrLike("page_token: invalid page token"))
			assert.Loosely(t, st.Code(), should.Equal(codes.InvalidArgument))
		})
		t.Run("Ordering by UI Priority", func(t *ftt.Test) {
			q.Order = OrderingByUIPriority
			// Expected order:
			// 1. Failed (t2) - Priority 100
			// 2. Execution Errored (t7) - Priority 70
			// 3. Precluded (t6) - Priority 70
			// 4. Flaky (t3) - Priority 30
			// 5. Exonerated (t5) - Priority 10
			// 6. Passed (t1) - Priority 0
			// 7. Skipped (t4) - Priority 0
			// Note: Within same priority, sort by TestID (module, scheme, variant, coarse, fine, case).

			// Map expected items to map for easy retrieval
			m := make(map[string]*pb.TestVerdict)
			for _, v := range expected {
				m[v.TestId] = v
			}

			expectedUIOrder := []*pb.TestVerdict{
				m[flatTestID("m1", "c1", "f1", "t2")], // Failed
				m[flatTestID("m1", "c2", "f1", "t6")], // Precluded
				m[flatTestID("m2", "c1", "f1", "t7")], // Execution Errored
				m[flatTestID("m1", "c1", "f1", "t3")], // Flaky
				m[flatTestID("m1", "c1", "f2", "t5")], // Exonerated
				m[flatTestID("m1", "c1", "f1", "t1")], // Passed
				m[flatTestID("m1", "c1", "f1", "t4")], // Skipped
			}

			t.Run("Without pagination", func(t *ftt.Test) {
				verdicts, token, err := q.Fetch(span.Single(ctx), "")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, token, should.Equal(""))
				assert.Loosely(t, verdicts, should.Match(expectedUIOrder))
			})
			t.Run("With pagination", func(t *ftt.Test) {
				q.PageSize = 1
				results := fetchAll(q)
				assert.Loosely(t, results, should.Match(expectedUIOrder))
			})
		})
		t.Run("With result limit", func(t *ftt.Test) {
			// Because test status is computed at the database side, reducing the
			// number of results should not result in changes to test status (e.g.
			// from flaky to failed).
			q.ResultLimit = 1

			expected := ExpectedVerdicts(rootInvID)
			for _, tv := range expected {
				tv.Results = tv.Results[:1]
				if len(tv.Exonerations) > 0 {
					tv.Exonerations = tv.Exonerations[:1]
				}
			}

			results := fetchAll(q)
			assert.Loosely(t, results, should.Match(expected))
		})
		t.Run("With response limit bytes", func(t *ftt.Test) {
			t.Run("Makes progress", func(t *ftt.Test) {
				// While results may be split over multiple pages, they should all be
				// returned.
				q.ResponseLimitBytes = 1
				results := fetchAll(q)
				assert.Loosely(t, results, should.Match(expected))
			})
			t.Run("Minimum limit", func(t *ftt.Test) {
				// Expect only one row to be returned, because the limit is very low.
				// One row is the minimum that must be returned to make progress.
				q.ResponseLimitBytes = 1
				verdicts, token, err := q.Fetch(span.Single(ctx), "")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, token, should.NotBeEmpty)
				assert.Loosely(t, verdicts, should.Match(expected[:1]))
			})
			t.Run("Limit is applied correctly", func(t *ftt.Test) {
				// Should return three rows, as the limit fits more than
				// the first two rows and the limit is soft.
				q.ResponseLimitBytes = (proto.Size(expected[0]) + 1000) + (proto.Size(expected[1]) + 1000) + 1

				verdicts, token, err := q.Fetch(span.Single(ctx), "")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, token, should.NotBeEmpty)
				assert.Loosely(t, verdicts, should.Match(expected[:3]))
			})
		})
		t.Run("With contains test result filter", func(t *ftt.Test) {
			expected := ExpectedVerdicts(rootInvID)

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
				assert.Loosely(t, fetchAll(q), should.Match(expected))
			})
			t.Run("With limited access (some upgraded to full)", func(t *ftt.Test) {
				// Only some results should match, because we filter on
				// tags and variant and these fields are only visible to us on those
				// results we have full access to.
				q.Access.Level = permissions.LimitedAccess
				q.Access.Realms = []string{"testproject:t4-r1"}
				expectedLimited := ExpectedVerdictsMasked(rootInvID, q.Access.Realms)
				expectedLimited = expectedLimited[3:4]
				assert.Loosely(t, fetchAll(q), should.Match(expectedLimited))
			})
			t.Run("With implicit filter", func(t *ftt.Test) {
				// Check an aip.dev/160 implicit filter.
				q.ContainsTestResultFilter = `t2`
				expected = expected[1:2]
				assert.Loosely(t, fetchAll(q), should.Match(expected))
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

			expected := ExpectedVerdicts(rootInvID)
			t.Run("module-level filter", func(t *ftt.Test) {
				expected = expected[0:6]
				assert.Loosely(t, fetchAll(q), should.Match(expected))
			})
			t.Run("coarse name-level filter", func(t *ftt.Test) {
				q.TestPrefixFilter.Level = pb.AggregationLevel_COARSE
				q.TestPrefixFilter.Id.CoarseName = "c1"
				expected = expected[0:5]
				assert.Loosely(t, fetchAll(q), should.Match(expected))
			})
			t.Run("fine name-level filter", func(t *ftt.Test) {
				q.TestPrefixFilter.Level = pb.AggregationLevel_FINE
				q.TestPrefixFilter.Id.CoarseName = "c1"
				q.TestPrefixFilter.Id.FineName = "f1"
				expected = expected[0:4]
				assert.Loosely(t, fetchAll(q), should.Match(expected))
			})
			t.Run("case name-level filter", func(t *ftt.Test) {
				q.TestPrefixFilter.Level = pb.AggregationLevel_CASE
				q.TestPrefixFilter.Id.CoarseName = "c1"
				q.TestPrefixFilter.Id.FineName = "f1"
				q.TestPrefixFilter.Id.CaseName = "t2"
				expected = expected[1:2]
				assert.Loosely(t, fetchAll(q), should.Match(expected))
			})
		})
		t.Run("With limited access", func(t *ftt.Test) {
			q.Access.Level = permissions.LimitedAccess

			t.Run("Baseline", func(t *ftt.Test) {
				expectedLimited := ExpectedVerdictsMasked(rootInvID, nil)
				assert.Loosely(t, fetchAll(q), should.Match(expectedLimited))
			})

			t.Run("With upgraded realms", func(t *ftt.Test) {
				q.Access.Realms = []string{"testproject:t3-r1", "testproject:t4-r1"}
				expectedLimited := ExpectedVerdictsMasked(rootInvID, q.Access.Realms)
				assert.Loosely(t, fetchAll(q), should.Match(expectedLimited))
			})
		})
		t.Run("With verdict filter", func(t *ftt.Test) {
			t.Run("status", func(t *ftt.Test) {
				q.Filter = "status = FAILED"
				expected := ExpectedVerdicts(rootInvID)
				// t2 is FAILED and t5 is FAILED (but exonerated).
				expected = []*pb.TestVerdict{expected[1], expected[4]}
				assert.Loosely(t, fetchAll(q), should.Match(expected))
			})
			t.Run("status_override", func(t *ftt.Test) {
				q.Filter = "status_override = EXONERATED"
				expected := ExpectedVerdicts(rootInvID)
				// Only t5 is EXONERATED.
				expected = expected[4:5]
				assert.Loosely(t, fetchAll(q), should.Match(expected))
			})
		})
	})
}
