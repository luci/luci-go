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

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQuery(t *testing.T) {
	Convey(`Query`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx, insert.Invocation("inv1", pb.Invocation_ACTIVE, nil))
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
			So(err, ShouldBeNil)
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

		Convey(`Populates fields correctly`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a", map[string]any{
					"ContentType": "text/plain",
					"Size":        64,
				}),
			)
			actual, _ := mustFetch(q)
			So(actual, ShouldHaveLength, 1)
			So(actual[0].ContentType, ShouldEqual, "text/plain")
			So(actual[0].SizeBytes, ShouldEqual, 64)
			So(actual[0].HasLines, ShouldBeTrue)
		})

		Convey(`Reads both invocation and test result artifacts`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t t/r", "a", nil),
			)
			actual := mustFetchNames(q)
			So(actual, ShouldResemble, []string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t%20t/results/r/artifacts/a",
			})
		})

		Convey(`Does not fetch artifacts of other invocations`, func() {
			testutil.MustApply(ctx,
				insert.Invocation("inv0", pb.Invocation_ACTIVE, nil),
				insert.Invocation("inv2", pb.Invocation_ACTIVE, nil),
				insert.Artifact("inv0", "", "a", nil),
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv2", "", "a", nil),
			)
			actual := mustFetchNames(q)
			So(actual, ShouldResemble, []string{"invocations/inv1/artifacts/a"})
		})

		Convey(`Test ID regexp`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t00/r", "a", nil),
				insert.Artifact("inv1", "tr/t10/r", "a", nil),
				insert.Artifact("inv1", "tr/t11/r", "a", nil),
				insert.Artifact("inv1", "tr/t20/r", "a", nil),
			)
			q.TestResultPredicate.TestIdRegexp = "t1."
			actual := mustFetchNames(q)
			So(actual, ShouldResemble, []string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t10/results/r/artifacts/a",
				"invocations/inv1/tests/t11/results/r/artifacts/a",
			})
		})

		Convey(`Follow edges`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a0", nil),
				insert.Artifact("inv1", "", "a1", nil),
				insert.Artifact("inv1", "tr/t/r", "a0", nil),
				insert.Artifact("inv1", "tr/t/r", "a1", nil),
			)

			Convey(`Unspecified`, func() {
				actual := mustFetchNames(q)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/artifacts/a0",
					"invocations/inv1/artifacts/a1",
					"invocations/inv1/tests/t/results/r/artifacts/a0",
					"invocations/inv1/tests/t/results/r/artifacts/a1",
				})
			})

			Convey(`Only invocations`, func() {
				q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{
					IncludedInvocations: true,
				}
				actual := mustFetchNames(q)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/artifacts/a0",
					"invocations/inv1/artifacts/a1",
				})

				Convey(`Test result predicate is ignored`, func() {
					q.TestResultPredicate.TestIdRegexp = "t."
					actual := mustFetchNames(q)
					So(actual, ShouldResemble, []string{
						"invocations/inv1/artifacts/a0",
						"invocations/inv1/artifacts/a1",
					})
				})
			})

			Convey(`Only test results`, func() {
				q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{
					TestResults: true,
				}
				actual := mustFetchNames(q)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/tests/t/results/r/artifacts/a0",
					"invocations/inv1/tests/t/results/r/artifacts/a1",
				})
			})

			Convey(`Both`, func() {
				q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{
					IncludedInvocations: true,
					TestResults:         true,
				}
				actual := mustFetchNames(q)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/artifacts/a0",
					"invocations/inv1/artifacts/a1",
					"invocations/inv1/tests/t/results/r/artifacts/a0",
					"invocations/inv1/tests/t/results/r/artifacts/a1",
				})
			})
		})

		Convey(`Artifacts of interesting test results`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t0/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/1", "a", nil),
				insert.Artifact("inv1", "tr/t1/1", "b", nil),
				insert.Artifact("inv1", "tr/t2/0", "a", nil),
			)
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv1", "t0", nil, pb.TestStatus_PASS),
				insert.TestResults("inv1", "t1", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestResults("inv1", "t2", nil, pb.TestStatus_FAIL),
			)...)

			q.TestResultPredicate.Expectancy = pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS
			actual := mustFetchNames(q)
			So(actual, ShouldResemble, []string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t1/results/0/artifacts/a",
				"invocations/inv1/tests/t1/results/1/artifacts/a",
				"invocations/inv1/tests/t1/results/1/artifacts/b",
				"invocations/inv1/tests/t2/results/0/artifacts/a",
			})

			Convey(`Without invocation artifacts`, func() {
				q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{TestResults: true}
				actual := mustFetchNames(q)
				So(actual, ShouldNotContain, "invocations/inv1/artifacts/a")
			})
		})

		Convey(`Artifacts of unexpected test results`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t0/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/1", "a", nil),
				insert.Artifact("inv1", "tr/t1/1", "b", nil),
				insert.Artifact("inv1", "tr/t2/0", "a", nil),
			)
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv1", "t0", nil, pb.TestStatus_PASS),
				insert.TestResults("inv1", "t1", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestResults("inv1", "t2", nil, pb.TestStatus_FAIL),
			)...)

			q.TestResultPredicate.Expectancy = pb.TestResultPredicate_VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS
			actual := mustFetchNames(q)
			So(actual, ShouldResemble, []string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t2/results/0/artifacts/a",
			})

		})

		Convey(`Variant equals`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t0/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/0", "b", nil),
				insert.Artifact("inv1", "tr/t2/0", "a", nil),
			)
			v1 := pbutil.Variant("k", "1")
			v2 := pbutil.Variant("k", "2")
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv1", "t0", v1, pb.TestStatus_PASS),
				insert.TestResults("inv1", "t1", v2, pb.TestStatus_PASS),
				insert.TestResults("inv1", "t2", v1, pb.TestStatus_PASS),
			)...)

			q.TestResultPredicate.Variant = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Equals{Equals: v2},
			}
			So(mustFetchNames(q), ShouldResemble, []string{
				"invocations/inv1/artifacts/a",
				"invocations/inv1/tests/t1/results/0/artifacts/a",
				"invocations/inv1/tests/t1/results/0/artifacts/b",
			})

			Convey(`Without invocation artifacts`, func() {
				q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{TestResults: true}
				actual := mustFetchNames(q)
				So(actual, ShouldNotContain, "invocations/inv1/artifacts/a")
			})
		})

		Convey(`Variant contains`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a", nil),
				insert.Artifact("inv1", "tr/t0/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/0", "a", nil),
				insert.Artifact("inv1", "tr/t1/0", "b", nil),
				insert.Artifact("inv1", "tr/t2/0", "a", nil),
			)
			v00 := pbutil.Variant("k0", "0")
			v01 := pbutil.Variant("k0", "0", "k1", "1")
			v10 := pbutil.Variant("k0", "1")
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv1", "t0", v00, pb.TestStatus_PASS),
				insert.TestResults("inv1", "t1", v01, pb.TestStatus_PASS),
				insert.TestResults("inv1", "t2", v10, pb.TestStatus_PASS),
			)...)

			Convey(`Empty`, func() {
				q.TestResultPredicate.Variant = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: pbutil.Variant()},
				}
				So(mustFetchNames(q), ShouldResemble, []string{
					"invocations/inv1/artifacts/a",
					"invocations/inv1/tests/t0/results/0/artifacts/a",
					"invocations/inv1/tests/t1/results/0/artifacts/a",
					"invocations/inv1/tests/t1/results/0/artifacts/b",
					"invocations/inv1/tests/t2/results/0/artifacts/a",
				})

				Convey(`Without invocation artifacts`, func() {
					q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{TestResults: true}
					actual := mustFetchNames(q)
					So(actual, ShouldNotContain, "invocations/inv1/artifacts/a")
				})
			})

			Convey(`Non-empty`, func() {
				q.TestResultPredicate.Variant = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: v00},
				}
				So(mustFetchNames(q), ShouldResemble, []string{
					"invocations/inv1/artifacts/a",
					"invocations/inv1/tests/t0/results/0/artifacts/a",
					"invocations/inv1/tests/t1/results/0/artifacts/a",
					"invocations/inv1/tests/t1/results/0/artifacts/b",
				})

				Convey(`Without invocation artifacts`, func() {
					q.FollowEdges = &pb.ArtifactPredicate_EdgeTypeSet{TestResults: true}
					actual := mustFetchNames(q)
					So(actual, ShouldNotContain, "invocations/inv1/artifacts/a")
				})
			})
		})

		Convey(`Paging`, func() {
			testutil.MustApply(ctx,
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
				So(actualArtifactIDs, ShouldResemble, expectedArtifactIDs)
				return token
			}

			Convey(`All results`, func() {
				token := mustReadPage("", 10, "a0", "a1", "a2", "a3", "a4")
				So(token, ShouldEqual, "")
			})

			Convey(`With pagination`, func() {
				token := mustReadPage("", 1, "a0")
				So(token, ShouldNotEqual, "")

				token = mustReadPage(token, 2, "a1", "a2")
				So(token, ShouldNotEqual, "")

				token = mustReadPage(token, 3, "a3", "a4")
				So(token, ShouldEqual, "")
			})

			Convey(`Bad token`, func() {
				q.PageToken = "CgVoZWxsbw=="
				_, _, err := q.FetchProtos(span.Single(ctx))
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
			})
		})

		Convey(`ContentTypes`, func() {
			Convey(`Works`, func() {
				testutil.MustApply(ctx,
					insert.Artifact("inv1", "", "a0", map[string]any{"ContentType": "text/plain; encoding=utf-8"}),
					insert.Artifact("inv1", "tr/t/r", "a0", map[string]any{"ContentType": "text/plain"}),
					insert.Artifact("inv1", "tr/t/r", "a1", nil),
					insert.Artifact("inv1", "tr/t/r", "a3", map[string]any{"ContentType": "image/jpg"}),
				)
				q.ContentTypeRegexp = "text/.+"

				actual := mustFetchNames(q)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/artifacts/a0",
					"invocations/inv1/tests/t/results/r/artifacts/a0",
				})
			})

			Convey(`Filter generated conditionally`, func() {
				q.ContentTypeRegexp = ""
				st, err := q.genStmt(ctx)
				So(err, ShouldBeNil)
				So(st.SQL, ShouldNotContainSubstring, "@contentTypeRegexp")
			})
		})

		Convey(`ArtifactIds`, func() {
			Convey(`Works`, func() {
				testutil.MustApply(ctx,
					insert.Artifact("inv1", "", "a0", nil),
					insert.Artifact("inv1", "tr/t/r", "a0", nil),
					insert.Artifact("inv1", "tr/t/r", "a1", nil),
					insert.Artifact("inv1", "tr/t/r", "a3", nil),
				)
				q.ArtifactIDRegexp = "a0"

				actual := mustFetchNames(q)
				So(actual, ShouldResemble, []string{
					"invocations/inv1/artifacts/a0",
					"invocations/inv1/tests/t/results/r/artifacts/a0",
				})
			})

			Convey(`Filter generated conditionally`, func() {
				q.ArtifactIDRegexp = ""
				st, err := q.genStmt(ctx)
				So(err, ShouldBeNil)
				So(st.SQL, ShouldNotContainSubstring, "@artifactIdRegexp")
			})
		})

		Convey(`WithRBECASHash`, func() {
			testutil.MustApply(ctx,
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
			So(err, ShouldBeNil)
			So(actual, ShouldHaveLength, 1)
			So(actual[0].RBECASHash, ShouldEqual, "deadbeef")
		})

		Convey(`WithGcsURI`, func() {
			testutil.MustApply(ctx,
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
			So(err, ShouldBeNil)
			So(actual, ShouldHaveLength, 1)
			So(actual[0].GcsUri, ShouldEqual, "gs://bucket/beyondbeef")
		})

		Convey(`Populates HasLines field correctly`, func() {
			testutil.MustApply(ctx,
				insert.Artifact("inv1", "", "a", map[string]any{
					"ContentType": "image/jpeg",
					"Size":        64,
				}),
			)
			actual, _ := mustFetch(q)
			So(actual, ShouldHaveLength, 1)
			So(actual[0].ContentType, ShouldEqual, "image/jpeg")
			So(actual[0].SizeBytes, ShouldEqual, 64)
			So(actual[0].HasLines, ShouldBeFalse)
		})
	})
}
