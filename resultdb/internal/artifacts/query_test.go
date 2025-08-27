// Copyright 2020 The LUCI Authors.
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

package artifacts

import (
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestQuery(t *testing.T) {
	ftt.Run(`Query`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx, t, insert.Invocation("inv1", pb.Invocation_ACTIVE, nil))
		q := &Query{
			InvocationIDs:       invocations.NewIDSet("inv1"),
			PageSize:            100,
			TestResultPredicate: &pb.TestResultPredicate{},
			WithRBECASHash:      false,
		}

		mustFetch := func(q *Query) (arts []*pb.Artifact, token string) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			arts, tok, err := q.FetchProtos(ctx)
			assert.Loosely(t, err, should.BeNil)
			return arts, tok
		}

		mustFetchNames := func(q *Query) []string {
			arts, _ := mustFetch(q)
			names := make([]string, len(arts))
			for i, a := range arts {
				names[i] = a.Name
			}
			return names
		}

		t.Run(`Populates fields correctly`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", map[string]any{
					"ContentType": "text/plain",
					"Size":        64,
				}),
			)
			actual, _ := mustFetch(q)
			assert.Loosely(t, actual, should.HaveLength(1))
			assert.Loosely(t, actual[0].ContentType, should.Equal("text/plain"))
			assert.Loosely(t, actual[0].SizeBytes, should.Equal(64))
			assert.Loosely(t, actual[0].HasLines, should.BeTrue)
		})

		t.Run(`Reads both invocation and test result artifacts`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t t/r", "a", nil),
			)
			actual := mustFetchNames(q)
			assert.Loosely(t, actual, should.Match([]string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t%20t/results/r/artifacts/a",
			}))
		})

		t.Run(`Does not fetch artifacts of other invocations`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Invocation("inv0", pb.Invocation_ACTIVE, nil),
				insert.Invocation("inv2", pb.Invocation_ACTIVE, nil),
				insert.Artifact("inv0", "", "a", nil),
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv2", "", "a", nil),
			)
			actual := mustFetchNames(q)
			assert.Loosely(t, actual, should.Match([]string{"invocations/inv1/artifacts/a"}))
		})

		t.Run(`Test ID regexp`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t00/r", "a", nil),
				insert.Artifact("inv1", "tr/t10/r", "a", nil),
				insert.Artifact("inv1", "tr/t11/r", "a", nil),
				insert.Artifact("inv1", "tr/t20/r", "a", nil),
			)
			q.TestResultPredicate.TestIdRegexp = "t1."
			actual := mustFetchNames(q)
			assert.Loosely(t, actual, should.Match([]string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t10/results/r/artifacts/a",
				"invocations/inv1/tests/t11/results/r/artifacts/a",
			}))
		})

		t.Run(`Follow edges`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a0", nil),
				insert.Artifact("inv1", "", "a1", nil),
				insert.Artifact("inv1", "tr/t/r", "a0", nil),
				insert.Artifact("inv1", "tr/t/r", "a1", nil),
			)

			t.Run(`Unspecified`, func(t *ftt.Test) {
				actual := mustFetchNames(q)
				assert.Loosely(t, actual, should.Match([]string{
					"invocations/inv1/artifacts/a0",
					"invocations/inv1/artifacts/a1",
					"invocations/inv1/tests/t/results/r/artifacts/a0",
					"invocations/inv1/tests/t/results/r/artifacts/a1",
				}))
			})

			t.Run(`Only invocations`, func(t *ftt.Test) {
				q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{
					IncludedInvocations: true,
				}
				actual := mustFetchNames(q)
				assert.Loosely(t, actual, should.Match([]string{
					"invocations/inv1/artifacts/a0",
					"invocations/inv1/artifacts/a1",
				}))

				t.Run(`Test result predicate is ignored`, func(t *ftt.Test) {
					q.TestResultPredicate.TestIdRegexp = "t."
					actual := mustFetchNames(q)
					assert.Loosely(t, actual, should.Match([]string{
						"invocations/inv1/artifacts/a0",
						"invocations/inv1/artifacts/a1",
					}))
				})
			})

			t.Run(`Only test results`, func(t *ftt.Test) {
				q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{
					TestResults: true,
				}
				actual := mustFetchNames(q)
				assert.Loosely(t, actual, should.Match([]string{
					"invocations/inv1/tests/t/results/r/artifacts/a0",
					"invocations/inv1/tests/t/results/r/artifacts/a1",
				}))
			})

			t.Run(`Both`, func(t *ftt.Test) {
				q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{
					IncludedInvocations: true,
					TestResults:         true,
				}
				actual := mustFetchNames(q)
				assert.Loosely(t, actual, should.Match([]string{
					"invocations/inv1/artifacts/a0",
					"invocations/inv1/artifacts/a1",
					"invocations/inv1/tests/t/results/r/artifacts/a0",
					"invocations/inv1/tests/t/results/r/artifacts/a1",
				}))
			})
		})

		t.Run(`Artifacts of interesting test results`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t0/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/1", "a", nil),
				insert.Artifact("inv1", "tr/t1/1", "b", nil),
				insert.Artifact("inv1", "tr/t2/0", "a", nil),
			)
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.TestResults(t, "inv1", "t0", nil, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "t1", nil, pb.TestResult_PASSED, pb.TestResult_FAILED),
				insert.TestResults(t, "inv1", "t2", nil, pb.TestResult_FAILED),
			)...)

			q.TestResultPredicate.Expectancy = pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS
			actual := mustFetchNames(q)
			assert.Loosely(t, actual, should.Match([]string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t1/results/0/artifacts/a",
				"invocations/inv1/tests/t1/results/1/artifacts/a",
				"invocations/inv1/tests/t1/results/1/artifacts/b",
				"invocations/inv1/tests/t2/results/0/artifacts/a",
			}))

			t.Run(`Without invocation artifacts`, func(t *ftt.Test) {
				q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{TestResults: true}
				actual := mustFetchNames(q)
				assert.Loosely(t, actual, should.NotContain("invocations/inv1/artifacts/a"))
			})
		})

		t.Run(`Artifacts of unexpected test results`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t0/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/1", "a", nil),
				insert.Artifact("inv1", "tr/t1/1", "b", nil),
				insert.Artifact("inv1", "tr/t2/0", "a", nil),
			)
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.TestResults(t, "inv1", "t0", nil, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "t1", nil, pb.TestResult_PASSED, pb.TestResult_FAILED),
				insert.TestResults(t, "inv1", "t2", nil, pb.TestResult_FAILED),
			)...)

			q.TestResultPredicate.Expectancy = pb.TestResultPredicate_VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS
			actual := mustFetchNames(q)
			assert.Loosely(t, actual, should.Match([]string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t2/results/0/artifacts/a",
			}))
		})

		t.Run(`Variant equals`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t0/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/0", "b", nil),
				insert.Artifact("inv1", "tr/t2/0", "a", nil),
			)
			v1 := pbutil.Variant("k", "1")
			v2 := pbutil.Variant("k", "2")
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.TestResults(t, "inv1", "t0", v1, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "t1", v2, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "t2", v1, pb.TestResult_PASSED),
			)...)

			q.TestResultPredicate.Variant = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Equals{Equals: v2},
			}
			assert.Loosely(t, mustFetchNames(q), should.Match([]string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t1/results/0/artifacts/a",
				"invocations/inv1/tests/t1/results/0/artifacts/b",
			}))

			t.Run(`Without invocation artifacts`, func(t *ftt.Test) {
				q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{TestResults: true}
				actual := mustFetchNames(q)
				assert.Loosely(t, actual, should.NotContain("invocations/inv1/artifacts/a"))
			})
		})

		t.Run(`Variant contains`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t0/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/0", "b", nil),
				insert.Artifact("inv1", "tr/t2/0", "a", nil),
			)
			v00 := pbutil.Variant("k0", "0")
			v01 := pbutil.Variant("k0", "0", "k1", "1")
			v10 := pbutil.Variant("k0", "1")
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.TestResults(t, "inv1", "t0", v00, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "t1", v01, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "t2", v10, pb.TestResult_PASSED),
			)...)

			t.Run(`Empty`, func(t *ftt.Test) {
				q.TestResultPredicate.Variant = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: pbutil.Variant()},
				}
				assert.Loosely(t, mustFetchNames(q), should.Match([]string{
					"invocations/inv1/artifacts/a",
					"invocations/inv1/tests/t0/results/0/artifacts/a",
					"invocations/inv1/tests/t1/results/0/artifacts/a",
					"invocations/inv1/tests/t1/results/0/artifacts/b",
					"invocations/inv1/tests/t2/results/0/artifacts/a",
				}))

				t.Run(`Without invocation artifacts`, func(t *ftt.Test) {
					q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{TestResults: true}
					actual := mustFetchNames(q)
					assert.Loosely(t, actual, should.NotContain("invocations/inv1/artifacts/a"))
				})
			})

			t.Run(`Non-empty`, func(t *ftt.Test) {
				q.TestResultPredicate.Variant = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: v00},
				}
				assert.Loosely(t, mustFetchNames(q), should.Match([]string{
					"invocations/inv1/artifacts/a",
					"invocations/inv1/tests/t0/results/0/artifacts/a",
					"invocations/inv1/tests/t1/results/0/artifacts/a",
					"invocations/inv1/tests/t1/results/0/artifacts/b",
				}))

				t.Run(`Without invocation artifacts`, func(t *ftt.Test) {
					q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{TestResults: true}
					actual := mustFetchNames(q)
					assert.Loosely(t, actual, should.NotContain("invocations/inv1/artifacts/a"))
				})
			})
		})

		t.Run(`Paging`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a0", nil),
				insert.Artifact("inv1", "", "a1", nil),
				insert.Artifact("inv1", "", "a2", nil),
				insert.Artifact("inv1", "", "a3", nil),
				insert.Artifact("inv1", "", "a4", nil),
			)

			mustReadPage := func(pageToken string, pageSize int, expectedArtifactIDs ...string) string {
				q2 := *q
				q2.PageToken = pageToken
				q2.PageSize = pageSize
				arts, token := mustFetch(&q2)

				actualArtifactIDs := make([]string, len(arts))
				for i, a := range arts {
					actualArtifactIDs[i] = a.ArtifactId
				}
				assert.Loosely(t, actualArtifactIDs, should.Match(expectedArtifactIDs))
				return token
			}

			t.Run(`All results`, func(t *ftt.Test) {
				token := mustReadPage("", 10, "a0", "a1", "a2", "a3", "a4")
				assert.Loosely(t, token, should.BeEmpty)
			})

			t.Run(`With pagination`, func(t *ftt.Test) {
				token := mustReadPage("", 1, "a0")
				assert.Loosely(t, token, should.NotEqual(""))

				token = mustReadPage(token, 2, "a1", "a2")
				assert.Loosely(t, token, should.NotEqual(""))

				token = mustReadPage(token, 3, "a3", "a4")
				assert.Loosely(t, token, should.BeEmpty)
			})

			t.Run(`Bad token`, func(t *ftt.Test) {
				q.PageToken = "CgVoZWxsbw=="
				_, _, err := q.FetchProtos(span.Single(ctx))
				as, ok := appstatus.Get(err)
				assert.That(t, ok, should.BeTrue)
				assert.That(t, as.Code(), should.Equal(codes.InvalidArgument))
				assert.That(t, as.Message(), should.ContainSubstring("invalid page_token"))
			})
		})

		t.Run(`ContentTypes`, func(t *ftt.Test) {
			t.Run(`Works`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t,
					insert.Artifact("inv1", "", "a0", map[string]any{"ContentType": "text/plain; encoding=utf-8"}),
					insert.Artifact("inv1", "tr/t/r", "a0", map[string]any{"ContentType": "text/plain"}),
					insert.Artifact("inv1", "tr/t/r", "a1", nil),
					insert.Artifact("inv1", "tr/t/r", "a3", map[string]any{"ContentType": "image/jpg"}),
				)
				q.ContentTypeRegexp = "text/.+"

				actual := mustFetchNames(q)
				assert.Loosely(t, actual, should.Match([]string{
					"invocations/inv1/artifacts/a0",
					"invocations/inv1/tests/t/results/r/artifacts/a0",
				}))
			})

			t.Run(`Filter generated conditionally`, func(t *ftt.Test) {
				q.ContentTypeRegexp = ""
				st, err := q.genStmt(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, st.SQL, should.NotContainSubstring("@contentTypeRegexp"))
			})
		})

		t.Run(`ArtifactIds`, func(t *ftt.Test) {
			t.Run(`Works`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t,
					insert.Artifact("inv1", "", "a0", nil),
					insert.Artifact("inv1", "tr/t/r", "a0", nil),
					insert.Artifact("inv1", "tr/t/r", "a1", nil),
					insert.Artifact("inv1", "tr/t/r", "a3", nil),
				)
				q.ArtifactIDRegexp = "a0"

				actual := mustFetchNames(q)
				assert.Loosely(t, actual, should.Match([]string{
					"invocations/inv1/artifacts/a0",
					"invocations/inv1/tests/t/results/r/artifacts/a0",
				}))
			})

			t.Run(`Filter generated conditionally`, func(t *ftt.Test) {
				q.ArtifactIDRegexp = ""
				st, err := q.genStmt(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, st.SQL, should.NotContainSubstring("@artifactIdRegexp"))
			})
		})

		t.Run(`WithRBECASHash`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "tr/t/r", "a", map[string]any{
					"ContentType": "text/plain",
					"Size":        64,
					"RBECASHash":  "deadbeef",
				}),
			)

			q.WithRBECASHash = true
			q.PageSize = 0
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			var actual []*Artifact
			err := q.Run(ctx, func(a *Artifact) error {
				actual = append(actual, a)
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.HaveLength(1))
			assert.Loosely(t, actual[0].RBECASHash, should.Equal("deadbeef"))
		})

		t.Run(`WithGcsURI`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "tr/t/r", "a", map[string]any{
					"ContentType": "text/plain",
					"Size":        64,
					"GcsURI":      "gs://bucket/beyondbeef",
				}),
			)

			q.WithGcsURI = true
			q.PageSize = 0
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			var actual []*Artifact
			err := q.Run(ctx, func(a *Artifact) error {
				actual = append(actual, a)
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.HaveLength(1))
			assert.Loosely(t, actual[0].GcsUri, should.Equal("gs://bucket/beyondbeef"))
		})

		t.Run(`Without WithGcsURI`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "tr/t/r", "a", map[string]any{
					"ContentType": "text/plain",
					"Size":        64,
					"GcsURI":      "gs://bucket/beyondbeef",
				}),
			)

			q.WithGcsURI = false
			q.PageSize = 0
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			var actual []*Artifact
			err := q.Run(ctx, func(a *Artifact) error {
				actual = append(actual, a)
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.HaveLength(1))
			assert.Loosely(t, actual[0].GcsUri, should.BeEmpty)
		})

		t.Run(`WithRbeURI`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "tr/t/r", "a", map[string]any{
					"ContentType": "text/plain",
					"Size":        64,
					"RBEURI":      "rbe://cas/default/blobs/deadbeef/128",
				}),
			)

			q.WithRbeURI = true
			q.PageSize = 0
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			var actual []*Artifact
			err := q.Run(ctx, func(a *Artifact) error {
				actual = append(actual, a)
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.HaveLength(1))
			assert.Loosely(t, actual[0].RbeUri, should.Equal("rbe://cas/default/blobs/deadbeef/128"))
		})

		t.Run(`Without WithRbeURI`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "tr/t/r", "a", map[string]any{
					"ContentType": "text/plain",
					"Size":        64,
					"RBEURI":      "rbe://cas/default/blobs/deadbeef/128",
				}),
			)

			q.WithRbeURI = false
			q.PageSize = 0
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			var actual []*Artifact
			err := q.Run(ctx, func(a *Artifact) error {
				actual = append(actual, a)
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.HaveLength(1))
			assert.Loosely(t, actual[0].RbeUri, should.BeEmpty)
		})

		t.Run(`Populates HasLines field correctly`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Artifact("inv1", "", "a", map[string]any{
					"ContentType": "image/jpeg",
					"Size":        64,
				}),
			)
			actual, _ := mustFetch(q)
			assert.Loosely(t, actual, should.HaveLength(1))
			assert.Loosely(t, actual[0].ContentType, should.Equal("image/jpeg"))
			assert.Loosely(t, actual[0].SizeBytes, should.Equal(64))
			assert.Loosely(t, actual[0].HasLines, should.BeFalse)
		})
	})
}
