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
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestQueryDetails(t *testing.T) {
	ftt.Run("QueryDetails", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("root-inv")
		rootInvRow := rootinvocations.NewBuilder(rootInvID).Build()
		ms := insert.RootInvocationWithRootWorkUnit(rootInvRow)
		ms = append(ms, CreateTestData(rootInvID)...)
		testutil.MustApply(ctx, t, ms...)

		q := &QueryDetails{
			RootInvocationID: rootInvID,
			Access: permissions.RootInvocationAccess{
				Level: permissions.FullAccess,
			},
		}

		fetchAll := func(q *QueryDetails, opts FetchOptions) []*pb.TestVerdict {
			var results []*pb.TestVerdict
			var token PageToken
			for {
				ctx, cancel := span.ReadOnlyTransaction(ctx)
				verdicts, nextToken, err := q.Fetch(ctx, token, opts)
				cancel()
				assert.Loosely(t, err, should.BeNil, truth.LineContext(1))
				results = append(results, verdicts...)
				if nextToken == (PageToken{}) {
					// This is the last page.
					assert.Loosely(t, len(verdicts), should.BeLessThanOrEqual(opts.PageSize))
					break
				} else {
					if opts.ResponseLimitBytes == 0 {
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

		expected := ExpectedVerdicts(rootInvID)
		fetchOptions := FetchOptions{
			PageSize:           100,
			VerdictResultLimit: 10,
			VerdictSizeLimit:   1024 * 1024,
		}

		t.Run("Baseline", func(t *ftt.Test) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			verdicts, token, err := q.Fetch(ctx, PageToken{}, fetchOptions)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, token, should.Match(PageToken{}))
			assert.Loosely(t, verdicts, should.Match(expected))
		})

		t.Run("With page size limit", func(t *ftt.Test) {
			fetchOptions.PageSize = 1
			results := fetchAll(q, fetchOptions)
			assert.Loosely(t, results, should.Match(expected))
		})

		t.Run("With response limit bytes", func(t *ftt.Test) {
			t.Run("Makes progress", func(t *ftt.Test) {
				limit := 0
				for _, v := range expected {
					// 1000 bytes to match the JSON-friendly size estimate used in the implementation.
					size := proto.Size(v) + 1000
					if size > limit {
						limit = size
					}
				}
				fetchOptions.ResponseLimitBytes = limit

				// While results may be split over multiple pages, they should all be
				// returned.
				results := fetchAll(q, fetchOptions)
				assert.Loosely(t, results, should.Match(expected))
			})
			t.Run("Limit is applied correctly", func(t *ftt.Test) {
				// Should return two rows, as the limit fits only
				// the first two rows and the limit is hard.
				fetchOptions.ResponseLimitBytes = (proto.Size(expected[0]) + 1000) + (proto.Size(expected[1]) + 1000) + 1

				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				verdicts, token, err := q.Fetch(ctx, PageToken{}, fetchOptions)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, token, should.NotMatch(PageToken{}))
				assert.Loosely(t, verdicts, should.Match(expected[:2]))
			})
		})

		t.Run("With verdict result limit", func(t *ftt.Test) {
			// t3 has 2 results.
			// t5 has 2 exonerations.
			// Limit to 1 result/exoneration.
			fetchOptions.VerdictResultLimit = 1
			results := fetchAll(q, fetchOptions)
			assert.Loosely(t, results, should.HaveLength(len(expected)))

			// Check t3 (Flaky)
			t3 := VerdictByCaseName(results, "t3")
			assert.Loosely(t, t3.Results, should.HaveLength(1))

			// Check t5 (Exonerated)
			t5 := VerdictByCaseName(results, "t5")
			assert.Loosely(t, t5.Exonerations, should.HaveLength(1))
		})

		t.Run("With verdict size limit", func(t *ftt.Test) {
			// Remove one result from t3 and measure its size. This will be our target.
			assert.Loosely(t, expected[1].Results, should.HaveLength(2))
			expected[1].Results = expected[1].Results[:1]
			fetchOptions.VerdictSizeLimit = proto.Size(expected[1]) + protoJSONOverheadBytes

			// As the implementation is conservative, give it a little bit of extra room.
			fetchOptions.VerdictSizeLimit += 2

			results := fetchAll(q, fetchOptions)
			assert.Loosely(t, results, should.HaveLength(len(expected)))
			assert.Loosely(t, results[1], should.Match(expected[1]))
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
				var moduleVerdicts []*pb.TestVerdict
				for _, v := range expected {
					if v.TestIdStructured.ModuleName == "m1" {
						moduleVerdicts = append(moduleVerdicts, v)
					}
				}
				assert.Loosely(t, fetchAll(q, fetchOptions), should.Match(moduleVerdicts))
			})
			t.Run("coarse name-level filter", func(t *ftt.Test) {
				var coarseVerdicts []*pb.TestVerdict
				for _, v := range expected {
					if v.TestIdStructured.ModuleName == "m1" && v.TestIdStructured.CoarseName == "c1" {
						coarseVerdicts = append(coarseVerdicts, v)
					}
				}
				q.TestPrefixFilter.Level = pb.AggregationLevel_COARSE
				q.TestPrefixFilter.Id.CoarseName = "c1"
				assert.Loosely(t, fetchAll(q, fetchOptions), should.Match(coarseVerdicts))
			})
		})

		t.Run("With nominated IDs", func(t *ftt.Test) {
			verdictID := func(v *pb.TestVerdict) testresultsv2.VerdictID {
				return testresultsv2.VerdictID{
					RootInvocationShardID: rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: 0},
					ModuleName:            v.TestIdStructured.ModuleName,
					ModuleScheme:          v.TestIdStructured.ModuleScheme,
					ModuleVariantHash:     v.TestIdStructured.ModuleVariantHash,
					CoarseName:            v.TestIdStructured.CoarseName,
					FineName:              v.TestIdStructured.FineName,
					CaseName:              v.TestIdStructured.CaseName,
				}
			}

			nominated := []testresultsv2.VerdictID{
				verdictID(expected[2]),
				verdictID(expected[0]),
				{
					// Does not exist.
					RootInvocationShardID: rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: 10},
					ModuleName:            "non_existant_module",
					ModuleVariantHash:     "1234567890abcdef",
					ModuleScheme:          "junit",
					CaseName:              "MyTest",
				},
				verdictID(expected[2]),
				verdictID(expected[2]),
			}
			q.VerdictIDs = nominated

			// Note: verdicts must be return in the order they are requested, including duplicates.
			// Missing verdicts are indicating with nils.
			expectedSubset := []*pb.TestVerdict{
				expected[2],
				expected[0],
				nil,
				expected[2],
				expected[2],
			}

			t.Run("Baseline", func(t *ftt.Test) {
				assert.Loosely(t, fetchAll(q, fetchOptions), should.Match(expectedSubset))
			})
			t.Run("With small page size", func(t *ftt.Test) {
				fetchOptions.PageSize = 1
				assert.Loosely(t, fetchAll(q, fetchOptions), should.Match(expectedSubset))
			})
		})

		t.Run("With limited access", func(t *ftt.Test) {
			q.Access.Level = permissions.LimitedAccess
			t.Run("Baseline", func(t *ftt.Test) {
				expectedLimited := ExpectedMaskedVerdicts(expected, nil)
				assert.Loosely(t, fetchAll(q, fetchOptions), should.Match(expectedLimited))
			})

			t.Run("With upgraded realms", func(t *ftt.Test) {
				q.Access.Realms = []string{"testproject:t3-r1", "testproject:t4-r1"}
				expectedLimited := ExpectedMaskedVerdicts(expected, q.Access.Realms)
				assert.Loosely(t, fetchAll(q, fetchOptions), should.Match(expectedLimited))
			})
		})

		t.Run("Errors", func(t *ftt.Test) {
			t.Run("Invalid page size", func(t *ftt.Test) {
				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				fetchOptions.PageSize = 0
				_, _, err := q.Fetch(ctx, PageToken{}, fetchOptions)
				assert.Loosely(t, err, should.ErrLike("page size must be positive"))
			})
			t.Run("Invalid response limit bytes", func(t *ftt.Test) {
				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				fetchOptions.ResponseLimitBytes = -1
				_, _, err := q.Fetch(ctx, PageToken{}, fetchOptions)
				assert.Loosely(t, err, should.ErrLike("response limit bytes must be positive"))
			})
			t.Run("Invalid verdict result limit", func(t *ftt.Test) {
				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				fetchOptions.VerdictResultLimit = 0
				_, _, err := q.Fetch(ctx, PageToken{}, fetchOptions)
				assert.Loosely(t, err, should.ErrLike("verdict result limit must be positive"))
			})
			t.Run("Invalid verdict size limit", func(t *ftt.Test) {
				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				fetchOptions.VerdictSizeLimit = 0
				_, _, err := q.Fetch(ctx, PageToken{}, fetchOptions)
				assert.Loosely(t, err, should.ErrLike("verdict size limit must be positive"))
			})
		})
	})
}
