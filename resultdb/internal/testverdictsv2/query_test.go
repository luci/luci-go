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
			Order:            Ordering{},
			Access: permissions.RootInvocationAccess{
				Level: permissions.FullAccess,
			},
			View: pb.TestVerdictView_TEST_VERDICT_VIEW_FULL,
		}
		opts := FetchOptions{
			PageSize:           100,
			ResponseLimitBytes: 0, // No limit.
			VerdictResultLimit: StandardVerdictResultLimit,
			VerdictSizeLimit:   StandardVerdictSizeLimit,
			TotalResultLimit:   10_000,
		}

		fetchOne := func(q *Query, token PageToken, opts FetchOptions) (verdicts []*pb.TestVerdict, nextToken PageToken, err error) {
			txn, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			verdicts, nextToken, err = q.Fetch(txn, token, opts)
			return
		}

		fetchAll := func(q *Query, opts FetchOptions) []*pb.TestVerdict {
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

		type QueryType struct {
			Name     string
			View     pb.TestVerdictView
			Expected []*pb.TestVerdict
		}
		queries := []QueryType{
			{
				Name:     "Summary query",
				View:     pb.TestVerdictView_TEST_VERDICT_VIEW_BASIC,
				Expected: ToBasicView(ExpectedVerdicts(rootInvID)),
			},
			{
				Name:     "Summary -> Details query",
				View:     pb.TestVerdictView_TEST_VERDICT_VIEW_FULL,
				Expected: ExpectedVerdicts(rootInvID),
			},
		}
		for _, query := range queries {
			t.Run(query.Name, func(t *ftt.Test) {
				expected := query.Expected
				q.View = query.View

				t.Run("Baseline", func(t *ftt.Test) {
					assert.Loosely(t, fetchAll(q, opts), should.Match(expected))
				})
				// The underlying queries already have extensive test cases to test correctness
				// of individual options. The tests below are to verify these options are
				// correctly passed to the underlying queries and responses integrated correctly.
				t.Run("Order", func(t *ftt.Test) {
					q.Order = Ordering{
						ByUIPriority: true,
					}
					gotVerdicts := fetchAll(q, opts)
					// The first verdict should be failing.
					assert.Loosely(t, gotVerdicts[0].Status, should.Equal(pb.TestVerdict_FAILED))
					assert.Loosely(t, gotVerdicts[0].StatusOverride, should.Equal(pb.TestVerdict_NOT_OVERRIDDEN))
				})
				t.Run("Filter", func(t *ftt.Test) {
					q.Filter = "status = FAILED"
					// t2 is FAILED and t5 is FAILED (but exonerated).
					expected = []*pb.TestVerdict{
						VerdictByCaseName(expected, "t2"),
						VerdictByCaseName(expected, "t5"),
					}
					assert.Loosely(t, fetchAll(q, opts), should.Match(expected))
				})
				t.Run("Contains test result filter", func(t *ftt.Test) {
					q.ContainsTestResultFilter = "t3"
					gotVerdicts := fetchAll(q, opts)
					expected = []*pb.TestVerdict{
						VerdictByCaseName(expected, "t3"),
					}
					assert.Loosely(t, gotVerdicts, should.Match(expected))
				})
				t.Run("Test prefix filter", func(t *ftt.Test) {
					q.TestPrefixFilter = &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_CASE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m1",
							ModuleScheme:      "junit",
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
							CoarseName:        "c1",
							FineName:          "f1",
							CaseName:          "t3",
						},
					}
					gotVerdicts := fetchAll(q, opts)
					expected = []*pb.TestVerdict{
						VerdictByCaseName(expected, "t3"),
					}
					assert.Loosely(t, gotVerdicts, should.Match(expected))
				})
				t.Run("Access", func(t *ftt.Test) {
					q.Access.Level = permissions.LimitedAccess
					q.Access.Realms = []string{}
					gotVerdicts := fetchAll(q, opts)
					expected = ExpectedMaskedVerdicts(expected, []string{})
					assert.Loosely(t, gotVerdicts, should.Match(expected))
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
				if q.View == pb.TestVerdictView_TEST_VERDICT_VIEW_FULL {
					// For queries using view = FULL which use the IteratedQuery for all or part,
					// verify verdict and total result limits are respected.
					t.Run("With verdict result limit", func(t *ftt.Test) {
						opts.VerdictResultLimit = 1
						for _, e := range expected {
							e.Results = e.Results[:1]
							if len(e.Exonerations) > 1 {
								e.Exonerations = e.Exonerations[:1]
							}
						}

						results := fetchAll(q, opts)
						assert.Loosely(t, results, should.Match(expected))
					})
					t.Run("With verdict size limit", func(t *ftt.Test) {
						// Remove one result from t3 and measure its size. This will be our target.
						assert.Loosely(t, expected[1].Results, should.HaveLength(2))
						expected[1].Results = expected[1].Results[:1]
						opts.VerdictSizeLimit = proto.Size(expected[1]) + protoJSONOverheadBytes

						// As the implementation is conservative, give it a little bit of extra room.
						opts.VerdictSizeLimit += 2

						results := fetchAll(q, opts)
						assert.Loosely(t, results, should.HaveLength(len(expected)))
						assert.Loosely(t, results[1], should.Match(expected[1]))
					})
					t.Run("With total result limit", func(t *ftt.Test) {
						t.Run("Makes progress", func(t *ftt.Test) {
							// Should return all verdicts, just split over more pages.
							// The result limit may be violated for the first verdict on each page
							// to make sure the query always makes progress.
							opts.TotalResultLimit = 1
							results := fetchAll(q, opts)
							assert.Loosely(t, results, should.Match(expected))
						})
						t.Run("Limit is applied correctly", func(t *ftt.Test) {
							// Should return t1 (1 result) and t2 (1 result).
							// t3 (2 results) would make total 4 results, plus 1 for
							// end of verdict detection and 5 > 4.
							opts.TotalResultLimit = 4
							verdicts, token, err := fetchOne(q, PageToken{}, opts)
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, token, should.NotEqual(PageToken{}))
							assert.Loosely(t, verdicts, should.Match(expected[:2]))
						})
						t.Run("Limit is applied correctly (case 2)", func(t *ftt.Test) {
							// Should return t1 (1), t2 (1), t3 (2).
							// Total results = 4, plus one overhead for end of verdict
							// detection which makes 5.
							opts.TotalResultLimit = 5
							verdicts, token, err := fetchOne(q, PageToken{}, opts)
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, token, should.NotEqual(PageToken{}))
							assert.Loosely(t, verdicts, should.Match(expected[:3]))
						})
					})
				}
			})
		}
	})
}
