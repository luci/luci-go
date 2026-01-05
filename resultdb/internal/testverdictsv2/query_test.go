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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
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
			Order:            OrderingByID,
		}

		fetchAll := func(q *Query) []*pb.TestVerdict {
			expectedPageSize := q.PageSize
			var results []*pb.TestVerdict
			var token string
			for {
				verdicts, nextToken, err := q.Fetch(span.Single(ctx), token)
				assert.Loosely(t, err, should.BeNil, truth.LineContext())
				results = append(results, verdicts...)
				if nextToken == "" {
					assert.Loosely(t, len(verdicts), should.BeLessThanOrEqual(expectedPageSize))
					break
				} else {
					assert.Loosely(t, len(verdicts), should.Equal(expectedPageSize))
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

			// Map expected items to map for easy retrieval.
			m := make(map[string]*pb.TestVerdict)
			for _, v := range expected {
				m[v.TestId] = v
			}

			// Note: Within same priority, sort by TestID (module, scheme, variant, coarse, fine, case).
			expectedUIOrder := []*pb.TestVerdict{
				m[flatTestID(rootInvID, "t2")], // Failed (priority 100)
				m[flatTestID(rootInvID, "t6")], // Precluded (priority 70)
				m[flatTestID(rootInvID, "t7")], // Execution Errored (priority 70)
				m[flatTestID(rootInvID, "t3")], // Flaky (priority 30)
				m[flatTestID(rootInvID, "t5")], // Exonerated (priority 10)
				m[flatTestID(rootInvID, "t1")], // Passed (priority 0)
				m[flatTestID(rootInvID, "t4")], // Skipped (priority 0)
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
	})
}
